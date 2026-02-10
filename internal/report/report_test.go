package report

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anthropic/who-wrote-it/internal/store"
)

var baseTime = time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC)

// setupTestStore creates a temporary SQLite store with test data and returns
// the store and a cleanup function.
func setupTestStore(t *testing.T) (*store.Store, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "report-test-*")
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

// insertAttribution inserts a test attribution with work type into the store.
func insertAttribution(t *testing.T, s *store.Store, filePath, projectPath, level, workType string, ts time.Time) {
	t.Helper()

	// Insert a file event first so we have a valid reference.
	if err := s.InsertFileEvent(projectPath, filePath, "write", ts); err != nil {
		t.Fatalf("insert file event: %v", err)
	}

	attr := store.AttributionRecord{
		FilePath:            filePath,
		ProjectPath:         projectPath,
		AuthorshipLevel:     level,
		Confidence:          0.95,
		FirstAuthor:         "ai",
		CorrelationWindowMs: 100,
		Timestamp:           ts,
	}

	id, err := s.InsertAttribution(attr)
	if err != nil {
		t.Fatalf("insert attribution: %v", err)
	}

	// Set work type on the attribution.
	if err := s.UpdateAttributionWorkType(id, workType); err != nil {
		t.Fatalf("update work type: %v", err)
	}
}

func almostEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// ---------------------------------------------------------------------------
// GenerateProject tests
// ---------------------------------------------------------------------------

func TestGenerateProjectFromStore_BasicReport(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	// Insert attributions for two files in the same project.
	insertAttribution(t, s, "main.go", "/proj", "fully_ai", "core_logic", baseTime)
	insertAttribution(t, s, "main.go", "/proj", "fully_ai", "core_logic", baseTime.Add(time.Second))
	insertAttribution(t, s, "main.go", "/proj", "fully_human", "core_logic", baseTime.Add(2*time.Second))
	insertAttribution(t, s, "go.mod", "/proj", "fully_human", "boilerplate", baseTime.Add(3*time.Second))

	report, err := GenerateProjectFromStore(s)
	if err != nil {
		t.Fatal(err)
	}

	if report.ProjectPath != "/proj" {
		t.Errorf("ProjectPath = %q, want %q", report.ProjectPath, "/proj")
	}

	if report.TotalFiles != 2 {
		t.Errorf("TotalFiles = %d, want 2", report.TotalFiles)
	}

	// Raw: 2 AI / 4 total = 50%
	if !almostEqual(report.RawAIPct, 50.0, 0.01) {
		t.Errorf("RawAIPct = %f, want 50.0", report.RawAIPct)
	}

	// Meaningful: (2*3 + 0*1) / (3*3 + 1*1) = 6/10 = 60%
	if !almostEqual(report.MeaningfulAIPct, 60.0, 0.01) {
		t.Errorf("MeaningfulAIPct = %f, want 60.0", report.MeaningfulAIPct)
	}

	// Authorship breakdown.
	if report.ByAuthorship["fully_ai"] != 2 {
		t.Errorf("ByAuthorship[fully_ai] = %d, want 2", report.ByAuthorship["fully_ai"])
	}
	if report.ByAuthorship["fully_human"] != 2 {
		t.Errorf("ByAuthorship[fully_human] = %d, want 2", report.ByAuthorship["fully_human"])
	}

	// Work type breakdown.
	cl, ok := report.ByWorkType["core_logic"]
	if !ok {
		t.Fatal("missing core_logic in ByWorkType")
	}
	if cl.Files != 1 {
		t.Errorf("core_logic.Files = %d, want 1", cl.Files)
	}
	if cl.AIEvents != 2 {
		t.Errorf("core_logic.AIEvents = %d, want 2", cl.AIEvents)
	}

	bp, ok := report.ByWorkType["boilerplate"]
	if !ok {
		t.Fatal("missing boilerplate in ByWorkType")
	}
	if bp.Files != 1 {
		t.Errorf("boilerplate.Files = %d, want 1", bp.Files)
	}
}

func TestGenerateProjectFromStore_FilesSortedByAIPct(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	// File A: 0% AI, File B: 100% AI.
	insertAttribution(t, s, "a.go", "/proj", "fully_human", "core_logic", baseTime)
	insertAttribution(t, s, "b.go", "/proj", "fully_ai", "core_logic", baseTime.Add(time.Second))

	report, err := GenerateProjectFromStore(s)
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Files) != 2 {
		t.Fatalf("Files = %d, want 2", len(report.Files))
	}

	// b.go (100% AI) should come first.
	if report.Files[0].FilePath != "b.go" {
		t.Errorf("Files[0] = %q, want %q (highest AI%%)", report.Files[0].FilePath, "b.go")
	}
	if report.Files[1].FilePath != "a.go" {
		t.Errorf("Files[1] = %q, want %q (lowest AI%%)", report.Files[1].FilePath, "a.go")
	}
}

func TestGenerateProjectFromStore_NoData(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	_, err := GenerateProjectFromStore(s)
	if err == nil {
		t.Fatal("expected error for empty database")
	}
}

// ---------------------------------------------------------------------------
// GenerateFile tests
// ---------------------------------------------------------------------------

func TestGenerateFileFromStore_SingleFile(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	insertAttribution(t, s, "handler.go", "/proj", "fully_ai", "core_logic", baseTime)
	insertAttribution(t, s, "handler.go", "/proj", "ai_first_human_revised", "core_logic", baseTime.Add(time.Second))
	insertAttribution(t, s, "handler.go", "/proj", "fully_human", "core_logic", baseTime.Add(2*time.Second))

	fr, err := GenerateFileFromStore(s, "handler.go")
	if err != nil {
		t.Fatal(err)
	}

	if fr.FilePath != "handler.go" {
		t.Errorf("FilePath = %q, want %q", fr.FilePath, "handler.go")
	}
	if fr.WorkType != "core_logic" {
		t.Errorf("WorkType = %q, want %q", fr.WorkType, "core_logic")
	}
	if fr.TotalEvents != 3 {
		t.Errorf("TotalEvents = %d, want 3", fr.TotalEvents)
	}
	// AI events: fully_ai + ai_first_human_revised = 2.
	if fr.AIEventCount != 2 {
		t.Errorf("AIEventCount = %d, want 2", fr.AIEventCount)
	}
	if !almostEqual(fr.RawAIPct, 2.0/3.0*100.0, 0.01) {
		t.Errorf("RawAIPct = %f, want %f", fr.RawAIPct, 2.0/3.0*100.0)
	}
}

func TestGenerateFileFromStore_FileNotFound(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	_, err := GenerateFileFromStore(s, "nonexistent.go")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ---------------------------------------------------------------------------
// Format tests
// ---------------------------------------------------------------------------

func TestFormatJSON_ValidOutput(t *testing.T) {
	data := map[string]interface{}{
		"key":   "value",
		"count": 42,
	}

	output := FormatJSON(data)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("FormatJSON produced invalid JSON: %v\nOutput: %s", err, output)
	}

	if parsed["key"] != "value" {
		t.Errorf("parsed[key] = %v, want %q", parsed["key"], "value")
	}
}

func TestFormatJSON_ReportRoundTrip(t *testing.T) {
	report := &ProjectReport{
		ProjectPath:    "/proj",
		MeaningfulAIPct: 65.5,
		RawAIPct:       70.0,
		TotalFiles:     3,
		ByAuthorship:   map[string]int{"fully_ai": 5, "fully_human": 2},
		ByWorkType: map[string]WorkTypeSummary{
			"core_logic": {Files: 2, AIEvents: 4, TotalEvents: 5, AIPct: 80.0, Tier: "high", Weight: 3.0},
		},
		Files: []FileReport{
			{FilePath: "main.go", WorkType: "core_logic", MeaningfulAIPct: 100.0, TotalEvents: 3, AIEventCount: 3},
		},
	}

	output := FormatJSON(report)

	var parsed ProjectReport
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("FormatJSON report produced invalid JSON: %v", err)
	}

	if parsed.ProjectPath != "/proj" {
		t.Errorf("ProjectPath = %q, want %q", parsed.ProjectPath, "/proj")
	}
	if !almostEqual(parsed.MeaningfulAIPct, 65.5, 0.01) {
		t.Errorf("MeaningfulAIPct = %f, want 65.5", parsed.MeaningfulAIPct)
	}
}

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		got := humanBytes(tt.input)
		if got != tt.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatProjectReport_ContainsKey(t *testing.T) {
	report := &ProjectReport{
		ProjectPath:    "/proj",
		MeaningfulAIPct: 42.5,
		RawAIPct:       50.0,
		TotalFiles:     2,
		ByAuthorship:   map[string]int{"fully_ai": 3, "fully_human": 3},
		ByWorkType: map[string]WorkTypeSummary{
			"core_logic": {Files: 2, AIEvents: 3, TotalEvents: 6, AIPct: 50.0, Tier: "high", Weight: 3.0},
		},
		Files: []FileReport{
			{FilePath: "main.go", WorkType: "core_logic", MeaningfulAIPct: 100.0, TotalEvents: 3, AIEventCount: 3},
			{FilePath: "util.go", WorkType: "core_logic", MeaningfulAIPct: 0.0, TotalEvents: 3, AIEventCount: 0},
		},
	}

	output := FormatProjectReport(report)

	// Check key elements are present.
	checks := []string{
		"Attribution Report",
		"42.5%",
		"/proj",
		"core_logic",
		"main.go",
		"Authorship Spectrum",
		"Work Type Distribution",
	}
	for _, check := range checks {
		if !containsStr(output, check) {
			t.Errorf("FormatProjectReport output missing %q", check)
		}
	}
}

func TestFormatFileReport_ContainsKey(t *testing.T) {
	fr := &FileReport{
		FilePath:         "handler.go",
		WorkType:         "core_logic",
		MeaningfulAIPct:  66.7,
		RawAIPct:         66.7,
		TotalEvents:      3,
		AIEventCount:     2,
		AuthorshipCounts: map[string]int{"fully_ai": 1, "ai_first_human_revised": 1, "fully_human": 1},
	}

	output := FormatFileReport(fr)

	checks := []string{
		"File Report",
		"handler.go",
		"core_logic",
		"66.7%",
		"fully_ai",
	}
	for _, check := range checks {
		if !containsStr(output, check) {
			t.Errorf("FormatFileReport output missing %q", check)
		}
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
