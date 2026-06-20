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
// 入参 anchor / contentSize 为纯内容语义（不含边框），title 撑宽由容器派生 outerW。
// display / handleEvent 是具体浮窗把"画自己"与"处理事件"打包成函数值塞进来（ADR-2）。
//
// 返回 bool（D-1 决策，语义扩展自设计 §四.1）：
//   - true  = 成功 open
//   - false = 没开成（调用方不区分原因）：FloatFrame 已有浮窗在打开（C1 拒绝再开）；
//     或屏幕放不下（失败前置检查，几何归容器）。
//
// SelectPane 等调用方看到 false 时直接 onSelect(nil) 返回，对调用方完全透明。
func (f *FloatFrame) Open(
	anchor Pos,
	contentSize Size,
	title string,
	frameColor tcell.Style,
	display func(Rect),
	handleEvent func(tcell.Event),
) bool {
	// C1：已有浮窗打开，拒绝再开
	if f.isOpen {
		return false
	}

	// 由内容尺寸派生含边框外尺寸：宽度可能被 title 撑宽（与 selectpane.go 旧实现一致）
	//   内容区：contentSize.W + 2（左右边框）
	//   title 区：len(title) + 6（边框2 + 前后 ── 各 2）
	outerW := contentSize.W + 2
	if titleW := len(title) + 6; titleW > outerW {
		outerW = titleW
	}
	outerH := contentSize.H + 2

	// 失败前置检查（迁移自 selectpane.go §4.6）：屏幕完全放不下则放弃 open。
	// 不修改任何状态（isOpen 保持 false），不 set fx/fy，不 Redraw。
	w, h := screen.Screen.Size()
	iOffset := config.GetInfoBarOffset()
	statusLineY := h - iOffset - 1
	bottomLimit := statusLineY - 1 // statusLine 上方 1 row 即显示底边界
	if outerH > bottomLimit+1 || outerW > w {
		return false
	}

	// 通过几何检查，准备打开
	f.anchor = anchor
	f.contentSize = contentSize
	f.title = title
	if frameColor == (tcell.Style{}) {
		frameColor = config.DefStyle
	}
	f.frameColor = frameColor
	f.display = display
	f.handleEvent = handleEvent

	f.outerW = outerW
	f.outerH = outerH
	f.fx, f.fy = expandAnchor(anchor.X, anchor.Y, outerW, outerH)
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
	f.handleEvent = nil // 断引用，便于 GC（设计 §五）
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

// HandleEvent 转发事件给具体浮窗。所有事件（含 resize）一律转发，
// 由具体浮窗自己决定如何处理（设计 §九 ADR-9：resize 归具体浮窗，SelectPane 视为取消）。
func (f *FloatFrame) HandleEvent(event tcell.Event) {
	if !f.isOpen {
		return
	}
	f.handleEvent(event)
}

// expandAnchor 算锚点自适应展开后的最终左上角 (fx, fy)。
//
// 算法体从 selectpane.go 旧实现原样迁移（设计 §七 / 实施计划 T1.2），保持逐字节一致：
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
