# micro 的 Pane 机制研究

> 用途：在设计 / 改进我们自己的 pane（notePane、selectPane）之前，先把 micro 已有的 pane 框架吃透。
> 这样才能判断：哪些 micro 框架可以直接复用，哪些只能"参照范式自己写"，哪些必须避开。
>
> 起因：notePane 设计时没深入研究 micro 的 pane 机制；selectPane 改进前需要补这一课。
>
> 本文所有论断都标注了代码出处（文件:行），便于复核。

---

## 一、核心结论（先看这条）

micro 的"pane"其实是**两个正交概念的合体**，被同一个 `Pane` 接口绑在一起：

1. **瓦片窗（tile）**：占 Tab 里一块矩形区域、被分屏布局管理、随窗口缩放的生命周期。
2. **事件/绘制宿主（handler/renderer）**：能收键盘事件、能画自己的东西。

micro 原生的 4 个 pane 实现里：
- **BufPane、TermPane** = 瓦片窗 + 宿主（两者都是）
- **InfoPane、NotePane** = **只是宿主，不是瓦片窗**（虽然结构上"碰巧满足"了 Pane 接口）

这个区分是理解后续一切（事件路由、绘制顺序、为什么浮层要专门分支、为什么 SelectPane 不该实现 Pane 接口）的钥匙。

---

## 二、Pane 接口的定义

`internal/action/pane.go:8`：

```go
// A Pane is a general interface for a window in the editor.
type Pane interface {
    Handler          // 事件合约
    display.Window   // 窗口/绘制合约
    ID() uint64
    SetID(i uint64)
    Name() string
    Close()
    SetTab(t *Tab)
    Tab() *Tab
}
```

它组合了两个更小的接口：

```go
// internal/action/events.go:185 —— 事件合约
type Handler interface {
    HandleEvent(tcell.Event)
    HandleCommand(string)
}

// internal/display/window.go:20 —— 窗口/绘制合约
type Window interface {
    Display()
    Clear()
    Relocate() bool
    GetView() *View
    SetView(v *View)
    LocFromVisual(vloc buffer.Loc) buffer.Loc
    Resize(w, h int)
    SetActive(b bool)
    IsActive() bool
}
```

**注意**：`display.Window` 接口里只有 `Display()`，没有 `HandleEvent`；`HandleEvent` 来自 `Handler` 接口。两者是分开的。合计 Pane 接口要求约 **16 个方法**。

---

## 三、Pane 接口的三大用处

### 用处 1：多态容器（最核心）——一个 Tab 能装混合 pane

`internal/action/tab.go:237`：

```go
type Tab struct {
    ...
    Panes  []Pane   // ← 接口切片
    active int
    ...
}
```

这个 `[]Pane` 能**同时装编辑器和终端**：

```
┌─────────────┬─────────────┐
│             │             │
│  BufPane    │  TermPane   │  ← 同一个 Tab.Panes，类型完全不同
│  (编辑文件) │  (跑 shell) │     都满足 Pane 接口
│             │             │
└─────────────┴─────────────┘
```

没有 Pane 接口，`[]Pane` 就只能写死成 `[]*BufPane`，micro 永远不可能支持终端分屏。

### 用处 2：事件/绘制/焦点的统一分发

Tab 用 Pane 接口的方法，一行代码服务所有 pane 类型：

| 调用点 | 代码 | 干什么 |
|---|---|---|
| `tab.go:337` | `t.Panes[t.active].HandleEvent(event)` | 事件投给 active pane（BufPane / TermPane 都行） |
| `tab.go:343` | `for j, p := range t.Panes { p.SetActive(...) }` | 切焦点时遍历所有 pane |
| `tab.go:308, 326` | `p.GetView()` 判断鼠标落点 | 点击切换 pane 时不关心类型 |
| `tab.go:364` | `p.ID()` | 按 ID 查找 pane |
| `tab.go:381` | `p.GetView()` | 布局时拿每个 pane 的矩形 |

没有 Pane 接口，这些代码每一个都得写成 `switch p := pane.(type)` 的类型分发。

### 用处 3：键绑定系统的统一签名（容易被忽略）

`internal/action/keytree.go:9-11`：

```go
type PaneKeyAction    func(Pane) bool
type PaneMouseAction  func(Pane, *tcell.EventMouse) bool
type PaneKeyAnyAction func(Pane, []KeyEvent) bool
```

整个键绑定树以 `Pane` 为参数类型。同一套 bindings 框架服务 BufPane、TermPane、InfoPane、NotePane，它们各自注册自己的 action（如 `bufpane.go:32` 的 `BufKeyActionGeneral`），骨架共享。具体 action 当然要类型断言拿回具体类型（`bufpane.go:34`: `a(p.(*BufPane))`），但**调度层是 pane-agnostic 的**。

---

## 四、micro 现有 pane 的实现方式（4 个 + 1 个反例）

### 4.1 BufPane —— 瓦片窗的正统实现

`internal/action/bufpane.go:206`：

```go
type BufPane struct {
    display.BWindow    // ← 嵌入接口（实现 display.Window）
    Buf *buffer.Buffer
    ...
}
```

自己实现 `Handler`（HandleEvent / HandleCommand）+ ID/Name/SetTab 等。是 Pane 接口的"标准实现"，进 Tab.Panes。

### 4.2 TermPane —— 接口抽象价值的最强证据

`internal/action/termpane.go:53`：

```go
type TermPane struct {
    *shell.Terminal
    display.Window     // ← 嵌入接口（但不是 BWindow！和 BufPane 不一样）
    mouseReleased bool
    id   uint64
    tab  *Tab
}
```

**TermPane 和 BufPane 没有任何继承关系**——一个是终端模拟器，一个是文件编辑器，数据模型完全不同。但 TermPane **手写了 Pane 接口要求的全部方法**（ID/Name/SetTab/Tab/Close/HandleCommand，见 termpane.go:76-230），就为了能塞进 `Tab.Panes []Pane`，和 BufPane 并排出现、被同一套事件分发和鼠标命中测试服务。

**这是 Pane 接口正向收益的教科书案例**：让两个本来八竿子打不着的类型，在"瓦片窗口"这个抽象层面互换。

### 4.3 InfoPane（InfoBar）—— 嵌入 BufPane 白嫖实现

`internal/action/infopane.go:59`：

```go
type InfoPane struct {
    *BufPane           // ← 嵌入 BufPane，白嫖全部 Pane 方法
    *info.InfoBuf
}
```

- **满足**Pane 接口（因为嵌入 BufPane）
- 但**从不使用** Pane 接口：不进 Tab.Panes，路由走 DoEvent 专门分支
- 重写 `HandleEvent` / `DoKeyEvent` 等（infopane.go:84-228）是为了**定制行为**（命令行输入/历史/补全），不是为了实现接口合约

### 4.4 NotePane（我们自己的）—— 同 InfoPane 模式

`internal/action/notepane.go:22`：

```go
type NotePane struct {
    *BufPane           // ← 同样嵌入 BufPane 白嫖
    isOpen bool
    x, y, width, height int
    ...
}
```

结构和 InfoPane 几乎一模一样：满足 Pane 接口、不进 Tab.Panes、走 DoEvent 专门分支。当时设计没深入研究 micro 的机制，但**恰好踩在了 micro 浮层的标准范式上**（DoEvent 专门 if 分支 + 顶层独立 Display 行），这是幸运的——结构对，所以能用。

### 4.5 SelectPane（反例）—— 不实现 Pane 接口

`internal/action/selectpane.go`：只实现 5 个方法（Open/Close/IsOpen/HandleEvent/Display），不嵌入 BufPane，不实现 Pane 接口（5/16）。靠**宿主 pane 的 hook 转发**收事件和绘制（详见 §六）。

---

## 五、事件路由：DoEvent 的 if-else 树（关键）

`cmd/micro/micro.go` 的 `DoEvent()` 是整个 micro 的事件入口。它把事件**互斥地分发到三条独立分支**：

```go
// micro.go:539-553
if resize {
    action.InfoBar.HandleEvent(event)              // resize 通知所有人
    action.Tabs.HandleEvent(event)
    if action.TheNotePane != nil && action.TheNotePane.IsOpen() {
        action.TheNotePane.HandleEvent(event)
    }
} else if action.TheNotePane != nil && action.TheNotePane.IsOpen() {
    action.TheNotePane.HandleEvent(event)          // ← 分支 1：notePane 独占
} else if action.InfoBar.HasPrompt {
    action.InfoBar.HandleEvent(event)              // ← 分支 2：命令行 prompt 独占
} else {
    action.Tabs.HandleEvent(event)                 // ← 分支 3：常规路由 → Tab → active pane
}
```

**关键事实**：
- 同一个事件只会进其中**一条**分支，三条互斥。
- notePane 开着时，**主 BufPane 整条链路被跳过**（事件不进 Tab，bufpane.go:432 的 `HandleEvent` 不执行）。
- 这就是为什么 SelectPane 寄生在主 BufPane 的 hook 在 notePane 上下文里失效——事件根本流不到主 BufPane。

### 分支 3 的内部：Tab → active pane

`tab.go:337`：`t.Panes[t.active].HandleEvent(event)`。

只有 BufPane 和 TermPane 会进 Panes，所以分支 3 的终点永远是这两种瓦片窗。

---

## 六、绘制顺序：DoEvent 的顺序调用链

`micro.go:488-502`：

```go
screen.Screen.Fill(' ', config.DefStyle)
screen.Screen.HideCursor()
action.Tabs.Display()                            // ① 标签栏
for _, ep := range action.MainTab().Panes {
    ep.Display()                                 // ② 主编辑器各 pane（瓦片窗）
}
action.MainTab().Display()                       // ③ 当前 Tab 的边框
action.InfoBar.Display()                         // ④ 底部命令栏
if action.TheNotePane != nil && action.TheNotePane.IsOpen() {
    action.TheNotePane.Display()                 // ⑤ notePane —— 最后画，盖在最上层
}
screen.Screen.Show()                             // 最后 flush 到终端
```

**绘制顺序 = z-order**：越靠后画的越在上面。notePane 排在最后，所以盖在所有瓦片窗和 InfoBar 之上——这正是浮层该有的视觉效果。

InfoBar 没有 `IsOpen` 判断（它永远画），notePane 有（开的时候才画）。

### 关键启示：浮层必须排在绘制序列的末尾

任何想"盖在所有内容之上"的浮层（notePane、SelectPane、未来的别的），**它的 Display 必须排在所有底层 pane 之后**。否则会被后续的 pane 覆盖。

SelectPane 当前寄生在 `BufPane.Display` 内部（`bufpane.go:543`），画完后会被 notePane 覆盖——这就是 D15 要在 notePane.Display 末尾追加 SelectPane.Display 的根本原因。

---

## 七、Go 接口的关键概念：满足 ≠ 使用

这点容易混淆，必须澄清。

- **满足（satisfy）**：一个类型的方法集恰好包含接口要求的全部方法 → Go 编译器**隐式**认定它实现了接口（Go 没有 `implements` 关键字）。
- **使用（use）**：把该类型的值**真的赋给接口类型变量 / 当接口参数传 / 塞进接口切片** → 接口多态被实际触发的时刻。

### InfoPane / NotePane 卡在第一层

证据：

1. **全局变量是具体类型指针**（globals.go）：
   ```go
   var InfoBar      *InfoPane      // ← 不是 Pane
   var TheNotePane  *NotePane      // ← 不是 Pane
   ```
2. **从不进 Tab.Panes**：`AddPane` 全代码库只有两处调用（bufpane.go:679、693），全是 BufPane 自己。InfoPane/NotePane 从未被 AddPane。
3. **DoEvent 路由用的是具体类型方法**，没经过 Pane 接口：
   ```go
   action.InfoBar.Display()              // *InfoPane.Display()
   action.TheNotePane.HandleEvent(event) // *NotePane.HandleEvent()
   ```

它们"满足"Pane 接口只是因为嵌入了 `*BufPane`（Go 嵌入自动继承方法集），**是搭便车的副作用，不是有意设计**。它们真正想要的是 BufPane 的编辑能力（光标、buffer 操作），不是 Pane 的多态身份。

---

## 八、micro 窗口体系全景图

### 8.1 结构图

```
                     Pane 接口（HandleEvent + Display + 瓦片窗属性，~16 个方法）
                          │
            ┌─────────────┴──────────────────┐
            ▼                                ▼
   真正使用 Pane 接口语义               只是满足、从不使用（浮层）
   （进 Tab.Panes，多态分发）            （DoEvent 专门分支路由）
            │                                │
     ┌──────┴──────┐                ┌────────┼────────┐
     ▼             ▼                ▼        ▼        ▼
  BufPane      TermPane         InfoPane  NotePane  SelectPane
  (嵌入BWindow)(嵌入Window      (嵌入     (嵌入     (不满足 Pane
   +手写接口)   +手写接口)       *BufPane) *BufPane) 接口，靠 hook)
   瓦片窗       瓦片窗            浮层      浮层      浮层
```

### 8.2 四维分类表（最重要的一张表）

> ⚠️ 重要：**"实现方式"和"如何绘制"是两个正交维度，不要混淆**。详见 §8.3。
> - **实现方式**回答的是"这个 pane 类型的**方法集从哪来**"（嵌入白嫖 / 自己手写）
> - **如何绘制**回答的是"`Display()` 方法体里**画什么内容**"，这是业务层细节，不构成 pane 分类维度

| 对象 | 是 pane 吗 | 是 tile 吗 | 路由方式（事件怎么来） | 实现方式（方法集怎么来） |
|---|---|---|---|---|
| **BufPane** | ✅ | ✅ | Tab → active pane | 独立实现（嵌入 BWindow + 手写 Handler/ID/Name） |
| **TermPane** | ✅ | ✅ | Tab → active pane | 独立实现（嵌入 Window + 手写全部 Pane 方法） |
| **InfoPane** | ✅ | ❌ | DoEvent 专门分支 | 寄生（嵌入 *BufPane，白嫖 14 个方法） |
| **NotePane** | ✅ | ❌ | DoEvent 专门分支 | 寄生（嵌入 *BufPane，白嫖 14 个方法） |
| **SelectPane** | ❌（5/16） | ❌ | 宿主 pane 转发（hook） | 独立实现（不嵌入任何东西） |
| **StatusLine** | ❌ **根本不是 pane** | ❌ | **不收事件**（BufWindow 内部画） | BufWindow 子组件 |
| 滚动条 / ruler | ❌ 不是 pane | ❌ | 不收事件 | BufWindow 子组件 |

### 8.3 「实现方式」和「如何绘制」是正交的——关键反例

最容易混淆的认知：把"实现方式"当成"如何绘制"。下面这个反例最能说明问题。

**NotePane 和 InfoPane 的实现方式完全一样**（都嵌入 `*BufPane`），但**如何绘制完全不同**：

```go
// 实现方式相同：都嵌入 *BufPane
type NotePane struct { *BufPane; ... }
type InfoPane  struct { *BufPane; *info.InfoBuf }

// 但如何绘制完全不同：
func (n *NotePane) Display() { /* 重写：画 box-drawing 浮窗 + receiver 名字 + 内容 */ }
// InfoPane.Display 没重写 → 用 BufPane.Display → 画命令行 buffer 内容
```

如果"实现方式 = 如何绘制"，它俩绘制应该一样——但事实完全不同。**这反证了两个维度正交**。

### 8.4 "寄生"要分清是哪个维度

讨论浮层时"寄生"这个词容易混。其实有两个正交维度：

| 维度 | 含义 | 谁属于这一类 |
|---|---|---|
| **路由寄生** | 没有自己的事件入口，靠别人转发才能收事件 | 只有 **SelectPane** |
| **实现寄生** | 没自己写完整方法集，靠嵌入 `*BufPane` 白嫖实现 | **InfoPane、NotePane** |

两个维度组合出四象限：

```
                    路由独立              路由寄生
                 (DoEvent 专门分支)   (靠宿主转发)
            ┌─────────────────────┬───────────────────┐
实现独立    │ BufPane, TermPane   │   （micro 没有）  │
(自己写)    │ ← 真正的"独立 pane" │                   │
            ├─────────────────────┼───────────────────┤
实现寄生    │ InfoPane, NotePane  │   SelectPane      │
(嵌入BufPane)│ ← "半独立"          │ ← "双重寄生"      │
            └─────────────────────┴───────────────────┘
```

- **BufPane/TermPane**：唯一既路由独立、又实现独立的 pane（micro 里真正的"完整独立 pane"）。
- **InfoPane/NotePane**：**路由独立、实现寄生**。路由上有 DoEvent 专门分支直接投递事件，这点和 BufPane 一样独立；但实现上嵌入 `*BufPane` 白嫖。
- **SelectPane**：**路由也寄生、实现也独立**。不嵌入任何东西，但事件得靠宿主转发。

### 8.5 重要澄清：StatusLine 不是 pane

**StatusLine 是 BufWindow 的一个渲染子模块，和滚动条、ruler 同级，根本不是 pane。**

- 定义在 `internal/display/statusline.go:25`（display 包，不是 action 包）
- 是 BufWindow 的字段：`bufwindow.go:25` 的 `sline *StatusLine`
- **没有 `HandleEvent`**：statusline.go 里只有 `Display` / `FindOpt` / `resolvePlaceholder` / `parseFormat`，**完全不收事件**——连事件挂点都没有，谈何 pane
- 绘制是被动的：`BufWindow.Display` 内部调 `w.sline.Display()`（bufwindow.go:901），是 BufWindow 顺手画的，不是 DoEvent 顶层分发

### 8.6 两类浮层对比

| 浮层 | 顶层 DoEvent 认识它吗？ | 怎么收事件和绘制？ |
|---|---|---|
| **InfoPane / NotePane** | ✅ 认识（专门 if 分支 + 专门 Display 行） | 顶层直接分发 |
| **SelectPane** | ❌ 不认识 | 靠当前焦点 pane 在自己的 `HandleEvent` / `Display` 里**转发**给它 |

SelectPane 是浮层里的"二等公民"——它没有顶层分发，必须寄生在某个宿主 pane 上。代价是：每多一个可能的宿主，就得在那个宿主里补一次转发 hook（这正是 D15 在 notePane 里要做的事）。

---

## 九、对我们自己 pane 的设计启示

### 9.1 notePane：结构对，但身份没想清楚

notePane 当时设计没研究 micro 机制，但**结构恰好踩在了 micro 浮层的标准范式上**（嵌入 `*BufPane` + DoEvent 专门分支 + 顶层独立 Display），所以能用。

未来如果要优化，可以考虑：
- **不必纠结 Pane 接口**：notePane 的路由不靠 Pane 接口，靠 DoEvent 专门分支。维持现状（嵌入 BufPane 白嫖编辑能力）即可，没必要为了"更像 micro"而改成 TermPane 那样手写全套接口——那是浪费，notePane 又不进 Tab.Panes。
- **resize 处理**：DoEvent 的 resize 分支（micro.go:544）会调 notePane.HandleEvent，notePane 内部判断 resize 后调 reposition（notepane.go:605）。这个范式对，复用即可。

### 9.2 SelectPane：当前架构是合理的，但扩展性要权衡

当前 SelectPane 靠"宿主 pane 转发"工作（hook 点）。每加一个宿主，就要在那个宿主里补两行转发：

```go
// HandleEvent 顶部
if TheSelectPane != nil && TheSelectPane.IsOpen() {
    TheSelectPane.HandleEvent(event)
    return
}
// Display 末尾
if TheSelectPane != nil && TheSelectPane.IsOpen() {
    TheSelectPane.Display()
}
```

**这种"每个宿主自己声明愿意承载"的模式，是 micro 浮层模型的固有成本**，不是设计缺陷。D15 在 notePane 里镜像这两行，方向是对的。

### 9.3 何时该抽小接口（v1 不必做，但要知道这个选项）

如果将来浮层变多（SelectPane 之外再加搜索浮层、命令面板浮层等），可以把"转发目标"抽成一个**三方法小接口**：

```go
// 概念示意，micro 当前没有
type FloatingWidget interface {
    HandleEvent(tcell.Event)
    Display()
    IsOpen() bool
}
```

然后宿主的 hook 改成 `var activeWidget FloatingWidget`，转发统一指向它。但现在只有 SelectPane，抽这个属于 YAGNI（过早抽象），先按 D15 把第二个宿主接上再说。

### 9.4 为什么不要让 SelectPane 实现 Pane 接口

前面 §四 §七 已论证，归纳三条：

1. **13/16 个方法是无意义的桩**：SelectPane 是浮层不是瓦片窗，GetView/SetView/Resize/SetActive/IsActive/Relocate/LocFromVisual/SetTab/Tab/ID/SetID/Name/HandleCommand 全是瓦片窗属性，对它毫无意义。
2. **就算实现了也换不来自动路由**：DoEvent 分支 3（`Tabs.HandleEvent`）只能投到 Tab.Panes 里的 active pane，SelectPane 要走这条路就得塞进某个 Tab 的 Panes 切片，等于占一个分屏格子，彻底破坏"浮层"语义。notePane/InfoPane 是反证——它们满足了 Pane 接口，但路由照样走专门分支。
3. **签名匹配已经是足够的隐形合约**：SelectPane 实现的 `HandleEvent(tcell.Event)` 和 `Display()` 签名与 Pane 接口里的完全一致，任何 pane 都能用同样的两行代码转发给它。鸭子类型足够，不必显式声明。

### 9.5 决策树：新加一个 pane 时怎么选范式

```
新 pane 是瓦片窗吗？（占 Tab 分屏区、随布局缩放）
│
├─ 是 → 实现 Pane 接口（参照 BufPane 或 TermPane）
│        进 Tab.Panes，走 DoEvent 分支 3
│        例：如果以后要加"图片预览瓦片"
│
└─ 否 → 是浮层
         │
         ├─ 需要顶层直接路由吗？（像 notePane 那样独占事件）
         │   │
         │   ├─ 是 → 在 DoEvent 加专门 if 分支（参照 notePane/InfoBar）
         │   │        嵌入 *BufPane 白嫖编辑能力，或独立实现 HandleEvent/Display
         │   │        代价：改 micro 原生 DoEvent（cmd/micro/micro.go）
         │   │
         │   └─ 否 → 靠宿主 pane 转发（参照 SelectPane）
         │            不动 DoEvent，在每个潜在宿主里加两行 hook
         │            代价：宿主越多，hook 点越多
```

**对 microNeo 的原则**：项目原则是"对 micro 原生代码侵入越小越好"。所以：
- 能用"宿主转发"就不动 DoEvent（SelectPane 模式）
- 必须动 DoEvent 时（如 notePane 已有分支），尽量复用现有分支，不新增分支
- 永远不要让浮层进 Tab.Panes（破坏瓦片窗语义）

---

## 十、关键代码索引（快速查阅）

| 文件 | 行 | 内容 |
|---|---|---|
| `internal/action/pane.go` | 8 | `Pane` 接口定义 |
| `internal/action/events.go` | 185 | `Handler` 接口（HandleEvent 来源） |
| `internal/display/window.go` | 20 | `Window` 接口（Display 来源） |
| `internal/action/bufpane.go` | 206 | BufPane 结构体（嵌入 BWindow） |
| `internal/action/bufpane.go` | 432 | BufPane.HandleEvent（含 SelectPane 转发 hook） |
| `internal/action/bufpane.go` | 543 | BufPane.Display（含 SelectPane 叠加 hook） |
| `internal/action/termpane.go` | 53 | TermPane 结构体（嵌入 Window + 手写接口） |
| `internal/action/infopane.go` | 59 | InfoPane 结构体（嵌入 *BufPane） |
| `internal/action/notepane.go` | 22 | NotePane 结构体（嵌入 *BufPane） |
| `internal/action/notepane.go` | 604 | NotePane.HandleEvent |
| `internal/action/notepane.go` | 615 | NotePane.Display |
| `internal/action/selectpane.go` | 33 | SelectPane 结构体（不实现 Pane 接口） |
| `internal/action/tab.go` | 231 | Tab 结构体（含 `Panes []Pane`） |
| `internal/action/tab.go` | 337 | Tab 把事件投给 active pane |
| `internal/action/keytree.go` | 9-11 | 键绑定系统用 Pane 作参数类型 |
| `cmd/micro/micro.go` | 488-502 | DoEvent 绘制序列 |
| `cmd/micro/micro.go` | 539-553 | DoEvent 事件分发（if-else 三分支） |
| `internal/action/globals.go` | 5-22 | 全局 pane 变量（具体类型指针，非 Pane） |
| `internal/display/statusline.go` | 25 | StatusLine（不是 pane，是 BufWindow 子组件） |
| `internal/display/bufwindow.go` | 25 | BufWindow 持有 `sline *StatusLine` 字段 |
| `internal/display/bufwindow.go` | 901 | BufWindow.Display 内部画 StatusLine |

---

## 附：术语对照

本文档（及 agent-comm 系列 D 文档）用的行话 vs micro 代码里的正式术语：

| 行话（D 文档） | 正式术语 | 出处 |
|---|---|---|
| 事件挂点 | `HandleEvent` 方法（Handler 接口） | events.go:185 |
| 绘制挂点 | `Display` 方法（Window 接口） | window.go:20 |
| 独立路由 | DoEvent 的专门 if 分支 | micro.go:547-553 |
| hook 点 | 宿主 pane 方法体里转发给浮层的那几行 | bufpane.go:434 / 545 |
| 瓦片窗 | 进 Tab.Panes、被分屏布局管理的 pane | tab.go:237 |
| 浮层 | 不进 Tab.Panes、靠专门分支或宿主转发的 pane | ——（micro 无此词，本文定义） |
| 路由寄生 | 无事件入口、靠宿主转发才能收事件 | 本文定义（详见 §8.4） |
| 实现寄生 | 靠嵌入 `*BufPane` 白嫖方法集 | 本文定义（详见 §8.4） |
| 如何绘制 | `Display()` 方法体里画什么内容（业务层，非分类维度） | —— |

### 警惕的认知混淆

| 容易混成的等式 | 事实 |
|---|---|
| 实现方式 = 如何绘制 | ❌ 正交。NotePane 和 InfoPane 实现方式相同（都嵌入 BufPane），绘制完全不同 |
| 满足 Pane 接口 = 使用 Pane 接口 | ❌ 详见 §七。InfoPane/NotePane 满足但不使用 |
| StatusLine 是寄生的 pane | ❌ StatusLine 根本不是 pane（详见 §8.5） |
| 进 DoEvent 的浮层都靠 Pane 接口路由 | ❌ notePane/InfoBar 走具体类型方法，不经 Pane 接口（详见 §七） |
