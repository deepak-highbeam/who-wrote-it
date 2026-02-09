---
phase: 01-data-pipeline
plan: 04
subsystem: daemon
tags: [go, daemon, goroutine, session-parser, git-integration, wiring, gap-closure]

# Dependency graph
requires:
  - 01-01 (daemon lifecycle, store, IPC)
  - 01-02 (file system watcher)
  - 01-03 (session parser and git integration packages)
provides:
  - "Session parser wired into daemon: Discover -> WatchForNew -> Tail -> ParseLine -> InsertSessionEvent"
  - "Git integration wired into daemon: Open -> initial SyncCommits -> periodic ticker SyncCommits"
  - "Offset persistence for session tailers via daemon_state"
  - "Ordered shutdown: session/git cancel before watcher/store close"
affects:
  - 02-01 (correlation engine can now read live session_events and git_commits from store)
  - Phase 1 verification (truths #3 and #4 now closable)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Per-subsystem context.WithCancel for independent goroutine lifecycle"
    - "Offset persistence via daemon_state KV store for resume across restarts"
    - "Non-fatal subsystem errors (log and continue, daemon stays alive)"

# File tracking
key-files:
  modified:
    - "internal/daemon/daemon.go"

# Decisions
decisions: []

# Metrics
duration: 2 min
completed: 2026-02-09
---

# Phase 01 Plan 04: Wire Session Parser and Git Integration Summary

Session parser and git integration packages wired into daemon Start()/shutdown() lifecycle via goroutine orchestration with per-subsystem cancellation contexts and offset persistence.

## What Was Done

### Task 1: Wire session parser and git integration into daemon lifecycle

Modified `internal/daemon/daemon.go` to connect the two orphaned subsystems (sessionparser and gitint) that were built in 01-03 but never called from the daemon.

**Changes to daemon struct:**
- Added `sessionParser *sessionparser.ClaudeCodeParser` field
- Added `gitRepo *gitint.Repository` field
- Added `sessionCancel` and `gitCancel` context.CancelFunc fields for independent shutdown

**Changes to Start():**
- Session parser block: Creates ClaudeCodeParser, calls Discover() to find existing session files, starts a tailer goroutine for each via startSessionTailer(), starts WatchForNew() goroutine for session rotation, starts consumer goroutine for new session files
- Git integration block: Opens git repo at first watch path (non-fatal if not a git repo), runs initial SyncCommits() with 30-day lookback, starts periodic sync goroutine with 30-second ticker

**New helper method startSessionTailer():**
- Restores tailer offset from daemon_state KV store for resume across daemon restarts
- Creates Tailer and lines channel (buffered, 100)
- Goroutine 1: Tail() reads new lines, persists final offset on context cancellation
- Goroutine 2: ParseLine() each line, InsertSessionEvent() for tool_use events

**Changes to shutdown():**
- Cancel session goroutines (sessionCancel) BEFORE watcher stop -- allows tailers to persist offsets
- Cancel git goroutine (gitCancel) BEFORE watcher stop
- Existing shutdown order preserved: watcher -> IPC -> store -> socket cleanup

## Verification

All verification criteria passed:

| Check | Result |
|-------|--------|
| `go build ./...` | Zero errors |
| `go vet ./...` | Zero warnings |
| `go test -race ./...` | All tests pass (watcher, sessionparser, gitint) |
| sessionparser import | 6 references in daemon.go |
| gitint import | 6 references in daemon.go |
| Discover() call | Present |
| SyncCommits() call | 2 references (initial + periodic) |
| InsertSessionEvent() call | Present |
| startSessionTailer | 4 references (definition + 3 call sites) |

## Data Flow Paths Now Complete

**Session events path:**
```
~/.claude/projects/**/*.jsonl -> Discover() -> NewTailer() -> Tail() -> ParseLine() -> InsertSessionEvent() -> session_events table
```

**Git data path:**
```
.git/ -> gitint.Open() -> SyncCommits() -> processCommit() -> InsertGitCommit()/InsertGitDiff() -> git_commits/git_diffs tables
```

**File events path (already wired in 01-02):**
```
Watch paths -> fsnotify -> Watcher -> InsertFileEvent() -> file_events table
```

All three input signals now flow through the daemon into the SQLite store.

## Deviations from Plan

None -- plan executed exactly as written.

## Commits

| Hash | Message |
|------|---------|
| 5834ea2 | feat(01-04): wire session parser and git integration into daemon lifecycle |

## Next Phase Readiness

Phase 1 data pipeline is now fully wired. All three data sources (file events, session events, git commits/diffs/blame) flow into the store through the daemon. Phase 2 (correlation engine) can begin reading from these tables.
