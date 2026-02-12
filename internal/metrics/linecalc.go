package metrics

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// LineAttribution holds the result of line-level attribution for a file.
type LineAttribution struct {
	TotalLines int
	AILines    int
	HumanLines int
}

// ComputeLineAttribution compares the current file content against all content
// Claude wrote to determine which lines are AI-authored vs human-authored.
//
// For each line in currentContent, if its hash appears in claudeContents
// (content from Write/Edit session events), it's counted as AI. Otherwise human.
// Duplicate handling: if a line like "}" appears N times in the file and M times
// in Claude's output, min(N, M) are attributed to AI.
//
// baseContent is the file content before tracking started. Lines that already
// existed in the base file are subtracted from Claude's hash map so that
// pre-existing patterns (like common annotations or boilerplate) are not
// falsely attributed to AI.
func ComputeLineAttribution(currentContent string, claudeContents []string, baseContent string) LineAttribution {
	if currentContent == "" {
		return LineAttribution{}
	}

	// Skip empty/whitespace-only lines — they carry no authorship signal.
	currentLines := splitNonEmpty(currentContent)
	result := LineAttribution{TotalLines: len(currentLines)}

	if len(claudeContents) == 0 {
		result.HumanLines = result.TotalLines
		return result
	}

	// Build a frequency map of line hashes from Claude's output.
	claudeHashes := make(map[string]int)
	for _, content := range claudeContents {
		for _, line := range splitNonEmpty(content) {
			h := hashLine(line)
			claudeHashes[h]++
		}
	}

	// Subtract pre-existing lines from the base file. If a line pattern
	// existed before Claude was involved, it shouldn't count as AI.
	if baseContent != "" {
		for _, line := range splitNonEmpty(baseContent) {
			h := hashLine(line)
			if claudeHashes[h] > 0 {
				claudeHashes[h]--
			}
		}
	}

	// For each line in the current file, check if Claude wrote it.
	for _, line := range currentLines {
		h := hashLine(line)
		if claudeHashes[h] > 0 {
			result.AILines++
			claudeHashes[h]-- // consume one occurrence
		} else {
			result.HumanLines++
		}
	}

	return result
}

// splitNonEmpty splits content into lines, excluding empty/whitespace-only
// lines and the trailing empty line from a trailing newline.
func splitNonEmpty(s string) []string {
	lines := strings.Split(s, "\n")
	var result []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return result
}

// splitLines splits content into lines, excluding empty trailing line from
// a trailing newline.
func splitLines(s string) []string {
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// hashLine returns a hex-encoded SHA-256 hash of a trimmed line.
// Trimming whitespace avoids false negatives from indentation changes.
func hashLine(line string) string {
	trimmed := strings.TrimSpace(line)
	h := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(h[:])
}
