# RenderedSegment 数据结构

> render 函数的输出产出物，定义在 `internal/md/md.go`

## 总览

```
RenderedSegment
  ├── BufStartLine, BufEndLine     // 片边界（buffer 行号）
  └── Rows []RenderedRow           // screen 行列表
        ├── BufLine                // 此 screen 行对应哪个 buffer 行
        └── Cells []Cell           // 字符列表
              ├── Rune             // 显示的字符
              ├── Combining        // 组合字符（通常 nil）
              ├── Style            // tcell 样式（颜色 + 粗斜体）
              ├── BufLine          // 对应 buffer 行号（装饰行为 -1）
              ├── BufX             // 对应 buffer 行内 rune 偏移（装饰行为 -1）
              └── IsDecorative     // true = 装饰字符，点击时忽略
```

## 逐层说明

### RenderedSegment

一个渲染片的完整渲染输出。

| 字段 | 类型 | 说明 |
|------|------|------|
| `Rows` | `[]RenderedRow` | 渲染后的 screen 行，数量可能多于 buffer 行（如表格有装饰行） |
| `BufStartLine` | `int` | 片起始 buffer 行号 |
| `BufEndLine` | `int` | 片结束 buffer 行号（含） |

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

Cell 级别的 `(BufLine, BufX)` 用于鼠标点击反向定位到 buffer 位置。

## 具体示例：表格渲染片输出

假设 buffer 第 15-17 行是一个 3 行的 Markdown 表格，render 后可能输出如下：

```
RenderedSegment {
    BufStartLine: 15,
    BufEndLine:   17,
    Rows: [
        Rows[0]: BufLine=-1  →  |----------|----------------------|    ← 表格外框（装饰行）
        Rows[1]: BufLine=15  →  |  项目    |  说明                |    ← 首行显示行号 15
        Rows[2]: BufLine=15  →  |          |  （续行）            |    ← 续行，行号留空
        Rows[3]: BufLine=16  →  | microNeo |  一个好玩的编辑器   |    ← 显示行号 16
        Rows[4]: BufLine=17  →  | 这个名字 |  这次要换了         |    ← 显示行号 17
        Rows[5]: BufLine=17  →  | 比较长   |                     |    ← 续行，行号留空
        Rows[6]: BufLine=-1  →  |----------|----------------------|    ← 表格外框（装饰行）
    ]
}
```

关键点：

- **Rows 只描述"画什么"**，共 7 行 screen 输出，由 3 行 buffer 扩展而来
- **BufLine=-1** 是装饰行（表格外框），没有对应 buffer 行，行号区留空，点击忽略
- **BufLine 相同的连续行**是续行（同一 buffer 行因内容 wrap 产生的额外 screen 行），只有首行显示行号
- **显示在哪一行（Y 坐标）不由 RenderedSegment 决定**，而是由 `displayBufferMD()` 的显示循环在遍历各渲染片时累加 Y 坐标决定

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
