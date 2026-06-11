# NotePane Phase 2：嵌入 BufPane 实施计划

## 目标

将原型中的手写编辑器替换为嵌入 `*BufPane`，使 NotePane 获得完整的 microNeo 编辑能力。

## 当前状态（原型）

- `NotePane` 自己手写了输入/退格/方向键/换行等基本编辑
- 自己管理 `lines []string`、`curLine`、`curCol`
- 自己用 `screen.SetContent()` 画文本和边框
- 渲染和事件拦截在 `DoEvent()` 中已验证通过

## 新架构

```
type NotePane struct {
    *BufPane                    // 嵌入，获得完整编辑能力
    isOpen     bool
    x, y       int              // 浮窗边框的屏幕坐标
    width      int              // 浮窗宽度（含边框）
    height     int              // 内容高度（固定 5）
    noteFile   string
    savedPane  Pane             // 打开前记录的活跃 pane（关闭时恢复焦点）
}
```

## 关键设计决策

### 1. 复用 BufPane 的方式

参照 `InfoPane` 的模式：
- InfoPane 嵌入 `*BufPane`，传 `nil` tab，正常运行
- NotePane 同样嵌入 `*BufPane`，传 `nil` tab

### 2. BufWindow 的位置和尺寸

BufWindow 的 `X, Y, Width, Height` 是屏幕绝对坐标。NotePane 的 BufWindow 定位在**边框内部**：
- `X = n.x + 1`（左边框右侧）
- `Y = n.y + 1`（上边框下方）
- `Width = n.width - 2`（减去左右边框）
- `Height = n.height`（纯内容高度，5 行）

### 3. 隐藏 statusLine

BufWindow.Display() 调用 `displayStatusLine()`，在 statusline 关闭时会画 divider。

**方案**：在 `BufWindow` 增加 `hideStatusLine bool` 字段，`displayStatusLine()` 开头加：

```go
func (w *BufWindow) displayStatusLine() {
    if w.hideStatusLine {
        return
    }
    // ... 原有逻辑
}
```

这样 NotePane 的 BufWindow 设 `hideStatusLine = true`，不画任何 statusLine/divider。这是对 micro 原生代码唯一的改动（3 行）。

### 4. 独立 KeyTree（白名单 bindings）

BufPane 有 142 个 action，其中 11 个在 `tab==nil` 时会 panic，约 32 个对 NotePane 有副作用（打开文件对话框、退出 micro 等）。

NotePane 的定位是**临时输入缓冲区**，不是完整编辑器。因此采用白名单策略：

**方案**：参照 InfoPane 设置 `InfoBufBindings` 的模式，给 NotePane 的 BufPane 设置独立的 `NotePaneBindings` KeyTree，只注册允许的 action。

**白名单的优势**：
- 以后新增的 action 不会自动泄露到 NotePane，必须显式添加
- 实现方式是 micro 已有的模式（InfoPane 就这么做）
- 从根本上杜绝危险 action，无需 hack 拦截

**允许的 action 列表（约 80 个）**：

| 类别 | Action |
|------|--------|
| 光标移动 | CursorUp, CursorDown, CursorLeft, CursorRight, CursorPageUp, CursorPageDown, CursorStart, CursorEnd, CursorToViewTop, CursorToViewCenter, CursorToViewBottom |
| 选择 | SelectUp, SelectDown, SelectLeft, SelectRight, SelectToStart, SelectToEnd, SelectWordRight, SelectWordLeft, SelectSubWordRight, SelectSubWordLeft, SelectLine, SelectToStartOfLine, SelectToStartOfText, SelectToStartOfTextToggle, SelectToEndOfLine, SelectPageUp, SelectPageDown, StartOfText, StartOfTextToggle, StartOfLine, EndOfLine |
| 段落导航 | ParagraphPrevious, ParagraphNext, SelectToParagraphPrevious, SelectToParagraphNext |
| 文本编辑 | InsertNewline, Backspace, Delete, InsertTab, DeleteWordRight, DeleteWordLeft, DeleteSubWordRight, DeleteSubWordLeft, IndentLine, OutdentLine, IndentSelection, OutdentSelection |
| 翻页滚动 | PageUp, PageDown, HalfPageUp, HalfPageDown, ScrollUpAction, ScrollDownAction, Center, Start, End |
| 剪贴板 | Copy, CopyLine, Cut, CutLine, Paste, PastePrimary, Duplicate, DuplicateLine, DeleteLine, MoveLinesUp, MoveLinesDown |
| 多光标 | SpawnMultiCursor, SpawnMultiCursorUp, SpawnMultiCursorDown, SpawnMultiCursorSelect, RemoveMultiCursor, RemoveAllMultiCursors, SkipMultiCursor, SkipMultiCursorBack |
| 其他 | JumpToMatchingBrace, SelectAll, Deselect, Escape, ToggleOverwriteMode, ClearInfo, ClearStatus, None, MousePress, MouseDrag, MouseRelease, MouseMultiCursor |

**不允许的功能**（部分举例）：
- 🔴 危险：Quit, ForceQuit, QuitAll, VSplitAction, HSplitAction, Unsplit, NextSplit, PreviousSplit, FirstSplit, LastSplit, ResizePane
- 🟡 不适合：Save, SaveAs, SaveAll, OpenFile, Find, FindLiteral, FindNext, FindPrevious, DiffNext, DiffPrevious, JumpLine, ShellMode, CommandMode, Undo, Redo, ToggleHelp, ToggleKeyMenu, ToggleDiffGutter, ToggleRuler, Autocomplete, CycleAutocompleteBack, ToggleMacro, PlayMacro

**实现方式**：

```go
// 在 init() 或专门的初始化函数中
var NotePaneBindings *KeyTree

func initNotePaneBindings() {
    NotePaneBindings = NewKeyTree()
    // 从 BufBindings 中复制允许的绑定到 NotePaneBindings
    // 或者直接注册需要的 action
}

// 在 NewNotePane() 中
func NewNotePane() *NotePane {
    // ...
    bp := newBufPane(buf, win, nil)
    bp.bindings = NotePaneBindings
    // ...
}
```

### 5. 渲染流程

```
Display():
  1. 画 box-drawing 边框（用 screen.SetContent，和原型一样）
  2. 调用 n.BufPane.BWindow.Display()（BufWindow 自己画内容到边框内部区域）
  3. 调用 screen.Screen.ShowCursor()（BufPane 的光标位置需要转换为屏幕坐标）
```

BufWindow.Display() 会调用 `screen.SetContent` 画到正确的屏幕位置，因为它用的是绝对坐标。

### 6. 光标

BufWindow.Display() → displayBuffer() 内部已经调用了 `showCursor()`（`bufwindow.go:708`），
该方法在 `w.active == true` 时调用 `screen.ShowCursor(x, y)`。

所以 **NotePane 不需要自己处理光标**，只要确保 BufWindow 的 `active` 设为 `true`（默认就是），
BufWindow.Display() 就会自动把光标画到正确位置。

### 7. Buffer 管理

- 用 `buffer.NewBufferFromFile(noteFile, buffer.BTDefault)` 加载 notes.md
- 文件不存在时 `NewBufferFromFile` 会自动创建空 buffer（已验证 `buffer.go:315-318`）
- 不需要额外判断文件是否存在
- 关闭时调用 `Buf.Save()` 保存

### 8. open() 时 BufWindow 的 resize

每次打开 NotePane 时，位置可能变化。需要在 `open()` 中重新定位 BufWindow。

View 的 `X, Y, Width, Height` 是公开字段，可以直接修改（已验证 `bufwindow.go:62-64`）。
BufWindow 的 `Resize()` 方法会调用 `updateDisplayInfo()` 和 `Relocate()`，这些调用在 tab==nil 时安全。

```go
func (n *NotePane) open() {
    // 计算浮窗位置（和原型一样的逻辑）
    // ...
    
    // 重新定位 BufWindow 到边框内部
    bw := n.BufPane.BWindow.(*display.BufWindow)
    bw.X = n.x + 1
    bw.Y = n.y + 1
    n.BufPane.Resize(n.width - 2, n.height)
    
    n.isOpen = true
}
```

注意：首次调用 `Resize()` 会触发 `finishInitialize()`，其中 `initialRelocate()` 不依赖 tab（已验证 `bufpane.go:384-394`），`RunPluginFn("onBufPaneOpen")` 也不依赖 tab。安全。

## 需要修改的文件

| 文件 | 改动 | 侵入程度 |
|------|------|----------|
| `internal/action/notepane.go` | **重写**，去掉手写编辑器，嵌入 BufPane | 我们自己的文件 |
| `internal/display/bufwindow.go` | 加 `hideStatusLine` 字段 + displayStatusLine 开头加 3 行 | **极低**（~4行） |
| `cmd/micro/micro.go` | 无需改动（DoEvent 里的逻辑不变） | 无 |

## 实施步骤

### Step 1：BufWindow 加 hideStatusLine

在 `internal/display/bufwindow.go`：
1. BufWindow struct 加 `hideStatusLine bool`
2. `displayStatusLine()` 开头加：
   ```go
   if w.hideStatusLine {
       return
   }
   ```

### Step 2：重写 notepane.go

1. NotePane struct 改为嵌入 `*BufPane`
2. `NewNotePane()`：
   - 确定 noteFile 路径
   - 打开或创建 buffer
   - 创建 BufWindow（位置先设 0,0，open 时再调整），设 `hideStatusLine = true`
   - 用 `newBufPane(buf, win, nil)` 创建 BufPane（注意不要用 NewBufPane，避免触发 finishInitialize 太早）
3. `open()`：
   - 计算浮窗位置（复用原型的逻辑）
   - 定位 BufWindow 到边框内部
   - 调用 BufPane.Resize()
   - 设 `isOpen = true`
4. `close()`：
   - 调用 Buf.Save()
   - 设 `isOpen = false`
5. `Display()`：
   - 画边框（复用原型的 SetContent 逻辑）
   - 调用 `n.BufPane.BWindow.Display()`（BufWindow 内部会自动画内容和光标）
6. `HandleEvent(event)`：
   - 直接转发给 `n.BufPane.HandleEvent(event)`
   - 后续根据实际测试过滤危险 action

### Step 3：编译测试

```bash
make build-quick
```

### Step 4：验证

1. Alt-i 打开浮窗 → 看到带边框的编辑区域
2. 输入文字 → 正常显示
3. 测试 Ctrl-s（保存）→ 不应该触发（因为 NotePane 自己管理保存）
4. 测试 Backspace, Enter, 方向键 → 完整编辑体验
5. Alt-i 关闭 → 确认 notes.md 已保存
6. 再次打开 → 内容正确加载

## 风险与应对

| 风险 | 应对 |
|------|------|
| BufPane 的危险 action 在 tab=nil 时 panic | 用独立 NotePaneBindings KeyTree 白名单策略，从根本上杜绝 |
| BufWindow 的 Display() 画到了边框外面 | 检查 BufWindow 的 Width/Height 是否正确扣除了边框 |
| BufPane 的 MD 渲染配置 | notes.md 是 .md 文件，但我们自己创建 BufPane + BufWindow（不用 NewBufPaneFromBuf），不会调用 initMDConfig，所以 MD 渲染不会被初始化 |

## 注意事项

- **必须用 `newBufPane`**（小写），不用 `NewBufPane` 或 `NewBufPaneFromBuf`：
  - `newBufPane`：仅创建对象，不触发 finishInitialize（`bufpane.go:259-267`）
  - `NewBufPane`：会立即触发 `finishInitialize()`，但此时窗口尺寸未定
  - `NewBufPaneFromBuf`：会调用 `initMDConfig()`，NotePane 不需要 MD 渲染初始化
- 首次 `open()` 时的 `Resize()` 调用会触发 `finishInitialize()`，其中 `initialRelocate()` 不依赖 tab（`bufpane.go:384-394`），`RunPluginFn` 也不依赖 tab。安全。
