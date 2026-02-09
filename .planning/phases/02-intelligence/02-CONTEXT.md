# Phase 2: Intelligence - Context

**Gathered:** 2026-02-09
**Status:** Ready for planning

<domain>
## Phase Boundary

Raw signals from Phase 1 (file system events, Claude Code session data, git history) are correlated and classified into a five-level authorship spectrum with work-type labels and a meaningful AI percentage metric. This phase builds the internal intelligence engine — no user-facing output (that's Phase 3).

</domain>

<decisions>
## Implementation Decisions

### Authorship spectrum rules
- Any human edit to Claude-written code moves it from "fully AI" to "AI-first/human-revised" — no threshold, any touch counts
- Session context match counts as AI influence: if Claude discussed or suggested an approach, and the human then wrote code matching it (even without Claude's Write tool), it's "AI-suggested/human-written"
- First author wins for iterative collaboration: whoever wrote the initial version sets the spectrum direction, regardless of how much the other party later modified
- No tool use = fully human: if Claude didn't actively use tools (Write, Edit) in the relevant time window, the code is fully human regardless of session state — idle sessions don't taint attribution

### Work-type classification
- Three weighting tiers: High (architecture, core logic), Medium (bug fix, edge case handling), Low (boilerplate, test scaffolding)
- File + pattern based detection: use file paths (test files = test scaffolding), code patterns (interface/struct definitions = architecture, error handling = edge cases), and change size to classify — no session context analysis for work type
- Per-file granularity: each file gets one work-type label, commit breakdown is an aggregation of file-level classifications
- One-time overrides only: user corrections apply to the specific file/commit, no pattern learning or propagation to similar files

### Correlation strategy
- Tight time windows (seconds): match file write events to Claude tool uses within a few seconds for high confidence
- No session match = fully human: file events with no matching session event are classified as fully human by default, no retroactive influence from prior session context
- Closest in time wins: when multiple session events could match a file event, the temporally nearest session event is used
- Daemon data is truth: real-time daemon observations are authoritative for attribution. Git data (blame, Co-Authored-By, commits) fills gaps for code written before the daemon was running

### Claude's Discretion
- Exact time window size for "tight" correlation (2s, 5s, 10s — tune based on real data)
- Confidence score calculation and thresholds
- Specific code patterns for each work-type category
- How to handle the "uncertain" designation for ambiguous cases
- Internal data structures and query patterns for correlation

</decisions>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 02-intelligence*
*Context gathered: 2026-02-09*
