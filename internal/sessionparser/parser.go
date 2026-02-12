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
	FilePath  string `json:"file_path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
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
			event.LinesChanged = countLines(inp.Content)
			event.DiffContent = inp.Content

		case "Edit":
			var inp editInput
			if err := json.Unmarshal(block.Input, &inp); err != nil {
				// skip Edit with bad input
				return nil, nil
			}
			event.FilePath = inp.FilePath
			newOnly := editOnlyNewLines(inp.OldString, inp.NewString)
			event.ContentHash = hashContent(newOnly)
			event.LinesChanged = countLines(newOnly)
			event.DiffContent = newOnly

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

// countLines returns the number of lines in s. Returns 0 for empty string.
// A trailing newline does not count as an extra line (standard for file content).
func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// ExtractDiffContent parses a raw JSONL line and returns the Write content
// or Edit new_string. Used by the daemon at attribution time to feed the
// work type classifier.
func ExtractDiffContent(rawJSON string) string {
	line := trimLine([]byte(rawJSON))
	if len(line) == 0 || !containsToolUse(line) {
		return ""
	}

	var envelope jsonlEnvelope
	if err := json.Unmarshal(line, &envelope); err != nil {
		return ""
	}

	var blocks []contentBlock
	if len(envelope.Message) > 0 {
		var msg messageWrapper
		if err := json.Unmarshal(envelope.Message, &msg); err == nil {
			blocks = msg.Content
		}
	}
	if len(blocks) == 0 && len(envelope.Content) > 0 {
		_ = json.Unmarshal(envelope.Content, &blocks)
	}

	for _, block := range blocks {
		if block.Type != "tool_use" {
			continue
		}
		switch block.Name {
		case "Write":
			var inp writeInput
			if err := json.Unmarshal(block.Input, &inp); err == nil {
				return inp.Content
			}
		case "Edit":
			var inp editInput
			if err := json.Unmarshal(block.Input, &inp); err == nil {
				return editOnlyNewLines(inp.OldString, inp.NewString)
			}
		}
	}
	return ""
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

// editOnlyNewLines returns only the lines in newStr that are genuinely new
// (not present in oldStr). This prevents Edit context lines from being
// counted as AI-authored. Uses trimmed-string comparison (matching the
// approach in metrics/linecalc.go) so indentation changes don't cause
// false negatives.
func editOnlyNewLines(oldStr, newStr string) string {
	if oldStr == "" {
		return newStr
	}

	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")

	// Build frequency map of trimmed old lines.
	oldFreq := make(map[string]int)
	for _, line := range oldLines {
		trimmed := strings.TrimSpace(line)
		oldFreq[trimmed]++
	}

	// Collect lines from newStr that don't appear in oldStr.
	var result []string
	for _, line := range newLines {
		trimmed := strings.TrimSpace(line)
		if oldFreq[trimmed] > 0 {
			oldFreq[trimmed]--
		} else {
			result = append(result, line)
		}
	}

	if len(result) == 0 {
		return ""
	}
	return strings.Join(result, "\n")
}
