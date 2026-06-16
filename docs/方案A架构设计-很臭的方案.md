# 方案 A 架构设计（preRender-dryRun）

> 评估对象：`docs/光标滚动-方案A.md` 中的「preRender + dryRun」路线
> 关联：`docs/光标滚动-修改总结.md` §5（v1.0.5 已知限制）、`docs/方案B评估报告mm.md`（已排除 B2，参考 B' 思路收敛到 A）
> 约束：侵入 micro 原生代码越小越好；render 函数（`internal/md/render_*.go`）零改动
> 初版：2026-06-16
> 修订：2026-06-16 — Lisa 评审后修订（§6.2 路线命名统一、§6.7 prevCursorY editMode=false 安全性、§4 数据流时序保证、§9.1 多窗口风险升级、§10 补回归测试清单、§3.2 性能上限说明）
> 二次修订：2026-06-16 — Lisa 二次评审后修订（§5 改动表与§6.2 路线C对齐：renderSegmentNative 零改动+新增 renderSegmentNativeDryRun、§3.3 伪代码函数名统一、§10 补 bufWidth=0 退化测试、§3.4 补全路径调用成本分析 + 顺序扫描优化建议）

---

## 0. 结论

**方案 A 是性价比最高的 V2 修复路径。** 一句话：在 `relocateVerticalMD` 入口插一道「跨段检测 + 影子渲染」钩子，用新鲜地图算 scroll；Display 主循环、render 函数、actions.go、buffer 缓存机制全部不动。

刀落在两处：
- `internal/display/bufwindow_md.go`（核心：dryRun + preRender + 触发判定）
- `internal/display/bufwindow.go`（`prevCursorY` 字段 + 一行更新）

`internal/md/` 包（render_*.go / md.go / detect.go）**零改动**。

---

## 1. V2 bug 根因（与方案 B 评估一致）

**时序断层**：`viewportRowmap` 是**上一帧** `displayBufferMD` 的产物，`Relocate()` 决策时 map 反映"光标在旧位置时"的 viewport 长什么样。光标一跨段，地图失效，scroll 量算少。

```
Display()        ← 用旧 cursor 生成 viewportRowmap
event = ↓
MoveCursorDown   ← c.Loc 改了
Relocate()       ← 用【上一帧、已失效】的 map 算 scroll  ←  V2 bug 诞生地
Display()        ← 这时才重算 map，但已经晚了
```

**关键不变量**：装饰行（table frame / codeblock border）的有无是 cursor 位置的函数，map 永远落后一帧 → 靠改 `relocateVerticalMD` 内部判定逻辑**无法解决**（输入数据本身错）→ 必须先重算地图再决策。

---

## 2. 方案 A 核心思路

**触发条件**：光标跨 segment 边界（仅此时地图会失效）
**修复动作**：在 `relocateVerticalMD` 入口，对"假设光标停在 c.Line"的状态做一次 dry-run 渲染，重建 `viewportRowmap`
**后续决策**：用这份新鲜地图喂给原有的 `LineToScreenRow` + `viewportRowmap[delta]` 逻辑，scroll 量自然算对

**与方案 B' 收敛**：评估报告里 glm 推荐的"影子渲染"本质就是方案 A；A 文档（v1.0.5 时期）漏掉了 native 渲染器的 dryRun 处理和 prevCursor 跟踪的细节，本设计补齐。

---

## 3. 三个关键组件

### 3.1 组件① Render 层天然分层（无需重构）

回看现状：
- `seg.Render(seg, width, cfg)` → `*RenderedSegment`（**纯函数**，只产 `Rows`，不碰 screen）
- `renderSegmentMD()` 拿到 `RenderedSegment` 后才写 `screen.SetContent` + `viewportRowmap[vY]`

计算与绘制**本来就分层**了。dryRun 的本质：跳过 `screen.SetContent` 那一步，只走 `viewportRowmap` 写入。

→ **render 函数（`internal/md/render_*.go`）零改动**，dryRun 适配全在 `renderSegmentMD` 内部加参数。

### 3.2 组件② 触发条件：prevCursorY 跟踪 + editMode 守卫

BufWindow 加一个字段：

```go
prevCursorY int  // 每帧 Display 末尾更新为 activeCursor.Y
```

触发判定（在 `relocateVerticalMD` 入口，6 行）：

```go
prevSeg := w.findSegmentContaining(w.prevCursorY)
newSeg  := w.findSegmentContaining(c.Line)
needPreRender := w.editMode &&
    prevSeg != nil && newSeg != nil && prevSeg != newSeg
if needPreRender {
    w.preRenderAtCursor(c.Line)
}
```

`findSegmentContaining` 是 O(n) 线性扫 `b.MDSegments`（n = segment 数）。

**触发条件分析**：
1. **`w.editMode` 守卫**：editMode=false 时所有 segment 都走 MD 渲染，map 不依赖光标位置，不需要 preRender（map 自洽）。
2. **`prevSeg != newSeg` 指针比较**：跨 segment 时，oldSeg 从 native 切回 MD（可能重新展开装饰行），newSeg 从 MD 切到 native（可能折叠装饰行），两边布局都变，map 失效。
3. **同 segment 内移动不触发**：cursor 在同一 segment 内移动，该 segment 始终 native，map 不变。

**关于 `findSegmentContaining` 性能上限**：
- DetectSegments 对**非多行结构**（heading / hr / 裸 Normal 行）每行一个 segment
- 纯文本 MD 文档（无 codeblock / blockquote / table / list）→ N ≈ buffer 行数
- 典型结构化 MD 文档 → N 一般 50-200
- 实际调用不止触发阶段的 2 次，**preRender 内部还有 N 次**（while 循环每渲染一段调一次）
- **全路径总成本 O(N·n)**，最坏情况（纯文本 MD）~50μs，仍远低于一帧 16ms
- 详细分析与可选优化（单次扫描+游标）见 §3.4

**结论**：不优化，O(N·n) 足够。**真正的成本是 preRender 本身的渲染**（O(visible 行数)），而非 segment 查找。

**为什么不用 RenderType 比较**（用户提出的关键反例）：
```
line 10: # Heading A    ← Seg HA
line 11: # Heading B    ← Seg HB
```

cursor 从 10 → 11，两个 segment 的 RenderType 都是 "Heading"，但：
- 旧布局：HA native（1 行） + HB MD（2 行）
- 新布局：HA MD（2 行） + HB native（1 行）

**map 完全不同**——HA 的 deco 横线行从无到有，HB 的 deco 横线行从有到无。RenderType 比较会漏掉这种情况，必须用指针比较。
**Normal↔Normal 伪触发**：相邻 Normal 段间移动（detect 是每行一个 Normal segment），prevSeg != newSeg 但 Normal 无装饰行，map 实际不变，会多花一次 preRender。**接受这个浪费**——preRender 代价低（毫秒级），不值得加 RenderType 字段来过滤。
**指针稳定假设**：`DetectSegments` 一次产出后缓存在 `Buf.MDSegments`，每次 buffer 编辑才重跑。同一次会话内 segments 切片稳定，`prevSeg != newSeg` 可靠。

### 3.3 组件③ preRender 函数（新增）

```go
// preRenderAtCursor 渲染"假设光标在 cursorLine"状态的 viewportRowmap。
// 起点：当前 StartLine。
// 渲染规则：
//   - cursorLine 所在 segment → renderSegmentNativeDryRun（只写 map、不画屏）
//   - 其他 segment            → renderSegmentMD       （dryRun=true）
// 停止条件：见下，**目的驱动**（不是「渲染 N 行」魔法数）。
func (w *BufWindow) preRenderAtCursor(cursorLine int)
```

**为什么不用「渲染 N 行」式的停止条件**：

初稿用 `3*bufHeight`（≈150 行）当上限。问题：
- **浪费**：cursor 段 5 行时，渲染 145 行多余内容
- **不足**：cursor 段 100 行 + scrollmargin 时，150 行可能不够
- **意图不明**：3 倍这个数字怎么来的？为什么不是 2 倍或 5 倍？

**preRender 的三个目的**（方案 A 场景下）：

| 目的 | 含义 | 为什么必要 |
|------|------|----------|
| 1. 光标行 + scrollmargin 已渲染 | 屏行 + 滚动余量 | Relocate 要从 cursorRow 算 delta scroll |
| 2. 旧光标段已整段渲染 | 保持 map 自洽 | 防止「老 cursor 位置在 map 里消失」导致后续读 map 出错 |
| 3. viewport 已填满 | 渲染范围足够 | 避免 Display 时 vY 超 bufHeight 但 map 没填满 |

停止条件 = 三个目的**全部**达成。

**伪代码**：

```
function preRenderAtCursor(cursorLine):
    oldSeg = findSegmentContaining(prevCursorY)
    oldSegDone = (oldSeg == nil)    # 无旧段（首帧 / sentinel）→ 视为已完成
    cursorDone = false
    cursorRow = -1
    cursorSegEndVY = -1              # cursor 段渲染结束时的 vY 快照
    linesAfterCursorSegEnd = 0       # 命名：从 cursor 段结束处累计的屏行数（用于判断 scrollmargin）
    line = StartLine.Line
    vY = 0
    # map 直接写入 w.viewportRowmap（与 displayBufferMD 同源）

    while true:
        seg = findSegmentContaining(line)
        if seg == nil:
            break    # 越界（理论上不会发生）

        # 整段渲染（render 函数内部已把段完整写完）
        if seg contains cursorLine:
            vY = w.renderSegmentNativeDryRun(seg, vY)
            cursorDone = true
            cursorRow = vY - 1            # cursor 在段末屏行
            cursorSegEndVY = vY           # 记录段末 vY，供后续累计
        else:
            vY = w.renderSegmentMD(seg, vY, dryRun=true)

        # 跳到下一段
        line = seg.BufEndLine + 1

        # 旧段是否整段渲染完成（段整段渲染 → 「整段完成」是免费保证）
        if seg == oldSeg:
            oldSegDone = true

        # === 停止条件：三个目的全部满足才 break ===
        canBreak = true

        if not oldSegDone:
            canBreak = false    # 目的 2 未达成

        if not cursorDone:
            canBreak = false    # 目的 1.a 未达成
        else if linesAfterCursorSegEnd < scrollmargin:
            canBreak = false    # 目的 1.b 未达成

        if vY < bufHeight:
            canBreak = false    # 目的 3 未达成

        if canBreak:
            break

        # 推进 linesAfterCursorSegEnd：累加 cursor 段之后的屏行数
        # （cursorSegEndVY 是 cursor 段结束时的 vY 快照，vY 是当前 vY）
        if cursorDone:
            linesAfterCursorSegEnd = vY - cursorSegEndVY

    # map 在渲染时直接写入 w.viewportRowmap，无需额外赋值
```

**关键点**：
- 「段整段渲染」是天然停止保证：renderSegmentMD/Native 返回时整段已写完，不需要逐行算
- 三个目的都是「必达」不是「或」——**全部**满足才 break（一个失败继续渲染）
- 没有魔法数：渲染量由 buffer 结构和目的自然决定
- `oldSeg == nil` 时（如首帧 `prevCursorY=0` 命中 sentinel、或旧光标被裁出 viewport）→ 视为已完成，不多渲染

**复杂度（粗估）**：

| 场景 | 渲染量 |
|------|--------|
| cursor 段紧贴 StartLine | ≈ bufHeight |
| cursor 段在 viewport 底部 | ≈ bufHeight + cursor 段长 |
| 旧段在 cursor 段**之后**且 viewport 外 | + 旧段长（强制多渲染） |
| 旧段在 StartLine **之前** | 不增加（默认已完成） |

最坏情况：bufHeight + 2 个段长，一般 < 200 行。完全在毫秒级。

**和初稿「3*bufHeight」对比**：

| 场景 | 3*bufHeight | 目的驱动 |
|------|-------------|---------|
| cursor 段 5 行，scrollmargin 5 | 浪费 145 行 | 渲染到 cursor+5 即可 |
| cursor 段 100 行大表 | **不够**（100+5 边缘） | 段整段 + scrollmargin 必然达成 |
| 旧段在 viewport 外 | 150 行可能渲染不到 | 强制整段渲染（不论距离） |
| 首帧 prevCursorY=0 | 多渲染到 150 | oldSegDone 立即 true，不过度 |

### 3.4 findSegmentContaining 伪代码

触发代码里看似有「两个 find」，其实是**同一个函数被调了两次**：

```go
prevSeg := w.findSegmentContaining(w.prevCursorY)   // 旧光标位置
newSeg  := w.findSegmentContaining(c.Line)         // 新光标位置
```

**伪代码**：

```
function findSegmentContaining(line):
    if line < 0:
        return nil    # 越界保护：覆盖 prevCursorY=-1 sentinel
    
    for i in 0..len(Buf.MDSegments)-1:
        seg = &Buf.MDSegments[i]    # 取地址，返回真实指针
        if seg.BufStartLine <= line <= seg.BufEndLine:
            return seg
    
    return nil    # 理论上不会发生（detect 保证所有 buffer 行都被覆盖）
```

**为什么必须返回指针不返回拷贝**：

```go
// ❌ 拷贝：prevSeg 和 newSeg 永远不相等（不同栈地址）
prevSeg := w.findSegmentContaining(prevCursorY)  // struct 拷贝 #1
newSeg  := w.findSegmentContaining(c.Line)       // struct 拷贝 #2
prevSeg != newSeg  // 永远 true → 每次都触发 preRender

// ✅ 指针：prevSeg 和 newSeg 指向 MDSegments 中的同一个元素
prevSeg := w.findSegmentContaining(prevCursorY)  // &MDSegments[3]
newSeg  := w.findSegmentContaining(c.Line)       // &MDSegments[3]（同段时）
prevSeg == newSeg  // true → 段内移动不触发 preRender
```

**复杂度**：O(n)，n = segment 数。DetectSegments 重跑时会重建切片，指针稳定性详见 §3.2 末尾「指针稳定假设」。

**全路径调用总成本分析**（Lisa 评审后补充）：

实际调用点不止触发阶段的 2 次，preRender 内部还有 N 次：

| 阶段 | 调用点 | 调用次数 | 每次复杂度 |
|------|--------|---------|-----------|
| 触发判定 | prevSeg | 1 | O(n) |
| 触发判定 | newSeg | 1 | O(n) |
| preRender | oldSeg | 1 | O(n) |
| preRender | while 循环中找下一段 | N | O(n) |
| **总计** | | **N+3** | **O(N·n)** |

各 N / n 取值下的总比较次数与预估耗时：

| 场景 | N（viewport 段数）| n（总段数）| 总比较 | 耗时 |
|------|------|------|--------|------|
| 典型结构化 MD | 5-20 | 50-200 | 400-4600 | < 10μs |
| 复杂 MD（多表多代码块）| 20 | 500 | 11500 | ~30μs |
| 极长纯文本 MD | 20 | 1000 | 23000 | ~50μs |

**结论**：O(N·n) 在最坏情况下也只是几十微秒级，远低于一帧显示时间（16ms@60fps），不优化仍可接受。

**可选优化**（如未来发现 N·n 场景成为热点）：

preRender while 循环的 N 次 findSegmentContaining 是**顺序访问**的（line 每次 += 1，segments 按 BufStartLine 排序）——这是单调递增的二分查找问题，**可改为单次扫描 + 游标前进**：

```go
// 优化版：避免重复扫
func (w *BufWindow) preRenderAtCursor(cursorLine int) {
    // ... 初始化 ...
    segIdx := 0
    for segIdx < len(w.Buf.MDSegments) && w.Buf.MDSegments[segIdx].BufEndLine < StartLine.Line {
        segIdx++  // 跳过 viewport 之前的段
    }
    // 接下来从 segIdx 开始顺序迭代，不再调 findSegmentContaining
    for segIdx < len(w.Buf.MDSegments) {
        seg := &w.Buf.MDSegments[segIdx]
        // ... 渲染 seg ...
        segIdx++
    }
}
```

总复杂度从 **O(N·n) 降为 O(n)**。收益在纯文本 MD 上最明显（n 大），但代码复杂度略增（需要同时处理 startLine 跳跃）。**Commit 1 不实施，留作 v2.0 性能调优项**。

**Go 实现骨架**（仍采用简单版，O(N·n)）：

```go
func (w *BufWindow) findSegmentContaining(line int) *md.Segment {
    if line < 0 {
        return nil
    }
    for i := range w.Buf.MDSegments {
        seg := &w.Buf.MDSegments[i]
        if line >= seg.BufStartLine && line <= seg.BufEndLine {
            return seg
        }
    }
    return nil
}
```

### 3.5 relocateVerticalMD 整体流程

#### 1. 这个函数在 micro 里做什么

`Relocate` 决定 `StartLine`（viewport 顶部对应的 buffer 位置）应该设到哪里，**保证光标在 viewport 中尽量靠中间、不贴边**。micro 主循环里几乎所有造成「cursor 移动」的动作（光标键、滚轮、点击、搜索跳转）都会调它。

**对 MD 文件的特殊性**：micro 原生 Relocate 假设「buffer line ↔ 屏行」是 1:1（1 个 buffer 行 = 1 个屏行）。但 MD 渲染下，1 个 buffer 行可能是 0 行（被合并）、1 行（普通文本）、或多行（长标题、表格行可能在多屏行）。所以 MD 需要自己的 Relocate 路径——用 `viewportRowmap` 查表。

`relocateVerticalMD` 就是 MD 专属的垂直 Relocate，实现是 bufwindow_md.go 里的现状代码（v1.0.5 已有），方案 A 不重写它，只在**入口**加几行。

#### 2. 入参 / 出参

| 名称 | 含义 |
|------|------|
| `c SLoc` | 当前光标位置（Line, Row） |
| `scrollmargin int` | 滚动边距：光标离 viewport 边几个行时就开始滚 |
| `height int` | viewport 高度（屏行数） |
| **返回** `bool` | StartLine 是否被改过（true = 改了、可能需要重绘） |

#### 3. 整体流程（伪代码）

```
function relocateVerticalMD(c, scrollmargin, height):
    # === 方案 A 触发判定（§3.2，入口第一件事）===
    prevSeg = findSegmentContaining(prevCursorY)
    newSeg  = findSegmentContaining(c.Line)
    needPreRender = w.editMode
                    and prevSeg != nil
                    and newSeg != nil
                    and prevSeg != newSeg
    if needPreRender:
        preRenderAtCursor(c.Line)    # 刷 map，不画屏
    
    # === 原逻辑（5 个分支）===
    n = len(viewportRowmap)
    if n == 0:
        return relocateVerticalNativeFallback(...)    # 分支 A: 首帧
    
    cursorRow, ok = LineToScreenRow(c.Line, c.Row)
    if not ok:
        return relocateVerticalNativeFallback(...)    # 分支 B: 光标出 viewport
    
    botMarginRow = height - 1 - scrollmargin
    
    if cursorRow > botMarginRow:
        # 分支 C: 光标在底部滚动区
        ...
    elif cursorRow < scrollmargin:
        # 分支 D: 光标在顶部滚动区
        ...
    else:
        return true    # 分支 E: 光标在中间安全区，不动
```

#### 4. 五个分支详解

```
viewport (height 行)：
┌─────────────────────────────┐
│  scrollmargin 行的顶部区     │  → 分支 D
├─────────────────────────────┤
│                             │
│       中间安全区             │  → 分支 E（不动）
│                             │
├─────────────────────────────┤
│  scrollmargin 行的底部区     │  → 分支 C
└─────────────────────────────┘
```

| 分支 | 条件 | 动作 | 为什么要滚动 |
|------|------|------|------------|
| **A 首帧** | `n == 0` | fallback | map 还没生成，不能查表 |
| **B 出 viewport** | `LineToScreenRow` 返回 false | fallback | cursor 不在 map 覆盖范围（上下方） |
| **C 底部区** | `cursorRow > botMarginRow` | 向下滚 `delta = cursorRow - botMarginRow` 行 | cursor 贴底/出底 |
| **D 顶部区** | `cursorRow < scrollmargin` | 向上滚 `StartLine = Scroll(c, -scrollmargin)` | cursor 贴顶/出顶 |
| **E 中间区** | `scrollmargin <= cursorRow <= botMarginRow` | 不动 | cursor 在安全区，scroll 没意义 |

#### 5. 分支 C 的细节（最容易出错）

```go
if cursorRow > botMarginRow {
    delta := cursorRow - botMarginRow
    if delta >= n {
        return fallback    # 滚动量超过 map 长度
    }
    loc := viewportRowmap[delta]
    // 装饰行/空白行的 Line=-2（不是真实 buffer 行）
    // 跨过这种行去找首个真实内容行
    for loc.Line < 0 && delta+1 < n {
        delta++
        loc = viewportRowmap[delta]
    }
    if loc.Line < 0 {
        return fallback    # 后续全是装饰/空白
    }
    w.StartLine = loc
    return true
}
```

`delta` = cursor 需要往下移动多少行才能到 botMarginRow。`viewportRowmap[delta]` 就是新视口顶位置。但**这个位置可能落在装饰行**（表格 frame、分隔线、HTML 标签的「虚构行」）上，StartLine 是 buffer 位置，不能是装饰行。所以要往后扫到首个 Line ≥ 0 的位置。

**举例**：
- viewport 高 50，scrollmargin=5，botMarginRow=44
- cursorRow=49（光标在第 49 行）
- delta = 49-44 = 5
- viewportRowmap[5] 可能是「表格分隔行」→ Line=-2 → 往后扫
- viewportRowmap[7] 是「表格 body」→ Line=72 → StartLine=72
- 新 viewport 顶部 = buffer 第 72 行，光标落在第 44+4=48 屏行（在底部区内）✓

#### 6. 方案 A 在哪里介入

```
┌─────────────────────────────────────────┐
│ DoKeyEvent / 其它动作 触发 Relocate      │
└────────────────┬────────────────────────┘
                 ▼
┌─────────────────────────────────────────┐
│ relocateVerticalMD (bufwindow.go 调用)   │
│                                         │
│  1. 【方案 A 入口】 preRender 判定      │  ←  这里插入
│     - prevSeg != newSeg && editMode      │
│     - 满足 → preRenderAtCursor 刷 map   │
│                                         │
│  2. 原 5 分支逻辑（用刚刷过的 map 决策） │
│     - LineToScreenRow 精准命中           │
│     - botMarginRow 判定用真实 cursorRow  │
│     - delta 从 map 拿 StartLine          │
│                                         │
└─────────────────────────────────────────┘
```

**核心价值**：方案 A 不重写 5 分支逻辑，只在**入口**插几行让 map 是「光标在 c.Line 状态」的 fresh 版本。后续分支读到正确的 map，scroll 计算自然精准。

#### 7. 一个完整的走读示例

**场景**：viewport 高 50，scrollmargin=5。光标在第 10 行的表格里（table 渲染后是带 frame 的、占多行）。用户按 ↓ 跳到第 11 行（同表内）。

**无方案 A**（现状 bug）：
1. 上一帧 Display 生成的 map：第 10 行 = 屏行 15（表格顶部）
2. Relocate 入口：LineToScreenRow(11) → 查 map → 屏行 16
3. 16 < 44（botMarginRow）→ 光标在中下部 → **不动**
4. 用户看不出变化 → 选不中「下移了」的感觉
5. 实际问题是：map 反映的是「第 10 行所在段为 native」的布局，现在光标在第 11 行（可能从 native 变 MD 了），但 Relocate 不知道

**有方案 A**（修复后）：
1. 上一帧 Display：光标在 10，表格是 native 渲染，map 有表格在屏行 15-16
2. 触发：prevCursorY=10, c.Line=11，同表同段 → 触发条件 `prevSeg != newSeg` 为 false → **不 preRender**
3. Relocate 继续，map 还是准的（同段内 cursor 移动不需重算）✓

**有方案 A 另一个例子**（跨段）：
1. 上一帧 Display：光标在第 10 行（Normal 段）
2. 用户按 ↓ 跳到第 11 行（Heading 段）
3. 触发：`prevSeg != newSeg` = true → preRender
4. preRender：用新光标状态重算 map，Heading 段会用 MD 渲染（带 deco 横线）
5. Relocate 继续：LineToScreenRow(11) 查到 Heading 段在屏行 X
6. 如果 X > botMarginRow：向下滚到 botMarginRow 区间；如果 X < scrollmargin：向上滚；否则不动

#### 8. fallback 触发场景汇总

走 `relocateVerticalNativeFallback`（1:1 原生算术）的场景：

1. **首帧**（map 为空）→ Relocate 被调时 viewportRowmap 还没生成
2. **光标跳出 viewport**（LineToScreenRow 返回 false）→ cursor 在 viewport 上方/下方很远
3. **delta 超 map 长度** → scroll 需求太大，map 不够长
4. **delta 落装饰/空白行且后续全是装饰** → 找不到 Line ≥ 0 的位置当 StartLine

**这些场景走 1:1 fallback 算术**，对 MD 用户马马虎虎够用（用户不修的决策见 §6.7）。nativeFallback 完全不读 viewportRowmap，假定 buffer line = 屏行，MD 下不够精准但不会崩。


---

## 4. 数据流（修复后）

```
用户按 ↓
    ↓
actions.go: CursorDown() → MoveCursorDown(1)   ←  c.Loc 变了
    ↓
bufwindow.go: Relocate()
    ↓
bufwindow_md.go: relocateVerticalMD(c)
    ↓
    ┌─ 检测跨段：prevCursorY 所在 seg vs c.Line 所在 seg
    │    └─ 不同 → preRenderAtCursor(c.Line)
    │              ├─ 各 segment 渲染（dryRun=true，只写 map）
    │              └─ viewportRowmap 现在反映"光标在 c.Line 时的真实布局"
    │
    ├─ LineToScreenRow(c.Line, c.Row)  ← cursorRow 现在拿准了
    ├─ cursorRow > botMarginRow ?
    │    └─ 是 → viewportRowmap[delta] 直接给出新 StartLine（无装饰行估算错误）
    ↓
actions.go: 继续往下走（不变）
    ↓
Display() → displayBufferMD() 正常渲染
    ↓
displayBufferMD 末尾：w.prevCursorY = w.Buf.GetActiveCursor().Loc.Y   ←  为下一帧 Relocate 准备
```

**`prevCursorY` 更新的调用顺序保证**：

代码核查（`cmd/micro/micro.go:487-498` + `internal/display/bufwindow.go:932-944`）：
- **Display 必在 Relocate 之前跑一次**（主循环固定时序：先 Display 一帧，再等事件，再处理事件可能触发 Relocate）
- `BufWindow.Display()` 必调 `updateDisplayInfo()` 设置 `bufWidth`/`bufHeight`
- `displayBufferMD` 末尾更新 `prevCursorY` → 下一帧 Relocate 读到的是 "上一帧末尾的 cursor.Y"
- 即：Relocate 读到的 `prevCursorY` 对应「上一帧 Display 时的光标位置」，与「上一帧生成的 viewportRowmap」在语义上同步

为什么必须在 `displayBufferMD` 末尾更新、不能在 `Relocate` 末尾更新？
- Relocate 末尾更新 → 下次 Relocate 读到的 prevCursorY 是"上次 Relocate 决策时"的位置，但 viewportRowmap 是"上次 Display 时"的 → **两者不同步**
- displayBufferMD 末尾更新 → 下次 Relocate 读到的 prevCursorY 与"上次 Display map 时的光标位置"完全一致 → 同步

---

## 5. 关键代码改动点

| 文件 | 改动 | 行数估计 |
|------|------|---------|
| `internal/display/bufwindow_md.go` | `renderSegmentMD` 加 `dryRun bool` 参数；dryRun=true 时跳过 `screen.SetContent` + `expandLineStyles` + cell style 合并；只写 `viewportRowmap` | +20 |
| `internal/display/bufwindow_md.go` | `renderSegmentNative` **零改动**（路线 C 不动此函数） | 0 |
| `internal/display/bufwindow_md.go` | **新增** `renderSegmentNativeDryRun(seg, startVY) (newVY int)`（路线 C：用 `getRowCount` 累加 `viewportRowmap`，不画屏） | +15 |
| `internal/display/bufwindow_md.go` | 新增 `preRenderAtCursor(cursorLine int)` | +50 |
| `internal/display/bufwindow_md.go` | 新增 `findSegmentContaining(line int) *md.Segment` | +10 |
| `internal/display/bufwindow_md.go` | `relocateVerticalMD` 入口加跨段检测 + 触发 preRender（指针比较 + editMode 守卫） | +8 |
| `internal/display/bufwindow.go` | `BufWindow` 加 `prevCursorY int` 字段 | +1 |
| `internal/display/bufwindow.go` | `displayBufferMD` 末尾更新 `prevCursorY = activeCursor.Y` | +2 |
| `internal/md/render_*.go` | **零改动** | 0 |
| `internal/md/md.go` | **零改动** | 0 |
| `internal/md/detect.go` | **零改动** | 0 |
| `internal/action/actions.go` | **零改动** | 0 |
| `internal/display/softwrap.go` | **零改动** | 0 |
| `internal/buffer/buffer.go` | **零改动** | 0 |

**总侵入**：~105 行，全在 display 包内，micro 原生 `displayBuffer()` 不动。

---

## 6. 设计细节（拍板点）

### 6.1 preRender 里 cursor 段走 native 还是 MD？

**native**。原因：跨段瞬间，新光标所在 segment 在 Display() 里就是用 native 渲染的（`editMode=true` + `hasCursorInside` 命中）。preRender 必须模拟同样的状态，否则地图不一致 → 后续 `LineToScreenRow` 仍可能命中错误位置。

### 6.2 native 渲染器怎么加 dryRun？

`renderSegmentNative` 是 ~370 行庞然大物（`bufwindow_md.go:210` 起到 ~580 行），里面大量 `screen.SetContent`、`screen.ShowCursor`、`expandLineStyles`、selection 高亮、cursorline、color-column 等深度耦合显示。dryRun 不需要这些，只需要**几何**：每个 buffer 行折成几个屏行、Row 值如何分布。

**关键发现**：micro 已有 `getRowCount(line int) int`（`softwrap.go:226`），它封装了 `getVLocFromLoc`（`softwrap.go:70`），精确返回某 buffer 行在当前 `bufWidth + softwrap + wordwrap + tabsize` 下折成的屏行数，完整处理 tab、宽字符（runewidth）、wordwrap 三种情况。而且——它就是 `SLocFromLoc`（`softwrap.go:239`）用来生成 `c.Row` 的同一套逻辑。

→ **dryRun 直接复用 `getRowCount`**，与 `c.Row` 同源，`LineToScreenRow(c.Line, c.Row)` 必然命中，不存在分叉。

**三条路线总览**（命名 §6.2 全文统一）：

| 路线 | 做法 | 状态 |
|------|------|------|
| **A** | 完整跑 native 主体，每处 `screen.SetContent` 加 `if !dryRun` | 可行但代码多 |
| **B** | 短路累加：每行硬写 `Row: 0`、一行只 `startVY++` 一次 | ❌ 已被排除（§6.2 详述） |
| **C（本方案）** | 调用 `getRowCount(line)` 精确累加 `viewportRowmap` | ✅ 选定 |

**路线 C 实现骨架**：

```go
// renderSegmentNativeDryRun 用 getRowCount 精确累加 viewportRowmap，不画屏。
// 与 renderSegmentNative 的 map 写入在数学上等价，且与 SLocFromLoc(c.Row) 同源。
func (w *BufWindow) renderSegmentNativeDryRun(seg md.Segment, startVY int) (newVY int) {
    for line := seg.VisibleStart; line <= seg.VisibleEnd; line++ {
        rows := w.getRowCount(line) // 含 tab/宽字符/wordwrap
        for row := 0; row < rows; row++ {
            if startVY >= 0 && startVY < w.bufHeight {
                w.viewportRowmap[startVY] = SLoc{Line: line, Row: row}
            }
            startVY++
        }
    }
    return startVY
}
```

**为什么排除路线 B**（每行硬写 `Row: 0`、一行只 `startVY++` 一次）：

路线 B 的隐含假设是「1 个 buffer 行 = 1 个屏行」，**只在非 softwrap 下成立**。softwrap 开启时，长行会折成多个屏行，map 里应当出现 `{Line: L, Row: 1}`、`{Line: L, Row: 2}`… 路线 B 两个错叠加：
- **Row 全写 0** → `LineToScreenRow(c.Line, c.Row>=1)` 查不到 → 退 fallback
- **整段行数偏少**（一行只累加一次）→ 后续段屏行位置前移 → cursorRow 偏小 → 该滚不滚

光标段走 native 的高频场景是 code block（`editMode && hasCursorInside`），而 code block **长行 + softwrap** 是常见组合。路线 B 在这个交集下重现 V2 bug 变种。路线 C 无此问题——任意 softwrap/wordwrap/tabsize 设置下都与 `c.Row` 一致。

**为什么选 C 不选 A**（完整分支跳过）：

| | 路线 C（getRowCount 累加）| 路线 A（完整分支跳过）|
|---|---|---|
| 代码量 | ~8 行 | ~30 行（每处 SetContent 加判断）|
| 正确性 | ✅ 与 `SLocFromLoc` / `c.Row` 同源 | ✅ 与真实渲染同源 |
| 维护风险 | 低（复用 micro 已有 API）| 中（dryRun 分支须与主体同步）|
| 侵入原生 | 0（只读已有函数）| 0（但改动 renderSegmentNative 内部）|

**结论**：选路线 C。既比路线 B 正确（任意 softwrap 设置都对），又比路线 A 轻量，且复用 micro 已有函数、零增侵入，契合「原生代码零改动」约束。

**`bufWidth` 有效性验证**（Commit 1 完成后必须手动测一次）：

> `getRowCount` 依赖 `w.bufWidth` 与 `w.Buf.Settings`。preRender 在 `relocateVerticalMD` 入口跑（早于本帧 `Display` 的 `updateDisplayInfo`），但**晚于前一帧 Display**，所以 bufWidth 来自上一帧 Display 的赋值。

代码核查后的时序保证（`cmd/micro/micro.go:487-498`）：
- 主循环：先 `Tabs.Display()`（含 `BufWindow.Display` → `updateDisplayInfo` 设置 bufWidth）→ 等待事件 → 事件处理中调用 `h.Relocate()` → `relocateVerticalMD`
- 首次 Display 之后，bufWidth 永远有效。**首屏 Render 之前**没有事件可触发 Relocate

手动验证步骤：
1. 开 softwrap 的 MD 文件
2. 调小 terminal 宽度（触发 resize）→ 等一帧 Display → 按 ↓ 跨段
3. 观察光标滚动行为是否正确
4. 如不正确：插入 `log.Printf("bufWidth=%d", w.bufWidth)` 到 preRender 入口确认

若发现 bufWidth=0（理论上不会发生），退化方案：preRender 开头加 `if w.bufWidth == 0 { return }`，跳过本次 preRender，Relocate 走 fallback 兜底。

### 6.5 上滚对称覆盖

方案 A 的 bug 描述聚焦下滚，但**上滚对称**也成立：
- 光标在 line 60（table 外，MD 渲染 9 行）→ 按 ↑ 进入 line 59 → table 改 native 渲染 5 行 → viewport 上半"塌缩" → 下一帧按 ↑ 仍用错的地图

preRender 不区分方向（只看 c.Line 落在哪个 segment），**一次修复两种方向**。

### 6.6 多光标

按"只看主 cursor"实现。`prevCursorY` 只追踪 `activeCursor`；preRender 模拟"主光标在 c.Line" 状态。其他光标位置不影响 editMode 判定（editMode 本就只看主 cursor 所在段）。**够用**。

### 6.7 editMode toggle 时序（ESC 场景）

**用户提出的潜在场景**：
- 光标在长 table 底部，editMode=true，table native 渲染（占 N 行）
- 按 ESC，editMode 切到 false，table 改 MD 渲染（占 N+M 行，M 为 frame/separator 行数）
- table 向下扩展，把光标挤出 viewport 底部
- 担心：方案 A 触发条件能否处理这个场景？

**结论：方案 A 触发条件已足够，不需要额外处理**。

详细分析：

**ESC 后的状态序列**：
1. `observeEditModeToggle` 触发 → `w.editMode = false`
2. ESC 自身**不调用 Relocate**
3. `Display()` 自动跑 → 用 editMode=false 渲染 → **map 已反映新布局**（fresh map）
4. 光标可能已出 viewport，但 map 自洽（map 内的 cursor 位置是准确的）

**后续任意按键**（↓↑字母键）：
1. `DoKeyEvent` 运行 action → 触发 Relocate
2. Relocate 走方案 A 触发条件：`w.editMode && prevSeg != newSeg`
3. **editMode=false 短路 → preRender 不跑**
4. `LineToScreenRow(c.Line, c.Row)`：
   - 若 c.Line 在 map 屏幕行范围内 → 命中，正常逻辑
   - 若 c.Line 出 viewport → 返回 false → 走 `relocateVerticalNativeFallback`
5. nativeFallback 用 1:1 算术：`StartLine = Scroll(c, -height+1+scrollmargin)` → **光标回到 viewport**
6. `observeEditModeToggle` 触发 → editMode=true（非 ESC 键总是 true）
7. Display 用 editMode=true 重渲染 → map 更新，cursor 可见

**为什么方案 A 触发条件不专门处理 editMode toggle**：
- 触发条件 `w.editMode && prevSeg != newSeg` 已经覆盖 V2 bug 场景
- editMode toggle 场景下 map 总是 fresh（每次 Display 都重算），不存在「用旧 map 决策」问题
- 即使光标出 viewport，nativeFallback 能兜底（虽然不完美，但够用）
- 强行加 editMode 触发的特殊处理会增加复杂度，收益小

**`prevCursorY` 在 editMode=false 时的「过时」状态是安全的**：

用户可能担心：ESC 之后 `prevCursorY` 仍记录 ESC 前的光标位置，下次 Relocate 时这个值与新光标位置产生的 `prevSeg`/`newSeg` 比较会"错乱"。

不需担心：
1. ESC 后的第一次 Relocate 之前，Display 必跑一帧（主循环固定时序），这一帧**末尾会更新** `prevCursorY = activeCursor.Y`
2. 所以到 Relocate 被触发时，`prevCursorY` 已经是"ESC 后位置"
3. 但即使不更新（理论上不会发生），editMode=false 时触发条件直接短路，prevCursorY 完全不被读 → **不生效**
4. `prevCursorY` 是 BufWindow 实例字段，与 editMode 切换独立维护，不需要在 `observeEditModeToggle` 里 reset

**连续 ESC→↓ 场景**：
- ESC 1 次 → editMode=false、Display 跑（prevCursorY 更新为 ESC 后位置）→ map fresh
- ↓ 键 → action → Relocate → editMode 守卫为 false → 不 preRender → 走正常分支（map 准，LineToScreenRow 命中）或 fallback（cursor 出 viewport）
- **无两帧不同步问题**

**关于原草稿的 `w.prevCursorY = -1` 兜底建议**：

```go
// observeEditModeToggle 末尾
w.prevCursorY = -1  // 强制下次 Relocate 必走 preRender  ← 这是错的
```

在原触发逻辑下：`findSegmentContaining(-1)` 返回 nil → `prevSeg == nil` → `prevSeg != nil` 为 **false** → `needPreRender` 为 **false**。这行代码**不会**强制 preRender，是**无效建议**，**删除**。

### 6.8 首帧行为

`prevCursorY` 初始值 = 0。第一次 Relocate 时 `prevSeg` 可能是 nil（光标在 0 行，segment 从 0 开始的情况）或 line 0 所在 seg；多数情况会触发 preRender。**多花一次成本，无副作用**（首帧本来就用 nativeFallback 兜底，preRender 不破坏兜底路径）。

---

## 7. 不变性与代价

**不变量**（必须保证）：
- ✅ `internal/md/render_*.go` 零改动
- ✅ micro 原生 `displayBuffer()` 零改动
- ✅ `internal/action/actions.go` 零改动
- ✅ `Buf.MDSegments` 缓存机制零改动
- ✅ `softwrap.go`（Scroll / Diff / SLocFromLoc）零改动

**代价**：
- 跨段按键：1 次额外 dry-run 渲染（不写屏）
- 非跨段按键：0 开销（早退）
- 不跨段不增加任何成本

**性能基准**：当前 `displayBufferMD` 在 MD 文件上每帧约 O(bufHeight) = O(50)；preRender 多花的成本最多 3 倍上界 = 150 次循环，单次按键级别，**毫秒级**。

---

## 8. 与方案 B / B' 对比

| 方案 | 核心做法 | 侵入度 | 根治 V2 | 修跨段消失帧 | 风险 |
|------|---------|--------|--------|------------|------|
| **A（本方案）** | Relocate 入口插 preRender 钩子；Display 主循环不动 | ⭐⭐ | ✅ | ✅ | 低 |
| B（彻底重构） | Display 改 blit；preRender 永远先跑 | ⭐⭐⭐⭐⭐ | ✅ | ✅ | 高（违反"侵入最小"）|
| B'（影子渲染） | render 函数拆 computeLayout/blit；Display 可选复用 preRender 缓存 | ⭐⭐⭐ | ✅ | ✅ | 中 |

**A 与 B' 收敛**：本质都是"决策前先渲染"，A 的范围更小（不动 Display），B' 想顺手吃 B2 部分收益但代价更高。**先 A，看实际效果再决定是否升级到 B'**。

---

## 9. 风险与待确认

### 9.1 已知风险

| 风险 | 等级 | 缓解 |
|------|------|------|
| dryRun 路线 C（getRowCount）与完整 native 主体在边界场景（softwrap / 多光标 selection）行为不一致 | 中 | §6.2 路线 C 已选，与 `SLocFromLoc` 同源理论上 100% 一致；softwrap 模式下手动测一遍跨段 |
| `b.MDSegments` 在 buffer 极端编辑（大量增删）后未及时更新 | 低 | buffer.go:211-212 已保证事件驱动更新，不动此层 |
| 多窗口同时打开同一 buffer 时 `prevCursorY` 各窗口独立、切换时可能短暂不一致 | 中 | 各 BufWindow 实例的 `prevCursorY` 字段独立；切换窗口后第一次按键的 Relocate 会自然修正（可能多花一次 preRender）。在 `BufWindow` 加 `prevCursorY` 是必须的：不能用 Buf 级别字段，否则两窗口会互相覆盖 |

### 9.2 待拍板的 3 件事

1. **native 渲染器 dryRun 路线**：§6.2 三条路线（A 完整分支跳过 / B 短路累加 / C getRowCount 累加）。**推荐 C**——与 `SLocFromLoc` 同源、代码最少、零增侵入。详见 §6.2。
2. **触发条件**（已定）：`w.editMode && prevSeg != newSeg`（指针比较 + editMode 守卫）。Normal↔Normal 存在伪触发（preRender 多跑一次但 map 不变），接受这个 ~0 成本的浪费，换代码简洁。
3. **editMode toggle 兜底**：用户提出 ESC 后光标可能被挤出 viewport 的场景。经分析：方案 A 触发条件 `w.editMode && ...` 在 editMode=false 时短路 + nativeFallback 兜底 + 下次按键的 Relocate 已能解决。**不增加额外兜底代码**。详见 §6.7。

### 9.3 已知限制（与 v1.0.5 总结文档 §5 重叠）

- 跨段切换时光标"消失一帧"：本方案**修复**（preRender 把地图算准后 scroll 量对，cursor 始终在 viewport 内）。
- 跨段进入 MD 段的首次按键走 fallback：本方案**修复**（地图准了，LineToScreenRow 命中，不再 fallback）。
- `internal/display` 包测试缺失：本方案 Commit 2 补测试（`ScreenRowToLine` / `LineToScreenRow` / 新增 `preRenderAtCursor` / `findSegmentContaining`）。

---

## 10. 实施步骤（待确认后执行）

1. **Commit 1**：核心修复
   - `renderSegmentMD` 加 `dryRun bool`
   - `renderSegmentNative` **零改动**
   - **新增** `renderSegmentNativeDryRun(seg, startVY) (newVY int)`（§6.2 路线 C 实现）
   - 新增 `preRenderAtCursor` + `findSegmentContaining`
   - `relocateVerticalMD` 入口加跨段检测
   - `BufWindow` 加 `prevCursorY` + `displayBufferMD` 末尾更新
2. **Commit 2**：测试补充
   - `internal/display/bufwindow_md_test.go`（C4 2D 后已删除，需重建）
   - 覆盖：跨段 preRender 触发、dryRun viewportRowmap 正确性、prevCursorY 状态机

**回归测试清单**（Commit 2 必须覆盖的最小场景集合）：

- [ ] **跨段下滚**（Normal→Table）：光标在 Normal 段末行，按 ↓ 进入 Table 段 → 期望光标落在 viewport 底部滚动区
- [ ] **跨段上滚**（Table→Normal）：光标在 Table 段首行，按 ↑ 进入上一 Normal 段 → 期望光标落在 viewport 顶部滚动区
- [ ] **跨段进入 MD 段**（Codeblock→Heading）：cursor 从 codeblock 移出后跨入 Heading 段 → 期望不再走 fallback、LineToScreenRow 命中
- [ ] **preRender 地图正确性**：调用 `preRenderAtCursor(c.Line)` 后，遍历 `viewportRowmap` 验证：(a) 段内 `Row >= 0` 正确反映 softwrap 折行 (b) 装饰行 `Line: -1` 位置正确 (c) bufHeight 之外不写
- [ ] **editMode toggle 后**：ESC 切到阅读模式，map fresh；按 ↓ 后光标位置正确（§6.7）
- [ ] **首帧行为**：`prevCursorY=0`（初始值）触发 preRender 不崩，oldSegDone 立即 true
- [ ] **scrollmargin=0**：光标贴边即触发滚动，preRender 目的 1.b 简化为"光标行已渲染即可"
- [ ] **softwrap 长行**：单 buffer 行折成多屏行时，preRender 生成的 map 与正常 Display 的 map 一致
- [ ] **Normal↔Normal 伪触发**：相邻 Normal 段光标移动触发 preRender → map 实际不变 → 浪费一次渲染但行为正确
- [ ] **多窗口同 buffer**：两个 BufWindow 都打开同一 MD，各窗口 prevCursorY 独立，切换窗口后第一次按键不出现布局跳变
- [ ] **段内移动不触发**：同一 Heading 段内光标上下移动，preRender 不被调用
- [ ] **oldSeg 在 viewport 外**：旧光标段位于 StartLine 之前，oldSegDone 立即 true，不过度渲染
- [ ] **`bufWidth=0` 退化场景**（§6.2 退化方案）：preRender 入口检测 `w.bufWidth == 0` 时提前 return、不刷 map，Relocate 走 `relocateVerticalNativeFallback` 兜底
3. **Commit 3**（可选）：清理
   - 评估是否还需要 `relocateVerticalNativeFallback`（preRender 后多数兜底场景不再命中）

---

## 11. 关联文档索引

- `docs/光标滚动-方案A.md` —— 原始问题描述与 preRender 设想（v1.0.5 时期，细节缺失）
- `docs/光标滚动-修改总结.md` §5 —— v1.0.5 已知限制（本方案直接对应）
- `docs/方案B评估报告mm.md` —— 排除 B2、收敛到 B'（与本方案思路一致）
- `docs/方案B评估报告glm.md` —— 推荐 B' 影子渲染（本方案是其最小化版本）
