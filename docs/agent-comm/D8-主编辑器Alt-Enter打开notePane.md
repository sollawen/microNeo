# D8 — 主编辑器快捷键打开 notePane（可配置化）

> ⚠️ **本文档经过两次设计调整**，请看底部"设计变更历史"了解与原方案的差异。

## 目标

把 notePane 的"打开"快捷键从 **硬编码** 改造为 **micro 标准 binding 机制**，让用户通过 `bindings.json` 修改默认键位。

## 设计原则

- **不写死在源码里**——主事件循环里不应该有 `Alt-xxx` 的特判
- **有合理默认值**——首次安装不读 `bindings.json` 也能用
- **用户可覆盖**——`bindings.json` 优先级高于默认值
- **最少回归**——不破坏现有 notePane 内部行为（`Alt-Enter` → 发送）

## 现状

### 硬编码拦截（应被替换）

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
- 写死在主事件循环里
- 占用主循环 `goto done` 特判路径
- 用户无法在 `bindings.json` 里覆盖
- 与 micro 其余快捷键机制（`defaults_*.go` + `BufKeyActions`）格格不入

### notePane 内部快捷键

`internal/action/notepane.go:108-112`：

```go
func init() {
    NotePaneBindings = NewKeyTree()
    notePaneMapDefaults(DefaultBindings("buffer"))
    // Bind Alt-Enter to NotePaneSend (fallback Alt-s if Alt-Enter not available in terminal)
    notePaneMapBinding("Alt-Enter", "NotePaneSend")
}
```

`Alt-Enter` → `NotePaneSend`（发送并关闭）是 notePane 的白名单 binding，独立于主 buffer binding 树。

### Esc 在 notePane 里的实际行为（已核实）

`Esc` 在 notePane 里的默认 binding（`defaults_*.go:97`）：

```
"Esc": "Escape,Deselect,ClearInfo,RemoveAllMultiCursors,UnhighlightSearch"
```

经白名单过滤后变成：

```
"Escape,Deselect,ClearInfo,RemoveAllMultiCursors"
```

各 action 实际效果：

| Action | 在 notePane 白名单？ | 效果 |
|---|---|---|
| `Escape` | ✅ | 啥也不做（`actions.go:1875-1878` 空壳） |
| `Deselect` | ✅ | 取消选区 |
| `ClearInfo` | ✅ | 清空 infobar 消息 |
| `RemoveAllMultiCursors` | ✅ | 移除多光标 |
| `UnhighlightSearch` | ❌ | 被过滤 |

**结论：`Esc` 当前不能关闭 notePane**，只是清掉 infobar 消息。

## 改动方案

### 改动 1：注册新 action 到 `BufKeyActions`

**文件**：`internal/action/bufpane.go`

在 `BufKeyActions` 字典（`bufpane.go:728` 起）的 "NotePaneSend" 附近添加：

```go
"NotePaneOpen":              notePaneOpen,
```

实现（放在同文件 / `notepane.go` 都行）：

```go
// notePaneOpen 从主编辑器打开 NotePane
// 注册为 BufKeyAction，因此可以走标准 bindings.json 机制
func notePaneOpen(h *BufPane) bool {
    if TheNotePane != nil && !TheNotePane.IsOpen() {
        TheNotePane.Open()
    }
    return true
}
```

**为什么不需要 `Toggle` 版本？** 标准 buffer binding 只在主 buffer 焦点时触发，notePane 打开时事件不会走到 buffer。所以"打开态再按 `Alt-i` 关闭"在标准机制下**自然不可达**——这反而是好事，把"打开"和"关闭"拆成两个明确的语义，由不同按键承担（详见"待定决策"）。

### 改动 2：加默认 binding

**文件**：`internal/action/defaults_darwin.go` 和 `internal/action/defaults_other.go`

在 buffer 段（darwin 第 16 行附近、other 第 19 行附近）添加：

```go
"Alt-i":          "NotePaneOpen",
```

放在 `Alt-g → ToggleKeyMenu` 附近以体现"Alt 修饰键组"。

### 改动 3：删除主循环硬编码

**文件**：`cmd/micro/micro.go:540-548`

整段 `// Alt-i toggles NotePane regardless of open/close state ...` 块删除。

`if event != nil { ... }` 内部的 `goto done` 标签也对应删除。

### 改动 4：notePane 内部 `Alt-Enter` → 发送 是否也开放配置？

详见"待定决策 ❸"。

## 用户改键方式

`~/.config/microNeo/bindings.json`（micro 的标准 binding 加载机制见 `bindings.go:40-79`）：

```json
{
  "Alt-i": "NotePaneOpen"  // 默认值（和源码 defaults 一样，可省略）
  // 改成 Alt-Enter：
  // "Alt-Enter": "NotePaneOpen"
  // 改成 Ctrl-p：
  // "Ctrl-p": "NotePaneOpen"
  // 禁用：
  // "Alt-i": "None"
}
```

加载顺序（`bindings.go:55-79`）：

1. 注册所有 `DefaultBindings(p)` 到 `Binder`
2. 读 `bindings.json`，**覆盖** 默认值
3. 用户配置永远赢

所以"可配置"是天然支持，不用写额外代码。

## 待定决策（需要用户拍板）

### ❶ 主编辑器默认键

| 选项 | 含义 |
|---|---|
| A. `Alt-i`（保留现状） | 最小变动，已在文档里多次出现，老用户无感 |
| B. `Alt-Enter`（按最早 D8 需求） | 与 notePane 内的发送键区分清楚 |
| C. 其他 | 用户提议 |

### ❸ notePane 内部的 `Alt-Enter` → 发送 是否也开放配置

| 选项 | 含义 |
|---|---|
| A. 继续硬编码 | `notepane.go:111` 不动，最小改动 |
| B. 也走 `bindings.json` | 需要在 `notepane.go` 改造 `init()` 从用户配置读 key——较大改动 |
| C. 顺带换键 | 比如改成 `Esc` 发送（违反常识，不推荐） |

### ⤴️ `Esc` 关闭 notePane 移交给独立计划

`Esc` 关闭 notePane（“不发直接关”）已抽出为独立计划 **[D9 — Esc 在 notePane 里直接关闭（不发送）](./D9-Esc直接关闭notePane.md)**，与 D8 范围解耦，可独立 commit。

## 不动的东西

- `internal/action/notepane.go` 中 `Alt-Enter → NotePaneSend` 的硬编码（除非 ❸ 选 B）
- `NotePaneSend` 函数本体（`notepane.go:299-381`）
- notePane 内部的 `NotePaneBindings` KeyTree 机制
- `~/.config/microNeo/bindings.json`（首次安装时保持 `{}`，让默认值生效）

## 验证步骤

1. `make build` 编译
2. 首次启动（`bindings.json = {}`）→ 按 `Alt-i`（默认）→ notePane 打开 ✅
3. 写 `bindings.json`：覆盖为 `"Alt-Enter": "NotePaneOpen"` → 重启 → 按 `Alt-Enter` → notePane 打开 ✅，按 `Alt-i` 无效（已解绑）
4. 写 `bindings.json`：`"Alt-i": "None"` → 重启 → 按 `Alt-i` 无效 ✅
5. notePane 内部：`Alt-Enter` 仍然发送并关闭 ✅
6. 主编辑器 `Enter` 仍插入换行 ✅（无回归）

## 嵌套保护分析（用户提问补记）

> "在 notePane 里按热键会不会再开一个 notePane？"

**答案：不会。三层保护。**

### 1. 事件路由（主循环层级）

`cmd/micro/micro.go` 主事件循环改完后，notePane 打开态下事件**直接**走：

```go
} else if action.TheNotePane != nil && action.TheNotePane.IsOpen() {
    action.TheNotePane.HandleEvent(event)   // ← 不到主 buffer
} else {
    action.Tabs.HandleEvent(event)
}
```

主 buffer 的 `BufKeyActions` 表（注册了 `NotePaneOpen`）**完全不会被查**。

### 2. 白名单过滤（notePane 层级）

即使事件路由改了，`NotePaneBindings`（`notepane.go:108`）只接受白名单内的 action。`notePaneOpen` / `NotePaneOpen` 不在白名单（`allowedNotePaneActions`），所以：

- `notePaneMapDefaults(DefaultBindings("buffer"))` 把 buffer defaults 灌进来时
- `Alt-i → NotePaneOpen` 经过 `filterActions()` 过滤后**整条 binding 丢弃**（`len(filteredActions) == 0` → return）
- `NotePaneBindings` 里**根本不存在** `Alt-i` 这条 binding

### 3. 单例 + 守卫

`TheNotePane` 全局唯一（`notepane.go:30-31`、`globals.go:16`），没"嵌套实例"概念。

`notePaneOpen` 实现里加守卫：

```go
func notePaneOpen(h *BufPane) bool {
    if TheNotePane != nil && !TheNotePane.IsOpen() {
        TheNotePane.Open()
    }
    return true
}
```

### 各场景汇总

| 场景 | 触发路径 | 结果 |
|---|---|---|
| 主编辑器按 `Alt-i` | 主 buffer binding → `NotePaneOpen` | 打开 ✅ |
| notePane 内按 `Alt-i` | 事件不进主 buffer；白名单过滤掉 | 无操作 ✓ |
| notePane 内按 `Alt-Enter` | 走 `NotePaneBindings → NotePaneSend` | 发送 + 关闭 ✅ |

### ✅ 真问题已独立修复

`Open()` 非幂等问题 + `resize` 误调 `open()` 导致 buffer 被销毁的隐患，已在 commit `e926a640`（refactor: extract reposition(), decouple resize from buffer reset）中通过结构改造彻底解决：抽出 `reposition()`，resize 改调 `reposition()`，buffer 永远只在 `open()` 关闭→开转换时重建。

D8 不需再处理这个。

## 设计变更历史

| 版本 | 时间 | 关键变化 |
|---|---|---|
| v1（初稿） | 早于本文 | 把 `Alt-i` 改成 `Alt-Enter`，仍走主事件循环硬编码 |
| v2 | 用户提出"应该可配置" | 改用标准 binding 机制；默认值仍为 `Alt-i`（未确定），可被 `bindings.json` 覆盖 |
| v3 | 用户问"会不会嵌套" | 补"嵌套保护分析"段落；发现 `Open()` 非幂等，记为独立修复项 |
