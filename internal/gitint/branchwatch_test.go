package gitint

import (
	"os/exec"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Branch watch tests
// ---------------------------------------------------------------------------

func TestBranchWatch_DetectsSwitch(t *testing.T) {
	dir := t.TempDir()
	gitInitShell(t, dir)
	gitCheckoutNewBranch(t, dir, "feature-x")
	gitCheckoutBranch(t, dir, "main")

	var mu sync.Mutex
	var received []string

	cancel, err := WatchBranch(dir, func(newBranch string) {
		mu.Lock()
		received = append(received, newBranch)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("WatchBranch: %v", err)
	}
	if cancel != nil {
		defer cancel()
	}

	gitCheckoutBranch(t, dir, "feature-x")
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 callback, got %d", len(received))
	}
	if received[0] != "feature-x" {
		t.Errorf("callback branch = %q, want %q", received[0], "feature-x")
	}
}

func TestBranchWatch_MultipleSwitches(t *testing.T) {
	dir := t.TempDir()
	gitInitShell(t, dir)
	gitCheckoutNewBranch(t, dir, "a")
	gitCheckoutNewBranch(t, dir, "b")
	gitCheckoutBranch(t, dir, "main")

	var mu sync.Mutex
	var received []string

	cancel, err := WatchBranch(dir, func(newBranch string) {
		mu.Lock()
		received = append(received, newBranch)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("WatchBranch: %v", err)
	}
	if cancel != nil {
		defer cancel()
	}

	for _, branch := range []string{"a", "b", "main"} {
		gitCheckoutBranch(t, dir, branch)
		time.Sleep(500 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 3 {
		t.Fatalf("expected 3 callbacks, got %d: %v", len(received), received)
	}
	expected := []string{"a", "b", "main"}
	for i, want := range expected {
		if received[i] != want {
			t.Errorf("callback[%d] = %q, want %q", i, received[i], want)
		}
	}
}

func TestBranchWatch_NoFalsePositives(t *testing.T) {
	dir := t.TempDir()
	gitInitShell(t, dir)

	var mu sync.Mutex
	var received []string

	cancel, err := WatchBranch(dir, func(newBranch string) {
		mu.Lock()
		received = append(received, newBranch)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("WatchBranch: %v", err)
	}
	if cancel != nil {
		defer cancel()
	}

	// Commit on the same branch should NOT trigger callback.
	gitCommitFile(t, dir, "new.txt", "hello", "add file")
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 0 {
		t.Errorf("expected 0 callbacks for same-branch commit, got %d: %v", len(received), received)
	}
}

func TestBranchWatch_DetachedHEAD(t *testing.T) {
	dir := t.TempDir()
	gitInitShell(t, dir)
	gitCommitFile(t, dir, "file.txt", "content", "add file")
	hash := gitRevParseHelper(t, dir, "HEAD")

	var mu sync.Mutex
	var received []string

	cancel, err := WatchBranch(dir, func(newBranch string) {
		mu.Lock()
		received = append(received, newBranch)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("WatchBranch: %v", err)
	}
	if cancel != nil {
		defer cancel()
	}

	// Detach HEAD.
	cmd := exec.Command("git", "checkout", "--detach", "HEAD")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout --detach failed: %v\n%s", err, out)
	}
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 callback, got %d", len(received))
	}
	if received[0] != hash {
		t.Errorf("callback = %q, want commit hash %q", received[0], hash)
	}
}
