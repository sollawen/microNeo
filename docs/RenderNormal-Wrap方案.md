# RenderNormal Wrap 方案

> 为段落渲染器添加 word wrap 功能

## 现状

`RenderNormal` 当前直接调用 `renderLinesWithBg()`，逐字符原样输出，不做换行。一行 buffer 内容超过 `width` 时，Cell 数量超出屏幕宽度，显示溢出。

```go
// 当前实现（Step 0 占位）
func RenderNormal(lines []string, width int, cfg MDConfig) *RenderedSegment {
    return renderLinesWithBg(lines, width, normalBgStyle)
}
```

## 目标

段落渲染器按 `width` 自动换行，输出多个 RenderedRow，每个 Row 的 Cells 数量恰好 = `width`。

## 设计

### 不改 renderLinesWithBg

`renderLinesWithBg` 是 Step 0 的公共占位逻辑，其他 renderer 也在用。在 RenderNormal 里写新的 wrap 逻辑，不动它。

### width 参数的含义

render 函数接收的 `width` 是 `bufWidth`，已经减去了 gutter 和 scrollbar：

```
bufWidth = Window.Width - gutterOffset - scrollbarWidth
```

由 `renderSegmentMD` 调用时传入（`bufwindow_md.go:48`）。render 函数不用关心 gutter，只管在 `width` 列的内容区内排版。

### Tab 的处理

Tab 交给 `renderInline` 展开：renderInline 把 Tab 转成对应数量的空格 Cell，wrapCells 拿到的就是干净的 Cells，不用管 Tab。

Micro 原生处理 Tab 的方式是展开为 `tabsize - (已占列数 % tabsize)` 个占位列。在 Markdown 段落中 Tab 极其罕见，`renderInline` 简单展开为空格即可。

### 处理流程

```
输入: lines []string, width int, cfg MDConfig
输出: *RenderedSegment

对每个 buffer 行:
  1. renderInline() 把整行转成 []Cell（保留每个 Cell 的 BufLine/BufX）
  2. wrapCells() 按 width wrap 成若干 RenderedRow
  3. 每个 Row 填充到 width 列
```

### 换行算法：word wrap

```
输入: 一行的 []Cell, width, bufLine
输出: []RenderedRow

1. 按"词"分组（以空格 Cell 为分界）
2. 逐词尝试放入当前行：
   - 放得下 → 追加到当前行
   - 放不下 → 当前行补空格填到 width，开新行，词放到新行
   - 单个词超过 width → 强制按字符截断（CJK 等没有空格的语言会走这条路）
3. 每行的 Cells 数量 = width
```

#### 边界情况

- **空行**：空 buffer 行（`""`）经过 renderInline 后得到空 `[]Cell`，wrapCells 应输出**一个空的 Row**（Cells 全是填充空格），而不是零个 Row，否则空行"消失"，行号和 buffer 行的对应关系错位
- **行尾空格**：renderInline 输出末尾的空格 Cell，分词时行尾的空格丢弃（不带到下一行开头），行首的空格也丢弃（避免续行开头出现空格）
- **中文换行**：中文没有空格分词，每个字符独立成"词"，逐字换行，行为和 Micro 原版一致

### BufLine 规则

复用 softwrap 的多行规则：

| 行类型 | Row.BufLine | Cell.BufLine | Cell.BufX | 说明 |
|--------|:-----------:|:------------:|:---------:|------|
| 首行 | lineIdx | lineIdx | 原始 rune 偏移 | 显示行号 |
| 续行 | -1 | lineIdx | 原始 rune 偏移 | 行号留空，但点击定位仍有效 |
| 行尾填充 | 同上 | lineIdx | -1 | 补空格 Cell |
| CJK 占位 | 同上 | lineIdx | -1 | 宽字符后的占位 |

关键区别：
- **Row.BufLine**：用于行号显示。首行有值，续行 = -1，行号区留空
- **Cell.BufLine**：用于点击定位。续行的 Cell 仍然有正确的 buffer 行号映射

### MDConfig 加入 Colorscheme

当前各 renderer 硬编码颜色（如 `tcell.Color236`），应该从 Micro 的 colorscheme 取。md 包不能 import config（依赖方向约束），所以颜色通过 MDConfig 传入。

在 MDConfig 中新增 `MDColorscheme` 子结构：

```go
type MDConfig struct {
    // 功能开关
    MDRender      bool
    MDRenderIdle  float64
    MDTableAlign  bool
    MDTableBorder bool
    MDBoldItalic  bool
    MDCodeBlock   bool
    MDHeading     bool
    MDList        bool
    MDLink        bool

    // 颜色方案（BufPane 构造时从 config.Colorscheme 填入）
    Colorscheme MDColorscheme
}

type MDColorscheme struct {
    DefStyle tcell.Style             // config.DefStyle 快照
    Styles   map[string]tcell.Style  // "default", "comment", "statement" 等
}
```

填入时机：BufPane 构造时

```go
cfg := md.DefaultMDConfig()
cfg.Colorscheme = md.MDColorscheme{
    DefStyle: config.DefStyle,
    Styles:   config.Colorscheme,  // 直接引用 map，换主题自动生效
}
```

render 里使用：

```go
func RenderNormal(lines []string, width int, cfg MDConfig) *RenderedSegment {
    style := cfg.Colorscheme.DefStyle  // 普通段落用 default 背景
    // ...
}

func RenderCodeBlock(lines []string, width int, cfg MDConfig) *RenderedSegment {
    style := cfg.Colorscheme.DefStyle
    if s, ok := cfg.Colorscheme.Styles["comment"]; ok {
        // 代码块用 comment 的背景色
    }
}
```

好处：
- render 想用什么颜色都能从 Colorscheme 取，不用来回改 MDConfig
- md 包零依赖 config，colorscheme 作为纯数据传入
- 换主题自动生效（map 是引用，config 刷新时下一帧就拿到新颜色）

注意事项：
- `Styles` 是 `config.Colorscheme` 的直接引用（不复制），md 包代码**只读不写**，避免污染全局状态

### Style 的来源

render 函数内部从 `cfg.Colorscheme` 取，不由外部传入。后续会分两层：

1. **底色**：从 Colorscheme 取（如普通段落用 DefStyle，代码块用 comment 背景色）
2. **字符样式**：由 `renderInline` 处理（粗体加粗体属性，链接加下划线等）

### 代码结构

```go
func RenderNormal(lines []string, width int, cfg MDConfig) *RenderedSegment {
    result := &RenderedSegment{}
    style := cfg.Colorscheme.DefStyle  // 普通段落用 default 背景
    for lineIdx, line := range lines {
        cells := renderInline(line, style, lineIdx)
        rows := wrapCells(cells, width, lineIdx)
        result.Rows = append(result.Rows, rows...)
    }
    return result
}

// wrapCells 把一行的 cells 按宽度 wrap 成多个 RenderedRow
// 被 RenderNormal / RenderList / RenderBlockquote 复用
func wrapCells(cells []Cell, width int, bufLine int) []RenderedRow {
    // 0. 空行 → 返回一个填充空格的 Row
    // 1. 按空格分词（行尾/行首空格丢弃）
    // 2. 逐词填行
    // 3. 每行填充到 width
}
```

### wrapCells 的复用

`wrapCells` 是通用函数，可被以下 renderer 复用：

| Renderer | 是否复用 wrapCells | 说明 |
|----------|:------------------:|------|
| RenderNormal | ✅ | 段落主体 |
| RenderList | ✅ | 列表项内容（前缀缩进后，剩余部分走 wrapCells） |
| RenderBlockquote | ✅ | 引用内容（左侧竖线后，剩余部分走 wrapCells） |
| RenderTable | ❌ | 表格有自己的列宽分配和 cell 内换行逻辑 |
| RenderCodeBlock | ❌ | 代码不做 word wrap，原样输出或硬截断 |
| RenderHeading | 可选 | 标题通常较短，wrap 意义不大，但复用也没坏处 |

## 实施步骤

1. **先加 wrap**：实现 `wrapCells`，RenderNormal 调用它，解决溢出问题
2. **后加 inline 格式**：在 `renderInline` 里处理粗体/斜体/链接，叠加在 wrap 之上

每步可独立验证。
