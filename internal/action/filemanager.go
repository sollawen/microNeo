package action

import (
	"os"
	"path/filepath"

	"github.com/micro-editor/micro/v2/internal/buffer"
)

// isNoNameBuf 判定 buffer 是否「noName」：无路径 + 默认类型 + 空内容。
// OpenBirthSelector 用它作门控：true 才置 pane.isNoName=true 并开 birth selector；
// false（file-born / info / raw / note / 管道内容）直接 bail，不介入。
//   - NewBufferFromString("", "", BTDefault) → true
//   - 文件 buffer（AbsPath!=""）→ false
//   - 管道内容（Size>0）→ false（等价旧 isatty 效果，F4a §11）
//   - Help/Log/Raw/Scratch/Info（Type!=BTDefault）→ false
//   - Size()==0 精确语义：buffer.go:726 Size() 逐行字节求和、末行不加分隔符，
//     故 NewBufferFromString("","") 的单行空 buffer Size==0（满足）；用户敲过字
//     （含回车产生第二行）的空 buffer Size≥1（不满足→非 noName）。勿改成 len 判空。
func isNoNameBuf(buf *buffer.Buffer) bool {
	if buf == nil {
		return false
	}
	return buf.AbsPath == "" && buf.Type == buffer.BTDefault && buf.Size() == 0
}

// birthDir 返回 pane 当前 buffer 所在目录；空（noName / 无 buf）→ ""（调用方回退 cwd）。
func birthDir(h *BufPane) string {
	if h.Buf == nil {
		return ""
	}
	if p := h.Buf.AbsPath; p != "" {
		return filepath.Dir(p)
	}
	return ""
}

// OpenBirthSelector 对刚诞生的 pane 开 birth selector（若它是 noName）。
// 调用方：6 个 spawn 包装（包内）+ micro.go 启动段（跨包，故导出大写）。
//   - dir = 父 pane 目录（spawn 包装传入）或 ""（启动 → 回退 cwd）。
//   - noName pane → 置 isNoName=true（一次性、终身不变）+ 开 birth selector（isQuit=false）。
//   - file-born pane（如 :vsplit foo）→ 直接 return，isNoName 保持零值 false，不开。
//
// 为什么能在 spawn 之后立刻开：三种 spawn（VSplitIndex/HSplitIndex/AddTab 及对应 *Cmd）
// 末尾都同步调 SetActive + Resize，返回时新 pane 已是 active（MainTab().CurPane() 即它）、
// 且 BWindow 已有真实几何（computeLayout 预检不会 0×0 误判）。详见 F4a §6.1 时序验证。
func OpenBirthSelector(pane *BufPane, dir string) {
	if !isNoNameBuf(pane.Buf) {
		return
	}
	pane.isNoName = true
	if dir == "" {
		dir, _ = os.Getwd()
	}
	NewFileSelector().Open(pane, dir, func(r SelectResult) {
		if r.Kind != Picked {
			return // ReasonEsc → no-op，继续编辑当前（空）buffer
		}
		if pane.Buf == nil { // R7 防御：pane 在打开期间被关
			return
		}
		b, err := buffer.NewBufferFromFile(r.Path, buffer.BTDefault)
		if err != nil {
			InfoBar.Error(err)
			return
		}
		pane.OpenBuffer(b)
	}, false) // isQuit=false：birth 态 Esc 可关（→继续编辑空 buffer），Ctrl-q 不收
}

// QuitNeo 是 microNeo 的 Ctrl-q / :quit 路由（重写，替换 welcome_md.go 旧版）。
//   - file-born pane（isNoName=false）→ 直接 h.Quit()（原生自带存盘提示；最后→退程序）。
//   - noName-born pane → 开 quit selector（isQuit=true）：
//       Enter 选文件 → 原地换入；selector 关闭（Ctrl-q=ReasonQuit，或窗口过窄=ReasonResize）→ h.Quit()。
func (h *BufPane) QuitNeo() bool {
	if !h.isNoName {
		return h.Quit() // file-born：完全等价原生 Quit，零行为变化
	}
	proceed := func() {
		d := birthDir(h)
		if d == "" {
			d, _ = os.Getwd()
		}
		NewFileSelector().Open(h, d, func(r SelectResult) {
			if r.Kind == Picked {
				if h.Buf == nil { // R7 防御
					return
				}
				b, err := buffer.NewBufferFromFile(r.Path, buffer.BTDefault)
				if err != nil {
					InfoBar.Error(err)
					return
				}
				h.OpenBuffer(b)
				return
			}
			// Closed：selector 关闭即关 pane。ReasonQuit（Ctrl-q 收）与 ReasonResize
			// （窗口过窄 computeLayout 失败）都走 h.Quit——否则窄窗口下 noName pane 退不出去（死锁）。
			h.Quit() // → ForceQuit：非最后 pane 关本 pane；最后一个 runtime.Goexit 退程序
		}, true) // isQuit=true：不收 Esc，只收 Ctrl-q
	}
	if h.Buf.Modified() && !h.Buf.Shared() {
		h.closePrompt("Close", proceed) // y/n → proceed（开 selector）；esc → 取消留编辑
		return true
	}
	proceed()
	return true
}

// InitNeoBindings 注册 microNeo 的键位覆盖。
// 必须在 action.InitBindings() 之后调用（micro.go:408）。
// Ctrl-q 始终绑 QuitNeo：运行时按 pane.isNoName 分流，file-born 等价原生 Quit。
func InitNeoBindings() {
	BufKeyActions["QuitNeo"] = (*BufPane).QuitNeo
	BindKey("Ctrl-q", "QuitNeo", Binder["buffer"])
	// F4 / F10 留原生（整体移除 F 键默认绑定属独立清理任务，非本任务）。
}

// ---- spawn 包装：捕获父目录 → 调原生 spawn → 对新 pane 开 birth selector ----
// 覆盖 split/tab 的 3 个 key action 与 3 个 command（见 command_neo.go::InitNeoCommands）。
// 关键时序事实：三种 spawn 末尾都同步 SetActive+Resize，返回时新 pane = MainTab().CurPane()、
// 已有真实几何，故 OpenBirthSelector 可立即开（不在 Resize 里搞，Resize 保持纯净）。
// 带文件参数的 :vsplit foo / :tab foo 开 file-born pane（isNoNameBuf=false），OpenBirthSelector 直接 bail。

func (h *BufPane) neoAddTabAction() bool {
	dir := birthDir(h)
	r := h.AddTab()
	np := MainTab().CurPane()
	MainTab().Resize() // AddTab 不像 VSplitIndex 那样内部 Resize，这里补上让新 pane BWindow 几何就绪
	OpenBirthSelector(np, dir)
	return r
}
func (h *BufPane) neoVSplitAction() bool {
	dir := birthDir(h)
	r := h.VSplitAction()
	OpenBirthSelector(MainTab().CurPane(), dir)
	return r
}
func (h *BufPane) neoHSplitAction() bool {
	dir := birthDir(h)
	r := h.HSplitAction()
	OpenBirthSelector(MainTab().CurPane(), dir)
	return r
}

func (h *BufPane) neoNewTabCmd(args []string) {
	dir := birthDir(h)
	h.NewTabCmd(args)
	np := MainTab().CurPane()
	MainTab().Resize() // NewTabCmd 同 AddTab 不内部 Resize，补上
	OpenBirthSelector(np, dir)
}
func (h *BufPane) neoVSplitCmd(args []string) {
	dir := birthDir(h)
	h.VSplitCmd(args)
	OpenBirthSelector(MainTab().CurPane(), dir)
}
func (h *BufPane) neoHSplitCmd(args []string) {
	dir := birthDir(h)
	h.HSplitCmd(args)
	OpenBirthSelector(MainTab().CurPane(), dir)
}
