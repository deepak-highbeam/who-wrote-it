# Roadmap: Who Wrote It

## Overview

This roadmap delivers a Go-based daemon that monitors code authorship across human-AI collaboration, classifies the nature of contributions, and surfaces actionable reports. The journey moves from building a reliable data collection pipeline (daemon + file watching + session parsing + git), through the intelligence layer (correlation, authorship spectrum, work-type classification), to the output layer (CLI reports, GitHub PR comments, code survival tracking). Three phases, each delivering a coherent capability that builds on the previous.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: Data Pipeline** - Daemon foundation with all three input signals flowing and persisted
- [ ] **Phase 2: Intelligence** - Correlation engine, authorship spectrum, work-type classification, and meaningful AI metric
- [ ] **Phase 3: Output** - CLI reports, GitHub PR integration, and code survival tracking

## Phase Details

### Phase 1: Data Pipeline
**Goal**: All three input signals (file system events, Claude Code session data, git history) flow through a stable daemon into a local SQLite store
**Depends on**: Nothing (first phase)
**Requirements**: DAEM-01, DAEM-02, DAEM-03, DAEM-04, FSWT-01, FSWT-02, FSWT-03, FSWT-04, CCSP-01, CCSP-02, CCSP-03, CCSP-04, GITI-01, GITI-02, GITI-03, GITI-04
**Success Criteria** (what must be TRUE):
  1. Daemon starts as a background process, runs continuously, and shuts down gracefully without orphan processes or resource leaks
  2. File changes in a watched project directory produce debounced, filtered events visible in the SQLite store (editor event storms do not cause duplicates or CPU spikes)
  3. Claude Code session Write events are discovered, tailed, and parsed into structured records in the store as the developer uses Claude Code
  4. Git commits, diffs, blame data, and Co-Authored-By tags are parsed and stored, including correct handling of rebases, squash merges, and renamed files
  5. CLI client can communicate with the daemon over Unix domain socket to confirm status and data collection stats
**Plans**: 3 plans

Plans:
- [ ] 01-01: Daemon lifecycle, SQLite store, and Unix socket IPC
- [ ] 01-02: File system watcher with debouncing, filtering, and recursive watching
- [ ] 01-03: Claude Code session parser and git integration

### Phase 2: Intelligence
**Goal**: Raw signals are correlated and classified into a five-level authorship spectrum with work-type labels and a meaningful AI percentage metric
**Depends on**: Phase 1
**Requirements**: CORR-01, CORR-02, CORR-03, AUTH-01, AUTH-02, AUTH-03, WTYP-01, WTYP-02, WTYP-03, METR-01, METR-02
**Success Criteria** (what must be TRUE):
  1. File system events and Claude Code session events are correlated using time-window matching, producing per-line attribution records with an authorship level and confidence score
  2. Each code contribution is classified on the five-level spectrum (fully AI, AI-first/human-revised, human-first/AI-revised, AI-suggested/human-written, fully human) using session timestamps and file event ordering
  3. Code changes are classified by work type (architecture, core logic, boilerplate, bug fix, edge case handling, test scaffolding) using heuristic rules, and users can override misclassifications
  4. A "meaningful AI %" metric is computed per-file and per-project, weighting architecture and core logic higher than boilerplate
  5. Ambiguous attribution cases are marked as "uncertain" rather than guessed
**Plans**: 2 plans

Plans:
- [ ] 02-01: Event correlation engine and five-level authorship classification
- [ ] 02-02: Work-type classification and meaningful AI metric

### Phase 3: Output
**Goal**: Users can see attribution and classification results through CLI reports, GitHub PR comments, and code survival analysis
**Depends on**: Phase 2
**Requirements**: CLIO-01, CLIO-02, CLIO-03, GHPR-01, GHPR-02, GHPR-03, SURV-01, SURV-02
**Success Criteria** (what must be TRUE):
  1. `who-wrote-it analyze` produces a full attribution report for the current repo showing per-file authorship spectrum and work-type distribution
  2. `who-wrote-it status` shows daemon status, data collection stats, and health information
  3. A collaboration summary comment is automatically posted on GitHub PR creation, showing authorship breakdown by work type and per-file collaboration patterns
  4. AI-written code survival is tracked across subsequent commits, with survival rates reported by authorship level and work type
**Plans**: 2 plans

Plans:
- [ ] 03-01: CLI commands and report generation
- [ ] 03-02: GitHub PR integration and code survival tracking

## Progress

**Execution Order:**
Phases execute in numeric order: 1 --> 2 --> 3

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Data Pipeline | 0/3 | Not started | - |
| 2. Intelligence | 0/2 | Not started | - |
| 3. Output | 0/2 | Not started | - |
