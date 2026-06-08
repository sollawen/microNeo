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

	// 流式填充算法：
	// 逐字符填充，当 "hello wo" (8) + 'r' 不会溢出，"hello wor" (9) + 'l' = 10 不溢出，
	// "hello worl" (10) + 'd' = 11 > 10 → 找断点：空格在 pos 5
	// Row 0: "hello " (空格在断点处，空格留在当前行) + 4 padding
	// Row 1: "world foo" + 1 padding

	row0 := rows[0]
	if len(row0.Cells) != 10 {
		t.Fatalf("row 0 cells = %d, want 10", len(row0.Cells))
	}
	// "hello " + 4 padding (空格留在断点行)
	checkRunes(t, row0.Cells, "hello     ")

	row1 := rows[1]
	if len(row1.Cells) != 10 {
		t.Fatalf("row 1 cells = %d, want 10", len(row1.Cells))
	}
	// "world foo" + 1 padding
	checkRunes(t, row1.Cells, "world foo ")

	// BufLine: 所有 row 保留真实 bufLine（下游 softwrap 检测依赖此值）
	if row0.BufLine != 0 {
		t.Errorf("row 0 BufLine = %d, want 0", row0.BufLine)
	}
	if row1.BufLine != 0 {
		t.Errorf("row 1 BufLine = %d, want 0", row1.BufLine)
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
	// "你好世界再见" = 6 CJK chars, each 2 wide = 12 display cols, width=8
	// CJK 每个字符独立可断：
	//   Row 0: 你好世 (3×2=6 cols) + 2 padding = 8 cells
	//   放第4个字 '界' 时：6+2=8, 刚好不溢出
	//   放第5个字 '再' 时：8+2=10 > 8 → CJK 直接断行
	//   Row 0: 你好世界 (4×2=8 cols) = 8 cells (4 real + 4 placeholder)
	//   Row 1: 再见 (2×2=4 cols) + 4 padding = 8 cells

	cells := makeCells("你好世界再见", 0)
	rows := wrapCells(cells, 8, 0, testStyle)

	if len(rows) != 2 {
		t.Fatalf("CJK line should produce 2 rows, got %d", len(rows))
	}

	row0 := rows[0]
	if len(row0.Cells) != 8 {
		t.Fatalf("row 0 cells = %d, want 8", len(row0.Cells))
	}
	// 你好世界 = 4 CJK chars → 4 real + 4 placeholder = 8 cells
	if row0.Cells[0].Rune != '你' {
		t.Errorf("row 0 cell[0] = %q, want '你'", row0.Cells[0].Rune)
	}

	row1 := rows[1]
	if len(row1.Cells) != 8 {
		t.Fatalf("row 1 cells = %d, want 8", len(row1.Cells))
	}
	// 再见 = 2 CJK chars → 2 real + 2 placeholder + 4 padding = 8
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

func TestWrapCells_LeadingSpaces_Preserved(t *testing.T) {
	// "   hello" → leading spaces should be preserved
	cells := makeCells("   hello", 0)
	rows := wrapCells(cells, 10, 0, testStyle)

	if len(rows) != 1 {
		t.Fatalf("should produce 1 row, got %d", len(rows))
	}
	// "   hello" + 2 padding (leading spaces preserved)
	checkRunes(t, rows[0].Cells, "   hello  ")
}

func TestWrapCells_MultipleSpaces_Preserved(t *testing.T) {
	// "hello   world" → spaces are preserved as-is (流式填充，不折叠空格)
	cells := makeCells("hello   world", 0)
	rows := wrapCells(cells, 20, 0, testStyle)

	if len(rows) != 1 {
		t.Fatalf("should produce 1 row, got %d", len(rows))
	}
	// "hello   world" + 7 padding
	checkRunes(t, rows[0].Cells, "hello   world       ")
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

	// All rows: BufLine = 3（真实 bufLine，下游 softwrap 检测依赖此值）
	for i, row := range rows {
		if row.BufLine != 3 {
			t.Errorf("row %d BufLine = %d, want 3", i, row.BufLine)
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
