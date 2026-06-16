# 方案 B 评估报告（screenBuffer 版本）

> 评估对象：`docs/光标滚动-方案B.md`（displayToBuffer + screenBuffer 路线）
> 关联：`docs/光标滚动-修改总结.md`、`docs/光标滚动-方案A.md`、`docs/方案A架构设计-很臭的方案.md`
> 历史：`docs/方案B-评估报告.md`（旧评估，给出"5-7 倍复杂"结论）已被本文档**部分推翻**
> 立场：用户原以为 A 简单、分析后发现 A 也麻烦 → 倾向 B。本报告回答「精炼后的 B 到底有多复杂」。
> 日期：2026-06-16

---

## 0. 一句话结论

**精炼后的方案 B（screenBuffer 抽象）代码量约 ~170 行，是方案 A（~105 行）的 1.6 倍；主循环不需要改动；案例 A/B/C 分发被 Display 入口的 startLine 比对自然吸收；残留的核心风险只剩 screenBuffer 容量与 blit 边界两个老问题。**

**方案 B 仍然不是"无脑选"，但已经从"灾难性复杂（5-7x）"降到"中等复杂（1.6x）"**——如果用户愿意为架构美感与未来扩展性多写 ~65 行，B 是合理选项；如果只求修 bug，A 仍是性价比最高。

---

## 1. 关键洞察：从旧评估的"B"到新评估的"B"

旧评估（《方案B-评估报告.md》）得出"B 是 A 的 5-7 倍复杂"的结论，依赖三个前提：

| 旧评估前提 | 真实情况 | 新方案 B |
|---|---|---|
| `renderSegmentMD/Native` 要改写逻辑（~80 行改动） | render 函数**主体不动**，只换 SetContent 目标（5 处替换） | 改动量从 ~80 行降到 ~5 行 |
| 主循环要 case A/B/C 分发（侵入 micro.go / actions.go 76 处） | Display 入口自动检测 startLine 变化，case A/B/C 自然收敛 | 主循环**零改动**，actions.go **零改动** |
| 需要 bigViewportMap + viewportRowmap 双数据结构 | 一个 `screenBuffer` 同时承载 cells 和 rowmap | 单数据结构，复杂度降低 |

这三点的澄清来自最近几轮对方案 B 的精炼讨论（**精炼来自用户**，不是我）：
1. `screenBuffer` 与 tcell `screenCell` 结构等价，渲染函数主体可以零改动
2. `bigViewportMap` 与 `viewportRowmap` 被 `screenBuffer` 吸收，不需要双层抽象
3. case A/B/C 分发通过 Display 入口的 lazy render 收敛，主循环与 actions.go 不动

**新方案 B 的本质**：把"渲染数据从 tcell screen 搬到二维数组"，其余保持现状。

---

## 2. 新方案 B 的核心设计

### 2.1 screenBuffer 数据结构

```go
type screenCell struct {
    r     rune
    combc []rune
    style tcell.Style
}

// screenBuffer 是单一渲染缓存：
//   - cells：渲染好的二维 [vY][vX] 屏幕字符（供 ShowToScreen blit）
//   - rowmap：每个屏行对应的 buffer 位置（替代现有 viewportRowmap，供 Relocate 查询）
//   - startLine：渲染时的起点 buffer 行号（用于 rowmap 对齐 + blit 起点计算）
//   - cap：cells 已分配的物理行数（用于决定是否需要扩容）
//   - overflow：2×bufHeight 兜底触发的标记（说明 Relocate 可能不够用）
type screenBuffer struct {
    cells     [][]screenCell
    rowmap    []SLoc
    startLine int
    cap       int
    overflow  bool
    cursorX   int     // ShowToScreen 时调 screen.ShowCursor
    cursorY   int
    bufWidth  int
}
```

**为什么一个数据结构够用**：
- `cells` 是渲染输出（供显示）
- `rowmap` 是几何信息（供 Relocate 查询 cursorRow）
- 两者在同一渲染过程中同步生成，自然一致

**对比旧评估的"bigViewportMap + viewportRowmap 双结构"**：单结构更简单、内存布局更紧凑（cells 和 rowmap 的第 i 行描述同一屏行的渲染结果与几何位置）。

### 2.2 时序：Display / Relocate 入口的 lazy render

**核心承诺**：决策（Relocate）前永远有一份 fresh 的 screenBuffer，从根本上消灭「map 落后一帧」的时序断层。

**实现路径**（主循环零改动）：

```
// cmd/micro/micro.go 主循环（不变）：
for {
    Display()       ← 由 BufWindow.Display 调度
    event = ...
    HandleEvent()  ← 可能改 StartLine / cursor，可能调 Relocate
}

// BufWindow.Display（displayBufferMD 入口）：
func (w *BufWindow) Display() {
    if !w.Buf.IsMD {
        w.displayBuffer()  // 原生路径
        return
    }
    // MD 路径
    if w.screenBuf == nil ||
       w.screenBuf.startLine != w.StartLine.Line {
        w.displayToBuffer(w.screenBuf, w.editMode)
    }
    w.screenBuf.ShowToScreen(w)
}

// BufWindow.Relocate 入口（bufwindow.go IsMD 分支）：
func (w *BufWindow) Relocate() bool {
    if w.Buf.IsMD {
        // ★ 关键：Relocate 决策前先渲染一份 fresh rowmap
        if w.screenBuf == nil || w.screenBuf.startLine != w.StartLine.Line {
            w.displayToBuffer(w.screenBuf, w.editMode)
        }
        ret = w.relocateVerticalMD(c, scrollmargin, height)
    } else {
        // 原生 Relocate（不变）
    }
}
```

### 2.3 三种事件路径的统一处理

方案 B 的最终 API：

```go
// 渲染：永远生成完整的 screenBuffer
func (w *BufWindow) displayToBuffer(buf *screenBuffer, startLine int, editMode bool) {
    buf.startLine = startLine
    // 渲染完整 screenBuffer，覆盖 [startLine, startLine + 2×bufHeight]
    // cells + rowmap 同步生成
}

// blit：纯粹从 startLine 开始 blit，然后清空 cells
func (buf *screenBuffer) showBuffer(w *BufWindow, startLine int) {
    screenStartRow := startLine - buf.startLine
    // blit cells[screenStartRow..screenStartRow+bufHeight] 到 viewport
    buf.cells = nil  // 清空所有 cell（rowmap 保留）
}
```

**三种 case 的统一处理**：

| 场景 | displayToBuffer 用什么 startLine | showBuffer 用什么 startLine |
|---|---|---|
| **case A**：编辑/光标移动（跨段） | 当前 `w.StartLine.Line` | Relocate 精算后的 `realStartLine`（在 cells 范围内 → 复用）|
| **case B**：纯视口（scroll/page/center/start/end） | 新的 `w.StartLine.Line`（事件后）| 同一 `w.StartLine.Line` |
| **case C**：goto/search 大幅移动 | **估算的 startLine1**（1:1 估算） | Relocate 精算后的 `realStartLine`（在 cells 范围内 → 复用）|

**完整流程示例**：

```go
// case A（跨段移动）：
MoveCursor → Relocate:
    displayToBuffer(buf, w.StartLine.Line, w.editMode)  // 当前 StartLine 渲染
    relocateVerticalMD(buf, ...)                          // 精算 → realStartLine
    showBuffer(buf, realStartLine)                        // 复用 cells
    w.StartLine = realStartLine

// case B（纯滚动）：
HandleEvent 改 w.StartLine → 下一帧 Display:
    displayToBuffer(buf, w.StartLine.Line, w.editMode)  // 新 StartLine 渲染
    showBuffer(buf, w.StartLine.Line)                    // 从 0 开始 blit

// case C（Goto/Search）：
startLine1 = c.Line - bufHeight/2                       // 1:1 估算
displayToBuffer(buf, startLine1, w.editMode)             // 估算 startLine 渲染
relocateVerticalMD(buf, ...)                             // 精算 → realStartLine
showBuffer(buf, realStartLine)                           // 复用 cells（realStartLine 在范围内）
w.StartLine = realStartLine
```

**关键洞察**：

1. **cells 复用窗口 = 单次 Display 调用内**：
   - displayToBuffer(估算 startLine1) → cells 覆盖 [startLine1, startLine1 + N]
   - Relocate 精算 → realStartLine 在 cells 范围内
   - showBuffer(realStartLine) → 从 cells 对应位置 blit（**复用估算渲染的 cells**）

2. **跨 Display 调用 = 永远重新渲染**：
   - 下次 Display 时 cells=nil（showBuffer 已清空）
   - 必须重新 displayToBuffer
   - rowmap 保留供下次 Relocate 决策

3. **cells 是临时资源，用完就扔**：
   - showBuffer 完成后 cells = nil
   - 不存在"cells 是否可复用"的判断
   - 不存在"cells 释放时机"的纠结

4. **主循环零改动**：
   - 所有 case 都收敛在 BufWindow 内部
   - 主循环只调 Display，不关心 case 分发

### 2.4 displayToBuffer 的停止条件

```go
func (w *BufWindow) displayToBuffer(buf *screenBuffer, startLine int, editMode bool) {
    buf.startLine = startLine
    buf.cells = buf.cells[:0]
    buf.rowmap = buf.rowmap[:0]

    line := buf.startLine
    vY := 0
    cursorDone := false
    oldSeg := findSegmentContaining(w.prevCursorY)
    oldSegDone := (oldSeg == nil)
    cursorSegEndVY := -1

    for {
        // 兜底：2×bufHeight 强制退出，防止内存/时间爆炸
        if vY >= 2*w.bufHeight {
            buf.overflow = true
            break
        }

        seg := findSegmentContaining(line)
        if seg == nil { break }

        if editMode && hasCursorInside(seg, w.Buf.GetCursors()) {
            vY = w.renderSegmentNative(seg, vY, buf)
            cursorDone = true
            cursorSegEndVY = vY
        } else {
            vY = w.renderSegmentMD(seg, vY, buf)
        }
        line = seg.BufEndLine + 1

        // 三目的（与方案 A preRender 相同）
        canBreak := true
        if !cursorDone { canBreak = false }
        if !oldSegDone && seg != oldSeg { canBreak = false }
        if vY < w.bufHeight { canBreak = false }

        if canBreak { break }
    }
}
```

**与方案 A preRender 的关系**：停止条件逻辑几乎一致（都是"目的驱动"）。差异：
- 方案 A preRender：dry-run，**只写 rowmap**，cells 用临时占位
- 方案 B displayToBuffer：正常渲染，**同时写 rowmap 和 cells**

**2×bufHeight 兜底的合理性**：
- 通常 Relocate 改 StartLine 不会超过 bufHeight
- 跨段时光标在 segment 末尾，最大 delta ≈ maxSegmentLength
- 2×bufHeight ≈ 100 行（bufHeight=50），覆盖典型 maxSegmentLength（最大单 segment 通常 < 100 行）
- 超过兜底时设 overflow=true，ShowToScreen/Relocate 可降级到 nativeFallback

**startLine 参数化的关键**：
- displayToBuffer 接受任意 startLine，不一定是当前 `w.StartLine.Line`
- case C 用估算的 startLine1（可能与最终 realStartLine 不同）
- realStartLine 在 [startLine1, startLine1 + 2×bufHeight] 范围内时，cells 复用

---

## 3. 改动清单（精确到行）

### 3.1 文件级总览

| 文件 | 改动类型 | 改动量 |
|---|---|---|
| `internal/display/bufwindow_md.go` | 核心改动 | ~150 行新增/改 |
| `internal/display/bufwindow.go` | 微调（字段 + 1 行更新） | ~10 行 |
| `internal/md/render_*.go` | **零改动** | 0 |
| `internal/md/md.go` / `detect.go` | **零改动** | 0 |
| `internal/action/actions.go` | **零改动** | 0 |
| `internal/action/bufpane.go` / `bufpane_md.go` | **零改动** | 0 |
| `internal/buffer/buffer.go` | **零改动** | 0 |
| `internal/display/softwrap.go` | **零改动** | 0 |
| `cmd/micro/micro.go` 主循环 | **零改动** | 0 |

**总侵入**：~153 行，全在 display 包内。

### 3.2 详细代码量盘点

| 组件 | 行数 | 说明 |
|---|---|---|
| `screenBuffer` + `screenCell` 数据结构 | ~25 | types + SetCell/ShowCursor 方法 |
| `newScreenBuffer` / 容量管理 | ~10 | 懒分配 + 2×bufHeight capacity |
| `displayToBuffer(startLine, editMode)` | ~50 | 三目的停止 + while 循环 + startLine 参数化 |
| `showBuffer(startLine)` | ~15 | blit + cells=nil |
| `Display` 入口（displayBufferMD） | ~20 | displayToBuffer + showBuffer 调度 |
| `Relocate` 入口的 displayToBuffer + 精算 | ~25 | 入口加 displayToBuffer + rowmap 查表 |
| `renderSegmentNative` 改动 | ~5 | 1 处 SetContent 替换 |
| `renderSegmentMD` 改动 | ~5 | 1 处 SetContent 替换 |
| `drawGutterAndLineNumMD` 改动 | ~3 | 1 处 SetContent 替换 |
| 字段调整（`bufwindow.go`） | ~5 | `viewportRowmap` → `screenBuf` + `prevCursorY` |
| **总计** | **~153 行** | |

**对比之前的方案 B**：
- 默认方案 B：~168 行
- 默认 + cells 复用窗口优化：~208 行
- **最终方案 B（cells 临时 + 单次复用）：~153 行**

代码量比方案 A（~105 行）多 ~48 行（1.45x）。

**对比方案 A 的 ~105 行**：方案 B 多 ~63 行（1.6x），全部是 screenBuffer 这个数据结构的成本。

### 3.3 renderSegmentNative 主体真的不动吗？

**是的**。现状 `renderSegmentNative`（~485 行，bufwindow_md.go:210-694）是从 micro 原生 `displayBuffer` 复制出来的庞然大物，里面大量 `screen.SetContent` 调用。

**但 screen.SetContent 调用在 renderSegmentNative 主体中只有 1 处**（`bufwindow_md.go:520`）。这是因为大部分 SetContent 调用被抽取到了内部辅助函数 `draw()` 里。

**精确盘点**：

| 位置 | 函数 | 改动 |
|---|---|---|
| `bufwindow_md.go:195` | `renderSegmentMD` 主循环 | 1 处 SetContent → `buf.SetCell` |
| `bufwindow_md.go:520` | `renderSegmentNative` 主循环 | 1 处 SetContent → `buf.SetCell` |
| `bufwindow_md.go:667` | `renderSegmentNative` 行尾填充 | 1 处 SetContent → `buf.SetCell` |
| `bufwindow_md.go:751` | `displayBufferMD` 末尾填充 | 1 处 SetContent → `buf.SetCell` |
| `bufwindow_md.go:813` | `drawGutterAndLineNumMD` | 1 处 SetContent → `buf.SetCell` |

**总计 5 处替换**，函数主体逻辑（颜色、tab、wrap、selection、cursorline、matchbrace 等）**100% 不动**。

**与方案 A 的对比**：
- 方案 A：`renderSegmentNative` 主体不动，新增 ~15 行的 `renderSegmentNativeDryRun` 复用 `getRowCount`
- 方案 B：`renderSegmentNative` 主体改 1 行 SetContent，复用现有渲染逻辑

方案 B 在 native 渲染路径上**比方案 A 更轻**——不需要单独的 dry-run 函数，因为整个 native 渲染天然就是"写一次到目标"。

---

## 4. 与方案 A 的对比（最终）

| 维度 | 方案 A | 方案 B（最终） |
|---|---|---|
| **代码量** | ~105 行 | **~153 行**（1.45x） |
| **新增数据结构** | 0（复用 viewportRowmap） | `screenBuffer`（cells + rowmap 合并） |
| **侵入文件数** | 2（display 包） | 2（display 包） |
| **主循环改动** | 0 | 0 |
| **actions.go 改动** | 0 | 0 |
| **md 包改动** | 0 | 0 |
| **stale map 风险** | ⚠️ 仍存在（preRender 触发后立即决策） | ❌ **完全消除**（Relocate 入口永远 fresh） |
| **跨段光标消失帧** | ✅ preRender 修复 | ✅ fresh render 修复 |
| **每帧渲染量** | O(bufHeight) + 跨段时再 O(bufHeight) | **O(bufHeight)**（cells 复用窗口在 Relocate 精算后复用）|
| **cells 渲染缓存（临时）** | 不存在（直接写屏） | ~448 KB/active window（showBuffer 后 nil） |
| **未来扩展性** | 局部修复 | **架构级**（统一渲染抽象，支撑更多 MD 增强） |
| **代码可读性** | 触发条件需理解 preRender 三目的 | 单一 lazy render 路径，逻辑更线性 |
| **维护成本（长期）** | preRender 触发条件需要持续维护 | screenBuffer 是统一抽象，新 MD 增强直接复用 |

---

## 5. 残留风险与边界条件

### 5.1 screenBuffer 容量兜底（残留缺陷①）

旧评估指出的致命缺陷：screenBuffer 尺寸不够，ShowToBuffer blit 时可能越界。

**2×bufHeight 兜底缓解方案**：
- 2×bufHeight ≈ 100 行（bufHeight=50），覆盖典型 Relocate 调整范围
- 超过兜底时设 `overflow=true`
- ShowToScreen 遇到 overflow → 走 nativeFallback（用原生 displayBuffer 兜底）
- Relocate 遇到 overflow → 用 nativeFallback 算 StartLine

**仍未根治的场景**：
- 极大单 segment（如长代码块 > 100 行）跨段时，screenBuffer 可能不够
- 极端的 case C（goto 到很远的位置）：screenBuffer 覆盖不到新位置

**缓解**：
- overflow=true 触发降级到 nativeFallback（fallback 不崩，但 scroll 估算不准）
- 用户按 ↓/↑ 时，下一帧 Display 检测到 startLine 变化，重新 displayToBuffer 到正确位置
- **本质是"偶尔 1 帧不完美，下一帧自纠正"**——与方案 A 的 nativeFallback 兜底策略一致

**对比方案 A 的处理**：
- 方案 A：viewportRowmap 是 bufHeight 大小，超出范围走 nativeFallback
- 方案 B：screenBuffer 是 2×bufHeight 大小，超出范围走 nativeFallback
- 两者行为一致，方案 B 兜底范围稍大

### 5.2 blit 边界（残留缺陷③）

旧评估指出的"blit 边界 = bug 高发区"。新方案 B 也有类似问题：

**边界场景**：
- `newStartLine` 落装饰行（应跳到首个内容行）
- screenBuffer 尺寸不足（缺陷①）
- 空白填充（buffer 内容不够填满 viewport）
- softwrap 下 `newStartLine.Row ≠ 0` 的偏移

**复杂度评估**：
- ShowToScreen 的 blit 逻辑本身 ~20 行（双层 for 循环）
- 边界处理集中在 `overflow` 标志 + `screenStartRow` 计算上
- 与方案 A 的 `relocateVerticalMD` 五分支复杂度相当（同样要处理装饰行、空白、delta 超范围）

**缓解**：
- screenStartRow 计算集中在 ShowToScreen 入口（约 5 行）
- overflow 检查分散到 ShowToScreen 与 Relocate 两个函数
- **总边界分支数比方案 A 略多**（方案 A 集中在 Relocate 一个函数）

### 5.3 多 BufWindow 内存开销

**重要**：viewportRowmap 与 screenBuffer **不是同一抽象层次**，不能直接对比倍数。本节澄清真实成本。

| 数据结构 | 抽象层次 | 方案 A | 方案 B（默认）| 方案 B（cells 释放优化）|
|---|---|---|---|---|
| 几何元数据（rowmap） | 屏行↔buffer 行的位置映射 | `viewportRowmap[bufHeight] ≈ 800B` | `screenBuffer.rowmap[2×bufHeight] ≈ 1600B` | `screenBuffer.rowmap[2×bufHeight] ≈ 1600B` |
| 渲染内容缓存 | 渲染好的字符（rune + style + combc） | **不存在** | `screenBuffer.cells[2×bufHeight][bufWidth] ≈ 448KB`（长期保留） | `screenBuffer.cells ≈ 448KB`（仅 active window 保留） |

#### 默认方案 B（cells 长期保留）

- 单 BufWindow：~450 KB
- 2 个 BufWindow：~900 KB
- 4 个 BufWindow（micro 默认）：**~1.8 MB**

#### 优化版方案 B（cells 按需释放）

**关键洞察**：用户只与一个 window 交互，任意时刻只有一个 active window 需要 cells。

- 单 BufWindow（active）：~450 KB
- 单 BufWindow（non-active）：~1.6 KB（只剩 rowmap）
- 4 BufWindow（1 active）：**~450 KB**（1 个 cells + 3 个 rowmap）
- 切换 window 时：旧 active 释放 + 新 active 填充 = **~450 KB**

**cells 释放/重建的时序**：

```
displayToBuffer(buf, editMode) 时：
    if buf.cells == nil {
        buf.cells = make([][]screenCell, ...)  // cells 重新分配
    }
    // 渲染到 cells

ShowBuffer(buf) 时：
    // 复制 cells 到真实屏幕

ShowBuffer 完成后：
    buf.cells = nil  // 立即释放，保留 rowmap
```

**边界场景分析**：

| 场景 | 行为 | 副作用 |
|---|---|---|
| window 切换 | 旧 active 释放 cells；新 active 触发 displayToBuffer | 1 帧延迟（< 1ms），无屏闪 |
| editMode toggle | cells 必须重新渲染（native vs MD 内容不同） | 自然触发 displayToBuffer |
| buffer 编辑 | MDSegments 重算，rowmap/cells 都失效 | 自然触发 displayToBuffer |
| Display 重复触发 | cells=nil 时重新渲染；cells 存在时直接 blit | 性能略低于原生（多一次内存拷贝）|

**代码复杂度**：+5-10 行（cells 释放/重建的状态机）。

**对实际使用的影响**：
- 优化后任意时刻 cells 内存 ≈ 450 KB（单 active window）
- 4 BufWindow 总开销 ~450 KB（不是 ~1.8 MB）
- rowmap ~1600 B 可忽略

**为什么说"cells 是按需资源"而不是"cells 是基石成本"**：

- 默认方案 B 的论断："cells 缓存是基石，无法避免" — **错误**
- 实际：cells 是 ShowBuffer 的临时存储，blit 完就可以释放
- 类比：tcell 的内部 framebuffer 也是临时 blit 后可丢弃
- **正确的论断**：方案 B 引入一份**临时**渲染缓存（450 KB），不是永久基石

**对方案 B vs 方案 A 的内存结论**：
- 优化前：方案 B 比方案 A 多 ~1.8 MB（4 窗口）
- 优化后：方案 B 比方案 A 多 ~450 KB（任意时刻，与窗口数无关）
- **差距从"百倍级别"降到"450 KB 量级"**

**仍然存在的差距**：
- 450 KB cells 拷贝 vs 800 B rowmap
- 绝对值差异 ~449 KB（一个 active window 的 cells 渲染缓存）
- 但这是按需资源，不是永久占用

### 5.4 screenBuffer 与 micro 原生 displayBuffer 的同步

micro 原生 `displayBuffer` 写屏会调 `screen.SetContent` —— 这部分仍然写屏。方案 B 的 screenBuffer 不接管原生 displayBuffer。

**风险**：micro 原生路径（BufWindow.displayBuffer）仍然直接写屏，不走 screenBuffer。如果未来需要在原生路径上做 blit 优化，需要额外工作。

**当前评估**：原生路径不涉及方案 B 的核心问题（stale map），保持不动即可。

---

## 6. 实施步骤

### Commit 1：核心数据结构 + 渲染函数改造

1. `internal/display/bufwindow_md.go`：
   - 新增 `screenCell` / `screenBuffer` 类型
   - 新增 `SetCell` / `ShowCursor` 方法
   - 新增 `newScreenBuffer` 构造函数（懒分配 + 2×bufHeight capacity）
   - `renderSegmentMD` 加 `buf *screenBuffer` 参数，1 处 SetContent 替换
   - `renderSegmentNative` 加 `buf *screenBuffer` 参数，2 处 SetContent 替换
   - `drawGutterAndLineNumMD` 加 `buf` 参数，1 处 SetContent 替换
2. **验证**：运行现有 MD 文件，肉眼对比渲染结果应与现状一致（screenBuffer 内容最终 blit 到 screen）

### Commit 2：调度器与 Relocate 集成

1. `internal/display/bufwindow_md.go`：
   - 新增 `displayToBuffer` 函数（三目的停止条件 + while 循环 + 2×bufHeight 兜底）
   - 新增 `ShowToScreen` 函数（blit 逻辑 + overflow 处理）
   - 新增 `findSegmentContaining` 函数
   - `relocateVerticalMD` 入口加 `displayToBuffer` 调用（fresh rowmap）
2. `internal/display/bufwindow_md.go`：
   - `displayBufferMD` 重写为调度器（displayToBuffer + ShowToScreen）
3. `internal/display/bufwindow.go`：
   - `BufWindow` 字段：`viewportRowmap` → `screenBuf`，新增 `prevCursorY`
   - `relocateVerticalMD` 入口的 displayToBuffer 调用（如果选在 Relocate 入口而非 displayBufferMD）
4. **验证**：跨段光标移动（v1.0.5 已知限制场景），观察是否修复

### Commit 3：测试补充

1. `internal/display/bufwindow_md_test.go`：
   - `screenBuffer` SetCell / ShowToScreen 单元测试
   - `displayToBuffer` 三目的停止条件测试
   - 跨段场景：cursor 移动后 Relocate 决策正确
   - overflow 场景：超过 2×bufHeight 时降级到 nativeFallback

---

## 7. 决策建议

### 7.1 选方案 A 的场景

- 项目目标**只是修这个 bug**，不打算大幅扩展
- 内存敏感（多 BufWindow、低端设备）
- 时间紧、想快速验证修复效果
- 倾向"最小侵入、聚焦修复"

### 7.2 选方案 B 的场景

- 项目会持续扩展更多 MD 增强（语法高亮、diff 渲染、inline 增强等）
- 倾向"架构级重构、为未来打基础"
- 接受多 BufWindow 内存开销
- 不希望 Relocate 入口有"prevSeg != newSeg 触发条件"这种 hack，希望用统一抽象彻底解决

### 7.3 折中建议：先 A 后 B

如果不确定，先实施方案 A（~105 行，1-2 天）：
- 解决已知 bug
- 验证 v1.0.5 限制确实被修复
- 评估实际效果

如果 A 实施后发现：
- 仍有边缘场景无法覆盖 → 升级到 B
- 修复彻底，但代码组织混乱 → 评估 B 的架构价值
- 修复彻底且代码清晰 → 停在 A

**这样既验证修复效果，又保留向 B 升级的路径**。

---

## 8. 与旧评估的差异总结

| 维度 | 旧评估（《方案B-评估报告.md》）| 新评估（本文） |
|---|---|---|
| 代码量差距 | 5-7x | **1.6x** |
| 缺陷① screenBuffer 尺寸 | 致命（无解）| 残留（2×bufHeight 兜底缓解）|
| 缺陷② 主循环分发 | 严重（要改 actions.go 76 处 + micro.go）| **完全消除**（lazy render 收敛）|
| 缺陷③ blit 边界 | 严重（bug 高发区）| 残留（~20 行集中处理）|
| 总评 | 坚持方案 A | **方案 B 重新成为合理选项** |

旧评估基于未精炼的方案 B 文档（displayToBuffer + screenBuffer + 主循环分发），新评估基于精炼后的方案 B（screenBuffer 抽象 + lazy render）。精炼来自用户对设计的多轮追问。

**核心教训**：方案 B 的原始文档把"主循环分发 + bigViewportMap 双结构 + render 函数大改"等难点藏起来了，看起来短但实现重。展开讨论后真正必要的改动只是"渲染数据从 tcell 搬到二维数组"——这是用户洞察的价值。

---

## 9. 关键代码事实（评估依据）

| 事实 | 位置 | 对评估的意义 |
|---|---|---|
| `screen.SetContent` 内部就一行调 tcell | `internal/screen/screen.go:116` | screenBuffer 抽象成本极低 |
| `screenBuffer` 字段需求 = `screenCell` 字段 | 设计推演 | render 函数主体不动 |
| actions.go 有 76 处 `h.Relocate()` | `internal/action/actions.go` | 旧评估的"76 处侵入"已被 lazy render 消除 |
| micro 主循环 DoEvent 时序固定 | `cmd/micro/micro.go:486-498` | 任何方案都不能改这个时序，但可以在 BufWindow 内部收敛 |
| ScrollUp/Down 直接改 StartLine 不调 Relocate | `actions.go:25-37` | case B 不需要 Relocate，Display 入口自动处理 |
| viewportRowmap 已存在 | `bufwindow.go:42` | 现状有 rowmap 抽象基础，方案 B 把它扩展成 screenBuffer 自然 |
| renderSegmentNative ~485 行 | `bufwindow_md.go:210-694` | 主体不动，只改 1 行 SetContent |
| getRowCount 已封装 softwrap 几何 | `softwrap.go:226` | 方案 A 用，方案 B 不需要（直接复用 native 渲染）|

---

## 10. 关联文档索引

- `docs/光标滚动-方案B.md` —— 原始问题描述与方案 B 设想
- `docs/光标滚动-方案A.md` —— 原始问题描述与方案 A 设想
- `docs/光标滚动-修改总结.md` §5 —— v1.0.5 已知限制（方案 B 直接对应）
- `docs/方案A架构设计-很臭的方案.md` —— 方案 A 完整设计
- `docs/方案B-评估报告.md` —— 旧评估（5-7x 复杂），本文档部分推翻