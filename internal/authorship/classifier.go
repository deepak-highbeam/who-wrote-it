// Package authorship provides a three-level authorship spectrum classifier.
// It assigns attribution labels (mostly_ai, mixed, mostly_human) based on
// correlation results that pair file system events with AI session events.
// The final per-file level is computed at metrics time from aggregated line ratios.
package authorship

import (
	"strings"
	"time"

	"github.com/anthropic/who-wrote-it/internal/store"
)

// AuthorshipLevel represents one of three levels on the authorship spectrum.
type AuthorshipLevel string

const (
	// MostlyAI means the code was written primarily by an AI tool.
	// Per-event: assigned when a session match is found (exact_file, fuzzy_file).
	// Per-file: assigned when aggregated AI line ratio > 70%.
	MostlyAI AuthorshipLevel = "mostly_ai"

	// Mixed means attribution is uncertain or shared between AI and human.
	// Per-event: assigned when only session activity is detected nearby.
	// Per-file: assigned when aggregated AI line ratio is 30-70%.
	Mixed AuthorshipLevel = "mixed"

	// MostlyHuman means the code was written primarily by a human.
	// Per-event: assigned when no AI session activity is detected.
	// Per-file: assigned when aggregated AI line ratio < 30%.
	MostlyHuman AuthorshipLevel = "mostly_human"
)

// CorrelationResult is the output of the correlation engine. Defined here
// to avoid a circular import between correlation and authorship packages.
// The correlation package produces values of this type; the classifier consumes them.
type CorrelationResult struct {
	FileEvent      store.FileEvent
	MatchedSession *store.StoredSessionEvent // nil if no match found
	TimeDeltaMs    int64                     // absolute ms between events; 0 if no match
	MatchType      string                    // "exact_file", "fuzzy_file", "none"
}

// Attribution is the final authorship classification for a file event.
type Attribution struct {
	FilePath            string
	ProjectPath         string
	FileEventID         *int64
	SessionEventID      *int64
	Level               AuthorshipLevel
	Confidence          float64
	Uncertain           bool
	FirstAuthor         string // "ai" or "human"
	CorrelationWindowMs int
	Timestamp           time.Time
}

// Classifier assigns authorship levels to correlation results.
type Classifier struct{}

// NewClassifier creates a new Classifier.
func NewClassifier() *Classifier {
	return &Classifier{}
}

// Classify assigns an authorship level and confidence to a single
// CorrelationResult using the locked decision rules.
//
// Rules (per-event classification):
//  1. No match (MatchType "none")                    -> MostlyHuman, confidence 1.0
//  2. Exact file match (any delta)                   -> MostlyAI,    confidence 0.95
//  3. Fuzzy file match (same name, different prefix) -> MostlyAI,    confidence 0.85
//  4. Confidence < 0.5                               -> Uncertain = true
func (c *Classifier) Classify(result CorrelationResult) Attribution {
	attr := Attribution{
		FilePath:    result.FileEvent.FilePath,
		ProjectPath: result.FileEvent.ProjectPath,
		Timestamp:   result.FileEvent.Timestamp,
	}

	feID := result.FileEvent.ID
	attr.FileEventID = &feID

	if result.MatchedSession != nil {
		seID := result.MatchedSession.ID
		attr.SessionEventID = &seID
		attr.CorrelationWindowMs = int(result.TimeDeltaMs)
	}

	switch {
	case result.MatchType == "none" || result.MatchedSession == nil:
		attr.Level = MostlyHuman
		attr.Confidence = 1.0
		attr.FirstAuthor = "human"

	case result.MatchType == "exact_file":
		attr.Level = MostlyAI
		attr.Confidence = 0.95
		attr.FirstAuthor = "ai"

	case result.MatchType == "fuzzy_file":
		attr.Level = MostlyAI
		attr.Confidence = 0.85
		attr.FirstAuthor = "ai"

	}

	if attr.Confidence < 0.5 {
		attr.Uncertain = true
	}

	return attr
}

// ClassifyWithHistory applies first-author-wins logic when a prior attribution
// exists for the same file. If priorAttribution is nil, falls through to Classify.
//
// Rules:
//   - Prior author = "ai", current has no session match -> Mixed
//     (human editing AI code)
//   - Prior author = "human", current has session match -> Mixed
//     (AI editing human code)
//   - Otherwise: standard Classify
func (c *Classifier) ClassifyWithHistory(result CorrelationResult, priorAttribution *Attribution) Attribution {
	if priorAttribution == nil {
		return c.Classify(result)
	}

	// Prior was AI-authored, current has no session match -> human is revising AI code
	if priorAttribution.FirstAuthor == "ai" &&
		(result.MatchType == "none" || result.MatchedSession == nil) {
		attr := c.Classify(result)
		attr.Level = Mixed
		attr.Confidence = 0.8
		attr.FirstAuthor = "ai" // first-author-wins: AI was first
		attr.Uncertain = false
		return attr
	}

	// Prior was human-authored, current has session match -> AI is revising human code
	if priorAttribution.FirstAuthor == "human" &&
		result.MatchedSession != nil &&
		result.MatchType != "none" {
		attr := c.Classify(result)
		attr.Level = Mixed
		attr.Confidence = 0.8
		attr.FirstAuthor = "human" // first-author-wins: human was first
		attr.Uncertain = false
		return attr
	}

	// Default: standard classification
	return c.Classify(result)
}

// ClassifyFromGit provides attribution when only git data is available
// (no daemon session data). Uses the Co-Authored-By tag as the signal.
//
// Lower confidence than daemon-based attribution because git tags can be
// manually added/removed and don't capture the full interaction pattern.
func (c *Classifier) ClassifyFromGit(hasCoauthorTag bool, coauthorName string) Attribution {
	attr := Attribution{
		Timestamp: time.Now().UTC(),
	}

	lower := strings.ToLower(coauthorName)
	if hasCoauthorTag && (strings.Contains(lower, "claude") || strings.Contains(lower, "anthropic")) {
		attr.Level = MostlyAI
		attr.Confidence = 0.6
		attr.FirstAuthor = "ai"
	} else {
		attr.Level = MostlyHuman
		attr.Confidence = 0.8
		attr.FirstAuthor = "human"
	}

	if attr.Confidence < 0.5 {
		attr.Uncertain = true
	}

	return attr
}
