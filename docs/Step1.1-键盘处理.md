# Step 1.1：键盘处理 + per-segment 回退

> 前置：Step 0 已完成
> 交付：方向键滚动 / 字母键进编辑模式 / Esc 回阅读模式 / 光标所在 segment 回退原生

---

## 验收标准

打开 .md 文件 → 看到背景色渲染 → ↑↓ 滚动 → 按字母键 → 光标附近几行变原生，其余继续渲染 → Esc → 全部恢复渲染。

非 MD 文件不受影响。

---

## 改动清单

### 1. BufWindow 增加 editMode

**文件**：`internal/display/bufwindow.go`

```go
type BufWindow struct {
    // ... 现有字段 ...
    editMode  bool  // true=编辑模式, false=阅读模式
}

func (w *BufWindow) SetEditMode(mode bool) {
    w.editMode = mode
}

func (w *BufWindow) GetMDConfig() md.MDConfig {
    return w.mdConfig
}
```

### 2. Display() 分流

**文件**：`internal/display/bufwindow.go`

```go
// 原来（Step 0）：
if w.Buf.IsMD {
    w.displayBufferMD()
} else {
    w.displayBuffer()
}

// Step 1.1 改为：
if !w.Buf.IsMD || !w.mdConfig.MDRender {
    w.displayBuffer()
} else {
    w.displayBufferMD(w.editMode)
}
```

先排除 `!IsMD || !MDRender`（非 MD 文件或总开关关闭），剩下的全部交给 `displayBufferMD(editMode)`——`editMode=false` 纯渲染，`editMode=true` 光标片回退原生。

### 3. displayBufferMD(hasEditSegment bool) 改造

**文件**：`internal/display/bufwindow.go`

editMode=true 时，只有光标所在 segment 回退原生：

```go
func (w *BufWindow) displayBufferMD(hasEditSegment bool) {
    b := w.Buf
    // ... 前置逻辑同 Step 0 ...

    // 确定光标所在 segment
    var cursorSegStart, cursorSegEnd int = -1, -1
    if hasEditSegment {
        cursorY := b.GetActiveCursor().Loc.Y
        for _, seg := range segments {
            if cursorY >= seg.BufStartLine && cursorY <= seg.BufEndLine {
                cursorSegStart = seg.BufStartLine
                cursorSegEnd = seg.BufEndLine
                break
            }
        }
    }

    // 渲染管线主循环
    vY := 0
    for _, seg := range segments {
        if hasEditSegment && seg.BufStartLine == cursorSegStart {
            // 光标所在 segment → 原生渲染
            vY = w.displayBufferLinesNative(vY, cursorSegStart, cursorSegEnd)
        } else {
            // 其他 segment → 渲染输出（同 Step 0）
            // ... 原有 render + 写 screen 逻辑 ...
        }

        // mdCache 副作用（同 Step 0）
    }

    // 填充剩余空间（同 Step 0）
}
```

### 4. displayBufferLinesNative 提取

**文件**：`internal/display/bufwindow.go`

从 `displayBuffer()` 提取行渲染核心为独立函数：

```go
// displayBufferLinesNative 用原生逻辑渲染指定 buffer 行范围。
// startVY: 起始 screen 行（相对窗口）
// bufStart, bufEnd: buffer 行范围（含两端）
// 返回: 消耗的 screen 行数
func (w *BufWindow) displayBufferLinesNative(startVY, bufStart, bufEnd int) int {
    // 从 displayBuffer() 复制核心循环：
    // - matchbrace 检测
    // - showchars 解析
    // - getRuneStyle 闭包
    // - draw 闭包
    // - wrap 闭包
    // 只遍历 bufStart..bufEnd 行
    // 返回消耗的行数
}
```

提取后 `displayBuffer()` 也改为调用它：

```go
func (w *BufWindow) displayBuffer() {
    // ... 前置逻辑（gutter、行号长度等）...
    w.displayBufferLinesNative(0, w.StartLine.Line, lastLine)
    // ... 后置逻辑（status line、scroll bar）...
}
```

**提取要点**：
- `displayBuffer()` 的 5 层闭包变量需要作为参数传入（或封装成结构体）
- 闭包本身可以保持为匿名函数，在 `displayBufferLinesNative` 内部定义
- 保证提取后 `displayBuffer()` 行为不变——这是回归验证的基础

**估计工作量**：~80-120 行提取 + 重构

### 5. BufPane 增加 editMode + 基本方法

**文件**：`internal/action/bufpane.go`

```go
type BufPane struct {
    // ... 现有字段 ...
    editMode bool  // true=编辑模式, false=阅读模式
}

func (h *BufPane) enterEditMode() {
    h.editMode = true
    h.BWindow.SetEditMode(true)
}

func (h *BufPane) exitEditMode() {
    h.editMode = false
    h.BWindow.SetEditMode(false)
}
```

### 6. HandleEvent 键拦截

**文件**：`internal/action/bufpane.go`

```go
case *tcell.EventKey:
    ke := keyEvent(e)

    if h.Buf.IsMD && !h.editMode {
        switch ke.Key() {
        case tcell.KeyUp:
            h.ScrollUp(1)
            return
        case tcell.KeyDown:
            h.ScrollDown(1)
            return
        case tcell.KeyPgUp:
            h.ScrollUp(h.BufView().Height)
            return
        case tcell.KeyPgDn:
            h.ScrollDown(h.BufView().Height)
            return
        case tcell.KeyHome:
            h.scrollMDToTop()
            return
        case tcell.KeyEnd:
            h.scrollMDToBottom()
            return
        case tcell.KeyCtrlHome:
            h.scrollMDToTop()
            return
        case tcell.KeyCtrlEnd:
            h.scrollMDToBottom()
            return
        case tcell.KeyEsc:
            // Esc 在阅读模式下无操作（或忽略）
            return
        default:
            h.enterEditMode()
            // 不 return，继续走 DoKeyEvent
        }
    }

    // 编辑模式下的键处理
    if h.Buf.IsMD && h.editMode {
        // Esc → 回到阅读模式（1.3 做完前的主要出口）
        if ke.Key() == tcell.KeyEsc {
            h.exitEditMode()
            return
        }
    }

    done := h.DoKeyEvent(ke)
    // ... 后续逻辑不变 ...
```

### 7. scrollMDToTop / scrollMDToBottom

**文件**：`internal/action/bufpane.go`

这两个函数在 Micro action 系统里不存在，需要自己实现：

```go
func (h *BufPane) scrollMDToTop() {
    // 方案 A：如果 BufWindow 有 SetStart 方法
    h.BufView().SetStart(display.SLoc{Line: 0, Row: 0})

    // 方案 B（备选）：移动光标到顶部 + Relocate
    // h.GetActiveCursor().Loc = buffer.Loc{X: 0, Y: 0}
    // h.Relocate()
}

func (h *BufPane) scrollMDToBottom() {
    lastLine := h.Buf.LinesNum() - 1
    height := h.BufView().Height
    startLine := lastLine - height + 1
    if startLine < 0 {
        startLine = 0
    }
    h.BufView().SetStart(display.SLoc{Line: startLine, Row: 0})
}
```

**实现时确认** BufWindow 是否有 `SetStart(SLoc)` 方法。如果没有，需要在 `softwrap.go` 或 `bufwindow.go` 中添加。

### 8. Relocate 问题

`ScrollUp`/`ScrollDown` action 内部调 `Relocate()`，会把视窗拉回光标附近。阅读模式下光标不可见，纯滚动时不应该受影响。

**Step 1.1 的临时方案**：先接受 Relocate 的行为。如果体验明显差（按↓时画面跳到光标位置），再在 1.1 过程中加 `renderMode` 标志让 Relocate 提前返回。

---

## 实施顺序

```
1. BufWindow editMode 字段 + SetEditMode + GetMDConfig
2. BufPane editMode 字段 + enterEditMode / exitEditMode
3. Display() 分流逻辑
4. displayBufferLinesNative 提取
5. displayBufferMD(hasEditSegment) 改造
6. HandleEvent 键拦截
7. scrollMDToTop / scrollMDToBottom
8. Esc 键处理
```

## 改动文件

| 文件 | 改动 |
|------|------|
| `internal/display/bufwindow.go` | editMode 字段、SetEditMode/GetMDConfig、Display 分流、displayBufferLinesNative 提取、displayBufferMD 改造 |
| `internal/action/bufpane.go` | editMode 字段、enter/exitEditMode、HandleEvent 键拦截、scrollMDToTop/Bottom、Esc 处理 |
| `internal/display/softwrap.go` | 可能需要加 SetStart 方法 |

## 风险

| 风险 | 缓解 |
|------|------|
| displayBufferLinesNative 提取复杂（520 行 5 层闭包） | 提取后 displayBuffer() 改为调用它，行为可通过对比测试验证 |
| Relocate 在阅读模式下干扰滚动 | 先接受，体验差再修 |
| 滚动函数不存在（ScrollToTop/Bottom） | 实现时确认 BufWindow 公开方法 |

**估计代码量**：~150-250 行（主要是 displayBufferLinesNative 提取）
