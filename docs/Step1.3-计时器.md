# Step 1.3：计时器

> 前置：Step 1.1 已完成（editMode 状态 + 键盘拦截 + per-segment 回退）
> 交付：10 秒无操作自动回到阅读模式

---

## 验收标准

- 编辑模式下 10 秒不按键/不点击 → 自动回到阅读模式
- 每次按键/点击重置倒计时
- `mdrenderidle=0` → 永不自动切回（保持编辑模式直到手动 Esc）
- `:set mdrenderidle 5` 运行时修改立即生效
- `mdrender=false` 关闭后计时器不启动

---

## 改动清单

### 1. 全局 idle channel

**文件**：`internal/action/bufpane.go`

```go
// MDIdleExpired 是计时器到期 channel。
// 每个 BufPane 的 idle timer goroutine 往这里发 *BufPane。
// 主循环 select case 消费，保证 exitEditMode 在主 goroutine 执行。
var MDIdleExpired = make(chan *BufPane, 16)
```

### 2. BufPane 增加计时器字段

**文件**：`internal/action/bufpane.go`

```go
type BufPane struct {
    // ... 现有字段 ...
    editMode    bool         // 1.1 已加
    mdIdleTimer *time.Timer  // nil = 非 MD 或 mdrenderidle=0
}
```

### 3. 计时器初始化

**文件**：`internal/action/bufpane.go`

在 `NewBufPaneFromBuf()` 的 `buf.IsMD` 分支里：

```go
if buf.IsMD {
    // ... 已有的 SetMDConfig ...

    h := newBufPane(buf, w, tab)

    h.editMode = false
    h.BWindow.SetEditMode(false)

    cfg := w.GetMDConfig()
    if cfg.MDRenderIdle > 0 {
        h.mdIdleTimer = time.NewTimer(time.Duration(cfg.MDRenderIdle) * time.Second)
        h.mdIdleTimer.Stop() // 初始阅读模式不计时
        // 启动等待 goroutine
        go h.idleTimerWatcher()
    }

    return h
}
```

### 4. idleTimerWatcher goroutine

**文件**：`internal/action/bufpane.go`

```go
// idleTimerWatcher 等待 timer 到期，然后通知主循环。
// 不碰任何共享状态——只读 timer.C，写 MDIdleExpired channel。
func (h *BufPane) idleTimerWatcher() {
    if h.mdIdleTimer == nil {
        return
    }
    <-h.mdIdleTimer.C
    MDIdleExpired <- h
}
```

**问题**：`time.Timer.C` 只会触发一次。`Reset` 之后需要重新启动 watcher goroutine。

**修正方案**：不用 watcher goroutine，改为每次 reset 时启动一个新的 one-shot goroutine：

```go
func (h *BufPane) resetIdleTimer() {
    if h.mdIdleTimer == nil {
        return
    }
    if !h.mdIdleTimer.Stop() {
        // drain
        select {
        case <-h.mdIdleTimer.C:
        default:
        }
    }
    h.mdIdleTimer.Reset(time.Duration(
        h.BWindow.GetMDConfig().MDRenderIdle,
    ) * time.Second)
    // 启动新的 watcher
    go func() {
        <-h.mdIdleTimer.C
        MDIdleExpired <- h
    }()
}
```

每次 reset 启动一个新 goroutine。旧 goroutine 的 `<-h.mdIdleTimer.C` 被 Stop+drain 后会阻塞，goroutine 泄漏。

**更好的方案**：直接用 `time.AfterFunc`，但回调只做一件事——发 channel：

```go
func (h *BufPane) resetIdleTimer() {
    if h.mdIdleTimer != nil {
        h.mdIdleTimer.Stop()
    }
    duration := time.Duration(h.BWindow.GetMDConfig().MDRenderIdle) * time.Second
    h.mdIdleTimer = time.AfterFunc(duration, func() {
        MDIdleExpired <- h // 只发 channel，不碰 editMode
    })
}
```

`time.AfterFunc` 的回调在 goroutine 里执行，但它只写 channel，不碰 `editMode`。`editMode` 的变更由主循环在收到 `MDIdleExpired` 后执行。**线程安全**。

`Stop()` 返回 false 说明已触发但 channel 还没消费——主循环会处理，不需要 drain。

### 5. enterEditMode / exitEditMode 改造

**文件**：`internal/action/bufpane.go`

1.1 已有基本版本，1.3 加上计时器逻辑：

```go
func (h *BufPane) enterEditMode() {
    h.editMode = true
    h.BWindow.SetEditMode(true)
    h.resetIdleTimer()
}

func (h *BufPane) exitEditMode() {
    h.editMode = false
    h.BWindow.SetEditMode(false)
    if h.mdIdleTimer != nil {
        h.mdIdleTimer.Stop()
    }
}
```

### 6. 主循环消费 MDIdleExpired

**文件**：`cmd/micro/micro.go`

在 `DoEvent()` 函数的 `select` 语句加一个 case：

```go
// 现有：
select {
case event := <-screen.Events:
    // ... 原有处理 ...
}

// Step 1.3 改为：
select {
case event := <-screen.Events:
    // ... 原有处理 ...
case bp := <-action.MDIdleExpired:
    bp.exitEditMode()
    // 下一帧 Display() 自然走 displayBufferMD(false)
}
```

不需要额外 Redraw——如果主循环是每帧重绘，下一帧自然生效。如果主循环是事件驱动（没事件就阻塞），可能需要加 `screen.Redraw()`：

```go
case bp := <-action.MDIdleExpired:
    bp.exitEditMode()
    screen.Screen.Fill(' ', config.DefStyle)  // 触发重绘
    // 或者直接让 select 继续，下一轮 DoEvent 开头的 Display 会处理
```

**实现时确认** Micro 的主循环模式（事件驱动 vs 连续重绘），选择是否需要显式 Redraw。

### 7. HandleEvent 中的 resetIdleTimer

**文件**：`internal/action/bufpane.go`

在 1.1 的 HandleEvent 改造基础上，编辑模式下每次按键/鼠标操作都调 `resetIdleTimer()`。

1.1 已经有了基本框架，1.3 只需确保 `resetIdleTimer()` 被正确调用：

```go
// EventKey 分支（1.1 已写）
if h.Buf.IsMD && h.editMode {
    if ke.Key() == tcell.KeyEsc {
        h.exitEditMode()
        return
    }
    h.resetIdleTimer()  // ← 1.3 新增
}
```

```go
// EventMouse 分支（1.2 已写）
if h.Buf.IsMD && h.editMode {
    h.resetIdleTimer()  // ← 1.3 新增
}
```

```go
// EventPaste 分支（1.2 已写）
if h.Buf.IsMD && h.editMode {
    h.resetIdleTimer()  // ← 1.3 新增
}
```

### 8. mdrenderidle 运行时修改

**文件**：`internal/action/bufpane.go`

用户 `:set mdrenderidle 5` 时需要更新 timer。Micro 的配置变更走 `OnOptionChange` 回调。

需要确认回调机制的具体接入点。可能的方案：

- **方案 A**：BufPane 有 `OnOptionChange(key string, value interface{})` 回调，在里面处理
- **方案 B**：在 `BufWindow` 的 `SetMDConfig` 里处理（每次 config 变更都重新传 config）

```go
// 方案 A：
func (h *BufPane) onMDOptionChange(key string, value interface{}) {
    if key == "mdrenderidle" {
        newIdle := value.(float64)
        if newIdle <= 0 {
            // 永不自动切回
            if h.mdIdleTimer != nil {
                h.mdIdleTimer.Stop()
            }
        }
        // 下次 enterEditMode 时会用新值 reset
    }
}
```

**实现时调研** Micro 的 config change callback 机制。

### 9. mdrenderidle=0 语义

初始化时：

```go
if cfg.MDRenderIdle > 0 {
    h.mdIdleTimer = time.AfterFunc(duration, func() {
        MDIdleExpired <- h
    })
    h.mdIdleTimer.Stop() // 初始不计时
} else {
    h.mdIdleTimer = nil // mdrenderidle=0 → 不创建 timer
}
```

`mdrenderidle=0` 时：
- `mdIdleTimer` 为 nil
- `enterEditMode()` 里的 `resetIdleTimer()` 检查 nil 直接 return
- 永不自动切回阅读模式
- 用户只能通过 Esc 手动切回

---

## 实施顺序

```
1. MDIdleExpired channel 声明
2. BufPane mdIdleTimer 字段
3. resetIdleTimer / exitEditMode 计时器逻辑
4. NewBufPaneFromBuf 里初始化 timer（mdrenderidle=0 处理）
5. HandleEvent 各分支加 resetIdleTimer 调用
6. cmd/micro/micro.go 主循环加 MDIdleExpired case
7. mdrenderidle 运行时修改支持
```

## 改动文件

| 文件 | 改动 |
|------|------|
| `internal/action/bufpane.go` | mdIdleTimer 字段、MDIdleExpired channel、resetIdleTimer、enter/exitEditMode 计时器逻辑、初始化、HandleEvent 各分支 resetIdleTimer |
| `cmd/micro/micro.go` | 主循环 select 加 MDIdleExpired case |

## 风险

| 风险 | 缓解 |
|------|------|
| `time.AfterFunc` 回调在 goroutine 里 | 回调只写 channel，不碰 editMode，线程安全 |
| Timer Stop 后旧的 AfterFunc 是否还触发 | `Stop()` 返回 false 说明已触发，但 MDIdleExpired 是 buffered channel(16)，不会阻塞；主循环消费时 pane 可能已经 exitEditMode 了——exitEditMode 是幂等的（再调一次无害） |
| mdrenderidle 运行时修改不生效 | 实现时调研 Micro config callback 机制 |

**估计代码量**：~40-60 行
