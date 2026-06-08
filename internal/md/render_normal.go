package md

import (
	"github.com/micro-editor/tcell/v2"
)

// RenderNormal 渲染普通文本行，按 width 自动换行。
func RenderNormal(seg Segment, width int, cfg MDConfig) *RenderedSegment {
	lines := linesFromBuf(cfg.Buf, seg.BufStartLine, seg.BufEndLine)
	result := &RenderedSegment{}
	// 颜色由 highlighter 决定，这里只传默认样式
	style := tcell.StyleDefault
	for lineIdx, line := range lines {
		cells := renderInline(line, style, cfg, lineIdx)
		rows := wrapCells(cells, width, lineIdx, style)
		result.Rows = append(result.Rows, rows...)
	}
	return result
}
