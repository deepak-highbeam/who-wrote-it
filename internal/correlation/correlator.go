// Package correlation matches file system events to AI session events using
// time-window proximity. The output is a CorrelationResult for each file event
// which can then be fed into the authorship classifier.
package correlation

import (
	"time"

	"github.com/anthropic/who-wrote-it/internal/authorship"
	"github.com/anthropic/who-wrote-it/internal/store"
)

// DefaultWindowMs is the default correlation window in milliseconds.
// File events within this window of a Write session event are considered
// correlated.
const DefaultWindowMs = 5000

// StoreReader is the minimal interface the correlator needs from the store.
// Defined here so the correlator does not depend on the concrete *store.Store.
type StoreReader interface {
	QueryFileEventsInWindow(filePath string, start, end time.Time) ([]store.FileEvent, error)
	QueryFileEventsByProject(projectPath string, since time.Time) ([]store.FileEvent, error)
	QuerySessionEventsInWindow(filePath string, start, end time.Time) ([]store.StoredSessionEvent, error)
	QuerySessionEventsNearTimestamp(timestamp time.Time, windowMs int) ([]store.StoredSessionEvent, error)
}

// Correlator matches file events to session events by time proximity.
type Correlator struct {
	store    StoreReader
	WindowMs int // Configurable window; defaults to DefaultWindowMs.
}

// New creates a new Correlator with the given store reader and default window.
func New(reader StoreReader) *Correlator {
	return &Correlator{
		store:    reader,
		WindowMs: DefaultWindowMs,
	}
}

// CorrelateFileEvent attempts to match a single file event to the closest
// Write session event.
//
// Matching strategy (in priority order):
//  1. Exact file match: look for Write session events on the same file_path
//     within the time window. Pick closest in time.
//  2. Time proximity: look for ANY Write session event within the time window
//     regardless of file path. Pick closest in time.
//  3. No match: no session activity detected near this file event.
func (c *Correlator) CorrelateFileEvent(fe store.FileEvent) (*authorship.CorrelationResult, error) {
	windowDur := time.Duration(c.WindowMs) * time.Millisecond
	start := fe.Timestamp.Add(-windowDur)
	end := fe.Timestamp.Add(windowDur)

	// Step 1: exact file match
	sessions, err := c.store.QuerySessionEventsInWindow(fe.FilePath, start, end)
	if err != nil {
		return nil, err
	}

	if len(sessions) > 0 {
		closest := pickClosest(fe.Timestamp, sessions)
		delta := absDurationMs(fe.Timestamp, closest.Timestamp)
		return &authorship.CorrelationResult{
			FileEvent:      fe,
			MatchedSession: &closest,
			TimeDeltaMs:    delta,
			MatchType:      "exact_file",
		}, nil
	}

	// Step 2: time proximity (any Write event in window)
	allSessions, err := c.store.QuerySessionEventsNearTimestamp(fe.Timestamp, c.WindowMs)
	if err != nil {
		return nil, err
	}

	if len(allSessions) > 0 {
		closest := pickClosest(fe.Timestamp, allSessions)
		delta := absDurationMs(fe.Timestamp, closest.Timestamp)
		return &authorship.CorrelationResult{
			FileEvent:      fe,
			MatchedSession: &closest,
			TimeDeltaMs:    delta,
			MatchType:      "time_proximity",
		}, nil
	}

	// Step 3: no match
	return &authorship.CorrelationResult{
		FileEvent:      fe,
		MatchedSession: nil,
		TimeDeltaMs:    0,
		MatchType:      "none",
	}, nil
}

// CorrelateAll correlates all file events for a project since the given time.
func (c *Correlator) CorrelateAll(projectPath string, since time.Time) ([]authorship.CorrelationResult, error) {
	events, err := c.store.QueryFileEventsByProject(projectPath, since)
	if err != nil {
		return nil, err
	}

	results := make([]authorship.CorrelationResult, 0, len(events))
	for _, fe := range events {
		cr, err := c.store.QuerySessionEventsInWindow(fe.FilePath, fe.Timestamp.Add(-time.Duration(c.WindowMs)*time.Millisecond), fe.Timestamp.Add(time.Duration(c.WindowMs)*time.Millisecond))
		if err != nil {
			return nil, err
		}

		var result authorship.CorrelationResult
		result.FileEvent = fe

		if len(cr) > 0 {
			closest := pickClosest(fe.Timestamp, cr)
			result.MatchedSession = &closest
			result.TimeDeltaMs = absDurationMs(fe.Timestamp, closest.Timestamp)
			result.MatchType = "exact_file"
		} else {
			// Try time proximity
			allSessions, err := c.store.QuerySessionEventsNearTimestamp(fe.Timestamp, c.WindowMs)
			if err != nil {
				return nil, err
			}
			if len(allSessions) > 0 {
				closest := pickClosest(fe.Timestamp, allSessions)
				result.MatchedSession = &closest
				result.TimeDeltaMs = absDurationMs(fe.Timestamp, closest.Timestamp)
				result.MatchType = "time_proximity"
			} else {
				result.MatchType = "none"
			}
		}

		results = append(results, result)
	}

	return results, nil
}

// pickClosest returns the session event closest in time to the reference timestamp.
func pickClosest(ref time.Time, sessions []store.StoredSessionEvent) store.StoredSessionEvent {
	best := sessions[0]
	bestDelta := absDuration(ref, best.Timestamp)
	for _, s := range sessions[1:] {
		d := absDuration(ref, s.Timestamp)
		if d < bestDelta {
			best = s
			bestDelta = d
		}
	}
	return best
}

// absDuration returns the absolute duration between two times.
func absDuration(a, b time.Time) time.Duration {
	d := a.Sub(b)
	if d < 0 {
		return -d
	}
	return d
}

// absDurationMs returns the absolute milliseconds between two times.
func absDurationMs(a, b time.Time) int64 {
	return absDuration(a, b).Milliseconds()
}
