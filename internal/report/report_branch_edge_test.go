package report

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

)

// ---------------------------------------------------------------------------
// Edge-case git helpers (rebase simulation, Graphite stack helpers)
// ---------------------------------------------------------------------------

// gitSquashCommits simulates squashing the last n commits into one.
func gitSquashCommits(t *testing.T, dir string, n int, message string) {
	t.Helper()
	cmd := exec.Command("git", "reset", "--soft", fmt.Sprintf("HEAD~%d", n))
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git reset --soft failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit (squash) failed: %v\n%s", err, out)
	}
}

// gitDropCommit simulates dropping a specific commit using rebase --onto.
func gitDropCommit(t *testing.T, dir, commitHash string) {
	t.Helper()
	cmd := exec.Command("git", "rebase", "--onto", commitHash+"^", commitHash)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git drop commit %s failed: %v\n%s", commitHash, err, out)
	}
}

// gitRebaseOnto rebases the current branch onto the given branch.
func gitRebaseOnto(t *testing.T, dir, onto string) {
	t.Helper()
	cmd := exec.Command("git", "rebase", onto)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git rebase %s failed: %v\n%s", onto, err, out)
	}
}

// gitAmendLastCommit amends the last commit with a new file content and message.
func gitAmendLastCommit(t *testing.T, dir, file, content, message string) {
	t.Helper()
	writeFile(t, dir, file, content)
	cmd := exec.Command("git", "add", file)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add (amend) failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "--amend", "-m", message)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit --amend failed: %v\n%s", err, out)
	}
}

// gitCherryPick cherry-picks a commit onto the current branch.
func gitCherryPick(t *testing.T, dir, hash string) {
	t.Helper()
	cmd := exec.Command("git", "cherry-pick", hash)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git cherry-pick %s failed: %v\n%s", hash, err, out)
	}
}

// gitCherryPickResolve cherry-picks a commit, expects a conflict, resolves it
// with the provided content, and continues.
func gitCherryPickResolve(t *testing.T, dir, hash, resolveFile, resolveContent string) {
	t.Helper()
	cmd := exec.Command("git", "cherry-pick", hash)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return // no conflict, pick succeeded
	}
	// Expect conflict — resolve it.
	if !strings.Contains(string(out), "conflict") && !strings.Contains(string(out), "CONFLICT") {
		t.Fatalf("cherry-pick failed without conflict: %v\n%s", err, out)
	}
	writeFile(t, dir, resolveFile, resolveContent)
	cmd = exec.Command("git", "add", resolveFile)
	cmd.Dir = dir
	if out, err = cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add (resolve) failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "cherry-pick", "--continue")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_EDITOR=true")
	if out, err = cmd.CombinedOutput(); err != nil {
		t.Fatalf("cherry-pick --continue failed: %v\n%s", err, out)
	}
}

// gitDeleteFileBranch removes a file and commits the deletion.
func gitDeleteFileBranch(t *testing.T, dir, file, message string) {
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

// gitMergeFastForward merges a branch into the current branch (fast-forward).
func gitMergeFastForward(t *testing.T, dir, branch string) {
	t.Helper()
	cmd := exec.Command("git", "merge", "--ff-only", branch)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git merge --ff-only %s failed: %v\n%s", branch, err, out)
	}
}

// ---------------------------------------------------------------------------
// Rebase edge-case tests
// ---------------------------------------------------------------------------

func TestBranchReport_RebaseSquash(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	gitCheckoutCreate(t, projDir, "feature-x")

	// 3 commits, each adding a file.
	for i := 1; i <= 3; i++ {
		file := fmt.Sprintf("file%d.go", i)
		content := fmt.Sprintf("package file%d\n\nvar V%d = %d\n", i, i, i)
		gitCommitOnBranch(t, projDir, file, content, fmt.Sprintf("add file%d", i))
		insertAttributionOnBranch(t, s, file, projDir, "mostly_ai", "core_logic", "feature-x",
			baseTime.Add(time.Duration(i)*time.Second), 2)
	}

	// Squash all 3 into 1.
	gitSquashCommits(t, projDir, 3, "squashed: add all files")

	report, err := GenerateProjectForBranch(s, "feature-x", "main")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch: %v", err)
	}

	// All 3 files should still appear in the diff.
	if report.TotalFiles != 3 {
		t.Errorf("TotalFiles = %d, want 3 (squash doesn't change diff)", report.TotalFiles)
	}
}

func TestBranchReport_RebaseDropAICommit(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	gitCheckoutCreate(t, projDir, "feature-x")

	// Commit 1: AI file.
	gitCommitOnBranch(t, projDir, "ai.go", "package ai\n\nvar AI = 1\n", "ai file")
	aiHash := gitRevParseReport(t, projDir, "HEAD")
	insertAttributionOnBranch(t, s, "ai.go", projDir, "mostly_ai", "core_logic", "feature-x", baseTime, 2)

	// Commit 2: human file.
	gitCommitOnBranch(t, projDir, "human.go", "package human\n\nvar H = 1\n", "human file")
	insertAttributionOnBranch(t, s, "human.go", projDir, "mostly_human", "core_logic", "feature-x", baseTime.Add(time.Second), 2)

	// Drop the AI commit.
	gitDropCommit(t, projDir, aiHash)

	report, err := GenerateProjectForBranch(s, "feature-x", "main")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch: %v", err)
	}

	// AI file should be gone from the diff.
	for _, f := range report.Files {
		if f.FilePath == "ai.go" {
			t.Error("ai.go should not appear in report after dropping its commit")
		}
	}
}

func TestBranchReport_RebaseDropHumanCommit(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	gitCheckoutCreate(t, projDir, "feature-x")

	// Commit 1: human file.
	gitCommitOnBranch(t, projDir, "human.go", "package human\n\nvar H = 1\n", "human file")
	humanHash := gitRevParseReport(t, projDir, "HEAD")
	insertAttributionOnBranch(t, s, "human.go", projDir, "mostly_human", "core_logic", "feature-x", baseTime, 2)

	// Commit 2: AI file.
	gitCommitOnBranch(t, projDir, "ai.go", "package ai\n\nvar AI = 1\n", "ai file")
	insertAttributionOnBranch(t, s, "ai.go", projDir, "mostly_ai", "core_logic", "feature-x", baseTime.Add(time.Second), 2)

	// Drop the human commit.
	gitDropCommit(t, projDir, humanHash)

	report, err := GenerateProjectForBranch(s, "feature-x", "main")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch: %v", err)
	}

	// Only the AI file should remain.
	if report.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1 (only AI file after dropping human commit)", report.TotalFiles)
	}
	if len(report.Files) > 0 && report.Files[0].FilePath != "ai.go" {
		t.Errorf("FilePath = %q, want %q", report.Files[0].FilePath, "ai.go")
	}
}

func TestBranchReport_RebaseFixupModifiesAICode(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	gitCheckoutCreate(t, projDir, "feature-x")

	// Commit 1: AI writes file.go with unique content.
	aiContent := "ai_func_alpha\nai_compute_beta\nai_process_gamma\nai_result_delta\n"
	gitCommitOnBranch(t, projDir, "file.go", aiContent, "AI writes file")
	insertSessionEvent(t, s, "s1", filepath.Join(projDir, "file.go"),
		makeWriteRawJSON(filepath.Join(projDir, "file.go"), aiContent), baseTime)
	insertAttributionOnBranch(t, s, "file.go", projDir, "mostly_ai", "core_logic", "feature-x", baseTime, 3)

	// Commit 2: human replaces AI content entirely with non-overlapping content.
	humanContent := "human_validate_epsilon\nhuman_check_zeta\nhuman_verify_eta\nhuman_output_theta\n"
	gitCommitOnBranch(t, projDir, "file.go", humanContent, "human fixup")

	// Squash the two commits (simulates fixup).
	gitSquashCommits(t, projDir, 2, "fixuped: file.go")

	report, err := GenerateProjectForBranch(s, "feature-x", "main")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch: %v", err)
	}

	// After fixup, the file content is all human-written.
	if len(report.Files) == 0 {
		t.Fatal("expected at least 1 file")
	}
	f := report.Files[0]
	// AI% should be low since human replaced all AI content.
	if f.AILines > 0 && f.RawAIPct > 10 {
		t.Errorf("after human fixup, expected low AI%%, got %.1f%% (%d AI lines)", f.RawAIPct, f.AILines)
	}
}

func TestBranchReport_RebaseReorder(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	gitCheckoutCreate(t, projDir, "feature-x")
	mergeBase := gitRevParseReport(t, projDir, "HEAD")

	// 3 commits on feature-x, each adding a separate file.
	var hashes [3]string
	for i := 0; i < 3; i++ {
		file := fmt.Sprintf("reorder%d.go", i)
		content := fmt.Sprintf("package reorder%d\n\nvar R%d = %d\n", i, i, i)
		gitCommitOnBranch(t, projDir, file, content, fmt.Sprintf("add reorder%d", i))
		hashes[i] = gitRevParseReport(t, projDir, "HEAD")
		insertAttributionOnBranch(t, s, file, projDir, "mostly_ai", "core_logic", "feature-x",
			baseTime.Add(time.Duration(i)*time.Second), 2)
	}

	// Simulate reorder: B, A, C via cherry-pick onto merge-base.
	cmd := exec.Command("git", "checkout", "-B", "feature-x", mergeBase)
	cmd.Dir = projDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("reset feature-x failed: %v\n%s", err, out)
	}
	for _, idx := range []int{1, 0, 2} {
		gitCherryPick(t, projDir, hashes[idx])
	}

	report, err := GenerateProjectForBranch(s, "feature-x", "main")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch: %v", err)
	}

	// Reorder doesn't change the diff — all 3 files still present.
	if report.TotalFiles != 3 {
		t.Errorf("TotalFiles = %d, want 3 (reorder doesn't change diff)", report.TotalFiles)
	}
}

func TestBranchReport_RebaseReword(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	gitCheckoutCreate(t, projDir, "feature-x")
	gitCommitOnBranch(t, projDir, "file.go", "package pkg\n\nvar V = 1\n", "original message")
	insertAttributionOnBranch(t, s, "file.go", projDir, "mostly_ai", "core_logic", "feature-x", baseTime, 2)

	// Amend (reword) the commit message.
	cmd := exec.Command("git", "commit", "--amend", "-m", "reworded message")
	cmd.Dir = projDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit --amend failed: %v\n%s", err, out)
	}

	report, err := GenerateProjectForBranch(s, "feature-x", "main")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch: %v", err)
	}

	// Reword has no effect on lines — report unchanged.
	if report.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1 (reword doesn't change diff)", report.TotalFiles)
	}
}

func TestBranchReport_RebaseFixupPartialOverwrite(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	gitCheckoutCreate(t, projDir, "feature-x")

	// AI writes a 10-line file.
	var lines []string
	for i := 1; i <= 10; i++ {
		lines = append(lines, fmt.Sprintf("var line%d = %d", i, i))
	}
	aiContent := "package pkg\n\n" + strings.Join(lines, "\n") + "\n"
	gitCommitOnBranch(t, projDir, "file.go", aiContent, "AI writes 10 lines")
	// Session event only contains the var lines (not "package pkg" boilerplate).
	aiVarContent := strings.Join(lines, "\n") + "\n"
	insertSessionEvent(t, s, "s1", filepath.Join(projDir, "file.go"),
		makeWriteRawJSON(filepath.Join(projDir, "file.go"), aiVarContent), baseTime)
	insertAttributionOnBranch(t, s, "file.go", projDir, "mostly_ai", "core_logic", "feature-x", baseTime, 10)

	// Human fixup replaces 3 of the 10 lines (lines 1-3).
	lines[0] = "var line1 = 100 // human edit"
	lines[1] = "var line2 = 200 // human edit"
	lines[2] = "var line3 = 300 // human edit"
	fixupContent := "package pkg\n\n" + strings.Join(lines, "\n") + "\n"
	gitCommitOnBranch(t, projDir, "file.go", fixupContent, "human fixup 3 lines")

	// Squash into one commit.
	gitSquashCommits(t, projDir, 2, "fixuped partial")

	report, err := GenerateProjectForBranch(s, "feature-x", "main")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch: %v", err)
	}

	if len(report.Files) == 0 {
		t.Fatal("expected at least 1 file")
	}
	f := report.Files[0]
	// 7 of 10 AI lines should survive the fixup.
	if f.AILines != 7 {
		t.Errorf("AILines = %d, want 7 (3 overwritten by human)", f.AILines)
	}
}

func TestBranchReport_RebaseOntoUpdatedMain(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	// Main has a base file.
	gitCommitOnBranch(t, projDir, "base.go", "package base\n", "base")

	gitCheckoutCreate(t, projDir, "feature-x")
	gitCommitOnBranch(t, projDir, "feature.go", "package feature\n\nvar F = 1\n", "feature file")
	insertAttributionOnBranch(t, s, "feature.go", projDir, "mostly_ai", "core_logic", "feature-x", baseTime, 2)

	// Advance main.
	gitCheckoutExisting(t, projDir, "main")
	gitCommitOnBranch(t, projDir, "main2.go", "package main2\n\nvar M = 1\n", "main advance")

	// Rebase feature onto updated main.
	gitCheckoutExisting(t, projDir, "feature-x")
	gitRebaseOnto(t, projDir, "main")

	report, err := GenerateProjectForBranch(s, "feature-x", "main")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch: %v", err)
	}

	// Report should be unchanged — still shows feature.go only.
	if report.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1 (rebase onto updated main)", report.TotalFiles)
	}
	if len(report.Files) > 0 && report.Files[0].FilePath != "feature.go" {
		t.Errorf("FilePath = %q, want %q", report.Files[0].FilePath, "feature.go")
	}
}

// ---------------------------------------------------------------------------
// Graphite stack tests
// ---------------------------------------------------------------------------

func TestStack_DiffAgainstParentBranch(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	// main → branch-a → branch-b → branch-c
	gitCheckoutCreate(t, projDir, "branch-a")
	gitCommitOnBranch(t, projDir, "a.go", "package a\n\nvar A = 1\n", "add a")
	insertAttributionOnBranch(t, s, "a.go", projDir, "mostly_ai", "core_logic", "branch-a", baseTime, 2)

	gitCheckoutCreate(t, projDir, "branch-b")
	gitCommitOnBranch(t, projDir, "b.go", "package b\n\nvar B = 1\n", "add b")
	insertAttributionOnBranch(t, s, "b.go", projDir, "mostly_ai", "core_logic", "branch-b", baseTime.Add(time.Second), 2)

	gitCheckoutCreate(t, projDir, "branch-c")
	gitCommitOnBranch(t, projDir, "c.go", "package c\n\nvar C = 1\n", "add c")
	insertAttributionOnBranch(t, s, "c.go", projDir, "mostly_ai", "core_logic", "branch-c", baseTime.Add(2*time.Second), 2)

	// Report for branch-b against branch-a: should only show b.go.
	report, err := GenerateProjectForBranch(s, "branch-b", "branch-a")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch: %v", err)
	}
	if report.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1 (only branch-b's delta)", report.TotalFiles)
	}
	if len(report.Files) > 0 && report.Files[0].FilePath != "b.go" {
		t.Errorf("FilePath = %q, want %q", report.Files[0].FilePath, "b.go")
	}
}

func TestStack_SharedFileAcrossStack(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	// Main has shared.go with funcA.
	gitCommitOnBranch(t, projDir, "shared.go",
		"package shared\n\nfunc FuncA() {}\n", "base shared")

	gitCheckoutCreate(t, projDir, "branch-a")
	gitCommitOnBranch(t, projDir, "shared.go",
		"package shared\n\nfunc FuncA() {}\n\nfunc FuncB() {}\n", "add FuncB")
	insertAttributionOnBranch(t, s, "shared.go", projDir, "mostly_ai", "core_logic", "branch-a", baseTime, 1)

	gitCheckoutCreate(t, projDir, "branch-b")
	gitCommitOnBranch(t, projDir, "shared.go",
		"package shared\n\nfunc FuncA() {}\n\nfunc FuncB() {}\n\nfunc FuncC() {}\n", "add FuncC")
	insertAttributionOnBranch(t, s, "shared.go", projDir, "mostly_ai", "core_logic", "branch-b", baseTime.Add(time.Second), 1)

	// branch-b vs branch-a: should only show FuncC addition, not FuncB.
	report, err := GenerateProjectForBranch(s, "branch-b", "branch-a")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch: %v", err)
	}
	// Only branch-b's delta (FuncC).
	if report.TotalLines > 2 {
		t.Errorf("TotalLines = %d, want <= 2 (only FuncC lines, no double-counting)", report.TotalLines)
	}
}

func TestStack_FullStackVsMain(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	// main → a → b → c
	gitCheckoutCreate(t, projDir, "branch-a")
	gitCommitOnBranch(t, projDir, "a.go", "package a\n\nvar A = 1\n", "add a")
	insertAttributionOnBranch(t, s, "a.go", projDir, "mostly_ai", "core_logic", "branch-a", baseTime, 2)

	gitCheckoutCreate(t, projDir, "branch-b")
	gitCommitOnBranch(t, projDir, "b.go", "package b\n\nvar B = 1\n", "add b")
	insertAttributionOnBranch(t, s, "b.go", projDir, "mostly_ai", "core_logic", "branch-b", baseTime.Add(time.Second), 2)

	gitCheckoutCreate(t, projDir, "branch-c")
	gitCommitOnBranch(t, projDir, "c.go", "package c\n\nvar C = 1\n", "add c")
	insertAttributionOnBranch(t, s, "c.go", projDir, "mostly_ai", "core_logic", "branch-c", baseTime.Add(2*time.Second), 2)

	// Per-branch reports.
	ra, errA := GenerateProjectForBranch(s, "branch-a", "main")
	rb, errB := GenerateProjectForBranch(s, "branch-b", "branch-a")
	rc, errC := GenerateProjectForBranch(s, "branch-c", "branch-b")
	rFull, errFull := GenerateProjectForBranch(s, "branch-c", "main")

	for _, err := range []error{errA, errB, errC, errFull} {
		if err != nil {
			t.Fatalf("GenerateProjectForBranch: %v", err)
		}
	}

	sumFiles := ra.TotalFiles + rb.TotalFiles + rc.TotalFiles
	if sumFiles != rFull.TotalFiles {
		t.Errorf("sum of per-branch files (%d) != full stack files (%d)", sumFiles, rFull.TotalFiles)
	}

	sumLines := ra.TotalLines + rb.TotalLines + rc.TotalLines
	if sumLines != rFull.TotalLines {
		t.Errorf("sum of per-branch lines (%d) != full stack lines (%d)", sumLines, rFull.TotalLines)
	}
}

func TestStack_Restack_ModifyBottomBranch(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	// main → branch-a → branch-b
	gitCheckoutCreate(t, projDir, "branch-a")
	gitCommitOnBranch(t, projDir, "a.go", "package a\n\nvar A = 1\n", "add a")
	insertAttributionOnBranch(t, s, "a.go", projDir, "mostly_ai", "core_logic", "branch-a", baseTime, 2)

	gitCheckoutCreate(t, projDir, "branch-b")
	gitCommitOnBranch(t, projDir, "b.go", "package b\n\nvar B = 1\n", "add b")
	bCommit := gitRevParseReport(t, projDir, "HEAD")
	insertAttributionOnBranch(t, s, "b.go", projDir, "mostly_ai", "core_logic", "branch-b", baseTime.Add(time.Second), 2)

	// Amend branch-a.
	gitCheckoutExisting(t, projDir, "branch-a")
	gitAmendLastCommit(t, projDir, "a.go", "package a\n\nvar A = 100 // amended\n", "amended a")

	// Restack: reset branch-b to amended branch-a, cherry-pick only b's commit.
	cmd := exec.Command("git", "checkout", "-B", "branch-b", "branch-a")
	cmd.Dir = projDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("reset branch-b failed: %v\n%s", err, out)
	}
	gitCherryPick(t, projDir, bCommit)

	report, err := GenerateProjectForBranch(s, "branch-b", "branch-a")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch: %v", err)
	}

	// branch-b report should be unchanged (still just b.go).
	if report.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1 (restack doesn't change upper branch)", report.TotalFiles)
	}
}

func TestStack_Restack_ConflictResolution(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	// Main has shared.go.
	gitCommitOnBranch(t, projDir, "shared.go", "line1\nline2\nline3\n", "base shared")

	// branch-a modifies line2.
	gitCheckoutCreate(t, projDir, "branch-a")
	gitCommitOnBranch(t, projDir, "shared.go", "line1\nline2-a\nline3\n", "edit line2 on a")
	insertAttributionOnBranch(t, s, "shared.go", projDir, "mostly_ai", "core_logic", "branch-a", baseTime, 1)

	// branch-b (off branch-a original) also modifies line2.
	gitCheckoutExisting(t, projDir, "main")
	gitCheckoutCreate(t, projDir, "branch-b")
	gitCommitOnBranch(t, projDir, "shared.go", "line1\nline2-b\nline3\n", "edit line2 on b")
	bCommit := gitRevParseReport(t, projDir, "HEAD")
	insertAttributionOnBranch(t, s, "shared.go", projDir, "mostly_ai", "core_logic", "branch-b", baseTime.Add(time.Second), 1)

	// Restack branch-b onto branch-a → conflict on line2.
	cmd := exec.Command("git", "checkout", "-B", "branch-b", "branch-a")
	cmd.Dir = projDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("reset branch-b failed: %v\n%s", err, out)
	}
	// Cherry-pick with conflict resolution (human resolves).
	resolvedContent := "line1\nline2-resolved-by-human\nline3\n"
	gitCherryPickResolve(t, projDir, bCommit, "shared.go", resolvedContent)

	report, err := GenerateProjectForBranch(s, "branch-b", "branch-a")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch: %v", err)
	}

	// Conflict resolution lines should be attributed to human.
	if len(report.Files) > 0 {
		f := report.Files[0]
		if f.AILines > 0 {
			t.Errorf("conflict resolution lines should be human, got %d AI lines", f.AILines)
		}
	}
}

func TestStack_BottomMerges_RetargetToMain(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	// main → branch-a → branch-b
	gitCheckoutCreate(t, projDir, "branch-a")
	gitCommitOnBranch(t, projDir, "a.go", "package a\n\nvar A = 1\n", "add a")
	insertAttributionOnBranch(t, s, "a.go", projDir, "mostly_ai", "core_logic", "branch-a", baseTime, 2)

	gitCheckoutCreate(t, projDir, "branch-b")
	gitCommitOnBranch(t, projDir, "b.go", "package b\n\nvar B = 1\n", "add b")
	insertAttributionOnBranch(t, s, "b.go", projDir, "mostly_ai", "core_logic", "branch-b", baseTime.Add(time.Second), 2)

	// Merge branch-a into main (fast-forward).
	gitCheckoutExisting(t, projDir, "main")
	gitMergeFastForward(t, projDir, "branch-a")

	// Now branch-b's report against main should only show its delta (b.go).
	gitCheckoutExisting(t, projDir, "branch-b")
	report, err := GenerateProjectForBranch(s, "branch-b", "main")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch: %v", err)
	}

	if report.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1 (only branch-b's delta after merge)", report.TotalFiles)
	}
	if len(report.Files) > 0 && report.Files[0].FilePath != "b.go" {
		t.Errorf("FilePath = %q, want %q", report.Files[0].FilePath, "b.go")
	}
}

func TestStack_Split_SingleBranchToTwo(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	// Single branch with 2 commits.
	gitCheckoutCreate(t, projDir, "feature-all")
	gitCommitOnBranch(t, projDir, "file1.go", "package f1\n\nvar F1 = 1\n", "add file1")
	commit1 := gitRevParseReport(t, projDir, "HEAD")
	insertAttributionOnBranch(t, s, "file1.go", projDir, "mostly_ai", "core_logic", "feature-part1", baseTime, 2)

	gitCommitOnBranch(t, projDir, "file2.go", "package f2\n\nvar F2 = 2\n", "add file2")
	insertAttributionOnBranch(t, s, "file2.go", projDir, "mostly_ai", "core_logic", "feature-part2", baseTime.Add(time.Second), 2)

	// Split: branch-1 = just commit1, branch-2 = commit2 off branch-1.
	gitCheckoutExisting(t, projDir, "main")
	cmd := exec.Command("git", "checkout", "-b", "feature-part1", commit1)
	cmd.Dir = projDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create feature-part1 failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "checkout", "-b", "feature-part2", "feature-all")
	cmd.Dir = projDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create feature-part2 failed: %v\n%s", err, out)
	}

	// Report for part1: only file1.go.
	r1, err := GenerateProjectForBranch(s, "feature-part1", "main")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch (part1): %v", err)
	}
	if r1.TotalFiles != 1 {
		t.Errorf("part1 TotalFiles = %d, want 1", r1.TotalFiles)
	}

	// Report for part2 against part1: only file2.go.
	r2, err := GenerateProjectForBranch(s, "feature-part2", "feature-part1")
	if err != nil {
		t.Fatalf("GenerateProjectForBranch (part2): %v", err)
	}
	if r2.TotalFiles != 1 {
		t.Errorf("part2 TotalFiles = %d, want 1", r2.TotalFiles)
	}
}

func TestStack_SameFileEditedAtEveryLevel(t *testing.T) {
	s, projDir, cleanup := setupBranchTestStore(t)
	defer cleanup()

	// Main has shared.go with one line.
	gitCommitOnBranch(t, projDir, "shared.go", "line1\n", "base")

	// branch-a adds line2.
	gitCheckoutCreate(t, projDir, "branch-a")
	gitCommitOnBranch(t, projDir, "shared.go", "line1\nline2\n", "add line2")
	insertAttributionOnBranch(t, s, "shared.go", projDir, "mostly_ai", "core_logic", "branch-a", baseTime, 1)

	// branch-b (off a) adds line3.
	gitCheckoutCreate(t, projDir, "branch-b")
	gitCommitOnBranch(t, projDir, "shared.go", "line1\nline2\nline3\n", "add line3")
	insertAttributionOnBranch(t, s, "shared.go", projDir, "mostly_ai", "core_logic", "branch-b", baseTime.Add(time.Second), 1)

	// branch-c (off b) adds line4.
	gitCheckoutCreate(t, projDir, "branch-c")
	gitCommitOnBranch(t, projDir, "shared.go", "line1\nline2\nline3\nline4\n", "add line4")
	insertAttributionOnBranch(t, s, "shared.go", projDir, "mostly_ai", "core_logic", "branch-c", baseTime.Add(2*time.Second), 1)

	// Each level should only report its own addition.
	ra, errA := GenerateProjectForBranch(s, "branch-a", "main")
	rb, errB := GenerateProjectForBranch(s, "branch-b", "branch-a")
	rc, errC := GenerateProjectForBranch(s, "branch-c", "branch-b")

	for _, err := range []error{errA, errB, errC} {
		if err != nil {
			t.Fatalf("GenerateProjectForBranch: %v", err)
		}
	}

	// Each branch adds exactly 1 line.
	if ra.TotalLines != 1 {
		t.Errorf("branch-a TotalLines = %d, want 1", ra.TotalLines)
	}
	if rb.TotalLines != 1 {
		t.Errorf("branch-b TotalLines = %d, want 1", rb.TotalLines)
	}
	if rc.TotalLines != 1 {
		t.Errorf("branch-c TotalLines = %d, want 1", rc.TotalLines)
	}
}
