# 我们的 pane 复用策略：notePane 与 selectPane

> 三段式研究的第三篇（前两篇见末尾）。
>
> 前两篇是**客观调研 micro 的机制**（micro 有什么 pane 框架 / micro 没有弹窗框架）。
> 本文是**主观反思我们自己的设计选择**——notePane 和 selectPane 在"复用 micro 已有框架"这件事上，为什么一个深、一个浅，各自是否选对了。
>
> 起因：连续几轮讨论澄清了几个关键认知（notePane 是"分身"不是"瘤子"、复用的是 BufPane 实现不是 Pane 接口、复用深度由能力重合度决定），值得沉淀。这些是前两篇分散提及但没讲透的点。

---

## 一、定位：三篇研究的关系

| 文档 | 性质 | 核心问题 |
|---|---|---|
| `研究-microPane机制.md` | 客观调研 | micro **有**什么 pane 框架？（答：tile 框架，Pane 接口） |
| `研究-micro弹窗机制.md` | 客观调研 | micro **没有**什么框架？（答：弹窗框架，连 YNPrompt 都是 InfoBar 假装的） |
| **本文** | 主观反思 | **我们自己**的 pane 复用对了吗？（答：notePane 深复用正确，selectPane 浅复用也正确） |

本文不重复前两篇的细节，只在需要时交叉引用。增量在三处：
1. **notePane 是 BufPane 的"分身"不是"瘤子"**（拓扑澄清，§三）
2. **复用的是 BufPane 编辑实现，不是 Pane 多态机制**（动机澄清，§三）
3. **复用深度 = 能力重合度**（统摄原则，§二、§六）

---

## 二、核心原则：复用深度 = 能力重合度

一句话：**一个 pane 该复用 micro 多深的框架，取决于它需要的能力和 micro 已有能力重合多少。**

- 重合多（如编辑能力）→ 深度复用（嵌入 BufPane 拿现成实现）
- 重合少（如列表选择、浮窗导航）→ 浅层复用（只对齐签名、借鉴路由范式）
- 不重合（如几何计算、z-order）→ 自己建，没有便车

这条原则比"用没用 Pane 接口"更本质。它解释了为什么 notePane 和 selectPane 走了两条不同的复用路径——不是因为"一个想用 micro、一个不想"，而是因为它们的能力需求不同。

---

## 三、notePane：BufPane 的"分身"（深复用）

### 3.1 先纠正一个易错的比喻：是"分身"不是"瘤子"

讨论中冒出过一个直觉说法："notePane 是长在某个主 BufPane 里的瘤子。" 这个比喻**方向对了一半，关键点反了**。

**对的那半**：notePane 确实不在 tile 树里。它是全局单例（`notepane.go:36` 的 `var TheNotePane *NotePane`，`globals.go:16` 初始化），从不在任何 Tab 的 `Panes []Pane` 里。

**反的那半**：notePane **不是附着在某个主 BufPane 身上**，它有**完全属于自己的三件套**。看构造函数 `NewNotePane()`（`notepane.go:338-359`）：

```go
func NewNotePane() *NotePane {
    n := &NotePane{height: 5}

    buf := buffer.NewBufferFromString("", "", buffer.BTScratch)   // ← 自己的 buffer（非主编辑器的）
    buf.SetOptionNative("ruler", false)

    win := display.NewBufWindow(0, 0, 80, n.height, buf)         // ← 自己的 BufWindow（非主编辑器的）
    win.SetHideStatusLine(true)

    n.BufPane = newBufPane(buf, win, nil)                        // ← 自己的 BufPane 内核（非引用主编辑器的）
    n.BufPane.bindings = NotePaneBindings

    return n
}
```

**notePane 和主编辑器的 BufPane 是两个各自独立、平等的 BufPane 实例**——不是"主 BufPane 身上长出来的"，是"另外独立存在的一个 BufPane"。所以更准确的比喻是：

| 比喻 | 是否准确 | 为什么 |
|---|---|---|
| ❌ 长在 BufPane 上的瘤子 | 不准 | 暗示附着在某个 BufPane 上、从属于它。实际是全局独立的另一个 BufPane 实例 |
| ✅ BufPane 的分身/克隆体 | 准 | 继承 BufPane 的"基因"（嵌入 `*BufPane` 拿到编辑能力），但有自己的身体（独立 buffer/window），没去 tile 树上户口 |

### 3.2 notePane 打开时只是"借坐标"，不从属主 pane

`reposition()`（`notepane.go:396`）打开/重定位时，会从主编辑器当前 pane **借**光标坐标来算自己画在哪：

```go
func (n *NotePane) reposition() {
    pane := MainTab().CurPane()           // ← 借主 pane 的存在
    ...
    view := pane.BWindow.GetView()        // ← 借主 pane 的视图矩形
    lowestRow := n.lowestCursorScreenRow(bw, view)  // ← 借主 pane 的光标行
    n.x = view.X
    n.width = view.Width
    ...
}
```

这是**借坐标定位**，不是**从属**。打开后你切到别的 pane，notePane 不会跟着旧 pane 走——它浮在屏幕上固定位置，等下次 reopen/reposition 才重新借坐标。

### 3.3 复用的是 BufPane 编辑实现，不是 Pane 多态机制

notePane 嵌入 `*BufPane` 的真正动机是什么？看它的 bindings 表（`notepane.go:48-62`）：

```go
var allowedNotePaneActions = map[string]bool{
    // Cursor movement
    "CursorUp": true, "CursorDown": true, "CursorLeft": true, "CursorRight": true,
    "CursorPageUp": true, "CursorPageDown": true,
    ...
    // Selection
    "SelectUp": true, "SelectDown": true, ...
    // Text editing
    ...
}
```

这张表证明了：**notePane 是真·编辑 pane**——它需要光标移动、文本选择、插入删除这些编辑能力。而 BufPane 恰好就是 micro 里"编辑能力"的载体。所以嵌入 `*BufPane` 的动机是**白嫖编辑实现**，不是"加入 Pane 多态俱乐部"。

| notePane 复用了什么 | 真正用了吗 | 有意为之吗 |
|---|---|---|
| **BufPane 的编辑实现**（cursor / buffer / 文本插入删除） | ✅ 真用了 | ✅ **有意**——嵌入的真正动机 |
| **Pane 接口的多态机制**（进 Tab.Panes、被多态分发） | ❌ 没用 | ❌ 没用——嵌入 BufPane 的副作用 |

notePane 从不进 Tab.Panes，DoEvent 用专门分支（`micro.go:547`）路由它，走的是具体类型方法 `*NotePane.HandleEvent`，不经 Pane 接口。**它满足 Pane 接口只是嵌入 BufPane 的副作用，无害，但不是它想要的。**

### 3.4 拓扑图：飘在 tile 树之外

```
tile 树（views.Node 分屏树，micro 管理）
├── Tab 1
│   ├── BufPane A（主编辑器，编辑 file.md）     ← 在树里、有户口
│   └── BufPane B（分屏出来的另一个）           ← 在树里、有户口
└── Tab 2
    └── TermPane（终端）                         ← 在树里、有户口

─────────── 以上是 tile 树 ───────────

notePane（TheNotePane，全局单例）               ← 不在树里、没户口
  └─ 内核：自己的 BufPane + 自己的 buffer + 自己的 window
     （BufPane 的"分身"，浮在 tile 树之上，和主 BufPane 是兄弟关系）
```

---

## 四、selectPane：纯选择浮窗（浅复用）

### 4.1 没有编辑功能 → 浅复用

selectPane 的本质是"选/不选，无输入"。它不需要光标、不需要 buffer、不需要文本编辑——**而这些恰恰是 BufPane 提供的东西**。能力和需求不重合，所以 selectPane 没有嵌入 BufPane 的动机。

结构体（`selectpane.go:33`）只持有列表选择相关字段：

```go
type SelectPane struct {
    items    []string
    selected int
    title    string
    onSelect func(*string)
    x, y, width, height int
    maxHeight int
    isOpen bool
}
```

没有 `*BufPane`，没有 buffer，没有 cursor。它是纯选择浮窗。

### 4.2 仍搭了两层轻形态（不是零利用）

虽然用不上 Pane 接口和 tile 树（这两块详见 `研究-microPane机制.md` §9.4），selectPane 仍然搭了 micro 的两层**很薄**的形态——这正是"复用深度 = 能力重合度"原则的体现：重合少，所以浅。

| 层级 | selectPane 用了吗 | 是什么 |
|---|---|---|
| **Pane 接口 / tile 树 / 多态分发** | ❌ 没用 | 这是"基本用不上"的部分 |
| **方法签名约定** | ✅ 用了 | `HandleEvent` + `Display` 签名让任何 pane 能用同样两行转发给它（鸭子类型红利） |
| **路由范式** | ✅ 用了 | "宿主 pane 转发"模式（bufpane.go:434/545 的 hook 是 micro 已有的形态） |

这两层很薄（只是"签名"和"模式"，不是"基础设施"），但确实省了一点事——selectPane 不用自己发明"宿主怎么把键转给我"的协议，照着 micro 已有的 hook 形态写就行。

---

## 五、notePane vs selectPane 全维度对比

| | notePane | selectPane |
|---|---|---|
| 本质 | 编辑 pane（输入文本） | 选择浮窗（选/不选，无输入） |
| 需要的能力 | 光标 / buffer / 文本编辑 | 列表渲染 / 键导航 / 回调 |
| 和 micro 能力重合度 | 高（编辑是 BufPane 的主业） | 低（micro 没有列表选择浮窗） |
| 复用深度 | **深**（嵌入 `*BufPane`，白嫖 14 个方法） | **浅**（不嵌入任何东西，只对齐签名） |
| 是 BufPane 的分身吗 | ✅ 是（克隆了 BufPane 当编辑内核） | ❌ 不是（没有 BufPane 血统） |
| 有自己的 buffer 吗 | ✅ 有（scratch buffer） | ❌ 没有（只有 items 列表） |
| 全局单例吗 | ✅ 是（`TheNotePane`） | ✅ 是（`TheSelectPane`） |
| 在 tile 树里吗 | ❌ 不在 | ❌ 不在 |
| 怎么收事件 | DoEvent 专门分支（`micro.go:547`） | 宿主 pane 转发（hook） |
| 怎么绘制 | DoEvent 顶层独立 Display 行（`micro.go:500`） | 宿主 pane Display 末尾追加 |
| 设计选择对吗 | ✅ 对（深复用是正确选择） | ✅ 对（浅复用也是正确选择） |

**最关键的一行是"复用深度"**——它不是任意选的，而是由"能力重合度"决定的。notePane 需要 BufPane 擅长的编辑能力，所以深复用；selectPane 不需要，所以浅复用。两者各自适应了自己的需求。

---

## 六、决策原则：未来新 pane 怎么选

把"复用深度 = 能力重合度"原则落地成一个简单流程：

### 6.1 三步判断

```
第 1 步：新 pane 需要什么能力？
  （列出来：编辑？列表？导航？渲染某种内容？）

第 2 步：micro 已有的东西里，有没有现成的？
  ├─ BufPane → 编辑能力（cursor / buffer / 文本操作）
  ├─ display 包各种 Window → 内容渲染（buf / term / tab）
  ├─ views 包 → 分屏布局
  └─ 其它（弹窗几何 / z-order / 事件隔离）→ 没有，得自建

第 3 步：复用深度 = 重合度
  ├─ 重合度高 → 深复用（嵌入对应类型，白嫖实现）
  │             例：notePane 嵌入 *BufPane
  ├─ 重合度低但形态对得上 → 浅复用（对齐签名 + 借鉴路由范式）
  │             例：selectPane 用 HandleEvent/Display 签名 + 宿主转发模式
  └─ 不重合 → 自建（没有便车可搭）
               例：浮窗几何计算、z-order、事件隔离
```

### 6.2 几个反模式的提醒

- ❌ **为了"更像 micro"而实现 Pane 接口**：Pane 接口是 tile 专属抽象，浮窗实现它只会得到 13/16 个无意义的方法桩（详见 `研究-microPane机制.md` §9.4）。
- ❌ **把浮窗塞进 Tab.Panes**：这会破坏浮层语义（占一个分屏格子），且换不来自动路由（详见 `研究-microPane机制.md` §9.4 第 2 条）。
- ❌ **期待 micro 提供"弹窗框架"**：micro 在弹窗领域没有基础设施（详见 `研究-micro弹窗机制.md`），几何/z-order/事件隔离都得自己建。
- ❌ **把"嵌入 BufPane"等同于"加入 Pane 多态机制"**：嵌入 BufPane 拿的是编辑实现，不是多态身份。notePane 满足 Pane 接口是副作用，不是目的。

---

## 附：与前两篇的分工

| 文档 | 用本文的话概括 |
|---|---|
| `研究-microPane机制.md` | 解释了 micro 的 tile 框架，论证了为什么 selectPane 不该实现 Pane 接口（§9.4） |
| `研究-micro弹窗机制.md` | 解释了 micro 没有弹窗框架，论证了为什么 selectPane 的几何/z-order 得自建 |
| **本文** | 解释了 notePane 和 selectPane 为什么复用深度不同（能力重合度），以及为什么两者都选对了 |

三篇合起来回答一个完整问题：**"我们自己的 pane，该怎么对待 micro 的框架？"**
- 看需要什么能力（本文 §二、§六）
- 看 micro 有没有（前两篇）
- 能力重合度决定复用深度（本文核心原则）
