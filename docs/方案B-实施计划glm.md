# 方案 B 实施计划（glm 版）

> 依据：`docs/方案B-评估报告glm.md`（七轮澄清后的定稿设计）
> 范围：把光标滚动从「每帧 render + viewportRowmap shadow」重构为「displayToBuffer → screenBuffer（单一数据源）→ showBuffer blit」
> 目标根 bug：跨段（表格/代码块）时 cursor 切换渲染模式增删装饰行，旧 viewportRowmap 失配导致 Relocate 算错滚动量
> 编译：`make build`（或 `make build-quick` 跳过 generate）

---

## 0. 概述与边界

### 0.1 改动文件清单

| 文件 | 改动性质 | 行数估算 | 说明 |
|------|---------|---------|------|
| `internal/display/bufwindow_md.go` | **主战场** | ~150 行 | 新增 screenRow/screenBuffer/displayToBuffer/showBuffer；改造 relocateVerticalMD、displayBufferMD；retarget SetContent |
| `internal/display/bufwindow.go` | 微改 | ~10 行 | 给 `Relocate`（249）和 `Display`（940）的 MD 分支换调用名；drawGutter/drawDiffGutter/drawLineNum 的 `screen.SetContent` → `w.setCell`（行为等价 retarget） |
| `internal/display/softwrap.go` | 零 | 0 | SLoc 类型不动 |
| `internal/display/bufwindow.go` 之外的 display | 零 | 0 | softwrap.go / window.go 不动 |
| `internal/action/actions.go` | **零** | 0 | 76 处 Relocate 无感 |
| `cmd/micro/micro.go` | **零** | 0 | DoEvent 主循环无感 |
| `internal/screen/screen.go` | **零** | 0 | 真屏接口不动 |
| `internal/buffer/`、`internal/md/` | **零** | 0 | renderer / detect / config 全不动 |

**侵入面与方案 A 等同**（display 包 2 个文件），与评估报告 §3/§6 一致。

### 0.2 现状关键事实（实施前必读）

- `viewportRowmap []SLoc` 是 `BufWindow` 字段（`bufwindow.go:41`），`displayBufferMD` 每帧重置为 `{Line:-2}` 并由 renderer 直接写入
- `SLoc{Line, Row}`：`Line=-1` 装饰行、`Line=-2` 空白、`Line>=0` 内容行（`Row`= softwrap 段号）
- 渲染写屏用**绝对坐标**：`screen.SetContent(w.X+w.gutterOffset+col, w.Y+vY, ...)`（内容区）、`screen.SetContent(w.X+vloc.X, w.Y+vY, ...)`（gutter 区，`vloc.X < gutterOffset`）
- 分发点已存在：`bufwindow.go:249`（`Relocate` → `relocateVerticalMD`）、`bufwindow.go:940`（`Display` → `displayBufferMD`）
- renderer 内部本就有 line 信息：`renderSegmentMD` 的 `row.BufLine`（`bufwindow_md.go:99` 绝对化后）、`renderSegmentNative` 的 `bloc.Y`（`bufwindow_md.go:316`）

---

## 1. 数据结构

```go
// internal/display/bufwindow_md.go 顶部新增

// mdCell 是 screenBuffer 里一个显示单元（rune + 合字 + 样式）。
type mdCell struct {
    r     rune
    combc []rune
    style tcell.Style
}

// screenRow 是 screenBuffer 的一行：line 元数据 + cells 显示数据同源生成。
// cells 覆盖整行宽度（gutterOffset + bufWidth），gutter 区也纳入，供 showBuffer 整行 blit。
type screenRow struct {
    line  int        // 对应 buffer 行（绝对）；-1=装饰行，-2=空白填充。长期保留供查询
    segRow int       // softwrap 段号（同 SLoc.Row）。长期保留供查询
    cells []mdCell   // 显示数据；showBuffer blit 后置 nil 释放
}

// screenBuffer 是 MD 渲染的离屏单一数据源：既是渲染结果，又是 line↔vY 查询地图。
type screenBuffer struct {
    rows      []screenRow  // 长度动态，≤ 2*bufHeight
    startLine SLoc         // 本批 rows 渲染时的起点（case B 范围检查用）
    originX   int          // 渲染窗口原点 X（= w.X），SetContent 绝对坐标→本地坐标转换
    originY   int          // 渲染窗口原点 Y（= w.Y）
    width     int          // 每行 cell 宽度（= gutterOffset + bufWidth）
}
```

**BufWindow 新增字段**（`bufwindow.go` 结构体，替换 `viewportRowmap`）：

```go
// 删除：viewportRowmap []SLoc
// 新增：
sb        *screenBuffer  // MD 离屏缓冲（单一数据源）
sink      cellSink       // 当前渲染目标；默认 realScreenSink，displayToBuffer 期间切到 sb
```

**设计要点**（评估报告 §2.1）：
- `line`/`segRow` 与 `cells` **同源生成**——displayToBuffer 一次渲染同时产出，永不失配（对比方案 A 的 viewportRowmap shadow 双份账）
- `cells` 用完即弃：showBuffer blit 后 `row.cells = nil`；下一帧 displayToBuffer 重建
- LineToScreenRow 查询扫 `sb.rows`，O(行数)，与现状扫 viewportRowmap 同复杂度

---

## 2. retarget 机制（cellSink 接口）

这是整个方案唯一有设计陷阱的点，单列一节。

### 2.1 问题

`displayToBuffer` 期间，renderer 和 gutter 函数**都不能写真屏**（真屏由 showBuffer 统一 blit）。但现状它们都直接调 `screen.SetContent`（绝对坐标），且 `drawGutter/drawDiffGutter/drawLineNum` 是 native + MD 共享的。

### 2.2 方案：runtime switchable sink（推荐）

```go
// cellSink 是渲染目标抽象。SetContent 签名与 screen.SetContent 对齐（绝对坐标）。
type cellSink interface {
    SetContent(x, y int, mainc rune, combc []rune, style tcell.Style)
}

// realScreenSink 包装 screen.SetContent，原生路径行为字节级不变。
type realScreenSink struct{}
func (realScreenSink) SetContent(x, y int, mainc rune, combc []rune, style tcell.Style) {
    screen.SetContent(x, y, mainc, combc, style)
}

// screenBuffer 实现 cellSink：绝对坐标 → 本地 (row, cell)。
func (s *screenBuffer) SetContent(x, y int, mainc rune, combc []rune, style tcell.Style) {
    row := y - s.originY
    col := x - s.originX
    if row < 0 || row >= len(s.rows) || col < 0 || col >= s.width {
        return // 越界丢弃（screenBuffer 是 2x cap，渲染可能溢出尾部）
    }
    s.rows[row].cells[col] = mdCell{r: mainc, combc: combc, style: style}
}

// BufWindow 统一入口：renderer / gutter 全部改调这个。
func (w *BufWindow) setCell(x, y int, mainc rune, combc []rune, style tcell.Style) {
    w.sink.SetContent(x, y, mainc, combc, style)
}
```

### 2.3 retarget 改动点（机械替换，行为等价）

把 `screen.SetContent(...)` → `w.setCell(...)`：
- `bufwindow_md.go`：5 处内容区 SetContent + `drawGutterAndLineNumMD` 内部的 SetContent
- `bufwindow.go`：`drawGutter` / `drawDiffGutter` / `drawLineNum` 的 SetContent（native 路径也走，但 `sink` 默认 = realScreenSink，**native 行为字节级不变**）

**关键安全性**：native 路径（非 MD）从不切换 sink，`w.sink` 恒为 realScreenSink。所以这套 retarget 对 micro 原生渲染**零行为影响**，只是多一层方法调用。验证手段：开非 MD 文件，行为应与改动前完全一致（见 §8 验证）。

### 2.4 sink 切换时机

```go
// displayToBuffer 入口
w.sink = w.sb              // 切到离屏
... renderer 跑 ...
w.sink = realScreenSink{}  // 复位（防御性，displayToBuffer 退出前）

// showBuffer：不切 sink（它直接读 sb.rows，不经过 renderer）
```

---

## 3. 核心函数实现

### 3.1 displayToBuffer（render 到 screenBuffer）

```go
// displayToBuffer 从 startLine 渲染到 screenBuffer，单一数据源。
// showCursor: 是否把 cursor 段走 native（按 call site 固定取值，见 §5）。
func (w *BufWindow) displayToBuffer(startLine SLoc, showCursor bool) {
    b := w.Buf
    if w.Height <= 0 || w.Width <= 0 {
        return
    }
    w.ensureMDConfigReady()

    bufWidth := w.bufWidth
    bufHeight := w.bufHeight
    cap := 2 * bufHeight  // 反驳②：2x cap 吸收跨段展开增量

    // 初始化 screenBuffer（容量 2x，行宽含 gutter）
    w.sb.reset(cap, w.gutterOffset+bufWidth, w.X, w.Y)
    w.sb.startLine = startLine

    // 切 sink（§2.4）
    w.sink = w.sb
    defer func() { w.sink = realScreenSink{} }()

    // 可见段（沿用现状 filter）
    visibleStart := startLine.Line
    visibleEnd := visibleStart + cap  // 多渲染一屏余量
    if visibleEnd >= b.LinesNum() {
        visibleEnd = b.LinesNum() - 1
    }
    segments := filterSegmentsToVisible(b.MDSegments, visibleStart, visibleEnd)
    cursors := b.GetCursors()

    vY := 0
    for _, seg := range segments {
        // ★ 写 line 元数据（同源）：见 §3.5
        if showCursor && w.editMode && hasCursorInside(seg, cursors) {
            vY = w.renderSegmentNativeToSB(seg, vY)
        } else {
            vY = w.renderSegmentMDToSB(seg, vY)
        }
        if vY >= cap { break }  // 2x 兜底
    }
    // 剩余不填满（screenBuffer 只渲染实际内容，showBuffer 负责尾部空白填充）
}
```

### 3.2 showBuffer（blit 到真屏）

```go
// showBuffer 把 screenBuffer 从 startLine 起的一段 blit 到真屏。
// 职责单一：copy + 边界处理，不做 dirty 判定（评估报告 §4.1）。
func (w *BufWindow) showBuffer(startLine SLoc) {
    bufHeight := w.bufHeight
    bufWidth := w.bufWidth

    // case B 范围检查（纯算术）：startLine 超出 screenBuffer 覆盖区间 → 补渲染
    if !w.sb.covers(startLine) {
        w.displayToBuffer(startLine, showCursor=false)  // 滚动场景，不关心 cursor 段
    }

    // 找 startLine 在 sb.rows 中的起点 vY
    startVY, ok := w.sb.rowIndexOf(startLine)
    if !ok {
        // startLine 落在装饰行/边界：向下找首个内容行（沿用现状 §修改总结3.3 处理）
        startVY, ok = w.sb.rowIndexNearest(startLine)
        if !ok { return }
    }

    // blit，处理尾部不足
    endVY := startVY + bufHeight
    if endVY > len(w.sb.rows) {
        endVY = len(w.sb.rows)
    }
    for vY := 0; vY < bufHeight; vY++ {
        srcRow := startVY + vY
        if srcRow >= endVY {
            // 尾部空白填充（沿用 displayBufferMD 现状逻辑）
            w.drawGutterAndLineNumMD(vY, -1, false)
            for col := 0; col < bufWidth; col++ {
                screen.SetContent(w.X+w.gutterOffset+col, w.Y+vY, ' ', nil, config.DefStyle)
            }
            continue
        }
        row := &w.sb.rows[srcRow]
        // 整行 blit（gutter + content，§2.2 决定 cells 覆盖全宽）
        for col := 0; col < w.gutterOffset+bufWidth; col++ {
            c := row.cells[col]
            screen.SetContent(w.X+col, w.Y+vY, c.r, c.combc, c.style)
        }
    }

    // cells 用完即弃（评估报告 §2.1 内存优化）
    for i := range w.sb.rows {
        w.sb.rows[i].cells = nil
    }
}
```

### 3.3 relocateVerticalMD 改造（统一 case A/C）

```go
// relocateVerticalMD：入口 displayToBuffer（唯一一次渲染）+ 查询微调。
// case A（方向键）与 case C（goto/search/pageup/pagedown）同一逻辑，
// 唯一区别是渲染起点选择（评估报告 §4.3）。
func (w *BufWindow) relocateVerticalMD(c SLoc, scrollmargin, height int) bool {
    // 1. 估算渲染起点（cheap 预判，渲染前）
    var displayStart SLoc
    if w.sb.coversLine(c.Line) {
        displayStart = w.StartLine             // case A：cursor 在上一帧 screenBuffer 覆盖内
    } else {
        displayStart = SLoc{Line: c.Line - scrollmargin, Row: 0}  // case C：cursor 跳远，1:1 估算
    }

    // 2. 唯一一次渲染（showCursor=true：导航场景，希望看到 cursor 行）
    w.displayToBuffer(displayStart, showCursor=true)

    // 3. 查询微调（非渲染）：让 cursor 落在期望 margin
    cursorRow, ok := w.sb.rowIndexOf(c)
    if !ok {
        // cursor 不在 2x 内（极端：单 segment > 2x viewport）→ native 兜底
        return w.relocateVerticalNativeFallback(c, scrollmargin, height)
    }
    botMarginRow := height - 1 - scrollmargin
    if cursorRow > botMarginRow {
        delta := cursorRow - botMarginRow
        loc, ok := w.sb.slocAt(delta)
        if !ok || loc.Line < 0 {
            // delta 落装饰行：向下找首个内容行（沿用现状）
            for d := delta + 1; d < len(w.sb.rows); d++ {
                if l, ok := w.sb.slocAt(d); ok && l.Line >= 0 { loc = l; break }
            }
            if loc.Line < 0 {
                return w.relocateVerticalNativeFallback(c, scrollmargin, height)
            }
        }
        w.StartLine = loc
        return true
    }
    if cursorRow < scrollmargin {
        w.StartLine = w.Scroll(c, -scrollmargin)  // 向上：段内 1:1（沿用现状）
        return true
    }
    return true
}
```

**case C 为什么一次就够**（评估报告 §4.3）：估算时 cursor 在 `displayStart + scrollmargin`（1:1），渲染后 segment 展开让 cursor 屏行偏移 ±Δ，但 screenBuffer 是 2x，`scrollmargin < bufHeight`，Δ 被 2x 余量吸收 → cursor 必在 sb 内 → 查询微调即可，无需二次渲染。

### 3.4 displayBufferMD → showBuffer（语义翻转）

```go
// bufwindow.go:940 分发点不变，displayBufferMD 改造为薄包装：
func (w *BufWindow) displayBufferMD(editMode bool) {
    // editMode 不再驱动 hasCursorInside（renderer 内部自决），
    // 直接 showBuffer。StartLine 已由上一帧 Relocate 算好。
    w.showBuffer(w.StartLine)
}
```

> 注意：现状 `displayBufferMD(editMode)` 在 `Display()`（bufwindow.go:940）被调用。改造后 editMode 参数由 showBuffer/showCursor 内部消费（见 §5），调用点签名可保留兼容。

### 3.5 line 元数据的同源填充（renderSegment*ToSB）

renderer 写 viewportRowmap 的现状逻辑（`bufwindow_md.go:131-143`）要迁移成「写 sb.rows[vY].line/segRow」。两条路线：

```go
// renderSegmentMDToSB：复制 renderSegmentMD，把两处写入改成同源：
//   原: w.viewportRowmap[vY] = SLoc{Line: row.BufLine, Row: segRow}
//   新: w.sb.setRowMeta(vY, row.BufLine, segRow)   // 同时填 line/segRow
//   原: screen.SetContent(...) → w.setCell(...)（§2 retarget）
// renderer 主体逻辑（cell 样式计算、lineStyles、可见性过滤）零改动。

// renderSegmentNativeToSB：复制 renderSegmentNative，同理：
//   bloc.Y 即 line 元数据来源（bufwindow_md.go:316）
//   viewportRowmap 写入 → sb.setRowMeta
//   screen.SetContent → w.setCell
```

**screenBuffer 辅助方法**：
```go
func (s *screenBuffer) reset(cap, width, ox, oy int) { ... 初始化 rows[cap]，每行 cells[width] ... }
func (s *screenBuffer) setRowMeta(vY, line, segRow int) {
    if vY >= 0 && vY < len(s.rows) {
        s.rows[vY].line = line; s.rows[vY].segRow = segRow
    }
}
func (s *screenBuffer) covers(sl SLoc) bool { ... sl 在 [startLine, startLine+2*bufHeight) ... }
func (s *screenBuffer) coversLine(line int) bool { ... }
func (s *screenBuffer) rowIndexOf(sl SLoc) (int, bool) { ... 线性扫 rows 找 line+segRow 匹配 ... }
func (s *screenBuffer) slocAt(vY int) (SLoc, bool) { ... 从 rows[vY].line/segRow 还原 SLoc ... }
```

---

## 4. viewportRowmap 迁移与删除

### 4.1 读取方迁移

| 现状函数 | 现状读法 | 迁移后 |
|---------|---------|--------|
| `LineToScreenRow(line, row)` | O(bufHeight) 扫 viewportRowmap | O(len(sb.rows)) 扫 sb.rows，签名不变 |
| `ScreenRowToLine(offset)` | viewportRowmap[offset] | sb.rows[offset].line（offset 需转成 sb 内 vY，或保留 viewportRowmap 语义见 §4.2） |
| 点击映射（`LocFromVisual`，bufwindow.go:300） | viewportRowmap[svloc.Y-w.Y] | sb 查询 |

### 4.2 ⚠️ 待决：ScreenRowToLine 的 offset 语义

`ScreenRowToLine(offset)` 现状 offset 是「相对 viewport 顶部的屏行」。B 方案下 viewport 显示内容由 showBuffer 从 sb 的某个 vY blit，offset↔sb.vY 需要一个映射。两个选项：

- **选项 a**：showBuffer 记录 `w.sbBlitStartVY`（本次 blit 的 sb.rows 起始下标），`ScreenRowToLine(offset)` 读 `sb.rows[sbBlitStartVY + offset].line`。简单，但多一个状态字段
- **选项 b**：保留一个轻量 `viewportDisplay []int`（仅 line，showBuffer 时写入，bufHeight 大小）专供点击映射——等于把 viewportRowmap 的「点击用途」留下，只删「relocate 用途」

**推荐选项 a**：`sbBlitStartVY` 与 showBuffer 同源产生（一次赋值），无第二份数据。

### 4.3 删除时机

viewportRowmap 字段在 §5 阶段 4 删除（所有读取方迁移完且验证通过后）。删除前用 git 保留一个 feature flag 可回退。

---

## 5. showCursor 与 editMode 的职责分离（评估报告 §4.4）

showCursor 按 **call site 固定取值**，不沿调用栈传操作类型：

| call site | showCursor | editMode gate |
|-----------|-----------|---------------|
| `relocateVerticalMD` → `displayToBuffer` | **true**（导航：方向键/goto/search/pageup/pagedown） | `editMode and showCursor` → cursor 段 native |
| `showBuffer` 自愈（case B 越界补渲染） | **false**（滚动：鼠标滚/ScrollUp·Down） | cursor 段也 MD（不关心） |

```go
// displayToBuffer 内部 native 分支
if showCursor && w.editMode && hasCursorInside(seg, cursors) {
    // native
} else {
    // MD
}
```

editMode=false（纯阅读）时 `editMode and showCursor` 恒 false，全 MD，与 showCursor 无关。

---

## 6. 实施阶段（建议按此顺序，每阶段可独立验证）

### 阶段 1：数据结构 + sink 基础设施（不接线）
- [ ] 定义 `mdCell / screenRow / screenBuffer / cellSink / realScreenSink`
- [ ] 实现 `screenBuffer.SetContent / reset / setRowMeta / covers / rowIndexOf / slocAt`
- [ ] BufWindow 加 `sb *screenBuffer`、`sink cellSink`（默认 realScreenSink）字段
- [ ] `setCell` 方法
- **验证**：编译通过；非 MD 文件行为不变（sink 恒 realScreenSink）

### 阶段 2：retarget 改造（行为等价）
- [ ] `bufwindow_md.go` 5 处 + drawGutterAndLineNumMD：`screen.SetContent` → `w.setCell`
- [ ] `bufwindow.go` drawGutter/drawDiffGutter/drawLineNum：`screen.SetContent` → `w.setCell`
- **验证**：开 .md 文件，渲染像素级与改动前一致（此时 sink 仍恒真屏，screenBuffer 未启用）

### 阶段 3：displayToBuffer + renderer ToSB 变体（不接线，可单测）
- [ ] `renderSegmentMDToSB` / `renderSegmentNativeToSB`（复制 + retarget + setRowMeta）
- [ ] `displayToBuffer` 主体
- **验证**：写一个临时函数，调 displayToBuffer 后 dump sb.rows，肉眼对比渲染正确性

### 阶段 4：showBuffer + 显示语义翻转（关键里程碑）
- [ ] `showBuffer` 主体（blit + 边界 + cells 释放 + sbBlitStartVY）
- [ ] `displayBufferMD` 改造成 `showBuffer(w.StartLine)` 薄包装
- [ ] 此时每帧：Relocate 仍用旧 viewportRowmap，但 Display 走 showBuffer——**会显示错乱**（因为 sb 没在 Relocate 里填充）。这是预期的中间态
- **验证**：先不接 Relocate，手动验证 showBuffer 对一个固定 sb 的 blit 正确性

### 阶段 5：relocate 接线（统一 case A/C）
- [ ] `relocateVerticalMD` 改造成 §3.3（入口 displayToBuffer + 查询微调）
- [ ] 此时 viewportRowmap 仍存在但不再被 relocate 写入（renderer ToSB 变体写 sb）
- **验证**：★ 跨段光标移动 bug 是否消失（核心目标）；case A/B/C 各场景（见 §8）

### 阶段 6：读取方迁移 + 删除 viewportRowmap
- [ ] `LineToScreenRow` / `ScreenRowToLine` / 点击映射 → sb 查询（§4.2 选项 a）
- [ ] 删除 `viewportRowmap` 字段、renderer 旧 `renderSegmentMD/Native`（保留 ToSB 变体）
- **验证**：全套 §8 回归

### 阶段 7：打磨
- [ ] §4.5 blit 边界边角 case（startLine 落装饰行、尾部不足、softwrap Row 偏移）
- [ ] 内存：确认 cells 释放后 sb 在帧间只占 line/segRow 索引
- [ ] 性能：大文件连续滚动 / Goto 跳跃帧率

---

## 7. 风险与回退

### 7.1 feature flag 回退

建议加一个配置开关（如 `mdrender.strategy = "B" | "legacy"`），legacy 走旧 viewportRowmap 路径。在阶段 5/6 出问题时可一键回退，避免阻塞。删除 viewportRowmap（阶段 6）前必须确认 flag 路径稳定。

### 7.2 主要风险点

| 风险 | 触发 | 缓解 |
|------|------|------|
| retarget 漏改某处 SetContent | displayToBuffer 期间该处写真屏 | 阶段 2 全文 grep `screen.SetContent` 确认无遗漏；displayToBuffer 期间可断言 sink==sb |
| showBuffer blit 边界错（落装饰行） | startLine 恰好是表格 frame 行 | 沿用现状 §修改总结3.3 的「向下找首个内容行」逻辑（已踩过坑） |
| case C 极端（单 segment > 2x viewport） | 超大 codeblock | relocate 内 cursorRow !ok → native fallback（§3.3） |
| sb.rows 2x cap 仍不够 | 极端 softwrap 一行展开很多 | 2x 是启发式；若实测不够，提到 3x（Goto 低频，cap 可放大） |

---

## 8. 验证用例

### 8.1 核心目标：跨段光标移动
- `sample.md` line55-59 表格：cursor 在表格内（native 5 行）按 ↓ 到表格后 → 表格切 MD（9 行，多 4 装饰行）
- **预期**：cursor 平滑跟随，不飞出 viewport，不跳变（现状会因 viewportRowmap 失配跳变）
- 反向同理：从 MD 表格按 ↑ 进入表格内 native

### 8.2 case A（方向键，showCursor=true）
- 上下方向键在 normal/heading/table/codeblock/blockquote/list 各段间移动
- **预期**：每段切换正确增删装饰行，cursor 始终在 scrollmargin 内

### 8.3 case B（滚动，showCursor=false）
- 鼠标滚轮 / ScrollUp·ScrollDown 连续滚动
- **预期**：cursor 可滚出 viewport（不强制拉回）；连续滚动累计越 2x 时 showBuffer 范围检查补渲染无闪烁

### 8.4 case C（导航跳远，showCursor=true）
- Goto 行号、Search 跳匹配、PageUp/PageDown
- **预期**：目标行进 viewport 且在 scrollmargin 内；只渲染一次（无二次渲染闪烁）

### 8.5 softwrap
- 窄窗口下长段落 softwrap，cursor 在 wrap 续行
- **预期**：segRow 元数据正确，垂直滚动 cursor 不丢

### 8.6 非 MD 文件回归（侵入安全）
- 开 .go/.txt 等非 MD 文件
- **预期**：行为字节级与改动前一致（sink 恒 realScreenSink，displayBufferMD 不被调用）

### 8.7 点击映射
- 点击装饰行、内容行、空白区
- **预期**：装饰行/空白回退原始 Scroll 逻辑；内容行定位到正确 buffer 行

---

## 9. 与评估报告的对应

| 评估报告 | 本计划 |
|---------|--------|
| §2.1 screenRow 数据结构 | §1 |
| §2.2 displayToBuffer | §3.1 |
| §2.3 showBuffer + 范围检查 | §3.2 |
| §2.4 relocate 统一 case A/C | §3.3 |
| §2.1 渲染层 retarget | §2（cellSink） |
| §4.1 dirty 标记（已删） | — 本计划无 dirty |
| §4.2 查询地图（screenRow.line） | §1 + §3.5 setRowMeta |
| §4.3 case C 无迭代 | §3.3 注释 |
| §4.4 showCursor 按 call site | §5 |
| §4.5 blit 边界（共性，与 A 同级） | §6 阶段 7 |
| §6 实施清单 6 步 | §6 阶段 1-7（细化） |

---

## 10. 待拍板的开放问题（实现时定）

1. **§4.2 ScreenRowToLine 的 offset 语义**：选项 a（sbBlitStartVY）vs 选项 b（保留轻量 viewportDisplay）——倾向 a
2. **2x cap 是否够**：阶段 7 性能验证后定，不够则 3x
3. **displayToBuffer 的精确停止条件**：现状 renderer 用 `vY >= bufHeight` 停止；B 需渲染到 `cursor 行 + scrollmargin + 一屏余量`，具体算式在 §3.1 实现时打磨（保证 case C cursor 必在 sb 内）
4. **feature flag 命名与默认值**：阶段 5 前定
