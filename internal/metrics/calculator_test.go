package metrics

import (
	"math"
	"testing"
	"time"

	"github.com/anthropic/who-wrote-it/internal/store"
)

var baseTime = time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC)

func makeAttr(id int64, filePath, projectPath, level, workType string) store.AttributionWithWorkType {
	return store.AttributionWithWorkType{
		AttributionRecord: store.AttributionRecord{
			ID:              id,
			FilePath:        filePath,
			ProjectPath:     projectPath,
			AuthorshipLevel: level,
			Confidence:      0.95,
			Timestamp:       baseTime,
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
		makeAttr(1, "foo.go", "/proj", "fully_ai", "core_logic"),
		makeAttr(2, "foo.go", "/proj", "fully_ai", "core_logic"),
		makeAttr(3, "foo.go", "/proj", "fully_ai", "core_logic"),
		makeAttr(4, "foo.go", "/proj", "fully_ai", "core_logic"),
		makeAttr(5, "foo.go", "/proj", "fully_ai", "core_logic"),
	}

	fm := calc.ComputeFileMetrics("foo.go", attrs)

	if fm.TotalEvents != 5 {
		t.Errorf("TotalEvents = %d, want 5", fm.TotalEvents)
	}
	if fm.AIEventCount != 5 {
		t.Errorf("AIEventCount = %d, want 5", fm.AIEventCount)
	}
	if !almostEqual(fm.RawAIPct, 100.0, 0.01) {
		t.Errorf("RawAIPct = %f, want 100.0", fm.RawAIPct)
	}
	if !almostEqual(fm.MeaningfulAIPct, 100.0, 0.01) {
		t.Errorf("MeaningfulAIPct = %f, want 100.0", fm.MeaningfulAIPct)
	}
	if fm.WorkType != "core_logic" {
		t.Errorf("WorkType = %q, want %q", fm.WorkType, "core_logic")
	}
}

func TestComputeFileMetrics_Mixed(t *testing.T) {
	calc := NewCalculator()

	attrs := []store.AttributionWithWorkType{
		makeAttr(1, "foo.go", "/proj", "fully_ai", "architecture"),
		makeAttr(2, "foo.go", "/proj", "fully_ai", "architecture"),
		makeAttr(3, "foo.go", "/proj", "fully_ai", "architecture"),
		makeAttr(4, "foo.go", "/proj", "fully_human", "architecture"),
		makeAttr(5, "foo.go", "/proj", "fully_human", "architecture"),
	}

	fm := calc.ComputeFileMetrics("foo.go", attrs)

	if fm.TotalEvents != 5 {
		t.Errorf("TotalEvents = %d, want 5", fm.TotalEvents)
	}
	if fm.AIEventCount != 3 {
		t.Errorf("AIEventCount = %d, want 3", fm.AIEventCount)
	}
	if !almostEqual(fm.RawAIPct, 60.0, 0.01) {
		t.Errorf("RawAIPct = %f, want 60.0", fm.RawAIPct)
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

func TestComputeFileMetrics_AIIncludesAIFirstHumanRevised(t *testing.T) {
	calc := NewCalculator()

	attrs := []store.AttributionWithWorkType{
		makeAttr(1, "foo.go", "/proj", "fully_ai", "core_logic"),
		makeAttr(2, "foo.go", "/proj", "ai_first_human_revised", "core_logic"),
		makeAttr(3, "foo.go", "/proj", "fully_human", "core_logic"),
		makeAttr(4, "foo.go", "/proj", "human_first_ai_revised", "core_logic"),
	}

	fm := calc.ComputeFileMetrics("foo.go", attrs)

	// fully_ai + ai_first_human_revised = 2 AI events out of 4
	if fm.AIEventCount != 2 {
		t.Errorf("AIEventCount = %d, want 2 (fully_ai + ai_first_human_revised)", fm.AIEventCount)
	}
	if !almostEqual(fm.RawAIPct, 50.0, 0.01) {
		t.Errorf("RawAIPct = %f, want 50.0", fm.RawAIPct)
	}
}

func TestComputeFileMetrics_NoWorkType_DefaultsCoreLogic(t *testing.T) {
	calc := NewCalculator()

	attrs := []store.AttributionWithWorkType{
		makeAttr(1, "foo.go", "/proj", "fully_ai", ""),
	}

	fm := calc.ComputeFileMetrics("foo.go", attrs)

	if fm.WorkType != "core_logic" {
		t.Errorf("WorkType = %q, want %q (default)", fm.WorkType, "core_logic")
	}
}

// ---------------------------------------------------------------------------
// ComputeProjectMetrics tests
// ---------------------------------------------------------------------------

func TestComputeProjectMetrics_WeightedCalculation(t *testing.T) {
	calc := NewCalculator()

	// File A: architecture (weight 3.0), 2 AI / 4 total
	// File B: boilerplate (weight 1.0), 3 AI / 3 total
	attrs := []store.AttributionWithWorkType{
		// File A - architecture
		makeAttr(1, "a.go", "/proj", "fully_ai", "architecture"),
		makeAttr(2, "a.go", "/proj", "fully_ai", "architecture"),
		makeAttr(3, "a.go", "/proj", "fully_human", "architecture"),
		makeAttr(4, "a.go", "/proj", "fully_human", "architecture"),
		// File B - boilerplate
		makeAttr(5, "go.mod", "/proj", "fully_ai", "boilerplate"),
		makeAttr(6, "go.mod", "/proj", "fully_ai", "boilerplate"),
		makeAttr(7, "go.mod", "/proj", "fully_ai", "boilerplate"),
	}

	pm := calc.ComputeProjectMetrics("/proj", attrs)

	// RawAIPct = (2+3) / (4+3) = 5/7 = 71.43%
	expectedRaw := 5.0 / 7.0 * 100.0
	if !almostEqual(pm.RawAIPct, expectedRaw, 0.01) {
		t.Errorf("RawAIPct = %f, want %f", pm.RawAIPct, expectedRaw)
	}

	// MeaningfulAIPct = (2*3 + 3*1) / (4*3 + 3*1) = 9/15 = 60%
	expectedMeaningful := 9.0 / 15.0 * 100.0
	if !almostEqual(pm.MeaningfulAIPct, expectedMeaningful, 0.01) {
		t.Errorf("MeaningfulAIPct = %f, want %f", pm.MeaningfulAIPct, expectedMeaningful)
	}

	if pm.TotalFiles != 2 {
		t.Errorf("TotalFiles = %d, want 2", pm.TotalFiles)
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
		makeAttr(1, "a.go", "/proj", "fully_ai", "architecture"),
		makeAttr(2, "b.go", "/proj", "fully_human", "core_logic"),
		makeAttr(3, "c_test.go", "/proj", "fully_ai", "test_scaffolding"),
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
		makeAttr(1, "a.go", "/proj", "fully_ai", "core_logic"),
		makeAttr(2, "a.go", "/proj", "fully_ai", "core_logic"),
		makeAttr(3, "b.go", "/proj", "fully_human", "core_logic"),
		makeAttr(4, "b.go", "/proj", "ai_first_human_revised", "core_logic"),
	}

	pm := calc.ComputeProjectMetrics("/proj", attrs)

	if pm.ByAuthorship["fully_ai"] != 2 {
		t.Errorf("ByAuthorship[fully_ai] = %d, want 2", pm.ByAuthorship["fully_ai"])
	}
	if pm.ByAuthorship["fully_human"] != 1 {
		t.Errorf("ByAuthorship[fully_human] = %d, want 1", pm.ByAuthorship["fully_human"])
	}
	if pm.ByAuthorship["ai_first_human_revised"] != 1 {
		t.Errorf("ByAuthorship[ai_first_human_revised] = %d, want 1", pm.ByAuthorship["ai_first_human_revised"])
	}
}

func TestComputeProjectMetrics_NoWorkType_DefaultsCoreLogic(t *testing.T) {
	calc := NewCalculator()

	attrs := []store.AttributionWithWorkType{
		makeAttr(1, "a.go", "/proj", "fully_ai", ""),
		makeAttr(2, "a.go", "/proj", "fully_human", ""),
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
	// Raw AI % is 50%, but meaningful AI % should be higher because
	// architecture (weight 3.0) has all the AI.
	attrs := []store.AttributionWithWorkType{
		// Architecture: 2 AI events
		makeAttr(1, "types.go", "/proj", "fully_ai", "architecture"),
		makeAttr(2, "types.go", "/proj", "fully_ai", "architecture"),
		// Boilerplate: 2 human events
		makeAttr(3, "go.mod", "/proj", "fully_human", "boilerplate"),
		makeAttr(4, "go.mod", "/proj", "fully_human", "boilerplate"),
	}

	pm := calc.ComputeProjectMetrics("/proj", attrs)

	// Raw: 2/4 = 50%
	if !almostEqual(pm.RawAIPct, 50.0, 0.01) {
		t.Errorf("RawAIPct = %f, want 50.0", pm.RawAIPct)
	}

	// Meaningful: (2*3 + 0*1) / (2*3 + 2*1) = 6/8 = 75%
	if !almostEqual(pm.MeaningfulAIPct, 75.0, 0.01) {
		t.Errorf("MeaningfulAIPct = %f, want 75.0 (high tier boosts AI contribution)", pm.MeaningfulAIPct)
	}
}

func TestComputeProjectMetrics_PerWorkTypeAIPct(t *testing.T) {
	calc := NewCalculator()

	attrs := []store.AttributionWithWorkType{
		makeAttr(1, "a.go", "/proj", "fully_ai", "core_logic"),
		makeAttr(2, "a.go", "/proj", "fully_human", "core_logic"),
		makeAttr(3, "b.go", "/proj", "fully_ai", "boilerplate"),
	}

	pm := calc.ComputeProjectMetrics("/proj", attrs)

	cl, ok := pm.ByWorkType["core_logic"]
	if !ok {
		t.Fatal("missing core_logic in ByWorkType")
	}
	// core_logic: 1 AI / 2 total = 50%
	if !almostEqual(cl.AIPct, 50.0, 0.01) {
		t.Errorf("core_logic AIPct = %f, want 50.0", cl.AIPct)
	}

	bp, ok := pm.ByWorkType["boilerplate"]
	if !ok {
		t.Fatal("missing boilerplate in ByWorkType")
	}
	// boilerplate: 1 AI / 1 total = 100%
	if !almostEqual(bp.AIPct, 100.0, 0.01) {
		t.Errorf("boilerplate AIPct = %f, want 100.0", bp.AIPct)
	}
}
