# 方案 A 架构设计（preRender-dryRun）

> 评估对象：`docs/光标滚动-方案A.md` 中的「preRender + dryRun」路线
> 关联：`docs/光标滚动-修改总结.md` §5（v1.0.5 已知限制）、`docs/方案B评估报告mm.md`（已排除 B2，参考 B' 思路收敛到 A）
> 约束：侵入 micro 原生代码越小越好；render 函数（`internal/md/render_*.go`）零改动
> 日期：2026-06-16

---

## 0. 结论

**方案 A 是性价比最高的 V2 修复路径。** 一句话：在 `relocateVerticalMD` 入口插一道「跨段检测 + 影子渲染」钩子，用新鲜地图算 scroll；Display 主循环、render 函数、actions.go、buffer 缓存机制全部不动。

刀只落在 `internal/display/bufwindow_md.go` 一个文件 + `bufwindow.go` 加 2 行字段。

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

> **触发条件**：光标跨 segment 边界（仅此时地图会失效）
> **修复动作**：在 `relocateVerticalMD` 入口，对"假设光标停在 c.Line"的状态做一次 dry-run 渲染，重建 `viewportRowmap`
> **后续决策**：用这份新鲜地图喂给原有的 `LineToScreenRow` + `viewportRowmap[delta]` 逻辑，scroll 量自然算对

**与方案 B' 收敛**：评估报告里 glm 推荐的"影子渲染"本质就是方案 A；A 文档（v1.0.5 时期）漏掉了 native 渲染器的 dryRun 处理和 prevCursor 跟踪的细节，本设计补齐。

---

## 3. 三个关键组件

### 3.1 组件① Render 层天然分层（无需重构）

回看现状：
- `seg.Render(seg, width, cfg)` → `*RenderedSegment`（**纯函数**，只产 `Rows`，不碰 screen）
- `renderSegmentMD()` 拿到 `RenderedSegment` 后才写 `screen.SetContent` + `viewportRowmap[vY]`

计算与绘制**本来就分层**了。dryRun 的本质：跳过 `screen.SetContent` 那一步，只走 `viewportRowmap` 写入。

→ **render 函数（`internal/md/render_*.go`）零改动**，dryRun 适配全在 `renderSegmentMD` 内部加参数。

### 3.2 组件② 触发条件：prevCursorY 跟踪

BufWindow 加一个字段：

```go
prevCursorY int  // 每帧 Display 末尾更新为 activeCursor.Y
```

触发判定（在 `relocateVerticalMD` 入口，5 行）：

```go
prevSeg := w.findSegmentContaining(w.prevCursorY)
newSeg  := w.findSegmentContaining(c.Line)
needPreRender := prevSeg != nil && newSeg != nil && prevSeg != newSeg
if needPreRender {
    w.preRenderAtCursor(c.Line)
}
```

`findSegmentContaining` 是 O(n) 线性扫 `b.MDSegments`（n = segment 数，几十以内，开销忽略）。

> **指针比较的合法性**：`DetectSegments` 一次产出后缓存在 `Buf.MDSegments`，每次 buffer 编辑才重跑。同一份 segments 的指针稳定，可直接 `==`。

### 3.3 组件③ preRender 函数（新增）

```go
// preRenderAtCursor 渲染"假设光标在 cursorLine"状态的 viewportRowmap。
// 起点：当前 StartLine；范围：[StartLine.Line, StartLine.Line + 3*bufHeight)
// 渲染规则：
//   - cursorLine 所在 segment → renderSegmentNative（dryRun=true）
//   - 其他 segment            → renderSegmentMD       （dryRun=true）
// 停止条件：bufHeight + scrollmargin 行已写入 且 cursorLine 对应屏行已确定
func (w *BufWindow) preRenderAtCursor(cursorLine int)
```

---

## 4. 数据流（修复后）

```
用户按 ↓
    ↓
actions.go: CursorDown() → MoveCursorDown(1)   ←  c.Loc 变了（不变）
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
Display() → displayBufferMD() 正常渲染 + 末尾更新 prevCursorY = activeCursor.Y
```

---

## 5. 关键代码改动点

| 文件 | 改动 | 行数估计 |
|------|------|---------|
| `internal/display/bufwindow_md.go` | `renderSegmentMD` 加 `dryRun bool` 参数；dryRun=true 时跳过 `screen.SetContent` + `expandLineStyles` + cell style 合并；只写 `viewportRowmap` | +20 |
| `internal/display/bufwindow_md.go` | `renderSegmentNative` 整体加 `dryRun bool` 参数；dryRun=true 时**短路**为「1:1 累加 viewportRowmap」循环（见 §6.2 详述） | +30 |
| `internal/display/bufwindow_md.go` | 新增 `preRenderAtCursor(cursorLine int)` | +50 |
| `internal/display/bufwindow_md.go` | 新增 `findSegmentContaining(line int) *md.Segment` | +10 |
| `internal/display/bufwindow_md.go` | `relocateVerticalMD` 入口加跨段检测 + 触发 preRender | +8 |
| `internal/display/bufwindow.go` | `BufWindow` 加 `prevCursorY int` 字段 | +1 |
| `internal/display/bufwindow.go` | `displayBufferMD` 末尾（或 Display 末尾）更新 `prevCursorY = activeCursor.Y` | +2 |
| `internal/md/render_*.go` | **零改动** | 0 |
| `internal/action/actions.go` | **零改动** | 0 |
| `internal/display/softwrap.go` | **零改动** | 0 |
| `internal/buffer/buffer.go` | **零改动** | 0 |

**总侵入**：~120 行，全在 display 包内，且 micro 原生 `displayBuffer()` 不动。

---

## 6. 设计细节（拍板点）

### 6.1 preRender 里 cursor 段走 native 还是 MD？

**native**。原因：跨段瞬间，新光标所在 segment 在 Display() 里就是用 native 渲染的（`editMode=true` + `hasCursorInside` 命中）。preRender 必须模拟同样的状态，否则地图不一致 → 后续 `LineToScreenRow` 仍可能命中错误位置。

### 6.2 native 渲染器怎么加 dryRun？

`renderSegmentNative` 是 ~370 行庞然大物（`bufwindow_md.go:210` 起到 ~580 行），里面大量 `screen.SetContent`、`screen.ShowCursor`、`expandLineStyles`、selection 高亮、cursorline、color-column 等深度耦合显示。

**两种路线**：

| 路线 | 实现 | 风险 |
|------|------|------|
| A：加 `dryRun bool` 走完整分支跳过 | 每条 `screen.SetContent` 加 if 判断；`expandLineStyles` 跳过 | ⭐⭐ 改点多但保守 |
| B：dryRun 短路为简化循环 | dryRun=true 时**整体跳过 native 主体**，改写 `viewportRowmap[vY..vY+行数]` 为 `(c.Line, row=0,1,2...)` 连续条目 | ⭐ 改点少 |

**选 B**：native 渲染下「段内每行 = 屏内一行」（无装饰行），简化的累加循环在数学上等价于跑完整 native 主体再读 viewportRowmap。代码量减半、风险更低。

```go
func (w *BufWindow) renderSegmentNative(seg md.Segment, startVY int, dryRun bool) (newVY int) {
    if dryRun {
        // 短路：1:1 累加 viewportRowmap
        for line := seg.VisibleStart; line <= seg.VisibleEnd; line++ {
            if startVY >= 0 && startVY < w.bufHeight {
                w.viewportRowmap[startVY] = SLoc{Line: line, Row: 0}
            }
            startVY++
        }
        return startVY
    }
    // ... 原 370 行代码不变 ...
}
```

### 6.3 preRender 渲染起点与上界

- **起点**：`w.StartLine.Line`（当前视口顶端）
- **上界**：`StartLine.Line + 3 * bufHeight`（粗暴上界，丢弃超出部分；避免"鸡生蛋"迭代）

3*bufHeight ≈ 150 行（终端高 50）。即便命中超大 table/codeblock（一个 segment 几百行），也只是渲染这一段然后丢弃。**性能压力可接受**：displayBufferMD 现状就是 O(bufHeight)，多 3 倍 = 一次按键成本从 O(bufHeight) 变成 O(bufHeight)，**不会卡顿**。

### 6.4 preRender 停止条件

更精准的停止条件（在 §3.3 基础上的强化）：

```
可以 break 当且仅当：
  (bufHeight + scrollmargin) 行已写入 viewportRowmap
  AND cursorLine 对应的 SLoc 已写入（即 LineToScreenRow(cursorLine, 0) 能命中）
```

否则：cursorLine 还没进入 map 时调用 `LineToScreenRow` 会失败 → 退化为 nativeFallback → V2 bug 重现。

### 6.5 上滚对称覆盖

方案 A 的 bug 描述聚焦下滚，但**上滚对称**也成立：
- 光标在 line 60（table 外，MD 渲染 9 行）→ 按 ↑ 进入 line 59 → table 改 native 渲染 5 行 → viewport 上半"塌缩" → 下一帧按 ↑ 仍用错的地图

preRender 不区分方向（只看 c.Line 落在哪个 segment），**一次修复两种方向**。

### 6.6 多光标

按"只看主 cursor"实现。`prevCursorY` 只追踪 `activeCursor`；preRender 模拟"主光标在 c.Line" 状态。其他光标位置不影响 editMode 判定（editMode 本就只看主 cursor 所在段）。**够用**。

### 6.7 editMode toggle 时序

遗留坑：`observeEditModeToggle` 在 `Relocate` 之后触发，Relocate 用的还是旧 editMode。方案 A 不解决这个时序问题，但**建议加一行兜底**：

```go
// observeEditModeToggle 末尾
w.prevCursorY = -1  // 强制下次 Relocate 必走 preRender
```

逻辑清晰、无副作用。

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
| dryRun 短路与完整 native 主体在边界场景（softwrap / 多光标 selection）行为不一致 | 中 | §6.2 路线 B 仅在段内无装饰行场景下使用；softwrap 模式下手动测一遍跨段 |
| `b.MDSegments` 在 buffer 极端编辑（大量增删）后未及时更新 | 低 | buffer.go:211-212 已保证事件驱动更新，不动此层 |
| 多窗口共享 buffer 时 `prevCursorY` 状态错乱 | 低 | 多窗口各持各的 BufWindow，prevCursorY 独立 |

### 9.2 待拍板的 3 件事

1. **native 渲染器 dryRun 路线**：A（完整分支跳过） vs B（短路累加）？推荐 **B**（代码少、风险低）。
2. **触发粒度**：「prevSeg != newSeg」（按 segment 指针比）vs「光标跨任何 segment 边界」？推荐**前者**（多数单步↑↓不跨段，0 开销）。
3. **editMode toggle 兜底**：是否在 `observeEditModeToggle` 里把 `prevCursorY = -1` 强制下次 preRender？推荐**是**（逻辑清晰）。

### 9.3 已知限制（与 v1.0.5 总结文档 §5 重叠）

- 跨段切换时光标"消失一帧"：本方案**修复**（preRender 把地图算准后 scroll 量对，cursor 始终在 viewport 内）。
- 跨段进入 MD 段的首次按键走 fallback：本方案**修复**（地图准了，LineToScreenRow 命中，不再 fallback）。
- `internal/display` 包测试缺失：本方案 Commit 2 补测试（`ScreenRowToLine` / `LineToScreenRow` / 新增 `preRenderAtCursor` / `findSegmentContaining`）。

---

## 10. 实施步骤（待确认后执行）

1. **Commit 1**：核心修复
   - `renderSegmentMD` 加 `dryRun bool`
   - `renderSegmentNative` 加 `dryRun bool`（§6.2 路线 B）
   - 新增 `preRenderAtCursor` + `findSegmentContaining`
   - `relocateVerticalMD` 入口加跨段检测
   - `BufWindow` 加 `prevCursorY` + `displayBufferMD` 末尾更新
   - `observeEditModeToggle` 末尾兜底重置
2. **Commit 2**：测试补充
   - `internal/display/bufwindow_md_test.go`（C4 2D 后已删除，需重建）
   - 覆盖：跨段 preRender 触发、dryRun viewportRowmap 正确性、prevCursorY 状态机
3. **Commit 3**（可选）：清理
   - 评估是否还需要 `relocateVerticalNativeFallback`（preRender 后多数兜底场景不再命中）

---

## 11. 关联文档索引

- `docs/光标滚动-方案A.md` —— 原始问题描述与 preRender 设想（v1.0.5 时期，细节缺失）
- `docs/光标滚动-修改总结.md` §5 —— v1.0.5 已知限制（本方案直接对应）
- `docs/方案B评估报告mm.md` —— 排除 B2、收敛到 B'（与本方案思路一致）
- `docs/方案B评估报告glm.md` —— 推荐 B' 影子渲染（本方案是其最小化版本）