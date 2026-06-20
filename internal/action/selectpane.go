package action

import (
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

// SelectPane 是 receiver 选择列表（具体浮窗，参见 docs/弹窗机制/弹窗框架设计.md §四.2）。
//
// v1 整文件重写：边框 / title / 清屏 / 锚点展开 / 几何 / 生命周期全部移交 FloatFrame，
// 本文件只保留业务逻辑（列表状态、键盘映射、回调通知）。
//
// 生命周期（设计 §五）：
//   - 调用方 new 一个 SelectPane，调 Open(...)，把 display / handleEvent 塞给 TheFloatFrame；
//   - 运行期 SelectPane 对象本身不再被主循环引用（D2'：用完即弃，靠 GC 回收）；
//   - 关闭由 handleEvent 内部发起：先 TheFloatFrame.Close() 再 onSelect(...)。
type SelectPane struct {
	items    []string
	selected int
	title    string
	onSelect func(*string)
}

// NewSelectPane 返回空 SelectPane（未打开状态）。
func NewSelectPane() *SelectPane {
	return &SelectPane{}
}

// Open 打开选择浮窗。
//
//   items      候选项
//   title      上边框标签（如 "Receiver"）
//   anchor     触发锚点（屏坐标，通常来自光标位置）
//   frameColor 浮窗边框色；tcell.Style{} 零值 = config.DefStyle（D-2 决定 v1 用零值）
//   onSelect   回调：Enter → onSelect(&items[selected])；Esc / resize → onSelect(nil)
//
// 几何 / 边框 / 失败前置检查全部归 FloatFrame：SelectPane 不读屏幕尺寸、不碰任何几何逻辑。
// 如果 FloatFrame.Open 返回 false（C1 拒绝再开 / 屏幕放不下），直接 onSelect(nil) 返回，
// 不设置任何业务状态——对调用方完全透明（与旧实现一致）。
func (s *SelectPane) Open(
	items []string,
	title string,
	anchor Pos,
	frameColor tcell.Style,
	onSelect func(*string),
) {
	s.items = items
	s.selected = 0
	s.title = title
	s.onSelect = onSelect

	// 算纯内容尺寸（宽含左右 padding 各 1；高写死上限 10，不滚动）。
	// 边框 / title 撑宽都不归 SelectPane 管，由 FloatFrame 派生。
	contentSize := Size{
		W: maxItemLen(items) + 2,
		H: min(len(items), 10),
	}

	if !TheFloatFrame.Open(anchor, contentSize, title, frameColor, s.display, s.handleEvent) {
		// 没开成（FloatFrame 已有浮窗在打开 / 屏幕放不下）：清状态 + 回调取消
		s.items = nil
		s.onSelect = nil
		if onSelect != nil {
			onSelect(nil)
		}
	}
}

// display 只画 items 列表；当前选中项 Reverse。边框 / title 由 FloatFrame 负责。
func (s *SelectPane) display(area Rect) {
	revStyle := config.DefStyle.Reverse(true)

	for i, item := range s.items {
		if i >= area.H {
			break
		}
		row := area.Y + i
		style := config.DefStyle
		if i == s.selected {
			style = revStyle
		}
		// 左 padding
		screen.Screen.SetContent(area.X, row, ' ', nil, style)
		// 文本
		col := area.X + 1
		for _, r := range item {
			if col >= area.X+area.W {
				break
			}
			screen.Screen.SetContent(col, row, r, nil, style)
			col++
		}
		// 右 padding（保持 Reverse 视觉连续）
		for col < area.X+area.W {
			screen.Screen.SetContent(col, row, ' ', nil, style)
			col++
		}
	}
}

// handleEvent 处理键盘事件。Up/Down 改选中态；Enter/Esc/Resize 先关容器再回调；
// 其它键吞掉（modal）。
func (s *SelectPane) handleEvent(event tcell.Event) {
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
			// 回调顺序：先关容器，再回调（设计 §五）
			picked := s.items[s.selected]
			onSelect := s.onSelect
			TheFloatFrame.Close()
			if onSelect != nil {
				onSelect(&picked)
			}
			return
		case tcell.KeyEscape:
			onSelect := s.onSelect
			TheFloatFrame.Close()
			if onSelect != nil {
				onSelect(nil)
			}
			return
		}
		// 其它键（含字母 j / Ctrl-X 等）：完全吞掉
	case *tcell.EventResize:
		// ADR-9：resize 等同 Esc（Close + onSelect(nil)）
		onSelect := s.onSelect
		TheFloatFrame.Close()
		if onSelect != nil {
			onSelect(nil)
		}
		return
	}
}

// maxItemLen 返回 items 中最长项的字符数（rune 计数）。
func maxItemLen(items []string) int {
	m := 0
	for _, it := range items {
		if len(it) > m {
			m = len(it)
		}
	}
	return m
}

// min 返回 a/b 中较小者。
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
