# F2 · FileSelector 鼠标交互（点击选行 + 滚轮移光标）实现方案

**前置**：finder 已稳定（F0–F1），F8 已为右侧预览区实现「滚轮滚动预览正文」。本方案只给左侧 finder 列表区加两种鼠标交互，零预览改动。
**依据**：`internal/finder/session.go`（`HandleEvent` / `drawContent` / `moveCursor` / `Open` 布局）、`internal/finder/preview.go`（`handleRightMouse`，重命名自 F8 的 `handlePreviewWheel`）、`internal/action/tab.go`（`Tab.HandleEvent` 鼠标路由）、`internal/action/bufpane.go`（`HandleEvent` modal 转发、`SetActive` 失焦通道）、`cmd/micro/micro.go`（顶层事件分发）。
**交付**：finder 打开期间——① 鼠标左键点文件/子目录/面包屑行 → 选择光标移到该行；点空白（条目下空区 / 边框 / hint 行 / 预览区）什么都不做；② 滚轮在 finder 列表区上下 → 等同按 ↑/↓（每次 1 行）。
**原生侵入**：零。改动仅限 `internal/finder/session.go`。

---

## 1. 背景与目标

### 1.1 现状

finder 列表区目前**完全不响应鼠标**：

- `HandleEvent` 收到 `*tcell.EventMouse` 后只调 `handleRightMouse`（原 `handlePreviewWheel`，重命名后语义为「收到右栏全部鼠标事件，内部只识别 WheelUp/Down」）。坐标落在 finder 列表区时，`whereIsMouse` 返回 `mouseLeft`，事件转给 `handleLeftMouse`；落在 preview 区时同理。
- 所以现在点 finder 里的文件行、子目录行、面包屑行都没反应；滚轮在列表区也没反应（滚轮在预览区才动，那是 F8）。

### 1.2 目标

给 finder 列表区加两种鼠标交互，行为严格对齐已有键盘语义：

- **点击选行**：左键点中「面包屑行 / 某条目行」→ 光标移到该行（等价把 ↑/↓ 按到那行）。点中其它位置（条目下方的空白、上下边框、底部 hint 行、整个预览区）→ 无反应。**仅移动光标，不 activate**（不 Enter、不进目录、不开文件）——这符合「单击 = 选中」的通用约定，也精确匹配需求「将选择光标移动到对应项」。
- **滚轮移光标**：滚轮在列表区上下 → 等同按 ↑/↓ 各一次（每次 1 行）。预览区的滚轮行为（F8：滚预览正文）不变。

---

## 2. 关键发现

### 2.1 鼠标事件送到 finder 的路径与 F2 无关

owner pane（`BufPane.HandleEvent`）在 finder 开着时会透传 `tcell.Event` 给 `finder.Session.HandleEvent`。这就是 F2 唯一需要关心的“事件从哪里来”：

```
BufPane.HandleEvent → finder.IsOpen() → finder.Session.HandleEvent(event)
```

事件路径中的 tab 路由 / 顶层分发 / 弹窗隔离都是路由层的事，本方案一行不动；F2 只在 `finder.Session.HandleEvent` 内部把现在被丢弃的 `*tcell.EventMouse` 接住、按坐标分派到左栏 / 右栏 / 栏外。

### 2.2 弹窗期间 finder 收不到鼠标（天然隔离）

rename/delete 弹窗都走 `dialog.TheFloatFrame`，这是 modal 浮窗，开着时会拦截所有事件，根本到不了 owner pane 的 `HandleEvent`，自然也不会到达 finder。F2 无需在 finder 内部判「弹窗是否开着」——开着时 finder 压根不被调用，零额外 guard。

---

## 3. 坐标映射（核心新增逻辑）

finder 内容区的屏坐标布局（全部从 `fm.rect` + `fm.state` 推导，与 `drawContent` 完全对齐）：

```
fm.rect.Y        ───  上边框 ──<Open File>──…
fm.rect.Y + 1    ───  面包屑行（cursor=0）         ← 点击 → cursor=0
fm.rect.Y + 2    ───  条目行 0（showEntries[topIdx]）  ← 点击 → cursor=topIdx+1
fm.rect.Y + 3    ───  条目行 1                       ← 点击 → cursor=topIdx+2
  …                 （共 visibleH = min(len, listH) 行）
fm.rect.Y + 2+visibleH  ──  空白行（条目少时）       ← 点击 → 无反应
  …
fm.rect.Y + listH + 2  ──  hint 行（perms/size/mtime） ← 点击 → 无反应
fm.rect.Y + fm.rect.H - 1  ──  下边框 ──…            ← 点击 → 无反应
```

横向：内容画在 `[fm.rect.X, fm.rect.X + fm.state.pickerW)`（`pickerW` = 内容宽 = 外框宽 − 1，已扣掉右侧 `│` 分隔符列）。落在这个 x 区间外（分隔符列、预览区）的点击不归列表区管。

「点中哪一行」的判定（纯函数，无任何 screen 调用，可单测）。只取 my：横向是否在 finder 内容区由 Session 入口的 `whereIsMouse` 保证（`mx` 已被过滤），函数内部不重复判区域：

```go
// listRowAt 把 finder 内容区内的屏坐标映射成目标 cursor 值。
// 入口处 whereIsMouse 已保证 mx 落在 finder 内容区，函数内不再重复判横向。
// 命中面包屑行 → 0；命中可见条目行 → topIdx+(my-listTop)+1；
// 落在空白 / hint / 边框 / 越界 → ok=false（调用方据此 no-op）。
func (fm *Session) listRowAt(my int) (cursor int, ok bool) {
    s := fm.state
    bcRow := fm.rect.Y + 1     // 面包屑行
    listTop := fm.rect.Y + 2   // 条目首行
    if my == bcRow {
        return 0, true
    }
    visibleH := len(s.showEntries)
    if visibleH > s.listH {
        visibleH = s.listH
    }
    if my >= listTop && my < listTop+visibleH {
        idx := s.topIdx + (my - listTop)
        return idx + 1, true // cursor = showEntries 索引 + 1
    }
    return 0, false // 条目下方空白 / hint / 边框 / 越界
}
```

注意 `visibleH` 用 `min(len(showEntries), listH)`——条目比视口少时，只到最后一行可点；多于视口时只点得到当前可见的 `listH` 行（这正是「视口内点击」的语义，点不到的行先靠滚轮/键盘滚出来）。`▲/▼` 滚动指示符画在首/末可见条目行的 scroll 列上，那列在内容区内，点它仍算点中那一行（合理）。

---

## 4. 改动清单

**唯一改动文件**：`internal/finder/session.go`。preview.go、tab.go、bufpane.go 全不动。

### 4.1 `HandleEvent` 的鼠标分派（1 处微调）

finder 打开期间，所有鼠标事件都先到达 `finder.Session.HandleEvent`。入口只负责「按坐标落在哪个区域」把原始 `*tcell.EventMouse` 送达对应栏的 handler，不识别事件是什么。各 handler 收到本栏内的全部鼠标事件后，自己去识别 Button1 / WheelUp/Down / 释放 / 其他，再决定如何处理。其余位置（不在任何栏内的事件）直接 no-op。

入口设计为「坐标 → 枚举位置 → switch 分发」，区域判断本身不返回 bool，而是返回一个枚举（以后加栏不需改函数签名，只需加枚举值）：

```go
// mousePos 是鼠标坐标落在 finder 内的哪个区域；定义在 session.go。
// 入口 switch 这里只枚举现有区域，新加区域只需扩枚举 + 加 case，不动现有栏。
type mousePos uint8

const (
    mouseOutside mousePos = iota // 坐标不在任何栏内（边框、分隔符列、栏外）→ no-op
    mouseLeft                    // 坐标落在 finder 左栏（面包屑/条目/空白/hint）
    mouseRight                   // 坐标落在 preview 右栏
)

// whereIsMouse 把鼠标坐标转换成对应的 mousePos。
func (fm *Session) whereIsMouse(mx, my int) mousePos {
    s := fm.state
    // 左栏优先判断：finder 内容区是「左边一块」，以它为起点看 my 范围。
    // 横向在 [rect.X, rect.X+pickerW)，纵向在 [rect.Y+1, rect.Y+1+listH+1)：
    // 上含面包屑、下不含 hint 行（hint 与下边框不是条目，不归左栏），
    // 也严格不包含底边框行（rect.Y+1+listH+1 == rect.Y+h-1，listH+2 == h-1）。
    if mx >= fm.rect.X && mx < fm.rect.X+s.pickerW &&
        my >= fm.rect.Y+1 && my < fm.rect.Y+1+s.listH+1 {
        return mouseLeft
    }
    // 右栏：落在 preview 矩形内。注意：需要同时排除「上面被左栏占用」的左部分；
    // 由于上面左栏横向已独占了 finder 内容区，这里只需判 pvRect 命中即可。
    r := s.pvRect
    if mx >= r.X && mx < r.X+r.W && my >= r.Y && my < r.Y+r.H {
        return mouseRight
    }
    return mouseOutside
}

// session.go  HandleEvent 内
if ev, ok := event.(*tcell.EventMouse); ok {
    mx, my := ev.Position()
    switch fm.whereIsMouse(mx, my) {
    case mouseRight:
        fm.handleRightMouse(ev) // 右栏全部鼠标事件，内部自己识别滚轮
    case mouseLeft:
        fm.handleLeftMouse(ev)  // 左栏全部鼠标事件，内部自己识别点击/滚轮
    case mouseOutside:
        // 不在任何栏内的事件（上下边框、分隔符列、栏外）→ no-op
    }
    return
}
```

`whereIsMouse(mx, my)` 是 Session 入口唯一定位函数，返回 `mousePos` 枚举。各 handler 内部不再判区域，只识别事件类型。设计上这样安排的好处：

- 入口是「一个 switch、一个枚举」，加栏不需改函数签名（不增加 bool 返回值的可能性），只需扩枚举 + 加 case，不动现有栏。
- 入口只送「该栏的全部鼠标事件」，与栏内如何识别、识别出哪些事件类型彻底解耦。
- 后续如果要加鼠标交互（例如 preview 区加 WheelLeft 翻页、finder 区加右键重命名预览等），只需在对应 handler 内部 switch 里补 case，不需要再动入口。
- 不会发生「同一个鼠标事件被两个 handler 重复处理」（枚举互斥）；也不会发生「该栏事件被另一栏拦截」（入口按枚举独占送）。

### 4.2 新增 `handleLeftMouse` + `listRowAt`（同文件，紧挨 Controller 区）

```go
// handleLeftMouse 收到 Session 入口送来的「左栏内的全部鼠标事件」，由它内部
// 识别事件类型并处理：左键点中面包屑/条目行 → 移光标；滚轮上下 → 等同 ↑/↓。
// 区域外的事件已经在入口被筛掉，到这里一定是落在 finder 内容区内的。
func (fm *Session) handleLeftMouse(ev *tcell.EventMouse) {
    s := fm.state
    switch ev.Buttons() {
    case tcell.Button1:
        _, my := ev.Position()
        target, ok := fm.listRowAt(my) // 横向已由入口 whereIsMouse(mouseLeft) 保证
        if !ok {
            return // 空白 / 边框 / hint：什么都不做
        }
        if target == s.cursor {
            return // 点的就是当前行：免一次无谓重绘
        }
        fm.moveCursor(target - s.cursor) // clamp+ensureVisible+refreshPreview+Redraw 全在里面
    case tcell.WheelUp:
        fm.moveCursor(-1)
    case tcell.WheelDown:
        fm.moveCursor(1)
    }
}
```

### 4.3 新增 `handleRightMouse`（preview.go，与原 `handlePreviewWheel` 同位置）

```go
// handleRightMouse 收到 Session 入口送来的「右栏内的全部鼠标事件」，由它内部
// 识别事件类型并处理：滚轮上下 → 滚动 preview 正文。
// 区域外的事件已经在入口被筛掉，到这里一定是落在 preview 矩形内的。
func (fm *Session) handleRightMouse(ev *tcell.EventMouse) {
    switch ev.Buttons() {
    case tcell.WheelUp:
        fm.scrollPreview(-previewScrollStep)
        screen.Redraw()
    case tcell.WheelDown:
        fm.scrollPreview(previewScrollStep)
        screen.Redraw()
    }
}
```

入口的 `whereIsMouse` 返回 `mouseRight` 后事件才到这里，所以原来的「不在 pvRect 就 return」边界判定被入口替掉了，这里只需识别事件类型。

`listRowAt` 见 §3（这里只取 my，mx 已经被入口 whereIsMouse 保证落在左栏）。`handleRightMouse` 是 preview.go 里原本 `handlePreviewWheel` 的重命名：现在它收到的是「右栏内的全部鼠标事件」，内部 switch Buttons 只识别 WheelUp/WheelDown 滚动正文，其他 no-op——与左栏一致，「送全部 + 内部识别」。

whereIsMouse 在哪边先判顺序：左栏先判（finder 是“左部那一块”），然后右栏判 pvRect，两者互不重叠，返回值唯一。最后只剩 mouseOutside。

区域定位全部集中在 Session 入口的 `whereIsMouse(mx, my) -> mousePos`，handler 内部不重复判定；preview 那边同样——区域判定只在入口的 `whereIsMouse` 做，handler 不再判区域。

说明：入口 `whereIsMouse` 返回枚举位置，switch 独占地送本栏全部鼠标事件，handler 内部 switch Buttons 识别事件类型。

---

## 5. 复用项（不新写）

| 能力 | 复用位置 | F2 怎么用 |
|---|---|---|
| 光标移动 + clamp + 视口滚动 + 预览刷新 + 重绘 | `session.go` `moveCursor(delta)` | 点击 = `moveCursor(target-cursor)`；滚轮 = `moveCursor(±1)` |
| 右栏鼠标 | `preview.go` `handleRightMouse`（重命名自 `handlePreviewWheel`：收到右栏全部事件，内部识别 WheelUp/Down） |
| 屏坐标→行 映射所需的布局常量 | `fm.rect` / `fm.state.pickerW` / `fm.state.listH` / `fm.state.topIdx` | `listRowAt` 直接读，与 `drawContent` 同源 |
| 重绘 | `screen.Redraw()`（由 moveCursor 调） | 不另调 |

已搜过的「现成机制」：`moveCursor`、`ensureVisible`、`refreshPreview`、`handleRightMouse`（原 `handlePreviewWheel`）、`SetActive` 失焦通道、floatFrame modal 隔离——均无需新写，直接复用或维持现状。

---

## 6. 边界行为

| 场景 | 行为 |
|---|---|
| 左键点面包屑行 | 光标移到 cursor=0（等同 ↑ 到顶）；预览显 `Select a file` |
| 左键点某文件/子目录条目行 | 光标移到该行；预览随光标刷新（文件显内容、目录显占位） |
| 左键点「当前已选中行」 | no-op（不重绘） |
| 左键点条目下方的空白行 | no-op |
| 左键点上/下边框、底部 hint 行 | no-op |
| 左键点预览区 | no-op（预览只读，F8 既有行为） |
| 左键点右侧 `│` 分隔符列 | no-op（横向不在 `[X, X+pickerW)` 内） |
| 滚轮在列表区上下 | 光标 ±1 / 格，等同 ↑/↓；到顶/底 clamp 不越界（moveCursor 已 clamp） |
| 滚轮指针在「条目少时的空白行」上 | 仍响应滚轮（`whereIsMouse` 在内容区纵向放宽，但横向只认左栏） |
| 滚轮在预览区上下 | 滚预览正文（F8，不变） |
| 按住左键在条目间拖动 | 每个拖动事件都重算目标行并 moveCursor → 光标跟手指走（可接受的副效果，见 §7 #4） |
| 左键按下后松开（release，ButtonNone） | `handleLeftMouse` 的 switch 不匹配任何 case → no-op（不会重复移动） |
| rename/delete 弹窗开着时点鼠标 | 事件被 floatFrame 拦走，finder 收不到 → 弹窗自己处理，互不干扰 |
| 多 pane：左键点别的 pane | 既有行为：owner 失焦 → finder close(Esc)；本方案不动这条路径 |
| 运行中 resize | 既有：finder close(Resize)，本方案无影响 |
| owner pane 极窄（finder 仍能开） | 布局常量照常算，点击/滚轮按实际行列映射，无误 |
| 光标在目录行时点它 | 只选中、不进入（不 activate）；要进入按 →/Enter |

---

## 7. 设计取舍

| # | 决定 | 理由 |
|---|------|------|
| 1 | **点击只移光标、不 activate** | 需求原文「将选择光标移动到对应项」。单击=选中是通用约定；进目录/开文件交给 →/Enter 或未来的双击（§9 #1）。避免了「点错就开错文件」的破坏性。 |
| 2 | **滚轮 = ±1 行（同 ↑/↓），不用预览的 3 行步长** | 需求原文「等同于用户按上键/下键」。预览滚的是「正文内容」用 3 行手感合理；finder 滚的是「光标」必须 1 格 1 个条目，语义不同，各取所需。 |
| 3 | **按区域送「栏内全部鼠标事件」，各 handler 内部识别事件类型** | 入口只决定送给谁（左栏 / 右栏 / no-op），不识别事件类型；handler 收到本栏全部事件后，自己 switch Buttons 决定如何处理。各栏互不越界（区域互斥），同一个事件不会被两个 handler 重复处理。 |
| 4 | **拖动也会 moveCursor（光标跟手指）** | tcell 拖动 = 持续 Button1 事件，finder 无 `release` 状态机可区分「首按 vs 拖」。让光标跟随是最省事且符合直觉的（拖过文件列表边看边选）。要严格「仅首按」需引入 press 状态，不值当。 |
| 5 | **同行点击直接 return 免重绘** | 点当前行是高频操作（确认选中），`moveCursor(0)` 会走一遍 refreshPreview(no-op)+Redraw。一行 `if target==cursor return` 省掉，零副作用。 |
| 6 | **`whereIsMouse` 返回枚举，横向纵向都在左栏独站** | 条目少于视口时，指针停在中部空白行滚轮也应能翻光标；横向必须在左栏范围内才返回 `mouseLeft`，避免 preview 区被误判给 finder 逻辑。 |
| 7 | **坐标映射全部从 `fm.rect`+`fm.state` 推导，不缓存** | 这些字段由 `Open` 一次性算好、resize 时 finder 整体重开（既有），运行期恒定。每帧现算几个 int 比较，开销可忽略，且永远和 `drawContent` 画的位置一致（同源），不会出现「画在第 N 行却点不中」的漂移。 |
| 8 | **把 `handlePreviewWheel` 重命名为 `handleRightMouse`** | 现在它收到的是右栏的全部鼠标事件（不只是 WheelUp/Down），名字与职责一致；内部 switch 仍然只识别 WheelUp/Down 滚动，其他事件 no-op。 |
| 9 | **不引入 press/drag 状态字段** | finderState 现有无鼠标状态机；为「单击」加状态字段是过度设计。用「Button1 事件即移动」+「同行免重绘」已足够覆盖需求。 |

---

## 8. 验证

### 8.1 单元测试（纯逻辑，无需 screen）

新增 `internal/finder/session_test.go`（包内 `package finder`），表驱动测 `listRowAt` 与 `whereIsMouse`——两者只读 `fm.rect` / `fm.state` 的 int 字段、不碰 screen，可直接手搓 `finderState` 构造场景：

- `listRowAt`：
  - 点面包屑行 `(X, Y+1)` → `(0, true)`
  - 点首条目行 `(X, Y+2)` → `(topIdx+1, true)`
  - 点第 k 可见条目行 → `(topIdx+k+1, true)`
  - 点分隔符列 `(X+pickerW, Y+2)` → `(_, false)`
  - 点预览区 `(X+pickerW+5, Y+2)` → `(_, false)`
  - 点上边框 `(X, Y)` / 下边框 / hint 行 `(X, Y+listH+2)` → `(_, false)`
  - 条目少于视口时点空白行 `(X, Y+2+len)` → `(_, false)`
  - `topIdx>0`（滚过视口）时首行映射到 `topIdx+1`
- `whereIsMouse`：
  - 左栏面包屑行 / 条目行 / 空白行 / hint 行 → `mouseLeft`
  - 分隔符列 → `mouseOutside`
  - preview 区所有点 → `mouseRight`
  - 上下边框行 → `mouseOutside`
  - 左栏 × 右栏之外的区域 → `mouseOutside`

注意：不要在单测里调 `moveCursor` / `handleLeftMouse`——它们依赖 `screen.Redraw()` 和 `os.Open`（refreshPreview→loadFile），需真 screen + 真文件，留给手测。`listRowAt` 和 `whereIsMouse` 都是纯坐标逻辑，可在无 screen 条件下覆盖点击与滚轮的全部区域边界。

### 8.2 手测清单（`make build` 后）

- [ ] finder 打开，左键点第 3 个条目 → 光标高亮跳到第 3 行，右侧预览切到该文件（若是文件）/显占位（若是目录）。
- [ ] 左键点面包屑行 → 光标到顶（面包屑高亮），预览显 `Select a file`。
- [ ] 左键点子目录行 → 仅选中（高亮），不进入；再按 → 才进目录。
- [ ] 左键点当前已选中行 → 无视觉变化（不闪、不重绘错位）。
- [ ] 左键点条目下方的空白（条目少时）→ 无反应。
- [ ] 左键点上/下边框、底部 hint（perms/size/mtime）行 → 无反应。
- [ ] 左键点预览区任意位置 → 无反应（预览只读）。
- [ ] 左键点右侧 `│` 分隔符列 → 无反应。
- [ ] 滚轮在列表区向上 → 光标上移 1 行（等同 ↑）；向下 → 下移 1 行（等同 ↓）。
- [ ] 滚轮把光标滚到顶/底 → clamp 不越界（不循环、不越过面包屑/末条目）。
- [ ] 滚轮指针停在「条目少时的空白行」上 → 仍能上下移光标。
- [ ] 滚轮在预览区 → 滚的是预览正文（F8 行为不变），不动 finder 光标。
- [ ] 按住左键从条目 A 拖到条目 B → 光标跟随到 B（可接受）。
- [ ] 左键按下再松开（完整 click）→ 只移动一次（松开不重复移动）。
- [ ] 按 `r` 开 rename 弹窗 → 此时点鼠标 → 事件归弹窗（finder 不响应、不乱动光标）；Esc 关弹窗后 finder 鼠标恢复。
- [ ] 多 pane（VSplit）：左键点 owner pane 内的 finder 行 → 正常选中、finder 不关；左键点另一个 pane → finder 按 Esc 关（既有行为）。
- [ ] 运行中 resize → finder close(Resize)（既有），无残留。
- [ ] owner pane 极窄（finder 勉强能开）→ 点击/滚轮按实际行列映射，无误选中。

---

## 9. 范围外（记录，不在本次做）

| # | 项 | 说明 |
|----|------|------|
| 1 | **双击打开/进入（activate on double-click）** | 本期单击只选中。双击 = 选中 + Enter（文件打开 / 目录进入）可作为后续增强，需要引入 click 时间窗判定，另开方案。 |
| 2 | **拖动选区间 / 框选** | 本期拖动只是「光标跟手指」的副效果，不做多选。多文件操作见 `Z0-多文件操作.md`。 |
| 3 | **列表区滚动条拖拽定位** | `▲/▼` 目前只是指示符；点击/拖拽它做长距跳转属另一特性。本期点中它等同点中那一行。 |
| 4 | **鼠标侧键 / 水平滚轮** | 仅接 WheelUp/Down + Button1；WheelLeft/Right 及侧键不处理。 |
| 5 | **预览区点击定位/选中** | 预览纯只读（F8 约束），点击仍 no-op。 |

---

## 附：与既有架构的对应

| 本方案 | 既有代码 |
|--------|---------|
| §2.1 Button1 直达 finder 不关会话 | `tab.go` `Tab.HandleEvent` button-press 分支（wasReleased→SetActive→fall-through）；`bufpane.go` `SetActive` 早 return（`IsActive()==b`） |
| §2.1 点别的 pane 关 finder | `bufpane.go` `SetActive(false)` → `fileops.go` `onOwnerBlur` → `finder.NotifyBlur` |
| §2.1 滚轮直达 finder | `tab.go` `Tab.HandleEvent` default 分支（wheel→`p.HandleEvent`+return） |
| §2.2 弹窗期 finder 收不到鼠标 | `cmd/micro/micro.go` 顶层 `dialog.TheFloatFrame.IsOpen()` 先于 Tabs |
| §2.4 光标移动复用 | `session.go` `moveCursor`（clamp+ensureVisible+refreshPreview+Redraw） |
| §3 坐标布局 | `session.go` `drawContent`（`x=rect.X`、`y=rect.Y+1`、`listTop=y+1`、`visibleH=min(total,listH)`）、`Open`（`pickerW`/`listH`/`pvRect`） |
| §4.3 右栏鼠标 | `preview.go` `handleRightMouse`（原 `handlePreviewWheel`：入口已由 `whereIsMouse` 分派，内部识别 WheelUp/Down） |
