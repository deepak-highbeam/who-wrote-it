// Package worktype provides a heuristic work-type classifier that labels each
// file change as one of six categories: architecture, core_logic, boilerplate,
// bug_fix, edge_case, or test_scaffolding. Classification uses file path
// patterns and code content keywords rather than LLM inference.
package worktype

// WorkType represents the category of work a file change falls into.
type WorkType string

const (
	// Architecture represents structural definitions: interfaces, structs,
	// type hierarchies, protocols, and schema definitions.
	Architecture WorkType = "architecture"

	// CoreLogic represents primary business logic and application behavior.
	// This is the default when no other pattern matches.
	CoreLogic WorkType = "core_logic"

	// Boilerplate represents configuration files, lock files, manifests,
	// and generated or repetitive scaffolding.
	Boilerplate WorkType = "boilerplate"

	// BugFix represents changes identified as bug fixes via commit message
	// keywords (fix, bug, patch, hotfix, resolve, issue).
	BugFix WorkType = "bug_fix"

	// EdgeCase represents defensive code: error handling, fallback logic,
	// boundary conditions, and exception management.
	EdgeCase WorkType = "edge_case"

	// TestScaffolding represents test files identified by naming conventions
	// and path patterns.
	TestScaffolding WorkType = "test_scaffolding"
)

// AllWorkTypes returns the complete set of work type constants.
func AllWorkTypes() []WorkType {
	return []WorkType{
		Architecture,
		CoreLogic,
		Boilerplate,
		BugFix,
		EdgeCase,
		TestScaffolding,
	}
}

// WeightTier categorizes work types into importance tiers for the meaningful
// AI percentage metric.
type WeightTier string

const (
	// TierHigh represents work types with the highest impact on meaningful
	// AI percentage: architecture and core logic.
	TierHigh WeightTier = "high"

	// TierMedium represents work types with moderate impact: bug fixes and
	// edge case handling.
	TierMedium WeightTier = "medium"

	// TierLow represents work types with the lowest impact: boilerplate
	// and test scaffolding.
	TierLow WeightTier = "low"
)

// WorkTypeWeights maps each work type to its numeric weight for the meaningful
// AI percentage calculation.
//
//	High tier   (architecture, core_logic):     3.0
//	Medium tier (bug_fix, edge_case):           2.0
//	Low tier    (boilerplate, test_scaffolding): 1.0
var WorkTypeWeights = map[WorkType]float64{
	Architecture:    3.0,
	CoreLogic:       3.0,
	BugFix:          2.0,
	EdgeCase:        2.0,
	Boilerplate:     1.0,
	TestScaffolding: 1.0,
}

// WorkTypeTier maps each work type to its weight tier.
var WorkTypeTier = map[WorkType]WeightTier{
	Architecture:    TierHigh,
	CoreLogic:       TierHigh,
	BugFix:          TierMedium,
	EdgeCase:        TierMedium,
	Boilerplate:     TierLow,
	TestScaffolding: TierLow,
}

// PatternRule defines a heuristic rule for classifying file changes.
// Rules are evaluated in descending priority order; the first match wins.
type PatternRule struct {
	WorkType  WorkType
	FileGlobs []string // Match against file path (uses path.Match).
	Keywords  []string // Match against file content / diff content.
	Priority  int      // Higher priority wins when multiple rules match.
}

// DefaultRules returns the built-in heuristic pattern rules ordered by
// priority (lowest first; evaluation happens highest-first in the classifier).
//
// Priority tiers:
//
//	10 - Test scaffolding (file naming conventions)
//	20 - Boilerplate (config/lock/manifest files)
//	30 - Edge case (error handling keywords)
//	35 - Bug fix (commit message keywords, secondary signal)
//	40 - Architecture (interface/struct/type definitions)
//	50 - Core logic (default fallback, not a pattern rule)
func DefaultRules() []PatternRule {
	return []PatternRule{
		{
			WorkType: TestScaffolding,
			FileGlobs: []string{
				"*_test.go",
				"*_test.ts",
				"*_test.py",
				"test_*.py",
				"*.test.js",
				"*.test.ts",
				"*.test.tsx",
				"*.spec.js",
				"*.spec.ts",
				"*.spec.tsx",
			},
			// Path segments checked separately in the classifier.
			Keywords: nil,
			Priority: 10,
		},
		{
			WorkType: Boilerplate,
			FileGlobs: []string{
				"go.mod",
				"go.sum",
				"package.json",
				"package-lock.json",
				"*.lock",
				"Makefile",
				"Dockerfile",
				"*.yml",
				"*.yaml",
				"*.toml",
				".gitignore",
				".dockerignore",
				".editorconfig",
				"LICENSE",
				"LICENSE.*",
			},
			Keywords: nil,
			Priority: 20,
		},
		{
			WorkType: EdgeCase,
			FileGlobs: nil,
			Keywords: []string{
				"if err != nil",
				"if err != nil {",
				"catch (",
				"catch(",
				"except ",
				"except:",
				"default:",
				"fallback",
				"// edge case",
				"// handle",
				"// fallback",
			},
			Priority: 30,
		},
		{
			WorkType:  BugFix,
			FileGlobs: nil,
			// These match against commit messages, not file content.
			Keywords: []string{
				"fix:",
				"fix(",
				"bug:",
				"bug(",
				"patch:",
				"patch(",
				"hotfix:",
				"hotfix(",
				"resolve:",
				"resolve(",
				"issue:",
				"issue(",
			},
			Priority: 35,
		},
		{
			WorkType:  Architecture,
			FileGlobs: nil,
			Keywords: []string{
				"interface {",
				"interface{",
				"type %s struct",
				"trait ",
				"abstract class ",
				"protocol ",
			},
			Priority: 40,
		},
	}
}

// testPathSegments are directory path segments that indicate test files.
var testPathSegments = []string{
	"/test/",
	"/tests/",
	"/__tests__/",
	"/testing/",
}

// architecturePathSegments are directory path segments that indicate
// architecture/type definition files.
var architecturePathSegments = []string{
	"/models/",
	"/schema/",
	"/types/",
	"/interfaces/",
}

// edgeCaseKeywordThreshold is the minimum number of edge-case keyword
// occurrences in file content to classify as EdgeCase.
const edgeCaseKeywordThreshold = 3
