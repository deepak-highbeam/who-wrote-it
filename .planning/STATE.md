# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-09)

**Core value:** Reveal how AI is being used in development -- not vanity line counts, but the nature of human-AI collaboration on every piece of work.
**Current focus:** Phase 1: Data Pipeline (complete, all gaps closed)

## Current Position

Phase: 1 of 3 (Data Pipeline)
Plan: 4 of 4 in current phase (3 core + 1 gap closure)
Status: Phase complete (all subsystems wired)
Last activity: 2026-02-09 -- Completed 01-04-PLAN.md (Gap closure: wire session parser and git integration into daemon)

Progress: [████░░░░░░] 57%

## Performance Metrics

**Velocity:**
- Total plans completed: 4
- Average duration: 5 min
- Total execution time: 0.35 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 1. Data Pipeline | 4/4 | 19 min | 5 min |

**Recent Trend:**
- Last 5 plans: 01-01 (6 min), 01-02 (4 min), 01-03 (7 min), 01-04 (2 min)
- Trend: Consistent, gap closure fast

*Updated after each plan completion*

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- Go chosen over Rust (research recommendation: I/O-bound daemon, go-git pure Go, simpler concurrency)
- Heuristic classifier over LLM (lower latency, local, deterministic)
- Claude Code only in v1 (team uses it, richest session data, extensible later)
- Daemon-captured data is primary attribution source, git is secondary (squash merges destroy history)
- modernc.org/sqlite chosen over mattn/go-sqlite3 (pure Go, no CGO, clean cross-compilation)
- Interface-based DI pattern for daemon<->IPC circular dependency resolution
- Socket file in data dir (~/.whowroteit/) not /tmp for security
- Go 1.25.7 installed (latest stable)
- 100ms debounce window for file events (balances responsiveness with burst collapse)
- Component-based path matching for ignore filter (catches nested dirs at any depth)
- Watcher shutdown before IPC/store (ensures debounced events flush before DB closes)
- Defensive JSONL parsing with fast-path skip for non-tool_use lines
- Polling-based tailer (500ms) over fsnotify for individual file append monitoring
- Selective blame (changed files only) to keep git sync fast
- Merge commits diff against first parent only (standard git behavior)
- go-git/go-git/v5 pure Go git library (no CGO, consistent with project approach)
- Per-subsystem context.WithCancel for independent goroutine lifecycle management
- Non-fatal subsystem errors (session/git failures logged, daemon stays alive)

### Pending Todos

None.

### Blockers/Concerns

- Claude Code JSONL format has no stability contract -- SessionProvider abstraction layer built to isolate this risk
- Background daemonization not yet implemented (foreground only) -- fine for development

## Session Continuity

Last session: 2026-02-09T19:25:43Z
Stopped at: Completed 01-04-PLAN.md (Gap closure: wire session parser and git integration). Phase 1 fully complete with all subsystems wired.
Resume file: None
