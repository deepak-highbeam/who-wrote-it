# Who Wrote It

## What This Is

An always-on Go daemon that monitors how code gets written in real-time, attributing contributions not just by authorship (human vs AI) but by type of work — architecture decisions, core logic, boilerplate, bug fixes, edge case handling, test scaffolding. Generates CLI reports, GitHub PR collaboration summaries, and code survival analytics showing how AI-written code persists over time.

## Core Value

Reveal **how** AI is being used in development — not vanity line counts, but the nature of human-AI collaboration on every piece of work.

## Requirements

### Validated

- ✓ Background daemon watches code being written in real-time — v1.0
- ✓ Ingests Claude Code session data to understand AI contributions — v1.0
- ✓ Monitors file system events to correlate changes with active processes — v1.0
- ✓ Uses git history (blame, annotate, Co-Authored-By) as attribution signal — v1.0
- ✓ Classifies contributions by type of work: architecture, core logic, boilerplate, bug fixes, edge case handling, test scaffolding — v1.0
- ✓ Distinguishes the five authorship categories: fully AI, AI-first then human-revised, human-first then AI-revised, human-written but AI-suggested, fully human — v1.0
- ✓ CLI report output for local use — v1.0
- ✓ GitHub PR comment with summary stats and collaboration breakdown — v1.0
- ✓ Per-file authorship and work-type breakdown — v1.0
- ✓ AI code survival tracking across subsequent commits — v1.0
- ✓ Meaningful AI % metric weighted by work type — v1.0

### Active

- [ ] Tracks decision attribution: who initiated the approach, not just who typed
- [ ] Tracks trends over time across PRs
- [ ] Collaboration pattern flow visualization (who initiated, who refined)
- [ ] Web dashboard for local data visualization
- [ ] Multi-repo aggregated analysis
- [ ] Additional AI tool support (Copilot, Cursor, Codeium)

### Out of Scope

- Developer rankings / leaderboards — creates perverse incentives, gaming, surveillance feel
- AI code percentage as KPI — meaningless without work-type context
- Screen monitoring / keylogging — privacy violation, destroys trust
- Automated performance scoring — no tool has solved this without backlash
- PR blocking based on AI % — arbitrary thresholds punish good AI collaboration
- Predictive delegation suggestions — moves from analysis to recommendation (much harder problem)
- LLM-based classification — higher latency, API costs, non-deterministic; heuristics first
- Cloud/SaaS deployment — local-first is both a simplification and a differentiator
- Enterprise features (team dashboards, org-wide analytics) — internal team tool for now

## Context

Shipped v1.0 with 8,815 LOC Go across 40 source files.
Tech stack: Go 1.25, modernc.org/sqlite (pure Go), fsnotify, go-git, cobra CLI.
105 tests across 11 packages, all passing with race detector.
Pure Go build (no CGO) for clean cross-compilation.

Initial architecture proven: daemon -> file events + session events + git data -> correlation -> authorship classification -> work-type classification -> reports/PR comments/survival tracking.

## Constraints

- **Language**: Go — chosen for simpler concurrency, go-git pure Go library, I/O-bound daemon workload
- **AI Tool**: Claude Code integration first, SessionProvider interface designed for Copilot/Cursor extensibility
- **Privacy**: All data stays local, no telemetry — sensitive development activity data
- **Performance**: Daemon must have negligible CPU/memory footprint when idle

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Go over Rust | I/O-bound daemon, go-git pure Go, simpler concurrency | ✓ Good — 8.8K LOC in 1 day, pure Go build |
| Heuristic classifier over LLM | Lower latency, runs locally, no API costs, deterministic | ✓ Good — 6 work types, user overrides for corrections |
| Claude Code first | Team uses Claude Code; tighter integration yields better signal | ✓ Good — SessionProvider abstraction ready for expansion |
| Work-type classification over line counting | Line counts are vanity metrics; work-type reveals actual AI leverage | ✓ Good — meaningful AI % weights architecture 3x over boilerplate |
| modernc.org/sqlite (pure Go) | No CGO, clean cross-compilation, simpler build | ✓ Good — zero build issues, WAL mode works well |
| Interface-based DI | Break daemon<->IPC circular dependency cleanly | ✓ Good — clean import graph, acyclic dependencies |
| 5s correlation window | Tight time matching for file-to-session correlation | ✓ Good — graduated confidence (0.95 to 0.5) |
| Content-hash survival tracking | Compare attribution hashes against blame line hashes | ✓ Good — simple, deterministic |
| No external GitHub SDK | Standard net/http for REST API calls | ✓ Good — minimal dependencies |
| Daemon data primary, git secondary | Squash merges destroy history; daemon has richer real-time signal | ✓ Good — git fallback works for repos without daemon data |

---
*Last updated: 2026-02-09 after v1.0 milestone*
