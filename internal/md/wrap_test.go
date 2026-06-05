package md

import (
	"testing"

	"github.com/micro-editor/tcell/v2"
)

// testStyle is a default style used in tests.
var testStyle = tcell.StyleDefault

// helper: make a cell with given rune, bufLine and bufX
func makeCell(r rune, bufLine, bufX int) Cell {
	return Cell{
		Rune:    r,
		Style:   testStyle,
		BufLine: bufLine,
		BufX:    bufX,
	}
}

// helper: make cells from a string
func makeCells(s string, bufLine int) []Cell {
	cells := make([]Cell, 0, len(s))
	for x, r := range s {
		cells = append(cells, makeCell(r, bufLine, x))
	}
	return cells
}

func TestWrapCells_EmptyLine(t *testing.T) {
	rows := wrapCells(nil, 10, 0, testStyle)

	if len(rows) != 1 {
		t.Fatalf("empty line should produce 1 row, got %d", len(rows))
	}
	row := rows[0]
	if len(row.Cells) != 10 {
		t.Fatalf("row cells count = %d, want 10", len(row.Cells))
	}
	if row.BufLine != 0 {
		t.Fatalf("row BufLine = %d, want 0", row.BufLine)
	}
	// All cells should be spaces
	for i, c := range row.Cells {
		if c.Rune != ' ' {
			t.Errorf("cell[%d] rune = %q, want space", i, c.Rune)
		}
		if c.BufX != -1 {
			t.Errorf("cell[%d] BufX = %d, want -1 (padding)", i, c.BufX)
		}
	}
}

func TestWrapCells_ShortLine_NoWrap(t *testing.T) {
	cells := makeCells("hello", 0)
	rows := wrapCells(cells, 10, 0, testStyle)

	if len(rows) != 1 {
		t.Fatalf("short line should produce 1 row, got %d", len(rows))
	}
	row := rows[0]
	if len(row.Cells) != 10 {
		t.Fatalf("row cells count = %d, want 10", len(row.Cells))
	}
	// First 5 cells should be "hello"
	for i, r := range "hello" {
		if row.Cells[i].Rune != r {
			t.Errorf("cell[%d] rune = %q, want %q", i, row.Cells[i].Rune, r)
		}
		if row.Cells[i].BufX != i {
			t.Errorf("cell[%d] BufX = %d, want %d", i, row.Cells[i].BufX, i)
		}
	}
	// Remaining cells should be padding spaces
	for i := 5; i < 10; i++ {
		if row.Cells[i].Rune != ' ' {
			t.Errorf("padding cell[%d] rune = %q, want space", i, row.Cells[i].Rune)
		}
		if row.Cells[i].BufX != -1 {
			t.Errorf("padding cell[%d] BufX = %d, want -1", i, row.Cells[i].BufX)
		}
	}
	// Row BufLine = 0 (first row)
	if row.BufLine != 0 {
		t.Errorf("row BufLine = %d, want 0", row.BufLine)
	}
}

func TestWrapCells_LongLine_Wraps(t *testing.T) {
	// "hello world foo" = 15 chars, width=10 → wraps to 2 rows
	cells := makeCells("hello world foo", 0)
	rows := wrapCells(cells, 10, 0, testStyle)

	if len(rows) != 2 {
		t.Fatalf("long line should produce 2 rows, got %d", len(rows))
	}

	// Row 0: "hello worl" → "hello" + space + "world" = 11, too wide
	// Actually: "hello" (5) + space (1) = 6, "world" (5) = 11, > 10
	// So row 0 = "hello" + padding, row 1 = "world" + space + "foo" + padding
	// Wait: let me recalculate.
	// Words: ["hello", "world", "foo"]
	// Row 0: "hello" (5) → try "world": 5+1+5=11 > 10 → output "hello" + 5 padding
	// Row 1: "world" (5) → try "foo": 5+1+3=9 <= 10 → "world foo" + 1 padding

	row0 := rows[0]
	if len(row0.Cells) != 10 {
		t.Fatalf("row 0 cells = %d, want 10", len(row0.Cells))
	}
	// "hello" + 5 padding
	checkRunes(t, row0.Cells, "hello     ")

	row1 := rows[1]
	if len(row1.Cells) != 10 {
		t.Fatalf("row 1 cells = %d, want 10", len(row1.Cells))
	}
	// "world" + space + "foo" + 1 padding
	checkRunes(t, row1.Cells, "world foo ")

	// BufLine: row 0 = 0 (first), row 1 = -1 (continuation)
	if row0.BufLine != 0 {
		t.Errorf("row 0 BufLine = %d, want 0", row0.BufLine)
	}
	if row1.BufLine != -1 {
		t.Errorf("row 1 BufLine = %d, want -1", row1.BufLine)
	}

	// Cell BufLine should always be 0
	for i, c := range row0.Cells {
		if c.BufLine != 0 {
			t.Errorf("row0 cell[%d] BufLine = %d, want 0", i, c.BufLine)
		}
	}
	for i, c := range row1.Cells {
		if c.BufLine != 0 {
			t.Errorf("row1 cell[%d] BufLine = %d, want 0", i, c.BufLine)
		}
	}
}

func TestWrapCells_CJK_ChineseLine(t *testing.T) {
	// Chinese characters are wide (2 columns each), no spaces → each char is a "word"
	// "你好世界再见" = 6 chars × 2 col = 12 col, width=8
	// Expected: row 0 = "你好世" (6 col) + 2 padding, row 1 = "界再见" (6 col) + 2 padding
	// Wait: each char is 2 col wide. Words are split by spaces, but Chinese has no spaces.
	// With no spaces, the entire string is one "word" → force truncate by character.

	cells := makeCells("你好世界再见", 0)
	rows := wrapCells(cells, 8, 0, testStyle)

	// "你好世界再见" = 6 CJK chars, each 2 wide = 12 display cols, width=8
	// As one word (no spaces), truncateToWidth gives us 4 chars (8 cols) first row
	// Then 2 chars (4 cols) + 4 padding second row
	if len(rows) != 2 {
		t.Fatalf("CJK line should produce 2 rows, got %d", len(rows))
	}

	row0 := rows[0]
	if len(row0.Cells) != 8 {
		t.Fatalf("row 0 cells = %d, want 8", len(row0.Cells))
	}
	// 4 CJK chars + 4 CJK padding cells = 8 cells... but wait, the CJK chars themselves
	// need placeholder cells. Let me re-check: each CJK char produces 2 cells in appendRow.
	// "你好世界" = 4 chars × 2 = 8 cells already = width. No padding needed.
	// Actually: 4 CJK chars → 4 real cells + 4 placeholder cells = 8 cells. Perfect fit.
	if row0.Cells[0].Rune != '你' {
		t.Errorf("row 0 cell[0] = %q, want '你'", row0.Cells[0].Rune)
	}

	row1 := rows[1]
	if len(row1.Cells) != 8 {
		t.Fatalf("row 1 cells = %d, want 8", len(row1.Cells))
	}
	// "再见" = 2 CJK chars → 2 real + 2 placeholder = 4 cells + 4 padding = 8
	if row1.Cells[0].Rune != '再' {
		t.Errorf("row 1 cell[0] = %q, want '再'", row1.Cells[0].Rune)
	}
}

func TestWrapCells_TrailingSpaces_Discarded(t *testing.T) {
	// "hello   " → trailing spaces should be discarded
	cells := makeCells("hello   ", 0)
	rows := wrapCells(cells, 10, 0, testStyle)

	if len(rows) != 1 {
		t.Fatalf("should produce 1 row, got %d", len(rows))
	}
	row := rows[0]
	if len(row.Cells) != 10 {
		t.Fatalf("row cells = %d, want 10", len(row.Cells))
	}
	// "hello" + 5 padding (trailing spaces discarded, then padded to width)
	checkRunes(t, row.Cells, "hello     ")

	// Verify BufX: first 5 cells have real BufX, rest are -1
	for i := 0; i < 5; i++ {
		if row.Cells[i].BufX != i {
			t.Errorf("cell[%d] BufX = %d, want %d", i, row.Cells[i].BufX, i)
		}
	}
	for i := 5; i < 10; i++ {
		if row.Cells[i].BufX != -1 {
			t.Errorf("padding cell[%d] BufX = %d, want -1", i, row.Cells[i].BufX)
		}
	}
}

func TestWrapCells_LeadingSpaces_Discarded(t *testing.T) {
	// "   hello" → leading spaces should be discarded
	cells := makeCells("   hello", 0)
	rows := wrapCells(cells, 10, 0, testStyle)

	if len(rows) != 1 {
		t.Fatalf("should produce 1 row, got %d", len(rows))
	}
	// "hello" + 5 padding (leading spaces discarded)
	checkRunes(t, rows[0].Cells, "hello     ")
}

func TestWrapCells_MultipleSpaces_Collapsed(t *testing.T) {
	// "hello   world" → multiple spaces between words become single space
	cells := makeCells("hello   world", 0)
	rows := wrapCells(cells, 20, 0, testStyle)

	if len(rows) != 1 {
		t.Fatalf("should produce 1 row, got %d", len(rows))
	}
	// "hello world" + 9 padding
	checkRunes(t, rows[0].Cells, "hello world         ")
}

func TestWrapCells_RowCellCount(t *testing.T) {
	// Test various widths to ensure every row has exactly `width` cells
	cells := makeCells("the quick brown fox jumps over the lazy dog", 0)
	for width := 5; width <= 20; width++ {
		rows := wrapCells(cells, width, 0, testStyle)
		for i, row := range rows {
			if len(row.Cells) != width {
				t.Errorf("width=%d row[%d] has %d cells, want %d", width, i, len(row.Cells), width)
			}
		}
	}
}

func TestWrapCells_BufLineRules(t *testing.T) {
	// Multi-line wrap: verify BufLine rules
	cells := makeCells("hello world foo bar baz", 3) // bufLine = 3
	rows := wrapCells(cells, 10, 3, testStyle)

	if len(rows) < 2 {
		t.Fatalf("expected at least 2 rows, got %d", len(rows))
	}

	// First row: BufLine = 3
	if rows[0].BufLine != 3 {
		t.Errorf("row 0 BufLine = %d, want 3", rows[0].BufLine)
	}
	// Continuation rows: BufLine = -1
	for i := 1; i < len(rows); i++ {
		if rows[i].BufLine != -1 {
			t.Errorf("row %d BufLine = %d, want -1", i, rows[i].BufLine)
		}
	}
	// All cells should have BufLine = 3
	for i, row := range rows {
		for j, c := range row.Cells {
			if c.BufLine != 3 {
				t.Errorf("row[%d] cell[%d] BufLine = %d, want 3", i, j, c.BufLine)
			}
		}
	}
}

// checkRunes verifies that cells' runes match the expected string.
func checkRunes(t *testing.T, cells []Cell, expected string) {
	t.Helper()
	for i, r := range expected {
		if i >= len(cells) {
			t.Errorf("expected rune %q at position %d but cells has only %d cells", r, i, len(cells))
			return
		}
		if cells[i].Rune != r {
			t.Errorf("cell[%d] rune = %q, want %q", i, cells[i].Rune, r)
		}
	}
}
