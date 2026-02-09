package gitint

import (
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"
)

// diffStat holds per-file diff statistics for a commit.
type diffStat struct {
	FilePath   string
	OldPath    string // Non-empty for renames.
	ChangeType string // "add", "modify", "delete", "rename"
	Additions  int
	Deletions  int
}

// DetectCoAuthor parses a commit message for Co-Authored-By trailer lines.
// The match is case-insensitive per git convention.
// Returns whether a tag was found and the first coauthor name.
//
// Recognized formats:
//
//	Co-Authored-By: Name <email>
//	Co-authored-by: Name <email>
//	co-authored-by: Name <email>
func DetectCoAuthor(message string) (bool, string) {
	matches := coAuthorRe.FindAllStringSubmatch(message, -1)
	if len(matches) == 0 {
		return false, ""
	}
	// Return the first coauthor name (trimmed).
	name := strings.TrimSpace(matches[0][1])
	return true, name
}

// AllCoAuthors returns all coauthor names found in the message.
func AllCoAuthors(message string) []string {
	matches := coAuthorRe.FindAllStringSubmatch(message, -1)
	var names []string
	for _, m := range matches {
		name := strings.TrimSpace(m[1])
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// coAuthorRe matches "Co-Authored-By: Name <email>" (case insensitive, multi-line).
var coAuthorRe = regexp.MustCompile(`(?im)co-authored-by:\s*(.+?)(?:\s*<[^>]*>)?\s*$`)

// commitDiffs computes per-file diff stats for a commit.
// For merge commits (multiple parents), diffs against first parent only.
// For initial commits (no parents), all files are additions.
func commitDiffs(c *object.Commit) ([]diffStat, error) {
	var parentTree *object.Tree
	if c.NumParents() > 0 {
		parent, err := c.Parent(0)
		if err != nil {
			return nil, err
		}
		parentTree, err = parent.Tree()
		if err != nil {
			return nil, err
		}
	}

	commitTree, err := c.Tree()
	if err != nil {
		return nil, err
	}

	changes, err := parentTree.Diff(commitTree)
	if err != nil {
		return nil, err
	}

	var stats []diffStat
	for _, change := range changes {
		ds := diffStat{}

		// Determine change type and paths.
		fromName := change.From.Name
		toName := change.To.Name

		switch {
		case fromName == "" && toName != "":
			ds.ChangeType = "add"
			ds.FilePath = toName
		case fromName != "" && toName == "":
			ds.ChangeType = "delete"
			ds.FilePath = fromName
		case fromName != toName:
			ds.ChangeType = "rename"
			ds.FilePath = toName
			ds.OldPath = fromName
		default:
			ds.ChangeType = "modify"
			ds.FilePath = toName
		}

		// Compute line-level stats using patch.
		patch, err := change.Patch()
		if err != nil {
			// If patch fails (binary file, etc.), record without line counts.
			stats = append(stats, ds)
			continue
		}

		for _, filePatch := range patch.FilePatches() {
			for _, chunk := range filePatch.Chunks() {
				content := chunk.Content()
				lineCount := strings.Count(content, "\n")
				if len(content) > 0 && content[len(content)-1] != '\n' {
					lineCount++ // Count last line without trailing newline.
				}

				switch chunk.Type() {
				case 0: // Equal
					// No additions or deletions.
				case 1: // Add
					ds.Additions += lineCount
				case 2: // Delete
					ds.Deletions += lineCount
				}
			}
		}

		stats = append(stats, ds)
	}

	return stats, nil
}
