package report

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anthropic/gap-map/internal/gitint"
	"github.com/anthropic/gap-map/internal/metrics"
	"github.com/anthropic/gap-map/internal/store"
)

// GenerateProjectForBranch produces a project report scoped to a specific branch,
// using the merge-base diff between baseBranch and branch to determine changed lines.
func GenerateProjectForBranch(s *store.Store, branch, baseBranch string) (*ProjectReport, error) {
	// Discover project path from any attributions in the DB.
	projectPath, err := discoverProjectPath(s)
	if err != nil {
		return nil, fmt.Errorf("no attribution data found: %w", err)
	}

	// Verify the branch has at least one attribution (to reject nonexistent branches).
	branchAttrs, err := s.QueryAttributionsByBranch(projectPath, branch)
	if err != nil {
		return nil, fmt.Errorf("query attributions for branch %q: %w", branch, err)
	}
	if len(branchAttrs) == 0 {
		return nil, fmt.Errorf("no attribution data for branch %q", branch)
	}

	// Compute merge-base between baseBranch and branch.
	mergeBase := gitMergeBaseCommit(projectPath, baseBranch, branch)
	if mergeBase == "" {
		return nil, fmt.Errorf("cannot compute merge-base for %s and %s", baseBranch, branch)
	}

	// Check if we're currently on the target branch.
	currentBranch, _ := gitint.CurrentBranch(projectPath)
	onBranch := currentBranch == branch

	// Get Claude session events for content comparison.
	sessionEvents, err := s.QueryWriteEditSessionEvents()
	if err != nil {
		return nil, fmt.Errorf("query session events: %w", err)
	}
	claudeContentByFile := buildClaudeContentMap(s, sessionEvents)

	// Get ALL attributions for the project (any branch) so stacked branch
	// reports include files attributed on ancestor branches.
	allAttrs, err := s.QueryAttributionsWithWorkType(projectPath)
	if err != nil {
		return nil, fmt.Errorf("query all attributions: %w", err)
	}
	fileAttrs := make(map[string][]store.AttributionWithWorkType)
	for _, attr := range allAttrs {
		fileAttrs[attr.FilePath] = append(fileAttrs[attr.FilePath], attr)
	}

	report := &ProjectReport{
		ProjectPath:  projectPath,
		ByAuthorship: make(map[string]int),
		ByWorkType:   make(map[string]WorkTypeSummary),
	}

	for filePath, fileAttrList := range fileAttrs {
		// Get diff additions for this file between merge-base and branch.
		var additions string
		var baseContent string

		if onBranch {
			// Working tree diff (includes uncommitted changes).
			additions = gitDiffAdditionsForBranch(projectPath, filePath, mergeBase, "")
			// If no tracked diff, check for untracked new files.
			if additions == "" {
				baseFileContent := gitShowFile(projectPath, filePath, mergeBase)
				if baseFileContent == "" {
					// File doesn't exist at merge-base — it may be an untracked new file.
					absPath := resolveFilePath(projectPath, filePath)
					if _, statErr := os.Stat(absPath); statErr == nil {
						additions = readFileContent(absPath)
					}
				}
			}
		} else {
			// Committed-only diff (not on the branch).
			additions = gitDiffAdditionsForBranch(projectPath, filePath, mergeBase, branch)
		}

		if additions == "" {
			continue
		}

		// Get base content at merge-base for pre-existing pattern subtraction.
		baseContent = gitShowFile(projectPath, filePath, mergeBase)

		// Find Claude's content for this file.
		claudeContents := findClaudeContent(filePath, claudeContentByFile)

		// Compute line-level attribution.
		la := metrics.ComputeLineAttribution(additions, claudeContents, baseContent)
		if la.TotalLines == 0 {
			continue
		}

		// Determine work type.
		wt := fileAttrList[0].WorkType
		if wt == "" {
			wt = "core_logic"
		}

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

		fr := FileReport{
			FilePath:         filePath,
			WorkType:         wt,
			MeaningfulAIPct:  aiPct,
			RawAIPct:         aiPct,
			TotalLines:       la.TotalLines,
			AILines:          la.AILines,
			AuthorshipLevel:  level,
			TotalEvents:      len(fileAttrList),
			AuthorshipCounts: map[string]int{level: len(fileAttrList)},
		}

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
	}

	// Compute project-level AI%.
	if report.TotalLines > 0 {
		report.RawAIPct = float64(report.AILines) / float64(report.TotalLines) * 100.0
		report.MeaningfulAIPct = report.RawAIPct
	}

	// Sort files by AI% descending.
	sort.Slice(report.Files, func(i, j int) bool {
		return report.Files[i].MeaningfulAIPct > report.Files[j].MeaningfulAIPct
	})

	return report, nil
}

// gitMergeBaseCommit returns the merge-base commit hash between two refs.
func gitMergeBaseCommit(repoPath, ref1, ref2 string) string {
	cmd := exec.Command("git", "merge-base", ref1, ref2)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitDiffAdditionsForBranch returns the added lines for a specific file between
// mergeBase and target. If target is empty, compares against the working tree.
func gitDiffAdditionsForBranch(projectPath, filePath, mergeBase, target string) string {
	absPath := resolveFilePath(projectPath, filePath)
	relPath, err := filepath.Rel(projectPath, absPath)
	if err != nil {
		relPath = filePath
	}

	var args []string
	if target == "" {
		args = []string{"diff", mergeBase, "--", relPath}
	} else {
		args = []string{"diff", mergeBase, target, "--", relPath}
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = projectPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	return parseDiffAdditions(string(out))
}
