package daemon

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/anthropic/who-wrote-it/internal/authorship"
	"github.com/anthropic/who-wrote-it/internal/correlation"
	"github.com/anthropic/who-wrote-it/internal/store"
	"github.com/anthropic/who-wrote-it/internal/worktype"
)

// TestAttributionPipeline exercises the full attribution pipeline end-to-end:
// file event + session event -> correlation -> classify -> work-type -> store.
func TestAttributionPipeline(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC().Truncate(time.Millisecond)

	// Insert a file event.
	if err := s.InsertFileEvent("/testproject", "/testproject/main.go", "write", now); err != nil {
		t.Fatalf("InsertFileEvent: %v", err)
	}

	// Insert a session event on the same file at the same timestamp.
	if err := s.InsertSessionEvent("sess1", "tool_use", "Write", "/testproject/main.go", "abc123", now, "{}", 10); err != nil {
		t.Fatalf("InsertSessionEvent: %v", err)
	}

	// Create pipeline components.
	correlator := correlation.New(s)
	classifier := authorship.NewClassifier()
	wtClassifier := worktype.NewClassifier(s)

	// Query unprocessed events -- should return 1.
	events, err := s.QueryUnprocessedFileEvents(100)
	if err != nil {
		t.Fatalf("QueryUnprocessedFileEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 unprocessed event, got %d", len(events))
	}

	fe := events[0]

	// Step 1: Correlate.
	result, err := correlator.CorrelateFileEvent(fe)
	if err != nil {
		t.Fatalf("CorrelateFileEvent: %v", err)
	}
	if result == nil {
		t.Fatal("CorrelateFileEvent returned nil result")
	}
	if result.MatchType != "exact_file" {
		t.Errorf("MatchType = %q, want %q", result.MatchType, "exact_file")
	}

	// Step 2: Classify authorship.
	attr := classifier.Classify(*result)
	if attr.Level != authorship.MostlyAI {
		t.Errorf("Level = %q, want %q", attr.Level, authorship.MostlyAI)
	}
	if attr.Confidence != 0.95 {
		t.Errorf("Confidence = %f, want 0.95", attr.Confidence)
	}
	if attr.FirstAuthor != "ai" {
		t.Errorf("FirstAuthor = %q, want %q", attr.FirstAuthor, "ai")
	}

	// Step 3: Classify work type.
	wt := wtClassifier.ClassifyFile(fe.FilePath, "", "")
	if wt != worktype.CoreLogic {
		t.Errorf("WorkType = %q, want %q", wt, worktype.CoreLogic)
	}

	// Step 4: Persist attribution.
	record := store.AttributionRecord{
		FilePath:            attr.FilePath,
		ProjectPath:         attr.ProjectPath,
		FileEventID:         attr.FileEventID,
		SessionEventID:      attr.SessionEventID,
		AuthorshipLevel:     string(attr.Level),
		Confidence:          attr.Confidence,
		Uncertain:           attr.Uncertain,
		FirstAuthor:         attr.FirstAuthor,
		CorrelationWindowMs: attr.CorrelationWindowMs,
		Timestamp:           attr.Timestamp,
	}
	id, err := s.InsertAttribution(record)
	if err != nil {
		t.Fatalf("InsertAttribution: %v", err)
	}
	if id <= 0 {
		t.Fatalf("InsertAttribution returned id %d, want > 0", id)
	}

	// Step 5: Set work type.
	if err := s.UpdateAttributionWorkType(id, string(wt)); err != nil {
		t.Fatalf("UpdateAttributionWorkType: %v", err)
	}

	// Verify: no more unprocessed events.
	remaining, err := s.QueryUnprocessedFileEvents(100)
	if err != nil {
		t.Fatalf("QueryUnprocessedFileEvents (after): %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected 0 unprocessed events after processing, got %d", len(remaining))
	}

	// Verify: attribution is stored correctly.
	attributions, err := s.QueryAttributionsByProject("/testproject")
	if err != nil {
		t.Fatalf("QueryAttributionsByProject: %v", err)
	}
	if len(attributions) != 1 {
		t.Fatalf("expected 1 attribution, got %d", len(attributions))
	}

	a := attributions[0]
	if a.AuthorshipLevel != "mostly_ai" {
		t.Errorf("stored AuthorshipLevel = %q, want %q", a.AuthorshipLevel, "mostly_ai")
	}
	if a.FirstAuthor != "ai" {
		t.Errorf("stored FirstAuthor = %q, want %q", a.FirstAuthor, "ai")
	}
	if a.Confidence != 0.95 {
		t.Errorf("stored Confidence = %f, want 0.95", a.Confidence)
	}
	if a.ProjectPath != "/testproject" {
		t.Errorf("stored ProjectPath = %q, want %q", a.ProjectPath, "/testproject")
	}
}

// TestAttributionPipelineNoSessionMatch verifies that file events without
// matching session events produce fully_human attributions.
func TestAttributionPipelineNoSessionMatch(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC().Truncate(time.Millisecond)

	// Insert a file event only (no session event).
	if err := s.InsertFileEvent("/testproject", "/testproject/handler.go", "write", now); err != nil {
		t.Fatalf("InsertFileEvent: %v", err)
	}

	// Create pipeline components.
	correlator := correlation.New(s)
	classifier := authorship.NewClassifier()
	wtClassifier := worktype.NewClassifier(s)

	// Query unprocessed events.
	events, err := s.QueryUnprocessedFileEvents(100)
	if err != nil {
		t.Fatalf("QueryUnprocessedFileEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 unprocessed event, got %d", len(events))
	}

	fe := events[0]

	// Correlate -- should find no match.
	result, err := correlator.CorrelateFileEvent(fe)
	if err != nil {
		t.Fatalf("CorrelateFileEvent: %v", err)
	}
	if result.MatchType != "none" {
		t.Errorf("MatchType = %q, want %q", result.MatchType, "none")
	}

	// Classify -- should be fully human.
	attr := classifier.Classify(*result)
	if attr.Level != authorship.MostlyHuman {
		t.Errorf("Level = %q, want %q", attr.Level, authorship.MostlyHuman)
	}
	if attr.Confidence != 1.0 {
		t.Errorf("Confidence = %f, want 1.0", attr.Confidence)
	}
	if attr.FirstAuthor != "human" {
		t.Errorf("FirstAuthor = %q, want %q", attr.FirstAuthor, "human")
	}

	// Work type classification still works.
	wt := wtClassifier.ClassifyFile(fe.FilePath, "", "")
	if wt != worktype.CoreLogic {
		t.Errorf("WorkType = %q, want %q", wt, worktype.CoreLogic)
	}

	// Persist.
	record := store.AttributionRecord{
		FilePath:            attr.FilePath,
		ProjectPath:         attr.ProjectPath,
		FileEventID:         attr.FileEventID,
		SessionEventID:      attr.SessionEventID,
		AuthorshipLevel:     string(attr.Level),
		Confidence:          attr.Confidence,
		Uncertain:           attr.Uncertain,
		FirstAuthor:         attr.FirstAuthor,
		CorrelationWindowMs: attr.CorrelationWindowMs,
		Timestamp:           attr.Timestamp,
	}
	id, err := s.InsertAttribution(record)
	if err != nil {
		t.Fatalf("InsertAttribution: %v", err)
	}
	if id <= 0 {
		t.Fatalf("InsertAttribution returned id %d, want > 0", id)
	}

	if err := s.UpdateAttributionWorkType(id, string(wt)); err != nil {
		t.Fatalf("UpdateAttributionWorkType: %v", err)
	}

	// Verify no more unprocessed events.
	remaining, err := s.QueryUnprocessedFileEvents(100)
	if err != nil {
		t.Fatalf("QueryUnprocessedFileEvents (after): %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected 0 unprocessed events after processing, got %d", len(remaining))
	}

	// Verify stored attribution.
	attributions, err := s.QueryAttributionsByProject("/testproject")
	if err != nil {
		t.Fatalf("QueryAttributionsByProject: %v", err)
	}
	if len(attributions) != 1 {
		t.Fatalf("expected 1 attribution, got %d", len(attributions))
	}

	a := attributions[0]
	if a.AuthorshipLevel != "mostly_human" {
		t.Errorf("stored AuthorshipLevel = %q, want %q", a.AuthorshipLevel, "mostly_human")
	}
	if a.FirstAuthor != "human" {
		t.Errorf("stored FirstAuthor = %q, want %q", a.FirstAuthor, "human")
	}
	if a.Confidence != 1.0 {
		t.Errorf("stored Confidence = %f, want 1.0", a.Confidence)
	}
}
