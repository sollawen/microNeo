package action

import (
	"os"
	"path/filepath"
	"runtime"

	isatty "github.com/mattn/go-isatty"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/screen"
)

// WelcomeMode 标记当前是否处于 welcome 会话（F3 §4.3 / §7.6）。
// 启动时（无文件名）→ true；屏幕太小装不下选择器时→ false（降级回原生）。
var WelcomeMode = false

// DetectWelcomeMode 根据命令行参数和终端类型判断是否进入 welcome 模式（F3 §4.2）。
// 必须在 action.InitBindings() 之后调用。
// - 有文件名参数 → false（正常打开文件）
// - 管道 / -clean 等特殊模式 → false
// - 无文件名 + 标准终端 → true
func DetectWelcomeMode(args []string) {
	if len(args) > 0 {
		WelcomeMode = false
		return
	}
	// flag.Args() 为空，说明没有文件名参数
	// 再看 stdin 是否为终端：管道输入时应走原生空 buffer（LoadInput 逻辑）
	WelcomeMode = isatty.IsTerminal(os.Stdin.Fd())
}

// InitNeoBindings 按 WelcomeMode 决定是否重绑 Ctrl-q / F4 / F10（F3 §4.4）。
// 必须在 action.InitBindings() 之后调用。
func InitNeoBindings() {
	// 先把 QuitNeo 注册到 BufKeyActions（惰性、无条件）
	BufKeyActions["QuitNeo"] = (*BufPane).QuitNeo

	if !WelcomeMode {
		return
	}

	// welcome 会话：Ctrl-q / F4 / F10 均重绑到 QuitNeo
	// 现状三者都绑 Quit，一并重绑保持一致（未来整体移除 F 键默认绑定，属独立清理任务）
	BindKey("Ctrl-q", "QuitNeo", Binder["buffer"])
	BindKey("F4", "QuitNeo", Binder["buffer"])
	BindKey("F10", "QuitNeo", Binder["buffer"])
}

// EnterWelcome 启动时进入 welcome 状态（F3 §4.2）。
// 必须在 action.InitTabs(b) 之后调用（先有 pane 才能开选择器）。
func EnterWelcome() {
	if !WelcomeMode {
		return
	}
	pane := MainTab().CurPane()
	// 启动时用 cwd（空 startDir → 内部 fallback）
	pane.openWelcomeSelector("")
}

// QuitNeo 是 welcome 会话下的退出逻辑（F3 §4.3 / §7.2）。
// - 非 welcome 会话：完全等价原生 Quit
// - welcome 会话 + 非最后 pane/tab：等价原生 Quit（关掉这个 split/tab）
// - welcome 会话 + 最后一个 pane/tab：回 welcome
func (h *BufPane) QuitNeo() bool {
	if !WelcomeMode {
		return h.Quit()
	}
	isLast := len(Tabs.List) == 1 && len(MainTab().Panes) == 1
	if !isLast {
		return h.Quit()
	}
	// welcome 会话 + 最后一个 pane → 回 welcome
	if h.Buf.Modified() && !h.Buf.Shared() {
		h.closePrompt("Close", h.gotoWelcome)
		return true
	}
	h.gotoWelcome()
	return true
}

// gotoWelcome 关闭当前 buffer 并重新进入 welcome 态（F3 §4.3）。
// 起始目录用「刚关文件所在目录」（用户大概率想开同目录的兄弟文件）。
func (h *BufPane) gotoWelcome() {
	dir := filepath.Dir(h.Buf.AbsPath)
	if h.Buf.AbsPath == "" {
		dir = ""
	}
	h.Buf.Close()
	h.OpenBuffer(buffer.NewBufferFromString("", "", buffer.BTDefault))
	h.openWelcomeSelector(dir)
}

// openWelcomeSelector 打开 welcome 态文件选择器（F3 §4.3 / §6 / §7.9）。
// 入口分两处（EnterWelcome 启动 / gotoWelcome 回 welcome），共用此函数。
//
// 降级预检（Graceful degradation）：
//   若 pane 装不下选择器（宽<20 或矮<10），computeLayout 返回 ok=false → 置 WelcomeMode=false、
//   不开选择器、return。此后 QuitNeo 走 WelcomeMode=false → 原生 Quit 退出，
//   避免「预检失败 → 重开 → 再失败」的死循环。
func (h *BufPane) openWelcomeSelector(startDir string) {
	fs := NewFileSelector()
	if _, _, ok := fs.computeLayout(h); !ok {
		WelcomeMode = false
		return
	}
	if startDir == "" {
		startDir, _ = os.Getwd()
	}
	fs.Open(h, startDir, func(r SelectResult) {
		switch r.Kind {
		case Picked:
			// 选中文件 → 开进 pane（W → E）
			b, err := buffer.NewBufferFromFile(r.Path, buffer.BTDefault)
			if err != nil {
				InfoBar.Error(err)
				return
			}
			h.OpenBuffer(b)
		case Closed:
			switch r.Reason {
			case ReasonQuit:
				// Ctrl-q → 退出程序
				exitProgram()
			case ReasonResize:
				// resize 后重开（同目录）
				h.openWelcomeSelector(startDir)
			}
			// ReasonEsc 不会到达：welcome 态 Esc 在 handleEvent 里是 no-op（见 fileselector.go §4.1c）
		}
	}, true) // isWelcome=true
}

// exitProgram 退出程序，精确对齐 actions.go::QuitAll 的 quit() 闭包（F3 §4.3）。
func exitProgram() {
	screen.Screen.Fini()
	InfoBar.Close()
	runtime.Goexit()
}
