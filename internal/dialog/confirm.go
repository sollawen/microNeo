package dialog

import (
	runewidth "github.com/mattn/go-runewidth"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

// 按钮索引（focus 字段取值）
const (
	btnOK     = iota // 0：OK 按钮，默认焦点
	btnCancel        // 1：Cancel 按钮
)

// ConfirmDialog 是带 OK/Cancel 双按钮的多行文本确认浮窗（modal，走 floatFrame）。
// 边框 / title / 清屏 / 锚点 / Layout / 生命周期全部移交 FloatFrame；
// 本结构只保留 softwrap 后的文本行、按钮命中缓存、关闭回调。
type ConfirmDialog struct {
	lines    []string // softwrap 后的文本行（已按 width 换行）
	width    int      // 内容区宽度（字符列，不含边框），由调用者传入
	align    Align    // 文本对齐方式
	maxH     int      // 最大显示行数（0=不限制）
	textH    int      // 实际显示的文本行数
	title    string
	focus    int                  // 当前焦点按钮：btnOK(0) 默认 / btnCancel(1)
	onResult func(confirmed bool) // 关闭回调（一次性）

	// 鼠标命中判断用：display 每帧刷新
	lastArea Rect    // 最近一次 display 收到的 contentArea
	btnRects [2]Rect // [btnOK, btnCancel] 的屏坐标命中矩形（H 恒为 1）
}

// NewConfirmDialog 返回空 ConfirmDialog（未打开状态）。
func NewConfirmDialog() *ConfirmDialog {
	return &ConfirmDialog{}
}

// Open 打开确认浮窗。
//
//	text        多行文本（\n 分隔）
//	title       上边框标签；空串=纯横线
//	anchor      锚点屏坐标；AutoExpand=true 时为展开中心
//	width       内容区宽度（字符列，不含边框），需 >= 18
//	align       文本对齐方式（Left/Center/Right）
//	maxH        最大显示行数（0=不限制）
//	frameColor  边框色；零值 = config.DefStyle
//	onResult    结果回调：true = OK，false = Cancel/Esc/Resize/Open失败
func (d *ConfirmDialog) Open(
	text string,
	title string,
	anchor Pos,
	width int,
	align Align,
	maxH int,
	frameColor tcell.Style,
	onResult func(confirmed bool),
) {
	d.title = title
	d.align = align
	d.maxH = maxH
	d.width = width
	d.focus = btnOK
	d.onResult = onResult

	// 按 width 进行 softwrap 换行
	d.lines = softwrap(text, width)

	// 计算显示行数（按 maxH 限制）
	totalLines := len(d.lines)
	if maxH > 0 && totalLines > maxH {
		d.textH = maxH
	} else {
		d.textH = totalLines
	}

	// contentH = 文本 + 空行分隔 + 按钮行
	contentH := d.textH + 2

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
		// 开启失败，不设置业务状态，直接回调
		d.lines = nil
		d.onResult = nil
		if onResult != nil {
			onResult(false)
		}
	}
}

// display 画文本行 + 空行分隔 + 双按钮行，并缓存命中矩形。
func (d *ConfirmDialog) display(contentArea Rect) {
	d.lastArea = contentArea

	revStyle := config.DefStyle.Reverse(true)
	style := config.DefStyle

	// 1. 文本行 [0, textH)
	drawTextLines(contentArea, d.lines, d.textH, d.width, d.align, style)

	// 2. 空行（文本与按钮的分隔）
	emptyRow := contentArea.Y + d.textH
	for col := contentArea.X; col < contentArea.X+d.width; col++ {
		screen.SetContent(col, emptyRow, ' ', nil, style)
	}

	// 3. 双按钮行（整组居中，焦点按钮反白）
	okText := "[  OK  ]"
	cancelText := "[Cancel]"
	gap := 2
	okW := runewidth.StringWidth(okText)
	cancelW := runewidth.StringWidth(cancelText)
	groupW := okW + gap + cancelW

	btnRow := contentArea.Y + d.textH + 1
	okX := contentArea.X + (d.width-groupW)/2
	cancelX := okX + okW + gap

	// 缓存命中矩形
	d.btnRects[btnOK] = Rect{X: okX, Y: btnRow, W: okW, H: 1}
	d.btnRects[btnCancel] = Rect{X: cancelX, Y: btnRow, W: cancelW, H: 1}

	// 画按钮（焦点按钮反白）
	if d.focus == btnOK {
		drawButton(d.btnRects[btnOK], okText, revStyle)
		drawButton(d.btnRects[btnCancel], cancelText, style)
	} else {
		drawButton(d.btnRects[btnOK], okText, style)
		drawButton(d.btnRects[btnCancel], cancelText, revStyle)
	}
}

// handleEvent 处理键盘和鼠标事件。
func (d *ConfirmDialog) handleEvent(event tcell.Event) {
	switch ev := event.(type) {
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyTab:
			// Tab 切换焦点
			d.focus = 1 - d.focus
			screen.Redraw()
			return
		case tcell.KeyBacktab:
			// Shift+Tab 切换焦点
			d.focus = 1 - d.focus
			screen.Redraw()
			return
		case tcell.KeyLeft, tcell.KeyRight:
			// 方向键切换焦点
			d.focus = 1 - d.focus
			screen.Redraw()
			return
		case tcell.KeyEnter:
			// 激活焦点按钮
			d.close(d.focus == btnOK)
			return
		case tcell.KeyEscape:
			// 取消（无论焦点在哪）
			d.close(false)
			return
		default:
			// Space 键：KeyRune with ' '
			if ev.Key() == tcell.KeyRune && ev.Rune() == ' ' {
				d.close(d.focus == btnOK)
				return
			}
		}
	case *tcell.EventMouse:
		mx, my := ev.Position()
		// 左键点击按钮 → 激活对应按钮
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
