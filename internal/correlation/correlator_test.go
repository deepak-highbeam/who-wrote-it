package correlation

import (
	"testing"
	"time"

	"github.com/anthropic/who-wrote-it/internal/store"
)

// ---------------------------------------------------------------------------
// Mock StoreReader
// ---------------------------------------------------------------------------

type mockStoreReader struct {
	fileEvents    []store.FileEvent
	sessionEvents []store.StoredSessionEvent
	nearEvents    []store.StoredSessionEvent
}

func (m *mockStoreReader) QueryFileEventsInWindow(filePath string, start, end time.Time) ([]store.FileEvent, error) {
	var out []store.FileEvent
	for _, fe := range m.fileEvents {
		if fe.FilePath == filePath &&
			!fe.Timestamp.Before(start) &&
			!fe.Timestamp.After(end) {
			out = append(out, fe)
		}
	}
	return out, nil
}

func (m *mockStoreReader) QueryFileEventsByProject(projectPath string, since time.Time) ([]store.FileEvent, error) {
	var out []store.FileEvent
	for _, fe := range m.fileEvents {
		if fe.ProjectPath == projectPath && !fe.Timestamp.Before(since) {
			out = append(out, fe)
		}
	}
	return out, nil
}

func (m *mockStoreReader) QuerySessionEventsInWindow(filePath string, start, end time.Time) ([]store.StoredSessionEvent, error) {
	var out []store.StoredSessionEvent
	for _, se := range m.sessionEvents {
		if se.FilePath == filePath &&
			!se.Timestamp.Before(start) &&
			!se.Timestamp.After(end) {
			out = append(out, se)
		}
	}
	return out, nil
}

func (m *mockStoreReader) QuerySessionEventsNearTimestamp(timestamp time.Time, windowMs int) ([]store.StoredSessionEvent, error) {
	windowDur := time.Duration(windowMs) * time.Millisecond
	start := timestamp.Add(-windowDur)
	end := timestamp.Add(windowDur)

	var out []store.StoredSessionEvent
	for _, se := range m.nearEvents {
		if !se.Timestamp.Before(start) && !se.Timestamp.After(end) {
			out = append(out, se)
		}
	}
	return out, nil
}


// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// baseTime is a fixed reference for deterministic tests.
var baseTime = time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC)

func TestCorrelateFileEvent_ExactFileMatchWithin1s(t *testing.T) {
	// File event at T, session Write event for same file at T+500ms.
	fe := store.FileEvent{
		ID: 1, ProjectPath: "/proj", FilePath: "foo.go",
		EventType: "write", Timestamp: baseTime,
	}
	se := store.StoredSessionEvent{
		ID: 10, SessionID: "s1", EventType: "tool_use", ToolName: "Write",
		FilePath: "foo.go", Timestamp: baseTime.Add(500 * time.Millisecond),
	}

	mock := &mockStoreReader{
		sessionEvents: []store.StoredSessionEvent{se},
	}

	c := New(mock)
	result, err := c.CorrelateFileEvent(fe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.MatchType != "exact_file" {
		t.Errorf("MatchType = %q, want %q", result.MatchType, "exact_file")
	}
	if result.TimeDeltaMs != 500 {
		t.Errorf("TimeDeltaMs = %d, want 500", result.TimeDeltaMs)
	}
	if result.MatchedSession == nil {
		t.Fatal("MatchedSession is nil, want non-nil")
	}
	if result.MatchedSession.ID != 10 {
		t.Errorf("MatchedSession.ID = %d, want 10", result.MatchedSession.ID)
	}
}

func TestCorrelateFileEvent_ClosestWins(t *testing.T) {
	// File event at T, two session events: T-3s and T+1s. T+1s is closer.
	fe := store.FileEvent{
		ID: 1, ProjectPath: "/proj", FilePath: "foo.go",
		EventType: "write", Timestamp: baseTime,
	}
	se1 := store.StoredSessionEvent{
		ID: 10, SessionID: "s1", EventType: "tool_use", ToolName: "Write",
		FilePath: "foo.go", Timestamp: baseTime.Add(-3 * time.Second),
	}
	se2 := store.StoredSessionEvent{
		ID: 11, SessionID: "s1", EventType: "tool_use", ToolName: "Write",
		FilePath: "foo.go", Timestamp: baseTime.Add(1 * time.Second),
	}

	mock := &mockStoreReader{
		sessionEvents: []store.StoredSessionEvent{se1, se2},
	}

	c := New(mock)
	result, err := c.CorrelateFileEvent(fe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.MatchType != "exact_file" {
		t.Errorf("MatchType = %q, want %q", result.MatchType, "exact_file")
	}
	if result.MatchedSession == nil {
		t.Fatal("MatchedSession is nil")
	}
	if result.MatchedSession.ID != 11 {
		t.Errorf("MatchedSession.ID = %d, want 11 (closer event)", result.MatchedSession.ID)
	}
	if result.TimeDeltaMs != 1000 {
		t.Errorf("TimeDeltaMs = %d, want 1000", result.TimeDeltaMs)
	}
}

func TestCorrelateFileEvent_NoSessionEvents(t *testing.T) {
	fe := store.FileEvent{
		ID: 1, ProjectPath: "/proj", FilePath: "foo.go",
		EventType: "write", Timestamp: baseTime,
	}

	mock := &mockStoreReader{} // no session events at all

	c := New(mock)
	result, err := c.CorrelateFileEvent(fe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.MatchType != "none" {
		t.Errorf("MatchType = %q, want %q", result.MatchType, "none")
	}
	if result.MatchedSession != nil {
		t.Error("MatchedSession should be nil for no match")
	}
	if result.TimeDeltaMs != 0 {
		t.Errorf("TimeDeltaMs = %d, want 0", result.TimeDeltaMs)
	}
}

func TestCorrelateFileEvent_FuzzyFileMatch(t *testing.T) {
	// File event for "foo.go" (relative), no exact session match,
	// but Claude wrote "/abs/path/to/foo.go" (same basename, different prefix).
	fe := store.FileEvent{
		ID: 1, ProjectPath: "/proj", FilePath: "foo.go",
		EventType: "write", Timestamp: baseTime,
	}
	fuzzySe := store.StoredSessionEvent{
		ID: 20, SessionID: "s1", EventType: "tool_use", ToolName: "Write",
		FilePath: "/abs/path/to/foo.go", Timestamp: baseTime.Add(2 * time.Second),
	}

	mock := &mockStoreReader{
		// No exact match for "foo.go"
		sessionEvents: []store.StoredSessionEvent{},
		// But "/abs/path/to/foo.go" is nearby — same file, different prefix
		nearEvents: []store.StoredSessionEvent{fuzzySe},
	}

	c := New(mock)
	result, err := c.CorrelateFileEvent(fe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.MatchType != "fuzzy_file" {
		t.Errorf("MatchType = %q, want %q", result.MatchType, "fuzzy_file")
	}
	if result.MatchedSession == nil {
		t.Fatal("MatchedSession is nil, want non-nil for fuzzy_file")
	}
	if result.MatchedSession.ID != 20 {
		t.Errorf("MatchedSession.ID = %d, want 20", result.MatchedSession.ID)
	}
}

func TestCorrelateFileEvent_DifferentFileNoFuzzyMatch(t *testing.T) {
	// File event for "foo.go", Claude wrote "bar.go" nearby in time.
	// These are different files — should NOT match via fuzzy.
	fe := store.FileEvent{
		ID: 1, ProjectPath: "/proj", FilePath: "foo.go",
		EventType: "write", Timestamp: baseTime,
	}
	differentSe := store.StoredSessionEvent{
		ID: 20, SessionID: "s1", EventType: "tool_use", ToolName: "Write",
		FilePath: "bar.go", Timestamp: baseTime.Add(2 * time.Second),
	}

	mock := &mockStoreReader{
		sessionEvents: []store.StoredSessionEvent{},
		nearEvents:    []store.StoredSessionEvent{differentSe},
	}

	c := New(mock)
	result, err := c.CorrelateFileEvent(fe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// bar.go is a completely different file — should fall through to "none"
	if result.MatchType != "none" {
		t.Errorf("MatchType = %q, want %q (different file should not fuzzy match)", result.MatchType, "none")
	}
}

func TestCorrelateFileEvent_OutsideWindow(t *testing.T) {
	// File event at T, session event at T+6s (outside 5s window).
	fe := store.FileEvent{
		ID: 1, ProjectPath: "/proj", FilePath: "foo.go",
		EventType: "write", Timestamp: baseTime,
	}

	// The session event is at T+6s -- outside the 5s window, so the mock
	// will not return it (filtering by window boundaries).
	mock := &mockStoreReader{
		sessionEvents: []store.StoredSessionEvent{},
		nearEvents:    []store.StoredSessionEvent{},
	}

	c := New(mock)
	result, err := c.CorrelateFileEvent(fe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.MatchType != "none" {
		t.Errorf("MatchType = %q, want %q", result.MatchType, "none")
	}
	if result.MatchedSession != nil {
		t.Error("MatchedSession should be nil when event is outside window")
	}
}

func TestCorrelateAll_MultipleFileEvents(t *testing.T) {
	fe1 := store.FileEvent{
		ID: 1, ProjectPath: "/proj", FilePath: "a.go",
		EventType: "write", Timestamp: baseTime,
	}
	fe2 := store.FileEvent{
		ID: 2, ProjectPath: "/proj", FilePath: "b.go",
		EventType: "write", Timestamp: baseTime.Add(10 * time.Second),
	}
	se := store.StoredSessionEvent{
		ID: 10, SessionID: "s1", EventType: "tool_use", ToolName: "Write",
		FilePath: "a.go", Timestamp: baseTime.Add(500 * time.Millisecond),
	}

	mock := &mockStoreReader{
		fileEvents:    []store.FileEvent{fe1, fe2},
		sessionEvents: []store.StoredSessionEvent{se},
	}

	c := New(mock)
	results, err := c.CorrelateAll("/proj", baseTime.Add(-1*time.Second))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	// First event should match (same file)
	if results[0].MatchType != "exact_file" {
		t.Errorf("results[0].MatchType = %q, want %q", results[0].MatchType, "exact_file")
	}

	// Second event should have no match (different file, no nearby events)
	if results[1].MatchType != "none" {
		t.Errorf("results[1].MatchType = %q, want %q", results[1].MatchType, "none")
	}
}

func TestCorrelateFileEvent_ExactMatchPreferredOverProximity(t *testing.T) {
	// Both exact file match and proximity match available. Exact should win.
	fe := store.FileEvent{
		ID: 1, ProjectPath: "/proj", FilePath: "foo.go",
		EventType: "write", Timestamp: baseTime,
	}
	exactSe := store.StoredSessionEvent{
		ID: 10, SessionID: "s1", EventType: "tool_use", ToolName: "Write",
		FilePath: "foo.go", Timestamp: baseTime.Add(4 * time.Second),
	}
	proximitySe := store.StoredSessionEvent{
		ID: 20, SessionID: "s1", EventType: "tool_use", ToolName: "Write",
		FilePath: "bar.go", Timestamp: baseTime.Add(500 * time.Millisecond),
	}

	mock := &mockStoreReader{
		sessionEvents: []store.StoredSessionEvent{exactSe},
		nearEvents:    []store.StoredSessionEvent{proximitySe, exactSe},
	}

	c := New(mock)
	result, err := c.CorrelateFileEvent(fe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should match exact, not proximity, even though proximity is closer in time
	if result.MatchType != "exact_file" {
		t.Errorf("MatchType = %q, want %q (exact preferred over proximity)", result.MatchType, "exact_file")
	}
	if result.MatchedSession.ID != 10 {
		t.Errorf("MatchedSession.ID = %d, want 10 (exact match)", result.MatchedSession.ID)
	}
}

// ---------------------------------------------------------------------------
// Helper tests
// ---------------------------------------------------------------------------

func TestPickClosest(t *testing.T) {
	ref := baseTime
	sessions := []store.StoredSessionEvent{
		{ID: 1, Timestamp: baseTime.Add(-3 * time.Second)},
		{ID: 2, Timestamp: baseTime.Add(1 * time.Second)},
		{ID: 3, Timestamp: baseTime.Add(2 * time.Second)},
	}

	got := pickClosest(ref, sessions)
	if got.ID != 2 {
		t.Errorf("pickClosest returned ID=%d, want 2 (closest to ref)", got.ID)
	}
}

func TestPathSuffixMatch(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"foo.go", "/abs/path/to/foo.go", true},
		{"src/main.go", "/project/src/main.go", true},
		{"/project/src/main.go", "src/main.go", true},
		{"foo.go", "foo.go", true},
		{"foo.go", "bar.go", false},
		{"/a/b/handler.go", "/x/y/handler.go", false}, // different parent dir
		{"main.go", "omain.go", false},                 // suffix of name, not path
		{"", "foo.go", false},
	}

	for _, tt := range tests {
		got := pathSuffixMatch(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("pathSuffixMatch(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestAbsDurationMs(t *testing.T) {
	a := baseTime
	b := baseTime.Add(-2500 * time.Millisecond)
	if got := absDurationMs(a, b); got != 2500 {
		t.Errorf("absDurationMs = %d, want 2500", got)
	}
	if got := absDurationMs(b, a); got != 2500 {
		t.Errorf("absDurationMs (reversed) = %d, want 2500", got)
	}
}
