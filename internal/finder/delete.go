package finder

import (
	"os"
	"path/filepath"

	"github.com/micro-editor/micro/v2/internal/dialog"
	"github.com/micro-editor/tcell/v2"
)

// startDelete 确认后删除光标所在文件或目录，并刷新当前目录。
func (l *fileList) startDelete() {
	if l.cursor == 0 {
		return
	}
	idx := l.cursor - 1
	if idx < 0 || idx >= len(l.showEntries) {
		return
	}

	e := l.showEntries[idx]
	name := e.name
	isDir := e.isDir
	dir := l.currentDir

	message := "Delete file <" + name + ">?"
	if isDir {
		message = "Delete folder [" + name + "] and all its contents?"
	}

	anchorY := l.fm.rect.Y + 2 + l.cursor - l.topIdx
	anchor := dialog.Pos{
		X: l.fm.rect.X,
		Y: anchorY,
	}

	dlg := dialog.NewConfirmDialog()
	if !dlg.Open(
		message,
		"Delete",
		anchor,
		l.pickerW-2,
		dialog.AlignCenter,
		0,
		tcell.Style{},
		dialog.KindYesNo,
		dialog.FocusOK,
		func(result dialog.ConfirmResult) {
			if result != dialog.ConfirmYes {
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
				l.showError("delete: " + err.Error())
				return
			}

			l.chdirTo(dir, "")
		},
	) {
		return
	}
}