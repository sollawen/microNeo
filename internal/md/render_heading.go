package md

import (
	"github.com/mattn/go-runewidth"
	"github.com/micro-editor/tcell/v2"
)

// RenderHeading 渲染标题行。
// 规则：
// 1. `#` 号不隐藏，原样显示
// 2. 标题行全行粗体（包括 `#` 号）
// 3. 标题下方加装饰横线（灰色，不加粗）
func RenderHeading(seg Segment, width int, cfg MDConfig) *RenderedSegment {
	lines := linesFromBuf(cfg.Buf, seg.BufStartLine, seg.BufEndLine)
	// MDHeading 关闭时退化为普通文本渲染
	if !cfg.MDHeading {
		return RenderNormal(seg, width, cfg)
	}

	result := &RenderedSegment{}

	// 颜色由 highlighter 决定，这里只设置粗体属性
	baseStyle := tcell.StyleDefault.Bold(true)

	// 装饰横线样式：从 colorscheme 查 md-frame，不加粗
	_, defBg2, _ := cfg.Colorscheme.DefStyle.Decompose()
	decoStyle := resolveStyle(cfg.Colorscheme.Styles, "md-frame", cfg.Colorscheme.DefStyle).Background(defBg2)

	for lineIdx, line := range lines {
		// 处理行内标记（粗体/斜体/代码等），叠加在粗体基础上
		cells := renderInline(line, baseStyle, cfg, lineIdx)
		// Word wrap + padding
		rows := wrapCells(cells, width, lineIdx, baseStyle)
		result.Rows = append(result.Rows, rows...)

		// 找所有 wrap row 中最长的可视宽度（去掉尾部 padding）
		maxWidth := 0
		for _, row := range rows {
			w := rowContentWidth(row)
			if w > maxWidth {
				maxWidth = w
			}
		}

		// 添加装饰横线
		decoRow := makeDecoRow(maxWidth, width, decoStyle)
		result.Rows = append(result.Rows, decoRow)
	}

	return result
}

// rowContentWidth 计算一行去掉尾部 padding 空格后的可视宽度。
// padding 空格 Cell 的 BufX == -1。
// 不能用 cellDisplayWidth，因为它会把 CJK 占位空格也算成 1 列。
func rowContentWidth(row RenderedRow) int {
	// 从右往左跳过 padding 空格（BufX == -1 的空格）
	end := len(row.Cells)
	for end > 0 && row.Cells[end-1].Rune == ' ' && row.Cells[end-1].BufX == -1 {
		end--
	}
	w := 0
	for i := 0; i < end; i++ {
		rw := runewidth.RuneWidth(row.Cells[i].Rune)
		w += rw
		if rw == 2 {
			i++ // 跳过 CJK 占位 Cell
		}
	}
	return w
}

// makeDecoRow 创建装饰横线行。
// dashCount 个 `-` 号 + padding 到 width，所有 Cell 标记为装饰。
func makeDecoRow(dashCount, width int, style tcell.Style) RenderedRow {
	row := RenderedRow{
		BufLine: -1, // 装饰行
		Cells:   make([]Cell, 0, width),
	}

	// 横线字符
	for i := 0; i < dashCount; i++ {
		row.Cells = append(row.Cells, Cell{
			Rune:         '-',
			Style:        style,
			BufLine:      -1,
			BufX:         -1,
			IsDecorative: true,
		})
	}

	// 填充到 width
	for i := dashCount; i < width; i++ {
		row.Cells = append(row.Cells, Cell{
			Rune:         ' ',
			Style:        style,
			BufLine:      -1,
			BufX:         -1,
			IsDecorative: true,
		})
	}

	return row
}