package store

import (
	"database/sql"
	"fmt"

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
