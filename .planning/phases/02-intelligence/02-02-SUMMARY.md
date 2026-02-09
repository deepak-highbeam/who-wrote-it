---
phase: "02-intelligence"
plan: "02"
subsystem: "worktype-classification-and-metrics"
tags: ["worktype", "metrics", "classifier", "heuristic", "weighted-percentage", "schema-migration"]
dependency-graph:
  requires: ["02-01"]
  provides: ["worktype-classifier", "metrics-calculator", "work-type-overrides", "schema-v3"]
  affects: ["03-01"]
tech-stack:
  added: []
  patterns: ["heuristic-pattern-rules", "priority-based-evaluation", "weight-tier-system", "override-reader-interface"]
key-files:
  created:
    - "internal/worktype/patterns.go"
    - "internal/worktype/classifier.go"
    - "internal/worktype/classifier_test.go"
    - "internal/metrics/calculator.go"
    - "internal/metrics/calculator_test.go"
  modified:
    - "internal/store/schema.go"
    - "internal/store/sqlite.go"
decisions:
  - id: "wt-three-tiers"
    description: "Three weight tiers: high (3.0), medium (2.0), low (1.0) applied to architecture/core_logic, bug_fix/edge_case, boilerplate/test_scaffolding respectively"
  - id: "wt-edge-case-threshold"
    description: "Edge case classification requires >= 3 keyword occurrences in diff content to avoid false positives from single error checks"
  - id: "wt-core-logic-default"
    description: "CoreLogic is the default fallback work type -- most application code lives in files that don't match special patterns"
  - id: "wt-bugfix-commit-level"
    description: "Bug fix detection uses commit message keywords (fix, bug, patch, hotfix) as a secondary signal since it's commit-level not file-level"
  - id: "wt-ai-events-definition"
    description: "AI events = fully_ai + ai_first_human_revised; human_first_ai_revised and ai_suggested_human_written are NOT counted as AI-authored"
metrics:
  duration: "5 min"
  completed: "2026-02-09"
---

# Phase 02 Plan 02: Work-Type Classifier and Meaningful AI Metrics Summary

Heuristic work-type classifier labels file changes into 6 categories (architecture, core_logic, boilerplate, bug_fix, edge_case, test_scaffolding) using file path patterns, code keywords, and commit messages. Meaningful AI percentage weights architecture/core_logic at 3x over boilerplate/test_scaffolding, making "AI wrote 80% of go.mod" count less than "AI wrote 30% of the core logic." Per-file/commit overrides allow users to correct misclassifications without propagation.

## What Was Built

### Work-Type Classifier (`internal/worktype/`)

**patterns.go** -- Constants, weights, and heuristic rules:
- 6 `WorkType` constants: `architecture`, `core_logic`, `boilerplate`, `bug_fix`, `edge_case`, `test_scaffolding`
- 3 weight tiers with numeric weights: high (3.0), medium (2.0), low (1.0)
- `PatternRule` struct with `FileGlobs`, `Keywords`, and `Priority` fields
- `DefaultRules()` returns 5 built-in rules evaluated by priority (10-50)
- Path segment lists for test directories and architecture directories

**classifier.go** -- Classification engine:
- `Classifier` struct with `OverrideReader` interface for DI
- `ClassifyFile(filePath, diffContent, commitMessage) WorkType` -- main classification method
- `ClassifyFileWithCommit(filePath, diffContent, commitMessage, commitHash) WorkType` -- commit-specific override support
- Evaluation order: override check -> test path segments -> architecture path segments -> pattern rules (descending priority) -> default CoreLogic
- Edge case detection uses keyword threshold (>= 3 occurrences) to avoid false positives
- Bug fix detection matches commit message keywords only (not file content)

### Metrics Calculator (`internal/metrics/`)

**calculator.go** -- Weighted AI percentage computation:
- `FileMetrics` struct: per-file authorship counts, AI event count, raw and meaningful AI percentages
- `ProjectMetrics` struct: aggregate metrics with `ByWorkType` breakdown and `ByAuthorship` counts
- `WorkTypeBreakdown` struct: per-category file count, events, AI percentage, tier, weight
- `ComputeFileMetrics` -- per-file: RawAIPct = MeaningfulAIPct (weighting is cross-file)
- `ComputeProjectMetrics` -- project-level: `MeaningfulAIPct = sum(ai_events * weight) / sum(total_events * weight) * 100`
- AI events defined as: `fully_ai` + `ai_first_human_revised` (direct AI authorship)
- Zero events produce 0% (not NaN); empty work_type defaults to core_logic

### Schema Migration v3 (`internal/store/`)

**schema.go** -- `schemaVersion = 3`:
- `work_type_overrides` table: `id`, `file_path`, `commit_hash`, `work_type`, `created_at`
- Unique index on `(file_path, commit_hash)` for upsert support
- `ALTER TABLE attributions ADD COLUMN work_type TEXT NOT NULL DEFAULT ''`

**sqlite.go** -- New store methods:
- `InsertWorkTypeOverride(filePath, commitHash, workType)` -- INSERT OR REPLACE
- `QueryWorkTypeOverride(filePath, commitHash)` -- returns (workType, found, error)
- `UpdateAttributionWorkType(attrID, workType)` -- UPDATE single attribution
- `QueryAttributionsWithWorkType(projectPath)` -- project-level query with work_type
- `QueryAttributionsByFileWithWorkType(filePath)` -- file-level query with work_type
- `AttributionWithWorkType` struct extending `AttributionRecord` with `WorkType` field
- `scanAttributionsWithWorkType` row scanner for the extended type

## Test Coverage

**28 new tests total (16 worktype + 12 metrics), all passing with race detector.**

### Work-Type Classifier Tests (16 tests):
- Test file path patterns (`*_test.go`, `*.test.js`, `*.spec.ts`, etc.) -> TestScaffolding
- Test path segments (`/test/`, `/tests/`, `/__tests__/`) -> TestScaffolding
- Config file globs (`go.mod`, `Dockerfile`, `.gitignore`, etc.) -> Boilerplate
- Interface definitions in diff content -> Architecture
- Architecture path segments (`/models/`, `/types/`, etc.) -> Architecture
- Error handling keyword threshold (>= 3) -> EdgeCase
- Commit message keywords (`fix:`, `bug:`, `hotfix:`, etc.) -> BugFix
- Regular Go file with no special patterns -> CoreLogic (default)
- Override reader takes precedence over pattern rules
- Commit-specific override vs. generic override
- Priority ordering: path segments > rules
- All 6 work type constants present
- Three weight tiers with correct numeric values
- Tier mappings correctness
- Default rules count

### Metrics Calculator Tests (12 tests):
- Single file, all AI: 100% both metrics
- Single file, mixed: correct AI event counting
- Zero events: 0% without NaN
- AI includes `ai_first_human_revised` (not just `fully_ai`)
- Empty work_type defaults to core_logic
- Weighted project calculation: architecture (weight 3) + boilerplate (weight 1) = correct MeaningfulAIPct
- Zero events project: all zeros
- WorkType breakdown: correct tier, weight, file count per category
- Authorship level aggregation across files
- High tier boosts AI: architecture-heavy AI > boilerplate-heavy human = MeaningfulAIPct > RawAIPct
- Per-work-type AI percentage computation
- Default core_logic weight for files with empty work_type

## Deviations from Plan

None -- plan executed exactly as written.

## Decisions Made

1. **Edge case threshold = 3**: Requires 3+ keyword occurrences to classify as edge_case. Single `if err != nil` in a file doesn't trigger edge_case classification -- the file needs to be primarily error handling.

2. **AI event definition**: Only `fully_ai` and `ai_first_human_revised` count as AI events. `human_first_ai_revised` (human wrote first, AI revised) and `ai_suggested_human_written` (human wrote with AI context) are NOT counted as AI-authored because the human is the primary author.

3. **Per-file meaningful AI % equals raw AI %**: Weighting only applies at the project level (cross-file). A single file's meaningful and raw percentages are identical. The weight system amplifies or dampens a file's contribution to the project aggregate.

4. **Bug fix as commit-level signal**: Bug fix detection uses commit message keywords because bug fixes are a property of the change intent, not the code structure. A file containing `fix` in its diff content could be anything.

## Key Links

- `internal/worktype/classifier.go` reads from `OverrideReader` interface (implemented by `internal/store/sqlite.go` via `QueryWorkTypeOverride`)
- `internal/metrics/calculator.go` imports `internal/worktype` for `WorkType` constants and `WorkTypeWeights` lookup
- `internal/metrics/calculator.go` consumes `store.AttributionWithWorkType` which extends `store.AttributionRecord`
- Schema v3 migration adds `work_type` column to attributions table created in v2

## Next Phase Readiness

Phase 02 (Intelligence) is complete. Both plans delivered:
- 02-01: Correlation engine + authorship classifier (5 levels, confidence scoring)
- 02-02: Work-type classifier + meaningful AI metrics (6 categories, weighted percentages)

Ready for Phase 03 (Interface) which will expose these metrics through CLI commands and dashboard views.

### Commits

| Task | Commit | Description |
|------|--------|-------------|
| 1 | `4cdbeee` | Work-type classifier, patterns, schema v3, store methods |
| 2 | `9978078` | Metrics calculator and work-type classifier tests (28 tests) |
