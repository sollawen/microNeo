# D9 — `Esc` 在 notePane 里直接关闭（不发送）

> 与 D8 范围解耦：D8 改"主编辑器侧打开键"，D9 改"notePane 内部侧关闭键"。

## 目标

让 notePane 打开时按 `Esc` **直接关闭**（不发送内容）。符合 TUI 通用约定（Esc = cancel）。

## 现状

notePane 内部当前 `Esc` 的默认 binding（经白名单过滤后）：

```
"Escape,Deselect,ClearInfo,RemoveAllMultiCursors"
```

| Action | 实际效果 |
|---|---|
| `Escape` | 空壳（`actions.go:1875-1878`，啥也不做） |
| `Deselect` | 取消选区 |
| `ClearInfo` | 清 infobar 消息 |
| `RemoveAllMultiCursors` | 移除多光标 |

**结果**：Esc 当前**不能关闭 notePane**，只是清 infobar + 取消选区 + 移除多光标。

**用户痛点**：想取消草稿但又不想发送，只能看着 notePane 杵在屏幕上干瞪眼。半残废的 Esc 还误导用户以为能关。

## 设计原则

- **Esc = 取消**是 vim / less / micro / 几乎所有 TUI 的约定
- notePane 关闭 = 销毁 buffer（已在 `open()` 时承诺"开新"）
- 不需要单独的"取消"键——Esc 已经存在且无有用功能

## 改动方案

### 改动 1：新增 `NotePaneClose` action

**文件**：`internal/action/bufpane.go`（与 `NotePaneSend` 同处）

在 `BufKeyActions` 字典的 `"NotePaneSend"` 附近添加：

```go
"NotePaneClose":            notePaneClose,
```

实现（可放同文件 / `notepane.go`）：

```go
// notePaneClose 关闭 notePane（不发送任何内容）。
// 注册为 BufKeyAction 以便走 notePane 内部 binding 树。
func notePaneClose(h *BufPane) bool {
    if TheNotePane != nil && TheNotePane.IsOpen() {
        TheNotePane.close()
    }
    return true
}
```

`close()` 已存在（`notepane.go:481`），由 `Toggle()` 和 `NotePaneSend()` 共用。**直接复用**，不重写。

### 改动 2：notePane 内部绑定 `Esc → NotePaneClose`

**文件**：`internal/action/notepane.go` `init()`（`notepane.go:108-112`）

```go
func init() {
    NotePaneBindings = NewKeyTree()
    notePaneMapDefaults(DefaultBindings("buffer"))
    // Bind Alt-Enter to NotePaneSend (fallback Alt-s if Alt-Enter not available in terminal)
    notePaneMapBinding("Alt-Enter", "NotePaneSend")
    // Bind Esc to NotePaneClose (cancel without sending)
    notePaneMapBinding("Esc", "NotePaneClose")
}
```

### 改动 3：白名单放行 `NotePaneClose`

**文件**：`internal/action/notepane.go` `allowedNotePaneActions`（`notepane.go:42-94`）

在白名单数组里添加 `"NotePaneClose"`（具体位置看现有结构）。

**为什么必须**：白名单过滤机制（`filterActions()`）会丢掉不在白名单的 action。如果不加，`Esc → NotePaneClose` 在 init 时被过滤掉，binding 根本没生效。

## 行为变化矩阵

| 场景 | 旧行为 | D9 修复后 |
|---|---|---|
| notePane 内按 Esc | 仅清 infobar + 取消选区 + 移除多光标 | **直接关闭 notePane** ✅ |
| notePane 内按 Alt-Enter | 发送 + 关闭 | 发送 + 关闭（不变） |
| 主编辑器内按 Esc | 走 `Esc → Escape,...`（buffer binding）| 不变 |
| notePane 未开 | Esc 走 buffer binding | 不变 |

## 与 D8 的关系

- **D8**：主编辑器侧"打开"键的可配置化（删除 `cmd/micro/micro.go` 硬编码，加 `NotePaneOpen` 到 `BufKeyActions` + `defaults_*.go`）
- **D9**：notePane 内部侧"关闭"键的引入（加 `NotePaneClose` action，绑定到 `Esc`）

**互不冲突**：
- D8 改的是 `BufKeyActions` 中"buffer scope"的部分 + 主循环硬编码
- D9 改的是 `BufKeyActions` 中"notePane 内部 binding 树 scope"的部分 + `init()` + 白名单
- 两个 action 名字不同（`NotePaneOpen` vs `NotePaneClose`），互不干扰

**合并顺序**：
- 可以 D8 先 D9 后
- 可以 D9 先 D8 后
- 互不阻塞

## 不动的东西

- `close()` 函数本体（`notepane.go:481`）—— 复用
- `NotePaneSend` 函数本体
- `notePaneMapDefaults(DefaultBindings("buffer"))` —— 仍把 buffer defaults 灌进来（`Escape` 等其他 action 不动）
- notePane 的 `NotePaneBindings` KeyTree 机制
- `~/.config/microNeo/bindings.json`（`Esc → NotePaneClose` 是 notePane 内部 binding，**不走**用户级 `bindings.json`，与 `NotePaneSend` 同等待遇）

### 关于"为什么 `Esc → NotePaneClose` 不走 `bindings.json`"

notePane 内部 binding 用 `NotePaneBindings` KeyTree，**不**接入 `bindings.json` 加载机制（与 `NotePaneSend → Alt-Enter` 同等）。原因：
- `NotePaneBindings` 是 notePane 内部的独立 binding 树，初始化在 `notepane.go:108`
- 主 `bindings.json` 加载走 `internal/action/bindings.go:40-79`，只针对 `BufKeyActions`
- 走 `bindings.json` 需要重写 `NotePaneBindings` 的加载链路（scope 蔓延，另开 D11 处理）

当前 `Esc → NotePaneClose` 在源码硬编码，与 `Alt-Enter → NotePaneSend` 对称。

## 验证步骤

1. `make build` 编译
2. 打开任意文件 → `Alt-i` 开 notePane → 输入文字
3. 按 `Esc` → **notePane 应关闭**，文字丢（草稿未发送）✅
4. `Alt-i` 重新开 → 文字已清空（scratch 行为）✅
5. `Alt-i` 开 → `Alt-Enter` 发送 → 仍正常发送 + 关闭 ✅
6. 主编辑器内按 `Esc` → 仍走 buffer binding（不影响）✅
7. 终端 resize → 不再丢内容（D10 已修，但 smoke test 一下）✅
8. `go vet ./...` 无新增 warning
9. `git diff --stat` 确认只动 1-2 个文件

## 风险评估

- **行为变化**：notePane 内 Esc 行为从"清信息栏"→"关闭"。少数用户可能依赖旧行为清信息栏。
  - **缓解**：清信息栏本就是"想关没关掉"的副产物，notePane 关闭后用户主要在主编辑器活动，那里有自己的信息栏
  - **不破坏任何数据流**：发送路径走 `Alt-Enter`，独立于 Esc
- **白名单**：`NotePaneClose` 必须加进 `allowedNotePaneActions`，否则 binding 被过滤掉
- **风险等级**：**低**——纯增量改动，复用已有 `close()`

## 实施建议

作为独立 commit 提交：

```
feat(notepane): bind Esc to close notePane without sending

Esc 在 notePane 内当前只能清 infobar / 取消选区，不能关闭——半残废状态
误导用户以为能关。按 TUI 通用约定，Esc = 取消 = 关闭。

新增 NotePaneClose action（复用已有 close()），在 notePane 内部 binding
树绑定 Esc → NotePaneClose，同时加白名单放行。Alt-Enter → NotePaneSend
发送路径不变。

Scope：与 D8（主编辑器侧打开键可配置化）解耦，可独立 commit。
```
