package md

import (
	"unicode"

	"github.com/mattn/go-runewidth"
	"github.com/micro-editor/tcell/v2"
)

// wrapCells 把一行的 cells 按 width 自动换行成多个 RenderedRow。
// 被 RenderNormal / RenderList / RenderBlockquote 复用。
//
// padStyle 用于填充/占位 Cell 的样式。
//
// 算法：逐字符流式填充，填满即断。
// - 逐字符追加到当前行，累加显示宽度。
// - 当放不下时：CJK 字符直接断行；非 CJK 往回找第一个可断点（空格/标点/CJK）。
// - 每行 Cells 数量 = width。
// - 空行返回一个填充空格的 Row（不返回零个 Row）。
// - 行尾空格丢弃（不保留在行末）。
//
// BufLine 规则：Row.BufLine 始终保留真实 bufLine；
// Cell.BufLine 始终保留正确映射。Cell.BufX：真实字符保留原始偏移，填充/CJK占位为 -1。
func wrapCells(cells []Cell, width int, bufLine int, padStyle tcell.Style) []RenderedRow {
	// 空行 → 返回一个填充空格的 Row
	if len(cells) == 0 {
		return []RenderedRow{makePaddingRow(width, bufLine, padStyle)}
	}

	// 去除尾部空格
	cells = trimTrailingSpaces(cells)
	if len(cells) == 0 {
		return []RenderedRow{makePaddingRow(width, bufLine, padStyle)}
	}

	var rows []RenderedRow
	curCells := make([]Cell, 0, width)
	curWidth := 0
	isFirstRow := true

	i := 0
	for i < len(cells) {
		c := cells[i]
		rw := runewidth.RuneWidth(c.Rune)

		if curWidth+rw > width {
			// 放不下了，需要断行
			if isCJK(c.Rune) {
				// CJK：直接在当前位置断行
				rows = appendRow(rows, curCells, width, bufLine, padStyle, isFirstRow)
				isFirstRow = false
				curCells = nil
				curWidth = 0
				// 不推进 i，当前字符放到新行
			} else {
				// 非 CJK：往回找可断点
				breakPos := findBreakPoint(curCells)
				if breakPos > 0 {
					// 在断点处拆分
					rows = appendRow(rows, curCells[:breakPos], width, bufLine, padStyle, isFirstRow)
					isFirstRow = false
					// 回退 i 到断点之后未处理的字符
					i -= len(curCells) - breakPos
					curCells = nil
					curWidth = 0
					// 跳过行首空格
					for i < len(cells) && cells[i].Rune == ' ' {
						i++
					}
					continue
				} else {
					// 找不到可断点（超长英文等），硬切
					rows = appendRow(rows, curCells, width, bufLine, padStyle, isFirstRow)
					isFirstRow = false
					curCells = nil
					curWidth = 0
				}
			}
			continue
		}

		curCells = append(curCells, c)
		curWidth += rw
		i++
	}

	// 处理最后一行
	if len(curCells) > 0 {
		rows = appendRow(rows, curCells, width, bufLine, padStyle, isFirstRow)
	} else if len(rows) == 0 {
		rows = appendRow(rows, nil, width, bufLine, padStyle, isFirstRow)
	}

	return rows
}

// isCJK 判断一个 rune 是否为 CJK（中日韩）字符。
// CJK 字符可以在任意位置断行。
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r)
}

// isBreakPoint 判断 curCells 中位置 pos 处是否可以作为断行点。
// 规则：前一个字符为空格、标点或 CJK 时，在当前位置断行，
// 这样这些字符会留在当前行末尾，下一行从下一个字符开始。
func isBreakPoint(curCells []Cell, pos int) bool {
	if pos <= 0 || pos >= len(curCells) {
		return false
	}
	prev := curCells[pos-1].Rune
	return prev == ' ' || unicode.IsPunct(prev) || isCJK(prev)
}

// findBreakPoint 从 curCells 尾部往回找第一个可断点，返回断点位置。
// 返回 0 表示找不到可断点。
func findBreakPoint(curCells []Cell) int {
	for j := len(curCells) - 1; j > 0; j-- {
		if isBreakPoint(curCells, j) {
			return j
		}
	}
	return 0
}

// trimTrailingSpaces 去除 cells 尾部的空格。
func trimTrailingSpaces(cells []Cell) []Cell {
	end := len(cells)
	for end > 0 && cells[end-1].Rune == ' ' {
		end--
	}
	return cells[:end]
}

// cellDisplayWidth 计算 cells 的总显示宽度（含 CJK 宽字符）。
func cellDisplayWidth(cells []Cell) int {
	w := 0
	for _, c := range cells {
		w += runewidth.RuneWidth(c.Rune)
	}
	return w
}

// truncateToWidth 从 cells 开头截取恰好不超过 maxWidth 显示宽度的 cells。
func truncateToWidth(cells []Cell, maxWidth int) []Cell {
	var result []Cell
	col := 0
	for _, c := range cells {
		rw := runewidth.RuneWidth(c.Rune)
		if col+rw > maxWidth {
			break
		}
		result = append(result, c)
		col += rw
	}
	return result
}

// padCell 返回一个空格填充 Cell。
func padCell(bufLine int, style tcell.Style) Cell {
	return Cell{
		Rune:    ' ',
		Style:   style,
		BufLine: bufLine,
		BufX:    -1,
	}
}

// makePaddingRow 创建一个全空格填充的 Row。
func makePaddingRow(width int, bufLine int, style tcell.Style) RenderedRow {
	row := RenderedRow{
		BufLine: bufLine,
		Cells:   make([]Cell, width),
	}
	for i := range row.Cells {
		row.Cells[i] = Cell{
			Rune:    ' ',
			Style:   style,
			BufLine: bufLine,
			BufX:    -1,
		}
	}
	return row
}

// wrapCellsRaw 字符级硬切 wrap，返回每行 cells（不做尾部 padding）。
// 供表格 cell 内部使用。
// 与 wrapCellsHard 的区别：不调 appendRow（不做 padding），直接返回 [][]Cell。
func wrapCellsRaw(cells []Cell, maxWidth int) [][]Cell {
	if len(cells) == 0 {
		return [][]Cell{nil} // 一个空行
	}

	var lines [][]Cell
	var cur []Cell
	curW := 0
	for _, c := range cells {
		rw := runewidth.RuneWidth(c.Rune)
		if curW+rw > maxWidth && len(cur) > 0 {
			lines = append(lines, cur)
			cur = nil
			curW = 0
		}
		cur = append(cur, c)
		curW += rw
		// CJK 宽字符占位在 assembleRow 的 fillCellsToWidth 统一处理
	}
	if len(cur) > 0 {
		lines = append(lines, cur)
	}
	// 确保至少返回一个空行
	if len(lines) == 0 {
		lines = append(lines, nil)
	}
	return lines
}

// appendRow 把 cells 填充到 width 并追加到 rows。
// 处理 CJK 占位 Cell 和行尾填充。
func appendRow(rows []RenderedRow, cells []Cell, width int, bufLine int, padStyle tcell.Style, isFirstRow bool) []RenderedRow {
	row := RenderedRow{
		BufLine: bufLine,
		Cells:   make([]Cell, 0, width),
	}

	col := 0
	for _, c := range cells {
		rw := runewidth.RuneWidth(c.Rune)
		row.Cells = append(row.Cells, c)
		col += rw
		if rw == 2 {
			// CJK 宽字符后的占位 Cell（不额外增加 col，rw 已含两列）
			row.Cells = append(row.Cells, Cell{
				Rune:    ' ',
				Style:   padStyle,
				BufLine: c.BufLine,
				BufX:    -1,
			})
		}
	}

	// 填充到 width
	for ; col < width; col++ {
		row.Cells = append(row.Cells, Cell{
			Rune:    ' ',
			Style:   padStyle,
			BufLine: bufLine,
			BufX:    -1,
		})
	}

	return append(rows, row)
}
