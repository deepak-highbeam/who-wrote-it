package authorship

import (
	"testing"
	"time"

	"github.com/anthropic/who-wrote-it/internal/store"
)

var baseTime = time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC)

func makeFileEvent(id int64, filePath string) store.FileEvent {
	return store.FileEvent{
		ID: id, ProjectPath: "/proj", FilePath: filePath,
		EventType: "write", Timestamp: baseTime,
	}
}

func makeSessionEvent(id int64, filePath string, delta time.Duration) store.StoredSessionEvent {
	return store.StoredSessionEvent{
		ID: id, SessionID: "s1", EventType: "tool_use", ToolName: "Write",
		FilePath: filePath, Timestamp: baseTime.Add(delta),
	}
}

func ptrSession(se store.StoredSessionEvent) *store.StoredSessionEvent {
	return &se
}

// ---------------------------------------------------------------------------
// Classify tests
// ---------------------------------------------------------------------------

func TestClassify_NoMatch_FullyHuman(t *testing.T) {
	c := NewClassifier()
	result := CorrelationResult{
		FileEvent: makeFileEvent(1, "foo.go"),
		MatchType: "none",
	}

	attr := c.Classify(result)

	if attr.Level != FullyHuman {
		t.Errorf("Level = %q, want %q", attr.Level, FullyHuman)
	}
	if attr.Confidence != 1.0 {
		t.Errorf("Confidence = %f, want 1.0", attr.Confidence)
	}
	if attr.FirstAuthor != "human" {
		t.Errorf("FirstAuthor = %q, want %q", attr.FirstAuthor, "human")
	}
	if attr.Uncertain {
		t.Error("Uncertain should be false")
	}
}

func TestClassify_ExactMatchUnder2s_FullyAI(t *testing.T) {
	c := NewClassifier()
	se := makeSessionEvent(10, "foo.go", 500*time.Millisecond)
	result := CorrelationResult{
		FileEvent:      makeFileEvent(1, "foo.go"),
		MatchedSession: ptrSession(se),
		TimeDeltaMs:    500,
		MatchType:      "exact_file",
	}

	attr := c.Classify(result)

	if attr.Level != FullyAI {
		t.Errorf("Level = %q, want %q", attr.Level, FullyAI)
	}
	if attr.Confidence != 0.95 {
		t.Errorf("Confidence = %f, want 0.95", attr.Confidence)
	}
	if attr.FirstAuthor != "ai" {
		t.Errorf("FirstAuthor = %q, want %q", attr.FirstAuthor, "ai")
	}
}

func TestClassify_ExactMatch2to5s_AIFirstHumanRevised(t *testing.T) {
	c := NewClassifier()
	se := makeSessionEvent(10, "foo.go", 3*time.Second)
	result := CorrelationResult{
		FileEvent:      makeFileEvent(1, "foo.go"),
		MatchedSession: ptrSession(se),
		TimeDeltaMs:    3000,
		MatchType:      "exact_file",
	}

	attr := c.Classify(result)

	if attr.Level != AIFirstHumanRevised {
		t.Errorf("Level = %q, want %q", attr.Level, AIFirstHumanRevised)
	}
	if attr.Confidence != 0.7 {
		t.Errorf("Confidence = %f, want 0.7", attr.Confidence)
	}
	if attr.FirstAuthor != "ai" {
		t.Errorf("FirstAuthor = %q, want %q", attr.FirstAuthor, "ai")
	}
}

func TestClassify_TimeProximity_AISuggestedHumanWritten(t *testing.T) {
	c := NewClassifier()
	se := makeSessionEvent(20, "bar.go", 2*time.Second)
	result := CorrelationResult{
		FileEvent:      makeFileEvent(1, "foo.go"),
		MatchedSession: ptrSession(se),
		TimeDeltaMs:    2000,
		MatchType:      "time_proximity",
	}

	attr := c.Classify(result)

	if attr.Level != AISuggestedHumanWritten {
		t.Errorf("Level = %q, want %q", attr.Level, AISuggestedHumanWritten)
	}
	if attr.Confidence != 0.5 {
		t.Errorf("Confidence = %f, want 0.5", attr.Confidence)
	}
	if attr.FirstAuthor != "human" {
		t.Errorf("FirstAuthor = %q, want %q", attr.FirstAuthor, "human")
	}
}

func TestClassify_LowConfidence_Uncertain(t *testing.T) {
	// We test the uncertain flag by constructing a result that would
	// naturally produce a confidence < 0.5.
	// Since our current rules don't produce confidence < 0.5 naturally,
	// we verify the threshold logic by checking that values at 0.5 are NOT
	// marked uncertain (boundary condition).
	c := NewClassifier()

	// Time proximity gives confidence 0.5 -- not uncertain (< 0.5 required)
	se := makeSessionEvent(20, "bar.go", 2*time.Second)
	result := CorrelationResult{
		FileEvent:      makeFileEvent(1, "foo.go"),
		MatchedSession: ptrSession(se),
		TimeDeltaMs:    2000,
		MatchType:      "time_proximity",
	}

	attr := c.Classify(result)
	if attr.Uncertain {
		t.Error("Confidence=0.5 should NOT be uncertain (threshold is < 0.5)")
	}
}

// ---------------------------------------------------------------------------
// ClassifyWithHistory tests
// ---------------------------------------------------------------------------

func TestClassifyWithHistory_HumanEditingAICode(t *testing.T) {
	c := NewClassifier()

	prior := &Attribution{
		Level:       FullyAI,
		FirstAuthor: "ai",
	}
	result := CorrelationResult{
		FileEvent: makeFileEvent(2, "foo.go"),
		MatchType: "none",
	}

	attr := c.ClassifyWithHistory(result, prior)

	if attr.Level != AIFirstHumanRevised {
		t.Errorf("Level = %q, want %q", attr.Level, AIFirstHumanRevised)
	}
	if attr.FirstAuthor != "ai" {
		t.Errorf("FirstAuthor = %q, want %q (first-author-wins)", attr.FirstAuthor, "ai")
	}
	if attr.Confidence != 0.8 {
		t.Errorf("Confidence = %f, want 0.8", attr.Confidence)
	}
}

func TestClassifyWithHistory_AIEditingHumanCode(t *testing.T) {
	c := NewClassifier()

	prior := &Attribution{
		Level:       FullyHuman,
		FirstAuthor: "human",
	}
	se := makeSessionEvent(10, "foo.go", 500*time.Millisecond)
	result := CorrelationResult{
		FileEvent:      makeFileEvent(2, "foo.go"),
		MatchedSession: ptrSession(se),
		TimeDeltaMs:    500,
		MatchType:      "exact_file",
	}

	attr := c.ClassifyWithHistory(result, prior)

	if attr.Level != HumanFirstAIRevised {
		t.Errorf("Level = %q, want %q", attr.Level, HumanFirstAIRevised)
	}
	if attr.FirstAuthor != "human" {
		t.Errorf("FirstAuthor = %q, want %q (first-author-wins)", attr.FirstAuthor, "human")
	}
	if attr.Confidence != 0.8 {
		t.Errorf("Confidence = %f, want 0.8", attr.Confidence)
	}
}

func TestClassifyWithHistory_NoPrior_FallsThrough(t *testing.T) {
	c := NewClassifier()

	result := CorrelationResult{
		FileEvent: makeFileEvent(1, "foo.go"),
		MatchType: "none",
	}

	attr := c.ClassifyWithHistory(result, nil)

	// Should behave exactly like Classify
	if attr.Level != FullyHuman {
		t.Errorf("Level = %q, want %q (nil prior -> standard classify)", attr.Level, FullyHuman)
	}
	if attr.Confidence != 1.0 {
		t.Errorf("Confidence = %f, want 1.0", attr.Confidence)
	}
}

// ---------------------------------------------------------------------------
// ClassifyFromGit tests
// ---------------------------------------------------------------------------

func TestClassifyFromGit_CoauthorTagClaude(t *testing.T) {
	c := NewClassifier()

	attr := c.ClassifyFromGit(true, "Claude Opus 4.6 <noreply@anthropic.com>")

	if attr.Level != FullyAI {
		t.Errorf("Level = %q, want %q", attr.Level, FullyAI)
	}
	if attr.Confidence != 0.6 {
		t.Errorf("Confidence = %f, want 0.6", attr.Confidence)
	}
	if attr.FirstAuthor != "ai" {
		t.Errorf("FirstAuthor = %q, want %q", attr.FirstAuthor, "ai")
	}
}

func TestClassifyFromGit_CoauthorTagAnthropic(t *testing.T) {
	c := NewClassifier()

	attr := c.ClassifyFromGit(true, "Anthropic AI Bot")

	if attr.Level != FullyAI {
		t.Errorf("Level = %q, want %q", attr.Level, FullyAI)
	}
	if attr.Confidence != 0.6 {
		t.Errorf("Confidence = %f, want 0.6", attr.Confidence)
	}
}

func TestClassifyFromGit_NoCoauthorTag(t *testing.T) {
	c := NewClassifier()

	attr := c.ClassifyFromGit(false, "")

	if attr.Level != FullyHuman {
		t.Errorf("Level = %q, want %q", attr.Level, FullyHuman)
	}
	if attr.Confidence != 0.8 {
		t.Errorf("Confidence = %f, want 0.8", attr.Confidence)
	}
	if attr.FirstAuthor != "human" {
		t.Errorf("FirstAuthor = %q, want %q", attr.FirstAuthor, "human")
	}
}

func TestClassifyFromGit_CoauthorTagNonClaude(t *testing.T) {
	c := NewClassifier()

	// Has a coauthor tag but not claude/anthropic -- treated as human
	attr := c.ClassifyFromGit(true, "GitHub Copilot")

	if attr.Level != FullyHuman {
		t.Errorf("Level = %q, want %q", attr.Level, FullyHuman)
	}
	if attr.Confidence != 0.8 {
		t.Errorf("Confidence = %f, want 0.8", attr.Confidence)
	}
}

// ---------------------------------------------------------------------------
// First-author-wins rule
// ---------------------------------------------------------------------------

func TestFirstAuthorWins_AIAuthoredWithSubsequentAIEdit(t *testing.T) {
	c := NewClassifier()

	prior := &Attribution{
		Level:       FullyAI,
		FirstAuthor: "ai",
	}

	// New correlation has session match (AI editing again) -- first author stays "ai"
	se := makeSessionEvent(10, "foo.go", 500*time.Millisecond)
	result := CorrelationResult{
		FileEvent:      makeFileEvent(2, "foo.go"),
		MatchedSession: ptrSession(se),
		TimeDeltaMs:    500,
		MatchType:      "exact_file",
	}

	attr := c.ClassifyWithHistory(result, prior)

	// Prior was AI, current is AI -- standard classify applies (FullyAI)
	// because the history rules only trigger on author transitions
	if attr.FirstAuthor != "ai" {
		t.Errorf("FirstAuthor = %q, want %q (first author wins)", attr.FirstAuthor, "ai")
	}
}

// ---------------------------------------------------------------------------
// Authorship level constants
// ---------------------------------------------------------------------------

func TestAllAuthorshipLevels(t *testing.T) {
	levels := []AuthorshipLevel{
		FullyAI,
		AIFirstHumanRevised,
		HumanFirstAIRevised,
		AISuggestedHumanWritten,
		FullyHuman,
	}

	expected := []string{
		"fully_ai",
		"ai_first_human_revised",
		"human_first_ai_revised",
		"ai_suggested_human_written",
		"fully_human",
	}

	for i, level := range levels {
		if string(level) != expected[i] {
			t.Errorf("level[%d] = %q, want %q", i, level, expected[i])
		}
	}
}
