# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-09)

**Core value:** Reveal how AI is being used in development -- not vanity line counts, but the nature of human-AI collaboration on every piece of work.
**Current focus:** All phases complete. v1 milestone fully delivered including gap closure.

## Current Position

Phase: 4 of 4 (Wire Attribution Pipeline) -- COMPLETE
Plan: 1 of 1 in current phase -- COMPLETE
Status: All phases complete. v1 milestone delivered.
Last activity: 2026-02-10 -- Completed 04-01-PLAN.md (Wire Attribution Pipeline)

Progress: [██████████] 100%

## Performance Metrics

**Velocity:**
- Total plans completed: 9
- Average duration: 5 min
- Total execution time: 0.7 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 1. Data Pipeline | 4/4 | 19 min | 5 min |
| 2. Intelligence | 2/2 | 10 min | 5 min |
| 3. Output | 2/2 | 8 min | 4 min |
| 4. Wire Attribution Pipeline | 1/1 | 3 min | 3 min |

**Recent Trend:**
- Last 5 plans: 02-02 (5 min), 03-01 (3 min), 03-02 (5 min), 04-01 (3 min)
- Trend: Consistent, fast

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
- Direct DB reads for analyze (no daemon dependency for offline analysis)
- ANSI codes inline (no external color library)
- Top 20 files shown in project report (truncated with count)
- Content-hash comparison for survival tracking (blame line hash vs session event hash)
- Files without blame data skipped in survival analysis (not penalized)
- Notable files require >= 3 events to appear in PR comment
- No external GitHub SDK (standard net/http for REST API)
- SurvivalReport type duplicated in github package to avoid circular imports
- 2s polling interval for attribution processor (balances latency with CPU for burst event patterns)
- Batch size 100 per attribution tick (bounds per-tick processing time)
- Empty diff/commit for work-type classification in daemon (path heuristics still functional)

### Pending Todos

None.

### Blockers/Concerns

- Claude Code JSONL format has no stability contract -- SessionProvider abstraction layer built to isolate this risk
- Background daemonization not yet implemented (foreground only) -- fine for development

## Session Continuity

Last session: 2026-02-10
Stopped at: Completed 04-01-PLAN.md. All phases complete. v1 milestone delivered.
Resume file: None
