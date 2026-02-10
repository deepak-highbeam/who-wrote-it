// Package report generates attribution reports from the store data.
// It reads the SQLite database directly (no daemon required) and uses
// the metrics calculator to produce structured report data.
package report

import (
	"fmt"
	"sort"

	"github.com/anthropic/who-wrote-it/internal/metrics"
	"github.com/anthropic/who-wrote-it/internal/store"
)

// ProjectReport holds the full project attribution report data.
type ProjectReport struct {
	ProjectPath    string                    `json:"project_path"`
	MeaningfulAIPct float64                  `json:"meaningful_ai_pct"`
	RawAIPct       float64                   `json:"raw_ai_pct"`
	TotalFiles     int                       `json:"total_files"`
	ByAuthorship   map[string]int            `json:"by_authorship"`
	ByWorkType     map[string]WorkTypeSummary `json:"by_work_type"`
	Files          []FileReport              `json:"files"`
}

// WorkTypeSummary holds per-work-type aggregate data for the report.
type WorkTypeSummary struct {
	Files       int     `json:"files"`
	AIEvents    int     `json:"ai_events"`
	TotalEvents int     `json:"total_events"`
	AIPct       float64 `json:"ai_pct"`
	Tier        string  `json:"tier"`
	Weight      float64 `json:"weight"`
}

// FileReport holds the attribution report data for a single file.
type FileReport struct {
	FilePath         string         `json:"file_path"`
	WorkType         string         `json:"work_type"`
	MeaningfulAIPct  float64        `json:"meaningful_ai_pct"`
	RawAIPct         float64        `json:"raw_ai_pct"`
	AuthorshipCounts map[string]int `json:"authorship_counts"`
	TotalEvents      int            `json:"total_events"`
	AIEventCount     int            `json:"ai_event_count"`
}

// GenerateProject reads the store at dbPath and produces a full project report.
// This reads the database directly -- the daemon does not need to be running.
func GenerateProject(dbPath string) (*ProjectReport, error) {
	s, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	return GenerateProjectFromStore(s)
}

// GenerateProjectFromStore produces a full project report from an open store.
// Exported for testing with in-memory stores.
func GenerateProjectFromStore(s *store.Store) (*ProjectReport, error) {
	// Discover project paths.
	projectPath, err := discoverProjectPath(s)
	if err != nil {
		return nil, err
	}

	attrs, err := s.QueryAttributionsWithWorkType(projectPath)
	if err != nil {
		return nil, fmt.Errorf("query attributions: %w", err)
	}

	calc := metrics.NewCalculator()
	pm := calc.ComputeProjectMetrics(projectPath, attrs)

	report := &ProjectReport{
		ProjectPath:    pm.ProjectPath,
		MeaningfulAIPct: pm.MeaningfulAIPct,
		RawAIPct:       pm.RawAIPct,
		TotalFiles:     pm.TotalFiles,
		ByAuthorship:   pm.ByAuthorship,
		ByWorkType:     make(map[string]WorkTypeSummary),
	}

	// Convert metrics work-type breakdown to report format.
	for key, bd := range pm.ByWorkType {
		report.ByWorkType[key] = WorkTypeSummary{
			Files:       bd.FileCount,
			AIEvents:    bd.AIEventCount,
			TotalEvents: bd.TotalEvents,
			AIPct:       bd.AIPct,
			Tier:        bd.Tier,
			Weight:      bd.Weight,
		}
	}

	// Convert file metrics to file reports and sort by AI% descending.
	for _, fm := range pm.FileMetrics {
		report.Files = append(report.Files, FileReport{
			FilePath:         fm.FilePath,
			WorkType:         fm.WorkType,
			MeaningfulAIPct:  fm.MeaningfulAIPct,
			RawAIPct:         fm.RawAIPct,
			AuthorshipCounts: fm.AuthorshipCounts,
			TotalEvents:      fm.TotalEvents,
			AIEventCount:     fm.AIEventCount,
		})
	}

	sort.Slice(report.Files, func(i, j int) bool {
		return report.Files[i].MeaningfulAIPct > report.Files[j].MeaningfulAIPct
	})

	return report, nil
}

// GenerateFile reads the store at dbPath and produces a report for a single file.
// This reads the database directly -- the daemon does not need to be running.
func GenerateFile(dbPath string, filePath string) (*FileReport, error) {
	s, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	return GenerateFileFromStore(s, filePath)
}

// GenerateFileFromStore produces a single-file report from an open store.
// Exported for testing with in-memory stores.
func GenerateFileFromStore(s *store.Store, filePath string) (*FileReport, error) {
	attrs, err := s.QueryAttributionsByFileWithWorkType(filePath)
	if err != nil {
		return nil, fmt.Errorf("query attributions for file %q: %w", filePath, err)
	}

	if len(attrs) == 0 {
		return nil, fmt.Errorf("no attribution data found for file %q", filePath)
	}

	calc := metrics.NewCalculator()
	fm := calc.ComputeFileMetrics(filePath, attrs)

	return &FileReport{
		FilePath:         fm.FilePath,
		WorkType:         fm.WorkType,
		MeaningfulAIPct:  fm.MeaningfulAIPct,
		RawAIPct:         fm.RawAIPct,
		AuthorshipCounts: fm.AuthorshipCounts,
		TotalEvents:      fm.TotalEvents,
		AIEventCount:     fm.AIEventCount,
	}, nil
}

// discoverProjectPath finds the project path from the attributions table.
// For v1, if multiple projects exist, the first one (alphabetically) is used.
func discoverProjectPath(s *store.Store) (string, error) {
	rows, err := s.DB().Query("SELECT DISTINCT project_path FROM attributions ORDER BY project_path LIMIT 1")
	if err != nil {
		return "", fmt.Errorf("discover project path: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return "", fmt.Errorf("no attribution data found in database")
	}

	var path string
	if err := rows.Scan(&path); err != nil {
		return "", fmt.Errorf("scan project path: %w", err)
	}

	return path, rows.Err()
}
