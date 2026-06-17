# D10 — 抽取 `reposition()`，解耦 resize 与 buffer 销毁（第一优先）

> **本计划取代 D9**。D9 的"早退补丁"是治标，本计划治本——把"重建 buffer"和"重新定位"两个职责拆成两个函数。

## 目标

把 `NotePane.open()` 的双职责拆开，让 `open()` 只管 buffer 重建，新增 `reposition()` 专门管窗口位置计算。resize 事件改调 `reposition()`，**永远不碰 buffer**。

## 现状（设计问题）

`internal/action/notepane.go:288-326` 的 `open()` 函数干了**两件事**：

```go
func (n *NotePane) open() {
    pane := MainTab().CurPane()
    if pane == nil { return }

    // ── 职责 A：销毁并重建 buffer ──
    if n.BufPane.Buf != nil {
        n.BufPane.Buf.Close()
    }
    buf := buffer.NewBufferFromString("", "", buffer.BTScratch)
    buf.SetOptionNative("ruler", false)
    nbw := n.BufPane.BWindow.(*display.BufWindow)
    nbw.SetBuffer(buf)
    n.BufPane.Buf = buf

    // ── 职责 B：计算窗口位置 ──
    n.filePath = pane.Buf.AbsPath
    view := pane.BWindow.GetView()
    bw := pane.BWindow.(*display.BufWindow)
    lowestRow := n.lowestCursorScreenRow(bw, view)
    n.x = view.X
    n.width = view.Width
    // ... 30 行位置计算 ...

    n.isOpen = true
}
```

调用方（`notepane.go:495-501`）：

```go
func (n *NotePane) HandleEvent(event tcell.Event) {
    if _, ok := event.(*tcell.EventResize); ok {
        if n.isOpen {
            n.open()  // ← 只想"重定位"，却被强塞了"重建 buffer"
        }
        return
    }
    n.BufPane.HandleEvent(event)
}
```

**设计错误**：resize 只想要"重定位窗口"（职责 B），但 `open()` 是 `职责 A + 职责 B` 打包的。调用方为了 B，被迫接受 A 的副作用——**buffer 被销毁**。

这就是个"时炸弹"：终端 resize 极频繁（任何 panel 尺寸变化都触发），用户输入内容后只要 resize 一次就丢。

## 修复方案

### 1. 抽取 `reposition()` 函数

```go
// reposition 重新计算 NotePane 在屏幕上的位置。
// 不修改 buffer 内容，可在已开态下重复调用（用于 resize）。
// 内部从 MainTab().CurPane() 取主编辑器 pane。
func (n *NotePane) reposition() {
    pane := MainTab().CurPane()
    if pane == nil {
        return
    }
    // 防御：主编辑器 buffer 被关（如最后一个 tab 关闭）时 pane.Buf 为 nil
    if pane.Buf == nil {
        return
    }

    // Capture file path from the main editor buffer
    n.filePath = pane.Buf.AbsPath

    // Get the pane's view and BufWindow
    view := pane.BWindow.GetView()
    bw := pane.BWindow.(*display.BufWindow)

    // 1. Find the lowest cursor/selection screen row
    lowestRow := n.lowestCursorScreenRow(bw, view)

    // 2. Calculate NotePane position
    n.x = view.X
    n.width = view.Width
    notePaneTopBorder := lowestRow + 1
    notePaneBottomBorder := notePaneTopBorder + n.height + 1

    // 3. If not enough space below, scroll the main editor up
    viewBottom := view.Y + view.Height
    if notePaneBottomBorder > viewBottom {
        deficit := notePaneBottomBorder - viewBottom + 2

        scrollmargin := int(pane.Buf.Settings["scrollmargin"].(float64))
        maxDeficit := lowestRow - scrollmargin
        if deficit > maxDeficit {
            deficit = maxDeficit
        }

        if deficit > 0 {
            oldStartLine := view.StartLine
            view.StartLine = bw.Scroll(view.StartLine, deficit)
            lowestRow -= bw.Diff(oldStartLine, view.StartLine)
            notePaneTopBorder = lowestRow + 1
        }
    }
    n.y = notePaneTopBorder

    // 4. Reposition BufWindow
    nbw := n.BufPane.BWindow.(*display.BufWindow)
    nbw.X = n.x + 1
    nbw.Y = n.y + 1
    n.BufPane.Resize(n.width-2, n.height)
}
```

### 2. `open()` 简化为只管 buffer 重建 + 调 reposition

```go
// open 在"关闭→打开"转换时调用：销毁旧 buffer、建新 scratch、调 reposition 算位置。
// 已在开态下重复调用是 no-op（防御深度）。
func (n *NotePane) open() {
    if n.isOpen {
        return  // 防御：只在关闭态生效
    }
    pane := MainTab().CurPane()
    if pane == nil {
        return
    }

    // 1. Reset buffer
    if n.BufPane.Buf != nil {
        n.BufPane.Buf.Close()
    }
    buf := buffer.NewBufferFromString("", "", buffer.BTScratch)
    buf.SetOptionNative("ruler", false)
    nbw := n.BufPane.BWindow.(*display.BufWindow)
    nbw.SetBuffer(buf)
    n.BufPane.Buf = buf

    // 2. Calculate position（委托给 reposition）
    n.reposition()

    n.isOpen = true
}
```

### 3. resize 处理改调 `reposition()`

```go
func (n *NotePane) HandleEvent(event tcell.Event) {
    if _, ok := event.(*tcell.EventResize); ok {
        if n.isOpen {
            n.reposition()  // ← 关键：只调需要的，不动 buffer
        }
        return
    }
    n.BufPane.HandleEvent(event)
}
```

## 行为矩阵（修复后）

| 触发场景 | 旧实现 | D10 修复后 |
|---|---|---|
| `Toggle()` 关 → 开 | open: 重置 buffer + 算位置 | open: 重置 buffer + 调 reposition ✅ 一致 |
| `Toggle()` 开 → 关 | 不调 open | 不调 open |
| **Resize 时已开** | **open: 重置 buffer ❌ 丢内容** | **reposition: 只算位置 ✅ 内容保留** |
| Resize 时关闭 | 不调 open | 不调 reposition |
| `notePaneOpen` action（D8）| 重置 buffer | 守卫 + 只在 `!isOpen` 时跑 ✅ |

## API 设计要点

### `reposition()` 内部 fetch pane

为什么不让 caller 传 `pane *BufPane` 参数：

- caller（resize handler）原本就没有 pane 在作用域里
- 外部 caller 加参数会污染 resize handler
- "reposition 自己 fetch"和"open 自己 fetch"对称，调用点更简洁

### `open()` 加 `if n.isOpen { return }`

即使 caller（`Toggle()`）已经守卫，双重保险值得要——

- 未来如果有别的 caller 误调，defense in depth
- 行为变成幂等（idempotent），符合 Go 习惯

## 改动量

**单文件 1 处**：`internal/action/notepane.go`

| 位置 | 变化 |
|---|---|
| 新增 `reposition()` 方法 | +30 行（原 open() 内的位置计算 + 函数包裹） |
| `open()` 精简 | -30 行 / +10 行（buffer reset + 调 reposition + 守卫） |
| `HandleEvent` resize 分支 | -1 / +1（`n.open()` → `n.reposition()`） |
| **净变化** | **+10 行** |

零新文件，零新类型，零 API 破坏（`open()` 仍存在，签名不变）。

## 与其他计划的关系

| 计划 | 状态 |
|---|---|
| **D8**（主键位可配置）| 等 ❶❷❸ 决策，与 D10 独立 |
| **D9**（`if n.isOpen { return }` 早退补丁）| **被 D10 取代**——D10 内部已包含 `if n.isOpen` 守卫 + 职责分离，更彻底。不必单独实施。 |

## 验证步骤

1. `make build` 编译
2. 打开任意文件 → `Alt-i` 开 notePane → 输入一段文字
3. **resize 终端窗口（最大化、还原、改宽度）多次** → 文字应**持续保留** ✅
4. `Alt-i` 关 → `Alt-i` 再开 → 内容应清空（scratch 行为）✅
5. `Alt-i` 开 → 输入 → `Alt-Enter` 发送 → 关闭 → 重新 `Alt-i` 开 → 内容应清空 ✅
6. 在 notePane 输入 → resize → 重新选主编辑器文件 → notePane 应跟随主编辑器 x/width ✅
7. `go vet ./...` 不应有 warning
8. `git diff` 确认只动 `notepane.go` 一个文件
9. 边缘场景：开 notePane → 输入文字 → 关主编辑器最后一个 tab → `pane.Buf == nil` 守卫应生效，**不 panic** ✅

## 风险评估

- **行为变化**：`open()` 改后只在关闭态生效，但**唯一 caller（`Toggle()`）本来就只在关闭态调用**（`notepane.go:265`），实际无变化
- **API 变化**：`open()` 签名未变，`reposition()` 是新增方法
- **测试覆盖**：需要手动测试 resize（自动测试 TUI 较难）
- **风险等级**：**低**——结构清晰了，bug 反而更少

## Lisa 评审反馈（已采纳）

Lisa 评审结论：**计划可以实施，无 blocker**。采纳的加固建议：

- **`reposition()` 增加 `if pane.Buf == nil { return }` 守卫**——处理"主编辑器 buffer 被关掉"的边缘场景，避免 NPE。这是预存在的脆弱性，D10 是修复它的好机会（因为 `pane.Buf` 访问路径在重构后被收敛到 `reposition()` 一处）。

未采纳的（仅为提醒）：
- `git diff --stat` 验证步骤：已并入"确认只动一个文件"那一步

---

## 实施建议

作为独立 commit 提交，commit message 大致：

```
refactor(notepane): extract reposition(), decouple resize from buffer reset

NotePane.open() previously did two things: reset buffer and recalculate
position. The resize handler reused open() for position recalc, but
inherited the destructive buffer-reset side effect — causing user-typed
content to vanish on every terminal resize.

Split the two responsibilities:
- open(): reset buffer + delegate to reposition()
- reposition(): pure position calculation, idempotent, safe to call
  on every resize

Bug: terminal resize now preserves user-typed content in notePane.
```
