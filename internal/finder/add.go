package finder

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/dialog"
)

// startAdd 打开 InputDialog 让用户输入新名字。
func (l *fileList) startAdd() {
	anchor := dialog.Pos{
		X: l.fm.rect.X,
		Y: l.fm.rect.Y + 2 + l.cursor - l.topIdx,
	}

	dlg := dialog.NewInputDialog()
	dlg.Open(
		"",
		"Add",
		anchor,
		l.pickerW-2,
		config.DefStyle,
		func(result string, canceled bool) {
			if canceled {
				return
			}
			isDir := strings.HasSuffix(result, string(filepath.Separator))
			name := strings.TrimSuffix(result, string(filepath.Separator))
			if name == "" {
				return
			}
			if strings.Contains(name, string(filepath.Separator)) {
				l.showError("Name must be a plain file or directory name (no path separators)")
				return
			}

			dir := l.currentDir
			full := filepath.Join(dir, name)

			if _, err := os.Stat(full); err == nil {
				l.showError(name + " already exists")
				return
			}

			if isDir {
				if err := os.Mkdir(full, 0755); err != nil {
					l.showError("mkdir: " + err.Error())
					return
				}
				l.chdirTo(dir, name)
				return
			}

			f, err := os.Create(full)
			if err != nil {
				l.showError("create: " + err.Error())
				return
			}
			f.Close()
			l.fm.closePicked(name)
		},
	)
}