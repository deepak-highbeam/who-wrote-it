---
phase: 01-data-pipeline
plan: 02
subsystem: watcher
tags: [go, fsnotify, debounce, filter, file-events, sqlite]

# Dependency graph
requires:
  - 01-01 (daemon lifecycle, store, config)
provides:
  - "File system watcher with fsnotify that monitors project directories recursively"
  - "Event debouncer with configurable 100ms window that collapses editor storms"
  - "Path filter with glob-based ignore patterns (defaults + user config)"
  - "InsertFileEvent store method for persisting file change events"
  - "Daemon integration: watcher starts/stops with daemon lifecycle"
affects:
  - 01-03 (session parser + git integration complete the data pipeline)
  - 02-01 (correlation engine reads file_events to match with session data)

# Tech tracking
tech-stack:
  added:
    - "github.com/fsnotify/fsnotify v1.9.0 (cross-platform file system notifications)"
  patterns:
    - "Per-path debouncing with timer map and mutex (timer-per-key pattern)"
    - "Component-based path matching: split path, match each component against patterns"
    - "Recursive directory walking with filtered skip (filepath.WalkDir + SkipDir)"
    - "Goroutine-based watcher with context cancellation for lifecycle"

key-files:
  created:
    - internal/watcher/watcher.go
    - internal/watcher/debounce.go
    - internal/watcher/filter.go
    - internal/watcher/watcher_test.go
  modified:
    - internal/daemon/daemon.go
    - internal/store/sqlite.go
    - go.mod
    - go.sum

key-decisions:
  - "100ms debounce window: balances responsiveness with burst collapse. Editors typically fire 2-5 events per save within 10-50ms."
  - "Component-based path matching over full-path glob: catches node_modules at any depth without requiring **/node_modules/**"
  - "Debouncer emits LAST event per path, not first: captures final state (e.g., modify after create)"
  - "Watcher stops before IPC in shutdown: ensures all pending debounced events are flushed to store before store closes"

patterns-established:
  - "Timer-per-key debouncing: map[path]*time.Timer with mutex, reset on new event, emit on expiry"
  - "Stop-drains pattern: Stop() cancels timers and immediately emits pending events"
  - "Daemon subsystem pattern: New() -> Start(ctx) -> Stop(), wired into daemon lifecycle"

# Metrics
duration: 4min
completed: 2026-02-09
---

# Phase 1 Plan 2: File System Watcher Summary

**fsnotify-based recursive file watcher with 100ms per-path debouncing, glob-pattern filtering, and SQLite event persistence**

## Performance

- **Duration:** 4 min
- **Started:** 2026-02-09T18:59:28Z
- **Completed:** 2026-02-09T19:03:58Z
- **Tasks:** 2
- **Files created:** 4
- **Files modified:** 4

## Accomplishments

- File creation, modification, deletion, and rename events flow from watched directories into SQLite file_events table
- Editor event storms are debounced: 10 rapid saves produce exactly 1 stored event (verified)
- .git, node_modules, .idea, .vscode, __pycache__, swap files, and other configured patterns produce zero events
- New subdirectories created in watched paths are automatically added to the watcher
- Status command shows non-zero file_events_count after generating events
- 11 unit tests cover filter and debouncer logic, all pass with race detector enabled

## Task Commits

Each task was committed atomically:

1. **Task 1: File system watcher with fsnotify, debouncing, and filtering** - `042af80` (feat)
2. **Task 2: Unit tests for debouncer and filter** - `2a5dab8` (test)

## Files Created/Modified

- `internal/watcher/watcher.go` - Watcher struct with fsnotify integration, recursive directory walking, event handling, daemon lifecycle (Start/Stop)
- `internal/watcher/debounce.go` - Debouncer with per-path timer map, configurable window, thread-safe Feed/Stop with drain
- `internal/watcher/filter.go` - Filter with glob patterns, component-based matching, default + custom pattern merge
- `internal/watcher/watcher_test.go` - 11 tests: 5 filter tests (defaults, normals, nested, custom, duplicates), 6 debouncer tests (single, burst, paths, drain, last-event, post-stop)
- `internal/daemon/daemon.go` - Added watcher field, Start() launches watcher goroutine, shutdown() stops watcher before IPC
- `internal/store/sqlite.go` - Added InsertFileEvent method for persisting file change events with RFC3339Nano timestamps
- `go.mod` / `go.sum` - Added github.com/fsnotify/fsnotify v1.9.0 dependency

## Decisions Made

- **100ms debounce window:** Editors (VS Code, vim, IntelliJ) typically fire 2-5 filesystem events per save within 10-50ms. A 100ms window comfortably collapses these while still being responsive enough for real-time monitoring.
- **Component-based path matching:** Rather than requiring glob patterns like `**/node_modules/**`, the filter splits each path into components and matches each one. This means `node_modules` catches `a/b/node_modules/c.js` naturally.
- **Emit last event, not first:** When a burst includes create then modify for the same path, the debouncer emits the modify (most recent). This captures the final state rather than the initial trigger.
- **Shutdown order:** Watcher stops before IPC server, which stops before store closes. This ensures debounced events flush to the store before the database connection is closed.

## Deviations from Plan

None -- plan executed exactly as written.

## Issues Encountered

None -- all verifications passed on first attempt. Integration test confirmed:
- Create event: 1 file created -> 1 event in store
- Debounce: 10 rapid writes -> 1 event in store
- Filter: .git/test and node_modules/test -> 0 events
- Recursive: mkdir sub && touch sub/file.txt -> events for both
- Delete: rm sub/file.txt -> delete event in store
- Status: file_events_count shows 4 after test sequence

## User Setup Required

None -- no external service configuration required.

## Next Phase Readiness

- File system watcher is complete and verified. Events flow into file_events table.
- Ready for Plan 01-03 (session parser + git): session parser will write to session_events table, git integrator will write to git_commits/git_diffs/git_blame_lines tables.
- Ready for Phase 2 (correlation engine): file_events table provides the "when did files change" signal that the correlation engine will match against session timestamps.
- No blockers for subsequent plans.

---
*Phase: 01-data-pipeline*
*Completed: 2026-02-09*
