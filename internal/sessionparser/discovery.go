package sessionparser

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// watchForNewSessions uses fsnotify to monitor baseDir for newly created
// .jsonl session files. It watches recursively so that new project
// directories (and session files within them) are detected.
//
// This handles session rotation (CCSP-03): when Claude Code starts a new
// session, a new .jsonl file appears and is sent on the found channel.
func watchForNewSessions(ctx context.Context, baseDir string, found chan<- SessionFile) error {
	// Ensure the base directory exists.
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	// Add the base directory and all existing subdirectories.
	_ = addDirRecursive(watcher, baseDir)

	for {
		select {
		case <-ctx.Done():
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			// New directory created -- watch it recursively.
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = addDirRecursive(watcher, event.Name)
				}
			}

			// New or modified .jsonl file -- report it.
			if (event.Has(fsnotify.Create) || event.Has(fsnotify.Write)) &&
				strings.HasSuffix(event.Name, ".jsonl") {
				sf := SessionFile{
					Path:      event.Name,
					SessionID: sessionIDFromPath(event.Name),
					Provider:  "claude-code",
				}

				select {
				case found <- sf:
				case <-ctx.Done():
					return nil
				}
			}

		case _, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
		}
	}
}

// addDirRecursive adds dir and all its subdirectories to the watcher.
func addDirRecursive(w *fsnotify.Watcher, dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			_ = w.Add(path)
		}
		return nil
	})
}
