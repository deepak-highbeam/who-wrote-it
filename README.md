# who-wrote-it

A learning tool for engineers who use AI. It runs in the background while you code, tracks what the AI wrote vs. what you wrote, and classifies it by **type of work** — architecture, core logic, boilerplate, bug fixes, edge cases, tests. The result is a map of your knowledge gaps: where you leaned on AI, what you're avoiding, and what you need to go back and actually learn.

Also generates CLI reports, GitHub PR collaboration summaries, and code survival analytics.

> **Disclaimer:** This project was vibe coded on [Claude Code](https://claude.com/claude-code) with [Get Shit Done](https://github.com/gsd-build/get-shit-done). Use at your own risk.

## Why

AI makes it easy to gloss over the stuff you don't know. You ship working code without ever building a mental model of *why* it works. The struggle of figuring things out yourself — that's where comprehension actually forms. AI removes that struggle, and you don't even notice the gap.

**who-wrote-it is a mirror.** It shows you where you leaned on AI and what kind of work you offloaded. When you're learning a new language or framework — TypeScript, React, Go, whatever — it maps your knowledge gaps so you can go back and fill them.

### What it actually tells you

- **Where your blind spots are** — files classified as `mostly_ai` + `core_logic` are things you shipped but might not be able to debug, extend, or explain in a review
- **What you're avoiding** — if `edge_case` handling or error boundaries are always AI-written, you're not building intuition for failure modes in that language
- **What you think you know but don't** — `mostly_ai` files with high survival rates are deceptively comfortable. The code works, it stuck around, so you never went back to understand it. The blind spot persists.
- **Whether AI is doing the hard work or the easy work** — if your "Meaningful AI %" is much lower than "Raw AI %", AI is mostly generating boilerplate while you handle the architecture. If they're close, AI is making the decisions too.

### The honest move

After a sprint, look at your `mostly_ai` files. Pick one. Delete the AI's version. Rewrite it yourself. You already know what it *should* do — now force yourself through the *how*.

### It also answers

- Is AI writing the architecture, or just the boilerplate?
- Does AI-written core logic survive, or does it get rewritten?
- How does human-AI collaboration actually flow on this PR?

## How It Works

The daemon runs in the background while you code, combining three data sources:

1. **File system events** — watches your project directories for changes via fsnotify
2. **AI session data** — tails Claude Code JSONL session files for Write/Edit events
3. **Git history** — syncs commits, blame data, and Co-Authored-By tags every 30 seconds

A correlation engine matches file changes to AI session activity, then classifies each event by authorship level and work type. Results are stored in a local SQLite database.

At report time, attribution is computed by comparing **git diff additions** (only the lines that changed) against Claude's session event content using line-level SHA-256 hash matching.

```
file events ─┐
              ├─→ correlation engine ─→ authorship classifier ─→ work-type classifier ─→ SQLite
session data ─┘                                                        ↑
git history ───────────────────────────────────────────────────────────┘

At report time:
git diff (changed lines) + session event content ─→ line-level hash comparison ─→ AI% per file
```

## Authorship Classification

Every tracked file is classified into one of three authorship levels based on the proportion of changed lines attributable to AI:

| Level | Meaning |
|-------|---------|
| **mostly_ai** | >70% of changed lines match Claude's session output |
| **mixed** | 30-70% of changed lines match Claude's session output |
| **mostly_human** | <30% of changed lines match Claude's session output |

Only lines added in the git diff count — if Claude edited 1 line in a 500-line file, the denominator is 1, not 500. Empty/whitespace-only lines are excluded. Duplicate lines (like `}`) are frequency-counted, and pre-existing patterns from before tracking began are subtracted from AI attribution.

## Work Type Classification

Every event is also classified by the type of work:

| Work Type | Weight | How It's Detected |
|-----------|-------:|-------------------|
| **architecture** | 3.0x | Interface/struct definitions, `/models/`, `/schema/` paths |
| **core_logic** | 3.0x | Default — primary business logic |
| **bug_fix** | 2.0x | Commit message keywords (`fix:`, `bug:`, `hotfix:`) |
| **edge_case** | 2.0x | Error handling patterns (`if err != nil`, `catch`, `except`) |
| **boilerplate** | 1.0x | Config files, manifests (`go.mod`, `package.json`, `*.yml`) |
| **test_scaffolding** | 1.0x | Test files (`*_test.go`, `*.test.js`, `*.spec.ts`) |

Weights feed into the **Meaningful AI %** metric — architecture and core logic count 3x more than boilerplate, because not all lines of code are equal.

```
Meaningful AI % = Σ(AI_lines × weight) / Σ(total_changed_lines × weight) × 100
```

## Install

```bash
go install github.com/anthropic/who-wrote-it/cmd/whowroteit@latest
```

Or build from source:

```bash
git clone https://github.com/anthropic/who-wrote-it.git
cd who-wrote-it
go build -o whowroteit ./cmd/whowroteit
```

Requires Go 1.25+. Pure Go build — no CGO required.

## Prerequisites

who-wrote-it shells out to external CLI tools at runtime:

| Tool | Required | Used For |
|------|----------|----------|
| **[git](https://git-scm.com/)** | Yes | Diff computation, blame, commit history, merge-base resolution, branch detection |
| **[gh](https://cli.github.com/)** | Only for `pr-comment` | Auto-detecting PR number from current branch when posting GitHub PR comments |

Verify they're installed:

```bash
git --version   # any recent version works
gh --version    # only needed if you use `whowroteit pr-comment`
```

## Quick Start

```bash
# Start the daemon (runs in background)
whowroteit start

# Check if it's running
whowroteit ping

# View daemon status
whowroteit status

# Generate attribution report
whowroteit analyze

# Analyze a single file
whowroteit analyze --file internal/daemon/daemon.go

# Stop the daemon
whowroteit stop
```

## Configuration

Config lives at `~/.whowroteit/config.json`. All fields are optional — sensible defaults are used.

```json
{
  "watch_paths": ["/path/to/your/project"],
  "ignore_patterns": [".git", "node_modules", "vendor"]
}
```

Data is stored in `~/.whowroteit/` by default (`data_dir`, `socket_path`, and `db_path` can be overridden in config).

## CLI Commands

### `whowroteit analyze`

Project-level attribution report with authorship spectrum, work type distribution, and per-file breakdown.

```
Who Wrote It - Attribution Report
========================================

Project: /Users/dev/myproject
Meaningful AI: 45.3%
Raw AI:        52.1%
Total files:   15
Total lines:   342 (178 AI)

Authorship Spectrum
-----------------------------------
Level                        Files
-----------------------------------
mostly_ai                 8 (53.3%)
mixed                     3 (20.0%)
mostly_human              4 (26.7%)

Work Type Distribution
----------------------------------------------------------------------
Work Type          Tier     Files    Lines    AI%  Weight
----------------------------------------------------------------------
architecture       high         3       45  67.2%    3.0
core_logic         high         8      180  42.1%    3.0
bug_fix            medium       2       60  33.5%    2.0
boilerplate        low          1       57  90.0%    1.0
```

Use `--json` for machine-readable output. Use `--file` for single-file detail.

### `whowroteit pr-comment`

Posts a collaboration summary to a GitHub PR.

```bash
# Auto-detects owner/repo/PR from git context
whowroteit pr-comment --token $GITHUB_TOKEN

# Or specify explicitly
whowroteit pr-comment --owner myorg --repo myrepo --pr 42 --token $GITHUB_TOKEN

# Preview without posting
whowroteit pr-comment --dry-run
```

### `whowroteit survival`

Shows how AI-written code persists across subsequent commits by comparing attribution content hashes against current git blame.

```bash
whowroteit survival
whowroteit survival --json
```

Breaks down survival rates by authorship level and work type.

## Architecture

```
cmd/whowroteit/          CLI entry point (cobra)
internal/
  authorship/            3-level authorship classifier
  config/                JSON config loading with defaults
  correlation/           File-path event correlation (exact + fuzzy match)
  daemon/                Daemon lifecycle, goroutine orchestration
  github/                PR comment generation, GitHub API
  gitint/                Git blame, commit sync, Co-Authored-By parsing
  ipc/                   Unix domain socket server/client
  metrics/               Line-level attribution (SHA-256 hash comparison)
  report/                CLI report formatting (text + JSON)
  sessionparser/         Claude Code JSONL parser
  store/                 SQLite storage, migrations
  survival/              Content-hash survival analysis
  watcher/               fsnotify file system watcher
  worktype/              6-type work classifier
```

## AI Tool Support

Currently supports **Claude Code** via the SessionProvider interface. The architecture is designed for extension to other tools (Copilot, Cursor, Codeium) by implementing the same interface.

## Privacy

All data stays local. No telemetry, no cloud, no external API calls (except GitHub PR comments when you explicitly request them). The SQLite database lives in `~/.whowroteit/`.

## Known Limitations

### Linter/formatter attribution

When an AI writes code and an automated formatter (`gofmt`, `prettier`, `eslint --fix`) modifies it afterward, some lines may shift from AI to human attribution. The tool uses `strings.TrimSpace` before hashing, so **indentation changes and import reordering are handled correctly** (still attributed to AI). However, content-altering changes like operator spacing (`x:=1` → `x := 1`) or line splitting (single-line if → multi-line block) produce different hashes and are attributed to the linter/human.

In practice, modern LLMs write well-formatted code that linters rarely touch substantially. See `internal/metrics/linecalc_linter_test.go` for detailed test cases.

### Claude Code session format

The Claude Code JSONL session log format has no stability contract. Updates to Claude Code could change the structure, which would require updating the session parser.

### Single AI tool support

Currently only supports Claude Code. Other AI coding tools (Copilot, Cursor, Codeium) would need their own SessionProvider implementation.

## License

MIT
