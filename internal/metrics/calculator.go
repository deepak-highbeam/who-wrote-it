// Package metrics computes meaningful AI percentage metrics that weight
// different types of work differently. Architecture and core logic have
// higher impact than boilerplate and test scaffolding.
// Line counts from session events are used for per-file AI% calculation.
package metrics

import (
	"github.com/anthropic/gap-map/internal/store"
	"github.com/anthropic/gap-map/internal/worktype"
)

// FileMetrics holds computed metrics for a single file.
type FileMetrics struct {
	FilePath         string
	WorkType         string
	AuthorshipCounts map[string]int // authorship_level -> count
	TotalEvents      int
	AIEventCount     int     // events where authorship is AI-related
	MeaningfulAIPct  float64 // weighted AI percentage
	RawAIPct         float64 // unweighted AI percentage (for comparison)
	TotalLines       int     // total lines across all attributions
	AILines          int     // lines from AI-authored attributions
	AuthorshipLevel  string  // 3-level computed from line ratio: mostly_ai, mixed, mostly_human
}

// ProjectMetrics holds aggregate metrics for an entire project.
type ProjectMetrics struct {
	ProjectPath     string
	TotalFiles      int
	FileMetrics     []FileMetrics
	MeaningfulAIPct float64                    // weighted aggregate
	RawAIPct        float64                    // unweighted aggregate
	ByWorkType      map[string]WorkTypeBreakdown
	ByAuthorship    map[string]int             // authorship_level -> total count
	TotalLines      int                        // total lines across all files
	AILines         int                        // total AI lines across all files
}

// WorkTypeBreakdown holds per-work-type aggregate metrics.
type WorkTypeBreakdown struct {
	WorkType     string
	Tier         string
	Weight       float64
	FileCount    int
	AIEventCount int
	TotalEvents  int
	AIPct        float64
	AILines      int
	TotalLines   int
}

// aiAuthorshipLevels are the authorship levels that count as AI-authored.
// Includes both new 3-level names and legacy 5-level names for backward compat.
var aiAuthorshipLevels = map[string]bool{
	"mostly_ai":              true,
	"fully_ai":               true,
	"ai_first_human_revised": true,
}

// Calculator computes meaningful AI percentage metrics.
type Calculator struct{}

// NewCalculator creates a new Calculator.
func NewCalculator() *Calculator {
	return &Calculator{}
}

// effectiveLines returns the lines_changed for an attribution, treating 0 as 1
// for backward compatibility with old data that doesn't have line counts.
func effectiveLines(linesChanged int) int {
	if linesChanged <= 0 {
		return 1
	}
	return linesChanged
}

// ComputeFileMetrics computes metrics for a single file from its attributions.
// Line counts are used to weight the AI percentage — a 200-line Write counts
// more than a 2-line Edit.
func (c *Calculator) ComputeFileMetrics(filePath string, attributions []store.AttributionWithWorkType) FileMetrics {
	fm := FileMetrics{
		FilePath:         filePath,
		AuthorshipCounts: make(map[string]int),
	}

	if len(attributions) == 0 {
		return fm
	}

	// Use the first attribution's work_type for the file-level work type.
	fm.WorkType = attributions[0].WorkType
	if fm.WorkType == "" {
		fm.WorkType = string(worktype.CoreLogic)
	}

	for _, attr := range attributions {
		fm.TotalEvents++
		fm.AuthorshipCounts[attr.AuthorshipLevel]++

		lines := effectiveLines(attr.LinesChanged)
		fm.TotalLines += lines

		if aiAuthorshipLevels[attr.AuthorshipLevel] {
			fm.AIEventCount++
			fm.AILines += lines
		}
	}

	if fm.TotalLines > 0 {
		fm.RawAIPct = float64(fm.AILines) / float64(fm.TotalLines) * 100.0
		fm.MeaningfulAIPct = fm.RawAIPct
	}

	// Compute 3-level authorship from line ratio.
	switch {
	case fm.RawAIPct > 70:
		fm.AuthorshipLevel = "mostly_ai"
	case fm.RawAIPct >= 30:
		fm.AuthorshipLevel = "mixed"
	default:
		fm.AuthorshipLevel = "mostly_human"
	}

	return fm
}

// ComputeProjectMetrics computes aggregate metrics for a project from all
// attributions. The meaningful AI percentage weights each file's contribution
// by its work type weight and uses line counts for accuracy.
func (c *Calculator) ComputeProjectMetrics(projectPath string, allAttributions []store.AttributionWithWorkType) ProjectMetrics {
	pm := ProjectMetrics{
		ProjectPath:  projectPath,
		ByWorkType:   make(map[string]WorkTypeBreakdown),
		ByAuthorship: make(map[string]int),
	}

	if len(allAttributions) == 0 {
		return pm
	}

	// Group attributions by file path.
	byFile := make(map[string][]store.AttributionWithWorkType)
	for _, attr := range allAttributions {
		byFile[attr.FilePath] = append(byFile[attr.FilePath], attr)
	}

	pm.TotalFiles = len(byFile)

	var totalWeightedAI float64
	var totalWeightedAll float64

	for filePath, fileAttrs := range byFile {
		fm := c.ComputeFileMetrics(filePath, fileAttrs)
		pm.FileMetrics = append(pm.FileMetrics, fm)

		// Look up the weight for this file's work type.
		wt := worktype.WorkType(fm.WorkType)
		weight, ok := worktype.WorkTypeWeights[wt]
		if !ok {
			weight = worktype.WorkTypeWeights[worktype.CoreLogic]
		}

		// Weighted contribution using line counts.
		totalWeightedAI += float64(fm.AILines) * weight
		totalWeightedAll += float64(fm.TotalLines) * weight

		// Raw (unweighted) totals using line counts.
		pm.AILines += fm.AILines
		pm.TotalLines += fm.TotalLines

		// Aggregate by work type.
		breakdown, exists := pm.ByWorkType[fm.WorkType]
		if !exists {
			tier := worktype.WorkTypeTier[wt]
			breakdown = WorkTypeBreakdown{
				WorkType: fm.WorkType,
				Tier:     string(tier),
				Weight:   weight,
			}
		}
		breakdown.FileCount++
		breakdown.AIEventCount += fm.AIEventCount
		breakdown.TotalEvents += fm.TotalEvents
		breakdown.AILines += fm.AILines
		breakdown.TotalLines += fm.TotalLines
		pm.ByWorkType[fm.WorkType] = breakdown

		// Aggregate by authorship level.
		for level, count := range fm.AuthorshipCounts {
			pm.ByAuthorship[level] += count
		}
	}

	// Compute project-level percentages.
	if totalWeightedAll > 0 {
		pm.MeaningfulAIPct = totalWeightedAI / totalWeightedAll * 100.0
	}
	if pm.TotalLines > 0 {
		pm.RawAIPct = float64(pm.AILines) / float64(pm.TotalLines) * 100.0
	}

	// Compute per-work-type AI percentages using line counts.
	for key, bd := range pm.ByWorkType {
		if bd.TotalLines > 0 {
			bd.AIPct = float64(bd.AILines) / float64(bd.TotalLines) * 100.0
		}
		pm.ByWorkType[key] = bd
	}

	return pm
}
