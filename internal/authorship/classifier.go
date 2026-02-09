// Package authorship provides a five-level authorship spectrum classifier.
// It assigns attribution labels (fully AI, mixed, fully human) based on
// correlation results that pair file system events with AI session events.
package authorship

import (
	"strings"
	"time"

	"github.com/anthropic/who-wrote-it/internal/store"
)

// AuthorshipLevel represents one of five levels on the authorship spectrum.
type AuthorshipLevel string

const (
	// FullyAI means the code was written entirely by an AI tool (e.g. Claude
	// Write event matched within tight time window).
	FullyAI AuthorshipLevel = "fully_ai"

	// AIFirstHumanRevised means AI wrote the initial code and a human
	// subsequently edited it.
	AIFirstHumanRevised AuthorshipLevel = "ai_first_human_revised"

	// HumanFirstAIRevised means a human wrote the initial code and AI
	// subsequently edited it.
	HumanFirstAIRevised AuthorshipLevel = "human_first_ai_revised"

	// AISuggestedHumanWritten means AI was active (session event nearby) but
	// did not directly write this file -- the human wrote it with AI context.
	AISuggestedHumanWritten AuthorshipLevel = "ai_suggested_human_written"

	// FullyHuman means no AI session activity was detected near the edit.
	FullyHuman AuthorshipLevel = "fully_human"
)

// CorrelationResult is the output of the correlation engine. Defined here
// to avoid a circular import between correlation and authorship packages.
// The correlation package produces values of this type; the classifier consumes them.
type CorrelationResult struct {
	FileEvent      store.FileEvent
	MatchedSession *store.StoredSessionEvent // nil if no match found
	TimeDeltaMs    int64                     // absolute ms between events; 0 if no match
	MatchType      string                    // "exact_file", "time_proximity", "none"
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
// Rules:
//  1. No match (MatchType "none")          -> FullyHuman,               confidence 1.0
//  2. Exact file match, delta < 2000ms     -> FullyAI,                  confidence 0.95
//  3. Exact file match, delta 2000-5000ms  -> AIFirstHumanRevised,      confidence 0.7
//  4. Time proximity only (different file)  -> AISuggestedHumanWritten,  confidence 0.5
//  5. Confidence < 0.5                     -> Uncertain = true
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
		attr.Level = FullyHuman
		attr.Confidence = 1.0
		attr.FirstAuthor = "human"

	case result.MatchType == "exact_file" && result.TimeDeltaMs < 2000:
		attr.Level = FullyAI
		attr.Confidence = 0.95
		attr.FirstAuthor = "ai"

	case result.MatchType == "exact_file" && result.TimeDeltaMs >= 2000:
		attr.Level = AIFirstHumanRevised
		attr.Confidence = 0.7
		attr.FirstAuthor = "ai"

	case result.MatchType == "time_proximity":
		attr.Level = AISuggestedHumanWritten
		attr.Confidence = 0.5
		attr.FirstAuthor = "human"
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
//   - Prior author = "ai", current has no session match -> AIFirstHumanRevised
//     (human editing AI code)
//   - Prior author = "human", current has session match -> HumanFirstAIRevised
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
		attr.Level = AIFirstHumanRevised
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
		attr.Level = HumanFirstAIRevised
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
		attr.Level = FullyAI
		attr.Confidence = 0.6
		attr.FirstAuthor = "ai"
	} else {
		attr.Level = FullyHuman
		attr.Confidence = 0.8
		attr.FirstAuthor = "human"
	}

	if attr.Confidence < 0.5 {
		attr.Uncertain = true
	}

	return attr
}
