package diff

import (
	"fmt"
	"strings"
)

// SideBySideRow represents a single line comparison in a side-by-side (split) diff view.
type SideBySideRow struct {
	LeftNum    string // Line number in the base version (empty if none)
	LeftLine   string // Content of the line in the base version
	LeftClass  string // CSS styling class ("diff-delete", "diff-equal", "diff-empty")
	RightNum   string // Line number in the target version (empty if none)
	RightLine  string // Content of the line in the target version
	RightClass string // CSS styling class ("diff-insert", "diff-equal", "diff-empty")
}

// InlineRow represents a single line in a unified (inline) diff view.
type InlineRow struct {
	Num   string // Line number (corresponds to whichever side is active)
	Sign  string // Operation marker: " ", "-", or "+"
	Line  string // Content of the line
	Class string // CSS styling class ("diff-delete", "diff-insert", "diff-equal")
}

// DiffResult holds the computed split and unified representations along with total stats.
type DiffResult struct {
	SplitRows   []SideBySideRow
	UnifiedRows []InlineRow
	Additions   int
	Deletions   int
}

// splitLines converts content string into a slice of lines, normalizing newlines.
func splitLines(content string) []string {
	if len(content) == 0 {
		return []string{}
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "") // Strip stray CR from SSH
	return strings.Split(content, "\n")
}

// GenerateDiff computes a visual diff between two strings.
func GenerateDiff(leftContent, rightContent string) DiffResult {
	leftLines := splitLines(leftContent)
	rightLines := splitLines(rightContent)

	m, n := len(leftLines), len(rightLines)

	// Standard dynamic programming table for LCS
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if leftLines[i-1] == rightLines[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				if dp[i-1][j] > dp[i][j-1] {
					dp[i][j] = dp[i-1][j]
				} else {
					dp[i][j] = dp[i][j-1]
				}
			}
		}
	}

	// Backtracking structure
	type rawRow struct {
		leftNum   int
		leftLine  string
		leftType  string // "equal", "delete", "empty"
		rightNum  int
		rightLine string
		rightType string // "equal", "insert", "empty"
	}

	var rawRows []rawRow
	i, j := m, n
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && leftLines[i-1] == rightLines[j-1] {
			rawRows = append(rawRows, rawRow{
				leftNum:   i,
				leftLine:  leftLines[i-1],
				leftType:  "equal",
				rightNum:  j,
				rightLine: rightLines[j-1],
				rightType: "equal",
			})
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			rawRows = append(rawRows, rawRow{
				leftNum:   0,
				leftLine:  "",
				leftType:  "empty",
				rightNum:  j,
				rightLine: rightLines[j-1],
				rightType: "insert",
			})
			j--
		} else if i > 0 {
			rawRows = append(rawRows, rawRow{
				leftNum:   i,
				leftLine:  leftLines[i-1],
				leftType:  "delete",
				rightNum:  0,
				rightLine: "",
				rightType: "empty",
			})
			i--
		}
	}

	// Reverse to restore top-down line ordering
	for l, r := 0, len(rawRows)-1; l < r; l, r = l+1, r-1 {
		rawRows[l], rawRows[r] = rawRows[r], rawRows[l]
	}

	// 1. Build Unified Rows & count additions/deletions
	var unifiedRows []InlineRow
	var additions, deletions int
	for _, r := range rawRows {
		if r.leftType == "delete" {
			deletions++
			unifiedRows = append(unifiedRows, InlineRow{
				Num:   fmt.Sprintf("%d", r.leftNum),
				Sign:  "-",
				Line:  r.leftLine,
				Class: "diff-delete",
			})
		} else if r.rightType == "insert" {
			additions++
			unifiedRows = append(unifiedRows, InlineRow{
				Num:   fmt.Sprintf("%d", r.rightNum),
				Sign:  "+",
				Line:  r.rightLine,
				Class: "diff-insert",
			})
		} else {
			unifiedRows = append(unifiedRows, InlineRow{
				Num:   fmt.Sprintf("%d", r.leftNum),
				Sign:  " ",
				Line:  r.leftLine,
				Class: "diff-equal",
			})
		}
	}

	// 2. Build Split (Side-by-Side) Rows with post-processing alignment
	var splitRows []SideBySideRow
	rawLen := len(rawRows)
	for k := 0; k < rawLen; k++ {
		// Align consecutive deleted line followed immediately by inserted line
		if k < rawLen-1 && rawRows[k].leftType == "delete" && rawRows[k].rightType == "empty" &&
			rawRows[k+1].leftType == "empty" && rawRows[k+1].rightType == "insert" {

			splitRows = append(splitRows, SideBySideRow{
				LeftNum:    fmt.Sprintf("%d", rawRows[k].leftNum),
				LeftLine:   rawRows[k].leftLine,
				LeftClass:  "diff-delete",
				RightNum:   fmt.Sprintf("%d", rawRows[k+1].rightNum),
				RightLine:  rawRows[k+1].rightLine,
				RightClass: "diff-insert",
			})
			k++
		} else {
			row := SideBySideRow{}
			if rawRows[k].leftType == "delete" {
				row.LeftNum = fmt.Sprintf("%d", rawRows[k].leftNum)
				row.LeftLine = rawRows[k].leftLine
				row.LeftClass = "diff-delete"
				row.RightClass = "diff-empty"
			} else if rawRows[k].rightType == "insert" {
				row.RightNum = fmt.Sprintf("%d", rawRows[k].rightNum)
				row.RightLine = rawRows[k].rightLine
				row.RightClass = "diff-insert"
				row.LeftClass = "diff-empty"
			} else {
				row.LeftNum = fmt.Sprintf("%d", rawRows[k].leftNum)
				row.LeftLine = rawRows[k].leftLine
				row.LeftClass = "diff-equal"
				row.RightNum = fmt.Sprintf("%d", rawRows[k].rightNum)
				row.RightLine = rawRows[k].rightLine
				row.RightClass = "diff-equal"
			}
			splitRows = append(splitRows, row)
		}
	}

	return DiffResult{
		SplitRows:   splitRows,
		UnifiedRows: unifiedRows,
		Additions:   additions,
		Deletions:   deletions,
	}
}
