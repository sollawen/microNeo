package md

// RenderNormal 渲染普通文本行，按 width 自动换行。
func RenderNormal(lines []string, width int, cfg MDConfig) *RenderedSegment {
	result := &RenderedSegment{}
	style := cfg.Colorscheme.DefStyle
	for lineIdx, line := range lines {
		cells := renderInline(line, style, lineIdx)
		rows := wrapCells(cells, width, lineIdx, style)
		result.Rows = append(result.Rows, rows...)
	}
	return result
}
