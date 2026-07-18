# F5 · FileSelector Delete 实现方案

**前置**：F4 Rename 已完成；ConfirmDialog（`internal/dialog/confirm.go`）已实现。
**范围**：在 finder 中按 `d` 删除光标所在文件或目录。删除目录时递归删除其中全部内容；不含批量删除、回收站和撤销。

---

## 1. 目标

用户在 finder 中将光标停在文件或目录上，按 `d` 后弹出 ConfirmDialog。用户确认 OK 后删除目标并刷新当前目录；选择 Cancel、按 Esc 或发生 resize 时不做任何操作。光标在面包屑上时不响应。

---

## 2. 复用现有实现

Delete 直接照 `internal/finder/rename.go` 的结构实现，不增加新抽象：

| 能力 | 复用位置 |
|---|---|
| 获取光标条目 | `startRename()` 的 `cursor` / `showEntries` 检查 |
| 弹窗位置 | `startRename()` 的当前行下方 anchor 计算 |
| 确认弹窗 | `dialog.NewConfirmDialog()` |
| 错误展示 | 已有 `fm.showError()` |
| 刷新目录 | 已有 `fm.chdirTo()` |
| 模态事件路由 | `FloatFrame` 已接入主循环，无需修改 |

---

## 3. 交互流程

```text
用户在条目上按 d
  → fm.startDelete()
  → 当前行下方打开 ConfirmDialog

用户选择 Cancel / 按 Esc / resize
  → onResult(false)
  → 不删除、不刷新

用户选择 OK
  → onResult(true)
  → 文件：os.Remove
  → 目录：os.RemoveAll
  → 成功：chdirTo 当前目录完成 refresh
  → 失败：showError 显示错误，列表保持不变
```

确认文案：

```text
Delete <name>?
This cannot be undone.
```

目录名显示 `/` 后缀，与 finder 列表保持一致。Dialog title 使用 `Delete`。

---

## 4. 实现步骤

### 4.1 接入 `d` 键

**文件**：`internal/finder/session.go`

在 `handleKey` 的 KeyRune switch 中加入：

```go
case 'd':
	fm.startDelete()
```

### 4.2 实现删除流程

**新文件**：`internal/finder/delete.go`

```go
package finder

import (
	"os"
	"path/filepath"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/dialog"
)

// startDelete 确认后删除光标所在文件或目录，并刷新当前目录。
func (fm *Session) startDelete() {
	s := fm.state
	if s.cursor == 0 {
		return
	}
	idx := s.cursor - 1
	if idx < 0 || idx >= len(s.showEntries) {
		return
	}

	e := s.showEntries[idx]
	name := e.name
	isDir := e.isDir
	dir := s.currentDir

	displayName := name
	if isDir {
		displayName += string(filepath.Separator)
	}

	anchor := dialog.Pos{
		X: fm.rect.X,
		Y: fm.rect.Y + 2 + s.cursor - s.topIdx,
	}

	dlg := dialog.NewConfirmDialog()
	dlg.Open(
		"Delete "+displayName+"?\nThis cannot be undone.",
		"Delete",
		anchor,
		s.pickerW,
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

			fm.chdirTo(dir, "")
		},
	)
}
```

文件使用 `os.Remove`；目录使用 `os.RemoveAll`，因此非空目录也会被完整删除。符号链接被视为文件，只删除链接本身，不跟随链接删除目标目录。

### 4.3 编译

按项目约定执行：

```bash
make build
```

---

## 5. 文件变更

| 文件 | 改动 |
|---|---|
| `internal/finder/session.go` | KeyRune switch 增加 `case 'd'` |
| `internal/finder/delete.go` | 新增 `startDelete()` |

无需修改：

- `internal/finder/rename.go`：直接复用其中已有的 `showError()`。
- `internal/dialog/confirm.go`：ConfirmDialog API 已满足需求。
- `cmd/micro/micro.go`：FloatFrame 的渲染与模态事件路由已完成。

---

## 6. 边界行为

| 场景 | 行为 |
|---|---|
| 光标在面包屑 | 静默 no-op |
| 光标越界或目录为空 | 静默 no-op |
| Cancel / Esc | 不删除、不刷新 |
| resize | ConfirmDialog 返回 false，不删除 |
| 删除普通文件 | `os.Remove` |
| 删除空目录 | `os.RemoveAll` |
| 删除非空目录 | 递归删除目录及全部内容 |
| 删除符号链接 | 只删除链接本身 |
| 权限不足、只读文件系统等 | 打开 MsgDialog 显示错误，不刷新 |
| 删除成功 | 调用 `chdirTo(dir, "")` 重读目录并重新查询 git 状态 |

---

## 7. 手测清单

1. 光标停在普通文件上按 `d`，ConfirmDialog 显示正确文件名。
2. 选择 Cancel、按 Esc，文件仍存在。
3. 选择 OK，文件被删除，finder 立即刷新。
4. 删除空目录成功。
5. 删除含文件和子目录的非空目录成功。
6. 光标在面包屑上按 `d` 无反应。
7. 删除无权限目标时显示 Error MsgDialog，列表不变。
8. ConfirmDialog 打开期间 resize，不删除目标且 finder 正常关闭。
9. 删除 git 仓库中的文件后，刷新后的 git 标志正确更新。
