# RenderedSegment 数据结构

> render 函数的输出产出物，定义在 `internal/md/md.go`

## 总览

```
RenderedSegment
  ├── BufStartLine, BufEndLine     // 片边界（buffer 行号，display 层使用绝对值）
  └── Rows []RenderedRow           // screen 行列表
        ├── BufLine                // 此 screen 行对应哪个 buffer 行
        └── Cells []Cell           // 字符列表
              ├── Rune             // 显示的字符
              ├── Combining        // 组合字符（通常 nil）
              ├── Style            // tcell 样式（颜色 + 粗斜体）
              ├── BufLine          // 对应 buffer 行号（装饰行为 -1）
              ├── BufX             // 对应 buffer 行内 rune 偏移（装饰行为 -1）
              └── IsDecorative     // true = 装饰字符，点击时忽略

SegmentMeta        // 渲染元数据，缓存到 BufWindow 用于 click 坐标映射
  ├── BufStartLine, BufEndLine     // 可见范围（绝对行号）
  └── RowBufLines []int            // 每个 screen row 的 BufLine（装饰行 = -1）

Segment            // 检测步骤的输出单位
  ├── BufStartLine, BufEndLine     // 片边界（绝对行号）
  ├── VisibleStart, VisibleEnd     // 当前 viewport 中的可见范围
  └── Render func(...)            // 渲染函数
```

## 逐层说明

### RenderedSegment

一个渲染片的完整渲染输出。

| 字段 | 类型 | 说明 |
|------|------|------|
| `Rows` | `[]RenderedRow` | 渲染后的 screen 行，数量可能多于 buffer 行（如表格有装饰行） |
| `BufStartLine` | `int` | 片起始 buffer 行号 |
| `BufEndLine` | `int` | 片结束 buffer 行号（含） |

**重要**：renderer 产出的 `BufLine` 是**相对行号**（从 0 开始），display 层会加上 `seg.BufStartLine` 转换为**绝对行号**。

### RenderedRow

渲染后的一行屏幕输出。

| 字段 | 类型 | 说明 |
|------|------|------|
| `Cells` | `[]Cell` | 这一行的所有字符，长度 = 屏幕宽度 |
| `BufLine` | `int` | 对应的 buffer 行号。首行有值，续行和装饰行为 `-1` |

BufLine 用于行号显示判断：有值则显示行号，`-1` 则行号位留空。复用 softwrap 的多行规则。

### Cell

渲染管线输出的最小单位，一个屏幕字符。

| 字段 | 类型 | 说明 |
|------|------|------|
| `Rune` | `rune` | 要显示的字符 |
| `Combining` | `[]rune` | 组合字符（如变音符号），通常为 nil |
| `Style` | `tcell.Style` | 颜色和字体样式。颜色来自 Micro 语法高亮，render 叠加粗体/斜体 |
| `BufLine` | `int` | 对应 buffer 行号。装饰字符为 `-1` |
| `BufX` | `int` | 对应 buffer 行内的 rune 偏移（与 `buffer.Loc.X` 一致）。装饰字符为 `-1` |
| `IsDecorative` | `bool` | `true` = 装饰字符（如表格边框），鼠标点击时忽略 |

### SegmentMeta

检测步骤输出的轻量元数据，缓存到 `BufWindow.mdCache` 上供 Scroll/Diff 使用。

| 字段 | 类型 | 说明 |
|------|------|------|
| `BufStartLine` | `int` | 可见范围起始行（绝对行号） |
| `BufEndLine` | `int` | 可见范围结束行（绝对行号） |
| `RowBufLines` | `[]int` | `RowBufLines[i]` = 第 i 个 screen row 的 BufLine（装饰行 = -1） |

`RowBufLines` 用于 `screenOffsetToBufferLine()` 将屏幕坐标映射回 buffer 位置。

### Segment

检测步骤的输出单位，每一行 buffer 都属于某个 Segment。

| 字段 | 类型 | 说明 |
|------|------|------|
| `BufStartLine` | `int` | 片起始 buffer 行（绝对行号） |
| `BufEndLine` | `int` | 片结束 buffer 行（绝对行号，含） |
| `VisibleStart` | `int` | 当前 viewport 中的实际起始行（由 `filterSegmentsToVisible` 设置） |
| `VisibleEnd` | `int` | 当前 viewport 中的实际结束行 |
| `Render` | `func(seg Segment, width int, cfg MDConfig) *RenderedSegment` | 渲染函数，接收完整 Segment 从 `cfg.Buf` 取 lines |

Cell 级别的 `(BufLine, BufX)` 用于鼠标点击反向定位到 buffer 位置。

## 具体示例：表格渲染片输出

假设 buffer 第 15-17 行是一个 3 行的 Markdown 表格，render 后可能输出如下：

```
RenderedSegment {
    BufStartLine: 15,
    BufEndLine:   17,
    Rows: [
        Rows[0]: BufLine=-1  →  ┌─────────────────┬────────────────────┐   ← 表格顶边框（装饰行）
        Rows[1]: BufLine=15  →  │ 项目            │ 说明               │   ← 首行显示行号 15
        Rows[2]: BufLine=15  →  │                 │ （续行）           │   ← 续行，行号留空
        Rows[3]: BufLine=16  →  │ microNeo        │ 一个好玩的编辑器   │   ← 显示行号 16
        Rows[4]: BufLine=17  →  │ 这个名字         │ 这次要换了          │   ← 显示行号 17
        Rows[5]: BufLine=17  →  │ 比较长           │                    │   ← 续行，行号留空
        Rows[6]: BufLine=-1  →  └─────────────────┴────────────────────┘   ← 表格底边框（装饰行）
    ]
}
```

关键点：

- **Rows 只描述"画什么"**，共 7 行 screen 输出，由 3 行 buffer 扩展而来
- **BufLine=-1** 是装饰行（表格边框），没有对应 buffer 行，行号区留空，点击忽略
- **BufLine 相同的连续行**是续行（同一 buffer 行因内容 wrap 产生的额外 screen 行），只有首行显示行号
- **显示在哪一行（Y 坐标）不由 RenderedSegment 决定**，而是由 `displayBufferMD()` 的显示循环在遍历各渲染片时累加 Y 坐标决定
- 边框使用 Unicode 框线字符（`┌─┬─┐`、`│`、`├─┼─┤`、`└─┴─┘`），所有边框 Cell 的 `IsDecorative=true`

## Row.BufLine vs Cell.BufLine

两处都有 BufLine，用途不同：

- **Row.BufLine**：行号区显示判断——有值显示行号，`-1` 留空
- **Cell.BufLine**：点击定位——查表得到 `(bufLine, bufX)`，光标定位到 buffer 位置

## CJK 宽字符处理

宽字符（占 2 列）后自动补一个空占位 Cell，保持背景色连续和对齐正确：

```
真实 Cell:  Rune='你', BufX=3, IsDecorative=false
占位 Cell:  Rune=' ',  BufX=-1, IsDecorative=false（不算装饰，但无 buffer 映射）
```

占位 Cell 的 BufX 为 `-1`，点击时跳过。

## 生命周期

- **每帧即弃**：RenderedSegment 不持久化，每帧由 render 函数重新生成
- **纯数据**：render 函数不碰 tcell screen，只产出数据，由 `displayBufferMD()` 负责写入 screen
- **BufLine 转换**：renderer 产出相对行号（从 0 开始），`renderSegmentMD()` 统一加上 `seg.BufStartLine` 转为绝对行号

## 元数据缓存

`SegmentMeta` 会被缓存到 `BufWindow.mdCache`，用于：

1. **点击坐标映射**：`screenOffsetToBufferLine()` 查 `RowBufLines` 将 screen Y 坐标映射回 buffer 行号
2. **滚动同步**：`Scroll` / `Diff` 等操作可利用缓存的元数据快速定位

缓存每帧重建（`displayBufferMD` 开始时 `mdCache = mdCache[:0]`），避免无限增长。
