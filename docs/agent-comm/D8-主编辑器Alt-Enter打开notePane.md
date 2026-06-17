# D8 — 主编辑器 Alt-Enter 打开 notePane

> 配套计划：[D9 — Esc 直接关闭 notePane](./D9-Esc直接关闭notePane.md)（notePane 内部侧关闭键，已实施）。
> 本文档为 **v2 重写版**，以已敲定的决策为基线。

## 目标

把 notePane 的**打开**键从主循环硬编码 `Alt-i`，改成走 micro 标准 binding 机制注册的 `Alt-Enter`。

## 已敲定的决策

| 项 | 决定 | 理由 |
|---|---|---|
| 主编辑器打开键 | **`Alt-Enter`** | 与 notePane 内发送键同键，形成"Alt-Enter = notePane 主操作键"的统一心智 |
| notePane 关闭键 | **`Esc`**（硬编码） | TUI 通用约定，已在 D9 实施 |
| Alt-Enter 发送键 | **`Alt-Enter`**（硬编码） | 现状，与打开键同键，保持对称 |
| Alt-i | **彻底放弃** | 历史硬编码键，删硬编码块 + 不进 defaults，直接退役 |
| `Toggle()` 函数 | **删除** | "打开/关闭拆成两个语义由不同键承担"后，二合一语义已无用 |
| `NotePaneOpen` 注册位置 | **`BufKeyActions`（bufpane.go）** | 主编辑器侧 action，与 notePane 内部的 `notePaneActions` 私有 map 物理隔离 |

## 三个键的最终语义（一张表说清）

| 键 | 主编辑器上下文 | notePane 上下文 |
|---|---|---|
| `Alt-Enter` | **打开 notePane** | **发送 + 关闭**（既有硬编码，不变） |
| `Esc` | 原生 buffer 行为（清 info / 取消选区 / 移除多光标） | **关闭不发送**（D9 已实施） |
| `Alt-i` | 无操作（已退役） | 无操作 |

心智模型：**Alt-Enter 是 notePane 的主操作键，语义随上下文切换**——没开就开，开着就把草稿发出去。Esc 永远是"算了，不要"。

## 现状（要改掉的东西）

### 1. 主循环硬编码 Alt-i

`cmd/micro/micro.go:540-548`：

```go
// Alt-i toggles NotePane regardless of open/close state
if action.TheNotePane != nil && !resize {
    if e, ok := event.(*tcell.EventKey); ok {
        if e.Key() == tcell.KeyRune && e.Modifiers() == tcell.ModAlt && e.Rune() == 'i' {
            action.TheNotePane.Toggle()
            goto done
        }
    }
}
```

问题：
- 写死在主事件循环里，占 `goto done` 特判路径
- 用户无法在 `bindings.json` 里覆盖
- 与 micro 其余快捷键机制格格不入

### 2. `Toggle()` 成死代码

`internal/action/notepane.go:284`：

```go
func (n *NotePane) Toggle() { ... }
```

删硬编码块后无人调用（grep 确认全项目唯一调用点是 `micro.go:545`）。

### 3. `NotePaneSend` 已在 D9 中从 `BufKeyActions` 迁出

D9 已把 `NotePaneSend` 从 `bufpane.go` 迁到 `notepane.go` 的私有 `notePaneActions` map，`Alt-Enter → NotePaneSend` 在 `notepane.go:init()` 里硬编码绑定。**本计划不动这部分**。

## 改动方案

### 改动 1：注册 `NotePaneOpen` 到 `BufKeyActions`

**文件**：`internal/action/bufpane.go`

在 `BufKeyActions` 字典里新增一行（位置：之前 `NotePaneSend` 在的那一行附近）：

```go
"NotePaneOpen": notePaneOpen,
```

函数实现放在 `notepane.go`（与 `notePaneClose` / `NotePaneSend` 邻近，保持 notePane 相关函数内聚）：

```go
// notePaneOpen 从主编辑器打开 NotePane。
// 注册为主编辑器的 BufKeyAction，可走标准 bindings.json 机制覆盖默认键位。
// 守卫：notePane 已开态下重复触发是 no-op。
func notePaneOpen(h *BufPane) bool {
	if TheNotePane != nil && !TheNotePane.IsOpen() {
		TheNotePane.open()   // 小写：包内可见。notePaneOpen 与 open() 同在 internal/action 包
	}
	return true
}
```

> `NotePaneOpen` 是**主编辑器侧** action → 进 `BufKeyActions`。
> `NotePaneSend` / `NotePaneClose` 是 **notePane 内部侧** action → 进 `notePaneActions` 私有 map（D9 已就位）。
> 两个 map 的边界：谁的事件路由命中谁，action 就归谁。不要混。

### 改动 2：加默认 binding

**文件**：`internal/action/defaults_darwin.go` 和 `internal/action/defaults_other.go`

在 buffer 段的 `"Enter": "InsertNewline",` 附近添加：

```go
"Alt-Enter":      "NotePaneOpen",
```

### 改动 3：删除主循环硬编码

**文件**：`cmd/micro/micro.go:540-548`

整段 `// Alt-i toggles NotePane regardless of open/close state ...` 块（含内层 `if` 和 `goto done`）删除。

### 改动 4：删除 `Toggle()` 死代码

**文件**：`internal/action/notepane.go:284-290`

删除 `func (n *NotePane) Toggle() { ... }` 整个函数。

> 删硬编码块（改动 3）后 `Toggle()` 无任何调用点，是死代码。

## 为什么"同一个 Alt-Enter 干两件事"不会乱

### 两棵物理隔离的 binding 树

| 树 | 定义 | 谁在用 | 读 bindings.json？ |
|---|---|---|---|
| `BufBindings` | `bufpane.go` | 主编辑器 | ✅（通过 `defaults_*.go` + `bindings.json`） |
| `NotePaneBindings` | `notepane.go:108` | notePane 内部 | ❌（D9 已确认，源码 init() 灌入，不接入 bindings.json） |

- 主编辑器 `Alt-Enter → NotePaneOpen` 活在 `BufBindings`，走默认值 + 用户可覆盖
- notePane 内部 `Alt-Enter → NotePaneSend` 活在 `NotePaneBindings`，源码硬编码

### 事件路由（主循环层级）

notePane 打开态下事件**直接**进 notePane，不到主 buffer：

```go
} else if action.TheNotePane != nil && action.TheNotePane.IsOpen() {
    action.TheNotePane.HandleEvent(event)   // ← 不到主 buffer
} else {
    action.Tabs.HandleEvent(event)
}
```

主 buffer 的 `BufKeyActions`（注册了 `NotePaneOpen`）**完全不会被查**。

### init() 注册顺序的微妙点（后人易踩坑）

`notepane.go:init()`：

```go
notePaneMapDefaults(DefaultBindings("buffer"))   // 主编辑器 defaults 灌进 NotePaneBindings
                                                 //   Alt-Enter → NotePaneOpen 经白名单过滤后被丢弃
                                                 //   （NotePaneOpen 不在白名单 → 整条 binding 丢弃）
notePaneMapBinding("Alt-Enter", "NotePaneSend")  // 注册成功，独占 Alt-Enter
```

→ notePane 内 Alt-Enter 最终 = NotePaneSend。**不依赖 KeyTree 覆盖语义**，靠的是 `filterActions()` 先把不在白名单的 `NotePaneOpen` 整条丢掉。后人调整 init() 顺序时要注意这一点。

### 各场景汇总

| 场景 | 触发路径 | 结果 |
|---|---|---|
| 主编辑器按 `Alt-Enter` | 主 buffer binding → `NotePaneOpen` | 打开 ✅ |
| notePane 内按 `Alt-Enter` | 事件不进主 buffer；走 `NotePaneBindings → NotePaneSend` | 发送 + 关闭 ✅ |
| notePane 内按 `Esc` | 走 `NotePaneBindings → NotePaneClose` | 关闭不发送 ✅（D9） |
| 主编辑器按 `Alt-i` | 无 binding（已退役） | 无操作 ✅ |
| notePane 内按 `Alt-i` | 无 binding | 无操作 ✅ |

### 单例 + 守卫

`TheNotePane` 全局唯一（`notepane.go:30`、`globals.go:16`）。`notePaneOpen` 实现里加了 `!IsOpen()` 守卫，不会重复打开。

## 用户改键方式

`~/.config/microNeo/bindings.json`：

```json
{
  "Alt-Enter": "NotePaneOpen"      // 默认值（可省略）
  // 改成 Ctrl-p：
  // "Ctrl-p": "NotePaneOpen"
  // 禁用：
  // "Alt-Enter": "None"
}
```

**已知局限**（接受，不解决）：
- `bindings.json` 只驱动**主编辑器侧**的 `BufBindings`。notePane 内部的 `NotePaneBindings`（`Alt-Enter → NotePaneSend`）是源码硬编码，不读 bindings.json。
- 用户若改主侧键 → 得到"X 键打开、Alt-Enter 发送"的半残状态（不破坏功能，但心智别扭）。
- 不升级到"设置驱动两棵树"的原因：terminal 对修饰键支持参差（Shift-Enter 多数终端检测不到），可换的键本就不多，撑不起额外 setting + post-config 注册机制的代价。
- 真有用户碰到 Alt-Enter 不行的终端，他们改主侧 + 接受 send 仍在 Alt-Enter，不会被堵死。

## 不动的东西

- `internal/action/notepane.go` 的 `Alt-Enter → NotePaneSend`（既有硬编码）
- `internal/action/notepane.go` 的 `Esc → NotePaneClose`（D9 已实施）
- `NotePaneSend` / `notePaneClose` 函数本体
- `notePaneActions` 私有 map、`NotePaneBindings` KeyTree 机制、`notePaneRegisterBinding()` 查询顺序（D9 已就位）
- `~/.config/microNeo/bindings.json`（首次安装保持 `{}`，让默认值生效）

## 验证步骤

1. `make build` 编译（不要直接 `go build`）
2. `go vet ./...` 无新增 warning
3. `grep -rn "Toggle\|Alt-i\|goto done" cmd/micro/ internal/action/` 确认：
   - `Toggle()` 已从 `notepane.go` 删除
   - `micro.go` 不再有 Alt-i 硬编码块 / `goto done`
   - `Alt-i` 不出现在 defaults 文件
4. `grep -n "NotePaneOpen" internal/action/bufpane.go` 确认注册到位
5. 手动测试：
   - 打开任意文件 → 按 `Alt-Enter` → notePane 打开 ✅
   - notePane 内输入文字 → 按 `Alt-Enter` → 发送并关闭 ✅
   - 重开 → 输入文字 → 按 `Esc` → 关闭不发送 ✅（D9 行为）
   - 主编辑器按 `Alt-i` → **无反应** ✅（已退役）
   - 主编辑器按 `Enter` → 仍插入换行 ✅（无回归）
   - 写 `bindings.json`：`"Ctrl-p": "NotePaneOpen"` → 重启 → `Ctrl-p` 开 ✅；`Alt-Enter` 开（默认值仍在）✅

## 风险评估

- **行为变化**：主编辑器 Alt-i 不再开 notePane。老用户若习惯 Alt-i，需要改用 Alt-Enter。
  - **缓解**：Alt-Enter 比 Alt-i 更符合"主操作键"心智；Esc + Alt-Enter 是 TUI 通用约定
- **注册顺序**：`NotePaneOpen` 进 `BufKeyActions` 时，`notePaneMapDefaults` 会把它灌进 `NotePaneBindings` 但被白名单过滤掉——这正是我们想要的（见"init() 注册顺序的微妙点"）。需确保 `NotePaneOpen` **不**进 `allowedNotePaneActions` 白名单（自然状态，无需主动维护）
- **`Toggle()` 删除**：grep 确认无其它调用点；`Open()` / `close()` 已各自承担打开/关闭语义
- **风险等级**：**低**——纯删除（硬编码块 + Toggle）+ 纯新增（一个 action + 一个默认 binding + 一个函数），无既有行为路径的语义改动

## 实施建议

作为独立 commit 提交：

```
feat(notepane): bind Alt-Enter to open notePane in main editor

把 notePane 打开键从主循环硬编码的 Alt-i 改成走标准 binding 机制
的 Alt-Enter。与 notePane 内发送键同键，形成"Alt-Enter = notePane
主操作键"的统一心智。

具体：
- 删 cmd/micro/micro.go 的 Alt-i 硬编码块（含 goto done）
- 删 notepane.go 的 Toggle() 死代码（打开/关闭已由 Open/close 各自承担）
- bufpane.go BufKeyActions 注册 NotePaneOpen（主编辑器侧 action）
- defaults_darwin.go / defaults_other.go 加 "Alt-Enter":"NotePaneOpen"
- notepane.go 实现 notePaneOpen 函数

NotePaneOpen 进 BufKeyActions（主编辑器侧），与 D9 中 NotePaneSend/
NotePaneClose 进 notePaneActions 私有 map（notePane 内部侧）形成清晰
边界。两棵 binding 树物理隔离，Alt-Enter 不会在 notePane 内嵌套打开
（靠白名单先过滤掉 NotePaneOpen）。

Scope：与 D9（Esc 关闭）解耦，可独立 commit。
```

## 设计变更历史

| 版本 | 时间 | 关键变化 |
|---|---|---|
| v1（初稿） | 早于本文 | 把 `Alt-i` 改成 `Alt-Enter`，仍走主事件循环硬编码 |
| v2（用户提出"应该可配置"） | 中期 | 改用标准 binding 机制；默认值仍为 `Alt-i`，引入 `bindings.json` 覆盖路径 |
| v3（用户问"会不会嵌套"） | 后期 | 补"嵌套保护分析"段落 |
| **v4（本文，重写）** | D9 完成后 | 以已敲定决策为基线重写：Alt-Enter 敲定；Alt-i 彻底退役；`Toggle()` 删除；Esc=关闭已在 D9 实施；保护分析合并进正文；新增 `NotePaneOpen` 注册位置说明（主编辑器侧 BufKeyActions vs notePane 内部侧 notePaneActions） |
