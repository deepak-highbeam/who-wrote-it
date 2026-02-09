package watcher

import (
	"sync"
	"time"
)

// Event represents a single file system change.
type Event struct {
	Path      string
	Type      string // "create", "modify", "delete", "rename"
	Timestamp time.Time
}

// Debouncer collapses rapid events for the same file path into a single
// emission after a configurable quiet window. It is safe for concurrent use.
type Debouncer struct {
	window time.Duration
	emit   func(Event)

	mu      sync.Mutex
	timers  map[string]*time.Timer
	pending map[string]Event
	stopped bool
}

// NewDebouncer creates a Debouncer that waits for `window` of silence on a
// given path before emitting the most recent event for that path.
func NewDebouncer(window time.Duration, emit func(Event)) *Debouncer {
	return &Debouncer{
		window:  window,
		emit:    emit,
		timers:  make(map[string]*time.Timer),
		pending: make(map[string]Event),
	}
}

// Feed receives a raw event. If a timer already exists for the event's path,
// it is reset and the stored event is updated. Otherwise a new timer is started.
func (d *Debouncer) Feed(e Event) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped {
		return
	}

	d.pending[e.Path] = e

	if t, ok := d.timers[e.Path]; ok {
		t.Reset(d.window)
		return
	}

	path := e.Path
	d.timers[path] = time.AfterFunc(d.window, func() {
		d.mu.Lock()
		ev, ok := d.pending[path]
		delete(d.timers, path)
		delete(d.pending, path)
		d.mu.Unlock()
		if ok {
			d.emit(ev)
		}
	})
}

// Stop cancels all pending timers and immediately emits their events.
// After Stop returns, subsequent Feed calls are no-ops.
func (d *Debouncer) Stop() {
	d.mu.Lock()
	d.stopped = true

	// Collect pending events and stop all timers.
	var toEmit []Event
	for path, t := range d.timers {
		t.Stop()
		if ev, ok := d.pending[path]; ok {
			toEmit = append(toEmit, ev)
		}
	}
	d.timers = nil
	d.pending = nil
	d.mu.Unlock()

	// Emit outside the lock to avoid potential deadlocks in callbacks.
	for _, ev := range toEmit {
		d.emit(ev)
	}
}
