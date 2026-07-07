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

// FileCmd 是 :file 命令的 action。
// 打开文件选择器，选中后开进当前 pane。
// modified 时弹 y/n 提示（n=取消，不同于 :open 的 n=不保存继续）。
func (h *BufPane) FileCmd(args []string) {
	// 1. modified 保护（F0 §8 / F1 §8.1 / R5）：n=取消，不同于 :open 的 n=不保存继续
	if h.Buf.Modified() && !h.Buf.Shared() {
		InfoBar.YNPrompt("Buffer modified. Save? (y,n,esc)", func(yes, canceled bool) {
			if canceled || !yes {
				return // n / Esc → 取消（不继续）
			}
			// y → 先保存，成功才继续
			if h.SaveAll() {
				h.openSelector()
			}
		})
		return // YNPrompt 异步：立即返回，等回调
	}
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
	NewFileSelector().Open(h, startDir, func(picked *string) {
		if picked == nil {
			return // Esc / resize / 拒开
		}
		// pane 在打开期间被关的防御（R7）
		if h.Buf == nil {
			return
		}
		b, err := buffer.NewBufferFromFile(*picked, buffer.BTDefault)
		if err != nil {
			InfoBar.Error(err)
			return
		}
		h.OpenBuffer(b)
	})
}
