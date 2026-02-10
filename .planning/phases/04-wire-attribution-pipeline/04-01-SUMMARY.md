---
phase: 04-wire-attribution-pipeline
plan: 01
subsystem: daemon-attribution-pipeline
tags: [correlation, authorship, worktype, daemon, goroutine, sqlite]
dependency_graph:
  requires: [01-01, 01-02, 01-03, 01-04, 02-01, 02-02, 03-01, 03-02]
  provides: [background-attribution-processor, unprocessed-events-query, attribution-pipeline-integration]
  affects: []
tech_stack:
  added: []
  patterns: [polling-goroutine, left-join-unprocessed-query, pipeline-composition]
key_files:
  created:
    - internal/daemon/daemon_test.go
  modified:
    - internal/store/sqlite.go
    - internal/daemon/daemon.go
decisions:
  - 2s polling interval for attribution processor (balances latency with CPU for burst event patterns)
  - Batch size 100 per tick (bounds per-tick processing time)
  - Empty diff/commit for work-type classification (file_events lack diff content; path heuristics still work)
  - Non-fatal errors: log and continue processing other events
metrics:
  duration: 3 min
  completed: 2026-02-10
---

# Phase 4 Plan 1: Wire Attribution Pipeline Summary

Background attribution processor goroutine wired into daemon, processing file events through correlation engine, authorship classifier, and work-type classifier into stored attribution records with 2s polling, 100-event batches, and LEFT JOIN-based unprocessed event query.

## What Was Done

### Task 1: Add QueryUnprocessedFileEvents store method and background attribution processor
- Added `QueryUnprocessedFileEvents(batchSize int)` method to `store.Store` using LEFT JOIN against attributions on `file_event_id` to find events without attribution records, ordered oldest-first, limited by batch size
- Added `startAttributionProcessor(ctx context.Context)` method to `Daemon` struct that creates a correlator, authorship classifier, and work-type classifier, then runs a 2-second ticker loop processing batches of unprocessed events
- Added imports for `correlation`, `authorship`, and `worktype` packages to daemon
- Added `attrCancel context.CancelFunc` field to Daemon struct
- Wired processor startup into `Start()` method (after git integration, before final log)
- Wired processor cancellation into `shutdown()` method (after git cancel, before watcher stop)
- Pipeline per event: `QueryUnprocessedFileEvents` -> `CorrelateFileEvent` -> `Classify` -> `ClassifyFile` -> `InsertAttribution` -> `UpdateAttributionWorkType`
- Commit: `aa9b6b4`

### Task 2: Add integration test for attribution pipeline
- Created `internal/daemon/daemon_test.go` with two integration tests
- `TestAttributionPipeline`: Inserts file event + session event at same timestamp, runs full pipeline, asserts exact_file match, FullyAI level (0.95 confidence), CoreLogic work type, and correct store persistence. Verifies no unprocessed events remain after processing.
- `TestAttributionPipelineNoSessionMatch`: Inserts file event only (no session event), runs full pipeline, asserts no match (FullyHuman, 1.0 confidence), CoreLogic work type, and correct store persistence.
- Both tests pass with `-race` flag
- Commit: `7b92327`

## Deviations from Plan

None -- plan executed exactly as written.

## Verification Results

| Check | Result |
|-------|--------|
| `go build ./...` | Pass (zero errors) |
| `go vet ./...` | Pass (zero warnings) |
| `go test -race ./...` | Pass (all 11 packages, 0 failures) |
| daemon imports correlation, authorship, worktype | Confirmed (3 packages) |
| startAttributionProcessor defined and called | Confirmed (line 345, 192) |
| QueryUnprocessedFileEvents in sqlite.go | Confirmed (line 417) |
| InsertAttribution in daemon.go | Confirmed (line 400) |
| TestAttributionPipeline passes | Pass |
| TestAttributionPipelineNoSessionMatch passes | Pass |

## Gap Closure Status

This plan closes the critical integration gap identified in the v1 milestone audit. The daemon now invokes all three intelligence packages (correlation, authorship, worktype) on incoming file events, producing fully-classified attribution records. The 8 previously unsatisfied requirements are now functionally wired:

| Requirement | Status |
|------------|--------|
| CORR-01: File events correlated with session events | Wired via `correlator.CorrelateFileEvent` |
| CORR-02: Time-window matching produces attribution records | Wired via `classifier.Classify` |
| AUTH-01: Five-level authorship spectrum assigned | Wired via `classifier.Classify` |
| AUTH-02: Confidence scores assigned | Wired via `classifier.Classify` |
| WTYP-01: Work-type labels assigned | Wired via `wtClassifier.ClassifyFile` |
| WTYP-02: Work types stored in attributions | Wired via `UpdateAttributionWorkType` |
| METR-01: AI metric computation has data | Attribution records now exist for metric queries |
| METR-02: Per-file and per-project metrics | Attribution records have project_path and file_path |

## Commits

| Hash | Message |
|------|---------|
| `aa9b6b4` | feat(04-01): add QueryUnprocessedFileEvents and background attribution processor |
| `7b92327` | test(04-01): add integration tests for attribution pipeline |
