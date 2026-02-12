// Package correlation matches file system events to AI session events using
// time-window proximity. The output is a CorrelationResult for each file event
// which can then be fed into the authorship classifier.
package correlation

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropic/who-wrote-it/internal/authorship"
	"github.com/anthropic/who-wrote-it/internal/store"
)

// DefaultWindowMs is the default correlation window in milliseconds.
// File events within this window of a Write/Edit session event are considered
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
// session event.
//
// Matching strategy (in priority order):
//  1. Exact file match: look for Write/Edit session events on the same
//     file_path within the time window. Pick closest in time.
//  2. Fuzzy file match: look for Write/Edit session events on a file with
//     the same name but different path prefix (handles path normalization,
//     relative vs absolute paths, etc.). Pick closest in time.
//  3. No match: no AI Write/Edit detected near this file event.
func (c *Correlator) CorrelateFileEvent(fe store.FileEvent) (*authorship.CorrelationResult, error) {
	windowDur := time.Duration(c.WindowMs) * time.Millisecond
	start := fe.Timestamp.Add(-windowDur)
	end := fe.Timestamp.Add(windowDur)

	// Step 1: exact file match (Write/Edit on same file)
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

	// Step 2: Fuzzy file match — Claude wrote a file with the same name but
	// different path prefix (handles path normalization, relative vs absolute, etc.)
	allSessions, err := c.store.QuerySessionEventsNearTimestamp(fe.Timestamp, c.WindowMs)
	if err != nil {
		return nil, err
	}

	if len(allSessions) > 0 {
		// Filter to sessions where the file path is a suffix match.
		var fuzzyMatches []store.StoredSessionEvent
		for _, se := range allSessions {
			if pathSuffixMatch(fe.FilePath, se.FilePath) {
				fuzzyMatches = append(fuzzyMatches, se)
			}
		}
		if len(fuzzyMatches) > 0 {
			closest := pickClosest(fe.Timestamp, fuzzyMatches)
			delta := absDurationMs(fe.Timestamp, closest.Timestamp)
			return &authorship.CorrelationResult{
				FileEvent:      fe,
				MatchedSession: &closest,
				TimeDeltaMs:    delta,
				MatchType:      "fuzzy_file",
			}, nil
		}
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
			// Try fuzzy file match (same name, different prefix)
			allSessions, err := c.store.QuerySessionEventsNearTimestamp(fe.Timestamp, c.WindowMs)
			if err != nil {
				return nil, err
			}
			var fuzzyMatches []store.StoredSessionEvent
			for _, se := range allSessions {
				if pathSuffixMatch(fe.FilePath, se.FilePath) {
					fuzzyMatches = append(fuzzyMatches, se)
				}
			}
			if len(fuzzyMatches) > 0 {
				closest := pickClosest(fe.Timestamp, fuzzyMatches)
				result.MatchedSession = &closest
				result.TimeDeltaMs = absDurationMs(fe.Timestamp, closest.Timestamp)
				result.MatchType = "fuzzy_file"
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

// pathSuffixMatch returns true if the two paths refer to the same file but
// with different prefixes. This handles path normalization issues like
// relative vs absolute paths, or symlinked directories.
//
// Examples:
//
//	pathSuffixMatch("foo.go", "/abs/path/to/foo.go")                        -> true
//	pathSuffixMatch("src/main.go", "/project/src/main.go")                  -> true
//	pathSuffixMatch("/a/b/handler.go", "/x/y/handler.go")                   -> false (different parent dir)
//	pathSuffixMatch("main.go", "other.go")                                  -> false
func pathSuffixMatch(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if a == b {
		return true
	}
	// Ensure the shorter path appears as a path-boundary suffix of the longer.
	// We prepend "/" to ensure we match at a directory boundary, not mid-name.
	if len(a) > len(b) {
		return strings.HasSuffix(a, string(filepath.Separator)+b)
	}
	return strings.HasSuffix(b, string(filepath.Separator)+a)
}
