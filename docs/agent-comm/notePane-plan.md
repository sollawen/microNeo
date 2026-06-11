# NotePane 浮窗功能 — 可行性分析与实施计划

## 需求回顾（来自 用户界面-V2.md）

- 在编辑 fileA 时，按某快捷键打开浮动窗口 `notePane`
- `notePane` 在当前光标位置下方出现，左右充满编辑窗口，高度固定 5 行
- 直接覆盖在 fileA 前面，冻结 fileA（unfocus）
- notePane 内有完整的 microNeo 编辑功能
- 内容保存在固定名字的 md 文件里（如 `~/.config/microNeo/notes.md`）
- 不要 micro 的 statusLine，边框用 ASCII 表格外框风格（`┌─┐│└─┘` 等 box-drawing 字符）

---

## 可行性分析

### 结论：**完全可行**，且可以做到对 micro 原生代码 **极低侵入**。

### 理由

1. **micro 的架构天然支持新 Pane 类型**  
   - 已有 `BufPane`、`TermPane`、`RawPane`、`InfoPane` 四种 Pane 实现
   - `Pane` 接口非常简洁：`Handler` + `display.Window` + `ID/SetID/Name/Close/SetTab/Tab`
   - 新增 `NotePane` 只需实现 `Pane` 接口即可

2. **浮窗渲染可以直接用 tcell 的 `SetContent`**  
   - 当前 `DoEvent()` 的渲染流程是：`Fill → Tabs.Display → Panes[i].Display → UI borders → InfoBar.Display → Screen.Show()`
   - NotePane 不走 Tab 的 split tree，而是作为一个 **全局覆盖层** 独立渲染
   - 只需在 `DoEvent()` 的渲染链中，在 `InfoBar.Display()` 之后、`Screen.Show()` 之前插入 NotePane 的渲染
   - Box-drawing 边框直接用 `screen.SetContent()` 画

3. **事件拦截机制清晰**  
   - 在 `DoEvent()` 的事件分发阶段，若 NotePane 已打开，直接将事件发给 NotePane 而非 Tabs
   - 这与当前 `InfoBar.HasPrompt` 的拦截模式完全一致
   - 无需修改 Tab/Pane 的任何事件处理逻辑

4. **BufPane 可复用**  
   - NotePane 内部可以嵌入 `*BufPane`（与 `InfoPane` 嵌入 `*BufPane` 的模式相同）
   - Buffer 从固定路径 `notes.md` 加载，享有全部编辑能力
   - 唯一需要定制的是不显示 statusLine（BufWindow 的 statusLine 可以通过配置关闭或跳过）

---

## 架构设计

### 整体思路

```
┌──────────────────────────────────────────────┐
│  DoEvent() 渲染链                              │
│                                               │
│  1. Screen.Fill(' ')                          │
│  2. Tabs.Display()          ← 正常的 tab/pane  │
│  3. for panes: pane.Display()                 │
│  4. MainTab().Display()      ← split borders  │
│  5. InfoBar.Display()                         │
│  ★ 6. NotePane.Display()   ← [新增] 覆盖层     │
│  7. Screen.Show()                             │
└──────────────────────────────────────────────┘

┌──────────────────────────────────────────────┐
│  DoEvent() 事件分发                            │
│                                               │
│  if NotePane isOpen → NotePane.HandleEvent()  │
│  else if InfoBar.HasPrompt → InfoBar.Handle() │
│  else → Tabs.HandleEvent()                    │
└──────────────────────────────────────────────┘
```

### NotePane 结构

```go
// internal/action/notepane.go (新文件)

type NotePane struct {
    *BufPane                         // 复用全部编辑能力
    isOpen     bool                  // 是否处于打开状态
    origView   display.View          // 打开时记录的底层 pane 视口（用于定位）
    savedPane  Pane                  // 打开时被冻结的底层 pane
    width      int                   // 浮窗宽度
    height     int                   // 浮窗高度（含边框 = 7: 上框1 + 内容5 + 下框1）
    noteFile   string                // notes.md 的路径
}
```

### 渲染策略

NotePane 的 BufWindow 定位在光标下方的屏幕区域：
- `X = 当前 pane 的 View.X`（左右充满编辑窗口）
- `Y = min(cursorScreenY + 1, screenH - totalHeight)`（在光标下方，但不超出屏幕底部）
- `Width = 当前 pane 的 View.Width`
- `Height = 5`（纯内容区，不含边框）

边框用 box-drawing 字符直接用 `screen.SetContent()` 绘制，不经过 BufWindow。

### BufWindow 定制

NotePane 需要一个**不带 statusLine** 的 BufWindow。方案：
- 方案 A：BufWindow 增加一个 `hideStatusLine bool` 字段（改动 1 行 display 逻辑）
- 方案 B：创建 `NoteWindow` 嵌入 BufWindow，覆盖 `Display()` 方法

**推荐方案 A**，改动极小，只需在 `BufWindow.displayStatusLine()` 开头加一行判断。

---

## 实施步骤

### Phase 1：核心框架（新文件 + 最小改动）

#### 1.1 新建 `internal/action/notepane.go`

实现 `NotePane` 结构体，满足 `Pane` 接口：

```go
type NotePane struct {
    *BufPane
    isOpen    bool
    savedPane Pane
    x, y      int     // 浮窗屏幕位置
    width     int     // 内容宽度
    height    int     // 内容高度（固定 5）
    noteFile  string
}
```

关键方法：
- `Open()` — 计算位置，创建/加载 buffer，显示浮窗
- `Close()` — 保存 buffer，隐藏浮窗，恢复焦点
- `Toggle()` — 切换开关（绑定到快捷键）
- `Display()` — 画边框 + 让内部 BufPane 的 BufWindow 只画内容区
- `HandleEvent()` — **完全模态**：所有键盘/鼠标事件都只给 NotePane，连 `Ctrl-E`、`Escape` 也不会穿透到底层。**唯一**关闭方式是再次按 `Alt-i`（和打开是同一个键），其余全部转发给内部 BufPane

#### 1.2 新建全局变量

在 `globals.go` 或 `notepane.go` 中添加：

```go
var NotePaneInst *NotePane
```

在 `InitGlobals()` 中初始化（预加载 notes.md buffer）。

#### 1.3 修改 `DoEvent()` (cmd/micro/micro.go)

**渲染部分**（在 `InfoBar.Display()` 之后加一行）：
```go
action.InfoBar.Display()
if action.NotePaneInst != nil && action.NotePaneInst.IsOpen() {
    action.NotePaneInst.Display()
}
screen.Screen.Show()
```

**事件分发部分**（在现有 `InfoBar.HasPrompt` 判断之前加，**优先级最高**）：
```go
if action.NotePaneInst != nil && action.NotePaneInst.IsOpen() {
    // 完全模态：NotePane 独占一切事件，不穿透到底层
    action.NotePaneInst.HandleEvent(event)
} else if action.InfoBar.HasPrompt {
    action.InfoBar.HandleEvent(event)
} else {
    action.Tabs.HandleEvent(event)
}
```

NotePane 打开时，resize 事件也由 NotePane 处理：它会重新计算浮窗位置和尺寸，同时让底层 Tabs 也执行 resize（否则底层 panes 的尺寸会在关闭 NotePane 后错乱）。即：NotePane 先让 Tabs 做 resize，然后自己重新定位覆盖上去。

### Phase 2：渲染细节

#### 2.1 浮窗边框绘制

在 `NotePane.Display()` 中：
1. 先画 box-drawing 边框（`screen.SetContent`）
   - 上边框：`┌` + `─`×N + `┐`
   - 左右边框：每行 `│`
   - 下边框：`└` + `─`×N + `┘`
2. 让内部 BufWindow 的 View 定位在边框内部（X+1, Y+1, Width-2, Height）

#### 2.2 BufWindow 隐藏 statusLine

在 `internal/display/bufwindow.go` 的 `displayStatusLine()` 中：
```go
func (w *BufWindow) displayStatusLine() {
    if w.hideStatusLine {
        return
    }
    // ... 原有逻辑
}
```

在 BufWindow 增加 `hideStatusLine bool` 字段，NotePane 创建 Window 时设为 `true`。

#### 2.3 光标位置计算

NotePane 打开时：
1. 获取当前活跃 BufPane 的 cursor 屏幕坐标
2. 浮窗 Y = cursorScreenY + 1（如果空间不够，向上偏移）
3. 浮窗 X/Width = 当前 pane 的 View.X / View.Width

### Phase 3：Buffer 管理

#### 3.1 notes.md 的加载与保存

- 路径：`~/.config/microNeo/notes.md`
- 首次打开时若文件不存在，创建空文件
- 打开时加载，关闭时自动保存（如果 modified）
- Buffer 类型使用 `BTDefault`，保持正常的文件读写行为

### Phase 4：快捷键绑定

在 `defaults_darwin.go` / `defaults_other.go` 中添加默认绑定：

```go
"AltI": "ToggleNotePane",
```

在 `BufKeyActions` 中注册 `ToggleNotePane` action。

### Phase 5：完善与测试

- 处理终端 resize（NotePane 重新计算位置）
- 边框颜色可跟随 colorscheme
- 处理 notes.md 已在其他 pane 中打开的边缘情况
- 底层 pane 内容滚动时光标移出可视区域时的定位

---

## 需要修改的已有文件清单

| 文件 | 改动内容 | 侵入程度 |
|------|----------|----------|
| `cmd/micro/micro.go` | DoEvent() 中加 2 处 if 判断 | 极低（~10 行） |
| `internal/display/bufwindow.go` | 加 `hideStatusLine` 字段 + displayStatusLine 开头加 3 行 | 极低 |
| `internal/action/globals.go` | InitGlobals 中初始化 NotePaneInst | 极低（~5 行） |
| `internal/action/defaults_darwin.go` | 加 1 个默认键绑定 | 极低 |
| `internal/action/defaults_other.go` | 加 1 个默认键绑定 | 极低 |
| `internal/action/actions.go` | 注册 ToggleNotePane action | 极低（~5 行） |

## 需要新建的文件

| 文件 | 用途 |
|------|------|
| `internal/action/notepane.go` | NotePane 全部实现（~200 行） |

---

## 风险与注意事项

1. **光标位置映射**：NotePane 内的 BufPane 使用独立的 BufWindow，光标是 screen 级别的，需要确保 tcell 的 `ShowCursor` 指向正确位置
2. **BufPane 完整性**：由于嵌入 `*BufPane`，几乎所有 BufPane 的 action 都能工作（包括多光标、搜索替换等）。但某些 action 如 VSplit/HSplit/Quit 需要在 NotePane 的 HandleEvent 中拦截，避免误操作
3. **resize 处理**：终端大小变化时 NotePane 需要重新定位
4. **底层 pane 刷新**：NotePane 关闭后底层 pane 可能需要重绘（`screen.Redraw()` 即可）
5. **notes.md 并发**：如果用户同时在另一个 pane 打开了 notes.md，需要避免冲突。可以检查 buffer 是否已打开并复用

---

## 总结

这个方案的核心优势：
- **极低侵入**：对 micro 原生代码的改动总计约 25 行，都是添加性的（if 判断、字段声明），不修改任何现有逻辑
- **复用度高**：嵌入 BufPane 获得 100% 的编辑能力
- **独立性好**：NotePane 完全在 Tab/split tree 之外，作为一个覆盖层存在
- **新模式**：这实际上为 microNeo 引入了一个新的 UI 概念——"overlay pane"，未来可以复用（如浮动文件树、浮动命令面板等）
