## 现有基础设施

BufWindow 已有 `mdCache`（每帧 `displayBufferMD` 后存储），结构为 `[]md.SegmentMeta`：

```go
type SegmentMeta struct {
    BufStartLine int
    BufEndLine   int
    RowBufLines  []int  // RowBufLines[i] = 第 i 个 screen row 的 BufLine（装饰行 = -1）
}
```

这是 screen row → buffer line 的正向映射。已有方法 `screenOffsetToBufferLine()` 利用它做点击坐标映射。我们只需**反向查找**。
