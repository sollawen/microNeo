package finder

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/dialog"
	"github.com/micro-editor/tcell/v2"
)

// startRename 在光标行下方打开 InputDialog 预填当前条目名，供用户编辑改名。
// 光标在面包屑或越界时静默 no-op。目录名带 "/" 后缀传入，回调里再去掉。
func (fm *Session) startRename() {
	s := fm.state
	if s.cursor == 0 {
		return // 面包屑行：不响应 rename
	}
	idx := s.cursor - 1
	if idx < 0 || idx >= len(s.showEntries) {
		return // 越界防御
	}
	e := s.showEntries[idx]

	// initial：目录带 "/" 后缀（与 drawEntry 显示一致）
	initial := e.name
	if e.isDir {
		initial += string(filepath.Separator)
	}

	// anchor：当前行下方一行，X = finder 外框右 1 格（让出 finder 的左/右边框）
	anchorY := fm.rect.Y + 2 + s.cursor - s.topIdx
	anchor := dialog.Pos{
		X: fm.rect.X + 1,
		Y: anchorY,
	}

	oldName := e.name

	dlg := dialog.NewInputDialog()
	dlg.Open(
		initial,
		"Rename",
		anchor,
		fm.state.pickerW-9, // 内容区宽：比 finder 内容区窄 9 格，让出右边框
		config.DefStyle,
		func(result string, canceled bool) {
			if canceled {
				return
			}
			newName := strings.TrimSuffix(result, string(filepath.Separator))
			if newName == "" || newName == oldName {
				return // 空名或同名：no-op
			}
			oldPath := filepath.Join(s.currentDir, oldName)
			newPath := filepath.Join(s.currentDir, newName)
			if err := os.Rename(oldPath, newPath); err != nil {
				fm.showError("rename: " + err.Error())
				return
			}
			// 刷新：重读目录 + 光标落到新名字上 + 异步 git
			fm.chdirTo(s.currentDir, newName)
		},
	)
}

// showError 显示错误消息弹窗（rename 失败时调用）。
func (fm *Session) showError(msg string) {
	if msg == "" {
		msg = "Unknown error"
	}
	dlg := dialog.NewMsgDialog()
	// anchor：左对齐 finder 内容区，用 AlignCenter 居中文本
	anchorY := fm.rect.Y + (fm.rect.H / 2)
	anchor := dialog.Pos{
		X: fm.rect.X + 1, // 内容区左边界
		Y: anchorY,
	}
	dlg.Open(
		msg,
		"Error",
		anchor,
		50, // 合理宽度
		dialog.AlignCenter,
		0, // 不限行数
		tcell.StyleDefault,
		func() {}, // 关闭后无需特殊处理
	)
}
