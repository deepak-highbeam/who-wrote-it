package gitint

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// WatchBranch watches the .git/HEAD file at repoPath for branch switches.
// When a switch is detected, onChange is called with the new branch name.
// Returns a cancel function to stop watching and any error.
func WatchBranch(repoPath string, onChange func(newBranch string)) (cancel func(), err error) {
	gitDir := filepath.Join(repoPath, ".git")
	headPath := filepath.Join(gitDir, "HEAD")

	currentBranch, err := readBranchFromHEAD(headPath)
	if err != nil {
		return nil, err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := watcher.Add(gitDir); err != nil {
		watcher.Close()
		return nil, err
	}

	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if filepath.Base(event.Name) != "HEAD" {
					continue
				}
				if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
					continue
				}

				newBranch, err := readBranchFromHEAD(headPath)
				if err != nil {
					continue
				}

				mu.Lock()
				changed := newBranch != currentBranch
				if changed {
					currentBranch = newBranch
				}
				mu.Unlock()

				if changed {
					onChange(newBranch)
				}
			case <-watcher.Errors:
				// Ignore errors silently.
			case <-done:
				return
			}
		}
	}()

	cancel = func() {
		close(done)
		watcher.Close()
	}
	return cancel, nil
}

// readBranchFromHEAD reads the .git/HEAD file and returns the current branch
// name or commit hash for detached HEAD.
func readBranchFromHEAD(headPath string) (string, error) {
	data, err := os.ReadFile(headPath)
	if err != nil {
		return "", err
	}
	content := strings.TrimSpace(string(data))
	if strings.HasPrefix(content, "ref: refs/heads/") {
		return strings.TrimPrefix(content, "ref: refs/heads/"), nil
	}
	// Detached HEAD — content is the commit hash.
	return content, nil
}
