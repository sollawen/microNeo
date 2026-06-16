# 光标滚动 — 方案A (preRender) 架构设计

> 关联文档：
> - 问题与思路：`docs/光标滚动-方案A.md`、`docs/光标滚动-方案B.md`
> - 历史方案总结：`docs/光标滚动-修改总结.md`
> - 评估记录：`docs/方案A架构设计评估.md`
>
> 本文档是**方案A**（preRender）的架构设计与实施蓝图，目标：**彻底修复"跨段切换时光标飞出 viewport 底部"的 bug**。
>
> **v2 重大修订（2026-06-16）**：v1 引入的 `editModeDirty` / `lastCursorSegIdx` / `mapPossiblyStale` 等 dirty tracking 机制经 CTO 反思后判定为**过度工程**——通过严格分析事件调用顺序（`bufpane.go:443-532`）可知，`observeEditModeToggle` 永远在 `Relocate` 之后才运行，因此 editMode 永远不会在 Relocate 看到不一致的 map。v2 采用更简洁的"**无条件 preRender**"架构，并删除 4 个字段 + 1 个函数。详见 §16。

---

## 0. TL;DR

在 `Relocate` 的 MD 分支中，**无条件**做一次 dry-run 渲染，写入 `viewportRowmap`，然后用既有的 `relocateVerticalMD` 做 scroll 判定。改动集中在 `internal/display/bufwindow_md.go`，对 micro 原生代码零侵入，对现有 renderer 最小侵入。

**v1 vs v2 核心差异**：

| 维度 | v1（已废弃）| v2 |
|------|------------|-----|
| 新增字段 | 4 个（`preRowmap`, `preRowmapValid`, `editModeDirty`, `lastCursorSegIdx`）| **0 个** |
| 新增函数 | `mapPossiblyStale`, `segmentOfLine` | **0 个**（只保留必须的 `preRenderRowmap`）|
| 触发条件 | 段归属变化 OR editModeDirty | **无条件** |
| Relocate 入口改动 | 4-5 行（含 dirty 消费 + reset）| **1 行**：`w.preRenderRowmap(c)` |
| 正确性论证 | 复杂（要论证 dirty 时序、缓存安全、editMode 窗口...）| **简单**：Relocate 看到的一定是陈旧的，无条件刷新 |

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
按键时序（bufpane.go:443-532）:
  ① DoKeyEvent / DoMouseEvent / paste  → 内部调 MoveCursor + Relocate
     ↑ 此时 editMode 还是旧值（observeEditModeToggle 还没跑）
     ↑ viewportRowmap 还是上一帧 Display 建的（基于旧 editMode）
     ↑ 两者天然一致！
  ② observeEditModeToggle  → 调 SetEditMode  ← 此刻 editMode 才变
  ③ Display()  → 基于新段归属 + 新 editMode 重建 viewportRowmap
```

**关键观察（CTO 反思核心）**：editMode 变更**永远发生在 Relocate 之后**（事件处理函数末尾，第 532 行），所以 Relocate 看到的 editMode 和 viewportRowmap 永远来自同一时刻 —— **根本不需要 editModeDirty 追踪**。

唯一真正的"map 陈旧"来源：**光标所属 segment 变化**（因为新段会改变 `renderSegmentMD/Native` 的输出形状）。

---

## 3. 解决思路

**核心思想**：不试图判断"map 是否陈旧"，直接承认"Relocate 时刻 map 一定陈旧"，**无条件**做一次 dry-run 渲染，用新 map 做 scroll 判定。

为什么不直接调用 `Display()`？因为：
1. `Display()` 会真的写屏（`screen.SetContent`），造成重复渲染和闪烁
2. `Display()` 依赖 `w.StartLine`，而 `Relocate` 正是要算新的 `w.StartLine` —— 循环依赖

因此引入 **dry-run 渲染**：复用 `renderSegmentMD` / `renderSegmentNative` 的 map 构建逻辑，但跳过所有 `screen.SetContent`，把产物写入 `viewportRowmap`。

---

## 4. 核心数据结构

### 4.1 字段（BufWindow，v2 简化后**零新增**）

```go
// internal/display/bufwindow.go

type BufWindow struct {
    // ... 原有字段不变 ...

    // MicroNeo: MD 渲染支持（现有字段，行为微调）
    viewportRowmap []SLoc  // Display 每帧重建；v2 新增：preRenderRowmap 在 Relocate 入口也会刷新它
    editMode bool          // 光标所在 segment 回退原生显示
}
```

**v2 删除的字段**（v1 设计但 v2 证明不需要）：
- ❌ `preRowmap`：直接复用 `viewportRowmap`
- ❌ `preRowmapValid`：preRender 总是成功或清空，不需要 validity 标志
- ❌ `editModeDirty`：editMode 变更不会影响 Relocate 看到的 map（见 §2 时序分析）
- ❌ `lastCursorSegIdx`：不需要缓存"上一帧的段归属"——v2 不做"段是否变化"判定

### 4.2 为什么不需要独立 preRowmap

- `preRenderRowmap` 在 Relocate 入口执行，紧接其后的 `relocateVerticalMD` 立即消费
- Display 紧随 Relocate 之后调用，**会重建** `viewportRowmap`
- 中间窗口期内 `viewportRowmap` 已反映"**新段归属 + 旧 StartLine**"，对 `LocFromVisual`（鼠标点击映射）反而比 v1 的"旧段归属 + 旧 StartLine"**对 cursor 的 segment 更准确**

### 4.3 为什么不新增类型

`viewportRowmap` 复用既有的 `[]SLoc`（`Line=-1` 装饰行，`Line=-2` 空白，`Line>=0` 内容行 + `Row` 段内序号）。下游 `relocateVerticalMD` / `LineToScreenRow` 仅需把 `w.viewportRowmap` 引用替换为参数化的 map，逻辑零变化。

---

## 5. 触发条件设计

**v2 没有触发条件**。

`Relocate` 的 MD 分支无条件调用 `preRenderRowmap(c)`，再调用 `relocateVerticalMD(c, ...)`：

```go
if w.Buf.IsMD {
    w.preRenderRowmap(c)                          // ← 无条件
    ret = w.relocateVerticalMD(c, scrollmargin, height)
} else {
    // ... 原生分支不变 ...
}
```

**为什么不需要判定"什么时候陈旧"**：
- Relocate 看到的 `viewportRowmap` 永远是**上一帧** Display 建的——这是事实，无法改变
- preRender 的代价：单行段 < 0.1ms，多行段 < 1ms
- 60 次按键/分钟 × 1ms = 60ms/分钟，**可忽略**

---

## 6. dry-run 路径设计

### 6.1 目标

把 `renderSegmentMD` / `renderSegmentNative` 中"写 viewportRowmap"的逻辑与"写屏"逻辑解耦，新增 `target []SLoc, dryRun bool` 参数。`dryRun=true` 时：
- ✅ 写入 `target`（v2 中即 `viewportRowmap`）
- ✅ 计算 `newVY`（行布局必须算对）
- ❌ 不调用 `screen.SetContent`
- ❌ 不调用 `drawGutterAndLineNumMD` / `drawGutter` / `drawLineNum`
- ❌ 不调用 `w.showCursor`
- ❌ **不修改 buffer 状态副作用**（见 §6.3）

### 6.2 改造方式

#### 改动 1：render 函数签名增加参数

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
- `preRenderRowmap`（dry-run 路径）调用时传 `(w.viewportRowmap, true)`

#### 改动 2：写屏语句加 dryRun 守卫

```go
if !dryRun {
    screen.SetContent(screenX, screenY, cell.Rune, cell.Combining, style)
}
```

`renderSegmentMD` 中 `drawGutterAndLineNumMD` 调用同理加守卫（见 §6.4）。

**写 map 语句不受 dryRun 影响**（dry-run 也要写 map，这正是它的目的）。

#### 6.2.1 改动 3：filterSegmentsToVisible 复制 segs

`filterSegmentsToVisible` 当前原地修改 `s.VisibleStart/VisibleEnd`（`bufwindow_md.go:773-779`）。v1 中 preRender 复用同一函数可依赖"Display 会再调一次"的兜底语义；v2 中 preRender 是 Relocate 的必经路径，需要更清晰的语义：

```go
// preRenderRowmap 内：先复制 segs 再过滤
segmentsCopy := make([]md.Segment, len(b.MDSegments))
copy(segmentsCopy, b.MDSegments)
segments := filterSegmentsToVisible(segmentsCopy, visibleStart, visibleEnd)
```

或更彻底：抽出一个不修改原 segs 的 `filterSegmentsToVisibleCopy`。Commit 1 实施时决定。

### 6.3 状态副作用清单（dryRun 必须跳过的副作用）

渲染函数入口处隐藏 buffer 状态修改，dryRun=true 时**必须跳过**：

| 位置 | 副作用 | dryRun 行为 |
|------|--------|-------------|
| `renderSegmentNative:238-243` | `b.UpdateDiff()` + `b.ModifiedThisFrame = false` | 跳过整个 if 块（否则提前消费 `ModifiedThisFrame`，破坏 "Display 只画屏" 契约）|
| `renderSegmentMD` 入口 | （已审计，无副作用）| n/a |

具体改动：
```go
// renderSegmentNative 入口
if !dryRun && b.ModifiedThisFrame {
    if b.Settings["diffgutter"].(bool) {
        b.UpdateDiff()
    }
    b.ModifiedThisFrame = false
}
```

### 6.4 drawGutterAndLineNumMD 的 dryRun 传播

`drawGutterAndLineNumMD` 内部调用 `w.drawGutter` / `w.drawDiffGutter` / `w.drawLineNum`，三者直接 `screen.SetContent`。

**传播方案**：签名加 `dryRun bool` 参数，向下传播。

```go
func (w *BufWindow) drawGutterAndLineNumMD(vY int, bufLine int, softwrapped bool, dryRun bool) {
    // 内部所有 screen.SetContent 加 if !dryRun 守卫
}
```

**注意**：`renderSegmentNative` 不走 `drawGutterAndLineNumMD`，而是直接调那三个底层函数（`bufwindow_md.go:300-317` 等），所以**这两条函数（`renderSegmentMD` 和 `renderSegmentNative`）的 dryRun 守卫要分别加**。

---

## 7. preRender 主流程

### 7.1 函数签名

```go
// internal/display/bufwindow_md.go

// preRenderRowmap 以当前 w.StartLine 为起点，用本帧的段归属（取决于 cursor 新位置）
// 做一次 dry-run 渲染，把屏行 → buffer 行的映射写入 w.viewportRowmap。
//
// 调用前提：w.Buf.IsMD == true
// 副作用：覆盖 w.viewportRowmap 的全部内容；不改 w.StartLine
// 不副作用：不写屏
func (w *BufWindow) preRenderRowmap(c SLoc) {
    // ... 见 7.2 ...
}
```

### 7.2 实现伪代码

```go
func (w *BufWindow) preRenderRowmap(c SLoc) {
    defer func() {
        if r := recover(); r != nil {
            // panic 时清空 viewportRowmap，让 relocateVerticalMD 走 fallback
            for i := range w.viewportRowmap {
                w.viewportRowmap[i] = SLoc{Line: -2}
            }
        }
    }()

    b := w.Buf
    bufHeight := w.bufHeight
    w.ensureMDConfigReady()

    // 用当前 StartLine 作为渲染起点（这正是"上一帧 viewport 的起点"）
    visibleStart := w.StartLine.Line
    visibleEnd := visibleStart + bufHeight
    if visibleEnd >= b.LinesNum() {
        visibleEnd = b.LinesNum() - 1
    }

    // 分配/重置 viewportRowmap
    if cap(w.viewportRowmap) < bufHeight {
        w.viewportRowmap = make([]SLoc, bufHeight)
    }
    w.viewportRowmap = w.viewportRowmap[:bufHeight]
    for i := range w.viewportRowmap {
        w.viewportRowmap[i] = SLoc{Line: -2}
    }

    // 复制 segs 再过滤（见 §6.2.1）
    segmentsCopy := make([]md.Segment, len(b.MDSegments))
    copy(segmentsCopy, b.MDSegments)
    segments := filterSegmentsToVisible(segmentsCopy, visibleStart, visibleEnd)

    cursors := b.GetCursors()

    // 复用 displayBufferMD 的主循环
    vY := 0
    for _, seg := range segments {
        if w.editMode && hasCursorInside(seg, cursors) {
            vY = w.renderSegmentNative(seg, vY, w.viewportRowmap, true)
        } else {
            vY = w.renderSegmentMD(seg, vY, w.viewportRowmap, true)
        }
        if vY >= bufHeight {
            break
        }
    }
    // 剩余空间保持 {Line:-2}，无需额外处理
}
```

**关键点**：
1. **起点用 `w.StartLine`**（旧值）：preRender 是为了"算出新 StartLine"，渲染时自然要以旧 StartLine 为基准画 viewport
2. **`hasCursorInside` 使用新 cursor 位置**：`cursors` 来自 `b.GetCursors()`，此时 cursor 已经是 `MoveCursorDown` 之后的新位置，所以 table 已经"脱离光标"，会正确切回 MD 渲染
3. **`editMode` 不变**：preRender 不改变用户模式状态
4. **与 Display 共享 MDSegments staleness tolerance**：`b.MDSegments` 在 `buffer.go:1032` goroutine 里更新，preRender 可能读到旧 MDSegments —— 但这与 Display 完全同帧，无新增问题

### 7.3 实施阶段说明（与最终态差异）

§13 实施步骤会分两步走：
- **Commit 1-3（过渡态）**：`renderSegmentNative` 还没 dryRun 支持，`preRenderRowmap` 第一版**只走 `renderSegmentMD` 路径**（normal 段跨段回退到 viewportRowmap 旧值）
- **Commit 4（最终态）**：`renderSegmentNative` 也加 dryRun，`preRenderRowmap` 完整支持

§7.2 是最终态的实现伪代码，Commit 1-3 的过渡实现见 §13。

---

## 8. Relocate 接入（v2 极简化）

### 8.1 修改点：`bufwindow.go:Relocate()`

```go
// 修改前
if w.Buf.IsMD {
    ret = w.relocateVerticalMD(c, scrollmargin, height)
} else {
    // ... 原生 ...
}

// 修改后（v2 极简）
if w.Buf.IsMD {
    w.preRenderRowmap(c)                         // ← 新增：永远跑
    ret = w.relocateVerticalMD(c, scrollmargin, height)
} else {
    // ... 原生分支不变 ...
}
```

**对比 v1**：v1 在这里要写 `if mapPossiblyStale(c) { preRender }` + 消费 `editModeDirty` + 更新 `lastCursorSegIdx`，共 4-5 行。v2 只有 1 行新增。

### 8.2 修改点：`relocateVerticalMD` 读取 viewportRowmap

v2 中 `relocateVerticalMD` 几乎不变——`viewportRowmap` 已被 `preRenderRowmap` 填充（或 panic 后清空）。

```go
func (w *BufWindow) relocateVerticalMD(c SLoc, scrollmargin, height int) bool {
    n := len(w.viewportRowmap)
    if n == 0 {
        return w.relocateVerticalNativeFallback(c, scrollmargin, height) // 首帧 / panic
    }
    cursorRow, ok := w.lineToScreenRowIn(w.viewportRowmap, c.Line, c.Row)
    if !ok {
        return w.relocateVerticalNativeFallback(c, scrollmargin, height) // cursor 不在 map
    }
    // ... 后续 botMarginRow / delta 逻辑与现状完全相同 ...
    botMarginRow := height - 1 - scrollmargin
    if cursorRow > botMarginRow {
        delta := cursorRow - botMarginRow
        if delta >= n {
            return w.relocateVerticalNativeFallback(c, scrollmargin, height)
        }
        loc := w.viewportRowmap[delta]
        for loc.Line < 0 && delta+1 < n {
            delta++
            loc = w.viewportRowmap[delta]
        }
        if loc.Line < 0 {
            return w.relocateVerticalNativeFallback(c, scrollmargin, height)
        }
        w.StartLine = loc
        return true
    }
    if cursorRow < scrollmargin {
        w.StartLine = w.Scroll(c, -scrollmargin)
        return true
    }
    return true
}
```

需要把 `LineToScreenRow` 拆出一个内部版本 `lineToScreenRowIn(map, line, row)`，原 `LineToScreenRow` 变成它的 wrapper（默认读 viewportRowmap）。保持对外 API 不变。

**窗口期说明**：`LineToScreenRow` / `ScreenRowToLine` 读 `viewportRowmap`，由 `LocFromVisual` 复用。v2 中 Relocate 结束后 `viewportRowmap` 反映"**新段归属 + 旧 StartLine**"——比 v1 的"旧段归属 + 旧 StartLine"**对 cursor 的 segment 更准确**。Display 紧随其后重建为"新 StartLine"版。鼠标点击窗口期内映射偏差仍可忽略（用户几乎不可能同时点击）。

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

### 9.2 方案A v2（fixed）

```
帧 N:    MoveCursorDown(59→60)
         Relocate:
           preRenderRowmap():                          ← v2 新增：永远跑
             dry-run 渲染，table 已脱离光标 → 切 MD 渲染 → map 含 9 row
             viewportRowmap = [新 map，table 9 row]
           relocateVerticalMD 读 viewportRowmap:
             c.Line = 60, cursorRow 正确（基于 9 row map）
             delta 算对 → StartLine 多滚 4 row
         observeEditModeToggle: SetEditMode(true)     ← 此刻 editMode 才变（但已无关）
         Display:
           displayBufferMD 基于新 StartLine 渲染，table 9 row 完整显示
           viewportRowmap 被重建（redundant 但无副作用）
           光标 line 60 在 viewport 底部 scrollmargin 内 ✅
```

**关键差异（v1 → v2）**：v1 有"触发条件判定"步骤（mapPossiblyStale），v2 没有——preRender 总是跑。

---

## 10. 边界场景

| 场景 | 处理 |
|---|---|
| **首帧 / MDSegments 为 nil** | viewportRowmap 长度为 0，relocateVerticalMD 走 fallback |
| **preRender panic（如 nil deref）**| defer recover 清空 viewportRowmap 为 {Line:-2}，`lineToScreenRowIn` 找不到 cursor → fallback |
| **光标跳出 viewport（大跳转）** | preRender 仍跑（无条件），但 `lineToScreenRowIn` 找不到光标 → fallback |
| **cursor 在装饰行（BufLine=-1）** | cursor 永远不在装饰行（c.Line 是 buffer 行号）|
| **多个 cursor（多光标编辑）** | preRender 用 `b.GetCursors()`，与现状一致 |
| **用户连续按 ↓ 跨多个段** | 每次 Relocate 独立跑 preRender，跨段时每帧都 preRender（典型 < 0.1ms/次）|
| **编辑动作导致段边界变化** | `b.MDSegments` 由 `buffer.go:1032` 在 re-highlight 时更新（异步）；preRender 与 Display 共享 1 帧容忍度 |
| **editMode 翻转（ESC / 任意键）** | **不影响**：observeEditModeToggle 在 Relocate 之后才调 SetEditMode（`bufpane.go:532`），Relocate 看到的 editMode 和 map 仍一致。**v2 不需要 editModeDirty 字段** |
| **resize（bufHeight 变化）** | preRender 内部按新 bufHeight 重新分配 viewportRowmap |
| **softwrap 开启** | 方案A 不依赖 softwrap 开关；`renderSegmentNative` 已有 softwrap 处理（vloc.Y -= StartLine.Row），dry-run 路径同样适用 |

---

## 11. 性能评估

### 11.1 触发频率
- 每次 Relocate（MD 文件）跑一次 preRender
- 典型频率：60 次/分钟（活跃编辑）→ 60 次 preRender/分钟

### 11.2 单次开销
- dry-run 渲染跳过所有 `screen.SetContent`，主要成本是 `seg.Render()`（CPU）+ map 写入（内存）
- Normal 段：< 0.1ms
- 多行段（table/codeblock）：< 1ms
- `displayBufferMD` 本来每帧就要调一次同样的 Render，preRender 是"提前调一次" + 后续 Display 再调一次 = 每次 Relocate 多约 1 次 displayBufferMD 开销

### 11.3 与方案B 的对比
**v2 实际上就是方案B 的"Display 不动"精简版**：

| 维度 | v2 | 方案B |
|------|-----|-------|
| preRender 时机 | 每次 Relocate（MD）| 每次 Relocate（MD）|
| Display 是否 render | 是（重建 viewportRowmap）| 否（纯 blit，用 bigViewportMap）|
| 复杂度 | 中 | 高（Display 全改）|
| 回归面 | 小 | 大 |
| 收益 | 修复核心 bug | 修复核心 bug + Display 性能提升 |

如果 v2 修复后用户不再报 Display 性能问题，**方案B 可不实施**。

---

## 12. 风险与回归

### 12.1 主要风险

| 风险 | 概率 | 缓解 |
|---|---|---|
| render 函数加参数后，dry-run 与真实路径行为不一致 | 中 | 单测覆盖：`TestRenderSegmentMD_DryRunMatchesWetRun`，断言同一 seg 在 dryRun=true/false 下产出的 map 完全相同 |
| preRender 内部 panic（如 nil deref）| 低 | defer recover 清空 viewportRowmap，relocateVerticalMD 自动回退 fallback |
| **`b.ModifiedThisFrame` 被 preRender 提前清掉**（`renderSegmentNative:238-243` 入口）| 中 | §6.3 加 `if !dryRun` 守卫；dry-run 路径完全不碰 `b.ModifiedThisFrame` / `b.UpdateDiff()` |
| **`drawGutterAndLineNumMD` 内部 screen.SetContent 在 dry-run 下泄漏** | 高 | §6.4 加 `dryRun` 参数向下传播 |
| **`filterSegmentsToVisible` 原地修改 `b.MDSegments[i].VisibleStart/End`** 在 dry-run 路径污染原数据 | 低 | §6.2.1：preRender 内部复制 segs 副本再过滤 |
| re-highlight 异步 → MDSegments 竞态 | 低 | preRender 与 Display 同帧读 MDSegments，共享 1 帧容忍度，无新增问题 |
| 性能回归（极端大文件 + 高频 Relocate）| 低 | 单次 preRender < 1ms，60 次/分钟 = 60ms/分钟；如测出问题，可加"同方向连续移动复用 preRender"节流 |
| **`renderSegmentNative` ~250 行中漏加 dryRun 守卫** | 中 | §13 实施步骤分两步：Commit 1-3 先只对 renderSegmentMD 加 dryRun（过渡态），Commit 4 再补 native（达到最终态）|

### 12.2 panic 防护

```go
func (w *BufWindow) preRenderRowmap(c SLoc) {
    defer func() {
        if r := recover(); r != nil {
            // 清空 viewportRowmap → relocateVerticalMD 走 fallback
            for i := range w.viewportRowmap {
                w.viewportRowmap[i] = SLoc{Line: -2}
            }
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
| 光标在段内连续 ↓↑ | preRender 每次都跑（< 0.1ms/次），行为与现状一致 |
| 大跳转（GotoLine 到文件尾）| preRender 跑但找不到 cursor → fallback |
| resize 后首帧 | viewportRowmap 长度 0 → fallback |
| 切换 editMode（ESC / 任意键）| editMode 变更在 Relocate 之后发生，Relocate 不受影响（v2 不需要 editModeDirty）|
| `sample.md` 全文浏览 | 无可感知的滚动瑕疵 |

---

## 13. 实施步骤（渐进式 commit）

**v2 关键决策**：架构上是"无条件 preRender"，但**实施时分两步走**，避免一次性改动过大：

1. **Step 1（Commit 1-3，过渡态）**：先用"段归属变化触发"，只动 `renderSegmentMD`，先完成核心 bug 修复
2. **Step 2（Commit 4，最终态）**：补全 `renderSegmentNative` 的 dryRun 支持后，删除触发条件，切换为"无条件 preRender"

### Commit 1: 抽象 renderSegmentMD 的 dry-run 能力
- `renderSegmentMD` 加 `target []SLoc, dryRun bool` 参数
- `drawGutterAndLineNumMD` 加 `dryRun bool` 参数向下传播
- `displayBufferMD` 现有调用点改为传 `(w.viewportRowmap, false)`
- §6.3 状态副作用清单同步检查（本 commit `renderSegmentMD` 入口审计）
- §6.2.1 改动 3 落实（filterSegmentsToVisible 复制）
- 加单测：`TestRenderSegmentMD_DryRunMatchesWetRun`
- **不改任何行为**，纯重构，可独立验证

### Commit 2: 新增 preRender 基础设施（过渡态：只走 MD 路径）
- 实现 `preRenderRowmap`：**第一版只走 `renderSegmentMD` 路径**（renderSegmentNative 还没 dryRun 支持）
- 实现 `segmentOfLine` / `cursorSegmentChanged` 辅助函数（过渡态用）
- 拆分 `lineToScreenRowIn`
- 新建 `bufwindow_md_test.go`：覆盖新增 API（ScreenRowToLine / LineToScreenRow / lineToScreenRowIn / cursorSegmentChanged）
- **暂不接入 Relocate**，可单测 preRender 产出

### Commit 3: 接入 Relocate（过渡态：段归属变化触发）
- 修改 `Relocate` 的 MD 分支：光标段变化时调 `preRenderRowmap`
- 修改 `relocateVerticalMD`：读 preRender 填充的 viewportRowmap
- 手测核心场景（sample.md table 上下穿越）

**Commit 3 时的架构状态**：过渡版本，保留 `cursorSegmentChanged` 触发条件。

### Commit 4: 切换为最终态（无条件 preRender）
- `renderSegmentNative` 加 `target + dryRun` 参数（~250 行守卫散落，按 §6.3 / §6.4 模式）
- `preRenderRowmap` 改为完整版本（同时支持 renderSegmentMD / renderSegmentNative 路径）
- `Relocate` 的 MD 分支改为**无条件**调 `preRenderRowmap`
- 删除 `cursorSegmentChanged` / `segmentOfLine`
- 性能验证（连续按 ↓ 场景，预期无感知）

**Commit 4 完成后**：达到 §0 TL;DR 描述的最终架构（无条件 preRender，零字段新增）。

### Commit 5: 边界打磨
- panic 防护（如需）
- 文档更新：把本方案合并到 `光标滚动-修改总结.md §5` 第一条标记为"已解决"

---

## 14. 代码改动清单（预估）

| 文件 | 改动类型 | 规模 |
|---|---|---|
| `internal/display/bufwindow.go` | `Relocate` MD 分支加 1 行 | **极小** |
| `internal/display/bufwindow_md.go` | render 函数加参数；新增 preRenderRowmap / segmentOfLine / cursorSegmentChanged / lineToScreenRowIn；修改 relocateVerticalMD；drawGutterAndLineNumMD 加 dryRun | 中 |
| `internal/display/bufwindow_md_test.go` | 新增 dry-run 一致性测试、preRender 测试 | 中（先修复现有编译失败）|
| micro 原生文件 | **零改动** | 0 |
| `internal/md/*.go` | **零改动**（renderer 不动）| 0 |
| `internal/action/bufpane_md.go` | **零改动**（observeEditModeToggle 不再触发 dirty flag）| 0 |

**v1 vs v2 改动量对比**：
- v1: bufwindow.go 改动 4-5 行（dirty 消费 + reset + 新增 4 个字段），bufwindow_md.go 改 mapPossiblyStale + segmentOfLine + 触发逻辑
- v2: bufwindow.go 改动 **1 行**，bufwindow_md.go 改 preRenderRowmap（逻辑更简单，因为不需要触发判定）

---

## 15. 与方案B 的关系

**v2 实际上就是方案B 的"Display 不动"精简版**。

| 维度 | v2（方案A 终态）| 方案B |
|------|-----------------|-------|
| preRender 时机 | 每次 Relocate（MD）| 每次 Relocate（MD）|
| Display 路径 | 不动（仍 Render + 重建 viewportRowmap）| 大改（纯 blit，读 bigViewportMap）|
| 字段 | 零新增 | 新增 bigViewportMap（容量增大、跨帧持有）|
| 复杂度 | 中 | 高 |
| 回归面 | 小 | 大 |
| 收益 | 修复核心 bug | 修复核心 bug + Display 性能提升 |

**v2 → 方案B 的迁移路径**（如果未来需要）：
1. `viewportRowmap` 改名 `bigViewportMap`，容量从 bufHeight 改为"光标可见 + scrollmargin + 溢出"
2. `displayBufferMD` 改为"读 bigViewportMap 渲染"，不再调 Render
3. `renderSegmentMD/Native` 维持 dryRun 路径（用于 Relocate 的 preRender）

如果 v2 修复后用户不再报 Display 性能问题，**方案B 可不实施**——这是 v1 与 v2 的关键判断差异（v1 把"Plan A 是 Plan B 第一阶段"作为论据，v2 视为可选）。

---

## 16. Review 修订记录

### v2 重大简化（2026-06-16，CTO 反思）

CTO 反思后判定 v1 的 `editModeDirty` / `lastCursorSegIdx` / `mapPossiblyStale` 等 dirty tracking 机制是**过度工程**。

**核心理由**：通过 `bufpane.go:443-532` 调用顺序分析，确认 `observeEditModeToggle`（第 532 行）永远在 action 派发（含 Relocate）**之后**才运行。因此：
- Relocate 看到的 editMode = 上一帧 Display 用的 editMode（两者一致）
- Relocate 看到的 viewportRowmap = 上一帧 Display 建的 map（与 editMode 一致）
- **editModeDirty 永远不会在 Relocate 看到不一致**

用户原话：
> "老实说，这个架构设计，经过几次修补之后，有点乱了，既不简洁也不优雅。你应该做为CTO从全局角度来做架构设计方案。"
> "BufWindow.editModeDirty，有必要吗？当用户光标正在某行，然后按esc退出editMode的时候，就没有光标什么事情了，直接把这个segment渲染展开就行了。根本用不着relocate"

**v1 → v2 架构变更清单**：

| 维度 | v1 | v2 |
|------|-----|-----|
| 新增字段 | 4 个 | **0 个** |
| 新增函数 | `mapPossiblyStale`, `segmentOfLine` | **0 个**（`cursorSegmentChanged` 留到过渡态）|
| Relocate 入口 | 4-5 行（含 dirty 消费 + reset）| **1 行** |
| 触发条件 | 段归属变化 OR editModeDirty | **无条件**（最终态）|
| 实施路径 | 一次性重构 | 4 个 commit 渐进（先段触发，后无条件）|

### Lisa review 第一轮（2026-06-16，v1）

| Review 意见 | 等级 | 处理 |
|---|---|---|
| `cursorSegmentChanged` 的"同行⇒段不变"短路不成立 | Blocker | v1 §5.3 已修复 |
| editMode 翻转是 map 陈旧的独立来源 | Suggestion→采纳 | **v2 撤销采纳**：通过调用顺序分析证明此来源不存在 |
| `LineToScreenRow` 窗口期 1 帧偏差 | Question | v2 §8.2 补说明：v2 中偏差更小（用新段归属）|
| "方案B ≈ 持平"性能结论过粗 | Question | v2 §11.3 改为 v2 ≈ 精简版方案B |
| Commit 2 引用已删除的测试文件 | Blocker(文档) | v2 §13 改为"新建测试" |
| `preRowmapValid` 生命周期描述歧义 | Suggestion | v2 整体删除此 flag |

### planner 第二轮评估（2026-06-16，v1）

| 评估意见 | v2 处理 |
|---|---|
| `b.ModifiedThisFrame` 副作用 | §6.3 采纳 |
| "段内零开销" 表述有误 | v2 不再有此表述（无条件 preRender）|
| `drawGutterAndLineNumMD` 传播未展开 | §6.4 采纳 |
| `renderSegmentNative` 改动面过大 | §13 Commit 1 先只动 MD 路径，Commit 4 再补 native |
| `filterSegmentsToVisible` 原地修改 | §6.2.1 采纳 |
| MDSegments 异步竞态 | §10 / §12 保留 |
| `lastCursorSegIdx` 缓存不重置 | v2 整体删除此字段 |
| "Plan A 是 Plan B 第一阶段" 论证 | v2 §15 改为 v2 ≈ 精简版方案B（Plan B 改为可选）|

---

**v2 架构宣言**：用最简单的方案解决唯一真正的问题——光标跨段时 map 与段归属不符。
不要 dirty tracking，不要触发判定，不要字段缓存。承认 Relocate 看到的 map 一定陈旧，无条件刷新一次即可。