# F9 · FileSelector Add 实现方案

**前置**：F4 Rename、F5 Delete 已完成；InputDialog / MsgDialog（`internal/dialog`）已实现并接线。
**范围**：在 finder 中按 `a` 新建一个文件或子目录。新建子目录只刷新列表；新建文件在关闭 finder 后自动交给 owner 打开编辑。不含批量创建、模板文件、跨目录路径（输入含 `/` 的中间段不特殊处理）。

---

## 1. 目标

用户在 finder 中按 `a`，当前行下方弹出 InputDialog（不预填）。用户输入一个名字：

- 以 `/` 结尾 → 在当前目录下新建一个**子目录**（空）。
- 否则 → 在当前目录下新建一个**空文本文件**。

名字若已存在，弹 MsgDialog 提示，不开建。新建子目录成功后刷新列表并把光标落到新目录上；新建文件成功后关闭 finder，由 owner（`onFinderClose`）用 `OpenCmd` 打开它进入编辑。

与 rename/delete 不同，**add 不依赖光标所在条目**：光标在面包屑上（`cursor==0`）、甚至空目录里也能 `a` 新建——新建是独立动作，不读取 / 改写现有条目。

---

## 2. 复用现有实现

照 `internal/finder/rename.go` / `delete.go` 的同构结构，不引入新抽象：

| 能力 | 复用位置 |
|---|---|
| 弹输入框 | `dialog.NewInputDialog()`（`rename.go` 已用同款） |
| anchor 计算 | `rename.go` 的 `fm.rect.X` / `fm.rect.Y + 2 + cursor - topIdx` |
| 错误 / 提示弹窗 | 已有 `fm.showError(msg)`（`rename.go`） |
| 刷新 + 定位（目录场景） | 已有 `fm.chdirTo(dir, focusName)` |
| 模态事件路由 | `FloatFrame` 已接主循环，按 `a` → InputDialog 与 rename 路径完全一致 |
| 打开文件（文件场景） | owner 侧 `onFinderClose` 的 `Picked` 分支 → `OpenCmd` |

**唯一的新点**：新建文件后要让 finder「带指定文件名」触发 `Picked` 关闭。现有 `close(Picked)` 走 `pickedName()`，是按**光标位置**取文件名，add 不能依赖它（见 §4.1）。需要给 Session 补一个小切口。

---

## 3. 交互流程

```text
用户按 a（finder 开、InputDialog 未开）
  → handleKey 的 KeyRune 'a' → fm.startAdd()
  → InputDialog 在当前行下方弹出（initial="", title="Add"）

用户编辑（InputDialog 开）
  → TheFloatFrame 路由事件给 InputDialog，finder 被旁路（与 rename 同）

用户按 Esc / resize
  → onResult("", canceled=true) → 回调直接 return，不开建、不刷新

用户按 Enter
  → onResult(result, canceled=false)
  → 判定 isDir = result 以 "/" 结尾；name = 去掉尾部 "/"
  → name 为空 → no-op
  → os.Stat(name) 命中 → showError(name + " already exists")，列表不变
  → isDir：os.Mkdir → 成功 chdirTo(dir, name) 刷新 + 定位光标到新目录
            → 失败 showError("mkdir: ...")
  → 文件：os.Create → 成功 fm.closePicked(name) 关闭 finder、交给 owner 打开
            → 失败 showError("create: ...")
```

文件场景下，`closePicked` 触发 `onFinderClose(Picked)` → `OpenCmd(新文件绝对路径)` → 进入编辑。

---

## 4. 设计决策

### 4.1 自动打开文件：新增 `closePicked(name)`，不复用 `close(Picked)`

**问题**：`close(Picked)` 内部用 `pickedName()` 取光标条目的文件名。要让 add 复用它，得先把光标定位到新文件上（`chdirTo(dir, name)`），但 `chdirTo` 的 `locate` 在新名字被 `showHidden=false` 过滤时（用户建了个 `.foo`）会落首条目——此时 `pickedName()` 返回的是别的文件，`onFinderClose` 会打开错误文件。

**决策**：给 Session 加一个私有方法 `closePicked(name string)`，直接用参数指定的文件名构造 `Result{Reason: Picked, ...}` 并走统一关闭流程（填 Cwd/IsQuit → reset → 回调）。add 建文件成功后调它，**绕过光标定位**。

**实现方式（小重构）**：把 `close` 的「构造 Result + reset + 回调」主体抽到 `finishClose(r Result)`，`close` 和 `closePicked` 都委托给它。这是纯内部拆分，`close` 现有 5 个调用点（Resize/Esc/Quit/Picked 等）零改动，且消除了即将出现的重复。

```go
// close 统一关闭路径：5 条路径只换 Reason。Picked 时按光标取 File。
func (fm *Session) close(reason CloseReason) {
	r := Result{Reason: reason, Cwd: fm.state.currentDir, IsQuit: fm.isQuit}
	if reason == Picked {
		r.File = fm.pickedName()
	}
	fm.finishClose(r)
}

// closePicked 以指定文件名触发 Picked 关闭（add 新建文件后用，绕过光标定位）。
func (fm *Session) closePicked(name string) {
	fm.finishClose(Result{
		Reason: Picked, Cwd: fm.state.currentDir, File: name, IsQuit: fm.isQuit,
	})
}

// finishClose 构造好 Result 后的统一收尾：reset 会话、回调 owner。
// 顺序必须与现有 close 一致：先保存 cb 引用、reset、最后调 cb（reset 会把 onClose 置 nil）。
func (fm *Session) finishClose(r Result) {
	if !fm.isOpen {
		return
	}
	cb := fm.onClose
	fm.reset()
	if cb != nil {
		cb(r)
	}
}
```

**已否决**：直接在 add 回调里手动构造 `Result` 并调 `fm.onClose`——绕过 `reset`，会留下 `isOpen=true` 的脏状态，且 `onClose` 是私有字段、语义上不应跨方法直接碰。

### 4.2 重名检查：`os.Stat` 直接问文件系统

**决策**：`os.Stat(full)` 命中即视为重名，弹 MsgDialog 提示 `<name> already exists`。

**理由**：`allEntries` 虽含 hidden、内存里就能查，但 `os.Stat` 语义最直白、不依赖 finder 内部视图状态；单用户编辑器场景无并发，TOCTOU 窗口可忽略。检查在前、建文件在后，错误信息可定制（系统自带的 `file exists` 不友好）。

### 4.3 文件 / 目录判定：只看尾部 `/`，拒绝含路径分隔符的输入

**决策**：`isDir = strings.HasSuffix(result, "/")`，`name = strings.TrimSuffix(result, "/")`。在去掉尾部 `/` 得到 `name` 后，**检查 `name` 是否仍含路径分隔符**（如输入 `a/b` → `name` 含 `/`），若有则弹 MsgDialog 提示用户「名字只能是纯文件名或纯目录名，不能包含路径分隔符」。

**理由**：功能语义是「在当前目录下新建一个文件/子目录」，输入应该是单个名字，而非路径。Unix 文件名本身不能含 `/`，用户输入 `a/b` 多数是误解了功能边界；明确拒绝比让 `os.Mkdir` / `os.Create` 报错更友好。

### 4.4 建文件用 `os.Create`、建目录用 `os.Mkdir`

- 文件：`os.Create(full)` 建空文件（已预检不存在，截断风险无）；拿到 `*File` 后立即 `Close()`。
- 目录：`os.Mkdir(full, 0755)`，只建一级子目录（不用 `MkdirAll`——用户输入 `a/b/` 时 `MkdirAll` 会偷偷建多层，不符合「在当前目录下新建一个子目录」的语义，让 `os.Mkdir` 报错走 showError 更诚实）。

### 4.5 anchor：当前行下方，与 rename/delete 一致

复用 `anchor = {X: fm.rect.X, Y: fm.rect.Y + 2 + s.cursor - s.topIdx}`。add 虽不读光标条目，但用户视线在光标处，弹窗贴着当前行下方最自然；光标在面包屑（`cursor==0`）时 `anchorY = fm.rect.Y + 2`，InputDialog 紧贴面包屑下方，也合理。`AutoExpand` 已由 InputDialog.Open 内部处理屏底不足。

### 4.6 title 文案

`"Add"`。InputDialog 会把它嵌进上边框 `──Add──...─`。

---

## 5. 实现步骤

### 5.1 `session.go`：重构 close + 新增 closePicked

**文件**：`internal/finder/session.go`，替换现有 `close`（约 295 行起），新增 `closePicked` / `finishClose`（代码见 §4.1）。`close` 的 5 个现有调用点不改。

### 5.2 `session.go`：handleKey 加 `'a'` 分支

**文件**：`internal/finder/session.go`，`handleKey` 的 KeyRune switch（约 796 行），在 `case 'r':` 旁加：

```go
case 'a':
	fm.startAdd()
```

`'a'` 当前空闲（已占用：`.` / `q` / `d` / `r`）。

### 5.3 新建 `add.go`

**新文件**：`internal/finder/add.go`

```go
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
```

### 5.4 编译验证

```bash
make build
```

预期无循环 import（add.go 只引 `os` / `path/filepath` / `strings` / `config` / `dialog`，与 rename.go 同构）。

---

## 6. 文件变更

| 文件 | 改动 |
|---|---|
| `internal/finder/session.go` | `close` 拆出 `finishClose`（纯内部重构，调用点不变）；新增 `closePicked(name)`；`handleKey` KeyRune switch 加 `case 'a'` 调 `startAdd()` |
| `internal/finder/add.go` | **新文件**，实现 `startAdd()` |

**不改的文件**：
- `internal/finder/rename.go`：直接复用 `showError()`。
- `internal/dialog/*.go`：InputDialog / MsgDialog API 已满足，零改动。
- `internal/action/fileops.go`：`onFinderClose` 的 `Picked` 分支已能处理「打开指定文件」，零改动。
- `cmd/micro/micro.go`：FloatFrame 模态路由已天然支持 finder → InputDialog 切换（F4 已论证）。

---

## 7. 边界情况

| 场景 | 处理 | 代码位置 |
|---|---|---|
| 光标在面包屑（cursor==0） | **照常响应**（add 不依赖条目）；InputDialog 紧贴面包屑下方 | `startAdd` 无 cursor 检查 |
| 空目录（只有面包屑） | 同上，可正常新建 | 同上 |
| Esc / resize | `canceled==true` → 回调 return，不开建、不刷新 | 回调头部 |
| 输入全为空 / 仅 `/` | `name==""` → no-op | `name==""` 检查 |
| 名字已存在（文件或目录同名都算） | `os.Stat` 命中 → showError，列表不变 | 重名检查分支 |
| 新建 `.hidden` 文件且 showHidden=false | 建文件成功 → `closePicked(name)` 直接带名关闭，**不**走光标定位，不会打开错误文件 | §4.1 决策 |
| 新建 `.hidden` 目录且 showHidden=false | `chdirTo(dir, name)` 的 locate 找不到 → 光标落首条目（复用 locate 现有行为），不崩溃；用户按 `.` 可显示 | 复用 chdirTo |
| 输入含中间 `/`（如 `a/b`） | `os.Mkdir` / `os.Create` 报错 → showError | 不特殊处理（同 rename） |
| 权限不足 / 只读文件系统 | `os.Mkdir` / `os.Create` 返回 error → showError，列表不变 | err 分支 |
| 新建文件后 onFinderClose 的同文件 no-op | 新建空文件 AbsPath 不会等于当前 buffer，正常走 `OpenCmd` | `fileops.go:50-52` 不触发 |
| resize 期间 InputDialog 和 finder 同时关 | InputDialog onResult("",true) → 回调 return；finder 由主循环 resize 分支 `close(Resize)` 收尾，互不踩状态 | F4 已论证 |
| 输入含路径分隔符（如 `a/b`） | 回调检查命中 → showError 提示「Name must be a plain file or directory name」、列表不变 | §4.3 决策 |
| InputDialog 开着时按 `q` / `Ctrl-q` | FloatFrame 拦截 Quit → 先关 InputDialog → 回调 canceled=true → return；磁盘无残留 | 模态路由保证 |

---

## 8. 风险

| 风险 | 说明 | 应对 |
|---|---|---|
| `close` 重构遗漏调用点 | 抽 `finishClose` 是内部拆分，但需确认 5 个 `close(...)` 调用点行为不变 | `make build` + 跑一遍 rename/delete/Enter/Esc/Ctrl-q 手测 |
| `closePicked` 与 `chdirTo` 异步 git 竞态 | 文件场景直接 `closePicked` 关闭，不调 `chdirTo`，无 fetchGit goroutine 启动；目录场景的 `chdirTo` 内 `go fetchGit` 写的是即将被 GC 的 state，`reset` 已注释保留 state 过渡持有，不崩溃 | 复用现有并发保护 |
| InputDialog 回调里再开 MsgDialog（重名/失败）时序 | InputDialog 的 onResult 在 `TheFloatFrame.Close` 之后触发，回调里开 MsgDialog 是「先关一个再开一个」，与 F4 rename 名字冲突路径同构 | 已有先例，无额外处理 |
| 用户连按 `a` 快速操作 | InputDialog 开着时 finder 不收事件（FloatFrame 旁路），不会重入 `startAdd` | 架构天然保证 |

---

## 9. 手测清单

1. **基本新建文件**：finder 中按 `a` → InputDialog 弹出、无预填 → 输入 `new.txt` → Enter → finder 关闭、新文件被打开进入编辑（空 buffer）。
2. **基本新建目录**：按 `a` → 输入 `subdir/` → Enter → 列表刷新、光标落在 `subdir/` 上、`▸` 标志与 `/` 后缀正常显示。
3. **面包屑上可建**：光标移到顶部面包屑 → 按 `a` → 正常弹出 InputDialog（rename/delete 此处 no-op，add 不 no-op）。
4. **空目录可建**：进入一个空目录（只有面包屑行）→ 按 `a` → 正常新建。
5. **Esc 取消**：按 `a` → Esc → InputDialog 关闭、列表不变、磁盘无新文件。
6. **空名字**：按 `a` → 不输直接 Enter → 无操作。
7. **仅斜杠**：按 `a` → 输入 `/` → Enter → 无操作（name 为空）。
8. **重名（文件）**：按 `a` → 输入已存在文件名 → Enter → MsgDialog 显示 `xxx already exists`、列表不变、磁盘无改动。
9. **重名（目录）**：按 `a` → 输入已存在目录名（带 `/`）→ Enter → 同上提示。
10. **新建 `.hidden` 文件（showHidden=false）**：按 `a` → 输入 `.secret` → Enter → finder 关闭、`.secret` 被打开（验证 `closePicked` 绕过光标定位正确，未打开错误文件）。
11. **新建 `.hidden` 目录（showHidden=false）**：按 `a` → 输入 `.secret/` → Enter → 列表刷新但新目录不可见；按 `.` 显示 → 看到新目录、光标落在其上。
12. **中文 / 双宽字符**：新建中文名文件 → 正常打开；新建中文目录 → 列表正确显示。
13. **长名字水平滚动**：输入超长名字 → InputDialog 内光标滚动正常。
14. **权限不足**：在只读目录按 `a` 建文件 → MsgDialog 显示 `create: ...`、列表不变。
15. **resize**：InputDialog 开着时缩放终端 → InputDialog 和 finder 都关闭、不崩溃、磁盘无残留。
16. **新建文件后 onFinderClose 正确触发**：确认新建的文件成为当前 buffer（状态栏文件名更新），原 buffer 仍在 tab 列表里。
17. **rename/delete 回归**：重构 `close` 后，按 `r` 改名、按 `d` 删除、Enter 打开、Esc / Ctrl-q 退出全部正常（验证 `finishClose` 拆分无回归）。
18. **InputDialog 内按 `q` / `Ctrl-q`**：Input 弹出后按 `q` 或 `Ctrl-q` → InputDialog 关闭、回调 canceled=true → 无新建文件、finder 随后由主循环处理 Quit（验证 FloatFrame 事件路由正确）。
19. **输入含路径分隔符**：按 `a` → 输入 `foo/bar` → Enter → MsgDialog 显示「Name must be a plain file or directory name (no path separators)」、列表不变。
