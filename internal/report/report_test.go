package report

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/anthropic/who-wrote-it/internal/store"
)

var baseTime = time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC)

// setupTestStore creates a temporary SQLite store, a git-initialized project
// directory, and returns the store, project dir, and cleanup function.
func setupTestStore(t *testing.T) (*store.Store, string, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "report-test-*")
	if err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(dir, "test.db")
	s, err := store.New(dbPath)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatal(err)
	}

	// Create a project dir and initialize a git repo.
	projDir := filepath.Join(dir, "proj")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		s.Close()
		os.RemoveAll(dir)
		t.Fatal(err)
	}

	// Init git repo so git diff works.
	gitInit(t, projDir)

	cleanup := func() {
		s.Close()
		os.RemoveAll(dir)
	}

	return s, projDir, cleanup
}

// gitInit initializes a git repo and makes an initial empty commit.
func gitInit(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init command %v failed: %v\n%s", args, err, out)
		}
	}
}

// gitAdd stages and commits files in the project dir with a specific date.
func gitAdd(t *testing.T, dir string, files []string, message string) {
	t.Helper()
	gitAddAt(t, dir, files, message, commitTime)
}

// gitAddAt stages and commits files with a specific date.
func gitAddAt(t *testing.T, dir string, files []string, message string, when time.Time) {
	t.Helper()
	args := append([]string{"add"}, files...)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	dateStr := when.Format(time.RFC3339)
	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE="+dateStr,
		"GIT_COMMITTER_DATE="+dateStr,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}
}

// commitTime is well before baseTime so git log --before=baseTime finds these commits.
var commitTime = baseTime.Add(-24 * time.Hour)

// writeFile writes content to a file in the project directory.
func writeFile(t *testing.T, projDir, name, content string) {
	t.Helper()
	path := filepath.Join(projDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// makeWriteRawJSON creates a raw JSON line simulating a Claude Write tool_use.
func makeWriteRawJSON(filePath, content string) string {
	return fmt.Sprintf(
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":%q,"content":%s}}]}}`,
		filePath, mustJSON(content),
	)
}

// mustJSON marshals a value and returns the raw string.
func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// insertAttribution inserts a test attribution with work type into the store.
func insertAttribution(t *testing.T, s *store.Store, filePath, projectPath, level, workType string, ts time.Time, linesChanged int) {
	t.Helper()

	if err := s.InsertFileEvent(projectPath, filePath, "write", ts); err != nil {
		t.Fatalf("insert file event: %v", err)
	}

	attr := store.AttributionRecord{
		FilePath:            filePath,
		ProjectPath:         projectPath,
		AuthorshipLevel:     level,
		Confidence:          0.95,
		FirstAuthor:         "ai",
		CorrelationWindowMs: 100,
		Timestamp:           ts,
		LinesChanged:        linesChanged,
	}

	id, err := s.InsertAttribution(attr)
	if err != nil {
		t.Fatalf("insert attribution: %v", err)
	}

	if err := s.UpdateAttributionWorkType(id, workType); err != nil {
		t.Fatalf("update work type: %v", err)
	}
}

// insertSessionEvent inserts a session event with raw JSON into the store.
func insertSessionEvent(t *testing.T, s *store.Store, sessionID, filePath, rawJSON string, ts time.Time) {
	t.Helper()
	if err := s.InsertSessionEvent(sessionID, "tool_use", "Write", filePath, "", ts, rawJSON, 0); err != nil {
		t.Fatalf("insert session event: %v", err)
	}
}

func almostEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// ---------------------------------------------------------------------------
// GenerateProject tests
// ---------------------------------------------------------------------------

func TestGenerateProjectFromStore_NewFiles(t *testing.T) {
	s, projDir, cleanup := setupTestStore(t)
	defer cleanup()

	// main.go: Claude wrote the whole file (all lines are new additions).
	mainContent := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	writeFile(t, projDir, "main.go", mainContent)
	// Commit so we have git history; attribution timestamp is AFTER this commit.
	gitAdd(t, projDir, []string{"main.go"}, "add main.go")

	// go.mod: human-written file (no Claude content).
	gomodContent := "module example.com/proj\n\ngo 1.21\n"
	writeFile(t, projDir, "go.mod", gomodContent)
	gitAdd(t, projDir, []string{"go.mod"}, "add go.mod")

	// Insert session event for main.go with Claude's content.
	insertSessionEvent(t, s, "s1", filepath.Join(projDir, "main.go"),
		makeWriteRawJSON(filepath.Join(projDir, "main.go"), mainContent), baseTime)

	// Attributions timestamp AFTER the commits, so no base commit → full file fallback.
	insertAttribution(t, s, "main.go", projDir, "mostly_ai", "core_logic", baseTime, 7)
	insertAttribution(t, s, "go.mod", projDir, "mostly_human", "boilerplate", baseTime.Add(time.Second), 0)

	report, err := GenerateProjectFromStore(s)
	if err != nil {
		t.Fatal(err)
	}

	if report.ProjectPath != projDir {
		t.Errorf("ProjectPath = %q, want %q", report.ProjectPath, projDir)
	}
	if report.TotalFiles != 2 {
		t.Errorf("TotalFiles = %d, want 2", report.TotalFiles)
	}
	// main.go: 5 non-empty lines, 5 AI. go.mod: 2 non-empty lines, 0 AI.
	// (Empty lines are excluded from attribution.)
	if report.TotalLines != 7 {
		t.Errorf("TotalLines = %d, want 7", report.TotalLines)
	}
	if report.AILines != 5 {
		t.Errorf("AILines = %d, want 5", report.AILines)
	}
	if report.ByAuthorship["mostly_ai"] != 1 {
		t.Errorf("ByAuthorship[mostly_ai] = %d, want 1", report.ByAuthorship["mostly_ai"])
	}
	if report.ByAuthorship["mostly_human"] != 1 {
		t.Errorf("ByAuthorship[mostly_human] = %d, want 1", report.ByAuthorship["mostly_human"])
	}

	// Work type breakdown.
	if _, ok := report.ByWorkType["core_logic"]; !ok {
		t.Fatal("missing core_logic in ByWorkType")
	}
	if _, ok := report.ByWorkType["boilerplate"]; !ok {
		t.Fatal("missing boilerplate in ByWorkType")
	}
}

func TestGenerateProjectFromStore_DiffBasedAttribution(t *testing.T) {
	s, projDir, cleanup := setupTestStore(t)
	defer cleanup()

	// Step 1: Create an existing file and commit it (before tracking).
	originalContent := "package handler\n\nfunc Handle() {\n\treturn nil\n}\n"
	writeFile(t, projDir, "handler.go", originalContent)
	gitAdd(t, projDir, []string{"handler.go"}, "add handler.go")

	// Step 2: Simulate Claude editing 1 line (change "return nil" → "return ok").
	// The attribution timestamp is AFTER the initial commit.
	editedContent := "package handler\n\nfunc Handle() {\n\treturn ok\n}\n"
	writeFile(t, projDir, "handler.go", editedContent)
	// Don't commit the edit — it stays as a working tree change.

	claudeEditContent := "\treturn ok"
	insertSessionEvent(t, s, "s1", filepath.Join(projDir, "handler.go"),
		makeWriteRawJSON(filepath.Join(projDir, "handler.go"), claudeEditContent), baseTime)

	// Attribution timestamp is AFTER the initial commit so git finds the base.
	insertAttribution(t, s, "handler.go", projDir, "mostly_ai", "core_logic", baseTime, 1)

	report, err := GenerateProjectFromStore(s)
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Files) != 1 {
		t.Fatalf("Files = %d, want 1", len(report.Files))
	}

	fr := report.Files[0]
	// Git diff should show 1 addition ("return ok"), which matches Claude's content.
	// So AI% should be 100% for the changes (1/1).
	if fr.TotalLines != 1 {
		t.Errorf("TotalLines = %d, want 1 (only changed lines)", fr.TotalLines)
	}
	if fr.AILines != 1 {
		t.Errorf("AILines = %d, want 1", fr.AILines)
	}
	if !almostEqual(fr.RawAIPct, 100.0, 0.1) {
		t.Errorf("RawAIPct = %f, want 100.0 (Claude made the only change)", fr.RawAIPct)
	}
	if fr.AuthorshipLevel != "mostly_ai" {
		t.Errorf("AuthorshipLevel = %q, want %q", fr.AuthorshipLevel, "mostly_ai")
	}
}

func TestGenerateProjectFromStore_FilesSortedByAIPct(t *testing.T) {
	s, projDir, cleanup := setupTestStore(t)
	defer cleanup()

	// a.go: fully human (3 lines, 0 AI).
	writeFile(t, projDir, "a.go", "package a\n\nvar x = 1\n")
	gitAdd(t, projDir, []string{"a.go"}, "add a.go")

	// b.go: fully AI (3 lines, 3 AI).
	bContent := "package b\n\nvar y = 2\n"
	writeFile(t, projDir, "b.go", bContent)
	gitAdd(t, projDir, []string{"b.go"}, "add b.go")

	insertSessionEvent(t, s, "s1", filepath.Join(projDir, "b.go"),
		makeWriteRawJSON(filepath.Join(projDir, "b.go"), bContent), baseTime)

	insertAttribution(t, s, "a.go", projDir, "mostly_human", "core_logic", baseTime, 0)
	insertAttribution(t, s, "b.go", projDir, "mostly_ai", "core_logic", baseTime.Add(time.Second), 3)

	report, err := GenerateProjectFromStore(s)
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Files) != 2 {
		t.Fatalf("Files = %d, want 2", len(report.Files))
	}

	// b.go (100% AI) should come first.
	if report.Files[0].FilePath != "b.go" {
		t.Errorf("Files[0] = %q, want %q (highest AI%%)", report.Files[0].FilePath, "b.go")
	}
	if report.Files[1].FilePath != "a.go" {
		t.Errorf("Files[1] = %q, want %q (lowest AI%%)", report.Files[1].FilePath, "a.go")
	}
}

func TestGenerateProjectFromStore_NoData(t *testing.T) {
	s, _, cleanup := setupTestStore(t)
	defer cleanup()

	_, err := GenerateProjectFromStore(s)
	if err == nil {
		t.Fatal("expected error for empty database")
	}
}

// ---------------------------------------------------------------------------
// GenerateFile tests
// ---------------------------------------------------------------------------

func TestGenerateFileFromStore_SingleFile(t *testing.T) {
	s, projDir, cleanup := setupTestStore(t)
	defer cleanup()

	// handler.go: 5 lines total, Claude wrote 3 of them.
	handlerContent := "package handler\n\nfunc Handle() {\n\treturn\n}\n"
	claudeContent := "func Handle() {\n\treturn\n}\n"
	writeFile(t, projDir, "handler.go", handlerContent)
	gitAdd(t, projDir, []string{"handler.go"}, "add handler.go")

	insertSessionEvent(t, s, "s1", filepath.Join(projDir, "handler.go"),
		makeWriteRawJSON(filepath.Join(projDir, "handler.go"), claudeContent), baseTime)

	insertAttribution(t, s, "handler.go", projDir, "mostly_ai", "core_logic", baseTime, 3)
	insertAttribution(t, s, "handler.go", projDir, "mostly_human", "core_logic", baseTime.Add(time.Second), 2)

	fr, err := GenerateFileFromStore(s, "handler.go")
	if err != nil {
		t.Fatal(err)
	}

	if fr.FilePath != "handler.go" {
		t.Errorf("FilePath = %q, want %q", fr.FilePath, "handler.go")
	}
	if fr.WorkType != "core_logic" {
		t.Errorf("WorkType = %q, want %q", fr.WorkType, "core_logic")
	}
	if fr.TotalEvents != 2 {
		t.Errorf("TotalEvents = %d, want 2", fr.TotalEvents)
	}
	if fr.AIEventCount != 1 {
		t.Errorf("AIEventCount = %d, want 1", fr.AIEventCount)
	}
	// File was committed before attribution → no base commit found → full file fallback.
	// 3 AI lines / 4 non-empty total = 75%. (Empty lines excluded from attribution.)
	if fr.TotalLines != 4 {
		t.Errorf("TotalLines = %d, want 4", fr.TotalLines)
	}
	if fr.AILines != 3 {
		t.Errorf("AILines = %d, want 3", fr.AILines)
	}
	if !almostEqual(fr.RawAIPct, 75.0, 0.1) {
		t.Errorf("RawAIPct = %f, want 75.0", fr.RawAIPct)
	}
	if fr.AuthorshipLevel != "mostly_ai" {
		t.Errorf("AuthorshipLevel = %q, want %q", fr.AuthorshipLevel, "mostly_ai")
	}
}

func TestGenerateFileFromStore_FileNotFound(t *testing.T) {
	s, _, cleanup := setupTestStore(t)
	defer cleanup()

	_, err := GenerateFileFromStore(s, "nonexistent.go")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ---------------------------------------------------------------------------
// parseDiffAdditions tests
// ---------------------------------------------------------------------------

func TestParseDiffAdditions(t *testing.T) {
	diff := `diff --git a/foo.go b/foo.go
index 1234567..abcdefg 100644
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,4 @@
 package foo

+var x = 1
 func main() {}
`
	got := parseDiffAdditions(diff)
	want := "var x = 1\n"
	if got != want {
		t.Errorf("parseDiffAdditions() = %q, want %q", got, want)
	}
}

func TestParseDiffAdditions_MultipleLines(t *testing.T) {
	diff := `--- a/foo.go
+++ b/foo.go
@@ -1,2 +1,5 @@
 package foo
+
+import "fmt"
+
+func main() { fmt.Println("hi") }
`
	got := parseDiffAdditions(diff)
	want := "\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hi\") }\n"
	if got != want {
		t.Errorf("parseDiffAdditions() = %q, want %q", got, want)
	}
}

func TestParseDiffAdditions_NoDiff(t *testing.T) {
	got := parseDiffAdditions("")
	if got != "" {
		t.Errorf("parseDiffAdditions('') = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// Format tests
// ---------------------------------------------------------------------------

func TestFormatJSON_ValidOutput(t *testing.T) {
	data := map[string]interface{}{
		"key":   "value",
		"count": 42,
	}

	output := FormatJSON(data)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("FormatJSON produced invalid JSON: %v\nOutput: %s", err, output)
	}

	if parsed["key"] != "value" {
		t.Errorf("parsed[key] = %v, want %q", parsed["key"], "value")
	}
}

func TestFormatJSON_ReportRoundTrip(t *testing.T) {
	report := &ProjectReport{
		ProjectPath:     "/proj",
		MeaningfulAIPct: 65.5,
		RawAIPct:        70.0,
		TotalFiles:      3,
		TotalLines:      500,
		AILines:         350,
		ByAuthorship:    map[string]int{"mostly_ai": 5, "mostly_human": 2},
		ByWorkType: map[string]WorkTypeSummary{
			"core_logic": {Files: 2, AIEvents: 4, TotalEvents: 5, AIPct: 80.0, Tier: "high", Weight: 3.0, AILines: 300, TotalLines: 400},
		},
		Files: []FileReport{
			{FilePath: "main.go", WorkType: "core_logic", MeaningfulAIPct: 100.0, TotalEvents: 3, AIEventCount: 3, TotalLines: 200, AILines: 200, AuthorshipLevel: "mostly_ai"},
		},
	}

	output := FormatJSON(report)

	var parsed ProjectReport
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("FormatJSON report produced invalid JSON: %v", err)
	}

	if parsed.ProjectPath != "/proj" {
		t.Errorf("ProjectPath = %q, want %q", parsed.ProjectPath, "/proj")
	}
	if !almostEqual(parsed.MeaningfulAIPct, 65.5, 0.01) {
		t.Errorf("MeaningfulAIPct = %f, want 65.5", parsed.MeaningfulAIPct)
	}
}

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		got := humanBytes(tt.input)
		if got != tt.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatProjectReport_ContainsKey(t *testing.T) {
	report := &ProjectReport{
		ProjectPath:     "/proj",
		MeaningfulAIPct: 42.5,
		RawAIPct:        50.0,
		TotalFiles:      2,
		TotalLines:      100,
		AILines:         50,
		ByAuthorship:    map[string]int{"mostly_ai": 3, "mostly_human": 3},
		ByWorkType: map[string]WorkTypeSummary{
			"core_logic": {Files: 2, AIEvents: 3, TotalEvents: 6, AIPct: 50.0, Tier: "high", Weight: 3.0, TotalLines: 100},
		},
		Files: []FileReport{
			{FilePath: "main.go", WorkType: "core_logic", MeaningfulAIPct: 100.0, TotalEvents: 3, AIEventCount: 3, TotalLines: 50, AILines: 50, AuthorshipLevel: "mostly_ai"},
			{FilePath: "util.go", WorkType: "core_logic", MeaningfulAIPct: 0.0, TotalEvents: 3, AIEventCount: 0, TotalLines: 50, AuthorshipLevel: "mostly_human"},
		},
	}

	output := FormatProjectReport(report)

	checks := []string{
		"Attribution Report",
		"42.5%",
		"/proj",
		"core_logic",
		"main.go",
		"Authorship Spectrum",
		"Work Type Distribution",
	}
	for _, check := range checks {
		if !containsStr(output, check) {
			t.Errorf("FormatProjectReport output missing %q", check)
		}
	}
}

func TestFormatFileReport_ContainsKey(t *testing.T) {
	fr := &FileReport{
		FilePath:         "handler.go",
		WorkType:         "core_logic",
		MeaningfulAIPct:  66.7,
		RawAIPct:         66.7,
		TotalEvents:      3,
		AIEventCount:     2,
		TotalLines:       90,
		AILines:          60,
		AuthorshipLevel:  "mixed",
		AuthorshipCounts: map[string]int{"mostly_ai": 1, "mixed": 1, "mostly_human": 1},
	}

	output := FormatFileReport(fr)

	checks := []string{
		"File Report",
		"handler.go",
		"core_logic",
		"66.7%",
		"mostly_ai",
	}
	for _, check := range checks {
		if !containsStr(output, check) {
			t.Errorf("FormatFileReport output missing %q", check)
		}
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
