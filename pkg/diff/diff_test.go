package diff

import (
	"testing"
)

func TestGenerateDiff_Identical(t *testing.T) {
	left := "line1\nline2\nline3"
	right := "line1\nline2\nline3"

	result := GenerateDiff(left, right)

	if result.Additions != 0 || result.Deletions != 0 {
		t.Errorf("Expected 0 additions and 0 deletions, got Additions=%d, Deletions=%d", result.Additions, result.Deletions)
	}

	if len(result.SplitRows) != 3 {
		t.Errorf("Expected 3 split rows, got %d", len(result.SplitRows))
	}

	for i, row := range result.SplitRows {
		if row.LeftClass != "diff-equal" || row.RightClass != "diff-equal" {
			t.Errorf("Row %d should be diff-equal, got Left=%s, Right=%s", i, row.LeftClass, row.RightClass)
		}
	}
}

func TestGenerateDiff_Edits(t *testing.T) {
	left := "line1\nline2\nline3"
	right := "line1\nline2_modified\nline3\nline4"

	result := GenerateDiff(left, right)

	// deletions: "line2" -> 1
	// additions: "line2_modified", "line4" -> 2
	if result.Deletions != 1 {
		t.Errorf("Expected 1 deletion, got %d", result.Deletions)
	}
	if result.Additions != 2 {
		t.Errorf("Expected 2 additions, got %d", result.Additions)
	}

	// In Split View, "line2" and "line2_modified" should be aligned:
	// row 0: equal
	// row 1: delete & insert (aligned)
	// row 2: equal
	// row 3: empty & insert
	if len(result.SplitRows) != 4 {
		t.Fatalf("Expected 4 split rows, got %d", len(result.SplitRows))
	}

	r1 := result.SplitRows[1]
	if r1.LeftClass != "diff-delete" || r1.RightClass != "diff-insert" {
		t.Errorf("Expected aligned edit at row 1, got Left=%s, Right=%s", r1.LeftClass, r1.RightClass)
	}
	if r1.LeftLine != "line2" || r1.RightLine != "line2_modified" {
		t.Errorf("Expected text at row 1: 'line2' vs 'line2_modified', got Left='%s', Right='%s'", r1.LeftLine, r1.RightLine)
	}

	r3 := result.SplitRows[3]
	if r3.LeftClass != "diff-empty" || r3.RightClass != "diff-insert" {
		t.Errorf("Expected insertion at row 3, got Left=%s, Right=%s", r3.LeftClass, r3.RightClass)
	}
	if r3.RightLine != "line4" {
		t.Errorf("Expected 'line4' inserted, got '%s'", r3.RightLine)
	}
}

func TestGenerateDiff_Empty(t *testing.T) {
	left := ""
	right := "hello\nworld"

	result := GenerateDiff(left, right)

	if result.Deletions != 0 {
		t.Errorf("Expected 0 deletions, got %d", result.Deletions)
	}
	if result.Additions != 2 {
		t.Errorf("Expected 2 additions, got %d", result.Additions)
	}
}
