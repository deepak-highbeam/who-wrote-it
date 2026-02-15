package report

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/anthropic/gap-map/internal/store"
)

// ---------------------------------------------------------------------------
// Git helpers for branch tests (shared with report_branch_edge_test.go)
// ---------------------------------------------------------------------------

// gitInitMain initializes a git repo with "main" as the default branch.
func gitInitMain(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "symbolic-ref", "HEAD", "refs/heads/main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git command %v failed: %v\n%s", args, err, out)
		}
	}
}

func gitCheckoutExisting(t *testing.T, dir, branch string) {
	t.Helper()
	cmd := exec.Command("git", "checkout", branch)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout %s failed: %v\n%s", branch, err, out)
	}
}

func gitCheckoutCreate(t *testing.T, dir, branch string) {
	t.Helper()
	cmd := exec.Command("git", "checkout", "-b", branch)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout -b %s failed: %v\n%s", branch, err, out)
	}
}

// gitCommitOnBranch writes a file and commits it using current time.
func gitCommitOnBranch(t *testing.T, dir, file, content, message string) {
	t.Helper()
	writeFile(t, dir, file, content)
	cmd := exec.Command("git", "add", file)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}
}

func gitRevParseReport(t *testing.T, dir, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse %s failed: %v", ref, err)
	}
	return string(out[:len(out)-1]) // trim trailing newline
}

// setupBranchTestStore creates a temp store + git repo with "main" as default branch.
func setupBranchTestStore(t *testing.T) (*store.Store, string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "report-branch-test-*")
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "test.db")
	s, err := store.New(dbPath)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatal(err)
	}
	projDir := filepath.Join(dir, "proj")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		s.Close()
		os.RemoveAll(dir)
		t.Fatal(err)
	}
	gitInitMain(t, projDir)
	cleanup := func() {
		s.Close()
		os.RemoveAll(dir)
	}
	return s, projDir, cleanup
}

// insertAttributionOnBranch inserts a test attribution tagged with a branch.
func insertAttributionOnBranch(t *testing.T, s *store.Store, filePath, projectPath, level, workType, branch string, ts time.Time, linesChanged int) {
	t.Helper()
	if err := s.InsertFileEvent(projectPath, filePath, "write", ts); err != nil {
		t.Fatalf("insert file event: %v", err)
	}
	attr := store.AttributionRecord{
		FilePath:        filePath,
		ProjectPath:     projectPath,
		AuthorshipLevel: level,
		Confidence:      0.95,
		FirstAuthor:     "ai",
		Timestamp:       ts,
		LinesChanged:    linesChanged,
		Branch:          branch,
	}
	id, err := s.InsertAttribution(attr)
	if err != nil {
		t.Fatalf("insert attribution: %v", err)
	}
	if err := s.UpdateAttributionWorkType(id, workType); err != nil {
		t.Fatalf("update work type: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Branch-scoped report tests
// ---------------------------------------------------------------------------

func TestBranchReport_BasicAttribution(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	// Create a pre-existing file on main.
	gitCommitOnBranch(t, projDir, "handler.go",
		"package handler\n\nfunc Handle() {\n\treturn nil\n}\n",
		"add handler")

	// Create feature branch; Claude edits the file.
	gitCheckoutCreate(t, projDir, "feature-x")
	editedContent := "package handler\n\nimport \"fmt\"\n\nfunc Handle() {\n\tfmt.Println(\"ok\")\n\treturn nil\n}\n"
	gitCommitOnBranch(t, projDir, "handler.go", editedContent, "claude edit")

	// Insert session event + attribution on the branch.
	insertSessionEvent(t, s, "s1", filepath.Join(projDir, "handler.go"),
		makeWriteRawJSON(filepath.Join(projDir, "handler.go"), editedContent), baseTime)
	insertAttributionOnBranch(t, s, "handler.go", projDir, "mostly_ai", "core_logic", "feature-x", baseTime, 3)

	report, err := GenerateProjectForBranch(s, "feature-x", "main")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch: %v", err)
	}

	if report.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1", report.TotalFiles)
	}
	if len(report.Files) == 0 {
		t.Fatal("expected at least 1 file in report")
	}
	if report.Files[0].FilePath != "handler.go" {
		t.Errorf("FilePath = %q, want %q", report.Files[0].FilePath, "handler.go")
	}
	// Only the branch changes should be counted, not the whole file.
	if report.TotalLines <= 0 {
		t.Error("expected positive TotalLines for branch changes")
	}
}

func TestBranchReport_NewFileOnBranch(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	gitCheckoutCreate(t, projDir, "feature-x")
	newContent := "package newpkg\n\nfunc NewFunc() int {\n\treturn 42\n}\n"
	gitCommitOnBranch(t, projDir, "new.go", newContent, "add new file")

	insertSessionEvent(t, s, "s1", filepath.Join(projDir, "new.go"),
		makeWriteRawJSON(filepath.Join(projDir, "new.go"), newContent), baseTime)
	insertAttributionOnBranch(t, s, "new.go", projDir, "mostly_ai", "core_logic", "feature-x", baseTime, 4)

	report, err := GenerateProjectForBranch(s, "feature-x", "main")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch: %v", err)
	}

	if report.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1", report.TotalFiles)
	}
	if len(report.Files) == 0 {
		t.Fatal("expected 1 file")
	}
	// New file: 100% of lines are changes.
	if report.Files[0].TotalLines != 4 {
		t.Errorf("TotalLines = %d, want 4 (all non-empty lines)", report.Files[0].TotalLines)
	}
}

func TestBranchReport_IgnoresOtherBranches(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	// Branch feature-x adds file-x.go.
	gitCheckoutCreate(t, projDir, "feature-x")
	gitCommitOnBranch(t, projDir, "file-x.go", "package x\n\nvar X = 1\n", "add x")
	insertAttributionOnBranch(t, s, "file-x.go", projDir, "mostly_ai", "core_logic", "feature-x", baseTime, 2)

	// Branch feature-y adds file-y.go.
	gitCheckoutExisting(t, projDir, "main")
	gitCheckoutCreate(t, projDir, "feature-y")
	gitCommitOnBranch(t, projDir, "file-y.go", "package y\n\nvar Y = 2\n", "add y")
	insertAttributionOnBranch(t, s, "file-y.go", projDir, "mostly_ai", "core_logic", "feature-y", baseTime.Add(time.Second), 2)

	// Report for feature-x should only show file-x.go.
	report, err := GenerateProjectForBranch(s, "feature-x", "main")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch: %v", err)
	}

	if report.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1 (only feature-x files)", report.TotalFiles)
	}
	if len(report.Files) > 0 && report.Files[0].FilePath != "file-x.go" {
		t.Errorf("FilePath = %q, want %q", report.Files[0].FilePath, "file-x.go")
	}
}

func TestBranchReport_SameFileMultipleBranches(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	// Base file on main.
	gitCommitOnBranch(t, projDir, "shared.go",
		"package shared\n\nfunc Base() {}\n", "base file")

	// feature-x edits shared.go.
	gitCheckoutCreate(t, projDir, "feature-x")
	gitCommitOnBranch(t, projDir, "shared.go",
		"package shared\n\nfunc Base() {}\n\nfunc FeatureX() {}\n", "add FeatureX")
	insertAttributionOnBranch(t, s, "shared.go", projDir, "mostly_ai", "core_logic", "feature-x", baseTime, 1)

	// feature-y edits shared.go differently.
	gitCheckoutExisting(t, projDir, "main")
	gitCheckoutCreate(t, projDir, "feature-y")
	gitCommitOnBranch(t, projDir, "shared.go",
		"package shared\n\nfunc Base() {}\n\nfunc FeatureY() {}\n", "add FeatureY")
	insertAttributionOnBranch(t, s, "shared.go", projDir, "mostly_ai", "core_logic", "feature-y", baseTime.Add(time.Second), 1)

	// Report for feature-x should show FeatureX changes only.
	reportX, err := GenerateProjectForBranch(s, "feature-x", "main")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch (feature-x): %v", err)
	}
	if reportX.TotalFiles != 1 {
		t.Errorf("feature-x TotalFiles = %d, want 1", reportX.TotalFiles)
	}

	// Report for feature-y should show FeatureY changes only.
	reportY, err := GenerateProjectForBranch(s, "feature-y", "main")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch (feature-y): %v", err)
	}
	if reportY.TotalFiles != 1 {
		t.Errorf("feature-y TotalFiles = %d, want 1", reportY.TotalFiles)
	}
}

func TestBranchReport_RevertedOnBranch(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	// Base file on main.
	gitCommitOnBranch(t, projDir, "handler.go",
		"package handler\n\nfunc Handle() {\n\treturn nil\n}\n", "base")

	// Feature branch: edit then revert.
	gitCheckoutCreate(t, projDir, "feature-x")
	gitCommitOnBranch(t, projDir, "handler.go",
		"package handler\n\nfunc Handle() {\n\treturn ok\n}\n", "edit")
	gitCommitOnBranch(t, projDir, "handler.go",
		"package handler\n\nfunc Handle() {\n\treturn nil\n}\n", "revert")
	insertAttributionOnBranch(t, s, "handler.go", projDir, "mostly_ai", "core_logic", "feature-x", baseTime, 1)

	report, err := GenerateProjectForBranch(s, "feature-x", "main")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch: %v", err)
	}

	// All changes reverted: 0 files in report.
	if report.TotalFiles != 0 {
		t.Errorf("TotalFiles = %d, want 0 (all changes reverted)", report.TotalFiles)
	}
	if report.TotalLines != 0 {
		t.Errorf("TotalLines = %d, want 0", report.TotalLines)
	}
}

func TestBranchReport_UncommittedChanges(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	gitCheckoutCreate(t, projDir, "feature-x")
	// Committed change.
	gitCommitOnBranch(t, projDir, "a.go", "package a\n\nvar A = 1\n", "committed")
	insertAttributionOnBranch(t, s, "a.go", projDir, "mostly_ai", "core_logic", "feature-x", baseTime, 2)

	// Uncommitted change in working tree.
	writeFile(t, projDir, "b.go", "package b\n\nvar B = 2\n")
	insertAttributionOnBranch(t, s, "b.go", projDir, "mostly_ai", "core_logic", "feature-x", baseTime.Add(time.Second), 2)

	report, err := GenerateProjectForBranch(s, "feature-x", "main")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch: %v", err)
	}

	// Both committed and uncommitted files should appear.
	if report.TotalFiles < 2 {
		t.Errorf("TotalFiles = %d, want >= 2 (committed + uncommitted)", report.TotalFiles)
	}
}

func TestBranchReport_EmptyBranch(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()
	_ = projDir

	_, err := GenerateProjectForBranch(s, "nonexistent-branch", "main")
	if err == nil {
		t.Fatal("expected error for empty/nonexistent branch, got nil")
	}
}
