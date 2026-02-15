// Package report generates attribution reports from the store data.
// It compares git diff additions against Claude Code session events
// to determine AI vs human line attribution.
package report

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anthropic/gap-map/internal/metrics"
	"github.com/anthropic/gap-map/internal/sessionparser"
	"github.com/anthropic/gap-map/internal/store"
	"github.com/anthropic/gap-map/internal/worktype"
)

// ProjectReport holds the full project attribution report data.
type ProjectReport struct {
	ProjectPath    string                    `json:"project_path"`
	MeaningfulAIPct float64                  `json:"meaningful_ai_pct"`
	RawAIPct       float64                   `json:"raw_ai_pct"`
	TotalFiles     int                       `json:"total_files"`
	TotalLines     int                       `json:"total_lines"`
	AILines        int                       `json:"ai_lines"`
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
	AILines     int     `json:"ai_lines"`
	TotalLines  int     `json:"total_lines"`
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
	TotalLines       int            `json:"total_lines"`
	AILines          int            `json:"ai_lines"`
	AuthorshipLevel  string         `json:"authorship_level"`
}

// GenerateProject reads the store at dbPath and produces a full project report.
func GenerateProject(dbPath string) (*ProjectReport, error) {
	s, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	return GenerateProjectFromStore(s)
}

// GenerateProjectFromStore produces a full project report from an open store.
// For each tracked file, it gets the git diff additions (lines changed since
// tracking began) and compares against Claude's session event content.
// This ensures attribution is based on changes, not full file content.
func GenerateProjectFromStore(s *store.Store) (*ProjectReport, error) {
	projectPath, err := discoverProjectPath(s)
	if err != nil {
		return nil, err
	}

	// Get all Claude Write/Edit session events.
	sessionEvents, err := s.QueryWriteEditSessionEvents()
	if err != nil {
		return nil, fmt.Errorf("query session events: %w", err)
	}

	// Extract content from each session event and group by file path.
	claudeContentByFile := buildClaudeContentMap(s, sessionEvents)

	// Get all tracked files from attributions (so we know which files to report on).
	attrs, err := s.QueryAttributionsWithWorkType(projectPath)
	if err != nil {
		return nil, fmt.Errorf("query attributions: %w", err)
	}

	// Group attributions by file to get work type and event counts.
	fileAttrs := make(map[string][]store.AttributionWithWorkType)
	for _, attr := range attrs {
		fileAttrs[attr.FilePath] = append(fileAttrs[attr.FilePath], attr)
	}

	report := &ProjectReport{
		ProjectPath:  projectPath,
		ByAuthorship: make(map[string]int),
		ByWorkType:   make(map[string]WorkTypeSummary),
	}

	wtClassifier := worktype.NewClassifier(s)

	for filePath, fileAttrList := range fileAttrs {
		// Verify the file still exists on disk.
		absPath := resolveFilePath(projectPath, filePath)
		if _, err := os.Stat(absPath); err != nil {
			continue
		}

		// Find Claude's content for this file (using suffix matching for paths).
		claudeContents := findClaudeContent(filePath, claudeContentByFile)

		// Get the changed lines (git diff additions) instead of full file.
		changedContent, baseContent := getChangedLinesWithBase(s, projectPath, filePath)

		// Compute line-level attribution against the changes.
		la := metrics.ComputeLineAttribution(changedContent, claudeContents, baseContent)

		// Skip files with no changed lines (e.g. fully reverted).
		if la.TotalLines == 0 {
			continue
		}

		// Determine work type from attributions or content.
		wt := fileAttrList[0].WorkType
		if wt == "" {
			wt = string(wtClassifier.ClassifyFile(filePath, "", ""))
		}

		// Compute AI%.
		aiPct := 0.0
		if la.TotalLines > 0 {
			aiPct = float64(la.AILines) / float64(la.TotalLines) * 100.0
		}

		// Compute 3-level authorship from line ratio.
		level := "mostly_human"
		switch {
		case aiPct > 70:
			level = "mostly_ai"
		case aiPct >= 30:
			level = "mixed"
		}

		fr := FileReport{
			FilePath:        filePath,
			WorkType:        wt,
			MeaningfulAIPct: aiPct,
			RawAIPct:        aiPct,
			TotalLines:      la.TotalLines,
			AILines:         la.AILines,
			AuthorshipLevel: level,
			TotalEvents:     len(fileAttrList),
			AuthorshipCounts: map[string]int{level: len(fileAttrList)},
		}

		// Count AI events from attributions.
		for _, attr := range fileAttrList {
			if isAIAuthorship(attr.AuthorshipLevel) {
				fr.AIEventCount++
			}
		}

		report.Files = append(report.Files, fr)
		report.TotalFiles++
		report.TotalLines += la.TotalLines
		report.AILines += la.AILines
		report.ByAuthorship[level]++

		// Aggregate by work type.
		wtKey := wt
		wtWeight, ok := worktype.WorkTypeWeights[worktype.WorkType(wtKey)]
		if !ok {
			wtWeight = worktype.WorkTypeWeights[worktype.CoreLogic]
		}
		summary := report.ByWorkType[wtKey]
		summary.Files++
		summary.AILines += la.AILines
		summary.TotalLines += la.TotalLines
		summary.AIEvents += fr.AIEventCount
		summary.TotalEvents += fr.TotalEvents
		summary.Weight = wtWeight
		tier := worktype.WorkTypeTier[worktype.WorkType(wtKey)]
		summary.Tier = string(tier)
		report.ByWorkType[wtKey] = summary
	}

	// Compute project-level AI%.
	if report.TotalLines > 0 {
		report.RawAIPct = float64(report.AILines) / float64(report.TotalLines) * 100.0
	}

	// Meaningful AI% uses work-type weights.
	var totalWeightedAI, totalWeightedAll float64
	for _, fr := range report.Files {
		wt := worktype.WorkType(fr.WorkType)
		weight, ok := worktype.WorkTypeWeights[wt]
		if !ok {
			weight = worktype.WorkTypeWeights[worktype.CoreLogic]
		}
		totalWeightedAI += float64(fr.AILines) * weight
		totalWeightedAll += float64(fr.TotalLines) * weight
	}
	if totalWeightedAll > 0 {
		report.MeaningfulAIPct = totalWeightedAI / totalWeightedAll * 100.0
	}

	// Compute per-work-type AI%.
	for key, summary := range report.ByWorkType {
		if summary.TotalLines > 0 {
			summary.AIPct = float64(summary.AILines) / float64(summary.TotalLines) * 100.0
		}
		report.ByWorkType[key] = summary
	}

	// Sort files by AI% descending.
	sort.Slice(report.Files, func(i, j int) bool {
		return report.Files[i].MeaningfulAIPct > report.Files[j].MeaningfulAIPct
	})

	return report, nil
}

// GenerateFile reads the store at dbPath and produces a report for a single file.
func GenerateFile(dbPath string, filePath string) (*FileReport, error) {
	s, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	return GenerateFileFromStore(s, filePath)
}

// GenerateFileFromStore produces a single-file report from an open store.
func GenerateFileFromStore(s *store.Store, filePath string) (*FileReport, error) {
	attrs, err := s.QueryAttributionsByFileWithWorkType(filePath)
	if err != nil {
		return nil, fmt.Errorf("query attributions for file %q: %w", filePath, err)
	}

	if len(attrs) == 0 {
		return nil, fmt.Errorf("no attribution data found for file %q", filePath)
	}

	// Discover project path for resolving file.
	projectPath := attrs[0].ProjectPath

	// Verify the file exists.
	absPath := resolveFilePath(projectPath, filePath)
	if _, err := os.Stat(absPath); err != nil {
		return nil, fmt.Errorf("read file %q: %w", absPath, err)
	}

	// Get Claude's session event content for this file.
	sessionEvents, err := s.QueryWriteEditSessionEvents()
	if err != nil {
		return nil, fmt.Errorf("query session events: %w", err)
	}

	claudeContentByFile := buildClaudeContentMap(s, sessionEvents)
	claudeContents := findClaudeContent(filePath, claudeContentByFile)

	// Get the changed lines (git diff additions) instead of full file.
	changedContent, baseContent := getChangedLinesWithBase(s, projectPath, filePath)

	// Compute line-level attribution against the changes.
	la := metrics.ComputeLineAttribution(changedContent, claudeContents, baseContent)

	aiPct := 0.0
	if la.TotalLines > 0 {
		aiPct = float64(la.AILines) / float64(la.TotalLines) * 100.0
	}

	level := "mostly_human"
	switch {
	case aiPct > 70:
		level = "mostly_ai"
	case aiPct >= 30:
		level = "mixed"
	}

	wt := attrs[0].WorkType
	if wt == "" {
		wtClassifier := worktype.NewClassifier(s)
		wt = string(wtClassifier.ClassifyFile(filePath, "", ""))
	}

	fr := &FileReport{
		FilePath:        filePath,
		WorkType:        wt,
		MeaningfulAIPct: aiPct,
		RawAIPct:        aiPct,
		TotalLines:      la.TotalLines,
		AILines:         la.AILines,
		AuthorshipLevel: level,
		TotalEvents:     len(attrs),
		AuthorshipCounts: map[string]int{level: len(attrs)},
	}

	for _, attr := range attrs {
		if isAIAuthorship(attr.AuthorshipLevel) {
			fr.AIEventCount++
		}
	}

	return fr, nil
}

// getChangedLinesWithBase returns the changed lines for a file and the base file
// content (before tracking started). The base content is used to subtract
// pre-existing patterns from AI attribution.
// If git diff is unavailable or the file was created during tracking, it falls
// back to reading the full file content with empty base.
func getChangedLinesWithBase(s *store.Store, projectPath, filePath string) (changed string, base string) {
	absPath := resolveFilePath(projectPath, filePath)

	// Find the earliest attribution timestamp for this file.
	ts, err := s.QueryEarliestAttributionTimestamp(filePath)
	if err != nil || ts == "" {
		return readFileContent(absPath), ""
	}

	// Parse the timestamp to find a base commit before tracking started.
	attrTime, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return readFileContent(absPath), ""
	}

	// Find the latest commit before the earliest attribution.
	baseCommit := findBaseCommit(projectPath, filePath, attrTime)
	if baseCommit == "" {
		return readFileContent(absPath), ""
	}

	// Get git diff additions between the base commit and current working tree.
	// If additions is empty, the file is unchanged from the base commit
	// (e.g. all changes were reverted), so there are zero changed lines.
	additions := gitDiffAdditions(projectPath, filePath, baseCommit)
	if additions == "" {
		return "", ""
	}

	// Get the base file content at the base commit.
	baseContent := gitShowFile(projectPath, filePath, baseCommit)

	return additions, baseContent
}

// gitShowFile returns the content of a file at a specific commit.
func gitShowFile(projectPath, filePath, commit string) string {
	absPath := resolveFilePath(projectPath, filePath)
	relPath, err := filepath.Rel(projectPath, absPath)
	if err != nil {
		relPath = filePath
	}

	cmd := exec.Command("git", "show", commit+":"+relPath)
	cmd.Dir = projectPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// findBaseCommit finds the latest git commit hash that modified the file
// before the given timestamp. Returns empty string if not found.
func findBaseCommit(projectPath, filePath string, before time.Time) string {
	absPath := resolveFilePath(projectPath, filePath)
	relPath, err := filepath.Rel(projectPath, absPath)
	if err != nil {
		relPath = filePath
	}

	cmd := exec.Command("git", "log",
		"--before="+before.UTC().Format(time.RFC3339),
		"--format=%H",
		"-1",
		"--", relPath,
	)
	cmd.Dir = projectPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitDiffAdditions runs git diff between a base commit and the current working
// tree, returning only the added lines (without the "+" prefix).
func gitDiffAdditions(projectPath, filePath, baseCommit string) string {
	absPath := resolveFilePath(projectPath, filePath)
	relPath, err := filepath.Rel(projectPath, absPath)
	if err != nil {
		relPath = filePath
	}

	cmd := exec.Command("git", "diff", baseCommit, "--", relPath)
	cmd.Dir = projectPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	return parseDiffAdditions(string(out))
}

// parseDiffAdditions extracts added lines from unified diff output.
// It returns only the content of lines starting with "+" (excluding the
// "+++" file header) joined by newlines.
func parseDiffAdditions(diff string) string {
	var additions []string
	scanner := bufio.NewScanner(strings.NewReader(diff))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			// Strip the leading "+" to get the actual line content.
			additions = append(additions, line[1:])
		}
	}
	if len(additions) == 0 {
		return ""
	}
	return strings.Join(additions, "\n") + "\n"
}

// readFileContent reads a file and returns its content as a string.
// Returns empty string on error.
func readFileContent(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// buildClaudeContentMap extracts content from session event raw JSON and groups
// it by file path.
func buildClaudeContentMap(s *store.Store, events []store.StoredSessionEvent) map[string][]string {
	result := make(map[string][]string)
	for _, se := range events {
		rawJSON, err := s.QuerySessionEventRawJSON(se.ID)
		if err != nil {
			continue
		}
		content := sessionparser.ExtractDiffContent(rawJSON)
		if content != "" {
			result[se.FilePath] = append(result[se.FilePath], content)
		}
	}
	return result
}

// findClaudeContent finds all Claude-authored content for a file, using suffix
// matching to handle path differences (relative vs absolute, different prefixes).
func findClaudeContent(filePath string, contentByFile map[string][]string) []string {
	// Try exact match first.
	if contents, ok := contentByFile[filePath]; ok {
		return contents
	}

	// Try suffix match.
	cleanPath := filepath.Clean(filePath)
	for sessionPath, contents := range contentByFile {
		cleanSession := filepath.Clean(sessionPath)
		if cleanPath == cleanSession {
			return contents
		}
		// Check if one is a suffix of the other at a path boundary.
		if len(cleanPath) > len(cleanSession) {
			if strings.HasSuffix(cleanPath, string(filepath.Separator)+cleanSession) {
				return contents
			}
		} else {
			if strings.HasSuffix(cleanSession, string(filepath.Separator)+cleanPath) {
				return contents
			}
		}
	}

	return nil
}

// resolveFilePath tries to resolve a file path relative to the project root.
func resolveFilePath(projectPath, filePath string) string {
	if filepath.IsAbs(filePath) {
		return filePath
	}
	return filepath.Join(projectPath, filePath)
}

// isAIAuthorship returns true if the authorship level indicates AI involvement.
func isAIAuthorship(level string) bool {
	switch level {
	case "mostly_ai", "fully_ai", "ai_first_human_revised":
		return true
	}
	return false
}

// discoverProjectPath finds the project path from the attributions table.
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
