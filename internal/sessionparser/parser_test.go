package sessionparser

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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

func TestDiffLineCount(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
		want int
	}{
		{"identical", "a\nb\nc", "a\nb\nc", 0},
		{"one line changed", "a\nb\nc", "a\nB\nc", 1},
		{"all lines changed", "a\nb", "x\ny", 2},
		{"line added", "a\nb", "a\nb\nc", 1},
		{"line removed", "a\nb\nc", "a\nb", 1},
		{"trailing newline diff", "a\nb\n", "a\nb", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffLineCount(tt.old, tt.new)
			if got != tt.want {
				t.Errorf("diffLineCount = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWriteDiffContent(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
		want string
	}{
		{"identical", "a\nb\nc", "a\nb\nc", ""},
		{"one line changed", "a\nb\nc", "a\nB\nc", "B"},
		{"line added", "a\nb", "a\nb\nc", "c"},
		{"multiple changes", "a\nb\nc", "a\nX\nY", "X\nY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := writeDiffContent(tt.old, tt.new)
			if got != tt.want {
				t.Errorf("writeDiffContent = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriteDiffIntegration(t *testing.T) {
	p := NewClaudeCodeParser("", 24*time.Hour)

	// First write: 4 lines, no prior content → all lines counted.
	line1 := []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"/tmp/diff.xml","content":"<root>\n  <a>1</a>\n  <b>2</b>\n</root>"}}]}}`)
	ev1, err := p.ParseLine(line1)
	if err != nil {
		t.Fatal(err)
	}
	if ev1.LinesChanged != 4 {
		t.Errorf("first write LinesChanged = %d, want 4", ev1.LinesChanged)
	}

	// Second write: change one line → should count 1 changed line.
	line2 := []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"/tmp/diff.xml","content":"<root>\n  <a>CHANGED</a>\n  <b>2</b>\n</root>"}}]}}`)
	ev2, err := p.ParseLine(line2)
	if err != nil {
		t.Fatal(err)
	}
	if ev2.LinesChanged != 1 {
		t.Errorf("second write LinesChanged = %d, want 1", ev2.LinesChanged)
	}

	// Third write: undo (identical to first) → should count 1 changed line.
	line3 := []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"/tmp/diff.xml","content":"<root>\n  <a>1</a>\n  <b>2</b>\n</root>"}}]}}`)
	ev3, err := p.ParseLine(line3)
	if err != nil {
		t.Fatal(err)
	}
	if ev3.LinesChanged != 1 {
		t.Errorf("undo write LinesChanged = %d, want 1", ev3.LinesChanged)
	}
}

func TestReadSeedsWriteDiff(t *testing.T) {
	// Read event seeds the content cache from git, enabling the
	// subsequent Write to compute an accurate diff.
	tmpDir := t.TempDir()
	original := "line1\nline2\nline3\nline4\nline5\nline6\nline7\n"
	filePath := initGitRepo(t, tmpDir, "test.txt", original)

	p := NewClaudeCodeParser("", 24*time.Hour)

	// Step 1: Claude reads the file → seeds cache from git.
	readLine := []byte(fmt.Sprintf(
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"%s"}}]}}`,
		filePath))
	_, err := p.ParseLine(readLine)
	if err != nil {
		t.Fatal(err)
	}

	// Step 2: Claude writes with one line changed.
	modified := "line1\nline2\nCHANGED\nline4\nline5\nline6\nline7\n"
	writeLine := []byte(fmt.Sprintf(
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"%s","content":"%s"}}]}}`,
		filePath, strings.ReplaceAll(modified, "\n", `\n`)))

	ev, err := p.ParseLine(writeLine)
	if err != nil {
		t.Fatal(err)
	}
	if ev == nil {
		t.Fatal("expected event, got nil")
	}

	// Should detect only 1 changed line, not 7 (total lines).
	if ev.LinesChanged != 1 {
		t.Errorf("Read-then-Write: LinesChanged = %d, want 1", ev.LinesChanged)
	}
}

// initGitRepo creates a git repo in dir, adds and commits a file.
// Returns the absolute path to the committed file.
func initGitRepo(t *testing.T, dir, fileName, content string) string {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s\n%s", args, err, out)
		}
	}

	filePath := filepath.Join(dir, fileName)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"git", "add", fileName},
		{"git", "commit", "-m", "initial"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s\n%s", args, err, out)
		}
	}
	return filePath
}

func TestWriteLinesChanged_RealisticTiming(t *testing.T) {
	// REALISTIC scenario:
	//
	// In real Claude Code usage:
	//   1. Claude API returns assistant message with Write tool_use
	//   2. Claude Code logs the message to JSONL
	//   3. Claude Code EXECUTES the Write tool (file on disk now has new content)
	//   4. The session tailer reads the JSONL line and calls ParseLine
	//
	// By step 4, the file on disk already has the NEW content (same as
	// inp.Content). The parser must use git to get the committed baseline.

	tmpDir := t.TempDir()

	// Original 33-line XML file committed in git.
	original := strings.Join([]string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<Configuration status="WARN">`,
		`  <Appenders>`,
		`    <Console name="Console" target="SYSTEM_OUT">`,
		`      <PatternLayout>`,
		`        <Pattern>%d{HH:mm:ss.SSS} [%t] %-5level %logger{36} - %msg%n</Pattern>`,
		`      </PatternLayout>`,
		`    </Console>`,
		`    <RollingFile name="File" fileName="logs/app.log"`,
		`                 filePattern="logs/app-%d{yyyy-MM-dd}-%i.log">`,
		`      <PatternLayout>`,
		`        <Pattern>%d{yyyy-MM-dd HH:mm:ss.SSS} [%t] %-5level %logger{36} - %msg%n</Pattern>`,
		`      </PatternLayout>`,
		`      <Policies>`,
		`        <TimeBasedTriggeringPolicy/>`,
		`        <SizeBasedTriggeringPolicy size="10 MB"/>`,
		`      </Policies>`,
		`      <DefaultRolloverStrategy max="10"/>`,
		`    </RollingFile>`,
		`  </Appenders>`,
		`  <Loggers>`,
		`    <Logger name="com.example" level="DEBUG" additivity="false">`,
		`      <AppenderRef ref="Console"/>`,
		`      <AppenderRef ref="File"/>`,
		`    </Logger>`,
		`    <Logger name="org.hibernate" level="WARN" additivity="false">`,
		`      <AppenderRef ref="Console"/>`,
		`    </Logger>`,
		`    <Root level="INFO">`,
		`      <AppenderRef ref="Console"/>`,
		`      <AppenderRef ref="File"/>`,
		`    </Root>`,
		`  </Loggers>`,
		`</Configuration>`,
	}, "\n") + "\n"

	filePath := initGitRepo(t, tmpDir, "log4j2-test.xml", original)

	// Modified version: only 1 line changed (DEBUG → TRACE on line 22).
	modified := strings.Replace(original,
		`<Logger name="com.example" level="DEBUG" additivity="false">`,
		`<Logger name="com.example" level="TRACE" additivity="false">`, 1)

	if original == modified {
		t.Fatal("test setup: original and modified should differ")
	}

	// Simulate realistic timing: Write tool has ALREADY executed.
	// File on disk has the NEW content.
	if err := os.WriteFile(filePath, []byte(modified), 0644); err != nil {
		t.Fatal(err)
	}

	p := NewClaudeCodeParser("", 24*time.Hour)

	// Write event with NO preceding Read event.
	writeLine := []byte(fmt.Sprintf(
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"%s","content":"%s"}}]}}`,
		filePath, strings.ReplaceAll(strings.ReplaceAll(modified, `"`, `\"`), "\n", `\n`)))

	ev, err := p.ParseLine(writeLine)
	if err != nil {
		t.Fatal(err)
	}
	if ev == nil {
		t.Fatal("expected event, got nil")
	}

	if ev.LinesChanged != 1 {
		t.Errorf("LinesChanged = %d, want 1", ev.LinesChanged)
	}
}

func TestWriteLinesChanged_ReadThenWrite_DiskAlreadyUpdated(t *testing.T) {
	// The tailer is behind and processes both Read + Write after the
	// Write tool has already executed. File on disk has the NEW content
	// when we process the Read event. Git provides the correct baseline.

	tmpDir := t.TempDir()

	original := "line1\nline2\nline3\nline4\nline5\nline6\nline7\n"
	modified := "line1\nline2\nCHANGED\nline4\nline5\nline6\nline7\n"

	filePath := initGitRepo(t, tmpDir, "config.xml", original)

	// Write has already executed — disk has modified content.
	if err := os.WriteFile(filePath, []byte(modified), 0644); err != nil {
		t.Fatal(err)
	}

	p := NewClaudeCodeParser("", 24*time.Hour)

	// Process Read event — file on disk already has new content,
	// but git still has the original.
	readLine := []byte(fmt.Sprintf(
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"%s"}}]}}`,
		filePath))
	_, err := p.ParseLine(readLine)
	if err != nil {
		t.Fatal(err)
	}

	// Process Write event.
	writeLine := []byte(fmt.Sprintf(
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"%s","content":"%s"}}]}}`,
		filePath, strings.ReplaceAll(modified, "\n", `\n`)))

	ev, err := p.ParseLine(writeLine)
	if err != nil {
		t.Fatal(err)
	}
	if ev == nil {
		t.Fatal("expected event, got nil")
	}

	if ev.LinesChanged != 1 {
		t.Errorf("LinesChanged = %d, want 1", ev.LinesChanged)
	}
}
