# Architecture Research

**Domain:** Always-on developer tool daemon for AI code authorship attribution
**Researched:** 2026-02-09
**Confidence:** HIGH

## System Overview

```
                          WHO WROTE IT - SYSTEM ARCHITECTURE
                          ==================================

  INPUT SIGNALS                  DAEMON CORE                    OUTPUT
  =============            =====================              =========

  +-----------+            +---------------------+
  | FSEvents/ |---events-->|   Event Router      |
  | inotify   |            |   (debounce +       |
  +-----------+            |    filter)           |
                           +--------+------------+
  +-----------+                     |
  | Claude    |---tail/poll-->+-----v-----------+      +----------------+
  | Session   |               |  Correlation    |----->| Attribution    |
  | JSONL     |               |  Engine         |      | Store (SQLite) |
  +-----------+               +-----+-----------+      +-------+--------+
                                    |                          |
  +-----------+                     |                          |
  | Git Repo  |---on-demand--->+---v-----------+      +-------v--------+
  | (blame,   |                | Classification|      | Report         |
  |  log,     |                | Engine         |      | Generator      |
  |  diff)    |                +---------------+      +-------+--------+
  +-----------+                                               |
                                                     +--------v--------+
                                                     | CLI Output      |
                                                     | GitHub PR       |
                                                     | Comment         |
                                                     +-----------------+

  DAEMON LIFECYCLE
  ================
  [start] -> [idle: ~0 CPU] -> [active: event burst] -> [idle] -> ... -> [shutdown: flush + cleanup]
```

### Component Responsibilities

| Component | Responsibility | Typical Implementation |
|-----------|----------------|------------------------|
| **File Watcher** | Monitors working directory for file changes (create, modify, delete). Debounces rapid edits. Filters noise (build artifacts, .git, node_modules). | Go: `fsnotify/fsnotify` with recursive directory walking. macOS uses kqueue, Linux uses inotify. |
| **Session Tailer** | Continuously reads Claude Code JSONL session files. Extracts tool_use Write events, Bash commands, file-history-snapshots. Maintains read cursor position. | Go: `nxadm/tail` with `Follow: true` or custom inotify-based tailer on `~/.claude/projects/` |
| **Event Router** | Receives raw events from File Watcher and Session Tailer. Debounces, deduplicates, and dispatches to Correlation Engine via internal channel bus. | Go channels with fan-in pattern. Debounce window of 500ms-2s for file events. |
| **Correlation Engine** | Matches file changes to session activity within time windows. Determines whether a file change was AI-driven (session write precedes FS event) or human-driven. | Time-window correlation: if session Write for file X at time T, and FS event for file X at T+delta (delta < 2s), mark as AI-originated. |
| **Git Analyzer** | On-demand git operations: blame for line-level attribution, log for commit history, diff for change analysis, Co-Authored-By tag extraction. | Go: `go-git/go-git/v5` (pure Go, no CGO) for blame/log/diff. Parse commit messages for `Co-Authored-By`. |
| **Classification Engine** | Categorizes code contributions by work type: architecture, core logic, boilerplate, bug fix, test scaffolding, edge case handling. Heuristic-based, not LLM. | Rule-based classifier analyzing: file path patterns, AST complexity signals, session conversation context, change size/nature. |
| **Attribution Store** | Persists attribution data: per-line authorship, per-file work-type breakdown, per-session contribution summary, trend data over time. | SQLite in WAL mode. Schema: files, lines, sessions, attributions, classifications, snapshots. |
| **Report Generator** | Produces attribution reports on demand (CLI) or on trigger (PR creation). Formats as terminal output, Markdown for GitHub comments, JSON for programmatic access. | Go templates for output formatting. `gh` CLI or GitHub API for PR comment posting. |
| **Daemon Controller** | Manages daemon lifecycle: start, stop, status, config reload. Unix domain socket for CLI-to-daemon communication. | Socket-based IPC (not signals). PID file for process discovery. |

## Recommended Project Structure

```
who-wrote-it/
├── cmd/
│   ├── wwid/                  # Daemon binary ("who-wrote-it daemon")
│   │   └── main.go           # Entry point, signal handling, daemon setup
│   └── wwi/                   # CLI binary ("who-wrote-it")
│       └── main.go           # CLI commands: start, stop, status, report, pr
├── internal/
│   ├── daemon/                # Daemon lifecycle management
│   │   ├── daemon.go         # Start, run loop, graceful shutdown
│   │   ├── config.go         # Configuration loading/validation
│   │   └── socket.go         # Unix domain socket server for CLI communication
│   ├── watcher/               # File system event watching
│   │   ├── watcher.go        # FSEvents/inotify abstraction
│   │   ├── debounce.go       # Event debouncing and deduplication
│   │   └── filter.go         # Path filtering (gitignore-aware)
│   ├── session/               # Claude Code session parsing
│   │   ├── tailer.go         # JSONL file tailing with cursor persistence
│   │   ├── parser.go         # JSONL line parsing into typed events
│   │   └── types.go          # Session event type definitions
│   ├── router/                # Internal event bus
│   │   ├── router.go         # Fan-in from watchers, fan-out to processors
│   │   └── events.go         # Unified event types
│   ├── correlator/            # File change + session data correlation
│   │   ├── correlator.go     # Time-window matching engine
│   │   ├── window.go         # Sliding window implementation
│   │   └── attribution.go    # Attribution decision logic
│   ├── git/                   # Git operations
│   │   ├── blame.go          # Line-level blame queries
│   │   ├── log.go            # Commit history analysis
│   │   ├── diff.go           # Change diff analysis
│   │   └── coauthor.go       # Co-Authored-By tag extraction
│   ├── classifier/            # Work-type classification
│   │   ├── classifier.go     # Main classifier orchestrator
│   │   ├── rules.go          # Heuristic rule definitions
│   │   ├── path.go           # File path pattern analysis
│   │   └── complexity.go     # Change complexity heuristics
│   ├── store/                 # Attribution data persistence
│   │   ├── store.go          # Store interface
│   │   ├── sqlite.go         # SQLite implementation
│   │   ├── schema.go         # Database schema and migrations
│   │   └── queries.go        # Prepared query definitions
│   └── report/                # Report generation
│       ├── report.go         # Report data aggregation
│       ├── cli.go            # Terminal output formatter
│       ├── markdown.go       # GitHub PR comment formatter
│       └── json.go           # JSON output for programmatic use
├── pkg/
│   └── types/                 # Shared types used by both cmd/ binaries
│       ├── attribution.go    # Attribution model types
│       └── config.go         # Configuration types
├── .planning/                 # Project planning (this research)
├── go.mod
├── go.sum
├── Makefile                   # Build, test, install targets
└── README.md
```

### Structure Rationale

- **cmd/ split (wwid + wwi):** Two separate binaries. The daemon (`wwid`) runs continuously in the background. The CLI (`wwi`) is a lightweight client that communicates with the daemon over a Unix domain socket. This separation means the CLI binary is tiny and starts instantly.
- **internal/:** All business logic is unexported. Nothing in `internal/` is importable by external packages. This enforces encapsulation.
- **Package-per-concern:** Each package owns one domain concept. `watcher/` knows about file events but nothing about sessions. `session/` knows about JSONL parsing but nothing about git. The `correlator/` is the only package that combines signals from both.
- **store/ behind interface:** The `Store` interface in `store.go` allows swapping SQLite for something else later (e.g., DuckDB for analytics) without changing upstream code.
- **pkg/types/:** Minimal shared types needed by both binaries. Keep this extremely small -- only data structures, no logic.

## Architectural Patterns

### Pattern 1: Fan-In Event Bus with Go Channels

**What:** Multiple input sources (file watcher, session tailer) emit events into separate channels. A router goroutine merges them into a single stream using `select`, then dispatches to the correlation engine.

**When to use:** Always. This is the central nervous system of the daemon.

**Trade-offs:** Simple and native to Go. No external dependencies. The router is a single goroutine, which becomes a bottleneck only at extremely high event rates (thousands per second) -- far beyond what a single developer generates.

**Example:**
```go
// router/router.go
type Router struct {
    fsEvents      <-chan watcher.Event
    sessionEvents <-chan session.Event
    output        chan<- CorrelatedEvent
}

func (r *Router) Run(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case ev := <-r.fsEvents:
            r.handleFSEvent(ev)
        case ev := <-r.sessionEvents:
            r.handleSessionEvent(ev)
        }
    }
}
```

### Pattern 2: Time-Window Correlation

**What:** The correlation engine maintains a sliding window of recent session events (last 5-10 seconds). When a file system event arrives, it checks the window for a matching session Write event targeting the same file path. If found within the time threshold, the change is attributed to AI.

**When to use:** For every file change event. This is the core attribution logic.

**Trade-offs:** Simple and deterministic. The time window size is the key tuning parameter -- too small and we miss slow writes, too large and we get false correlations. Start with 2-second window, make configurable.

**Example:**
```go
// correlator/window.go
type Window struct {
    mu     sync.Mutex
    events []TimestampedEvent
    ttl    time.Duration // default 5s
}

func (w *Window) Add(ev TimestampedEvent) {
    w.mu.Lock()
    defer w.mu.Unlock()
    w.events = append(w.events, ev)
    w.prune() // remove events older than TTL
}

func (w *Window) FindMatch(filePath string, at time.Time) (TimestampedEvent, bool) {
    w.mu.Lock()
    defer w.mu.Unlock()
    // Search backward (most recent first) for session Write to this file
    for i := len(w.events) - 1; i >= 0; i-- {
        ev := w.events[i]
        if ev.FilePath == filePath && at.Sub(ev.Timestamp) < w.ttl {
            return ev, true
        }
    }
    return TimestampedEvent{}, false
}
```

### Pattern 3: Daemon Lifecycle via Context Tree

**What:** The daemon creates a root context from `signal.NotifyContext(SIGINT, SIGTERM)`. All subsystems receive derived contexts. Shutdown cascades naturally through context cancellation. Each subsystem runs in an `errgroup` goroutine.

**When to use:** Always. This is the daemon's lifecycle backbone.

**Trade-offs:** Clean and idiomatic Go. Context cancellation is cooperative -- each subsystem must check `ctx.Done()` in its loops. The errgroup ensures all goroutines complete before the daemon exits.

**Example:**
```go
// daemon/daemon.go
func Run() error {
    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    cfg := loadConfig()
    db := store.Open(cfg.DBPath)
    defer db.Close()

    g, ctx := errgroup.WithContext(ctx)

    fsCh := make(chan watcher.Event, 256)
    sessCh := make(chan session.Event, 256)
    corrCh := make(chan CorrelatedEvent, 256)

    g.Go(func() error { return watcher.Run(ctx, cfg.WatchDir, fsCh) })
    g.Go(func() error { return session.Run(ctx, cfg.SessionDir, sessCh) })
    g.Go(func() error { return router.Run(ctx, fsCh, sessCh, corrCh) })
    g.Go(func() error { return correlator.Run(ctx, corrCh, db) })
    g.Go(func() error { return daemon.ServeSocket(ctx, cfg.SocketPath, db) })

    return g.Wait()
}
```

### Pattern 4: CLI-to-Daemon Communication via Unix Domain Socket

**What:** The daemon listens on a Unix domain socket (e.g., `/tmp/wwi-<uid>.sock`). The CLI binary connects to this socket to send commands (status, report, trigger-pr-report) and receive responses. This replaces signal-based IPC.

**When to use:** All CLI commands that need daemon state. Direct commands (like `wwi init` to set up a project) can run independently.

**Trade-offs:** More robust than signals (which are "one of the worst designed parts of Unix" per Laurence Tratt). Allows structured request/response communication. Adds complexity of a small protocol, but JSON-over-socket is straightforward.

**Example:**
```go
// daemon/socket.go
type Request struct {
    Command string          `json:"command"`
    Args    json.RawMessage `json:"args,omitempty"`
}

type Response struct {
    Status string          `json:"status"`
    Data   json.RawMessage `json:"data,omitempty"`
    Error  string          `json:"error,omitempty"`
}

func ServeSocket(ctx context.Context, path string, db *store.Store) error {
    listener, err := net.Listen("unix", path)
    if err != nil {
        return err
    }
    defer os.Remove(path)

    go func() {
        <-ctx.Done()
        listener.Close()
    }()

    for {
        conn, err := listener.Accept()
        if err != nil {
            if ctx.Err() != nil {
                return nil // graceful shutdown
            }
            continue
        }
        go handleConnection(conn, db)
    }
}
```

## Data Flow

### Primary Flow: File Change Attribution

```
[Developer saves file]
       |
       v
[1] FSEvents/inotify fires
       |
       v
[2] Watcher: debounce (500ms), filter (ignore .git/, build/, etc.)
       |
       v
[3] Router: receives FileChanged{path, timestamp, eventType}
       |
       v
[4] Correlator: check session window for matching Write event
       |
       +--[Match found]----> [5a] Mark as AI-contributed
       |                           - Extract session context (conversation, tool_use)
       |                           - Determine authorship category
       |
       +--[No match]-------> [5b] Mark as human-contributed
                                   - Will refine later with git blame
       |
       v
[6] Classifier: analyze work type
       - File path heuristics (test file? config? core logic?)
       - Change complexity (new file vs small edit vs refactor)
       - Session conversation context (if AI: what was the prompt about?)
       |
       v
[7] Store: persist to SQLite
       - attribution record (file, line range, author, session_id)
       - classification record (work_type, confidence, reasoning)
       - update file-level aggregates
```

### Secondary Flow: PR Report Generation

```
[Developer runs: wwi report --pr or git hook fires on branch push]
       |
       v
[1] CLI sends "generate-report" command to daemon via socket
       |
       v
[2] Daemon: query store for all attributions since branch diverged from main
       |
       v
[3] Git Analyzer: run blame on changed files, get diff stats,
                   extract Co-Authored-By tags from commits
       |
       v
[4] Report Generator: aggregate data
       - Per-file: % AI vs human, work-type breakdown
       - Per-PR:   overall collaboration pattern
       - Classification: architecture/core/boilerplate/test/fix distribution
       |
       v
[5] Output: format as CLI table / GitHub Markdown / JSON
       |
       +--[CLI]-----> Print to terminal
       +--[GitHub]--> POST to GitHub PR as comment (gh api or GitHub API)
       +--[JSON]----> Write to stdout for piping
```

### Session Data Flow (Detail)

Understanding the Claude Code JSONL format is critical. Based on examination of actual session files:

```
~/.claude/projects/<project-slug>/<session-uuid>.jsonl
       |
       v
[Each line is one JSON object with these key fields:]
  - type: "user" | "assistant" | "progress" | "file-history-snapshot"
  - sessionId: UUID
  - timestamp: ISO 8601
  - message.content[].type: "text" | "tool_use" | "tool_result"

[Key events to extract:]

  1. tool_use with name="Write":
     - input.file_path  -> which file AI wrote
     - input.content    -> what AI wrote (full file content)
     - timestamp        -> when

  2. tool_result for Write:
     - toolUseResult.type: "create" | (other)
     - toolUseResult.filePath -> confirmed written path
     - toolUseResult.content  -> confirmed content

  3. tool_use with name="Bash":
     - input.command -> what command was run (may create/modify files)

  4. file-history-snapshot:
     - snapshot.trackedFileBackups -> files being tracked
     - Useful for understanding file state at point in time

  5. user messages:
     - The prompt that led to AI actions (useful for classification)
     - Contains conversation context about what work is being done
```

### Key Data Flows Summary

1. **File events -> Correlation -> Store:** The hot path. Must be non-blocking and fast. File events are debounced and correlated against the session window before persisting.
2. **Session JSONL -> Parser -> Session Window:** The context path. Continuously tails session files and maintains an in-memory window of recent AI actions.
3. **Store -> Git Analyzer -> Report Generator -> Output:** The query path. On-demand, triggered by CLI command or git hook. Can be slower since it runs interactively.

## Scaling Considerations

This is a single-developer, local-machine tool. "Scaling" here means handling large repos and high-frequency save patterns without degrading the developer's experience.

| Concern | Small repo (< 1K files) | Large repo (10K-100K files) | Monorepo (100K+ files) |
|---------|------------------------|-----------------------------|------------------------|
| File watching | Watch project root recursively. Negligible overhead. | Watch only src/ directories. Use gitignore patterns to exclude build artifacts. | Watch only changed directories. Use a manifest of active directories. |
| SQLite size | < 10MB. No concerns. | 10-100MB. Enable VACUUM on shutdown. | 100MB-1GB. Consider partitioning by time period. Archive old data. |
| Memory (idle) | < 10MB RSS. Session window is tiny. | < 20MB RSS. Larger gitignore filter set. | < 50MB RSS. May need to limit watched directories. |
| Memory (active) | < 30MB during report generation. | < 100MB for large diffs. | Stream diffs instead of loading into memory. |
| CPU (idle) | ~0%. Blocked on channel select. | ~0%. Same pattern. | ~0%. Same pattern. |
| CPU (active burst) | Brief spike during correlation + classification. | Noticeable spike during blame on large files. | Blame caching required for frequently changed files. |

### Scaling Priorities

1. **First bottleneck: File watcher descriptor limits.** On Linux, `fs.inotify.max_user_watches` defaults to 8192. Large repos may exceed this. **Mitigation:** Monitor watched directory count, warn user, provide guidance to increase limit. On macOS with kqueue, each watched path consumes a file descriptor -- same concern.
2. **Second bottleneck: Git blame performance on large files.** Blame is O(n * commits) and can take seconds on files with long histories. **Mitigation:** Cache blame results keyed on (file_path, HEAD commit hash). Invalidate on new commits. Only re-blame changed lines.
3. **Third bottleneck: SQLite write contention during rapid saves.** During heavy coding with frequent saves, the daemon may receive bursts of events. **Mitigation:** WAL mode handles concurrent reads + single writer. Batch writes using a write-behind buffer that flushes every 1-2 seconds.

## Anti-Patterns

### Anti-Pattern 1: Polling Instead of Event-Driven File Watching

**What people do:** Use `time.Ticker` to periodically scan directories for changes.
**Why it's wrong:** Wastes CPU when idle. Misses rapid changes between polls. Scales terribly with large repos. Violates the "negligible idle CPU" requirement.
**Do this instead:** Use OS-native file watching (`fsnotify` which uses inotify/kqueue/FSEvents). Block on channel receive when idle, consuming zero CPU.

### Anti-Pattern 2: Parsing Entire Session File on Every Change

**What people do:** When the session JSONL file changes, re-read and re-parse the entire file.
**Why it's wrong:** Session files grow throughout a coding session (the examined file was 285KB for a short session). Re-parsing on every append is O(n) per event.
**Do this instead:** Tail the file. Maintain a byte offset cursor. On change, seek to cursor position, read only new lines, advance cursor. Use `nxadm/tail` library or custom implementation with `os.File.Seek`.

### Anti-Pattern 3: Signal-Based Daemon Control

**What people do:** Use SIGHUP for config reload, SIGUSR1 for status, etc.
**Why it's wrong:** Signals are unreliable for structured communication. PID files go stale. Signal handlers have severe restrictions on what operations are safe. Can't send parameters or receive responses.
**Do this instead:** Unix domain socket for all CLI-to-daemon communication. Structured JSON request/response protocol. Socket existence is the daemon health check.

### Anti-Pattern 4: Monolithic Binary With In-Process CLI

**What people do:** Single binary that is both daemon and CLI. Parse arguments to decide mode.
**Why it's wrong:** The daemon stays resident with all its state. The CLI should start instantly and exit fast. Mixing them means the CLI loads unnecessary subsystems (file watcher, session tailer, database) just to send a status query.
**Do this instead:** Two binaries. `wwid` (daemon) and `wwi` (CLI client). The CLI is a thin socket client. The daemon is the heavy process.

### Anti-Pattern 5: Using an ORM or Heavy Database Abstraction

**What people do:** Use GORM or similar for SQLite access in a performance-critical daemon.
**Why it's wrong:** ORMs add allocation overhead, reflection, and unpredictable query generation. For a daemon with a known, fixed schema and specific query patterns, this is pure overhead.
**Do this instead:** Use `database/sql` directly with `mattn/go-sqlite3`. Define prepared statements at startup. Write explicit SQL. The schema is small enough (6-8 tables) that manual SQL is clearer and faster.

### Anti-Pattern 6: Trying to Use CGO-Free SQLite for Simplicity

**What people do:** Use `modernc.org/sqlite` (pure Go SQLite) to avoid CGO.
**Why it's wrong:** Pure Go SQLite implementations are 2-5x slower than the C-based `mattn/go-sqlite3` for write-heavy workloads. For a daemon that writes on every file save, this performance gap matters.
**Do this instead:** Accept the CGO dependency for `mattn/go-sqlite3`. It is battle-tested, fully featured, and significantly faster. Set `CGO_ENABLED=1` in the build process. Cross-compilation is more complex but solvable with `zig cc` as a cross-compiler.

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| Claude Code Session Files | Tail `~/.claude/projects/<slug>/*.jsonl` | Read-only. Session files are append-only JSONL. New sessions create new files. Must discover new session files dynamically. |
| Git Repository | `go-git/go-git/v5` for blame/log/diff | Pure Go, no libgit2 dependency. Operates on `.git` directory directly. Read-only for attribution; never writes to repo. |
| GitHub API | `gh` CLI or `google/go-github` library | For posting PR comments. Triggered by CLI command or git hook. Needs a GitHub token (from `gh auth`). |
| macOS FSEvents / Linux inotify | Via `fsnotify/fsnotify` | OS-level file watching. Requires appropriate permissions. On Linux, check `max_user_watches`. |
| Unix Domain Socket | `net.Listen("unix", path)` | CLI-to-daemon IPC. Socket at `/tmp/wwi-<uid>.sock` or `$XDG_RUNTIME_DIR/wwi.sock`. |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| Watcher -> Router | `chan watcher.Event` (buffered, 256) | One-way. Watcher produces, Router consumes. Backpressure via channel blocking. |
| Session Tailer -> Router | `chan session.Event` (buffered, 256) | One-way. Same pattern as watcher. |
| Router -> Correlator | `chan CorrelatedEvent` (buffered, 256) | One-way. Router enriches and dispatches. |
| Correlator -> Store | Direct function call | Correlator calls `store.PersistAttribution()`. Synchronous but fast (SQLite WAL). |
| CLI -> Daemon | Unix domain socket (JSON) | Request/response. CLI blocks until response received. Timeout after 5s. |
| Daemon -> GitHub | HTTP API call | Async from daemon perspective. Fire-and-forget with error logging. |

## Database Schema (Conceptual)

```sql
-- Core attribution data
CREATE TABLE files (
    id          INTEGER PRIMARY KEY,
    path        TEXT NOT NULL UNIQUE,
    first_seen  DATETIME NOT NULL,
    last_seen   DATETIME NOT NULL
);

CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,  -- Claude session UUID
    project     TEXT NOT NULL,
    started_at  DATETIME NOT NULL,
    ended_at    DATETIME,
    jsonl_path  TEXT NOT NULL
);

CREATE TABLE attributions (
    id           INTEGER PRIMARY KEY,
    file_id      INTEGER REFERENCES files(id),
    session_id   TEXT REFERENCES sessions(id),  -- NULL if human-only
    line_start   INTEGER NOT NULL,
    line_end     INTEGER NOT NULL,
    author_type  TEXT NOT NULL,  -- 'ai_full', 'ai_then_human', 'human_then_ai', 'ai_suggested', 'human_full'
    work_type    TEXT NOT NULL,  -- 'architecture', 'core_logic', 'boilerplate', 'bug_fix', 'test', 'edge_case', 'config'
    confidence   REAL NOT NULL,  -- 0.0 to 1.0
    created_at   DATETIME NOT NULL,
    commit_hash  TEXT,
    UNIQUE(file_id, line_start, line_end, commit_hash)
);

CREATE TABLE snapshots (
    id          INTEGER PRIMARY KEY,
    commit_hash TEXT NOT NULL UNIQUE,
    branch      TEXT NOT NULL,
    created_at  DATETIME NOT NULL,
    summary     TEXT  -- JSON blob with aggregate stats
);

-- Index for fast PR report queries
CREATE INDEX idx_attributions_commit ON attributions(commit_hash);
CREATE INDEX idx_attributions_file ON attributions(file_id);
CREATE INDEX idx_attributions_session ON attributions(session_id);
```

## Build Order (Dependencies Between Components)

The components have clear dependency relationships that dictate build order:

```
Phase 1: Foundation
  ├── store/ (SQLite schema, migrations, basic CRUD)
  ├── pkg/types/ (shared type definitions)
  └── daemon/ (skeleton: start, stop, signal handling, socket listener)

Phase 2: Input Signals
  ├── watcher/ (file system events with debouncing)
  │   └── Depends on: pkg/types
  ├── session/ (JSONL parser + tailer)
  │   └── Depends on: pkg/types
  └── router/ (fan-in event bus)
      └── Depends on: watcher, session, pkg/types

Phase 3: Core Logic
  ├── correlator/ (time-window matching)
  │   └── Depends on: router, store, session
  ├── git/ (blame, log, diff, Co-Authored-By)
  │   └── Depends on: pkg/types
  └── classifier/ (work-type heuristics)
      └── Depends on: git, session, pkg/types

Phase 4: Output
  ├── report/ (aggregation + formatting)
  │   └── Depends on: store, git, classifier
  ├── cmd/wwi (CLI client)
  │   └── Depends on: daemon/socket (client side), report
  └── cmd/wwid (daemon binary)
      └── Depends on: everything in internal/

Phase 5: Integration
  ├── GitHub PR comment posting
  ├── Git hooks for automatic report triggering
  └── Trend analysis and historical views
```

**Rationale:** Build storage first because everything writes to it. Build inputs second because the correlation engine needs events to process. Build core logic third because it transforms raw signals into attribution data. Build output last because it queries the accumulated data.

## Daemon Lifecycle

### Startup Sequence

```
[wwid start]
    |
    v
1. Parse config (CLI flags -> env vars -> config file -> defaults)
2. Check for existing daemon (probe socket)
   - If running: exit with "daemon already running"
3. Open/create SQLite database, run migrations
4. Create Unix domain socket listener
5. Write PID file (optional, socket is primary health indicator)
6. Start subsystems in errgroup:
   a. File watcher (begin watching project directory)
   b. Session tailer (begin tailing active session files)
   c. Event router (begin fan-in from watchers)
   d. Correlator (begin processing correlated events)
   e. Socket server (begin accepting CLI connections)
7. Log "daemon started, watching <dir>"
8. Block on errgroup.Wait() or ctx.Done()
```

### Idle State

```
[All goroutines blocked on channel select or socket accept]
    - CPU: ~0% (no polling, no timers except optional keepalive)
    - Memory: ~10-20MB RSS (Go runtime + loaded config + empty channel buffers)
    - Disk: 0 I/O (no periodic writes)
    - Network: 0 (socket listener is passive)
```

### Active State (Event Burst)

```
[Developer saves a file / AI writes via tool_use]
    |
    v
1. FSEvents fires -> watcher goroutine wakes, sends to channel
2. Session tailer reads new JSONL line -> sends to channel
3. Router goroutine wakes, receives both events
4. Correlator matches events, classifies, persists to SQLite
5. All goroutines return to blocked state
    - Total burst duration: < 50ms for a single file save
    - CPU spike: brief, single-core
    - Memory: minimal allocation (reuse buffers where possible)
```

### Shutdown Sequence

```
[SIGINT or SIGTERM received, or CLI sends "shutdown" command]
    |
    v
1. Root context canceled via signal.NotifyContext
2. All subsystems receive ctx.Done():
   a. Socket server: stop accepting, close listener, drain active connections
   b. File watcher: close fsnotify watcher, release file descriptors
   c. Session tailer: flush current position, close tail
   d. Correlator: process remaining buffered events, flush to store
   e. Router: drain channels
3. errgroup.Wait() returns
4. Close SQLite database (final WAL checkpoint)
5. Remove socket file and PID file
6. Exit 0
    - Total shutdown time: < 2 seconds
    - Data safety: all buffered attributions flushed to SQLite before exit
```

### Config Reload (Runtime)

```
[CLI sends "reload-config" command via socket]
    |
    v
1. Daemon reads updated config file
2. Apply changes that can be hot-reloaded:
   - Watch directory additions/removals
   - Gitignore pattern updates
   - Classification rule changes
   - Report format preferences
3. Changes requiring restart (logged as warning):
   - Database path change
   - Socket path change
4. Respond to CLI with "config reloaded" + what changed
```

## Sources

- [fsnotify/fsnotify GitHub](https://github.com/fsnotify/fsnotify) - Cross-platform filesystem notifications for Go. Verified: supports kqueue (macOS), inotify (Linux). No recursive watching built-in. **[HIGH confidence]**
- [go-git/go-git GitHub](https://github.com/go-git/go-git) - Pure Go git implementation. Verified: blame, log, diff support. v5.16.5 current (Feb 2026). **[HIGH confidence]**
- [nxadm/tail GitHub](https://github.com/nxadm/tail) - Active fork of hpcloud/tail for tailing files in Go. Supports Follow mode with truncation/rotation detection. **[HIGH confidence]**
- [mattn/go-sqlite3](https://github.com/mattn/go-sqlite3) - CGO-based SQLite driver for Go. Best tested, most featureful option. **[HIGH confidence]**
- [Go + SQLite Best Practices](https://jacob.gold/posts/go-sqlite-best-practices/) - WAL mode, busy timeout, NORMAL sync, connection pooling guidance. **[MEDIUM confidence]**
- [Go Concurrency Patterns: Pipelines](https://go.dev/blog/pipelines) - Official Go blog on pipeline/fan-in patterns with channels. **[HIGH confidence]**
- [Graceful Shutdown in Go: Practical Patterns](https://victoriametrics.com/blog/go-graceful-shutdown/) - signal.NotifyContext, errgroup, context tree patterns. **[MEDIUM confidence]**
- [Some Reflections on Writing Unix Daemons](https://tratt.net/laurie/blog/2024/some_reflections_on_writing_unix_daemons.html) - Laurence Tratt on Unix domain sockets over signals for daemon IPC. **[MEDIUM confidence]**
- [Claude Code JSONL format](file:///Users/deepakbhimaraju/.claude/projects/) - Examined actual session files. Verified structure: type, sessionId, timestamp, message.content with tool_use/tool_result entries containing Write file_path and content. **[HIGH confidence - primary source]**
- [SemEval-2026 Task 13: GenAI Code Detection & Attribution](https://github.com/GiovanniIacuzzo/SemEval-2026-Task-13) - Academic competition on AI code attribution. Confirms the domain is active and unsolved at scale. **[LOW confidence - tangential]**
- [Go Project Structure: Clean Architecture Patterns](https://dasroot.net/posts/2026/01/go-project-structure-clean-architecture/) - cmd/ + internal/ layout conventions for Go services. **[MEDIUM confidence]**

---
*Architecture research for: always-on developer tool daemon for AI code authorship attribution*
*Researched: 2026-02-09*
