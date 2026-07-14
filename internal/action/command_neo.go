package action

import (
	"sort"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/micro/v2/internal/views"
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

	BufKeyActions["GrowPane"]   = (*BufPane).GrowPane
	BufKeyActions["ShrinkPane"] = (*BufPane).ShrinkPane
	BindKey("Alt-=", "GrowPane",   Binder["buffer"])
	BindKey("Alt--", "ShrinkPane", Binder["buffer"])
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
	if len(h.tab.Panes) >= 2 {
		InfoBar.Message("already 2 panes in this tab")
		return false
	}
	dir := birthDir(h)
	r := h.VSplitAction()
	OpenBirthSelector(MainTab().CurPane(), dir)
	return r
}
func (h *BufPane) neoHSplitAction() bool {
	if len(h.tab.Panes) >= 2 {
		InfoBar.Message("already 2 panes in this tab")
		return false
	}
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
	if len(h.tab.Panes) >= 2 {
		InfoBar.Message("already 2 panes in this tab")
		return
	}
	dir := birthDir(h)
	h.VSplitCmd(args)
	OpenBirthSelector(MainTab().CurPane(), dir)
}
func (h *BufPane) neoHSplitCmd(args []string) {
	if len(h.tab.Panes) >= 2 {
		InfoBar.Message("already 2 panes in this tab")
		return
	}
	dir := birthDir(h)
	h.HSplitCmd(args)
	OpenBirthSelector(MainTab().CurPane(), dir)
}

// stepPaneRatio moves the current pane to the next discrete ratio step.
// grow=true: increase pane share; grow=false: decrease pane share.
// Ratios are {0.25, 0.5, 0.75}, compared in pixel space (see stepPixel).
// Returns the new target pixel size, or -1 if already at boundary.
func (h *BufPane) stepPaneRatio(grow bool) int {
	ratios := []float64{0.25, 0.5, 0.75}

	n := h.tab.GetNode(h.splitID)
	p := n.Parent()
	if p == nil {
		InfoBar.Message("no other split")
		return -1
	}

	children := p.Children()
	isFirst := len(children) > 0 && children[0] == n

	var cur, total int
	if p.Kind == views.STVert {
		cur = n.H
		total = p.H
	} else {
		cur = n.W
		total = p.W
	}

	// 档位用「像素」比较，不用比例 cur/total：pane 尺寸是整数像素，按 resize 实际产生的
	// 截断方式算每个档位对应的像素，cur 就能精确命中某个档位，无需 epsilon 容差。resize 对
	// 第一个子节点取 int(ratio*total)（截断），对第二个是 total - int((1-ratio)*total)，
	// stepPixel 必须照搬这套——否则 cur 和档位像素会差 1，导致 grow/shrink 卡在原地。
	stepPixel := func(r float64) int {
		if isFirst {
			return int(r * float64(total))
		}
		return total - int((1-r)*float64(total))
	}

	var target float64
	found := false
	if grow {
		for _, s := range ratios {
			if stepPixel(s) > cur {
				target = s
				found = true
				break
			}
		}
		if !found {
			InfoBar.Message("pane already at max")
			return -1
		}
	} else {
		for i := len(ratios) - 1; i >= 0; i-- {
			if stepPixel(ratios[i]) < cur {
				target = ratios[i]
				found = true
				break
			}
		}
		if !found {
			InfoBar.Message("pane already at min")
			return -1
		}
	}

	// size 是传给 ResizeSplit 的「第一个子节点」尺寸：当前 pane 若是第一个直接用 target，
	// 若是第二个取 1-target（vResizeSplit/hResizeSplit 恒把 c1=children[0] 设成 size）。
	ratio := target
	if !isFirst {
		ratio = 1.0 - target
	}
	return int(ratio * float64(total))
}

// GrowPane expands the current pane to the next larger ratio step.
func (h *BufPane) GrowPane() bool {
	size := h.stepPaneRatio(true)
	if size < 0 {
		return false
	}
	h.ResizePane(size)
	return true
}

// ShrinkPane shrinks the current pane to the next smaller ratio step.
func (h *BufPane) ShrinkPane() bool {
	size := h.stepPaneRatio(false)
	if size < 0 {
		return false
	}
	h.ResizePane(size)
	return true
}
