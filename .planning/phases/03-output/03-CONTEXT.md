# Phase 3: Output - Context

**Gathered:** 2026-02-09
**Status:** Ready for planning

<domain>
## Phase Boundary

Users can see attribution and classification results through CLI reports, GitHub PR comments, and code survival analysis. This phase surfaces the intelligence built in Phase 2 through three output channels: CLI commands (`analyze`, `status`), a CLI-triggered GitHub PR comment, and code survival tracking.

</domain>

<decisions>
## Implementation Decisions

### CLI report design
- Summary-first layout: project-level summary up top, drill into files on demand
- Headline metrics: Meaningful AI % as the primary number, spectrum breakdown (5-level distribution) immediately below
- Drill-down via flags: `analyze --file path/to/file` for single file detail, default shows all files sorted by AI %
- Output format: Rich colored terminal tables by default, `--json` flag for machine-readable piping

### PR comment format
- Tone: Insight-driven — facts plus brief callouts (e.g., "Heavy AI usage in boilerplate (expected)", "Core logic is 90% human-written")
- Detail level: Compact summary — overall PR stats + top 3-5 notable files only. Devs skim PR comments, keep it short.
- No explicit flagging of concerning patterns — show the data with insights, let reviewers draw conclusions
- Trigger: CLI command (`who-wrote-it pr-comment`) that user runs locally or in CI, posts via GitHub API

### Claude's Discretion
- Status command layout and health check details
- Code survival tracking presentation (separate command vs inline, time windows, what counts as "survived")
- Color scheme and terminal formatting choices
- Exact wording of insight callouts in PR comments
- How `--json` output is structured

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

*Phase: 03-output*
*Context gathered: 2026-02-09*
