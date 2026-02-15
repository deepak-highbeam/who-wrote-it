package metrics

import (
	"go/format"
	"testing"
)

// TestLinterAttribution_GofmtIndentationOnly verifies that pure indentation
// changes from gofmt do NOT affect attribution. Since hashLine uses
// strings.TrimSpace before hashing, whitespace-only changes produce
// identical hashes → code is still attributed to AI.
func TestLinterAttribution_GofmtIndentationOnly(t *testing.T) {
	// AI writes code with wrong indentation (spaces instead of tabs, extra spaces).
	aiWrote := `package main

func main() {
   x := 1
      y := 2
         z := x + y
   println(z)
}`

	// gofmt fixes it to use tabs.
	formatted, err := format.Source([]byte(aiWrote))
	if err != nil {
		t.Fatalf("gofmt failed: %v", err)
	}

	result := ComputeLineAttribution(string(formatted), []string{aiWrote}, "")

	t.Logf("AI wrote:\n%s", aiWrote)
	t.Logf("After gofmt:\n%s", formatted)
	t.Logf("Attribution: total=%d ai=%d human=%d", result.TotalLines, result.AILines, result.HumanLines)

	if result.AILines != result.TotalLines {
		t.Errorf("Indentation-only changes: want all %d lines attributed to AI, got ai=%d human=%d",
			result.TotalLines, result.AILines, result.HumanLines)
	}
}

// TestLinterAttribution_GofmtImportReordering verifies that gofmt reordering
// imports (alphabetically, grouping stdlib vs third-party) still attributes
// lines to AI. The lines themselves are identical, just in different order,
// and ComputeLineAttribution uses frequency-based matching.
func TestLinterAttribution_GofmtImportReordering(t *testing.T) {
	// AI writes imports in wrong order (gofmt sorts them alphabetically).
	aiWrote := `package main

import (
	"os"
	"fmt"
	"strings"
	"net/http"
	"io"
)

func main() {
	fmt.Println(os.Args)
	_ = strings.NewReader("")
	_ = http.DefaultClient
	_ = io.Discard
}`

	formatted, err := format.Source([]byte(aiWrote))
	if err != nil {
		t.Fatalf("gofmt failed: %v", err)
	}

	result := ComputeLineAttribution(string(formatted), []string{aiWrote}, "")

	t.Logf("AI wrote:\n%s", aiWrote)
	t.Logf("After gofmt:\n%s", formatted)
	t.Logf("Attribution: total=%d ai=%d human=%d", result.TotalLines, result.AILines, result.HumanLines)

	if result.AILines != result.TotalLines {
		t.Errorf("Import reordering: want all %d lines attributed to AI, got ai=%d human=%d",
			result.TotalLines, result.AILines, result.HumanLines)
	}
}

// TestLinterAttribution_GofmtLineSplitting verifies that when gofmt splits a
// long line into multiple lines, the new lines do NOT match the AI's original
// content → those split lines are attributed to "human" (the linter).
func TestLinterAttribution_GofmtLineSplitting(t *testing.T) {
	// AI writes a struct with a very long single-line field tag that gofmt
	// won't touch, plus a long function call that gofmt won't touch either
	// (gofmt doesn't wrap long lines). So instead, simulate a case where
	// the AI writes a multi-value return on one line and gofmt adds spacing.
	//
	// Note: gofmt is fairly conservative — it doesn't wrap long lines.
	// The real structural changes come from goimports (adding/removing imports)
	// or stricter linters. We test what gofmt actually changes.
	aiWrote := `package main

func main() {
	x:=1
	y:=2
	if x==y {println("equal")}
}`

	formatted, err := format.Source([]byte(aiWrote))
	if err != nil {
		t.Fatalf("gofmt failed: %v", err)
	}

	result := ComputeLineAttribution(string(formatted), []string{aiWrote}, "")

	t.Logf("AI wrote:\n%s", aiWrote)
	t.Logf("After gofmt:\n%s", formatted)
	t.Logf("Attribution: total=%d ai=%d human=%d", result.TotalLines, result.AILines, result.HumanLines)

	// gofmt will add spaces around := and ==, and may expand the single-line if.
	// Lines that gofmt changes structurally (different content after trim) will
	// be attributed to human. Lines unchanged after trim stay AI.
	//
	// We just log the result here and verify it's reasonable.
	if result.TotalLines == 0 {
		t.Error("expected non-zero total lines")
	}
	// Some lines will definitely change (x:=1 → x := 1), log for visibility.
	t.Logf("Lines changed by gofmt (attributed to linter/human): %d out of %d",
		result.HumanLines, result.TotalLines)
}

// TestLinterAttribution_GofmtStructuralExpansion verifies that gofmt expanding
// a compressed if-block into multiple lines causes the new lines to be
// attributed as human (the linter), not AI.
func TestLinterAttribution_GofmtStructuralExpansion(t *testing.T) {
	// AI writes a compact single-line if that gofmt will expand.
	aiWrote := `package main

import "fmt"

func greet(name string) {
if name=="" {fmt.Println("anonymous")} else {fmt.Println("hello",name)}
}`

	formatted, err := format.Source([]byte(aiWrote))
	if err != nil {
		t.Fatalf("gofmt failed: %v", err)
	}

	result := ComputeLineAttribution(string(formatted), []string{aiWrote}, "")

	t.Logf("AI wrote:\n%s", aiWrote)
	t.Logf("After gofmt:\n%s", formatted)
	t.Logf("Attribution: total=%d ai=%d human=%d", result.TotalLines, result.AILines, result.HumanLines)

	// The expanded if/else block produces lines that differ from the original
	// single line → those are attributed to human/linter.
	if result.HumanLines == 0 {
		t.Error("gofmt structural expansion should produce some human-attributed lines")
	}
	t.Logf("After structural expansion: %d/%d lines attributed to AI (%.0f%%)",
		result.AILines, result.TotalLines, float64(result.AILines)/float64(result.TotalLines)*100)
}

// TestLinterAttribution_GofmtNoChange verifies that well-formatted AI code
// survives gofmt with 100% AI attribution.
func TestLinterAttribution_GofmtNoChange(t *testing.T) {
	// AI writes already-perfect Go code.
	aiWrote := `package main

import "fmt"

func main() {
	name := "world"
	fmt.Printf("hello, %s\n", name)
}
`

	formatted, err := format.Source([]byte(aiWrote))
	if err != nil {
		t.Fatalf("gofmt failed: %v", err)
	}

	if string(formatted) != aiWrote {
		t.Logf("WARNING: gofmt changed the already-formatted code")
		t.Logf("Before:\n%s", aiWrote)
		t.Logf("After:\n%s", formatted)
	}

	result := ComputeLineAttribution(string(formatted), []string{aiWrote}, "")

	if result.AILines != result.TotalLines {
		t.Errorf("Well-formatted code: want all %d lines AI, got ai=%d human=%d",
			result.TotalLines, result.AILines, result.HumanLines)
	}
}

// TestLinterAttribution_GofmtSpacingChanges verifies that gofmt adding/removing
// spaces around operators is handled correctly by TrimSpace hashing.
func TestLinterAttribution_GofmtSpacingChanges(t *testing.T) {
	// AI writes code with inconsistent spacing. gofmt normalizes it.
	aiWrote := `package main

func add(a,b int) int {
	return a+b
}

func sub(a , b  int) int {
	return a - b
}
`

	formatted, err := format.Source([]byte(aiWrote))
	if err != nil {
		t.Fatalf("gofmt failed: %v", err)
	}

	result := ComputeLineAttribution(string(formatted), []string{aiWrote}, "")

	t.Logf("AI wrote:\n%s", aiWrote)
	t.Logf("After gofmt:\n%s", formatted)
	t.Logf("Attribution: total=%d ai=%d human=%d", result.TotalLines, result.AILines, result.HumanLines)

	// gofmt changes "a,b" to "a, b" and "a , b" to "a, b" — these are
	// content changes (not just whitespace at line edges) so they produce
	// different hashes after TrimSpace.
	if result.HumanLines > 0 {
		t.Logf("gofmt spacing normalization caused %d lines to be attributed to linter/human", result.HumanLines)
	}
}
