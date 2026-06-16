# 方案 B 实施计划（screenBuffer 抽象）

> 评估依据：`docs/方案B-评估报告mm.md`（代码量 ~153 行，主循环零改动，残留风险 = screenBuffer 容量 + blit 边界）
> 关联：`docs/光标滚动-修改总结.md`（v1.0.5 现状）、`docs/光标滚动-方案B.md`（方案原始设计）、`docs/方案A架构设计-很臭的方案.md`（备选方案 A）
> 立场：方案 A 与方案 B 都已经达到「代码量可接受」的临界，本计划为方案 B 的可执行实施文档
> 日期：2026-06-16

---

## 0. 一句话总结

**方案 B = 「把渲染目标从 tcell screen 搬到二维 screenBuffer 数组」**。Display 入口先 render 到 buffer、再 blit 到 screen；Relocate 入口先 render 拿 fresh rowmap、再做 scroll 决策。

**侵入范围**：仅 `internal/display/` 包（~153 行新增/改）；render 函数、actions.go、buffer 包、micro 主循环 **零改动**。

---

## 1. 现状回顾（从哪里出发）

### 1.1 v1.0.5 的两套渲染路径

`BufWindow.Display()` 入口（`bufwindow.go:932`）按 `w.Buf.IsMD` 分流：

| 路径 | 函数 | 关键产物 | 已知限制 |
|---|---|---|---|
| 原生（非 MD）| `w.displayBuffer()` | `screen.SetContent` 直接写屏 | 1:1 假设，micro 原生处理 |
| MD | `w.displayBufferMD(w.editMode)` | 写屏 + 写 `viewportRowmap` | 跨段切换时光标消失一帧 |

**MD 路径的主循环**（`bufwindow_md.go:736-743`）：

```go
for _, seg := range segments {
    if editMode && hasCursorInside(seg, cursors) {
        vY = w.renderSegmentNative(seg, vY)
    } else {
        vY = w.renderSegmentMD(seg, vY)
    }
}
```

### 1.2 现状的 stale map 风险

`viewportRowmap` 是**上一帧** `displayBufferMD` 的产物。`Relocate()` 决策时这张 map 反映"光标在旧位置时"的 viewport 长什么样。光标一跨段，map 失效。

**这是方案 B 要彻底解决的根因**。

### 1.3 Relocate 入口的现状

`bufwindow.go:249`：

```go
if w.Buf.IsMD {
    ret = w.relocateVerticalMD(c, scrollmargin, height)
} else {
    // 原生逻辑
}
```

`relocateVerticalMD` 直接读 `w.viewportRowmap` 做 scroll 决策。**不重新渲染** → map 失效风险。

---

## 2. 目标

| 目标 | 度量 |
|---|---|
| 修复跨段切换时光标偶发"消失一帧" | `docs/sample.md` line55-59 表格场景：连续按 ↓ 时光标始终可见 |
| Relocate 决策永远基于 fresh map | `viewportRowmap` 与当前 cursor 位置一致 |
| 主循环零改动 | `cmd/micro/micro.go` git diff = 0 |
| render 函数零改动 | `internal/md/render_*.go` git diff = 0 |
| 内存可控 | 4 BufWindow 同时打开 ≤ 500 KB cells 总开销（cells 按需释放） |

**非目标**：
- 不重写原生 `displayBuffer()`（保持原样）
- 不改动 `softwrap.go`（`Scroll` / `Diff` 全部复用）
- 不改动 `actions.go`（主循环外 84 处 `h.Relocate()` 调用全部不动）
- 不解决 micro 原生跨段渲染问题（这是 micro 上游的 bug）

---

## 3. 核心设计

### 3.1 screenBuffer 数据结构

**位置**：`internal/display/bufwindow_md.go`（与 render 函数同文件，便于维护）

```go
// screenCell 是渲染输出的最小单位。
// 与 tcell.SetContent 的入参对齐：r + combc + style。
type screenCell struct {
    r     rune
    combc []rune
    style tcell.Style
}

// screenBuffer 是单次 Display 调用的渲染缓存：
//   - cells:     渲染好的二维 [vY][vX] 屏幕字符（供 showBuffer blit）
//   - rowmap:    每个屏行对应的 buffer 位置（替代现有 viewportRowmap，供 Relocate 查询）
//   - startLine: 渲染时的起点 buffer 行号（用于 showBuffer 时 screenStartRow 计算）
//   - cap:       cells 已分配的物理行数（用于决定是否需要扩容，区分当前显示范围）
//   - bufWidth:  渲染时的列数（用于 cells[i] 长度校验）
//   - overflow:  2×bufHeight 兜底触发的标记（说明 Relocate 可能不够用）
//   - cursorX/Y: 光标在 cells 坐标系中的位置（showBuffer 时调 screen.ShowCursor）
type screenBuffer struct {
    cells     [][]screenCell
    rowmap    []SLoc
    startLine int
    cap       int
    bufWidth  int
    overflow  bool
    cursorX   int
    cursorY   int
}

// SetCell 是 SetContent 的等价物，写到 cells[vY][vX] 而不是直接写屏。
// vX = col 偏移（相对 BufWindow 左上角，含 gutterOffset 由调用方传入）
// vY = row 偏移（相对 BufWindow 顶部）
func (buf *screenBuffer) SetCell(vX, vY int, r rune, combc []rune, style tcell.Style) {
    if vY < 0 || vY >= len(buf.cells) {
        return // overflow 兜底区
    }
    if vX < 0 || vX >= buf.bufWidth {
        return // gutter 由调用方处理，这里只接 bufWidth 范围
    }
    buf.cells[vY][vX] = screenCell{r: r, combc: combc, style: style}
}

// ShowCursor 记录光标位置（cells 坐标系）。
// showBuffer 时会调 screen.ShowCursor 把光标真实位置同步到 terminal。
func (buf *screenBuffer) ShowCursor(vX, vY int) {
    buf.cursorX = vX
    buf.cursorY = vY
}
```

**为什么一个数据结构够用**：
- `cells` 是渲染输出（供显示 blit）
- `rowmap` 是几何信息（供 Relocate 查询 cursorRow）
- 两者在同一渲染过程中同步生成，自然一致
- 对比方案 A 的 `viewportRowmap`（仅几何），方案 B 多了 cells 字段 → 渲染产物双份

### 3.2 时序：Display / Relocate 入口的 lazy render

**核心承诺**：决策（Relocate）前永远有一份 fresh 的 screenBuffer，从根本上消灭「map 落后一帧」的时序断层。

**完整时序图**：

```
文件刚打开时：
    displayToBuffer(w.screenBuf, w.StartLine.Line, w.editMode)

主循环（cmd/micro/micro.go，不动）：
    for {
        Display()              ← 由 BufWindow.Display 调度
        event = ...
        HandleEvent()          ← 可能改 StartLine / cursor，可能调 Relocate
    }

BufWindow.Display (bufwindow.go:932, 微调)：
    if !w.Buf.IsMD {
        w.displayBuffer()      // 原生路径，不动
        return
    }
    // MD 路径
    if w.screenBuf == nil ||
       w.screenBuf.startLine != w.StartLine.Line ||
       w.screenBuf.overflow {
        w.displayToBuffer(w.screenBuf, w.StartLine.Line, w.editMode)
    }
    w.showBuffer(w.screenBuf, w.StartLine.Line)

BufWindow.Relocate (bufwindow.go:249, IsMD 分支)：
    if w.Buf.IsMD {
        // ★ 关键：Relocate 决策前先渲染一份 fresh rowmap
        if w.screenBuf == nil ||
           w.screenBuf.startLine != w.StartLine.Line ||
           w.screenBuf.overflow {
            w.displayToBuffer(w.screenBuf, w.StartLine.Line, w.editMode)
        }
        ret = w.relocateVerticalMD(c, scrollmargin, height)
    } else {
        // 原生 Relocate，不动
    }
```

### 3.3 三种事件路径的统一处理

**结论**：三种 case 在 Display 入口 + Relocate 入口的 lazy render 收敛下，**不需要 case 分发**。

| 场景 | 触发点 | displayToBuffer 用什么 startLine | showBuffer 用什么 startLine |
|---|---|---|---|
| **case A**：编辑/光标移动（跨段） | MoveCursor → Relocate | 当前 `w.StartLine.Line` | Relocate 精算后的 `realStartLine`（在 cells 范围内 → 复用）|
| **case B**：纯视口（scroll/page/center/start/end） | HandleEvent 改 w.StartLine → Display | 新的 `w.StartLine.Line` | 同一 `w.StartLine.Line`（cells 直接复用） |
| **case C**：goto/search 大幅移动 | Relocate 估算 → 精算 | **估算的 startLine1**（1:1 估算）| Relocate 精算后的 `realStartLine`（在 cells 范围内 → 复用）|

**关键洞察**：

1. **cells 复用窗口 = 单次 Display 调用内**：
   - displayToBuffer(估算 startLine) → cells 覆盖 [startLine, startLine + 2×bufHeight]
   - Relocate 精算 → realStartLine 在 cells 范围内
   - showBuffer(realStartLine) → 从 cells 对应位置 blit（**复用估算渲染的 cells**）

2. **跨 Display 调用 = 永远重新渲染**：
   - 下次 Display 时 cells 已被 showBuffer 清空（cells = nil）
   - 必须重新 displayToBuffer
   - rowmap 保留供下次 Relocate 决策

3. **cells 是临时资源，用完就扔**：
   - showBuffer 完成后 cells = nil（避免长期占内存）
   - 不存在「cells 是否可复用」的判断
   - 不存在「cells 释放时机」的纠结

### 3.4 displayToBuffer 的停止条件

```go
func (w *BufWindow) displayToBuffer(buf *screenBuffer, startLine int, editMode bool) {
    buf.startLine = startLine
    buf.overflow = false
    buf.cells = buf.cells[:0]
    buf.rowmap = buf.rowmap[:0]
    // cells 按需重新分配（cells=nil 时分配，容量不够时扩容）
    if cap(buf.cells) < 2*w.bufHeight {
        buf.cells = make([][]screenCell, 0, 2*w.bufHeight)
        buf.cap = 2 * w.bufHeight
    }
    for i := range buf.cells {
        buf.cells[i] = buf.cells[i][:0]
    }

    line := startLine
    vY := 0
    cursorDone := false
    oldSeg := w.findSegmentContaining(w.prevCursorY)
    oldSegDone := (oldSeg == nil)
    scrollmargin := int(w.Buf.Settings["scrollmargin"].(float64))

    for {
        // 兜底：2×bufHeight 强制退出，防止内存/时间爆炸
        if vY >= 2*w.bufHeight {
            buf.overflow = true
            break
        }

        seg := w.findSegmentContaining(line)
        if seg == nil {
            break // buffer 末尾
        }

        if editMode && hasCursorInside(seg, w.Buf.GetCursors()) {
            vY = w.renderSegmentNative(seg, vY, buf) // buf 新增参数
            cursorDone = true
        } else {
            vY = w.renderSegmentMD(seg, vY, buf) // buf 新增参数
        }
        line = seg.BufEndLine + 1

        // 三目的停止（与方案 A preRender 相同）：
        canBreak := true
        if !cursorDone {
            canBreak = false
        }
        if !oldSegDone && seg != oldSeg {
            canBreak = false
        }
        if vY < w.bufHeight {
            canBreak = false // viewport 还没填满
        }

        if canBreak {
            break
        }
    }

    // 填充剩余空白（buf 内容不够填满 viewport）
    defStyle := config.DefStyle
    for ; vY < w.bufHeight; vY++ {
        // ... 调用 drawGutterAndLineNumMD + 空白填充（参数加 buf）
    }
}
```

**与方案 A preRender 的关系**：停止条件逻辑几乎一致（都是「目的驱动」）。差异：
- 方案 A preRender：dry-run，**只写 rowmap**，cells 用临时占位
- 方案 B displayToBuffer：正常渲染，**同时写 rowmap 和 cells**

### 3.5 showBuffer 的 blit 逻辑

```go
func (w *BufWindow) showBuffer(buf *screenBuffer, startLine int) {
    if buf.overflow {
        // overflow 兜底：走原生 displayBuffer 渲染
        w.displayBuffer()
        return
    }

    screenStartRow := startLine - buf.startLine

    // 边界检查：startLine 必须在 [buf.startLine, buf.startLine + 2*bufHeight) 范围内
    if screenStartRow < 0 || screenStartRow >= len(buf.cells) {
        // startLine 超出 cells 覆盖范围 → 重新渲染
        w.displayToBuffer(buf, startLine, w.editMode)
        screenStartRow = 0
    }

    // blit cells[screenStartRow..screenStartRow+bufHeight] 到 viewport
    for vY := 0; vY < w.bufHeight && screenStartRow+vY < len(buf.cells); vY++ {
        row := buf.cells[screenStartRow+vY]
        for vX, cell := range row {
            screen.SetContent(w.X+w.gutterOffset+vX, w.Y+vY, cell.r, cell.combc, cell.style)
        }
    }

    // 还原 gutter 和行号（gutter 不进 cells 缓存）
    // ...

    // 同步光标位置
    screen.ShowCursor(w.X+buf.cursorX, w.Y+buf.cursorY)

    // cells 用完即弃，rowmap 保留
    buf.cells = nil
}
```

### 3.6 5 处 SetContent 替换

**精确盘点**（与评估报告一致）：

| 位置 | 函数 | 改动 |
|---|---|---|
| `bufwindow_md.go:195`（renderSegmentMD 主循环） | `screen.SetContent(screenX, screenY, ...)` | 改为 `buf.SetCell(col, vY, ...)` |
| `bufwindow_md.go:520`（renderSegmentNative 主循环） | `screen.SetContent(w.X+vloc.X, ...)` | 改为 `buf.SetCell(vloc.X-w.gutterOffset, vloc.Y, ...)` |
| `bufwindow_md.go:667`（renderSegmentNative 行尾填充） | `screen.SetContent(i+w.X, vloc.Y+w.Y, ...)` | 改为 `buf.SetCell(...)` |
| `bufwindow_md.go:751`（displayBufferMD 末尾填充） | `screen.SetContent(w.X+w.gutterOffset+col, ...)` | 改为 `buf.SetCell(col, vY, ...)` |
| `bufwindow_md.go:813`（drawGutterAndLineNumMD） | `screen.SetContent(w.X+vloc.X, ...)` | 改为 `buf.SetCell(vloc.X, vY, ...)`（gutter 也进 cells） |

**render 函数主体逻辑零改动**：颜色、tab、wrap、selection、cursorline、matchbrace 等 100% 不动。

---

## 4. 改动清单（精确到行）

### 4.1 文件级总览

| 文件 | 改动类型 | 改动量 | 风险等级 |
|---|---|---|---|
| `internal/display/bufwindow_md.go` | 核心改动（screenBuffer + displayToBuffer + showBuffer + 5 处 SetContent） | ~140 行新增/改 | 中 |
| `internal/display/bufwindow.go` | 微调（字段 + 1 行更新） | ~10 行 | 低 |
| `internal/md/render_*.go` | **零改动** | 0 | — |
| `internal/md/md.go` / `detect.go` | **零改动** | 0 | — |
| `internal/action/actions.go` | **零改动** | 0 | — |
| `internal/action/bufpane.go` / `bufpane_md.go` | **零改动** | 0 | — |
| `internal/buffer/buffer.go` | **零改动** | 0 | — |
| `internal/display/softwrap.go` | **零改动** | 0 | — |
| `cmd/micro/micro.go` 主循环 | **零改动** | 0 | — |

**总侵入**：~150 行，全在 display 包内。

### 4.2 详细代码量盘点

| 组件 | 行数 | 说明 |
|---|---|---|
| `screenBuffer` + `screenCell` 数据结构 | ~25 | types + SetCell/ShowCursor 方法 |
| `newScreenBuffer` / 容量管理 | ~10 | 懒分配 + 2×bufHeight capacity |
| `displayToBuffer(startLine, editMode)` | ~50 | 三目的停止 + while 循环 + startLine 参数化 |
| `showBuffer(startLine)` | ~20 | blit + cells=nil + overflow 处理 |
| `Display` 入口微调（bufwindow.go） | ~5 | displayToBuffer + showBuffer 调度 |
| `Relocate` 入口微调（bufwindow.go） | ~3 | 入口加 displayToBuffer 调用 |
| `findSegmentContaining` | ~10 | 线性扫 segments，返回指针 |
| `renderSegmentMD` 改造 | ~5 | 1 处 SetContent → `buf.SetCell` |
| `renderSegmentNative` 改造 | ~5 | 2 处 SetContent → `buf.SetCell` |
| `drawGutterAndLineNumMD` 改造 | ~3 | 1 处 SetContent → `buf.SetCell` |
| 字段调整（`bufwindow.go`） | ~5 | `viewportRowmap` → `screenBuf` + `prevCursorY` |
| `displayBufferMD` 末尾填充改造 | ~5 | 1 处 SetContent → `buf.SetCell` |
| 测试代码 | ~100 | 单元测试 + 跨段场景 |
| **总计（不含测试）** | **~146 行** | |

### 4.3 字段调整（bufwindow.go）

**现状**（`bufwindow.go:40-42`）：

```go
viewportRowmap []SLoc
editMode bool
```

**改为**：

```go
screenBuf *screenBuffer // MD 渲染缓存（cells + rowmap 合一）
editMode   bool
```

`prevCursorY` 字段：

```go
prevCursorY int // 上一帧 Display 结束时的 activeCursor.Y（用于跨段检测）
```

`BufWindow` 需要新增方法：

```go
// updatePrevCursor 每帧 Display 末尾调用，记录光标位置
func (w *BufWindow) updatePrevCursor() {
    w.prevCursorY = w.Buf.GetActiveCursor().Y
}
```

`viewportRowmap` 字段不再直接持有，改为从 `w.screenBuf.rowmap` 访问。**所有现有引用 `w.viewportRowmap` 的地方改为 `w.screenBuf.rowmap`**：

| 引用位置 | 现状 | 改为 |
|---|---|---|
| `bufwindow_md.go:719`（懒分配） | `w.viewportRowmap = make([]SLoc, bufHeight)` | `w.screenBuf.rowmap = make([]SLoc, bufHeight)` |
| `bufwindow_md.go:722`（重置 -2） | `for i := range w.viewportRowmap` | `for i := range w.screenBuf.rowmap` |
| `bufwindow_md.go:131`（renderSegmentMD 写入） | `w.viewportRowmap[vY] = ...` | `w.screenBuf.rowmap[vY] = ...` |
| `bufwindow_md.go:624`（renderSegmentNative 写入） | 同上 | 同上 |
| `bufwindow_md.go:874`（ScreenRowToLine 读取） | `len(w.viewportRowmap)` | `len(w.screenBuf.rowmap)` |
| `bufwindow_md.go:890`（LineToScreenRow 读取） | `w.viewportRowmap[i]` | `w.screenBuf.rowmap[i]` |
| `bufwindow_md.go:907`（relocateVerticalMD 读取） | `len(w.viewportRowmap)` | `len(w.screenBuf.rowmap)` |
| `bufwindow_md.go:916`（relocateVerticalMD 读取） | `w.viewportRowmap[delta]` | `w.screenBuf.rowmap[delta]` |

---

## 5. 实施步骤（按 commit）

### Commit 1：screenBuffer 数据结构 + SetCell 适配

**目标**：引入 screenBuffer 数据结构，所有渲染函数从写 `screen.SetContent` 改为写 `buf.SetCell`。

**改动**：

1. `internal/display/bufwindow_md.go` 顶部新增 `screenCell` / `screenBuffer` 类型定义（~25 行）
2. 新增 `SetCell` / `ShowCursor` 方法（~10 行）
3. `renderSegmentMD` 加 `buf *screenBuffer` 参数，1 处 `screen.SetContent` 改为 `buf.SetCell`（~5 行）
4. `renderSegmentNative` 加 `buf *screenBuffer` 参数，2 处 `screen.SetContent` 改为 `buf.SetCell`（~5 行）
5. `drawGutterAndLineNumMD` 加 `buf *screenBuffer` 参数，1 处 `screen.SetContent` 改为 `buf.SetCell`（~3 行）
6. `displayBufferMD` 末尾填充 1 处 `screen.SetContent` 改为 `buf.SetCell`（~5 行）
7. `BufWindow` 加 `screenBuf *screenBuffer` 字段（`bufwindow.go` 1 行）
8. `NewBufWindow` 初始化 `screenBuf`（~3 行）

**验证**：
- 编译通过（`make build-quick`）
- 运行 micro 打开 MD 文件，**渲染结果应与现状完全一致**（因为这个 commit 只是改了写屏路径，行为不变）
- 肉眼对比 sample.md

**风险**：参数传递漏改导致 panic。**对策**：编译时所有调用点必须全部加 `buf` 参数，否则报错。

---

### Commit 2：displayToBuffer + showBuffer 调度器

**目标**：实现 lazy render 入口，把"渲染"和"显示"分离。

**改动**：

1. 新增 `displayToBuffer(buf *screenBuffer, startLine int, editMode bool)` 函数（~50 行）
2. 新增 `findSegmentContaining(line int) *md.Segment` 函数（~10 行）
3. 新增 `showBuffer(buf *screenBuffer, startLine int)` 函数（~20 行）
4. `displayBufferMD` 重写为调度器（~30 行）：
   - 调 `displayToBuffer`
   - 调 `showBuffer`
   - 末尾调 `updatePrevCursor`
5. `BufWindow` 加 `prevCursorY int` 字段 + `updatePrevCursor` 方法（~5 行）

**验证**：
- 编译通过
- 打开 MD 文件，按 ↓/↑ 移动光标，渲染仍正常
- 跨段切换场景（v1.0.5 已知限制）：表格下方的光标移动仍能看到（暂时不要求完美，先验证机制跑通）

**风险**：
- cells 释放时机不对 → 闪烁 / 黑屏
- showBuffer 的 blit 边界处理错位 → 文本错位
- 2×bufHeight 兜底覆盖不到 → overflow 频繁 → 走 nativeFallback 路径

**对策**：
- Commit 2 完成后，跑一遍 sample.md 的人工验收
- 对比 v1.0.5 行为基线，列出「行为变化清单」

---

### Commit 3：Relocate 入口集成 fresh render

**目标**：解决 stale map 风险，跨段切换时光标不再消失一帧。

**改动**：

1. `Relocate` 的 IsMD 分支入口加 `displayToBuffer` 调用（~3 行，`bufwindow.go:249`）
2. `relocateVerticalMD` 读 `w.screenBuf.rowmap`（~3 行）

**验证**：
- 编译通过
- 重点场景：v1.0.5 已知限制（`docs/sample.md` line55-59 表格，光标按 ↓ 到底边）
- 期望：跨段时不再"消失一帧"

**风险**：
- Relocate 入口调 displayToBuffer 后 cells 不在预期范围 → Relocate 决策错位
- prevCursorY 没及时更新 → findSegmentContaining 拿到旧段 → 三目的停止条件算错

**对策**：
- Commit 3 完成后，**专门跑 v1.0.5 已知限制场景**，对比修复前后
- 如果发现新问题，记录到 `docs/光标滚动-修改总结.md` 后续章节

---

### Commit 4：测试补充 + overflow 路径完善

**目标**：覆盖核心数据结构和关键场景。

**改动**：

1. 新建 `internal/display/bufwindow_md_test.go`（旧测试文件已于 2026-06-16 删除，参考 `docs/光标滚动-修改总结.md` §5）
2. 测试覆盖：
   - `screenBuffer.SetCell` / `ShowCursor` 单元测试
   - `displayToBuffer` 三目的停止条件
   - `findSegmentContaining` 边界（line < 0、line 越界）
   - `showBuffer` overflow 路径
   - 跨段场景：cursor 移动后 Relocate 决策正确

**验证**：
- `go test ./internal/display/` 全绿
- 回归：原 MD 渲染的所有场景不破

---

## 6. 关键函数实现细节

### 6.1 findSegmentContaining

**位置**：`internal/display/bufwindow_md.go`

```go
// findSegmentContaining 返回包含 line 的 Segment 指针。
// 返回指针而非拷贝，确保 prevSeg == newSeg 比较稳定（方案 A §3.4 强调的坑）。
// line < 0 → 返回 nil（prevCursorY=-1 sentinel 时的边界保护）。
func (w *BufWindow) findSegmentContaining(line int) *md.Segment {
    if line < 0 {
        return nil
    }
    segs := w.Buf.MDSegments
    for i := range segs {
        if segs[i].BufStartLine <= line && line <= segs[i].BufEndLine {
            return &segs[i]
        }
    }
    return nil
}
```

**复杂度**：O(n)，n = segment 数。Display 入口每次调用 1 次，Relocate 入口每次调用 1 次，displayToBuffer while 循环每次调用 1 次。**单次 Display 总成本 O(N·n)**，N = 渲染段数。与方案 A preRender 一致。

### 6.2 renderSegmentMD 加 buf 参数

**改造前**（`bufwindow_md.go:62`）：

```go
func (w *BufWindow) renderSegmentMD(
    seg md.Segment, vY int,
) (newVY int)
```

**改造后**：

```go
func (w *BufWindow) renderSegmentMD(
    seg md.Segment, vY int, buf *screenBuffer,
) (newVY int)
```

**内部变化**：

```go
// 改造前
screen.SetContent(screenX, screenY, cell.Rune, cell.Combining, style)

// 改造后
buf.SetCell(col, vY, cell.Rune, cell.Combining, style)
// 其中 screenX = w.X + w.gutterOffset + col → col = screenX - w.X - w.gutterOffset
```

`viewportRowmap` 写入改为 `buf.rowmap`：

```go
// 改造前
w.viewportRowmap[vY] = SLoc{Line: row.BufLine, Row: segRow}

// 改造后
buf.rowmap[vY] = SLoc{Line: row.BufLine, Row: segRow}
```

`showCursor` 改为 `buf.ShowCursor`：

```go
// 改造前
w.showCursor(w.X+vloc.X, w.Y+vloc.Y, c.Num == 0)

// 改造后
buf.ShowCursor(vloc.X, vloc.Y) // cells 坐标系，showBuffer 时再加 w.X/w.Y
```

### 6.3 renderSegmentNative 加 buf 参数

**改造前**（`bufwindow_md.go:219`）：

```go
func (w *BufWindow) renderSegmentNative(
    seg md.Segment, startVY int,
) (newVY int)
```

**改造后**：

```go
func (w *BufWindow) renderSegmentNative(
    seg md.Segment, startVY int, buf *screenBuffer,
) (newVY int)
```

**两处 SetContent 替换**：

```go
// 改造前（行内主循环 ~520）
screen.SetContent(w.X+vloc.X, w.Y+vloc.Y, r, combc, style)
// 改造后
buf.SetCell(vloc.X, vloc.Y, r, combc, style)

// 改造前（行尾填充 ~667）
screen.SetContent(i+w.X, vloc.Y+w.Y, ' ', nil, curStyle)
// 改造后
buf.SetCell(i, vloc.Y, ' ', nil, curStyle)
// 其中 i 是从 vloc.X 到 maxWidth 的循环变量，等价于相对列偏移
```

`drawGutter` / `drawLineNum` / `drawDiffGutter` / `draw` 内部的所有 `screen.SetContent` 改为 `buf.SetCell`。这些函数已经以 `(vloc *buffer.Loc, bloc *buffer.Loc)` 形式接收坐标，**vloc.X 和 vloc.Y 已经是相对窗口坐标**，无需转换。

### 6.4 displayBufferMD 重写为调度器

**改造前**（`bufwindow_md.go:700`）：

```go
func (w *BufWindow) displayBufferMD(editMode bool) {
    // ... 渲染主循环（写屏 + 写 viewportRowmap）
}
```

**改造后**：

```go
func (w *BufWindow) displayBufferMD(editMode bool) {
    b := w.Buf
    if w.Height <= 0 || w.Width <= 0 {
        return
    }
    w.ensureMDConfigReady()

    // lazy render：startLine 变化或 cells 失效时重新渲染
    if w.screenBuf == nil ||
       w.screenBuf.startLine != w.StartLine.Line ||
       w.screenBuf.overflow {
        w.displayToBuffer(w.screenBuf, w.StartLine.Line, editMode)
    }

    w.showBuffer(w.screenBuf, w.StartLine.Line)

    // 更新 prevCursorY（供下次 Relocate 决策）
    w.updatePrevCursor()
}
```

### 6.5 Relocate 入口微调

**改造前**（`bufwindow.go:249`）：

```go
if w.Buf.IsMD {
    ret = w.relocateVerticalMD(c, scrollmargin, height)
} else {
    // 原生
}
```

**改造后**：

```go
if w.Buf.IsMD {
    // ★ 关键：Relocate 决策前先渲染一份 fresh rowmap
    if w.screenBuf == nil ||
       w.screenBuf.startLine != w.StartLine.Line ||
       w.screenBuf.overflow {
        w.displayToBuffer(w.screenBuf, w.StartLine.Line, w.editMode)
    }
    ret = w.relocateVerticalMD(c, scrollmargin, height)
} else {
    // 原生 Relocate，不动
}
```

---

## 7. 边界场景与对策

### 7.1 跨段光标切换（v1.0.5 已知限制）

**场景**：`docs/sample.md` line55-59 表格，光标从 line59 按 ↓ 到 line60（脱离表格）。

**问题**：表格段从 native 切回 MD，重新展开装饰行，viewportRowmap 失效。

**对策**（Commit 3 修复）：
- Relocate 入口调 `displayToBuffer` 拿 fresh rowmap
- displayToBuffer 的三目的停止条件确保旧段完整渲染、新 cursor 段完整渲染、viewport 填满
- realStartLine 在 cells 范围内 → showBuffer 复用 cells

**回归验证**：连续按 ↓ 直到 viewport 底边，光标应始终可见。

### 7.2 screenBuffer 容量不足

**场景**：极大单 segment（如长代码块 > 100 行）跨段时，2×bufHeight 不够。

**对策**：
- `buf.overflow = true`
- `showBuffer` 走 `w.displayBuffer()` 原生路径兜底
- `Relocate` 入口看到 overflow 时也走 `relocateVerticalNativeFallback`

**降级行为**：fallback 不崩，但 scroll 估算不准（1 帧不完美）。用户按 ↓/↑ 时下一帧 Display 检测到 startLine 变化 → 重新 displayToBuffer → 自纠正。

### 7.3 showBuffer blit 边界

**边界场景**：
- `screenStartRow` 落装饰行（应跳到首个内容行）
- screenBuffer 尺寸不足
- 空白填充（buffer 内容不够填满 viewport）
- softwrap 下 `newStartLine.Row ≠ 0` 的偏移

**对策**：
- `screenStartRow` 计算集中在 showBuffer 入口（约 5 行）
- overflow 检查分散到 showBuffer 与 Relocate 两个函数
- 总边界分支数比方案 A 略多（方案 A 集中在 Relocate 一个函数）

**复杂度评估**：showBuffer 的 blit 逻辑本身 ~15 行（双层 for 循环），边界处理集中在 overflow 标志 + screenStartRow 计算上。

### 7.4 多 BufWindow 内存

**关键设计决策**：cells 用完即弃（showBuffer 完成后 `cells = nil`）。

| 场景 | cells 状态 | 内存 |
|---|---|---|
| 单一 active window 渲染中 | 保留 | ~448 KB |
| showBuffer 后 | nil | 0 |
| 切换 window 后 | nil（释放）| 0 |
| 新 active window 触发 displayToBuffer | 重新分配 | ~448 KB |

**总开销**：任意时刻，1 个 BufWindow 持有 cells = ~448 KB；4 个 BufWindow 同时持有 cells = ~1.8 MB。

**注意**：cells 释放决策是 `showBuffer 完成后立即 nil`，而不是 "切换 window 时释放"。这样设计的好处：
- 单 window 场景：cells 复用率高（displayToBuffer 一次 → showBuffer 多次）
- 多 window 场景：cells 占用窗口短（blit 完立即释放）
- 实现简单：无状态机、无 LRU

### 7.5 首帧渲染

**场景**：BufWindow 刚创建，screenBuf 为 nil。

**对策**：
- `displayBufferMD` 入口检查 `w.screenBuf == nil` → 触发 displayToBuffer
- Relocate 入口同样检查 → 触发 displayToBuffer
- 不会出现 nil panic

**prevCursorY 初始值**：`NewBufWindow` 设为 -1（sentinel），首次 Relocate 时 `findSegmentContaining(-1)` 返回 nil → `oldSegDone = true` → 不会过度渲染。

### 7.6 softwrap 下的 rowmap

**场景**：softwrap 模式下，单 buffer 行可分多屏行。

**现状**：方案 A 和方案 B 都依赖 v1.0.5 的 2D `viewportRowmap`（`LineToScreenRow(line, row)`）。方案 B 复用此逻辑，`w.viewportRowmap` 改为 `w.screenBuf.rowmap`。

**对策**：无需额外改动，rowmap 字段已经是 SLoc 二元组。

### 7.7 scrollbar / statusline / infobar

**现状**：方案 B 不接管这些 UI 元素。`BufWindow.Display` 入口先调 `displayStatusLine()` + `displayScrollBar()`，再调 `displayBufferMD()`。

**对策**：保持现状不动。

---

## 8. 测试策略

### 8.1 单元测试（`internal/display/bufwindow_md_test.go`）

**screenBuffer**：

```go
func TestScreenBuffer_SetCell(t *testing.T) {
    buf := &screenBuffer{bufWidth: 80, cells: make([][]screenCell, 50)}
    buf.SetCell(0, 0, 'A', nil, tcell.StyleDefault)
    if buf.cells[0][0].r != 'A' {
        t.Error("SetCell failed")
    }
}

func TestScreenBuffer_Overflow(t *testing.T) {
    buf := &screenBuffer{bufWidth: 80}
    buf.SetCell(0, 100, 'A', nil, tcell.StyleDefault) // 越界
    // 期望：no-op，不 panic
}
```

**findSegmentContaining**：

```go
func TestFindSegmentContaining(t *testing.T) {
    // 准备 MDSegments
    w.Buf.MDSegments = []md.Segment{
        {BufStartLine: 0, BufEndLine: 5},
        {BufStartLine: 10, BufEndLine: 20},
    }
    if w.findSegmentContaining(3) == nil { t.Error("expected non-nil") }
    if w.findSegmentContaining(7) != nil { t.Error("expected nil") }
    if w.findSegmentContaining(-1) != nil { t.Error("expected nil for negative") }
}
```

**displayToBuffer 停止条件**：

```go
func TestDisplayToBuffer_StopsAtViewportFilled(t *testing.T) {
    // 准备足够多的 segments
    // 调 displayToBuffer
    // 期望：vY >= bufHeight 时停止
}

func TestDisplayToBuffer_StopsAfterOldSeg(t *testing.T) {
    // 准备 prevCursorY 命中的段
    // 调 displayToBuffer
    // 期望：旧段渲染完后继续，但满足其他目的时可 break
}
```

### 8.2 集成测试（人工）

**场景 1：跨段光标切换**（v1.0.5 已知限制）：

1. 打开 `docs/sample.md`
2. 光标移到 line59（表格最后一行）
3. 连续按 ↓ 直到 viewport 底边
4. **期望**：光标始终在 viewport 内可见，不再"消失一帧"

**场景 2：纯滚动**（case B）：

1. 打开任意 MD 文件
2. 按 PageDown / Ctrl+D 滚动
3. **期望**：渲染结果与 v1.0.5 一致

**场景 3：goto / search**（case C）：

1. 打开 MD 文件
2. Ctrl+G 输入行号跳到文件末尾
3. **期望**：渲染正常，无 panic

**场景 4：editMode 切换**：

1. 打开 MD 文件
2. 按 ESC → 阅读模式
3. 按任意键 → 编辑模式
4. **期望**：render 与 native 切换正常，无闪烁

### 8.3 回归测试

**与 v1.0.5 行为基线对比**：

- `docs/sample.md` 渲染结果不变（Commit 1 验证）
- 行号、gutter、diff、selection、cursorline 等所有 UI 元素位置不变
- softwrap 行为不变
- scrollbar / statusline 不受影响

---

## 9. 风险登记

| 风险 | 等级 | 缓解 |
|---|---|---|
| **SetContent 漏改**（渲染函数写屏但 cells 没填充） | 高 | Commit 1 编译时所有 SetContent 调用必须全部替换；人工 review |
| **cells 坐标系错位**（vX / vY 偏移计算错） | 高 | Commit 2 重点验证；与 v1.0.5 基线对比 |
| **prevCursorY 没及时更新**（Display 末尾漏调 updatePrevCursor） | 中 | Commit 3 验证跨段场景 |
| **overflow 频繁触发**（2×bufHeight 不够用） | 中 | 长期代码块场景人工验收；如频繁可调为 3×bufHeight |
| **多 BufWindow 内存泄漏**（cells 引用未释放） | 低 | showBuffer 末尾 cells = nil 保证；测试覆盖 |
| **测试覆盖不足**（display 包测试缺失） | 中 | Commit 4 补全；旧测试已删，参考 `docs/光标滚动-修改总结.md` §5 |
| **cells 释放时机抖动**（频繁 displayToBuffer） | 低 | 单 window 场景 cells 复用率高；多 window cells 占用窗口短 |

---

## 10. 回退方案

如果方案 B 实施中发现无法解决的问题（如 cells 释放导致闪烁、overflow 触发频繁、内存压力），回退路径：

**路径 A**：回退到方案 A（`docs/方案A架构设计-很臭的方案.md`）

- 方案 A 的代码量更小（~105 行）
- 方案 A 不引入 cells 抽象，只用 viewportRowmap
- 方案 A 的 stale map 风险用 preRender dryRun 缓解（不如方案 B 彻底，但够用）

**路径 B**：回退到 v1.0.5

- 保留现状的 stale map 风险
- 用户接受"按 ↓ 到底边时光标偶发消失一帧"作为已知限制

**回退成本**：方案 B → v1.0.5 是 git revert，方案 B → 方案 A 是 `git reset` 到 Commit 2 然后 cherry-pick 方案 A 的 commit。

---

## 11. 性能与内存评估

### 11.1 单次 displayToBuffer 成本

| 阶段 | 成本 |
|---|---|
| `findSegmentContaining`（while 循环 N 次）| O(N·n) ≈ 50-200μs |
| `renderSegmentMD` / `renderSegmentNative` | O(visible 行数) ≈ 1-3ms |
| cells 分配 / 重用 | < 0.1ms |
| rowmap 写入 | O(bufHeight) < 0.1ms |
| **总计** | **~2-3ms** |

**对比 v1.0.5**：displayToBuffer 比 displayBufferMD 略慢（多一次 cells 写入），但都在 16ms 内。

### 11.2 单次 showBuffer 成本

| 阶段 | 成本 |
|---|---|
| blit cells 到 screen | O(bufHeight × bufWidth) ≈ 2000-4000 次 SetContent |
| cells = nil | O(1) |
| **总计** | **~0.5ms** |

### 11.3 内存占用

**单 BufWindow**：

| 字段 | 大小 |
|---|---|
| `cells[2×bufHeight][bufWidth]` | 100 × 80 × 24B = 192 KB（活跃时）|
| `rowmap[2×bufHeight]` | 100 × 8B = 800 B |
| `cap` / `bufWidth` / `startLine` 等 | < 100 B |
| **单 BufWindow 峰值** | **~200 KB** |

**4 BufWindow 同时打开**（cells 都在活跃）：

- 4 × 200 KB = **~800 KB**

**切换 window 后**：

- 旧 window cells 释放（= nil）
- 新 window cells 重新分配
- 4 BufWindow 中只有 1 个 cells 活跃：**~200 KB**

**对比方案 A 的 viewportRowmap**：

- 方案 A 单 BufWindow：800 B（仅 rowmap）
- 方案 B 单 BufWindow：200 KB（rowmap + cells）
- **差距 250 倍**，但绝对值 < 500 KB（仍可接受）

---

## 12. 决策检查清单

实施前确认：

- [ ] 是否已读完 `docs/方案B-评估报告mm.md` 全文
- [ ] 是否已读完 `docs/光标滚动-修改总结.md` v1.0.5 现状
- [ ] 是否已读完 `docs/光标滚动-方案B.md` 原始设计
- [ ] 是否已对比 `docs/方案A架构设计-很臭的方案.md` 备选方案
- [ ] 是否接受 ~153 行的 display 包侵入
- [ ] 是否接受 cells 渲染缓存的 ~200 KB / active window 内存开销
- [ ] 是否接受 2×bufHeight overflow 兜底（可能偶发 fallback 路径）
- [ ] 是否接受 5 处 SetContent 替换 + render 函数体零改动的实现策略

实施后验证：

- [ ] `make build` 通过
- [ ] `make build-quick` 通过
- [ ] 跨段场景（`docs/sample.md` 表格）光标不再消失
- [ ] 纯滚动场景渲染正常
- [ ] goto / search 场景无 panic
- [ ] editMode 切换无闪烁
- [ ] 4 BufWindow 同时打开不卡顿

---

## 13. 关联文档索引

| 文档 | 用途 |
|---|---|
| `docs/方案B-评估报告mm.md` | 实施前的可行性评估（本文档的依据） |
| `docs/光标滚动-方案B.md` | 方案 B 原始设计文档（displayToBuffer + screenBuffer 设想） |
| `docs/光标滚动-修改总结.md` | v1.0.5 现状总结（含已知限制、本计划修复目标） |
| `docs/方案A架构设计-很臭的方案.md` | 备选方案 A（preRender + dryRun） |
| `docs/RenderedSegment数据结构.md` | 渲染数据结构参考（cells + rowmap 字段语义） |
| `docs/光标滚动-方案A.md` | 方案 A 原始问题描述 |
| `internal/display/bufwindow_md.go` | 主要改动文件（~140 行新增/改） |
| `internal/display/bufwindow.go` | 次要改动文件（~10 行） |
| `internal/md/md.go` / `detect.go` / `render_*.go` | **零改动**，只读参考 |
| `internal/display/softwrap.go` | **零改动**，Scroll/Diff 全部复用 |
