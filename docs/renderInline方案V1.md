# renderInline 方案 V1（讨论稿）

## 一、问题

当前 `renderInline` 是空壳，逐字符原样输出。需要实现行内 Markdown 标记的解析和样式输出。

核心问题：**颜色从哪来？标记符号被隐藏后，颜色锚点怎么保住？**

## 二、颜色方案：display 层解决，renderInline 不管

### 原生链路

Micro 原生的颜色链路：

```
markdown.yaml（规则 + group 名）
  → highlighter 给每个字符打 group 标签 → b.Match(line)[x]
    → colorscheme 把 group 名映射成颜色 → config.GetColor(group)
```

我们不跳过这条链路，也不自己发明颜色。

### 核心矛盾

当前 `renderSegmentMD`（bufwindow_md.go:75-105）用稀疏 Match map 查颜色：

```
if group, ok := w.Buf.Match(cell.BufLine)[cell.BufX]; ok {
    s := config.GetColor(group.String())
    lastFg = ...
}
```

问题：Match 是稀疏的，只在 group 变化位置有条目。`**加粗**普通` 的锚点在 BufX=0（type）和 BufX=5（default）。如果 renderInline 把 `**` 隐藏了，第一个 Cell 的 BufX=2，查 Match[2] 没有条目，颜色丢失。

### 解决方案：稀疏 → 稠密

display 层在查颜色之前，先把 buffer 原始行的稀疏 Match map **展开为稠密 charStyles 数组**：

```
**加粗**普通

Match map（稀疏）:
  BufX=0 → group=type
  BufX=5 → group=default

展开为 charStyles（稠密）:
  BufX:  0    1    2    3    4    5    6    7
  字符:  *    *    加   粗   *    *    普   通
  style: type type type type type type def  def
```

然后 renderSegmentMD 显示 Cell 时，直接用 `charStyles[cell.BufX]` 取颜色。标记符号虽然被隐藏了，但内容字符的 BufX 没变，查稠密数组颜色完全正确。

```
renderInline 输出的 Cell:
  {加, BufX=2}  → charStyles[2] = type 色 ✓
  {粗, BufX=3}  → charStyles[3] = type 色 ✓
  {普, BufX=6}  → charStyles[6] = default 色 ✓
  通, BufX=7}  → charStyles[7] = default 色 ✓
```

### 稠密展开的实现

在 display 层加一个工具函数：

```go
// expandLineStyles 将稀疏 Match map 展开为稠密 style 数组。
// charStyles[i] = 第 i 个 rune 经过 highlighter + colorscheme 之后的样式。
// 调用方：renderSegmentMD
func expandLineStyles(bufLine int, lineLen int, baseStyle tcell.Style) []tcell.Style {
    charStyles := make([]tcell.Style, lineLen)
    match := w.Buf.Match(bufLine)
    curStyle := baseStyle
    for i := 0; i < lineLen; i++ {
        if group, ok := match[i]; ok {
            curStyle = config.GetColor(group.String())
        }
        charStyles[i] = curStyle
    }
    return charStyles
}
```

这段逻辑仍在 display 层（BufWindow），不进 md 包。

## 三、renderInline 的职责

### 职责划分

| 层 | 管什么 | 不管什么 |
|---|---|---|
| **renderInline（md 包）** | 标记解析、隐藏标记符号、叠加 Bold/Italic/Strikethrough/Underline | 颜色 |
| **display 层** | 颜色（稀疏 Match → 稠密 charStyles，按 BufX 覆盖前景色） | Bold/Italic |

### renderInline 做的

1. **解析标记结构**：识别 `**`、`*`、`~~`、`` ` ``、`[]()` 等标记边界
2. **隐藏标记符号**：标记字符不输出为 Cell
3. **叠加文本属性**：对标记内的内容 Cell，在 baseStyle 基础上叠加 Bold/Italic/Strikethrough/Underline

例如：
- `**加粗**` → 内容 Cell 的 Style = baseStyle.Bold(true)
- `*斜体*` → 内容 Cell 的 Style = baseStyle.Italic(true)
- `~~删除~~` → 内容 Cell 的 Style = baseStyle.StrikeThrough(true)
- `` `代码` `` → 内容 Cell 的 Style = baseStyle（行内代码的视觉差异靠颜色区分，不靠属性）

### renderInline 不做的

1. 不管颜色（不 import config，不查 colorscheme，颜色由 display 层覆盖）
2. 不处理块级语法（`#`、`- `、`> ` 等由各 render 函数处理）
3. 不做换行（由 `wrapCells` 处理）

### 签名不变

```go
func renderInline(line string, baseStyle tcell.Style, bufLineOffset int) []Cell
```

输入输出都不变，只是内部逻辑从"原样输出"变为"解析标记 + 隐藏标记符号 + 叠加文本属性"。

## 四、renderSegmentMD 的改动

### 删除：稀疏 Match 查询（bufwindow_md.go:75-105）

当前 `renderSegmentMD` 里的这段逻辑 **全部删除**：

```go
// 删除 ↓
var lastFg tcell.Color
var lastAttr tcell.AttrMask
hasLast := false

for col, cell := range row.Cells {
    ...
    if group, ok := w.Buf.Match(cell.BufLine)[cell.BufX]; ok {
        s := config.GetColor(group.String())
        lastFg, _, lastAttr = s.Decompose()
        hasLast = true
    }
    if hasLast {
        style = tcell.StyleDefault.Foreground(lastFg).Background(bg)
        // ... 逐个加 attr
    }
}
```

**删除理由**：
1. 稀疏查询会被 renderInline 隐藏标记后丢锚点
2. 暴力重建 style（`tcell.StyleDefault.Foreground(...)` + 逐个加 attr）会覆盖 renderInline 叠加的 Bold/Italic
3. 稠密数组方案完全替代，没有保留价值

### 新增：expandLineStyles 工具函数

```go
// expandLineStyles 将稀疏 Match map 展开为稠密 style 数组。
// result[i] = 第 i 个 rune 经过 highlighter + colorscheme 之后的完整 style。
func (w *BufWindow) expandLineStyles(bufLine int, runeCount int, baseStyle tcell.Style) []tcell.Style {
    charStyles := make([]tcell.Style, runeCount)
    match := w.Buf.Match(bufLine)
    curStyle := baseStyle
    for i := 0; i < runeCount; i++ {
        if group, ok := match[i]; ok {
            curStyle = config.GetColor(group.String())
        }
        charStyles[i] = curStyle
    }
    return charStyles
}
```

### 改写：renderSegmentMD 第 4 步（写 screen）

```go
// 3.5 预展开稠密 style 数组（在遍历 rows 之前，一次性算好）
lineStyles := map[int][]tcell.Style{}
for bufLine := seg.BufStartLine; bufLine <= seg.BufEndLine; bufLine++ {
    line := w.Buf.Line(bufLine)
    lineStyles[bufLine] = w.expandLineStyles(bufLine, utf8.RuneCountInString(line), config.DefStyle)
}

// 4. 遍历 rows 写 screen
for _, row := range rendered.Rows {
    // ... 画 gutter + 行号（不变）

    for col, cell := range row.Cells {
        // ... screenX/screenY 计算（不变）
        style := cell.Style
        if cell.BufLine >= 0 && cell.BufX >= 0 {
            if styles, ok := lineStyles[cell.BufLine]; ok && cell.BufX < len(styles) {
                // 只覆盖前景色，保留 renderInline 的背景色和文本属性
                fg, _, _ := styles[cell.BufX].Decompose()
                _, bg, _ := style.Decompose()
                style = style.Foreground(fg).Background(bg)
            }
        }
        screen.SetContent(screenX, screenY, cell.Rune, cell.Combining, style)
    }
}
```

### 不改的部分

- `displayBufferMD()` 主循环：不变
- `renderSegmentNative()`：不变，走原生 `getStyle` 逻辑
- `drawGutterAndLineNumMD()`：不变
- md 包所有代码：不变

改动集中在 `renderSegmentMD` 一个函数内部 + 新增 `expandLineStyles` 一个工具函数。

## 五、支持的标记

| 标记 | 语法 | markdown.yaml group（参考） | renderInline 行为 |
|---|---|---|---|
| 行内代码 | `` `code` `` | special | 隐藏反引号，内部不解析其他标记 |
| 加粗 | `**text**` | type | 隐藏 `**`，叠加 Bold |
| 斜体 | `*text*` | type | 隐藏 `*`，叠加 Italic |
| 加粗斜体 | `***text***` | type | 隐藏 `***`，叠加 Bold+Italic |
| 删除线 | `~~text~~` | type | 隐藏 `~~`，叠加 Strikethrough |
| 链接 | `[text](url)` | constant | 隐藏 `[](url)`，叠加 Underline |
| 图片 | `![alt](url)` | underlined | 隐藏 `![](url)` |

**颜色由 markdown.yaml + colorscheme 决定**，renderInline 和 display 层都不写死具体颜色。

以 monokai 为例，highlighter 会给 `**加粗**` 整体（含 `**` 和内容文字）打上 `type` group → colorscheme 映射为 #66D9EF。
稠密展开后，即使 `**` 被 renderInline 隐藏，内容文字的 BufX 位置仍是 `type` 色定。
不同的 colorscheme 会有不同的颜色，完全由用户配置决定。

### 解析算法

从左到右扫描，优先匹配长标记，每次匹配成功跳过整个范围继续扫描：

```
从左到右扫描 line：
  1. 遇到 ` → 找下一个 `，中间全部当行内代码输出
     找不到 → 当普通字符输出这个 `
  2. 遇到 *** → 找下一个 ***，内容叠加 Bold+Italic
     找不到 → 尝试 ** → *
  3. 遇到 ** → 找下一个 **，内容叠加 Bold
     找不到 → 尝试 *，或者当普通字符
  4. 遇到 * → 找下一个 *，内容叠加 Italic
     找不到 → 当普通字符
  5. 遇到 ~~ → 找下一个 ~~，内容叠加 Strikethrough
     找不到 → 当普通字符
  6. 遇到 [ → 尝试匹配 ](url) 模式，成功则隐藏标记显示 text
     找不到 → 当普通字符
  7. 其他字符 → 原样输出
```

匹配优先级：`` ` `` > `***` > `**` > `*` > `~~` > `[]()`

### 未闭合标记

**未闭合标记当普通文本原样输出**，不应用任何属性。

例如 `**加粗` 只有一个 `**` 开头没有配对结尾 → `*`、`*`、`加`、`粗` 全部原样输出，不叠加 Bold。

理由：
- 符合 CommonMark 规范
- 用户编辑过程中经常出现未闭合的中间状态，原样输出最安全
- 实现简单：找不到配对就跳过

### 嵌套标记

**v1 不支持嵌套**，只做单层匹配。从左到右扫描，遇到开标记就找最近的闭标记，不递归。

示例：

| 输入 | v1 解析结果 |
|------|------|
| `***粗斜体***` | Bold+Italic（`***` 作为单一 token 匹配）✓ |
| `**加粗** 普通` | 加粗 + 普通 ✓ |
| `` `**加粗**` `` | 行内代码，不解析内部 `**` ✓ |
| `**bold *italic***` | `**` 匹配到倒数第2、3个 `*`，内容为 "bold *italic" 全部 Bold，中间的 `*` 是普通字符 |
| `**bold *italic* text**` | `**` 找最近 `**` 失败，逐个 `*` 尝试，不产生正确嵌套 |

不处理的场景：`**加粗 *斜体* 普通**` 这种真正的嵌套。

理由：
- 嵌套在终端 Markdown 里是极低频场景
- 正确处理嵌套需要引入栈/递归解析器，复杂度陡增
- v1 覆盖 95% 的实际使用场景
- 后续加栈解析器是纯内部改动，不影响外部接口

## 六、调用关系

```
display 层 (renderSegmentMD):
  1. lines = linesFromBuffer(seg)
  2. rendered = seg.Render(lines, bufWidth, mdConfig)   ← md 包纯函数
  3. lineStyles = 预展开每个 buffer 行的稠密 style 数组
  4. 遍历 rendered.Cells，用 (BufLine, BufX) 查 lineStyles 覆盖前景色
     （保留 renderInline 叠加的 Bold/Italic 等文本属性）
  5. 写 screen

md 包 (render 函数):
  RenderNormal / RenderHeading / RenderList / ...
    1. 处理块级前缀（去掉 #、-、> 等）
    2. 调用 renderInline(剩余文本, baseStyle, bufLineOffset)
       → 返回 []Cell（标记符号已隐藏，Bold/Italic 已叠加）
    3. 调用 wrapCells(cells, width, ...) 自动换行
```

## 七、总结

| 维度 | 设计 |
|------|------|
| **颜色** | display 层解决，稀疏 Match → 稠密 charStyles，按 BufX 查，只覆盖前景色 |
| **Bold/Italic 等文本属性** | renderInline 叠加，display 层保留不改 |
| **标记隐藏** | renderInline 做，标记符号不输出 Cell |
| **renderInline 签名** | 不变，仍然是纯函数 |
| **display 层改动** | renderSegmentMD 改逐 Cell 查稀疏 Match 为预展开稠密数组 |
