package action

import (
	"encoding/json"
	"net"
	"os"
	"strings"
	"time"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/display"
	"github.com/micro-editor/micro/v2/internal/eabp"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/micro/v2/internal/util"
	"github.com/micro-editor/tcell/v2"
)

// NotePane is a floating overlay pane for quick notes.
// It embeds *BufPane to get full editing capabilities while
// restricting available actions via a whitelist.
type NotePane struct {
	*BufPane
	isOpen           bool
	x, y             int
	width            int
	height           int
	filePath         string       // main editor buffer.AbsPath
	fileCursor       buffer.Loc   // captured cursor (X=col, Y=line)
	fileSelection    *[2]buffer.Loc // nil = no selection
	fileSelectionText string          // main editor's selected text, captured at open()
	selectedReceiver eabp.RegFile  // D12: 本次使用 + 下次缓存（Socket=="" 表示未缓存）
}

// TheNotePane is the global NotePane instance
var TheNotePane *NotePane

// MaxSelectionLines 是 selection 文字内联发送的行数上限。
// 超过此行数，selection.Text 不发送（仅发 Start/End 位置），
// 接收端 fallback 到 @path :lineA-lineB 让 LLM 自己读文件。
const MaxSelectionLines = 30

// NotePaneBindings is the whitelist KeyTree for NotePane
var NotePaneBindings *KeyTree

// allowedNotePaneActions is the whitelist of allowed action names
var allowedNotePaneActions = map[string]bool{
	// Cursor movement
	"CursorUp": true, "CursorDown": true, "CursorLeft": true, "CursorRight": true,
	"CursorPageUp": true, "CursorPageDown": true,
	"CursorStart": true, "CursorEnd": true,
	"CursorToViewTop": true, "CursorToViewCenter": true, "CursorToViewBottom": true,

	// Selection
	"SelectUp": true, "SelectDown": true, "SelectLeft": true, "SelectRight": true,
	"SelectToStart": true, "SelectToEnd": true,
	"SelectWordRight": true, "SelectWordLeft": true,
	"SelectSubWordRight": true, "SelectSubWordLeft": true,
	"SelectLine": true,
	"SelectToStartOfLine": true, "SelectToStartOfText": true,
	"SelectToStartOfTextToggle": true, "SelectToEndOfLine": true,
	"SelectPageUp": true, "SelectPageDown": true,
	"StartOfText": true, "StartOfTextToggle": true,
	"StartOfLine": true, "EndOfLine": true,

	// Paragraph navigation
	"ParagraphPrevious": true, "ParagraphNext": true,
	"SelectToParagraphPrevious": true, "SelectToParagraphNext": true,

	// Text editing
	"InsertNewline": true, "Backspace": true, "Delete": true, "InsertTab": true,
	"DeleteWordRight": true, "DeleteWordLeft": true,
	"DeleteSubWordRight": true, "DeleteSubWordLeft": true,
	"IndentLine": true, "OutdentLine": true,
	"IndentSelection": true, "OutdentSelection": true,

	// Scrolling
	"PageUp": true, "PageDown": true,
	"HalfPageUp": true, "HalfPageDown": true,
	"ScrollUpAction": true, "ScrollDownAction": true,
	"Center": true, "Start": true, "End": true,
	"ScrollUp": true, "ScrollDown": true,

	// Clipboard
	"Copy": true, "CopyLine": true, "Cut": true, "CutLine": true,
	"Paste": true, "PastePrimary": true,
	"Duplicate": true, "DuplicateLine": true,
	"DeleteLine": true, "MoveLinesUp": true, "MoveLinesDown": true,

	// Multi-cursor
	"SpawnMultiCursor": true, "SpawnMultiCursorUp": true,
	"SpawnMultiCursorDown": true, "SpawnMultiCursorSelect": true,
	"RemoveMultiCursor": true, "RemoveAllMultiCursors": true,
	"SkipMultiCursor": true, "SkipMultiCursorBack": true,

	// Other
	"JumpToMatchingBrace": true, "SelectAll": true,
	"Deselect": true, "Escape": true, "ToggleOverwriteMode": true,
	"ClearInfo": true, "ClearStatus": true, "None": true,

	// EABP send
	"NotePaneSend":             true,
	"NotePaneClose":           true,
	"NotePaneSwitchReceiver":  true,

	// Mouse
	"MousePress": true, "MouseDrag": true, "MouseRelease": true,
	"MouseMultiCursor": true,

	// Word navigation
	"WordRight": true, "WordLeft": true,
	"SubWordRight": true, "SubWordLeft": true,
}

// notePaneActions 是 notePane 专属 action 的私有注册表。
// notePaneRegisterBinding() 优先查它，找不到再 fallback 到全局 BufKeyActions。
// 这样 notePane 专属 action 不必污染 bufpane.go 的 BufKeyActions（原生文件零侵入）。
var notePaneActions = map[string]BufKeyAction{
	"NotePaneSend":           NotePaneSend,
	"NotePaneClose":          notePaneClose,
	"NotePaneSwitchReceiver": NotePaneSwitchReceiver,
}

// NotePaneSwitchReceiver 在 notePane 已开态下切换 receiver。
// 绑定 alt-i。只更新 selectedReceiver 字段，不调 open()（保留草稿）。
// 守卫：notePane 未开时静默 no-op（alt-i 在主编辑器无意义）。
func NotePaneSwitchReceiver(h *BufPane) bool {
	n := TheNotePane
	if n == nil || !n.IsOpen() {
		return false // 静默：notePane 没开时 alt-i 无效
	}

	// 1. Discover
	receivers, err := eabp.Discover()
	if err != nil {
		InfoBar.Message("✗ discover error: " + err.Error())
		return false
	}

	// 2. case 0：提示用户
	if len(receivers) == 0 {
		InfoBar.Message("✗ no receiver found")
		return false
	}

	// 3. case 1：直接赋值（不做"是否当前"判断 —— 赋值同一个值无副作用）
	if len(receivers) == 1 {
		n.selectedReceiver = receivers[0]
		// 不需要 screen.Redraw()：本函数在 DoEvent 事件回调里执行，
		// 改完字段下一帧 DoEvent 的 Display 阶段会无条件重画整个屏幕，
		// notePane.Display 会读到新的 selectedReceiver.Name。
		return true
	}

	// 4. case 2+：弹 SelectPane
	names := make([]string, len(receivers))
	for i, r := range receivers {
		names[i] = r.Name
	}
	// 锚点 = notePane 左上角。展开方向交给 FloatFrame 自适应（D13）。
	NewSelectPane().Open(names, "Receivers", Pos{X: n.x, Y: n.y}, tcell.Style{}, func(s *string) {
		if s == nil {
			// Esc：selectedReceiver 不变（伪代码明确要求）
			return
		}
		// Enter：找到 name 对应的 RegFile
		for _, r := range receivers {
			if r.Name == *s {
				n.selectedReceiver = r
				// 不需要 screen.Redraw()：本回调在 DoEvent 事件链里执行（SelectPane
				// 的 KeyEnter 经 FloatFrame.HandleEvent 走到这里），下一帧自然重画。
				return
			}
		}
		// 防御：items 来自 receivers，理论上到不了这
		InfoBar.Message("✗ internal: selected name not in receivers")
	})
	return true
}

// notePaneClose 关闭 notePane（不发送任何内容，符合 TUI "Esc = 取消" 约定）。
// 复用已有 (*NotePane).close()，不重写。
func notePaneClose(h *BufPane) bool {
	if n := TheNotePane; n != nil && n.IsOpen() {
		n.close()
	}
	return true
}

// notePaneOpen 从主编辑器打开 NotePane。
// 注册为主编辑器的 BufKeyAction，可走标准 bindings.json 机制覆盖默认键位。
// 守卫：notePane 已开态下重复触发是 no-op。
// 流程：discover → (1 个 / 缓存命中 / SelectPane 弹窗) → n.open(receiver)（D16：receiver 作为显式参数）。
func notePaneOpen(h *BufPane) bool {
	n := TheNotePane
	if n == nil {
		return false
	}
	if n.isOpen {
		return true  // 已开，幂等
	}

	// 1. Discover
	receivers, err := eabp.Discover()
	if err != nil {
		InfoBar.Message("✗ discover error: " + err.Error())
		return false
	}
	if len(receivers) == 0 {
		InfoBar.Message("✗ no receiver found")
		return false
	}

	// 2. case 1：直接赋值（无需查缓存，只有一个）
	if len(receivers) == 1 {
		n.open(receivers[0])
		return true
	}

	// 3. case 2+：先查缓存命中（Socket 在 receivers 列表里）
	if n.selectedReceiver.Socket != "" {
		for _, r := range receivers {
			if r.Socket == n.selectedReceiver.Socket {
				n.open(n.selectedReceiver)  // 命中，复用缓存
				return true
			}
		}
	}

	// 4. case 2+ 未命中：弹 SelectPane（D14：传真实锚点）
	pane := MainTab().CurPane()
	view := pane.BWindow.GetView()
	bw := pane.BWindow.(*display.BufWindow)
	lowestRow := n.lowestCursorScreenRow(bw, view)
	ax := view.X + 2
	ay := lowestRow + 1
	names := make([]string, len(receivers))
	for i, r := range receivers {
		names[i] = r.Name
	}
	NewSelectPane().Open(names, "Receiver", Pos{X: ax, Y: ay}, tcell.Style{}, func(s *string) {
		if s == nil {
			// Esc：清零缓存（走到此分支时缓存已失效，决策 14）
			n.selectedReceiver = eabp.RegFile{}
			InfoBar.Message("✗ 已取消")
			return
		}
		// Enter：找到 name 对应的 RegFile
		for _, r := range receivers {
			if r.Name == *s {
				n.open(r)
				return
			}
		}
		// 理论上不会到这（SelectPane 的 items 来自 receivers），防御性
		InfoBar.Message("✗ internal: selected name not in receivers")
	})
	return true
}

func init() {
	NotePaneBindings = NewKeyTree()
	notePaneMapDefaults(DefaultBindings("buffer"))
	// Bind Alt-Enter to NotePaneSend (fallback Alt-s if Alt-Enter not available in terminal)
	notePaneMapBinding("Alt-Enter", "NotePaneSend")
	// Bind Esc to NotePaneClose (cancel draft without sending — TUI convention).
	// 依赖 KeyTree 覆盖语义（后注册覆盖前注册），无需 DeleteBinding。
	notePaneMapBinding("Esc", "NotePaneClose")
	notePaneMapBinding("Alt-i", "NotePaneSwitchReceiver")
}

// notePaneMapDefaults registers allowed key bindings from defaults into NotePaneBindings.
// It uses the same mechanism as BufMapEvent but filters by whitelist.
func notePaneMapDefaults(defaults map[string]string) {
	for keyStr, actionStr := range defaults {
		notePaneMapBinding(keyStr, actionStr)
	}
}

// notePaneMapBinding maps a key string to action(s), filtering by whitelist.
// An action string can contain multiple actions separated by & | , (e.g. "Autocomplete|IndentSelection|InsertTab").
// Only allowed actions are kept; if none are allowed, the binding is skipped entirely.
func notePaneMapBinding(keyStr, actionStr string) {
	// Parse the key event
	ev, err := findEvent(keyStr)
	if err != nil {
		return
	}

	// Split action string by & | , and filter by whitelist
	filteredActions := filterActions(actionStr)
	if len(filteredActions) == 0 {
		return
	}

	// Rebuild the action string with only allowed actions
	filteredStr := strings.Join(filteredActions, "|")

	// Use the same binding mechanism as BufMapEvent
	notePaneRegisterBinding(ev, filteredStr)
}

// filterActions parses a composite action string (e.g. "Autocomplete|IndentSelection|InsertTab")
// and returns only the actions that are in the whitelist.
func filterActions(actionStr string) []string {
	var result []string
	for {
		idx := util.IndexAnyUnquoted(actionStr, "&|,")
		a := actionStr
		sep := byte('|')
		if idx >= 0 {
			a = actionStr[:idx]
			sep = actionStr[idx]
			actionStr = actionStr[idx+1:]
		} else {
			actionStr = ""
		}

		if isActionAllowed(a) {
			result = append(result, a)
		}

		if actionStr == "" {
			break
		}
		_ = sep // we rejoin with | regardless
	}
	return result
}

// isActionAllowed checks if an action name is in the whitelist.
// It also handles "command:" and "lua:" prefixed actions (which are not allowed).
func isActionAllowed(a string) bool {
	if strings.HasPrefix(a, "command:") || strings.HasPrefix(a, "command-edit:") || strings.HasPrefix(a, "lua:") {
		return false
	}
	return allowedNotePaneActions[a]
}

// notePaneRegisterBinding registers a filtered action binding to NotePaneBindings.
// This mirrors the logic in BufMapEvent but targets NotePaneBindings.
func notePaneRegisterBinding(k Event, action string) {
	var actionfns []BufAction
	for i := 0; ; i++ {
		if action == "" {
			break
		}

		idx := util.IndexAnyUnquoted(action, "&|,")
		a := action
		if idx >= 0 {
			a = action[:idx]
			action = action[idx+1:]
		} else {
			action = ""
		}

		var afn BufAction
		if f, ok := notePaneActions[a]; ok {   // notePane 专属 action 优先
			afn = f
		} else if f, ok := BufKeyActions[a]; ok {
			afn = f
		} else if f, ok := BufMouseActions[a]; ok {
			afn = f
		} else {
			continue
		}
		actionfns = append(actionfns, afn)
	}

	if len(actionfns) == 0 {
		return
	}

	bufAction := func(h *BufPane, te *tcell.EventMouse) bool {
		for _, a := range actionfns {
			var success bool
			h.Buf.SetCurCursor(0)
			h.Cursor = h.Buf.GetActiveCursor()
			success = h.execAction(a, "", te)
			_ = success
		}
		return true
	}

	switch e := k.(type) {
	case KeyEvent, KeySequenceEvent, RawEvent:
		NotePaneBindings.RegisterKeyBinding(e, BufKeyActionGeneral(func(h *BufPane) bool {
			return bufAction(h, nil)
		}))
	case MouseEvent:
		NotePaneBindings.RegisterMouseBinding(e, BufMouseActionGeneral(bufAction))
	}
}

// NewNotePane creates a new NotePane instance
func NewNotePane() *NotePane {
	n := &NotePane{
		height: 5,
	}

	// Create an in-memory scratch buffer. Content is discarded on close;
	// see buffer.BTScratch ("Cannot save scratch buffer" in save.go:237).
	buf := buffer.NewBufferFromString("", "", buffer.BTScratch)

	// Disable ruler for NotePane
	buf.SetOptionNative("ruler", false)

	// Create BufWindow with initial position (will be adjusted in open())
	win := display.NewBufWindow(0, 0, 80, n.height, buf)
	win.SetHideStatusLine(true)

	// Create BufPane using newBufPane (lowercase, does not trigger finishInitialize)
	n.BufPane = newBufPane(buf, win, nil)
	n.BufPane.bindings = NotePaneBindings

	return n
}

// IsOpen returns whether the NotePane is currently open
func (n *NotePane) IsOpen() bool {
	return n.isOpen
}

// open 接收 receiver 作为显式入参，并在内部第一行写入 selectedReceiver。
// 这样 receiver 状态的赋值点收敛到唯一入口（D16），消除"调用方提前 set"的隐式协议。
// selectedReceiver 字段语义不变：本次发送目标 + 下次缓存。
func (n *NotePane) open(receiver eabp.RegFile) {
	if n.isOpen {
		return
	}
	n.selectedReceiver = receiver
	pane := MainTab().CurPane()
	if pane == nil {
		return
	}

	// 兑现"打开 = 全新"承诺：关掉旧 buffer（如有），建新的
	if n.BufPane.Buf != nil {
		n.BufPane.Buf.Close()      // 从 OpenBuffers 移除 + 调 Fini 清理
	}
	buf := buffer.NewBufferFromString("", "", buffer.BTScratch)
	buf.SetOptionNative("ruler", false)
	nbw := n.BufPane.BWindow.(*display.BufWindow)
	nbw.SetBuffer(buf)             // 切 BufWindow.Buf + 装 OptionCallback
	n.BufPane.Buf = buf            // 同步 BufPane.Buf 引用

	// Calculate position via reposition
	n.reposition()

	n.isOpen = true
}

// reposition 重新计算 NotePane 在屏幕上的位置。
// 不修改 buffer 内容，可在已开态下重复调用（用于 resize）。
// 内部从 MainTab().CurPane() 取主编辑器 pane。
func (n *NotePane) reposition() {
	pane := MainTab().CurPane()
	if pane == nil {
		return
	}
	// 防御：主编辑器 buffer 被关（如最后一个 tab 关闭）时 pane.Buf 为 nil
	if pane.Buf == nil {
		return
	}

	// Capture file path from the main editor buffer
	n.filePath = pane.Buf.AbsPath

	// Get the pane's view and BufWindow
	view := pane.BWindow.GetView()
	bw := pane.BWindow.(*display.BufWindow)

	// 1. Find the lowest cursor/selection screen row
	lowestRow := n.lowestCursorScreenRow(bw, view)

	// 2. Calculate NotePane position
	n.x = view.X
	n.width = view.Width
	notePaneTopBorder := lowestRow + 1
	notePaneBottomBorder := notePaneTopBorder + n.height + 1

	// 3. If not enough space below, scroll the main editor up
	viewBottom := view.Y + view.Height
	if notePaneBottomBorder > viewBottom {
		deficit := notePaneBottomBorder - viewBottom + 2

		// Safety constraint: don't scroll cursor above scrollmargin
		scrollmargin := int(pane.Buf.Settings["scrollmargin"].(float64))
		maxDeficit := lowestRow - scrollmargin
		if deficit > maxDeficit {
			deficit = maxDeficit
		}

		if deficit > 0 {
			oldStartLine := view.StartLine
			view.StartLine = bw.Scroll(view.StartLine, deficit)
			lowestRow -= bw.Diff(oldStartLine, view.StartLine)
			notePaneTopBorder = lowestRow + 1
		}
	}

	// 4. Reposition BufWindow
	n.y = notePaneTopBorder
	nbw := n.BufPane.BWindow.(*display.BufWindow)
	nbw.X = n.x + 1
	nbw.Y = n.y + 1
	n.BufPane.Resize(n.width-2, n.height)
}

// NotePaneSend sends the note content via EABP to a receiver.
// It reads the current notePane buffer as message, discovers receivers,
// and sends the context payload to the unix socket of the single live receiver.
func NotePaneSend(h *BufPane) bool {
	n := TheNotePane
	if n == nil {
		return false
	}

	// 0. 空内容拦截（决策 1）：用户没写东西就按 Alt-Enter → 提示 + 直接关闭，不发送。
	// "打开 pane + 不写 + 发送" 无合理用户意图，避免空 message 走完整 eabp 链路。
	message := string(n.BufPane.Buf.Bytes())
	if strings.TrimSpace(message) == "" {
		InfoBar.Message("✗ 内容为空,未发送")
		n.close()
		return false
	}

	// 1. 用 notePaneOpen 时确定的 receiver（决策 1：Discover 已前移）
	receiver := n.selectedReceiver

	// 2. Build payload
	// Convert buffer.Loc {X,Y} = {col,row} (0-based) to EABP Position {Line,Col} = {row,col} (1-based)
	cursorPos := eabp.Position{Line: n.fileCursor.Y + 1, Col: n.fileCursor.X + 1}

	// message 已在守卫里读取（TrimSpace 判空后必非空）
	payload := eabp.ContextPayload{
		Path:    n.filePath,
		Cursor:  cursorPos,
		Message: message,
	}

	// Selection: pre-captured and normalized at open() time
	if n.fileSelection != nil {
		payload.Selection = &eabp.Selection{
			Start: eabp.Position{Line: n.fileSelection[0].Y + 1, Col: n.fileSelection[0].X + 1},
			End:   eabp.Position{Line: n.fileSelection[1].Y + 1, Col: n.fileSelection[1].X + 1},
			Text:  n.fileSelectionText,
		}
	}

	// Serialize payload
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		InfoBar.Message("✗ marshal error: " + err.Error())
		return false
	}

	// 3. Build envelope
	env := eabp.Envelope{
		V:       1,
		Type:    "context",
		Sender:  eabp.Sender{PID: os.Getpid(), Name: "microNeo", Instance: "default"},
		TS:      float64(time.Now().UnixNano()) / 1e9,
		Payload: payloadJSON,
	}

	// 4. Dial receiver's unix socket, write JSON line, close
	c, err := net.Dial("unix", receiver.Socket)
	if err != nil {
		InfoBar.Message("✗ send failed: " + err.Error())
		return false
	}
	lineBytes, err := env.MarshalLine()
	if err != nil {
		c.Close()
		InfoBar.Message("✗ marshal error: " + err.Error())
		return false
	}
	if _, err := c.Write(lineBytes); err != nil {
		c.Close()
		InfoBar.Message("✗ send failed: " + err.Error())
		return false
	}
	c.Close()

	// 5. Success: close notePane and show confirmation
	n.close()
	InfoBar.Message("✓ sent to " + receiver.Name)
	return false
}

// locToScreenRow converts a buffer location to its screen row.
// It correctly handles softwrap by using SLocFromLoc and Diff.
func (n *NotePane) locToScreenRow(bw *display.BufWindow, view *display.View, loc buffer.Loc) int {
	sloc := bw.SLocFromLoc(loc)
	if bw.Buf.IsMD {
		if row, ok := bw.LineToScreenRow(sloc.Line, sloc.Row); ok {
			return row + view.Y
		}
	}
	row := bw.Diff(view.StartLine, sloc)
	return row + view.Y
}

// lowestCursorScreenRow finds the lowest screen row among all cursors and selections.
// For cursors with selection, it uses the bottom of the selection (max Y).
func (n *NotePane) lowestCursorScreenRow(bw *display.BufWindow, view *display.View) int {
	lowestRow := -1

	for _, cursor := range bw.Buf.GetCursors() {
		var loc buffer.Loc
		if cursor.HasSelection() {
			// Use the selection endpoint with larger Y
			sel := cursor.CurSelection
			if sel[0].Y > sel[1].Y {
				loc = sel[0]
			} else {
				loc = sel[1]
			}
		} else {
			loc = cursor.Loc
		}

		screenRow := n.locToScreenRow(bw, view, loc)
		if screenRow > lowestRow {
			lowestRow = screenRow
			// Capture cursor position and selection
			n.fileCursor = loc
			if cursor.HasSelection() {
				sel := cursor.CurSelection
				start, end := sel[0], sel[1]
				if start.GreaterThan(end) {
					start, end = end, start
				}
				normalized := [2]buffer.Loc{start, end}
				n.fileSelection = &normalized
				selText := string(bw.Buf.Substr(start, end))
				lineSpan := end.Y - start.Y + 1
				if lineSpan > MaxSelectionLines {
					selText = ""
				}
				n.fileSelectionText = selText
			} else {
				n.fileSelection = nil
				n.fileSelectionText = ""
			}
		}
	}

	return lowestRow
}

// close closes the NotePane
func (n *NotePane) close() {
	n.isOpen = false

	// Restore main editor's normal scroll position
	if pane := MainTab().CurPane(); pane != nil {
		pane.BWindow.Relocate()
	}
}

// HandleEvent handles keyboard events for the NotePane
func (n *NotePane) HandleEvent(event tcell.Event) {
	if _, ok := event.(*tcell.EventResize); ok {
		if n.isOpen {
			n.reposition()  // ← changed from n.open()
		}
		return
	}
	n.BufPane.HandleEvent(event)
}

// Display renders the NotePane on the screen
func (n *NotePane) Display() {
	if !n.isOpen {
		return
	}

	// Clear the entire NotePane area (border + content) to hide underlying editor content
	for row := 0; row < n.height+2; row++ {
		for col := 0; col < n.width; col++ {
			screen.Screen.SetContent(n.x+col, n.y+row, ' ', nil, config.DefStyle)
		}
	}

	// Box-drawing characters
	topLeft := '┌'
	topRight := '┐'
	bottomLeft := '└'
	bottomRight := '┘'
	horizontal := '─'
	vertical := '│'

	// Draw top border
	screen.Screen.SetContent(n.x, n.y, topLeft, nil, config.DefStyle)
	for i := 1; i < n.width-1; i++ {
		screen.Screen.SetContent(n.x+i, n.y, horizontal, nil, config.DefStyle)
	}
	screen.Screen.SetContent(n.x+n.width-1, n.y, topRight, nil, config.DefStyle)

	// 嵌入 receiver 名字到上边框（直接替换横线，无箭头）
	name := n.selectedReceiver.Name
	if name != "" {
		// 截断：上边框可用宽度 = n.width - 2（去边框）；名字段上限 = 可用 / 2
		avail := n.width - 2
		nameCap := avail / 2
		if len(name) > nameCap {
			name = name[:nameCap]
		}
		// 从位置 n.x+2 开始写入名字（保留 ┌- 前缀）
		for i, ch := range name {
			if n.x+2+i < n.x+n.width-1 { // 不覆盖右上角
				screen.Screen.SetContent(n.x+2+i, n.y, ch, nil, config.DefStyle)
			}
		}
	}

	// Draw bottom border
	screen.Screen.SetContent(n.x, n.y+n.height+1, bottomLeft, nil, config.DefStyle)
	for i := 1; i < n.width-1; i++ {
		screen.Screen.SetContent(n.x+i, n.y+n.height+1, horizontal, nil, config.DefStyle)
	}
	screen.Screen.SetContent(n.x+n.width-1, n.y+n.height+1, bottomRight, nil, config.DefStyle)

	// Draw side borders
	for row := 0; row < n.height; row++ {
		screenY := n.y + 1 + row
		screen.Screen.SetContent(n.x, screenY, vertical, nil, config.DefStyle)
		screen.Screen.SetContent(n.x+n.width-1, screenY, vertical, nil, config.DefStyle)
	}

	// Display the BufWindow content (includes cursor)
	n.BufPane.BWindow.Display()
}
