package action

import (
	"sort"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/micro/v2/internal/views"
	"github.com/micro-editor/tcell/v2"
)

// paneLimit 标记 stepPaneRatio 命中的边界状态。
type paneLimit int

const (
	limitNone paneLimit = iota // 正常档位
	limitMax                   // 已到放大极限（75% 或单 pane 全屏）
	limitMin                   // 已到缩小极限（25% 或单 pane）
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
	commands["tab"]   = Command{(*BufPane).neoNewTabCmd, buffer.FileComplete}
	commands["vsplit"] = Command{(*BufPane).neoVSplitCmd, buffer.FileComplete}
	commands["hsplit"] = Command{(*BufPane).neoHSplitCmd, buffer.FileComplete}

	// :big / :small pane 原语
	BufKeyActions["BigPane"]   = (*BufPane).BigPane
	BufKeyActions["SmallPane"] = (*BufPane).SmallPane
	commands["big"]   = Command{(*BufPane).BigCmd, nil}
	commands["small"] = Command{(*BufPane).SmallCmd, nil}
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

// ---- :big / :small ----

// BigPane 把活动 pane 提升为独立全屏 tab（promote），新 tab 成为焦点。
// 单 pane 时 no-op：当前 tab 已是「最大」状态，无需提升。
func (h *BufPane) BigPane() bool {
	tab := h.tab
	if len(tab.Panes) < 2 {
		InfoBar.Message("pane already at max")
		return false
	}
	return extractPaneToNewTab(tab, h, true)
}

// SmallPane 路由：按当前 pane 数和其他 tab 存在性分发到对应场景。
//   场景 1（≥2 pane）：把活动 pane demote 到新单 pane tab，原 tab 保持 active。
//   场景 2/3（1 pane + 有其他 tab）：从另一个 tab 吞一个 pane 进当前 tab（HSplit）。
//   场景 4（1 pane + 无其他 tab）：等价原生 HSplit，开一个空 pane 与当前 pane 配对。
func (h *BufPane) SmallPane() bool {
	tab := h.tab
	if len(tab.Panes) >= 2 {
		return extractPaneToNewTab(tab, h, false)
	}
	if otherPane, otherTab, found := pickAbsorbTarget(); found {
		return h.absorbPaneIntoTab(otherPane, otherTab)
	}
	// 场景 4：单 pane + 无其他 tab → 复用 :hsplit 包装（HSplit + OpenBirthSelector），
	// 不是裸 h.HSplitAction()：后者不开 birth selector、不置 isNoName，
	// 新 pane 退出时也走不到 quit selector（QuitNeo 靠 isNoName 分流）。
	return h.neoHSplitAction()
}

// BigCmd / SmallCmd 是 MakeCommand 签名的薄包装，与 QuitNeoCmd 同模式。
func (h *BufPane) BigCmd(args []string)   { h.BigPane() }
func (h *BufPane) SmallCmd(args []string) { h.SmallPane() }

// extractPaneToNewTab 把指定 pane 从原 tab 摘下，装进新建的全屏单 pane tab。
// activate=true → 新 tab 成为焦点（:big）；activate=false → 原 tab 保持焦点（:small）。
// 步骤：capture → reparent → 从原 tab 摘 → 入 TabList → Resize 补刀 → 切焦点。
// capture 必须在 reparent 之前：NewTabFromPane 调 SetTab/SetID 会改写 pane.id。
func extractPaneToNewTab(tab *Tab, h *BufPane, activate bool) bool {
	// step 1: capture
	idx := tab.GetPane(h.splitID)
	oldSplitID := h.splitID

	// step 2: reparent——新 tab 全屏 Layout，NewTabFromPane 调 SetTab/SetID 把 h 归属改到 newTab
	w, height := screen.Screen.Size()
	iOffset := config.GetInfoBarOffset()
	newTab := NewTabFromPane(0, 0, w, height-iOffset, h)

	// step 3: 先从原 tab 摘 h + Unsplit 旧叶子，再 AddTab
	// 顺序硬约束：AddTab 内部调 TabList.Resize 遍历所有 tab 调 Tab.Resize，
	// 若 h 还在原 tab.Panes（且 h.ID 已被 NewTabFromPane 改成 newTab root id），
	// 原 tab.GetNode(h.ID()) 返回 nil → Resize 里 n.X nil deref。
	// Unsplit 后原 tab root flatten 成剩余叶子的 id，剩余 pane.ID() 仍能 GetNode 命中。
	tab.RemovePane(idx)
	tab.GetNode(oldSplitID).Unsplit()
	tab.SetActive(0)

	// step 4: 入 TabList（内部已调 Resize；此时原 tab 已无 h，遍历安全）
	Tabs.AddTab(newTab)

	// step 5: Resize 补刀（幂等保险）
	newTab.Resize()
	tab.Resize()

	if activate {
		Tabs.SetActive(len(Tabs.List) - 1)
	}
	return true
}

// absorbPaneIntoTab 把 srcTab 里的 srcPane 移入当前 tab，HSplit 出下叶子挂 srcPane。
// 场景 2：src tab 单 pane，整 tab 删除（RemoveTab 在 RemovePane 之前：RemoveTab
// 靠 Panes[0].ID() 定位，若先 RemovePane 则 Panes 空，RemoveTab 的 len==0 continue 跳过）。
// 场景 3：src tab ≥2 pane，摘 pane 留 tab，拆分旧叶子，root flatten 成单叶。
// 不调 HSplitIndex（copy 语义），而是底层 SetTab/SetID 改归属，是 move 语义。
func (h *BufPane) absorbPaneIntoTab(srcPane *BufPane, srcTab *Tab) bool {
	// step 1: capture src
	srcIdx := srcTab.GetPane(srcPane.splitID)
	srcSplitID := srcPane.splitID

	// step 2: reparent——当前 tab split tree 建下叶子，srcPane 挂过去
	// HSplit(true) 把 root 叶子变 STVert：原 id 保留为上叶子(h)，返回下叶子 newNodeID
	newNodeID := h.tab.GetNode(h.splitID).HSplit(true)
	srcPane.SetTab(h.tab)
	srcPane.SetID(newNodeID)
	h.tab.AddPane(srcPane, h.tab.GetPane(h.splitID)+1)

	if len(srcTab.Panes) == 1 {
		// 场景 2：src tab 单 pane，整 tab 删除
		// 必须在 RemovePane 之前 RemoveTab：此时 Panes[0] 还是 srcPane，ID()==newNodeID 能匹配自己
		Tabs.RemoveTab(srcPane.ID())
	} else {
		// 场景 3：src tab ≥2 pane，摘 srcPane 留 tab
		srcTab.RemovePane(srcIdx)
		srcTab.GetNode(srcSplitID).Unsplit()
		srcTab.SetActive(0)
		srcTab.Resize()
	}

	// step 5: Resize 补刀
	h.tab.Resize()
	Tabs.Resize()
	return true
}

// pickAbsorbTarget 从非当前 tab 中选一个 pane 作为 absorb 源。
// 简化启发式：取 Tabs.List 末位非当前 tab 的 tab，再取其 Panes 末位 pane（最近打开）。
// HSplitIndex 总 append 到末尾，末位 pane = 最近打开，符合直觉。
// 未来可升级为 lastActive 排序（BufPane 加 lastActive 字段，SetActive(true) 时更新取 max）。
func pickAbsorbTarget() (*BufPane, *Tab, bool) {
	for i := len(Tabs.List) - 1; i >= 0; i-- {
		t := Tabs.List[i]
		if t == MainTab() {
			continue
		}
		if len(t.Panes) == 0 {
			continue
		}
		bp, ok := t.Panes[len(t.Panes)-1].(*BufPane)
		if !ok {
			continue
		}
		return bp, t, true
	}
	return nil, nil, false
}

// ---- spawn 包装：捕获父目录 → 调原生 spawn → 对新 pane 开 birth selector ----
// 覆盖 split/tab 的 3 个 key action 与 3 个 command（见 command_neo.go::InitNeoCommands）。
// 关键时序事实：三种 spawn 末尾都同步 SetActive+Resize，返回时新 pane = MainTab().CurPane()、
// 已有真实 Layout，故 OpenBirthSelector 可立即开（不在 Resize 里搞，Resize 保持纯净）。
// 带文件参数的 :vsplit foo / :tab foo 开 file-born pane（isNoNameBuf=false），OpenBirthSelector 直接 bail。

func (h *BufPane) neoAddTabAction() bool {
	dir := birthDir(h)
	r := h.AddTab()
	np := MainTab().CurPane()
	MainTab().Resize() // AddTab 不像 VSplitIndex 那样内部 Resize，这里补上让新 pane BWindow Layout 就绪
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
// Returns (targetSize, limit):
//   - single pane, grow:    (-1, limitMax)
//   - single pane, shrink:  (-1, limitMin)
//   - already at grow cap (75%): (-1, limitMax)
//   - already at shrink cap (25%): (-1, limitMin)
//   - normal step: (pixel value, limitNone)
func (h *BufPane) stepPaneRatio(grow bool) (int, paneLimit) {
	ratios := []float64{0.25, 0.5, 0.75}

	n := h.tab.GetNode(h.splitID)
	p := n.Parent()
	if p == nil {
		// 单 pane 占满整个 tab：grow 视为已到放大极限，shrink 视为已到缩小极限
		if grow {
			return -1, limitMax
		}
		return -1, limitMin
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
			return -1, limitMax
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
			return -1, limitMin
		}
	}

	// size 是传给 ResizeSplit 的「第一个子节点」尺寸：当前 pane 若是第一个直接用 target，
	// 若是第二个取 1-target（vResizeSplit/hResizeSplit 恒把 c1=children[0] 设成 size）。
	ratio := target
	if !isFirst {
		ratio = 1.0 - target
	}
	return int(ratio * float64(total)), limitNone
}

// GrowPane 放大当前 pane 到下一档位；到放大极限（limitMax）时溢出到 :big。
func (h *BufPane) GrowPane() bool {
	size, lim := h.stepPaneRatio(true)
	if lim == limitMax {
		return h.BigPane()
	}
	h.ResizePane(size)
	return true
}

// ShrinkPane 缩小当前 pane 到下一档位；到缩小极限（limitMin）时溢出到 :small。
func (h *BufPane) ShrinkPane() bool {
	size, lim := h.stepPaneRatio(false)
	if lim == limitMin {
		return h.SmallPane()
	}
	h.ResizePane(size)
	return true
}
