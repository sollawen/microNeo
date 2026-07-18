package dialog

import (
	"strings"

	runewidth "github.com/mattn/go-runewidth"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

// Align 文本对齐方式
type Align int

const (
	AlignLeft Align = iota
	AlignCenter
	AlignRight
)

// MsgDialog 是只读多行文本展示浮窗（modal，走 floatFrame）。
// 边框 / title / 清屏 / 锚点 / Layout / 生命周期全部移交 FloatFrame；
// 本结构只保留 softwrap 后的文本行、按钮命中缓存、关闭回调。
type MsgDialog struct {
	lines   []string // softwrap 后的文本行（已按 width 换行）
	width   int      // 内容区宽度（字符列，不含边框），由调用者传入
	align   Align    // 文本对齐方式
	maxH    int      // 最大显示行数（0=不限制）
	textH   int      // 实际显示的文本行数
	title   string
	onClose func() // 关闭回调（一次性）

	// 鼠标命中判断用：display 每帧刷新
	lastArea Rect // 最近一次 display 收到的 contentArea
	btnX     int  // 按钮左上屏坐标 X
	btnY     int  // 按钮左上屏坐标 Y
	btnW     int  // 按钮可视宽度
}

// NewMsgDialog 返回空 MsgDialog（未打开状态）。
func NewMsgDialog() *MsgDialog {
	return &MsgDialog{}
}

// softwrap 按 width 对文本进行软换行。
// 正确处理 CJK 双宽字符（不在双宽字符中间断开）和 Tab 展开（从 config 读 tabsize）。
// 空行保留。
func softwrap(text string, width int) []string {
	tabsize := int(config.GetGlobalOption("tabsize").(float64))
	rawLines := strings.Split(text, "\n")
	lines := []string{}

	for _, rawLine := range rawLines {
		if rawLine == "" {
			lines = append(lines, "")
			continue
		}

		// 按 width 分割这一行
		currentLine := ""
		currentWidth := 0

		for _, r := range rawLine {
			rWidth := runeWidthInLine(r, currentWidth, tabsize)

			if currentWidth+rWidth > width && currentLine != "" {
				// 当前行已满，开始新行
				lines = append(lines, currentLine)
				currentLine = string(r)
				currentWidth = rWidth
			} else {
				currentLine += string(r)
				currentWidth += rWidth
			}
		}

		if currentLine != "" {
			lines = append(lines, currentLine)
		}
	}

	return lines
}

// runeWidthInLine 计算字符在行中的显示宽度。
// Tab 宽度取决于当前列位置。
func runeWidthInLine(r rune, col int, tabsize int) int {
	switch r {
	case '\t':
		return tabsize - (col % tabsize)
	default:
		return runewidth.RuneWidth(r)
	}
}

// drawTextLines 在指定区域绘制多行文本（不含按钮）。
func drawTextLines(area Rect, lines []string, textH, width int, align Align, style tcell.Style) {
	for vi := 0; vi < textH; vi++ {
		row := area.Y + vi
		line := lines[vi]
		lineWidth := runewidth.StringWidth(line)

		// 按 align 计算起始 X
		var startX int
		switch align {
		case AlignLeft:
			startX = area.X
		case AlignCenter:
			startX = area.X + (width-lineWidth)/2
		case AlignRight:
			startX = area.X + width - lineWidth
		}

		// 先清空整行（底色一致）
		for col := area.X; col < area.X+width; col++ {
			screen.SetContent(col, row, ' ', nil, style)
		}

		// 从 startX 起逐 rune 画文本
		col := startX
		for _, r := range line {
			w := runewidth.RuneWidth(r)
			screen.SetContent(col, row, r, nil, style)
			if w > 1 {
				// 双宽字符后半格写空格占位
				for k := 1; k < w; k++ {
					screen.SetContent(col+k, row, ' ', nil, style)
				}
			}
			col += w
		}
	}
}

// drawButton 在指定矩形绘制单个按钮文案（不处理双宽字符对齐）。
func drawButton(rect Rect, text string, style tcell.Style) {
	col := rect.X
	row := rect.Y
	for _, r := range text {
		w := runewidth.RuneWidth(r)
		screen.SetContent(col, row, r, nil, style)
		if w > 1 {
			for k := 1; k < w; k++ {
				screen.SetContent(col+k, row, ' ', nil, style)
			}
		}
		col += w
	}
}

// Open 打开消息展示浮窗。
//
//	text       多行文本（\n 分隔）；空串按 1 行空文本处理
//	title      上边框标签（如 "Info"）；空串=纯横线
//	anchor     锚点屏坐标；AutoExpand=true 时为展开中心
//	width      内容区宽度（字符列，不含边框）
//	align      文本对齐方式（Left/Center/Right）
//	maxH       最大显示行数（0=不限制，超出行截断）
//	frameColor 边框色；零值 = config.DefStyle
//	onClose    关闭回调（恒触发一次）
func (d *MsgDialog) Open(
	text string,
	title string,
	anchor Pos,
	width int,
	align Align,
	maxH int,
	frameColor tcell.Style,
	onClose func(),
) {
	d.title = title
	d.align = align
	d.maxH = maxH
	d.width = width
	d.onClose = onClose

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
		d.onClose = nil
		if onClose != nil {
			onClose()
		}
	}
}

// display 画文本行 + 空行分隔 + 按钮行，并缓存命中区域。
func (d *MsgDialog) display(contentArea Rect) {
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

	// 3. 按钮行（文本下方 +1 行，水平居中，方括号样式）
	btnRow := contentArea.Y + d.textH + 1
	btnText := "[Close]"
	btnVisualW := runewidth.StringWidth(btnText)
	d.btnX = contentArea.X + (d.width-btnVisualW)/2
	d.btnY = btnRow
	d.btnW = btnVisualW
	drawButton(Rect{X: d.btnX, Y: btnRow, W: btnVisualW, H: 1}, btnText, revStyle)
}

// handleEvent 处理键盘和鼠标事件。
func (d *MsgDialog) handleEvent(event tcell.Event) {
	switch ev := event.(type) {
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyEnter:
			// 激活按钮（关闭）
			d.close()
			return
		case tcell.KeyEscape:
			// 取消
			d.close()
			return
		default:
			// Space 键：KeyRune with ' '
			if ev.Key() == tcell.KeyRune && ev.Rune() == ' ' {
				d.close()
				return
			}
		}
	case *tcell.EventMouse:
		mx, my := ev.Position()
		btns := ev.Buttons()
		// 左键点击按钮 → 关闭
		if btns&tcell.Button1 != 0 {
			if mx >= d.btnX && mx < d.btnX+d.btnW && my == d.btnY {
				d.close()
			}
		}
	}
}

// close 关闭浮窗并触发回调。
func (d *MsgDialog) close() {
	cb := d.onClose
	d.onClose = nil
	TheFloatFrame.Close()
	if cb != nil {
		cb()
	}
}

// onResize Resize 事件由容器拦截，容器先 Close 再调用本函数。
func (d *MsgDialog) onResize() {
	cb := d.onClose
	d.onClose = nil
	if cb != nil {
		cb()
	}
}
