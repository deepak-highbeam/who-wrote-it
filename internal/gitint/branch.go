package gitint

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CurrentBranch returns the current branch name for the git repository at repoPath.
// For detached HEAD, it returns the commit hash.
func CurrentBranch(repoPath string) (string, error) {
	cmd := exec.Command("git", "symbolic-ref", "--short", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	// Detached HEAD — return the commit hash.
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	out, err = cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// MergeBaseDiffAdditions returns the added lines from git diff between the
// merge-base of baseBranch and HEAD. Includes committed, staged, unstaged,
// and untracked file changes.
func MergeBaseDiffAdditions(repoPath, baseBranch string) string {
	mergeBase := findMergeBase(repoPath, baseBranch, "HEAD")
	if mergeBase == "" {
		return ""
	}

	// Tracked changes: committed + staged + unstaged.
	cmd := exec.Command("git", "diff", mergeBase)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	result := parseBranchDiffAdditions(string(out))

	// Also include content from untracked files.
	cmd = exec.Command("git", "ls-files", "--others", "--exclude-standard")
	cmd.Dir = repoPath
	untrackedOut, err := cmd.Output()
	if err == nil {
		untrackedFiles := strings.TrimSpace(string(untrackedOut))
		if untrackedFiles != "" {
			for _, file := range strings.Split(untrackedFiles, "\n") {
				if file == "" {
					continue
				}
				data, err := os.ReadFile(filepath.Join(repoPath, file))
				if err != nil || len(data) == 0 {
					continue
				}
				if result != "" && !strings.HasSuffix(result, "\n") {
					result += "\n"
				}
				result += string(data)
				if !strings.HasSuffix(result, "\n") {
					result += "\n"
				}
			}
		}
	}

	return result
}

// MergeBaseDiffAdditionsCommitted returns the added lines from git diff between
// baseBranch and branch (committed changes only, using three-dot notation).
func MergeBaseDiffAdditionsCommitted(repoPath, baseBranch, branch string) string {
	cmd := exec.Command("git", "diff", baseBranch+"..."+branch)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return parseBranchDiffAdditions(string(out))
}

// findMergeBase returns the merge-base commit hash between two refs.
func findMergeBase(repoPath, ref1, ref2 string) string {
	cmd := exec.Command("git", "merge-base", ref1, ref2)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// parseBranchDiffAdditions extracts added lines from unified diff output.
func parseBranchDiffAdditions(diff string) string {
	var additions []string
	scanner := bufio.NewScanner(strings.NewReader(diff))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			additions = append(additions, line[1:])
		}
	}
	if len(additions) == 0 {
		return ""
	}
	return strings.Join(additions, "\n") + "\n"
}
