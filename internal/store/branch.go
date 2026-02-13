package store

import (
	"database/sql"
	"fmt"
	"time"
)

// InsertFileEventWithBranch records a file system event with branch tracking.
func (s *Store) InsertFileEventWithBranch(projectPath, filePath, eventType string, timestamp time.Time, branch string) error {
	_, err := s.db.Exec(
		`INSERT INTO file_events (project_path, file_path, event_type, timestamp, branch)
		 VALUES (?, ?, ?, ?, ?)`,
		projectPath, filePath, eventType, timestamp.UTC().Format(time.RFC3339Nano), branch,
	)
	return err
}

// InsertSessionEventWithBranch records an AI tool session event with branch tracking.
func (s *Store) InsertSessionEventWithBranch(sessionID, eventType, toolName, filePath, contentHash string, timestamp time.Time, rawJSON string, linesChanged int, branch string) error {
	_, err := s.db.Exec(
		`INSERT INTO session_events (session_id, event_type, tool_name, file_path, content_hash, timestamp, raw_json, lines_changed, branch)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, eventType, toolName, filePath, contentHash,
		timestamp.UTC().Format(time.RFC3339Nano), rawJSON, linesChanged, branch,
	)
	return err
}

// QueryAttributionsByBranch returns all attributions for a project on a specific branch,
// with work type information, ordered by timestamp ascending.
func (s *Store) QueryAttributionsByBranch(projectPath, branch string) ([]AttributionWithWorkType, error) {
	rows, err := s.db.Query(
		`SELECT id, file_path, project_path, file_event_id, session_event_id,
		        authorship_level, confidence, uncertain, first_author,
		        correlation_window_ms, timestamp, COALESCE(work_type, ''), lines_changed, branch
		 FROM attributions
		 WHERE project_path = ? AND branch = ?
		 ORDER BY timestamp ASC`,
		projectPath, branch,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAttributionsWithWorkTypeAndBranch(rows)
}

func scanAttributionsWithWorkTypeAndBranch(rows *sql.Rows) ([]AttributionWithWorkType, error) {
	var records []AttributionWithWorkType
	for rows.Next() {
		var r AttributionWithWorkType
		var ts string
		var uncertain int
		if err := rows.Scan(
			&r.ID, &r.FilePath, &r.ProjectPath,
			&r.FileEventID, &r.SessionEventID,
			&r.AuthorshipLevel, &r.Confidence, &uncertain,
			&r.FirstAuthor, &r.CorrelationWindowMs, &ts,
			&r.WorkType, &r.LinesChanged, &r.Branch,
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
