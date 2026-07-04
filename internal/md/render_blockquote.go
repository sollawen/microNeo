package md

import (
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/micro-editor/tcell/v2"
)

// stripBlockquoteMarker 剥离行首的 `>` 及紧跟的一个可选空格。
// 例如：
//   "> text" → "text"
//   ">text" → "text"
//   ">" → ""
//   "> " → ""
func stripBlockquoteMarker(line string) string {
	s := strings.TrimLeft(line, " \t")
	if len(s) > 0 && s[0] == '>' {
		s = s[1:]
		if len(s) > 0 && s[0] == ' ' {
			s = s[1:]
		}
	}
	return s
}

// RenderBlockquote 渲染引用块。
// 布局：
//   │ This is blockquote content that may wrap
//   │ to multiple lines if it's too long
//   │
//   │ Second paragraph
//
// 特性：
// - 去掉 `>` 标记，用 │ 竖线替代
// - 所有文字统一蓝色斜体，原文本不改
// - 左侧画灰色竖线 │（含 wrap 续行）
func RenderBlockquote(seg Segment, width int, cfg MDConfig) *RenderedSegment {
	lines := linesFromBuf(cfg.Buf, seg.BufStartLine, seg.BufEndLine)
	result := &RenderedSegment{}

	// 竖线前缀占 2 列（│ + 空格）
	barStyle := resolveStyle(cfg.Colorscheme.Styles, "md-blockquote", cfg.Colorscheme.DefStyle)
	prefixCells := []Cell{
		{Rune: '│', Style: barStyle, BufLine: -1, BufX: -1, IsDecorative: true},
		{Rune: ' ', Style: barStyle, BufLine: -1, BufX: -1, IsDecorative: true},
	}
	contentWidth := width - 2

	// 内容区太窄，退化为普通渲染
	if contentWidth < 2 {
		return RenderNormal(seg, width, cfg)
	}

	// 颜色由 highlighter 决定，这里只设置斜体属性
	baseStyle := tcell.StyleDefault.Italic(true)

	for lineIdx, line := range lines {
		// 剥离 > 前缀
		content := stripBlockquoteMarker(line)

		// highlight 对 > 行是整行匹配，行内规则不会生效，无需走 renderInline
		// 直接逐 rune 输出，只设斜体属性，颜色由 highlight 决定
		cells := make([]Cell, 0, utf8.RuneCountInString(content))
		// col 从前缀宽度开始，让 tab 对齐到绝对列网格（与 edit 原生渲染一致）
		col := len(prefixCells)
		runeIdx := 0
		tabSize := cfg.TabSize
		if tabSize <= 0 {
			tabSize = 4
		}
		for _, r := range content {
			if r == '\t' {
				ts := tabSize - (col % tabSize)
				for j := 0; j < ts; j++ {
					cells = append(cells, Cell{
						Rune:    ' ',
						Style:   baseStyle,
						BufLine: lineIdx,
						BufX:    runeIdx,
					})
				}
				col += ts
				runeIdx++
				continue
			}
			cells = append(cells, Cell{
				Rune:    r,
				Style:   baseStyle,
				BufLine: lineIdx,
				BufX:    runeIdx,
			})
			col += runewidth.RuneWidth(r)
			runeIdx++
		}

		// 用 wrapCells 对内容区做 word wrap
		contentRows := wrapCells(cells, contentWidth, lineIdx, baseStyle)

		// 每一行（首行和续行）都拼上 │ 前缀
		// 用索引访问避免 range 值拷贝的语义歧义
		for i := range contentRows {
			contentRows[i].Cells = append(prefixCells, contentRows[i].Cells...)
			result.Rows = append(result.Rows, contentRows[i])
		}
	}

	return result
}