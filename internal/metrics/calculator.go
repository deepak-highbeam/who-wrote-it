// Package metrics computes meaningful AI percentage metrics that weight
// different types of work differently. Architecture and core logic have
// higher impact than boilerplate and test scaffolding.
package metrics

import (
	"github.com/anthropic/who-wrote-it/internal/store"
	"github.com/anthropic/who-wrote-it/internal/worktype"
)

// FileMetrics holds computed metrics for a single file.
type FileMetrics struct {
	FilePath         string
	WorkType         string
	AuthorshipCounts map[string]int // authorship_level -> count
	TotalEvents      int
	AIEventCount     int     // fully_ai + ai_first_human_revised
	MeaningfulAIPct  float64 // weighted AI percentage
	RawAIPct         float64 // unweighted AI percentage (for comparison)
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
}

// aiAuthorshipLevels are the authorship levels that count as AI-authored.
// Both fully_ai and ai_first_human_revised represent direct AI authorship.
var aiAuthorshipLevels = map[string]bool{
	"fully_ai":               true,
	"ai_first_human_revised": true,
}

// Calculator computes meaningful AI percentage metrics.
type Calculator struct{}

// NewCalculator creates a new Calculator.
func NewCalculator() *Calculator {
	return &Calculator{}
}

// ComputeFileMetrics computes metrics for a single file from its attributions.
func (c *Calculator) ComputeFileMetrics(filePath string, attributions []store.AttributionWithWorkType) FileMetrics {
	fm := FileMetrics{
		FilePath:         filePath,
		AuthorshipCounts: make(map[string]int),
	}

	if len(attributions) == 0 {
		return fm
	}

	// Use the first attribution's work_type for the file-level work type.
	// All attributions for a file should have the same work_type since
	// classification is per-file.
	fm.WorkType = attributions[0].WorkType
	if fm.WorkType == "" {
		fm.WorkType = string(worktype.CoreLogic)
	}

	for _, attr := range attributions {
		fm.TotalEvents++
		fm.AuthorshipCounts[attr.AuthorshipLevel]++
		if aiAuthorshipLevels[attr.AuthorshipLevel] {
			fm.AIEventCount++
		}
	}

	if fm.TotalEvents > 0 {
		fm.RawAIPct = float64(fm.AIEventCount) / float64(fm.TotalEvents) * 100.0
		// Per-file meaningful AI % is the same as raw AI % because
		// weighting applies across files at the project level.
		fm.MeaningfulAIPct = fm.RawAIPct
	}

	return fm
}

// ComputeProjectMetrics computes aggregate metrics for a project from all
// attributions. The meaningful AI percentage weights each file's contribution
// by its work type weight.
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
	var totalRawAI int
	var totalRawAll int

	for filePath, fileAttrs := range byFile {
		fm := c.ComputeFileMetrics(filePath, fileAttrs)
		pm.FileMetrics = append(pm.FileMetrics, fm)

		// Look up the weight for this file's work type.
		wt := worktype.WorkType(fm.WorkType)
		weight, ok := worktype.WorkTypeWeights[wt]
		if !ok {
			// Unknown work type defaults to CoreLogic weight.
			weight = worktype.WorkTypeWeights[worktype.CoreLogic]
		}

		// Weighted contribution to project metric.
		totalWeightedAI += float64(fm.AIEventCount) * weight
		totalWeightedAll += float64(fm.TotalEvents) * weight

		// Raw (unweighted) totals.
		totalRawAI += fm.AIEventCount
		totalRawAll += fm.TotalEvents

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
	if totalRawAll > 0 {
		pm.RawAIPct = float64(totalRawAI) / float64(totalRawAll) * 100.0
	}

	// Compute per-work-type AI percentages.
	for key, bd := range pm.ByWorkType {
		if bd.TotalEvents > 0 {
			bd.AIPct = float64(bd.AIEventCount) / float64(bd.TotalEvents) * 100.0
		}
		pm.ByWorkType[key] = bd
	}

	return pm
}
