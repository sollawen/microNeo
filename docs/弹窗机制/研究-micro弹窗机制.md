# micro 的弹窗机制研究

> 配套文档：[`研究-microPane机制.md`](./研究-microPane机制.md)（讲 micro **有**的 pane 框架）
> 本文：讲 micro **没有**的弹窗（popup / modal / overlay）框架。
>
> 起因：讨论 SelectPane 改进时，发现 micro 的 Pane 接口对它帮助有限，深入查证后确认这不是"帮助有限"，而是"micro 在这个领域根本没有基础设施"。这个发现直接影响 SelectPane 及未来所有浮窗的设计取向，值得单独成文。
>
> 本文所有论断标注代码出处（文件:行），便于复核。

---

## 一、核心结论（一句话）

**micro 没有任何弹窗框架——连一点基础设施都没有。** 整个 UI 架构是"一棵分屏树（tile 树）"，树上没有"浮在树之上的节点"这种东西。所有看起来像弹窗的交互（确认框、消息提示、命令输入），micro 都是用 InfoBar 切换状态标志位来"假装"的，没有独立的弹出窗口。

**对 SelectPane 和未来所有浮窗的含义**：我们基本 on our own，micro 能给的只有"方法签名约定"和"路由范式"，rect计算 / z-order / 事件隔离 / 生命周期全得自建。这不是"没用 micro 的成熟框架"，而是"micro 在这个领域没有成熟框架可用"。

---

## 二、三层证据：micro 只有 tile 框架

把可能藏"弹窗框架"的三个层级都查了，结论一致——没有。

| 层 | 包 / 文件 | 提供的东西 | 有没有弹窗原语？ |
|---|---|---|---|
| **UI 布局层** | `internal/views/splits.go` | 分屏树（`Node` / `View` / `SplitType`） | ❌ 搜 popup/overlay/float 只命中 `float64`（分屏比例计算），无浮窗概念 |
| **渲染层** | `internal/display/` | `BufWindow` / `TabWindow` / `TermWindow` / `InfoWindow` / `UIWindow` / `StatusLine` | ❌ 全是 tile 的渲染模块，没有任何 overlay/popup 渲染器 |
| **抽象层** | `internal/action/pane.go` | `Pane` 接口（16 个方法） | ❌ 13/16 个方法是 tile 专属（详见 §五） |

### views 包的全貌（最直接的证据）

`internal/views/` 目录下**只有 `splits.go` 一个源文件**（加一个 test），导出的全部类型只有三个：

```go
// internal/views/splits.go
type SplitType uint8   // :8  分屏方向（水平/垂直）
type View struct {...} // :25 矩形视图
type Node struct {...} // :34 分屏树节点（叶子 = tile pane）
```

整个 views 包就是"分屏树的实现"。分屏树的本质是：父节点按比例切分矩形给子节点，每个叶子持有一个 pane。**这棵树里不存在"浮在某个节点之上的、不参与矩形切分的节点"**——架构层面就没有这个抽象。弹窗在分屏树里无处安放。

---

## 三、micro 唯一的"弹窗"：InfoBar 状态标志位（最有意思的发现）

这是整个调研里最反直觉的发现。

micro 里"是/否确认框"、"消息提示"、"命令输入"、"错误提示"——这些**最典型的弹窗场景**，全都不是弹窗，而是 InfoBar 通过状态标志位切换自己的渲染模式。

### 证据：InfoBuf 的状态标志位

`internal/info/infobuffer.go:11`：

```go
type InfoBuf struct {
    *buffer.Buffer
    HasPrompt  bool    // ← 命令输入模式
    HasMessage bool    // ← 消息提示模式
    HasError   bool    // ← 错误提示模式
    HasYN      bool    // ← 是/否确认模式
    PromptType string
    Msg    string
    YNResp bool
    ...
}
```

那几个看起来像"弹出"的方法，**只是设置标志位**：

```go
// internal/info/infobuffer.go
func (i *InfoBuf) Message(msg ...any) {...}                  // :58  设 HasMessage=true
func (i *InfoBuf) Prompt(...) {...}                          // :97  设 HasPrompt=true
func (i *InfoBuf) YNPrompt(prompt string, donecb func(bool, bool)) {...}  // :123 设 HasYN=true
```

### "确认框"在 micro 里到底是什么

调用 `YNPrompt("确定删除？", callback)` 之后发生的事：

1. **不是**：弹出一个独立的、带边框的、盖在编辑区之上的确认窗口
2. **而是**：底部 InfoBar 把自己从"状态栏模式"切换到"确认框模式"（HasYN=true），显示 `确定删除？(y,n,esc)`，然后等用户按键

- **没有独立窗口**：确认框就是 InfoBar 自己
- **没有独立 z-order**：确认框在屏幕底部那个固定位置，不"盖"在任何东西之上
- **没有独立事件路由**：DoEvent 看 `InfoBar.HasPrompt` 标志（micro.go:549）决定把事件给 InfoBar，确认框复用的就是这条命令输入的路由分支

### 这说明什么

**micro 连"做一个真正的弹出框"这件事都没做过**。YNPrompt 这种最典型的弹窗用例，micro 都选择用"状态栏变脸"来回避。弹窗在 micro 的 UI 词汇表里是**彻头彻尾的二等公民**——它甚至没被当作一类对象，只是 InfoBar 的几种"表情"之一。

对比一下就清楚差距：

| 框架类型 | 代表 | 弹窗是一等公民吗 | 典型实现 |
|---|---|---|---|
| **通用 UI 框架** | React、Qt、GTK | ✅ | 独立的 `<Modal>` 组件 / `QDialog` 类，有自己的 z-order、生命周期、事件隔离 |
| **tile 编辑器框架** | micro、vim、tmux | ❌ | 状态栏变脸（micro）或屏幕区域复用（vim 的 cmdline） |

micro 属于第二类。指望它的 Pane 接口服务弹窗，就像指望 tmux 的 window/pane 抽象服务弹窗——架构层面就没这个意图。

---

## 四、为什么会这样：micro 是 tile 编辑器框架，不是通用 UI 框架

这是底层架构选择决定的，不是疏忽。

- **micro 的 UI 心智模型 = 一个 Tab 里若干 tile pane**（编辑器 + 终端的分屏）。这是它的核心场景，Pane 接口、views 分屏树、DoEvent 路由三个核心 UI 基础设施全围绕这个设计。
- **弹窗不在 micro 的设计目标里**：micro 自身的交互（命令、确认、搜索、替换）全部走 InfoBar 这条线，从来不弹独立窗口。既然自己用不上，自然不会抽象出弹窗框架。
- **micro 的 UI 三件套（Pane / views / DoEvent）都是 tile 专属工具**，不是"通用窗口框架"。

一句话：**Pane 接口是"tile 编辑器的瓦片窗抽象"，不是"通用窗口抽象"**。把"pane"这个词理解成"任何 UI 面板"是误解——在 micro 的语境里，pane ≈ tile。

---

## 五、Pane 接口对弹窗的真实可用率：3/16

如果硬要让一个弹窗实现 Pane 接口，把 16 个方法按"弹窗用不用得上"分类：

| 类别 | 方法 | 弹窗用得上吗 |
|---|---|---|
| **事件/绘制** | `HandleEvent`, `Display` | ✅ 有用 |
| **生命周期** | `Close` | ✅ 有用 |
| 窗口视图 | `GetView` / `SetView` / `LocFromVisual` / `Relocate` / `Resize` / `Clear` / `SetActive` / `IsActive` | ❌ tile 专属（弹窗不进分屏树，没有 View 概念） |
| 身份/归属 | `ID` / `SetID` / `Name` / `SetTab` / `Tab` | ❌ tile 管理专属 |
| 命令 | `HandleCommand` | ❌ tile 专属 |

**可用率 3/16 ≈ 19%**。而且这 3 个（`HandleEvent` / `Display` / `Close`）SelectPane **已经自己实现了**——所以 Pane 接口对它的实际增益是 **0**。详见 [`研究-microPane机制.md`](./研究-microPane机制.md) §9.4 的三条论证。

---

## 六、micro 留给弹窗的全部"遗产"（很轻）

既然没有框架，那有没有任何可复用的东西？有，但只有两样，都很轻：

| 遗产 | 内容 | 价值 | SelectPane 是否已吃红利 |
|---|---|---|---|
| **签名约定** | `HandleEvent(tcell.Event)` + `Display()` 这两个方法签名 | 任何 pane 都能用同样的两行代码转发给弹窗（鸭子类型） | ✅ 当前 hook 转发就靠这个 |
| **路由范式** | ① 改 DoEvent 加专门分支（notePane 模式）② 宿主 pane 转发（SelectPane 模式） | 这是"模式"不是"基础设施"，每次得自己接线 | ✅ 用了模式 ② |

**没有遗产的部分**（全得自己写，没有捷径）：

| 能力 | micro 提供？ | SelectPane 当前怎么解决 |
|---|---|---|
| 浮窗rect计算（位置、大小、自适应展开） | ❌ | 自己写了 100+ 行（selectpane.go 的 Open 方法） |
| z-order 管理（盖在谁上面） | ❌ | 靠 "Display 调用顺序" 手动保证（D15 在 notePane 末尾追加） |
| 事件隔离（开着时不让底层收键） | ❌ | 靠 hook 里 early return 手动保证 |
| 生命周期（Open / Close / IsOpen） | ❌ | 每个弹窗自己管 |
| 浮窗的统一抽象（多个浮窗共用一套机制） | ❌ | 暂无（见 §八，v1 不必做） |

---

## 七、对 SelectPane / 未来浮窗的实际含义

结论很干脆，而且是个**好消息式的坏消息**：

### 7.1 坏消息：没有便车可搭

micro 在弹窗领域没有成熟框架，SelectPane 改进时**不能指望"用上 micro 的 X 框架"**——因为 X 不存在。rect计算、z-order、事件隔离都得我们自己设计、自己维护。这部分工作量是逃不掉的。

### 7.2 好消息：当前设计已被验证是合理的

SelectPane 当前的设计**不实现 Pane 接口、不嵌入 BufPane、自己管rect和生命周期**——这种"自给自足"的取向，正是因为 micro 没给弹窗任何基础设施。**这不是"没用 micro 的成熟框架"，而是"micro 在这个领域没有成熟框架可用"**。

换句话说，SelectPane 没有错过任何本可利用的东西。它的设计是适应现实（micro 无弹窗框架）的正确选择，不是浪费。

### 7.3 中间地带：可借鉴范式，但不是调用 API

micro 留下的两样遗产（签名约定 + 路由范式）属于"可以借鉴其形态，但不能当 API 调用"的东西：

- 签名约定：让 SelectPane 能被任何 pane 用同样两行转发——这是**形态红利**，不是 API
- 路由范式：宿主转发模式——这是**设计模式**，每个宿主仍要自己接线（D15 在 notePane 补两行就是接这个线）

---

## 八、给未来浮窗的判断标准

如果以后要加更多弹窗（搜索浮窗、命令面板、快捷键提示浮窗等），两个工具：

### 8.1 单条复用判断规则

> **问：这块 micro 代码是为"tile 里的内容渲染/事件处理"写的，还是为"窗口本身"写的？**
> - 前者（如 buffer 渲染、光标、syntax 高亮、分屏树）→ 和浮窗无关，别碰
> - 后者（如 DoEvent 的路由结构、Display 的调用顺序）→ 是唯一可借鉴的，但也只是"借鉴范式"，不是"调用 API"

### 8.2 多浮窗共用的潜在抽象（v1 不必做，但要知道这个选项）

当浮窗变多时，可以把 micro 没给的那部分抽象自己建起来。**注意：这建的是我们自己的小框架，不是 micro 的**。

```go
// 概念示意，micro 当前没有，是我们可能要自己造的
type FloatingWidget interface {
    HandleEvent(tcell.Event)
    Display()
    IsOpen() bool
}

// + 一个统一的rect计算工具（位置/自适应展开方向）
// + 一个统一的 z-order 注册表（画在哪个层级）
// + 一个统一的宿主转发 hook（任意 pane 一行接入）
```

**何时做**：当浮窗数量 ≥ 3 个，且每个都在重复写rect计算和 hook 接线时。现在只有 SelectPane 一个，做这个属于 YAGNI（过早抽象），先按 D15 把第二个宿主接上再说。

---

## 九、关键代码索引

| 文件 | 行 | 内容 | 说明 |
|---|---|---|---|
| `internal/views/splits.go` | - | 整个 views 包只有这一个源文件 | 证明 views 是"分屏树实现"，无浮窗概念 |
| `internal/views/splits.go` | 8 | `type SplitType` | 分屏方向 |
| `internal/views/splits.go` | 25 | `type View` | 矩形视图 |
| `internal/views/splits.go` | 34 | `type Node` | 分屏树节点（叶子 = tile pane） |
| `internal/display/bufwindow.go` | 17 | `type BufWindow` | tile 渲染模块 |
| `internal/display/tabwindow.go` | 12 | `type TabWindow` | tile 渲染模块 |
| `internal/display/termwindow.go` | 13 | `type TermWindow` | tile 渲染模块 |
| `internal/display/infowindow.go` | 13 | `type InfoWindow` | InfoBar 渲染模块 |
| `internal/display/statusline.go` | 25 | `type StatusLine` | 状态条（BufWindow 子组件，非 pane） |
| `internal/info/infobuffer.go` | 11 | `type InfoBuf`（含 HasPrompt/HasMessage/HasError/HasYN） | "假弹窗"的状态标志位 |
| `internal/info/infobuffer.go` | 58 | `Message()` | 设 HasMessage（不是弹窗） |
| `internal/info/infobuffer.go` | 97 | `Prompt()` | 设 HasPrompt（不是弹窗） |
| `internal/info/infobuffer.go` | 123 | `YNPrompt()` | 设 HasYN（确认框也是状态栏变脸，不是弹窗） |
| `internal/action/pane.go` | 8 | `Pane` 接口 | tile 瓦片窗抽象（详见 §五） |
| `cmd/micro/micro.go` | 549 | `else if action.InfoBar.HasPrompt` | DoEvent 看 HasPrompt 标志路由（确认框/命令输入共用这条分支） |

---

## 附：与 [`研究-microPane机制.md`](./研究-microPane机制.md) 的分工

| 文档 | 回答什么 | 核心论点 |
|---|---|---|
| `研究-microPane机制.md` | micro **有**什么 pane 框架？ | Pane 接口是 tile 瓦片窗的成熟抽象，BufPane/TermPane 是真正用户；InfoPane/NotePane 是"碰巧满足"的浮层；SelectPane 不实现它是对的 |
| **本文**（`研究-micro弹窗机制.md`） | micro **没有**什么框架？ | micro 没有任何弹窗框架，连 YNPrompt 都是 InfoBar 状态标志位假装的；SelectPane 和未来所有浮窗都得自建rect/z-order/事件隔离，micro 只给签名约定和路由范式 |

两篇合起来回答一个完整问题：**"SelectPane 改进时，micro 的哪些东西能用、哪些不能用？"**
- Pane 接口不能用（见 Pane 机制研究 §9.4）
- 弹窗框架不存在（见本文 §一）
- 能用的只有：签名约定 + 两种路由范式（见本文 §六）
