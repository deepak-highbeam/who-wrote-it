---
phase: 03-output
plan: 02
subsystem: output
tags: [github-api, pr-comment, code-survival, git-blame, markdown, rest-api]

# Dependency graph
requires:
  - phase: 03-01
    provides: "Report generation package with ProjectReport struct and formatting utilities"
  - phase: 02-02
    provides: "Work-type classifier and metrics calculator for weighted AI percentages"
  - phase: 01-03
    provides: "Git blame tracking with content hashes stored in git_blame_lines table"
provides:
  - "GitHub PR comment generation from ProjectReport data (Markdown)"
  - "GitHub REST API comment posting with Bearer token auth"
  - "Code survival analysis comparing AI attributions against current git blame"
  - "Schema migration v4 with code_survival table"
  - "CLI pr-comment command with --dry-run, --token, --pr, --owner, --repo flags"
  - "CLI survival command with --json output"
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Content-hash based survival comparison (attributions vs blame lines)"
    - "Insight-driven PR comments with conditional callouts based on thresholds"
    - "Auto-detect GitHub remote and PR number from git/env/gh CLI"

key-files:
  created:
    - "internal/github/prcomment.go"
    - "internal/github/prcomment_test.go"
    - "internal/survival/tracker.go"
    - "internal/survival/tracker_test.go"
  modified:
    - "internal/store/schema.go"
    - "internal/store/sqlite.go"
    - "cmd/whowroteit/main.go"

key-decisions:
  - "Survival tracked by content_hash match (blame line hash vs session event hash)"
  - "Files without blame data skipped in survival analysis (not counted as not-survived)"
  - "PR comment format: headline metric, work-type table, conditional insight callouts, top 3-5 notable files"
  - "Notable files threshold: minimum 3 events to appear in PR comment"
  - "SurvivalReport type duplicated in github package to avoid circular imports (survival->store, github->report)"
  - "Auto-detect chain: git remote -> ParseGitHubRemote, GITHUB_PR_NUMBER env -> gh CLI"
  - "No external GitHub SDK (standard net/http for REST API calls)"

patterns-established:
  - "Content-hash comparison for code persistence tracking"
  - "Conditional insight callouts: only show noteworthy patterns (>80% boilerplate AI, <30% core logic AI)"
  - "CLI auto-detection with manual override flags pattern"

# Metrics
duration: 5min
completed: 2026-02-09
---

# Phase 3 Plan 2: PR Comment and Code Survival Summary

**GitHub PR comment posting with insight-driven Markdown format and content-hash based AI code survival tracking against git blame**

## Performance

- **Duration:** 5 min
- **Started:** 2026-02-10T03:46:21Z
- **Completed:** 2026-02-10T03:51:04Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- PR comment generation produces compact Markdown with headline metric, work-type breakdown table, conditional insight callouts, and top notable files
- PostComment posts to GitHub REST API with Bearer token auth and proper error handling
- Code survival tracker compares AI attribution content hashes against current git blame line hashes
- Survival rates aggregated by authorship level (fully_ai, ai_first_human_revised) and work type
- Schema migration v4 adds code_survival table for persisting survival check results
- CLI pr-comment command with auto-detection of GitHub remote, PR number, and token
- CLI survival command with ANSI-formatted terminal output and --json mode

## Task Commits

Each task was committed atomically:

1. **Task 1: GitHub PR comment and code survival packages** - `3f1ea35` (feat)
2. **Task 2: Wire pr-comment and survival commands into CLI** - `9346124` (feat)

## Files Created/Modified
- `internal/github/prcomment.go` - PR comment generation, posting, remote URL parsing, PR number detection
- `internal/github/prcomment_test.go` - 8 tests for comment generation, URL parsing, formatting
- `internal/survival/tracker.go` - Code survival analysis comparing attributions vs blame data
- `internal/survival/tracker_test.go` - 4 tests for survival analysis with mock store data
- `internal/store/schema.go` - Schema migration v4 adding code_survival table
- `internal/store/sqlite.go` - QueryBlameLinesByFile, QuerySessionEventByID, InsertSurvivalRecord, QuerySurvivalByProject methods
- `cmd/whowroteit/main.go` - pr-comment and survival cobra commands with flags

## Decisions Made
- Content-hash comparison chosen for survival tracking: blame line content_hash matched against session event content_hash. Simple and deterministic.
- Files without blame data are skipped (not penalized as "not survived") since blame may not have run yet.
- Notable files require minimum 3 events to avoid noise from single-event files in PR comments.
- SurvivalReport type is duplicated in the github package for ANSI formatting to avoid circular imports between survival (depends on store) and github (depends on report).
- No external GitHub SDK -- standard net/http keeps dependencies minimal and aligns with project approach.
- Auto-detection chain: GITHUB_TOKEN env var for token, GITHUB_PR_NUMBER env var or `gh pr view` for PR number, `git remote get-url origin` with regex parsing for owner/repo.

## Deviations from Plan
None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Phase 3 (Output) is now complete: analyze, pr-comment, and survival commands all wired
- All three project phases complete: Data Pipeline, Intelligence, Output
- Full test suite passes (14 test packages, all green)

---
*Phase: 03-output*
*Completed: 2026-02-09*
