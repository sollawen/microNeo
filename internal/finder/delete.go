package finder

import (
	"os"
	"path/filepath"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/dialog"
)

// startDelete 确认后删除光标所在文件或目录，并刷新当前目录。
// 光标在面包屑或越界时静默 no-op。
func (fm *Session) startDelete() {
	s := fm.state
	if s.cursor == 0 {
		return // 面包屑行：不响应
	}
	idx := s.cursor - 1
	if idx < 0 || idx >= len(s.showEntries) {
		return // 越界防御
	}

	e := s.showEntries[idx]
	name := e.name
	isDir := e.isDir
	dir := s.currentDir

	// 确认文案：单行，尖括号/方括号均为实际显示字符
	message := "Delete file <" + name + ">?"
	if isDir {
		message = "Delete folder [" + name + "] and all its contents?"
	}

	// anchor：当前行下方一行，左对齐 finder 外框
	anchorY := fm.rect.Y + 2 + s.cursor - s.topIdx
	anchor := dialog.Pos{
		X: fm.rect.X,
		Y: anchorY,
	}

	dlg := dialog.NewConfirmDialog()
	dlg.Open(
		message,
		"Delete",
		anchor,
		fm.state.pickerW,
		dialog.AlignCenter,
		0,
		config.DefStyle,
		func(confirmed bool) {
			if !confirmed {
				return
			}

			path := filepath.Join(dir, name)
			var err error
			if isDir {
				err = os.RemoveAll(path)
			} else {
				err = os.Remove(path)
			}
			if err != nil {
				fm.showError("delete: " + err.Error())
				return
			}

			// 成功：刷新目录并重查 git 状态
			fm.chdirTo(dir, "")
		},
	)
}
