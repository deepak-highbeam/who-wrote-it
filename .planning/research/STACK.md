# Stack Research

**Domain:** Code authorship attribution / AI collaboration intelligence daemon
**Researched:** 2026-02-09
**Confidence:** HIGH (core stack), MEDIUM (some version specifics)

---

## Language Decision: Go vs Rust

**Recommendation: Go**

This is the highest-impact decision. Both languages are viable, but Go wins for this specific project.

### Comparison Matrix

| Criterion | Go | Rust | Winner |
|-----------|-----|------|--------|
| Compile speed | Sub-second incremental | Minutes for full, seconds incremental | Go |
| Runtime performance | Excellent (GC pauses ~ms) | Superior (no GC) | Rust |
| Memory footprint idle | ~5-10MB baseline | ~1-3MB baseline | Rust |
| Git library maturity | go-git (pure Go, production-proven) | git2 (libgit2 C bindings) | Go |
| JSON/JSONL parsing | stdlib `encoding/json` + streaming | serde_json (excellent but verbose) | Tie |
| GitHub API client | go-github (Google-maintained, v82+) | octocrab (community, v0.49) | Go |
| File system watching | fsnotify (mature, v1.9) | notify (mature, v8.2/v9.0-rc) | Tie |
| Embedded DB options | SQLite, bbolt, badger (all mature) | redb, rusqlite, sled (redb stable, sled unstable) | Go |
| Concurrency model | Goroutines (trivially easy) | async/tokio (powerful but complex) | Go |
| Cross-compilation | Trivial (GOOS/GOARCH) | Requires toolchain setup | Go |
| Developer velocity | Fast iteration, readable code | Slower iteration, borrow checker friction | Go |
| Ecosystem for this domain | Stronger (CLI tools, daemons) | Stronger for systems/perf-critical | Go |
| Binary distribution | Single binary, no deps | Single binary, no deps | Tie |

### Why Go Wins for "Who Wrote It"

1. **This is an I/O-bound daemon, not a CPU-bound one.** The tool watches files, reads git data, parses JSON, and makes API calls. None of these are CPU-intensive. Go's goroutine model handles concurrent I/O elegantly with minimal code.

2. **go-git is pure Go with no C dependencies.** Rust's git2 crate wraps libgit2 (a C library), requiring C toolchain for compilation. go-git is a complete Git implementation in pure Go -- no CGO, no linker headaches, single binary output.

3. **Google maintains go-github.** The GitHub API client is official, versioned with semver, and tracks the GitHub API closely (v82.0.0 as of Jan 2025). Rust's octocrab is at v0.49.5 -- pre-1.0 and community-maintained.

4. **Go 1.26's Green Tea GC reduces overhead by 10-40%.** The new garbage collector (enabled by default in Go 1.26, releasing Feb 2026) specifically targets the concern about GC pauses in long-running daemons. Memory overhead for idle daemons is now negligible.

5. **Developer velocity matters for a greenfield project.** Go compiles in under a second. Rust's borrow checker and compile times create friction during rapid iteration. For a daemon where correctness matters more than nanosecond performance, Go's simplicity is a feature.

### When You Would Choose Rust Instead

- If the daemon needed to process millions of files per second (it does not)
- If memory must stay under 2MB (Go 1.26 with GOGC tuning can approach this)
- If you needed zero-copy parsing of massive git histories (Go is fast enough here)
- If the team had deep Rust expertise and wanted to optimize for long-term maintenance

**Confidence: HIGH** -- Based on multiple 2026 comparisons, library ecosystem analysis, and the specific requirements of this project (I/O-bound, daemon, JSON/git/API integration). Go is the clear fit.

---

## Recommended Stack

### Go Version

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go | 1.26 (Feb 2026) | Language runtime | Green Tea GC (10-40% less GC overhead), goroutine leak profiles, faster cgo calls. Falls back gracefully to 1.24/1.25 if 1.26 is not yet stable at project start. |

**Note:** Go 1.26 is expected to release February 2026 (the release notes are still marked "work in progress"). Go 1.24 (released Feb 2025, supported until May 2026) is a safe fallback with excellent performance.

**Confidence: HIGH** -- Verified via [Go 1.26 release notes](https://go.dev/doc/go1.26) and [Go release history](https://go.dev/doc/devel/release).

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| fsnotify | v1.9.0 | File system watching | The standard Go FS watcher. Cross-platform (Linux inotify, macOS kqueue, Windows ReadDirectoryChanges). Used by Docker, Kubernetes, Hugo. Last release Apr 2024, actively maintained. |
| go-git | v5.16.5 | Git repository access (blame, log, diff) | Pure Go git implementation. No CGO required. Supports blame, log, diff, commit traversal. Used by Gitea, Pulumi, Keybase. v5 is stable; v6 is in alpha (no tagged release yet). |
| go-github | v68+ | GitHub API (PR comments, reviews) | Google-maintained. Tracks GitHub REST API v3 closely. Semver releases. Active development (v82.0.0 released Jan 2025). |
| modernc.org/sqlite | latest (supports SQLite 3.51.2) | Local data store | Pure Go SQLite -- no CGO required. Single-binary deployment. WAL mode for concurrent reads. 2x slower than C SQLite at worst, which is irrelevant for this use case. Imported by 2,500+ projects. |
| cobra | v1.10.2 | CLI framework | The standard Go CLI framework. Used by kubectl, docker, hugo, gh. Subcommands, flags, help generation. |

**Confidence: HIGH for all** -- Versions verified via GitHub releases and pkg.go.dev.

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| go-tree-sitter | latest | Multi-language code parsing | When analyzing code structure beyond line-level attribution. Parse any language's AST to classify code as "boilerplate", "logic", "tests", etc. Official tree-sitter org maintains Go bindings. |
| encoding/json (stdlib) | Go stdlib | JSON/JSONL parsing | Parsing Claude Code session files. Stdlib is sufficient -- use `json.Decoder` for streaming JSONL line-by-line without loading entire file into memory. |
| bufio (stdlib) | Go stdlib | Line-by-line file reading | JSONL streaming. `bufio.Scanner` with `json.Unmarshal` per line is the standard Go pattern for JSONL. |
| filepath (stdlib) | Go stdlib | Recursive directory walking | Augment fsnotify with `filepath.WalkDir` for initial directory scan and adding watches recursively. |
| oauth2 | latest | GitHub authentication | Standard Go OAuth2 library. Use with go-github for authenticated API calls. |
| slog (stdlib) | Go stdlib (1.21+) | Structured logging | Built into Go since 1.21. Structured, leveled logging without external deps. Use for daemon operational logs. |
| viper | v1.19+ | Configuration management | Reads config from files, env vars, flags. Pairs with cobra for CLI config. Handles YAML/TOML/JSON config files. |
| testify | v1.9+ | Testing assertions | Standard Go testing assertions. `assert` and `require` packages for readable tests. |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| golangci-lint | Linting | Runs 50+ linters. Use default config plus `errcheck`, `govet`, `staticcheck`. |
| goreleaser | Binary distribution | Cross-compile and release for macOS/Linux/Windows. Single binary per platform. |
| go test -race | Race condition detection | Always run tests with `-race` flag. Essential for concurrent daemon code. |
| dlv (delve) | Debugging | Go-native debugger. Critical for debugging goroutine-heavy daemon code. |
| go tool pprof | Profiling | Built-in CPU and memory profiling. Use to verify "negligible footprint" requirement. |

---

## Installation

```bash
# Initialize module
go mod init github.com/deepakbhimaraju/who-wrote-it

# Core dependencies
go get github.com/fsnotify/fsnotify@v1.9.0
go get github.com/go-git/go-git/v5@v5.16.5
go get github.com/google/go-github/v68
go get modernc.org/sqlite
go get github.com/spf13/cobra@v1.10.2
go get github.com/spf13/viper

# Code analysis (add when needed)
go get github.com/tree-sitter/go-tree-sitter

# Testing
go get github.com/stretchr/testify@v1.9.0

# Dev tools
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/goreleaser/goreleaser/v2@latest
```

---

## Alternatives Considered

| Category | Recommended | Alternative | When to Use Alternative |
|----------|-------------|-------------|------------------------|
| Language | Go | Rust | Only if sub-millisecond latency and <2MB memory are hard requirements, or team has deep Rust experience |
| FS Watching | fsnotify v1.9 | rfsnotify (recursive wrapper) | Only if fsnotify + filepath.WalkDir proves too cumbersome for recursive watching. rfsnotify adds AddRecursive/RemoveRecursive methods but is less maintained. |
| Git Access | go-git v5 | go-git v6 | When v6 reaches a stable tagged release. Currently alpha -- no tagged version, API still changing. Monitor https://github.com/go-git/go-git/releases |
| Git Access | go-git v5 | git2go (libgit2 bindings) | Only if you need advanced blame options (track copies across files, etc.) that go-git doesn't support. Requires CGO and libgit2 C library. |
| Git Access | go-git v5 | exec `git` CLI | Fallback for any git operation go-git doesn't support. Shell out to `git blame --porcelain` and parse output. Simple but creates process overhead. |
| Database | modernc.org/sqlite | mattn/go-sqlite3 | Only if benchmarks show SQLite performance is a bottleneck. mattn uses CGO (faster) but prevents pure Go binary and complicates cross-compilation. |
| Database | modernc.org/sqlite | bbolt v1.4.3 | If you only need key-value storage without SQL queries. bbolt is simpler but lacks relational queries that attribution data will need (joins, aggregations, time-range queries). |
| Database | modernc.org/sqlite | badger v4.9.1 | If you need LSM-tree performance for high write throughput. Overkill for this use case -- we're writing attribution records, not a transaction log. |
| CLI | cobra v1.10 | urfave/cli v2 | Personal preference. cobra is the de facto standard. urfave/cli is lighter but less feature-rich. |
| Code Parsing | tree-sitter | Go stdlib go/ast | Only for Go source files. go/ast only parses Go code. tree-sitter parses 40+ languages, which is essential for a tool that analyzes any codebase. |
| Config | viper | koanf | If viper's dependency tree feels heavy. koanf is lighter but less ecosystem support. |
| GitHub API | go-github | go-github v82+ | Pin to a specific major version (v68+) to avoid churn. go-github releases major versions frequently; each bumps the import path. Pick one and stay on it unless you need newer API endpoints. |

---

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| go-git v6 | Not yet stable. No tagged release. API is still changing. "This documentation is subject to change until v6 is officially released." | go-git v5.16.5 (stable, production-proven) |
| sled (Rust) | Self-described as "the champagne of beta embedded databases." Unstable on-disk format. Not forward-compatible between versions. | If using Rust: redb v3.1.0 (stable file format, better benchmarks) |
| mattn/go-sqlite3 | Requires CGO. Breaks single-binary deployment. Complicates cross-compilation. Performance difference irrelevant for this workload. | modernc.org/sqlite (pure Go, no CGO) |
| Polling-based file watchers (go-fswatch) | CPU-intensive. Wastes resources for an always-on daemon. Violates "negligible footprint" requirement. | fsnotify (OS-native events via inotify/kqueue) |
| BoltDB (original) | Archived/unmaintained since 2018. | bbolt v1.4.3 (maintained fork by etcd team) if you need KV store |
| go/ast for multi-language parsing | Only parses Go source code. "Who Wrote It" must analyze any language. | tree-sitter with language-specific grammars |
| External databases (PostgreSQL, MySQL) | Violates "all data stays local" constraint. Adds deployment complexity. Daemon should be zero-config. | SQLite (embedded, zero-config, file-based) |
| inotify directly (syscall) | Platform-specific. macOS uses kqueue, Windows uses ReadDirectoryChanges. | fsnotify (cross-platform abstraction) |

---

## Stack Patterns by Variant

**If targeting macOS-only initially (likely for developer tool):**
- fsnotify uses kqueue on macOS, which is efficient and reliable
- Consider fsevents (github.com/fsnotify/fsevents) for even deeper macOS integration if needed later
- SQLite WAL mode works perfectly on macOS local filesystem

**If supporting Linux servers (CI/CD integration later):**
- fsnotify uses inotify on Linux with configurable watch limits (`/proc/sys/fs/inotify/max_user_watches`)
- Default inotify limit (8192) may be too low for large repos -- daemon should detect and warn
- SQLite WAL mode requires local filesystem (not NFS) -- fine for developer machines

**If the Claude Code session format changes:**
- Design JSONL parser as an interface (`SessionParser`) so format changes only require a new implementation
- Claude Code session files live at `~/.claude/projects/{encoded-path}/{session-id}.jsonl`
- Key fields: `type` (user/assistant/summary), `message.role`, `message.content`, `timestamp`, `uuid`, `sessionId`, `parentUuid`, `cwd`, `version`
- Assistant messages include: `model`, usage stats (`input_tokens`, `output_tokens`, `cache_read_input_tokens`), `stop_reason`
- Parse with streaming `bufio.Scanner` + `json.Unmarshal` per line -- never load full file

---

## Version Compatibility

| Package | Compatible With | Notes |
|---------|-----------------|-------|
| go-git v5.16.5 | Go 1.23-1.26 | Tests run against Go 1.23-1.25. Should work with 1.26. |
| fsnotify v1.9.0 | Go 1.17+ | Broad compatibility. No issues expected. |
| modernc.org/sqlite | Go 1.21+ | Published Jan 2026 with SQLite 3.51.2. Requires Go 1.21 for generics. |
| cobra v1.10.2 | Go 1.16+ | Very broad compatibility. |
| go-github v68+ | Go 1.21+ | Each major version bumps min Go version. v68 requires 1.21+. |
| tree-sitter Go | Go 1.22+ | Uses CGO for tree-sitter C core. This is the one dependency that uses CGO. |

**CGO Note:** The stack is CGO-free except for tree-sitter. If CGO is unacceptable, defer tree-sitter integration and use simpler heuristics (regex patterns, line counting) for code classification in MVP. Tree-sitter can be added in a later phase when code structure analysis becomes important.

---

## Claude Code Session File Schema Reference

This is critical domain knowledge for the project. The daemon must parse these files.

**Location:** `~/.claude/projects/{encoded-project-path}/{session-uuid}.jsonl`

**Format:** JSONL (one JSON object per line)

**Message Types:**

```
user     -- Human input (prompts, instructions)
assistant -- Claude responses (code, explanations, tool calls)
summary  -- Session summaries (condensed conversation)
```

**Key Fields Per Line:**

```json
{
  "type": "assistant",
  "message": {
    "role": "assistant",
    "content": [{"type": "text", "text": "..."}],
    "model": "claude-opus-4-20250514",
    "stop_reason": "end_turn",
    "usage": {
      "input_tokens": 1234,
      "output_tokens": 567,
      "cache_creation_input_tokens": 0,
      "cache_read_input_tokens": 890
    }
  },
  "timestamp": "2026-01-05T10:00:00Z",
  "uuid": "abc-123",
  "sessionId": "session-456",
  "parentUuid": "prev-789",
  "cwd": "/path/to/project",
  "version": "2.1.31",
  "isSidechain": false
}
```

**Confidence: MEDIUM** -- Verified via multiple community tools (claude-JSONL-browser, claude-code-log, DuckDB analysis blog) but not from official Anthropic documentation. The schema is inferred from real files and may evolve.

---

## Sources

- [Go 1.26 Release Notes](https://go.dev/doc/go1.26) -- Verified Green Tea GC, goroutine leak profiles (HIGH confidence)
- [Go 1.24 Release Notes](https://go.dev/doc/go1.24) -- Verified features, support timeline (HIGH confidence)
- [fsnotify GitHub releases](https://github.com/fsnotify/fsnotify/releases) -- v1.9.0, Apr 2024 (HIGH confidence)
- [go-git pkg.go.dev](https://pkg.go.dev/github.com/go-git/go-git/v5) -- v5.16.5, Feb 2025 (HIGH confidence)
- [go-git v6 pkg.go.dev](https://pkg.go.dev/github.com/go-git/go-git/v6) -- Pre-release, no tagged version (HIGH confidence)
- [go-git v5 to v6 migration](https://go-git.github.io/docs/tutorials/migrating-from-v5-to-v6/) -- API changes documented (HIGH confidence)
- [go-github GitHub releases](https://github.com/google/go-github/releases) -- v82.0.0, Jan 2025 (HIGH confidence)
- [modernc.org/sqlite pkg.go.dev](https://pkg.go.dev/modernc.org/sqlite) -- Published Jan 2026, SQLite 3.51.2 (HIGH confidence)
- [cobra GitHub releases](https://github.com/spf13/cobra) -- v1.10.2, Dec 2024 (HIGH confidence)
- [notify-rs GitHub releases](https://github.com/notify-rs/notify/releases) -- v8.2.0 stable, v9.0.0-rc.1 (HIGH confidence)
- [git2-rs GitHub](https://github.com/rust-lang/git2-rs) -- libgit2 bindings, no GitHub releases (uses crates.io) (MEDIUM confidence)
- [octocrab GitHub releases](https://github.com/XAMPPRocky/octocrab/releases) -- v0.49.5 (HIGH confidence)
- [redb GitHub releases](https://github.com/cberner/redb/releases) -- v3.1.0 (HIGH confidence)
- [bbolt GitHub releases](https://github.com/etcd-io/bbolt/releases) -- v1.4.3, Aug 2025 (HIGH confidence)
- [badger pkg.go.dev](https://pkg.go.dev/github.com/dgraph-io/badger/v4) -- v4.9.1, Feb 2025 (HIGH confidence)
- [tree-sitter Go bindings](https://github.com/tree-sitter/go-tree-sitter) -- Official org, active (MEDIUM confidence)
- [Claude Code JSONL format](https://liambx.com/blog/claude-code-log-analysis-with-duckdb) -- Community analysis of session files (MEDIUM confidence)
- [Claude Code local storage design](https://milvus.io/blog/why-claude-code-feels-so-stable-a-developers-deep-dive-into-its-local-storage-design.md) -- Storage architecture (MEDIUM confidence)
- [claude-JSONL-browser](https://github.com/withLinda/claude-JSONL-browser) -- JSONL schema reference (MEDIUM confidence)
- [Bitfield Consulting: Rust vs Go 2026](https://bitfieldconsulting.com/posts/rust-vs-go) -- Language comparison (MEDIUM confidence)
- [SQLite WAL documentation](https://sqlite.org/wal.html) -- Concurrent read/write model (HIGH confidence)
- [Go SQLite benchmarks](https://github.com/cvilsmeier/go-sqlite-bench) -- modernc vs mattn performance (MEDIUM confidence)
- [Akita: Taming Go Memory](https://www.akitasoftware.com/blog-posts/taming-gos-memory-usage-or-how-we-avoided-rewriting-our-client-in-rust) -- Go daemon memory optimization (MEDIUM confidence)

---
*Stack research for: Code authorship attribution daemon ("Who Wrote It")*
*Researched: 2026-02-09*
