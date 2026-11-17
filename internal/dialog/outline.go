package dialog

import (
	"github.com/mattn/go-runewidth"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

// OutlineDialog 是「锚点+尺寸」的列表弹窗：调用方算好几何并传入等宽条目串，
// 本组件只负责渲染（选中高亮、▲▼ 指示器）与上下选择，不做排版。
// 与 SelectDialog 共用 FloatFrame 容器和同一套键盘/滚动逻辑，差别在：
// AutoExpand=false（锚点直接当外框左上角）、内容区宽由调用方给定、
// 文本无左侧 padding、逐 rune 按显示宽度推进列数、回传整条串。
type OutlineDialog struct {
	items     []string
	selected  int    // 绝对选中下标（0..len-1）
	topIdx    int    // 视口顶部下标
	size      Size   // 纯内容区尺寸（不含边框）
	title     string
	itemStyle tcell.Style
	onSelect  func(*string)
}

// NewOutlineDialog returns an empty OutlineDialog (closed state).
func NewOutlineDialog() *OutlineDialog {
	return &OutlineDialog{}
}

// Open opens the outline floating window.
//
//	items      整行显示文本，调用方已排好版（含引导线与行号），等宽
//	title      top border label
//	anchor     外框左上角（AutoExpand=false 语义）
//	size       纯内容区宽高，不含边框
//	frameColor frame color; tcell.Style{} zero = config.DialogBorderColor()
//	itemStyle  正文颜色; tcell.Style{} zero = config.DefStyle
//	onSelect   callback: Enter → onSelect(&items[selected]); Esc/resize → onSelect(nil)
//
// If FloatFrame.Open returns false (already open / doesn't fit), we call
// onSelect(nil) directly without setting any business state.
func (o *OutlineDialog) Open(
	items []string,
	title string,
	anchor Pos,
	size Size,
	frameColor tcell.Style,
	itemStyle tcell.Style,
	onSelect func(*string),
) {
	o.items = items
	o.selected = 0
	o.topIdx = 0
	o.size = size
	o.title = title
	o.itemStyle = itemStyle
	if itemStyle == (tcell.Style{}) {
		o.itemStyle = config.DefStyle
	}
	o.onSelect = onSelect

	spec := FloatOpenSpec{
		Anchor:      anchor,
		ContentSize: size,
		Title:       title,
		FrameColor:  frameColor,
		Display:     o.display,
		HandleEvent: o.handleEvent,
		AutoExpand:  false,
		OnResize: func() {
			if o.onSelect != nil {
				o.onSelect(nil)
			}
		},
	}
	if !TheFloatFrame.Open(spec) {
		o.items = nil
		o.onSelect = nil
		if onSelect != nil {
			onSelect(nil)
		}
	}
}

// display draws only visible items [topIdx, topIdx+visibleH); current selection is reversed.
// Text starts at area.X (no left padding) and stops before the last content column,
// which is reserved for the ▲▼ indicators. Each rune advances the column by its
// display width so wide characters never overwrite the next cell.
func (o *OutlineDialog) display(area Rect) {
	revStyle := o.itemStyle.Reverse(true)
	visibleH := min(len(o.items), o.size.H)

	for vi := 0; vi < visibleH; vi++ {
		i := o.topIdx + vi
		if i >= len(o.items) {
			break
		}
		row := area.Y + vi
		style := o.itemStyle
		if i == o.selected {
			style = revStyle
		}
		maxX := area.X + area.W - 1 // 最后一列留给指示器
		col := area.X
		for _, r := range o.items[i] {
			if col >= maxX {
				break
			}
			screen.Screen.SetContent(col, row, r, nil, style)
			col += runewidth.RuneWidth(r)
		}
		for col < maxX {
			screen.Screen.SetContent(col, row, ' ', nil, style)
			col++
		}
	}

	// Scroll indicators (only when content overflows)
	if len(o.items) > visibleH {
		rightCol := area.X + area.W - 1
		topStyle := o.itemStyle
		if o.topIdx == o.selected {
			topStyle = revStyle
		}
		botStyle := o.itemStyle
		botIdx := o.topIdx + visibleH - 1
		if botIdx == o.selected {
			botStyle = revStyle
		}
		if o.topIdx > 0 {
			screen.Screen.SetContent(rightCol, area.Y, '▲', nil, topStyle)
		}
		if o.topIdx+visibleH < len(o.items) {
			screen.Screen.SetContent(rightCol, area.Y+visibleH-1, '▼', nil, botStyle)
		}
	}
}

// handleEvent processes keyboard events. Up/Down move the selection with wrap-around
// and scroll the viewport; Enter/Esc close the container first then callback;
// other keys are swallowed (modal).
func (o *OutlineDialog) handleEvent(event tcell.Event) {
	switch ev := event.(type) {
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyDown:
			o.selected = (o.selected + 1) % len(o.items)
			o.ensureVisible()
			screen.Redraw()
			return
		case tcell.KeyUp:
			o.selected = (o.selected - 1 + len(o.items)) % len(o.items)
			o.ensureVisible()
			screen.Redraw()
			return
		case tcell.KeyEnter:
			picked := o.items[o.selected]
			onSelect := o.onSelect
			TheFloatFrame.Close()
			if onSelect != nil {
				onSelect(&picked)
			}
			return
		case tcell.KeyEscape:
			onSelect := o.onSelect
			TheFloatFrame.Close()
			if onSelect != nil {
				onSelect(nil)
			}
			return
		}
	}
}

// ensureVisible scrolls the viewport to include the selected index.
func (o *OutlineDialog) ensureVisible() {
	visibleH := min(len(o.items), o.size.H)
	if o.selected < o.topIdx {
		o.topIdx = o.selected
	}
	if o.selected >= o.topIdx+visibleH {
		o.topIdx = o.selected - visibleH + 1
	}
}
