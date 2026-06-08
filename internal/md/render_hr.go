package md

// RenderHR 渲染分割线：一条从左到右的灰色横线，背景色为 default。
// BufLine=0 标记为内容行（保证可见性判断正确），BufX=-1 跳过 highlight 覆盖
// 保留灰色横线样式，IsDecorative=true 使点击忽略。
func RenderHR(seg Segment, width int, cfg MDConfig) *RenderedSegment {
	hrStyle := resolveStyle(cfg.Colorscheme.Styles, "md-frame", cfg.Colorscheme.DefStyle)

	row := RenderedRow{
		BufLine: 0, // 内容行：对应 buffer 中 --- 这一行
		Cells:   make([]Cell, width),
	}
	for i := range row.Cells {
		row.Cells[i] = Cell{
			Rune:         '─',
			Style:        hrStyle,
			BufLine:      0,
			BufX:         -1, // 跳过 highlight 覆盖，保留灰色横线样式
			IsDecorative: true,
		}
	}

	return &RenderedSegment{
		Rows: []RenderedRow{row},
	}
}
