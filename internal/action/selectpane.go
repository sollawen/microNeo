package action

import (
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

// SelectPane 是 receiver 选择列表（具体浮窗，参见 docs/弹窗机制/弹窗框架设计.md §四.2）。
//
// v2 列表滚动：调用方传入 maxVisible / wrap，SelectPane 内部管理 topIdx（视口偏移）。
// 边框 / title / 清屏 / 锚点展开 / 几何 / 生命周期全部移交 FloatFrame，
// 本文件只保留业务逻辑（列表状态、键盘映射、回调通知）。
//
// 生命周期（设计 §五）：
//   - 调用方 new 一个 SelectPane，调 Open(...)，把 display / handleEvent 塞给 TheFloatFrame；
//   - 运行期 SelectPane 对象本身不再被主循环引用（D2'：用完即弃，靠 GC 回收）；
//   - 关闭由 handleEvent 内部发起：先 TheFloatFrame.Close() 再 onSelect(...)。
type SelectPane struct {
	items      []string
	selected   int   // 绝对选中索引（0..len-1）
	topIdx     int   // 视口顶端在 items 中的索引
	maxVisible int   // 视口上限（<=0 视为无上限）
	wrap       bool  // true: 上下循环；false: 到顶/底停住
	title      string
	onSelect   func(*string)
}

// NewSelectPane 返回空 SelectPane（未打开状态）。
func NewSelectPane() *SelectPane {
	return &SelectPane{}
}

// Open 打开选择浮窗。
//
//   items       候选项
//   title       上边框标签（如 "Receiver"）
//   anchor      触发锚点（屏坐标，通常来自光标位置）
//   frameColor  浮窗边框色；tcell.Style{} 零值 = config.DefStyle
//   maxVisible  视口最大可见行数；<=0 时 fallback 到 len(items)（不滚动）
//   wrap        true: 上下循环（默认）；false: 到顶/底停住
//   onSelect    回调：Enter → onSelect(&items[selected])；Esc / resize → onSelect(nil)
//
// 几何 / 边框 / 失败前置检查全部归 FloatFrame：SelectPane 不读屏幕尺寸、不碰任何几何逻辑。
// 如果 FloatFrame.Open 返回 false（拒绝再开 / 屏幕放不下），直接 onSelect(nil) 返回，
// 不设置任何业务状态——对调用方完全透明（与旧实现一致）。
func (s *SelectPane) Open(
	items []string,
	title string,
	anchor Pos,
	frameColor tcell.Style,
	maxVisible int,
	wrap bool,
	onSelect func(*string),
) {
	s.items = items
	s.selected = 0
	s.topIdx = 0
	s.maxVisible = maxVisible
	s.wrap = wrap
	s.title = title
	s.onSelect = onSelect

	// 算纯内容尺寸（宽含左右 padding 各 1；高为 maxVisible，过长时由 FloatFrame 拒绝）。
	visibleH := len(s.items)
	if maxVisible > 0 && visibleH > maxVisible {
		visibleH = maxVisible
	}
	contentSize := Size{
		W: maxItemLen(items) + 2,
		H: visibleH,
	}

	spec := FloatOpenSpec{
		Anchor:      anchor,
		ContentSize: contentSize,
		Title:       title,
		FrameColor:  frameColor,
		Display:     s.display,
		HandleEvent: s.handleEvent,
		AutoExpand:  true, // SelectPane: 贴光标/贴 statusLine 展开(旧行为)
		OnCancel: func() { // resize 即关时清理业务回调
			if s.onSelect != nil {
				s.onSelect(nil)
			}
		},
	}
	if !TheFloatFrame.Open(spec) {
		// 没开成：清状态 + 回调取消
		s.items = nil
		s.onSelect = nil
		if onSelect != nil {
			onSelect(nil)
		}
	}
}

// display 只画视口内 [topIdx, topIdx+visibleH) 的项；当前选中项 Reverse。
// 边框 / title 由 FloatFrame 负责。
func (s *SelectPane) display(area Rect) {
	revStyle := config.DefStyle.Reverse(true)
	visibleH := min(len(s.items), s.effMaxVisible())

	for vi := 0; vi < visibleH; vi++ {
		i := s.topIdx + vi
		if i >= len(s.items) {
			break
		}
		row := area.Y + vi
		style := config.DefStyle
		if i == s.selected {
			style = revStyle
		}
		// 左 padding
		screen.Screen.SetContent(area.X, row, ' ', nil, style)
		// 文本
		col := area.X + 1
		for _, r := range s.items[i] {
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

	// 滚动指示符（仅当内容溢出时）；style 跟随所在行（ADR A4 修订版）。
	if len(s.items) > visibleH {
		rightCol := area.X + area.W - 1
		topStyle := config.DefStyle
		if s.topIdx == s.selected {
			topStyle = revStyle
		}
		botStyle := config.DefStyle
		botIdx := s.topIdx + visibleH - 1
		if botIdx == s.selected {
			botStyle = revStyle
		}
		if s.topIdx > 0 {
			screen.Screen.SetContent(rightCol, area.Y, '▲', nil, topStyle)
		}
		if s.topIdx+visibleH < len(s.items) {
			screen.Screen.SetContent(rightCol, area.Y+visibleH-1, '▼', nil, botStyle)
		}
	}
}

// handleEvent 处理键盘事件。Up/Down 改选中态并跟随视口；Enter/Esc/Resize 先关容器再回调；
// 其它键吞掉（modal）。
func (s *SelectPane) handleEvent(event tcell.Event) {
	switch ev := event.(type) {
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyDown:
			if s.wrap {
				s.selected = (s.selected + 1) % len(s.items)
			} else if s.selected < len(s.items)-1 {
				s.selected++
			}
			s.ensureVisible()
			screen.Redraw()
			return
		case tcell.KeyUp:
			if s.wrap {
				s.selected = (s.selected - 1 + len(s.items)) % len(s.items)
			} else if s.selected > 0 {
				s.selected--
			}
			s.ensureVisible()
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

	}
}

// ensureVisible 把视口拉到包含 selected 的最小位置。
func (s *SelectPane) ensureVisible() {
	visibleH := min(len(s.items), s.effMaxVisible())
	if s.selected < s.topIdx {
		s.topIdx = s.selected
	}
	if s.selected >= s.topIdx+visibleH {
		s.topIdx = s.selected - visibleH + 1
	}
}

// effMaxVisible 返回实际生效的视口上限（maxVisible<=0 视为无上限）。
func (s *SelectPane) effMaxVisible() int {
	if s.maxVisible <= 0 {
		return len(s.items)
	}
	return s.maxVisible
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
