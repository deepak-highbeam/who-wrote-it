// Package gitint provides git repository integration using go-git.
// It parses commits, diffs, blame, and detects Co-Authored-By tags.
//
// Git is a SECONDARY attribution source. Daemon-captured data (file events
// + session events) is primary. Git data enriches and validates, but does
// not override authorship decisions (that is Phase 2's job).
package gitint

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/anthropic/who-wrote-it/internal/store"
)

// Repository wraps a go-git repository with a store for persistence.
type Repository struct {
	repo  *git.Repository
	store *store.Store
	path  string
}

// Open opens an existing git repository at repoPath and returns a Repository
// wired to the given store for persistence.
func Open(repoPath string, s *store.Store) (*Repository, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("open git repo at %s: %w", repoPath, err)
	}
	return &Repository{
		repo:  repo,
		store: s,
		path:  repoPath,
	}, nil
}

// SyncCommits scans commits since the given time and stores any that are new.
// It uses daemon_state to track the last synced commit hash and avoid reprocessing.
func (r *Repository) SyncCommits(ctx context.Context, since time.Time) error {
	// Check last synced commit to avoid reprocessing.
	lastHash, err := r.store.GetDaemonState("git_last_synced_commit")
	if err != nil {
		return fmt.Errorf("get last synced commit: %w", err)
	}

	head, err := r.repo.Head()
	if err != nil {
		return fmt.Errorf("get HEAD: %w", err)
	}

	// If HEAD hasn't changed, nothing to do.
	if lastHash == head.Hash().String() {
		return nil
	}

	// Iterate commits from HEAD.
	iter, err := r.repo.Log(&git.LogOptions{
		Since: &since,
		Order: git.LogOrderCommitterTime,
	})
	if err != nil {
		return fmt.Errorf("git log: %w", err)
	}
	defer iter.Close()

	var synced int
	err = iter.ForEach(func(c *object.Commit) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Stop if we've reached the last synced commit.
		if c.Hash.String() == lastHash {
			return errStopIteration
		}

		if err := r.processCommit(c); err != nil {
			log.Printf("gitint: process commit %s: %v", c.Hash.String()[:7], err)
			// Continue processing other commits.
		}
		synced++
		return nil
	})

	if err != nil && err != errStopIteration {
		return fmt.Errorf("iterate commits: %w", err)
	}

	// Update last synced commit.
	if synced > 0 {
		if err := r.store.SetDaemonState("git_last_synced_commit", head.Hash().String()); err != nil {
			return fmt.Errorf("set last synced commit: %w", err)
		}
		// synced commits silently
	}

	return nil
}

// processCommit extracts metadata, diffs, and coauthor info from a single commit.
func (r *Repository) processCommit(c *object.Commit) error {
	hash := c.Hash.String()
	author := fmt.Sprintf("%s <%s>", c.Author.Name, c.Author.Email)
	hasCoauthor, coauthorName := DetectCoAuthor(c.Message)

	commitID, err := r.store.InsertGitCommit(hash, author, c.Message, c.Author.When, hasCoauthor, coauthorName)
	if err != nil {
		return fmt.Errorf("insert commit: %w", err)
	}

	// Compute diffs.
	diffs, err := commitDiffs(c)
	if err != nil {
		log.Printf("gitint: diff for %s: %v", hash[:7], err)
		return nil // Non-fatal: store the commit even if diffs fail.
	}

	for _, d := range diffs {
		if err := r.store.InsertGitDiff(commitID, d.FilePath, d.OldPath, d.ChangeType, d.Additions, d.Deletions); err != nil {
			log.Printf("gitint: insert diff for %s %s: %v", hash[:7], d.FilePath, err)
		}
	}

	return nil
}

// SyncInterval returns the recommended interval between sync calls.
func SyncInterval() time.Duration {
	return 30 * time.Second
}

// DefaultLookback returns the default lookback period for first sync.
func DefaultLookback() time.Duration {
	return 30 * 24 * time.Hour
}

// LastSyncedCommit returns the hash of the last synced commit, or empty string.
func (r *Repository) LastSyncedCommit() (string, error) {
	return r.store.GetDaemonState("git_last_synced_commit")
}

// LastSyncedTime returns when the last sync was performed.
func (r *Repository) LastSyncedTime() (time.Time, error) {
	val, err := r.store.GetDaemonState("git_last_sync_time")
	if err != nil || val == "" {
		return time.Time{}, err
	}
	epoch, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(epoch, 0), nil
}

// Repo returns the underlying go-git repository for direct access (e.g. blame).
func (r *Repository) Repo() *git.Repository {
	return r.repo
}

// Store returns the associated store.
func (r *Repository) Store() *store.Store {
	return r.store
}

// errStopIteration is used to stop commit iteration when we reach the last synced commit.
var errStopIteration = fmt.Errorf("stop iteration")
