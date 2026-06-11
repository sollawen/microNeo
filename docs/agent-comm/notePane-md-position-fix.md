# NotePane MD 位置计算修复

## 问题

MD 文件开启渲染后，NotePane 位置计算偏高，上边框会盖住光标。

### 根因

MD 渲染会插入**装饰行**（如 H1 的 `===`、H2 的 `---`、blockquote 边框等），这些行不对应任何 buffer line，但占据屏幕行。

当前 `locToScreenRow()` 使用 `SLocFromLoc()` + `Diff()`，这两个函数只理解 softwrap（一个 buffer line 被折成多个 screen row），不知道装饰行的存在：

```
buffer line 0: # Heading    → 渲染为 2 screen rows（内容行 + === 装饰行）
buffer line 1: normal text  → 渲染为 1 screen row

Diff(StartLine={0,0}, SLocFromLoc({x,1})) = 1   ← 算出来 cursor 在 screen row 1
实际 cursor 在 screen row 2                      ← 差了 1 行（装饰行）
```

`lowestCursorScreenRow()` 返回值偏小，NotePane 上边框 = lowestRow + 1 太靠上，盖住光标。

## 现有基础设施

BufWindow 已实现 `viewportRowBufLine []int` 扁平数组：

```go
// viewportRowBufLine[i] = viewport 第 i 个屏幕行对应的 buffer 行号
// -1 = 装饰行（代码块边框、表格分隔线等）
// -2 = 空白填充区域
// >=0 = 内容行对应的 buffer 行号
viewportRowBufLine []int
```

这是 screen row → buffer line 的正向映射。已有方法 `screenOffsetToBufferLine()` 利用它做点击坐标映射。我们只需**反向查找**。

## 修复方案

### 1. 使用已有的 `bufferLineToScreenOffset()` 方法

根据 `viewportRowBufLine` 实现计划，已新增了 `bufferLineToScreenOffset` 方法（在 `bufwindow_md.go` 中）：

```go
// bufferLineToScreenOffset finds the LAST screen row offset for a given buffer line.
// Returns the bottom-most screen row this buffer line occupies.
func (w *BufWindow) bufferLineToScreenOffset(bufferLine int) (int, bool) {
    for i := len(w.viewportRowBufLine) - 1; i >= 0; i-- {
        if w.viewportRowBufLine[i] == bufferLine {
            return i, true
        }
    }
    return 0, false
}
```

**注意**：此方法返回**最后一个**匹配的 offset（即该 buffer line 在屏幕上最底部的位置）。这对 NotePane "确保不盖住光标所在行" 来说是正确的（使用最后一个 row，光标一定在 pane 上边框以下）。

### 2. 修改 `locToScreenRow()` 在 MD 文件时走 viewportRowBufLine 路径

```go
func (n *NotePane) locToScreenRow(bw *display.BufWindow, view *display.View, loc buffer.Loc) int {
    if bw.Buf.IsMD && bw.IsMDRender() {
        if offset, ok := bw.bufferLineToScreenOffset(loc.Y); ok {
            return view.Y + offset
        }
        // 回退到原始逻辑（viewportRowBufLine 未填充等边界情况）
    }
    sloc := bw.SLocFromLoc(loc)
    row := bw.Diff(view.StartLine, sloc)
    return row + view.Y
}
```

### 3. 新增 `IsMDRender()` 方法

`BufWindow` 目前没有公开方法判断 MD 渲染是否开启，需要在 `bufwindow_md.go` 中新增：

```go
// IsMDRender returns true if MD rendering is enabled for this buffer.
func (w *BufWindow) IsMDRender() bool {
    return w.Buf.IsMD && w.mdConfig.MDRender
}
```

## 改动清单

| 文件 | 改动 |
|------|------|
| `internal/display/bufwindow_md.go` | 使用已有的 `bufferLineToScreenOffset()` 方法 |
| `internal/display/bufwindow_md.go` | 新增 `IsMDRender()` 方法 |
| `internal/action/notepane.go` | `locToScreenRow()` 中 MD 文件走 viewportRowBufLine 路径 |

## 注意事项


- `viewportRowBufLine` 在每帧 `Display()` → `displayBufferMD()` 开始时重置。`open()` 在 Alt-i 按下时调用，此时 viewportRowBufLine 是上一帧渲染的数据，应该是最新的。
- `lowestCursorScreenRow()` 不需要改动，它调用 `locToScreenRow()`，后者会自动走 MD 路径。
- 装饰行的 `viewportRowBufLine[i] = -1`，不会匹配任何 buffer line，所以不会误返回装饰行的 offset。
- 如果 buffer line 不在可见区域（用户滚动走了），`bufferLineToScreenOffset` 返回 false，回退到原始逻辑。
