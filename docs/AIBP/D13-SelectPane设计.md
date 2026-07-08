# D13 — SelectPane 设计（通用列表选择浮窗）

> **状态**：已实施完成（D12 已集成）
>
> **范围**：一个**通用**的列表选择浮窗 `SelectPane`——`Open(items, title, x, y, onSelect)`，从列表里选一个 string。当前用于 D12 的多 receiver 选择，未来可复用于选文件、选配色等任何"从列表选一个"的场景。
>
> **实施偏差回填**（以代码为准）：
> - ① title **不反色**（原文 §3.4 写 "Reverse 突出"）
> - ② 上下键 **wrap-around**（原文 §3.5 写 "到边停"）
> - ③ **位置算法**从"固定左下角向上"演进为"锚点自适应"（Open 签名增加 `x, y *int`；详见 §3.2 与代码 `selectpane.go:44`）
> - ④ D13 测试阶段的 Alt-I trigger 与 `selectTestOpen` 已在集成时清理（仅保留 notepane 一处调用）
> - ⑤ 另有几处微改进（位置留 1 行间隔、宽度同时考虑 title、测试 title 用长串）未逐字回填
>
> **不做**：浮窗框架（Float interface / 栈 / dispatcher）、其它 pane 类型（FilePicker / ColorPicker 不是新 pane，是 SelectPane 的不同调用方）。
>
> **不改**：AIBP 协议、接收端、micro 原生代码（`cmd/micro/micro.go` 不动）。

---

## 一、动机

### 1.1 直接需求：D12 需要"选 receiver"

D12 的 notePane 多 receiver 流程：Discover 到 2+ receiver 时，需要弹一个浮窗让用户选一个。

### 1.2 SelectPane 是通用的（不是只给 notePane 用）

这个"展示列表 → 上下键移动 → Enter 确认 / Esc 取消 → 回调返回结果"的能力**非常通用**。同一个 SelectPane 组件可以服务很多场景：

| 场景 | items | title | 锚点 (x, y) | onSelect 做什么 |
|------|-------|-------|--------------|-----------------|
| **选 receiver**（v1，D12） | `["pi-Alpha", "pi-Bravo", ...]` | `"Receiver"` | `nil, nil`（走旧位置） | 把选中的 receiver 名字存进 notePane 上下文 |
| **选文件**（未来） | `["a.md", "b.go", ...]` | `"File"` | 光标位置 / 调用方自定义 | 打开对应文件 |
| **选配色**（未来） | `["solarized", "gruvbox", ...]` | `"Color"` | 同上 | 切换配色 |
| **选命令**（未来） | `["> open", "> save", ...]` | `"Command"` | 同上 | 执行对应命令 |

**SelectPane 自己不知道在选什么**——它只负责"展示 items + 返回选中的 string"，语义由调用方通过 title 和 onSelect 赋予。

> **注意区分**：我们要的是"**通用 selectPane**"（一个组件复用于不同选择场景），**不是**"通用 floating pane"（Float interface / 浮窗栈 / 不同 pane 类型的框架）。FilePicker / ColorPicker **不是**新 pane 类型——它们就是 SelectPane 拿不同参数调用。

### 1.3 micro 原生没有这个组件

micro 原生只有：
- `InfoBar.Prompt`（单行输入 + autocomplete，不是列表）
- `InfoBar.YNPrompt`（yes/no 二选一）
- 没有列表选择 / picker / chooser

所以需要自己做。

---

## 二、设计目标

- **通用**：一个 SelectPane 服务所有"从列表选一个"的场景，调用方传 items + title + onSelect
- **v1 极简**：只支持必要的功能（位置、尺寸、键位、边框、回调），其余进 §3.9 未来扩展
- **零原生侵入**：不改 `cmd/micro/micro.go`

---

## 三、SelectPane 设计

### 3.0 定位

`SelectPane` 是**通用列表选择浮窗**——`Open(items, title, x, y, onSelect)`，从列表里选一个 string。

- **独立文件** `internal/action/selectpane.go`，与 notePane.go 平级
- **单例**：全局 `TheSelectPane`，同一时刻只允许一个 SelectPane 打开（v1：没有"多个堆叠"概念）
- **不抽象**：不做 Float interface、不做浮窗栈、不预设其它 pane 类型。SelectPane 是唯一的浮窗组件，**它的通用性体现在参数（items/title/onSelect），不体现在继承/接口**
- **v1 极简**：必要功能（位置、尺寸、键位、边框、回调），其它进 §3.9 未来扩展

### 3.1 API 契约

```go
// SelectPane 通用列表选择浮窗
type SelectPane struct {
    items    []string
    selected int            // 当前高亮索引
    title    string         // 上边框标签（如 "Receiver" / "File" / "Color"）
    onSelect func(*string)  // nil=取消（Esc），&items[selected]=确认（Enter）
    // ... 显示相关字段（见 §3.6）
}

// TheSelectPane 全局单例。
// 同一时刻只允许一个 SelectPane 打开（v1：没有"堆叠"概念）。
// Open 时如果有旧的会先关掉（防御性）。
var TheSelectPane *SelectPane

// Open 打开选择浮窗。
//   items    候选项文本（已渲染好，不做转义/截断，由调用方保证）
//   title    上边框标签，告诉用户"现在在选什么"（如 "Receiver" / "File" / "Color"）
//   x, y     锚点（屏坐标）。均为 nil 时走旧逻辑（左下角、向上展开贴 statusLine）
//            两者都非 nil 时按 §3.2 自适应展开
//   onSelect 回调：
//            用户按 Enter → onSelect(&items[selected])，浮窗关闭
//            用户按 Esc   → onSelect(nil)，浮窗关闭
//            屏幕太小放不下（见 §3.2.3）→ onSelect(nil)，浮窗不 open
func (s *SelectPane) Open(items []string, title string, x, y *int, onSelect func(selected *string))

// Close 关闭（外部触发，比如主编辑器抢焦点）。
func (s *SelectPane) Close()

// IsOpen 返回是否处于打开状态。
func (s *SelectPane) IsOpen() bool

// HandleEvent 处理 tcell 事件（由 D12 的事件路由转发；D13 不设计此层）。
func (s *SelectPane) HandleEvent(event tcell.Event)

// Display 渲染浮窗（由 D12 的事件路由转发；D13 不设计此层）。
func (s *SelectPane) Display()
```

**调用示例**：
```go
// D12：选 receiver（现阶段传 nil 走旧位置）
TheSelectPane.Open(
    []string{"pi-Alpha", "pi-Bravo", "pi-Charlie"},
    "Receiver",
    nil, nil,
    func(s *string) {
        if s == nil { /* 用户取消 */ return }
        // 用 *s 打开 notePane...
    },
)

// 未来：选文件（传光标位置作为锚点）
TheSelectPane.Open(filenames, "File", &cx, &cy, func(s *string) { ... })

// 未来：选配色（传光标位置作为锚点）
TheSelectPane.Open(colorNames, "Color", &cx, &cy, func(s *string) { ... })
```

> **调用方契约**：`(x, y)` 必须**都为 nil**（走旧行为）或**都非 nil**（走自适应）——**禁止半 nil**。半 nil 会静默回退到旧位置，是隐性陷阱。

**设计要点**：
- **返回 `*string`**：通过 callback 异步返回
  - `nil` = 用户按 Esc 取消（**没选任何东西**）
  - `&"xxx"` = 用户按 Enter 确认（**选中了第 N 项**）
  - 用 `*string` 而非 `string` + 哨兵值（如 `""`），符合 Go 习惯（"零值即无"），更类型安全
- **title 必填**：调用方必须告诉用户"现在在选什么"，避免用户看到一个无标题的列表发懵

### 3.2 位置（锚点自适应）

`Open()` 接受锚点 `(x, y)` 参数，调用方传屏幕坐标的任意点，SelectPane 自动决定向哪边展开。`(x, y)` 都是 nil 时走旧逻辑：左下角、向上展开贴 statusLine。

#### 3.2.1 锚点语义

`(x, y)` 是**锚点**（不是最终左上角）。最终左上角由展开方向决定：

| 展开方向 | 锚点角色 | 计算 |
|----------|----------|------|
| 向下 | 浮窗顶边 | `s.y = ay` |
| 向上 | 浮窗底边 | `s.y = ay - paneH + 1` |
| 向右 | 浮窗左边 | `s.x = ax` |
| 向左 | 浮窗右边 | `s.x = ax - paneW + 1` |

> "锚点 vs 左上角"选择锚点：左上角与"自适应方向"语义互斥——传左上角就锁死了方向。

#### 3.2.2 自适应展开算法

**y 与 x 各自独立**判定，互不影响——可组合出 4 种象限方向。每个方向三段式：

1. 主方向有足够空间 → 按主方向展开
2. 反方向有足够空间 → 按反方向展开
3. 都不够 → 选空间更大的一侧作为偏好方向 + clamp 到合法范围

记号（与代码 `selectpane.go:80-122` 一致）：

| 名称 | 含义 |
|------|------|
| `w, h` | `screen.Screen.Size()` |
| `statusLineY` | `h - iOffset - 1` |
| `bottomLimit` | `statusLineY - 1`（向下展开的底边最大行 = statusLine 上方 1 row） |
| `paneW` | `s.width` |
| `paneH` | `s.height + 2`（含上下边框） |

**y 方向**（向下优先）：

```go
downSpace := bottomLimit - ay + 1  // [ay, bottomLimit] 的行数
upSpace   := ay + 1                // [0, ay] 的行数
switch {
case downSpace >= paneH:  s.y = ay                       // 向下
case upSpace   >= paneH:  s.y = ay - paneH + 1           // 向上
case downSpace >= upSpace: s.y = ay                      // 兜底:偏向下
default:                  s.y = ay - paneH + 1           // 兜底:偏向上
}
// clamp 到 [0, bottomLimit - paneH + 1]
```

**x 方向**（向右优先，与 y 完全对称）：

```go
rightSpace := w - ax                // [ax, w-1] 的列数
leftSpace  := ax + 1                // [0, ax] 的列数
switch {
case rightSpace >= paneW:  s.x = ax                       // 向右
case leftSpace  >= paneW:  s.x = ax - paneW + 1           // 向左
case rightSpace >= leftSpace: s.x = ax                    // 兜底:偏向右
default:                    s.x = ax - paneW + 1          // 兜底:偏向左
}
// clamp 到 [0, w - paneW]
```

#### 3.2.3 强约束与失败路径

**底边界**：向下展开时浮窗底边（`s.y + paneH - 1`）最大等于 `bottomLimit = statusLineY - 1`，即 statusLine **上方那一行**——不盖 statusLine。

**屏幕完全放不下**（`paneH > bottomLimit+1` 或 `paneW > w`）：
- 不修改任何状态（`isOpen` 保持 false、`x/y/width/height` 不赋值）
- 不渲染、不 `screen.Redraw()`
- 直接回调 `onSelect(nil)` 后返回
- 与 Esc 取消**完全同语义**：调用方看到的都是"用户啥也没选"
- 不在 InfoBar 写"屏幕太小"提示——提示责任完全转移给调用方

> **副作用提示**：因为走的是和 Esc 完全同的 `onSelect(nil)` 路径，notePane 现有的 `InfoBar.Message("✗ 已取消")` 会自然复用。用户看到的不是"屏幕太小"而是"已取消"——这是有意接受的语义统一（屏幕太小 = 用户未选中）。

#### 3.2.4 nil 分支（向后兼容）

`(x, y)` 任一为 nil → 走 D13 原版写死位置（**左下角、向上展开贴 statusLine**）：

```go
s.x = 0
s.y = statusLineY - s.height - 2
```

> **调用方契约**：要么都 nil（走旧行为）、要么都非 nil（走自适应）——**禁止半 nil**。半 nil 不会编译报错，会静默回退到旧位置，是隐性陷阱。混合入参（一 nil 一非 nil）语义模糊，防御性按"任一为 nil 走旧逻辑"处理。

#### 3.2.5 当前唯一调用点

notePane 的 D12 多 receiver 选择（`internal/action/notepane.go:176`）传 `nil, nil`，保留旧位置（左下角）。后续若需要锚点跟随光标/触发点，传具体 `(x, y)` 即可。

### 3.3 尺寸

**宽度**（同时满足内容区和 title 显示，取较大值）：
```go
// 内容区：边框(2) + 左右 padding(2) + maxItemLen = maxItemLen + 4
// title 区：边框(2) + ──(2) + title + ──(2) = len(title) + 6
width := max(maxItemLen + 4, len(title) + 6)
```

> 为什么同时考虑 title：避免长 title 被自己的边框截断（如 `"TestLongTitleForSelectPane"`）。

**高度**（写死 `maxHeight`）：
```go
h := len(items)
if h > maxHeight { h = maxHeight } // v1 写死常量，比如 10
```

> maxHeight 写死是为了 v1 简洁，超出后**不滚动**——列表项通常 < 10，超出场景罕见。**未来扩展**时再支持滚动 + 模糊搜索。

### 3.4 边框

**复用 notePane 风格**（`internal/action/notepane.go:548-589`）：

```
┌──Receiver─────────┐     ← 调用方传 title="Receiver"
│ item 1            │
│ item 2            │
│ ...               │
│ item N            │
└───────────────────┘
```

- 转角：`┌` `┐` `└` `┘`
- 边：`─` `│`
- **上边框嵌入 `──{title}──`**（双连字符包裹调用方传入的 title）：
  - 紧接 `┌` 之后写 `──` + title + `──`，后面续 `─` 填满到 `┐`
  - title 部分**不反色**，与边框 `─` 同样式（`config.DefStyle`）——有意决策，视觉更干净（D12 notePane 边框名字跟随同一样式）
  - 告诉用户"现在在选什么"——`Receiver` / `File` / `Color` / ...
  - **不同调用方传不同 title**，SelectPane 自己不知道 title 的语义

绘制流程参考 notePane 的 `Display()`：先 `Clear` 整个矩形 → 画 4 角 + 上下边 + 左右边 → 写每行内容（**当前选中项**用 `Reverse` 样式高亮——注意只有选中项反色，title 不反色）→ 上边框中间嵌入 `{title}` 标签。

### 3.5 键位

| 键 | 行为 |
|----|------|
| `↑` | 上一项（**wrap-around**：到顶后跳到末项） |
| `↓` | 下一项（**wrap-around**：到底后跳回首项） |
| `Enter` | 确认选择 → callback(&items[selected]) → Close |
| `Esc` | 取消 → callback(nil) → Close |
| 其它键 | **完全吞掉**（不传给主编辑器；modal 语义由 D12 决定如何实现） |

> v1 不绑任何 Ctrl 组合（`Ctrl-p/n/c` 都不支持），保持键表最小。**未来扩展**可加 vim 习惯（j/k/g/G/Home/End）和 Ctrl 组合。

### 3.6 状态 struct

```go
type SelectPane struct {
    items    []string
    selected int            // 当前高亮索引
    title    string         // 上边框标签
    onSelect func(*string)  // nil=取消（Esc），&items[selected]=确认（Enter）

    x, y       int          // 浮层左上角（屏坐标）
    width      int          // 含边框
    height     int          // 内容高度（不含边框），<= maxHeight
    maxHeight  int          // v1 写死常量（比如 10）

    isOpen     bool
}
```

### 3.7 渲染

- 每次按键或状态变化 → 主循环下一帧自然重绘
- 关闭时**不**主动清屏——主编辑器下一帧会覆盖该区域（和 notePane 一致）
- 当前选中项用 `config.DefStyle.Reverse(true)` 高亮

### 3.8 事件接入（BufPane forwarding，D12 已采用）

`SelectPane` 自身不直接挂接到 micro 主循环，而是由 `BufPane` 在事件入口和绘制出口做薄转发：

**`BufPane.HandleEvent` 顶部**（`bufpane.go:433-438`）：
```go
// microNeo D12 集成：多 receiver 选择（forward events to SelectPane when open）
if TheSelectPane != nil && TheSelectPane.IsOpen() {
    TheSelectPane.HandleEvent(event)
    return
}
```

**`BufPane.Display` 末尾**（`bufpane.go:541-548`）：
```go
// Display 重写嵌入的 BWindow.Display，在主编辑器画完后叠加 SelectPane。
func (h *BufPane) Display() {
    h.BWindow.Display()
    if TheSelectPane != nil && TheSelectPane.IsOpen() {
        TheSelectPane.Display()
    }
}
```

**为什么是 BufPane 不是 notePane**：
- 选择器常被"主编辑器按某键"触发，forwarding 在 BufPane 是最一致的位置
- notePane 槽位意味着 "SelectPane 与 notePane 强耦合"——这跟 §3.0 设计原则（SelectPane 是独立组件）冲突

**侵入范围**：BufPane 是 micro 原生代码，但改动仅 2 个 if（HandleEvent 顶部 + Display 末尾），不动原有逻辑。

**状态演进**：
- D13 阶段：曾标为 "test scaffolding"，用于挂 Alt-I 测试 trigger
- D12 集成：保留 BufPane forwarding 作为**生产接入方式**，删除 Alt-I 测试 trigger（与 §1 实施偏差 ④ 一致）
- 当前：forwarding 是 SelectPane 的唯一接入点；D12 notePane 多 receiver 选择即走此路径

### 3.9 未来扩展（明确不做）

- 滚动（items 超过 maxHeight）
- 模糊搜索（输入字符过滤）
- 鼠标点击
- 其它 vim 风格键位（j/k/g/G/Home/End）和 Ctrl 组合
- 自定义位置（不贴 statusLine）
- 自定义 maxHeight / 颜色 / 边框字符
- 多选（checkbox 模式）
- 异步加载 items（开 SelectPane 时 items 还在加载）
- **抽象成浮窗框架**（Float interface / 栈 / dispatcher）——SelectPane 的通用性靠参数（items/title/onSelect）就够，不需要抽象层

这些都等 v1 用稳了再考虑。**v1 只做最简单的版本**。

---

## 四、决策项（已拍板）

| # | 决策 | 拍板结果 | 理由 |
|---|------|----------|------|
| 1 | SelectPane 是什么 | **通用列表选择浮窗**（一个组件，复用于不同选择场景） | items/title/onSelect 参数化即可通用；不需要 Float interface 或栈 |
| 2 | SelectPane 文件位置 | **新文件 `internal/action/selectpane.go`** | 与 notePane.go 平级，不互相包含 |
| 3 | 实例数量 | **单例 `TheSelectPane`**（不堆叠/嵌套） | v1 流程永远只有 0 或 1 个浮窗打开，不需要栈 |
| 4 | 上边框标签 | **参数化** `title`（调用方传 "Receiver" / "File" / "Color" ...） | SelectPane 通用，标签必须随调用方变化 |
| 5 | SelectPane 尺寸 | 宽度 = `max(maxItemLen + 4, len(title) + 6)`（内容区与 title 取大）；高度 = 候选数（**写死 maxHeight=10** 暂不滚动） | 同时满足内容和 title 显示，避免长 title 截断 |
| 6 | SelectPane 键位 | `↑↓ Enter Esc`，上下键 **wrap-around**（**不绑任何 Ctrl 组合**） | wrap 在短列表（2–5 项）里更顺手；保持键表最小；未来扩 vim 风格 |
| 7 | callback 签名 | `func(*string)` —— nil=取消 / &str=确认 | 符合 Go 习惯（零值即无），比 string+哨兵值更类型安全 |
| 8 | 事件接入方式 | **BufPane.HandleEvent / Display 顶部加 if 转发**（D13 期为 test scaffolding，D12 集成时转正为生产接入） | trigger 在 BufPane，forwarding 在 BufPane，一致；D12 已采用此方案，Alt-I 测试 trigger 已清理（见 §3.8） |
| 9 | 焦点模型 | **modal**（打开时主编辑器冻结） | 选择器是临时决策点，避免用户误操作主编辑器 |

### 决策点补充说明

**决策 8 解释**：

D13 阶段在 `BufPane.HandleEvent` / `Display` 加 if 转发，最初标为 **test scaffolding**。
D12 集成时**全权决定**接入方式，已选定：**保留 BufPane forwarding 作为生产接入**。
当前 `bufpane.go` 的 2 个 if 是 SelectPane 的唯一接入点。

设计层面：§3.0、§3.1 明确 SelectPane 是独立组件，不与 notePane 强耦合。
实现层面：§3.8 详细描述当前 BufPane forwarding 的代码位置与状态演进。

**决策 7 callback 签名选择**：

| 方案 | 签名 | 评估 |
|------|------|------|
| **A. `func(*string)`** ✅ | `nil` / `&str` | Go 习惯，类型安全，可区分"空字符串选中"和"没选" |
| B. `func(string, bool)` | `(str, ok)` | 两个返回值稍笨重，签名不直观 |
| C. `func(string)` 哨兵 | `""` = 取消 | 不能区分"用户选了空字符串"和"取消"（类型上不安全） |
| D. 两个 callback `onSelect + onCancel` | 分开 | 调用方要写两个，啰嗦 |

选 A：最简洁且类型最安全。

**决策 3 为什么不做栈**：

v1 的实际场景永远只有 0 或 1 个浮窗打开：
- 选 receiver → SelectPane → 关 → （可能开 notePane）
- 选 file → SelectPane → 关 → （打开文件）
- 任意时刻只有 1 个浮窗

栈的"嵌套"能力（多个浮窗叠起来）v1 **完全用不到**。单例变量足够，代码更简单：
- `Open`：直接赋值 `TheSelectPane = ...`
- `Close`：直接 `TheSelectPane.isOpen = false`
- 不需要 push/pop，不需要"我是不是栈顶"的防御性检查

---

## 五、范围之外（v1 不做）

### 5.1 不做浮窗框架
- **不引入 Float interface**：SelectPane 的通用性靠参数（items/title/onSelect）就够，不需要抽象成 interface
- **不做浮窗栈 / 堆叠**：v1 流程永远只有 0 或 1 个浮窗
- **不做"真的 modal"**（改 micro.go 加 ModalStack）：项目原则不允许改 micro 原生代码
- **不做浮窗间通信 / 动画 / 拖拽 / 缩放**

> **关键澄清**：FilePicker / ColorPicker / CommandPalette **不是**新的 pane 类型。它们就是 SelectPane 拿不同的 items/title/onSelect 调用。所以不需要为它们做任何抽象——SelectPane 已经够通用。

### 5.2 SelectPane 自身不做（详见 §3.9）
- 滚动 / 模糊搜索 / 鼠标 / vim 键位 / Ctrl 组合
- 自定义位置 / maxHeight / 颜色 / 边框字符
- 多选 / 异步加载 items

---

## 六、实施步骤（已完成）

> **当前状态**：D13 + D12 集成全部完成，所有步骤已交付。

```
☑ 决策项拍板（§四）

☑ D13.1 SelectPane 实现（internal/action/selectpane.go）
   - struct（§3.6）+ Open/Close/IsOpen/HandleEvent/Display（§3.1）
   - 边框 + 上边框标签 {title}（§3.4）
   - 键位（↑↓ Enter Esc，wrap-around）（§3.5）
   - 尺寸算法（§3.3，宽度同时考虑 title）
   - 位置算法（§3.2 锚点自适应；D13.1 初版是"左下角向上"，后扩展为锚点自适应）
   - 全局单例 TheSelectPane（globals.go:17 初始化）

☑ D13.2 BufPane forwarding（已从 test scaffolding 转正为生产接入）
   - bufpane.go:433-438 HandleEvent 顶部 if 转发
   - bufpane.go:541-548 Display 末尾 if 叠加绘制
   - 注释已更新为 "microNeo D12 集成"，不再是 test scaffolding

☑ D13.3 Alt-I 触发测试 SelectPane — 已按集成计划清理
   - Alt-I trigger 与 selectTestOpen 函数均已移除
   - 保留 notepane.go:176 一处调用作为回归入口
   - 验证方式改为：打开 notePane，Discover 多 receiver，验证 SelectPane 弹出与选择

☑ D13.4 与 D12 集成（已完成）
   - D12 选定方案：保留 BufPane forwarding 作为生产接入（见 D12 决策 11）
   - notepane.go:176 调用点：TheSelectPane.Open(names, "Receiver", nil, nil, ...)
   - 取消 Esc 后清零 selectedReceiver，InfoBar 显示 "✗ 已取消"（与屏幕太小走同语义）
```

---

## 七、参考

- **D11** — `D11-名字分配方案.md`（名字分配机制，SelectPane 的 receiver items 是 D11 给出的名字）
- **D12** — `D12-多receiver选择.md`（SelectPane 的第一个调用方；本 D13 只负责 SelectPane 自身，不涉及 notePane 业务逻辑）
- **D13 位置算法** — 本文档 §3.2（锚点自适应展开算法）
- **`selectpane.go:44`** — `Open` 签名与位置算法实现
- **`selectpane.go:80-122`** — 自适应展开算法（y 方向 + x 方向 + clamp）
- **`bufpane.go:433-438, 541-548`** — BufPane forwarding（D12 生产接入点）
- **`notepane.go:176`** — notePane D12 多 receiver 选择的唯一调用点（传 `nil, nil`）
- **`notepane.go:548-589`** — notePane 边框绘制流程（SelectPane 复用）
- **`micro.go:533-554`** — micro 主循环事件分发优先级链（不动，只理解）
