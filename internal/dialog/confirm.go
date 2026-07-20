package dialog

import (
	runewidth "github.com/mattn/go-runewidth"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

// Kind 确认框交互模式
type Kind int

const (
	KindOkCancel Kind = iota // 两按钮模式（默认）
	KindYesNo                // 单键模式（y/n/Esc），无按钮无焦点
)

// Focus OkCancel 模式的初始焦点
type Focus int

const (
	FocusOK     Focus = iota // 默认焦点 = 确认位（左按钮）
	FocusCancel              // 默认焦点 = 取消位（右按钮）
	FocusNone                // 无焦点，两按钮初始都不高亮
)

// 按钮索引（用于渲染顺序与命中判断）
const (
	btnOK = iota
	btnCancel
)

// yesNoHint 在 Open 阶段拼到 text 末尾，跟 text 一起走 softwrap，
// 短行末尾拼接 / 长行自动换行——无需任何坐标或样式特殊处理。
const yesNoHint = " [y/n/esc]"

// ConfirmDialog 是带 OK/Cancel 双按钮的多行文本确认浮窗（modal，走 floatFrame）。
// 边框 / title / 清屏 / 锚点 / Layout / 生命周期全部移交 FloatFrame；
// 本结构只保留 softwrap 后的文本行、按钮命中缓存、关闭回调。
type ConfirmDialog struct {
	lines    []string // softwrap 后的文本行（已按 width 换行）
	width    int      // 内容区宽度（字符列，不含边框）
	align    Align
	maxH     int      // 最大显示行数（0=不限制）
	textH    int      // 实际显示的文本行数
	title    string
	kind     Kind  // 交互模式：OkCancel 或 YesNo
	focus    Focus // 当前焦点（仅 OkCancel 有意义）
	onResult func(confirmed bool)

	lastArea Rect    // 最近一次 display 收到的 contentArea
	btnRects [2]Rect // [btnOK, btnCancel] 的屏坐标命中矩形（H 恒为 1）
}

// NewConfirmDialog 返回空 ConfirmDialog（未打开状态）。
func NewConfirmDialog() *ConfirmDialog {
	return &ConfirmDialog{}
}

// Open 打开确认浮窗。
//
//	text          多行文本（\n 分隔）
//	title         上边框标签；空串=纯横线
//	anchor        锚点屏坐标；AutoExpand=true 时为展开中心
//	width         内容区宽度（字符列，不含边框），OkCancel 需 >= 18
//	align         文本对齐方式（Left/Center/Right）
//	maxH          最大显示行数（0=不限制）
//	frameColor    边框色；零值 = config.DefStyle
//	kind          交互模式：KindOkCancel（默认）/ KindYesNo
//	defaultFocus  OkCancel 初始焦点（YesNo 忽略）
//	onResult      结果回调：true = 确认，false = 取消/Esc/Resize/Open失败
func (d *ConfirmDialog) Open(
	text string,
	title string,
	anchor Pos,
	width int,
	align Align,
	maxH int,
	frameColor tcell.Style,
	kind Kind,
	defaultFocus Focus,
	onResult func(confirmed bool),
) {
	d.title = title
	// YesNo 模式强制左对齐：text+hint 是一行操作提示，对齐右/居中不自然
	if kind == KindYesNo {
		align = AlignLeft
	}
	d.align = align
	d.maxH = maxH
	d.width = width
	d.kind = kind
	d.focus = defaultFocus
	d.onResult = onResult

	// YesNo 模式下 hint 拼到 text 末尾，让 softwrap 自然处理换行
	if kind == KindYesNo {
		text += yesNoHint
	}
	d.lines = softwrap(text, width)

	totalLines := len(d.lines)
	if maxH > 0 && totalLines > maxH {
		d.textH = maxH
	} else {
		d.textH = totalLines
	}

	// OkCancel = 文本+空行+按钮；YesNo = 纯文本（hint 已含在 text 里）
	var contentH int
	if d.kind == KindOkCancel {
		contentH = d.textH + 2
	} else {
		contentH = d.textH
	}

	spec := FloatOpenSpec{
		Anchor:      anchor,
		ContentSize: Size{W: width, H: contentH},
		Title:       title,
		FrameColor:  frameColor,
		Display:     d.display,
		HandleEvent: d.handleEvent,
		OnResize:    d.onResize,
		AutoExpand:  true,
	}

	if !TheFloatFrame.Open(spec) {
		d.lines = nil
		d.onResult = nil
		if onResult != nil {
			onResult(false)
		}
	}
}

// display 画文本行；OkCancel 模式下追加空行 + 双按钮行。
// YesNo 模式下 hint 已随 text 拼入并 softwrap，无需任何额外渲染。
func (d *ConfirmDialog) display(contentArea Rect) {
	d.lastArea = contentArea
	drawTextLines(contentArea, d.lines, d.textH, d.width, d.align, config.DefStyle)
	if d.kind == KindOkCancel {
		d.drawButtons(contentArea)
	}
}

// drawButtons 画 OkCancel 模式的空行分隔 + 双按钮（焦点按钮反白）。
func (d *ConfirmDialog) drawButtons(contentArea Rect) {
	style := config.DefStyle
	revStyle := style.Reverse(true)

	emptyRow := contentArea.Y + d.textH
	for col := contentArea.X; col < contentArea.X+d.width; col++ {
		screen.SetContent(col, emptyRow, ' ', nil, style)
	}

	okText := "[  OK  ]"
	cancelText := "[Cancel]"
	gap := 2
	okW := runewidth.StringWidth(okText)
	cancelW := runewidth.StringWidth(cancelText)
	groupW := okW + gap + cancelW

	btnRow := contentArea.Y + d.textH + 1
	okX := contentArea.X + (d.width-groupW)/2
	cancelX := okX + okW + gap

	d.btnRects[btnOK] = Rect{X: okX, Y: btnRow, W: okW, H: 1}
	d.btnRects[btnCancel] = Rect{X: cancelX, Y: btnRow, W: cancelW, H: 1}

	okStyle, cancelStyle := style, style
	if d.focus == FocusOK {
		okStyle = revStyle
	} else if d.focus == FocusCancel {
		cancelStyle = revStyle
	}
	drawButton(d.btnRects[btnOK], okText, okStyle)
	drawButton(d.btnRects[btnCancel], cancelText, cancelStyle)
}

// handleEvent 按 kind 分发事件。
func (d *ConfirmDialog) handleEvent(event tcell.Event) {
	if d.kind == KindYesNo {
		d.handleEventYesNo(event)
		return
	}
	d.handleEventOkCancel(event)
}

// handleEventOkCancel 处理 OkCancel 模式键盘和鼠标事件。
func (d *ConfirmDialog) handleEventOkCancel(event tcell.Event) {
	switch ev := event.(type) {
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyTab, tcell.KeyBacktab, tcell.KeyLeft, tcell.KeyRight:
			d.cycleFocus()
		case tcell.KeyEnter:
			if d.focus == FocusNone {
				return
			}
			d.close(d.focus == FocusOK)
		case tcell.KeyEscape:
			d.close(false)
		default:
			if ev.Key() == tcell.KeyRune && ev.Rune() == ' ' {
				if d.focus == FocusNone {
					return
				}
				d.close(d.focus == FocusOK)
			}
		}
	case *tcell.EventMouse:
		mx, my := ev.Position()
		if ev.Buttons()&tcell.Button1 != 0 {
			for i, r := range d.btnRects {
				if mx >= r.X && mx < r.X+r.W && my == r.Y {
					d.close(i == btnOK)
					return
				}
			}
		}
	}
}

// handleEventYesNo 只认 y/Y/n/N/Esc，鼠标与其它键一律吞掉。
func (d *ConfirmDialog) handleEventYesNo(event tcell.Event) {
	ev, ok := event.(*tcell.EventKey)
	if !ok {
		return
	}
	switch ev.Key() {
	case tcell.KeyEscape:
		d.close(false)
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'y', 'Y':
			d.close(true)
		case 'n', 'N':
			d.close(false)
		}
	}
}

// cycleFocus 在 OK ↔ Cancel 之间切换；FocusNone 首次切换落到 Cancel。
func (d *ConfirmDialog) cycleFocus() {
	if d.focus == FocusCancel {
		d.focus = FocusOK
	} else {
		d.focus = FocusCancel
	}
	screen.Redraw()
}

// close 关闭浮窗并触发回调。
func (d *ConfirmDialog) close(confirmed bool) {
	cb := d.onResult
	d.onResult = nil
	TheFloatFrame.Close()
	if cb != nil {
		cb(confirmed)
	}
}

// onResize Resize 事件由容器拦截，容器先 Close 再调用本函数。
func (d *ConfirmDialog) onResize() {
	cb := d.onResult
	d.onResult = nil
	if cb != nil {
		cb(false)
	}
}