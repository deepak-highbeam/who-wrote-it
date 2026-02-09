# Project Research Summary

**Project:** Who Wrote It - Code Authorship Attribution & AI Collaboration Intelligence Daemon
**Domain:** Developer Intelligence / AI Collaboration Analytics
**Researched:** 2026-02-09
**Confidence:** HIGH

## Executive Summary

"Who Wrote It" is an always-on developer tool daemon that tracks and attributes code authorship across human-AI collaboration workflows. The research reveals a significant market gap: existing tools (GitHub Copilot Metrics, Claude Code Analytics, GitClear, CodeScene, LinearB) provide either binary "AI or not" attribution or process-focused velocity metrics, but none classify the *nature* of human-AI collaboration or the *type of work* produced. This tool occupies the white space by answering "What kind of work did the AI do vs. the human?"

The recommended approach is to build a Go-based daemon using event-driven architecture with three input signals: file system events (fsnotify), Claude Code session data (JSONL parsing), and git repository analysis (go-git). The core value proposition is a five-level authorship spectrum (completely AI, AI-then-human, human-then-AI, AI-suggested, completely human) combined with work-type classification (architecture, core logic, boilerplate, bug fixes, tests, edge cases). This produces the signature metric: "meaningful AI %" that filters out boilerplate inflation and focuses on substantive contributions.

The critical risks are: (1) file watcher event storms causing CPU spikes without proper debouncing, (2) git squash merges destroying authorship history if relying solely on commits, (3) Claude Code's JSONL format having no stability contract and changing with updates, (4) work-type classification being inherently fuzzy and undermining trust if over-engineered, (5) resource leaks in long-running daemon operation, and (6) race conditions between file watching and git operations. All are mitigated through architectural decisions (event-driven with debouncing, daemon-captured data as primary source, abstraction layers for external formats, coarse-grained classification, health monitoring, git operation detection).

## Key Findings

### Recommended Stack

**Go wins over Rust for this specific project.** While Rust offers superior raw performance and smaller memory footprint, Go's ecosystem, developer velocity, and I/O-optimized concurrency model are better suited for an I/O-bound daemon. The deciding factors: go-git is pure Go with no C dependencies (Rust's git2 wraps libgit2 requiring CGO), Google-maintained go-github client (vs. pre-1.0 octocrab in Rust), trivial cross-compilation, and Go 1.26's Green Tea GC reducing daemon overhead by 10-40%. This is a file-watching, API-calling, JSON-parsing tool—not a CPU-intensive one—so Go's goroutine model for concurrent I/O is the natural fit.

**Core technologies:**
- **Go 1.26** (language runtime) — Green Tea GC for lower daemon overhead, goroutine leak profiles, falls back to 1.24/1.25 if needed
- **fsnotify v1.9.0** (file system watching) — Cross-platform OS-native events (inotify/kqueue/FSEvents), used by Docker/Kubernetes, zero CPU when idle
- **go-git v5.16.5** (Git operations) — Pure Go implementation, no CGO, supports blame/log/diff, used by Gitea/Pulumi
- **go-github v68+** (GitHub API) — Google-maintained, tracks GitHub API closely, semver releases
- **modernc.org/sqlite** (local data store) — Pure Go SQLite, no CGO, WAL mode for concurrent reads, single-binary deployment
- **cobra v1.10.2** (CLI framework) — Industry standard, used by kubectl/docker/hugo

**Critical version note:** Go 1.26 releases Feb 2026 with Green Tea GC. If starting before release, use Go 1.24 (stable, supported through May 2026).

### Expected Features

**The core insight:** Binary "AI or not" attribution is insufficient. A file that is "40% AI-written" could be 40% boilerplate (low value) or 40% core architecture (high value). The meaningful metric requires both authorship and work-type classification.

**Must have (table stakes):**
- Git integration (commit/diff parsing, handling rebases/squash merges/renames correctly)
- Per-file and per-line authorship tracking (baseline expectation from git blame)
- Human vs. AI binary classification (Copilot Metrics and Claude Code Analytics already provide this)
- Session-level data capture (Claude Code JSONL parsing)
- Dashboard/visualization (users expect to see data, not just query it)
- Time-range filtering (standard in all analytics tools)
- Privacy-respecting design (all data local, no surveillance, developer-facing not manager-facing)
- Accurate attribution not inflated (Claude Code and GitClear set the bar with conservative metrics)

**Should have (competitive advantage - these are differentiators):**
- **Five-level authorship spectrum** — Captures real collaboration: completely AI, AI-first/human-rewrote, human-first/AI-rewrote, AI-suggested, completely human. No competitor does this. Requires correlating session timestamps with file events to infer "who went first."
- **Work-type classification** — Categorize as architecture, core logic, boilerplate, bug fixes, tests, edge cases. GitClear classifies operations (move/copy/refactor) but not work type. CodeScene classifies health but not type.
- **"Meaningful AI %" metric** — The headline metric: % of meaningful work (architecture + core logic + bug identification) that was AI-driven vs. human-driven, excluding boilerplate inflation.
- **Collaboration pattern visualization** — Show the flow: "Human identified bug → AI proposed fix → Human refined edge cases." The narrative is more valuable than raw numbers.
- **Real-time monitoring** — Capture signals during development (file system events, session activity) rather than retrospective git analysis only.
- **AI delegation pattern insights** — Per Anthropic 2026 report: devs use AI in 60% of work but fully delegate only 0-20%. Show what they hand off vs. co-author vs. keep.
- **Code survival tracking** — Track whether AI-generated code survives or gets rewritten (GitClear research shows AI code leads to 4x more cloning and declining refactoring).

**Defer (v2+):**
- Additional AI tool support (Copilot, Cursor, Codeium) — start with Claude Code (richest session data), add others based on demand
- Team/org analytics — individual developer use case must work first
- CI/CD integration — core analysis must be reliable first
- PR-level attribution annotations — requires GitHub/GitLab API integration
- Historical analysis mode (analyze existing git history without session data)
- Contribution quality weighting (not all lines equal—architecture decisions vs. implementations)

### Architecture Approach

Always-on daemon with event-driven pipeline: file system watcher + Claude Code session tailer feed into event router (fan-in with Go channels), which dispatches to correlation engine (time-window matching), classification engine (work-type heuristics), and attribution store (SQLite in WAL mode). Separate binaries for daemon (wwid) and CLI client (wwi) communicating via Unix domain socket. All subsystems run in errgroup goroutines with context-based lifecycle management.

**Major components:**
1. **File Watcher** — fsnotify with debouncing (500ms-2s), gitignore-aware filtering, recursive directory walking. Monitors working directory for create/modify/delete events.
2. **Session Tailer** — Tail-reads Claude Code JSONL files (`~/.claude/projects/`), extracts tool_use Write events and conversation context, maintains byte offset cursor for incremental reads.
3. **Event Router** — Fan-in from multiple input channels, deduplicates and debounces, dispatches to correlation engine. Central nervous system using Go channel select.
4. **Correlation Engine** — Sliding time window (5-10 second TTL) of recent session events. Matches file changes to session Write events to determine if AI-originated or human-originated.
5. **Git Analyzer** — On-demand blame/log/diff operations via go-git. Handles Co-Authored-By extraction. Never writes to repo, read-only analysis.
6. **Classification Engine** — Heuristic-based work-type classification: file path patterns first (60-70% coverage), then change complexity, session conversation context. Not LLM-based.
7. **Attribution Store** — SQLite with WAL mode. Schema: files, sessions, attributions (line ranges + author_type + work_type + confidence), snapshots. Indexed for fast PR queries.
8. **Report Generator** — Aggregates store data, formats as CLI table/GitHub Markdown/JSON. Triggered by CLI command or git hook.

**Data flow:** Developer saves file → FSEvents fires → Watcher debounces → Router receives → Correlator checks session window for matching Write → Classifier analyzes work type → Store persists → Report generator queries on demand.

**Project structure:** cmd/wwid (daemon), cmd/wwi (CLI client), internal/ packages (daemon, watcher, session, router, correlator, git, classifier, store, report), pkg/types (shared definitions).

### Critical Pitfalls

1. **File watcher event storms causing CPU spikes** — Editors produce 3-5 filesystem events per save (temp file, rename, chmod). Without debouncing: CPU spikes, redundant git operations, race conditions with partially-written files. **Avoid:** Debounce with 100-200ms windows, deduplicate by path, filter noise (.git, node_modules, build artifacts), process only final state per window. **Address in Phase 1.**

2. **Git squash merges destroying authorship history** — Squash merge replaces 15 commits from 3 contributors with one commit by the merger. Relying on git commit metadata produces completely wrong attribution. **Avoid:** Never rely solely on git history. Capture authorship at write-time via file watcher and store independently. Use git as confirmation/reconciliation, not source of truth. Daemon-captured data is primary. **Address in Phase 1 data model design.**

3. **Claude Code JSONL format has no stability contract** — Session logs are internal diagnostic data with no published schema or versioning. Anthropic ships monthly updates; any release could change structure. Community parsers break when format changes. **Avoid:** Build abstraction layer, never leak JSONL field names into codebase. Defensive parsing (all fields optional, warnings not errors). Version detection heuristics. Store raw JSONL alongside parsed data for re-parsing. Monitor Claude Code changelog. **Address in Phase 2 with abstraction layer designed in Phase 1.**

4. **Work-type classification is fundamentally fuzzy** — Distinguishing "boilerplate" from "core logic" is subjective. Is a React component following a pattern "boilerplate" or "core logic"? False precision ("87% core logic") undermines trust. **Avoid:** Use coarse-grained categories (test code, configuration, generated, application code). Classify by file path first (unambiguous, covers 60-70%). Present with explicit uncertainty. Let users override. Start with attribution (who wrote it) before tackling complexity (what kind of work). **Address in Phase 3+, do not block MVP.**

5. **Daemon resource leaks in long-running operation** — File handles from watchers never torn down, unbounded in-memory buffers, unclosed SQLite connections, growing caches. After 3-7 days: 500MB+ RAM or hit file descriptor limit (256 macOS, 1024 Linux default). **Avoid:** Health checks logging memory/handles/watchers every 5 minutes. Hard memory limits with graceful restart. Connection pooling (single SQLite connection in WAL mode). Watcher lifecycle management. Soak tests (8+ hours simulated activity). Check inotify limits on Linux (`max_user_watches`). **Address in Phase 1 architecture.**

6. **Race condition between file watcher and git operations** — `git checkout` changes 50+ files simultaneously. File watcher sees same events as if user edited 50 files. Without git operation detection, daemon misattributes git-operational changes as authored changes. **Avoid:** Monitor `.git/HEAD`, `.git/index`, `.git/refs/` for changes. Pause attribution during git operations (2-5 second window after .git/ changes). Bulk file events (>10 files in 500ms) likely indicate git operation. Use git hooks (post-checkout, post-merge) as explicit signals. **Address in Phase 2, but Phase 1 watcher must support pausing.**

## Implications for Roadmap

Based on research, suggested phase structure:

### Phase 1: Daemon Foundation + File Watching
**Rationale:** The daemon lifecycle and file watcher are the bedrock. All downstream features depend on reliable event capture. Solving event storms, debouncing, resource leaks, and git operation detection here prevents compounding problems later. The data model must be designed from day one to treat daemon-captured data as primary (not git), which fundamentally shapes the architecture.

**Delivers:**
- Daemon process with graceful start/stop/restart
- File system watcher with debouncing, filtering, recursive watching
- SQLite store with schema for attributions (file, line range, timestamp, author_type, work_type, confidence)
- Event router (fan-in from watcher to store)
- CLI-to-daemon communication via Unix domain socket
- Health monitoring (memory, handles, watcher count)

**Addresses:**
- Pitfall 1 (event storms) via debouncing architecture
- Pitfall 5 (resource leaks) via health monitoring and lifecycle management
- Pitfall 6 (git operation races) via git operation detection foundation
- Feature: Privacy-first local-only design (table stakes)

**Avoids:** Building any attribution logic before the data pipeline is proven stable. Get events flowing reliably first.

### Phase 2: Session Data Integration + Git Analysis
**Rationale:** With event capture stable, add the second input signal (Claude Code sessions) and the reconciliation layer (git operations). These three signals (file events, session data, git analysis) together enable authorship attribution. Session parsing abstraction layer must be built now, before formats change. Git integration must handle squash merges, rebases, renames correctly.

**Delivers:**
- Claude Code JSONL parser with abstraction layer and version detection
- Session tailer with byte offset cursor and incremental reading
- Git analyzer (blame, log, diff, Co-Authored-By extraction) via go-git
- Integration into event router (session events flow to correlator)
- Git operation lock (pause attribution during checkout/merge/rebase)

**Uses:**
- go-git v5.16.5 for pure Go git operations
- fsnotify integration with git detection
- SQLite sessions table

**Implements:**
- Session Tailer component
- Git Analyzer component
- Session event → router integration

**Addresses:**
- Pitfall 2 (squash merge destruction) via daemon-as-primary-source architecture
- Pitfall 3 (JSONL format instability) via abstraction layer and defensive parsing
- Pitfall 6 (git operation races) completion
- Feature: Session-level data capture (table stakes)
- Feature: Git integration (table stakes)

**Avoids:** Attempting any classification or advanced metrics before raw attribution signals are captured.

### Phase 3: Correlation + Basic Attribution
**Rationale:** With all input signals flowing, implement the core correlation logic: time-window matching between file events and session events to determine authorship category. Start with basic three-level (human, AI, uncertain) before implementing the full five-level spectrum. Prove the correlation algorithm works before adding complexity.

**Delivers:**
- Correlation engine with sliding time window (5s default, configurable)
- Basic attribution classification (human-only, AI-assisted, uncertain)
- Attribution persistence to SQLite with confidence scores
- CLI command to query attributions for a file or directory
- Time-range filtering

**Addresses:**
- Feature: Human vs. AI binary classification (table stakes)
- Feature: Per-file and per-line authorship tracking (table stakes)
- Feature: Time-range filtering (table stakes)

**Avoids:** Overcomplicating with five-level spectrum before proving three-level works. Five-level requires more sophisticated session analysis (who went first).

### Phase 4: Five-Level Authorship Spectrum
**Rationale:** Enhance the attribution engine from binary/uncertain to the differentiated spectrum that is the product's key differentiator. This requires richer session context analysis (conversation prompts, edit sequences, tool_use timing) to infer collaboration direction.

**Delivers:**
- Five-level classification: completely AI, AI-then-human, human-then-AI, AI-suggested, completely human
- Session conversation context extraction (what was the user prompt about?)
- Edit sequence detection (who made the first change, who refined)
- Enhanced attribution confidence scoring

**Addresses:**
- Feature: Five-level authorship spectrum (primary differentiator)
- Feature: Collaboration pattern foundation (enables visualization later)

**Avoids:** Building visualization before the underlying data model is validated with real usage.

### Phase 5: Work-Type Classification (Heuristic)
**Rationale:** Layer work-type classification on top of proven attribution. Start with simple, defensible heuristics (file path patterns, new vs. existing file, change size) rather than complex AST analysis. This gets to the "meaningful AI %" metric without over-engineering.

**Delivers:**
- Heuristic work-type classifier: test code, configuration, architecture (new structure), core logic (existing file edits), boilerplate (imports, scaffolding)
- File path pattern rules (e.g., `__tests__/` = test, `*.config.*` = config)
- Change complexity heuristics (new file + imports + dependencies = architecture; small edit in existing = bug fix or refinement)
- "Meaningful AI %" metric calculation (architecture + core logic AI % vs. total)

**Addresses:**
- Feature: Work-type classification (differentiator)
- Feature: "Meaningful AI %" metric (signature metric)
- Pitfall 4 (classification fuzziness) via coarse categories and explicit uncertainty

**Avoids:** AST parsing, LLM-based classification, contribution quality weighting—all deferred to v2+. Get the simple version working first.

### Phase 6: CLI + Report Generation
**Rationale:** With attribution and classification working, build the output layer. CLI for developer self-service, report generation for PR reviews, basic visualization in terminal.

**Delivers:**
- CLI commands: status, report (file/directory/PR), analyze (time range)
- Terminal-based report formatting (tables with work-type breakdown)
- GitHub Markdown formatter for PR comments
- JSON output for programmatic access
- PR report workflow (trigger on branch, aggregate since divergence from main)

**Addresses:**
- Feature: Dashboard/visualization (table stakes—start with CLI tables)
- Feature: Accurate attribution (conservative metrics following Claude Code/GitClear approach)

**Avoids:** Web dashboard in MVP. Terminal output is sufficient for validation. Web UI is Phase 7+.

### Phase 7: Real-Time Monitoring + Code Survival
**Rationale:** Once core functionality is validated with users, add depth features that require historical data and extended observation.

**Delivers:**
- Always-on real-time monitoring dashboard (show active sessions, recent attributions)
- Code survival tracking (track AI-written lines over time, detect rewrites)
- AI delegation pattern insights (what user typically delegates vs. keeps)
- Trend analysis over weeks/months

**Addresses:**
- Feature: Real-time monitoring (differentiator)
- Feature: Code survival tracking (differentiator)
- Feature: AI delegation pattern insights (differentiator)

**Avoids:** Shipping this before enough data accumulates to make patterns meaningful. Requires weeks of usage data.

### Phase Ordering Rationale

- **Daemon first (Phase 1)** because everything runs on top of it. Resource leaks, event storms, and lifecycle management must be solved before building features, or features inherit the problems.
- **Session + Git integration second (Phase 2)** because these are the input signals. Correlation (Phase 3) cannot work without both flowing reliably.
- **Basic attribution before advanced (Phase 3 → 4)** to validate the core algorithm with simple three-level before adding five-level complexity.
- **Classification after attribution (Phase 5)** because you must know *who* wrote code before you can classify *what kind* of code. These are orthogonal but attribution is more foundational.
- **Output layer after data layer (Phase 6)** because reports need proven attribution and classification data. Building reports first would create mocks that get rewritten.
- **Advanced features last (Phase 7)** because they require historical data accumulation and user validation of core functionality.

**Dependency chain:** Daemon → Signals (File + Session + Git) → Correlation → Classification → Reports → Advanced

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 2 (Session Integration):** Claude Code JSONL format is reverse-engineered, not documented. May need to examine multiple session file versions, test with different Claude Code releases, and build fallback strategies. Research: JSONL schema evolution, OpenTelemetry format (if available), community parser analysis.
- **Phase 4 (Five-Level Spectrum):** Inferring collaboration direction ("who went first") from session data and file events is novel. May need to prototype correlation algorithms and validate with real coding sessions before committing to approach. Research: edit sequence detection patterns, session conversation context extraction.
- **Phase 5 (Work-Type Classification):** Heuristic taxonomy needs validation. What constitutes "architecture" vs. "core logic" may vary by language, framework, project style. Research: code classification literature, review GitClear operation types and CodeScene health metrics for inspiration.

Phases with standard patterns (skip deeper research):
- **Phase 1 (Daemon Foundation):** Well-documented Go patterns. fsnotify, errgroup, context trees, Unix sockets—all standard.
- **Phase 2 (Git Integration):** go-git has extensive examples. Blame/log/diff are well-understood operations.
- **Phase 3 (Basic Attribution):** Time-window correlation is straightforward pattern.
- **Phase 6 (CLI/Reports):** cobra, report formatting, GitHub API—all mature ecosystems.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Go ecosystem, library versions, and tooling are verified via official documentation (go.dev, GitHub releases, pkg.go.dev). The Go vs. Rust decision is well-supported by 2026 comparisons and domain fit analysis. |
| Features | MEDIUM | Competitive landscape is well-mapped (official vendor docs for Copilot Metrics, Claude Code Analytics, GitClear, CodeScene, LinearB). The identified gap (work-type + collaboration pattern) is validated by absence in competitor feature sets. Lower confidence on market demand for five-level spectrum since it's novel and unvalidated by existing products. |
| Architecture | HIGH | Component patterns (fan-in channels, time-window correlation, daemon lifecycle, socket IPC) are established Go idioms with extensive documentation. The event-driven architecture for file watching is proven in production tools (Docker, Kubernetes use fsnotify). SQLite WAL mode for concurrent access is documented and battle-tested. |
| Pitfalls | MEDIUM-HIGH | Event storm, squash merge, format instability, resource leaks, git operation races are all well-documented across multiple sources (chokidar issues, GitHub discussions, daemon best practices). Claude Code JSONL format stability is LOW confidence (reverse-engineered, no official schema) but the risk itself is HIGH confidence. Work-type classification fuzziness is well-established in software engineering research. |

**Overall confidence:** HIGH

The stack, architecture, and major pitfalls are well-researched with authoritative sources. The core technical approach is sound. The uncertainty is in product validation: does the market want five-level authorship spectrum and work-type classification, or is binary "AI or not" sufficient? The architecture supports iterating from simple (three-level, path-based classification) to complex (five-level, heuristic classification) without rewrites, which mitigates this risk.

### Gaps to Address

- **Claude Code JSONL schema evolution:** No official documentation. Mitigation: build abstraction layer with version detection, store raw JSONL for re-parsing, monitor Claude Code changelog, test with multiple versions.
- **Market validation of five-level spectrum:** Unproven differentiation. Mitigation: ship three-level first (Phase 3), validate with users, only build five-level (Phase 4) if demand emerges.
- **Work-type classification accuracy:** Heuristics may not generalize across languages/frameworks. Mitigation: start with file path patterns (high confidence), defer AST/LLM classification, let users override classifications, iterate based on feedback.
- **GitHub Copilot and other AI tool integration:** Research focused on Claude Code. Mitigation: design session abstraction layer to be tool-agnostic even if v1 only implements Claude Code. Add Copilot/Cursor/Codeium in v2 based on user demand.
- **Cross-platform file watcher behavior:** Linux inotify vs. macOS FSEvents have different semantics and limits. Mitigation: test on both platforms in Phase 1, document inotify `max_user_watches` requirements, implement platform-specific tuning.

## Sources

### Primary (HIGH confidence)
- Go 1.26 Release Notes (go.dev/doc/go1.26) — Green Tea GC, goroutine leak profiles
- Go 1.24 Release Notes (go.dev/doc/go1.24) — Feature verification, support timeline
- fsnotify GitHub (github.com/fsnotify/fsnotify) — v1.9.0 cross-platform file watching
- go-git pkg.go.dev (pkg.go.dev/github.com/go-git/go-git/v5) — v5.16.5 pure Go implementation
- go-github GitHub (github.com/google/go-github) — v82.0.0 Google-maintained client
- modernc.org/sqlite pkg.go.dev — Pure Go SQLite driver
- cobra GitHub (github.com/spf13/cobra) — v1.10.2 CLI framework
- Claude Code Analytics Docs (code.claude.com/docs/en/analytics) — Attribution methodology, available metrics
- GitHub Copilot Usage Metrics Docs (docs.github.com/en/copilot/concepts/copilot-metrics)
- GitClear Diff Delta Factors (gitclear.com/diff_delta_factors) — Code operation classification
- CodeScene Product Page (codescene.com/product) — Behavioral code analysis features
- SQLite WAL documentation (sqlite.org/wal.html) — Concurrent read/write model
- Chokidar GitHub (github.com/paulmillr/chokidar) — File watcher event handling, known issues

### Secondary (MEDIUM confidence)
- GitClear 2025 AI Code Quality Report — 211M lines analyzed, code cloning trends
- Anthropic 2026 Agentic Coding Trends Report — Developer AI usage patterns (60% use, 0-20% full delegation)
- SemEval-2026 Task 13: GenAI Code Detection & Attribution (github.com/mbzuai-nlp/SemEval-2026-Task13)
- Claude Code JSONL format (liambx.com/blog/claude-code-log-analysis-with-duckdb, community tools)
- Bitfield Consulting: Rust vs Go 2026 — Language comparison
- Go + SQLite Best Practices (jacob.gold/posts/go-sqlite-best-practices)
- Go Concurrency Patterns: Pipelines (go.dev/blog/pipelines) — Official Go blog
- Graceful Shutdown in Go (victoriametrics.com/blog/go-graceful-shutdown)
- Laurence Tratt: Writing Unix Daemons (tratt.net/laurie/blog/2024/some_reflections_on_writing_unix_daemons)
- GitHub Community discussions on squash merge attribution issues
- Linux inotify limits documentation (watchexec.github.io/docs/inotify-limits)

### Tertiary (LOW confidence - needs validation)
- Claude Code local storage design (milvus.io blog) — Storage architecture reverse-engineering
- claude-JSONL-browser (github.com/withLinda/claude-JSONL-browser) — Community JSONL parser
- Simon Willison blog post on Claude Code log retention
- ACM Computing Surveys: Code Authorship Attribution Methods — Academic review

---
*Research completed: 2026-02-09*
*Ready for roadmap: yes*
