package action

import (
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

// SelectPane 通用列表选择浮窗（D13 设计文档 §三）。
// 单例全局变量 TheSelectPane 在 globals.go 初始化。
//
// 当前用途：作为 D13 测试 scaffolding，触发热键是 Alt-I（见 defaults_other.go）。
// D12 集成时会重构——可能保留 BufPane forwarding，也可能换接入点。
type SelectPane struct {
	items    []string
	selected int
	title    string
	onSelect func(*string)

	x, y      int // 浮层左上角（屏坐标）
	width     int // 含边框（边框 2 + 左右 padding 2 + 文本）
	height    int // 内容高度（不含边框），<= maxHeight
	maxHeight int // v1 写死 10

	isOpen bool
}

var TheSelectPane *SelectPane

// NewSelectPane 构造空 SelectPane（未打开状态）。
func NewSelectPane() *SelectPane {
	return &SelectPane{
		maxHeight: 10,
	}
}

// Open 打开选择浮窗。
//   items    候选项
//   title    上边框标签（如 "test" / "Receiver" / "File"）
//   x, y     锚点（屏坐标）。均为 nil 时走旧逻辑（左下角、向上展开）
//            两者都非 nil 时按 §四自适应展开
//   onSelect 回调：Enter → onSelect(&items[selected])；Esc → onSelect(nil)；
//            屏幕太小放不下（见 §4.6）→ onSelect(nil)，浮窗不 open
func (s *SelectPane) Open(items []string, title string, x, y *int, onSelect func(*string)) {
	if s.isOpen {
		s.Close()
	}
	s.items = items
	s.selected = 0
	s.title = title
	s.onSelect = onSelect

	// 宽度：需要同时满足内容区和 title 显示
	// 内容区：边框(2) + padding(2) + maxContentLen = maxContentLen + 4
	// title区：边框(2) + ──(2) + title + ──(2) = len(title) + 6
	maxItemLen := 0
	for _, it := range items {
		if len(it) > maxItemLen {
			maxItemLen = len(it)
		}
	}
	contentWidth := maxItemLen + 4
	titleWidth := len(title) + 6
	if contentWidth > titleWidth {
		s.width = contentWidth
	} else {
		s.width = titleWidth
	}

	// 高度：写死 maxHeight=10（不滚动）
	s.height = len(items)
	if s.height > s.maxHeight {
		s.height = s.maxHeight
	}

	// 位置算法：锚点自适应（D13-SelectPane设计.md §3.2）
	w, h := screen.Screen.Size()
	iOffset := config.GetInfoBarOffset()
	statusLineY := h - iOffset - 1
	bottomLimit := statusLineY - 1 // statusLine 上方 1 row 即显示底边界
	paneH := s.height + 2         // 含上下边框的完整高度
	paneW := s.width

	// §3.3 nil 分支：任一为 nil → 旧逻辑（左下角、向上展开）
	if x == nil || y == nil {
		s.x = 0
		s.y = statusLineY - s.height - 2
		s.isOpen = true
		screen.Redraw()
		return
	}

	// §4.6 失败前置检查：屏幕完全放不下则放弃 open
	// 失败时不修改任何状态（isOpen 保持 false），不 set x/y，不 Redraw，直接 onSelect(nil) 返回
	if paneH > bottomLimit+1 || paneW > w {
		if s.onSelect != nil {
			s.onSelect(nil)
		}
		return
	}

	// §4 自适应展开算法：y 与 x 各自独立、各自对称
	ax, ay := *x, *y

	// §4.3 y 方向
	downSpace := bottomLimit - ay + 1 // [ay, bottomLimit] 的行数
	upSpace := ay + 1                 // [0, ay] 的行数
	switch {
	case downSpace >= paneH:
		s.y = ay // 向下:锚点 = 顶边
	case upSpace >= paneH:
		s.y = ay - paneH + 1 // 向上:锚点 = 底边
	case downSpace >= upSpace:
		s.y = ay // 兜底:偏向下
	default:
		s.y = ay - paneH + 1 // 兜底:偏向上
	}
	// 边界保护（防御性：§4.6 已判定 paneH ≤ bottomLimit+1）
	maxY := bottomLimit - paneH + 1
	if maxY < 0 {
		maxY = 0
	}
	if s.y > maxY {
		s.y = maxY
	}
	if s.y < 0 {
		s.y = 0
	}

	// §4.4 x 方向（与 y 对称）
	rightSpace := w - ax // [ax, w-1] 的列数
	leftSpace := ax + 1  // [0, ax] 的列数
	switch {
	case rightSpace >= paneW:
		s.x = ax // 向右:锚点 = 左边
	case leftSpace >= paneW:
		s.x = ax - paneW + 1 // 向左:锚点 = 右边
	case rightSpace >= leftSpace:
		s.x = ax // 兜底:偏向右
	default:
		s.x = ax - paneW + 1 // 兜底:偏向左
	}
	// 边界保护（防御性：§4.6 已判定 paneW ≤ w）
	maxX := w - paneW
	if maxX < 0 {
		maxX = 0
	}
	if s.x > maxX {
		s.x = maxX
	}
	if s.x < 0 {
		s.x = 0
	}

	s.isOpen = true
	screen.Redraw()
}

// Close 关闭浮窗。
func (s *SelectPane) Close() {
	if !s.isOpen {
		return
	}
	s.isOpen = false
	screen.Redraw()
}

// IsOpen 返回是否打开。
func (s *SelectPane) IsOpen() bool {
	return s.isOpen
}

// HandleEvent 处理键盘事件。
// 由 bufpane.go 的 forwarding 转发到此（见 bufpane.go HandleEvent 顶部）。
func (s *SelectPane) HandleEvent(event tcell.Event) {
	switch ev := event.(type) {
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyUp:
			s.selected = (s.selected - 1 + len(s.items)) % len(s.items)
			screen.Redraw()
			return
		case tcell.KeyDown:
			s.selected = (s.selected + 1) % len(s.items)
			screen.Redraw()
			return
		case tcell.KeyEnter:
			// 拷贝值，避免 Close 后指针悬空
			picked := s.items[s.selected]
			s.Close()
			if s.onSelect != nil {
				s.onSelect(&picked)
			}
			return
		case tcell.KeyEscape:
			s.Close()
			if s.onSelect != nil {
				s.onSelect(nil)
			}
			return
		}
		// 其它键（含字母 j / Ctrl-X 等）：完全吞掉
	}
}

// Display 渲染浮窗。
// 由 bufpane.go 的 Display 方法转发到此（见 bufpane.go Display 末尾）。
func (s *SelectPane) Display() {
	if !s.isOpen {
		return
	}

	revStyle := config.DefStyle.Reverse(true)

	// 1. Clear 整个矩形（边框 + 内容）
	for row := 0; row < s.height+2; row++ {
		for col := 0; col < s.width; col++ {
			screen.Screen.SetContent(s.x+col, s.y+row, ' ', nil, config.DefStyle)
		}
	}

	// 2. 4 角 + 上下边
	screen.Screen.SetContent(s.x, s.y, '┌', nil, config.DefStyle)
	screen.Screen.SetContent(s.x+s.width-1, s.y, '┐', nil, config.DefStyle)
	screen.Screen.SetContent(s.x, s.y+s.height+1, '└', nil, config.DefStyle)
	screen.Screen.SetContent(s.x+s.width-1, s.y+s.height+1, '┘', nil, config.DefStyle)
	for i := 1; i < s.width-1; i++ {
		screen.Screen.SetContent(s.x+i, s.y, '─', nil, config.DefStyle)
		screen.Screen.SetContent(s.x+i, s.y+s.height+1, '─', nil, config.DefStyle)
	}

	// 3. 左边 + 右边
	for row := 0; row < s.height; row++ {
		screenY := s.y + 1 + row
		screen.Screen.SetContent(s.x, screenY, '│', nil, config.DefStyle)
		screen.Screen.SetContent(s.x+s.width-1, screenY, '│', nil, config.DefStyle)
	}

	// 4. 上边框嵌入 "──<title>──...─"
	col := s.x + 1
	write := func(r rune, style tcell.Style) {
		if col < s.x+s.width-1 {
			screen.Screen.SetContent(col, s.y, r, nil, style)
			col++
		}
	}
	write('─', config.DefStyle) // ──
	write('─', config.DefStyle)
	for _, r := range s.title { // <title>
		write(r, config.DefStyle)
	}
	write('─', config.DefStyle) // ──
	write('─', config.DefStyle)
	for col < s.x+s.width-1 { // 余下填 ─
		write('─', config.DefStyle)
	}

	// 5. 内容列表（当前选中项 Reverse）
	for i, item := range s.items {
		if i >= s.height {
			break
		}
		row := s.y + 1 + i
		style := config.DefStyle
		if i == s.selected {
			style = revStyle
		}
		// 左 padding
		screen.Screen.SetContent(s.x+1, row, ' ', nil, style)
		// 文本
		col := s.x + 2
		for _, r := range item {
			if col >= s.x+s.width-1 {
				break
			}
			screen.Screen.SetContent(col, row, r, nil, style)
			col++
		}
		// 右 padding（保持 Reverse 视觉连续）
		for col < s.x+s.width-1 {
			screen.Screen.SetContent(col, row, ' ', nil, style)
			col++
		}
	}
}

