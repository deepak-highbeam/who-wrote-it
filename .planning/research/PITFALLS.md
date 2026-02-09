# Pitfalls Research

**Domain:** Code authorship attribution / AI collaboration intelligence daemon
**Researched:** 2026-02-09
**Confidence:** MEDIUM-HIGH (verified across multiple sources; Claude Code session format details are LOW confidence due to undocumented schema)

---

## Critical Pitfalls

Mistakes that cause rewrites or major issues.

### Pitfall 1: File Watcher Event Storms Causing CPU Spikes and Missed Events

**What goes wrong:**
A single file save in VS Code or other editors triggers 3-5 filesystem events in rapid succession (create temp, write, rename, chmod, etc.). Without debouncing, the daemon processes each event independently, causing CPU spikes, redundant git operations, and potential race conditions where partially-written files are analyzed. At scale (monorepo with thousands of files), this cascades into the watcher consuming more resources than the editor itself.

**Why it happens:**
Editors use atomic save patterns: write to a temporary file, then rename to the target. This produces `unlink` + `add` (or `change`) sequences instead of a single `change`. Some editors (VS Code, JetBrains) also update metadata files, `.git/index`, and other auxiliary files on every save. Developers building watchers test with simple `echo > file` and never encounter the multi-event pattern.

**How to avoid:**
- Use chokidar's `awaitWriteFinish` option with `stabilityThreshold: 300` (not the default 2000ms which feels sluggish) and `pollInterval: 50`
- Implement a custom debounce layer on top: collect events into 100-200ms windows, deduplicate by file path, process only the final state
- Maintain an explicit ignore list: `.git/**`, `node_modules/**`, `*.swp`, `*.tmp`, `*~`, `.DS_Store`
- Filter by file extension early -- only process known code file types
- Set `depth` option to limit recursive watching when possible

**Warning signs:**
- Daemon CPU usage > 5% when idle
- Same file appearing in processing queue multiple times per save
- Log output showing rapid-fire events for a single user action
- Users reporting "my editor feels slow" after installing the daemon

**Phase to address:** Phase 1 (File Watcher Foundation) -- this must be solved before any downstream processing is built on top. Get the event pipeline right first or everything built on it inherits the problem.

**Confidence:** HIGH -- well-documented across chokidar issues, .NET FileSystemWatcher discussions, and cross-platform watcher libraries.

---

### Pitfall 2: Squash Merges and Rebases Destroying Authorship History

**What goes wrong:**
Git `squash merge` replaces all individual commits with a single commit authored by whoever clicked "merge." A branch with 15 commits from 3 contributors (including AI-assisted ones) becomes one commit attributed to the merge author. Similarly, `rebase` rewrites commit hashes, and `force push` obliterates the previous branch history entirely. Any authorship attribution system that relies solely on git commit metadata (author, committer) will produce completely wrong results after these operations.

**Why it happens:**
Squash merge is the default merge strategy on many GitHub repositories because it produces a clean linear history. Teams adopt it without considering that it destroys per-commit author granularity. The `git merge --squash` command explicitly uses the merger as the author. Force pushes after interactive rebases similarly rewrite history.

**How to avoid:**
- **Never rely solely on git history for attribution.** The daemon must capture authorship at write-time (file watcher events) and store it independently from git
- Record attribution data in a local database keyed by file + line range + timestamp, not by commit hash
- When processing git events, use them as confirmation/reconciliation signals, not as the source of truth
- Parse `Co-authored-by` trailers in commit messages as supplementary data
- For squash merges: detect them (single commit with large diff touching many files), flag as "attribution uncertain from git alone," fall back to daemon-captured data
- Store pre-squash branch data if accessible (GitHub PR API has individual commits)

**Warning signs:**
- Attribution reports showing one person "wrote" everything in a PR that had multiple contributors
- Sudden spikes in attributed lines for merge-button-clickers
- Test cases passing with merge commits but failing with squash merges
- Users saying "this says I wrote code I never touched"

**Phase to address:** Phase 1-2 (Data Model Design and Git Integration) -- the data model must be designed from day one to treat git as a secondary source, not the primary one. This is an architectural decision that is extremely expensive to change later.

**Confidence:** HIGH -- extensively documented in GitHub community discussions, GitLab forums, and git documentation itself.

---

### Pitfall 3: Work-Type Classification is Fundamentally Fuzzy

**What goes wrong:**
Attempting to classify code changes as "boilerplate," "core logic," "tests," "configuration," etc. produces ambiguous results that undermine user trust. Is a React component that follows a pattern "boilerplate" or "core logic"? Is a complex test setup "tests" or "infrastructure"? The boundaries are inherently subjective, and different developers will disagree on the classification of the same code. Systems that present fuzzy classifications with false precision (e.g., "87% core logic") actively mislead users.

**Why it happens:**
The desire to distinguish "meaningful" from "mechanical" work is the core value proposition, but it conflates multiple orthogonal dimensions: complexity, novelty, creativity, domain relevance, and effort. AST-based analysis can identify syntactic patterns (imports, config objects, type definitions) but cannot determine semantic significance. A one-line config change might be the result of hours of debugging; a 200-line generated migration file took zero thought.

**How to avoid:**
- Use coarse-grained categories that are defensible: "test code," "configuration," "generated/scaffolded," "application code" -- resist subdividing further
- Classify by file path patterns first (tests are in `__tests__/`, configs are `*.config.*`), which is unambiguous and covers 60-70% of cases
- For application code, classify by change characteristics, not code content: "AI-assisted" (written during Claude Code session) vs "human-authored" (no AI tool active)
- Present classifications with explicit uncertainty: "likely boilerplate (config file pattern)" not "boilerplate: 94%"
- Let users override/correct classifications and learn from corrections
- Start with the attribution dimension (who/what wrote it) before tackling the complexity dimension (what kind of work is it)

**Warning signs:**
- Spending more than 2 weeks on classification taxonomy before shipping anything
- Users disagreeing with classifications and having no way to correct them
- Classification accuracy below 80% on your own test data
- Designing an ML model for classification in Phase 1

**Phase to address:** Phase 3+ (Classification Engine) -- ship attribution (who wrote it) first. Classification (what kind of work) is a layer on top that can iterate. Do not block MVP on accurate work-type classification.

**Confidence:** MEDIUM -- based on code analysis research literature and practical experience with code complexity metrics. The fundamental fuzziness is well-established in software engineering research.

---

### Pitfall 4: Claude Code Session Format Has No Stability Contract

**What goes wrong:**
The daemon parses Claude Code's JSONL session logs from `~/.claude/projects/<encoded-dir>/*.jsonl` to determine which code was AI-assisted. The log format includes fields like `parentUuid`, `sessionId`, `version`, `type`, `message` (with `role`, `content` arrays of mixed types including `text`, `tool_use`, `tool_result`), `uuid`, and `timestamp`. However, this format has no published schema, no versioning contract, and no backward compatibility guarantee. Anthropic ships Claude Code updates frequently (monthly feature releases throughout 2025-2026), and any update could change the log structure, add fields, rename fields, or change how sessions are delimited.

**Why it happens:**
These JSONL logs are internal diagnostic/session data, not a public API. Anthropic optimizes for Claude Code functionality, not for third-party consumers of their log files. The `version` field exists but there is no documentation on what schema changes correspond to which version numbers. Community-built parsers (claude-code-log, claude-JSONL-browser, DuckDB analysis approaches) all reverse-engineer the format and break when it changes.

**How to avoid:**
- Build a dedicated session log parser module with its own abstraction layer -- never let raw JSONL field names leak into the rest of the codebase
- Use defensive parsing: treat every field as optional, provide defaults, log warnings (not errors) for unexpected structures
- Implement a version detection heuristic: check the `version` field and the presence/absence of known fields to select the appropriate parsing strategy
- Store the raw JSONL alongside your parsed representation so you can re-parse when you update the parser
- Monitor Claude Code's changelog (https://claudefa.st/blog/guide/changelog and release notes) for format changes
- Consider Claude Code's hooks system as an alternative or supplement to log parsing -- hooks fire on pre/post events and may provide a more stable interface
- Write parser tests against real sample JSONL files from multiple Claude Code versions
- Be aware: Claude Code auto-deletes logs after 30 days by default (configurable via `cleanupPeriodDays` in `~/.claude/settings.json`)

**Warning signs:**
- Parser throwing exceptions after a Claude Code update
- Sessions showing 0 tool_use entries when they clearly involved tool use
- Fields you depend on suddenly being null/undefined
- Session boundaries being incorrectly detected (merging separate sessions or splitting one session)

**Phase to address:** Phase 2 (Claude Code Integration) -- but design the abstraction layer in Phase 1 data model. This needs ongoing maintenance as a "living" component, not a build-once module.

**Confidence:** LOW-MEDIUM -- the log structure is reverse-engineered from community tools and blog posts, not from official documentation. The instability risk is HIGH confidence based on the rapid release cadence.

---

### Pitfall 5: Daemon Resource Leak Under Long-Running Operation

**What goes wrong:**
A daemon that runs continuously for days/weeks accumulates resource leaks that are invisible in short test runs: file handles from watchers that are set up but never torn down, growing in-memory buffers from event queues that are never flushed, SQLite connections that are opened per-operation but never closed, and unbounded caches that grow with repository size. After 3-7 days of continuous operation, the daemon consumes 500MB+ RAM or hits the OS file descriptor limit (default 256 on macOS, 1024 on many Linux distros).

**Why it happens:**
Developers test by starting the daemon, making a few changes, checking the output, and stopping it. This cycle takes minutes, never hours. Resource leaks that grow at 1MB/hour are invisible in testing but catastrophic over a week. Node.js garbage collection handles most memory, but external resources (file handles, watcher subscriptions, database connections) require explicit cleanup. Additionally, chokidar watchers on Linux consume inotify watches (540 bytes kernel memory each on 32-bit, 1080 bytes on 64-bit), and the default `max_user_watches` limit is only 8192.

**How to avoid:**
- Implement a health check that logs memory usage, file handle count, watcher count, and database connection count every 5 minutes
- Set hard memory limits: if RSS exceeds threshold (e.g., 150MB), log a warning; if it exceeds 2x threshold, gracefully restart
- Use connection pooling for SQLite (single connection in WAL mode is fine for a single-process daemon)
- Implement watcher lifecycle management: when a project is closed/inactive, tear down its watchers
- Run soak tests: start the daemon, simulate 8 hours of development activity, verify resource metrics stay flat
- On Linux: check and document required inotify limits (`fs.inotify.max_user_watches`); on macOS: check file descriptor limits (`ulimit -n`)
- Use `WeakRef` / `FinalizationRegistry` for caches where appropriate
- Implement graceful restart: save state, shut down cleanly, restart fresh, restore state

**Warning signs:**
- Memory usage graph that trends upward over hours (even slowly)
- "EMFILE: too many open files" errors after days of operation
- Daemon becoming unresponsive without crashing
- OS-level warnings about inotify watch limits

**Phase to address:** Phase 1 (Daemon Foundation) -- resource management must be baked into the architecture from the start. Retrofitting resource limits onto a working daemon is much harder than designing them in.

**Confidence:** HIGH -- well-established patterns in long-running Node.js services; inotify/FSEvents limits documented in OS documentation.

---

### Pitfall 6: Race Condition Between File Watcher and Git Operations

**What goes wrong:**
The daemon watches files and also monitors git operations. When a user runs `git checkout`, `git stash pop`, or `git merge`, dozens or hundreds of files change simultaneously. The file watcher fires events for each changed file, and the daemon tries to attribute these changes. But these are not "authored" changes -- they are git operations moving existing code around. If the daemon does not distinguish between "user edited a file" and "git changed files as part of an operation," it will misattribute git-operational changes as new authorship events.

**Why it happens:**
File watchers operate at the filesystem level and have no concept of "why" a file changed. A `git checkout other-branch` looks identical to a user simultaneously editing 50 files. The daemon sees the same filesystem events either way.

**How to avoid:**
- Monitor `.git/HEAD`, `.git/index`, and `.git/refs/` for changes -- when these change, a git operation is likely in progress
- Implement a "git operation lock": when a git operation is detected, pause attribution recording for a configurable window (2-5 seconds after the last `.git/` change)
- After the git operation completes, reconcile file states using `git diff` rather than treating individual file events as authored changes
- Use `git status --porcelain` after the operation window closes to determine the actual working tree state
- Ignore file events that happen in bulk (> 10 files within 500ms) as likely git operations, then verify with git state
- Consider using git hooks (`post-checkout`, `post-merge`, `post-rewrite`) as explicit signals instead of inferring from filesystem events

**Warning signs:**
- Attribution data showing a user "wrote" hundreds of files in a one-second burst
- Branch switches appearing as large authorship events
- `git stash pop` generating attribution entries
- Merge conflicts being attributed as authored code

**Phase to address:** Phase 2 (Git Integration) -- but the file watcher (Phase 1) must be designed to support pausing/filtering, so the git operation lock can be layered on.

**Confidence:** HIGH -- this is a straightforward consequence of filesystem-level watching that anyone building this type of tool will encounter.

---

## Technical Debt Patterns

Shortcuts that seem reasonable but create long-term problems.

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Parsing git log output with regex instead of a proper library | Quick implementation, no dependencies | Breaks on commit messages with special characters, multiline messages, non-ASCII authors | Never -- use `simple-git` or spawn `git log --format` with structured output |
| Storing all data in flat JSON files instead of SQLite | No database dependency, easy to inspect | O(n) queries, file corruption on crash, no concurrent access, unbounded file growth | Only for prototyping in first 2 weeks |
| Polling git status on a timer instead of watching `.git/` | Simpler implementation | Misses rapid changes, wastes CPU when nothing changed, adds latency | Only for initial prototype; replace with event-driven in Phase 2 |
| Hardcoding Claude Code JSONL field names throughout codebase | Faster initial development | Every Claude Code update potentially requires changes across entire codebase | Never -- build abstraction layer from day one |
| Using `process.exit()` for crash recovery | Simple restart logic | Loses in-flight data, corrupts partially-written database entries | Never -- implement graceful shutdown with state preservation |
| Ignoring file encoding (assuming UTF-8) | Works for most code | Crashes or produces garbage for UTF-16 (PowerShell scripts), Latin-1 (legacy code), or binary files | Acceptable in MVP if you detect and skip non-UTF-8 gracefully |

## Integration Gotchas

Common mistakes when connecting to external systems and tools.

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| Git CLI | Spawning a new `git` process for every query (git log, git blame, git diff) -- each spawn costs 50-100ms | Batch git queries; use `simple-git` which reuses child processes; cache results with TTL |
| Claude Code JSONL logs | Reading the entire log file on every check -- files can be 100MB+ | Track file position (byte offset); only read new lines appended since last check; use `fs.createReadStream` with `start` option |
| SQLite database | Opening/closing connections per operation | Open a single connection at daemon startup in WAL mode; SQLite allows unlimited concurrent reads and serialized writes, which is perfect for a single-process daemon |
| File system (chokidar) | Watching the entire project root recursively including `node_modules`, `.git`, build output | Use explicit `ignored` patterns; watch only source directories; use `depth` limits |
| VS Code / editor detection | Assuming only one editor is running; not handling editor restarts | Check for editor process periodically; handle the case where no editor is detected gracefully |
| Claude Code session boundaries | Assuming one JSONL file = one session | Sessions span multiple JSONL entries within a file; use `sessionId` field to group; handle concurrent sessions (user has multiple terminals) |

## Performance Traps

Patterns that work at small scale but fail as usage grows.

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Running `git log` on entire repo history for every attribution query | Slow queries, high CPU | Limit log depth (`--max-count`), cache results, use incremental log (since last known commit) | Repos with > 10K commits |
| Storing per-line attribution data without compression | Database grows to gigabytes | Store attribution at hunk/range level, not per-line; use run-length encoding for contiguous same-author blocks | Repos with > 50K LOC |
| Parsing every file change through AST for classification | High CPU, blocks event processing | Classify by file path/extension first (covers 60-70%); only AST-parse when ambiguous; do AST work asynchronously | Projects with > 100 files changing per hour |
| Keeping full file content in memory for diff computation | RAM grows with project size | Use streaming diffs; keep only hashes + metadata in memory; read file content on demand | Projects with > 1000 tracked files |
| Synchronous database writes in the event handler path | Event processing stalls, watcher buffer overflows | Write to an in-memory queue, flush to database asynchronously in batches every 1-2 seconds | Burst of > 50 file events (git checkout, npm install) |
| Re-reading Claude Code JSONL from beginning on each poll | CPU/IO spike proportional to session length | Track last-read byte offset; use `fs.stat` to check if file has grown before reading; seek to offset | Sessions > 30 minutes / files > 10MB |

## Security Mistakes

Domain-specific security issues for a code authorship tool.

| Mistake | Risk | Prevention |
|---------|------|------------|
| Storing code content in attribution database | Sensitive source code persisted in a second location outside version control; potential IP exposure | Store only metadata: file path, line ranges, timestamps, hashes -- never store actual code content |
| Logging full Claude Code prompts/responses | Prompts may contain secrets, API keys, or proprietary business logic that the developer pasted into Claude | Log only metadata: message type, token counts, tool names -- never log message content |
| Database file with world-readable permissions | Other users/processes on the machine can read attribution data | Set database file permissions to `0600` (owner read/write only) at creation time |
| Daemon socket/port without authentication | Other processes can query or manipulate attribution data | Use Unix domain sockets with filesystem permissions rather than TCP ports; if TCP is needed, bind to localhost only and require a token |
| Including file paths in error messages sent to any external service | File paths reveal project structure, internal naming conventions | All error reporting must be local-only; if external error reporting is ever added, scrub all file paths |
| Not encrypting the SQLite database | Physical access to machine exposes all attribution data | For MVP, rely on OS-level disk encryption (FileVault/LUKS); document this as a requirement; consider SQLCipher for sensitive environments |

## UX Pitfalls

Common user experience mistakes in developer tooling.

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Showing attribution confidence as precise percentages ("87.3% human-authored") | False precision breeds false trust; users argue about 2% differences | Use qualitative labels: "Human", "AI-Assisted", "Uncertain" with clear definitions |
| Notifications on every attribution event | Alert fatigue within minutes; users disable the tool | Silent by default; provide dashboard/query interface; notify only on explicit anomalies |
| Requiring manual setup for each project | Friction prevents adoption; users forget to enable it | Auto-detect projects by presence of `.git/`; zero-config startup with sensible defaults |
| CLI-only interface with no visualization | Attribution data is inherently visual (who wrote what, over time); raw data is not actionable | Provide at minimum a terminal-based summary table; plan for a web dashboard |
| Blocking editor/git operations while daemon processes | Developer flow interrupted; daemon gets disabled | All processing must be fully asynchronous; daemon must never hold locks that affect the editor or git |
| Showing stale data without indicating staleness | Users make decisions based on outdated attribution | Always show "last updated" timestamp; indicate when daemon is behind/catching up |

## "Looks Done But Isn't" Checklist

Things that appear complete but are missing critical pieces.

- [ ] **File watcher:** Works for single-file saves but not tested with git checkout (50+ files changing simultaneously), editor "save all," or npm install (thousands of files in node_modules)
- [ ] **Git integration:** Works with merge commits but not tested with squash merges, rebase, cherry-pick, or force push
- [ ] **Claude Code parsing:** Works with current version but no abstraction layer for version changes; no test fixtures from multiple Claude Code versions
- [ ] **Attribution accuracy:** Works for clean cases (pure human, pure AI) but not tested with mixed sessions (human starts, AI modifies, human refines)
- [ ] **Daemon lifecycle:** Starts and runs but no graceful shutdown; no crash recovery; no state preservation across restarts
- [ ] **Cross-platform:** Works on developer's macOS but not tested on Linux (different filesystem events, inotify limits, path separators)
- [ ] **Large repos:** Works on 100-file test project but not tested on 10K+ file production repos
- [ ] **Concurrent sessions:** Works with one terminal running Claude Code but not tested with multiple concurrent Claude Code sessions on the same project
- [ ] **Database integrity:** Works during normal operation but no WAL mode checkpoint strategy; no handling of unexpected process termination mid-write
- [ ] **Session log rotation:** Works with current session logs but does not handle Claude Code's 30-day auto-deletion (`cleanupPeriodDays` setting)

## Recovery Strategies

When pitfalls occur despite prevention, how to recover.

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Event storm causing CPU spikes | LOW | Add debouncing layer; can be done without data model changes |
| Squash merge destroying attribution | HIGH | If daemon-captured data exists, re-derive from it; if not, attribution for squash-merged code is permanently lost |
| Work-type misclassification | MEDIUM | Allow user corrections; retrain/adjust heuristics; re-classify historical data with updated rules |
| Claude Code format change breaking parser | MEDIUM | Update parser module; re-parse stored raw JSONL; no data loss if raw logs were preserved |
| Resource leak after days of running | LOW | Implement health monitoring + auto-restart; no data loss if database writes are committed |
| Git operation misattribution | MEDIUM | Retroactively filter attribution entries that correlate with detected git operations; requires git operation timestamps |
| Database corruption from crash | HIGH if no backups | Implement WAL mode + periodic checkpoints from day one; keep last-known-good backup; rebuild from git + JSONL logs as last resort |

## Pitfall-to-Phase Mapping

How roadmap phases should address these pitfalls.

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| Event storms / CPU spikes | Phase 1: File Watcher Foundation | Soak test: 8 hours of simulated development with CPU < 5% baseline |
| Resource leaks in long-running daemon | Phase 1: Daemon Foundation | Memory and handle count remain flat over 24-hour soak test |
| Git operation misattribution | Phase 1-2: Watcher + Git Integration | Test suite includes git checkout, merge, rebase, stash scenarios |
| Squash merge author destruction | Phase 2: Data Model Design | Attribution survives squash merge in test suite; daemon data is primary source |
| Claude Code format instability | Phase 2: Claude Code Integration | Parser handles at least 3 different JSONL format variants in test fixtures |
| Concurrent session confusion | Phase 2: Claude Code Integration | Test with 2+ simultaneous Claude Code terminals on same project |
| Work-type misclassification erosion of trust | Phase 3+: Classification Engine | User study: > 80% agreement with classifications on sample data |
| Database corruption on crash | Phase 1: Data Storage | Kill -9 the daemon during writes; verify database recovers cleanly |
| Cross-platform filesystem differences | Phase 1: File Watcher Foundation | CI runs on both macOS and Linux; test inotify limit handling |
| Performance degradation on large repos | Phase 2-3: Optimization | Benchmark suite with 10K+ file synthetic repo |

## Sources

- [Chokidar GitHub - v4/v5 documentation, known issues, awaitWriteFinish](https://github.com/paulmillr/chokidar)
- [Chokidar race condition when watching dirs (Issue #1112)](https://github.com/paulmillr/chokidar/issues/1112)
- [GitHub Community: Preserve blame in squash merge (Discussion #38240)](https://github.com/orgs/community/discussions/38240)
- [GitHub: Contributors of squashed commits don't get any love (Issue #1303)](https://github.com/isaacs/github/issues/1303)
- [Why git squash merges are bad - Felix Moessbauer](https://felixmoessbauer.com/blog-reader/why-git-squash-merges-are-bad.html)
- [isomorphic-git: log is slow (Issue #446)](https://github.com/isomorphic-git/isomorphic-git/issues/446)
- [Simon Willison: Don't let Claude Code delete your session logs](https://simonwillison.net/2025/Oct/22/claude-code-logs/)
- [Claude Code Log Analysis with DuckDB - Liam ERD](https://liambx.com/blog/claude-code-log-analysis-with-duckdb)
- [claude-code-log GitHub - JSONL parsing tool](https://github.com/daaain/claude-code-log)
- [Claude Code Changelog](https://claudefa.st/blog/guide/changelog)
- [Linux inotify limits - Watchexec documentation](https://watchexec.github.io/docs/inotify-limits.html)
- [SQLite WAL mode documentation](https://sqlite.org/wal.html)
- [SQLite File Locking and Concurrency](https://sqlite.org/lockingv3.html)
- [Node.js fs.watch issues (nodejs/node#47058)](https://github.com/nodejs/node/issues/47058)
- [FileSystemWatcher duplicate events - CodeProject](https://www.codeproject.com/Articles/1220093/A-Robust-Solution-for-FileSystemWatcher-Firing-Eve)
- [ACM Computing Surveys: Code Authorship Attribution Methods and Challenges](https://dl.acm.org/doi/10.1145/3292577)
- [SemEval-2026 Task 13: GenAI Code Detection & Attribution](https://github.com/GiovanniIacuzzo/SemEval-2026-Task-13)
- [AI Code Quality Report - CodeRabbit](https://www.coderabbit.ai/blog/state-of-ai-vs-human-code-generation-report)
- [OpenTelemetry: Handling Sensitive Data](https://opentelemetry.io/docs/security/handling-sensitive-data/)
- [Node.js Process Exit Strategies - Leapcell](https://leapcell.io/blog/nodejs-process-exit-strategies)

---
*Pitfalls research for: Code authorship attribution / AI collaboration intelligence daemon ("Who Wrote It")*
*Researched: 2026-02-09*
