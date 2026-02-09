package store

// schemaVersion is the current schema version. Increment when adding migrations.
const schemaVersion = 2

// migrations maps version numbers to SQL statements that bring the schema
// from (version-1) to (version). Version 1 is the initial schema.
var migrations = map[int]string{
	1: `
-- File system events captured by the watcher.
CREATE TABLE IF NOT EXISTS file_events (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	project_path    TEXT    NOT NULL,
	file_path       TEXT    NOT NULL,
	event_type      TEXT    NOT NULL,
	timestamp       TEXT    NOT NULL,
	checksum        TEXT    NOT NULL DEFAULT '',
	debounce_group  TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_file_events_project ON file_events(project_path);
CREATE INDEX IF NOT EXISTS idx_file_events_timestamp ON file_events(timestamp);

-- Session events from AI coding tools (e.g. Claude Code JSONL).
CREATE TABLE IF NOT EXISTS session_events (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id   TEXT    NOT NULL,
	event_type   TEXT    NOT NULL,
	tool_name    TEXT    NOT NULL DEFAULT '',
	file_path    TEXT    NOT NULL DEFAULT '',
	content_hash TEXT    NOT NULL DEFAULT '',
	timestamp    TEXT    NOT NULL,
	raw_json     TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_session_events_session ON session_events(session_id);
CREATE INDEX IF NOT EXISTS idx_session_events_timestamp ON session_events(timestamp);

-- Git commits with AI co-author detection.
CREATE TABLE IF NOT EXISTS git_commits (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	hash            TEXT    NOT NULL UNIQUE,
	author          TEXT    NOT NULL,
	message         TEXT    NOT NULL DEFAULT '',
	timestamp       TEXT    NOT NULL,
	has_coauthor_tag INTEGER NOT NULL DEFAULT 0,
	coauthor_name   TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_git_commits_hash ON git_commits(hash);
CREATE INDEX IF NOT EXISTS idx_git_commits_timestamp ON git_commits(timestamp);

-- Per-file diff stats for each commit.
CREATE TABLE IF NOT EXISTS git_diffs (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	commit_id   INTEGER NOT NULL REFERENCES git_commits(id) ON DELETE CASCADE,
	file_path   TEXT    NOT NULL,
	old_path    TEXT    NOT NULL DEFAULT '',
	change_type TEXT    NOT NULL,
	additions   INTEGER NOT NULL DEFAULT 0,
	deletions   INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_git_diffs_commit ON git_diffs(commit_id);

-- Line-level blame data for tracked files.
CREATE TABLE IF NOT EXISTS git_blame_lines (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	file_path    TEXT    NOT NULL,
	line_number  INTEGER NOT NULL,
	commit_hash  TEXT    NOT NULL,
	author       TEXT    NOT NULL,
	content_hash TEXT    NOT NULL DEFAULT '',
	last_updated TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_git_blame_file ON git_blame_lines(file_path);
CREATE UNIQUE INDEX IF NOT EXISTS idx_git_blame_file_line ON git_blame_lines(file_path, line_number);

-- Key-value store for daemon metadata (schema version, cursors, etc).
CREATE TABLE IF NOT EXISTS daemon_state (
	key        TEXT PRIMARY KEY,
	value      TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL
);
`,

	2: `
-- Attribution records linking file events to AI session events.
CREATE TABLE IF NOT EXISTS attributions (
	id                   INTEGER PRIMARY KEY AUTOINCREMENT,
	file_path            TEXT    NOT NULL,
	project_path         TEXT    NOT NULL,
	file_event_id        INTEGER REFERENCES file_events(id),
	session_event_id     INTEGER REFERENCES session_events(id),
	authorship_level     TEXT    NOT NULL,
	confidence           REAL    NOT NULL DEFAULT 1.0,
	uncertain            INTEGER NOT NULL DEFAULT 0,
	first_author         TEXT    NOT NULL DEFAULT '',
	correlation_window_ms INTEGER NOT NULL DEFAULT 0,
	timestamp            TEXT    NOT NULL,
	created_at           TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_attributions_file ON attributions(file_path);
CREATE INDEX IF NOT EXISTS idx_attributions_project ON attributions(project_path);
CREATE INDEX IF NOT EXISTS idx_attributions_level ON attributions(authorship_level);
CREATE INDEX IF NOT EXISTS idx_attributions_timestamp ON attributions(timestamp);
`,
}
