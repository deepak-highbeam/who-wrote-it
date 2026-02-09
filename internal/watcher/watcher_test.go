package watcher

import (
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Filter tests
// ---------------------------------------------------------------------------

func TestFilterDefaultPatterns(t *testing.T) {
	f := NewFilter(nil)

	cases := []struct {
		path string
		want bool
	}{
		{".git/config", true},
		{".git", true},
		{"node_modules/package.json", true},
		{"node_modules", true},
		{".DS_Store", true},
		{".idea/workspace.xml", true},
		{".vscode/settings.json", true},
		{"__pycache__/foo.pyc", true},
		{"backup.swp", true},
		{"file.swo", true},
		{"notes~", true},
	}

	for _, tc := range cases {
		if got := f.ShouldIgnore(tc.path); got != tc.want {
			t.Errorf("ShouldIgnore(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestFilterAllowsNormalFiles(t *testing.T) {
	f := NewFilter(nil)

	cases := []string{
		"main.go",
		"src/app.ts",
		"README.md",
		"internal/watcher/watcher.go",
		"docs/guide.html",
	}

	for _, path := range cases {
		if f.ShouldIgnore(path) {
			t.Errorf("ShouldIgnore(%q) = true, expected false for normal file", path)
		}
	}
}

func TestFilterNestedIgnore(t *testing.T) {
	f := NewFilter(nil)

	cases := []struct {
		path string
		want bool
	}{
		{"deep/path/node_modules/foo.js", true},
		{"a/b/c/.git/HEAD", true},
		{"project/.idea/modules.xml", true},
		{"very/deep/path/.DS_Store", true},
		{"lib/__pycache__/module.pyc", true},
	}

	for _, tc := range cases {
		if got := f.ShouldIgnore(tc.path); got != tc.want {
			t.Errorf("ShouldIgnore(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestFilterCustomPatterns(t *testing.T) {
	f := NewFilter([]string{"*.log", "tmp"})

	cases := []struct {
		path string
		want bool
	}{
		{"app.log", true},
		{"logs/error.log", true},
		{"app.txt", false},
		{"tmp/cache", true},
		{"data/tmp/file", true},
		// Default patterns still work.
		{".git/config", true},
		{"node_modules/x.js", true},
	}

	for _, tc := range cases {
		if got := f.ShouldIgnore(tc.path); got != tc.want {
			t.Errorf("ShouldIgnore(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestFilterDuplicatePatterns(t *testing.T) {
	// Passing a default pattern again should not cause double entries.
	f := NewFilter([]string{".git", "node_modules", "*.log"})
	// Just verify it doesn't panic and works correctly.
	if !f.ShouldIgnore(".git/HEAD") {
		t.Error("expected .git/HEAD to be ignored")
	}
	if !f.ShouldIgnore("test.log") {
		t.Error("expected test.log to be ignored")
	}
}

// ---------------------------------------------------------------------------
// Debouncer tests
// ---------------------------------------------------------------------------

func TestDebouncerSingleEvent(t *testing.T) {
	var mu sync.Mutex
	var emitted []Event

	d := NewDebouncer(50*time.Millisecond, func(e Event) {
		mu.Lock()
		emitted = append(emitted, e)
		mu.Unlock()
	})
	defer d.Stop()

	d.Feed(Event{Path: "/a/b.txt", Type: "modify", Timestamp: time.Now()})

	// Wait for the debounce window to expire plus a little buffer.
	time.Sleep(120 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(emitted) != 1 {
		t.Fatalf("expected 1 emission, got %d", len(emitted))
	}
	if emitted[0].Path != "/a/b.txt" {
		t.Errorf("expected path /a/b.txt, got %s", emitted[0].Path)
	}
}

func TestDebouncerBurstCollapse(t *testing.T) {
	var mu sync.Mutex
	var emitted []Event

	d := NewDebouncer(50*time.Millisecond, func(e Event) {
		mu.Lock()
		emitted = append(emitted, e)
		mu.Unlock()
	})
	defer d.Stop()

	// Feed 10 events for the same path in rapid succession.
	for i := 0; i < 10; i++ {
		d.Feed(Event{
			Path:      "/a/b.txt",
			Type:      "modify",
			Timestamp: time.Now(),
		})
		time.Sleep(5 * time.Millisecond) // well within the 50ms window
	}

	// Wait for debounce to fire.
	time.Sleep(120 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(emitted) != 1 {
		t.Fatalf("expected exactly 1 emission after burst of 10, got %d", len(emitted))
	}
}

func TestDebouncerDifferentPaths(t *testing.T) {
	var mu sync.Mutex
	var emitted []Event

	d := NewDebouncer(50*time.Millisecond, func(e Event) {
		mu.Lock()
		emitted = append(emitted, e)
		mu.Unlock()
	})
	defer d.Stop()

	d.Feed(Event{Path: "/a.txt", Type: "modify", Timestamp: time.Now()})
	d.Feed(Event{Path: "/b.txt", Type: "create", Timestamp: time.Now()})

	// Wait for debounce window.
	time.Sleep(120 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(emitted) != 2 {
		t.Fatalf("expected 2 emissions (one per path), got %d", len(emitted))
	}

	paths := map[string]bool{}
	for _, e := range emitted {
		paths[e.Path] = true
	}
	if !paths["/a.txt"] || !paths["/b.txt"] {
		t.Errorf("expected both /a.txt and /b.txt, got %v", paths)
	}
}

func TestDebouncerStopDrains(t *testing.T) {
	var mu sync.Mutex
	var emitted []Event

	d := NewDebouncer(5*time.Second, func(e Event) {
		mu.Lock()
		emitted = append(emitted, e)
		mu.Unlock()
	})

	// Feed events -- with a 5s window they won't fire naturally.
	d.Feed(Event{Path: "/x.txt", Type: "create", Timestamp: time.Now()})
	d.Feed(Event{Path: "/y.txt", Type: "modify", Timestamp: time.Now()})

	// Stop should drain all pending events immediately.
	d.Stop()

	mu.Lock()
	defer mu.Unlock()
	if len(emitted) != 2 {
		t.Fatalf("expected 2 drained emissions, got %d", len(emitted))
	}

	paths := map[string]bool{}
	for _, e := range emitted {
		paths[e.Path] = true
	}
	if !paths["/x.txt"] || !paths["/y.txt"] {
		t.Errorf("expected /x.txt and /y.txt, got %v", paths)
	}
}

func TestDebouncerEmitsLastEvent(t *testing.T) {
	var mu sync.Mutex
	var emitted []Event

	d := NewDebouncer(50*time.Millisecond, func(e Event) {
		mu.Lock()
		emitted = append(emitted, e)
		mu.Unlock()
	})
	defer d.Stop()

	// Feed create then modify for the same path.
	d.Feed(Event{Path: "/a.txt", Type: "create", Timestamp: time.Now()})
	time.Sleep(10 * time.Millisecond)
	d.Feed(Event{Path: "/a.txt", Type: "modify", Timestamp: time.Now()})

	time.Sleep(120 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(emitted) != 1 {
		t.Fatalf("expected 1 emission, got %d", len(emitted))
	}
	if emitted[0].Type != "modify" {
		t.Errorf("expected last event type 'modify', got %q", emitted[0].Type)
	}
}

func TestDebouncerFeedAfterStop(t *testing.T) {
	emitted := 0
	d := NewDebouncer(50*time.Millisecond, func(e Event) {
		emitted++
	})

	d.Stop()

	// Feed after stop should be a no-op, not panic.
	d.Feed(Event{Path: "/a.txt", Type: "create", Timestamp: time.Now()})
	time.Sleep(100 * time.Millisecond)

	if emitted != 0 {
		t.Errorf("expected 0 emissions after stop, got %d", emitted)
	}
}
