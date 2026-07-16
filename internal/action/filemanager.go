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

// startDirOf 返回 pane 的起始目录：有 AbsPath 取父目录，否则 cwd。
// 取代旧 openSelector 的内联计算与 QuitNeo.proceed 里的 birthDir+cwd 回退。
func startDirOf(h *BufPane) string {
	if h.Buf != nil && h.Buf.AbsPath != "" {
		return filepath.Dir(h.Buf.AbsPath)
	}
	d, _ := os.Getwd()
	return d
}

// OpenBirthSelector 对刚诞生的 pane 开 birth selector（若它是 noName）。
// 调用方：6 个 spawn 包装（包内）+ micro.go 启动段（跨包，故导出大写）。
//   - dir = 父 pane 目录（spawn 包装传入）或 ""（启动 → 回退 cwd）。
//   - noName pane → 置 isNoName=true（一次性、终身不变）+ 开 birth selector。
//   - file-born pane（如 :vsplit foo）→ 直接 return，isNoName 保持零值 false，不开。
//
// 为什么能在 spawn 之后立刻开：三种 spawn（VSplitIndex/HSplitIndex/AddTab 及对应 *Cmd）
// 末尾都同步调 SetActive + Resize，返回时新 pane 已是 active（MainTab().CurPane() 即它）、
// 且 BWindow 已有真实 Layout（computeLayout 预检不会 0×0 误判）。详见 F4a §6.1 时序验证。
func OpenBirthSelector(pane *BufPane, dir string) {
	if !isNoNameBuf(pane.Buf) {
		return
	}
	pane.isNoName = true
	if dir == "" {
		dir, _ = os.Getwd()
	}
	NewFileSelector().Open(pane, dir, func(r SelectResult) {
		if r.Kind == Picked {
			if pane.Buf == nil { // R7 防御：OpenCmd 访问 h.Buf，nil 会 panic
				return
			}
			pane.OpenCmd([]string{r.Path}) // 复用原生 :open，自带 modified 检查
			return
		}
		if r.Reason == ReasonQuit {
			pane.Quit() // selector 内 Ctrl-q → 退出该空 pane（F5 §5.3）
			return
		}
		// ReasonEsc / ReasonSize / ReasonResize → no-op，继续编辑空 buffer
	})
}

// openBrowseSelector：Ctrl-o / :file 执行者。目录取自当前 pane。
func (h *BufPane) openBrowseSelector() {
	NewFileSelector().Open(h, startDirOf(h), func(r SelectResult) {
		if r.Kind == Picked {
			if h.Buf == nil { // R7 防御：OpenCmd 访问 h.Buf，nil 会 panic
				return
			}
			h.OpenCmd([]string{r.Path})
			return
		}
		if r.Reason == ReasonQuit {
			h.Quit()
			return
		}
		// ReasonEsc / ReasonSize / ReasonResize → no-op
	})
}

// openQuitSelector：Ctrl-q（noName pane）执行者。从 QuitNeo.proceed 抽出。
// 与上两者唯一差异：ReasonSize 也 h.Quit()（窄窗口破死锁，F5 场景 1）。
func (h *BufPane) openQuitSelector() {
	NewFileSelector().Open(h, startDirOf(h), func(r SelectResult) {
		if r.Kind == Picked {
			if h.Buf == nil { // R7 防御：OpenCmd 访问 h.Buf，nil 会 panic
				return
			}
			h.OpenCmd([]string{r.Path})
			return
		}
		if r.Reason == ReasonQuit || r.Reason == ReasonSize {
			h.Quit()
			return
		}
		// ReasonEsc / ReasonResize → no-op（取消退出）
	})
}
