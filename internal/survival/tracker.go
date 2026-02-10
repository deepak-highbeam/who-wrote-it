// Package survival provides code survival analysis. It compares AI-authored
// attributions against current git blame data to measure how much AI-written
// code persists over time.
package survival

import (
	"fmt"

	"github.com/anthropic/who-wrote-it/internal/store"
)

// SurvivalReport holds the results of a code survival analysis.
type SurvivalReport struct {
	TotalTracked  int                          `json:"total_tracked"`
	SurvivedCount int                          `json:"survived_count"`
	SurvivalRate  float64                      `json:"survival_rate"`
	ByAuthorship  map[string]SurvivalBreakdown `json:"by_authorship"`
	ByWorkType    map[string]SurvivalBreakdown `json:"by_work_type"`
}

// SurvivalBreakdown holds survival statistics for a single category
// (authorship level or work type).
type SurvivalBreakdown struct {
	Tracked  int     `json:"tracked"`
	Survived int     `json:"survived"`
	Rate     float64 `json:"rate"`
}

// aiAuthorshipLevels defines which authorship levels are considered AI-authored.
var aiAuthorshipLevels = map[string]bool{
	"fully_ai":               true,
	"ai_first_human_revised": true,
}

// Analyze performs code survival analysis for a project. For each file with
// AI attribution, it checks whether the content still exists in the current
// git blame data by comparing content hashes. Lines where the content_hash
// matches the original session event's content_hash are counted as "survived".
//
// Files without blame data are skipped (not counted as "not survived").
func Analyze(s *store.Store, projectPath string) (*SurvivalReport, error) {
	// Query all attributions with AI authorship for this project.
	allAttrs, err := s.QueryAttributionsWithWorkType(projectPath)
	if err != nil {
		return nil, fmt.Errorf("query attributions: %w", err)
	}

	report := &SurvivalReport{
		ByAuthorship: make(map[string]SurvivalBreakdown),
		ByWorkType:   make(map[string]SurvivalBreakdown),
	}

	// Group AI attributions by file.
	type fileAttr struct {
		attrs []store.AttributionWithWorkType
	}
	byFile := make(map[string]*fileAttr)

	for _, attr := range allAttrs {
		if !aiAuthorshipLevels[attr.AuthorshipLevel] {
			continue
		}
		fa, ok := byFile[attr.FilePath]
		if !ok {
			fa = &fileAttr{}
			byFile[attr.FilePath] = fa
		}
		fa.attrs = append(fa.attrs, attr)
	}

	if len(byFile) == 0 {
		return report, nil
	}

	for filePath, fa := range byFile {
		// Get current blame lines for this file.
		blameLines, err := s.QueryBlameLinesByFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("query blame lines for %q: %w", filePath, err)
		}

		// If no blame data, skip this file entirely.
		if len(blameLines) == 0 {
			continue
		}

		// Build a set of content hashes present in current blame.
		blameHashes := make(map[string]bool)
		for _, bl := range blameLines {
			if bl.ContentHash != "" {
				blameHashes[bl.ContentHash] = true
			}
		}

		// Check each AI attribution's associated session event content_hash.
		for _, attr := range fa.attrs {
			// Get the session event content_hash if we have a session event ID.
			var contentHash string
			if attr.SessionEventID != nil {
				se, err := s.QuerySessionEventByID(*attr.SessionEventID)
				if err != nil {
					// If session event not found, we can't compare. Skip.
					continue
				}
				contentHash = se.ContentHash
			}

			if contentHash == "" {
				// No content hash to compare -- skip this attribution.
				continue
			}

			survived := blameHashes[contentHash]

			report.TotalTracked++
			if survived {
				report.SurvivedCount++
			}

			// Aggregate by authorship level.
			bd := report.ByAuthorship[attr.AuthorshipLevel]
			bd.Tracked++
			if survived {
				bd.Survived++
			}
			report.ByAuthorship[attr.AuthorshipLevel] = bd

			// Aggregate by work type.
			wt := attr.WorkType
			if wt == "" {
				wt = "core_logic"
			}
			wtBd := report.ByWorkType[wt]
			wtBd.Tracked++
			if survived {
				wtBd.Survived++
			}
			report.ByWorkType[wt] = wtBd
		}
	}

	// Compute rates.
	if report.TotalTracked > 0 {
		report.SurvivalRate = float64(report.SurvivedCount) / float64(report.TotalTracked) * 100.0
	}
	for key, bd := range report.ByAuthorship {
		if bd.Tracked > 0 {
			bd.Rate = float64(bd.Survived) / float64(bd.Tracked) * 100.0
		}
		report.ByAuthorship[key] = bd
	}
	for key, bd := range report.ByWorkType {
		if bd.Tracked > 0 {
			bd.Rate = float64(bd.Survived) / float64(bd.Tracked) * 100.0
		}
		report.ByWorkType[key] = bd
	}

	return report, nil
}
