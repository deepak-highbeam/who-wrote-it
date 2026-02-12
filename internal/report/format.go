package report

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropic/who-wrote-it/internal/ipc"
)

// ANSI escape codes for terminal formatting.
const (
	bold  = "\033[1m"
	red   = "\033[31m"
	green = "\033[32m"
	yellow = "\033[33m"
	reset = "\033[0m"
)

// FormatProjectReport formats a ProjectReport as a terminal-friendly string.
// Uses ANSI color codes: >70% AI = red, 30-70% = yellow, <30% = green.
func FormatProjectReport(r *ProjectReport) string {
	var b strings.Builder

	// Header.
	b.WriteString(bold + "Who Wrote It - Attribution Report" + reset + "\n")
	b.WriteString(strings.Repeat("=", 40) + "\n\n")

	// Headline metric.
	b.WriteString(fmt.Sprintf("Project: %s\n", r.ProjectPath))
	b.WriteString(fmt.Sprintf("Meaningful AI: %s%s%.1f%%%s\n",
		bold, colorForPct(r.MeaningfulAIPct), r.MeaningfulAIPct, reset))
	b.WriteString(fmt.Sprintf("Raw AI:        %.1f%%\n", r.RawAIPct))
	b.WriteString(fmt.Sprintf("Total files:   %d\n", r.TotalFiles))
	b.WriteString(fmt.Sprintf("Total lines:   %d (%d AI)\n\n", r.TotalLines, r.AILines))

	// Spectrum breakdown table (3 levels).
	b.WriteString(bold + "Authorship Spectrum" + reset + "\n")
	b.WriteString(strings.Repeat("-", 35) + "\n")
	b.WriteString(fmt.Sprintf("%-20s %12s\n", "Level", "Files"))
	b.WriteString(strings.Repeat("-", 35) + "\n")

	spectrumLevels := []string{
		"mostly_ai",
		"mixed",
		"mostly_human",
	}
	totalFiles := 0
	for _, count := range r.ByAuthorship {
		totalFiles += count
	}
	for _, level := range spectrumLevels {
		count := r.ByAuthorship[level]
		pct := 0.0
		if totalFiles > 0 {
			pct = float64(count) / float64(totalFiles) * 100.0
		}
		b.WriteString(fmt.Sprintf("%-20s %4d (%4.1f%%)\n", level, count, pct))
	}
	b.WriteString("\n")

	// Work-type distribution table.
	b.WriteString(bold + "Work Type Distribution" + reset + "\n")
	b.WriteString(strings.Repeat("-", 70) + "\n")
	b.WriteString(fmt.Sprintf("%-18s %-8s %5s %8s %6s %6s\n", "Work Type", "Tier", "Files", "Lines", "AI%", "Weight"))
	b.WriteString(strings.Repeat("-", 70) + "\n")

	// Sort work types for stable output.
	wtOrder := []string{"architecture", "core_logic", "bug_fix", "edge_case", "boilerplate", "test_scaffolding"}
	for _, wt := range wtOrder {
		summary, ok := r.ByWorkType[wt]
		if !ok {
			continue
		}
		b.WriteString(fmt.Sprintf("%-18s %-8s %5d %8d %s%5.1f%%%s %6.1f\n",
			wt, summary.Tier, summary.Files,
			summary.TotalLines,
			colorForPct(summary.AIPct), summary.AIPct, reset,
			summary.Weight))
	}
	b.WriteString("\n")

	// Top files sorted by AI%.
	if len(r.Files) > 0 {
		b.WriteString(bold + "Files by AI %" + reset + "\n")
		b.WriteString(strings.Repeat("-", 80) + "\n")
		b.WriteString(fmt.Sprintf("%-35s %-16s %6s %7s %8s\n", "File", "Work Type", "AI%", "Lines", "Level"))
		b.WriteString(strings.Repeat("-", 80) + "\n")

		maxFiles := len(r.Files)
		if maxFiles > 20 {
			maxFiles = 20
		}
		for _, f := range r.Files[:maxFiles] {
			name := f.FilePath
			if len(name) > 34 {
				name = "..." + name[len(name)-31:]
			}
			b.WriteString(fmt.Sprintf("%-35s %-16s %s%5.1f%%%s %7d %8s\n",
				name, f.WorkType,
				colorForPct(f.MeaningfulAIPct), f.MeaningfulAIPct, reset,
				f.TotalLines, f.AuthorshipLevel))
		}
		if len(r.Files) > 20 {
			b.WriteString(fmt.Sprintf("... and %d more files\n", len(r.Files)-20))
		}
	}

	return b.String()
}

// FormatFileReport formats a single FileReport as a terminal-friendly string.
func FormatFileReport(r *FileReport) string {
	var b strings.Builder

	b.WriteString(bold + "Who Wrote It - File Report" + reset + "\n")
	b.WriteString(strings.Repeat("=", 40) + "\n\n")

	b.WriteString(fmt.Sprintf("File:      %s\n", r.FilePath))
	b.WriteString(fmt.Sprintf("Work Type: %s\n", r.WorkType))
	b.WriteString(fmt.Sprintf("AI %%:      %s%s%.1f%%%s\n",
		bold, colorForPct(r.MeaningfulAIPct), r.MeaningfulAIPct, reset))
	b.WriteString(fmt.Sprintf("Raw AI %%:  %.1f%%\n", r.RawAIPct))
	b.WriteString(fmt.Sprintf("Lines:     %d total, %d AI\n", r.TotalLines, r.AILines))
	b.WriteString(fmt.Sprintf("Level:     %s\n", r.AuthorshipLevel))
	b.WriteString(fmt.Sprintf("Events:    %d total, %d AI\n\n", r.TotalEvents, r.AIEventCount))

	b.WriteString(bold + "Authorship Breakdown" + reset + "\n")
	b.WriteString(strings.Repeat("-", 40) + "\n")
	b.WriteString(fmt.Sprintf("%-20s %6s\n", "Level", "Count"))
	b.WriteString(strings.Repeat("-", 40) + "\n")

	levels := []string{
		"mostly_ai",
		"mixed",
		"mostly_human",
	}
	for _, level := range levels {
		count := r.AuthorshipCounts[level]
		if count > 0 {
			b.WriteString(fmt.Sprintf("%-20s %6d\n", level, count))
		}
	}

	return b.String()
}

// FormatStatus formats daemon StatusData as a terminal-friendly table.
func FormatStatus(status *ipc.StatusData) string {
	var b strings.Builder

	b.WriteString(bold + "Who Wrote It - Daemon Status" + reset + "\n")
	b.WriteString(strings.Repeat("=", 40) + "\n\n")

	b.WriteString(fmt.Sprintf("%-20s %s\n", "Uptime:", status.Uptime))
	b.WriteString(fmt.Sprintf("%-20s %s\n", "DB Size:", humanBytes(status.DBSizeBytes)))
	b.WriteString(fmt.Sprintf("%-20s %d\n", "File Events:", status.FileEventsCount))
	b.WriteString(fmt.Sprintf("%-20s %d\n", "Session Events:", status.SessionEventsCount))
	b.WriteString(fmt.Sprintf("%-20s %d\n", "Git Commits:", status.GitCommitsCount))

	if len(status.WatchedPaths) > 0 {
		b.WriteString(fmt.Sprintf("\n%sWatched Paths:%s\n", bold, reset))
		for _, p := range status.WatchedPaths {
			b.WriteString(fmt.Sprintf("  %s\n", p))
		}
	} else {
		b.WriteString(fmt.Sprintf("%-20s %s\n", "Watched Paths:", "(none)"))
	}

	return b.String()
}

// FormatJSON marshals any value as indented JSON.
func FormatJSON(v interface{}) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(data)
}

// colorForPct returns an ANSI color code based on the AI percentage.
// >70% = red, 30-70% = yellow, <30% = green.
func colorForPct(pct float64) string {
	switch {
	case pct > 70:
		return red
	case pct >= 30:
		return yellow
	default:
		return green
	}
}

// humanBytes formats bytes as a human-readable string (KB, MB, GB).
func humanBytes(b int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)

	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
