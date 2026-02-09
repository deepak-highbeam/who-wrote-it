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
