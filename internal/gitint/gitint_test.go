package gitint

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/anthropic/who-wrote-it/internal/store"
)

// --- Co-Author Detection Tests ---

func TestDetectCoAuthor(t *testing.T) {
	cases := []struct {
		name      string
		message   string
		wantFound bool
		wantName  string
	}{
		{
			name:      "standard format",
			message:   "feat: add auth\n\nCo-Authored-By: Claude <claude@anthropic.com>",
			wantFound: true,
			wantName:  "Claude",
		},
		{
			name:      "lowercase",
			message:   "fix: bug\n\nco-authored-by: GPT-4 <gpt@openai.com>",
			wantFound: true,
			wantName:  "GPT-4",
		},
		{
			name:      "mixed case",
			message:   "chore: cleanup\n\nCo-authored-by: Copilot <copilot@github.com>",
			wantFound: true,
			wantName:  "Copilot",
		},
		{
			name:      "no email angle brackets",
			message:   "feat: thing\n\nCo-Authored-By: Claude",
			wantFound: true,
			wantName:  "Claude",
		},
		{
			name:      "no coauthor",
			message:   "feat: add feature\n\nSome description.",
			wantFound: false,
			wantName:  "",
		},
		{
			name:      "empty message",
			message:   "",
			wantFound: false,
			wantName:  "",
		},
		{
			name:      "coauthor in middle of message",
			message:   "fix: stuff\n\nCo-Authored-By: AI Helper <ai@example.com>\n\nSigned-off-by: Dev",
			wantFound: true,
			wantName:  "AI Helper",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			found, name := DetectCoAuthor(tc.message)
			if found != tc.wantFound {
				t.Errorf("DetectCoAuthor found = %v, want %v", found, tc.wantFound)
			}
			if name != tc.wantName {
				t.Errorf("DetectCoAuthor name = %q, want %q", name, tc.wantName)
			}
		})
	}
}

func TestAllCoAuthors(t *testing.T) {
	msg := "feat: multi-author\n\nCo-Authored-By: Alice <alice@example.com>\nCo-Authored-By: Bob <bob@example.com>"
	names := AllCoAuthors(msg)
	if len(names) != 2 {
		t.Fatalf("AllCoAuthors returned %d names, want 2", len(names))
	}
	if names[0] != "Alice" {
		t.Errorf("names[0] = %q, want %q", names[0], "Alice")
	}
	if names[1] != "Bob" {
		t.Errorf("names[1] = %q, want %q", names[1], "Bob")
	}
}

// --- Integration Tests (require temp git repo) ---

func TestSyncCommitsAndDiffs(t *testing.T) {
	// Create a temp directory with a git repo.
	tmpDir := t.TempDir()
	repo := initTestRepo(t, tmpDir)

	// Create initial commit.
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, tmpDir, "hello.go", "package main\n\nfunc main() {}\n")
	if _, err := wt.Add("hello.go"); err != nil {
		t.Fatal(err)
	}
	commitHash1, err := wt.Commit("feat: initial commit\n\nCo-Authored-By: Claude <claude@anthropic.com>", &gogit.CommitOptions{
		Author: testAuthor(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Second commit modifying the file.
	writeFile(t, tmpDir, "hello.go", "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n")
	if _, err := wt.Add("hello.go"); err != nil {
		t.Fatal(err)
	}
	commitHash2, err := wt.Commit("fix: add greeting", &gogit.CommitOptions{
		Author: testAuthor(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Third commit renaming a file.
	writeFile(t, tmpDir, "greeting.go", "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n")
	if _, err := wt.Add("greeting.go"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Remove("hello.go"); err != nil {
		t.Fatal(err)
	}
	_, err = wt.Commit("refactor: rename hello.go to greeting.go", &gogit.CommitOptions{
		Author: testAuthor(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Open store.
	dbPath := filepath.Join(tmpDir, "test.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Open repository with store.
	r, err := Open(tmpDir, s)
	if err != nil {
		t.Fatal(err)
	}

	// Sync all commits.
	since := time.Now().Add(-1 * time.Hour)
	if err := r.SyncCommits(context.Background(), since); err != nil {
		t.Fatalf("SyncCommits: %v", err)
	}

	// Verify commits were stored.
	count, err := s.GitCommitsCount()
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("GitCommitsCount = %d, want 3", count)
	}

	// Verify coauthor detection via DB.
	var hasCoauthor int
	var coauthorName string
	err = s.DB().QueryRow(`SELECT has_coauthor_tag, coauthor_name FROM git_commits WHERE hash = ?`, commitHash1.String()).Scan(&hasCoauthor, &coauthorName)
	if err != nil {
		t.Fatal(err)
	}
	if hasCoauthor != 1 {
		t.Error("expected coauthor tag on first commit")
	}
	if coauthorName != "Claude" {
		t.Errorf("coauthor_name = %q, want %q", coauthorName, "Claude")
	}

	// Verify no coauthor on second commit.
	err = s.DB().QueryRow(`SELECT has_coauthor_tag FROM git_commits WHERE hash = ?`, commitHash2.String()).Scan(&hasCoauthor)
	if err != nil {
		t.Fatal(err)
	}
	if hasCoauthor != 0 {
		t.Error("expected no coauthor tag on second commit")
	}

	// Verify diffs were stored.
	var diffCount int64
	err = s.DB().QueryRow(`SELECT COUNT(*) FROM git_diffs`).Scan(&diffCount)
	if err != nil {
		t.Fatal(err)
	}
	if diffCount < 3 {
		t.Errorf("git_diffs count = %d, want >= 3", diffCount)
	}

	// Verify rename was detected (file with old_path set).
	var renameCount int64
	err = s.DB().QueryRow(`SELECT COUNT(*) FROM git_diffs WHERE old_path != '' AND change_type = 'rename'`).Scan(&renameCount)
	if err != nil {
		t.Fatal(err)
	}
	// Note: go-git may or may not detect it as a rename depending on similarity.
	// At minimum we should have delete + add or a rename entry.
	var deleteAddCount int64
	err = s.DB().QueryRow(`SELECT COUNT(*) FROM git_diffs WHERE (change_type = 'delete' AND file_path = 'hello.go') OR (change_type = 'add' AND file_path = 'greeting.go') OR (change_type = 'rename' AND file_path = 'greeting.go')`).Scan(&deleteAddCount)
	if err != nil {
		t.Fatal(err)
	}
	if deleteAddCount < 1 {
		t.Error("expected rename or delete+add for hello.go -> greeting.go")
	}

	// Verify second sync is a no-op (HEAD unchanged).
	if err := r.SyncCommits(context.Background(), since); err != nil {
		t.Fatalf("second SyncCommits: %v", err)
	}
}

func TestBlameFile(t *testing.T) {
	tmpDir := t.TempDir()
	repo := initTestRepo(t, tmpDir)

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	content := "line1\nline2\nline3\n"
	writeFile(t, tmpDir, "test.txt", content)
	if _, err := wt.Add("test.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("add test.txt", &gogit.CommitOptions{
		Author: testAuthor(),
	}); err != nil {
		t.Fatal(err)
	}

	lines, err := BlameFile(repo, "test.txt")
	if err != nil {
		t.Fatalf("BlameFile: %v", err)
	}

	if len(lines) != 3 {
		t.Fatalf("BlameFile returned %d lines, want 3", len(lines))
	}

	for i, line := range lines {
		if line.LineNumber != i+1 {
			t.Errorf("line %d: LineNumber = %d, want %d", i, line.LineNumber, i+1)
		}
		if line.CommitHash == "" {
			t.Errorf("line %d: CommitHash is empty", i)
		}
		if line.Author == "" {
			t.Errorf("line %d: Author is empty", i)
		}
		if line.ContentHash == "" {
			t.Errorf("line %d: ContentHash is empty", i)
		}
	}
}

func TestBlameAndStore(t *testing.T) {
	tmpDir := t.TempDir()
	repo := initTestRepo(t, tmpDir)

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, tmpDir, "blame.txt", "alpha\nbeta\n")
	if _, err := wt.Add("blame.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("add blame.txt", &gogit.CommitOptions{
		Author: testAuthor(),
	}); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := BlameAndStore(repo, s, "blame.txt"); err != nil {
		t.Fatalf("BlameAndStore: %v", err)
	}

	var count int64
	err = s.DB().QueryRow(`SELECT COUNT(*) FROM git_blame_lines WHERE file_path = 'blame.txt'`).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("blame lines count = %d, want 2", count)
	}
}

func TestBlameDeletedFile(t *testing.T) {
	tmpDir := t.TempDir()
	repo := initTestRepo(t, tmpDir)

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, tmpDir, "temp.txt", "content\n")
	if _, err := wt.Add("temp.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("add temp.txt", &gogit.CommitOptions{
		Author: testAuthor(),
	}); err != nil {
		t.Fatal(err)
	}

	// Delete the file and commit.
	os.Remove(filepath.Join(tmpDir, "temp.txt"))
	if _, err := wt.Remove("temp.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("remove temp.txt", &gogit.CommitOptions{
		Author: testAuthor(),
	}); err != nil {
		t.Fatal(err)
	}

	// Blame of deleted file should return an error.
	_, err = BlameFile(repo, "temp.txt")
	if err == nil {
		t.Error("expected error blaming deleted file, got nil")
	}
}

// --- Helpers ---

func initTestRepo(t *testing.T, dir string) *gogit.Repository {
	t.Helper()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func testAuthor() *object.Signature {
	return &object.Signature{
		Name:  "Test Author",
		Email: "test@example.com",
		When:  time.Now(),
	}
}
