# Requirements: Who Wrote It

**Defined:** 2026-02-09
**Core Value:** Reveal how AI is being used in development — not vanity line counts, but the nature of human-AI collaboration on every piece of work.

## v1 Requirements

### Daemon Foundation

- [ ] **DAEM-01**: Daemon starts as background process and runs continuously with negligible idle resource usage
- [ ] **DAEM-02**: Daemon communicates with CLI client over Unix domain socket
- [ ] **DAEM-03**: Daemon handles graceful shutdown and crash recovery
- [ ] **DAEM-04**: Local SQLite database stores all attribution data (WAL mode)

### File System Watching

- [ ] **FSWT-01**: Daemon watches project directory for file creation, modification, and deletion events
- [ ] **FSWT-02**: Daemon debounces editor event storms (multiple events per save)
- [ ] **FSWT-03**: Daemon recursively watches new subdirectories as they are created
- [ ] **FSWT-04**: Daemon ignores configured paths (.git, node_modules, build artifacts)

### Claude Code Session Parsing

- [ ] **CCSP-01**: Daemon discovers and tails Claude Code JSONL session files
- [ ] **CCSP-02**: Daemon extracts Write tool_use events with file paths and content
- [ ] **CCSP-03**: Daemon handles new session file creation (session rotation)
- [ ] **CCSP-04**: Parser sits behind abstraction layer for future AI tool extensibility

### Git Integration

- [ ] **GITI-01**: Parse git commits, diffs, and blame for line-level authorship
- [ ] **GITI-02**: Handle rebases, squash merges, and renamed files correctly
- [ ] **GITI-03**: Detect Co-Authored-By tags as AI attribution signal
- [ ] **GITI-04**: Use git as secondary attribution source (daemon data is primary)

### Event Correlation

- [ ] **CORR-01**: Match file system events to Claude Code session events using time-window correlation
- [ ] **CORR-02**: Produce per-line attribution records with authorship level
- [ ] **CORR-03**: Handle ambiguous cases (mark as "uncertain" rather than guess)

### Authorship Classification

- [ ] **AUTH-01**: Classify each code contribution on 5-level spectrum: fully AI, AI-first/human-revised, human-first/AI-revised, AI-suggested/human-written, fully human
- [ ] **AUTH-02**: Use session timestamps + file events to determine who went first
- [ ] **AUTH-03**: Aggregate line-level attribution to file and project levels

### Work-Type Classification

- [ ] **WTYP-01**: Classify code changes as: architecture, core logic, boilerplate, bug fix, edge case handling, test scaffolding
- [ ] **WTYP-02**: Use heuristic rules (file paths, AST patterns, change patterns) not LLM
- [ ] **WTYP-03**: Allow user override of misclassified work types

### Meaningful AI Metric

- [ ] **METR-01**: Compute "meaningful AI %" weighting by work type (architecture + core logic weighted higher than boilerplate)
- [ ] **METR-02**: Compute per-file and per-project meaningful AI percentages

### Code Survival

- [ ] **SURV-01**: Track whether AI-written code persists across subsequent commits
- [ ] **SURV-02**: Report code survival rate by authorship level and work type

### CLI Output

- [ ] **CLIO-01**: `who-wrote-it analyze` produces attribution report for current repo
- [ ] **CLIO-02**: `who-wrote-it status` shows daemon status and data collection stats
- [ ] **CLIO-03**: Per-file breakdown showing authorship spectrum and work-type distribution

### GitHub PR Integration

- [ ] **GHPR-01**: Auto-post collaboration summary comment on PR creation
- [ ] **GHPR-02**: PR comment shows authorship breakdown by work type (not just line counts)
- [ ] **GHPR-03**: PR comment shows which files had which collaboration pattern

## v2 Requirements

### Trend Analytics

- **TRND-01**: Track AI usage patterns over time across PRs
- **TRND-02**: Show collaboration pattern changes over weeks/months

### Visualization

- **VIZL-01**: Collaboration pattern flow visualization (who initiated, who refined)
- **VIZL-02**: Web dashboard for local data visualization

### Expansion

- **EXPN-01**: Multi-repo aggregated analysis
- **EXPN-02**: Additional AI tool support (Copilot, Cursor, Codeium)
- **EXPN-03**: AI delegation pattern insights (what you hand off vs keep)

### Team Analytics

- **TEAM-01**: Team-level aggregate patterns (not individual rankings)
- **TEAM-02**: CI/CD pipeline integration

## Out of Scope

| Feature | Reason |
|---------|--------|
| Developer rankings / leaderboards | Creates perverse incentives, gaming, surveillance feel |
| AI code percentage as KPI | Meaningless without work-type context; incentivizes inflating AI usage |
| Screen monitoring / keylogging | Privacy violation, destroys trust, developers revolt |
| Automated performance scoring | No tool has solved this without backlash |
| PR blocking based on AI % | Arbitrary thresholds punish good AI collaboration |
| Predictive delegation suggestions | Moves from analysis to recommendation — much harder problem |
| LLM-based classification | Higher latency, API costs, non-deterministic; heuristics first |
| Cloud/SaaS deployment | Local-first is both a simplification and a differentiator |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| DAEM-01 | Phase 1 | Pending |
| DAEM-02 | Phase 1 | Pending |
| DAEM-03 | Phase 1 | Pending |
| DAEM-04 | Phase 1 | Pending |
| FSWT-01 | Phase 1 | Pending |
| FSWT-02 | Phase 1 | Pending |
| FSWT-03 | Phase 1 | Pending |
| FSWT-04 | Phase 1 | Pending |
| CCSP-01 | Phase 1 | Pending |
| CCSP-02 | Phase 1 | Pending |
| CCSP-03 | Phase 1 | Pending |
| CCSP-04 | Phase 1 | Pending |
| GITI-01 | Phase 1 | Pending |
| GITI-02 | Phase 1 | Pending |
| GITI-03 | Phase 1 | Pending |
| GITI-04 | Phase 1 | Pending |
| CORR-01 | Phase 2 | Pending |
| CORR-02 | Phase 2 | Pending |
| CORR-03 | Phase 2 | Pending |
| AUTH-01 | Phase 2 | Pending |
| AUTH-02 | Phase 2 | Pending |
| AUTH-03 | Phase 2 | Pending |
| WTYP-01 | Phase 2 | Pending |
| WTYP-02 | Phase 2 | Pending |
| WTYP-03 | Phase 2 | Pending |
| METR-01 | Phase 2 | Pending |
| METR-02 | Phase 2 | Pending |
| CLIO-01 | Phase 3 | Pending |
| CLIO-02 | Phase 3 | Pending |
| CLIO-03 | Phase 3 | Pending |
| GHPR-01 | Phase 3 | Pending |
| GHPR-02 | Phase 3 | Pending |
| GHPR-03 | Phase 3 | Pending |
| SURV-01 | Phase 3 | Pending |
| SURV-02 | Phase 3 | Pending |

**Coverage:**
- v1 requirements: 35 total
- Mapped to phases: 35
- Unmapped: 0

---
*Requirements defined: 2026-02-09*
*Last updated: 2026-02-09 after roadmap creation*
