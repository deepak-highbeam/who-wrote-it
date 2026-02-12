package sessionparser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseLineWrite(t *testing.T) {
	p := NewClaudeCodeParser("", 24*time.Hour)

	line := []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"/tmp/test.go","content":"package main\nfunc main() {}\n"}}]}}`)

	event, err := p.ParseLine(line)
	if err != nil {
		t.Fatalf("ParseLine: %v", err)
	}
	if event == nil {
		t.Fatal("expected event, got nil")
	}

	if event.ToolName != "Write" {
		t.Errorf("ToolName = %q, want %q", event.ToolName, "Write")
	}
	if event.FilePath != "/tmp/test.go" {
		t.Errorf("FilePath = %q, want %q", event.FilePath, "/tmp/test.go")
	}
	if event.ContentHash == "" {
		t.Error("ContentHash is empty, want SHA-256 hash")
	}
	if event.EventType != "tool_use" {
		t.Errorf("EventType = %q, want %q", event.EventType, "tool_use")
	}
	// Write with 2 lines of content.
	if event.LinesChanged != 2 {
		t.Errorf("LinesChanged = %d, want 2", event.LinesChanged)
	}
	if event.DiffContent != "package main\nfunc main() {}\n" {
		t.Errorf("DiffContent = %q, want full content", event.DiffContent)
	}
}

func TestParseLineWrite_LinesChanged(t *testing.T) {
	p := NewClaudeCodeParser("", 24*time.Hour)

	// Content with 4 lines.
	line := []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"/tmp/multi.go","content":"line1\nline2\nline3\nline4"}}]}}`)

	event, err := p.ParseLine(line)
	if err != nil {
		t.Fatalf("ParseLine: %v", err)
	}
	if event == nil {
		t.Fatal("expected event, got nil")
	}

	if event.LinesChanged != 4 {
		t.Errorf("LinesChanged = %d, want 4", event.LinesChanged)
	}
}

func TestParseLineEdit(t *testing.T) {
	p := NewClaudeCodeParser("", 24*time.Hour)

	line := []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/tmp/edit.go","old_string":"old line","new_string":"new line 1\nnew line 2\nnew line 3"}}]}}`)

	event, err := p.ParseLine(line)
	if err != nil {
		t.Fatalf("ParseLine: %v", err)
	}
	if event == nil {
		t.Fatal("expected event, got nil")
	}

	if event.ToolName != "Edit" {
		t.Errorf("ToolName = %q, want %q", event.ToolName, "Edit")
	}
	if event.FilePath != "/tmp/edit.go" {
		t.Errorf("FilePath = %q, want %q", event.FilePath, "/tmp/edit.go")
	}
	// Edit new_string has 3 lines.
	if event.LinesChanged != 3 {
		t.Errorf("LinesChanged = %d, want 3", event.LinesChanged)
	}
	if event.DiffContent != "new line 1\nnew line 2\nnew line 3" {
		t.Errorf("DiffContent = %q, want new_string content", event.DiffContent)
	}
	if event.ContentHash == "" {
		t.Error("ContentHash is empty, want hash of new_string")
	}
}

func TestParseLineRead(t *testing.T) {
	p := NewClaudeCodeParser("", 24*time.Hour)

	line := []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/tmp/readme.md"}}]}}`)

	event, err := p.ParseLine(line)
	if err != nil {
		t.Fatalf("ParseLine: %v", err)
	}
	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.ToolName != "Read" {
		t.Errorf("ToolName = %q, want %q", event.ToolName, "Read")
	}
	if event.FilePath != "/tmp/readme.md" {
		t.Errorf("FilePath = %q, want %q", event.FilePath, "/tmp/readme.md")
	}
	// Read events should have no lines changed.
	if event.LinesChanged != 0 {
		t.Errorf("LinesChanged = %d, want 0 for Read", event.LinesChanged)
	}
}

func TestParseLineBash(t *testing.T) {
	p := NewClaudeCodeParser("", 24*time.Hour)

	line := []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"go build ./..."}}]}}`)

	event, err := p.ParseLine(line)
	if err != nil {
		t.Fatalf("ParseLine: %v", err)
	}
	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.ToolName != "Bash" {
		t.Errorf("ToolName = %q, want %q", event.ToolName, "Bash")
	}
	if event.FilePath != "go build ./..." {
		t.Errorf("FilePath = %q, want %q", event.FilePath, "go build ./...")
	}
}

func TestParseLineSkipsNonToolUse(t *testing.T) {
	p := NewClaudeCodeParser("", 24*time.Hour)

	cases := []struct {
		name string
		line string
	}{
		{"empty", ""},
		{"whitespace", "   "},
		{"text block", `{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}`},
		{"user message", `{"type":"user","message":{"content":"hello"}}`},
		{"random json", `{"foo":"bar"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event, err := p.ParseLine([]byte(tc.line))
			if err != nil {
				t.Fatalf("ParseLine: %v", err)
			}
			if event != nil {
				t.Errorf("expected nil event for %q, got %+v", tc.name, event)
			}
		})
	}
}

func TestParseLineMalformedJSON(t *testing.T) {
	p := NewClaudeCodeParser("", 24*time.Hour)

	// Malformed JSON should not return an error -- just skip (nil event).
	event, err := p.ParseLine([]byte(`{malformed json tool_use`))
	if err != nil {
		t.Fatalf("ParseLine should not error on malformed JSON, got: %v", err)
	}
	if event != nil {
		t.Errorf("expected nil event for malformed JSON, got %+v", event)
	}
}

func TestParseLineMissingFields(t *testing.T) {
	p := NewClaudeCodeParser("", 24*time.Hour)

	// Write without content -- should still parse (empty content hash).
	line := []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"/tmp/test.go"}}]}}`)
	event, err := p.ParseLine(line)
	if err != nil {
		t.Fatalf("ParseLine: %v", err)
	}
	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.FilePath != "/tmp/test.go" {
		t.Errorf("FilePath = %q, want %q", event.FilePath, "/tmp/test.go")
	}
	if event.LinesChanged != 0 {
		t.Errorf("LinesChanged = %d, want 0 for empty content", event.LinesChanged)
	}
}

func TestParseLineContentAtTopLevel(t *testing.T) {
	p := NewClaudeCodeParser("", 24*time.Hour)

	// Alternative format: content blocks at top level.
	line := []byte(`{"type":"assistant","content":[{"type":"tool_use","name":"Write","input":{"file_path":"/tmp/alt.go","content":"package alt"}}]}`)
	event, err := p.ParseLine(line)
	if err != nil {
		t.Fatalf("ParseLine: %v", err)
	}
	if event == nil {
		t.Fatal("expected event for top-level content, got nil")
	}
	if event.FilePath != "/tmp/alt.go" {
		t.Errorf("FilePath = %q, want %q", event.FilePath, "/tmp/alt.go")
	}
}

func TestParseLineBOM(t *testing.T) {
	p := NewClaudeCodeParser("", 24*time.Hour)

	// Line with UTF-8 BOM.
	bom := []byte{0xEF, 0xBB, 0xBF}
	line := append(bom, []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"/tmp/bom.go","content":"test"}}]}}`)...)

	event, err := p.ParseLine(line)
	if err != nil {
		t.Fatalf("ParseLine: %v", err)
	}
	if event == nil {
		t.Fatal("expected event with BOM, got nil")
	}
}

func TestHashContent(t *testing.T) {
	hash := hashContent("hello world")
	// SHA-256 of "hello world"
	expected := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if hash != expected {
		t.Errorf("hashContent = %q, want %q", hash, expected)
	}
}

func TestSessionIDFromPath(t *testing.T) {
	id := sessionIDFromPath("/home/user/.claude/projects/abc123/sess456.jsonl")
	if id != "abc123/sess456" {
		t.Errorf("sessionIDFromPath = %q, want %q", id, "abc123/sess456")
	}
}

func TestDiscover(t *testing.T) {
	// Create a temp directory with a fake session file.
	tmpDir := t.TempDir()
	projDir := filepath.Join(tmpDir, "project-hash")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}
	sessionFile := filepath.Join(projDir, "session-001.jsonl")
	if err := os.WriteFile(sessionFile, []byte(`{"type":"test"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	p := NewClaudeCodeParser(tmpDir, 24*time.Hour)
	ctx := context.Background()

	files, err := p.Discover(ctx)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Discover found %d files, want 1", len(files))
	}

	if files[0].Path != sessionFile {
		t.Errorf("Path = %q, want %q", files[0].Path, sessionFile)
	}
	if files[0].Provider != "claude-code" {
		t.Errorf("Provider = %q, want %q", files[0].Provider, "claude-code")
	}
	if files[0].SessionID != "project-hash/session-001" {
		t.Errorf("SessionID = %q, want %q", files[0].SessionID, "project-hash/session-001")
	}
}

func TestDiscoverSkipsOldFiles(t *testing.T) {
	tmpDir := t.TempDir()
	projDir := filepath.Join(tmpDir, "project-hash")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}
	sessionFile := filepath.Join(projDir, "old-session.jsonl")
	if err := os.WriteFile(sessionFile, []byte(`{"type":"test"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Set mod time to 48 hours ago.
	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(sessionFile, past, past); err != nil {
		t.Fatal(err)
	}

	p := NewClaudeCodeParser(tmpDir, 24*time.Hour)
	ctx := context.Background()

	files, err := p.Discover(ctx)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("Discover found %d files, want 0 (old file should be skipped)", len(files))
	}
}

func TestTailerBasic(t *testing.T) {
	tmpDir := t.TempDir()
	fpath := filepath.Join(tmpDir, "test.jsonl")

	// Write initial content.
	if err := os.WriteFile(fpath, []byte("line1\nline2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	tailer := NewTailer(fpath, 0, 50*time.Millisecond)
	lines := make(chan []byte, 10)

	go func() {
		_, _ = tailer.Tail(ctx, lines)
	}()

	// Read initial lines.
	var got []string
	timer := time.NewTimer(500 * time.Millisecond)
	for {
		select {
		case line := <-lines:
			got = append(got, string(line))
			if len(got) == 2 {
				goto done
			}
		case <-timer.C:
			goto done
		}
	}
done:

	if len(got) != 2 {
		t.Fatalf("got %d lines, want 2: %v", len(got), got)
	}
	if got[0] != "line1" || got[1] != "line2" {
		t.Errorf("got %v, want [line1, line2]", got)
	}

	// Append a new line and verify the tailer picks it up.
	f, err := os.OpenFile(fpath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("line3\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	timer2 := time.NewTimer(1 * time.Second)
	select {
	case line := <-lines:
		if string(line) != "line3" {
			t.Errorf("appended line = %q, want %q", string(line), "line3")
		}
	case <-timer2.C:
		t.Error("timed out waiting for appended line")
	}

	cancel()
}

// ---------------------------------------------------------------------------
// ExtractDiffContent tests
// ---------------------------------------------------------------------------

func TestExtractDiffContent_WriteEvent(t *testing.T) {
	rawJSON := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"/tmp/test.go","content":"package main\nfunc main() {}\n"}}]}}`

	content := ExtractDiffContent(rawJSON)
	if content != "package main\nfunc main() {}\n" {
		t.Errorf("ExtractDiffContent(Write) = %q, want content", content)
	}
}

func TestExtractDiffContent_EditEvent(t *testing.T) {
	rawJSON := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/tmp/test.go","old_string":"old","new_string":"new line 1\nnew line 2"}}]}}`

	content := ExtractDiffContent(rawJSON)
	if content != "new line 1\nnew line 2" {
		t.Errorf("ExtractDiffContent(Edit) = %q, want new_string", content)
	}
}

func TestExtractDiffContent_EditStripsContextLines(t *testing.T) {
	// Edit where new_string contains context lines from old_string.
	// Only genuinely new lines should be returned.
	rawJSON := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/tmp/test.kt","old_string":"  ) : RestEndpoint<Nothing, Rep>()\n\n  @Method(\"GET\")\n  @Path(\"/items\")\n  data class Search(","new_string":"  ) : RestEndpoint<Nothing, Rep>()\n\n  @Method(\"GET\")\n  @Path(\"/items/:id/details\")\n  data class GetDetails(\n    @PathParam val id: KairoId,\n  ) : RestEndpoint<Nothing, Rep>()\n\n  @Method(\"GET\")\n  @Path(\"/items\")\n  data class Search("}}]}}`

	content := ExtractDiffContent(rawJSON)

	// Should only contain the new lines, not the context lines that existed in old_string.
	if strings.Contains(content, "data class Search(") {
		t.Errorf("ExtractDiffContent(Edit) should not include context line 'data class Search(', got %q", content)
	}
	if !strings.Contains(content, "@Path(\"/items/:id/details\")") {
		t.Errorf("ExtractDiffContent(Edit) should include new line '@Path(\"/items/:id/details\")', got %q", content)
	}
	if !strings.Contains(content, "data class GetDetails(") {
		t.Errorf("ExtractDiffContent(Edit) should include new line 'data class GetDetails(', got %q", content)
	}
	if !strings.Contains(content, "@PathParam val id: KairoId,") {
		t.Errorf("ExtractDiffContent(Edit) should include new line '@PathParam val id: KairoId,', got %q", content)
	}
}

func TestEditOnlyNewLines(t *testing.T) {
	tests := []struct {
		name   string
		old    string
		new    string
		want   string
	}{
		{
			name: "empty old returns full new",
			old:  "",
			new:  "line1\nline2",
			want: "line1\nline2",
		},
		{
			name: "no overlap returns full new",
			old:  "old1\nold2",
			new:  "new1\nnew2",
			want: "new1\nnew2",
		},
		{
			name: "full overlap returns empty",
			old:  "same1\nsame2",
			new:  "same1\nsame2",
			want: "",
		},
		{
			name: "partial overlap strips context",
			old:  "context1\ncontext2",
			new:  "context1\nnew line\ncontext2",
			want: "new line",
		},
		{
			name: "duplicate lines consumed correctly",
			old:  "dup\ndup",
			new:  "dup\nnew\ndup\ndup",
			want: "new\ndup",
		},
		{
			name: "indentation differences still match",
			old:  "  indented",
			new:  "    indented\nnew line",
			want: "new line",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := editOnlyNewLines(tt.old, tt.new)
			if got != tt.want {
				t.Errorf("editOnlyNewLines(%q, %q) = %q, want %q", tt.old, tt.new, got, tt.want)
			}
		})
	}
}

func TestExtractDiffContent_NonWriteEvent(t *testing.T) {
	rawJSON := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/tmp/test.go"}}]}}`

	content := ExtractDiffContent(rawJSON)
	if content != "" {
		t.Errorf("ExtractDiffContent(Read) = %q, want empty", content)
	}
}

func TestExtractDiffContent_EmptyInput(t *testing.T) {
	content := ExtractDiffContent("")
	if content != "" {
		t.Errorf("ExtractDiffContent('') = %q, want empty", content)
	}
}

func TestCountLines(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"single line", 1},
		{"line1\nline2", 2},
		{"line1\nline2\nline3", 3},
		{"line1\nline2\n", 2},
	}

	for _, tt := range tests {
		got := countLines(tt.input)
		if got != tt.want {
			t.Errorf("countLines(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
