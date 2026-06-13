# C2 光标滚动 —— buffer 级映射方案（彻底收敛）

> 关联：调研依据见 `docs/C0-光标滚动-现状调研.md`；止血方案见 `docs/C1-光标滚动-段级前缀和方案.md`。
> 本文是**彻底方案**：修根因的同时收敛架构，把"屏幕行 ↔ buffer 行"的映射建立为**唯一真源**，消除当前三套并存数据的割裂。

---

## 0. 定位

- **修什么**：与 C1 相同的 bug 范围（`Relocate`/`Scroll`/`Diff`/`SLocFromLoc` + 全部受影响场景）。
- **怎么修**：建立 buffer 级 `screenRow ↔ bufLine` 双向映射为唯一真源，`viewportRowBufLine` 降级为它的一个视图，`SegmentMeta` 死代码被吸收。
- **与 C1 的区别**：C1 在碎片化数据上再叠一层段级聚合（`segRowCount`），修了 bug 但架构更乱；C2 把整屏/段级/死代码三套收敛为一套，修 bug 的同时清架构债。
- **代价**：改动比 C1 大一档——要把 display 时的扁平化逻辑上提成 buffer 级、并接上增量更新。

---

## 1. 出发点：架构直觉 + 现状核对

### 1.1 一个一直存在的担心

microNeo 早期的位置计算都基于"遍历一个或多个 `MDSegment` 碎片"组合得出。但 **screen 是一个整体**——它本应有一个整体的"screen row → line"数据，让位置计算变成直接查表，而非扫描遍历渲染片。

后期为此加入了 `viewportRowBufLine`（整屏模型），位置查询（click/上下键映射）确实因此变简单了。但它引入较晚，且有两个结构性限制（见下），导致滚动模型一直没接上它。

### 1.2 现状核对（详见 C0 §2.3）

经代码核查，当前与"屏幕行 ↔ buffer 行"相关的数据有三类：

| 数据 | 作用域 | 生命周期 | 服务对象 |
|------|--------|---------|---------|
| `MDSegments` | 全 buffer（碎片） | reHighlight 时重建 | 仅渲染管线（**生产者**） |
| `viewportRowBufLine` | **仅当前视口** `[0,height)` | `displayBufferMD`（`Relocate` **之后**） | click / 上下键映射 |
| `SegmentMeta` | — | — | **死代码，零引用** |

**`viewportRowBufLine` 的两个硬限制**，正是它没法服务滚动的根本原因：

1. **作用域太窄**：只覆盖当前视口 `[0, height)`，回答不了"视口下方第 5 个屏幕行是哪个 buffer 行"——而 `Scroll`/`Diff` 天然要跨视口。
2. **生命周期错位**：只在 `displayBufferMD`（即 `Relocate` **之后**）重建——这正是 C0 §4.1 的时序坑：`Relocate` 执行时它还是上一帧的数据。

→ 所以滚动模型既不能直接复用 `viewportRowBufLine`，也不该另起炉灶再叠一层（C1 的做法）。正确做法是把 `viewportRowBufLine` 的"整屏直接查表"思想**从视口级提升到 buffer 级**。

---

## 2. 终态架构

```
                    ┌─────────────────────────────────────────┐
                    │  buffer 级 screenRow ↔ bufLine 双向映射   │  ← 唯一真源
                    │  （按段增量更新，编辑时只重算脏段）        │
                    └───────────────────┬─────────────────────┘
                          ↙             │              ↘
              视口窗口            滚动模型            百分比/滚动条
       viewportRowBufLine         Scroll/Diff          statusline
       (= 当前 StartLine           SLocFromLoc          scrollbar
          对应的那个切片)          Relocate
```

在这个终态下：
- **唯一真源**是一个 buffer 级映射，覆盖整篇 buffer 的所有屏幕行（含装饰行）。
- **`viewportRowBufLine` 不再独立构建**，而是这个 buffer 级映射在当前 `StartLine` 处的一个切片（`[screenRowOf(StartLine), screenRowOf(StartLine)+height)`）。天然一致，不可能再有"两套数据对不上"。
- **滚动函数**（`Scroll/Diff/SLocFromLoc`）从同一个源取数 → C0 §4.1 的时序坑消失（buffer 级映射在 `Relocate` 时就有效，不依赖 display）。
- **`SegmentMeta` 死代码**被这个结构吸收：段的屏幕行数就是映射的天然副产品，不再需要单独的类型。

---

## 3. 核心数据结构

一个 `screenRow ↔ bufLine` 双向映射，按段组织以支持增量更新：

```go
// BufWindow 上的唯一真源（替代 C1 的 segRowCount/segStartRow，也替代 viewportRowBufLine 的独立构建）
type screenLineMap struct {
    // 按段存储：每段 [startScreenRow, endScreenRow) 与 [bufStartLine, bufEndLine] 对齐
    segStartRow []int // 前缀和：segStartRow[i] = 第 i 段之前累计的屏幕行数
    segRowCount []int // segRowCount[i] = 第 i 段的屏幕行数（含装饰行）
    gen         uint64 // 代际：b.MDSegments 引用变化 / bufWidth 变化时自增
}
```

双向查询：

```
screenRowOf(bufLine)   // buffer 行 → 它在屏幕上的"首屏幕行"绝对位置（段内可 O(1) 或 O(log)）
bufLineOf(screenRow)   // 屏幕行 → 所属 buffer 行（二分定位段，再段内查；装饰行归属相邻内容行，见 C0 §4.2）
```

> 为什么按段组织而非扁平 `[]int`：buffer 可能很大，扁平数组每次编辑都要整体重建；按段则可只重算受影响段（见 §4）。段是 `b.MDSegments` 已有的结构，零额外检测成本。

---

## 4. 增量更新

**关键：编辑只影响局部段，不全量重建。**

- `b.MDSegments` 在 reHighlight 时重建（`buffer.go:212` / `:1032`）。重建本身已是按可见范围/全文 detect，会产生新段列表。
- 映射与之对齐：比对新旧段，**只重算 bufLine 范围发生变化的段**的 `rowCount`，重建该段之后的前缀和。
- 视口内的段行数：渲染时免费获得（同 C1 §3，`len(rowBufLines)`）。
- 视口外的段行数：懒算 + 缓存（同 C1 §3）。

**导航期间**（纯光标移动）：`b.MDSegments` 不变 → 映射稳定，`Relocate`/`Scroll` 全部 O(log) 命中缓存，零渲染开销。这正是 C0 §4.4 指出的稳定窗口。

---

## 5. `viewportRowBufLine` 的降级

当前 `viewportRowBufLine` 在 `displayBufferMD` 里独立构建（`bufwindow_md.go:706-734`）。改造后：

```go
// 旧：displayBufferMD 渲染时边写屏幕边构建 viewportRowBufLine
// 新：displayBufferMD 渲染前，从 buffer 级映射切一片
func (w *BufWindow) viewportRowBufLineView() []int {
    start := w.screenRowOf(w.StartLine.Line)
    return w.bufLineOfRange(start, start+w.bufHeight) // buffer 级映射的切片
}
```

- click / 上下键映射的消费者（`screenOffsetToBufferLine` / `bufferLineToScreenOffset` / `LocFromVisual`）**签名不变**，只是底层从独立数组变成映射切片——它们天然一致。
- `displayBufferMD` 的渲染主循环也不再需要边渲染边收集 `rowBufLines`，渲染与映射职责分离。

---

## 6. 滚动函数的接入（与 C1 相同的接入点，不同的底层数据源）

`Scroll/Diff/SLocFromLoc`（`softwrap.go`）入口加 MD 分支：

```go
func (w *BufWindow) Scroll(s SLoc, n int) SLoc {
    if w.Buf.IsMD && w.mdConfig.MDRender && !softwrap {
        return w.scrollMD(s, n) // 底层查 buffer 级映射，而非 C1 的 segStartRow
    }
    // 原生路径，一字不改
}
```

`Relocate` 主体仍**一行都不用改**——只要它依赖的 `Scroll/SLocFromLoc/Diff` 在 MD 下变正确即可。

> 与 C1 的差别仅在底层：C1 查 `segStartRow`（段级聚合，与 `viewportRowBufLine` 并存），C2 查 buffer 级映射（`viewportRowBufLine` 是它的切片，同源）。接入点和函数签名可以保持一致，所以**从 C1 迁移到 C2 主要是换底层数据源**。

---

## 7. 收益与代价

**收益**
- 修好 C0 §3 的全部场景（与 C1 相同）。
- **架构收敛**：`MDSegments`（生产者）/ buffer 级映射（唯一真源）/ `viewportRowBufLine`（映射的视图）三者职责清晰，不再有"两套并行数据对不上"的风险。
- **清除死代码**：`SegmentMeta` 被吸收。
- **消除时序坑**：buffer 级映射在 `Relocate` 时刻就有效，不再有 C0 §4.1 的限制。
- **为未来扩展铺路**：任何新的"屏幕行 ↔ buffer 行"需求（如精确的行号显示、视口对齐、animatescroll）都有统一入口。

**代价 / 风险**
- 改动面比 C1 大：要把 `displayBufferMD` 的扁平化构建逻辑上提、改 `viewportRowBufLine` 为视图、设计增量更新与代际失效。
- 首次实现需谨慎处理：段边界、空 buffer、未闭合段、`bufWidth` 变化重算、超大 buffer 的内存（按段存储已大幅缓解）。
- 需要更完整的测试矩阵（编辑后增量更新正确性是重点）。

**与项目原则的关系**：本方案改动集中在 `bufwindow_md.go` 与 `softwrap.go` 的三个 MD 分支，**不修改 `Relocate`/`View`/文件持久化/分屏等原生代码**，符合"少改 micro 原生代码"。与 C0 中曾出现的"方案 C（把 `StartLine` 升级为屏幕行坐标）"不同——那个会牵动大量原生代码，本方案不动 `StartLine` 的语义。

---

## 8. 迁移路径：C1 → C2

两份方案不是非此即彼，C1 是 C2 的可复用子集：

1. **可先上 C1 止血**：用最小改动验证"让 `Scroll/Diff/SLocFromLoc` 走真实屏幕行"这个核心修复思路是否正确、是否能修好全部场景。C1 的 `scrollMD/diffMD/screenRowOfLine` 三个函数签名在 C2 可直接复用。
2. **再以 C2 收敛**：把 C1 的 `segRowCount/segStartRow`（段级）与 `viewportRowBufLine`（视口级）合并为 buffer 级映射，`viewportRowBufLine` 降级为视图，吸收 `SegmentMeta`。底层换源，上层接入点不变。

也可评估后**直接做 C2**——跳过 C1 的"先叠一层再合并"，但需承担一次性更大的改动与验证成本。取舍点：当前 bug 的紧迫程度 vs 对架构债的容忍度。
