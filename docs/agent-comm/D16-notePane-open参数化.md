# D16：notePaneOpen 把 receiver 显式传给 open

> **状态**：方案设计（待实施）
>
> **改动面**：仅 `internal/action/notepane.go`（1 个函数重写 + 1 个方法签名扩展），约 5–8 行实质改动。
>
> **上游依据**：用户提出的伪代码（见 §一）；本 D 是对 D8（Alt-Enter 打开）/ D12（多 receiver 选择）流程的接口清洁，不改任何外部可观测行为。
>
> **依赖 / 兼容**：
> - 与 [`D15-notePane切换receiver.md`](./D15-notePane切换receiver.md)（待实施）正交：D15 是「notePane 已开态下切 receiver」，不调 `open()`（不重建 buffer）；本 D 只改 `open()` 的入口，不影响 D15。
> - `NotePaneSend` / `Display` / D15 都读 `n.selectedReceiver` 字段——本 D **保留该字段**，只是把赋值时机从「open 调用之前」收敛到「open 内部第一行」。

---

## 一、用户需求（伪代码）

```
主编辑器内，用户按 alt-enter {
    receivers = discover()
    if no receiver then {
        infoPane 提示用户
        return
    }
    if 1 recievers {
        receiver = 这个唯一的 receiver
    } else {
        selectPane.open(receivers)
        reciever = 用户选择的那个 receiver
    }

    notePane.open(receiver)   // 直接把 receiver 作为参数传给 notePane
}
```

用户额外说明：**"现有代码里，有记忆上次选择的 receiver 是谁。这个功能要保留。"**（缓存命中跳过 SelectPane 的优化，伪代码里没写但必须保留）

---

## 二、现状 vs 目标

### 2.1 现状（`notepane.go:134-202`）

```
notePaneOpen:
    discover → 错误/无 receiver 提示
    case 1：    n.selectedReceiver = receivers[0]; n.open()
    case 2+ 命中：n.open()                           ← 复用缓存
    case 2+ 未命中：
        SelectPane.Open(..., func(s) {
            Esc:  n.selectedReceiver = eabp.RegFile{}    ← 清零缓存
            Enter: n.selectedReceiver = r; n.open()
        })
```

**问题**：`selectedReceiver` 字段被「外部 3 处分散赋值」（case 1 / 命中复用 / SelectPane Enter 回调），赋值点散落、依赖隐式状态。`open()` 自己不读 receiver 入参，靠"调用方提前 set 好"——这是用户希望消除的隐式协议。

### 2.2 目标

```
notePaneOpen:
    discover → 错误/无 receiver 提示
    case 1：    n.open(receivers[0])
    case 2+ 命中：n.open(n.selectedReceiver)        ← 复用缓存
    case 2+ 未命中：
        SelectPane.Open(..., func(s) {
            Esc:  n.selectedReceiver = eabp.RegFile{}    ← 清零缓存（保留决策 14）
            Enter: n.open(r)
        })

open(receiver):
    n.selectedReceiver = receiver                  ← 集中赋值（唯一入口）
    ...其余逻辑不变...
```

### 2.3 核心差别一句话

**赋值时机**：`selectedReceiver` 的赋值从「`open` 之前、调用方 3 处分散 set」收敛到「`open` 内部第一行、唯一入口」。`open` 由无参变成接收 receiver，receiver 状态成为 open 的显式入参而非隐式前驱。

**字段语义不变**：`selectedReceiver` 仍是双重职责——「本次发送目标」+「下次缓存」。区别只是谁负责 set：原来调用方 set，现在 open 自己 set。

---

## 三、改动清单（逐行）

全部在 `internal/action/notepane.go`。

### 3.1 `open` 方法签名扩展（行 368 附近）

**改前**：
```go
func (n *NotePane) open() {
    if n.isOpen {
        return
    }
    pane := MainTab().CurPane()
    if pane == nil {
        return
    }
    ...
```

**改后**：
```go
// open 接收 receiver 作为显式入参，并在内部第一行写入 selectedReceiver。
// 这样 receiver 状态的赋值点收敛到唯一入口（D16），消除"调用方提前 set"的隐式协议。
// selectedReceiver 字段语义不变：本次发送目标 + 下次缓存。
func (n *NotePane) open(receiver eabp.RegFile) {
    if n.isOpen {
        return
    }
    n.selectedReceiver = receiver   // ← 新增：集中赋值
    pane := MainTab().CurPane()
    if pane == nil {
        return
    }
    ...
```

**改动**：
- 签名加 `receiver eabp.RegFile` 参数
- `isOpen` 守卫之后、`pane := MainTab()...` 之前插入一行 `n.selectedReceiver = receiver`
- 方法文档注释更新（说明 receiver 入参与字段关系）

**守卫位置说明**：`n.selectedReceiver = receiver` 放在 `isOpen` 守卫**之后**——已开态下重复调用 `open` 是 no-op（不覆盖现有 receiver）。这与现有"已开态幂等"语义一致。

### 3.2 `notePaneOpen` 函数体改写（行 134-202）

**改动 1 — case 1 分支（行 155-158）**：

```go
// 改前
if len(receivers) == 1 {
    n.selectedReceiver = receivers[0]
    n.open()
    return true
}

// 改后
if len(receivers) == 1 {
    n.open(receivers[0])
    return true
}
```

**改动 2 — 缓存命中分支（行 161-168）**：

```go
// 改前
if n.selectedReceiver.Socket != "" {
    for _, r := range receivers {
        if r.Socket == n.selectedReceiver.Socket {
            n.open()  // 命中，复用缓存
            return true
        }
    }
}

// 改后（缓存字段在读，open 写；语义不变）
if n.selectedReceiver.Socket != "" {
    for _, r := range receivers {
        if r.Socket == n.selectedReceiver.Socket {
            n.open(n.selectedReceiver)  // 命中，复用缓存
            return true
        }
    }
}
```

> **保留缓存优化**（用户明确要求）：≥2 个 receiver 时先查缓存命中，命中则跳过 SelectPane。这部分逻辑不在用户伪代码里，但必须保留。

**改动 3 — SelectPane 回调 Enter 分支（行 188-196）**：

```go
// 改前
NewSelectPane().Open(names, "Receiver", Pos{X: ax, Y: ay}, tcell.Style{}, func(s *string) {
    if s == nil {
        // Esc：清零缓存（走到此分支时缓存已失效，决策 14）
        n.selectedReceiver = eabp.RegFile{}
        InfoBar.Message("✗ 已取消")
        return
    }
    // Enter：找到 name 对应的 RegFile
    for _, r := range receivers {
        if r.Name == *s {
            n.selectedReceiver = r
            n.open()
            return
        }
    }
    // 理论上不会到这（SelectPane 的 items 来自 receivers），防御性
    InfoBar.Message("✗ internal: selected name not in receivers")
})

// 改后（Esc 清缓存逻辑保留；Enter 改成 open(r)）
NewSelectPane().Open(names, "Receiver", Pos{X: ax, Y: ay}, tcell.Style{}, func(s *string) {
    if s == nil {
        // Esc：清零缓存（走到此分支时缓存已失效，决策 14）
        n.selectedReceiver = eabp.RegFile{}
        InfoBar.Message("✗ 已取消")
        return
    }
    // Enter：找到 name 对应的 RegFile，作为参数传给 open
    for _, r := range receivers {
        if r.Name == *s {
            n.open(r)
            return
        }
    }
    // 理论上不会到这（SelectPane 的 items 来自 receivers），防御性
    InfoBar.Message("✗ internal: selected name not in receivers")
})
```

### 3.3 注释微调

`notePaneOpen` 函数顶部注释（行 131-133）更新一句，反映 receiver 显式参数化：

```go
// notePaneOpen 从主编辑器打开 NotePane。
// 注册为主编辑器的 BufKeyAction，可走标准 bindings.json 机制覆盖默认键位。
// 守卫：notePane 已开态下重复触发是 no-op。
// 流程：discover → (1 个 / 缓存命中 / SelectPane 弹窗) → n.open(receiver)（D16：receiver 作为显式参数）。
```

---

## 四、不变项（核对清单）

| 项 | 现状 | 是否改 | 说明 |
|---|---|---|---|
| discover 出错提示 | `InfoBar.Message("✗ discover error: ...")` | 不改 | 伪代码"infoPane 提示"即 InfoBar（命令栏）；健壮性保留 |
| 无 receiver 提示 | `InfoBar.Message("✗ no receiver found")` | 不改 | 同上 |
| SelectPane 锚点计算（D14） | `ax := view.X + 2; ay := lowestRow + 1` | 不改 | D14 已定锚点语义 |
| SelectPane 接口签名 | `Open(items, title, anchor, frameColor, onSelect)` | 不改 | D13/FloatFrame 已定 |
| **缓存命中跳过 SelectPane** | `if n.selectedReceiver.Socket != ""` 查表 | **不改**（用户明确要求保留） | 伪代码没写，但用户口头强调"记忆功能要保留" |
| Esc 清零缓存 | `n.selectedReceiver = eabp.RegFile{}` | 不改 | 决策 14；走到 SelectPane 分支即说明旧缓存已失效 |
| `selectedReceiver` 字段 | 双重职责：发送目标 + 缓存 | 不改（字段保留） | 赋值点从外部收敛到 open 内部 |
| `NotePaneSend` 读 `n.selectedReceiver` | 行 469 | 不改 | 字段还在，open 后必然已赋值 |
| `Display` 嵌 receiver 名字 | 行 643 `name := n.selectedReceiver.Name` | 不改 | 同上 |
| D15 `NotePaneSwitchReceiver`（待实施） | 仅改 `n.selectedReceiver` 不调 open | 不冲突 | D15 不调 open（避免重建 buffer），不受本 D 影响 |

---

## 五、与伪代码的偏差说明

用户伪代码是流程意图，落到现有代码有两处必要偏差：

### 5.1 ≥2 个 receiver 时仍先查缓存（伪代码"else"分支）

**伪代码**：`else { selectPane.open(receivers); receiver = 用户选择的 }`——看起来 ≥2 时必弹 SelectPane。

**实际**：≥2 时先查 `selectedReceiver.Socket` 是否还在 receivers 列表里，命中则跳过 SelectPane 直接用缓存。

**理由**：用户明确说"记忆上次选择的 receiver 是谁。这个功能要保留"。这是对伪代码的口头补充，优先级高于伪代码字面。

### 5.2 Esc 时清零缓存

**伪代码**：未提及。

**实际**：Esc 时 `n.selectedReceiver = eabp.RegFile{}`（清零）。

**理由**：保留决策 14。走到 SelectPane 分支说明旧缓存已失效（socket 不在 receivers 列表里）；Esc 表示用户放弃选择，清掉失效缓存避免下次还试一次。

> 如希望改为「Esc 时保留缓存」（让下次还能命中），可在实施前提出。本 D 默认沿用决策 14。

---

## 六、验收

| # | 场景 | 期望 |
|---|------|------|
| 1 | 1 个 receiver，按 Alt-Enter | notePane 打开，上边框嵌该 receiver 名字，发送能送达 |
| 2 | ≥2 个 receiver，首次按 Alt-Enter | 弹 SelectPane，Enter 选定后 notePane 打开，发送能送达选定 receiver |
| 3 | ≥2 个 receiver，二次按 Alt-Enter（缓存命中） | 不弹 SelectPane，直接复用上次 receiver 打开 notePane |
| 4 | ≥2 个 receiver，SelectPane 里按 Esc | InfoBar 显示"✗ 已取消"，notePane 不打开；下次再触发时因缓存被清零会重新弹 SelectPane |
| 5 | 0 个 receiver，按 Alt-Enter | InfoBar 显示"✗ no receiver found"，notePane 不打开 |
| 6 | discover 出错（如权限问题） | InfoBar 显示"✗ discover error: ..."，notePane 不打开 |
| 7 | notePane 已开态下再按 Alt-Enter | no-op（幂等，沿用现有守卫） |

**编译**：`make build-quick` 0 错 0 警；最终 `make build` 通过。

---

## 七、影响面与回滚

### 7.1 影响面

- **外部可观测行为**：零变化（receiver 选择流程、缓存命中、Esc 行为全部保留）。
- **内部结构**：`open` 由无参变 `open(receiver)`，赋值点收敛。`NotePaneSend` / `Display` / D15 都不受影响（读 `n.selectedReceiver` 字段）。
- **micro 原生代码**：零侵入（全部改在 `notepane.go`，microNeo 自己的 `*_md.go` 边界外的专属文件）。

### 7.2 回滚

改动集中在 `notepane.go` 一个文件、两个函数。回滚 = 撤销对该文件的修改。无中间状态风险（不像 D13/D14 那种跨文件多阶段的改动）。

---

## 八、与历史 D 系列的关系

| 文档 | 关系 |
|------|------|
| [D8（Alt-Enter 打开）](./说明-notepane.md) | 本 D 是 D8 流程的接口精化，不改 D8 的交互 |
| [D12（多 receiver 选择）](./D12-多receiver选择.md) | 本 D 保留 D12 的所有分支（case 1 / 缓存命中 / SelectPane 弹窗），只改赋值时机 |
| [D13（SelectPane 设计）](./D13-SelectPane设计.md) | 不涉及——SelectPane 接口不变 |
| [D14（notePane-select 锚点）](./D14-notePane-select锚点.md) | 不涉及——锚点计算逻辑保留 |
| [D15（notePane 已开态切换 receiver）](./D15-notePane切换receiver.md) | 正交——D15 不调 `open()`（只改 `selectedReceiver` 字段不重建 buffer），本 D 改 `open` 签名不影响 D15 的实施路径 |
| [说明-notepane.md](./说明-notepane.md) | 收尾时同步：§四.2 `open()` 代码块加 receiver 参数；§三 `selectedReceiver` 字段注释更新（赋值入口收敛） |
