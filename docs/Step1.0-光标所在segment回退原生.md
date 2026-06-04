# Step 1.0：光标所在 segment 回退原生显示

> 前置：Step 0 已完成（背景色渲染管线跑通）
> 交付：MD 文件 editMode=true 时，**光标所在 segment 回退 Micro 原生显示**，其余 segment 继续走 MD 渲染
> 验证方式：临时将 MD 文件默认 editMode 设为 true，打开后即可直接观察回退效果

---

## 验收标准

打开 .md 文件（editMode=true）：

- 光标所在 segment 显示原生文本（带 `#`、`**`、`|` 等标记）
- 其余 segment 继续显示 MD 渲染输出（背景色）
- 上下移动光标：回退的 segment 跟随光标移动
- 多个光标在同一个 seg 内：该 seg 走原生（一个 seg 不会被部分原生部分渲染）
- 多个光标跨多个 seg：每个被光标命中的 seg 独立走原生（多块原生段之间夹 MD 渲染段，符合预期）

非 MD 文件、`mdrender=false` 时不受影响。

---

## 回退判定规则

`editMode && 任意一个光标落在 [seg.BufStartLine, seg.BufEndLine]（含两端）` 则走原生路径，否则走 MD 渲染路径。Step 1.0 中 `editMode` 恒为 `true`，条件退化为"光标在 seg 内"。

```go
// hasCursorInside 判定 seg 内是否有任意光标。
//
// 回退判定（displayBufferMD 主循环使用）：
//   - editMode=true（编辑模式）+ 光标在 seg 内 → 走原生
//   - editMode=false（阅读模式）             → 全部走 MD，不回退
func hasCursorInside(seg md.Segment, cursors []*buffer.Cursor) bool {
    for _, c := range cursors {
        if c.Y >= seg.BufStartLine && c.Y <= seg.BufEndLine {
            return true
        }
    }
    return false
}
```

> **注意**：`buffer.GetCursors()` 返回 `[]*Cursor`（指针切片），因此参数类型是 `[]*buffer.Cursor` 而非 `[]buffer.Cursor`。

---

## 改动清单

### 1. BufWindow 增加 editMode 字段

**文件**：`internal/display/bufwindow.go`

```go
type BufWindow struct {
    // ... 现有字段 ...
    editMode bool // Step 1.0 固定为 true（光标片回退原生）
}
```

Step 1.0 阶段 `editMode` **没有 setter、没有 getter**——只是一个内部标记，由 `BufPane` 在创建时设为 `true` 即可。Step 1.1 会增加 `SetEditMode` 等方法和键盘交互。

### 2. Display() 分流

**文件**：`internal/display/bufwindow.go`

```go
// Step 0：
if w.Buf.IsMD {
    w.displayBufferMD()
} else {
    w.displayBuffer()
}

// Step 1.0 改为：
if !w.Buf.IsMD || !w.mdConfig.MDRender {
    w.displayBuffer()
} else {
    w.displayBufferMD(w.editMode)
}
```

非 MD 文件或总开关 `MDRender=false` 时，**完全不走 MD 路径**，由原生 `displayBuffer()` 处理。`displayBufferMD(editMode)` 内部按"回退判定规则"分发——`editMode` 由 `BufPane` 在创建 BufWindow 时设为 `true`，Step 1.1 会增加切换机制。

### 3. renderSegmentNative 新建 ★ 本步核心改动

#### 3.1 设计原则：复制而非重构

**`displayBuffer()` 保持原样，0 改动**（符合 `CLAUDE.md`"对 micro 原生代码的侵入越小越好"）。

本步新增一个**独立函数** `renderSegmentNative`，把 `displayBuffer()` 主循环（约 458 行）原样复制过来，**只改 5 处**（含 1 处新增返回值）：

1. **bloc.Y 初值改为 `seg.BufStartLine`**（原版用 `w.StartLine.Line`，现在 seg 已被裁剪到可见区，从 seg 起始行开始）
2. **vloc.Y 初值 = `startVY`**（去掉 softwrap offset，seg 已被 `filterSegmentsToVisible` 裁到可见区，首行无 partial row）
3. **循环条件加 `bloc.Y <= seg.BufEndLine`**（双重终止：屏幕满 + buffer 行用完）
4. **末尾 `bloc.Y >= b.LinesNum()` 改为 `bloc.Y > seg.BufEndLine`**
5. **循环结束后构造 perLineCounts 并返回 `(newVY, perLineCounts)`**

**不需要** `displayContext` 结构体——`displayBuffer()` 不重构，新函数内部闭包捕获的变量全是 local var，与 `displayBuffer()` 完全对称。

#### 3.2 函数签名

```go
// renderSegmentNative 用原生逻辑渲染一个 segment 到 BufWindow。
// seg 必须已经过 filterSegmentsToVisible 裁剪（BufStartLine/BufEndLine 在可见区内）。
// 不修改 displayBuffer()，从那里复制主循环并按需调整。
//
// startVY : 起始 screen 行（相对窗口顶部）
// 返回:
//   newVY         - 渲染后的 vY（=startVY + 实际消耗的 screen 行数）
//   perLineCounts - 每个 buffer 行占几个 screen row（长度 = seg.BufEndLine-seg.BufStartLine+1）
//                   Step 1.0 softwrap off → 全 1
func (w *BufWindow) renderSegmentNative(
    seg md.Segment, startVY int,
) (newVY int, perLineCounts []int)
```

#### 3.3 从 displayBuffer() 复制的具体清单

**完整复制**（参考 `bufwindow.go` 当前行号；行号会随代码漂移，以实际为准）：

| 内容 | 当前行号 | 说明 |
|------|---------|------|
| 前置：`b := w.Buf`、宽高检查、maxWidth、`modified-this-frame`、`matchingBraces` 解析 | L389-446 | 完整复制 |
| 前置：lineNumStyle/curNumStyle、softwrap/wordwrap、tabsize/colorcolumn、showchars 解析、vloc/bloc 初始化、cursors、curStyle、外层 for 头 | L448-483 | 完整复制 |
| 闭包 `getRuneStyle` | L528-616 | 完整复制 |
| 闭包 `draw` | L617-693 | 完整复制 |
| 闭包 `wrap` | L694-722 | 完整复制 |
| word buffer 初始化 | L723-729 | 完整复制 |
| 内层 rune 解码 + 渲染 + wrap 主循环 | L730-819 | 完整复制 |
| EOL 填充 + newline-in-selection | L820-842 | 完整复制 |
| 末尾 `bloc.Y++` 和 `if bloc.Y >= b.LinesNum()` | L840-842 | 复制 + 调整 |
| 结尾大括号 | L843-846 | 完整复制 |

**复制后修改 5 处**：

**(1) bloc.Y 初值改为 seg.BufStartLine**（原 L467 附近）：

```go
// 原版：
bloc := buffer.Loc{X: -1, Y: w.StartLine.Line}
// 改为：
bloc := buffer.Loc{X: -1, Y: seg.BufStartLine}
```

理由：`renderSegmentNative` 渲染的是单个 seg，应从 `seg.BufStartLine` 开始，而非从视口起始行 `w.StartLine.Line` 开始。`X: -1` 保留不变（后面 `bloc.X = w.StartCol` 会覆盖）。

**(2) 去掉 softwrap offset，vloc.Y 初值改为 startVY**（原 L460-464）：

```go
// 原版：
vloc := buffer.Loc{X: 0, Y: 0}
if softwrap {
    vloc.Y = -w.StartLine.Row
}
// 改为：
vloc := buffer.Loc{X: 0, Y: startVY}
```

理由：`filterSegmentsToVisible` 已把 `seg.BufStartLine` 裁到 `visibleStart`，seg 内首行不可能是 partial row；不需要 `StartLine.Row` offset。`startVY` 由调用方传入，保证 ≥ 0。

**(3) 循环条件加 seg 边界**（原 L456 附近）：

```go
// 原版：
for ; vloc.Y < w.bufHeight; vloc.Y++ {
// 改为：
for ; vloc.Y < w.bufHeight && bloc.Y <= seg.BufEndLine; vloc.Y++ {
```

**(4) 末尾 break 条件改为 seg 边界**（原 L841）：

```go
// 原版：
if bloc.Y >= b.LinesNum() {
    break
}
// 改为：
if bloc.Y > seg.BufEndLine {
    break
}
```

**(5) 循环结束后构造 perLineCounts 并返回**（替换原版末尾的 `}`）：

```go
// softwrap off 时每个 buffer 行 = 1 screen row，简单构造全 1 slice
perLineCounts = make([]int, seg.BufEndLine-seg.BufStartLine+1)
for i := range perLineCounts {
    perLineCounts[i] = 1
}
return vloc.Y, perLineCounts
```

> **注**：softwrap off 时每个 buffer 行 = 1 screen row，perLineCounts 全 1。如果未来 MD 路径要支持 softwrap，需要在外层 for body 开头记录 `lineStartVY = vloc.Y`，在末尾用 `vloc.Y - lineStartVY + 1` 统计本行实际消耗的 screen 行数。Step 1.0 简化不做。

### 4. displayBufferMD 重写 ★ 本步主要逻辑改动

#### 4.1 目标

主循环里按"回退判定规则"分发到两个路径：
- **MD 路径**（`renderSegmentMD`）：现有逻辑
- **原生路径**（`renderSegmentNative`）

#### 4.2 伪代码

```go
func (w *BufWindow) displayBufferMD(editMode bool) {
    b := w.Buf
    if w.Height <= 0 || w.Width <= 0 { return }

    if b.ModifiedThisFrame {
        if b.Settings["diffgutter"].(bool) { b.UpdateDiff() }
        b.ModifiedThisFrame = false
    }

    visibleStart := w.StartLine.Line
    visibleEnd := visibleStart + w.bufHeight
    if visibleEnd >= b.LinesNum() { visibleEnd = b.LinesNum() - 1 }

    w.mdCache = w.mdCache[:0]
    segments := filterSegmentsToVisible(b.MDSegments, visibleStart, visibleEnd)
    cursors := b.GetCursors()

    vY := 0
    for _, seg := range segments {
        var perLine []int
        if editMode && hasCursorInside(seg, cursors) {
            // ★ 原生路径：renderSegmentNative 内部自己处理 gutter
            vY, perLine = w.renderSegmentNative(seg, vY)
        } else {
            // ★ MD 渲染路径
            vY, perLine = w.renderSegmentMD(seg, vY)
        }
        w.mdCache = append(w.mdCache, md.SegmentMeta{
            BufStartLine: seg.BufStartLine,
            BufEndLine:   seg.BufEndLine,
            RowCounts:    perLine,
        })
    }

    // 填剩余空间
    defStyle := config.DefStyle
    for ; vY < w.bufHeight; vY++ {
        w.drawGutterAndLineNumMD(vY, -1)
        for col := 0; col < w.bufWidth; col++ {
            screen.SetContent(w.X+w.gutterOffset+col, w.Y+vY, ' ', nil, defStyle)
        }
    }
}
```

#### 4.3 renderSegmentMD 提取

把现有 `displayBufferMD` 中的 "取行 → render → 行号绝对化 → 写 screen" 逻辑**提取为辅助函数** `renderSegmentMD(seg, vY) (newVY, perLineCounts)`（mdCache 写入提到 displayBufferMD 主循环统一处理）：

```go
// renderSegmentMD 渲染单个 MD segment 到 screen。
// 与 renderSegmentNative 对称：都是纯函数，渲染 + 返回 (newVY, perLineCounts)。
// 不写 mdCache——由 displayBufferMD 主循环统一写。
// 从 displayBufferMD 主循环中提取出来的 Step 0 逻辑。
func (w *BufWindow) renderSegmentMD(
    seg md.Segment, vY int,
) (newVY int, perLineCounts []int) {
    lines := w.linesFromBuffer(seg.BufStartLine, seg.BufEndLine)
    rendered := seg.Render(lines, w.bufWidth, w.mdConfig)

    // 相对 BufLine → 绝对
    for ri := range rendered.Rows {
        rendered.Rows[ri].BufLine += seg.BufStartLine
        for ci := range rendered.Rows[ri].Cells {
            if rendered.Rows[ri].Cells[ci].BufLine >= 0 {
                rendered.Rows[ri].Cells[ci].BufLine += seg.BufStartLine
            }
        }
    }

    for _, row := range rendered.Rows {
        if vY >= w.bufHeight { break }
        w.drawGutterAndLineNumMD(vY, row.BufLine)

        var lastFg tcell.Color
        var lastAttr tcell.AttrMask
        hasLast := false
        for col, cell := range row.Cells {
            screenX := w.X + w.gutterOffset + col
            screenY := w.Y + vY
            if screenX < w.X+w.gutterOffset || screenX >= w.X+w.gutterOffset+w.bufWidth {
                continue
            }
            style := cell.Style
            if cell.BufLine >= 0 && cell.BufX >= 0 {
                if group, ok := w.Buf.Match(cell.BufLine)[cell.BufX]; ok {
                    s := config.GetColor(group.String())
                    lastFg, _, lastAttr = s.Decompose()
                    hasLast = true
                }
                if hasLast {
                    _, bg, _ := style.Decompose()
                    style = tcell.StyleDefault.Foreground(lastFg).Background(bg)
                    if lastAttr&tcell.AttrBold != 0 {
                        style = style.Bold(true)
                    }
                    if lastAttr&tcell.AttrBlink != 0 {
                        style = style.Blink(true)
                    }
                    if lastAttr&tcell.AttrReverse != 0 {
                        style = style.Reverse(true)
                    }
                    if lastAttr&tcell.AttrUnderline != 0 {
                        style = style.Underline(true)
                    }
                    if lastAttr&tcell.AttrDim != 0 {
                        style = style.Dim(true)
                    }
                    if lastAttr&tcell.AttrItalic != 0 {
                        style = style.Italic(true)
                    }
                    if lastAttr&tcell.AttrStrikeThrough != 0 {
                        style = style.StrikeThrough(true)
                    }
                }
            }
            screen.SetContent(screenX, screenY, cell.Rune, cell.Combining, style)
        }
        vY++
    }

    return vY, computeRowCounts(rendered)
}
```

#### 4.4 关键注意点

1. **gutter 只画一次**：原生路径由 `renderSegmentNative` 内部调 `drawGutter`/`drawLineNum`；MD 路径由 `renderSegmentMD` 调 `drawGutterAndLineNumMD`。**`displayBufferMD` 主循环不再统一调 gutter**——避免画两次。

2. **行号风格一致**：原生 gutter 用 `line-number` / `current-line-number` 配色（`drawLineNum` 内部读取 `config.Colorscheme`），和 MD 路径的 `drawGutterAndLineNumMD` **完全共用同一套配色**——视觉上无差异。

3. **mdCache 写法对称**：两个分支都写 `mdCache`，`BufStartLine/BufEndLine/RowCounts` 字段一致，下游消费代码无需区分路径。

### 5. BufPane 在 MD 分支设置 editMode=true

**文件**：`internal/action/bufpane.go`

Step 1.0 阶段 BufPane **不增加任何字段和方法**——只在 `NewBufPaneFromBuf` 的 MD 分支里，在 `*BufWindow` 上设置 `editMode = true`：

```go
func NewBufPaneFromBuf(buf *buffer.Buffer, tab *Tab) *BufPane {
    w := display.NewBufWindow(0, 0, 0, 0, buf)

    // MicroNeo: MD 标志和配置
    if buf.IsMD {
        w.SetMDConfig(md.MDConfig{...})
        // Step 1.0：MD 文件默认进入编辑模式（光标片回退原生）
        w.editMode = true  // ← 在 *BufWindow 上直接设置（同包访问）
    }

    h := newBufPane(buf, w, tab)
    return h
}
```

> **为什么不能写 `h.BWindow.editMode = true`**：`h.BWindow` 的类型是 `display.BWindow` **接口**，不是 `*BufWindow` 结构体。接口不暴露未导出字段。必须在 `NewBufWindow` 返回的 `*BufWindow`（`w`）上直接设置，然后再传给 `newBufPane`。因为 `bufpane.go` 和 `bufwindow.go` 同属不同 package（`action` vs `display`），`editMode` 是小写字段（未导出），所以有两种方案：
>
> - **方案 A**（推荐）：`editMode` 改为导出字段 `EditMode`，或提供 `SetEditMode(bool)` 方法——Step 1.1 反正要加，Step 1.0 可以先加上。
> - **方案 B**：在 `display` 包内提供一个包级函数 `SetEditMode(bw *BufWindow, v bool)`——但不够优雅。
>
> Step 1.0 建议直接采用方案 A，给 `BufWindow` 加一个 `SetEditMode(bool)` 方法（约 3 行），在 `NewBufPaneFromBuf` 中调用 `w.SetEditMode(true)`。

Step 1.1 会：
- 把 `NewBufPaneFromBuf` 里这行去掉（默认 `false`，阅读模式）
- 新增 `enterEditMode` / `exitEditMode` 切换 `editMode` 字段
- 加键盘拦截调用切换方法

---

## 实施顺序

```
1. BufWindow editMode 字段（1 行）
2. renderSegmentNative 新建（从 displayBuffer() 复制 ~458 行 + 改 5 处）
3. hasCursorInside 辅助函数（~10 行）
4. renderSegmentMD 提取（从 displayBufferMD 抽出 MD 渲染逻辑，~80 行）
5. displayBufferMD 重写（if/else 分发，~30 行）
6. Display() 分流逻辑（MDRender 开关）
7. BufPane NewBufPaneFromBuf MD 分支调用 w.SetEditMode(true)（约 3 行，含方法定义）
8. 验证：
   - 打开 MD 文件：非光标所在 seg 仍渲染（Step 0 行为不破坏）
   - 移动光标进入 seg：该 seg 切回原生（带 #、**、| 等标记）
   - 多光标测试：跨 seg 时多个 seg 独立走原生
   - 对比 displayBuffer() 原生输出：行号、gutter、syntax 高亮视觉一致
```

---

## 改动文件

| 文件 | 改动 |
|------|------|
| `internal/display/bufwindow.go` | editMode 字段、`SetEditMode(bool)` 方法（~3 行）、renderSegmentNative 新建、hasCursorInside 新建、renderSegmentMD 提取、displayBufferMD 重写（加 editMode 参数 + 主循环条件改为 `editMode && hasCursorInside`）、Display 分流（传 w.editMode） |
| `internal/action/bufpane.go` | NewBufPaneFromBuf MD 分支调用 `w.SetEditMode(true)`（1 行） |

**注意**：`displayBuffer()` 0 改动。

---

## 风险

| 风险 | 缓解 |
|------|------|
| **代码克隆 ~458 行**（`renderSegmentNative` 复制自 `displayBuffer`） | 严格遵守 CLAUDE.md "对原生代码侵入最小"；未来 micro 上游升级时需手动 diff 同步，但 Step 1.0 阶段可接受 |
| 复制时漏改某处（如忘记去掉 softwrap offset） | 5 处修改清单明确列出（含关键的 bloc.Y 初值改为 seg.BufStartLine）；测试时可构造一个 softwrap on 的场景，验证 vY 计数没多/少 |
| `perLineCounts` 全填 1 可能在 softwrap on 时不准 | Step 1.0 假设 softwrap off（与 Step 0 一致）；未来 MD 路径要支持 softwrap 时再细化 |
| 原生路径与 MD 路径的 gutter 风格不一致 | 共用 `line-number` / `current-line-number` 配色（`drawGutter`/`drawLineNum` 本来就是 micro 的），走原生路径实际上就是用 micro 的原生行号绘制——必然一致 |
| 多光标跨多个 seg 时的视觉碎片化 | 设计上接受"多个原生块之间夹 MD 渲染块"；符合 Step 1.0 目标 |
| `hasCursorInside` 在大 seg（占满屏幕）下命中率 100% | 不影响功能：大 seg 全走原生，符合"光标所在的整片都用原生编辑"的语义 |

**估计代码量**：~575 行（renderSegmentNative ~458 + renderSegmentMD ~90 + hasCursorInside ~10 + SetEditMode ~3 + 杂项 ~15）
