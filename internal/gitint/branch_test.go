package gitint

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Git helpers for branch tests (using exec.Command, not go-git)
// ---------------------------------------------------------------------------

// gitInitShell initializes a git repo with "main" as the default branch.
func gitInitShell(t *testing.T, dir string) {
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

func gitCheckoutBranch(t *testing.T, dir, branch string) {
	t.Helper()
	cmd := exec.Command("git", "checkout", branch)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout %s failed: %v\n%s", branch, err, out)
	}
}

func gitCheckoutNewBranch(t *testing.T, dir, branch string) {
	t.Helper()
	cmd := exec.Command("git", "checkout", "-b", branch)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout -b %s failed: %v\n%s", branch, err, out)
	}
}

func gitCommitFile(t *testing.T, dir, file, content, message string) {
	t.Helper()
	path := filepath.Join(dir, file)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
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

func gitRevParseHelper(t *testing.T, dir, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse %s failed: %v", ref, err)
	}
	return strings.TrimSpace(string(out))
}

func gitDeleteFileCommit(t *testing.T, dir, file, message string) {
	t.Helper()
	cmd := exec.Command("git", "rm", file)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git rm failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit (delete) failed: %v\n%s", err, out)
	}
}

// ---------------------------------------------------------------------------
// CurrentBranch tests
// ---------------------------------------------------------------------------

func TestCurrentBranch_Main(t *testing.T) {
	dir := t.TempDir()
	gitInitShell(t, dir)

	branch, err := CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "main" {
		t.Errorf("CurrentBranch = %q, want %q", branch, "main")
	}
}

func TestCurrentBranch_FeatureBranch(t *testing.T) {
	dir := t.TempDir()
	gitInitShell(t, dir)
	gitCheckoutNewBranch(t, dir, "feature-x")

	branch, err := CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "feature-x" {
		t.Errorf("CurrentBranch = %q, want %q", branch, "feature-x")
	}
}

func TestCurrentBranch_DetachedHEAD(t *testing.T) {
	dir := t.TempDir()
	gitInitShell(t, dir)
	gitCommitFile(t, dir, "file.txt", "content", "add file")

	hash := gitRevParseHelper(t, dir, "HEAD")
	cmd := exec.Command("git", "checkout", "--detach", "HEAD")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout --detach failed: %v\n%s", err, out)
	}

	branch, err := CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != hash {
		t.Errorf("CurrentBranch = %q, want commit hash %q", branch, hash)
	}
}

func TestCurrentBranch_NewBranchNoCommits(t *testing.T) {
	dir := t.TempDir()
	gitInitShell(t, dir)
	gitCheckoutNewBranch(t, dir, "new-feature")

	branch, err := CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "new-feature" {
		t.Errorf("CurrentBranch = %q, want %q", branch, "new-feature")
	}
}

// ---------------------------------------------------------------------------
// MergeBaseDiff tests
// ---------------------------------------------------------------------------

func TestMergeBaseDiff_OnlyBranchChanges(t *testing.T) {
	dir := t.TempDir()
	gitInitShell(t, dir)

	// Create a file on main.
	gitCommitFile(t, dir, "file.go", "package main\n\nfunc main() {}\n", "initial file")

	// Create feature branch and modify the file.
	gitCheckoutNewBranch(t, dir, "feature-x")
	gitCommitFile(t, dir, "file.go",
		"package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n",
		"add greeting")

	diff := MergeBaseDiffAdditions(dir, "main")

	// Should contain only branch additions.
	if !strings.Contains(diff, "import \"fmt\"") {
		t.Errorf("diff should contain 'import \"fmt\"', got: %q", diff)
	}
	if !strings.Contains(diff, "fmt.Println") {
		t.Errorf("diff should contain 'fmt.Println', got: %q", diff)
	}
	// Should NOT contain content already on main.
	if strings.Contains(diff, "func main() {}") {
		t.Error("diff should not contain original main content")
	}
}

func TestMergeBaseDiff_NewFileOnBranch(t *testing.T) {
	dir := t.TempDir()
	gitInitShell(t, dir)

	gitCheckoutNewBranch(t, dir, "feature-x")
	gitCommitFile(t, dir, "new.go", "package new\n\nvar X = 1\n", "add new file")

	diff := MergeBaseDiffAdditions(dir, "main")

	if !strings.Contains(diff, "package new") {
		t.Errorf("diff should contain entire new file, got: %q", diff)
	}
	if !strings.Contains(diff, "var X = 1") {
		t.Errorf("diff should contain 'var X = 1', got: %q", diff)
	}
}

func TestMergeBaseDiff_DeletedFileOnBranch(t *testing.T) {
	dir := t.TempDir()
	gitInitShell(t, dir)

	gitCommitFile(t, dir, "delete-me.go", "package main\n\nvar Gone = true\n", "add file")

	gitCheckoutNewBranch(t, dir, "feature-x")
	gitDeleteFileCommit(t, dir, "delete-me.go", "delete file")

	diff := MergeBaseDiffAdditions(dir, "main")

	// Deleted content should not appear as additions.
	if strings.Contains(diff, "Gone") {
		t.Error("diff should not contain deleted content as additions")
	}
}

func TestMergeBaseDiff_MainAdvanced(t *testing.T) {
	dir := t.TempDir()
	gitInitShell(t, dir)

	gitCommitFile(t, dir, "base.go", "package base\n", "base file")

	// Create feature branch.
	gitCheckoutNewBranch(t, dir, "feature-x")
	gitCommitFile(t, dir, "feature.go", "package feature\n\nvar F = 1\n", "feature file")

	// Advance main with new commits.
	gitCheckoutBranch(t, dir, "main")
	gitCommitFile(t, dir, "main-new.go", "package main\n\nvar M = 1\n", "main advance")

	// Switch back to feature.
	gitCheckoutBranch(t, dir, "feature-x")

	diff := MergeBaseDiffAdditions(dir, "main")

	// Should show feature branch changes, not main's new commits.
	if !strings.Contains(diff, "var F = 1") {
		t.Errorf("diff should contain feature branch additions, got: %q", diff)
	}
	if strings.Contains(diff, "var M = 1") {
		t.Error("diff should not contain main's new content")
	}
}

func TestMergeBaseDiff_NoDifference(t *testing.T) {
	dir := t.TempDir()
	gitInitShell(t, dir)

	gitCheckoutNewBranch(t, dir, "empty-branch")
	// No commits on this branch.

	diff := MergeBaseDiffAdditions(dir, "main")

	if diff != "" {
		t.Errorf("diff should be empty for branch with no changes, got: %q", diff)
	}
}

func TestMergeBaseDiff_UncommittedChanges(t *testing.T) {
	dir := t.TempDir()
	gitInitShell(t, dir)

	gitCheckoutNewBranch(t, dir, "feature-x")
	gitCommitFile(t, dir, "committed.go", "package committed\n\nvar C = 1\n", "committed change")

	// Add uncommitted change (working tree only).
	writeFile(t, dir, "uncommitted.go", "package uncommitted\n\nvar U = 1\n")

	// Working-tree mode should include unstaged changes.
	diffAll := MergeBaseDiffAdditions(dir, "main")
	if !strings.Contains(diffAll, "var U = 1") {
		t.Errorf("MergeBaseDiffAdditions should include uncommitted changes, got: %q", diffAll)
	}

	// Committed-only mode should NOT include unstaged changes.
	diffCommitted := MergeBaseDiffAdditionsCommitted(dir, "main", "feature-x")
	if strings.Contains(diffCommitted, "var U = 1") {
		t.Errorf("MergeBaseDiffAdditionsCommitted should not include uncommitted changes, got: %q", diffCommitted)
	}

	// Both should include committed changes.
	if !strings.Contains(diffAll, "var C = 1") {
		t.Errorf("MergeBaseDiffAdditions should include committed changes, got: %q", diffAll)
	}
	if !strings.Contains(diffCommitted, "var C = 1") {
		t.Errorf("MergeBaseDiffAdditionsCommitted should include committed changes, got: %q", diffCommitted)
	}
}
