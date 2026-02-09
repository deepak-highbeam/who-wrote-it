package worktype

import (
	"path"
	"sort"
	"strings"
)

// OverrideReader checks for user-supplied work-type overrides.
// The store implements this interface; tests use a mock.
type OverrideReader interface {
	// QueryWorkTypeOverride returns the user-overridden work type for a
	// specific file path and commit hash. If no override exists, found is false.
	QueryWorkTypeOverride(filePath string, commitHash string) (string, bool, error)
}

// Classifier applies heuristic pattern rules to classify each file change
// into one of six work types. Overrides are checked first; pattern rules
// are applied in descending priority order; the default is CoreLogic.
type Classifier struct {
	rules    []PatternRule
	override OverrideReader
}

// NewClassifier creates a Classifier with default rules and an optional
// override reader. Pass nil for override if no override checking is needed.
func NewClassifier(override OverrideReader) *Classifier {
	rules := DefaultRules()
	// Sort rules by descending priority for evaluation.
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority > rules[j].Priority
	})
	return &Classifier{
		rules:    rules,
		override: override,
	}
}

// ClassifyFile determines the work type of a file change based on its path,
// diff content, and commit message.
//
// Evaluation order:
//  1. Check override store (specific file/commit override wins immediately).
//  2. Check file path against test path segments.
//  3. Check file path against architecture path segments.
//  4. Evaluate pattern rules in descending priority order.
//     - Architecture rules match keywords in diffContent.
//     - Bug fix rules match keywords in commitMessage (not diffContent).
//     - Edge case rules use keyword threshold (>= 3 occurrences).
//     - All other rules match file path globs or keywords in diffContent.
//  5. Default: CoreLogic.
func (c *Classifier) ClassifyFile(filePath string, diffContent string, commitMessage string) WorkType {
	// Step 1: Check overrides.
	if c.override != nil {
		if wt, found, err := c.override.QueryWorkTypeOverride(filePath, ""); err == nil && found {
			return WorkType(wt)
		}
	}

	// Step 2: Test path segments (checked before rules for fast detection).
	lowerPath := strings.ToLower(filePath)
	for _, seg := range testPathSegments {
		if strings.Contains(lowerPath, seg) {
			return TestScaffolding
		}
	}

	// Step 3: Architecture path segments.
	for _, seg := range architecturePathSegments {
		if strings.Contains(lowerPath, seg) {
			return Architecture
		}
	}

	// Step 4: Evaluate pattern rules in descending priority order.
	baseName := path.Base(filePath)
	lowerDiff := strings.ToLower(diffContent)
	lowerCommit := strings.ToLower(commitMessage)

	for _, rule := range c.rules {
		if c.matchRule(rule, baseName, lowerPath, lowerDiff, lowerCommit) {
			return rule.WorkType
		}
	}

	// Step 5: Default fallback.
	return CoreLogic
}

// ClassifyFileWithCommit is like ClassifyFile but also checks overrides for
// a specific commit hash before the generic (empty commit) override.
func (c *Classifier) ClassifyFileWithCommit(filePath string, diffContent string, commitMessage string, commitHash string) WorkType {
	// Check commit-specific override first.
	if c.override != nil && commitHash != "" {
		if wt, found, err := c.override.QueryWorkTypeOverride(filePath, commitHash); err == nil && found {
			return WorkType(wt)
		}
	}
	return c.ClassifyFile(filePath, diffContent, commitMessage)
}

// matchRule tests whether a single pattern rule matches the given inputs.
func (c *Classifier) matchRule(rule PatternRule, baseName, lowerPath, lowerDiff, lowerCommit string) bool {
	switch rule.WorkType {
	case BugFix:
		// Bug fix is detected from commit message, not file content.
		return c.matchCommitKeywords(rule.Keywords, lowerCommit)

	case EdgeCase:
		// Edge case requires multiple keyword occurrences (threshold).
		return c.matchKeywordThreshold(rule.Keywords, lowerDiff, edgeCaseKeywordThreshold)

	case Architecture:
		// Architecture matches keywords in diff content.
		return c.matchAnyKeyword(rule.Keywords, lowerDiff)

	default:
		// File glob match (test scaffolding, boilerplate).
		if c.matchGlob(rule.FileGlobs, baseName) {
			return true
		}
		// Keyword match in diff content.
		if len(rule.Keywords) > 0 && c.matchAnyKeyword(rule.Keywords, lowerDiff) {
			return true
		}
		return false
	}
}

// matchGlob tests baseName against a list of glob patterns.
func (c *Classifier) matchGlob(globs []string, baseName string) bool {
	for _, glob := range globs {
		if matched, _ := path.Match(glob, baseName); matched {
			return true
		}
	}
	return false
}

// matchAnyKeyword returns true if any keyword appears in the text.
func (c *Classifier) matchAnyKeyword(keywords []string, text string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// matchCommitKeywords returns true if any keyword appears in the commit message.
func (c *Classifier) matchCommitKeywords(keywords []string, lowerCommit string) bool {
	if lowerCommit == "" {
		return false
	}
	for _, kw := range keywords {
		if strings.Contains(lowerCommit, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// matchKeywordThreshold returns true if the total count of keyword
// occurrences in text meets or exceeds the threshold.
func (c *Classifier) matchKeywordThreshold(keywords []string, text string, threshold int) bool {
	count := 0
	for _, kw := range keywords {
		lower := strings.ToLower(kw)
		count += strings.Count(text, lower)
		if count >= threshold {
			return true
		}
	}
	return false
}
