// Package dialog provides modal floating window components for microNeo.
package dialog

import (
	runewidth "github.com/mattn/go-runewidth"
	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/micro/v2/internal/util"
	"github.com/micro-editor/tcell/v2"
)

// InputDialog 是单行文本输入浮窗（modal，走 floatFrame）。
// 边框 / title / 清屏 / 锚点 / Layout / 生命周期全部移交 FloatFrame；
// 本结构只保留单行编辑状态（buffer + cursor）、关闭回调。
// 键位映射直接读 config.Bindings["command"]（自动跟随用户自定义）。
type InputDialog struct {
	buf      *buffer.Buffer  // BTInfo 单行 buffer，编辑基座（与 InfoBar 同源）
	cursor   *buffer.Cursor // buf 的活跃光标，编辑原语入口
	initial  string         // 取消时回退的原值
	width    int            // 内容区宽度（字符列），不含边框
	hscroll  int            // 水平滚动偏移（光标出框时拉动），字符列
	title    string
	onResult func(result string, canceled bool) // 关闭回调（一次性）
}

// NewInputDialog 返回空 InputDialog（未打开状态）。
func NewInputDialog() *InputDialog {
	return &InputDialog{}
}

// Open 打开输入浮窗。
//
//	initial     初始内容（可空）
//	title       上边框标签（如 "Rename"）；空串=纯横线
//	anchor      锚点屏坐标；AutoExpand=false 时为外矩形左上角
//	width       内容区宽度（字符列，不含边框）；<=0 时给安全下限
//	frameColor  边框色；零值 = config.DefStyle
//	onResult    关闭回调：Enter → onResult(edited, false)；ESC/Resize/失败 → onResult("", true)
//
// 回调顺序：先 TheFloatFrame.Close()，再触发 onResult。
func (d *InputDialog) Open(
	initial    string,
	title      string,
	anchor     Pos,
	width      int,
	frameColor tcell.Style,
	onResult   func(result string, canceled bool),
) {
	d.initial = initial
	d.title = title
	d.onResult = onResult

	// 安全下限
	if width <= 0 {
		w, _ := screen.Screen.Size()
		width = min(40, w-4)
	}
	// 与屏幕宽做夹取，确保 outerW 能过 floatFrame 的失败前置检查
	wScreen, _ := screen.Screen.Size()
	if width > wScreen-4 {
		width = wScreen - 4
	}
	d.width = width
	d.hscroll = 0

	// 创建 BTInfo buffer
	d.buf = buffer.NewBufferFromString(initial, "", buffer.BTInfo)
	d.cursor = d.buf.GetActiveCursor()

	spec := FloatOpenSpec{
		Anchor:      anchor,
		ContentSize: Size{W: width, H: 1},
		Title:       title,
		FrameColor:  frameColor,
		Display:     d.display,
		HandleEvent: d.handleEvent,
		OnResize:    d.onResize,
		AutoExpand:  true, // 自动避开边界，支持 Y=-1 sentinel
	}

	if !TheFloatFrame.Open(spec) {
		// 开启失败，不设置业务状态，直接回调取消
		d.buf = nil
		d.cursor = nil
		d.onResult = nil
		if onResult != nil {
			onResult("", true)
		}
	}
}

// display 画单行文本 + 光标。水平滚动保证光标始终可见。
func (d *InputDialog) display(contentArea Rect) {
	if d.buf == nil {
		return
	}

	line := d.buf.LineBytes(0)
	tabsize := int(d.buf.Settings["tabsize"].(float64))

	// 光标可视列（用 GetVisualX false 参数，单行恒为 false）
	cursorVisualCol := d.cursor.GetVisualX(false)
	charCount := util.CharacterCount(line)

	// 水平滚动：保证光标在框内
	if cursorVisualCol < d.hscroll {
		d.hscroll = cursorVisualCol
	}
	if cursorVisualCol >= d.hscroll+contentArea.W {
		d.hscroll = cursorVisualCol - contentArea.W + 1
	}

	// hscroll 上限：行总可视宽 - width
	totalVisualWidth := util.StringWidth(line, charCount, tabsize)
	maxHScroll := totalVisualWidth - contentArea.W
	if maxHScroll < 0 {
		maxHScroll = 0
	}
	if d.hscroll > maxHScroll {
		d.hscroll = maxHScroll
	}

	// 从 hscroll 可视列反向查找起始字符列
	startCharCol := util.GetCharPosInLine(line, d.hscroll, tabsize)

	// 跳过前 startCharCol 个字符（按字符数，不是宽度）
	remaining := line
	for charCount := 0; charCount < startCharCol && len(remaining) > 0; charCount++ {
		_, _, size := util.DecodeCharacter(remaining)
		remaining = remaining[size:]
	}

	// 画可见字符
	vlocX := contentArea.X
	col := 0
	style := config.DefStyle

	for len(remaining) > 0 && col < contentArea.W {
		r, combc, size := util.DecodeCharacter(remaining)
		remaining = remaining[size:]

		width := 0
		switch r {
		case '\t':
			ts := tabsize - (col % tabsize)
			width = ts
			for j := 0; j < ts && col < contentArea.W; j++ {
				screen.SetContent(vlocX, contentArea.Y, ' ', nil, style)
				vlocX++
				col++
			}
			continue
		default:
			width = runewidth.RuneWidth(r)
			screen.SetContent(vlocX, contentArea.Y, r, combc, style)
		}

		for w := 0; w < width && col < contentArea.W; w++ {
			if w > 0 {
				// 双宽字符的后半格写空格占位
				screen.SetContent(vlocX, contentArea.Y, ' ', nil, style)
			}
			vlocX++
			col++
		}
	}

	// 清剩余格
	for col < contentArea.W {
		screen.SetContent(vlocX, contentArea.Y, ' ', nil, style)
		vlocX++
		col++
	}

	// 光标位置
	screenX := contentArea.X + cursorVisualCol - d.hscroll
	screen.ShowCursor(screenX, contentArea.Y)
}

// handleEvent 处理键盘事件。编辑动作落到 buffer 原语，每次修改后调 screen.Redraw()。
func (d *InputDialog) handleEvent(event tcell.Event) {
	ev, ok := event.(*tcell.EventKey)
	if !ok {
		return
	}

	// 先查键位映射
	keyName := config.KeyName(ev)
	action, ok := config.Bindings["command"][keyName]
	if !ok {
		action = ""
	}

	switch action {
	case "CursorLeft":
		d.cursor.Left()
		d.cursor.Relocate() // 与 InfoBar 一致
		screen.Redraw()
		return
	case "CursorRight":
		d.cursor.Right()
		d.cursor.Relocate() // 与 InfoBar 一致
		screen.Redraw()
		return
	case "StartOfLine":
		d.cursor.X = 0
		d.cursor.Relocate() // 与 InfoBar 一致
		screen.Redraw()
		return
	case "EndOfLine":
		d.cursor.X = charCount(d.buf.LineBytes(0))
		d.cursor.Relocate() // 与 InfoBar 一致
		screen.Redraw()
		return
	case "StartOfTextToggle":
		d.cursor.StartOfText()
		d.cursor.Relocate() // 与 InfoBar 一致
		screen.Redraw()
		return
	case "CursorStart":
		d.cursor.X = 0
		d.cursor.StoreVisualX() // 与 InfoBar CursorStart 一致
		d.cursor.Relocate()
		screen.Redraw()
		return
	case "CursorEnd":
		d.cursor.X = charCount(d.buf.LineBytes(0))
		d.cursor.StoreVisualX() // 与 InfoBar CursorEnd 一致
		d.cursor.Relocate()
		screen.Redraw()
		return
	case "WordLeft":
		d.cursor.WordLeft()
		d.cursor.Relocate() // 与 InfoBar 一致
		screen.Redraw()
		return
	case "WordRight":
		d.cursor.WordRight()
		d.cursor.Relocate() // 与 InfoBar 一致
		screen.Redraw()
		return
	case "Backspace":
		loc := d.cursor.Loc
		if loc.LessThan(d.buf.Start()) {
			// 行首，无操作
			return
		}
		prev := loc.Move(-1, d.buf)
		d.buf.Remove(prev, loc)
		d.cursor.StoreVisualX() // 保存视觉位置（与 InfoBar 一致）
		d.cursor.Relocate()      // Remove 已移动光标，Relocate 确保在正确位置（与 InfoBar 一致）
		screen.Redraw()
		return
	case "Delete":
		loc := d.cursor.Loc
		if !loc.LessThan(d.buf.End()) {
			// 行末，无操作
			return
		}
		next := loc.Move(1, d.buf)
		d.buf.Remove(loc, next)
		d.cursor.Relocate() // Remove 已移动光标，Relocate 确保在正确位置（与 InfoBar 一致）
		screen.Redraw()
		return
	case "DeleteWordLeft":
		// 与 InfoBar 完全一致：SelectWordLeft + DeleteSelection + ResetSelection + Relocate
		if !d.cursor.HasSelection() {
			// 保存选中起始点
			d.cursor.OrigSelection[0] = d.cursor.Loc
		}
		// 移动光标选中词
		d.cursor.WordLeft()
		d.cursor.SelectTo(d.cursor.Loc)
		// 删除选中
		if d.cursor.HasSelection() {
			d.cursor.DeleteSelection()
			d.cursor.ResetSelection()
		}
		d.cursor.Relocate()
		screen.Redraw()
		return
	case "DeleteWordRight":
		// 与 InfoBar 完全一致：SelectWordRight + DeleteSelection + ResetSelection + Relocate
		if !d.cursor.HasSelection() {
			// 保存选中起始点
			d.cursor.OrigSelection[0] = d.cursor.Loc
		}
		// 移动光标选中词
		d.cursor.WordRight()
		d.cursor.SelectTo(d.cursor.Loc)
		// 删除选中
		if d.cursor.HasSelection() {
			d.cursor.DeleteSelection()
			d.cursor.ResetSelection()
		}
		d.cursor.Relocate()
		screen.Redraw()
		return
	case "InsertTab":
		// 与 InfoBar InsertTab 一致：插入到下一个 tab stop
		tabsize := int(d.buf.Settings["tabsize"].(float64))
		indent := d.buf.IndentString(util.IntOpt(tabsize))
		tabBytes := len(indent)
		bytesUntilIndent := tabBytes - (d.cursor.GetVisualX(false) % tabsize)
		d.buf.Insert(d.cursor.Loc, indent[:bytesUntilIndent])
		d.cursor.Relocate() // 与 InfoBar 一致
		screen.Redraw()
		return
	}

	// 字符输入
	if ev.Key() == tcell.KeyRune {
		r := ev.Rune()
		// Tab 字符由 InsertTab 处理（switch 中），不在这里当作字符插入
		if r >= 0x20 && r != '\t' {
			d.buf.Insert(d.cursor.Loc, string(r))
			d.cursor.Relocate() // Insert 已移动光标，Relocate 确保在行末（与 InfoBar 一致）
			screen.Redraw()
		}
		return
	}

	// Enter → 确认
	if ev.Key() == tcell.KeyEnter {
		edited := string(d.buf.LineBytes(0))
		cb := d.onResult
		d.onResult = nil
		TheFloatFrame.Close()
		if cb != nil {
			cb(edited, false)
		}
		return
	}

	// ESC / Ctrl-C / Ctrl-q → 取消
	if ev.Key() == tcell.KeyEscape || ev.Key() == tcell.KeyCtrlC || ev.Key() == tcell.KeyCtrlQ {
		cb := d.onResult
		d.onResult = nil
		TheFloatFrame.Close()
		if cb != nil {
			cb("", true)
		}
		return
	}
}

// onResize Resize 事件由容器拦截，容器先 Close 再调用本函数。
func (d *InputDialog) onResize() {
	cb := d.onResult
	d.onResult = nil
	if cb != nil {
		cb("", true)
	}
}

// charCount 返回字节数组的字符数（rune count）。
func charCount(b []byte) int {
	return util.CharacterCount(b)
}
