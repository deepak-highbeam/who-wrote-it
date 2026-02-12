package watcher

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/anthropic/who-wrote-it/internal/config"
	"github.com/anthropic/who-wrote-it/internal/store"
)

// Watcher monitors file system events, filters ignored paths, debounces
// rapid changes, and writes clean events to the store.
type Watcher struct {
	store     *store.Store
	cfg       *config.Config
	fsw       *fsnotify.Watcher
	filter    *Filter
	debouncer *Debouncer
}

// New creates a Watcher wired to the given store and config.
func New(s *store.Store, cfg *config.Config) *Watcher {
	return &Watcher{
		store: s,
		cfg:   cfg,
	}
}

// Start begins watching all configured paths recursively.
// It blocks until ctx is cancelled. Call Stop() for ordered teardown.
func (w *Watcher) Start(ctx context.Context) error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.fsw = fsw

	// Build filter from config + defaults.
	w.filter = NewFilter(w.cfg.IgnorePatterns)

	// Build debouncer that writes events to the store.
	w.debouncer = NewDebouncer(100*time.Millisecond, func(e Event) {
		if err := w.store.InsertFileEvent(w.projectPath(e.Path), e.Path, e.Type, e.Timestamp); err != nil {
			log.Printf("watcher: store insert: %v", err)
		}
	})

	// Add all configured watch paths (recursively).
	for _, root := range w.cfg.WatchPaths {
		if err := w.addRecursive(root); err != nil {
			log.Printf("watcher: walk %s: %v", root, err)
		}
	}

	// watching paths silently

	// Event loop.
	for {
		select {
		case <-ctx.Done():
			return nil

		case ev, ok := <-fsw.Events:
			if !ok {
				return nil
			}
			w.handleEvent(ev)

		case err, ok := <-fsw.Errors:
			if !ok {
				return nil
			}
			log.Printf("watcher: fsnotify error: %v", err)
		}
	}
}

// Stop drains the debouncer (emitting pending events) and closes fsnotify.
func (w *Watcher) Stop() {
	if w.debouncer != nil {
		w.debouncer.Stop()
	}
	if w.fsw != nil {
		_ = w.fsw.Close()
	}
}

// handleEvent processes a single fsnotify event.
func (w *Watcher) handleEvent(ev fsnotify.Event) {
	// Skip if path matches an ignore pattern.
	if w.filter.ShouldIgnore(ev.Name) {
		return
	}

	// If a directory was created, start watching it recursively.
	if ev.Has(fsnotify.Create) {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			_ = w.addRecursive(ev.Name)
		}
	}

	// Map fsnotify operations to our event types.
	eventType := mapEventType(ev.Op)
	if eventType == "" {
		return // chmod-only, not interesting
	}

	w.debouncer.Feed(Event{
		Path:      ev.Name,
		Type:      eventType,
		Timestamp: time.Now(),
	})
}

// addRecursive walks root and adds every directory that is not ignored.
func (w *Watcher) addRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}
		if !d.IsDir() {
			return nil
		}
		if w.filter.ShouldIgnore(path) {
			return filepath.SkipDir
		}
		_ = w.fsw.Add(path)
		return nil
	})
}

// projectPath returns the configured watch root that contains path, or the
// path itself if no watch root matches.
func (w *Watcher) projectPath(path string) string {
	for _, root := range w.cfg.WatchPaths {
		absRoot, err1 := filepath.Abs(root)
		absPath, err2 := filepath.Abs(path)
		if err1 == nil && err2 == nil {
			if rel, err := filepath.Rel(absRoot, absPath); err == nil && rel != ".." && len(rel) > 0 && rel[0] != '.' {
				return absRoot
			}
		}
	}
	return path
}

// mapEventType converts fsnotify.Op to a string event type.
func mapEventType(op fsnotify.Op) string {
	switch {
	case op.Has(fsnotify.Create):
		return "create"
	case op.Has(fsnotify.Remove):
		return "delete"
	case op.Has(fsnotify.Rename):
		return "rename"
	case op.Has(fsnotify.Write):
		return "modify"
	default:
		return "" // e.g. Chmod only
	}
}
