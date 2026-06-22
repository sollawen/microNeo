# notePane 说明

> 本文档描述 microNeo notePane 浮窗的**当前实现状态**——以最终代码为准。
>
> **覆盖范围**：
> - notePane 浮窗的位置、尺寸、渲染
> - notePane 的 buffer 生命周期（无文件化 + 重建）
> - notePane 内部 binding 树（白名单 + 私有 action map）
> - notePane 打开/关闭/发送的键位
> - 与主编辑器的事件路由
>
> **不覆盖**：
> - AIBP 发送管线本身（见 `说明-发送端.md`）
> - pi 接收端（见 `说明-接收端.md`）
> - AIBP 协议（见 `说明-AIBP.md`）
>
> **与历史文档的关系**：D6（无文件化）、D7（buffer 生命周期 v2 修复）、D8（Alt-Enter 打开）、D9（Esc 关闭）四份计划合并描述——它们都围绕 notePane 这一组件的不同侧面。注：D7 v1 实施后暴露 panic 漏洞，v2 修复后才有当前稳定形态。

---

## 一、定位

notePane 是一个**浮动便签面板**，让用户在编辑文件时能"开个小窗记下想法/问题"，然后通过 AIBP 发送给 ai agent。

**核心特性**：

- **嵌入式 BufPane**：嵌入 `*BufPane` 获得完整 microNeo 编辑能力（输入/退格/方向键/选区/多光标/剪贴板等）
- **白名单 bindings**：只允许 ~80 个安全 action，从根本上杜绝危险 action（Quit/OpenFile/Shell 等）
- **独立 binding 树**：`NotePaneBindings` KeyTree，与主编辑器 `BufBindings` 物理隔离
- **无文件 buffer**：`BTScratch`，关闭后内容**立即从内存清空**，不写盘
- **打开 = 空白**：每次 `open()` 总是新建 buffer（`open` 总是 Close 旧 + NewBufferFromString），保证「打开就是空的」
- **光标下方定位**：浮窗出现在主编辑器光标所在行的下一行；下方空间不够时向上滚动主编辑器
- **主编辑器冻结**：notePane 开着时键盘/鼠标事件全部给它，主编辑器收不到
- **主操作键 = Alt-Enter**：在主编辑器是「打开」，在 notePane 内是「发送 + 关闭」；**Esc** 永远关闭不发送

---

## 二、代码位置

```
internal/action/notepane.go    # 全部 notePane 逻辑（~580 行）
```

**单文件实现**：所有逻辑（结构体、buffer 生命周期、binding 树、open/close/send、render、event route）都在 `notepane.go` 一个文件里。

---

## 三、NotePane 结构

```go
type NotePane struct {
    *BufPane                  // 嵌入：获得完整编辑能力
    isOpen        bool
    x, y          int         // 浮窗边框的屏幕坐标
    width         int
    height        int         // 内容高度（固定 5，D7+ 范围外：动态行高）
    filePath      string      // 主编辑器 buffer.AbsPath（open 时抓）
    fileCursor    buffer.Loc  // 主编辑器最低光标（X=col, Y=line, 0-based）
    fileSelection    *[2]buffer.Loc  // 主编辑器选区（归一化，0-based）
    fileSelectionText string         // 选区文字（> MaxSelectionLines 时为空）
}
```

**全局单例**：

```go
// TheNotePane is the global NotePane instance
var TheNotePane *NotePane
```

`cmd/micro/micro.go:540` 处 `TheNotePane = NewNotePane()`，**整个进程生命周期只调一次**。

---

## 四、Buffer 生命周期

### 4.1 创建时机

**`NewNotePane()` 只在启动时调一次**（micro 主循环初始化），创建初始 buffer A（`BTScratch`）。

```go
func NewNotePane() *NotePane {
    n := &NotePane{ height: 5 }
    buf := buffer.NewBufferFromString("", "", buffer.BTScratch)
    buf.SetOptionNative("ruler", false)
    win := display.NewBufWindow(0, 0, 80, n.height, buf)
    win.SetHideStatusLine(true)
    n.BufPane = newBufPane(buf, win, nil)  // 小写：不触发 finishInitialize
    n.BufPane.bindings = NotePaneBindings
    return n
}
```

**关键点**：

- **`BTScratch`**（`buffer.go:56-57`）——`{Scratch: true}`，`Save()` 直接返回 error（`save.go:237`）
- **`newBufPane`（小写）**——只 new 对象 + 设字段，**不**触发 `finishInitialize`（详细论证见 D6 §2.4）
- **`win.SetHideStatusLine(true)`**——不画任何 statusLine/divider

**为什么不用 `NewBufPane` 或 `NewBufPaneFromBuf`**：

| 入口 | 风险 |
|------|------|
| `NewBufPane` | 立即调 `finishInitialize()`，但此时 BufWindow 尺寸是占位值（0,0,80,5） |
| `NewBufPaneFromBuf` | 调 `initMDConfig(buf, w)`——buffer Path 为空时行为未验证，且启用 MD 渲染与"纯文本便签"冲突 |
| **`newBufPane`** | **仅设字段，不触发任何后续逻辑**——`finishInitialize` 延后到 `open()` 调 `Resize()` 时（此时尺寸是真坐标） |

> 第一次 `open()` 时 BufWindow 的 `Resize()` 会触发 `finishInitialize()`，其中 `initialRelocate()` 和 `RunPluginFn("onBufPaneOpen")` 都不依赖 tab（`bufpane.go:384-394`），安全。

### 4.2 打开 = 空白（D7 v2 修复后）

**核心原则**：**每次 `open()` 创建一个全新的空 buffer**——不依赖 `close()` 销毁。

```go
func (n *NotePane) open() {
    if n.isOpen { return }                  // 守卫：已开态下重复调用是 no-op
    pane := MainTab().CurPane()
    if pane == nil { return }

    // 兑现"打开 = 全新"承诺：关掉旧 buffer（如有），建新的
    if n.BufPane.Buf != nil {
        n.BufPane.Buf.Close()               // 从 OpenBuffers 移除 + 调 Fini 清理
    }
    buf := buffer.NewBufferFromString("", "", buffer.BTScratch)
    buf.SetOptionNative("ruler", false)
    nbw := n.BufPane.BWindow.(*display.BufWindow)
    nbw.SetBuffer(buf)                      // 切 BufWindow.Buf + 装 OptionCallback
    n.BufPane.Buf = buf                     // 同步 BufPane.Buf 引用

    n.reposition()                          // 算位置 + 抓主编辑器上下文
    n.isOpen = true
}
```

**关键步骤**：

1. **总是 Close 旧 buffer**（如有）——从 `OpenBuffers` 移除
2. **新建 `BTScratch` buffer**——空字符串
3. **`SetBuffer()`**——切 BufWindow.Buf + 装 OptionCallback + GetVisualX
4. **同步 `n.BufPane.Buf`**——BufWindow.Buf 和 BufPane.Buf 是独立字段
5. **`reposition()`**——计算位置、抓主编辑器上下文

**为什么用 `Close()` 而不是 `Fini()`**：

- `Close()`：从 `OpenBuffers` 移除 + 调 Fini 清理（**彻底关闭**）
- `Fini()`：只清理 Serialise/Backup（**不**从列表移除）

notePane 关闭后不需要这个 buffer 在 OpenBuffers 里——**彻底移除**。

### 4.3 关闭不动 Buf（D7 v2 关键修正）

**v1 设计**：close 时 `Buf.Close() + Buf = nil`——**有 bug**。

**v1 实施后 panic**：

```
bufpane.go:506 h.Buf.MergeCursors()  ← panic: invalid memory address or nil pointer dereference
```

**根因**：`n.close()` 是从 `NotePaneSend` action handler **内部**被调的。`NotePaneSend` 走 `BufPane.HandleEvent` 调用栈，handler 返回后 `BufPane.HandleEvent` 还会**继续走**到 `bufpane.go:506` 调 `h.Buf.MergeCursors()` 统一清理。v1 的 `Buf = nil` 导致 nil 访问 panic。

**v2 修复**：

```go
func (n *NotePane) close() {
    n.isOpen = false

    // Restore main editor's normal scroll position
    if pane := MainTab().CurPane(); pane != nil {
        pane.BWindow.Relocate()
    }
    // ← 不动 Buf！由 open() 路径在下次打开时 Close + 新建
}
```

**v2 行为**：

- close → `isOpen = false` + 主编辑器 Relocate
- **不**动 Buf——`n.BufPane.Buf` 引用保持，BufPane.HandleEvent 后续访问安全
- 下次 `open()` → Close 旧 buffer + 新建（见 §4.2）

**v1 → v2 关键对比**：

| 维度 | v1 | v2 |
|------|----|----|
| close 是否动 Buf | `Close() + set nil` | **不动** |
| open 重建条件 | `if Buf == nil` | **总是 Close 旧 + 新建** |
| close → open 之间 Buf 状态 | nil（**panic 风险**） | 旧 buffer 引用保持（**安全**） |
| 能否保证"打开 = 空白" | ✅ | ✅（**总是新建**） |
| 性能 | 首次 open 跳过重建 | 每次 open 都 Close + 新建（<1ms） |

> 详细 panic 复盘见 D7 §九「v1 实施回顾」。

### 4.4 buffer 关闭兜底防线

即便用户绑定了 Save 类 action，`Buf.Save()` 在 BTScratch 上也会被 micro 内置拦截（`save.go:237`）：

```go
if b.Type.Scratch {
    return errors.New("Cannot save scratch buffer")
}
```

**两道防线**：

1. `allowedNotePaneActions` 白名单**没有** Save 类 action
2. 即使进了白名单，BTScratch 也会拦截

`close()` 显式不调 `Buf.Save()`（D6 §3.1 改动 4），因为：

- BTScratch 上 `Save()` 会被拦截（不写盘）
- 保留调用会误导未来读代码的人
- 不写盘的语义更显式

---

## 五、位置计算（reposition）

```go
func (n *NotePane) reposition() {
    pane := MainTab().CurPane()
    if pane == nil { return }
    if pane.Buf == nil { return }  // 防御：主 buffer 被关

    n.filePath = pane.Buf.AbsPath
    view := pane.BWindow.GetView()
    bw := pane.BWindow.(*display.BufWindow)

    // 1. 找最低的屏幕行
    lowestRow := n.lowestCursorScreenRow(bw, view)

    // 2. 算 notePane 位置
    n.x = view.X
    n.width = view.Width
    notePaneTopBorder := lowestRow + 1
    notePaneBottomBorder := notePaneTopBorder + n.height + 1

    // 3. 下方空间不够 → 向上滚动主编辑器
    viewBottom := view.Y + view.Height
    if notePaneBottomBorder > viewBottom {
        deficit := notePaneBottomBorder - viewBottom + 2
        scrollmargin := int(pane.Buf.Settings["scrollmargin"].(float64))
        maxDeficit := lowestRow - scrollmargin
        if deficit > maxDeficit { deficit = maxDeficit }
        if deficit > 0 {
            oldStartLine := view.StartLine
            view.StartLine = bw.Scroll(view.StartLine, deficit)
            lowestRow -= bw.Diff(oldStartLine, view.StartLine)
            notePaneTopBorder = lowestRow + 1
        }
    }

    // 4. 定位 BufWindow 到边框内部
    n.y = notePaneTopBorder
    nbw := n.BufPane.BWindow.(*display.BufWindow)
    nbw.X = n.x + 1
    nbw.Y = n.y + 1
    n.BufPane.Resize(n.width-2, n.height)
}
```

**关键步骤**：

1. **抓 `filePath` + 最低光标**——`lowestCursorScreenRow` 顺便记录光标/选区到 `n.fileCursor` / `n.fileSelection`（详见 `说明-发送端.md §四`）
2. **notePane 位置 = 光标下一行**——`notePaneTopBorder = lowestRow + 1`
3. **下方空间不够时滚动主编辑器**——`ScrollUp`，最多滚到 `lowestRow - scrollmargin`（不滚出 scrollmargin）
4. **定位 BufWindow**——`X = n.x + 1`, `Y = n.y + 1`（边框内部），`Resize` 调整尺寸

**调用时机**：

- `open()` 时——首次定位 + 抓上下文
- `HandleEvent` 收到 `EventResize` 时——**已开态下**重新定位（不重抓上下文，因为上下文是"打开时"的）

---

## 六、Binding 树（白名单 + 私有 map）

### 6.1 NotePaneBindings KeyTree

```go
var NotePaneBindings *KeyTree
```

**独立 KeyTree**——与主编辑器的 `BufBindings` 物理隔离。

`bufpane.go:Bindings()` 方法决定一个 pane 用哪棵：

```go
func (h *BufPane) Bindings() *KeyTree {
    if h.bindings != nil {
        return h.bindings     // notePane: bindings = NotePaneBindings
    }
    return BufBindings        // 主编辑器: bindings 一直是 nil
}
```

**为什么需要独立树**：

- BufPane 有 142 个 action，其中 11 个在 `tab==nil` 时会 panic，约 32 个对 notePane 有副作用
- notePane 定位是「临时输入缓冲区」，不是完整编辑器
- 白名单策略：以后新增 action 不会自动泄露到 notePane，必须显式添加
- 模仿 InfoPane 的 `InfoBufBindings` 模式

### 6.2 allowedNotePaneActions 白名单

```go
var allowedNotePaneActions = map[string]bool{
    // 光标移动 (12)
    "CursorUp": true, "CursorDown": true, ...
    // 选择 (20)
    "SelectUp": true, "SelectDown": true, ...
    // 段落导航 (4)
    "ParagraphPrevious": true, ...
    // 文本编辑 (12)
    "InsertNewline": true, "Backspace": true, ...
    // 翻页滚动 (10)
    "PageUp": true, "PageDown": true, ...
    // 剪贴板 (10)
    "Copy": true, "Cut": true, ...
    // 多光标 (8)
    "SpawnMultiCursor": true, ...
    // 其他 + AIBP + 鼠标 + 词导航
    "NotePaneSend": true, "NotePaneClose": true, ...
}
```

**~80 个允许的 action**。不包含的危险 action（部分）：

- 🔴 `Quit`, `ForceQuit`, `VSplitAction`, `HSplitAction`, `Unsplit`
- 🟡 `Save`, `SaveAs`, `OpenFile`, `Find`, `ShellMode`, `CommandMode`, `Undo`, `Redo`

**`command:` / `lua:` 前缀 action**——`isActionAllowed()` 直接拒绝（避免 Lua 注入）。

### 6.3 私有 notePaneActions map（D9 引入）

```go
var notePaneActions = map[string]BufKeyAction{
    "NotePaneSend":  NotePaneSend,
    "NotePaneClose": notePaneClose,
}
```

**为什么需要私有 map**：

D9 之前的实现是 `NotePaneSend` 注册到 `bufpane.go:BufKeyActions`——这违反 AGENTS.md「原生零侵入」原则。D9 引入私有 map，把 notePane 专属 action 收进 `notepane.go`：

- `notePaneRegisterBinding()` 优先查私有 map
- 找不到再 fallback 到全局 `BufKeyActions` / `BufMouseActions`
- **`bufpane.go` 反而净减一行**（迁出 `NotePaneSend`）

```go
// notePaneRegisterBinding 查询顺序
if f, ok := notePaneActions[a]; ok {       // 私有 map 优先
    afn = f
} else if f, ok := BufKeyActions[a]; ok {  // fallback 到全局
    afn = f
} else if f, ok := BufMouseActions[a]; ok {
    afn = f
} else {
    continue  // 查不到，binding 静默丢弃
}
```

### 6.4 init() 注册

```go
func init() {
    NotePaneBindings = NewKeyTree()
    notePaneMapDefaults(DefaultBindings("buffer"))    // 灌入 buffer 默认 bindings（被白名单过滤）
    notePaneMapBinding("Alt-Enter", "NotePaneSend")   // 发送（硬编码）
    notePaneMapBinding("Esc", "NotePaneClose")        // 关闭（硬编码，D9 引入）
}
```

**注册顺序的微妙点**（后人易踩坑）：

1. `notePaneMapDefaults` 把 buffer defaults 灌入 `NotePaneBindings`
   - 包含 `Alt-Enter → InsertNewline`（被白名单放行）
   - 也包含 `Esc → Escape,Deselect,ClearInfo,RemoveAllMultiCursors`
2. `notePaneMapBinding("Alt-Enter", "NotePaneSend")`——**覆盖**前一步的 Alt-Enter → InsertNewline
3. `notePaneMapBinding("Esc", "NotePaneClose")`——**覆盖**前一步的 Esc

**依赖 KeyTree 覆盖语义**（`keytree.go:142-170`）：`newNode.actions = []TreeAction{a}`（直接替换，不是 append）。如果上游 micro 改回 append 语义，D9 会同时跑两套动作——需要 `DeleteBinding` 或调整顺序。

### 6.5 bindings.json 局限

`Esc → NotePaneClose` 和 `Alt-Enter → NotePaneSend` 都是 notePane 内部 binding，**不走**用户级 `bindings.json`（`internal/action/bindings.go:40-79` 只针对 `BufKeyActions`）。

**D8 引入的 `NotePaneOpen`** 在主编辑器侧，可走 `bindings.json`——但 notePane 内部 binding 暂不接入。

---

## 七、键位总览

| 键 | 主编辑器上下文 | notePane 上下文 | 来源 |
|----|----------------|-----------------|------|
| **Alt-Enter** | 打开 notePane | **发送 + 关闭** | D8 引入 NotePaneOpen / 原硬编码 |
| **Esc** | 原生 buffer 行为（清 info / 取消选区） | **关闭不发送** | D9 引入 |
| Alt-i | 无操作（已退役） | 无操作 | D8 退役 |
| Enter | InsertNewline | InsertNewline（notePane 内多行编辑） | buffer defaults |

**心智模型**：**Alt-Enter 是 notePane 的主操作键，语义随上下文切换**——没开就开，开着就把草稿发出去。Esc 永远是"算了，不要"。

**为什么不进 `bindings.json`**：

- terminal 对修饰键支持参差（Shift-Enter 多数终端检测不到）
- 可换的键本就不多
- 真有用户碰到 Alt-Enter 不行的终端：可改主侧（`"Ctrl-p": "NotePaneOpen"`），发送侧仍用 Alt-Enter
- 不堵死

---

## 八、事件路由（cmd/micro/micro.go）

```go
} else if action.TheNotePane != nil && action.TheNotePane.IsOpen() {
    action.TheNotePane.HandleEvent(event)   // ← notePane 开着时，事件全给它
} else {
    action.Tabs.HandleEvent(event)          // 主编辑器
}
```

**notePane 开着时**：

- 事件**直接**进 notePane
- 主 buffer 的 `BufKeyActions`（包括 `NotePaneOpen`）**完全不会被查**
- 不会嵌套打开（`NotePaneOpen` 的 `!IsOpen()` 守卫是双保险）

**关键点**：

- 切换上下文不需要手动 focus/unfocus
- `Bindings()` 方法自动选对应树（`bufpane.go:536-541`）
- 关闭 notePane 后事件**自然**回到主编辑器

---

## 九、Display 渲染

```go
func (n *NotePane) Display() {
    if !n.isOpen { return }

    // 1. 清浮窗区域（边框 + 内容）
    for row := 0; row < n.height+2; row++ {
        for col := 0; col < n.width; col++ {
            screen.Screen.SetContent(n.x+col, n.y+row, ' ', nil, config.DefStyle)
        }
    }

    // 2. 画 box-drawing 边框（┌─┐│└─┘）
    // ... topLeft, topRight, bottomLeft, bottomRight, horizontal, vertical ...

    // 3. 画 BufWindow 内容（自动含光标）
    n.BufPane.BWindow.Display()
}
```

**三层叠加**：

1. 清屏（覆盖在主编辑器之上的区域）
2. 边框（box-drawing 字符）
3. BufWindow 内容（文字 + 光标）

**BufWindow 不画 statusLine**（`win.SetHideStatusLine(true)` 在 `NewNotePane` 里设）。

---

## 十、关键变更（D6 / D7 / D8 / D9 / D10）

| 计划 | 核心变更 | 状态 |
|------|---------|------|
| **D6** | `notes.md` 文件持久化 → `BTScratch` 无文件 | ✅ 实施 |
| **D7 v1** | close 时 `Buf = nil` | ❌ panic，撤回 |
| **D7 v2** | close 路径不动 Buf + open 总是 Close 旧 + 新建 | ✅ 实施（commit `67ec2858`） |
| **D8** | 主编辑器 `Alt-i` 硬编码 → `Alt-Enter` + 标准 binding | ✅ 实施 |
| **D9** | notePane 内 `Esc` 直接关闭（不发送）+ 私有 `notePaneActions` map | ✅ 实施 |
| **D10** | 空内容拦截 + 30 行阈值 + pi 端 `formatText` 改写 | ✅ 实施（发送端在 notepane.go、接收端在 aibp-pi/index.ts） |

**D7 v1 → v2 的 panic 复盘**：

| 阶段 | 行为 |
|------|------|
| v1 设计 | `close()`: `Buf.Close() + Buf = nil`；`open()`: `if Buf == nil` 重建 |
| v1 实施 | microNeo 编译通过，notePane 发送时 panic |
| 根因 | `NotePaneSend` 走 `BufPane.HandleEvent` 调用栈，close 在 handler 内被调，handler 返回后 `bufpane.go:506` 调 `h.Buf.MergeCursors()` 访问 nil |
| v2 修复 | `close()` 删 set-nil 块；`open()` 改成「总是 Close 旧 + 新建」 |
| 教训 | 不要在 action handler 内部对 BufPane 状态做"假设不会再被访问"的修改 |

详细见 D7 §九「v1 实施回顾」。

---

## 十一、约束

| 约束 | 来源 |
|------|------|
| micro 原生代码零侵入（除 `win.SetHideStatusLine` 一处 4 行） | AGENTS.md |
| 嵌入 `*BufPane` + 独立 `NotePaneBindings` | 历史 notePane 实施计划（已清理） |
| `BTScratch` buffer 类型 | D6（save.go:237 兜底） |
| `newBufPane`（小写）创建 BufPane | D6 §2.4（避免 finishInitialize 时机问题） |
| close 不动 Buf | D7 v2（避免 nil panic） |
| 私有 `notePaneActions` map | D9（让 `bufpane.go` 净减一行） |
| 键位：Alt-Enter 打开 / Alt-Enter 发送 / Esc 关闭 | D8 + D9 |

---

## 十二、与历史文档的关系

| 历史文档 | 与本文关系 |
|----------|----------|
| 早期 Dx 计划（D6-D10、notePane实施计划） | 已被本文档吸收（设计已全部落地）。详细历史可查 git log 与 commit message。**这些文档已被清理**——D6（无文件化）→ §四、D7 v2（buffer 生命周期）→ §四 + §十、D8（Alt-Enter 打开）→ §六 + §七、D9（Esc 关闭）→ §六 + §七、D10 决策 1（空拦截）→ §四 + §六 |
| `用户界面-V2.md` | UI 需求源头（位置、尺寸、心智模型）。本文是落地后的「实现」 |
| `说明-发送端.md` | notePane 内的发送动作 `NotePaneSend` 详细描述 |
| `说明-AIBP.md` | AIBP 协议权威；本文档不重复 |
