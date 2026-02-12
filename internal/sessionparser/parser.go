package sessionparser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClaudeCodeParser implements SessionProvider for Claude Code JSONL session files.
// Claude Code stores session data as JSONL under ~/.claude/projects/.
type ClaudeCodeParser struct {
	// sessionDir is the base directory to scan (default: ~/.claude/projects/).
	sessionDir string

	// maxAge limits initial discovery to sessions modified within this duration.
	maxAge time.Duration
}

// NewClaudeCodeParser creates a parser that discovers sessions under sessionDir.
// If sessionDir is empty, it defaults to ~/.claude/projects/.
// maxAge controls how far back to look during Discover (default: 24h).
func NewClaudeCodeParser(sessionDir string, maxAge time.Duration) *ClaudeCodeParser {
	if sessionDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		sessionDir = filepath.Join(home, ".claude", "projects")
	}
	if maxAge == 0 {
		maxAge = 24 * time.Hour
	}
	return &ClaudeCodeParser{
		sessionDir: sessionDir,
		maxAge:     maxAge,
	}
}

// Name returns "claude-code".
func (p *ClaudeCodeParser) Name() string { return "claude-code" }

// Discover scans sessionDir recursively for *.jsonl files modified within maxAge.
func (p *ClaudeCodeParser) Discover(ctx context.Context) ([]SessionFile, error) {
	cutoff := time.Now().Add(-p.maxAge)
	var files []SessionFile

	err := filepath.WalkDir(p.sessionDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Directory might not exist yet -- that's fine.
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		if info.ModTime().Before(cutoff) {
			return nil
		}

		sessionID := sessionIDFromPath(path)
		files = append(files, SessionFile{
			Path:      path,
			SessionID: sessionID,
			Provider:  "claude-code",
		})

		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return files, fmt.Errorf("discover sessions in %s: %w", p.sessionDir, err)
	}

	return files, nil
}

// WatchForNew is implemented in discovery.go.
// It uses fsnotify to watch for new session files.
func (p *ClaudeCodeParser) WatchForNew(ctx context.Context, found chan<- SessionFile) error {
	return watchForNewSessions(ctx, p.sessionDir, found)
}

// ParseLine parses a single JSONL line from a Claude Code session file.
// It extracts tool_use events (Write, Read, Bash, etc.) and returns
// a SessionEvent for each. Returns nil if the line is not a tool_use event.
//
// Claude Code JSONL format (observed, no stability contract):
//
//	{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"...","content":"..."}}]}}
//
// The parser is defensive: unknown fields are ignored, missing fields produce
// nil (skip). A warning is logged on first unrecognized format.
func (p *ClaudeCodeParser) ParseLine(line []byte) (*SessionEvent, error) {
	line = trimLine(line)
	if len(line) == 0 {
		return nil, nil
	}

	// Fast path: skip lines that clearly aren't tool_use.
	if !containsToolUse(line) {
		return nil, nil
	}

	var envelope jsonlEnvelope
	if err := json.Unmarshal(line, &envelope); err != nil {
		// Malformed JSON -- log and skip.
		// skip malformed line silently
		return nil, nil
	}

	return extractToolUse(&envelope, string(line))
}

// --- internal types for JSON parsing ---

// jsonlEnvelope is the top-level structure of a Claude Code JSONL line.
// We use a flexible structure to handle format variations.
type jsonlEnvelope struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message,omitempty"`

	// Some formats put content directly at top level.
	Content json.RawMessage `json:"content,omitempty"`
}

type messageWrapper struct {
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type  string          `json:"type"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type writeInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

type editInput struct {
	FilePath string `json:"file_path"`
}

type readInput struct {
	FilePath string `json:"file_path"`
}

type bashInput struct {
	Command string `json:"command"`
}

// --- extraction logic ---

func extractToolUse(env *jsonlEnvelope, rawJSON string) (*SessionEvent, error) {
	// Try message.content first (standard assistant message format).
	var blocks []contentBlock

	if len(env.Message) > 0 {
		var msg messageWrapper
		if err := json.Unmarshal(env.Message, &msg); err == nil {
			blocks = msg.Content
		}
	}

	// Fallback: content at top level.
	if len(blocks) == 0 && len(env.Content) > 0 {
		if err := json.Unmarshal(env.Content, &blocks); err != nil {
			// Not an array of content blocks -- skip.
			return nil, nil
		}
	}

	// Find the first tool_use block.
	for _, block := range blocks {
		if block.Type != "tool_use" {
			continue
		}

		event := &SessionEvent{
			EventType: "tool_use",
			ToolName:  block.Name,
			Timestamp: time.Now(),
			RawJSON:   rawJSON,
		}

		switch block.Name {
		case "Write":
			var inp writeInput
			if err := json.Unmarshal(block.Input, &inp); err != nil {
				// skip Write with bad input
				return nil, nil
			}
			event.FilePath = inp.FilePath
			event.ContentHash = hashContent(inp.Content)

		case "Edit":
			var inp editInput
			if err := json.Unmarshal(block.Input, &inp); err != nil {
				// skip Edit with bad input
				return nil, nil
			}
			event.FilePath = inp.FilePath

		case "Read":
			var inp readInput
			if err := json.Unmarshal(block.Input, &inp); err == nil {
				event.FilePath = inp.FilePath
			}

		case "Bash":
			var inp bashInput
			if err := json.Unmarshal(block.Input, &inp); err == nil {
				// Store command as file_path field for correlation.
				if len(inp.Command) > 200 {
					event.FilePath = inp.Command[:200]
				} else {
					event.FilePath = inp.Command
				}
			}

		default:
			// Unknown tool -- still record it for future use.
		}

		return event, nil
	}

	return nil, nil
}

// containsToolUse is a fast check to avoid parsing lines that definitely
// don't contain tool_use events.
func containsToolUse(line []byte) bool {
	return strings.Contains(string(line), "tool_use")
}

// hashContent computes SHA-256 of content and returns hex string.
func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// sessionIDFromPath extracts a session identifier from the file path.
// For Claude Code: ~/.claude/projects/{project-hash}/{session-id}.jsonl
// Returns "project-hash/session-id" as the session identifier.
func sessionIDFromPath(path string) string {
	dir := filepath.Dir(path)
	projectHash := filepath.Base(dir)
	sessionFile := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	return projectHash + "/" + sessionFile
}

// trimLine removes leading/trailing whitespace and BOM.
func trimLine(line []byte) []byte {
	// Strip UTF-8 BOM if present.
	if len(line) >= 3 && line[0] == 0xEF && line[1] == 0xBB && line[2] == 0xBF {
		line = line[3:]
	}
	// Trim whitespace.
	start := 0
	for start < len(line) && (line[start] == ' ' || line[start] == '\t' || line[start] == '\n' || line[start] == '\r') {
		start++
	}
	end := len(line)
	for end > start && (line[end-1] == ' ' || line[end-1] == '\t' || line[end-1] == '\n' || line[end-1] == '\r') {
		end--
	}
	return line[start:end]
}
