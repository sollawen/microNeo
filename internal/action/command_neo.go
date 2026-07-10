package action

import (
	"os"
	"path/filepath"
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
	//   故 Ctrl-t→AddTab 改 map 后必须重新 BindKey 才生效；VSplit/HSplit 默认无快捷键（走 :vsplit/:hsplit 命令），BufKeyActions 覆盖仅备用。
	// commands：InitCommands 整表重赋值之后覆盖即可（命令执行每次查最新）。
	BufKeyActions["AddTab"] = (*BufPane).neoAddTabAction
	BindKey("Ctrl-t", "AddTab", Binder["buffer"]) // 重绑：让 Ctrl-t 重新解析到 neoAddTabAction（只改 map 不重绑则仍走原版）
	BufKeyActions["VSplit"] = (*BufPane).neoVSplitAction
	BufKeyActions["HSplit"] = (*BufPane).neoHSplitAction
	commands["tab"]   = Command{(*BufPane).neoNewTabCmd, buffer.FileComplete}
	commands["vsplit"] = Command{(*BufPane).neoVSplitCmd, buffer.FileComplete}
	commands["hsplit"] = Command{(*BufPane).neoHSplitCmd, buffer.FileComplete}
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

// FileCmd 是 :file 命令的 action（Ctrl-o 入口）。
// 打开文件选择器，选中后开进当前 pane。
// modified 检查推迟到 Enter 换入（OpenCmd 自带），不在开 selector 前预检——与 F4d 的
// QuitNeo/OpenBirthSelector 对称，行为与原生 :open 一致：Enter 选文件后由 OpenCmd 问
// 「保存? y/n/esc」，y=保存换入、n=丢弃修改换入、esc=取消换入。
func (h *BufPane) FileCmd(args []string) {
	h.openSelector()
}

// openSelector 打开文件选择器。
func (h *BufPane) openSelector() {
	// 2. 起始目录（F1 §8.1 / R6）
	startDir := filepath.Dir(h.Buf.AbsPath)
	if startDir == "." || h.Buf.AbsPath == "" {
		startDir, _ = os.Getwd()
	}

	// 3. 打开（onSelect 闭包：选中→开进发起 pane，与 :open 同路径）
	// 普通态回调只看 Picked，任何 Closed 当取消（F3 §3 / §4.4）
	NewFileSelector().Open(h, startDir, func(r SelectResult) {
		if r.Kind != Picked {
			return // Esc / resize / 拒开
		}
		if h.Buf == nil { // R7 防御：OpenCmd 访问 h.Buf，nil 会 panic
			return
		}
		h.OpenCmd([]string{r.Path}) // 复用原生 :open，自带 modified 检查
	}, false)
}
