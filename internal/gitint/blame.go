package gitint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"

	"github.com/anthropic/gap-map/internal/store"
)

// BlameFile runs git blame on a single file and returns per-line authorship.
// The file must exist in the HEAD commit. Deleted files return an error.
//
// Blame is expensive -- callers should run it selectively (e.g. only for
// files changed in new commits, not on every sync cycle).
func BlameFile(repo *git.Repository, filePath string) ([]store.BlameLine, error) {
	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("get HEAD: %w", err)
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("get HEAD commit: %w", err)
	}

	result, err := git.Blame(commit, filePath)
	if err != nil {
		return nil, fmt.Errorf("blame %s: %w", filePath, err)
	}

	var lines []store.BlameLine
	for i, line := range result.Lines {
		// go-git Line: Author is email, AuthorName is name.
		author := formatAuthor(line.AuthorName, line.Author)
		contentHash := hashLine(line.Text)
		lines = append(lines, store.BlameLine{
			LineNumber:  i + 1,
			CommitHash:  line.Hash.String(),
			Author:      author,
			ContentHash: contentHash,
		})
	}

	return lines, nil
}

// BlameAndStore runs blame on a file and persists the results.
// It replaces any existing blame data for the file.
func BlameAndStore(repo *git.Repository, s *store.Store, filePath string) error {
	lines, err := BlameFile(repo, filePath)
	if err != nil {
		return err
	}
	return s.InsertBlameLines(filePath, lines)
}

// BlameChangedFiles runs blame for each file that was changed in recent commits.
// This is the recommended usage: blame only what changed, not the whole repo.
func BlameChangedFiles(repo *git.Repository, s *store.Store, changedFiles []string) {
	for _, fp := range changedFiles {
		// File might be deleted or binary -- non-fatal.
		_ = BlameAndStore(repo, s, fp)
	}
}

// formatAuthor formats a name and email into "Name <email>" format.
func formatAuthor(name, email string) string {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if email == "" {
		return name
	}
	return fmt.Sprintf("%s <%s>", name, email)
}

// hashLine computes SHA-256 of a single line of text.
func hashLine(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

