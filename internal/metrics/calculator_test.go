package metrics

import (
	"math"
	"testing"
	"time"

	"github.com/anthropic/who-wrote-it/internal/store"
)

var baseTime = time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC)

func makeAttr(id int64, filePath, projectPath, level, workType string, linesChanged int) store.AttributionWithWorkType {
	return store.AttributionWithWorkType{
		AttributionRecord: store.AttributionRecord{
			ID:              id,
			FilePath:        filePath,
			ProjectPath:     projectPath,
			AuthorshipLevel: level,
			Confidence:      0.95,
			Timestamp:       baseTime,
			LinesChanged:    linesChanged,
		},
		WorkType: workType,
	}
}

func almostEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// ---------------------------------------------------------------------------
// ComputeFileMetrics tests
// ---------------------------------------------------------------------------

func TestComputeFileMetrics_AllAI(t *testing.T) {
	calc := NewCalculator()

	attrs := []store.AttributionWithWorkType{
		makeAttr(1, "foo.go", "/proj", "mostly_ai", "core_logic", 50),
		makeAttr(2, "foo.go", "/proj", "mostly_ai", "core_logic", 30),
		makeAttr(3, "foo.go", "/proj", "mostly_ai", "core_logic", 20),
	}

	fm := calc.ComputeFileMetrics("foo.go", attrs)

	if fm.TotalEvents != 3 {
		t.Errorf("TotalEvents = %d, want 3", fm.TotalEvents)
	}
	if fm.AIEventCount != 3 {
		t.Errorf("AIEventCount = %d, want 3", fm.AIEventCount)
	}
	if fm.TotalLines != 100 {
		t.Errorf("TotalLines = %d, want 100", fm.TotalLines)
	}
	if fm.AILines != 100 {
		t.Errorf("AILines = %d, want 100", fm.AILines)
	}
	if !almostEqual(fm.RawAIPct, 100.0, 0.01) {
		t.Errorf("RawAIPct = %f, want 100.0", fm.RawAIPct)
	}
	if fm.AuthorshipLevel != "mostly_ai" {
		t.Errorf("AuthorshipLevel = %q, want %q", fm.AuthorshipLevel, "mostly_ai")
	}
	if fm.WorkType != "core_logic" {
		t.Errorf("WorkType = %q, want %q", fm.WorkType, "core_logic")
	}
}

func TestComputeFileMetrics_LineWeighting(t *testing.T) {
	calc := NewCalculator()

	// AI writes 200 lines, human edits 2 lines. Without line weighting
	// that's 50/50, but with line weighting it's 200/202 = ~99%.
	attrs := []store.AttributionWithWorkType{
		makeAttr(1, "foo.go", "/proj", "mostly_ai", "core_logic", 200),
		makeAttr(2, "foo.go", "/proj", "mostly_human", "core_logic", 2),
	}

	fm := calc.ComputeFileMetrics("foo.go", attrs)

	if fm.TotalLines != 202 {
		t.Errorf("TotalLines = %d, want 202", fm.TotalLines)
	}
	if fm.AILines != 200 {
		t.Errorf("AILines = %d, want 200", fm.AILines)
	}
	expected := 200.0 / 202.0 * 100.0
	if !almostEqual(fm.RawAIPct, expected, 0.1) {
		t.Errorf("RawAIPct = %f, want %f (line-weighted)", fm.RawAIPct, expected)
	}
	if fm.AuthorshipLevel != "mostly_ai" {
		t.Errorf("AuthorshipLevel = %q, want %q (>70%% AI)", fm.AuthorshipLevel, "mostly_ai")
	}
}

func TestComputeFileMetrics_Mixed(t *testing.T) {
	calc := NewCalculator()

	attrs := []store.AttributionWithWorkType{
		makeAttr(1, "foo.go", "/proj", "mostly_ai", "architecture", 30),
		makeAttr(2, "foo.go", "/proj", "mostly_ai", "architecture", 20),
		makeAttr(3, "foo.go", "/proj", "mostly_human", "architecture", 50),
	}

	fm := calc.ComputeFileMetrics("foo.go", attrs)

	if fm.TotalLines != 100 {
		t.Errorf("TotalLines = %d, want 100", fm.TotalLines)
	}
	if fm.AILines != 50 {
		t.Errorf("AILines = %d, want 50", fm.AILines)
	}
	if !almostEqual(fm.RawAIPct, 50.0, 0.01) {
		t.Errorf("RawAIPct = %f, want 50.0", fm.RawAIPct)
	}
	if fm.AuthorshipLevel != "mixed" {
		t.Errorf("AuthorshipLevel = %q, want %q (30-70%%)", fm.AuthorshipLevel, "mixed")
	}
}

func TestComputeFileMetrics_ZeroEvents(t *testing.T) {
	calc := NewCalculator()

	fm := calc.ComputeFileMetrics("foo.go", nil)

	if fm.TotalEvents != 0 {
		t.Errorf("TotalEvents = %d, want 0", fm.TotalEvents)
	}
	if fm.RawAIPct != 0 {
		t.Errorf("RawAIPct = %f, want 0", fm.RawAIPct)
	}
	if fm.MeaningfulAIPct != 0 {
		t.Errorf("MeaningfulAIPct = %f, want 0", fm.MeaningfulAIPct)
	}
}

func TestComputeFileMetrics_BackwardCompatLegacyLevels(t *testing.T) {
	calc := NewCalculator()

	// Old data uses fully_ai and ai_first_human_revised which should still
	// count as AI in aiAuthorshipLevels.
	attrs := []store.AttributionWithWorkType{
		makeAttr(1, "foo.go", "/proj", "fully_ai", "core_logic", 0),
		makeAttr(2, "foo.go", "/proj", "ai_first_human_revised", "core_logic", 0),
		makeAttr(3, "foo.go", "/proj", "fully_human", "core_logic", 0),
	}

	fm := calc.ComputeFileMetrics("foo.go", attrs)

	// fully_ai + ai_first_human_revised = 2 AI events out of 3
	if fm.AIEventCount != 2 {
		t.Errorf("AIEventCount = %d, want 2 (legacy levels still count as AI)", fm.AIEventCount)
	}
	// With LinesChanged=0, effectiveLines returns 1 per event.
	if fm.TotalLines != 3 {
		t.Errorf("TotalLines = %d, want 3 (0 treated as 1)", fm.TotalLines)
	}
	if fm.AILines != 2 {
		t.Errorf("AILines = %d, want 2", fm.AILines)
	}
}

func TestComputeFileMetrics_ZeroLinesBackwardCompat(t *testing.T) {
	calc := NewCalculator()

	// Old data with LinesChanged=0 should be treated as 1 per event.
	attrs := []store.AttributionWithWorkType{
		makeAttr(1, "foo.go", "/proj", "mostly_ai", "core_logic", 0),
		makeAttr(2, "foo.go", "/proj", "mostly_human", "core_logic", 0),
	}

	fm := calc.ComputeFileMetrics("foo.go", attrs)

	if fm.TotalLines != 2 {
		t.Errorf("TotalLines = %d, want 2 (0 lines -> 1 per event)", fm.TotalLines)
	}
	if fm.AILines != 1 {
		t.Errorf("AILines = %d, want 1", fm.AILines)
	}
	if !almostEqual(fm.RawAIPct, 50.0, 0.01) {
		t.Errorf("RawAIPct = %f, want 50.0", fm.RawAIPct)
	}
}

func TestComputeFileMetrics_NoWorkType_DefaultsCoreLogic(t *testing.T) {
	calc := NewCalculator()

	attrs := []store.AttributionWithWorkType{
		makeAttr(1, "foo.go", "/proj", "mostly_ai", "", 10),
	}

	fm := calc.ComputeFileMetrics("foo.go", attrs)

	if fm.WorkType != "core_logic" {
		t.Errorf("WorkType = %q, want %q (default)", fm.WorkType, "core_logic")
	}
}

func TestComputeFileMetrics_ThreeLevelThresholds(t *testing.T) {
	calc := NewCalculator()

	// >70% -> mostly_ai
	attrs1 := []store.AttributionWithWorkType{
		makeAttr(1, "a.go", "/proj", "mostly_ai", "core_logic", 71),
		makeAttr(2, "a.go", "/proj", "mostly_human", "core_logic", 29),
	}
	fm1 := calc.ComputeFileMetrics("a.go", attrs1)
	if fm1.AuthorshipLevel != "mostly_ai" {
		t.Errorf("71%% AI -> AuthorshipLevel = %q, want mostly_ai", fm1.AuthorshipLevel)
	}

	// 30-70% -> mixed
	attrs2 := []store.AttributionWithWorkType{
		makeAttr(1, "b.go", "/proj", "mostly_ai", "core_logic", 50),
		makeAttr(2, "b.go", "/proj", "mostly_human", "core_logic", 50),
	}
	fm2 := calc.ComputeFileMetrics("b.go", attrs2)
	if fm2.AuthorshipLevel != "mixed" {
		t.Errorf("50%% AI -> AuthorshipLevel = %q, want mixed", fm2.AuthorshipLevel)
	}

	// <30% -> mostly_human
	attrs3 := []store.AttributionWithWorkType{
		makeAttr(1, "c.go", "/proj", "mostly_ai", "core_logic", 10),
		makeAttr(2, "c.go", "/proj", "mostly_human", "core_logic", 90),
	}
	fm3 := calc.ComputeFileMetrics("c.go", attrs3)
	if fm3.AuthorshipLevel != "mostly_human" {
		t.Errorf("10%% AI -> AuthorshipLevel = %q, want mostly_human", fm3.AuthorshipLevel)
	}
}

// ---------------------------------------------------------------------------
// ComputeProjectMetrics tests
// ---------------------------------------------------------------------------

func TestComputeProjectMetrics_WeightedCalculation(t *testing.T) {
	calc := NewCalculator()

	// File A: architecture (weight 3.0), 50 AI lines / 100 total lines
	// File B: boilerplate (weight 1.0), 30 AI lines / 30 total lines
	attrs := []store.AttributionWithWorkType{
		// File A - architecture
		makeAttr(1, "a.go", "/proj", "mostly_ai", "architecture", 25),
		makeAttr(2, "a.go", "/proj", "mostly_ai", "architecture", 25),
		makeAttr(3, "a.go", "/proj", "mostly_human", "architecture", 25),
		makeAttr(4, "a.go", "/proj", "mostly_human", "architecture", 25),
		// File B - boilerplate
		makeAttr(5, "go.mod", "/proj", "mostly_ai", "boilerplate", 10),
		makeAttr(6, "go.mod", "/proj", "mostly_ai", "boilerplate", 10),
		makeAttr(7, "go.mod", "/proj", "mostly_ai", "boilerplate", 10),
	}

	pm := calc.ComputeProjectMetrics("/proj", attrs)

	// RawAIPct = (50+30) / (100+30) = 80/130 = 61.5%
	expectedRaw := 80.0 / 130.0 * 100.0
	if !almostEqual(pm.RawAIPct, expectedRaw, 0.1) {
		t.Errorf("RawAIPct = %f, want %f", pm.RawAIPct, expectedRaw)
	}

	// MeaningfulAIPct = (50*3 + 30*1) / (100*3 + 30*1) = 180/330 = 54.5%
	expectedMeaningful := 180.0 / 330.0 * 100.0
	if !almostEqual(pm.MeaningfulAIPct, expectedMeaningful, 0.1) {
		t.Errorf("MeaningfulAIPct = %f, want %f", pm.MeaningfulAIPct, expectedMeaningful)
	}

	if pm.TotalFiles != 2 {
		t.Errorf("TotalFiles = %d, want 2", pm.TotalFiles)
	}
	if pm.TotalLines != 130 {
		t.Errorf("TotalLines = %d, want 130", pm.TotalLines)
	}
	if pm.AILines != 80 {
		t.Errorf("AILines = %d, want 80", pm.AILines)
	}
}

func TestComputeProjectMetrics_ZeroEvents(t *testing.T) {
	calc := NewCalculator()

	pm := calc.ComputeProjectMetrics("/proj", nil)

	if pm.MeaningfulAIPct != 0 {
		t.Errorf("MeaningfulAIPct = %f, want 0", pm.MeaningfulAIPct)
	}
	if pm.RawAIPct != 0 {
		t.Errorf("RawAIPct = %f, want 0", pm.RawAIPct)
	}
	if pm.TotalFiles != 0 {
		t.Errorf("TotalFiles = %d, want 0", pm.TotalFiles)
	}
}

func TestComputeProjectMetrics_WorkTypeBreakdown(t *testing.T) {
	calc := NewCalculator()

	attrs := []store.AttributionWithWorkType{
		makeAttr(1, "a.go", "/proj", "mostly_ai", "architecture", 10),
		makeAttr(2, "b.go", "/proj", "mostly_human", "core_logic", 10),
		makeAttr(3, "c_test.go", "/proj", "mostly_ai", "test_scaffolding", 10),
	}

	pm := calc.ComputeProjectMetrics("/proj", attrs)

	if len(pm.ByWorkType) != 3 {
		t.Errorf("ByWorkType has %d entries, want 3", len(pm.ByWorkType))
	}

	arch, ok := pm.ByWorkType["architecture"]
	if !ok {
		t.Fatal("missing architecture in ByWorkType")
	}
	if arch.FileCount != 1 {
		t.Errorf("architecture FileCount = %d, want 1", arch.FileCount)
	}
	if arch.Weight != 3.0 {
		t.Errorf("architecture Weight = %f, want 3.0", arch.Weight)
	}
	if arch.Tier != "high" {
		t.Errorf("architecture Tier = %q, want %q", arch.Tier, "high")
	}
	if arch.TotalLines != 10 {
		t.Errorf("architecture TotalLines = %d, want 10", arch.TotalLines)
	}
	if arch.AILines != 10 {
		t.Errorf("architecture AILines = %d, want 10", arch.AILines)
	}

	test, ok := pm.ByWorkType["test_scaffolding"]
	if !ok {
		t.Fatal("missing test_scaffolding in ByWorkType")
	}
	if test.Weight != 1.0 {
		t.Errorf("test_scaffolding Weight = %f, want 1.0", test.Weight)
	}
}

func TestComputeProjectMetrics_AuthorshipCounts(t *testing.T) {
	calc := NewCalculator()

	attrs := []store.AttributionWithWorkType{
		makeAttr(1, "a.go", "/proj", "mostly_ai", "core_logic", 10),
		makeAttr(2, "a.go", "/proj", "mostly_ai", "core_logic", 10),
		makeAttr(3, "b.go", "/proj", "mostly_human", "core_logic", 10),
		makeAttr(4, "b.go", "/proj", "mixed", "core_logic", 10),
	}

	pm := calc.ComputeProjectMetrics("/proj", attrs)

	if pm.ByAuthorship["mostly_ai"] != 2 {
		t.Errorf("ByAuthorship[mostly_ai] = %d, want 2", pm.ByAuthorship["mostly_ai"])
	}
	if pm.ByAuthorship["mostly_human"] != 1 {
		t.Errorf("ByAuthorship[mostly_human] = %d, want 1", pm.ByAuthorship["mostly_human"])
	}
	if pm.ByAuthorship["mixed"] != 1 {
		t.Errorf("ByAuthorship[mixed] = %d, want 1", pm.ByAuthorship["mixed"])
	}
}

func TestComputeProjectMetrics_NoWorkType_DefaultsCoreLogic(t *testing.T) {
	calc := NewCalculator()

	attrs := []store.AttributionWithWorkType{
		makeAttr(1, "a.go", "/proj", "mostly_ai", "", 10),
		makeAttr(2, "a.go", "/proj", "mostly_human", "", 10),
	}

	pm := calc.ComputeProjectMetrics("/proj", attrs)

	// File should be treated as core_logic (weight 3.0).
	bd, ok := pm.ByWorkType["core_logic"]
	if !ok {
		t.Fatal("expected core_logic entry in ByWorkType for files with empty work_type")
	}
	if bd.Weight != 3.0 {
		t.Errorf("core_logic Weight = %f, want 3.0", bd.Weight)
	}
}

func TestComputeProjectMetrics_HighTierBoostsAI(t *testing.T) {
	calc := NewCalculator()

	// Scenario: AI wrote all the architecture, humans wrote all the boilerplate.
	// Raw AI % should reflect line-based ratio.
	// Meaningful AI % should be higher because architecture (weight 3.0) has all the AI.
	attrs := []store.AttributionWithWorkType{
		// Architecture: 2 AI events with 10 lines each = 20 AI lines
		makeAttr(1, "types.go", "/proj", "mostly_ai", "architecture", 10),
		makeAttr(2, "types.go", "/proj", "mostly_ai", "architecture", 10),
		// Boilerplate: 2 human events with 10 lines each = 20 human lines
		makeAttr(3, "go.mod", "/proj", "mostly_human", "boilerplate", 10),
		makeAttr(4, "go.mod", "/proj", "mostly_human", "boilerplate", 10),
	}

	pm := calc.ComputeProjectMetrics("/proj", attrs)

	// Raw: 20/40 = 50%
	if !almostEqual(pm.RawAIPct, 50.0, 0.01) {
		t.Errorf("RawAIPct = %f, want 50.0", pm.RawAIPct)
	}

	// Meaningful: (20*3 + 0*1) / (20*3 + 20*1) = 60/80 = 75%
	if !almostEqual(pm.MeaningfulAIPct, 75.0, 0.01) {
		t.Errorf("MeaningfulAIPct = %f, want 75.0 (high tier boosts AI contribution)", pm.MeaningfulAIPct)
	}
}

func TestComputeProjectMetrics_PerWorkTypeAIPct(t *testing.T) {
	calc := NewCalculator()

	attrs := []store.AttributionWithWorkType{
		makeAttr(1, "a.go", "/proj", "mostly_ai", "core_logic", 10),
		makeAttr(2, "a.go", "/proj", "mostly_human", "core_logic", 10),
		makeAttr(3, "b.go", "/proj", "mostly_ai", "boilerplate", 10),
	}

	pm := calc.ComputeProjectMetrics("/proj", attrs)

	cl, ok := pm.ByWorkType["core_logic"]
	if !ok {
		t.Fatal("missing core_logic in ByWorkType")
	}
	// core_logic: 10 AI lines / 20 total = 50%
	if !almostEqual(cl.AIPct, 50.0, 0.01) {
		t.Errorf("core_logic AIPct = %f, want 50.0", cl.AIPct)
	}

	bp, ok := pm.ByWorkType["boilerplate"]
	if !ok {
		t.Fatal("missing boilerplate in ByWorkType")
	}
	// boilerplate: 10 AI lines / 10 total = 100%
	if !almostEqual(bp.AIPct, 100.0, 0.01) {
		t.Errorf("boilerplate AIPct = %f, want 100.0", bp.AIPct)
	}
}
