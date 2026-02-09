package store

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"
)

// runMigrations applies all pending schema migrations.
// It reads the current version from daemon_state and applies each
// subsequent migration up to schemaVersion.
func runMigrations(db *sql.DB) error {
	// Ensure daemon_state table exists so we can read the schema version.
	// This is idempotent because of IF NOT EXISTS.
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS daemon_state (
		key        TEXT PRIMARY KEY,
		value      TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create daemon_state: %w", err)
	}

	current, err := currentVersion(db)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for v := current + 1; v <= schemaVersion; v++ {
		sql, ok := migrations[v]
		if !ok {
			return fmt.Errorf("missing migration for version %d", v)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", v, err)
		}

		if _, err := tx.Exec(sql); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d: %w", v, err)
		}

		now := time.Now().UTC().Format(time.RFC3339)
		_, err = tx.Exec(
			`INSERT INTO daemon_state (key, value, updated_at) VALUES ('schema_version', ?, ?)
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			strconv.Itoa(v), now,
		)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("update schema version to %d: %w", v, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", v, err)
		}
	}

	return nil
}

// currentVersion reads the schema version from daemon_state.
// Returns 0 if no version is recorded yet.
func currentVersion(db *sql.DB) (int, error) {
	var val string
	err := db.QueryRow(`SELECT value FROM daemon_state WHERE key = 'schema_version'`).Scan(&val)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(val)
}
