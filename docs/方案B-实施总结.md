# 方案B 实施总结 — screenBuffer 单一数据源架构

> **本文档定位**：方案 B（v1.0.6）落地的工程总结，记录最终架构、关键决策、踩坑档案与调试方法论。
> **目标读者**：未来给 microNeo 添加新渲染特性、或重构 MD 显示管线的开发者。
> **前置文档**：
> - `docs/光标滚动-修改总结.md` — v1.0.5 的 viewportRowmap 方案（**已被方案 B 取代**，本文是其继任者）
> - `docs/光标滚动-方案B.md` — 方案 B 的原始设计意图
> - `docs/方案B-实施计划合并版001.md` — 实施时的详细伪代码蓝图
>
> **时间线**：v1.0.5（2026-06-15，viewportRowmap 影子数组）→ v1.0.6（2026-06-16，screenBuffer 单一数据源）

---

## 1. 为什么推倒 v1.0.5

v1.0.5 用一个 **`viewportRowmap []SLoc` 影子数组**做"屏行 ↔ buffer 行"的双向映射：渲染时
逐帧写屏 + 同步写 rowmap，`Relocate` 时查 rowmap 算滚动。它解决了"光标不上滚"的燃眉之急，
但留下了两个**方案级缺陷**（见旧总结 §5）：

1. **跨段切换时光标偶发消失一帧**——"光标段原生渲染特权"让屏行预算分配突变。
2. **MD 段 segmentRow 计数与原生 `c.Row` 不一致**——跨段首次按键走 fallback 近似。

根因都是同一个：**屏幕被画了两遍**（一次到真屏 tcell、一次到影子数组 rowmap），两者时序和
坐标系若即若离，装饰行/softwrap 续行的边界条件极难对齐。

方案 B 的核心思想：**只画一遍**。渲染目标是 `screenBuffer`（单一数据源），显示时 blit 到真屏。
所有查询（滚动判定、点击映射、光标定位）都查同一个 sb，不存在"两份拷贝不一致"的问题。

---

## 2. 核心架构：四阶段管线

```
buffer 变更 ──► DetectSegments() ──► b.MDSegments（事件驱动，与屏宽无关）
                                          │
                                          ▼
按键/鼠标 ──► Relocate ──► relocateVerticalMD ──► displayToBuffer ──► screenBuffer（sb）
                  │              （case A/C 选起点）  （唯一一次渲染）    （单一数据源）
                  │                                                     │
                  └──► 微调 StartLine ◄── 查 sb ◄──────────────────────┤
                                                                       ▼
Display() ──► displayBufferMD ──► [needRender?] displayToBuffer ──► showBuffer（blit 到真屏）
                  （失效检查 coversExtent）                                  │
                                                                            ▼
                                                                       tcell 真屏
```

### 2.1 四个职责单一的阶段

| 阶段 | 函数 | 职责 | 频率 |
|------|------|------|------|
| **检测** | `buffer.DetectSegments()` | 算全 buffer 的 segments（表格/代码块/标题…），与屏宽无关 | buffer 变更时（事件驱动） |
| **渲染** | `displayToBuffer(startLine, showCursor)` | 把可见 segments 渲染进 `sb`（cells + row 元数据） | 滚动/编辑时 |
| **查询** | `relocateVerticalMD` / `LineToScreenRow` / `ScreenRowToLine` | 查 sb 算滚动/映射 | 每次 Relocate |
| **显示** | `showBuffer(startLine)` | 把 sb 的一段 blit 到真屏 | 每帧 |

**关键不变量**：渲染只发生在 `displayToBuffer`。`showBuffer` 纯 copy，`relocateVerticalMD`
纯查询——三者都不再写屏内容。这消除了 v1.0.5 "双写不一致"的根源。

### 2.2 两条入口路径

- **Relocate 路径**（方向键 / goto / search）：`relocateVerticalMD` → `displayToBuffer` 拿 fresh sb →
  查 sb 微调 `StartLine`。sb 是新的，坐标系绝对正确。
- **Display 路径**（每帧）：`displayBufferMD` 先做失效检查（`coversExtent`），sb 仍有效则**跳过渲染直接 blit**，
  否则重渲染。这是 idle / 同段内移动时不重渲染的优化基础。

---

## 3. 关键数据结构

### 3.1 screenBuffer（`bufwindow_md.go:1254`）

```go
type screenBuffer struct {  // bufwindow_md.go:1254
    rows      []screenRow // 长度 = 2×bufHeight（预分配容量，吸收跨段展开增量）
    startLine SLoc        // 本批 rows 渲染时的起点
    nContent  int         // ★ 实际有效内容行数（rows 中 line≠-2 的）；coversExtent 用它而非 len(rows)
    lastLine  int         // ★ 实际渲染到的最后一个 buffer 行号；coversExtent 的 buffer-end 例外判断用
    overflow  bool        // 渲染到 vY≥cap（2×bufH）→ true，触发原生 fallback
    cursorX/Y int         // 光标在 sb 坐标系的绝对位置（showBuffer blit 时减 startVY）
    cursorOK  bool        // 是否记录到有效光标
    editMode  bool        // 本次 render 时的 editMode（跨帧比较，切换时强制重渲染）
    blitStart int         // 上次 blit 的 rows 起始下标（点击映射用）
    // …width/origin 等
}
```

### 3.2 screenRow 的 `(line, segRow)` 二元组

```go
type screenRow struct {
    cells  []mdCell
    line   int  // -2=未写（预分配空白）; -1=装饰行（表格 frame/标题下划线/代码块边框）; ≥0=内容行
    segRow int  // 该屏行是此 buffer 行的第几个 softwrap 段（装饰行=-1）
}
```

**为什么是二元组而非单值**：MD 装饰行（`-1`）和内容行续段（`segRow++`）若只用单值 `line`，
倒序反查会命中装饰行或续行末尾。`(line, segRow)` 精确匹配天然排除装饰行（`-1 ≠ c.Line`）和
错误段（`Row` 不一致）。这一点 v1.0.5 的 2D rowmap 已经证明，方案 B 直接继承。

### 3.3 渲染器如何写元数据（同源写入）

渲染分两条路径，**都直接写 sb**（不返回值再二次赋值）：

- **`renderSegmentNative`**（光标段，editMode）：`setRowMeta(vY, bloc.Y, screenRow-lineStartVY)`
- **`renderSegmentMD`**（其他段）：按 `row.BufLine` 累加推算 segRow，装饰行存 `{Line:-1, segRow:-1}`

`setRowMeta` 是唯一的元数据写入点，`renderSegmentMD` 内部对每个 cell 还调 `setCell` 写 cells。
cells 和元数据来自同一次渲染，永不漂移。

---

## 4. 渲染决策：MD vs Native

```go
// displayToBuffer 主循环（bufwindow_md.go:883）
if showCursor && w.editMode && hasCursorInside(seg, cursors) {
    vY = w.renderSegmentNative(seg, vY)  // 光标段走原生（可编辑）
    cursorShowed = true
} else {
    vY = w.renderSegmentMD(seg, vY)       // 其他段走 MD 渲染
}
```

- **editMode=true（编辑模式）**：光标所在段渲染成原生格式（可见 `#`、`|` 等原始字符），便于编辑；
  其他段渲染成 MD（标题加粗、表格成框等）。
- **editMode=false（阅读模式）**：全部 MD 渲染，光标段也无特权。

这是 microNeo "打开就看到完整渲染，光标处可编辑"体验的基础。

### 4.1 三目的停止条件（防止渲染不足或溢出）

```go
canBreak := g1 && g2 && g3
g1 = oldSegDone        // 旧光标段必须完整渲染（跨段切换时旧段装饰行不能截断）
g2 = cursorShowed      // cursor 段必须已渲染（showCursor=true 时）
g3 = vY >= bufH+margin // 必须渲染到 viewport 底边以下，留足 Relocate 微调余量
```

**⚠️ 这三个条件是 §8 Bug #3/#4 的源头**——其中任何一个永远无法满足，循环就会一路跑到
`vY >= cap`（2×bufHeight）触发 overflow fallback。设计时必须保证每个条件在合法场景下都能达成。

---

## 5. 滚动逻辑：relocateVerticalMD（`bufwindow_md.go:1128`）

### 5.1 三步流程（`bufwindow_md.go:1128`）

```go
// 1. 选渲染起点 displayStart（cheap 预判）
// 2. displayToBuffer(displayStart, true) 唯一一次渲染
// 3. 查 sb 微调 StartLine，让 cursor 落在期望 margin
```

### 5.2 case A vs case C（渲染起点选择）

```go
if sb.coversLine(c.Line) && curRow < height {
    displayStart = w.StartLine      // case A：连续导航，上帧 sb 仍有效
} else {
    displayStart = c.Line - margin  // case C：跳远，1:1 估算
    w.StartLine = displayStart      // ★ case C 必须立即采用，否则舒适区分支不更新 StartLine
}
```

**case A/C 的判定陷阱**（§8 Bug #2）：不能只用 `coversLine`。jump 到远处时，旧 sb 虽然**覆盖**
cursor 行（buffer 行在范围内），但 cursor 已在 sb 的**第二屏**（`curRow >= height`）——此时旧
StartLine 对新 cursor 无效，必须走 case C 重估。

### 5.3 微调：让 cursor 落在 margin

```go
cursorRow := sb.rowIndexOf(c)
botMarginRow := height - 1 - scrollmargin

if cursorRow > botMarginRow {        // SCROLLUP：光标超底边
    delta := cursorRow - botMarginRow
    loc := sb.slocAt(delta)
    if loc.Line < 0 {                // delta 落装饰行 → 向下找首个内容行
        ...decor-skip...
    }
    w.StartLine = loc                // 推进 StartLine
}
if cursorRow < scrollmargin {        // SCROLLDOWN：光标超顶边
    w.StartLine = w.Scroll(c, -scrollmargin)
}
// 否则：cursor 在舒适区 [scrollmargin, botMarginRow]，不动
```

`StartLine` 精确到 buffer 行级（非 softwrap 下 Row 恒 0），装饰行通过 `firstContentBufLine`/
`lastContentBufLine` 绑定内容行，随内容行正确滚动——这点继承自 v1.0.5。

---

## 6. 失效判定：coversExtent（最容易出 bug 的地方）

`displayBufferMD` 每帧决定是否重渲染，靠 `sb.coversExtent(StartLine, height, bufferLines)`：

```go
func (s *screenBuffer) coversExtent(sl SLoc, height, bufferLines int) bool {
    if s.nContent == 0 { return false }
    if sl.Line < s.startLine.Line { return false }          // ★ 起点不能在 sb 之前
    startVY, _ := s.rowIndexOf(sl)                           // 屏幕行精确（非 buffer 行估算）
    if startVY+height <= s.nContent { return true }         // 整个 viewport 在有效内容内
    if s.lastLine >= bufferLines-1 { return true }          // 已渲染到 buffer 末尾（EOF 空白合法）
    return false
}
```

**这个函数经历了 4 轮 bug 修复（§8 #3/#4/#5/#6）**，是整个方案 B 最微妙的部分。三个要点：

1. **用 `nContent`（实际写入行数）而非 `len(rows)`（=2×bufHeight 预分配容量）**。否则 sb 只写了
   38 行但 cap 是 70，`covers` 会误以为覆盖 70 行 → 尾部空白。
2. **用屏幕行精确判断（`rowIndexOf+height <= nContent`）而非 buffer 行估算**。MD 表格/代码块
   展开后 buffer 行与屏幕行非 1:1，buffer 行算术会系统性误判。
3. **起点必须在 sb 范围内**（`sl.Line >= startLine.Line`）。鼠标向上回滚时 StartLine 会减到
   sb.startLine 之下，若放行，`rowIndexNearest` 会错误返回 sb 第一行 → 画面卡死。

---

## 7. 调试方法论

### 7.1 编译期诊断日志（`microNeoDebug`）

```go
const microNeoDebug = true  // 定位后改 false 即零开销，无需删埋点

func dbgLog(format string, args ...any) {
    // 追加写 /tmp/microNeo_debug.log，带毫秒时间戳
}
```

**为什么用编译期常量而非 log 库**：MD 渲染每帧执行多次，运行时日志库开销会扭曲时序敏感的
bug（如光标位置漂移）。编译期常量让 Go 编译器在 `false` 时直接消除调用，零成本保留埋点。

### 7.2 日志埋点位置（五个关键点）

1. **`relocateVerticalMD`**：ENTER 参数、case A/C 选择、cursorRow/botMarginRow、SCROLLUP 的
   delta/slocAt/decor-skip、EXIT 新 StartLine。
2. **`displayToBuffer`**：ENTER startLine/showCursor、每次迭代的 seg 范围/cursorY/editMode/渲染类型/
   ΔvY、canBreak 的 g1/g2/g3 各分量、EXIT written/firstLine/lastLine。
3. **`displayBufferMD`**：needRender 及其各项分量。
4. **`showBuffer`**：sb.startLine、rowIndexOf startVY、BLIT startVY/endVY。
5. **关键决策点**：showCursor 被置 false、coversExtent 各分支、fallback 触发。

### 7.3 调试循环

```
清空 /tmp/microNeo_debug.log
  → 用户复现 bug（明确操作 + 现象 + 涉及行号）
  → 读日志，grep 关键时间戳的完整 trace
  → 对比"预期"与"实际"（StartLine vs sb.startLine、written vs 需要、cursorRow vs margin）
  → 定位根因 → 修复 → 清日志 → 复测
```

**黄金法则**：每个 bug 都能在日志里找到"某个值不符合预期"的精确证据。不要靠脑补，靠 trace。

---

## 8. 踩坑档案（最有价值的章节）

> 这 6 个 bug 都是在"v1.0.5 方案验证通过、方案 B 初版代码跑起来后"才暴露的。
> 它们的共同点：**根因都不在出 bug 的那行代码，而在另一个看似无关的子系统**。
> 未来加功能时，先读这一章。

### Bug #1 — 只显示第一行

| | |
|---|---|
| **现象** | 打开 sample.md，整屏只有第一行，其余全白 |
| **假根因** | renderSegmentMD 的可见性判断 |
| **真根因** | `displayToBuffer` 调 `filterSegmentsToVisible` 算出局部 `segments` 切片后用 `_ = segments` 丢弃；主循环改用 `findSegmentContaining` 查**原始** `b.MDSegments`，而这些段的 `VisibleStart/End` 是零值 → 可见性检查 `effectiveLine > seg.VisibleEnd(0)` 几乎全 false |
| **修复** | 直接迭代过滤后的 `segments` 切片（它携带正确的 VisibleStart/End）；移除 `_ = segments` |
| **教训** | 过滤后的副本和原始集合**语义不同**（后者带可见范围）。永远别丢弃你算出来的局部状态再去查全局。 |

### Bug #2 — goto 跳远后视口错位 / 光标消失

| | |
|---|---|
| **现象** | goto 50 正常；goto 30 后 viewport 显示从 L42 起，光标不见 |
| **假根因** | StartLine 计算错误 |
| **真根因** | case A/C 判定 `sb.coversLine(c.Line)` 只看 cursor 行是否在 sb buffer 范围内。jump 到远处时旧 sb 仍覆盖 cursor 行（如 L74 在 sb [L51,L121] 内），误走 case A → 用旧 StartLine 渲染 → cursor 跑到 sb 第二屏 → 舒适区分支不更新 StartLine → blit 错位 |
| **修复** | case A 增加"cursor 在旧 sb 第一屏内"检查：`curRow < height`。连续 ↓ 时 cursor 在第一屏（row~31），jump 时在第二屏（row~40） |
| **教训** | "覆盖"和"有效"是两个概念。sb 覆盖 cursor 的 buffer 行 ≠ 旧 StartLine 对新 cursor 仍有效。判定状态有效性时要看**相对位置**（第几屏），不只看**绝对范围**。 |

### Bug #3 — scrollup 时光标错位 / 消失到 statusLine 下

| | |
|---|---|
| **现象** | 按 ↓ 跨表格段时，光标落在错误行；离开表格后光标消失到状态栏下 |
| **假根因** | delta 计算错（"scrollup 了 2 行"） |
| **真根因** | `sb.cursorY` 是光标在 sb 的**绝对行号**（从 sb.startLine 算起），但 showBuffer 从 `startVY` 开始 blit（screen row 0 = sb[startVY]）。旧代码把光标画在 `w.Y + cursorY`，没减 startVY → scrollup 让 StartLine 推进后（startVY>0），光标画低了 startVY 行 |
| **修复** | `curScreenY := cursorY - startVY`，加 `[0, bufHeight)` 边界保护 |
| **教训** | 坐标系不一致是显示 bug 的头号杀手。cursorY 是"sb 内部绝对坐标"，screen 是"viewport 相对坐标"，blit 时必须做 `绝对 - 偏移 = 相对` 的转换。任何"存绝对值"的字段在被画到子窗口时都要减偏移。 |

### Bug #4 — 鼠标滚动后 L35/L70 之后空白

| | |
|---|---|
| **现象** | 鼠标滚到中间，line35 之后全白；滚到末尾，line70 之后全白 |
| **假根因** | 渲染不足 |
| **真根因** | 失效检查用 `sb.covers()`，它用 `len(rows)`（=2×bufHeight 预分配容量=70）判断范围，但 sb 实际只写了 38 行内容。鼠标把 StartLine 推到 sb 中部时，`covers` 仍 true → 不重渲染 → viewport 尾部（L49）超出实际内容末尾（L34）→ 空白 |
| **修复** | 新增 `nContent`（实际写入行数）字段 + `coversExtent`。用 nContent 替代 len(rows) |
| **教训** | **"预分配容量"和"有效内容"永远要分开**。Go 的 `make([]T, cap)` 给的是容量，不是内容。任何基于切片长度判断"是否有效"的逻辑，都要问：len 反映的是容量还是已写内容？这里要单独维护一个计数器。 |

### Bug #5 — 鼠标滚动到 startLine 5-28 全屏变原生格式

| | |
|---|---|
| **现象** | 鼠标滚到 startLine 5-28 区间，整屏显示原始 markdown（`#`、`|`），其他位置正常 |
| **假根因** | editMode 状态错误 |
| **真根因** | 鼠标把 cursor(L0) 滚出 viewport 后，displayToBuffer 的 g2 停止条件 `showCursor && !cursorShowed` **永远无法满足**（cursor 段不在 visible → cursorShowed 恒 false）→ 渲染循环一路跑到 vY≥cap → overflow=true → showBuffer 走 `w.displayBuffer()` **原生 fallback** → 用户看到原始字符 |
| **修复** | displayToBuffer 开头预判 cursor 是否在 `[visibleStart, visibleEnd]`，不在则 `showCursor=false`，g2 不启用 |
| **教训** | **停止条件必须保证可达**。设计循环退出条件时，要问：在所有合法场景下，这个条件都能变 true 吗？如果某个条件依赖外部状态（cursor 位置），而那状态可能不满足，循环就会失控。要么保证可达，要么有硬上限（这里是 cap，但 cap 触发的 overflow fallback 有副作用）。 |

### Bug #6 — 鼠标向上滚动卡住（L58/L12 等多处）

| | |
|---|---|
| **现象** | 鼠标 scroll up 到某处卡住，画面不再向上滚动 |
| **假根因** | coversExtent 尾部判断 |
| **真根因** | `coversExtent` 缺"起点不能在 sb 之前"检查。鼠标向上滚时 StartLine 减到 `sb.startLine` 之下，`rowIndexOf` 失败 → `rowIndexNearest` 兜底**错误返回 sb 第一行**（row 0 = sb.startLine）→ `0+height <= nContent` 误判 true → needRender=false → showBuffer 也 covers=true 不补渲染 → **永远 blit sb 第一行内容 → 画面冻结** |
| **修复** | `coversExtent` 开头加 `if sl.Line < s.startLine.Line { return false }` |
| **教训** | **兜底函数（nearest/fallback）会制造"假成功"**。`rowIndexNearest` 是为装饰行设计的，但当查询点根本不在 sb 范围内时，它返回一个"最近但错误"的结果，让调用方误以为命中。任何用 nearest 兜底的查询，外层必须先用严格范围检查（`sl.Line >= startLine.Line`）过滤掉"根本不在范围内"的情况。**兜底只能处理"范围内但非精确命中"，不能处理"范围外"。** |

---

## 9. 设计原则与陷阱清单

### 9.1 必须坚守的原则

1. **单一数据源**：渲染只发生在 `displayToBuffer`，写到 sb。`showBuffer` 纯 blit，`relocate` 纯查询。
   永远不要让两个地方写"同一份逻辑数据"。
2. **对 micro 原生代码零侵入**：MD 逻辑全隔离在 `*_md.go`；`bufwindow.go` 只加 IsMD 分发缝；
   `actions.go` / `softwrap.go` / `buffer.go`（除既定钩子）不动。非 MD 文件行为字节级不变。
3. **检测与渲染分离**：`DetectSegments`（事件驱动，与屏宽无关）→ `displayToBuffer`（按屏宽布局）。
   不要把分类逻辑塞进渲染循环。

### 9.2 反复踩的陷阱

| 陷阱 | 表现 | 防范 |
|------|------|------|
| **容量 ≠ 有效内容** | 用 `len(slice)` 判断有效范围，但 slice 是预分配的 | 维护独立的 `nContent` 计数器 |
| **绝对坐标 vs 相对坐标** | 存绝对值的字段被画到子窗口时没减偏移 | 画屏前统一做 `绝对 - 偏移 = 相对` |
| **兜底制造假成功** | nearest/fallback 在范围外返回错误值 | 外层先用严格范围检查过滤 |
| **覆盖 ≠ 有效** | sb 覆盖 buffer 行 ≠ 旧 StartLine 对新 cursor 有效 | 判定状态有效性看相对位置（第几屏） |
| **停止条件不可达** | 循环退出条件依赖外部状态，该状态可能不满足 | 保证每个条件在合法场景可达，或硬上限无副作用 |
| **过滤副本 vs 原始集合** | 丢弃局部过滤结果再去查全局，全局没带过滤后的语义 | 直接用你算出来的局部状态 |

### 9.3 坐标系备忘

| 坐标 | 含义 | 例子 |
|------|------|------|
| `buffer.Y` / `c.Y` | buffer 行号（0-indexed） | 光标在 L74 |
| `sb.cursorY` | 光标在 sb 的绝对行（从 sb.startLine 算） | sb 从 L51 起，cursor L74 → cursorY=40 |
| `startVY` | StartLine 在 sb.rows 中的下标 | StartLine=L57 在 sb → startVY=9 |
| **屏幕行** | viewport 内的行（0..bufHeight-1） | `cursorY - startVY = 40-9 = 31` |
| `segRow` | 该屏行是此 buffer 行的第几个 softwrap 段 | 装饰行=-1；新 buffer 行=0；续行++ |

---

## 10. 文件清单

### 10.1 方案 B 核心代码（`internal/display/`）

| 文件 | 角色 | 关键函数 |
|------|------|----------|
| `bufwindow_md.go` | **方案 B 全部逻辑** | `displayBufferMD` / `displayToBuffer` / `showBuffer` / `relocateVerticalMD` / `coversExtent` / `renderSegmentMD` / `renderSegmentNative` / `LineToScreenRow` / `ScreenRowToLine` |
| `bufwindow.go` | 原生 + 分发缝 | `Display()`（932 行 IsMD 分流）/ `Relocate`（MD 分发） |

### 10.2 不得改动的文件（零侵入红线）

- `internal/md/render_*.go` — MD 渲染器（表格/代码块/标题等），方案 B 只调用不改
- `cmd/micro/micro.go` — 入口，`git diff` 必须为 0
- `internal/action/actions.go` — 84 处 `h.Relocate()` 调用保持原样
- `internal/display/softwrap.go` — `Scroll`/`SLocFromLoc`/`Diff` 全部复用
- `internal/buffer/buffer.go` — 除既定 MD 钩子外不动

### 10.3 相关配置/数据

- `runtime/syntax/markdown.yaml` — MD 语法高亮（需完整版含 codeblock region）
- `internal/buffer/buffer.go:86,93` — `IsMD` / `MDSegments` 钩子
- `internal/action/bufpane_md.go` — editMode 切换观察
- `internal/config/settings_md.go` — MD 专用设置

---

## 11. 已知限制

| 限制 | 原因 | 影响 | 建议 |
|------|------|------|------|
| **多窗格同缓冲区陈旧状态** | sb 是 BufWindow 级字段，分屏时两个窗格共享 buffer 但各有 sb，A 窗格编辑后 B 窗格 sb 不自动失效 | 分屏看同一文件时 B 可能显示旧内容 | 用户从不分屏，接受为已知限制。彻底解决需 sb 失效订阅 buffer 变更事件 |
| **单 segment > 2×bufHeight 触发原生 fallback** | displayToBuffer 的 cap=2×bufHeight 上限；超大表格/代码块会 overflow → `displayBuffer()` 原生兜底（显示 `#`/`|`） | 极端长表格 | 实际罕见；需提升 cap 或分段渲染 |
| **`internal/display` 测试缺失** | v1.0.5 的测试引用旧 API，方案 B 重构后失效已删除 | `go test ./internal/display/` 编译通过但无断言 | 待补 `coversExtent` / `rowIndexOf` / `relocateVerticalMD` 的单元测试 |

---

## 12. 下一步建议

1. **补测试**：`coversExtent` 是 bug 高发区，应有完整边界用例（起点在 sb 前/后/装饰行、尾部在/超出、buffer 末尾例外）。`rowIndexOf` 的 `(line, segRow)` 匹配也值得覆盖。
2. **关掉 debug**：验收通过后 `microNeoDebug = false`，保留埋点代码备用。
3. **多窗格失效**（若将来支持分屏）：给 sb 加 buffer 变更订阅，编辑后标记所有相关 sb 失效。
4. **大表格分段渲染**：若 overflow fallback 影响体验，考虑把超大 segment 按屏高分段，或动态提升 cap。

---

## 附录 A：方案 B 与 v1.0.5 对比

| 维度 | v1.0.5（viewportRowmap） | 方案 B（screenBuffer） |
|------|--------------------------|------------------------|
| 数据源 | 真屏 + 影子 rowmap（双写） | screenBuffer 单一数据源 |
| 渲染次数 | 每帧两次（屏 + rowmap） | 每帧一次（写 sb，blit 是纯 copy） |
| 跨段光标漂移 | 有（双写时序不一致） | 无（同源写入） |
| 装饰行处理 | rowmap 存 -1 | sb.rows.line 存 -1（同思想） |
| 失效检查 | 每帧重算 rowmap | coversExtent 复用 sb（idle 零渲染） |
| 坐标系一致性 | 易漂移 | sb 内绝对 + blit 减偏移，明确 |

方案 B 不是推翻 v1.0.5 的设计思想（`(line, segRow)` 二元组、装饰行 -1、buffer 行级 StartLine
全部继承），而是把"双写对齐"换成"单源 blit"，从架构上消除了不一致的土壤。
