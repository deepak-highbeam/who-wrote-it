// Package sessionparser provides AI tool session file discovery, tailing,
// and event extraction. The SessionProvider interface allows different AI
// tools (Claude Code, Copilot, Cursor) to be handled uniformly.
package sessionparser

import (
	"context"
	"time"
)

// SessionProvider is the abstraction layer for AI tool session parsing.
// Each AI coding tool (Claude Code, Copilot, Cursor, etc.) implements this
// interface to discover session files, watch for new ones, and parse lines.
type SessionProvider interface {
	// Name returns the provider identifier (e.g. "claude-code", "copilot").
	Name() string

	// Discover scans known locations for existing session files.
	Discover(ctx context.Context) ([]SessionFile, error)

	// WatchForNew monitors for newly created session files and sends them
	// on the found channel. It blocks until ctx is cancelled.
	WatchForNew(ctx context.Context, found chan<- SessionFile) error

	// ParseLine parses a single line from a session file and returns
	// a SessionEvent if the line contains a tool_use event, or nil if
	// the line should be skipped (not a tool_use, malformed, etc.).
	ParseLine(line []byte) (*SessionEvent, error)
}

// SessionFile represents a discovered AI tool session file.
type SessionFile struct {
	Path      string // Absolute path to the session file.
	SessionID string // Unique session identifier (derived from filename/path).
	Provider  string // Provider name (e.g. "claude-code").
}

// SessionEvent represents a parsed tool_use event from a session file.
type SessionEvent struct {
	SessionID    string    // Session that produced this event.
	EventType    string    // "tool_use"
	ToolName     string    // "Write", "Read", "Bash", etc.
	FilePath     string    // File path affected (for Write/Read events).
	ContentHash  string    // SHA-256 hash of written content (for Write events).
	Timestamp    time.Time // When the event occurred (or was parsed).
	RawJSON      string    // Original JSON line for debugging/reprocessing.
	LinesChanged int       // Number of lines written/edited (0 for non-Write/Edit tools).
	DiffContent  string    // Written/edited content for work type classification.
}
