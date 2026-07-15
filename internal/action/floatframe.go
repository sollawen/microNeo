package action

import (
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

// FloatFrame 是 microNeo 的统一浮窗框架容器（v1，参见 docs/弹窗机制/弹窗框架设计.md）。
//
// 容器只做所有浮窗共享的事：锚点展开、画框、清屏、生命周期、事件路由。
// 业务能力（列表选择、滚动、动态刷新等）全部下沉到具体浮窗——具体浮窗在打开时
// 把内容尺寸、锚点、title、边框色、display 函数值、handleEvent 函数值一次性塞给 Open，
// 由 FloatFrame 接管用户交互；关闭时清空内部状态，回到空壳。
//
// 关键约束（设计 §一）：C1 单浮窗（同时全局最多一个）、C2 无嵌套、C3 模态、C5 框架要薄。
//
// 几何语义：Open 入参 contentSize 是纯内容尺寸（不含边框）；FloatFrame 内部派生
// 含边框外尺寸 outerW = max(contentSize.W+2, len(title)+6)、outerH = contentSize.H+2，
// 再做锚点展开得到 (fx, fy)。委托画内容时 contentArea 始终是
// Rect{fx+1, fy+1, contentSize.W, contentSize.H}——具体浮窗完全不需要知道边框存在。

// Rect 是屏坐标的一个矩形区域。
type Rect struct {
	X, Y, W, H int
}

// Pos 是屏坐标的一个点（用于锚点）。
type Pos struct {
	X, Y int
}

// Size 是宽高（用于内容尺寸）。
type Size struct {
	W, H int
}

// FloatOpenSpec 是打开浮窗的全部入参（options 模式）。
type FloatOpenSpec struct {
	Anchor      Pos                             // AutoExpand=true: 展开中心点; false: 外矩形左上角(含边框)
	ContentSize Size                            // 纯内容尺寸(不含边框); FloatFrame 内部派生 outerW/outerH
	Title       string                          // 嵌入上边框的标签; 空串=纯横线
	FrameColor  tcell.Style                     // 边框色; 零值 = config.DefStyle
	Display     func(contentArea Rect)          // 画内容(收到的 area 已扣除边框)
	HandleEvent func(event tcell.Event)         // 处理键事件(resize 不会到达, FloatFrame 已拦截)
	OnResize    func()                          // 仅 resize 导致容器自关时触发(ESC 取消/主动 Close 都不触发); 具体浮窗在此清理业务回调
	AutoExpand  bool                            // true: 锚点自适应展开(SelectPane); false: 钉死 Anchor(FileSelector)
}

// FloatFrame 容器本体。
type FloatFrame struct {
	isOpen bool

	// —— 从具体浮窗传入（均为纯内容语义，不含边框）——
	anchor      Pos         // 锚点（屏坐标）
	contentSize Size        // 纯内容尺寸
	title       string      // 嵌入上边框的标签
	frameColor  tcell.Style // 边框绘制颜色；零值 = config.DefStyle（见 ADR-7）
	display     func(contentArea Rect)
	handleEvent func(event tcell.Event)
	onResize    func()       // 仅 resize 自关时触发; Close() 清空防残留

	// —— FloatFrame 自己算 ——
	outerW, outerH int // 含边框总尺寸（contentSize + 2，title 撑宽取 max）
	fx, fy         int // 锚点展开后的最终左上角（屏坐标）
}

// TheFloatFrame 是全局单例容器（见 ADR-1），在 globals.go 的 InitGlobals 初始化，
// 永远非 nil，进程退出时销毁。
var TheFloatFrame *FloatFrame

// NewFloatFrame 返回一个空壳 FloatFrame。
func NewFloatFrame() *FloatFrame {
	return &FloatFrame{}
}

// Open 打开浮窗。
//
// 入参通过 FloatOpenSpec（options 模式）传入。layout 按 AutoExpand 分叉：
//   - AutoExpand=true  → 锚点自适应展开（SelectPane 贴光标弹窗）
//   - AutoExpand=false → 直接以 Anchor 为左上角（FileSelector 精确控制）
//
// 返回 bool：
//   - true  = 成功 open
//   - false = 没开成（已有浮窗在开 / 屏幕放不下），调用方应 onSelect(nil) 透明返回。
func (f *FloatFrame) Open(spec FloatOpenSpec) bool {
	if f.isOpen { return false } // C1 不变

	// —— 派生 outerW/outerH（不变）——
	outerW := spec.ContentSize.W + 2
	if titleW := len(spec.Title) + 6; titleW > outerW {
		outerW = titleW
	}
	outerH := spec.ContentSize.H + 2

	// —— 失败前置检查（不变, size-only 安全兜底）——
	w, h := screen.Screen.Size()
	iOffset := config.GetInfoBarOffset()
	bottomLimit := h - iOffset - 1 - 1
	if outerH > bottomLimit+1 || outerW > w {
		return false
	}

	// —— 存字段 ——
	ax, ay := spec.Anchor.X, spec.Anchor.Y
	if spec.AutoExpand && ay < 0 { // sentinel: 仅 SelectPane 用, 贴 statusLine
		ay = bottomLimit + ay + 1   // 直接读入参, 不抄到容器字段(避免抄写时机 bug)
	}
	fc := spec.FrameColor
	if fc == (tcell.Style{}) {
		fc = config.DefStyle
	}
	f.anchor = Pos{X: ax, Y: ay} // 变换后(spec.AutoExpand&&ay<0) / 原值(false)
	f.contentSize = spec.ContentSize
	f.title = spec.Title
	f.frameColor = fc
	f.display = spec.Display
	f.handleEvent = spec.HandleEvent
	f.onResize = spec.OnResize
	f.outerW, f.outerH = outerW, outerH

	// —— layout 分叉(F0a §3) ——
	if spec.AutoExpand {
		f.fx, f.fy = expandAnchor(ax, ay, outerW, outerH)
	} else {
		f.fx, f.fy = ax, ay // FileSelector: 直接采用, 不二次决策, 不 clamp(F0a §7: 调用者保证非负)
	}
	f.isOpen = true
	screen.Redraw()
	return true
}

// Close 关闭浮窗，回到空壳。回调顺序约定：调用方应先 Close() 再触发业务回调，
// 这样回调里读到的容器状态已是关闭后。
func (f *FloatFrame) Close() {
	if !f.isOpen {
		return
	}
	f.isOpen = false
	f.display = nil
	f.handleEvent = nil       // 断引用，便于 GC（设计 §五）
	f.onResize = nil          // 避免旧回调残留(F0a §4.4)
	screen.Redraw()
}

// IsOpen 返回是否打开。
func (f *FloatFrame) IsOpen() bool { return f.isOpen }

// Display 画浮窗：清屏 → 4 角 + 上下左右边 → title 嵌入上边框 → 委托画内容。
// 浮窗必须最后画（设计 §六：后写胜出 + 模态一致性）。
func (f *FloatFrame) Display() {
	if !f.isOpen {
		return
	}

	x, y := f.fx, f.fy
	w, h := f.outerW, f.outerH
	color := f.frameColor

	// 隐藏前序 pane（如 notePane / 主编辑器 BufWindow）残留的终端光标。
	// tcell 光标位置与 cell 内容是独立状态：FloatFrame 画 cell 不会覆盖光标，
	// 不主动 HideCursor 的话光标会浮在弹窗内闪（D15 实测 bug）。
	// 放在 Display 开头：若有需要光标的浮窗（如未来 inputPane），
	// 可在自己的 f.display 里调 ShowCursor 覆盖此 HideCursor。
	// 不需要在 close() 里恢复：DoEvent 每帧开头 micro.go:492 已无条件 HideCursor，
	// 浮窗关闭后下一帧由 notePane / 主编辑器各自的 ShowCursor 决定最终态。
	screen.Screen.HideCursor()

	// 1. Clear 整个矩形（边框 + 内容区）
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			screen.Screen.SetContent(x+col, y+row, ' ', nil, color)
		}
	}

	// 2. 4 角 + 上下边
	screen.Screen.SetContent(x, y, '┌', nil, color)
	screen.Screen.SetContent(x+w-1, y, '┐', nil, color)
	screen.Screen.SetContent(x, y+h-1, '└', nil, color)
	screen.Screen.SetContent(x+w-1, y+h-1, '┘', nil, color)
	for i := 1; i < w-1; i++ {
		screen.Screen.SetContent(x+i, y, '─', nil, color)
		screen.Screen.SetContent(x+i, y+h-1, '─', nil, color)
	}

	// 3. 左边 + 右边
	for row := 0; row < h-2; row++ {
		screenY := y + 1 + row
		screen.Screen.SetContent(x, screenY, '│', nil, color)
		screen.Screen.SetContent(x+w-1, screenY, '│', nil, color)
	}

	// 4. 上边框嵌入 "──<title>──...─"
	col := x + 1
	write := func(r rune) {
		if col < x+w-1 {
			screen.Screen.SetContent(col, y, r, nil, color)
			col++
		}
	}
	write('─') // ──
	write('─')
	for _, r := range f.title { // <title>
		write(r)
	}
	write('─') // ──
	write('─')
	for col < x+w-1 { // 余下填 ─
		write('─')
	}

	// 5. 委托画内容（contentArea = fx+1, fy+1, contentSize.W, contentSize.H）
	f.display(Rect{X: f.fx + 1, Y: f.fy + 1, W: f.contentSize.W, H: f.contentSize.H})
}

// HandleEvent 转发事件给具体浮窗。resize 由 FloatFrame 统一拦截（resize 即关 + onResize），
// 不再转发；其它事件照旧转发。
func (f *FloatFrame) HandleEvent(event tcell.Event) {
	if !f.isOpen {
		return
	}
	if _, ok := event.(*tcell.EventResize); ok {
		cb := f.onResize          // 先存(F0a §4.4 顺序)
		f.Close()                 // 关容器(内部已清 onResize)
		if cb != nil { cb() }     // 再触发业务取消回调
		return                    // 不再转发给具体浮窗
	}
	f.handleEvent(event)
}

// expandAnchor 算锚点自适应展开后的最终左上角 (fx, fy)。
//
// 仅在 AutoExpand=true 时调用（SelectPane 贴光标弹窗路径）。
// AutoExpand=false 时 FloatFrame 直接以 Anchor 为左上角，不调用本函数。
//
// 算法体从 selectpane.go 旧实现原样迁移，保持逐字节一致：
//   - 输入 ax, ay（锚点屏坐标）、outerW, outerH（含边框总尺寸）
//   - y 与 x 各自独立、各自对称（向 [右/下] 优先，向 [左/上] 次之，兜底偏向空间大的一侧）
//   - 边界 clamp 是防御性代码，正常路径靠 Open 的失败前置检查保证不触发
//
// 内部自行读取 screen.Screen.Size() 与 config.GetInfoBarOffset()，与旧实现一致。
func expandAnchor(ax, ay, outerW, outerH int) (fx, fy int) {
	w, h := screen.Screen.Size()
	iOffset := config.GetInfoBarOffset()
	statusLineY := h - iOffset - 1
	bottomLimit := statusLineY - 1 // statusLine 上方 1 row 即显示底边界

	// y 方向（§4.3）
	downSpace := bottomLimit - ay + 1 // [ay, bottomLimit] 的行数
	upSpace := ay + 1                 // [0, ay] 的行数
	switch {
	case downSpace >= outerH:
		fy = ay // 向下：锚点 = 顶边
	case upSpace >= outerH:
		fy = ay - outerH + 1 // 向上：锚点 = 底边
	case downSpace >= upSpace:
		fy = ay // 兜底：偏向下
	default:
		fy = ay - outerH + 1 // 兜底：偏向上
	}
	// 边界保护（防御性：Open 失败前置检查已判定 outerH ≤ bottomLimit+1）
	maxY := bottomLimit - outerH + 1
	if maxY < 0 {
		maxY = 0
	}
	if fy > maxY {
		fy = maxY
	}
	if fy < 0 {
		fy = 0
	}

	// x 方向（与 y 对称）
	rightSpace := w - ax // [ax, w-1] 的列数
	leftSpace := ax + 1  // [0, ax] 的列数
	switch {
	case rightSpace >= outerW:
		fx = ax // 向右：锚点 = 左边
	case leftSpace >= outerW:
		fx = ax - outerW + 1 // 向左：锚点 = 右边
	case rightSpace >= leftSpace:
		fx = ax // 兜底：偏向右
	default:
		fx = ax - outerW + 1 // 兜底：偏向左
	}
	// 边界保护（防御性：Open 失败前置检查已判定 outerW ≤ w）
	maxX := w - outerW
	if maxX < 0 {
		maxX = 0
	}
	if fx > maxX {
		fx = maxX
	}
	if fx < 0 {
		fx = 0
	}

	return fx, fy
}
