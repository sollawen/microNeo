package action

import (
	"sort"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

// InitNeoCommands 注册 microNeo 自定义命令。
// 在 cmd/micro/micro.go 的 action.InitCommands() 之后调用一次。
// 通过原生 MakeCommand 动态注册，不修改 commands map，零侵入。
func InitNeoCommands() {
	MakeCommand("theme", (*BufPane).ThemeCmd, nil)
	MakeCommand("file", (*BufPane).FileCmd, nil)
	MakeCommand("quit", (*BufPane).QuitNeoCmd, nil) // QuitNeoCmd 已存在，包 QuitNeo，不改

	// microNeo: spawn 包装覆盖（捕获父目录 + 开 birth selector）。
	// key action：BufKeyActions 在 BindKey 解析时被查一次并缓存函数指针，运行时按键用缓存、不再查 map。
	//   故 Ctrl-t→HSplit 改 map 后必须重新 BindKey 才生效；VSplit 仍走 :vsplit 命令，其 BufKeyActions 覆盖仅备用。
	// commands：InitCommands 整表重赋值之后覆盖即可（命令执行每次查最新）。
	BufKeyActions["AddTab"] = (*BufPane).neoAddTabAction
	BufKeyActions["VSplit"] = (*BufPane).neoVSplitAction
	BufKeyActions["HSplit"] = (*BufPane).neoHSplitAction
	BindKey("Ctrl-t", "HSplit", Binder["buffer"]) // 必须在 BufKeyActions["HSplit"] 赋值之后：BindKey 解析时查一次 map 并缓存函数指针，顺序错了会绑到原生 HSplitAction（无 birth selector）
	commands["tab"]   = Command{(*BufPane).neoNewTabCmd, buffer.FileComplete}
	commands["vsplit"] = Command{(*BufPane).neoVSplitCmd, buffer.FileComplete}
	commands["hsplit"] = Command{(*BufPane).neoHSplitCmd, buffer.FileComplete}
}

// InitNeoBindings 注册 microNeo 的键位覆盖。
// 必须在 action.InitBindings() 之后调用（micro.go:408）。
// Ctrl-q 始终绑 QuitNeo：运行时按 pane.isNoName 分流，file-born 等价原生 Quit。
func InitNeoBindings() {
	BufKeyActions["QuitNeo"] = (*BufPane).QuitNeo
	BindKey("Ctrl-q", "QuitNeo", Binder["buffer"])
	// F4 / F10 留原生（整体移除 F 键默认绑定属独立清理任务，非本任务）。
}

// ThemeCmd 是 :theme 命令的 action。
// 不带参数 → 弹 SelectPane 让用户选；选中后切换 colorscheme 并持久化。
func (h *BufPane) ThemeCmd(args []string) {
	_, items := colorschemeComplete("") // input="" → 返回全部；复用原生（见 3.4）
	sort.Strings(items)                 // 对齐原生展示：colorschemeComplete 自身不保证顺序，OptionValueComplete 也做了 sort
	if len(items) == 0 {
		InfoBar.Error("no colorscheme found")
		return
	}

	// anchor.Y = -1 是 FloatFrame sentinel：紧贴 statusLine 上方 1 行
	anchor := Pos{X: 0, Y: -1}

	NewSelectPane().Open(items, "Themes", anchor, tcell.Style{}, 8, false, func(picked *string) {
		if picked == nil {
			return // 用户按 Esc / resize，关闭即结束
		}
		// 选中 → 切换并持久化（writeToFile=true）
		// SetGlobalOption 内部会调 InitColorscheme + UpdateRules，但不显式 redraw；
		// 原生 set colorscheme 靠主循环自然渲染，这里显式补一次避免依赖时序假设。
		err := SetGlobalOption("colorscheme", *picked, true)
		if err != nil {
			InfoBar.Error(err)
		} else {
			InfoBar.Message("theme: ", *picked)
			screen.Redraw() // 只在切换成功后刷新；err 时 colorscheme 未变，不刷新
		}
	})
}

// QuitNeoCmd 是 :quit 命令的 action（F3 §4.4）。
// QuitNeo 本身是 BufKeyAction（无 args 参数），这里包一层满足 MakeCommand 的签名。
func (h *BufPane) QuitNeoCmd(args []string) {
	h.QuitNeo()
}

// QuitNeo 是 microNeo 的 Ctrl-q / :quit 路由（重写，替换 welcome_md.go 旧版）。
//   - file-born pane（isNoName=false）→ 直接 h.Quit()（原生自带存盘提示；最后→退程序）。
//   - noName-born pane → 开 quit selector，三出口各走原生检查、不在开 selector 前预检：
//       Enter 选文件 → OpenCmd（原生 :open，自带 modified 检查）；
//       Esc → 取消（回编辑，不丢数据、不检查）；
//       Ctrl-q（ReasonQuit）/ 打开时过小（ReasonSize）→ h.Quit()（原生自带 modified 检查）。
func (h *BufPane) QuitNeo() bool {
	if !h.isNoName {
		return h.Quit() // file-born：完全等价原生 Quit，零行为变化
	}
	h.openQuitSelector() // noName → 执行者
	return true
}

// FileCmd 是 :file 命令的 action（Ctrl-o 入口）。
// 打开文件选择器，选中后开进当前 pane。
// modified 检查推迟到 Enter 换入（OpenCmd 自带），不在开 selector 前预检——与 F4d 的
// QuitNeo/OpenBirthSelector 对称，行为与原生 :open 一致：Enter 选文件后由 OpenCmd 问
// 「保存? y/n/esc」，y=保存换入、n=丢弃修改换入、esc=取消换入。
func (h *BufPane) FileCmd(args []string) {
	h.openBrowseSelector()
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
