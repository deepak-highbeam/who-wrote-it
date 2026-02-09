# Feature Research: Code Authorship Attribution & AI Collaboration Intelligence

**Domain:** Development intelligence / Code authorship attribution / AI collaboration analytics
**Researched:** 2026-02-09
**Confidence:** MEDIUM (competitive landscape well-mapped; novel features are unvalidated by market)

---

## Competitor Landscape

Before defining features, understanding what exists and where the gaps are.

### Existing Tools and What They Do

| Tool | Category | Core Capability | Limitation for Our Use Case |
|------|----------|-----------------|----------------------------|
| **git blame / git log** | Built-in VCS | Line-level last-author attribution | Only tracks commit author, not human-vs-AI. Breaks on refactoring, moves, reformats. No concept of "work type." |
| **GitHub Copilot Metrics** | Vendor analytics | Acceptance rates, lines generated, sessions, PRs with Copilot code, daily active users | Binary "with Copilot / without Copilot." No granularity on collaboration pattern. No work-type classification. Only covers GitHub Copilot users. |
| **Claude Code Analytics** | Vendor analytics | Sessions, lines added/removed, commits, PRs, tool usage, cost per model, suggestion accept rate. PR attribution via line matching. | Conservative attribution (high-confidence only). Binary "with CC / without CC." No work-type classification. No collaboration pattern analysis. GitHub-only for contribution metrics. |
| **GitClear** | Engineering intelligence | Diff Delta (cognitive load per commit), code churn tracking, 65+ metrics, operation classification (move, copy/paste, find-replace, refactor), AI code quality research | Focuses on code quality and velocity metrics. Classifies code operations but not work types. Does not attribute human-vs-AI at line level. Research arm publishes aggregate AI impact data but the product is metrics-focused. |
| **CodeScene** | Behavioral code analysis | Hotspot detection, Code Health scoring, change coupling, knowledge distribution, bus factor, 25+ code smell detection across 30+ languages, team dynamics | Analyzes code health and team patterns, not AI collaboration patterns. No human-vs-AI attribution. Focused on technical debt and team coordination risks. |
| **LinearB** | Engineering intelligence | DORA metrics, SPACE framework, cycle time, PR analytics, AI code review, investment allocation, DevEx metrics, 20 SDLC metrics + 3 AI metrics | Tracks AI tool impact on delivery velocity but not at line or work-type level. Process-focused (cycle time, PR throughput), not content-focused (what kind of code). |
| **Swarmia** | Engineering effectiveness | DORA metrics, developer experience surveys, working agreements, real-time data updates, team health | Similar to LinearB. Process metrics, not code content analysis. No AI attribution. |
| **Jellyfish** | Engineering management | Business alignment, software capitalization, GenAI tool impact assessment, resource allocation | Executive-facing. Tracks GenAI impact on velocity but not collaboration patterns. 24-hour data delay. |
| **Codespy.ai** | AI code detection | Detects AI-generated code via AST analysis, token probability, entropy, neural fingerprinting. 98% accuracy. VS Code extension, GitHub integration. | Binary detection only (AI or not). No collaboration pattern. No work-type classification. Focused on "catching" AI code, not understanding it. Academic/compliance use case. |
| **GPTZero** | AI content detection | Burstiness and perplexity analysis for text and code. 99% accuracy on pure AI content. | Text-focused with code as secondary. Less effective on hybrid code. No developer workflow integration. |

### The Gap

**No tool currently answers: "What kind of work did the AI do vs. the human?"**

Every tool falls into one of three buckets:
1. **Binary attribution** -- "Was AI involved? Yes/No." (Copilot Metrics, Claude Code Analytics, Codespy)
2. **Process metrics** -- "How fast are we shipping?" (LinearB, Swarmia, Jellyfish, DORA/SPACE)
3. **Code quality analysis** -- "Is this code healthy?" (GitClear, CodeScene)

None classifies the *nature* of the contribution. None answers:
- Did the AI write the architecture or just the boilerplate?
- Did the human identify the bug or did the AI?
- Is "40% AI-written" mostly import statements, or mostly core business logic?

This is the white space "Who Wrote It" occupies.

---

## Feature Landscape

### Table Stakes (Users Expect These)

Features users assume exist. Missing these = product feels incomplete or untrustworthy.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| **Git integration (commit/diff parsing)** | Every dev tool reads git. Without it, tool has no data foundation. | MEDIUM | Must handle rebases, squash merges, cherry-picks, and renamed files correctly. git blame's limitations (breaks on refactors) are well-known -- we need to do better. |
| **Per-file and per-line authorship tracking** | Baseline expectation from git blame. Users expect at minimum "who touched this line." | MEDIUM | Must attribute at line level but also aggregate to file, directory, and project levels. |
| **Human vs. AI binary classification** | The most basic question users will ask. Copilot Metrics and Claude Code Analytics already provide this. Not having it makes the tool seem less capable than free vendor dashboards. | MEDIUM | This is necessary but insufficient. It's the "and also" on top of which differentiators live. |
| **Session-level data capture** | Users expect the tool to know when AI was involved in a coding session. Claude Code Analytics API already provides session data. | MEDIUM | Must ingest Claude Code session data (lines, tool usage, timestamps). Extensible to other AI tools later. |
| **Dashboard / visualization** | Users expect to see data, not just query it. Every competitor has dashboards. | HIGH | Start with CLI + simple web view. Full dashboard is high complexity but basic visualization is table stakes. |
| **Time-range filtering** | Users expect to filter by day, week, sprint, release. Every analytics tool has this. | LOW | Standard date-range picker on all views. |
| **Repository-scoped analysis** | Users expect to analyze one repo at a time. Multi-repo is a differentiator, single-repo is table stakes. | LOW | Start single-repo. Multi-repo is v2. |
| **Privacy-respecting design** | Developers hate surveillance tools. GitClear explicitly markets as "developer-friendly." CodeScene emphasizes actionable insights over tracking. If developers feel watched, adoption dies. | LOW (design) | All data stays local by default. No phoning home. No manager-only dashboards that devs cannot see. This is a design principle, not a feature -- but violating it is a dealbreaker. |
| **Accurate attribution (not inflated)** | Claude Code Analytics explicitly calls its metrics "deliberately conservative." GitClear filters 97.4% of changed lines as non-substantive. Users distrust inflated numbers. | HIGH | The "effective lines" concept from Claude Code (excluding trivial lines) and GitClear's Diff Delta (filtering non-cognitive-load changes) set the bar. Our numbers must be at least as rigorous. |

### Differentiators (Competitive Advantage)

Features that set "Who Wrote It" apart. These are the product's reason to exist.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Five-level authorship spectrum** | The core insight: binary "AI or not" is misleading. The 5 categories (completely AI, AI-first/human-rewrote, human-first/AI-rewrote, human-wrote/AI-suggested, completely human) capture the real collaboration pattern. No competitor does this. | HIGH | Requires correlating session timestamps, file system events, and git diffs to infer collaboration direction. The hardest technical challenge in the product. |
| **Work-type classification** | Classifying code changes as: architecture decisions, core logic, boilerplate/glue, bug fixes, edge case handling, test scaffolding. No competitor classifies the *nature* of what was written. | HIGH | Likely requires AST parsing + heuristics + possibly LLM classification. Architecture decisions (new interfaces, module structure) vs boilerplate (imports, config, standard patterns) have different signals. |
| **"Meaningful AI %" metric** | The headline metric: what percentage of *meaningful* work (architecture + core logic + bug identification) was AI-driven vs. human-driven, as opposed to raw line counts that include boilerplate inflation. | MEDIUM | Depends on both the authorship spectrum and work-type classification. This is the combined insight that makes the product compelling. |
| **Collaboration pattern visualization** | Show the flow: "Human identified bug -> AI proposed fix -> Human refined edge cases." This tells the story of how code actually gets written in an AI-assisted workflow. | HIGH | Timeline/flow visualization. Requires rich session data. The narrative is more valuable than the numbers. |
| **Real-time monitoring (always-on)** | Most analytics tools are retrospective (analyze git history after the fact). "Who Wrote It" captures signals in real-time during development: file system events, session activity, edit patterns. | HIGH | File system watcher + Claude Code session integration. This enables the authorship spectrum (you can see who typed first) which git history alone cannot tell you. |
| **Contribution quality weighting** | Not all lines are equal. An architecture decision that shapes 1000 subsequent lines matters more than those 1000 lines. Weight contributions by impact, not volume. | HIGH | Extremely hard to do well. Could use heuristics: files that many other files import from = architectural. New function signatures vs function bodies. Interface definitions vs implementations. |
| **AI delegation pattern insights** | Per Anthropic's 2026 report: devs use AI in 60% of work but fully delegate only 0-20%. Show the user their delegation pattern: what they hand off, what they co-author, what they keep. | MEDIUM | Derived from the authorship spectrum data. Requires enough data to identify patterns over time. |
| **Code survival tracking** | Track whether AI-generated code survives or gets rewritten. GitClear's research shows AI code leads to 4x more code cloning and declining refactoring. Our tool can track this per-project. | MEDIUM | Requires tracking lines over time across commits. GitClear does this for aggregate research; we do it per-project with human-vs-AI attribution. |

### Anti-Features (Commonly Requested, Often Problematic)

Features that seem good but create problems. Explicitly NOT building these.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **Developer ranking / leaderboard by AI usage** | Managers want to know "who uses AI most." Claude Code Analytics has a leaderboard. | Creates perverse incentives. Developers either game metrics or feel surveilled. High AI usage is not inherently good or bad. Anthropic's report shows even AI-heavy devs only fully delegate 0-20% of tasks. | Show team-level patterns, not individual rankings. If individual data is shown, make it self-service (developer sees their own data) not manager-facing. |
| **"AI code percentage" as a KPI** | Seems like the obvious metric. "40% of our code is AI-written!" | As the project's core insight states: this number is meaningless without work-type context. 40% AI-written boilerplate is very different from 40% AI-written core logic. Publishing this as a KPI incentivizes inflating AI usage for optics. | Show the "meaningful AI %" that accounts for work type. Present as insight, not target. |
| **Real-time surveillance / screen monitoring** | "If we watch the developer's screen, we get perfect attribution." | Developers will revolt. Privacy violation. Legal issues. Destroys trust and adoption. GitClear markets explicitly against surveillance-style analytics for good reason. | Use non-invasive signals: git events, file system timestamps, AI tool session APIs. These are sufficient and non-creepy. |
| **Automated performance scoring** | "Rate each developer's productivity based on AI collaboration." | No tool has solved the developer productivity measurement problem without creating backlash (see: the entire history of lines-of-code metrics). Automated scoring will be gamed or resented. | Provide data and insights. Let humans (the developer themselves, or their manager in conversation) interpret. |
| **Blocking / gatekeeping based on AI percentage** | "Block PRs that are more than X% AI-written." | Arbitrary thresholds punish good AI collaboration. Some tasks should be mostly AI-written (boilerplate, test scaffolding). Some should be mostly human (architecture). A blanket threshold makes no sense. | Show work-type breakdown in PR review. Let reviewers make informed decisions based on what was AI-written, not how much. |
| **Support for every AI coding tool from day one** | "We use Copilot, Cursor, Claude Code, and Codeium." | Each tool has different telemetry, different session formats, different APIs. Supporting all from v1 means doing all of them badly. | Start with Claude Code (richest session data via Analytics API + OpenTelemetry). Add tools based on user demand. Design the abstraction layer to be tool-agnostic even if v1 only supports one tool. |
| **Predictive "AI will write this" suggestions** | "Tell me which tasks to delegate to AI." | Moves the tool from analysis into recommendation territory. Much harder problem. High risk of bad suggestions. | Descriptive first: show what the developer already delegates effectively. Let them generalize from their own patterns. |

---

## Feature Dependencies

```
[Git Integration]
    |
    +--> [Per-line Authorship Tracking]
    |        |
    |        +--> [Human vs. AI Binary Classification]
    |                 |
    |                 +--> [Five-level Authorship Spectrum] (requires session data)
    |                          |
    |                          +--> [AI Delegation Pattern Insights]
    |                          |
    |                          +--> [Collaboration Pattern Visualization]
    |
    +--> [Code Survival Tracking]

[Session Data Capture] (Claude Code Analytics API / OpenTelemetry)
    |
    +--> [Five-level Authorship Spectrum]
    +--> [Real-time Monitoring]

[AST Parsing / Code Analysis]
    |
    +--> [Work-type Classification]
             |
             +--> ["Meaningful AI %" Metric] (requires authorship spectrum + work-type)
             +--> [Contribution Quality Weighting]

[Dashboard / Visualization]
    +--> [Time-range Filtering]
    +--> [Collaboration Pattern Visualization]
```

### Dependency Notes

- **Five-level authorship spectrum requires session data:** Git history alone cannot distinguish "AI-first, human-rewrote" from "human-first, AI-rewrote." You need timestamps from the AI tool's session to know who went first.
- **"Meaningful AI %" requires both authorship + work-type:** This is the product's signature metric and sits at the intersection of the two hardest features. Plan for it to arrive after both dependencies are solid.
- **Work-type classification is independent of authorship:** You can classify code as "architecture" vs "boilerplate" without knowing who wrote it. This means these two hard problems can be worked on in parallel.
- **Real-time monitoring enhances but is not required for authorship:** Retrospective analysis of git + session logs works. Real-time adds richness (file system events as they happen) but is not gating.
- **Dashboard is downstream of everything:** Build data pipelines and CLI first. Visualization can come last and iterate.

---

## MVP Definition

### Launch With (v1)

Minimum viable product -- what's needed to validate the core insight.

- [ ] **Git integration with smart diff parsing** -- Parse commits, handle renames/moves, extract per-line authorship. Better than git blame. Essential data foundation.
- [ ] **Claude Code session data ingestion** -- Read from Claude Code Analytics API or local session files. Map session activity to code changes.
- [ ] **Five-level authorship classification (basic)** -- Even a heuristic version (based on timestamps and edit deltas) validates the core product thesis.
- [ ] **Work-type classification (basic)** -- Start with heuristic rules: new file structure = architecture, import statements = boilerplate, test files = test scaffolding, changes inside existing functions = core logic or bug fix. Does not need to be perfect to be useful.
- [ ] **"Meaningful AI %" metric** -- The signature metric combining authorship + work type. Even a rough version tells a story no other tool tells.
- [ ] **CLI output** -- `who-wrote-it analyze` produces a report. No dashboard needed for v1. Developer-facing, runs locally.
- [ ] **Local-only, privacy-first** -- All analysis runs on the developer's machine. No cloud, no data leaving the repo.

### Add After Validation (v1.x)

Features to add once the core thesis is validated.

- [ ] **Collaboration pattern visualization** -- Once we have the data, show the human-AI collaboration flow visually. Trigger: users say "the numbers are useful but I want to see the story."
- [ ] **Code survival tracking** -- Track AI-written code longevity over weeks/months. Trigger: users ask "does the AI code actually stick?"
- [ ] **Real-time file system monitoring** -- Always-on watcher for richer data capture. Trigger: retrospective analysis proves valuable but users want more granularity.
- [ ] **Web dashboard** -- Local web UI for visualizing data. Trigger: CLI output is useful but teams want to share insights.
- [ ] **Multi-repo support** -- Aggregate across repositories. Trigger: individual developers validate the tool, teams want org-level view.
- [ ] **AI delegation pattern insights** -- "Here's what you typically delegate, here's what you keep." Trigger: enough data accumulated per user to identify patterns.

### Future Consideration (v2+)

Features to defer until product-market fit is established.

- [ ] **Additional AI tool support (Copilot, Cursor, Codeium)** -- Each tool has different telemetry. Defer until Claude Code support is proven. Design the abstraction layer in v1 for future extensibility.
- [ ] **Team/org analytics** -- Aggregate insights across developers. Defer because individual developer use case must work first, and team analytics require careful privacy design.
- [ ] **CI/CD integration** -- Run analysis as part of PR review pipeline. Defer because the core analysis engine must be reliable first.
- [ ] **PR-level attribution annotations** -- Label PR sections with authorship and work type. Defer because it requires GitHub/GitLab API integration and careful UX for the review workflow.
- [ ] **Historical analysis mode** -- Analyze existing git history without session data (degraded accuracy but still useful for work-type classification). Defer because it is a different analysis mode and the real-time + session data path is the primary value.

---

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Git integration (smart diff parsing) | HIGH | MEDIUM | P1 |
| Claude Code session data ingestion | HIGH | MEDIUM | P1 |
| Five-level authorship classification | HIGH | HIGH | P1 |
| Work-type classification (heuristic) | HIGH | HIGH | P1 |
| "Meaningful AI %" metric | HIGH | LOW (once deps built) | P1 |
| CLI output / local analysis | HIGH | LOW | P1 |
| Privacy-first local-only design | HIGH | LOW | P1 |
| Time-range filtering | MEDIUM | LOW | P1 |
| Collaboration pattern visualization | HIGH | HIGH | P2 |
| Code survival tracking | MEDIUM | MEDIUM | P2 |
| Real-time file system monitoring | MEDIUM | HIGH | P2 |
| Web dashboard | MEDIUM | HIGH | P2 |
| AI delegation pattern insights | MEDIUM | MEDIUM | P2 |
| Multi-repo support | MEDIUM | MEDIUM | P2 |
| Additional AI tool support | MEDIUM | HIGH | P3 |
| Team/org analytics | MEDIUM | HIGH | P3 |
| CI/CD integration | LOW | MEDIUM | P3 |
| PR-level attribution annotations | MEDIUM | HIGH | P3 |
| Contribution quality weighting | HIGH | HIGH | P3 |

**Priority key:**
- P1: Must have for launch -- validates the core thesis
- P2: Should have -- adds depth and usability once core works
- P3: Nice to have -- expands reach and use cases

---

## Competitor Feature Analysis

| Feature | git blame | Copilot Metrics | Claude Code Analytics | GitClear | CodeScene | LinearB | Codespy.ai | **Who Wrote It** |
|---------|-----------|-----------------|----------------------|----------|-----------|---------|------------|-----------------|
| Line-level authorship | Yes (last author only) | No | Yes (PR-level) | No (commit-level) | No | No | No | **Yes (with history)** |
| Human vs AI attribution | No | Yes (binary) | Yes (binary) | No (research only) | No | Partial (AI metrics) | Yes (binary) | **Yes (5-level spectrum)** |
| Work-type classification | No | No | No | Partial (operation type: move, copy, refactor) | No | No | No | **Yes (architecture, logic, boilerplate, bugfix, edge case, test)** |
| Collaboration pattern analysis | No | No | No | No | No | No | No | **Yes (who went first, who refined)** |
| Code quality metrics | No | No | No | Yes (Diff Delta, churn) | Yes (Code Health) | Yes (DORA) | No | No (v1) |
| Real-time monitoring | No | Partial (session tracking) | Partial (session tracking) | No | IDE extension | No | No | **Yes (file system + session)** |
| Privacy-first / local | Yes (git is local) | No (cloud) | No (API/cloud) | No (SaaS) | Partial (IDE local, SaaS cloud) | No (SaaS) | No (SaaS) | **Yes (local-first)** |
| Code survival tracking | No | No | No | Yes (churn metric) | No | No | No | **Yes (with AI attribution)** |
| Free / open source | Yes | No (included with Copilot) | Free API but requires plan | Freemium | Freemium | Freemium | Freemium | **Yes (open source target)** |

### Key Takeaways from Competitor Analysis

1. **The 5-level authorship spectrum is completely uncontested.** No tool goes beyond binary "AI or not."
2. **Work-type classification is the biggest gap in the market.** GitClear classifies code *operations* (move, copy, add, delete) but not code *purpose* (architecture, boilerplate, bug fix). CodeScene classifies code *health* but not code *type*.
3. **Collaboration pattern analysis does not exist.** The "who went first" question is unanswered by any tool.
4. **Local-first is a differentiator against SaaS competitors.** Developer trust matters enormously in this space.
5. **Real-time monitoring during development is rare.** Most tools are retrospective (analyze after commit). The always-on approach enables richer attribution.

---

## Sources

### Official Documentation (HIGH confidence)
- [Claude Code Analytics Docs](https://code.claude.com/docs/en/analytics) -- Detailed attribution methodology, available metrics, PR tagging
- [Claude Code Analytics API](https://docs.anthropic.com/en/api/claude-code-analytics-api) -- API endpoints, data structure, aggregation
- [GitHub Copilot Usage Metrics](https://docs.github.com/en/copilot/concepts/copilot-metrics) -- Available metrics and dashboards
- [GitClear Diff Delta Factors](https://www.gitclear.com/diff_delta_factors) -- Code operation classification methodology
- [CodeScene Product Page](https://codescene.com/product) -- Feature list, behavioral code analysis capabilities

### Research Reports (MEDIUM confidence)
- [GitClear 2025 AI Code Quality Report](https://www.gitclear.com/ai_assistant_code_quality_2025_research) -- 211M lines analyzed, code cloning 4x growth, refactoring decline
- [Anthropic 2026 Agentic Coding Trends Report](https://resources.anthropic.com/hubfs/2026%20Agentic%20Coding%20Trends%20Report.pdf) -- Developers use AI in 60% of work, fully delegate only 0-20%
- [SemEval-2026 Task 13: GenAI Code Detection & Attribution](https://github.com/mbzuai-nlp/SemEval-2026-Task13) -- Academic competition for AI code detection (detection, authorship, mixed-source)
- [CodeRabbit: AI vs Human Code Report](https://www.coderabbit.ai/blog/state-of-ai-vs-human-code-generation-report) -- AI code creates 1.7x more issues
- [IBM AI Attribution Toolkit](https://research.ibm.com/blog/AI-attribution-toolkit) -- Framework for granular AI contribution attribution

### Ecosystem Analysis (MEDIUM confidence)
- [LinearB Platform](https://linearb.io/platform/engineering-metrics) -- Engineering metrics feature set
- [Swarmia vs Jellyfish comparison](https://www.swarmia.com/alternative/jellyfish/) -- Engineering analytics landscape
- [Faros AI DORA tools](https://www.faros.ai/blog/best-dora-metrics-tools-2026) -- Engineering intelligence platform landscape

### AI Code Detection (MEDIUM confidence)
- [Codespy.ai](https://codespy.ai/) -- AI code detection via AST analysis, entropy, neural fingerprinting
- [AI Code Detectors 2026](https://futuramo.com/blog/ai-code-detectors-2026/) -- Landscape of AI code detection tools
- [Code Stylometry for Authorship Attribution](https://dl.acm.org/doi/10.1145/3733799.3762964) -- Academic work on code stylometry for AI identification

### Community / Market Analysis (LOW confidence -- included for context)
- [Gartner Developer Productivity Insight Platforms](https://www.gartner.com/reviews/market/developer-productivity-insight-platforms) -- Market category definition (renamed from SEIP to DPIP)
- [DORA vs SPACE Framework Comparison](https://www.swarmia.com/blog/comparing-developer-productivity-frameworks/) -- Framework analysis for productivity measurement
- [AI Attribution Paradox in Open Source](https://arxiv.org/html/2512.00867v1) -- Only 29.5% of AI-using commits explicitly disclose AI involvement

---
*Feature research for: Code Authorship Attribution & AI Collaboration Intelligence*
*Researched: 2026-02-09*
