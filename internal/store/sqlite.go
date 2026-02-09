package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver.
)

// Store wraps a SQLite database connection for the daemon.
type Store struct {
	db *sql.DB
}

// New opens (or creates) the SQLite database at dbPath with WAL mode
// and a 5-second busy timeout, then runs any pending migrations.
func New(dbPath string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)", dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Verify connection and WAL mode.
	var journalMode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("check journal mode: %w", err)
	}
	if journalMode != "wal" {
		_ = db.Close()
		return nil, fmt.Errorf("expected WAL journal mode, got %q", journalMode)
	}

	if err := runMigrations(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB returns the underlying *sql.DB for direct queries.
// Use sparingly; prefer adding methods to Store.
func (s *Store) DB() *sql.DB {
	return s.db
}

// FileEventsCount returns the number of file events recorded.
func (s *Store) FileEventsCount() (int64, error) {
	var count int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM file_events").Scan(&count)
	return count, err
}

// SessionEventsCount returns the number of session events recorded.
func (s *Store) SessionEventsCount() (int64, error) {
	var count int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM session_events").Scan(&count)
	return count, err
}

// GitCommitsCount returns the number of git commits recorded.
func (s *Store) GitCommitsCount() (int64, error) {
	var count int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM git_commits").Scan(&count)
	return count, err
}

// InsertFileEvent records a file system event in the store.
func (s *Store) InsertFileEvent(projectPath, filePath, eventType string, timestamp time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO file_events (project_path, file_path, event_type, timestamp)
		 VALUES (?, ?, ?, ?)`,
		projectPath, filePath, eventType, timestamp.UTC().Format(time.RFC3339Nano),
	)
	return err
}

// InsertSessionEvent records an AI tool session event in the store.
func (s *Store) InsertSessionEvent(sessionID, eventType, toolName, filePath, contentHash string, timestamp time.Time, rawJSON string) error {
	_, err := s.db.Exec(
		`INSERT INTO session_events (session_id, event_type, tool_name, file_path, content_hash, timestamp, raw_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sessionID, eventType, toolName, filePath, contentHash,
		timestamp.UTC().Format(time.RFC3339Nano), rawJSON,
	)
	return err
}

// GetDaemonState reads a value from the daemon_state key-value table.
// Returns empty string and nil error if the key does not exist.
func (s *Store) GetDaemonState(key string) (string, error) {
	var val string
	err := s.db.QueryRow(`SELECT value FROM daemon_state WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

// SetDaemonState upserts a value in the daemon_state key-value table.
func (s *Store) SetDaemonState(key, value string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO daemon_state (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, now,
	)
	return err
}

// InsertGitCommit records a git commit in the store and returns the row id.
func (s *Store) InsertGitCommit(hash, author, message string, timestamp time.Time, hasCoauthorTag bool, coauthorName string) (int64, error) {
	coauthor := 0
	if hasCoauthorTag {
		coauthor = 1
	}
	result, err := s.db.Exec(
		`INSERT INTO git_commits (hash, author, message, timestamp, has_coauthor_tag, coauthor_name)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(hash) DO NOTHING`,
		hash, author, message, timestamp.UTC().Format(time.RFC3339), coauthor, coauthorName,
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	// If ON CONFLICT fired, LastInsertId returns 0. Look up the existing row.
	if id == 0 {
		err = s.db.QueryRow(`SELECT id FROM git_commits WHERE hash = ?`, hash).Scan(&id)
	}
	return id, err
}

// InsertGitDiff records a per-file diff stat for a commit.
func (s *Store) InsertGitDiff(commitID int64, filePath, oldPath, changeType string, additions, deletions int) error {
	_, err := s.db.Exec(
		`INSERT INTO git_diffs (commit_id, file_path, old_path, change_type, additions, deletions)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		commitID, filePath, oldPath, changeType, additions, deletions,
	)
	return err
}

// InsertBlameLines batch-inserts blame data for a file, replacing existing data.
func (s *Store) InsertBlameLines(filePath string, lines []BlameLine) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	// Remove old blame data for this file.
	if _, err := tx.Exec(`DELETE FROM git_blame_lines WHERE file_path = ?`, filePath); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	stmt, err := tx.Prepare(
		`INSERT INTO git_blame_lines (file_path, line_number, commit_hash, author, content_hash, last_updated)
		 VALUES (?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, l := range lines {
		if _, err := stmt.Exec(filePath, l.LineNumber, l.CommitHash, l.Author, l.ContentHash, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// BlameLine represents a single line of git blame output.
type BlameLine struct {
	LineNumber  int
	CommitHash  string
	Author      string
	ContentHash string
}

// DBSizeBytes returns the database file size in bytes.
// This is an approximation using page_count * page_size.
func (s *Store) DBSizeBytes() (int64, error) {
	var pageCount, pageSize int64
	if err := s.db.QueryRow("PRAGMA page_count").Scan(&pageCount); err != nil {
		return 0, err
	}
	if err := s.db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		return 0, err
	}
	return pageCount * pageSize, nil
}

// ---------------------------------------------------------------------------
// Types for correlation and attribution queries
// ---------------------------------------------------------------------------

// FileEvent represents a file system event row from the store.
type FileEvent struct {
	ID          int64
	ProjectPath string
	FilePath    string
	EventType   string
	Timestamp   time.Time
}

// StoredSessionEvent represents a session event row from the store.
type StoredSessionEvent struct {
	ID          int64
	SessionID   string
	EventType   string
	ToolName    string
	FilePath    string
	ContentHash string
	Timestamp   time.Time
}

// AttributionRecord represents a row in the attributions table.
type AttributionRecord struct {
	ID                  int64
	FilePath            string
	ProjectPath         string
	FileEventID         *int64
	SessionEventID      *int64
	AuthorshipLevel     string
	Confidence          float64
	Uncertain           bool
	FirstAuthor         string
	CorrelationWindowMs int
	Timestamp           time.Time
}

// ---------------------------------------------------------------------------
// Query methods for correlation engine
// ---------------------------------------------------------------------------

// QueryFileEventsInWindow returns file events for a given file path within
// a time window [start, end], ordered by timestamp ascending.
func (s *Store) QueryFileEventsInWindow(filePath string, start, end time.Time) ([]FileEvent, error) {
	rows, err := s.db.Query(
		`SELECT id, project_path, file_path, event_type, timestamp
		 FROM file_events
		 WHERE file_path = ? AND timestamp >= ? AND timestamp <= ?
		 ORDER BY timestamp ASC`,
		filePath,
		start.UTC().Format(time.RFC3339Nano),
		end.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanFileEvents(rows)
}

// QueryFileEventsByProject returns all file events for a project since the
// given time, ordered by timestamp ascending.
func (s *Store) QueryFileEventsByProject(projectPath string, since time.Time) ([]FileEvent, error) {
	rows, err := s.db.Query(
		`SELECT id, project_path, file_path, event_type, timestamp
		 FROM file_events
		 WHERE project_path = ? AND timestamp >= ?
		 ORDER BY timestamp ASC`,
		projectPath,
		since.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanFileEvents(rows)
}

// QuerySessionEventsInWindow returns session events for a given file path
// within a time window [start, end], filtered to Write tool_name only,
// ordered by timestamp ascending.
func (s *Store) QuerySessionEventsInWindow(filePath string, start, end time.Time) ([]StoredSessionEvent, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, event_type, tool_name, file_path, content_hash, timestamp
		 FROM session_events
		 WHERE file_path = ? AND tool_name = 'Write' AND timestamp >= ? AND timestamp <= ?
		 ORDER BY timestamp ASC`,
		filePath,
		start.UTC().Format(time.RFC3339Nano),
		end.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSessionEvents(rows)
}

// QuerySessionEventsNearTimestamp returns Write session events within windowMs
// milliseconds of the given timestamp, regardless of file path. Results are
// ordered by absolute time distance ascending (closest first).
func (s *Store) QuerySessionEventsNearTimestamp(timestamp time.Time, windowMs int) ([]StoredSessionEvent, error) {
	// Compute the window boundaries in Go for reliable comparison.
	windowDur := time.Duration(windowMs) * time.Millisecond
	start := timestamp.Add(-windowDur)
	end := timestamp.Add(windowDur)

	rows, err := s.db.Query(
		`SELECT id, session_id, event_type, tool_name, file_path, content_hash, timestamp
		 FROM session_events
		 WHERE tool_name = 'Write' AND timestamp >= ? AND timestamp <= ?
		 ORDER BY timestamp ASC`,
		start.UTC().Format(time.RFC3339Nano),
		end.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSessionEvents(rows)
}

// ---------------------------------------------------------------------------
// Attribution persistence
// ---------------------------------------------------------------------------

// InsertAttribution persists an attribution record and returns its row ID.
func (s *Store) InsertAttribution(attr AttributionRecord) (int64, error) {
	uncertain := 0
	if attr.Uncertain {
		uncertain = 1
	}
	result, err := s.db.Exec(
		`INSERT INTO attributions
		 (file_path, project_path, file_event_id, session_event_id, authorship_level,
		  confidence, uncertain, first_author, correlation_window_ms, timestamp, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		attr.FilePath, attr.ProjectPath,
		attr.FileEventID, attr.SessionEventID,
		attr.AuthorshipLevel, attr.Confidence, uncertain,
		attr.FirstAuthor, attr.CorrelationWindowMs,
		attr.Timestamp.UTC().Format(time.RFC3339Nano),
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// QueryAttributionsByFile returns all attributions for a file, ordered by
// timestamp ascending.
func (s *Store) QueryAttributionsByFile(filePath string) ([]AttributionRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, file_path, project_path, file_event_id, session_event_id,
		        authorship_level, confidence, uncertain, first_author,
		        correlation_window_ms, timestamp
		 FROM attributions
		 WHERE file_path = ?
		 ORDER BY timestamp ASC`,
		filePath,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanAttributions(rows)
}

// QueryAttributionsByProject returns all attributions for a project, ordered
// by timestamp ascending.
func (s *Store) QueryAttributionsByProject(projectPath string) ([]AttributionRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, file_path, project_path, file_event_id, session_event_id,
		        authorship_level, confidence, uncertain, first_author,
		        correlation_window_ms, timestamp
		 FROM attributions
		 WHERE project_path = ?
		 ORDER BY timestamp ASC`,
		projectPath,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanAttributions(rows)
}

// ---------------------------------------------------------------------------
// Row scanner helpers
// ---------------------------------------------------------------------------

func scanFileEvents(rows *sql.Rows) ([]FileEvent, error) {
	var events []FileEvent
	for rows.Next() {
		var fe FileEvent
		var ts string
		if err := rows.Scan(&fe.ID, &fe.ProjectPath, &fe.FilePath, &fe.EventType, &ts); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return nil, fmt.Errorf("parse file_event timestamp %q: %w", ts, err)
		}
		fe.Timestamp = t
		events = append(events, fe)
	}
	return events, rows.Err()
}

func scanSessionEvents(rows *sql.Rows) ([]StoredSessionEvent, error) {
	var events []StoredSessionEvent
	for rows.Next() {
		var se StoredSessionEvent
		var ts string
		if err := rows.Scan(&se.ID, &se.SessionID, &se.EventType, &se.ToolName, &se.FilePath, &se.ContentHash, &ts); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return nil, fmt.Errorf("parse session_event timestamp %q: %w", ts, err)
		}
		se.Timestamp = t
		events = append(events, se)
	}
	return events, rows.Err()
}

func scanAttributions(rows *sql.Rows) ([]AttributionRecord, error) {
	var records []AttributionRecord
	for rows.Next() {
		var r AttributionRecord
		var ts string
		var uncertain int
		if err := rows.Scan(
			&r.ID, &r.FilePath, &r.ProjectPath,
			&r.FileEventID, &r.SessionEventID,
			&r.AuthorshipLevel, &r.Confidence, &uncertain,
			&r.FirstAuthor, &r.CorrelationWindowMs, &ts,
		); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return nil, fmt.Errorf("parse attribution timestamp %q: %w", ts, err)
		}
		r.Timestamp = t
		r.Uncertain = uncertain != 0
		records = append(records, r)
	}
	return records, rows.Err()
}
