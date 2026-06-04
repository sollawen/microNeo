package display

import (
	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/md"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

// SetMDConfig 设置 MD 渲染配置。
func (w *BufWindow) SetMDConfig(cfg md.MDConfig) {
	w.mdConfig = cfg
}

// displayBufferMD 是 displayBuffer 的 MD 渲染版本。
// Step 0 实现：检测渲染片 → 逐片输出背景色 → 写入 screen。
// editMode 相关逻辑在 Step 1 加入。
func (w *BufWindow) displayBufferMD() {
	b := w.Buf
	if w.Height <= 0 || w.Width <= 0 {
		return
	}

	if b.ModifiedThisFrame {
		if b.Settings["diffgutter"].(bool) {
			b.UpdateDiff()
		}
		b.ModifiedThisFrame = false
	}

	bufWidth := w.bufWidth
	bufHeight := w.bufHeight

	// 1. 检测可见区域
	visibleStart := w.StartLine.Line
	visibleEnd := visibleStart + bufHeight // 超过也行，detect 会截断
	if visibleEnd >= b.LinesNum() {
		visibleEnd = b.LinesNum() - 1
	}

	// 每帧清空 mdCache（防止无限增长）
	w.mdCache = w.mdCache[:0]

	// 读 buffer 上的分类结果（事件驱动算好，content-static）
	// 注意：非 MD 文件的 b.MDSegments 保持 nil，filterSegmentsToVisible 要处理 nil
	allSegs := b.MDSegments
	segments := filterSegmentsToVisible(allSegs, visibleStart, visibleEnd)

	// 2. 渲染管线主循环
	vY := 0 // 当前 screen 行（相对窗口顶部）

	for _, seg := range segments {
		// 跳过 StartLine 之前的行（softwrap offset 暂不处理，Step 0 简化）
		lines := w.linesFromBuffer(seg.BufStartLine, seg.BufEndLine)
		rendered := seg.Render(
			lines,
			bufWidth,
			w.mdConfig,
		)

		// 将 renderer 输出的相对 BufLine 转为绝对行号
		for ri := range rendered.Rows {
			rendered.Rows[ri].BufLine += seg.BufStartLine
			for ci := range rendered.Rows[ri].Cells {
				if rendered.Rows[ri].Cells[ci].BufLine >= 0 {
					rendered.Rows[ri].Cells[ci].BufLine += seg.BufStartLine
				}
			}
		}

		// 写入 screen
		for _, row := range rendered.Rows {
			if vY >= bufHeight {
				break
			}

			// 画 gutter + 行号
			w.drawGutterAndLineNumMD(vY, row.BufLine)

			// 画内容
			// LineMatch 是稀疏 map：只在语法组变化的起始位置有条目，
			// 所以需要记住上一次匹配的 group，没命中时继续沿用。
			var lastFg tcell.Color
			var lastAttr tcell.AttrMask
			hasLast := false

			for col, cell := range row.Cells {
				screenX := w.X + w.gutterOffset + col
				screenY := w.Y + vY
				if screenX < w.X+w.gutterOffset || screenX >= w.X+w.gutterOffset+bufWidth {
					continue
				}
				style := cell.Style
				if cell.BufLine >= 0 && cell.BufX >= 0 {
					if group, ok := w.Buf.Match(cell.BufLine)[cell.BufX]; ok {
						s := config.GetColor(group.String())
						lastFg, _, lastAttr = s.Decompose()
						hasLast = true
					}
					if hasLast {
						_, bg, _ := style.Decompose()
						style = tcell.StyleDefault.Foreground(lastFg).Background(bg)
						if lastAttr&tcell.AttrBold != 0 {
							style = style.Bold(true)
						}
						if lastAttr&tcell.AttrBlink != 0 {
							style = style.Blink(true)
						}
						if lastAttr&tcell.AttrReverse != 0 {
							style = style.Reverse(true)
						}
						if lastAttr&tcell.AttrUnderline != 0 {
							style = style.Underline(true)
						}
						if lastAttr&tcell.AttrDim != 0 {
							style = style.Dim(true)
						}
						if lastAttr&tcell.AttrItalic != 0 {
							style = style.Italic(true)
						}
						if lastAttr&tcell.AttrStrikeThrough != 0 {
							style = style.StrikeThrough(true)
						}
					}
				}
				screen.SetContent(screenX, screenY, cell.Rune, cell.Combining, style)
			}

			vY++
		}

		// MicroNeo: 副作用填 w.mdCache（render 后）
		w.mdCache = append(w.mdCache, md.SegmentMeta{
			BufStartLine: seg.BufStartLine,
			BufEndLine:   seg.BufEndLine,
			RowCounts:    computeRowCounts(rendered),
		})
	}

	// 3. 填充剩余空间
	defStyle := config.DefStyle
	for ; vY < bufHeight; vY++ {
		w.drawGutterAndLineNumMD(vY, -1) // 空行，不显示行号
		for col := 0; col < bufWidth; col++ {
			screen.SetContent(w.X+w.gutterOffset+col, w.Y+vY, ' ', nil, defStyle)
		}
	}
}

// linesFromBuffer 从 buffer 读取 start..end 行（含两端），返回 []string。
func (w *BufWindow) linesFromBuffer(start, end int) []string {
	lines := make([]string, 0, end-start+1)
	for i := start; i <= end && i < w.Buf.LinesNum(); i++ {
		lines = append(lines, w.Buf.Line(i))
	}
	return lines
}

// filterSegmentsToVisible 从事件驱动算好的全 buffer segments 中，截出当前可见区域。
// 非 MD 文件的 segs 为 nil，直接返回 nil（displayBufferMD 不被调用，原生路径处理）。
// 返回的 segment 列表中 BufStartLine/BufEndLine 已被截断到 [startY, endY]。
func filterSegmentsToVisible(segs []md.Segment, startY, endY int) []md.Segment {
	if segs == nil {
		return nil
	}
	var out []md.Segment
	for _, s := range segs {
		if s.BufEndLine < startY {
			continue // 完全在可视范围之上
		}
		if s.BufStartLine > endY {
			continue // 完全在可视范围之下
		}
		// 至少部分重叠，截断到可视范围
		if s.BufStartLine < startY {
			s.BufStartLine = startY
		}
		if s.BufEndLine > endY {
			s.BufEndLine = endY
		}
		out = append(out, s)
	}
	return out
}

// computeRowCounts 计算 render 后的 RenderedSegment 中，每个 buffer 行占几个 screen row。
// RenderedRow.BufLine：首行是有效 buffer 行号，续行/装饰行为 -1。
// 所以用 BufLine 变化点切分，统计连续相同 BufLine 的 Row 数。
func computeRowCounts(rendered *md.RenderedSegment) []int {
	counts := make([]int, 0, len(rendered.Rows))
	lastBufLine := -2 // 用 -2 保证首次遇到 BufLine==-1（装饰行）也能起一个计数
	for _, row := range rendered.Rows {
		if row.BufLine != lastBufLine {
			counts = append(counts, 1)
			lastBufLine = row.BufLine
		} else {
			counts[len(counts)-1]++
		}
	}
	return counts
}

// drawGutterAndLineNumMD 在指定 screen 行绘制 gutter 和行号。
// bufLine 为 -1 表示空行/续行，行号位留空。
func (w *BufWindow) drawGutterAndLineNumMD(vY int, bufLine int) {
	b := w.Buf
	vloc := buffer.Loc{X: 0, Y: vY}
	bloc := buffer.Loc{X: 0, Y: bufLine}

	lineNumStyle := config.DefStyle
	if style, ok := config.Colorscheme["line-number"]; ok {
		lineNumStyle = style
	}

	if w.hasMessage && bufLine >= 0 {
		w.drawGutter(&vloc, &bloc)
	}
	if b.Settings["diffgutter"].(bool) && bufLine >= 0 {
		w.drawDiffGutter(lineNumStyle, false, &vloc, &bloc)
	}
	if b.Settings["ruler"].(bool) {
		if bufLine >= 0 {
			w.drawLineNum(lineNumStyle, false, &vloc, &bloc)
		} else {
			// 续行/空行/装饰行：行号位留空
			for vloc.X < w.gutterOffset {
				screen.SetContent(w.X+vloc.X, w.Y+vY, ' ', nil, lineNumStyle)
				vloc.X++
			}
		}
	}
}
