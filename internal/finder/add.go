package finder

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/dialog"
)

// startAdd 打开 InputDialog 让用户输入新名字：以 "/" 结尾建子目录，否则建空文件。
// 重名用 MsgDialog 提示；建文件成功后关闭 finder 并交给 owner 打开编辑。
// 与 rename/delete 不同，光标在面包屑或空目录也响应（新建不依赖现有条目）。
func (fm *Session) startAdd() {
	s := fm.state

	// anchor：当前行下方一行，左对齐 finder 内容区（与 rename/delete 一致）
	anchor := dialog.Pos{
		X: fm.rect.X,
		Y: fm.rect.Y + 2 + s.cursor - s.topIdx,
	}

	dlg := dialog.NewInputDialog()
	dlg.Open(
		"", // 新建：不预填
		"Add",
		anchor,
		fm.state.pickerW-2, // 内容区宽：与 rename InputDialog 一致
		config.DefStyle,
		func(result string, canceled bool) {
			if canceled {
				return
			}
			isDir := strings.HasSuffix(result, string(filepath.Separator))
			name := strings.TrimSuffix(result, string(filepath.Separator))
			if name == "" {
				return // 空名 no-op
			}

			// 拒绝含路径分隔符的输入（尾部 "\" 已在上面去掉）
			if strings.Contains(name, string(filepath.Separator)) {
				fm.showError("Name must be a plain file or directory name (no path separators)")
				return
			}

			dir := fm.state.currentDir
			full := filepath.Join(dir, name)

			// 重名：直接问 FS，覆盖 hidden 名字
			if _, err := os.Stat(full); err == nil {
				fm.showError(name + " already exists")
				return
			}

			if isDir {
				if err := os.Mkdir(full, 0755); err != nil {
					fm.showError("mkdir: " + err.Error())
					return
				}
				fm.chdirTo(dir, name) // 刷新 + 光标落到新目录
				return
			}

			// 文件：建空文件后立即关，交给 owner 打开
			f, err := os.Create(full)
			if err != nil {
				fm.showError("create: " + err.Error())
				return
			}
			f.Close()
			fm.closePicked(name) // 绕过光标定位，直接带新名字触发 Picked
		},
	)
}
