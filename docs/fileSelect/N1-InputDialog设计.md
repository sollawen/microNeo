# InputDialog 设计方案

| 项 | 内容 |
|---|---|
| 组件类别 | Dialog（modal，独占交互，数据进数据出） |
| 容器 | floatFrame（主循环级，全局单实例） |
| 参考实现 | SelectDialog（`internal/action/selectdialog.go`） |
| 编辑基座复用 | `buffer.Buffer`（BTInfo）+ `buffer.Cursor` 原语，即 InfoBar 单行编辑的同一套底层 |
| 关联文档 | `docs/fileSelect/N0-组件层架构讨论.md`、`docs/弹窗机制/弹窗框架设计.md` |

## 一、背景与目标

InputDialog 是一个单行文本输入/编辑浮窗。需求要点（来自 N0 与任务书）：

- 输入参数：初始 string（可空）、锚点坐标（屏坐标）、宽度。
- 渲染：在 floatFrame 容器内画一个带边框的单行输入框。
- 编辑：复用 InfoBar 的单行编辑能力（光标移动、字符输入/删除等）。
- 退出三态：
  - Enter → 关闭，返回修改后的 string（确认）。
  - ESC → 关闭，返回原 string（取消）。
  - Resize → 等同 ESC（取消）。
- 首要应用场景：finder 重命名文件/目录；其它需要获取用户单行输入的场景复用同一组件。

它是 SelectDialog 的姐妹组件：同一个 floatFrame 容器、同一套生命周期约定、同一种「open 接管交互 → close 交回」的 modal 语义。区别只在内容形态——SelectDialog 画列表做选择，InputDialog 画单行做编辑。

## 二、设计约束

直接继承自 floatFrame 契约（`internal/action/floatframe.go`）与 SelectDialog 先例：

1. **C1 单浮窗**：全局同时最多一个浮窗。`TheFloatFrame.Open` 在已有浮窗开启时返回 false。
2. **C3 模态**：floatFrame 开启期间，主循环（`cmd/micro/micro.go:575-594`）把所有非 resize 事件都路由给 `TheFloatFrame.HandleEvent`，业务 pane / InfoBar / NotePane 都收不到。
3. **resize 由容器拦截**：floatFrame 在 `HandleEvent` 里拦截 `EventResize`，先 `Close()` 再触发 `OnResize` 回调，**不会**把 resize 转发给具体浮窗的 `handleEvent`。因此 InputDialog 的 resize 语义只能通过 `FloatOpenSpec.OnResize` 实现。
4. **Display 后写胜出**：主循环渲染顺序里 floatFrame 最后画（`micro.go:535-536`），覆盖一切；且 `FloatFrame.Display()` 开头会 `HideCursor()`。
5. **contentArea 语义**：`FloatFrame.Display` 委托画内容时传入的 `Rect` 已扣除边框，恒为 `{fx+1, fy+1, contentSize.W, contentSize.H}`。具体浮窗无需感知边框。
6. **失败前置检查**：`Open` 在 `outerH > bottomLimit+1 || outerW > w` 时返回 false；具体浮窗必须对此透明（不开即回调取消）。
7. **复用优先 / 勿增实体**（项目基本规则）：能复用已有机制的不新写。

## 三、总体结构

InputDialog 是 `action` 包内的一个纯结构体（不带 BufPane），形态与 SelectDialog 完全对称：

```
调用方                         floatFrame                    InputDialog
  │  Open(...)                    │                              │
  │──────────────────────────────▶│                              │
  │                               │  存 Display/HandleEvent/     │
  │                               │       OnResize 函数值         │
  │                               │  isOpen=true, Redraw         │
  │                               │                              │
  │              主循环每帧        │  Display() → f.display(area) │──画单行+光标
  │              每个事件          │  HandleEvent(ev)             │
  │                               │     ├ resize → Close+OnResize│──onResult("",true)
  │                               │     └ else → f.handleEvent   │──编辑 / Enter / ESC
  │  onResult(result, canceled)   │                              │
  │◀──────────────────────────────│◀─────────────────────────────│
```

- **检测/分类**：无。InputDialog 不依赖 buffer 变更检测或 MD 分段，状态自洽。
- **持有者**：调用方 new 一个 `InputDialog`，调 `Open`，之后对象本身不再被主循环引用（用完即弃，靠 GC）。这与 SelectDialog 生命周期一致（`selectdialog.go:20-24` 注释）。

## 四、数据结构

```go
// InputDialog 是单行文本输入浮窗（modal，走 floatFrame）。
// 边框 / title / 清屏 / 锚点 / Layout / 生命周期全部移交 FloatFrame；
// 本结构只保留单行编辑状态（buffer + cursor）、关闭回调。
// 键名解析由调用方注入 keyResolver，键位映射直接读 config.Bindings["command"]（详见 §6.2）。

type KeyResolver func(event tcell.Event) string

type InputDialog struct {
    buf      *buffer.Buffer    // BTInfo 单行 buffer，编辑基座（与 InfoBar 同源）
    cursor   *buffer.Cursor    // buf 的活跃光标，编辑原语入口
    initial  string            // 取消时回退的原值
    width    int               // 内容区宽度（字符列），不含边框
    hscroll  int               // 水平滚动偏移（光标出框时拉动），字符列
    keyResolver KeyResolver    // 注入的键位解析函数（ownerPane 闭包包住 keyEvent）
    title    string
    onResult func(result string, canceled bool) // 关闭回调（一次性）
}
```

字段说明：

- `buf` / `cursor`：编辑基座。复用方式见「六、编辑引擎」。`cursor = buf.GetActiveCursor()`，`Open` 时取一次缓存，避免每帧查找。
- `initial`：`Open` 时灌入 buffer 的初始内容（用于内部状态，不返回给调用方）。
- `width`：调用方指定，对应 `FloatOpenSpec.ContentSize.W`。高度恒为 1（单行）。
- `hscroll`：长文本超出宽度时，跟随光标水平滚动，保证光标始终可见。详见「七、渲染设计」。
- `onResult`：关闭回调，一次性触发，触发后在 close 流程里置 nil（断引用，便于 GC，对齐 floatFrame 的清理约定）。

不持有的东西（明确边界）：

- 不持有 `*BufPane`：单行编辑不需要 finder / 插件钩子 / 多光标 / tab，避免拖入无关机制（见「六」备选方案分析）。
- 不持有屏幕尺寸 / 布局结果：全部归 floatFrame。
- 不持有 KeyTree：键名解析走注入的 `keyResolver`，键位映射直接读 `config.Bindings["command"]`，再用 switch 分发到 buffer 原语（N1a §7.2）。

## 五、API 设计

### 5.1 构造

```go
// NewInputDialog 返回空 InputDialog（未打开状态）。
func NewInputDialog() *InputDialog
```

与 `NewSelectDialog` 对称，零参数，返回空壳。

### 5.2 打开

```go
func (d *InputDialog) Open(
    initial    string,        // 初始内容（可空）
    title      string,        // 上边框标签（如 "Rename"）；空串=纯横线
    anchor     Pos,           // 锚点屏坐标；AutoExpand=false 时为外矩形左上角
    width      int,           // 内容区宽度（字符列，不含边框）；<=0 时给一个安全下限
    frameColor tcell.Style,   // 边框色；零值 = config.DefStyle
    onResult   func(result string, canceled bool),
    keyResolver KeyResolver,  // 注入的键位解析函数（ownerPane 闭包包住 keyEvent）
)
```

`keyResolver` 由调用方注入：finder 由 ownerPane 创建时拿到（N1a §7.2），其他调用方（如 BufPane）在 action 包内直接构造闭包。键位映射不需要传——dialog 自己读 `config.Bindings["command"]`（已是 infodefaults + 用户配置的合并结果）。

行为约定：

- `onResult` 恒被调用**恰好一次**（成功开启并关闭、或开启失败两种情况下都要调）。
  - Enter → `onResult(edited, false)`。
  - ESC → `onResult("", true)`（`result` 无定义，调用方应忽略）。
  - Resize → `onResult("", true)`（`result` 无定义，调用方应忽略）。
- 回调顺序（对齐 SelectDialog 与 floatFrame 设计 §五）：**先 `TheFloatFrame.Close()`，再触发 `onResult`**。这样回调里读到的容器状态已是关闭后，调用方可在回调里安全地再次 `Open` 另一个浮窗。
- 开启失败（`TheFloatFrame.Open` 返回 false，例如已有浮窗在开 / 屏幕放不下）：**不设置任何业务状态**，直接 `onResult("", true)` 透明返回。这与 SelectDialog 的失败路径一致（`selectdialog.go:91-99`）。

### 5.3 回调签名选型

SelectDialog 用 `onSelect func(*string)`（nil = 取消）。InputDialog 选择 `func(result string, canceled bool)`，理由：

- `canceled bool` 语义清晰：一看即知是「结果 + 状态」，符合 Go 的 `val, ok` 惯例。
- Enter 时返回编辑后的 string，取消时 `canceled=true`，`result` 无定义（调用方应忽略）。
- 代价是「与 SelectDialog 的 idiom 不同」，但二者语义本就不同（SelectDialog 是「选择」，InputDialog 是「编辑」），一致性在此让位于语义清晰。

**完整 API 一致性说明**见 §十三。

### 5.4 内部 display / handleEvent（由 Open 塞给 floatFrame）

```go
d.display(contentArea Rect)        // 画单行文本 + 光标
d.handleEvent(event tcell.Event)   // 编辑键映射 + Enter/ESC
```

两者均不导出，仅以函数值形式注入 `FloatOpenSpec`，与 SelectDialog 同。

## 六、编辑引擎：复用策略（核心设计决策）

### 6.1 任务书的复用要求

任务书要求「复用 InfoBar 的单行编辑功能（光标移动、字符输入删除等）」。需先厘清 InfoBar 的单行编辑到底由什么组成：

```
InfoBar 单行编辑 = buffer.Buffer(BTInfo)  +  buffer.Cursor 原语  +  键映射(BufPane actions)
                  └────────────────────/    \─────────────────────/
                        数据与操作基座          （这两层就是「单行编辑能力」本身）
```

InfoPane（InfoBar）之所以「重」，是因为它把这套基座包进了一个完整 `BufPane`（带 finder 会话、插件钩子、多光标、mouse、tab 依赖）。但**单行编辑能力本身**完全由 `buffer` 包提供：`Buffer.Insert` / `Buffer.Remove` / `Cursor.Left|Right|...` / `Cursor.Loc` / `Buffer.LineBytes`。InfoBar 的 `Backspace` / `Delete` / `CursorLeft` 等 action 最终也都是落到这几个原语上（见 `internal/action/actions.go:723-832`、`bufpane.go:681` `DoRuneInsert`）。

### 6.2 选定方案：复用基座 + 跟随用户键位（注入 keyResolver + 读 config）

InputDialog 直接持有 `buffer.Buffer`(BTInfo) + `buffer.Cursor`，编辑操作全部调用 buffer 包原语；键名解析由调用方注入的 `keyResolver` 完成，键位映射直接读 `config.Bindings["command"]`，自动跟随用户自定义。

**核心机制**：
```go
// 1. 查询用户配置（优先）或默认绑定（fallback）
actionName := d.lookupAction(event)

// 2. 根据动作名调用 buffer 原语
switch actionName {
case "CursorLeft":
    d.cursor.Left()
case "CursorRight":
    d.cursor.Right()
case "WordLeft":
    d.cursor.WordLeft()
case "Backspace":
    loc := d.cursor.Loc
    d.buf.Remove(loc.Move(-1, d.buf), loc)
    d.cursor.Left()
// ... 更多动作
}
```

**lookupAction 实现**：
```go
// dialog import config，自己读键位映射；keyResolver 由调用方注入
func (d *InputDialog) lookupAction(event tcell.Event) string {
    keyName := d.keyResolver(event)  // 如 "Left", "CtrlLeft", "Alt-a"
    // config.Bindings["command"] 已含 infodefaults + 用户覆盖（InitBindings 时合并）
    if action, ok := config.Bindings["command"][keyName]; ok {
        return action
    }
    return ""
}
```

**为什么能直接读 config.Bindings["command"]**：`action.InitBindings()`（启动时）先把 `infodefaults` 写入 `config.Bindings["command"]`，再叠用户 `bindings.json` 的覆盖。所以这个 map 本身就是完整映射，dialog 不需要单独拿 `infodefaults`。

**keyResolver 从哪来**：finder 由 ownerPane（action 包内）创建，ownerPane 构造闭包包住私有 `keyEvent`，注入给 finder，finder 打开 InputDialog 时透传（N1a §7.2）。

**为什么这样设计**：
- **跟随用户键位**：用户在 `bindings.json` 里改的键位自动生效，与 InfoBar/主编辑器一致
- **dialog 不引用 action**：config 是叶子包，keyResolver 是注入的函数值，无循环依赖
- **零新建包**：不抽 keyevent 共享包，ownerPane 注入即可（N1a §7.2）
- **安全过滤**：只支持 InputDialog 需要的动作子集（见下节白名单），不支持的动作忽略
- **与 SelectDialog 对齐**：仍是 switch 分发，形态一致

### 6.3 动作白名单（支持的动作）

InputDialog 只支持单行编辑所需的动作子集（从 InfoBar 的实际支持动作筛选，与 InfoBar 行为对齐）：

| 动作名 | 原语调用 | 说明 |
|---|---|---|
| `CursorLeft` | `d.cursor.Left()` | 左移一格 |
| `CursorRight` | `d.cursor.Right()` | 右移一格 |
| `StartOfLine` | `d.cursor.X = 0` | 至行首（X=0，绑定为 Ctrl-A） |
| `EndOfLine` | `d.cursor.X = charCount` | 至行尾（绑定为 Ctrl-E / End） |
| `StartOfTextToggle` | 跳到第一个非空白字符 | 绑定为 Home |
| `CursorStart` | 同 `StartOfLine`（InputDialog 单行） | 绑定为 Alt-Up / Ctrl-Up / Ctrl-Home |
| `CursorEnd` | 同 `EndOfLine`（InputDialog 单行） | 绑定为 Alt-Down / Ctrl-Down / Ctrl-End |
| `WordLeft` | `d.cursor.WordLeft()` | 按词左移（绑定为 Ctrl-Left / Ctrl-B） |
| `WordRight` | `d.cursor.WordRight()` | 按词右移（绑定为 Ctrl-Right / Ctrl-F） |
| `Backspace` | `Remove(...)` + `cursor.Left()` | 删除前一字符（绑定为 Backspace / Ctrl-H） |
| `Delete` | `Remove(...)` | 删除当前字符（绑定为 Delete） |
| `DeleteWordLeft` | 按词删除 | 删除前一个词（绑定为 Alt-Ctrl-H / Alt-Backspace / Ctrl-W） |
| 字符输入（`KeyRune`） | `Insert(...) + cursor.Right()` | 插入字符（含中文输入） |
| `KeyEnter` | 确认 | 关闭并返回 edited |
| `KeyEscape` / `AbortCommand` | 取消 | 关闭并返回 canceled=true |
| `CtrlC` | 取消 | 同 Escape |

**不支持的动作**（忽略，即使有绑定）：
- Tab / Autocomplete / Indent 等多行操作（InputDialog 单行，无缩进逻辑）
- Ctrl-m / `ExecuteCommand`：InfoBar 不支持此键用于确认，InputDialog 也不支持
- Undo / Redo（首版不暴露，buffer 原语支持但 keymap 不暴露）
- Cut / Copy / Paste（首版不暴露）
- Ctrl-U（删除至行首）和 Ctrl-K（删除至行尾）：InfoBar 不支持，InputDialog 也不支持
- 任何与 finder / 插件 / 多光标相关的动作
- HistoryUp / HistoryDown（InfoBar 特有，InputDialog 无历史）

### 6.4 已评估的备选方案：嵌入 BufPane

另一条路是把 buffer 包进一个最小 `display.BWindow`（仿 `InfoWindow`），再用 `NewBufPane` + `InfoBufBindings`，事件转发给 BufPane。即完全照搬 InfoBar 的栈。

结论：**评估后不采用**，原因：

- `newBufPane` 会创建一个 `finder.Session` 字段（`bufpane.go:298`），单行输入框用不到。
- `finishInitialize` 会调 `RunPluginFn("onBufPaneOpen")`（`bufpane.go:326`）。InfoBar 仅在启动时付一次；InputDialog 每次 Open 都触发插件「buffer 打开」钩子，可能产生非预期副作用（插件无法区分「主编辑器开 buffer」与「输入框开 buffer」）。
- BufPane 的 `HandleEvent` 路径包含 mouse / 多光标 / searchOrig 等大量单行输入用不上的分支。
- 与 SelectDialog 的「纯结构体」形态不一致，Dialog 同族组件会变得一个轻一个重。

权衡：嵌入 BufPane 的好处是「零行重写、与 InfoBar 永远字节一致」；代价是「把无关机制塞进 modal」。鉴于 InputDialog 只需单行编辑子集，且 6.2 方案已做到原语级完全复用，6.2 的纯结构体形态更符合项目「框架要薄」「勿增实体」原则。**若未来 InputDialog 需要信息栏级的复杂能力（历史、自动补全、多光标），再升级为嵌入 BufPane**——届时本节作为升级路径记录。

## 七、渲染设计

`FloatFrame.Display` 已完成：清外矩形 → 画 4 角 + 上下左右边 → title 嵌入上边框 → `HideCursor()` → 委托 `d.display(contentArea)`。

`d.display(contentArea)` 职责：在单行的 `contentArea`（`H == 1`）里画 buffer 第 0 行的可见片段，并 `ShowCursor` 到光标所在屏格。这正是 `floatframe.go` 注释预留的路径（「若有需要光标的浮窗（如未来 inputPane），可在自己的 f.display 里调 ShowCursor 覆盖此 HideCursor」）。

算法（参照 `internal/display/infowindow.go` `displayBuffer` 的简化版，去掉 InfoBar 的 `Msg` 前缀偏移）：

1. `line := d.buf.LineBytes(0)`；`cX := d.cursor.X`（光标字符列，**非可视列**）。
2. **水平滚动**（保证光标在框内）：
   - 光标可视列：`cursorVisualCol = d.cursor.GetVisualX(false)`（`false` = 不考虑软换行，单行恒为 false）。`GetVisualX` 返回从行首到光标位置的累积可视宽度，对含双宽字符的行正确计算。
   - 若 `cursorVisualCol < d.hscroll` → `d.hscroll = cursorVisualCol`（光标左出框，左拉）。
   - 若 `cursorVisualCol >= d.hscroll + contentArea.W` → `d.hscroll = cursorVisualCol - contentArea.W + 1`（光标右出框，右拉）。
   - 上限：`d.hscroll = clamp(d.hscroll, 0, max(visualLineWidth-width, 0))`。
3. **画字符**：从 `hscroll` 起用 `util.GetCharPosInLine` 反向查找起始字符列 `startCharCol`；从 `startCharCol` 起逐 rune 解码（`util.DecodeCharacter`），用 `runewidth.RuneWidth` 累加可视宽，用 `screen.SetContent` 写到 `contentArea`（支持组合字符，combc 参数传递零宽组合标记），直到可视列达到 `contentArea.W`。此过程与 InfoBar `displayBuffer`（`internal/display/infowindow.go:80-126`）一致。
4. **光标**：`screen.ShowCursor(contentArea.X + cursorVisualCol - d.hscroll, contentArea.Y)`。
5. **清背景**：`FloatFrame.Display` 已把整个外矩形清成 frameColor；内容区因 `H==1` 且已被清，无需再清。若需区分内容区底色（例如用 `config.DefStyle`），可在此重刷一行——默认沿用边框色，与 SelectDialog 视觉一致。

已知边界与实现要点：

- **`cursor.X` 是字符列，不是可视列**：必须用 `cursor.GetVisualX(false)` 转换。例如 "你好A" 中 cursor.X=2（指向 'A'），但 GetVisualX 返回 4（从行首到光标的累积可视宽度，两个中文各占2列）。
- **`GetVisualX` 返回值**：返回从行首（X=0）到光标位置的累积可视宽度，单位是「屏幕格子」（即「可视列」，从 0 开始计数）。`"你好"` 的 GetVisualX = 4，`"A"` 的 GetVisualX = 1。
- **`GetVisualX` 性能**：内部用 `util.StringWidth` 从行头遍历到 cursor.X，对于单行输入框完全可接受（单行长度通常有限，micro 主编辑器每行都用同样算法）。
- **Tab / 双宽 CJK**：`GetVisualX` 已正确处理 Tab（按 tabsize 展开到下一个 tab stop）和双宽字符（runewidth），无需额外逻辑。
- **`util.GetCharPosInLine`**：反向查找（可视列 → 字符列），用于步骤 3 找到从哪个字符开始画。
- **`hscroll` 单位**：`hscroll` 以「可视列」（屏幕格子）为单位，值域 `[0, visualLineWidth - width + 1]`。保证光标始终可见：`hscroll <= cursorVisualCol <= hscroll + width - 1`。

## 八、交互逻辑（handleEvent keymap）

`d.handleEvent` 接到的是 floatFrame 转发的非 resize 事件。分发为 switch，编辑动作落到 buffer 原语，每次修改后调 `screen.Redraw()`：

| 键 | 动作 | 落地原语 |
|---|---|---|
| `KeyRune`（含中文输入） | 插入字符 | `d.buf.Insert(d.cursor.Loc, string(r)); d.cursor.Right()` |
| `KeyLeft` | 左移一格 | `d.cursor.Left()` |
| `KeyRight` | 右移一格 | `d.cursor.Right()` |
| Ctrl-A | 至行首（X=0） | `d.cursor.X = 0`（绑定 `StartOfLine`） |
| `KeyHome` | 跳到第一个非空白字符 | `cursor.StartOfTextToggle()`（绑定 `StartOfTextToggle`） |
| Ctrl-E / `KeyEnd` | 至行尾 | `d.cursor.X = charCount`（绑定 `EndOfLine`） |
| Ctrl-Left / Ctrl-B / Alt-b | 按词左移 | `d.cursor.WordLeft()` |
| Ctrl-Right / Ctrl-F / Alt-f | 按词右移 | `d.cursor.WordRight()` |
| `KeyBackspace` / Ctrl-H | 删除前一字符 | `loc := d.cursor.Loc; d.buf.Remove(loc.Move(-1, d.buf), loc)` 后 `cursor.Left()` |
| `KeyDelete` | 删除当前字符 | `if d.cursor.Loc.LessThan(d.buf.End()) { d.buf.Remove(loc, loc.Move(1, d.buf)) }`（与 InfoBar `actions.go:797-805` 边界检查一致）|
| Alt-Backspace / Alt-Ctrl-H / Ctrl-w / Ctrl-d | 按词删除 | 删除光标前的一个词（绑定 `DeleteWordLeft`） |
| `KeyEnter` | **确认** | 取 `edited := string(d.buf.LineBytes(0))` → `Close` → `onResult(edited, false)` |
| `KeyEscape` / Ctrl-C / Ctrl-q | **取消** | `Close` → `onResult("", true)`（result 无定义，调用方应忽略） |
| 其它任何键 | 吞掉（modal） | 不转发、不回显 |

说明与取舍：

- **Home vs Ctrl-A**：`Home` 绑定 `StartOfTextToggle`（跳到第一个非空白字符），`Ctrl-A` 绑定 `StartOfLine`（X=0）。InputDialog 同时支持，遵循 InfoBar 惯例。
- **按词移动**：`Ctrl-Left` / `Ctrl-B` / `Alt-b` 都绑定 `WordLeft`，遵循 Readline/Emacs 惯例。
- **按词删除**：`Alt-Backspace` / `Alt-Ctrl-H` / `Ctrl-w` 都绑定 `DeleteWordLeft`，与 InfoBar 一致（`actions.go:764`）。
- **Ctrl-K / Ctrl-U**：InfoBar 不支持删除至行首/行尾的快捷键，InputDialog 也不支持。
- Backspace 的 `cursor.Loc.Move(-1, buf)` 在光标处于 `buf.Start()` 时返回 Start 自身，`Remove(Start, Start)` 是 no-op，天然安全（与 `actions.go:723` Backspace 的边界一致，单行无 tab-to-spaces 分支需求）。
- Delete 在行末（`cursor.Loc == buf.End()`）时 `LessThan(End)` 为 false，不执行删除，与 InfoBar 一致（`actions.go:797-805`）。
- 不接 undo/redo（Ctrl-Z / Ctrl-Y）：BTInfo buffer 本身支持，但首版 keymap 不暴露，避免与「确认/取消」语义混淆；列为未来可选。
- 不接剪贴板 Cut/Copy/Paste：首版可省；若 finder 重命名场景需要，再补。
- 每个编辑分支末尾 `screen.Redraw()`，让 `d.display` 重画并把光标拉回框内（水平滚动在 display 里算）。

## 九、生命周期与回调顺序

正常开启 → 编辑 → 确认：

```
Open(initial, ...)
  ├ d.buf = NewBufferFromString(initial, "", BTInfo)
  ├ d.cursor = d.buf.GetActiveCursor()
  ├ d.initial, d.width, d.hscroll=0, d.onResult = ...
  ├ spec := FloatOpenSpec{
  │     Anchor: anchor, AutoExpand: false,
  │     ContentSize: Size{W: width, H: 1},
  │     Title, FrameColor,
  │     Display: d.display, HandleEvent: d.handleEvent,
  │     OnResize: d.onResize,   // 见下
  │ }
  └ TheFloatFrame.Open(spec) → true（成功）
  ... 用户编辑 ...
handleEvent(KeyEnter)
  ├ edited := string(d.buf.LineBytes(0))
  ├ cb := d.onResult; d.onResult = nil
  ├ TheFloatFrame.Close()
  └ cb(edited, false)
```

取消（ESC）：

```
handleEvent(KeyEscape)
  ├ cb := d.onResult; d.onResult = nil
  ├ TheFloatFrame.Close()
  └ cb("", true)   // result 无定义，调用方应忽略
```

取消（Resize，由容器拦截）：

```
floatFrame.HandleEvent(EventResize)
  ├ cb := f.onResize          // 容器先存
  ├ f.Close()                 // 清空 onResize
  └ cb()                      // 触发 InputDialog 的 onResize
        ├ cb2 := d.onResult; d.onResult = nil
        └ cb2("", true)   // result 无定义，调用方应忽略；不再 Close（容器已关）
```

`d.onResize` 实现：取出 `onResult`、置 nil、调 `onResult("", true)`。**不**再调 `TheFloatFrame.Close()`（容器在 `HandleEvent` 里已 Close，重复 Close 是 no-op 但语义上应避免）。

关键不变量：

- `onResult` 恒触发一次：成功路径（Enter）、ESC、Resize、以及 Open 失败，四条路径都覆盖。
- 触发前置 nil，防重入（防止回调里又触发关闭导致二次回调）。
- `onResult` 触发时 floatFrame 已是 `isOpen=false`，调用方可在回调里安全开下一个浮窗（如 finder 重命名后刷新列表）。

## 十、实现要点

1. **复用现有 `min`**：`selectdialog.go:230` 已有包级 `func min(a, b int) int`（Go 1.19 无 builtin min/max，`go.mod` 确认）。InputDialog 直接复用，不要再加。
2. **`util` 工具复用**：`util.DecodeCharacter`、`util.CharacterCount`、`runewidth.RuneWidth`（InfoBar 同款），水平滚动与渲染均用这些，不引新依赖。
3. **`ContentSize.H` 恒 1**：高度不由调用方指定。外框总高 = 3（上边框 + 内容 + 下边框），floatFrame 的失败前置检查会挡掉屏幕剩余高度 < 3 的极端情况。
4. **`width` 下限**：`width <= 0` 时给一个安全默认（如 20），避免 `outerW` 退化。建议默认值 = `min(40, screen.W - 4)`（40 字符宽，但不超过屏幕宽 - 边框余量）。同时在 `Open` 里把 `width` 与屏幕宽做夹取 `width = min(width, screen.W - 4)`，确保 `outerW = width + 2 <= screen.W` 能过失败前置检查（floatFrame 在 `Open` 时检查 `outerW > w` 会返回 false）。
5. **AutoExpand 选择**：取 `false`，anchor 由调用方（finder）精确给到目标行屏坐标，外框左上角 = anchor，放置可预测（符合「重命名框压在文件名位置」的直觉）。若后续某些调用点想要「贴光标自动避开屏边」，可再增一个 `Open` 变体或加 `autoExpand bool` 入参——首版不做。
6. **光标显示**：`d.display` 里 `screen.ShowCursor(...)` 覆盖 `FloatFrame.Display` 开头的 `HideCursor`。`Close` 后无需手动恢复：主循环下一帧开头 `micro.go:492` 会无条件 `HideCursor`，再由接管交互的 pane 决定是否重显。
7. **不写日志、不自建 print**：遵循项目 Debug 规则。需要排查时走 `display.DbgLog`（`internal/display/bufwindow_md.go`）。
8. **代码注释自包含**：不引用本文档文件名/章节号（项目规则）。

## 十一、应用场景接入（finder 重命名）

finder 是独立文件管理器（见产品定位文档），自己完成 rename。用户在文件列表上按重命名键（如 `r`）时，finder 自己调 InputDialog，透传由 ownerPane 注入的 keyResolver：

```go
// internal/finder/session.go（finder 透传 keyResolver 给 InputDialog）
anchor := fm.ScreenPosOfSelectedRow()   // finder 自己的屏坐标换算
initial := fm.SelectedName()
dlg := dialog.NewInputDialog()
dlg.Open(initial, "Rename", anchor, 40, config.DefStyle,
    func(result string, canceled bool) {
        if canceled || result == "" || result == initial {
            return
        }
        fm.RenameSelected(result)        // finder 自己执行重命名 + 刷新列表
    },
    fm.keyResolver,  // 由 ownerPane 注入的键位解析函数
)
```

**keyResolver 的来源**：ownerPane（BufPane，action 包内）在 `finder.NewSession` 时构造闭包，闭包捕获包内私有 `keyEvent`（N1a §7.4）：
```go
// internal/action/bufpane.go:282
h.finder = finder.NewSession(func(e tcell.Event) string {
    if k, ok := e.(*tcell.EventKey); ok {
        return keyEvent(k).Name()
    }
    return ""
})
```

要点：

- 回调里 `canceled` 为 true 时直接 return，无需检查 `result`（无定义）。
- 未取消时，若 `result == ""` 或 `result == initial` 可视为无变化，直接 return。
- 回调触发时 floatFrame 已关，`fm.RenameSelected` 内部若要刷新列表、甚至再开浮窗（如重名冲突提示），都安全。
- `anchor` 的精确语义由 finder 决定：推荐给「选中行文件名首字符」的屏坐标，这样输入框左上角压在文件名上，视觉上就是「就地改名」。

其它场景（如未来 NotePane / 命令需要一次性单行输入）接入方式相同：new → Open → 在回调里消费 `result`。

## 十二、风险与边界

| 风险 | 说明 | 应对 |
|---|---|---|
| 双宽字符水平滚动 | hscroll 按字符列近似可视列 | **已解决**：§七算法用 `GetVisualX` 以可视列为单位，精确计算 |
| 组合字符 / 零宽字符 | CJK 输入法可能产生组合字符 | **已解决**：渲染用 `screen.SetContent(vlocX, i.Y, r, combc, style)`，支持零宽组合标记（与 InfoBar `infowindow.go:80-102` 一致） |
| 与 InfoBar 同时活跃 | floatFrame modal 期间 InfoBar 收不到事件，反之 InfoBar prompt 期间 floatFrame 也开不出（Open 返回 false） | 由 C1/C3 自然保证，无额外处理 |
| 回调里再 Open 浮窗 | onResult 触发时 floatFrame 已 Close | 安全，已验证顺序（先 Close 后回调） |
| 插件 `onAnyEvent` | 主循环每事件后跑 `RunPluginFn("onAnyEvent")`（`micro.go:597`） | 与 InputDialog 无关，不拦截；插件读到的是 floatFrame 开启态，符合预期 |
| Ctrl-A 语义重定义 | 全局 Ctrl-A=SelectAll，InputDialog 内=至行首 | 自包含 switch 不读全局绑定，行为可预测；文档已注明 |
| BTInfo buffer 的 undo 栈 | 首版不暴露 undo/redo | 留作未来可选项；BTInfo 是否记 undo 不影响首版 |
| 回调函数签名差异 | SelectDialog 用 `func(*string)`，InputDialog 用 `func(string, bool)` | **已说明**：§5.3 明确 rationale，与 SelectDialog 的语义差异（选择 vs 编辑）决定一致性让位于清晰度 |

## 十三、API 一致性说明

InputDialog 与 SelectDialog 同属 `dialog` 包，生命周期一致（floatFrame modal、Open → 编辑 → Close → 回调），但回调签名不同：

| Dialog | 回调签名 | 语义 |
|---|---|---|
| SelectDialog | `onSelect func(*string)` | 选择：nil = 取消，非 nil = 选中项 |
| InputDialog | `onResult func(result string, canceled bool)` | 编辑：result 有效当且仅当 canceled=false |

此差异是语义驱动的：SelectDialog 是「从列表选一个」，InputDialog 是「编辑输入文本」。InputDialog 的 `result string, canceled bool` 更符合 Go 的 `val, ok` 惯例，一眼可读。一致性在此让位于语义清晰，且二者调用方不同（SelectDialog 被多个子系统调用，InputDialog 主要服务于 finder），不存在统一调用接口的需求。

## 十四、未来考虑（与 InfoBar 对齐）

首版 InputDialog 聚焦单行编辑核心功能，未来可按需增强，优先参考 InfoBar 已有的能力：

| 功能 | InfoBar 状态 | InputDialog 首版 | 未来考虑 |
|---|---|---|---|
| 鼠标点击重定位光标 | InfoBar 不支持 | 不支持 | 若需支持，参考 `GetCharPosInLine` 映射屏坐标到字符列 |
| Ctrl-K / Ctrl-U | 不支持 | 不支持 | 可添加，优先参考 InfoBar 实现方式（若未来 InfoBar 添加） |
| Tab 键输入 | 支持 | 不支持 | 文件名可含 Tab（Unix 合法），若需支持参考 InfoBar |
| 剪贴板 Cut/Copy/Paste | 支持 | 不支持 | 若需支持，复制 InfoBar 的 keymap 与原语调用 |
| Undo / Redo | 支持 | 不支持 | BTInfo buffer 已支持 undo 栈，仅需暴露 keymap |
| Emoji 渲染 | 按 micro 主编辑器方式 | 按主编辑器方式 | ZWJ 序列（🏳️‍🌈）可能拆分，是终端渲染层行为 |

**原则**：InfoBar 有的我们就有，抄它的实现。首版不抄的是 InputDialog 用不上的（历史记录、建议列表等 InfoBar 特有机制）。

## 十五、实现步骤

落地顺序（供执行）：

1. 新建 `internal/dialog/input.go`，package `dialog`（N1a 迁移后在 dialog包）。
2. 写 `InputDialog` 结构体 + `NewInputDialog()`（对齐 `NewSelectDialog`）。
3. 写 `Open(...)`：初始化 buf/cursor/字段，构造 `FloatOpenSpec`（`AutoExpand:false`、`ContentSize:{W:width,H:1}`、`OnResize:d.onResize`），调 `TheFloatFrame.Open`，失败即 `onResult("",true)`。
4. 写 `d.display(contentArea)`：水平滚动 + 画单行 + `ShowCursor`。
5. 写 `d.handleEvent`：第八节 keymap 的 switch；Enter/ESC 走第九节回调顺序。
6. 写 `d.onResize`：取 `onResult`、置 nil、`onResult("",true)`。
7. `make build-quick` 编译通过；手动构造一个调用点（临时在 finder 重命名键上接，或写个 `:inputtest` 命令）验证以下功能：
   - **三态退出**：Enter 返回 edited，ESC / Resize / Open 失败返回 canceled=true
   - **光标显示**：输入后光标位置正确（单行居中）
   - **中文输入**：输入中文字符后光标位置正确（双宽字符）
   - **组合字符**：输入带变音符号的字符（如 é）正确显示
   - **水平滚动**：长文本超出宽度时滚动跟随光标
   - **用户键位**：修改 bindings.json 后键位生效（如 Ctrl-Left 按词移动）；验证 keyResolver 注入 + config.Bindings["command"] 读取正确
   - **边界处理**：Backspace 在行首无操作，Delete 在行末无操作
   - **回调触发**：验证 `onResult` 恰好触发一次，无重复调用
8. 移除临时调用点，提交。
