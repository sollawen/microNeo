# Step 1.1：键盘处理

> 前置：Step 1.0 已完成（editMode 状态 + displayBufferLinesNative + per-segment 回退显示）
> 交付：方向键滚动 / 字母键进编辑模式 / Esc 回阅读模式

---

## 验收标准

打开 .md 文件 → 看到背景色渲染（阅读模式）→ ↑↓ 滚动 → 按字母键 → 光标附近几行变原生（编辑模式）→ Esc → 全部恢复渲染（阅读模式）。

非 MD 文件不受影响。

---

## 前置依赖（Step 1.0 已交付）

Step 1.0 已完成以下基础设施，本步直接使用：

- `BufWindow.editMode` 字段 + `SetEditMode()` / `GetMDConfig()`
- `BufPane.editMode` 字段 + `enterEditMode()` / `exitEditMode()`
- `Display()` 已分流：`displayBufferMD(w.editMode)`
- `displayBufferMD(hasEditSegment)` 已支持光标所在 segment 回退原生
- `displayBufferLinesNative` 已提取完成

本步需要做的：
- 将 MD 文件默认 editMode 从 `true`（1.0 临时）改为 `false`（阅读模式）
- 在 `HandleEvent` 中增加键盘拦截逻辑
- 实现 `scrollMDToTop` / `scrollMDToBottom`

---

## 改动清单

### 1. 默认模式改为阅读模式

**文件**：`internal/action/bufpane.go`

Step 1.0 临时默认 `editMode = true`，本步改为 `false`：

```go
// NewBufPaneFromBuf 的 MD 分支
if buf.IsMD {
    h := newBufPane(buf, w, tab)
    h.editMode = false          // 阅读模式（1.0 的 true 改为 false）
    h.BWindow.SetEditMode(false)
    return h
}
```

### 2. HandleEvent 键拦截

**文件**：`internal/action/bufpane.go`

```go
case *tcell.EventKey:
    ke := keyEvent(e)

    // MicroNeo: 阅读模式下拦截方向键
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
            // Esc 在阅读模式下无操作
            return
        default:
            // 其他键 → 进入编辑模式，继续走 DoKeyEvent
            h.enterEditMode()
        }
    }

    // 编辑模式下：Esc 回阅读模式
    if h.Buf.IsMD && h.editMode {
        if ke.Key() == tcell.KeyEsc {
            h.exitEditMode()
            return
        }
    }

    done := h.DoKeyEvent(ke)
    // ... 后续逻辑不变 ...
```

**拦截点选在 `HandleEvent` 而非 `DoKeyEvent` 的原因**（同架构设计 §5.2）：
- `DoKeyEvent` 走 KeyTree 绑定查找，在那里拦截需要先解析 key 再判断模式
- `HandleEvent` 拿到原始 `tcell.EventKey` 后第一时间判断，逻辑最直

**其他滚动入口**：
- `MouseWheelUp/Down`：已绑定到 `ScrollUp/ScrollDown` action，阅读模式下行为正确，**不需要额外拦截**
- `Ctrl+PageUp/Down`：默认是 `PreviousTab/NextTab`（标签页切换），**不应该拦截**

### 3. scrollMDToTop / scrollMDToBottom

**文件**：`internal/action/bufpane.go`

这两个函数在 Micro action 系统里不存在，需要自己实现：

```go
func (h *BufPane) scrollMDToTop() {
    h.BufView().SetStart(display.SLoc{Line: 0, Row: 0})
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

**实现时确认** `BufWindow` 是否有 `SetStart(SLoc)` 方法。如果没有，需要在 `softwrap.go` 或 `bufwindow.go` 中添加。

### 4. Relocate 问题

`ScrollUp`/`ScrollDown` action 内部调 `Relocate()`，会把视窗拉回光标附近。阅读模式下光标不可见，纯滚动时不应该受影响。

**Step 1.1 的临时方案**：先接受 Relocate 的行为。如果体验明显差（按 ↓ 时画面跳到光标位置），再在本步过程中加 `renderMode` 标志让 Relocate 提前返回。

---

## 实施顺序

```
1. 默认 editMode 改为 false（阅读模式）
2. HandleEvent 键拦截（阅读模式分支 + 编辑模式 Esc 分支）
3. scrollMDToTop / scrollMDToBottom 实现
4. 确认 BufWindow.SetStart 方法是否存在，不存在则添加
5. 验证：↑↓ 滚动、字母键进入编辑、Esc 退出
```

## 改动文件

| 文件 | 改动 |
|------|------|
| `internal/action/bufpane.go` | 默认 editMode 改 false、HandleEvent 键拦截、scrollMDToTop/Bottom |
| `internal/display/softwrap.go` | 可能需要加 SetStart 方法 |

## 风险

| 风险 | 缓解 |
|------|------|
| Relocate 在阅读模式下干扰滚动 | 先接受，体验差再修 |
| 滚动函数不存在（SetStart） | 实现时确认 BufWindow 公开方法 |
| Ctrl+Home/End 拦截可能与终端快捷键冲突 | 实现时在主流终端（iTerm2、Terminal.app）实测 |

**估计代码量**：~30-50 行
