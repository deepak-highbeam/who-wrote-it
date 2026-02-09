# Who Wrote It

## What This Is

An always-on development intelligence tool that monitors how code gets written and attributes contributions not just by authorship (human vs AI) but by type of work — architecture decisions, core logic, boilerplate, bug identification, edge case handling. When a PR is created, it generates a report showing the collaboration pattern: who drove what. Built in Go or Rust for maximum performance.

## Core Value

Reveal **how** AI is being used in development — not vanity line counts, but the nature of human-AI collaboration on every piece of work.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Background daemon watches code being written in real-time
- [ ] Ingests Claude Code session data to understand AI contributions
- [ ] Monitors file system events to correlate changes with active processes
- [ ] Uses git history (blame, annotate, Co-Authored-By) as attribution signal
- [ ] Classifies contributions by type of work: architecture decisions, core logic, boilerplate, bug fixes, edge case handling, test scaffolding
- [ ] Tracks decision attribution: who initiated the approach, not just who typed
- [ ] Distinguishes the five authorship categories: fully AI, AI-first then human-revised, human-first then AI-revised, human-written but AI-suggested, fully human
- [ ] Generates PR report on PR creation with collaboration pattern breakdown
- [ ] CLI report output for local use
- [ ] GitHub PR comment with summary stats and collaboration breakdown
- [ ] Tracks trends over time across PRs
- [ ] Per-file authorship and work-type breakdown

### Out of Scope

- Multi-AI tool support in v1 — Claude Code only first, extensible architecture for later
- Enterprise features (team dashboards, org-wide analytics) — internal team tool for now
- Keystroke logging / keylogger approach — use session data and file events instead
- LLM-based classification — use heuristic/deterministic classifier first

## Context

- Originated from internal team discussion about measuring AI contribution to codebases
- The core insight: raw "% AI-written" is misleading — import statements and boilerplate inflate numbers without reflecting real AI leverage
- What matters: understanding the collaboration pattern — did AI drive architecture or just fill in implementation? Did the human identify the bug or did AI?
- Claude Code provides session data that can be parsed to understand what was generated vs what was human-authored
- git-annotate (https://git-scm.com/docs/git-annotate) is a foundation for line-level attribution
- Target users: the team itself, developers wanting insight into their AI usage patterns

## Constraints

- **Language**: Go or Rust — performance-critical daemon that runs continuously
- **AI Tool**: Claude Code integration first, design for extensibility to Copilot/Cursor later
- **Privacy**: All data stays local, no telemetry — this is sensitive development activity data
- **Performance**: Daemon must have negligible CPU/memory footprint when idle

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Go or Rust (TBD) | Both offer performance; Go has simpler concurrency, Rust has stronger guarantees | -- Pending |
| Heuristic classifier over LLM | Lower latency, runs locally, no API costs, deterministic | -- Pending |
| Claude Code first | Team uses Claude Code; tighter integration yields better signal | -- Pending |
| Work-type classification over line counting | Line counts are vanity metrics; work-type reveals actual AI leverage | -- Pending |

---
*Last updated: 2026-02-09 after initialization*
