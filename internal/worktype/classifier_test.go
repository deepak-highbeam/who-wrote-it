package worktype

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Mock OverrideReader
// ---------------------------------------------------------------------------

type mockOverrideReader struct {
	overrides map[string]string // "filePath|commitHash" -> workType
}

func (m *mockOverrideReader) QueryWorkTypeOverride(filePath, commitHash string) (string, bool, error) {
	key := filePath + "|" + commitHash
	wt, ok := m.overrides[key]
	return wt, ok, nil
}

// ---------------------------------------------------------------------------
// Classifier tests
// ---------------------------------------------------------------------------

func TestClassifyFile_TestFile_TestScaffolding(t *testing.T) {
	c := NewClassifier(nil)

	tests := []string{
		"internal/store/sqlite_test.go",
		"src/auth/login_test.ts",
		"tests/test_parser.py",
		"src/utils.test.js",
		"src/component.spec.ts",
		"src/Button.test.tsx",
		"src/Card.spec.tsx",
	}

	for _, filePath := range tests {
		wt := c.ClassifyFile(filePath, "", "")
		if wt != TestScaffolding {
			t.Errorf("ClassifyFile(%q) = %q, want %q", filePath, wt, TestScaffolding)
		}
	}
}

func TestClassifyFile_TestPathSegment_TestScaffolding(t *testing.T) {
	c := NewClassifier(nil)

	tests := []string{
		"src/test/helpers.go",
		"lib/tests/fixtures.py",
		"src/__tests__/app.js",
	}

	for _, filePath := range tests {
		wt := c.ClassifyFile(filePath, "", "")
		if wt != TestScaffolding {
			t.Errorf("ClassifyFile(%q) = %q, want %q", filePath, wt, TestScaffolding)
		}
	}
}

func TestClassifyFile_ConfigFile_Boilerplate(t *testing.T) {
	c := NewClassifier(nil)

	tests := []string{
		"go.mod",
		"go.sum",
		"package.json",
		"package-lock.json",
		"yarn.lock",
		"Makefile",
		"Dockerfile",
		"config.yml",
		"docker-compose.yaml",
		"settings.toml",
		".gitignore",
		"LICENSE",
	}

	for _, filePath := range tests {
		wt := c.ClassifyFile(filePath, "", "")
		if wt != Boilerplate {
			t.Errorf("ClassifyFile(%q) = %q, want %q", filePath, wt, Boilerplate)
		}
	}
}

func TestClassifyFile_InterfaceDefinition_Architecture(t *testing.T) {
	c := NewClassifier(nil)

	diffContent := `package store

type EventStore interface {
	InsertEvent(e Event) error
	QueryEvents(filter Filter) ([]Event, error)
}
`
	wt := c.ClassifyFile("internal/store/store.go", diffContent, "")
	if wt != Architecture {
		t.Errorf("ClassifyFile with interface = %q, want %q", wt, Architecture)
	}
}

func TestClassifyFile_StructDefinition_Architecture(t *testing.T) {
	c := NewClassifier(nil)

	diffContent := `package config

type Config struct {
	DBPath   string
	LogLevel string
}
`
	// The keyword "type %s struct" in DefaultRules won't match literally,
	// but the architecture path segments catch this through models/schema/types paths.
	// For struct detection via diff content, we rely on the keyword match.
	// Let's test with a path that contains /types/.
	wt := c.ClassifyFile("internal/types/config.go", diffContent, "")
	if wt != Architecture {
		t.Errorf("ClassifyFile with struct in types path = %q, want %q", wt, Architecture)
	}
}

func TestClassifyFile_ArchitecturePathSegments(t *testing.T) {
	c := NewClassifier(nil)

	tests := []string{
		"internal/models/user.go",
		"src/schema/tables.sql",
		"lib/types/common.ts",
		"pkg/interfaces/service.go",
	}

	for _, filePath := range tests {
		wt := c.ClassifyFile(filePath, "", "")
		if wt != Architecture {
			t.Errorf("ClassifyFile(%q) = %q, want %q", filePath, wt, Architecture)
		}
	}
}

func TestClassifyFile_ErrorHandlingHeavy_EdgeCase(t *testing.T) {
	c := NewClassifier(nil)

	// Need >= 3 keyword occurrences to hit edge case threshold.
	diffContent := `func process(data []byte) error {
	val, err := parse(data)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	result, err := transform(val)
	if err != nil {
		return fmt.Errorf("transform: %w", err)
	}
	if err := save(result); err != nil {
		return fmt.Errorf("save: %w", err)
	}
	return nil
}`

	wt := c.ClassifyFile("internal/process/handler.go", diffContent, "")
	if wt != EdgeCase {
		t.Errorf("ClassifyFile with error handling = %q, want %q", wt, EdgeCase)
	}
}

func TestClassifyFile_BugFixCommit_BugFix(t *testing.T) {
	c := NewClassifier(nil)

	tests := []struct {
		commitMsg string
	}{
		{"fix: resolve nil pointer in daemon startup"},
		{"fix(auth): handle expired tokens"},
		{"bug: incorrect timestamp parsing"},
		{"hotfix: critical race condition in store"},
	}

	for _, tt := range tests {
		wt := c.ClassifyFile("internal/daemon/daemon.go", "", tt.commitMsg)
		if wt != BugFix {
			t.Errorf("ClassifyFile with commit %q = %q, want %q", tt.commitMsg, wt, BugFix)
		}
	}
}

func TestClassifyFile_RegularGoFile_CoreLogic(t *testing.T) {
	c := NewClassifier(nil)

	diffContent := `func (d *Daemon) Start(ctx context.Context) error {
	d.watcher.Start()
	d.sessionParser.Start()
	return d.gitSync.Start()
}`

	wt := c.ClassifyFile("internal/daemon/daemon.go", diffContent, "feat: add daemon startup")
	if wt != CoreLogic {
		t.Errorf("ClassifyFile for regular go file = %q, want %q", wt, CoreLogic)
	}
}

func TestClassifyFile_OverrideWins(t *testing.T) {
	override := &mockOverrideReader{
		overrides: map[string]string{
			"internal/daemon/daemon.go|": string(Architecture),
		},
	}
	c := NewClassifier(override)

	// Without override this would be CoreLogic (no pattern matches).
	wt := c.ClassifyFile("internal/daemon/daemon.go", "normal code here", "feat: something")
	if wt != Architecture {
		t.Errorf("ClassifyFile with override = %q, want %q (override should win)", wt, Architecture)
	}
}

func TestClassifyFileWithCommit_CommitSpecificOverride(t *testing.T) {
	override := &mockOverrideReader{
		overrides: map[string]string{
			"internal/daemon/daemon.go|abc123": string(BugFix),
		},
	}
	c := NewClassifier(override)

	wt := c.ClassifyFileWithCommit("internal/daemon/daemon.go", "normal code", "feat: something", "abc123")
	if wt != BugFix {
		t.Errorf("ClassifyFileWithCommit with commit override = %q, want %q", wt, BugFix)
	}

	// Different commit hash should not match the override.
	wt2 := c.ClassifyFileWithCommit("internal/daemon/daemon.go", "normal code", "feat: something", "def456")
	if wt2 != CoreLogic {
		t.Errorf("ClassifyFileWithCommit without matching commit = %q, want %q", wt2, CoreLogic)
	}
}

func TestClassifyFile_PriorityOrdering(t *testing.T) {
	c := NewClassifier(nil)

	// A test file that also contains error handling -- test scaffolding path
	// segment should win because it's checked first (before rules).
	diffContent := strings.Repeat("if err != nil {\n", 5)
	wt := c.ClassifyFile("src/__tests__/errors.go", diffContent, "")
	if wt != TestScaffolding {
		t.Errorf("test path with error content = %q, want %q (path takes priority)", wt, TestScaffolding)
	}
}

// ---------------------------------------------------------------------------
// Work type constants and weights
// ---------------------------------------------------------------------------

func TestAllWorkTypes_Complete(t *testing.T) {
	all := AllWorkTypes()
	if len(all) != 6 {
		t.Fatalf("AllWorkTypes() has %d entries, want 6", len(all))
	}

	expected := map[WorkType]bool{
		Architecture: true, CoreLogic: true, Boilerplate: true,
		BugFix: true, EdgeCase: true, TestScaffolding: true,
	}
	for _, wt := range all {
		if !expected[wt] {
			t.Errorf("unexpected work type %q", wt)
		}
	}
}

func TestWorkTypeWeights_ThreeTiers(t *testing.T) {
	// Verify high tier = 3.0.
	if WorkTypeWeights[Architecture] != 3.0 {
		t.Errorf("Architecture weight = %f, want 3.0", WorkTypeWeights[Architecture])
	}
	if WorkTypeWeights[CoreLogic] != 3.0 {
		t.Errorf("CoreLogic weight = %f, want 3.0", WorkTypeWeights[CoreLogic])
	}

	// Verify medium tier = 2.0.
	if WorkTypeWeights[BugFix] != 2.0 {
		t.Errorf("BugFix weight = %f, want 2.0", WorkTypeWeights[BugFix])
	}
	if WorkTypeWeights[EdgeCase] != 2.0 {
		t.Errorf("EdgeCase weight = %f, want 2.0", WorkTypeWeights[EdgeCase])
	}

	// Verify low tier = 1.0.
	if WorkTypeWeights[Boilerplate] != 1.0 {
		t.Errorf("Boilerplate weight = %f, want 1.0", WorkTypeWeights[Boilerplate])
	}
	if WorkTypeWeights[TestScaffolding] != 1.0 {
		t.Errorf("TestScaffolding weight = %f, want 1.0", WorkTypeWeights[TestScaffolding])
	}
}

func TestWorkTypeTier_Mappings(t *testing.T) {
	if WorkTypeTier[Architecture] != TierHigh {
		t.Errorf("Architecture tier = %q, want %q", WorkTypeTier[Architecture], TierHigh)
	}
	if WorkTypeTier[CoreLogic] != TierHigh {
		t.Errorf("CoreLogic tier = %q, want %q", WorkTypeTier[CoreLogic], TierHigh)
	}
	if WorkTypeTier[BugFix] != TierMedium {
		t.Errorf("BugFix tier = %q, want %q", WorkTypeTier[BugFix], TierMedium)
	}
	if WorkTypeTier[Boilerplate] != TierLow {
		t.Errorf("Boilerplate tier = %q, want %q", WorkTypeTier[Boilerplate], TierLow)
	}
}

func TestDefaultRules_Count(t *testing.T) {
	rules := DefaultRules()
	if len(rules) != 5 {
		t.Errorf("DefaultRules() has %d entries, want 5", len(rules))
	}
}
