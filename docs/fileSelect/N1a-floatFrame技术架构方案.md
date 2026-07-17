# floatFrame 技术架构方案

**状态**：架构阶段（N0b 序列）。在 N0a 产品方案之上，给出可落地的数据结构、接口、迁移步骤。落到代码前需按 §九 三步走拆 PR。

**范围**：解决 N0a §五「deferred 到技术架构阶段」的三项架构选择（数据结构 / Close 接口 / 字段命名与错误处理），并把 N0a §七「演进路径提案」从示意图升级为可执行改造步骤。NotePane、FileManager、未来 widget 抽象的归属判定引用 N0a，本文不重新讨论。

**与 N0a 的关系**：N0a 是产品规则与架构原则；本文是架构实现。N0a §五「产品规则 vs 架构实现」划界表里的 A/B/C 三行由本文落地。N0a §七的「提案」字样在本文生效后变成「计划」。

---

## 一、一句话

每个 `*BufPane` 自带一个 `floatFrame *FloatFrame` 字段（per-owner），全局 `TheFloatFrame` 删除；widget 的 `Open` 接收 owner 的 `*FloatFrame` 作为参数；事件由 owner 的 `HandleEvent` 头部转发，渲染由 owner 的 `Display` 末尾追加；focus 转移（A1）在 `Tab.SetActive` / `TabList.SetActive` 的源头关闭老 owner 的 widget；resize 由主循环 resize 分支直接路由到 active pane 的 floatFrame。`FloatFrame.Close(cause)` 用一个 `CloseCause` 枚举让 widget 在被外部关闭时区分 Resize / Focus。

---

## 二、现状速览（事实清单）

事实来源是当前 `main` 分支代码，不是 N0a 的描述。N0a 里凡是带「待定」「示意图」字样的，本文都给出定论。

### 2.1 FloatFrame 容器本身（`internal/action/floatframe.go`）

- 全局单例：`var TheFloatFrame *FloatFrame`（line 70），在 `globals.go:17` 的 `InitGlobals` 初始化
- `Open(FloatOpenSpec) bool`：失败前置检查（同时仅允许一个浮窗 / 屏幕放不下）；`AutoExpand` 分叉布局
- `Close()`：幂等清空 + `screen.Redraw()`，**不接收原因参数**
- `Display()`：HideCursor → 清外矩形 → 边框 + title → 委托 `display(contentArea)`
- `HandleEvent(event)`：resize 拦截（`Close()` + 触发 `OnResize` 回调）；其余转发给 `handleEvent`

### 2.2 主循环接入点（`cmd/micro/micro.go` 的 `DoEvent`）

| 阶段 | 行号 | 当前代码 |
|---|---|---|
| Render | 536-538 | `if TheFloatFrame.IsOpen() { TheFloatFrame.Display() }`（所有层最后画） |
| Dispatch（resize 分支） | 585-586 | resize 广播四方，含 `TheFloatFrame.HandleEvent(event)` |
| Dispatch（非 resize） | 587-588 | `else if TheFloatFrame.IsOpen() { TheFloatFrame.HandleEvent(event) }`（最高优先级） |

### 2.3 调用点（widget 实际使用 TheFloatFrame 的地方）

```
TheFloatFrame.Open(spec)     ← selectpane.go:88, fileselector.go:197
TheFloatFrame.Close()        ← selectpane.go:181,188; fileselector.go:838
```

widget 创建点（业务侧发起方）：

| 调用点 | owner | widget |
|---|---|---|
| `command_neo.go:70`（`:theme`） | BufPane | SelectPane |
| `filemanager.go:65`（birth selector） | BufPane（新 pane） | FileSelector |
| `filemanager.go:83`（browse selector） | BufPane | FileSelector |
| `filemanager.go:102`（quit selector） | BufPane | FileSelector |
| `notepane.go:167`（receiver 切换，case 2+） | NotePane（嵌入 BufPane） | SelectPane |
| `notepane.go:247`（receiver 列表，缓存未命中） | NotePane（嵌入 BufPane） | SelectPane |

6 个调用点全部走 BufPane 家族。InfoPane / RawPane / TermPane 当前不使用 FloatFrame。

### 2.4 派发优先级（当前四层）

```
resize       → 广播：InfoBar / Tabs / NotePane? / FloatFrame?
非 resize    → FloatFrame? > NotePane? > InfoBar.HasPrompt > Tabs（→ Tab → BufPane）
```

### 2.5 渲染顺序（当前）

```
Fill(' ') → Tabs.Display → 每个 pane.Display → MainTab().Display（divider）
         → InfoBar.Display → NotePane?.Display → FloatFrame?.Display → Show()
```

FloatFrame 最后画（最浮），NotePane 次浮。

### 2.6 现有 close 信号

- SelectPane：`onSelect func(*string)`，`nil` = 取消（不区分原因）
- FileSelector：`onSelect func(SelectResult)`，`SelectResult{Kind, Path, Reason}`，`Reason` ∈ {Esc, Quit, Size, Resize}
- FloatFrame：`OnResize func()`，仅 resize 自关时触发，不带原因参数

---

## 三、架构决策（ADR 总览）

每条都是 N0a §五 A/B/C 列表里的具体落地。 rejected alternative 写在每条末尾，避免日后被「优化」回反方向。

### ADR-1：per-owner 字段，不做全局路由表 / modal stack

**决策**：`*BufPane` 加 `floatFrame *FloatFrame` 字段，在 `newBufPane` 初始化。所有路由（事件、渲染、close）通过 owner 持有的字段完成。

**理由**：
- N0a §2.1 第 5 条的根本架构原则
- focus 转移语义（A1）天然成立：owner 失活 → 字段跟着 owner 走，不需要全局协调
- 多 pane 场景正确：pane A 的 widget 不会"穿透"到 pane B 的区域
- 与 NotePane 的"全局单例"路径不冲突——NotePane 走另一套（自管 modal），本来就不该共用 FloatFrame 的容器抽象

**Rejected**：
- **全局 var + 路由表**（`TheFloatFrame` 保留 + 维护 owner→frame 映射）：路由表是新的全局可变状态，焦点转移要查表、清表项，比 per-owner 字段多一层间接，没有额外收益
- **modal stack**（全局栈，支持嵌套 open）：直接违反 N0a §2.5「widget 不嵌套」，且 A1+A2 已保证全局单实例，栈是过度设计

### ADR-2：NotePane 通过嵌入继承字段，不另写一份

**决策**：`*NotePane` 嵌入 `*BufPane`（`notepane.go:22`），自动继承 `floatFrame` 字段。NotePane 的 `HandleEvent` 已经委托给 `n.BufPane.HandleEvent`（`notepane.go:681-687`），所以 BufPane 头部的 floatFrame 拦截对 NotePane 自动生效。NotePane 的 `Display` 是自绘的，**单独加一行** overlay（见 §五.3）。

**理由**：
- 现实：NotePane 当前就用 SelectPane（`notepane.go:167,247`），是事实上的 owner
- 嵌入是 Go 的语言机制，不写代码就拿到字段，最省事
- N0a §3.6 写「NotePane 不加」是产品层视角（NotePane 跟 FloatFrame 概念独立），不是禁止 NotePane 用 BufPane 的字段——产品独立 ≠ 代码不共享底层

**Rejected**：
- 给 NotePane 单独声明一个 `floatFrame`：字段重复，HandleEvent 还得另写一份拦截逻辑，违反「如非必要勿增实体」
- 把 NotePane 的两个 SelectPane 调用迁回主编辑器 BufPane：语义错位（receiver 选择是 NotePane 的功能，不该挂在底下的 BufPane 上）

### ADR-3：`Close(cause CloseCause)`，外部关闭走回调

**决策**：FloatFrame 的关闭接口升级为：

```go
type CloseCause uint8

const (
    CloseByUser   CloseCause = iota  // widget 自己关（Esc / picked / Ctrl-q 等）
    CloseByResize                     // resize 导致 FloatFrame 自关
    CloseByFocus                      // A1：focus 转移导致 FloatFrame 被关
)

func (f *FloatFrame) Close(cause CloseCause)
```

`FloatOpenSpec` 把现有 `OnResize func()` 升级为 `OnExternalClose func(CloseCause)`，覆盖 Resize 和 Focus 两种外部关闭。widget 自己发起的关闭（`Close(CloseByUser)`）**不**触发任何 FloatFrame 侧的回调——widget 自己已经在调 `Close` 之前/之后处理了自己的 `onSelect`。

**理由**：
- Resize 和 Focus 都是"widget 被动关闭"，语义同源，合并成一个回调 + cause 参数比平行两个回调（OnResize + OnFocus）更易扩展
- widget 自己的关闭原因（Esc / picked / Ctrl-q）由 widget 自己枚举，FloatFrame 不需要知道——`CloseByUser` 是个统称的"非外部"标记，FloatFrame 看到它就不触发回调
- FileSelector 现有的 `OnResize` 闭包迁移成 `OnExternalClose` 后，Resize 分支行为零变化，Focus 分支是新增能力

**Rejected**：
- 保留 `OnResize` + 新增 `OnFocus`（两个平行回调）：日后若再加第三种外部原因（如 `CloseByOwnerShutdown`），回调列表继续膨胀；cause 参数一次到位
- 让 FloatFrame.Close 直接调 widget 的 onSelect：FloatFrame 不该知道 widget 的回调签名（`func(*string)` vs `func(SelectResult)`），耦合方向反了

### ADR-4：widget.Open 接收 owner 的 `*FloatFrame` 作为显式参数

**决策**：所有 widget 的 `Open` 方法签名加一个 `ff *FloatFrame` 首参，widget 把它存为字段，后续的 `Close` / `IsOpen` 都用这个字段。调用方负责传 `h.floatFrame`。

```go
// SelectPane
func (s *SelectPane) Open(ff *FloatFrame, items []string, title string, ...)

// FileSelector
func (fs *FileSelector) Open(ff *FloatFrame, pane *BufPane, startDir string, onSelect func(SelectResult))
```

**理由**：
- 显式参数 > 隐式全局。widget 不再依赖 `TheFloatFrame` 这个包级 var，可独立测试（传 stub frame）
- 调用方一定持有 owner（否则没法决定在哪开），手上自然有 `h.floatFrame`，传参零负担
- 与 N0 文件管理器独立成包方案的 `Host` 接口同构——以后 file 包化时 `*FloatFrame` 自然演化成 `Host` 接口，迁移路径连续

**Rejected**：
- widget 内部仍读全局 `TheFloatFrame`：违反 per-owner 原则，且无法支持多 owner
- 在 FloatOpenSpec 里塞 `Owner` 字段：把 owner 暴露给 FloatFrame，违反 N0a §2.1「FloatFrame 不知道 owner 是谁」
- 让 widget 返回 `FloatOpenSpec`，由 owner 调 `ff.Open(spec)`：widget 失去对自身生命周期的控制（关闭时机、Close 顺序），现状的"widget 自己关自己"模式被打破

### ADR-5：A1 触发点放在 `Tab.SetActive` 与 `TabList.SetActive` 源头，不放在 `BufPane.SetActive`

**决策**：在两个"active index 真正改变"的入口加 hook，对比 old vs new active，关闭 old 的 widget。

```go
// Tab.SetActive（pane 间切换）
func (t *Tab) SetActive(i int) {
    if i != t.active {
        t.closeActiveFloatFrame(CloseByFocus)  // 关老 active pane 的 widget
    }
    t.active = i
    // ... 原循环不变
}

// TabList.SetActive（tab 间切换）
func (t *TabList) SetActive(a int) {
    if a != t.active {
        t.closeActiveTabFloatFrame(CloseByFocus)  // 关老 tab 的 CurPane 的 widget
    }
    t.TabWindow.SetActive(a)
    // ... 原循环不变
}
```

辅助方法集中在 tab.go：

```go
// closeActiveFloatFrame 关本 tab 当前 active pane 的 widget（若有）。
func (t *Tab) closeActiveFloatFrame(cause CloseCause) {
    if t.active < 0 || t.active >= len(t.Panes) { return }
    if bp, ok := t.Panes[t.active].(*BufPane); ok && bp.floatFrame.IsOpen() {
        bp.floatFrame.Close(cause)
    }
}

// closeActiveTabFloatFrame 关本 TabList 当前 active tab 的 CurPane 的 widget（若有）。
func (t *TabList) closeActiveTabFloatFrame(cause CloseCause) {
    if t.active < 0 || t.active >= len(t.List) { return }
    if bp := t.List[t.active].CurPane(); bp != nil && bp.floatFrame.IsOpen() {
        bp.floatFrame.Close(cause)
    }
}
```

**理由**：
- `Tab.SetActive` 和 `TabList.SetActive` 是仅有的两个"active 索引变更"入口，hook 在这里覆盖全部分支（鼠标点 pane、Ctrl-w、点 tab bar、关 tab 后回退、`NextSplit` 等）
- 在源头对比 old vs new 是 O(1) 检查，比"在 BufPane.SetActive(false) 里检查"覆盖更广（TabList.SetActive 现在不调用 BufPane.SetActive，仅在 Tab 上设 `isActive` 标记——见 `tab.go:152-167`）
- NotePane 是全局单例、不在任何 Tab.Panes 里，A1 触发不到它，符合 N0a §4「NotePane 独立演进」

**Rejected**：
- 在 `BufPane.SetActive(false)` 里 hook：覆盖不到 tab 切换（TabList.SetActive 不调用它），需要再补一个 TabList hook，hook 数量翻倍且语义重叠
- 在主循环 dispatch 里检测"active 变了"：主循环每帧对比上一帧 active，引入帧间状态，复杂且易漏
- 用 plugin hook（`onSetActive`）：plugin 看到的是新 pane，且 plugin 机制对内部架构改动过重

### ADR-6：resize 路径保留主循环的一行显式路由

**决策**：主循环 resize 分支把 `TheFloatFrame.HandleEvent(event)` 替换为对 active pane 的 floatFrame 的直接路由：

```go
if resize {
    action.InfoBar.HandleEvent(event)
    action.Tabs.HandleEvent(event)  // TabList → t.Resize()（重排，不转发到 BufPane.HandleEvent）
    if action.TheNotePane != nil && action.TheNotePane.IsOpen() {
        action.TheNotePane.HandleEvent(event)
    }
    // microNeo: resize 直达 active pane 的 floatFrame
    if bp := action.MainTab().CurPane(); bp != nil && bp.floatFrame.IsOpen() {
        bp.floatFrame.HandleEvent(event)
    }
}
```

非 resize 分支：`TheFloatFrame` 那行整体删除，事件经 `Tabs.HandleEvent → Tab → BufPane.HandleEvent` 自然到达 active pane，由 BufPane 头部拦截转发到自己的 floatFrame。

**理由**：
- `TabList.HandleEvent` 对 resize 的处理是 `t.Resize()` 后 return（`tab.go:104-105`），事件**不会**转发到 BufPane.HandleEvent；如果不在主循环显式路由，resize 永远到不了 active pane 的 floatFrame
- 这一行是"主循环知道 FloatFrame 存在"的唯一残留，但与 NotePane 的处理方式同构（`if TheNotePane.IsOpen() { TheNotePane.HandleEvent(event) }`），主循环本就为顶层 modal 保留这种位置
- 与 N0a §3.6「root Render / Dispatch 替代表」一致——那张表只针对**非 resize** 分支列出替代方案；resize 是 broadcast，本身就是主循环的事

**Rejected**：
- 修改 `TabList.HandleEvent` 让 resize 转发到 active pane 的 HandleEvent：侵入 micro 原生事件流，BufPane.HandleEvent 现在没有 `EventResize` 分支，转发后是 no-op，看似无害但语义混乱（"TabList 替我转发"），未来 micro 升级易冲突
- 让每个 pane 在自己的 Resize 回调里检查 widget：Resize 是 `BWindow` 层的方法，pane 没有干净的"我刚被 resize 了"钩子；且 inactive pane 也会被 resize，会有误关

### ADR-7：渲染——主循环末尾追加 active pane 的 floatFrame overlay

**决策**：BufPane.Display() **不**画自己的 floatFrame。主循环 Render 阶段在画完所有 pane + divider + InfoBar + NotePane 之后，追加一行：

```go
for _, ep := range action.MainTab().Panes {
    ep.Display()
}
action.MainTab().Display()  // split divider
action.InfoBar.Display()
if action.TheNotePane != nil && action.TheNotePane.IsOpen() {
    action.TheNotePane.Display()
}
// microNeo: active pane 的 floatFrame 作为最末 overlay
if bp := action.MainTab().CurPane(); bp != nil && bp.floatFrame.IsOpen() {
    bp.floatFrame.Display()
}
screen.Screen.Show()
```

**理由**：
- widget 可超出 owner pane 的视觉区域（SelectPane 的 `AutoExpand=true` 路径，如 `:theme` 锚点 `(0, -1)`、NotePane receiver 锚点 `(n.x, n.y)`），若在 BufPane.Display() 内画，可能被后渲染的 pane / divider / InfoBar 覆盖
- "全局任何时刻最多一个 widget 可见"（N0a §2.4）→ 主循环画一次就够，不需要每个 pane 各画各的
- 与 ADR-6 对称：resize / 渲染都在主循环用 `MainTab().CurPane().floatFrame` 这一统一寻址方式
- 与 NotePane 的画法同构（都是 `if <owner>.IsOpen() { <owner>.Display() }`），主循环的 modal overlay 习惯不变

**与 N0a §3.6 的差异**：N0a §3.6 的 Render 替代表写「BufPane.Display() 末尾：`if b.floatFrame.IsOpen() { b.floatFrame.Display() }`」。本方案改成「主循环末尾」——见上「widget 可超出 owner 区域」的实操问题。N0a 那行是示意图（N0a §五明确说「示意图，不是定论」），本方案是落地修正。**owner-local 的核心（字段归属、事件路由）不变**；只有渲染这一个动作回到主循环，因为渲染顺序是全局视野的问题，本来就该主循环管。

**Rejected**：
- 真的按 N0a §3.6 字面在 BufPane.Display() 末尾画：必须额外保证 active pane 是 `Panes` slice 里最后渲染的那个，要改 micro 的渲染顺序（`for _, ep := range MainTab().Panes` 改成 active 最后），侵入面比主循环加一行大得多
- 每个 pane 各画各的 floatFrame：违反"全局最多一个 widget"，多份检查冗余且解决不了覆盖问题

---

## 四、数据结构定义

### 4.1 FloatFrame（`floatframe.go`）

字段不变，新增 `onExternalClose` 替代 `onResize`：

```go
type FloatFrame struct {
    isOpen bool

    // —— 从 widget 传入（纯内容语义，不含边框）——
    anchor      Pos
    contentSize Size
    title       string
    frameColor  tcell.Style
    display     func(contentArea Rect)
    handleEvent func(event tcell.Event)
    onExternalClose func(CloseCause)  // 替代原 onResize；仅外部关闭（Resize/Focus）触发

    // —— FloatFrame 自己算 ——
    outerW, outerH int
    fx, fy         int
}
```

`TheFloatFrame` 全局变量删除（同时删 `globals.go:17` 的初始化行）。

### 4.2 FloatOpenSpec

```go
type FloatOpenSpec struct {
    Anchor      Pos
    ContentSize Size
    Title       string
    FrameColor  tcell.Style
    Display     func(contentArea Rect)
    HandleEvent func(event tcell.Event)
    OnExternalClose func(CloseCause)  // 替代 OnResize；cause ∈ {CloseByResize, CloseByFocus}
    AutoExpand  bool
}
```

`OnResize func()` 字段移除。所有现有 widget 的 `OnResize` 闭包迁移为 `OnExternalClose`（见 §八）。

### 4.3 CloseCause 枚举（新增）

```go
type CloseCause uint8

const (
    CloseByUser CloseCause = iota  // widget 自己关；FloatFrame 不触发回调
    CloseByResize                   // 主循环 resize 分支调 HandleEvent 时自关
    CloseByFocus                    // A1：Tab/TabList SetActive 关老 owner widget
)
```

### 4.4 BufPane（`bufpane.go`）

```go
type BufPane struct {
    display.BWindow
    // ... 现有字段不变
    floatFrame *FloatFrame  // microNeo: per-owner 浮窗容器；NewFloatFrame 在 newBufPane 注入，永不 nil
}
```

初始化：

```go
func newBufPane(buf *buffer.Buffer, win display.BWindow, tab *Tab) *BufPane {
    h := new(BufPane)
    h.Buf = buf
    h.BWindow = win
    h.tab = tab
    h.Cursor = h.Buf.GetActiveCursor()
    h.mousePressed = make(map[MouseEvent]bool)
    h.floatFrame = NewFloatFrame()  // 新增
    return h
}
```

所有 BufPane 创建路径都过 `newBufPane`（`NewBufPane` / `NewBufPaneFromBuf` / NotePane 的 `newBufPane(buf, win, nil)`），一处初始化全覆盖。

### 4.5 SelectPane（`selectpane.go`）

```go
type SelectPane struct {
    ff         *FloatFrame  // owner 的 frame（Open 入参注入）
    items      []string
    selected   int
    topIdx     int
    maxVisible int
    wrap       bool
    title      string
    onSelect   func(*string)
}
```

### 4.6 FileSelector（`fileselector.go`）

```go
type FileSelector struct {
    ff       *FloatFrame  // owner 的 frame（Open 入参注入）
    state    *fileSelectorState
    pane     *BufPane
    onSelect func(SelectResult)
}
```

FileSelector 现有的 `SelectResult` / `ResultKind` / `CloseReason`（含 ReasonEsc / ReasonQuit / ReasonSize / ReasonResize）**不变**。新增一个 `ReasonFocus`（值类型对齐现有 CloseReason）：

```go
const (
    ReasonEsc    CloseReason = iota
    ReasonQuit
    ReasonSize
    ReasonResize
    ReasonFocus  // 新增：A1 focus 转移（外部关闭）
)
```

`OnSelect` 分流矩阵（F1 §3.2）扩一列：

| 入口 | ReasonFocus（新） | 说明 |
|---|---|---|
| quit (Ctrl-q) | 取消（回编辑） | focus 走了，等价 Esc |
| browse (Ctrl-o) | 取消（回编辑） | 同上 |
| birth (spawn) | 取消（回编辑） | 同上 |

三入口对 ReasonFocus 行为一致：取消。语义上等价 Esc——用户的"注意力"离开了，selector 没必要继续占用。原 F1 §3.2 矩阵的其他列不变。

---

## 五、渲染流程

### 5.1 主循环 Render 阶段（`micro.go:511-538`）

按 ADR-7 改造。完整顺序：

```go
screen.Screen.Fill(' ', config.DefStyle)
screen.Screen.HideCursor()
action.Tabs.Display()
for _, ep := range action.MainTab().Panes {
    ep.Display()
}
action.MainTab().Display()
action.InfoBar.Display()
if action.TheNotePane != nil && action.TheNotePane.IsOpen() {
    action.TheNotePane.Display()
}
// microNeo: active pane 的 floatFrame 作为最末 overlay
if bp := action.MainTab().CurPane(); bp != nil && bp.floatFrame.IsOpen() {
    bp.floatFrame.Display()
}
screen.Screen.Show()
```

删除原 536-538 行的 `if TheFloatFrame.IsOpen() { TheFloatFrame.Display() }`。

### 5.2 BufPane.Display 不动

`BufPane` 现在没有自己的 `Display` 方法（继承自嵌入的 `display.BWindow`）。**保持现状，不新增 `(*BufPane).Display`**。floatFrame 的渲染责任完全在主循环。

这是 ADR-7 的直接结果：渲染顺序是全局视野，主循环管。

### 5.3 NotePane.Display 末尾追加 overlay（`notepane.go:692`）

NotePane 的 `Display` 是自绘的（画自己的 border + 内容），目前不画 floatFrame。由于 SelectPane 会在 NotePane 上弹出（`notepane.go:167,247`），NotePane 必须在自己的 Display 末尾补 overlay：

```go
func (n *NotePane) Display() {
    if !n.isOpen { return }
    // ... 现有 border + 内容绘制不变

    // microNeo: NotePane 上的 widget overlay（receiver 选择 SelectPane 用）
    if n.floatFrame.IsOpen() {
        n.floatFrame.Display()
    }
}
```

注意：NotePane 的 floatFrame 与主编辑器 BufPane 的 floatFrame 是**两个不同实例**（各自的字段）。NotePane 开着的时候，主循环只画 NotePane（不画 MainTab().Panes 的 floatFrame——因为 NotePane.Display 在主循环里被调用，而 MainTab().Panes 的 floatFrame 由主循环最后一行画，这一行会用 `MainTab().CurPane()` 找到主编辑器 pane，而不是 NotePane）。

这意味着：**NotePane 开着 + 主编辑器 pane 有 widget** 的组合下，主编辑器的 widget 会画在 NotePane 之上（主循环最后一行）。但 N0a §2.2 A1+A2 保证全局最多一个 widget，所以这种组合实际不会发生——主编辑器 pane 想开 widget 时，如果 NotePane 已开，A1 早就把主编辑器的 widget 关了。反之亦然。

### 5.4 渲染顺序小结

| 层 | 由谁画 | 何时画 |
|---|---|---|
| 主编辑器 pane 内容 | 各 BufPane.Display（嵌入 BWindow.Display） | 主循环 for 循环 |
| split divider | MainTab().Display | for 循环后 |
| InfoBar | InfoBar.Display | divider 后 |
| NotePane（border + 内容 + NotePane 自己的 widget overlay） | NotePane.Display | InfoBar 后 |
| 主编辑器 active pane 的 widget overlay | 主循环最后一行 | NotePane 后 |

---

## 六、事件分发

### 6.1 主循环 Dispatch 阶段（`micro.go:580-594`）

按 ADR-6 改造。完整逻辑：

```go
if event != nil {
    _, resize := event.(*tcell.EventResize)

    if resize {
        action.InfoBar.HandleEvent(event)
        action.Tabs.HandleEvent(event)
        if action.TheNotePane != nil && action.TheNotePane.IsOpen() {
            action.TheNotePane.HandleEvent(event)
        }
        // microNeo: resize 直达 active pane 的 floatFrame
        if bp := action.MainTab().CurPane(); bp != nil && bp.floatFrame.IsOpen() {
            bp.floatFrame.HandleEvent(event)
        }
    } else if action.TheNotePane != nil && action.TheNotePane.IsOpen() {
        action.TheNotePane.HandleEvent(event)
    } else if action.InfoBar.HasPrompt {
        action.InfoBar.HandleEvent(event)
    } else {
        action.Tabs.HandleEvent(event)
    }
}
```

删除原 587-588 行的 `else if TheFloatFrame.IsOpen() { ... }` 分支。

### 6.2 BufPane.HandleEvent 头部拦截（`bufpane.go:436`）

```go
func (h *BufPane) HandleEvent(event tcell.Event) {
    // microNeo: widget modal 拦截——本 pane 的 floatFrame 开着时事件直达 widget
    if h.floatFrame.IsOpen() {
        h.floatFrame.HandleEvent(event)
        return
    }

    // ... 原有逻辑（ExternallyModified 检查、EventRaw / EventPaste / EventKey / EventMouse 分发）不变
}
```

放在最前面，**早于** `ExternallyModified` 检查——widget 期间不该弹"文件已被外部修改"。

### 6.3 NotePane.HandleEvent 自动继承（`notepane.go:681`）

NotePane.HandleEvent 已经委托 `n.BufPane.HandleEvent(event)`，头部拦截对 NotePane 自动生效。无需改动。

NotePane 自己的 resize 分支（`notepane.go:682-686`，调 `n.reposition()`）保持不变。若 NotePane 的 floatFrame 开着时 resize，NotePane.HandleEvent 在 resize 分支 `return` 前不会调 BufPane.HandleEvent——但 resize 路径由主循环直达（§6.1），不走 NotePane.HandleEvent 的非 resize 路径，所以这点不影响。

### 6.4 事件路由小结（非 resize）

```
event → Tabs.HandleEvent → TabList → Tab → active BufPane.HandleEvent
                                            ↓ 头部检查
                                   floatFrame.IsOpen()? ──→ floatFrame.HandleEvent（modal）
                                            ↓ 否
                                   原有 BufPane 处理（键位 / 鼠标 / 粘贴）
```

事件路由完全 owner-local，主循环不感知 FloatFrame（非 resize 分支）。

---

## 七、A1 focus transfer 实现

### 7.1 Tab.SetActive hook（`tab.go:341`）

```go
func (t *Tab) SetActive(i int) {
    if i != t.active {
        t.closeActiveFloatFrame(CloseByFocus)
    }
    t.active = i
    for j, p := range t.Panes {
        if j == i {
            p.SetActive(true)
        } else {
            p.SetActive(false)
        }
    }
}
```

### 7.2 TabList.SetActive hook（`tab.go:152`）

```go
func (t *TabList) SetActive(a int) {
    if a != t.active {
        t.closeActiveTabFloatFrame(CloseByFocus)
    }
    t.TabWindow.SetActive(a)
    for i, p := range t.List {
        if i == a {
            if !p.isActive {
                p.isActive = true
                err := config.RunPluginFn("onSetActive", luar.New(ulua.L, p.CurPane()))
                if err != nil {
                    screen.TermMessage(err)
                }
            }
        } else {
            p.isActive = false
        }
    }
}
```

### 7.3 辅助方法（同文件 tab.go）

```go
// closeActiveFloatFrame 关本 tab 当前 active pane 的 widget（若有）。
// A1 规则：focus 从老 pane 转走，老 pane 的 widget 应自动关闭。
func (t *Tab) closeActiveFloatFrame(cause CloseCause) {
    if t.active < 0 || t.active >= len(t.Panes) {
        return
    }
    if bp, ok := t.Panes[t.active].(*BufPane); ok && bp.floatFrame.IsOpen() {
        bp.floatFrame.Close(cause)
    }
}

// closeActiveTabFloatFrame 关本 TabList 当前 active tab 的 CurPane 的 widget（若有）。
// A1 规则：focus 从老 tab 转走，老 tab 的 widget 应自动关闭。
func (t *TabList) closeActiveTabFloatFrame(cause CloseCause) {
    if t.active < 0 || t.active >= len(t.List) {
        return
    }
    if bp := t.List[t.active].CurPane(); bp != nil && bp.floatFrame.IsOpen() {
        bp.floatFrame.Close(cause)
    }
}
```

类型断言 `*BufPane` 是必要的：`Tab.Panes` 是 `[]Pane` 接口。当前所有 Pane 实现都是 `*BufPane`（NotePane 不在 Panes 里、InfoPane 不在 Panes 里、RawPane / TermPane 不开 widget），断言失败时 no-op 是安全兜底。

### 7.4 覆盖的 focus 转移场景

| 触发 | 路径 | hook |
|---|---|---|
| 鼠标点别的 pane | `Tab.HandleEvent` → `t.SetActive(i)`（`tab.go:312`） | Tab.SetActive |
| `NextSplit` / `PreviousSplit` 键位 | binding → `t.SetActive(...)` | Tab.SetActive |
| 鼠标点 tab bar | `TabList.HandleEvent` → `t.SetActive(ind)`（`tab.go:118`） | TabList.SetActive |
| 关 tab 后回退 | `t.SetActive(len(t.List) - 1)`（`tab.go:69`） | TabList.SetActive |

NotePane 不在覆盖范围（不在 Tab.Panes 里）。NotePane 自己开关由用户 Alt-Enter / Esc 控制，与 A1 无关。

---

## 八、widget 接口迁移

### 8.1 SelectPane（`selectpane.go`）

**Open 签名变更**：

```go
// 旧
func (s *SelectPane) Open(items []string, title string, anchor Pos, frameColor tcell.Style,
    maxVisible int, wrap bool, onSelect func(*string))

// 新
func (s *SelectPane) Open(ff *FloatFrame, items []string, title string, anchor Pos,
    frameColor tcell.Style, maxVisible int, wrap bool, onSelect func(*string))
```

**字段**：`ff *FloatFrame`（Open 入参注入）。

**spec 改造**：

```go
spec := FloatOpenSpec{
    Anchor:      anchor,
    ContentSize: contentSize,
    Title:       title,
    FrameColor:  frameColor,
    Display:     s.display,
    HandleEvent: s.handleEvent,
    AutoExpand:  true,
    OnExternalClose: func(cause CloseCause) {
        // SelectPane 的 onSelect 签名是 func(*string)，nil = 取消，cause 信息丢失。
        // 未来若 SelectPane 也想区分原因，先把 onSelect 签名升级（见 §12.1）。
        if s.onSelect != nil {
            s.onSelect(nil)
        }
    },
}
if !ff.Open(spec) {  // 用注入的 ff，不再用 TheFloatFrame
    s.items = nil
    s.onSelect = nil
    if onSelect != nil { onSelect(nil) }
}
```

**handleEvent 改造**：所有 `TheFloatFrame.Close()` → `s.ff.Close(CloseByUser)`。

### 8.2 FileSelector（`fileselector.go`）

**Open 签名变更**：

```go
// 旧
func (fs *FileSelector) Open(pane *BufPane, startDir string, onSelect func(SelectResult))

// 新
func (fs *FileSelector) Open(ff *FloatFrame, pane *BufPane, startDir string, onSelect func(SelectResult))
```

**字段**：`ff *FloatFrame`。

**spec 改造**（`buildSpec`）：

```go
return FloatOpenSpec{
    Anchor:      anchor,
    ContentSize: contentSize,
    Title:       "Open File",
    FrameColor:  tcell.Style{},
    Display:     fs.display,
    HandleEvent: fs.handleEvent,
    AutoExpand:  false,
    OnExternalClose: func(cause CloseCause) {
        switch cause {
        case CloseByResize:
            fs.finish(SelectResult{Kind: Closed, Reason: ReasonResize})
        case CloseByFocus:
            fs.finish(SelectResult{Kind: Closed, Reason: ReasonFocus})
        }
        // CloseByUser 不到这里
    },
}
```

**finish 改造**：

```go
func (fs *FileSelector) finish(r SelectResult) {
    cb := fs.onSelect
    fs.ff.Close(CloseByUser)  // 用注入的 ff；CloseByUser 不触发 OnExternalClose，无递归
    if cb != nil {
        cb(r)
    }
}
```

**所有 `TheFloatFrame.Open` / `TheFloatFrame.Close` 引用**：替换为 `fs.ff.Open` / `fs.ff.Close`。

### 8.3 调用点改造

| 文件 | 行号 | 改造 |
|---|---|---|
| `command_neo.go` | 70 | `NewSelectPane().Open(h.floatFrame, items, "Themes", ...)` |
| `filemanager.go` | 65 | `NewFileSelector().Open(pane.floatFrame, pane, dir, onSelect)` |
| `filemanager.go` | 83 | `NewFileSelector().Open(h.floatFrame, h, startDirOf(h), onSelect)` |
| `filemanager.go` | 102 | 同上 |
| `notepane.go` | 167 | `NewSelectPane().Open(n.floatFrame, names, "Receivers", ...)` |
| `notepane.go` | 247 | `NewSelectPane().Open(n.floatFrame, names, "Receiver", ...)` |

`h.floatFrame` / `n.floatFrame` 都来自 BufPane 字段（NotePane 经嵌入继承）。

---

## 九、迁移计划（三步走，对齐 N0a §七）

每步独立可编译、独立可提 PR。建议顺序执行——前一步是后一步的依赖。

### 第一步：FloatFrame 接口升级 + widget 接收 `*FloatFrame` 参数（不动主循环）

**改动范围**：`floatframe.go` / `selectpane.go` / `fileselector.go` / `globals.go` / 6 个调用点。

具体动作：

1. `floatframe.go`：
   - 加 `CloseCause` 枚举
   - `Close()` → `Close(cause CloseCause)`
   - `FloatOpenSpec.OnResize func()` → `OnExternalClose func(CloseCause)`
   - FloatFrame 字段 `onResize` → `onExternalClose`
   - `HandleEvent` 里 resize 分支：`f.Close(CloseByResize)` 后触发 `onExternalClose(CloseByResize)`
   - **暂时保留** `var TheFloatFrame *FloatFrame`（下一步删）
2. `selectpane.go` / `fileselector.go`：
   - 加 `ff *FloatFrame` 字段
   - Open 签名加首参 `ff *FloatFrame`
   - 内部 `TheFloatFrame.Open/Close` 改为 `s.ff.Open` / `fs.ff.Close`
   - spec 的 `OnResize` 改 `OnExternalClose`
3. 6 个调用点：传 owner 的 floatFrame——但这一步 owner 还没有 floatFrame 字段。**临时方案**：所有 6 处传 `TheFloatFrame`（包级单例还在，行为零变化）。

**收益**：widget 完全脱离全局 var 依赖，Open 签名稳定下来。后续步骤不再动 widget 代码。**回归**：所有现有交互行为不变（`:theme`、三个 selector、NotePane receiver 选择）。

### 第二步：BufPane 加 floatFrame 字段 + 事件/渲染/focus hook（主循环改）

**改动范围**：`bufpane.go` / `tab.go` / `micro.go` / `notepane.go`。

具体动作：

1. `bufpane.go`：
   - `BufPane` 加 `floatFrame *FloatFrame` 字段
   - `newBufPane` 末尾 `h.floatFrame = NewFloatFrame()`
   - `HandleEvent` 头部加 widget 拦截
2. `tab.go`：
   - `Tab.SetActive` 加 `closeActiveFloatFrame(CloseByFocus)` hook
   - `TabList.SetActive` 加 `closeActiveTabFloatFrame(CloseByFocus)` hook
   - 加两个辅助方法
3. `micro.go`：
   - Render 阶段：删 `if TheFloatFrame.IsOpen() { TheFloatFrame.Display() }`；末尾加 active pane 的 floatFrame overlay
   - Dispatch 非 resize：删 `else if TheFloatFrame.IsOpen()` 分支
   - Dispatch resize：`TheFloatFrame.HandleEvent` 替换为 active pane 的 floatFrame 路由
4. `notepane.go`：
   - `Display` 末尾加 `if n.floatFrame.IsOpen() { n.floatFrame.Display() }`
5. 6 个调用点：`TheFloatFrame` 替换为 `h.floatFrame` / `n.floatFrame` / `pane.floatFrame`。

**收益**：per-owner 架构完整生效。A1（focus 转移关 widget）首次可用。**回归**：见 §十一.

### 第三步：清理全局

**改动范围**：`globals.go` / `floatframe.go`。

具体动作：

1. `globals.go:17`：删 `TheFloatFrame = NewFloatFrame()` 行
2. `floatframe.go:68-70`：删 `var TheFloatFrame *FloatFrame` 及注释
3. 全局 grep `TheFloatFrame`，确认 0 命中

**收益**：「主循环不需要知道 FloatFrame」目标达成（除 §六.1 的 resize 一行 + §五.1 的 overlay 一行——这两行用 `MainTab().CurPane().floatFrame` 寻址，不依赖全局 var）。

---

## 十、关键不变量（落地后勿违反）

1. **同时最多一个 widget 可见**。A1（focus 转移关 widget）+ A2（owner 内单 widget，FloatFrame.Open 拒绝重开）联合保证。无需额外锁 / registry。
2. **BufPane.floatFrame 永不 nil**。`newBufPane` 一处注入。所有访问 `h.floatFrame.IsOpen()` 不需要 nil 检查。
3. **widget 自己关自己用 `Close(CloseByUser)`**。FloatFrame 看到这个 cause 不触发 `OnExternalClose`，避免 widget 自身关闭路径与外部关闭路径耦合。
4. **外部关闭用 `Close(CloseByResize)` 或 `Close(CloseByFocus)`**。FloatFrame 在 `isOpen=false` 之后触发 `OnExternalClose(cause)`，widget 在回调里转译成自己的 SelectResult / onSelect(nil)。
5. **A1 触发点仅在 `Tab.SetActive` / `TabList.SetActive` 两处**。其他地方（如 `BufPane.SetActive`、bindings 触发器）不要重复实现 A1——两处覆盖全部 focus 转移路径。
6. **NotePane 不在 A1 覆盖范围**。NotePane 是全局单例、不在任何 Tab.Panes。它的开关走 Alt-Enter / Esc，与 FloatFrame 的 A1 互不干扰。
7. **resize 路径主循环直达 active pane 的 floatFrame**。不要让 resize 经 TabList → BufPane.HandleEvent 转发——`TabList.HandleEvent` 对 resize 是 `t.Resize() + return`，不会转发。
8. **widget Open 签名 `ff *FloatFrame` 是第一参数**。调用方一定先持有 owner 才能调 Open，传 ff 是顺手的；ff 放第一参数降低漏传风险（编译器帮查）。
9. **渲染顺序：active pane 的 floatFrame 在 NotePane 之后**。NotePane 开着时，主编辑器 pane 的 widget 不会同时开（A1+A2 保证），渲染顺序无冲突。

---

## 十一、边界情况与回归矩阵

### 11.1 A1 触发的回归

| # | 场景 | 预期 |
|---|---|---|
| 1 | 主编辑器 pane A 有 SelectPane 开着，鼠标点 pane B | A 的 SelectPane 关，回调收到 `nil`（SelectPane）/ `ReasonFocus`（FileSelector） |
| 2 | 主编辑器 pane A 有 FileSelector 开着，按 `Ctrl-w` 切 pane | A 的 FileSelector 关，回调收到 `ReasonFocus`，三入口 onSelect 都"取消" |
| 3 | pane A 有 widget，切到 tab 2 | A 的 widget 关（`TabList.SetActive` hook） |
| 4 | tab 1 的 pane A 有 widget，关掉 tab 1 | widget 随 pane 销毁；若有引用泄漏，下一帧 GC 回收（widget 对象不再被主循环引用） |
| 5 | NotePane 开着（无 widget），鼠标点主编辑器 pane | 无变化（NotePane 不响应 A1，由用户 Esc 关） |

场景 4 验证点：`Tab.SetActive(len(t.List)-1)`（关 tab 后回退）会触发 `closeActiveTabFloatFrame`——但被关的 tab 的 CurPane 已经被销毁，`t.List[t.active]` 此时是新 active。需要在 `RemovePane` / `RemoveTab` 路径单独验证：先关 widget、再销毁 pane。**当前 micro 的关 tab 顺序是先从 List 删 tab 再 SetActive 新 tab**，老 tab 的 widget 在 SetActive 触发前就已经不可达——`closeActiveTabFloatFrame` 检查的是新 active，老 tab 的 widget 不会被这条路径关。需要在 `TabList` 关 tab 的代码路径里**单独**触发一次 `closeActiveTabFloatFrameOnIndex(oldIndex)`，或者依赖 GC（widget 对象销毁时 floatFrame 跟着销毁，isOpen 状态丢失——但回调不会触发）。

**裁决**：关 tab 时 widget 不触发 onSelect 回调是可以接受的——用户主动关 tab，意图明确，不需要回调通知"你的 widget 被关了"。但若 onSelect 持有外部资源（如 FileSelector 的后台 git goroutine），需要在 widget 对象销毁前显式 cleanup。**FileSelector 现状没有 OnFinalize 钩子**，靠 GC 回收 goroutine 引用。本期可接受，未来若有资源型 widget 再补 finalize 钩子。

### 11.2 resize 路径的回归

| # | 场景 | 预期 |
|---|---|---|
| 6 | widget 开着，终端 resize | widget 自关，回调收到 Reason（FileSelector）/ nil（SelectPane） |
| 7 | widget 没开，终端 resize | 各 pane 重排，无 widget 行为 |
| 8 | NotePane 开着，终端 resize | NotePane.HandleEvent 走 resize 分支调 `reposition`；NotePane 的 floatFrame（若也开着）由 NotePane.HandleEvent 的 resize 分支前的主循环直达路径处理 |

### 11.3 widget 不嵌套的回归

| # | 场景 | 预期 |
|---|---|---|
| 9 | widget 开着，触发"再开一个"的命令（如 `:theme` 在 FileSelector 开着时被 binding 触发） | 第二次 `Open` 返回 false（FloatFrame.Open 头部 `if f.isOpen { return false }`）；widget 调用方走"没开成"分支（SelectPane 调 onSelect(nil)、FileSelector 调 onSelect(ReasonSize)） |

**注意**：场景 9 在 per-owner 之前是全局拒绝；per-owner 之后，第二次 Open 用的是**另一个 owner 的 ff**——如果用户在 pane A 有 widget 时切到 pane B 触发 `:theme`，A1 先关了 A 的 widget，然后 B 的 `:theme` 正常开。不冲突。但如果用户**没切 pane**（仍 focus 在 A），触发 A 的另一个 widget 命令——A2 在 owner 内拒绝（`A.floatFrame.isOpen` 已是 true）。场景 9 仍成立。

### 11.4 渲染顺序的回归

| # | 场景 | 预期 |
|---|---|---|
| 10 | SelectPane（`:theme`）开着，widget 横跨多个 pane 区域 | widget 完整可见，不被其他 pane 的内容覆盖（主循环最后一行画） |
| 11 | NotePane 开着 + NotePane 上有 SelectPane（receiver 选择） | 主循环渲染顺序：`NotePane.Display`（含 NotePane 的 SelectPane overlay）→ 末尾 active pane 的 floatFrame overlay（此时是主编辑器 pane，但它的 widget 被 A1 关了，不画）→ 视觉正确 |
| 12 | FileSelector 开着，光标在文件列表上 | FileSelector 内容正确，背景的主编辑器 pane 内容不泄漏（FloatFrame.Display 已清外矩形） |

---

## 十二、开放问题

### 12.1 SelectPane 的 onSelect 要不要升级到带原因？

**现状**：SelectPane 的 onSelect 是 `func(*string)`，nil = 取消，原因丢失。

**何时升级**：N0a §3.5 已给出条件——「以后 SelectPane 也想区分原因，统一改」。当前两个使用方（`:theme`、NotePane receiver）对原因都不敏感（取消即结束）。**本期不动**。

**升级时的接口**（备用，本期不实现）：

```go
type SelectResult struct {
    Picked *string     // 非 nil = 选中
    Reason CloseReason // Picked=nil 时有效：Esc / Focus / Resize
}

onSelect func(SelectResult)
```

与 FileSelector 的 SelectResult 形态对齐。届时 SelectPane 的 `OnExternalClose` 改成传 `SelectResult{Reason: ReasonFocus / ReasonResize}`。

### 12.2 主循环那两行 FloatFrame 寻址能不能彻底删除？

**目标**：达到「主循环代码里 grep 不到 `floatFrame` / `FloatFrame` 任何一个字符」。

**当前方案的两行**（§五.1 + §六.1）：

```go
// Render 末尾
if bp := action.MainTab().CurPane(); bp != nil && bp.floatFrame.IsOpen() {
    bp.floatFrame.Display()
}
// Dispatch resize
if bp := action.MainTab().CurPane(); bp != nil && bp.floatFrame.IsOpen() {
    bp.floatFrame.HandleEvent(event)
}
```

**彻底删除的两种思路**：

1. **让 active pane 渲染 / 处理事件时带上自己的 floatFrame**：要求主循环的 `for ep := range MainTab().Panes { ep.Display() }` 改成"active 最后渲染"，且 `TabList.HandleEvent` 的 resize 分支转发到 active pane。两处都侵入 micro 原生流程，风险高于收益。
2. **抽象一个 `action.ActiveOverlay()` 函数**：主循环改成 `action.RenderOverlay()` / `action.DispatchResize(event)`，把 `MainTab().CurPane().floatFrame` 这层寻址包进 action 包。主循环 grep 不到 `floatFrame`，但代价是多一层间接。

**裁决**：**本期不删**。两行寻址等价于「主循环知道当前 active pane 有一个 overlay」，与主循环知道 `TheNotePane.IsOpen()` 同构。彻底删除是「洁癖优化」，没有实际架构收益。留作未来重构议题。

### 12.3 FileSelector 的 `pane *BufPane` 参数与 `ff *FloatFrame` 重复？

**观察**：FileSelector.Open 现在是 `Open(ff *FloatFrame, pane *BufPane, ...)`，`ff` 来自 `pane.floatFrame`，看似重复。

**为什么不合并**：

- N0 文件管理器独立成包方案里，`pane *BufPane` 会被替换成纯值（`rect Rect` + `currentPath string`）。`ff` 则演化成 `Host` 接口。两者未来都会变成不同的东西，现在合并反而阻碍演进。
- 当前 `pane *BufPane` 只用于两处只读（`pane.Buf.AbsPath` 取当前文件名高亮、`pane.BWindow.GetView()` 取 rect），与 `ff` 的用途正交。

**裁决**：保持双参数。N0 落地时各自演化。

### 12.4 关 tab 时的 widget cleanup（§11.1 场景 4）

**问题**：关 tab 流程是「先从 List 删 tab，再 SetActive 新 tab」。被删 tab 的 widget 不会经过 `closeActiveTabFloatFrame`（hook 看到的是新 active），也不会触发 onSelect 回调。

**当前可接受性**：

- FileSelector 的后台 git goroutine 持有 `fs.state` 引用，tab 销毁后 fs 对象不可达，goroutine 完成时检查 `fs.state == nil`（已实现，`fileselector.go` fetchGit 头部）后 no-op return。无资源泄漏。
- SelectPane 无后台资源。
- onSelect 不触发 = 调用方不知道 widget 被关。但调起方（如 `command_neo.go` 的 `:theme`）的 onSelect 闭包只是切换 colorscheme 或打印 message——不触发等于什么都没做，与用户预期一致（用户关 tab = 放弃当前操作）。

**未来若需要 cleanup**：在 `TabList.RemoveTab`（或 micro 原生关 tab 的代码路径）里加 `oldTab.closeActiveFloatFrame(CloseByFocus)`。本期不做。

---

## 十三、迁移工作量估算

代码改动行数粗估（不含注释）：

| 文件 | 改动 | 行数 |
|---|---|---|
| `floatframe.go` | Close 签名 + CloseCause + OnExternalClose + 删 TheFloatFrame | ~30 |
| `selectpane.go` | Open 签名 + ff 字段 + spec / handleEvent 改造 | ~15 |
| `fileselector.go` | Open 签名 + ff 字段 + buildSpec / finish 改造 + ReasonFocus | ~20 |
| `bufpane.go` | floatFrame 字段 + newBufPane 注入 + HandleEvent 拦截 | ~6 |
| `tab.go` | Tab/TabList SetActive hook + 两个辅助方法 | ~25 |
| `micro.go` | Render + Dispatch 改造 | ~10 |
| `notepane.go` | Display 末尾 overlay | ~3 |
| `globals.go` | 删 TheFloatFrame 初始化 | -1 |
| `command_neo.go` | `:theme` 调用点 | ~1 |
| `filemanager.go` | 三处 selector 调用点 | ~3 |
| **合计** | | **~115 行净增** |

测试：

- FileSelector 现有单测（截断、humanSize、formatMtime、parsePorcelain）零改动——这些是纯函数，不碰 FloatFrame。
- 新增单测：`Tab.closeActiveFloatFrame` / `TabList.closeActiveTabFloatFrame`（用 stub BufPane + stub FloatFrame 验证 hook 触发）。
- 集成测试：手动跑 §十一 的 12 个场景。

---

## 十四、关联文档

- [`N0a-floatFrame设计方案.md`](./N0a-floatFrame设计方案.md) — 产品方案与架构原则（本文件落地的基础）
- [`N0-文件管理器独立成包讨论.md`](./N0-文件管理器独立成包讨论.md) — fileManager 独立成包方案（本文件的 §12.3 引用其 `Host` 接口演化方向）
- [`F1-架构设计方案.md`](./F1-架构设计方案.md) — FileSelector 子系统架构（本文件 §八.2 / §11.1 对齐其 onSelect 分流矩阵）
- [`fileManager的产品定位.md`](./fileManager的产品定位.md) — FileManager 产品形态（决定 FileManager 走 Pane 范式、不走 FloatFrame）
- [`micro总流程调研.md`](./micro总流程调研.md) — micro 主流程（本文件 §五 / §六 的现状来源）

本文件与 N0a 的关系：N0a §五 A/B/C 三项架构选择由本文件 §三 的 ADR-1/3/4 落地；N0a §七的三步演进路径由本文件 §九 细化为可执行步骤（每步独立可编译、独立可提 PR）。
