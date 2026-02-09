---
phase: 01-data-pipeline
plan: 03
subsystem: session-parser-git-integration
tags: [session-parsing, jsonl, git, go-git, blame, co-author, polling-tailer, fsnotify]

dependency_graph:
  requires: ["01-01"]
  provides: ["session-parser", "git-integration", "store-methods", "daemon-state-kv"]
  affects: ["02-01", "02-02"]

tech_stack:
  added: ["github.com/go-git/go-git/v5"]
  patterns: ["SessionProvider interface for multi-tool extensibility", "polling-based file tailer", "daemon_state KV for offset/cursor tracking"]

file_tracking:
  created:
    - internal/sessionparser/provider.go
    - internal/sessionparser/parser.go
    - internal/sessionparser/tailer.go
    - internal/sessionparser/discovery.go
    - internal/sessionparser/parser_test.go
    - internal/gitint/repository.go
    - internal/gitint/commits.go
    - internal/gitint/blame.go
    - internal/gitint/gitint_test.go
  modified:
    - internal/store/sqlite.go
    - go.mod
    - go.sum

decisions:
  - id: CCSP-PARSE
    decision: "Defensive JSONL parsing with fast-path skip for non-tool_use lines"
    context: "Claude Code JSONL format has no stability contract"
  - id: TAIL-POLL
    decision: "Polling-based tailer (500ms) over fsnotify for individual files"
    context: "More reliable for files being appended to by other processes"
  - id: BLAME-SELECTIVE
    decision: "Blame runs selectively for changed files, not whole repo"
    context: "Blame is expensive; on-demand approach keeps sync fast"
  - id: MERGE-FIRST-PARENT
    decision: "Merge commits diff against first parent only"
    context: "Standard git merge diff behavior avoids double-counting"

metrics:
  duration: "7 min"
  completed: "2026-02-09"
  tasks: 2
  tests_added: 19
  files_created: 9
  files_modified: 3
---

# Phase 01 Plan 03: Session Parser and Git Integration Summary

**JSONL session parser with provider abstraction and go-git commit/blame/coauthor integration, with 19 tests and 7 store methods added.**

## Performance

- Duration: ~7 minutes
- 2 tasks, both auto
- 19 tests added (13 session parser + 6 git integration), all passing with -race

## Accomplishments

### Task 1: Claude Code Session Parser with Abstraction Layer
- **SessionProvider interface** (`provider.go`): Defines `Discover`, `WatchForNew`, `ParseLine` methods. Any AI tool (Copilot, Cursor) can implement this interface.
- **ClaudeCodeParser** (`parser.go`): Parses JSONL lines for `tool_use` events (Write, Read, Bash). SHA-256 content hashing for Write events. Fast-path skip for lines without "tool_use". Defensive against malformed JSON, unknown fields, missing fields.
- **Session discovery** (`discovery.go`): Recursive scan of `~/.claude/projects/` for `*.jsonl` files. fsnotify watcher for new session files (session rotation). Age-based filtering (default 24h).
- **File tailer** (`tailer.go`): Polling-based (500ms) tail from offset. Handles truncation, file removal, and context cancellation. Returns final offset for persistence.
- **Store methods**: `InsertSessionEvent`, `GetDaemonState`, `SetDaemonState` added to `sqlite.go`.
- **13 tests**: Write/Read/Bash parsing, non-tool_use skip, malformed JSON, BOM handling, top-level content format, discovery, old file filtering, tailer with live append.

### Task 2: Git Integration with go-git
- **Repository** (`repository.go`): Wraps go-git with store for persistence. `SyncCommits` iterates from HEAD, stops at last-synced commit. Uses `daemon_state` for cursor tracking.
- **Commit parsing** (`commits.go`): Per-file diffs with additions/deletions. Rename detection (old_path + new file_path). Merge commits diff against first parent. Initial commits treated as all-additions.
- **Co-Authored-By detection**: Case-insensitive regex with multi-line mode. Handles variations: `Co-Authored-By`, `Co-authored-by`, `co-authored-by`. `AllCoAuthors` for multiple trailers.
- **Blame** (`blame.go`): Per-line authorship with content hashing. `BlameAndStore` for persistence. `BlameChangedFiles` for selective blame on changed files.
- **Store methods**: `InsertGitCommit` (with ON CONFLICT), `InsertGitDiff`, `InsertBlameLines` (batch with transaction).
- **6 tests**: CoAuthor detection (7 cases), multiple coauthors, full sync+diff integration, blame, blame+store, deleted file blame.

## Task Commits

| Task | Name | Commit | Key Files |
|------|------|--------|-----------|
| 1 | Session parser with abstraction layer | babd291 | provider.go, parser.go, tailer.go, discovery.go, parser_test.go, sqlite.go |
| 2 | Git integration with go-git | 2f2a39c | repository.go, commits.go, blame.go, gitint_test.go, go.mod, go.sum |

## Files Created

- `internal/sessionparser/provider.go` -- SessionProvider interface, SessionFile, SessionEvent types
- `internal/sessionparser/parser.go` -- ClaudeCodeParser implementing SessionProvider
- `internal/sessionparser/tailer.go` -- Polling-based file tailer with offset tracking
- `internal/sessionparser/discovery.go` -- Session file discovery and fsnotify watching
- `internal/sessionparser/parser_test.go` -- 13 tests for parser, discovery, tailer
- `internal/gitint/repository.go` -- Repository wrapper with SyncCommits
- `internal/gitint/commits.go` -- Commit parsing, Co-Authored-By detection, diff computation
- `internal/gitint/blame.go` -- Git blame with per-line authorship
- `internal/gitint/gitint_test.go` -- 6 tests for coauthor, sync, blame

## Files Modified

- `internal/store/sqlite.go` -- Added 7 methods: InsertSessionEvent, GetDaemonState, SetDaemonState, InsertGitCommit, InsertGitDiff, InsertBlameLines, BlameLine type
- `go.mod` / `go.sum` -- Added github.com/go-git/go-git/v5 and transitive dependencies

## Decisions Made

1. **Defensive JSONL parsing**: Fast-path string check for "tool_use" before JSON parsing. Unknown fields ignored. Malformed lines logged and skipped. Handles UTF-8 BOM.
2. **Polling-based tailer**: 500ms poll interval rather than fsnotify for individual files. More reliable for files being appended to by other processes.
3. **Selective blame**: Blame only changed files, not entire repo. Blame is expensive (reads full file history).
4. **Multi-line regex for Co-Authored-By**: Uses `(?im)` flags for case-insensitive multi-line matching. Handles trailers anywhere in message body.
5. **Merge commit first-parent diff**: Standard behavior; avoids double-counting changes from merged branches.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed Co-Authored-By regex multi-line matching**
- **Found during:** Task 2 test execution
- **Issue:** Go regex `$` only matches end-of-string by default, not end-of-line. Co-authored-by tags in the middle of a message (before Signed-off-by etc.) were not detected.
- **Fix:** Added `(?m)` flag to regex for multi-line mode where `$` matches end-of-line.
- **Files modified:** `internal/gitint/commits.go`
- **Commit:** 2f2a39c (included in task 2 commit)

**2. [Rule 1 - Bug] Fixed go-git Blame Line field mapping**
- **Found during:** Task 2 compilation
- **Issue:** go-git's `Line.Author` is the email, `Line.AuthorName` is the name (counter-intuitive naming).
- **Fix:** Swapped field references: `formatAuthor(line.AuthorName, line.Author)`.
- **Files modified:** `internal/gitint/blame.go`
- **Commit:** 2f2a39c (included in task 2 commit)

## Issues & Risks

- **Claude Code JSONL format instability**: The parser handles current observed format but may need updates if Anthropic changes the session file structure. The SessionProvider abstraction isolates this risk.
- **Daemon integration not wired**: The session parser and git integration packages are built and tested independently. Wiring them into the daemon's Start() loop (goroutines for discovery, tailing, periodic git sync) is a natural next step but was not part of this plan's scope -- Phase 2 will orchestrate the full pipeline.

## Next Phase Readiness

Phase 1 data pipeline is complete (all 3 input signals):
1. **File events** (01-02): fsnotify watcher captures create/modify/delete/rename
2. **Session events** (01-03): JSONL parser extracts Write/Read/Bash tool_use events
3. **Git data** (01-03): Commits, diffs, blame, Co-Authored-By detection

All three signal types have store methods and schema tables. Phase 2 (Intelligence Engine) can begin correlating these signals to determine authorship.
