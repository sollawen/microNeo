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
func (l *fileList) startRename() {
	if l.cursor == 0 {
		return
	}
	idx := l.cursor - 1
	if idx < 0 || idx >= len(l.showEntries) {
		return
	}
	e := l.showEntries[idx]

	initial := e.name
	if e.isDir {
		initial += string(filepath.Separator)
	}

	anchorY := l.fm.rect.Y + 2 + l.cursor - l.topIdx
	anchor := dialog.Pos{
		X: l.fm.rect.X,
		Y: anchorY,
	}

	oldName := e.name

	dlg := dialog.NewInputDialog()
	dlg.Open(
		initial,
		"Rename",
		anchor,
		l.pickerW-2,
		config.DefStyle,
		func(result string, canceled bool) {
			if canceled {
				return
			}
			newName := strings.TrimSuffix(result, string(filepath.Separator))
			if newName == "" || newName == oldName {
				return
			}
			oldPath := filepath.Join(l.currentDir, oldName)
			newPath := filepath.Join(l.currentDir, newName)
			if err := os.Rename(oldPath, newPath); err != nil {
				l.showError("rename: " + err.Error())
				return
			}
			l.chdirTo(l.currentDir, newName)
		},
	)
}

// showError 显示错误消息弹窗。
func (l *fileList) showError(msg string) {
	if msg == "" {
		msg = "Unknown error"
	}
	dlg := dialog.NewMsgDialog()
	anchorY := l.fm.rect.Y + (l.fm.rect.H / 2)
	anchor := dialog.Pos{
		X: l.fm.rect.X,
		Y: anchorY,
	}
	dlg.Open(
		msg,
		"Error",
		anchor,
		50,
		dialog.AlignCenter,
		0,
		tcell.StyleDefault,
		func() {},
	)
}