package finder

import "github.com/micro-editor/tcell/v2"

// NoKeyboardRegion 是 finder 内一个自管理区域的最小契约。
//
// 构造时 session 塞入绝对屏坐标 Rect，region 整生命周期持有它。
// session 在每帧 Display 链上调用 Display() 画自己的内容（不含框线）。
// session 按 hit-test 把 mouse 事件交给 HandleMouse；返回 false 表示
// 「这个具体事件我不处理」。
//
// 接键盘事件的 region（fileList、Step 2 加入的 history）还应实现 KeyboardRegion；
// preview 这类不接键盘的 region 只需满足 NoKeyboardRegion。focus 管理、HandleKey 全部归属
// KeyboardRegion，preview 在结构上不参与 focus 路由。
type NoKeyboardRegion interface {
	Display()
	HandleMouse(ev *tcell.EventMouse) bool
	Rect() Rect // 构造时 session 把 rect 传给 region，整个生命周期内不变；session 通过它做 hit-test。
}

// KeyboardRegion 是 NoKeyboardRegion 之上加键盘相关方法的扩展接口。
//
// session 把键盘事件只送给当前 focus 的 KeyboardRegion（在 keyboardRegions[] 索引空间里）；
// FocusOn/FocusLost 是 session 切换 focus 时通知 region 的钩子。
// Step 1 全 no-op（无第二 key region、无视觉差异可表达），Step 2 起加视觉实现。
type KeyboardRegion interface {
	NoKeyboardRegion
	HandleKey(ev *tcell.EventKey) bool
	FocusOn()
	FocusLost()
}