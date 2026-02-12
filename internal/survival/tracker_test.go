package survival

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anthropic/who-wrote-it/internal/store"
)

var baseTime = time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC)

func setupTestStore(t *testing.T) (*store.Store, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "survival-test-*")
	if err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(dir, "test.db")
	s, err := store.New(dbPath)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatal(err)
	}

	cleanup := func() {
		s.Close()
		os.RemoveAll(dir)
	}

	return s, cleanup
}

func almostEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// insertTestData sets up attributions with session events and blame lines
// for testing the survival analyzer.
func insertTestData(t *testing.T, s *store.Store) {
	t.Helper()

	// Insert session events (AI tool writes) with content hashes.
	if err := s.InsertSessionEvent("sess1", "tool_result", "Write", "main.go", "hash_a", baseTime, "{}", 0); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertSessionEvent("sess1", "tool_result", "Write", "main.go", "hash_b", baseTime.Add(time.Second), "{}", 0); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertSessionEvent("sess1", "tool_result", "Write", "util.go", "hash_c", baseTime.Add(2*time.Second), "{}", 0); err != nil {
		t.Fatal(err)
	}

	// Insert file events for attribution references.
	if err := s.InsertFileEvent("/proj", "main.go", "write", baseTime); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertFileEvent("/proj", "main.go", "write", baseTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertFileEvent("/proj", "util.go", "write", baseTime.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}

	// Session event IDs (1, 2, 3 for the three inserts above).
	seID1 := int64(1)
	seID2 := int64(2)
	seID3 := int64(3)

	// Insert attributions: 2 for main.go (fully_ai), 1 for util.go (ai_first_human_revised).
	attr1 := store.AttributionRecord{
		FilePath:        "main.go",
		ProjectPath:     "/proj",
		SessionEventID:  &seID1,
		AuthorshipLevel: "fully_ai",
		Confidence:      0.95,
		Timestamp:       baseTime,
	}
	id1, err := s.InsertAttribution(attr1)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateAttributionWorkType(id1, "core_logic"); err != nil {
		t.Fatal(err)
	}

	attr2 := store.AttributionRecord{
		FilePath:        "main.go",
		ProjectPath:     "/proj",
		SessionEventID:  &seID2,
		AuthorshipLevel: "fully_ai",
		Confidence:      0.95,
		Timestamp:       baseTime.Add(time.Second),
	}
	id2, err := s.InsertAttribution(attr2)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateAttributionWorkType(id2, "core_logic"); err != nil {
		t.Fatal(err)
	}

	attr3 := store.AttributionRecord{
		FilePath:        "util.go",
		ProjectPath:     "/proj",
		SessionEventID:  &seID3,
		AuthorshipLevel: "ai_first_human_revised",
		Confidence:      0.8,
		Timestamp:       baseTime.Add(2 * time.Second),
	}
	id3, err := s.InsertAttribution(attr3)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateAttributionWorkType(id3, "boilerplate"); err != nil {
		t.Fatal(err)
	}

	// Also insert a fully_human attribution that should be IGNORED by survival analysis.
	feID := int64(3) // file event for util.go
	attr4 := store.AttributionRecord{
		FilePath:        "util.go",
		ProjectPath:     "/proj",
		FileEventID:     &feID,
		AuthorshipLevel: "fully_human",
		Confidence:      0.9,
		Timestamp:       baseTime.Add(3 * time.Second),
	}
	id4, err := s.InsertAttribution(attr4)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateAttributionWorkType(id4, "boilerplate"); err != nil {
		t.Fatal(err)
	}

	// Insert blame lines.
	// main.go: hash_a survived, hash_b changed.
	mainBlame := []store.BlameLine{
		{LineNumber: 1, CommitHash: "abc123", Author: "dev", ContentHash: "hash_a"},
		{LineNumber: 2, CommitHash: "abc123", Author: "dev", ContentHash: "hash_x"}, // changed
		{LineNumber: 3, CommitHash: "abc123", Author: "dev", ContentHash: "hash_y"},
	}
	if err := s.InsertBlameLines("main.go", mainBlame); err != nil {
		t.Fatal(err)
	}

	// util.go: hash_c survived.
	utilBlame := []store.BlameLine{
		{LineNumber: 1, CommitHash: "def456", Author: "dev", ContentHash: "hash_c"},
		{LineNumber: 2, CommitHash: "def456", Author: "dev", ContentHash: "hash_z"},
	}
	if err := s.InsertBlameLines("util.go", utilBlame); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyze_BasicSurvival(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	insertTestData(t, s)

	sr, err := Analyze(s, "/proj")
	if err != nil {
		t.Fatal(err)
	}

	// 3 AI attributions tracked (2 fully_ai for main.go, 1 ai_first for util.go).
	// fully_human attribution is excluded.
	if sr.TotalTracked != 3 {
		t.Errorf("TotalTracked = %d, want 3", sr.TotalTracked)
	}

	// hash_a survived (in main.go blame), hash_b did not, hash_c survived (in util.go blame).
	if sr.SurvivedCount != 2 {
		t.Errorf("SurvivedCount = %d, want 2", sr.SurvivedCount)
	}

	expectedRate := 2.0 / 3.0 * 100.0
	if !almostEqual(sr.SurvivalRate, expectedRate, 0.1) {
		t.Errorf("SurvivalRate = %.1f, want %.1f", sr.SurvivalRate, expectedRate)
	}

	// By authorship: fully_ai tracked 2, survived 1 (hash_a).
	fullyAI := sr.ByAuthorship["fully_ai"]
	if fullyAI.Tracked != 2 {
		t.Errorf("ByAuthorship[fully_ai].Tracked = %d, want 2", fullyAI.Tracked)
	}
	if fullyAI.Survived != 1 {
		t.Errorf("ByAuthorship[fully_ai].Survived = %d, want 1", fullyAI.Survived)
	}

	// ai_first_human_revised tracked 1, survived 1 (hash_c).
	aiFirst := sr.ByAuthorship["ai_first_human_revised"]
	if aiFirst.Tracked != 1 {
		t.Errorf("ByAuthorship[ai_first_human_revised].Tracked = %d, want 1", aiFirst.Tracked)
	}
	if aiFirst.Survived != 1 {
		t.Errorf("ByAuthorship[ai_first_human_revised].Survived = %d, want 1", aiFirst.Survived)
	}

	// By work type: core_logic tracked 2, survived 1.
	cl := sr.ByWorkType["core_logic"]
	if cl.Tracked != 2 {
		t.Errorf("ByWorkType[core_logic].Tracked = %d, want 2", cl.Tracked)
	}
	if cl.Survived != 1 {
		t.Errorf("ByWorkType[core_logic].Survived = %d, want 1", cl.Survived)
	}

	// boilerplate tracked 1, survived 1.
	bp := sr.ByWorkType["boilerplate"]
	if bp.Tracked != 1 {
		t.Errorf("ByWorkType[boilerplate].Tracked = %d, want 1", bp.Tracked)
	}
	if bp.Survived != 1 {
		t.Errorf("ByWorkType[boilerplate].Survived = %d, want 1", bp.Survived)
	}
}

func TestAnalyze_NoBlameData(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	// Insert attributions but NO blame lines.
	if err := s.InsertSessionEvent("sess1", "tool_result", "Write", "main.go", "hash_a", baseTime, "{}", 0); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertFileEvent("/proj", "main.go", "write", baseTime); err != nil {
		t.Fatal(err)
	}

	seID := int64(1)
	attr := store.AttributionRecord{
		FilePath:        "main.go",
		ProjectPath:     "/proj",
		SessionEventID:  &seID,
		AuthorshipLevel: "fully_ai",
		Confidence:      0.95,
		Timestamp:       baseTime,
	}
	id, err := s.InsertAttribution(attr)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateAttributionWorkType(id, "core_logic"); err != nil {
		t.Fatal(err)
	}

	sr, err := Analyze(s, "/proj")
	if err != nil {
		t.Fatal(err)
	}

	// No blame data means file is skipped entirely.
	if sr.TotalTracked != 0 {
		t.Errorf("TotalTracked = %d, want 0 (no blame data)", sr.TotalTracked)
	}
}

func TestAnalyze_EmptyProject(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	sr, err := Analyze(s, "/proj")
	if err != nil {
		t.Fatal(err)
	}

	if sr.TotalTracked != 0 {
		t.Errorf("TotalTracked = %d, want 0", sr.TotalTracked)
	}
	if sr.SurvivalRate != 0.0 {
		t.Errorf("SurvivalRate = %f, want 0", sr.SurvivalRate)
	}
}
