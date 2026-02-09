---
phase: 01-data-pipeline
plan: 01
subsystem: daemon
tags: [go, sqlite, unix-socket, ipc, cobra, daemon, wal]

# Dependency graph
requires: []
provides:
  - "Go module github.com/anthropic/who-wrote-it with cobra CLI"
  - "Daemon lifecycle with SIGTERM/SIGINT handling"
  - "SQLite store in WAL mode with 6-table schema"
  - "Unix domain socket IPC (JSON protocol) for CLI-daemon communication"
  - "Config system with sensible defaults (~/.whowroteit)"
affects:
  - 01-02 (file watcher plugs into daemon and store)
  - 01-03 (session parser and git integration plug into daemon and store)
  - 02-01 (correlation engine reads from store tables)

# Tech tracking
tech-stack:
  added:
    - "modernc.org/sqlite v1.45.0 (pure Go SQLite, no CGO)"
    - "github.com/spf13/cobra v1.10.2 (CLI framework)"
  patterns:
    - "Interface-based dependency injection to avoid circular imports (daemon.IPCServer, ipc.DaemonQuerier, ipc.StoreQuerier)"
    - "JSON-over-Unix-socket protocol (newline-delimited JSON)"
    - "Version-based transactional schema migrations"
    - "Signal-aware context via signal.NotifyContext"

key-files:
  created:
    - cmd/whowroteit/main.go
    - internal/daemon/daemon.go
    - internal/daemon/signals.go
    - internal/store/sqlite.go
    - internal/store/schema.go
    - internal/store/migrations.go
    - internal/ipc/server.go
    - internal/ipc/client.go
    - internal/ipc/protocol.go
    - internal/config/config.go
    - go.mod
    - go.sum
    - .gitignore
  modified: []

key-decisions:
  - "Used modernc.org/sqlite (pure Go) instead of mattn/go-sqlite3 to avoid CGO and enable clean cross-compilation"
  - "Interface-based DI pattern to break daemon<->IPC circular dependency: daemon owns IPCServer interface, IPC server uses DaemonQuerier/StoreQuerier interfaces"
  - "Schema migrations use simple version counter in daemon_state table, no third-party migration library"
  - "Socket file placed in data dir (~/.whowroteit/whowroteit.sock) not /tmp for security"

patterns-established:
  - "Interface injection: daemon package defines interfaces, ipc package implements them"
  - "JSON-over-Unix-socket IPC: Request{command, args} -> Response{ok, data, error}"
  - "WAL mode SQLite with 5s busy timeout and foreign keys enabled"
  - "Transactional migrations with version tracking in daemon_state"
  - "Config loads from JSON file with defaults, derives paths from DataDir"

# Metrics
duration: 6min
completed: 2026-02-09
---

# Phase 1 Plan 1: Daemon Foundation Summary

**Go daemon with cobra CLI, SQLite WAL store (6 tables), Unix socket IPC, and clean signal-driven lifecycle**

## Performance

- **Duration:** 6 min
- **Started:** 2026-02-09T18:49:25Z
- **Completed:** 2026-02-09T18:55:57Z
- **Tasks:** 2
- **Files modified:** 13

## Accomplishments
- Daemon starts in foreground, creates SQLite DB in WAL mode, and shuts down cleanly on SIGTERM/SIGINT
- All 6 schema tables created: file_events, session_events, git_commits, git_diffs, git_blame_lines, daemon_state
- CLI communicates with daemon over Unix domain socket: ping, status (uptime + data counts), stop
- Stale socket detection works: daemon starts cleanly after simulated crash
- Already-running detection: second start attempt prints "already running" and exits
- Pure Go build confirmed (CGO_ENABLED=0 compiles cleanly)

## Task Commits

Each task was committed atomically:

1. **Task 1: Go module, daemon lifecycle, and SQLite store** - `8a3ee68` (feat)
2. **Task 2: Unix domain socket IPC server and client** - `fe3899b` (feat)

## Files Created/Modified
- `cmd/whowroteit/main.go` - CLI entry point with start/stop/status/ping subcommands using cobra
- `internal/daemon/daemon.go` - Daemon struct with Start/Stop/shutdown lifecycle, store and IPC server orchestration
- `internal/daemon/signals.go` - SIGTERM/SIGINT handling via signal.NotifyContext
- `internal/store/sqlite.go` - SQLite connection with WAL mode, busy timeout, count/size query methods
- `internal/store/schema.go` - CREATE TABLE statements for all 6 tables with indexes
- `internal/store/migrations.go` - Version-based migration runner with transactional upgrades
- `internal/ipc/server.go` - Unix domain socket server with per-connection goroutines and 5s drain timeout
- `internal/ipc/client.go` - Unix domain socket client with Ping/Status/RequestStop methods
- `internal/ipc/protocol.go` - Request/Response/StatusData JSON message types
- `internal/config/config.go` - Config struct with defaults, JSON file loading, data dir creation
- `go.mod` - Module definition with cobra and modernc.org/sqlite dependencies
- `go.sum` - Dependency checksums
- `.gitignore` - Binary and IDE exclusions

## Decisions Made
- **modernc.org/sqlite over mattn/go-sqlite3:** Pure Go driver avoids CGO dependency, enables cross-compilation. Trades ~2x slower writes for deployment simplicity. Correct choice for a CLI tool.
- **Interface-based DI for circular dependency:** Daemon defines IPCServer/StoreAware interfaces; IPC defines DaemonQuerier/StoreQuerier interfaces. Main wires them together. Clean separation.
- **Socket in data dir:** Placed socket at ~/.whowroteit/whowroteit.sock rather than /tmp/whowroteit.sock for better security (user-owned directory, 0600 permissions).
- **No background daemonization yet:** Start command runs in foreground only. Background process management deferred (not needed for Phase 1 development).
- **Go 1.25.7:** Installed Go 1.25.7 (latest stable). The concern about Go 1.26 Green Tea GC is moot since 1.25 is the current stable.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Installed Go 1.25.7 via Homebrew**
- **Found during:** Pre-task setup
- **Issue:** Go was not installed on the system (brew info showed "Not installed")
- **Fix:** Ran `brew install go` to get Go 1.25.7
- **Files modified:** None (system tool installation)
- **Verification:** `go version` returns `go1.25.7 darwin/arm64`
- **Impact:** Required to execute any task in this plan

**2. [Rule 2 - Missing Critical] Created IPC server/client in Task 1 instead of Task 2**
- **Found during:** Task 1 (daemon lifecycle)
- **Issue:** The daemon's Start() method requires an IPCServer implementation to compile and run. The plan separates IPC into Task 2, but the daemon cannot function without it.
- **Fix:** Implemented full IPC server/client/protocol in Task 1 alongside the daemon. Task 2 added the ping subcommand and verified end-to-end IPC flow.
- **Files modified:** internal/ipc/server.go, internal/ipc/client.go, internal/ipc/protocol.go
- **Verification:** Full IPC verification passed (ping, status, stop, stale socket, already-running)
- **Impact:** Task 2 commit is smaller than expected since most IPC code was in Task 1

---

**Total deviations:** 2 auto-fixed (1 blocking, 1 missing critical)
**Impact on plan:** Both deviations were necessary for correct execution. No scope creep. All planned functionality delivered.

## Issues Encountered
None - all verifications passed on first attempt.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Daemon foundation is complete and verified. Store is open with all tables.
- Ready for Plan 01-02 (file system watcher): watcher will plug into daemon as a subsystem and write to file_events table
- Ready for Plan 01-03 (session parser + git): parser and git integrator will plug into daemon and write to session_events, git_commits, git_diffs, git_blame_lines tables
- No blockers for subsequent plans

---
*Phase: 01-data-pipeline*
*Completed: 2026-02-09*
