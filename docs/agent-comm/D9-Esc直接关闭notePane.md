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
- **对 micro 原生代码零侵入**：所有改动落在 `notepane.go`，原生文件 `bufpane.go` 反而**净减一行**

---

## 架构事实（理解方案为什么长这样）

> 这两点是从 micro 现有代码核实出来的，决定了方案的具体形态。

### 事实 1：notePane 内部 binding 树反查全局 `BufKeyActions`

notePane 的内部 binding 树（`NotePaneBindings`）解析 action 字符串时，在 `notePaneRegisterBinding()`（`notepane.go:200` 附近）里这样查函数指针：

```go
if f, ok := BufKeyActions[a]; ok {   // ← 反查全局表
    afn = f
} else if f, ok := BufMouseActions[a]; ok {
    afn = f
} else {
    continue                          // ← 查不到，binding 被静默丢弃
}
```

含义：**notePane 专属的 action 名字，必须能从 `BufKeyActions` 查到，否则 binding 不生效。** 这就是为什么之前 `NotePaneSend` 被迫注册在 `bufpane.go:860`（microNeo 加的行，非 micro 原生）。

**变体方案的切入点**：改 `notePaneRegisterBinding()` 的查询顺序，让 notePane 专属 action 从私有 map 查，不必再进 `BufKeyActions`。

### 事实 2：`KeyTree` 对同一个 key 重复注册是**覆盖**语义（不是追加）

`keytree.go:142-170` 的 `registerBinding()`：

```go
// newNode.actions = append(newNode.actions, a)   ← 旧的追加逻辑被注释掉了
newNode.actions = []TreeAction{a}                ← 现在是直接替换
```

含义：`init()` 里先 `notePaneMapDefaults(...)` 灌进 buffer 默认的 `Esc → Escape,Deselect,...`，**后** `notePaneMapBinding("Esc", "NotePaneClose")` 会**完全覆盖**前者。不存在"两套动作同时跑"的风险，不需要 `DeleteBinding`。

### 事实 3：`NotePaneBindings` 与主编辑器的 `BufBindings` 是两棵物理隔离的树

micro 有两棵独立的 KeyTree：

| 树 | 定义 | 谁在用 |
|---|---|---|
| `BufBindings` | `bufpane.go:28, 45` | 主编辑器 |
| `NotePaneBindings` | `notepane.go:38, 108` | notePane（D9 改的就是这棵） |

`bufpane.go:536-541` 的 `Bindings()` 方法决定一个 pane 用哪棵：

```go
func (h *BufPane) Bindings() *KeyTree {
	if h.bindings != nil {
		return h.bindings     // notePane：n.BufPane.bindings = NotePaneBindings（notepane.go:255）
	}
	return BufBindings      // 主编辑器：bindings 字段一直是 nil，走全局
}
```

关键点：notePane 是**单独 `newBufPane` 出来的一个 BufPane 对象**（`notepane.go:252-255`），它把自己的 `bindings` 字段设成 `NotePaneBindings`。而**主编辑器是另一个 BufPane 对象**，它的 `bindings` 字段从头到尾都是 `nil`，所以一直走 `BufBindings`。

**关闭 notePane 时**：`close()`（`notepane.go:507`）只设 `isOpen = false` + 恢复主编辑器滚动，**完全不碰主编辑器的 binding**。主编辑器一直就是 `BufBindings`，里面的 `Esc → Escape,Deselect,ClearInfo,RemoveAllMultiCursors` 从程序启动到现在就没动过。

**含义**：D9 改的 `Esc` 只活在 `NotePaneBindings` 这棵树里。notePane 关闭、回到主编辑器后，Esc 仍然是原来的 buffer 语义（清信息栏 + 取消选区 + 移除多光标）。**两棵树物理隔离，不存在污染全局 Esc 的风险**。

---

## 方案选择（为什么不用原始 D9 方案）

| | 原始方案 | **变体方案（采用）** |
|---|---|---|
| `NotePaneClose` 注册在哪 | `bufpane.go` 的 `BufKeyActions`（新增 1 行） | `notepane.go` 的私有 `notePaneActions` map |
| `NotePaneSend` 现状 | `bufpane.go:860`（microNeo 已加） | **从 `bufpane.go` 迁出**，归 `notePaneActions` |
| `bufpane.go` 净变化 | +1 行 | **−1 行**（净改善，原生文件越改越干净） |
| notePane 专属 action 内聚度 | 散落两个文件 | 全部收进 `notepane.go` |
| 符合 AGENTS.md "原生零侵入"原则 | ❌ 进一步侵入 | ✅ 反向改善 |

**决定**：采用变体方案。代价是引入一个 notePane 私有 action map（约 6 行），收益是 `bufpane.go` 净减一行、notePane 专属 action 全部内聚。

---

## 改动方案（集中在 `notepane.go` + `bufpane.go` 删一行）

### 改动 1：`notepane.go` 新增私有 action map + `notePaneClose` 函数

位置：`NotePaneSend` 函数定义附近（`notepane.go:358` 附近）。

```go
// notePaneActions 是 notePane 专属 action 的私有注册表。
// notePaneRegisterBinding() 优先查它，找不到再 fallback 到全局 BufKeyActions。
// 这样 notePane 专属 action 不必污染 bufpane.go 的 BufKeyActions（原生文件零侵入）。
var notePaneActions = map[string]BufKeyAction{
	"NotePaneSend":  NotePaneSend,
	"NotePaneClose": notePaneClose,
}

// notePaneClose 关闭 notePane（不发送任何内容，符合 TUI "Esc = 取消" 约定）。
// 复用已有 (*NotePane).close()，不重写。
func notePaneClose(h *BufPane) bool {
	if n := TheNotePane; n != nil && n.IsOpen() {
		n.close()
	}
	return true
}
```

> 实施时以实际代码为准：`BufKeyAction` 类型名参考 `bufpane.go:729` 的 `var BufKeyActions = map[string]BufKeyAction`；`close()` 位于 `notepane.go:507`。

### 改动 2：`notepane.go` 的 `notePaneRegisterBinding()` 优先查私有 map

位置：`notepane.go:200` 附近。把：

```go
var afn BufAction
if f, ok := BufKeyActions[a]; ok {
    afn = f
} else if f, ok := BufMouseActions[a]; ok {
    afn = f
} else {
    continue
}
```

改为（加一层私有 map 优先查询）：

```go
var afn BufAction
if f, ok := notePaneActions[a]; ok {   // notePane 专属 action 优先
    afn = f
} else if f, ok := BufKeyActions[a]; ok {
    afn = f
} else if f, ok := BufMouseActions[a]; ok {
    afn = f
} else {
    continue
}
```

### 改动 3：`bufpane.go:860` 删除 `"NotePaneSend": NotePaneSend,` 这一行

这一行是 microNeo 之前加的（不是 micro 原生）。删除后 `NotePaneSend` 由 `notePaneActions`（改动 1）接管，行为不变。**`bufpane.go` 净减一行**，原生文件越来越干净。

> 删除时注意周围格式，别留多余逗号或空行。

### 改动 4：`notepane.go` 白名单放行 `NotePaneClose`

位置：`allowedNotePaneActions`（`notepane.go:42` 附近），在 `"NotePaneSend": true,`（`notepane.go:96`）旁边加：

```go
"NotePaneClose": true,
```

**为什么必须**：白名单过滤机制（`filterActions()`）会丢掉不在白名单的 action。如果不加，`Esc → NotePaneClose` 在 init 时被过滤掉，binding 根本没生效。`NotePaneSend` 已在白名单，保留不动。

### 改动 5：`notepane.go` 的 `init()` 绑定 `Esc → NotePaneClose`

位置：`notepane.go:107` 附近。把：

```go
func init() {
	NotePaneBindings = NewKeyTree()
	notePaneMapDefaults(DefaultBindings("buffer"))
	// Bind Alt-Enter to NotePaneSend (fallback Alt-s if Alt-Enter not available in terminal)
	notePaneMapBinding("Alt-Enter", "NotePaneSend")
}
```

改为（追加一行）：

```go
func init() {
	NotePaneBindings = NewKeyTree()
	notePaneMapDefaults(DefaultBindings("buffer"))
	// Bind Alt-Enter to NotePaneSend (fallback Alt-s if Alt-Enter not available in terminal)
	notePaneMapBinding("Alt-Enter", "NotePaneSend")
	// Bind Esc to NotePaneClose (cancel draft without sending — TUI convention).
	// 依赖 KeyTree 覆盖语义（事实 2）：覆盖前一步灌进来的 Esc → Escape,Deselect,...
	notePaneMapBinding("Esc", "NotePaneClose")
}
```

**为什么不冲突**：见上面"事实 2"，`KeyTree` 对同一个 key 后注册的会完全覆盖前注册的。

---

## 行为变化矩阵

| 场景 | 旧行为 | D9 修复后 |
|---|---|---|
| notePane 内按 Esc | 仅清 infobar + 取消选区 + 移除多光标 | **直接关闭 notePane** ✅ |
| notePane 内按 Alt-Enter | 发送 + 关闭 | 发送 + 关闭（不变） |
| 主编辑器内按 Esc | 走 `Esc → Escape,...`（buffer binding）| 不变 |
| notePane 未开 | Esc 走 buffer binding | 不变 |

---

## 与 D8 的关系

- **D8**：主编辑器侧"打开"键的可配置化（删除 `cmd/micro/micro.go` 硬编码，加 `NotePaneOpen` 到 `BufKeyActions` + `defaults_*.go`）
- **D9**：notePane 内部侧"关闭"键的引入（加 `NotePaneClose` 到 `notePaneActions` 私有 map，绑定到 `Esc`）

**互不冲突**：
- D8 改的是 `BufKeyActions` 中"buffer scope"的部分 + 主循环硬编码
- D9 改的是 `notepane.go` 的私有 map + `notePaneRegisterBinding()` 查询顺序 + `init()` + 白名单；`bufpane.go` 反而删一行
- 两个 action 名字不同（`NotePaneOpen` vs `NotePaneClose`），互不干扰

**合并顺序**：
- 可以 D8 先 D9 后
- 可以 D9 先 D8 后
- 互不阻塞

---

## 不动的东西

- `close()` 函数本体（`notepane.go:507`）—— 复用
- `NotePaneSend` 函数本体 —— 不变，只改注册位置
- `notePaneMapDefaults(DefaultBindings("buffer"))` —— 仍把 buffer defaults 灌进来（`Escape` 等其他 action 不动）
- notePane 的 `NotePaneBindings` KeyTree 机制 —— 不动
- `keytree.go` 的 `registerBinding()` 覆盖语义 —— 不动（只是依赖它）
- `~/.config/microNeo/bindings.json`（`Esc → NotePaneClose` 是 notePane 内部 binding，**不走**用户级 `bindings.json`，与 `NotePaneSend` 同等待遇）

### 关于"为什么 `Esc → NotePaneClose` 不走 `bindings.json`"

notePane 内部 binding 用 `NotePaneBindings` KeyTree，**不**接入 `bindings.json` 加载机制（与 `NotePaneSend → Alt-Enter` 同等）。原因：
- `NotePaneBindings` 是 notePane 内部的独立 binding 树，初始化在 `notepane.go:107`
- 主 `bindings.json` 加载走 `internal/action/bindings.go:40-79`，只针对 `BufKeyActions`
- 走 `bindings.json` 需要重写 `NotePaneBindings` 的加载链路（scope 蔓延，另开 D11 处理）

当前 `Esc → NotePaneClose` 在源码硬编码，与 `Alt-Enter → NotePaneSend` 对称。

---

## 验证步骤

1. `make build` 编译（不要直接 `go build`）
2. `go vet ./...` 无新增 warning
3. `grep -rn "NotePaneSend\|NotePaneClose" internal/action/` 确认：
   - `NotePaneSend` / `NotePaneClose` **不再**出现在 `bufpane.go`
   - 两者都在 `notepane.go` 的 `notePaneActions` map 里
   - 两者都在 `allowedNotePaneActions` 白名单
4. 手动测试：打开任意文件 → `Alt-i` 开 notePane → 输入文字
   - 按 `Esc` → **notePane 应关闭**，文字丢（草稿未发送）✅
   - `Alt-i` 重新开 → 文字已清空（scratch 行为）✅
   - `Alt-i` 开 → `Alt-Enter` 发送 → 仍正常发送 + 关闭 ✅
   - 主编辑器内按 `Esc` → 仍走 buffer binding（不影响）✅
5. `git diff --stat` 确认只动 `internal/action/notepane.go`（新增）+ `internal/action/bufpane.go`（删 1 行）

---

## 风险评估

- **行为变化**：notePane 内 Esc 行为从"清信息栏"→"关闭"。少数用户可能依赖旧行为清信息栏。
  - **缓解**：清信息栏本就是"想关没关掉"的副产物，notePane 关闭后用户主要在主编辑器活动，那里有自己的信息栏
  - **不破坏任何数据流**：发送路径走 `Alt-Enter`，独立于 Esc
- **白名单**：`NotePaneClose` 必须加进 `allowedNotePaneActions`，否则 binding 被过滤掉
- **私有 map 是新机制**：`notePaneActions` 是 D9 引入的小机制，需要 `notePaneRegisterBinding()` 配合改查询顺序。改动局部、可回退
- **覆盖语义依赖**：D9 依赖 `keytree.go` 当前的覆盖语义（事实 2）。如果将来 micro 上游改回 append 语义，`Esc` 会同时跑两套动作——届时需要加 `DeleteBinding` 或调整注册顺序
- **风险等级**：**低**——纯增量改动（私有 map + 白名单 + init 一行）+ 一行删除（`bufpane.go`），复用已有 `close()`

---

## 实施建议

作为独立 commit 提交：

```
feat(notepane): bind Esc to close notePane without sending

Esc 在 notePane 内当前只能清 infobar / 取消选区，不能关闭——半残废状态
误导用户以为能关。按 TUI 通用约定，Esc = 取消 = 关闭。

在 notepane.go 内建私有 notePaneActions map（管 NotePaneSend +
新增 NotePaneClose），让 notePaneRegisterBinding() 优先查它再
fallback 到 BufKeyActions。同时把已有的 NotePaneSend 从 bufpane.go
迁出，bufpane.go 净减一行，原生文件零侵入。

Esc → NotePaneClose 在 init() 绑定，依赖 KeyTree 覆盖语义
（后注册覆盖前注册），无需 DeleteBinding。Alt-Enter → NotePaneSend
发送路径不变。

Scope：与 D8（主编辑器侧打开键可配置化）解耦，可独立 commit。
```
