---
phase: "02-intelligence"
plan: "01"
subsystem: "correlation-and-classification"
tags: ["correlation", "authorship", "classifier", "attribution", "sqlite"]

dependency-graph:
  requires: ["01-data-pipeline"]
  provides: ["correlation-engine", "authorship-classifier", "attributions-table", "store-query-methods"]
  affects: ["02-02", "03-interface"]

tech-stack:
  added: []
  patterns: ["interface-based-DI", "time-window-correlation", "five-level-authorship-spectrum", "first-author-wins"]

key-files:
  created:
    - "internal/correlation/correlator.go"
    - "internal/correlation/correlator_test.go"
    - "internal/authorship/classifier.go"
    - "internal/authorship/classifier_test.go"
  modified:
    - "internal/store/schema.go"
    - "internal/store/sqlite.go"

decisions:
  - id: "correlation-window-default"
    choice: "5000ms default window"
    why: "Matches locked decision for tight time windows; configurable via Correlator.WindowMs"
  - id: "exact-file-over-proximity"
    choice: "Exact file match always preferred over time proximity"
    why: "Higher confidence when Claude Write event targets the same file"
  - id: "authorship-in-authorship-pkg"
    choice: "AuthorshipLevel constants and CorrelationResult live in authorship package"
    why: "Avoids circular import; correlation package imports from authorship"
  - id: "confidence-thresholds"
    choice: "0.95 (exact <2s), 0.7 (exact 2-5s), 0.5 (proximity), 0.6 (git-coauthor), 0.8 (git-no-coauthor)"
    why: "Graduated confidence reflecting signal strength"

metrics:
  duration: "5 min"
  completed: "2026-02-09"
  tests-added: 23
  files-created: 4
  files-modified: 2
---

# Phase 02 Plan 01: Correlation Engine and Authorship Classifier Summary

Event correlation engine and five-level authorship classifier with 5s time-window matching, closest-in-time wins, first-author-wins history, and git-only fallback.

## What Was Built

### Schema Migration v2
Added `attributions` table to SQLite schema with columns for file path, project path, event IDs (file and session), authorship level, confidence score, uncertain flag, first author, correlation window, and timestamps. Four indexes for efficient querying by file, project, level, and timestamp.

### Store Query Methods
Seven new methods on `*Store`:
- `QueryFileEventsInWindow` - file events by path and time range
- `QueryFileEventsByProject` - all file events for a project since a time
- `QuerySessionEventsInWindow` - Write session events by file path and time range
- `QuerySessionEventsNearTimestamp` - Write session events within N ms of a timestamp (any file)
- `InsertAttribution` - persist an attribution record
- `QueryAttributionsByFile` - attributions for a file path
- `QueryAttributionsByProject` - attributions for a project path

### Correlation Engine (`internal/correlation/`)
`Correlator` struct with configurable `WindowMs` (default 5000ms). Uses `StoreReader` interface for dependency injection.

**Matching strategy (priority order):**
1. Exact file match: Write session event on same file path within window. Closest in time wins.
2. Time proximity: ANY Write session event within window regardless of file. Closest in time wins.
3. No match: No session activity detected.

Methods: `CorrelateFileEvent(fe)` for single events, `CorrelateAll(projectPath, since)` for batch.

### Authorship Classifier (`internal/authorship/`)
`Classifier` struct with three classification methods:

**`Classify(result)`** - Standard classification:
| Match Type | Time Delta | Level | Confidence |
|---|---|---|---|
| none | - | fully_human | 1.0 |
| exact_file | <2000ms | fully_ai | 0.95 |
| exact_file | 2000-5000ms | ai_first_human_revised | 0.7 |
| time_proximity | any | ai_suggested_human_written | 0.5 |

**`ClassifyWithHistory(result, prior)`** - First-author-wins:
- Prior=AI, current=no match -> ai_first_human_revised (human editing AI code)
- Prior=human, current=match -> human_first_ai_revised (AI editing human code)

**`ClassifyFromGit(hasCoauthorTag, coauthorName)`** - Git-only fallback:
- Co-author tag with "claude"/"anthropic" -> fully_ai, confidence 0.6
- No match -> fully_human, confidence 0.8

## Test Coverage

**Correlation tests (9 functions):** exact match within 1s, closest-wins with multiple candidates, no session events, time proximity fallback, outside window, batch correlation, exact preferred over proximity, pickClosest helper, absDurationMs helper.

**Authorship tests (14 functions):** all 5 levels via Classify, uncertain threshold boundary, history-based (human-editing-AI, AI-editing-human, no-prior), git-only (claude tag, anthropic tag, no tag, non-claude tag), first-author-wins, level constant values.

All 23 tests pass with `-race` flag.

## Deviations from Plan

None - plan executed exactly as written.

## Decisions Made

1. **CorrelationResult in authorship package** - The plan suggested defining it in correlation, but placing it in authorship avoids a circular import since correlation needs to import store types and authorship needs CorrelationResult.
2. **QueryFileEventsByProject added** - Required by CorrelateAll to fetch all project file events. Not explicitly in plan but necessary for the CorrelateAll method to work.
3. **Scanner helper functions** - Added `scanFileEvents`, `scanSessionEvents`, `scanAttributions` as private helpers in sqlite.go to DRY up row scanning logic.

## Commits

| Hash | Type | Description |
|---|---|---|
| 3884112 | feat | Store queries, schema migration, correlation engine, and authorship classifier |
| 67101a3 | test | Tests for correlation engine and authorship classifier |

## Next Phase Readiness

Phase 02 Plan 02 can proceed. The correlation engine and classifier are ready to be integrated into the daemon's event processing pipeline. The `StoreReader` interface enables easy testing of downstream consumers.
