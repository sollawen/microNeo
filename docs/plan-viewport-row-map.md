# Plan: Viewport 扁平化 Row→BufferLine 映射

## 问题

当前 `BufWindow.mdCache` 是 `[]md.SegmentMeta`，每个 segment 各自持有 `RowBufLines`。
每次 `screenOffsetToBufferLine()` 查询都需要遍历所有 segment 并累减 offset，复杂度 O(n)。

审查发现 `mdCache` 的**唯一消费者**就是 `screenOffsetToBufferLine`，没有其他代码读取它。
因此可以直接用扁平数组替代，彻底删除 `mdCache`。

## 方案

### 1. 新增数据结构

在 `BufWindow` 上新增扁平数组，**删除** `mdCache`：

```go
// 删除：
// mdCache  []md.SegmentMeta

// 新增：
// viewportRowBufLine[i] = viewport 第 i 个屏幕行对应的 buffer 行号
// -1 = 装饰行（代码块边框、表格分隔线等）
// -2 = 空白填充区域（buffer 内容不够填满 viewport）
// >=0 = 内容行对应的 buffer 行号
// 长度 = bufHeight，每帧 displayBufferMD 开始时重置
viewportRowBufLine []int
```

### 2. 写入时机：displayBufferMD 主循环

```go
func (w *BufWindow) displayBufferMD(editMode bool) {
    ...
    bufHeight := w.bufHeight

    // 懒分配：确保容量足够（resize 后 bufHeight 可能变化）
    if cap(w.viewportRowBufLine) < bufHeight {
        w.viewportRowBufLine = make([]int, bufHeight)
    }
    w.viewportRowBufLine = w.viewportRowBufLine[:bufHeight]
    // 重置为 -2
    for i := range w.viewportRowBufLine {
        w.viewportRowBufLine[i] = -2
    }

    // 渲染主循环
    vY := 0
    for _, seg := range segments {
        var rowBufLines []int
        if editMode && hasCursorInside(seg, cursors) {
            vY, rowBufLines = w.renderSegmentNative(seg, vY)
        } else {
            vY, rowBufLines = w.renderSegmentMD(seg, vY)
        }
        // ★ 写入扁平数组
        startVY := vY - len(rowBufLines)
        copy(w.viewportRowBufLine[startVY:vY], rowBufLines)
    }

    // 填充剩余空间（viewportRowBufLine 已经是 -2，无需额外写入）
    for ; vY < bufHeight; vY++ {
        w.drawGutterAndLineNumMD(vY, -1, false)
        for col := 0; col < bufWidth; col++ {
            screen.SetContent(w.X+w.gutterOffset+col, w.Y+vY, ' ', nil, defStyle)
        }
    }
}
```

注意：因为帧开始已经用 `-2` 填满整个数组，所以空白填充区域自动就是 `-2`，不需要额外处理。

### 3. 消费：screenOffsetToBufferLine 简化为 O(1)

装饰行（-1）和空白区域（-2）一视同仁：点击装饰行等于点击空白，`return (0, false)`。
装饰行本身没有任何 buffer 内容，用户点击它就应该像没点中任何东西一样。

```go
func (w *BufWindow) screenOffsetToBufferLine(screenOffset int) (int, bool) {
    if screenOffset < 0 || screenOffset >= len(w.viewportRowBufLine) {
        return 0, false
    }
    if w.viewportRowBufLine[screenOffset] >= 0 {
        return w.viewportRowBufLine[screenOffset], true
    }
    // 装饰行（-1）和空白区域（-2）一视同仁：没有对应 buffer 行
    return 0, false
}
```

### 4. 新增反向查找：bufferLineToScreenOffset

buffer 行号 → 该行在 viewport 中对应的**最后一个**屏幕行。
倒序遍历，第一个命中就是最大的 row。

```go
func (w *BufWindow) bufferLineToScreenOffset(bufferLine int) (int, bool) {
    for i := len(w.viewportRowBufLine) - 1; i >= 0; i-- {
        if w.viewportRowBufLine[i] == bufferLine {
            return i, true
        }
    }
    return 0, false
}
```

语义说明：返回的是该 buffer 行在 viewport 中占据的最后一个 screen row。
这对"滚动到 buffer 行并确保可见"场景是正确的。如果未来需要第一个 row，可以另加函数。

### 5. 删除 mdCache

`mdCache` 的唯一消费者是 `screenOffsetToBufferLine`，已被扁平数组替代，直接删除：

- 删除 `bufwindow.go` 中的 `mdCache` 字段
- 删除 `displayBufferMD` 中的 `mdCache` 写入逻辑（`w.mdCache = w.mdCache[:0]` 和 append）
- 删除 `screenOffsetToBufferLine` 中的旧遍历实现
- 更新注释

### 6. 测试重写

现有 `TestScreenOffsetToBufferLine` 基于 `mdCache` 构造测试数据，需要改为基于 `viewportRowBufLine`：

- 空数组 → `(0, false)`
- 内容行直接命中 → `(line, true)`
- 装饰行 → `(0, false)`（**语义变更**：旧行为是往后找 BufEndLine，新行为是一视同仁）
- 越界访问（负数、超长）→ `(0, false)`
- 多 segment 连续覆盖 → 验证扁平数组正确性
- 新增 `TestBufferLineToScreenOffset` 单元测试

## 涉及文件

| 文件 | 改动 |
|------|------|
| `internal/display/bufwindow.go` | `mdCache` → `viewportRowBufLine`；`LocFromVisual` 不变 |
| `internal/display/bufwindow_md.go` | 写入扁平数组；删除 mdCache 相关代码；简化 `screenOffsetToBufferLine`；新增 `bufferLineToScreenOffset` |
| `internal/display/bufwindow_md_test.go` | 重写测试 |

## 收益

- `screenOffsetToBufferLine`: O(n) → O(1)
- 新增 `bufferLineToScreenOffset` 反向查找能力
- 删除 `mdCache`，减少每帧内存分配和代码复杂度
- 装饰行点击语义更清晰一致
- 为后续需要 screen row ↔ buffer line 映射的功能（滚动同步、行号显示等）提供基础设施
