package finder

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

// ---- 持久化 ----

const historyMaxEntries = 50

// historyPath 返回 $ConfigDir/history.json 的绝对路径。
func historyPath() string {
	return filepath.Join(config.ConfigDir, "history.json")
}

// writeDirHistory 同步序列化并写入整个 history.json。所有写操作共用。
func writeDirHistory(dirs []string) {
	if err := os.MkdirAll(config.ConfigDir, os.ModePerm); err != nil {
		return
	}
	data, err := json.MarshalIndent(dirs, "", "    ")
	if err != nil {
		return
	}
	_ = os.WriteFile(historyPath(), data, 0o644)
}

// readHistory 读取访问历史。文件不存在 → 空；空文件或损坏 JSON → 删文件返回空；
// 其他读取错误 → 返回空不删文件。返回新 slice，调用方可自由修改。
func readHistory() []string {
	path := historyPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{}
	}
	if len(data) == 0 {
		_ = os.Remove(path)
		return []string{}
	}
	var raw []string
	if err := json.Unmarshal(data, &raw); err != nil {
		_ = os.Remove(path)
		return []string{}
	}
	dirs := make([]string, 0, len(raw))
	for _, d := range raw {
		if len(d) > 0 && d[len(d)-1] == filepath.Separator {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// writeHistory 把原始字符串推到队首，去重并截断到 historyMaxEntries 后整体写回。
// 本函数不区分目录与文件；目录的尾分隔符由调用方保证。空路径或 I/O 失败静默降级。
func writeHistory(dir string) {
	if dir == "" {
		return
	}
	dirs := readHistory()

	out := make([]string, 0, len(dirs)+1)
	out = append(out, dir)
	for _, d := range dirs {
		if d == dir {
			continue
		}
		out = append(out, d)
		if len(out) >= historyMaxEntries {
			break
		}
	}
	if len(out) > historyMaxEntries {
		out = out[:historyMaxEntries]
	}
	writeDirHistory(out)
}

// ---- region ----

// historyList 是底部「访问目录历史」region，实现 KeyboardRegion。
type historyList struct {
	fm      *Session
	rect    Rect     // 内容区矩形（不含上方分隔线）；Open 注入，生命周期内不变
	dirs    []string // 队首最新；空 = region 占位待清理
	cursor  int      // 0-based 行内索引
	topIdx  int      // 当前可见首行在 dirs 中的索引
	focused bool     // 当前 KeyboardRegion focus
}

func newHistoryList(fm *Session, rect Rect, dirs []string) *historyList {
	return &historyList{
		fm:      fm,
		rect:    rect,
		dirs:    dirs,
		cursor:  0,
		topIdx:  0,
		focused: false,
	}
}

func (h *historyList) Rect() Rect { return h.rect }

// Display 清屏 + 画每行（focus 反白、左侧路径截断、右侧滚动指示）。
func (h *historyList) Display() {
	defStyle := config.DefStyle
	revStyle := defStyle.Reverse(true)

	x := h.rect.X
	y := h.rect.Y
	w := h.rect.W
	visibleH := h.rect.H
	if visibleH < 0 {
		visibleH = 0
	}

	clearRect(x, y, w, visibleH, defStyle)
	if visibleH == 0 {
		return
	}

	scrollCol := x + w - 1
	nameLimit := w - 1

	total := len(h.dirs)
	lh := visibleH
	if lh > total {
		lh = total
	}

	for vi := 0; vi < lh; vi++ {
		idx := h.topIdx + vi
		path := h.dirs[idx]
		path = shortenHomePath(path)
		selected := h.focused && idx == h.cursor

		style := defStyle
		if selected {
			style = revStyle
		}

		disp := truncateLeftPath(path, nameLimit)
		col := x
		for _, r := range disp {
			rw := runeWidth(r)
			if col+rw > scrollCol {
				break
			}
			screen.Screen.SetContent(col, y+vi, r, nil, style)
			col += rw
		}
		for col < scrollCol {
			screen.Screen.SetContent(col, y+vi, ' ', nil, style)
			col++
		}
	}

	if total > lh && lh > 0 {
		topStyle := defStyle
		if h.topIdx > 0 && (h.focused && h.topIdx == h.cursor) {
			topStyle = revStyle
		}
		botStyle := defStyle
		if h.topIdx+lh < total && (h.focused && h.topIdx+lh-1 == h.cursor) {
			botStyle = revStyle
		}
		if h.topIdx > 0 {
			screen.Screen.SetContent(scrollCol, y, '▲', nil, topStyle)
		}
		if h.topIdx+lh < total {
			screen.Screen.SetContent(scrollCol, y+lh-1, '▼', nil, botStyle)
		}
	}
}

// HandleMouse 鼠标：click 选中行，wheel 等价 Up/Down，move 返回 false 让上层走 dispatch。
func (h *historyList) HandleMouse(ev *tcell.EventMouse) bool {
	switch ev.Buttons() {
	case tcell.Button1:
		_, my := ev.Position()
		target, ok := h.rowAt(my)
		if !ok {
			return false
		}
		if target == h.cursor {
			return true
		}
		h.moveCursor(target - h.cursor)
		return true
	case tcell.WheelUp:
		h.moveCursor(-1)
		return true
	case tcell.WheelDown:
		h.moveCursor(1)
		return true
	}
	return false
}

// rowAt 把屏坐标 my 映射成目标 cursor（0-based）。
func (h *historyList) rowAt(my int) (idx int, ok bool) {
	if my < h.rect.Y || my >= h.rect.Y+h.rect.H {
		return 0, false
	}
	row := my - h.rect.Y
	lh := len(h.dirs)
	if lh > h.rect.H {
		lh = h.rect.H
	}
	if row >= lh {
		return 0, false
	}
	return h.topIdx + row, true
}

// HandleKey 键盘：Up/Down 改 cursor，Enter/Right 激活，其它键返回 false。
func (h *historyList) HandleKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyUp:
		h.moveCursor(-1)
		return true
	case tcell.KeyDown:
		h.moveCursor(+1)
		return true
	case tcell.KeyEnter:
		h.activate()
		return true
	case tcell.KeyRight:
		h.activate()
		return true
	}
	return false
}

// FocusOn 切换聚焦状态并触发重绘。cursor/topIdx 不动。
func (h *historyList) FocusOn() {
	h.focused = true
	screen.Redraw()
}

// FocusLost 取消聚焦。cursor/topIdx 保留。
func (h *historyList) FocusLost() {
	h.focused = false
	screen.Redraw()
}

func (h *historyList) moveCursor(delta int) {
	n := len(h.dirs)
	if n == 0 {
		h.cursor = 0
		h.topIdx = 0
		return
	}
	h.cursor += delta
	if h.cursor < 0 {
		h.cursor = 0
	}
	if h.cursor > n-1 {
		h.cursor = n - 1
	}
	h.ensureVisible()
	screen.Redraw()
}

func (h *historyList) ensureVisible() {
	visibleH := h.rect.H
	if visibleH <= 0 {
		return
	}
	if h.cursor < h.topIdx {
		h.topIdx = h.cursor
	}
	if h.cursor >= h.topIdx+visibleH {
		h.topIdx = h.cursor - visibleH + 1
	}
	if h.topIdx < 0 {
		h.topIdx = 0
	}
}

// activate 对当前 cursor 路径 stat。有效目录 → Session.ActivateFromHistory；
// 其余（不存在 / 已不是目录 / stat 失败）一律从列表与 history.json 中移除。
func (h *historyList) activate() {
	if h.cursor < 0 || h.cursor >= len(h.dirs) {
		return
	}
	dir := h.dirs[h.cursor]
	info, err := os.Stat(dir)
	if err == nil && info.IsDir() {
		h.fm.ActivateFromHistory(dir)
		return
	}
	h.removeCurrent()
}

// removeCurrent 删除 cursor 处条目并持久化，调整 cursor/topIdx 防越界。
func (h *historyList) removeCurrent() {
	if h.cursor < 0 || h.cursor >= len(h.dirs) {
		return
	}
	h.dirs = append(h.dirs[:h.cursor], h.dirs[h.cursor+1:]...)
	if h.cursor >= len(h.dirs) {
		h.cursor = len(h.dirs) - 1
	}
	if h.cursor < 0 {
		h.cursor = 0
	}
	h.ensureVisible()
	cpy := make([]string, len(h.dirs))
	copy(cpy, h.dirs)
	writeDirHistory(cpy)
	screen.Redraw()
}
