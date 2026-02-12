# who-wrote-it

An always-on Go daemon that monitors how code gets written in real-time, attributing contributions not just by authorship (human vs AI) but by **type of work** — architecture decisions, core logic, boilerplate, bug fixes, edge case handling, test scaffolding.

Generates CLI reports, GitHub PR collaboration summaries, and code survival analytics showing how AI-written code persists over time.

## Why

Line counts are vanity metrics. Knowing "AI wrote 60% of the code" tells you nothing useful. What matters is **what kind of work** the AI is doing.

who-wrote-it answers questions like:
- Is AI writing the architecture, or just the boilerplate?
- Does AI-written core logic survive, or does it get rewritten?
- How does human-AI collaboration actually flow on this PR?

## How It Works

The daemon runs in the background while you code, combining three data sources:

1. **File system events** — watches your project directories for changes via fsnotify
2. **AI session data** — tails Claude Code JSONL session files for Write/Edit events
3. **Git history** — syncs commits, blame data, and Co-Authored-By tags every 5 minutes

A correlation engine matches file changes to AI session activity, then classifies each event by authorship level and work type. Results are stored in a local SQLite database.

At report time, attribution is computed by comparing **git diff additions** (only the lines that changed) against Claude's session event content using line-level hash matching. This ensures accurate attribution even for edits to existing files.

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

### How Line Attribution Works

1. For each tracked file, find the git state **before tracking began** (earliest attribution timestamp)
2. Compute `git diff` between that base and the current working tree — these are the **changed lines**
3. For Edit events, diff `old_string` vs `new_string` to extract only **genuinely new lines** — context lines that existed before the edit are stripped
4. Compare each changed line (by SHA-256 hash of trimmed content) against Claude's extracted content
5. **Subtract pre-existing patterns**: lines that already existed in the base file (before tracking) are removed from Claude's hash map, so common boilerplate like annotations or closing brackets aren't falsely attributed to AI
6. Empty/whitespace-only lines are excluded — they carry no authorship signal
7. AI% = AI lines / total non-empty changed lines

This means:
- **New files** created by Claude: all lines are additions in the diff, compared against session content
- **Existing files** edited by Claude: only the changed lines count, not the whole file. If Claude edited 1 line in a 500-line file, the denominator is 1, not 500
- **Duplicate lines** (like `}` or `return nil`): frequency-counted — if Claude wrote 3 closing braces and the diff has 5, only 3 are attributed to AI
- **Pre-existing patterns**: if `@Override` appeared 10 times in the file before Claude was involved, those patterns won't count as AI even if Claude's edits also contain them

Correlation uses two match tiers:
- **exact_file** — Claude's session event targets the same file path
- **fuzzy_file** — Same filename with different path prefix (e.g., `foo.go` vs `/abs/path/to/foo.go`)

Git Co-Authored-By tags serve as a supplementary signal.

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

## Quick Start

```bash
# Start the daemon (foreground for now)
whowroteit start --foreground

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
  "data_dir": "~/.whowroteit",
  "socket_path": "~/.whowroteit/whowroteit.sock",
  "db_path": "~/.whowroteit/whowroteit.db",
  "watch_paths": ["/path/to/your/project"],
  "ignore_patterns": [".git", "node_modules", "vendor"]
}
```

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
Total lines changed: 342
AI lines:      178

Authorship Spectrum
--------------------------------------------------
Level                            Count     Pct
--------------------------------------------------
mostly_ai                           8    53.3%
mixed                               3    20.0%
mostly_human                        4    26.7%

Work Type Distribution
------------------------------------------------------------
Work Type          Tier     Files    AI% Weight
------------------------------------------------------------
architecture       high         3   67.2%    3.0
core_logic         high         8   42.1%    3.0
bug_fix            medium       2   33.5%    2.0
boilerplate        low          1   90.0%    1.0
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

The comment includes:
- Meaningful AI % with authorship breakdown
- Work type table with tier and AI % per type
- Insight callouts (e.g., "Heavy AI in boilerplate — expected for scaffolding")
- Notable files with collaboration patterns

### `whowroteit survival`

Shows how AI-written code persists across subsequent commits by comparing attribution content hashes against current git blame.

```bash
whowroteit survival
whowroteit survival --json
```

Breaks down survival rates by authorship level and work type — answering whether AI-written architecture holds up or gets rewritten.

## The Meaningful AI % Metric

Raw AI percentage treats all code equally. Meaningful AI % weights by work type:

```
Meaningful AI % = Σ(AI_lines × weight) / Σ(total_changed_lines × weight) × 100
```

A project where AI writes 90% of the boilerplate (1.0x) but only 20% of the core logic (3.0x) will have a lower Meaningful AI % than raw numbers suggest — which is a more accurate picture of AI's actual contribution.

## Architecture

```
cmd/whowroteit/          CLI entry point (cobra)
internal/
  authorship/            3-level authorship classifier (mostly_ai / mixed / mostly_human)
  config/                JSON config loading with defaults
  correlation/           File-path event correlation engine (exact + fuzzy match)
  daemon/                Daemon lifecycle, goroutine orchestration
  github/                PR comment generation, GitHub API
  gitint/                Git blame, commit sync, Co-Authored-By parsing
  ipc/                   Unix domain socket server/client
  metrics/               Line-level attribution (SHA-256 hash comparison)
  report/                CLI report formatting (text + JSON), git-diff-based attribution
  sessionparser/         Claude Code JSONL parser, SessionProvider interface
  store/                 SQLite storage, migrations, queries
  survival/              Content-hash survival analysis
  watcher/               fsnotify file system watcher
  worktype/              6-type work classifier with user overrides
```

## AI Tool Support

Currently supports **Claude Code** via the SessionProvider interface. The architecture is designed for extension to other tools (Copilot, Cursor, Codeium) by implementing the same interface.

## Privacy

All data stays local. No telemetry, no cloud, no external API calls (except GitHub PR comments when you explicitly request them). The SQLite database lives in `~/.whowroteit/`.

## License

MIT
