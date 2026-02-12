package watcher

import (
	"path/filepath"
	"strings"
)

// defaultIgnorePatterns are always ignored regardless of user configuration.
var defaultIgnorePatterns = []string{
	".git",
	"node_modules",
	".idea",
	".vscode",
	"__pycache__",
	"*.swp",
	"*.swo",
	"*~",
	"*.tmp",
	"*.tmp.*",
	".DS_Store",
	"build",
	"dist",
	"target",
}

// Filter checks file paths against a set of ignore patterns.
// Patterns are glob-based and matched against each path component,
// so "node_modules" will match "foo/node_modules/bar.js".
type Filter struct {
	patterns []string
}

// NewFilter creates a Filter with the default patterns merged with any
// additional user-supplied patterns. Duplicates are removed.
func NewFilter(extra []string) *Filter {
	seen := make(map[string]struct{}, len(defaultIgnorePatterns)+len(extra))
	var merged []string
	for _, p := range defaultIgnorePatterns {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			merged = append(merged, p)
		}
	}
	for _, p := range extra {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			merged = append(merged, p)
		}
	}
	return &Filter{patterns: merged}
}

// ShouldIgnore returns true if path matches any ignore pattern.
//
// Matching rules:
//  1. Each component of the path (split by os separator) is tested against
//     every pattern using filepath.Match. This catches patterns like
//     "node_modules" even when deeply nested (e.g. "a/b/node_modules/c.js").
//  2. The full basename (last component) is also tested, which handles
//     extension patterns like "*.swp".
func (f *Filter) ShouldIgnore(path string) bool {
	// Normalise to forward slashes for consistent splitting, then also
	// try with the OS-native separator.
	cleaned := filepath.Clean(path)
	components := strings.Split(cleaned, string(filepath.Separator))

	for _, component := range components {
		for _, pattern := range f.patterns {
			if matched, _ := filepath.Match(pattern, component); matched {
				return true
			}
		}
	}
	return false
}
