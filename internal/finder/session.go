package finder

import (
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

// ---- 公共契约 ----

// Rect 是屏坐标的一个矩形区域。
type Rect struct {
	X, Y, W, H int
}

// CloseReason 描述 finder 会话以何种原因关闭。
type CloseReason uint8

const (
	Picked CloseReason = iota
	Esc
	Quit
	Resize
)

// Result 承载一次 finder 会话的关闭结果。
type Result struct {
	Reason CloseReason
	Cwd    string
	File   string
	IsQuit bool
}

// ---- 常量 ----

const (
	fsMinWidth  = 20
	fsMinHeight = 10
	// historyMinHeight 是 history region 出现的最低 finder 高度门槛（按 Open 收到的原始 rect.H）。
	historyMinHeight = 20
	// historyContentH 是 history region 内容可见行数；上方的横分隔线占额外 1 行由 session 画。
	historyContentH = 5
)

// ---- Session ----

// Session 是文件选择器的一次会话实例。
type Session struct {
	rect     Rect // 整个 finder 的外框（含标题行、底边、所有面板）
	previewX int  // 左右栏竖分隔线列（│ 画在此列），preview 区从该位置开始
	historyY int  // history 上方横分隔线的 Y 坐标；仅 fm.history != nil 时有效

	list    *fileList    // 恒构造
	prev    *preview     // 右栏宽 >= previewMinWidth 时构造；否则 nil
	history *historyList // 高度门槛满足且 dirs 非空时构造；否则 nil

	regions         []NoKeyboardRegion // 所有 region
	keyboardRegions []KeyboardRegion   // 仅接键盘的 region
	focus           int                // 索引 keyboardRegions；Step 1 恒 0，Step 2 起 0=fileList / 1=history

	isOpen  bool
	onClose func(Result)
	isQuit  bool
}

// NewSession 返回一个未打开的 Session。
func NewSession() *Session {
	return &Session{}
}

// IsOpen 返回 Session 是否处于打开状态。
func (fm *Session) IsOpen() bool {
	return fm.isOpen
}

// Open 打开 finder 会话。
func (fm *Session) Open(rect Rect, cwd, file string, isQuit bool, onClose func(Result)) bool {
	fm.onClose = onClose
	fm.isQuit = isQuit

	if rect.W < fsMinWidth || rect.H < fsMinHeight {
		return false
	}

	// 几何参数
	fm.rect = Rect{X: rect.X, Y: rect.Y, W: rect.W, H: rect.H - 1} // -1 留 statusLine

	widthFrac := config.GetGlobalOption("fileselectwidth").(float64)
	pickerW := int(widthFrac * float64(rect.W))
	if pickerW < fsMinWidth {
		pickerW = fsMinWidth
	}
	if pickerW > rect.W {
		pickerW = rect.W
	}
	fm.previewX = rect.X + pickerW - 1 // 分隔符列

	// fileList 内容区原始尺寸（让出标题行 + 右分隔符列 + 底边）
	fullListRect := Rect{
		X: fm.rect.X,
		Y: fm.rect.Y + 1,
		W: pickerW - 1,
		H: fm.rect.H - 2,
	}

	// 双 slice 构造
	fm.regions = make([]NoKeyboardRegion, 0, 3)
	fm.keyboardRegions = make([]KeyboardRegion, 0, 2)

	// —— history 条件构造 ——
	// 读取在判定之前，确保 dirs 为空也能正确不构造。
	dirs := readHistory()
	var histRect Rect
	var listRect Rect
	if rect.H >= historyMinHeight && len(dirs) > 0 {
		historyBlockH := 1 + historyContentH
		histRect = Rect{
			X: fullListRect.X,
			Y: fullListRect.Y + fullListRect.H - historyContentH,
			W: fullListRect.W,
			H: historyContentH,
		}
		fm.historyY = histRect.Y - 1
		listRect = fullListRect
		listRect.H -= historyBlockH
	} else {
		listRect = fullListRect
		fm.historyY = 0
	}

	fm.list = newFileList(fm, listRect, cwd, file, pickerW)
	fm.regions = append(fm.regions, fm.list)
	fm.keyboardRegions = append(fm.keyboardRegions, fm.list)

	if fm.history == nil && histRect.W > 0 && histRect.H > 0 && len(dirs) > 0 {
		fm.history = newHistoryList(fm, histRect, dirs)
		fm.regions = append(fm.regions, fm.history)
		fm.keyboardRegions = append(fm.keyboardRegions, fm.history)
	}

	// preview：仅右栏宽 >= previewMinWidth 时构造
	pvW := fm.rect.W - pickerW
	if pvW >= previewMinWidth {
		pvRect := Rect{X: fm.previewX + 1, Y: fm.rect.Y, W: pvW, H: fm.rect.H}
		fm.prev = newPreview(pvRect)
		fm.regions = append(fm.regions, fm.prev)
	}
	fm.focus = 0

	fm.isOpen = true
	fm.syncPreview()
	// 明确初始化焦点视觉（fm.list.FocusOn 写 focused=true 并 Redraw）
	fm.list.FocusOn()
	return true
}

// ActivateFromHistory 由 historyList 选中目录后调用：复用 fileList.chdirTo 切目录并把焦点切回 fileList。
// 通过类型断言定位 fileList 在 keyboardRegions 中的索引，不依赖构造顺序或新增其他 region。
func (fm *Session) ActivateFromHistory(dir string) {
	if fm.list == nil {
		return
	}
	fm.list.chdirTo(dir, "")
	for i, kr := range fm.keyboardRegions {
		if _, ok := kr.(*fileList); ok {
			fm.switchFocus(i)
			return
		}
	}
}

// ---- 关闭路径 ----

func (fm *Session) close(reason CloseReason) {
	if !fm.isOpen {
		return
	}
	r := Result{Reason: reason, Cwd: fm.list.currentDir, IsQuit: fm.isQuit}
	if reason == Picked {
		idx := fm.list.cursor - 1
		if idx >= 0 && idx < len(fm.list.showEntries) {
			r.File = fm.list.showEntries[idx].name
		}
	}
	fm.finishClose(r)
}

func (fm *Session) closePicked(name string) {
	fm.finishClose(Result{
		Reason: Picked, Cwd: fm.list.currentDir, File: name, IsQuit: fm.isQuit,
	})
}

func (fm *Session) finishClose(r Result) {
	if !fm.isOpen {
		return
	}
	cb := fm.onClose
	fm.reset()
	if cb != nil {
		cb(r)
	}
}

// reset 完整清理 Session 所有 region 与几何字段，避免被误复用时残留。
func (fm *Session) reset() {
	fm.list = nil
	fm.prev = nil
	fm.history = nil
	fm.regions = nil
	fm.keyboardRegions = nil
	fm.focus = 0
	fm.previewX = 0
	fm.historyY = 0
	fm.rect = Rect{}
	fm.isOpen = false
	fm.onClose = nil
	fm.isQuit = false
}

// ---- 事件处理 ----

// HandleEvent 转发事件给 Session。
func (fm *Session) HandleEvent(event tcell.Event) {
	if !fm.isOpen {
		return
	}
	if _, ok := event.(*tcell.EventResize); ok {
		fm.close(Resize)
		return
	}
	switch e := event.(type) {
	case *tcell.EventKey:
		switch e.Key() {
		case tcell.KeyTab:
			fm.cycleFocus(+1)
			return
		case tcell.KeyBacktab:
			fm.cycleFocus(-1)
			return
		}
		// 键盘事件只送给当前 focus 的 KeyboardRegion
		if fm.focus < len(fm.keyboardRegions) {
			if fm.keyboardRegions[fm.focus].HandleKey(e) {
				return
			}
		}
		fm.handleGlobalKey(e)

	case *tcell.EventMouse:
		// mouse→switchFocus（FFM）：命中 keyboardRegions 才切；命中 preview 不切。
		// 纯 mouse move（Buttons()==0）不触发 FFM——避免滑过 history 时误改焦点。
		if e.Buttons() != 0 {
			if kbIdx := hitTest(fm.keyboardRegions, e); kbIdx >= 0 && kbIdx != fm.focus {
				fm.switchFocus(kbIdx)
			}
		}
		// 一般 mouse dispatch
		if idx := hitTest(fm.regions, e); idx >= 0 {
			fm.regions[idx].HandleMouse(e)
		}
	}
}

// NotifyBlur 由 owner 在失焦时调用。
func (fm *Session) NotifyBlur() {
	if fm.isOpen {
		fm.close(Esc)
	}
}

// handleGlobalKey 处理 session 级全局键（Esc/Ctrl-Q/q）。
func (fm *Session) handleGlobalKey(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyEscape:
		fm.close(Esc)
	case tcell.KeyCtrlQ:
		fm.close(Quit)
	case tcell.KeyRune:
		if ev.Rune() == 'q' {
			fm.close(Quit)
		}
	}
}

// ---- Focus 路由 ----

func (fm *Session) cycleFocus(delta int) {
	n := len(fm.keyboardRegions)
	if n == 0 {
		return
	}
	next := (fm.focus + delta + n) % n
	fm.switchFocus(next)
}

func (fm *Session) switchFocus(to int) {
	n := len(fm.keyboardRegions)
	if to < 0 || to >= n {
		return
	}
	if to == fm.focus {
		return
	}
	if fm.focus >= 0 && fm.focus < n {
		fm.keyboardRegions[fm.focus].FocusLost()
	}
	fm.focus = to
	fm.keyboardRegions[to].FocusOn()
}

// ---- 跨 region 协调 ----

func (fm *Session) syncPreview() {
	if fm.prev == nil {
		return
	}
	path := fm.list.selectedFilePath()
	if path == "" {
		fm.prev.Clear()
	} else {
		fm.prev.Load(path)
	}
	screen.Redraw()
}

// ---- Display ----

func (fm *Session) Display() {
	if !fm.isOpen {
		return
	}
	screen.Screen.HideCursor()
	fm.drawBorder()
	for _, r := range fm.regions {
		r.Display()
	}
}

// ---- hitTest ----

// hitTest 把鼠标坐标映射到传入切片的下标；命中 gap / 外框 / 越界 → -1。
func hitTest[T interface{ Rect() Rect }](items []T, ev *tcell.EventMouse) int {
	mx, my := ev.Position()
	for i, item := range items {
		r := item.Rect()
		if mx >= r.X && mx < r.X+r.W && my >= r.Y && my < r.Y+r.H {
			return i
		}
	}
	return -1
}

// ---- drawBorder ----

func (fm *Session) drawBorder() {
	x, y, w, h := fm.rect.X, fm.rect.Y, fm.rect.W, fm.rect.H
	sep := fm.previewX
	color := config.DefStyle

	// 1. clear 整个区域
	clearRect(x, y, w, h, color)

	// 2. 全幅上下 ─，分隔符列用交叉字符
	for i := 0; i < w; i++ {
		c := '─'
		if i == sep-x {
			c = '┬'
		}
		screen.Screen.SetContent(x+i, y, c, nil, color)

		c = '─'
		if i == sep-x {
			c = '┴'
		}
		screen.Screen.SetContent(x+i, y+h-1, c, nil, color)
	}

	// 3. 分隔符 │
	for row := 1; row < h-1; row++ {
		screen.Screen.SetContent(sep, y+row, '│', nil, color)
	}

	// 4. 上边框嵌 ──<title>──...─
	title := "Open File"
	col := x + 1
	write := func(r rune) {
		if col < x+w-1 {
			screen.Screen.SetContent(col, y, r, nil, color)
			col++
		}
	}
	write('─')
	write('─')
	for _, r := range title {
		write(r)
	}
	write('─')
	write('─')
	for col < x+w-1 {
		if col == sep {
			screen.Screen.SetContent(col, y, '┬', nil, color)
		} else {
			screen.Screen.SetContent(col, y, '─', nil, color)
		}
		col++
	}

	// 5. history 上分隔线（仅当 history 存在）：
	//    跨左栏 [x, sep) 在 fm.historyY 行画 ─，与预览列交点用 ┤ 表示横线终止于左栏。
	//    嵌入式标签 " Recent Paths " 与顶部 title 同风格：2 dashes + 空格 + 标签 + 空格 + 2 dashes。
	if fm.history != nil {
		labelRunes := []rune(" Recent Paths ")
		sepX := sep
		col := x
		writeR := func(r rune) {
			if col >= sepX {
				return
			}
			screen.Screen.SetContent(col, fm.historyY, r, nil, color)
			col++
		}
		writeR('─')
		writeR('─')
		for _, r := range labelRunes {
			writeR(r)
		}
		writeR('─')
		writeR('─')
		for col < sepX {
			screen.Screen.SetContent(col, fm.historyY, '─', nil, color)
			col++
		}
		screen.Screen.SetContent(sepX, fm.historyY, '┤', nil, color)
	}
}
