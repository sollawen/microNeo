# F4a · FileSelector Rename 实现方案

**前置**：InputDialog（`internal/dialog/input.go`）已实现并可用 `:inputtest` 验证；dialog 包迁移（N1a）已完成。
**范围**：在 finder 中按 `r` 对光标所在文件 / 子目录重命名。不含批量重命名、跨目录移动。

---

## 1. 目标

用户在 finder 会话中，光标停在文件或子目录上时按 `r`，在当前行下方弹出一个 InputDialog（预填现有名字），用户编辑后返回新名字，finder 执行 `os.Rename` 并刷新列表。光标在面包屑上时不响应。

**同名处理**：如果用户返回的新名字与现有名字相同（去掉目录 `/` 后缀后比较），则不做任何处理，不执行 `os.Rename`，也不刷新列表。

---

## 2. 现状盘点（已搜已查，列出依据）

| 能力 | 现状 | 依据 |
|---|---|---|
| InputDialog 组件 | **已实现**，含三态退出、水平滚动、CJK、用户键位跟随 | `internal/dialog/input.go`（完整），`:inputtest` 命令在 `command_neo.go:142-172` |
| FloatFrame 容器 | **已实现**，modal 事件路由、resize 拦截、AutoExpand | `internal/dialog/frame.go`，主循环 `micro.go:536-588` 已接 |
| finder 键位分发 | 已有 `handleKey`，rune 分支里 `'.'` / `'q'` 已占，`'r'` 空闲 | `session.go:656-694` |
| finder 屏坐标换算 | drawContent 已有完整的 `listTop` / `cursor` / `topIdx` 映射，可推出光标行屏 Y | `session.go:346-422` |
| finder 目录刷新 | `chdirTo(target, focusName)` 已实现：重读目录 + 重建视图 + 定位光标 + 异步 git | `session.go:758-790` |
| finder → dialog 引用 | **已存在**。finder import dialog，InputDialog 直接用 `config.KeyName` 解析键位 | `session.go:8-12` import 块 |

**关键结论**：InputDialog 直接用 `config.KeyName` 解析键位，无需 keyResolver 注入。主要工作量在 finder 侧（加 `'r'` 入口、算 anchor、执行 rename、刷新）。

---

## 3. 架构：rename 期间的事件流与渲染流

### 3.1 事件路由（已由现有架构天然支持，无需改主循环）

```
用户按 r（finder 开、InputDialog 未开）
  micro.go:587  TheFloatFrame.IsOpen() == false
  → Tabs.HandleEvent → BufPane.HandleEvent
    → finder.IsOpen() == true → finder.HandleEvent → handleKey
      → 'r' → fm.startRename() → TheFloatFrame.Open(InputDialog)

用户编辑（InputDialog 开）
  micro.go:587  TheFloatFrame.IsOpen() == true
  → TheFloatFrame.HandleEvent → InputDialog.handleEvent
  （finder 不收事件；BufPane.HandleEvent 的 finder 转发分支根本不执行）

用户按 Enter
  InputDialog.handleEvent → TheFloatFrame.Close() → onResult(newName, false)
  → finder 回调：os.Rename + chdirTo 刷新

下一帧事件
  TheFloatFrame.IsOpen() == false → 回到 finder 正常路由
```

**无需改主循环**：`micro.go:587` 的 `else if dialog.TheFloatFrame.IsOpen()` 分支天然把 InputDialog 期间的事件路由给 FloatFrame，finder 自动被旁路。

### 3.2 渲染顺序（已天然正确）

```
micro.go:531-537
  Tabs.Display → BufPane.Display → finder.Display（画文件列表）
  InfoBar.Display
  TheFloatFrame.Display（画 InputDialog，覆盖在 finder 之上）
```

InputDialog 最后画，覆盖 finder 对应区域。finder 全帧照常渲染（用户能看到列表上下文）。

### 3.3 依赖图（无环）

```
finder → dialog → { config, screen, buffer, util, tcell, runewidth }
action → finder
action → dialog
```

dialog 不 import finder / action，finder import dialog 不构成环。已核实 dialog 包 import 列表（`input.go:6-13`、`frame.go:8-12`、`select.go:3-7`）。

---

## 4. 设计决策

### 4.1 错误处理：finder 内部直接用 MsgDialog

**决策**：rename 失败时，finder 内部直接打开 MsgDialog 显示错误，不通过 onError 回调 owner。

**理由**：
- **封装性**：owner 不需要知道 finder 运行时的错误处理逻辑，只负责创建 Session。
- **简单性**：NewSession 签名不变，无需注入回调。
- **用户体验**：MsgDialog 模态显示，用户确认后关闭，确保错误被看到。

**已否决**：
- onError 回调 → 需要 owner 知道 finder 的内部错误处理，违反封装原则。
- finder import action 调 InfoBar → 循环依赖（action → finder → action）。

### 4.2 anchor 位置：当前行下方，左右各离 finder 边框 1 格

**决策**：
- `anchor.Y` = 光标行屏 Y + 1（InputDialog 外框顶 = 当前行下一行）。
- `anchor.X` = `fm.rect.X + 1`（dialog 左边框离 finder 左边框 1 格）。
- `width` = `fm.state.pickerW - 2`（dialog 右边框离 finder 右边框 1 格）。
- `AutoExpand = true`（InputDialog.Open 已硬编码为 true）：屏底空间不足时自动翻到上方，屏右空间不足时左移 / clamp。

**光标行屏 Y 推导**（从 `drawContent` 的 `listTop` + `vi` 映射反推，`session.go:348-392`）：

```
contentTop = fm.rect.Y + 1           （外框 +1 边框）
listTop    = contentTop + 1          （面包屑占 contentTop）
entry at vi 画在 listTop + vi
entry at vi 对应 showEntries[topIdx + vi]，其 cursor = topIdx + vi + 1
→ vi = cursor - topIdx - 1
→ 光标行屏 Y = listTop + (cursor - topIdx - 1)
            = fm.rect.Y + 1 + cursor - topIdx     （cursor ≥ 1 时）
```

所以 `anchor.Y = fm.rect.Y + 1 + s.cursor - s.topIdx + 1 = fm.rect.Y + 2 + s.cursor - s.topIdx`。

`ensureVisible` 保证光标恒在视口内，故该屏 Y 恒有效。

### 4.3 目录识别：initial 带 `/`，result 去掉 `/`

**决策**：
- 打开 InputDialog 时，目录的 initial = `e.name + "/"`（与 drawEntry 的显示一致：目录恒带 `/` 后缀）。
- 回调拿到 result 后，先 `strings.TrimSuffix(result, "/")` 得到真实新名字，再做空 / 同名检查和 `os.Rename`。
- 不依赖 result 的 `/` 来判断是否目录——finder 自己有 `e.isDir`，逻辑层不靠字符串尾推断。

### 4.4 刷新：复用 chdirTo，不新写刷新逻辑

**决策**：rename 成功后调 `fm.chdirTo(fm.state.currentDir, newName)`。

**理由**：
- `chdirTo` 已封装「重读目录 + rebuildShow + 定位光标到 focusName + ensureVisible + 异步 fetchGit」全流程，恰好是 rename 后需要的一切。
- 改名后排序位置可能变（字母序），但 `chdirTo` 重读目录 + 重建 `showEntries` 后，`locate` 逻辑会自然找到新名字的位置，光标始终落在该条目上。
- 同步重读目录是 μs 级（`readDirEntries` 调 `os.ReadDir`），无性能问题。

### 4.5 title 文案

`"Rename"`。InputDialog 会把它嵌进上边框：`──Rename──...─`。

---

## 5. 实现步骤

### 5.1 finder 加 import

**文件**：`internal/finder/session.go` import 块加：

```go
"github.com/micro-editor/micro/v2/internal/dialog"
```

### 5.2 handleKey 加 `'r'` 分支

**文件**：`internal/finder/session.go`，handleKey 的 KeyRune switch（`session.go:656-694`）

在 `case 'q':` 旁加：

```go
case 'r':
	fm.startRename()
```

### 5.3 新建 rename.go 实现

**新文件**：`internal/finder/rename.go`

```go
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

	// anchor：当前行下方一行，左右各离 finder 边框 1 格
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
		fm.state.pickerW-2, // 内容区宽：比 finder 内容区左右各窄 1 格
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
```

### 5.4 编译验证

```bash
make build
```

若报循环 import：检查 finder 是否误引了 action（`rg '"internal/action"' internal/finder/`）。预期不会，因 dialog 是叶子包。

---

## 6. Files to Modify / New Files 速查

| 文件 | 改动 |
|---|---|
| `internal/finder/session.go` | import 加 `dialog`；`handleKey` 的 KeyRune switch 加 `case 'r'` 调用 `startRename()` |
| `internal/finder/rename.go` | **新文件**，实现 `startRename()` 和 `showError()` 方法 |

**不改的文件**：
- `internal/dialog/input.go`（InputDialog 已完备，零改动）
- `internal/dialog/frame.go`（容器已完备）
- `cmd/micro/micro.go`（主循环事件路由已天然支持 finder → InputDialog 切换）
- `internal/action/fileops.go`（OpenFinder / onFinderClose 不涉及 rename）

---

## 7. 边界情况处理

| 场景 | 处理 | 代码位置 |
|---|---|---|
| 光标在面包屑（cursor==0） | 静默 no-op，不开 InputDialog | `startRename` 头部 `s.cursor == 0` 检查 |
| 光标越界（空目录等） | 静默 no-op | `idx` 范围检查 |
| 用户按 ESC / resize | `canceled == true` → 回调直接 return | 回调头部 |
| 新名字为空（用户全删了） | no-op，不调 os.Rename | `newName == ""` 检查 |
| 新名字 == 旧名字（含去掉 `/` 后） | no-op | `newName == oldName` 检查 |
| 名字冲突（目标已存在） | `os.Rename` 返回 error → `fm.showError(msg)` → 弹窗显示错误 → 不刷新（列表不变） | 回调内 err 分支 |
| 权限不足 | 同上（os.Rename error） | 同上 |
| 新名字含 `/`（如 `a/b`） | `os.Rename` 会尝试跨目录移动或失败；首版不拦——Unix 下文件名合法不含 `/`，用户输入 `/` 时 os.Rename 自然报错，走 showError 路径 | 不特殊处理 |
| resize 期间 InputDialog 和 finder 同时关 | 主循环 resize 分支同时调 Tabs.HandleEvent（→ finder.close(Resize)）和 TheFloatFrame.HandleEvent（→ InputDialog onResize → onResult("",true)）。finder 先关、InputDialog 后关；回调里 `canceled==true` 直接 return，不碰已关闭的 finder 状态。若 rename 成功后 resize（chdirTo 执行中），两者在视觉上无冲突：chdirTo 的 Redraw 会被主循环的下一帧渲染覆盖 | 已由现有架构保证 |
| rename 后该条目因 showHidden=false 而不可见（如改成 `.` 开头） | `chdirTo` 的 `locate` 找不到 focusName 时落首条目（`session.go:262-272` 的 `locate` 逻辑），不崩溃 | 复用 locate 现有行为 |
| rename 期间目标目录被删除 | `os.Rename` 返回 error（如「no such file」），走 showError 路径；用户关闭弹窗后仍在原目录，finder 状态未改变 | 同名字冲突 |

---

## 8. 风险

| 风险 | 说明 | 应对 |
|---|---|---|
| finder import dialog 导致循环依赖 | 理论上不会（dialog 不引 finder/action），但需编译验证 | `make build` 后 `rg '"internal/action"' internal/finder/` 确认 |
| chdirTo 重读目录期间 fetchGit goroutine 竞态 | chdirTo 在 mu.Lock 内换 allEntries，fetchGit 检查 `s.currentDir != dir` 丢弃旧结果 | 已有并发保护（`session.go:204-213`），rename 场景 target==currentDir 不触发「导航离开」分支 |
| InputDialog 期间 finder 后台 fetchGit 回调 Redraw | Redraw 触发全帧重绘（finder + InputDialog），无竞态：finder.Display 读 state 有锁，InputDialog 状态自洽 | 无额外处理 |
| anchor.Y 在光标位于列表末行时可能屏底放不下 | AutoExpand 自动翻到上方（expandAnchor 的 upSpace 分支） | 已由 FloatFrame 处理 |
| 目录 rename 后 git 状态短暂为空 | chdirTo 首帧 gitChar 全 0，fetchGit 异步回填后更新 | 与普通 chdir 行为一致，用户可接受 |

---

## 9. 手测清单

实现后按以下步骤验证：

1. **基本 rename（文件）**：finder 中光标停在某文件 → 按 `r` → InputDialog 在下方弹出、预填文件名 → 改名 → Enter → 列表刷新、光标落在新名字上。
2. **基本 rename（目录）**：光标停在目录 → 按 `r` → InputDialog 预填 `dirname/` → 编辑 → Enter → 目录改名、`/` 后缀保留显示。
3. **面包屑不响应**：光标移到顶部面包屑 → 按 `r` → 无反应（不开 InputDialog）。
4. **ESC 取消**：按 `r` → ESC → InputDialog 关闭、列表不变。
5. **空名字**：按 `r` → 全删 → Enter → 无操作（不 rename、不报错）。
6. **同名**：按 `r` → 不改 → Enter → 无操作。
7. **名字冲突**：按 `r` → 改成已存在的名字 → Enter → MsgDialog 弹窗显示错误、列表不变。
8. **中文 / 双宽字符**：rename 成中文名 → 列表正确显示。
9. **长名字水平滚动**：输入超长名字 → InputDialog 内光标滚动正常。
10. **用户键位生效**：改 `bindings.json` 把 `CursorLeft` 绑到别的键 → InputDialog 内生效（验证 `config.KeyName` 链路）。
11. **resize**：InputDialog 开着时缩放终端 → InputDialog 和 finder 都关闭，不崩溃。
12. **rename 后 git 状态刷新**：在 git 仓库内 rename → 列表首帧无 git 标志 → 短暂延迟后 git 标志出现。
13. **连续 rename 快速操作**：连按两次 `r`，第一次 rename 成功后立刻再按 `r`（chdirTo 的 Redraw 可能未完成）—— 验证无状态残留或 panic。
