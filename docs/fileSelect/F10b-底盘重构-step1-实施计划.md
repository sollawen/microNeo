# F10b · 分区域管理底盘 · Step 1（底盘重构）实施计划

**性质**：实施计划。设计已全部定案（见 `F10a-访问历史-讨论稿.md` 议题 1–6），本文给出 Step 1 的精确改动清单与不变性验证，可直接照着改。

**范围**：仅 Step 1（底盘 PR）。把 `session.go` 拆出 `filelist.go`，把 `fileList` / `preview` 都抽成 region，session 退化为调度器。**代码里完全没有 history 这个概念**——不构造、不预留、不占位、不留门槛。history 区域 + 记录端是 Step 2 的事。

**验收标准**：纯重构，**行为零变化**。fileList / preview 现有的全部交互、视觉、边界 case 不变；外部对 `finder` 包的调用（`NewSession` / `Open` / `HandleEvent` / `Display` / `NotifyBlur` / `IsOpen` / `Rect` / `Result` / `CloseReason`）签名一字不动。

**原生侵入**：零。改动仅限 `internal/finder/`。

---

## 1. 目标与边界

### 1.1 本步交付

| 维度 | 现状 | Step 1 后 |
|---|---|---|
| 区域数 | 0（隐含：fileList 逻辑散在 session，preview 已半独立） | 1 个 `KeyboardRegion`（fileList）+ 0/1 个 `NoKeyboardRegion`（preview，过窄不构造） |
| session.go | 1037 行，全管 | ~360 行，只做调度 + 外框 + 全局键 + close + 双 slice focus 路由 |
| fileList | 散在 `finderState` + session 方法 | 独立 `filelist.go` + `fileList` 结构体，实现 `KeyboardRegion`（接键盘、参与 focus） |
| preview | `previewState` 数据 + session 方法 | `preview` 结构体持有 `previewState` + 自己的 rect，实现 `NoKeyboardRegion`（不接键盘、不参与 focus） |
| 事件路由 | `whereIsMouse` → `handleLeftMouse`/`handleRightMouse` | `hitTest(generic)` → `regions[idx].HandleMouse` / `keyboardRegions[kbIdx]` 动 focus |
| 焦点 | 无概念 | `fm.focus int` 存在，索引 `keyboardRegions[]`，Step 1 恒 0（仅 fileList），无视觉表达 |
| rect 存储 | session 既有 `state.pvRect` 又在 `drawContent` 算子 rect | 每个 region 自己存 rect（F10a D2.b），session 不维护平行 `rects[]` |

### 1.2 Step 1 硬限制（来自讨论稿 §7.1，逐条对齐）

1. `regions []NoKeyboardRegion` 长度 = 1 或 2（preview 过窄时不构造）。`keyboardRegions []KeyboardRegion` 长度 = 1（Step 1 仅 fileList；Step 2+ 加 history 时 = 2）。**不为 history 预留位置、不设占位、不加任何 history 判断**。
2. `session.Open()` 只构造 fileList + preview，不留 `historyRect` 计算、不留 `H<15` / `len(history)>0` 这类门槛。
3. `Tab` / `Shift-Tab` 轮转在 Step 1 仅 fileList 一个 key region，表现为 no-op（`cycleFocus` 走一圈回到自己）。
4. 议题 3（history 位置 / 高度 / 滚动 / 显示条件 / 键盘交互）**全部不出现在 Step 1**。

### 1.3 focus 是 KeyboardRegion 的专属语义（贯穿 Step 1..N）

**核心原则**：focus 回答的问题只有一个——「键盘事件此刻送给谁」。这是一个键盘收件人的**路由**问题。对不接键盘的 region（如 preview）天然无意义：路由表只对收键盘的 region 有索引，没收到键盘的 region 在表里**结构性缺席**，谈不上「焦点状态」。

鼠标事件另起一路——hit-test 派发到坐标所在的 region，与 focus 路由**正交**。视觉高亮（active panel / 选中边框）又是一路概念，需要时另开一套接口（如 `HighlightOn/HighlightLost`），与 focus 解耦。

由此推出三条 chassis 不变量，**不是 Step 1 权宜，贯穿所有步骤**：

- **C-1（接口分层）**：`FocusOn` / `FocusLost` / `HandleKey` 全部归属 `KeyboardRegion` 子接口，`NoKeyboardRegion` 只含 `Display` + `HandleMouse` + `Rect()`（F10a D2.b：每个 region 自己持 rect）。preview 满足 `NoKeyboardRegion`；fileList 满足 `KeyboardRegion`。

- **C-2（键盘路由）**：`Session.focus` 是 `keyboardRegions []KeyboardRegion` 的索引（**不是** `regions[]` 的索引——两 slice 各自独立）。键盘事件走 `fm.keyboardRegions[fm.focus].HandleKey(ev)`——静态类型就是 `KeyboardRegion`，编译期就保证 `HandleKey` 存在，**根本无法**编译过「把 key 发给 preview」这种调用。

- **C-3（mouse 不动 non-key focus）**：mouse 事件到达时独立 hit-test `keyboardRegions[]`——preview 不进 `keyboardRegions` 这个 slice（构造期结构性缺席），hit-test 自动对 preview 返 -1，`switchFocus` 不触发。要让 preview 接键盘（架构上不推荐但未来场景有需要），只能把 HandleKey 等方法给 preview 实现、让它满足 `KeyboardRegion`——一旦满足，结构升格，preview 自然进 `keyboardRegions`，focus 也自然开始对它有意义。

**双 slice 设计是 C-1..C-3 的载体**：把 `regions`（一般 region 视图）+ `keyboardRegions`（键盘 region 视图）分开，preview 只进前者，fileList 两个都进；`focus` 是后者的索引。`HandleEvent` 内部两路 hit-test 各管各的，不需要任何类型断言。

为什么不在 `regions[]` 上做 `.(KeyboardRegion)` 断言？F10a D2.b 已经让 region 自己管 rect，session 用不上平行 `rects[]`；hit-test 直接走 `region.Rect()`。再要 `regions[]` 兼任 focus 索引空间，就得让每个 region 自报家门是不是 KeyboardRegion——那就得回到类型断言或 `IsKeyboardRegion() bool` 标记方法。这两条路都不如「两个独立 slice，构造期就分流」干净。

「点 preview → focus 切到 preview → 键盘死」这种回归被**根本消除**：preview 只实现 `NoKeyboardRegion`，结构上不可能出现在 `keyboardRegions[]` 中——编译期保证，不需运行时检查。即便 `cycleFocus` / `HandleEvent` 哪天写错，`switchFocus` 入口再检查 `kbIdx` 是否在 `keyboardRegions` 范围内就兜回来了。

**FFM**（mouse move → 跨区 hover 切焦点）独立于 C-1..C-3，定位是 UX 选择。Step 1 启用（代码已就位，唯 fileList 一个 key region，效果无变化）。Step 2 history 加入后，mouse move 跨区自动切焦点。

---

## 2. 设计决策汇总（引用讨论稿）

全部已定案，本文不重新讨论，只标注落点：

| 决策 | 编号 | 落点 |
|---|---|---|
| Display 由 session 内循环调各 region | D1.a | `session.Display` |
| rect 在 `Open` 期间构造时注入 region，生命周期内不变；按屏面积决定构造哪些 region | D2.a | `session.Open` |
| region 自持 rect（F10a D2.b），session 不维护平行 `rects[]`；hit-test 走 `region.Rect()` | D2.b | `region.go` 加 `Rect() Rect`，session 无 `rects[]` 字段 |
| region 只画纯内容；外框 + 分隔符列 + 区间分隔线全由 session 画 | D3.c | `session.drawBorder` + 各 region.`Display` |
| 拆 `HandleKey`（仅 `KeyboardRegion`）+ `HandleMouse`（`NoKeyboardRegion`），各返回 bool | E1.b | `NoKeyboardRegion` + `KeyboardRegion` 接口分层 |
| mouse 路由由 session hit-test；`regions[]` 给一般 dispatch，`keyboardRegions[]` 给 mouse→switchFocus / FFM | E2.a | `hitTest[T]` 通用函数，`HandleEvent` 两个 call site 各跑一次 |
| `FocusOn`/`FocusLost` 无参 | F1.a | `KeyboardRegion` 接口 |
| fileList 的 FocusOn/Lost 调选中行高亮色 / preview 不实现 | F2 | Step 1 fileList 全 no-op（无第二 key region，无视觉差异可表达）；preview 结构上不实现这俩方法 |
| Tab/Shift-Tab 由 session 拦；`cycleFocus` 走 `keyboardRegions[]`（已经在焦点空间里）| F4 | `session.cycleFocus`：Step 1 仅 fileList 一个 KeyboardRegion、Tab/Shift-Tab 恒 no-op（cycleFocus 走一圈回到本身）；Step 2+ history 加入后跳过 preview 自动，`fileList ↔ history` 之间轮转 |
| `Session.focus` 是 `keyboardRegions[]` 索引；构造期 fileList 进 `keyboardRegions`、preview 不进 | L1 | `session.Open` + `Session` struct |
| session.Display 保持单一渲染总入口 | R1 | `session.Display` |
| preview 只实现 `NoKeyboardRegion`；fileList / Step 2 history 实现 `KeyboardRegion` | P1/P6 | `preview` 与 `fileList` 接口分层（preview 无 `FocusOn/Lost/HandleKey`，见 §1.3 C-1）；构造时 `keyboardRegions` 只追加 fileList |

接口分层（讨论稿 §2.4 基础上，分 `NoKeyboardRegion` / `KeyboardRegion`，F10a D2.b 已把 `Rect()` 加进 `NoKeyboardRegion`）：

```go
type NoKeyboardRegion interface {
    Display()
    HandleMouse(ev *tcell.EventMouse) bool
    Rect() Rect                 // 构造时 session 把 rect 传给 region，整个生命周期内不变；session 通过它做 hit-test。
}

type KeyboardRegion interface {
    NoKeyboardRegion
    HandleKey(ev *tcell.EventKey) bool
    FocusOn()
    FocusLost()
}
```

---

## 3. 结构对照

### 3.1 包文件布局

```text
internal/finder/
  session.go     [瘦身] 公共类型(Rect/CloseReason/Result) + Session 调度器 + drawBorder + 全局键 + close
  region.go      [NEW]  NoKeyboardRegion + KeyboardRegion 两个接口 + 各自 doc 注释（§1.3 C-1 体现 + F10a D2.b Rect()）
  filelist.go    [NEW]  fileList 结构体 + 文件列表全部状态与方法（从 session.go 迁入）+ 实现 KeyboardRegion（含 Rect）
  preview.go     [改造] preview 结构体（包住原 previewState）+ 满足 NoKeyboardRegion 接口（Display/HandleMouse/Rect）
  add.go         [微调] fm.state → fm.list
  delete.go      [微调] fm.state → fm.list
  rename.go      [微调] fm.state → fm.list
  model.go       [不动]
  git.go         [不动]
  strutil.go     [不动]
  session_test.go[改写] 适配新结构
```

### 3.2 Session 结构体：前 / 后

**前**（现状）：

```go
type Session struct {
    state   *finderState   // 全部可变状态（fileList + preview 混在一起）
    rect    Rect           // 外矩形（左栏外框，含分隔符列）
    isOpen  bool
    onClose func(Result)
    isQuit  bool
}
```

**后**（Step 1）：

```go
type Session struct {
    rect    Rect        // 整个 finder 的外框（含标题行、底边、所有面板）。区别于 fileList.rect 内容区，两个概念
    divX    int         // 左/右分栏竖线 X 坐标（分隔符列 `│` 画在此列）。Step 2+ 加 history 时再加 divY

    list    *fileList   // 恒构造（fileList 是 finder 主体）
    prev    *preview    // 右栏宽 >= previewMinWidth 时构造；否则 nil

    regions         []NoKeyboardRegion  // 所有 region。[list] 或 [list, prev]；Display 循环调 Display、HandleEvent mouse 分支 dispatch HandleMouse；hit-test 走 region.Rect()（F10a D2.b）
    keyboardRegions []KeyboardRegion    // 仅接键盘的 region：`focus` 的索引空间在这里（preview 不在场→结构性缺席）；key 路由 / cycleFocus / mouse→switchFocus / FFM 都走这个 slice
    focus           int                 // 索引 keyboardRegions。Step 1 恒 0；Step 2 起 Tab/mouse→switchFocus 才会动

    isOpen  bool
    onClose func(Result)
    isQuit  bool
}
```

要点：

- `list` / `prev` 是**类型化引用**，给跨 region 协调用（§5）；`regions []NoKeyboardRegion` 是**接口切片**，给通用调度用（Display 循环、mouse hit-test）；`keyboardRegions []KeyboardRegion` 是**第二个接口切片**，专门给 keyboard 路由 + focus 用。`list` 同时是 `regions[0]` 和 `keyboardRegions[0]`，两份指向同一对象——这是「类型化协作 + 双 slice 调度」的常规写法，不冗余。
- `fm.rect` 是整个 finder 的**外框**（含标题行、底边、所有面板），`fileList.rect` 是**内容区**（去标题行、去右分隔符列、去底边），两个概念。`fm.divX` 是分栏竖线 X 坐标，drawBorder 用它画分隔符列 `│`；各 region 的 rect 在构造时从 `fm.rect` + `fm.divX` 算出后注入。
- `finderState` 类型**删除**。其字段拆进 `fileList`（文件列表相关）和 `preview`（previewState 本就独立）。
- **删除** `fm.rects []Rect` 字段——rect 由 region 自己持（F10a D2.b），session 不维护平行副本。

### 3.3 `finderState` 字段去向

| 字段 | 去向 |
|---|---|
| `currentDir / allEntries / showEntries` | `fileList` |
| `isRepo / gitBranch / mu` | `fileList`（fetchGit 仍只碰这些，锁跟着走） |
| `rightMode / sortMode / sortDesc / showHidden` | `fileList` |
| `cursor / topIdx / pickerW / listH` | `fileList`（`pickerW`/`listH` 改为构造时由 rect 算好存入，语义不变） |
| `preview *previewState` | 删除（preview region 自持） |
| `pvRect Rect` | 删除（preview region 自持 rect；Open 中局部变量计算后注入，不存 session 字段）|

---

## 4. 改动清单

### 4.1 `region.go`（新文件）

只有接口定义（`Rect` / `CloseReason` / `Result` 仍在 `session.go`，它们是包的公共 API 面）：

```go
package finder

import "github.com/micro-editor/tcell/v2"

// NoKeyboardRegion 是 finder 内一个自管理区域的最小契约。
//
// 构造时 session 塞入绝对屏坐标 Rect，region 整生命周期持有它（F10a D2.b）。
// session 在每帧 Display 链上调用 Display() 画自己的内容（不含框线）。
// session 按 hit-test 把 mouse 事件交给 HandleMouse；返回 false 表示
// 「这个具体事件我不处理」。
//
// 接键盘事件的 region（fileList、Step 2 加入的 history）还应实现 KeyboardRegion；
// preview 这类不接键盘的 region 只需满足 NoKeyboardRegion。focus 管理、HandleKey 全部归属
// KeyboardRegion，preview 在结构上不参与 focus 路由（见 §1.3 C-1..C-3）。
type NoKeyboardRegion interface {
    Display()
    HandleMouse(ev *tcell.EventMouse) bool
    Rect() Rect                 // 构造时 session 把 rect 传给 region，整个生命周期内不变；session 通过它做 hit-test。
}

// KeyboardRegion 是 NoKeyboardRegion 之上加键盘相关方法的扩展接口。
//
// session 把键盘事件只送给当前 focus 的 KeyboardRegion（在 keyboardRegions[] 索引空间里）；
// FocusOn/FocusLost 是 session 切换 focus 时通知 region 的钩子。
// Step 1 全 no-op（无第二 key region、无视觉差异可表达），Step 2 起加视觉实现。
type KeyboardRegion interface {
    NoKeyboardRegion
    HandleKey(ev *tcell.EventKey) bool
    FocusOn()
    FocusLost()
}
```

### 4.2 `filelist.go`（新文件，从 session.go 迁入）

#### 4.2.1 结构体

```go
type fileList struct {
    fm   *Session // 回引用：syncPreview（选择变→刷新预览）、close（选文件→关 session）
    rect Rect     // 内容区矩形（去外框上边框 + 去右分隔符列）；Open 注入，生命周期内不变（F10a D2.b）

    // —— 目录与条目 ——
    currentDir  string
    allEntries  []entry
    showEntries []*entry

    // —— git（后台 goroutine 写、UI 读，mu 保护）——
    isRepo    bool
    gitBranch string
    mu        sync.RWMutex

    // —— 视图状态 ——
    cursor      int
    topIdx      int
    sortMode    sortMode
    sortDesc    bool
    rightMode   rightMode
    showHidden  bool
    pickerW     int    // = rect.W（内容区宽，截断用）。派生值，存着省得各处写 rect.W
    listH       int    // = rect.H - 2（文件列表可见行数，去面包屑行 + hint 行）。派生值，clamp 滚动时需要
}
```

#### 4.2.2 `NoKeyboardRegion` + `KeyboardRegion` 方法实现

```go
// —— NoKeyboardRegion ——
func (l *fileList) Display() {
    l.fm.hideCursor()
    l.drawContent()           // 直接复刻原 drawContent 函数体，r := fm.state.listRect → l.rect
}
func (l *fileList) HandleMouse(ev *tcell.EventMouse) bool {
    return l.handleLeftMouse(ev)        // 原 handleLeftMouse 函数体迁入；名称保留以反映 input +Button1
}
func (l *fileList) Rect() Rect {
    return l.rect                        // F10a D2.b：region 自持 rect
}

// —— KeyboardRegion ——
// Step 1 FocusOn/FocusLost 全 no-op（无第二 key region → 无视觉差异可表达）。
// Step 2 加视觉实现（选中行高亮 / 取消高亮）。
func (l *fileList) FocusOn()  {}
func (l *fileList) FocusLost() {}
```

#### 4.2.3 `HandleKey` 的键归属

```go
func (l *fileList) HandleKey(ev *tcell.EventKey) bool {
    switch ev.Key() {
    case tcell.KeyDown:
        l.moveCursor(+1); return true
    case tcell.KeyUp:
        l.moveCursor(-1); return true
    case tcell.KeyEnter:
        l.activate(); return true
    case tcell.KeyLeft:
        l.chdirParent(); return true
    case tcell.KeyRight:
        if l.cursorIsDir() { l.chdir(l.showEntries[l.cursor-1].name) }
        return true
    case tcell.KeyRune:
        switch ev.Rune() {
        case '.':
            l.toggleHidden(); return true
        case 'd':
            l.startDelete(); return true
        case 'a':
            l.startAdd(); return true
        case 'r':
            l.startRename(); return true
        // 'q' 不在 fileList 内处理，返 false 走全局路由
        }
        return false
    }
    return false // 未处理的键（Esc/Ctrl-Q/q 等）交 session.handleGlobalKey
}
```

Esc/Ctrl-Q 关整个 finder，是 session 级命令，不在 fileList 里处理。`handleGlobalKey` 统一处理。

#### 4.2.4 构造函数

```go
func newFileList(fm *Session, rect Rect, cwd, file string, pickerW int) *fileList {
    l := &fileList{
        fm:         fm,
        rect:       rect,
        pickerW:    pickerW,            // = rect.W（截断用），存着省得各处写 rect.W
        listH:      rect.H - 2,         // 去面包屑行 + hint 行
        currentDir: cwd,
        cursor:     0,
        topIdx:     0,
        sortMode:   sortName,
        sortDesc:   false,
        rightMode:  rightSize,          // Step 1 恒 rightSize
        showHidden: false,
    }
    l.loadDir(cwd, file)
    go l.fetchGit(cwd)
    return l
}
```

#### 4.3.1 结构体

```go
type preview struct {
    rect            Rect           // 预览区矩形；Open 注入，生命周期内不变（F10a D2.b）

    // —— 预览状态（原 previewState 字段全部展开在此）——
    path     string       // 当前预览的文件路径；空 = 无可预览（未选中/isDir/过窄）
    topLine  int          // 首行在 previewLines 中的索引（滚轮用）
    readErr  error        // 读取/解析失败时记错，Display 显示占位文案
    binary   bool         // 二进制文件：不尝试预览全文

    // previewLines 和 truncated 在 Display 内按需填充，用完即弃，不存为持久状态
}
```

#### 4.3.2 方法块：只实现 `NoKeyboardRegion`，**不**实现 `KeyboardRegion`

```go
func (p *preview) Display() {
    p.drawPreviewBody()      // 函数体基本照搬，pvRect → p.rect 字段化
}

func (p *preview) HandleMouse(ev *tcell.EventMouse) bool {
    switch ev.Buttons() {
    case tcell.WheelUp:
        p.scroll(-previewScrollStep); screen.Redraw(); return true
    case tcell.WheelDown:
        p.scroll(previewScrollStep); screen.Redraw(); return true
    }
    return false // Button1 click 等：不接（与现状 handleRightMouse 一致）
}

func (p *preview) Rect() Rect {
    return p.rect                 // F10a D2.b：region 自持 rect
}
```

preview **只实现 `NoKeyboardRegion`**，**不实现 `KeyboardRegion`**——结构上就不存在 `FocusOn` / `FocusLost` / `HandleKey` 三个方法。focus 路由、键盘事件路由都接触不到 preview（§1.3 C-1/C-2/C-3）。`HandleMouse` 与现状 `handleRightMouse` 等价。`drawPreviewBody` 是 `(*preview)` 方法，函数体照搬原 `drawPreviewBody`（`pvRect` → `p.rect`，`previewState` 字段 → `p.path` / `p.topLine` / `p.readErr` / `p.binary`）。

可见性与历史兼容：将来若 preview 接 PageUp/Down 翻页，做法是把 preview **结构升格**实现 `KeyboardRegion`——升格后自动进 `keyboardRegions[]`，自动能接 focus、接 key，无需任何 session 改动。当前 Step 1 不走这条路，最干净。

`preview.scroll` 里的 `visible := fm.state.pvRect.H - 2` → `p.rect.H - 2`。

#### 4.3.3 构造函数

```go
func newPreview(rect Rect) *preview {
    return &preview{rect: rect}
}
```

### 4.4 `session.go`（瘦身）

只保留：公共类型、`Session` 调度器、`drawBorder`、全局键（含 `handleGlobalKey`）、`close`、双 slice focus 调度。

#### 4.4.1 `Open`：构造期双 slice 分流

```go
func (fm *Session) Open(rect Rect, cwd, file string, isQuit bool, onClose func(Result)) bool {
    fm.onClose = onClose
    fm.isQuit = isQuit

    if rect.W < fsMinWidth || rect.H < fsMinHeight {
        return false // 预检放不下：不开会话、不触发 onClose
    }

    // —— 几何参数：整个 finder 外框、分栏 ——
    fm.rect = Rect{X: rect.X, Y: rect.Y, W: rect.W, H: rect.H - 1} // 全外框（含左右两栏），-1 留 statusLine

    widthFrac := config.GetGlobalOption("fileselectwidth").(float64)
    pickerW := int(widthFrac * float64(rect.W))
    if pickerW < fsMinWidth { pickerW = fsMinWidth }
    if pickerW > rect.W     { pickerW = rect.W }
    fm.divX = rect.X + pickerW

    // —— fileList：Session 算好 rect 和 pickerW，fileList 只管收 ——
    listRect := Rect{
        X: fm.rect.X,
        Y: fm.rect.Y + 1,      // 让出标题行
        W: pickerW - 1,         // 让出右分隔符列
        H: fm.rect.H - 1 - 2,  // 让出标题行 + hint 行 + 底边
    }
    fm.list = newFileList(fm, listRect, cwd, file, pickerW)

    // —— 双 slice ——
    fm.regions         = make([]NoKeyboardRegion, 0, 3)
    fm.keyboardRegions = make([]KeyboardRegion, 0, 2)

    // fileList 进两个 slice
    fm.regions         = append(fm.regions, fm.list)
    fm.keyboardRegions = append(fm.keyboardRegions, fm.list)

    // preview 仅进 regions（结构性缺席于 keyboardRegions）
    pvW := fm.rect.W - pickerW
    if pvW >= previewMinWidth {
        pvRect := Rect{X: fm.divX, Y: fm.rect.Y, W: pvW, H: fm.rect.H - 1}
        fm.prev = newPreview(pvRect)
        fm.regions = append(fm.regions, fm.prev)
    }
    fm.focus = 0

    fm.isOpen = true
    fm.syncPreview()
    return true
}
```

要点：

- `listRect = (fm.rect.X, fm.rect.Y+1, pickerW-1, fm.rect.H-3)`：内容区原点在 `Y+1`（让出标题行），宽 `pickerW-1`（让出右分隔符列），高 `fm.rect.H-3`（让出标题行 + hint 行 + 底边）。这正是原 `drawContent` 画的矩形。
- preview 过窄（右栏宽 < `previewMinWidth`）时**不构造** preview region，`fm.prev = nil`，`regions` 只有 fileList，与 `keyboardRegions` 等长——视觉等价现状「`drawPreview` 早返不画」；mouse 命中该区域 `hitTest` 返 -1 → 丢弃（与现状 `handleRightMouse` 进了也只 wheel-noop 等价）。
- `syncPreview` 内部判 `fm.prev == nil` 提前返回，所以首帧调用安全。
- **无 `fm.rects`**——rect 由 region 自持（F10a D2.b）。

#### 4.4.2 `Display`：循环 + 外框

```go
func (fm *Session) Display() {
    if !fm.isOpen { return }
    screen.Screen.HideCursor()
    fm.drawBorder()                       // session 画外框 + 分隔符列 + 标题（D3.c）
    for _, r := range fm.regions {
        r.Display()                       // 各 region 画自己的纯内容
    }
}
```

#### 4.4.3 `hitTest`：通用 hit-test（泛型 + 走 region.Rect）

```go
// hitTest 把鼠标坐标映射到传入切片的下标；命中 gap / 外框 / 越界 → -1。
// region 平铺不重叠，顺序遍历找第一个（也是唯一）命中即可。
// 用泛型（Go 1.18+）同时接受 []NoKeyboardRegion（=fm.regions）和 []KeyboardRegion
// （=fm.keyboardRegions），三个 call site 共用同一函数：
//   - HandleEvent.mouse: hitTest(fm.regions, e)         // 一般 dispatch
//   - HandleEvent.mouse: hitTest(fm.keyboardRegions, e) // mouse→switchFocus / FFM
func hitTest[T interface{ Rect() Rect }](items []T, ev *tcell.EventMouse) int {
    mx, my := ev.Position()
    for i, item := range items {
        r := item.Rect()                  // F10a D2.b：region 自持 rect
        if mx >= r.X && mx < r.X+r.W && my >= r.Y && my < r.Y+r.H {
            return i
        }
    }
    return -1
}
```

#### 4.4.4 `HandleEvent`：双 hit-test 各管各的

```go
func (fm *Session) HandleEvent(event tcell.Event) {
    if !fm.isOpen { return }
    if _, ok := event.(*tcell.EventResize); ok {
        fm.close(Resize); return
    }
    switch e := event.(type) {
    case *tcell.EventKey:
        switch e.Key() {
        case tcell.KeyTab:
            fm.cycleFocus(+1); return
        case tcell.KeyBacktab:
            fm.cycleFocus(-1); return
        }
        // C-2 兑现：keyboardRegions[] 静态类型就是 KeyboardRegion，直接 HandleKey 无需类型断言。
        // preview 结构性缺席（不在 keyboardRegions 里） → 编译期就不会走到这里处理 preview 的 key
        if fm.focus < len(fm.keyboardRegions) {
            if fm.keyboardRegions[fm.focus].HandleKey(e) { return }
        }
        fm.handleGlobalKey(e)           // Esc/Ctrl-Q/q 等全局命令

    case *tcell.EventMouse:
        // 1) mouse→switchFocus：先切焦点，再 dispatch。这样 region 的 HandleMouse
        //    看到的就是已切换后的 focus 状态。
        if kbIdx := hitTest(fm.keyboardRegions, e); kbIdx >= 0 && kbIdx != fm.focus {
            fm.switchFocus(kbIdx)
        }
        // 2) 一般 mouse dispatch：hit-test 走 regions[]。preview/fileList 都进这个 slice
        if idx := hitTest(fm.regions, e); idx >= 0 {
            fm.regions[idx].HandleMouse(e)
        }
    }
}
```

FFM（mouse move 切焦点）：`hitTest(fm.keyboardRegions, e)` 已对所有 mouse event 执行，Step 1 启用（唯 fileList 一个 key region，效果无变化）。Step 2+ history 加入后自动生效。

#### 4.4.5 `cycleFocus` / `switchFocus`

```go
// cycleFocus：Tab/Backtab 触发的 focus 轮转。索引空间是 keyboardRegions，preview 不在里。
// Step 1 唯 n=1（fileList），Tab/Backtab 都回到本身（cycleFocus 走一圈 = no-op）。
// Step 2+ 加 history：n=2，在 fileList ↔ history 之间轮转。
func (fm *Session) cycleFocus(delta int) {
    n := len(fm.keyboardRegions)
    if n == 0 { return }                              // C-3 运行期兜底：没有 key region → no-op
    next := (fm.focus + delta + n) % n                 // 模运算保证负 delta（Backtab）也走对
    fm.switchFocus(next)
}

// switchFocus：动 focus 前确认 to 在 keyboardRegions 范围内（防越界），且 ≠ current（no-op）。
// 结构上 keyboardRegions[to] 必为 KeyboardRegion，FocusOn/FocusLost 直接调，无需类型断言。
func (fm *Session) switchFocus(to int) {
    n := len(fm.keyboardRegions)
    if to < 0 || to >= n { return }                   // 越界拒收（C-3 运行期兜底）
    if to == fm.focus { return }                       // no-op
    if fm.focus >= 0 && fm.focus < n {
        fm.keyboardRegions[fm.focus].FocusLost()
    }
    fm.focus = to
    fm.keyboardRegions[to].FocusOn()
}
```

Step 1 的 fileList `FocusOn`/`FocusLost` 全 no-op，switchFocus 即便被调也无可观察效果。Step 1 唯一调用方是 `cycleFocus`（Tab/Backtab 触发，因 n=1 走一圈回到 fileList = no-op）。Step 2 history 加入后，`HandleEvent` mouse 分支也会调 switchFocus（点 history → 切过去）。

#### 4.4.6 其余 session 内容（迁移后）

- `NewSession` / `IsOpen` / `NotifyBlur`：签名不变，`NotifyBlur` → `close(Esc)` 不变。
- `closePicked` / `finishClose` / `reset`：照搬（只操作 `fm.isOpen` / `fm.onClose`）。

### 4.5 共享绘图工具：`drawString` / `clearRect` 提为包级函数

`drawString` / `clearRect` 现状是 `*Session` 方法，但函数体不读任何 Session 状态、只写 `screen.Screen`。fileList 和 preview 都要用。提为包级函数，把 `*Session` 接收者拿掉，全参数化 `screen` + 内容。

波及调用点：

| 文件 | 函数 | 调用内容 |
|------|------|----------|
| `session.go` | `drawBorder` | `drawString` × 2（标题行左/右）、`clearRect` × 2（title bar 防残余） |
| `session.go` | `drawStatusLine` | `drawString` × 1 |
| `filelist.go` | `drawContent` | `drawString`（文件名/大小/git 列/scroll 列/面包屑/hint）、`clearRect`（行清底） |
| `preview.go` | `drawPreviewBody` | `drawString`（预览行/占位文案/truncated 标记）、`clearRect`（行清底） |

### 4.6 `add.go` / `delete.go` / `rename.go`（微调）

- 唯一改动：`fm.state.xxx` → `fm.list.xxx`。机械替换。
- `delete.go` 内部要拿 path 走一下 `fm.list` 字段。
- `rename.go` 同步。
- 其余不动。

### 4.7 `session_test.go`（改写）

- 全部 `fm.state.xxx` 断言 → `fm.list.xxx`。
- 新增 `hitTest` 的 row/col 用例（替换 `whereIsMouse`）；case 不减。
  - `hitTest` 通用函数测试：`regions` 命中/未命中/越界，`keyboardRegions` 命中/未命中，gap 间隙。
- `fileList.listRowAt` 已有测试保留（rename 为 `listRowAt`）。

---

## 5. 跨 region 协调点

session 是协调中心，region 之间不直接相互引用——所有跨 region 通信都走 `fm.syncPreview` / `fm.selectedFilePath` 之类的小函数，避免循环依赖。

### 5.1 preview 刷新（fileList 选择变 → preview 重载）

```go
// syncPreview 由 session 拥有。fileList 任何会改变 selectedFilePath 的动作
// （moveCursor / chdir / chdirParent / activate / reload）末尾都调一次。
func (fm *Session) syncPreview() {
    if fm.prev == nil { return }                            // preview 过窄不构造
    path := fm.list.selectedFilePath()                       // 问 fileList 要 path
    fm.prev.Load(path)                                       // 喂给 preview
    screen.Redraw()
}

// selectedFilePath 挂在 fileList 上。把原 refreshPreview 的两个 gate
//（cursor 合法性 + isDir）与 loadFile 的 path 拼装合到一处。
func (l *fileList) selectedFilePath() string {
    cur := l.cursor
    if cur <= 0 || cur > len(l.showEntries) { return "" }   // 面包屑等空 path 走这里
    e := l.showEntries[cur-1]
    if e.isDir || e.name == "" { return "" }                 // 目录不预览
    if e.isLink && !e.linkTargetIsFile { return "" }        // 不可解析的 symlink 跳过
    return filepath.Join(l.currentDir, e.name)
}
```

### 5.2 pick（fileList Enter on file → session close）

```go
// closePicked 由 session 拥有。fileList.activate() 检测到光标在文件上时调一次。
func (fm *Session) closePicked(name string) {
    fm.finishClose(Result{
        Reason: Picked, Cwd: fm.list.currentDir, File: name, IsQuit: fm.isQuit,
    })
}

// finishClose / reset 照搬（只操作 fm.isOpen / fm.onClose）。
```

### 5.3 chdir（fileList 内自洽，但触发 preview + git）

`fm.list.chdir` / `fm.list.chdirParent` / `fm.list.reload` 内部行为不变：
- 改 `currentDir`、重读 `allEntries`、reset `cursor=0`、`showEntries` 重算
- `fm.syncPreview()` —— 调一次
- `go fm.list.fetchGit(fm.list.currentDir)` —— 后台刷新

session 不拦截。

---

## 6. 行为不变性验证清单

Step 1 合入前逐条核对（每条 = 一个手测脚本）：

### 6.1 视觉

- [ ] finder 外框、标题 `Open File`、上下 `─`、分隔符列 `│`、`┬`/`┴` 交叉字符：逐字符与现状一致（drawBorder 只改了右栏宽来源，画法不变）。
- [ ] 面包屑、文件条目（marker/name 截断/size 右对齐/git 列/scroll 列 ▲▼）、hint 行：与现状像素级一致（drawContent → fileList.Display，函数体照搬）。
- [ ] 光标反白高亮：颜色、覆盖范围（name+fill+git+scroll 列）与现状一致。
- [ ] preview 占位文案（`Select a file` / `Binary file` / `Unable to preview`）、正文逐行、超宽截断、`(truncated)` 末行：与现状一致。

### 6.2 键盘

- [ ] ↑/↓ 移光标、clamp 不循环；Enter 进目录 / 选文件 / 回上级；← 回上级；→ 进目录；`.` 切 hidden。全部与现状一致。
- [ ] Esc 关、Ctrl-Q 退、`q` 退：与现状一致（Esc 走 fileList.HandleKey 返 false → handleGlobalKey 关；Ctrl-Q 与 `q` 各自处理路径不同但行为等价）。
- [ ] `d`/`a`/`r` 开 modal：dialog 弹出位置、预填、回调行为（删除/新建/改名 + 刷新）与现状一致。
- [ ] 鼠标点 preview 右栏后，立即按 ↓ —— 光标仍应移动（preview 不在 `keyboardRegions`，C-3 保证）

### 6.3 鼠标

- [ ] 左栏点条目移光标、点面包屑移到 cursor=0、点空白/hint no-op：与现状一致（fileList.HandleMouse + listRowAt）。
- [ ] 左栏滚轮 ↑↓ 移光标 ±1：与现状一致。
- [ ] preview 滚轮 ↑↓ 滚正文（±`previewScrollStep` 行）、边界 clamp：与现状一致。
- [ ] 点外框 / 分隔符列 / finder 外：no-op（hitTest 返 -1）。与现状 `mouseOutside` 一致。
- [ ] **关键回归点**：mouse move（纯滑动不按键）—— 现状 no-op，Step 1 FFM 已启用但唯 fileList 一个 key region，效果仍 no-op（focus 恒 0）。Step 2+ history 加入后 mouse move 跨区自动切焦点。

### 6.4 布局 / 生命周期

- [ ] 极窄 pane（`W<20` 或 `H<10`）：`Open` 返 false、不开会话、不触发 onClose（owner 走各自回退）。与现状一致。
- [ ] preview 过窄（右栏宽 < `previewMinWidth`）：右栏不画预览、滚轮点右栏 no-op。与现状一致。
- [ ] 运行中 resize：`close(Resize)` → onClose(Resize) → owner no-op。与现状一致。
- [ ] owner 失焦（`NotifyBlur`）：`close(Esc)`。与现状一致。
- [ ] Ctrl-o / Ctrl-q / birth hook 三入口（fileops.go）：`Open` 签名不变、`Result` 字段不变 → 零改动。

### 6.5 git

- [ ] 进 git 仓库：条目 git 列出现 `M`/`A`/`U`/`D`/`R`/`I`，颜色对；非仓库全空。与现状一致（fetchGit 迁 fileList，锁跟着走）。
- [ ] 快速 chdir（fetchGit 未回已切走）：旧结果丢弃、不污染新目录。与现状一致（锁内 `currentDir != dir` 判定不变）。

### 6.6 测试

- [ ] `go test ./internal/finder/` 全绿（改写后的 `hitTest` + `fileList.listRowAt` 测试覆盖原 `whereIsMouse` + `listRowAt` 全部 case）。
- [ ] `make build` 通过（必须用 make build，不直接 go build）。

---

## 7. 不做的事（Step 2 范围，明确划出）

- **history 区域**：不构造 `historyList` region、不留 `keyboardRegions[1]` 占位、不加 `historyRect` 计算、不加 `H<15` / `len(history)>0` 门槛。
- **记录端**：不动 `config.RecordDirHistory`、不碰 `history.json`。
- **mouse→switchFocus / FFM**：§1.3 已把 mouse→switchFocus 收纳进 `HandleEvent` mouse 分支那一行 `hitTest(fm.keyboardRegions, e)`，preview 结构性缺席 → 自动不切焦点。Step 1 仅 fileList 一个 key region，`switchFocus(kbIdx)` 即便被调也只在 idx=0 上 no-op（`to == fm.focus` 短路）。FFM 已启用（Step 1 效果无变化，Step 2 history 加入后自然生效）。
- **FocusOn / FocusLost 视觉表达**：Step 1 全 no-op（fileList 的 FocusOn/FocusLost 方法体是空函数、preview 不实现这两个）。Step 2 加 fileList 选中行高亮色；preview 既然不实现 `KeyboardRegion`，就不存在「preview 边框高亮」这件事——「active panel 高亮」需要另开一套视觉接口（如 `HighlightOn/HighlightLost`），与 focus 解耦，需要时再加。
- **session.go 进一步瘦身**（add/delete/rename/git 命令再拆文件）：与本期正交，不做。
- **preview 接键盘**（PageUp/Down 等）：讨论稿 P8 暂按空操作；要做也是 preview 整体升格实现 `KeyboardRegion`，自动进 `keyboardRegions[]`，session 零改动。

---

## 8. 改动规模预估

| 文件 | 现行数 | Step 1 后 | 说明 |
|---|---|---|---|
| `session.go` | 1037 | ~360 | 删 ~680 行（迁出），留公共类型 + 调度 + drawBorder + close + 全局键 + cycleFocus/switchFocus/hitTest + Draw |
| `filelist.go` | 0（新） | ~570 | 接收 session.go 迁出的全部文件列表逻辑 + KeyboardRegion 方法（Display/HandleMouse/HandleKey/FocusOn/FocusLost/Rect）+ 2 accessor |
| `preview.go` | 219 | ~210 | 结构体包一层 + NoKeyboardRegion 方法（仅 Display/HandleMouse/Rect，**不写** FocusOn/Lost/HandleKey） |
| `region.go` | 0（新） | ~50 | NoKeyboardRegion + KeyboardRegion 两个接口 + 各自 doc 注释（§1.3 C-1 + F10a D2.b Rect() 体现） |
| `add.go` / `delete.go` / `rename.go` | 75 / 73 / 90 | 同 | `fm.state` → `fm.list` 机械替换 |
| `strutil.go` | 275 | ~295 | 收 `drawString` / `clearRect`（§4.5） |
| `session_test.go` | 229 | ~240 | 改写接收者 + 断言目标，case 不减 |

总 diff 以「代码搬家」为主，新增逻辑仅 `hitTest[泛型]` / `handleGlobalKey` / `cycleFocus` / `switchFocus` / `syncPreview` / `selectedFilePath` / `closePicked` 七个小函数（合计 < 70 行）。review 重点在「搬家是否完整、回引用通道是否单向干净」，不在新逻辑。
