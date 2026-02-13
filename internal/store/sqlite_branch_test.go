package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupBranchTestStore(t *testing.T) (*Store, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "store-branch-test-*")
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "test.db")
	s, err := New(dbPath)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatal(err)
	}
	cleanup := func() {
		s.Close()
		os.RemoveAll(dir)
	}
	return s, cleanup
}

// TestMigration_AddsBranchColumn verifies that migration v6 adds the branch
// column to file_events, session_events, and attributions tables.
func TestMigration_AddsBranchColumn(t *testing.T) {
	s, cleanup := setupBranchTestStore(t)
	defer cleanup()

	tables := []string{"file_events", "session_events", "attributions"}
	for _, table := range tables {
		var count int
		err := s.db.QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'branch'",
			table,
		).Scan(&count)
		if err != nil {
			t.Fatalf("check branch column on %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %s: expected branch column to exist, found %d columns named 'branch'", table, count)
		}
	}
}

// TestInsertFileEvent_WithBranch verifies that InsertFileEventWithBranch stores
// the branch and it can be queried back.
func TestInsertFileEvent_WithBranch(t *testing.T) {
	s, cleanup := setupBranchTestStore(t)
	defer cleanup()

	err := s.InsertFileEventWithBranch("/project", "file.go", "write", time.Now(), "feature-x")
	if err != nil {
		t.Fatalf("InsertFileEventWithBranch: %v", err)
	}

	var branch string
	err = s.db.QueryRow("SELECT branch FROM file_events WHERE file_path = 'file.go'").Scan(&branch)
	if err != nil {
		t.Fatalf("query branch: %v", err)
	}
	if branch != "feature-x" {
		t.Errorf("branch = %q, want %q", branch, "feature-x")
	}
}

// TestInsertSessionEvent_WithBranch verifies that InsertSessionEventWithBranch
// stores the branch and it can be queried back.
func TestInsertSessionEvent_WithBranch(t *testing.T) {
	s, cleanup := setupBranchTestStore(t)
	defer cleanup()

	err := s.InsertSessionEventWithBranch(
		"sess1", "tool_use", "Write", "file.go", "",
		time.Now(), "{}", 10, "feature-x",
	)
	if err != nil {
		t.Fatalf("InsertSessionEventWithBranch: %v", err)
	}

	var branch string
	err = s.db.QueryRow("SELECT branch FROM session_events WHERE session_id = 'sess1'").Scan(&branch)
	if err != nil {
		t.Fatalf("query branch: %v", err)
	}
	if branch != "feature-x" {
		t.Errorf("branch = %q, want %q", branch, "feature-x")
	}
}

// TestInsertAttribution_WithBranch verifies that inserting an AttributionRecord
// with Branch set persists the branch in the database.
func TestInsertAttribution_WithBranch(t *testing.T) {
	s, cleanup := setupBranchTestStore(t)
	defer cleanup()

	attr := AttributionRecord{
		FilePath:        "file.go",
		ProjectPath:     "/project",
		AuthorshipLevel: "mostly_ai",
		Confidence:      0.95,
		Timestamp:       time.Now(),
		Branch:          "feature-x",
	}
	id, err := s.InsertAttribution(attr)
	if err != nil {
		t.Fatalf("InsertAttribution: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero ID")
	}

	var branch string
	err = s.db.QueryRow("SELECT branch FROM attributions WHERE id = ?", id).Scan(&branch)
	if err != nil {
		t.Fatalf("query branch: %v", err)
	}
	if branch != "feature-x" {
		t.Errorf("branch = %q, want %q", branch, "feature-x")
	}
}

// TestQueryAttributions_FilterByBranch verifies that QueryAttributionsByBranch
// only returns records matching the specified branch.
func TestQueryAttributions_FilterByBranch(t *testing.T) {
	s, cleanup := setupBranchTestStore(t)
	defer cleanup()

	now := time.Now()
	for i, branch := range []string{"feature-x", "feature-y", "feature-x"} {
		attr := AttributionRecord{
			FilePath:        "file.go",
			ProjectPath:     "/project",
			AuthorshipLevel: "mostly_ai",
			Confidence:      0.95,
			Timestamp:       now.Add(time.Duration(i) * time.Second),
			Branch:          branch,
		}
		if _, err := s.InsertAttribution(attr); err != nil {
			t.Fatalf("InsertAttribution: %v", err)
		}
	}

	results, err := s.QueryAttributionsByBranch("/project", "feature-x")
	if err != nil {
		t.Fatalf("QueryAttributionsByBranch: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("got %d results, want 2 (only feature-x)", len(results))
	}
	for _, r := range results {
		if r.Branch != "feature-x" {
			t.Errorf("result branch = %q, want %q", r.Branch, "feature-x")
		}
	}
}

// TestQueryAttributions_EmptyBranchBackwardsCompat verifies that records with
// empty branch (pre-migration data) can still be queried.
func TestQueryAttributions_EmptyBranchBackwardsCompat(t *testing.T) {
	s, cleanup := setupBranchTestStore(t)
	defer cleanup()

	attr := AttributionRecord{
		FilePath:        "file.go",
		ProjectPath:     "/project",
		AuthorshipLevel: "mostly_human",
		Confidence:      0.90,
		Timestamp:       time.Now(),
		// Branch left empty (zero value).
	}
	if _, err := s.InsertAttribution(attr); err != nil {
		t.Fatalf("InsertAttribution: %v", err)
	}

	results, err := s.QueryAttributionsByBranch("/project", "")
	if err != nil {
		t.Fatalf("QueryAttributionsByBranch: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("got %d results, want 1 (backwards compat with empty branch)", len(results))
	}
}
