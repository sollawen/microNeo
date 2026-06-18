# D13 — SelectPane 设计（通用列表选择浮窗）

> **状态**：已实施完成（D12 集成中）
>
> **范围**：一个**通用**的列表选择浮窗 `SelectPane`——`Open(items, title, onSelect)`，从列表里选一个 string。当前用于 D12 的多 receiver 选择，未来可复用于选文件、选配色等任何"从列表选一个"的场景。
>
> **实施偏差回填**：实际代码相对本文原设计有两处有意偏差（用户拍板，UX 更好）：① title **不反色**（原文 §3.4 写 "Reverse 突出"）；② 上下键 **wrap-around**（原文 §3.5 写 "到边停"）。本文已据此回填。另有几处改进（位置留 1 行间隔、宽度同时考虑 title、测试 title 用长串）未逐字回填，以代码为准。
>
> **不做**：浮窗框架（Float interface / 栈 / dispatcher）、其它 pane 类型（FilePicker / ColorPicker 不是新 pane，是 SelectPane 的不同调用方）。
>
> **不改**：EABP 协议、接收端、micro 原生代码（`cmd/micro/micro.go` 不动）。

---

## 一、动机

### 1.1 直接需求：D12 需要"选 receiver"

D12 的 notePane 多 receiver 流程：Discover 到 2+ receiver 时，需要弹一个浮窗让用户选一个。

### 1.2 SelectPane 是通用的（不是只给 notePane 用）

这个"展示列表 → 上下键移动 → Enter 确认 / Esc 取消 → 回调返回结果"的能力**非常通用**。同一个 SelectPane 组件可以服务很多场景：

| 场景 | items | title | onSelect 做什么 |
|------|-------|-------|-----------------|
| **选 receiver**（v1，D12） | `["pi-Alpha", "pi-Bravo", ...]` | `"Receiver"` | 把选中的 receiver 名字存进 notePane 上下文 |
| **选文件**（未来） | `["a.md", "b.go", ...]` | `"File"` | 打开对应文件 |
| **选配色**（未来） | `["solarized", "gruvbox", ...]` | `"Color"` | 切换配色 |
| **选命令**（未来） | `["> open", "> save", ...]` | `"Command"` | 执行对应命令 |

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

`SelectPane` 是**通用列表选择浮窗**——`Open(items, title, onSelect)`，从列表里选一个 string。

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
//   onSelect 回调：
//            用户按 Enter → onSelect(&items[selected])，浮窗关闭
//            用户按 Esc   → onSelect(nil)，浮窗关闭
func (s *SelectPane) Open(items []string, title string, onSelect func(selected *string))

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
// D12：选 receiver
TheSelectPane.Open(
    []string{"pi-Alpha", "pi-Bravo", "pi-Charlie"},
    "Receiver",
    func(s *string) {
        if s == nil { /* 用户取消 */ return }
        // 用 *s 打开 notePane...
    },
)

// 未来：选文件
TheSelectPane.Open(filenames, "File", func(s *string) { ... })

// 未来：选配色
TheSelectPane.Open(colorNames, "Color", func(s *string) { ... })
```

**设计要点**：
- **返回 `*string`**：通过 callback 异步返回
  - `nil` = 用户按 Esc 取消（**没选任何东西**）
  - `&"xxx"` = 用户按 Enter 确认（**选中了第 N 项**）
  - 用 `*string` 而非 `string` + 哨兵值（如 `""`），符合 Go 习惯（"零值即无"），更类型安全
- **title 必填**：调用方必须告诉用户"现在在选什么"，避免用户看到一个无标题的列表发懵

### 3.2 位置（start menu 风格）

**固定在 statusLine 上面，向上弹出**（像 Windows 开始菜单）：

```
┌──Receiver────────────────────┐   ← title 由调用方决定
│ item 1                       │ ← 顶
│ item 2                       │
│ item 3                       │
│ ...                          │
│ item N                       │ ← 底
└──────────────────────────────┘
────────────────────────────────  ← statusLine（被覆盖/隐藏）
────────────────────────────────  ← infoBar
```

- **底边 Y** = `statusLine.Y - 1`（贴 statusLine 上面）
- **顶边 Y** = `底边 Y - height - 1`（向上展开 height 行；实现里多留 1 行间隔，以代码为准）
- **水平 X** = 暂定 `0`（左对齐屏幕最左边）。未来允许传入 `x` 参数。
- 弹出时**临时遮盖**该区域的主编辑器内容（先 `Clear` 整个矩形，再画边框 + 列表）。关闭时由下一帧主编辑器重绘覆盖。

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

### 3.8 事件接入（test scaffolding）

> **D13 测试期间在 BufPane.HandleEvent / BufPane.Display 顶部加 if 转发**
> （跟 Alt-I trigger 在同一个位置，保持一致）。
> 这是 **test scaffolding**，**不是 D13 的设计决策**——D12 集成时可以保留 / 重构 / 替换。

**为什么是 BufPane 不是 notePane**：
- Alt-I trigger 在 BufPane binding 里，forwarding 在 BufPane 是一致的选择
- notePane 槽位意味着 "SelectPane 与 notePane 强耦合"——这跟 §3.0 设计原则（SelectPane 是独立组件）冲突
- notePane 触发场景是"在 notePane 里按 Alt-Enter"，跟"主编辑器按 Alt-I 选 receiver"是两套场景

**BufPane 是 micro 原生代码，改动多少**：
- HandleEvent 顶部加 1 个 if（≤3 行）
- Display 末尾加 1 个 if（≤3 行）
- 不动 BufPane 原有逻辑

**D12 集成时的可能选项**：
- 保留 BufPane forwarding（最省事）
- 删 BufPane 改动，封装进 notePane 自己的入口（如果 D12 选定 notePane 场景）
- 重构成独立 modal pane
- 其他

D13 只提供组件 + scaffolding，不预判 D12 怎么选。

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
| 8 | 事件接入方式 | **BufPane.HandleEvent / Display 顶部加 if 转发**（D13 期作 test scaffolding，**D12 集成时转正**） | trigger 在 BufPane，forwarding 在 BufPane，一致；D12 选定保留并转正（见 D12 决策 11） |
| 9 | 焦点模型 | **modal**（打开时主编辑器冻结） | 选择器是临时决策点，避免用户误操作主编辑器 |

### 决策点补充说明

**决策 8 解释**：

D13 测试期间在 BufPane.HandleEvent / Display 顶部加 if 转发让 SelectPane 能跑起来。
这只是**测试 scaffolding**：
- D13 本身不设计"生产环境怎么接入"这个架构问题
- 组件本身（Open/Close/IsOpen/HandleEvent/Display）与任何槽位选择正交
- D12 集成时全权决定：保留 BufPane forwarding、重构、还是换成别的方案（独立 modal pane、新槽位等）

设计层面：§3.0、§3.1 明确 SelectPane 是独立组件，不与 notePane 强耦合。
实现层面：§六 D13.2 临时在 bufpane.go 加 2 个 if（HandleEvent / Display 各一个），仅供 D13 独立测试用。

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

## 六、实施步骤

> **核心原则**：D13 **单独开发、单独测试**——不依赖 D12、不接 EABP。
> 测试 trigger：**主编辑器里按 Alt-I** → 弹出测试 SelectPane → ↑↓ Enter/Esc 验证回调。
> **不动 micro.go，不动 notePane.go**。
>
> **怎么保证 SelectPane 能收到键、能画到屏上**：
> Alt-I trigger 在 BufPane，forwarding 也在 BufPane（HandleEvent / Display 顶部加 if）。
> 这是 test scaffolding，不是 D13 的设计决策——D12 集成时可以保留 / 重构 / 替换。

```
☑ 决策项拍板（§四）

□ D13.1 SelectPane 实现（internal/action/selectpane.go，新建）
   - struct（§3.6）+ Open/Close/IsOpen/HandleEvent/Display（§3.1）
   - 边框 + 上边框标签 {title}（§3.4）
   - 键位（↑↓ Enter Esc）（§3.5）
   - 尺寸算法（§3.3，maxLen+4 修正后）
   - 位置算法（§3.2，向上弹出）
   - 全局单例 TheSelectPane（globals.go 初始化，1 行）

□ D13.2 BufPane forwarding（test scaffolding，D12 重构）
   在 bufpane.go 的 HandleEvent / Display 各加一个 if，让 SelectPane 在打开期间
   劫持事件 / 叠加绘制：
   - HandleEvent 顶部加：
       if TheSelectPane != nil && TheSelectPane.IsOpen() {
           TheSelectPane.HandleEvent(event)
           return
       }
   - Display 末尾加：
       if TheSelectPane != nil && TheSelectPane.IsOpen() {
           TheSelectPane.Display()
       }
   明确标注为 test scaffolding（注释里写清楚）。
   bufpane.go 是 micro 原生代码，但改动仅 2 个 if，不动原有逻辑。

□ D13.3 Alt-I 触发测试 SelectPane（主编辑器 binding）
   - 注册 Alt-I → SelectTestOpen action 到 BufPane bindings
     （在 internal/action/bindings.go 加一行 BufMapEvent）
   - 实现 SelectTestOpen（放 internal/action/selectpane.go）：
       func selectTestOpen(h *BufPane) bool {
           TheSelectPane.Open(
               []string{"alpha", "bravo", "charlie", "delta", "echo"},
               "test",
               func(s *string) {
                   if s == nil {
                       InfoBar.Message("✗ 用户啥也没选择")
                   } else {
                       InfoBar.Message("✓ selected: " + *s)
                   }
               })
           return true
       }
   - 验收（11 步手动测试）：
       1. Alt-I → 弹出 SelectPane：左下贴 statusLine，
          上边框 `┌──test──...─┐`（title 反色高亮），
          第一项 `alpha` 反色高亮
       2. ↓ ×3 → 高亮跳到 `delta`
       3. ↓ ×1 → `echo`（最后一项，不 wrap）
       4. ↓ ×1 → 仍 `echo`（到边停）
       5. ↑ ×1 → `delta`
       6. 输入字母 `j` → 主编辑器 **无变化**（modal 吞键验证）
       7. Esc → 浮窗消失，InfoBar 显示 `✗ 用户啥也没选择`
       8. 再 Alt-I → 弹出
       9. Enter（默认 alpha）→ InfoBar `✓ selected: alpha`
       10. 再 Alt-I → ↓ ×1（bravo）→ Enter → `✓ selected: bravo`
       11. ↑ 到顶后再 ↑ → 仍第一项（到边停）
   - 回归（确保主编辑器未被破坏）：
       - 主编辑器正常输入字符 / 移动光标 ✓
       - notePane 触发 → 输入 → Esc 关闭 ✓（D13 不动 notePane.go，完全不受影响）
   - 编译：`make build-quick` 0 错 0 警

□ D13.4 与 D12 集成（D12 阶段做，本步不交付）
   - D12 决定 SelectPane 在生产环境怎么接入（保留 BufPane forwarding？重构？还是别的方式？）
   - D12 调 TheSelectPane.Open(receiverNames, "Receiver", onSelected)
   - Alt-I 测试触发可以保留（回归测试），也可以删除（由 D12 决定）
```

> **依赖关系**：
> - D13.1 + D13.2 + D13.3 是 D13 的完整交付，**本步完成即可独立验收**，不依赖 D12。
> - D13.4（D12 集成）是 D12 的工作，D13 提供的 BufPane forwarding scaffolding 可以被重定义。

---

## 七、参考

- **D11** — `D11-名字分配方案.md`（名字分配机制，SelectPane 的 receiver items 是 D11 给出的名字）
- **D12** — `D12-多receiver选择.md`（SelectPane 的第一个调用方；本 D13 只负责 SelectPane 自身，不涉及 notePane 业务逻辑）
- **`notepane.go:548-589`** — notePane 边框绘制流程（SelectPane 复用）
- **`notepane.go:556-563`** — `NotePane.HandleEvent` 当前实现（D12 集成时的参考）
- **`micro.go:533-554`** — micro 主循环事件分发优先级链（不动，只理解）
