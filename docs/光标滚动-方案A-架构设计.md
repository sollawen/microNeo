# 光标滚动 — 方案A (preRender) 架构设计

> 关联文档：
> - 问题与思路：`docs/光标滚动-方案A.md`、`docs/光标滚动-方案B.md`
> - 历史方案总结：`docs/光标滚动-修改总结.md`
>
> 本文档是**方案A**（preRender）的架构设计与实施蓝图，目标：**彻底修复"跨段切换时光标飞出 viewport 底部"的 bug**。

---

## 0. TL;DR

在 `Relocate` 的 MD 分支判定前，**当光标所属 segment 发生变化时**，做一次 dry-run 渲染，用新 map 替代上一帧的陈旧 `viewportRowmap`，再调用既有的 `relocateVerticalMD`。改动集中在 `internal/display/bufwindow_md.go`，对 micro 原生代码零侵入，对现有 renderer 零侵入。

---

## 1. 目标与非目标

### 目标
- 修复 `修改总结.md §5 已知限制` 中的**第一条**：跨段切换时光标偶发"消失一帧"（典型现象：光标在 table 末行按 ↓，table 从 5 row 切回 9 row 的 MD 渲染，光标飞出 viewport 底部）。
- 修复对称场景：光标 ↑ **进入** table，table 从 9 row 切回 5 row，map 缩水导致的多余滚动（`修改总结 §1` 提到的"symmetric 方向偶发多余滚动"）。

### 非目标（本方案不解决）
- **跨段进入 MD 段首次按键走 fallback**（`修改总结 §5 第二条`）：这是 softwrap 段内行号口径不一致的衍生问题，preRender 解决不了。留待 prefix-sum 或方案B。
- 性能优化、重构 displayBufferMD 主循环。

---

## 2. 问题复述（一句话）

`viewportRowmap` 每帧由 `displayBufferMD` 基于"光标段归属"重建，但 `Relocate` 在 `MoveCursor` 之后、`Display()` 之前调用 —— 此时 map 仍是**上一帧**的（基于上一帧的段归属）。一旦光标跨越 segment 边界，本帧 map 与本帧段归属不符，导致 scroll 量算错。

```
按键时序（actions.go:CursorDown）:
  MoveCursorDown(1)   // 光标 buffer 行变化
  Relocate()          // ← 这里读 viewportRowmap，但 map 还是上一帧的
  ...
  Display()           // ← 这里才基于新段归属重建 viewportRowmap
```

---

## 3. 解决思路

**核心思想**：把"基于新段归属的 map"在 `Relocate` 时刻就算出来一次，仅供本次 scroll 判定使用。

为什么不直接调用 `Display()`？因为：
1. `Display()` 会真的写屏（`screen.SetContent`），而 `Relocate` 之后还会再 `Display()` 一次，造成重复渲染和闪烁。
2. `Display()` 依赖 `w.StartLine`，而 `Relocate` 正是要算新的 `w.StartLine` —— 循环依赖。

因此引入 **dry-run 渲染**：复用 `renderSegmentMD` / `renderSegmentNative` 的 map 构建逻辑，但跳过所有 `screen.SetContent`，把产物写入临时 map。

---

## 4. 核心数据结构

### 4.1 新增字段（`BufWindow`）

```go
// internal/display/bufwindow.go

type BufWindow struct {
    // ... 原有字段不变 ...

    // MicroNeo: preRender 专用临时 map
    // 仅在 cursorSegmentChanged() 为 true 时由 preRenderRowmap() 填充，
    // 供本次 Relocate 的 relocateVerticalMD() 使用，用完即弃。
    // 字段复用 []SLoc 类型，与 viewportRowmap 同构。
    preRowmap []SLoc
    preRowmapValid bool // true = preRowmap 已填充且有效
}
```

**设计决策**：用独立字段 `preRowmap`，不复用 `viewportRowmap`。
- 理由：`viewportRowmap` 的语义是"本帧实际渲染出来的 map"，被 `LocFromVisual` / `ScreenRowToLine` 等多处读取。若被 preRender 覆盖，本帧 `Display()` 重建前的窗口期内所有读取都会读到 dry-run 数据，语义混乱。
- 成本：一次额外分配（每帧 preRender 仅在触发时执行）。

### 4.2 为什么不新增类型

`preRowmap` 与 `viewportRowmap` 同为 `[]SLoc`，复用既有的 `Line/Row` 语义（`Line=-1` 装饰行，`Line=-2` 空白，`Line>=0` 内容行 + `Row` 段内序号）。下游 `relocateVerticalMD` / `LineToScreenRow` 无需改动。

---

## 5. 触发条件设计

### 5.1 严格条件：光标所属 segment 变化

`光标滚动-方案A.md` 原文写的是"cursor 所在 Line +1"，**不够准确**。正确判定应是：

> **本帧光标所属的 segment ≠ 上一帧光标所属的 segment**

为什么是这个条件：
- **段内移动**（如 table 内 line57→line58）：段归属未变，map 仍精确反映屏行布局 → 不需 preRender。
- **跨段进出**（line59→line60 脱离 table，或 line54→line55 进入 table）：段归属变了，map 立刻失效 → 必须 preRender。
- 方向无关：↑ ↓ 都可能跨段，都要覆盖。

### 5.2 实现方式

```go
// internal/display/bufwindow_md.go

// segmentOfLine 返回包含 line 的 segment 的索引；找不到返回 -1。
// b.MDSegments 已是 content-static（buffer.go:212 全量 detect），可安全缓存查询。
func segmentOfLine(segs []md.Segment, line int) int {
    for i, s := range segs {
        if line >= s.BufStartLine && line <= s.BufEndLine {
            return i
        }
    }
    return -1
}
```

`Relocate` 需要同时知道"上一帧光标所属段"和"本帧光标所属段"。前者需要在每次 `Relocate` 结束时缓存：

```go
// BufWindow 新增字段
lastCursorSegIdx int   // 上一次 Relocate 时刻光标所属的 segment index
lastCursorLine   int   // 上一次 Relocate 时刻光标所在的 buffer line
```

**初始化**：`NewBufWindow` 时 `lastCursorSegIdx = -1`，保证首次按键必触发 preRender（首帧 map 也不可信）。

### 5.3 触发判定的精确逻辑

```go
func (w *BufWindow) cursorSegmentChanged(c SLoc) bool {
    if !w.Buf.IsMD || len(w.Buf.MDSegments) == 0 {
        return false
    }
    if c.Line == w.lastCursorLine && w.lastCursorSegIdx >= 0 {
        return false // 同行，段必然未变
    }
    curSeg := segmentOfLine(w.Buf.MDSegments, c.Line)
    return curSeg != w.lastCursorSegIdx
}
```

**注意**：`c` 是 `SLocFromLoc(activeC.Loc)`，对 MD 非 softwrap 场景 `c.Row` 恒 0，仅 `c.Line` 有意义。

---

## 6. dry-run 路径设计

### 6.1 目标

把 `renderSegmentMD` / `renderSegmentNative` 中"写 viewportRowmap"的逻辑与"写屏"逻辑解耦，新增一个 `dryRun bool` 参数。`dryRun=true` 时：
- ✅ 写入 `preRowmap`（或 `viewportRowmap`，由调用方传入目标）
- ✅ 计算 `newVY`（行布局必须算对）
- ❌ 不调用 `screen.SetContent`
- ❌ 不调用 `drawGutterAndLineNumMD` / `drawGutter` / `drawLineNum`
- ❌ 不调用 `w.showCursor`

### 6.2 改造方式：参数化目标 map + dryRun

为最小化对现有 `displayBufferMD` 路径的干扰，采用**两个改动**：

#### 改动 1：render 函数签名增加 `target []SLoc` 和 `dryRun bool`

```go
func (w *BufWindow) renderSegmentMD(
    seg md.Segment, vY int,
    target []SLoc, dryRun bool,
) (newVY int)

func (w *BufWindow) renderSegmentNative(
    seg md.Segment, startVY int,
    target []SLoc, dryRun bool,
) (newVY int)
```

- `displayBufferMD`（非 dry-run 路径）调用时传 `(w.viewportRowmap, false)`
- preRender 路径调用时传 `(w.preRowmap, true)`

#### 改动 2：写屏语句加 dryRun 守卫

`renderSegmentMD` 中：
```go
if !dryRun {
    screen.SetContent(screenX, screenY, cell.Rune, cell.Combining, style)
}
```

`renderSegmentMD` 中 `drawGutterAndLineNumMD` 调用、`renderSegmentNative` 中所有 `draw*` / `screen.SetContent` / `showCursor` 调用同理加守卫。

**写 map 语句不受 dryRun 影响**（dry-run 也要写 map，这正是它的目的）。

### 6.3 备选方案对比：抽纯函数 vs 加参数

| 方案 | 优点 | 缺点 |
|---|---|---|
| **A. 加 `target + dryRun` 参数**（推荐） | 改动集中、复用最大化、回归面小 | render 函数签名变长 |
| B. 抽出纯函数 `computeSegmentRows(seg, vY, cursors, editMode)` | dry-run 路径完全独立 | 与现有 render 函数有大量重复逻辑（map 构建部分），未来易漂移 |

**推荐方案A**：dry-run 与真实渲染共用同一段 map 构建代码，从根本上避免两条路径漂移。

---

## 7. preRender 主流程

### 7.1 函数签名

```go
// internal/display/bufwindow_md.go

// preRenderRowmap 以当前 w.StartLine 为起点，用本帧的段归属（取决于 cursor 新位置）
// 做一次 dry-run 渲染，把屏行 → buffer 行的映射写入 w.preRowmap。
//
// 前置条件：caller 已确认 cursorSegmentChanged() == true。
// 副作用：填充 w.preRowmap，置 w.preRowmapValid = true。
// 不副作用：不写屏、不改 w.StartLine、不改 w.viewportRowmap。
func (w *BufWindow) preRenderRowmap(c SLoc) {
    // ... 见 7.2 ...
}
```

### 7.2 实现伪代码

```go
func (w *BufWindow) preRenderRowmap(c SLoc) {
    b := w.Buf
    bufHeight := w.bufHeight
    w.ensureMDConfigReady()

    // 用当前 StartLine 作为渲染起点 —— 这正是"上一帧 viewport 的起点"，
    // 与本帧 displayBufferMD 的起点假设一致（displayBufferMD 也会用 w.StartLine.Line）
    visibleStart := w.StartLine.Line
    visibleEnd := visibleStart + bufHeight
    if visibleEnd >= b.LinesNum() {
        visibleEnd = b.LinesNum() - 1
    }

    // 分配/重置 preRowmap
    if cap(w.preRowmap) < bufHeight {
        w.preRowmap = make([]SLoc, bufHeight)
    }
    w.preRowmap = w.preRowmap[:bufHeight]
    for i := range w.preRowmap {
        w.preRowmap[i] = SLoc{Line: -2}
    }

    // 复用 displayBufferMD 的主循环，但传 dryRun=true 和 target=preRowmap
    segments := filterSegmentsToVisible(b.MDSegments, visibleStart, visibleEnd)
    cursors := b.GetCursors()

    vY := 0
    for _, seg := range segments {
        if w.editMode && hasCursorInside(seg, cursors) {
            vY = w.renderSegmentNative(seg, vY, w.preRowmap, true)
        } else {
            vY = w.renderSegmentMD(seg, vY, w.preRowmap, true)
        }
        if vY >= bufHeight {
            break
        }
    }
    // 剩余空间保持 {Line:-2}，无需额外处理

    w.preRowmapValid = true
}
```

**关键点**：
1. **起点用 `w.StartLine`**（旧值）：因为本次 preRender 是为了"算出新 StartLine"，渲染时自然要以旧 StartLine 为基准画 viewport。
2. **`hasCursorInside` 使用新 cursor 位置**：这是 preRender 成立的核心。`cursors` 来自 `b.GetCursors()`，此时 cursor 已经是 `MoveCursorDown` 之后的新位置，所以 table 已经"脱离光标"，会正确切回 MD 渲染。
3. **`editMode` 不变**：preRender 不改变用户模式状态。

### 7.3 与 displayBufferMD 的关系

两者共享同一套渲染主循环结构（filterSegmentsToVisible + 段循环），区别仅在：
- `displayBufferMD`：target=`viewportRowmap`，dryRun=false，本帧结束 visible 内容已在屏上
- `preRenderRowmap`：target=`preRowmap`，dryRun=true，本帧不写屏，仅供 Relocate 用

**可以考虑**进一步抽取 `renderViewportLoop(target, dryRun) int` 共用函数，两处都调用它。但首版为降低回归风险，**建议先复制粘贴主循环到 preRenderRowmap**，验证通过后再做抽取重构。

---

## 8. Relocate 接入

### 8.1 修改点：`bufwindow.go:Relocate()`

```go
// 修改前（现状）
if w.Buf.IsMD {
    ret = w.relocateVerticalMD(c, scrollmargin, height)
} else {
    // ... 原生 ...
}

// 修改后
if w.Buf.IsMD {
    // 方案A：跨段时先 preRender 刷新 map
    if w.cursorSegmentChanged(c) {
        w.preRenderRowmap(c)
    }
    ret = w.relocateVerticalMD(c, scrollmargin, height)

    // 缓存本次结果供下次比对
    w.lastCursorLine = c.Line
    w.lastCursorSegIdx = segmentOfLine(w.Buf.MDSegments, c.Line)
} else {
    // ... 原生分支不变 ...
}
```

### 8.2 修改点：`relocateVerticalMD` 读取 preRowmap

```go
// internal/display/bufwindow_md.go

func (w *BufWindow) relocateVerticalMD(c SLoc, scrollmargin, height int) bool {
    // 方案A：优先用 preRowmap（基于新段归属），否则回退 viewportRowmap（上一帧）
    var rowmap []SLoc
    if w.preRowmapValid {
        rowmap = w.preRowmap
    } else {
        rowmap = w.viewportRowmap
    }

    n := len(rowmap)
    if n == 0 {
        return w.relocateVerticalNativeFallback(c, scrollmargin, height)
    }
    cursorRow, ok := lineToScreenRowIn(rowmap, c.Line, c.Row)
    if !ok {
        return w.relocateVerticalNativeFallback(c, scrollmargin, height)
    }
    // ... 后续 botMarginRow / delta 逻辑完全不变，只是把 w.viewportRowmap 替换为 rowmap ...
    botMarginRow := height - 1 - scrollmargin
    if cursorRow > botMarginRow {
        delta := cursorRow - botMarginRow
        if delta >= n {
            return w.relocateVerticalNativeFallback(c, scrollmargin, height)
        }
        loc := rowmap[delta]
        for loc.Line < 0 && delta+1 < n {
            delta++
            loc = rowmap[delta]
        }
        if loc.Line < 0 {
            return w.relocateVerticalNativeFallback(c, scrollmargin, height)
        }
        w.StartLine = loc
        return true
    }
    // ... 上分支同理不变 ...
}
```

需要把 `LineToScreenRow` 拆出一个内部版本 `lineToScreenRowIn(map, line, row)`，原 `LineToScreenRow` 变成它的 wrapper（默认读 viewportRowmap）。保持对外 API 不变。

### 8.3 preRowmapValid 的生命周期

- 每帧 `Display()` 开始时**不需要**清零 `preRowmapValid`（它只由下一次 Relocate 时通过 `cursorSegmentChanged` 重新触发覆盖）。
- 但为防止"某次 preRender 成功、之后若干帧段没变、再后来段变了但 preRender 失败"的边界，**最稳妥的做法**是在 `relocateVerticalMD` 末尾置 `w.preRowmapValid = false`（用完即弃，语义最清晰）。

---

## 9. 时序对照

### 9.1 现状（buggy）

```
帧 N:    MoveCursorDown(59→60)
         Relocate:
           viewportRowmap = [上一帧 N-1 的 map，table 5 row]
           c.Line = 60, LineToScreenRow 找不到 → fallback (1:1)
           算出 StartLine 滚 1 row
         Display:
           displayBufferMD 重建 map，table 切回 9 row
           viewport 底部被 table 装饰行撑爆
           光标 line 60 跑到 viewport 之外 ❌
```

### 9.2 方案A（fixed）

```
帧 N:    MoveCursorDown(59→60)
         Relocate:
           cursorSegmentChanged()=true（table 段 → 下一段）
           preRenderRowmap():
             dry-run 渲染，table 已脱离光标 → 切 MD 渲染 → map 含 9 row
             preRowmap = [新 map，table 9 row]
           relocateVerticalMD 读 preRowmap:
             c.Line = 60, cursorRow 正确（基于 9 row map）
             delta 算对 → StartLine 多滚 4 row
         Display:
           displayBufferMD 基于新 StartLine 渲染，table 9 row 完整显示
           光标 line 60 在 viewport 底部 scrollmargin 内 ✅
```

---

## 10. 边界场景

| 场景 | 处理 |
|---|---|
| **首帧 / MDSegments 为 nil** | `cursorSegmentChanged` 返回 false（或 preRender 内 segments 为空 → preRowmap 全 -2）→ `relocateVerticalMD` 走 fallback，与现状一致 |
| **光标跳出 viewport（大跳转）** | preRender 仍会执行（cursorSegmentChanged=true），但 `LineToScreenRow` 找不到 → 走 fallback。fallback 是 1:1 原生逻辑，大跳转场景本来就该用原生 |
| **cursor 在装饰行（BufLine=-1）** | `segmentOfLine` 用 cursor 的 buffer 行号（c.Line），装饰行不参与判定，无影响 |
| **多个 cursor（多光标编辑）** | `cursorSegmentChanged` 只看 active cursor（`Relocate` 本来就只用 activeC），与现状一致 |
| **用户连续按 ↓ 跨多个段** | 每次 Relocate 独立判定，跨一次段触发一次 preRender。连续跨段时每帧都 preRender，开销线性增长但仍可控（见 §11） |
| **编辑动作（输入字符）导致段边界变化** | `b.MDSegments` 由 `buffer.go:1032` 在 re-highlight 时同步更新，`cursorSegmentChanged` 自然反映新边界 |
| **resize（bufHeight 变化）** | preRender 内部按新 bufHeight 重新分配 preRowmap，安全 |
| **softwrap 开启** | 方案A 不依赖 softwrap 开关。`renderSegmentNative` 已有 softwrap 处理（vloc.Y -= StartLine.Row），dry-run 路径同样适用 |

---

## 11. 性能评估

### 11.1 触发频率
- 仅在"光标跨段"时触发，单次按键最多 1 次 preRender
- 段内编辑/移动（绝大多数按键）零开销

### 11.2 单次开销
- dry-run 渲染跳过所有 `screen.SetContent`，主要成本是 `seg.Render()`（CPU）+ map 写入（内存）
- 最大段（table / codeblock）的 Render 是 O(rows × cols × width)，典型表格 < 1ms
- `displayBufferMD` 本来每帧就要调一次同样的 Render，preRender 只是"提前调一次"，相对增量约等于一次额外 `displayBufferMD`

### 11.3 与方案B 对比
方案B 每次操作都 preRender，但 `displayBufferMD` 不再 render（纯 blit）。净开销：
- 方案A：跨段时 +1 次渲染，非跨段时 0
- 方案B：每次操作 +1 次渲染，−1 次渲染（displayBufferMD 不再 render）≈ 持平

对于编辑/浏览为主的场景，跨段是低频事件，**方案A 在性能上优于方案B**。

---

## 12. 风险与回归

### 12.1 主要风险

| 风险 | 概率 | 缓解 |
|---|---|---|
| render 函数加参数后，dry-run 与真实路径行为不一致 | 中 | 单测覆盖：`TestRenderSegmentMD_DryRunMatchesWetRun`，断言同一 seg 在 dryRun=true/false 下产出的 map 完全相同 |
| `cursorSegmentChanged` 判定漏掉某种跨段场景 | 低 | `lastCursorSegIdx = -1` 初始值保证首次必触发；保守策略是"宁可多触发也不漏触发" |
| preRender 内部 panic（如 nil deref） | 低 | preRender 失败时 recover 并 fallback 到 viewportRowmap（参见 §12.2）|
| 性能回归（极端大文件 + 高频跨段）| 低 | preRender 内部加 `vY >= bufHeight` 提前 break；必要时可加节流（连续跨段时复用上一次 preRender）|

### 12.2 panic 防护

```go
func (w *BufWindow) preRenderRowmap(c SLoc) {
    defer func() {
        if r := recover(); r != nil {
            w.preRowmapValid = false // 失败则不启用，relocateVerticalMD 自动回退 viewportRowmap
        }
    }()
    // ... 主逻辑 ...
}
```

> 注：是否加 recover 取决于项目对 panic 策略的偏好。若项目整体不允许 recover（micro 原生无此习惯），可省略，改为靠单测保证不 panic。

### 12.3 回归测试矩阵

| 用例 | 期望行为 |
|---|---|
| 光标在 table 末行按 ↓ | 光标不飞出 viewport（核心 bug 修复）|
| 光标在 table 上方按 ↓ 进入 table | 光标进入 table 时不发生多余滚动（对称场景）|
| 光标在段内连续 ↓↑ | 行为与现状完全一致（preRender 不触发）|
| 大跳转（GotoLine 到文件尾）| 行为与现状一致（走 fallback）|
| resize 后首帧 | 行为与现状一致（首帧 fallback）|
| 切换 editMode（ESC）| 行为与现状一致（preRender 不改变 editMode）|
| `sample.md` 全文浏览 | 无可感知的滚动瑕疵 |

---

## 13. 实施步骤（建议分 commit）

### Commit 1: 抽象 render 函数的 dry-run 能力
- `renderSegmentMD` / `renderSegmentNative` 增加 `target []SLoc, dryRun bool` 参数
- 现有调用点（`displayBufferMD`）改为传 `(w.viewportRowmap, false)`
- 加单测：dryRun=true/false 产出的 map 一致
- **不改任何行为**，纯重构，可独立验证

### Commit 2: 新增 preRender 基础设施
- `BufWindow` 增加 `preRowmap` / `preRowmapValid` / `lastCursorSegIdx` / `lastCursorLine` 字段
- 实现 `segmentOfLine` / `cursorSegmentChanged` / `preRenderRowmap`
- 拆分 `lineToScreenRowIn`
- **暂不接入 Relocate**，可单测 preRender 产出

### Commit 3: 接入 Relocate
- 修改 `Relocate` 的 MD 分支：跨段时调 `preRenderRowmap`
- 修改 `relocateVerticalMD`：优先读 `preRowmap`
- 手测核心场景（sample.md table 上下穿越）

### Commit 4: 边界打磨
- panic 防护（如需）
- 连续跨段性能优化（如测出问题）
- 文档更新：把本方案合并到 `光标滚动-修改总结.md §5` 第一条标记为"已解决"

---

## 14. 代码改动清单（预估）

| 文件 | 改动类型 | 规模 |
|---|---|---|
| `internal/display/bufwindow.go` | 新增字段；`Relocate` 加 preRender 触发 | 小 |
| `internal/display/bufwindow_md.go` | render 函数加参数；新增 preRenderRowmap / cursorSegmentChanged / segmentOfLine / lineToScreenRowIn；修改 relocateVerticalMD | 中 |
| `internal/display/bufwindow_md_test.go` | 新增 dry-run 一致性测试、preRender 测试 | 中（先修复现有编译失败）|
| micro 原生文件 | **零改动** | 0 |
| `internal/md/*.go` | **零改动**（renderer 不动）| 0 |

---

## 15. 与方案B 的取舍说明

如果后续证明方案A 无法覆盖某个场景（例如 `修改总结 §5 第二条` 的 segmentRow 口径问题再次被用户感知），可平滑切换到方案B：
- 方案A 的 `preRenderRowmap` 已经把"dry-run 渲染"这条路打通，方案B 的 `bigViewportMap` 本质就是"每次都 preRender 并复用结果"
- 方案A 的 render 函数 dry-run 参数是方案B 的前置基础设施
- 因此**方案A 不是方案B 的竞争方案，而是方案B 的第一阶段**

建议先落地方案A（1-2 天），观察实际效果，再决定是否继续推进方案B。
