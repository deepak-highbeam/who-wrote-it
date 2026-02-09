# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-09)

**Core value:** Reveal how AI is being used in development -- not vanity line counts, but the nature of human-AI collaboration on every piece of work.
**Current focus:** Phase 2: Intelligence complete. Work-type classifier and meaningful AI metrics built. Ready for Phase 3: Interface.

## Current Position

Phase: 2 of 3 (Intelligence)
Plan: 2 of 2 in current phase
Status: Phase complete
Last activity: 2026-02-09 -- Completed 02-02-PLAN.md

Progress: [██████░░░░] 66%

## Performance Metrics

**Velocity:**
- Total plans completed: 6
- Average duration: 5 min
- Total execution time: 0.50 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 1. Data Pipeline | 4/4 | 19 min | 5 min |
| 2. Intelligence | 2/2 | 10 min | 5 min |

**Recent Trend:**
- Last 5 plans: 01-03 (7 min), 01-04 (2 min), 02-01 (5 min), 02-02 (5 min)
- Trend: Consistent

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
- 5s default correlation window, configurable via Correlator.WindowMs
- Exact file match preferred over time proximity in correlation
- CorrelationResult lives in authorship package to avoid circular imports
- Graduated confidence: 0.95 (exact <2s), 0.7 (exact 2-5s), 0.5 (proximity), 0.6 (git coauthor), 0.8 (git no coauthor)
- Three weight tiers: high (3.0) for architecture/core_logic, medium (2.0) for bug_fix/edge_case, low (1.0) for boilerplate/test_scaffolding
- Edge case threshold = 3 keyword occurrences (avoid false positives from single error checks)
- CoreLogic is default fallback work type
- Bug fix detected from commit message keywords (commit-level signal, not file content)
- AI events = fully_ai + ai_first_human_revised only (human_first_ai_revised not counted as AI)

### Pending Todos

None.

### Blockers/Concerns

- Claude Code JSONL format has no stability contract -- SessionProvider abstraction layer built to isolate this risk
- Background daemonization not yet implemented (foreground only) -- fine for development

## Session Continuity

Last session: 2026-02-09T20:38:00Z
Stopped at: Completed 02-02-PLAN.md. Work-type classifier (6 categories) and meaningful AI metrics calculator built with 28 tests. Phase 2 complete. Ready for Phase 3.
Resume file: None
