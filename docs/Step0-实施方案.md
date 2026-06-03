# Step 0 实施方案

> 依赖：架构设计V1.md、Step0-文件结构.md
> 目标：渲染管线端到端跑通，每个渲染模块用专属背景色标识领地
> 状态：待用户审阅

---

## 一、实施总览

Step 0 分 9 个任务，按依赖顺序执行。每个任务产出一个可编译的增量。

```
任务 1  md.go          类型定义（Segment、Cell、RenderedSegment、MDConfig）
任务 2  config.go      配置结构 + isMarkdownFile + settings 注册
任务 3  detect.go      集中式扫描器（核心）
任务 4  7 个 render_*.go  各渲染模块（只输出背景色）
任务 5  inline.go      行内渲染器骨架（空壳）
任务 6  bufwindow.go   改造：IsMD + displayBufferMD + Display 分支
任务 7  bufpane.go     改造：创建 BufWindow 时设 IsMD
任务 8  settings.go    改造：注册 MD 配置项
任务 9  编译验证 + 测试样本
```

任务 1-5 是 `internal/md/` 新包，任务 6-8 是改造现有文件，任务 9 是验收。

---

## 二、任务 1：md.go — 公共类型

**文件**：`internal/md/md.go`（新建）
**预估行数**：~100 行

### 2.1 类型定义

```go
package md

import (
    "github.com/micro-editor/tcell/v2"
)

// Cell 是渲染管线输出的最小单位：一个屏幕字符。
type Cell struct {
    Rune        rune         // 要显示的字符
    Combining   []rune       // 组合字符（通常为 nil）
    Style       tcell.Style  // 颜色和字体样式
    BufLine     int          // 对应的 buffer 行号，装饰行为 -1
    BufX        int          // 对应 buffer 行内的 rune 偏移，装饰行为 -1
    IsDecorative bool        // true = 装饰字符，点击忽略
}

// RenderedRow 是渲染后的一行屏幕输出。
type RenderedRow struct {
    Cells    []Cell
    BufLine  int  // 这行对应的 buffer 行号（复用 softwrap 规则：首行有值，续行/装饰行为 -1）
}

// RenderedSegment 是一个渲染片的完整渲染输出。
type RenderedSegment struct {
    Rows       []RenderedRow
    BufStartLine int           // 片起始 buffer 行
    BufEndLine   int           // 片结束 buffer 行（含）
}

// SegmentMeta 是检测步骤输出的轻量元数据，不包含渲染内容。
// 缓存到 BufWindow 上供 Scroll/Diff 使用。
type SegmentMeta struct {
    BufStartLine int
    BufEndLine   int
    RowCounts    []int  // RowCounts[i] = 片内第 i 个 buffer 行占几个 screen row
}

// Segment 是检测步骤的输出单位。每一行 buffer 都属于某个 Segment。
type Segment struct {
    BufStartLine int
    BufEndLine   int
    // Render 是渲染函数。接收 buffer 行内容和宽度，返回渲染结果。
    // Step 0 阶段只输出背景色。
    Render func(lines []string, width int, cfg MDConfig) *RenderedSegment
}
```

### 2.2 设计要点

- **Cell.BufLine / Cell.BufX**：架构设计V1 §3.4 要求的「每个真实字符记录 buffer 坐标」。rune 偏移与 `buffer.Loc.X` 一致。
- **RenderedRow.BufLine**：复用 softwrap 规则——首行显示行号（BufLine 有值），续行/装饰行为 -1（行号位留空）。
- **Segment.Render 是函数字段**：不是接口方法，不是 struct 方法。Go 函数是一等公民，`detect.go` 扫描时直接挂函数名。
- **SegmentMeta**：与 Segment 分离。SegmentMeta 是轻量元数据（片边界 + 行高），缓存到 BufWindow 供 Scroll/Diff 查询。Segment 的 Render 调用开销更大（逐字符计算）。

### 2.3 为什么 RenderedSegment 里用 []string 而不传 *buffer.Buffer

架构设计V1 §3.6 的设计：renderer 是纯函数，入参 `lines []string, width int, cfg MDConfig`。

- `lines` 由 `displayBufferMD()` 从 `w.Buf.LineBytes()` 转换后传入
- `width` 是 `w.bufWidth`
- `cfg` 是从 BufWindow 持有的 MDConfig

这样 renderer 不 import `internal/buffer`，测试时可以直接构造 `[]string` 输入，不需要 mock Buffer。

---

## 三、任务 2：config.go — 配置

**文件**：`internal/md/config.go`（新建）
**预估行数**：~100 行

### 3.1 MDConfig 结构

```go
package md

import (
    "path/filepath"
    "strings"
)

// MDConfig 持有所有 MD 渲染相关的配置。
// 由 BufPane 构造时从 config/buffer 层读取，塞到 BufWindow 上。
// display 和 md 包通过这个结构解耦，不直接 import config。
type MDConfig struct {
    MDRender      bool    // 功能总开关
    MDRenderIdle  float64 // 编辑模式超时秒数（Step 1 用）
    MDTableAlign  bool    // 表格对齐
    MDTableBorder bool    // 表格外框
    MDBoldItalic  bool    // 加粗斜体渲染
    MDCodeBlock   bool    // 代码块渲染
    MDHeading     bool    // 标题渲染
    MDList        bool    // 列表渲染
    MDLink        bool    // 链接渲染
}

// DefaultMDConfig 返回默认配置。
func DefaultMDConfig() MDConfig {
    return MDConfig{
        MDRender:      true,
        MDRenderIdle:  10,
        MDTableAlign:  true,
        MDTableBorder: false,
        MDBoldItalic:  true,
        MDCodeBlock:   true,
        MDHeading:     true,
        MDList:        true,
        MDLink:        true,
    }
}

// IsMarkdownFile 判断文件路径是否为 Markdown 文件。
// BufPane 创建 BufWindow 时调用一次。
func IsMarkdownFile(path string) bool {
    ext := strings.ToLower(filepath.Ext(path))
    return ext == ".md" || ext == ".markdown"
}
```

### 3.2 配置传递路径

```
config/settings.go (默认值)
    ↓  BufPane 构造时读取
BufPane.NewBufPaneFromBuf()
    ↓  写入 BufWindow
BufWindow.mdConfig = md.MDConfig{...}
    ↓  displayBufferMD() 传入 renderer
segment.Render(lines, width, w.mdConfig)
```

`md` 包完全不 import `internal/config`。配置值由 BufPane/BufWindow 从 config 层拉取后塞入 MDConfig 值类型。

### 3.3 settings.go 注册的配置项

在 `internal/config/settings.go` 的 `defaultCommonSettings` map 中添加：

```go
// MD 渲染相关配置
"mdrender":      true,
"mdtablealign":  true,
"mdtableborder": false,
"mdbolditalic":  true,
"mdcodeblock":   true,
"mdheading":     true,
"mdlist":        true,
"mdlink":        true,
```

在 `DefaultGlobalOnlySettings` map 中添加：

```go
"mdrenderidle": float64(10),
```

**为什么分开放**：按架构设计V1 §5.7 的建议——功能总开关 `mdrender` 和单模块开关放 `defaultCommonSettings`（buffer 本地，可被 ft: / glob: 覆盖），`mdrenderidle` 是全局偏好放 `DefaultGlobalOnlySettings`。

> 注：Step 0 只注册配置项，不读取也不使用。实际读取在任务 7（bufpane.go）的 BufPane 构造逻辑中完成。

---

## 四、任务 3：detect.go — 集中式扫描器

**文件**：`internal/md/detect.go`（新建）
**预估行数**：~200 行
**这是 Step 0 最核心的代码。**

### 4.1 函数签名

```go
package md

// DetectSegments 扫描 buffer 的可见区域，返回 []Segment。
// 每个 Segment 标记了它负责的 buffer 行范围和渲染函数。
// visibleStart/visibleEnd 是 buffer 行号范围（含两端）。
// buf 是 buffer 引用，用于读取行内容。
// bufWidth 是渲染区域宽度（列数）。
//   - Step 0: 透传给 renderer，detect 自身不使用
//   - Step 1+: detect 用于计算各渲染片的视觉行高，输出到 SegmentMeta 缓存，供 Scroll/Diff 查询
func DetectSegments(
    buf BufferReader,
    visibleStart, visibleEnd int,
    bufWidth int,
) []Segment
```

### 4.2 BufferReader 接口

detect.go 需要 buffer 的行数据，但不应该 import `internal/buffer`（会产生循环依赖风险，且违背纯函数设计）。

```go
// BufferReader 是 detect.go 对 buffer 的最小依赖接口。
// 由 BufWindow 调用 DetectSegments 时传入 w.Buf（*buffer.Buffer 满足此接口）。
type BufferReader interface {
    LinesNum() int
    LineBytes(n int) []byte
    Line(n int) string
}
```

`*buffer.Buffer` 通过嵌入 `*SharedBuffer` → `*LineArray` 已经实现了 `LinesNum()`、`LineBytes(n)`、`Line(n)` 三个方法，无需任何适配。

### 4.3 扫描逻辑

```
DetectSegments(buf, visibleStart, visibleEnd, bufWidth):
    segments = []
    state = stateNormal
    i = visibleStart

    while i <= visibleEnd:
        line = buf.Line(i)
        trimmed = strings.TrimSpace(line)

        switch state:
        case stateNormal:
            # 块结构优先检查（多行结构优先级高于单行）
            elif strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~"):
                → 进入 stateCodeblock, 记录 start = i, i++
            elif strings.HasPrefix(trimmed, ">"):
                → 进入 stateBlockquote, 连续 > 行合并, i++
            elif strings.Contains(trimmed, "|") && trimmed != "":
                → 进入 stateTable, 连续含 | 行合并, i++
            elif isListItem(trimmed):
                → 进入 stateList, 连续列表项合并, i++
            # 单行结构后检查
            elif strings.HasPrefix(trimmed, "#"):
                → 单行 heading, i++
            elif isHR(trimmed):
                → 单行 hr, i++
            # 兜底
            elif trimmed == "" → 单行 paragraph, i++
            else:
                → 单行 paragraph, i++

        case stateCodeblock:
            if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~"):
                → 闭合, segments ← Segment{start..i, RenderCodeBlock}
                → state = stateNormal, i++
            else:
                → i++ (继续收集)

        case stateBlockquote:
            if strings.HasPrefix(trimmed, ">") || trimmed == "":
                → i++ (继续收集)
            else:
                → segments ← Segment{start..i-1, RenderBlockquote}
                → state = stateNormal (不 i++，当前行重新判断)

        case stateTable:
            if strings.Contains(trimmed, "|") && trimmed != "":
                → i++ (继续收集)
            else:
                → segments ← Segment{start..i-1, RenderTable}
                → state = stateNormal (不 i++，当前行重新判断)

        case stateList:
            if isListItem(trimmed) || (trimmed != "" && !strings.HasPrefix(trimmed, "#")):
                → i++ (继续收集，允许列表项之间的空行和缩进内容)
            else:
                → segments ← Segment{start..i-1, RenderList}
                → state = stateNormal (不 i++，当前行重新判断)

    // 处理末尾未闭合的状态
    if state == stateCodeblock:
        segments ← Segment{start..visibleEnd, RenderCodeBlock}
    elif state == stateBlockquote:
        segments ← Segment{start..visibleEnd, RenderBlockquote}
    elif state == stateTable:
        segments ← Segment{start..visibleEnd, RenderTable}
    elif state == stateList:
        segments ← Segment{start..visibleEnd, RenderList}

    return segments
```

### 4.4 辅助判断函数

```go
// isHR 判断是否为水平分割线：--- 或 *** 或 ___ (至少 3 个，可更多)
func isHR(s string) bool {
    if len(s) < 3 {
        return false
    }
    c := s[0]
    if c != '-' && c != '*' && c != '_' {
        return false
    }
    for i := 1; i < len(s); i++ {
        if s[i] != c {
            return false
        }
    }
    return len(s) >= 3
}

// isListItem 判断是否为列表项：以 "- " / "* " / "+ " 开头，或 "1. " 等数字序号
func isListItem(s string) bool {
    if len(s) < 2 {
        return false
    }
    if s[0] == '-' || s[0] == '*' || s[0] == '+' {
        return s[1] == ' '
    }
    // 数字序号: "1. " / "12. " 等
    i := 0
    for i < len(s) && s[i] >= '0' && s[i] <= '9' {
        i++
    }
    if i > 0 && i+1 < len(s) && s[i] == '.' && s[i+1] == ' ' {
        return true
    }
    return false
}
```

### 4.5 状态常量

```go
type detectState int

const (
    stateNormal     detectState = iota
    stateCodeblock
    stateBlockquote
    stateTable
    stateList
)
```

### 4.6 Segment 填充示例

```go
// 标题
segments = append(segments, Segment{
    BufStartLine: i,
    BufEndLine:   i,
    Render:       RenderHeading,
})

// 代码块（多行）
segments = append(segments, Segment{
    BufStartLine: start,
    BufEndLine:   endLine,  // 包含闭合的 ```
    Render:       RenderCodeBlock,
})
```

### 4.7 为什么不用正则

每种类型的识别都是 `strings.HasPrefix` + 少量字符检查，平均不到 10 行。正则表达式会增加启动开销和可读性成本，对这种简单的行首匹配没有必要。

---

## 五、任务 4：7 个 render_*.go — 渲染模块（背景色）

**文件**：`internal/md/render_*.go`（7 个新建文件）
**预估行数**：共 ~450 行

### 5.1 统一模式

每个 renderer 导出一个纯函数，签名相同：

```go
func RenderXxx(lines []string, width int, cfg MDConfig) *RenderedSegment
```

**Step 0 阶段的渲染逻辑非常简单**：

1. 确定背景色（每种类型一种颜色）
2. 把 `lines` 中的每个字符原样输出为 Cell
3. 每行末尾用空格填充到 `width` 列
4. 填充 Cell 的 BufLine / BufX
5. 返回 `*RenderedSegment`

### 5.2 各模块的背景色方案

使用 tcell 的 256 色方案，确保在大多数终端可见但不过于刺眼：

| 模块 | 背景色 | tcell.Color | 说明 |
|------|--------|-------------|------|
| heading | 深蓝 | Color(17) | `#00005f` |
| codeblock | 深灰 | Color(235) | `#262626` |
| table | 深绿 | Color(22) | `#005f00` |
| blockquote | 深紫 | Color(55) | `#5f00af` |
| list | 深青 | Color(23) | `#005f5f` |
| hr | 深红 | Color(52) | `#5f0000` |
| paragraph | 无 | DefStyle | 默认背景，不变 |

> 这些颜色只是 Step 0 的可视化标记，Step 2 做实际渲染时会换为正式配色方案。

### 5.3 公共辅助函数

每个 renderer 都需要「逐字符输出 + 填充背景色」的逻辑，提取为内部工具函数：

```go
// renderLinesWithBg 是 Step 0 的公共渲染逻辑：
// 将 lines 逐字符输出为 Cell，每行填充到 width 列，全部使用 bgStyle。
// bufStartLine 是第一个 line 对应的 buffer 行号。
func renderLinesWithBg(lines []string, width int, bgStyle tcell.Style, bufStartLine int) *RenderedSegment {
    result := &RenderedSegment{
        BufStartLine: bufStartLine,
        BufEndLine:   bufStartLine + len(lines) - 1,
    }

    for lineIdx, line := range lines {
        row := RenderedRow{
            BufLine: bufStartLine + lineIdx,
        }

        // 逐字符输出
        for x, r := range line {
            row.Cells = append(row.Cells, Cell{
                Rune:   r,
                Style:  bgStyle,
                BufLine: bufStartLine + lineIdx,
                BufX:    x,
            })
        }

        // 填充到 width
        for x := len(line); x < width; x++ {
            row.Cells = append(row.Cells, Cell{
                Rune:   ' ',
                Style:  bgStyle,
                BufLine: bufStartLine + lineIdx,
                BufX:    -1, // 填充空格不是真实字符
            })
        }

        result.Rows = append(result.Rows, row)
    }

    return result
}
```

这个函数放在 `md.go` 里（同包内部可用），而不是单独建 `render_utils.go`，因为 Step 2 做实际渲染时会被替换掉。

### 5.4 各 renderer 文件内容

以 `render_heading.go` 为例：

```go
package md

import "github.com/micro-editor/tcell/v2"

var headingBgStyle tcell.Style = tcell.StyleDefault.Background(tcell.Color(17))

// RenderHeading 渲染标题行。Step 0 只输出背景色。
func RenderHeading(lines []string, width int, cfg MDConfig) *RenderedSegment {
    return renderLinesWithBg(lines, width, headingBgStyle, 0)
}
```

其余 6 个文件结构完全相同，只是背景色不同。

**render_paragraph.go 稍有不同**：使用 `config.DefStyle`（默认样式，无特殊背景色）。但 `md` 包不 import `config`，所以 paragraph 用 `tcell.StyleDefault`：

```go
package md

import "github.com/micro-editor/tcell/v2"

// RenderParagraph 渲染普通段落行。Step 0 无特殊处理，使用默认样式。
func RenderParagraph(lines []string, width int, cfg MDConfig) *RenderedSegment {
    return renderLinesWithBg(lines, width, tcell.StyleDefault, 0)
}
```

### 5.5 renderLinesWithBg 的 bufStartLine 参数

注意 `renderLinesWithBg` 接收 `bufStartLine` 参数，但 renderer 函数签名没有这个参数。调用方（`displayBufferMD`）在拿到 `*RenderedSegment` 后，用 Segment 的 `BufStartLine` 覆盖。

**简化方案**：`renderLinesWithBg` 内部所有 Cell 的 BufLine 都设为 `lineIdx`（从 0 开始的相对行号），`displayBufferMD` 调整为绝对行号。

修正版：

```go
func renderLinesWithBg(lines []string, width int, bgStyle tcell.Style) *RenderedSegment {
    result := &RenderedSegment{}
    for lineIdx, line := range lines {
        row := RenderedRow{
            BufLine: lineIdx, // 相对行号，displayBufferMD 调整
        }
        for x, r := range line {
            row.Cells = append(row.Cells, Cell{
                Rune:   r,
                Style:  bgStyle,
                BufLine: lineIdx,
                BufX:    x,
            })
        }
        for x := len(line); x < width; x++ {
            row.Cells = append(row.Cells, Cell{
                Rune:   ' ',
                Style:  bgStyle,
                BufLine: lineIdx,
                BufX:    -1,
            })
        }
        result.Rows = append(result.Rows, row)
    }
    return result
}
```

`displayBufferMD()` 会在写入 screen 时把 `BufLine` 加上 `segment.BufStartLine`。

---

## 六、任务 5：inline.go — 行内渲染器骨架

**文件**：`internal/md/inline.go`（新建）
**预估行数**：~30 行

Step 0 不做行内渲染（加粗、斜体、链接），但预留文件骨架：

```go
package md

// renderInline 处理行内的 Markdown 标记（加粗、斜体、链接等）。
// 输入一行字符串，输出 []Cell。
// Step 0 是空壳，直接原样输出。
func renderInline(line string, baseStyle Style, bufLineOffset int) []Cell {
    cells := make([]Cell, 0, len(line))
    for x, r := range line {
        cells = append(cells, Cell{
            Rune:   r,
            Style:  baseStyle,
            BufLine: bufLineOffset,
            BufX:    x,
        })
    }
    return cells
}
```

> 注：`Style` 是 `tcell.Style` 的简写需要 import。或者直接用 `tcell.Style`。这里先用全称。

Step 2 做实际行内渲染时，这个函数会展开为完整的 `**bold**` / `*italic*` / `[link](url)` 解析逻辑。

---

## 七、任务 6：bufwindow.go 改造

**文件**：`internal/display/bufwindow.go`（改造）
**预估改动**：+150 行（新增 displayBufferMD + IsMD 字段 + Display 分支）

### 7.1 新增字段

```go
type BufWindow struct {
    // ... 现有字段全部保留
    IsMD     bool       // MicroNeo: 由 BufPane 创建时设置
    mdConfig md.MDConfig  // MicroNeo: MD 渲染配置
    mdCache  []md.SegmentMeta  // MicroNeo: 上一帧检测的轻量元数据缓存
}
```

import 新增：

```go
import (
    // ... 现有 import 保留
    "github.com/micro-editor/micro/v2/internal/md"
)
```

### 7.2 Display() 加 if/else 分支

```go
func (w *BufWindow) Display() {
    w.updateDisplayInfo()

    w.displayStatusLine()
    w.displayScrollBar()
    if w.IsMD {
        w.displayBufferMD()
    } else {
        w.displayBuffer()
    }
}
```

改动极小：在现有 `displayBuffer()` 调用前加一个 if/else。

### 7.3 displayBufferMD() — 核心新增函数

```go
// displayBufferMD 是 displayBuffer 的 MD 渲染版本。
// Step 0 实现：检测渲染片 → 逐片输出背景色 → 写入 screen。
// editMode 相关逻辑在 Step 1 加入。
func (w *BufWindow) displayBufferMD() {
    b := w.Buf
    if w.Height <= 0 || w.Width <= 0 {
        return
    }

    if b.ModifiedThisFrame {
        if b.Settings["diffgutter"].(bool) {
            b.UpdateDiff()
        }
        b.ModifiedThisFrame = false
    }

    bufWidth := w.bufWidth
    bufHeight := w.bufHeight

    // 1. 检测可见区域
    visibleStart := w.StartLine.Line
    visibleEnd := visibleStart + bufHeight // 超过也行，detect 会截断
    if visibleEnd >= b.LinesNum() {
        visibleEnd = b.LinesNum() - 1
    }

    segments := md.DetectSegments(b, visibleStart, visibleEnd, bufWidth)
    // TODO(Step 3): 将检测结果写入 w.mdCache 供 Scroll/Diff 查询

    // 2. 渲染管线主循环
    vY := 0       // 当前 screen 行（相对窗口顶部）
    bloc := visibleStart  // 当前 buffer 行

    for _, seg := range segments {
        // 跳过 StartLine 之前的行（softwrap offset 暂不处理，Step 0 简化）
        rendered := seg.Render(
            w.linesFromBuffer(seg.BufStartLine, seg.BufEndLine),
            bufWidth,
            w.mdConfig,
        )

        // 将 renderer 输出的相对 BufLine 转为绝对行号
        for ri := range rendered.Rows {
            rendered.Rows[ri].BufLine += seg.BufStartLine
            for ci := range rendered.Rows[ri].Cells {
                if rendered.Rows[ri].Cells[ci].BufLine >= 0 {
                    rendered.Rows[ri].Cells[ci].BufLine += seg.BufStartLine
                }
            }
        }

        // 写入 screen
        for _, row := range rendered.Rows {
            if vY >= bufHeight {
                break
            }

            // 画 gutter + 行号
            w.drawGutterAndLineNumMD(vY, row.BufLine)

            // 画内容
            for col, cell := range row.Cells {
                screenX := w.X + w.gutterOffset + col
                screenY := w.Y + vY
                if screenX < w.X+w.gutterOffset || screenX >= w.X+w.gutterOffset+bufWidth {
                    continue
                }
                screen.SetContent(screenX, screenY, cell.Rune, cell.Combining, cell.Style)
            }

            vY++
        }

        bloc = seg.BufEndLine + 1
    }

    // 3. 填充剩余空间
    defStyle := config.DefStyle
    for ; vY < bufHeight; vY++ {
        w.drawGutterAndLineNumMD(vY, -1) // 空行，不显示行号
        for col := 0; col < bufWidth; col++ {
            screen.SetContent(w.X+w.gutterOffset+col, w.Y+vY, ' ', nil, defStyle)
        }
    }
}
```

### 7.4 辅助方法

```go
// linesFromBuffer 从 buffer 读取 start..end 行（含两端），返回 []string。
func (w *BufWindow) linesFromBuffer(start, end int) []string {
    lines := make([]string, 0, end-start+1)
    for i := start; i <= end && i < w.Buf.LinesNum(); i++ {
        lines = append(lines, w.Buf.Line(i))
    }
    return lines
}

// drawGutterAndLineNumMD 在指定 screen 行绘制 gutter 和行号。
// bufLine 为 -1 表示空行/续行，行号位留空。
func (w *BufWindow) drawGutterAndLineNumMD(vY int, bufLine int) {
    b := w.Buf
    vloc := buffer.Loc{X: 0, Y: vY}
    bloc := buffer.Loc{X: 0, Y: bufLine}

    lineNumStyle := config.DefStyle
    if style, ok := config.Colorscheme["line-number"]; ok {
        lineNumStyle = style
    }

    if w.hasMessage && bufLine >= 0 {
        w.drawGutter(&vloc, &bloc)
    }
    if b.Settings["diffgutter"].(bool) && bufLine >= 0 {
        w.drawDiffGutter(lineNumStyle, false, &vloc, &bloc)
    }
    if b.Settings["ruler"].(bool) {
        if bufLine >= 0 {
            w.drawLineNum(lineNumStyle, false, &vloc, &bloc)
        } else {
            // 续行/空行/装饰行：行号位留空
            for vloc.X < w.gutterOffset {
                screen.SetContent(w.X+vloc.X, w.Y+vY, ' ', nil, lineNumStyle)
                vloc.X++
            }
        }
    }
}
```

### 7.5 关于 displayBufferMD 的边界处理

**softwrap offset**：`w.StartLine.Row` 在 softwrap 模式下表示首行的起始行偏移。Step 0 不做 softwrap 处理（渲染片路径不走 softwrap），所以 `w.StartLine.Row` 应该为 0。如果 `w.StartLine.Row > 0`（从 softwrap 切过来的状态），Step 0 简化处理：直接从 `visibleStart` 行的第 0 行开始渲染，允许轻微错位。这是可接受的退化。

**超出可见区域的行**：`DetectSegments` 的 `visibleEnd` 参数会限制扫描范围，不会扫描整个文件。

---

## 八、任务 7：bufpane.go 改造

**文件**：`internal/action/bufpane.go`（改造）
**预估改动**：+15 行

### 8.1 改动点：NewBufPaneFromBuf

```go
func NewBufPaneFromBuf(buf *buffer.Buffer, tab *Tab) *BufPane {
    w := display.NewBufWindow(0, 0, 0, 0, buf)

    // MicroNeo: 设置 MD 标志和配置
    if md.IsMarkdownFile(buf.Path) {
        w.IsMD = true
        w.SetMDConfig(md.MDConfig{
            MDRender:      config.GetGlobalOption("mdrender").(bool),
            MDRenderIdle:  config.GetGlobalOption("mdrenderidle").(float64),
            MDTableAlign:  buf.Settings["mdtablealign"].(bool),
            MDTableBorder: buf.Settings["mdtableborder"].(bool),
            MDBoldItalic:  buf.Settings["mdbolditalic"].(bool),
            MDCodeBlock:   buf.Settings["mdcodeblock"].(bool),
            MDHeading:     buf.Settings["mdheading"].(bool),
            MDList:        buf.Settings["mdlist"].(bool),
            MDLink:        buf.Settings["mdlink"].(bool),
        })
    }

    h := newBufPane(buf, w, tab)
    return h
}
```

### 8.2 BufWindow 新增方法

在 `bufwindow.go` 中添加：

```go
// SetMDConfig 设置 MD 渲染配置。
func (w *BufWindow) SetMDConfig(cfg md.MDConfig) {
    w.mdConfig = cfg
}
```

### 8.3 import 变化

`bufpane.go` 需要新增 import：

```go
import (
    // ... 现有 import
    "github.com/micro-editor/micro/v2/internal/md"
)
```

---

## 九、任务 8：settings.go 改造

**文件**：`internal/config/settings.go`（改造）
**预估改动**：+8 行

### 9.1 DefaultGlobalOnlySettings 新增（全局配置）

在 `DefaultGlobalOnlySettings` map 中添加：

```go
// MicroNeo: MD 全局配置
"mdrender":      true,        // 功能总开关
"mdrenderidle":  float64(10), // 编辑模式超时秒数
```

### 9.2 defaultCommonSettings 新增（buffer 本地配置）

在 `defaultCommonSettings` map 末尾添加：

```go
// MicroNeo: MD 单模块渲染开关（buffer 本地，可被 ft: / glob: 覆盖）
"mdtablealign":  true,
"mdtableborder": false,
"mdbolditalic":  true,
"mdcodeblock":   true,
"mdheading":     true,
"mdlist":        true,
"mdlink":        true,
```

---

## 十、任务 9：编译验证 + 测试样本

### 10.1 编译验证

```bash
go build ./...
go vet ./...
```

### 10.2 测试样本文件

创建 `docs/Step0-测试样本.md`，包含所有 7 种结构：

```markdown
# 测试标题

普通段落文本。

## 二级标题

- 列表项 1
- 列表项 2
- 列表项 3

> 引用块第一行
> 引用块第二行

| 列 A | 列 B | 列 C |
|------|------|------|
| 数据1 | 数据2 | 数据3 |
| 数据4 | 数据5 | 数据6 |

---

```code
代码块内容
第二行
第三行
```

更多段落文本。
```

### 10.3 单元测试

创建 `internal/md/detect_test.go`：

```go
package md

import "testing"

// mockBuffer 实现 BufferReader 接口
type mockBuffer struct {
    lines []string
}

func (m *mockBuffer) LinesNum() int      { return len(m.lines) }
func (m *mockBuffer) LineBytes(n int) []byte { return []byte(m.lines[n]) }
func (m *mockBuffer) Line(n int) string  { return m.lines[n] }

func TestDetectHeading(t *testing.T) {
    buf := &mockBuffer{lines: []string{
        "# Title",
        "normal text",
    }}
    segments := DetectSegments(buf, 0, 1, 80)
    // 应检测到 2 个 segment: heading + paragraph
    if len(segments) != 2 {
        t.Fatalf("expected 2 segments, got %d", len(segments))
    }
    if segments[0].BufStartLine != 0 || segments[0].BufEndLine != 0 {
        t.Fatalf("heading segment: expected [0,0], got [%d,%d]",
            segments[0].BufStartLine, segments[0].BufEndLine)
    }
    if segments[1].BufStartLine != 1 || segments[1].BufEndLine != 1 {
        t.Fatalf("paragraph segment: expected [1,1], got [%d,%d]",
            segments[1].BufStartLine, segments[1].BufEndLine)
    }
}
```

测试用例覆盖：

1. 标题检测（`#`、`##`、`###`）
2. 代码块检测（多行 + 闭合）
3. 表格检测（连续 `|` 行）
4. 引用块检测（连续 `>` 行）
5. 列表检测（`-`、`*`、数字序号）
6. 分割线检测（`---`、`***`、`___`）
7. 段落检测（兜底）
8. 代码块内含 `|` 不应检测为表格（待实现：代码块内的内容不应重新检测）
9. 未闭合代码块（文件末尾无闭合标记）
10. 混合结构（标题 + 列表 + 表格 + 代码块）

### 10.4 验收标准

**编译通过 + 打开 .md 文件看到不同结构的背景色**：

1. `go build ./...` 无错误
2. 运行 `micro docs/Step0-测试样本.md`
3. 看到：
   - 标题行有深蓝背景
   - 代码块有深灰背景
   - 表格有深绿背景
   - 引用块有深紫背景
   - 列表有深青背景
   - 分割线有深红背景
   - 普通段落无特殊背景
4. 行号正常显示
5. 滚动无崩溃（可能有轻微行高不准，Step 0 可接受）
6. 非 .md 文件（如 .go 文件）行为完全不变

---

## 十一、依赖关系与执行顺序

```
任务 1 (md.go)
    ↓
任务 2 (config.go) ←── 依赖 md.go 的 MDConfig
    ↓
任务 3 (detect.go) ←── 依赖 md.go 的 Segment、BufferReader
    ↓
任务 4 (render_*.go) ←── 依赖 md.go 的 RenderedSegment、Cell、renderLinesWithBg
    ↓
任务 5 (inline.go) ←── 依赖 md.go 的 Cell（空壳，无实质依赖）
    ↓
任务 6 (bufwindow.go) ←── 依赖 md 包整体
    ↓
任务 7 (bufpane.go) ←── 依赖 md.IsMarkdownFile + bufwindow.go 的 IsMD
    ↓
任务 8 (settings.go) ←── 无代码依赖，只需注册配置项
    ↓
任务 9 (编译验证 + 测试)
```

任务 1-5 可以一次性全部写完后编译。任务 6 需要在 1-5 完成后做。任务 7-8 可以和任务 6 一起做。任务 9 最后。

---

## 十二、风险与缓解

| 风险 | 概率 | 影响 | 缓解 |
|------|:----:|:----:|------|
| tcell.Color(17) 等颜色在某些终端不可见 | 中 | 低 | 换成更亮的颜色或用 Reverse |
| 代码块内含 `\|` 被误判为表格 | 中 | 低 | detect.go 的代码块状态机优先级最高，代码块内的内容不会进入表格检测 |
| displayBufferMD 的行号绘制与 drawLineNum 不兼容 | 低 | 中 | drawGutterAndLineNumMD 复用现有 drawGutter/drawDiffGutter/drawLineNum，只是封装调用 |
| 滚动时行高不准导致跳动 | 高 | 低 | Step 0 可接受，首帧缓存未命中时退化为原生行为（架构设计V1 §3.6） |
| 非 MD 文件误触发 IsMD | 低 | 高 | isMarkdownFile 只检查 `.md` / `.markdown` 后缀，逻辑极简 |

---

## 十三、Step 0 完成后的代码量

| 文件 | 类型 | 行数 |
|------|------|------|
| `internal/md/md.go` | 新 | ~100 |
| `internal/md/config.go` | 新 | ~60 |
| `internal/md/detect.go` | 新 | ~200 |
| `internal/md/render_heading.go` | 新 | ~15 |
| `internal/md/render_codeblock.go` | 新 | ~15 |
| `internal/md/render_table.go` | 新 | ~15 |
| `internal/md/render_list.go` | 新 | ~15 |
| `internal/md/render_blockquote.go` | 新 | ~15 |
| `internal/md/render_hr.go` | 新 | ~15 |
| `internal/md/render_paragraph.go` | 新 | ~15 |
| `internal/md/inline.go` | 新 | ~30 |
| `internal/md/detect_test.go` | 新 | ~400 |
| `internal/display/bufwindow.go` | 改 | +120 |
| `internal/action/bufpane.go` | 改 | +15 |
| `internal/config/settings.go` | 改 | +10 |
| **新增合计** | | **+920** |
| **改动合计** | | **+145** |
| **测试合计** | | **~400** |
| **总计** | | **~1465** |

比 Step0-文件结构.md 估算的 1900 行少，原因是：
- render_*.go 文件极简（每个 ~15 行，而不是预估的 40-100 行）
- inline.go 是空壳（30 行 vs 预估 50 行）
- config.go 不含配置加载逻辑（60 行 vs 预估 100 行）

实际渲染逻辑的代码量在 Step 2 补上。
