# MsgDialog 设计方案

| 项 | 内容 |
|---|---|
| 组件类别 | Dialog（modal，独占交互，无数据出） |
| 容器 | floatFrame（主循环级，全局单实例） |
| 参考实现 | SelectDialog（`internal/dialog/select.go`）、InputDialog（`internal/dialog/input.go`） |
| 关联文档 | `docs/fileSelect/N0-组件层架构讨论.md`、`docs/fileSelect/N1-InputDialog设计.md`、`docs/fileSelect/F2-鼠标支持-设计方案.md` |

## 一、背景与目标

MsgDialog 是一个**只读多行文本展示**浮窗。需求要点（来自 N0 §「一些需要的功能」）：

- 输入参数：一段多行文本（`\n` 分隔）、锚点坐标（屏坐标）。
- 渲染：在 floatFrame 容器内画带边框的多行文本 + 一个 `[ Close ]` 按钮。
- 交互：只读，无编辑。用户读完文本后点按钮或按确认键关闭。
- 退出：所有关闭路径都等价（无「确认/取消」之分）：
  - Enter / Space / 鼠标点击按钮 / Esc → 关闭。
  - Resize → 关闭（由容器拦截，等同 Esc）。
  - Open 失败（已有浮窗 / 屏幕放不下）→ 直接回调关闭，不展示。
- 回调：`func()`，无返回数据，恒触发一次。

它是 InputDialog / SelectDialog 的姐妹组件：同一个 floatFrame 容器、同一套生命周期约定、同一种「open 接管交互 → close 交回」的 modal 语义。区别只在内容形态——SelectDialog 画列表做选择、InputDialog 画单行做编辑、MsgDialog 画多行文本做展示。

典型应用场景：finder 操作后的结果提示（如「3 个文件已复制」）、错误信息展示（如「权限不足：/root/foo」）、长文本帮助/说明。这些场景的共同点是「只需让用户看到一段文字，不需要收集输入」。

## 二、设计约束

直接继承自 floatFrame 契约（`internal/dialog/frame.go`）与 SelectDialog / InputDialog 先例：

1. **C1 单浮窗**：全局同时最多一个浮窗。`TheFloatFrame.Open` 在已有浮窗开启时返回 false。
2. **C3 模态**：floatFrame 开启期间，主循环（`cmd/micro/micro.go:587`）把所有非 resize 事件都路由给 `TheFloatFrame.HandleEvent`，业务 pane / InfoBar / NotePane 都收不到。
3. **resize 由容器拦截**：floatFrame 在 `HandleEvent` 里拦截 `EventResize`，先 `Close()` 再触发 `OnResize` 回调，不会把 resize 转发给具体浮窗。
4. **Display 最后画胜出**：主循环渲染顺序里 floatFrame 最后画（`micro.go:536-537`），覆盖一切；且 `FloatFrame.Display()` 开头会 `HideCursor()`。
5. **contentArea 语义**：`FloatFrame.Display` 委托画内容时传入的 `Rect` 已扣除边框，恒为 `{fx+1, fy+1, contentSize.W, contentSize.H}`。具体浮窗无需感知边框。
6. **失败前置检查**：`Open` 在 `outerH > bottomLimit+1 || outerW > w` 时返回 false。
7. **鼠标事件已到达**：floatFrame 的 `HandleEvent` 只拦截 `EventResize`，其余所有事件（含 `*tcell.EventMouse`）原样转发给具体浮窗（F2 §1.2 已证实）。MsgDialog 是 dialog 族里第一个接鼠标的——按钮天然需要点击。
8. **复用优先 / 勿增实体**：能复用已有机制的不新写。`min` 复用 `select.go:228`，渲染原语复用 `screen.SetContent` + `runewidth`，键码判定复用 SelectDialog 的直接 `ev.Key()` switch（不读 `config.Bindings`，因为无编辑动作）。

## 三、总体结构

MsgDialog 是 `dialog` 包内的一个纯结构体（不带 buffer / Cursor / BufPane），形态与 SelectDialog 完全对称：

```
调用方                         floatFrame                    MsgDialog
  │  Open(...)                    │                              │
  │──────────────────────────────▶│                              │
  │                               │  存 Display/HandleEvent/     │
  │                               │       OnResize 函数值         │
  │                               │  isOpen=true, Redraw         │
  │                               │                              │
  │              主循环每帧        │  Display() → d.display(area) │──画文本+按钮（缓存 area）
  │              每个事件          │  HandleEvent(ev)             │
  │                               │     ├ resize → Close+OnResize│──onClose()
  │                               │     └ else → d.handleEvent   │──按键/鼠标 → close
  │  onClose()                    │                              │
  │◀──────────────────────────────│◀─────────────────────────────│
```

- **检测/分类/编辑**：均无。MsgDialog 不依赖 buffer、不做编辑、不分段，状态自洽（只有滚动偏移）。
- **持有者**：调用方 new 一个 `MsgDialog`，调 `Open`，之后对象本身不再被主循环引用（用完即弃，靠 GC）。这与 SelectDialog / InputDialog 生命周期一致。

## 四、数据结构

```go
// Align 文本对齐方式
type Align int
const (
    AlignLeft Align = iota
    AlignCenter
    AlignRight
)

// MsgDialog 是只读多行文本展示浮窗（modal，走 floatFrame）。
// 边框 / title / 清屏 / 锚点 / Layout / 生命周期全部移交 FloatFrame；
// 本结构只保留 softwrap 后的文本行、按钮命中缓存、关闭回调。

type MsgDialog struct {
	lines      []string  // softwrap 后的文本行（已按 width 换行）
	width      int       // 内容区宽度（字符列，不含边框），由调用者传入
	align      Align     // 文本对齐方式
	maxH       int       // 最大显示行数（0=不限制）
	textH      int       // 实际显示的文本行数
	title      string
	onClose    func()    // 关闭回调（一次性）

	// 鼠标命中判断用：display 每帧刷新
	lastArea Rect  // 最近一次 display 收到的 contentArea
	btnX     int   // 按钮左上屏坐标 X
	btnY     int   // 按钮左上屏坐标 Y
	btnW     int   // 按钮可视宽度
}
```

字段说明：

- `lines`：`Open` 时先 `strings.Split(text, "\n")` 切行，再按 `width` softwrap 换行，得到最终显示的行数组。后续不可变。
- `width`：内容区宽度（不含边框），由调用者传入。用于 softwrap 换行计算。
- `align`：文本对齐方式（左/中/右）。
- `maxH`：最大显示行数，0 表示不限制。
- `textH`：实际显示的文本行数 = `min(len(lines), maxH)`（maxH=0 时取 len(lines)）。
- `onClose`：关闭回调，一次性触发，触发后在 close 流程里置 nil（断引用，便于 GC，对齐 InputDialog / SelectDialog 的清理约定）。
- `lastArea` / `btnX` / `btnY` / `btnW`：`display` 每帧把它们刷新为当前屏坐标；`handleEvent` 收到鼠标事件时直接读这几个字段做命中判断。位置恒定（resize 即关，见约束 3），缓存安全。

不持有的东西（明确边界）：

- 不持有 `*buffer.Buffer` / `*buffer.Cursor`：只读展示，不需要编辑基座。
- 不持有屏幕尺寸 / 布局结果：全部归 floatFrame。
- 不读 `config.Bindings`：无编辑动作，键码判定走 SelectDialog 同款的直接 `ev.Key()` switch。
- **无滚动功能**：超出 `maxH` 的文本直接截断，不显示。

## 五、API 设计

### 5.1 构造

```go
// NewMsgDialog 返回空 MsgDialog（未打开状态）。
func NewMsgDialog() *MsgDialog
```

与 `NewSelectDialog` / `NewInputDialog` 对称，零参数，返回空壳。

### 5.2 打开

```go
func (d *MsgDialog) Open(
	text       string,      // 多行文本（\n 分隔）；空串按 1 行空文本处理
	title      string,      // 上边框标签（如 "Info"）；空串=纯横线
	anchor     Pos,         // 锚点屏坐标；AutoExpand=true 时为展开中心
	width      int,         // 内容区宽度（字符列，不含边框）
	align      Align,       // 文本对齐方式（Left/Center/Right）
	maxH       int,         // 最大显示行数（0=不限制，超出行截断）
	frameColor tcell.Style, // 边框色；零值 = config.DefStyle
	onClose    func(),      // 关闭回调（恒触发一次）
)
```

行为约定：

- `width` 是内容区宽度（不含边框），调用方必须指定合理的值。
- `align` 控制每行文本的对齐方式：居左、居中或居右。
- `maxH` 控制最多显示多少行文本。softwrap 后的文本行数超过 `maxH` 时，只显示前 `maxH` 行，剩余行被截断（不显示、不可滚动）。
- `onClose` 恒被调用**恰好一次**：
  - 按钮 / Enter / Space / Esc → `onClose()`。
  - Resize → `onClose()`（由容器 `OnResize` 触发）。
  - Open 失败（`TheFloatFrame.Open` 返回 false）→ 不设业务状态，直接 `onClose()`。
- 回调顺序（对齐 SelectDialog / InputDialog）：**先 `TheFloatFrame.Close()`，再触发 `onClose`**。回调里读到的容器状态已是关闭后，调用方可在回调里再次 `Open` 另一个浮窗。
- 回调无参数：MsgDialog 是「无数据出」组件，所有关闭路径等价，调用方无需区分。

### 5.3 回调签名选型

| Dialog | 回调签名 | 语义 |
|---|---|---|
| SelectDialog | `onSelect func(*string)` | 选择：nil = 取消，非 nil = 选中项 |
| InputDialog | `onResult func(result string, canceled bool)` | 编辑：result 有效当且仅当 canceled=false |
| **MsgDialog** | `onClose func()` | 展示：无数据出，所有关闭路径等价 |

MsgDialog 选 `func()` 而非 `func(canceled bool)` 的理由：

- MsgDialog 没有「确认 vs 取消」的语义二元——按钮、Enter、Esc 都是「关闭」，对调用方意义相同。
- 不返回 text：text 由调用方传入，关闭后调用方本就持有，无需回传。
- `func()` 最简，调用方写 `func() { /* 收尾 */ }` 即可，无冗余参数。

### 5.4 内部 display / handleEvent（由 Open 塞给 floatFrame）

```go
d.display(contentArea Rect)        // 画文本 + 按钮 + 滚动指示，并缓存 area
d.handleEvent(event tcell.Event)   // 键盘 + 鼠标
```

两者均不导出，仅以函数值形式注入 `FloatOpenSpec`，与 SelectDialog / InputDialog 同。

## 六、布局与尺寸计算

这是 MsgDialog 与 InputDialog 最大的差异点：InputDialog 高度恒 1，MsgDialog 高度随文本行数变化。

### 6.1 宽度

```
width = 调用者传入（内容区宽度，不含边框）
```

说明：

- `width` 由调用者直接指定，不自动计算。宽度是固定的，不会随内容或屏幕尺寸变化。
- 调用者需要确保 `width + 2 <= screen.W`（外宽度 = 内容宽度 + 2 边框），否则 `TheFloatFrame.Open` 会返回 false。
- 文本按 `width` 进行 softwrap 换行，换行算法见 §6.2（复用 micro 的 `runewidth` 和 Tab 展开逻辑，但不需要 wordwrap）。

### 6.2 Softwrap 换行算法

micro 已有完整的 softwrap 实现（`internal/display/softwrap.go`），支持 wordwrap、Tab 展开、CJK 双宽字符、双向坐标转换（Loc ↔ VLoc）。但该实现依赖 `BufWindow`/`Buffer`，MsgDialog 需要一个简化的独立版本。

**简化算法**（不需要 wordwrap，只需基本的字符宽度计算与换行）：

```
func softwrap(text string, width int) []string {
    tabsize := int(config.GetGlobalOption("tabsize").(float64))
    rawLines := strings.Split(text, "\n")
    lines := []string{}
    
    for _, rawLine := range rawLines {
        if rawLine == "" {
            lines = append(lines, "")
            continue
        }
        
        // 按 width 分割这一行
        currentLine := ""
        currentWidth := 0
        
        for _, r := range rawLine {
            rWidth := runeWidth(r, currentWidth, tabsize)
            
            if currentWidth + rWidth > width && currentLine != "" {
                // 当前行已满，开始新行
                lines = append(lines, currentLine)
                currentLine = string(r)
                currentWidth = rWidth
            } else {
                currentLine += string(r)
                currentWidth += rWidth
            }
        }
        
        if currentLine != "" {
            lines = append(lines, currentLine)
        }
    }
    
    return lines
}

func runeWidth(r rune, col int, tabsize int) int {
    switch r {
    case '\t':
        return tabsize - (col % tabsize)
    default:
        return runewidth.RuneWidth(r)
    }
}
```

说明：

- **不依赖 BufWindow**：独立函数，只需 `width` 参数，`tabsize` 从 `config.GetGlobalOption("tabsize")` 读取。
- **Tab 展开**：按标准规则展开（`tabsize - (col % tabsize)`），`tabsize` 从 `config.GetGlobalOption("tabsize")` 读取，与 InfoBar / softwrap.go 一致。
- **CJK 双宽**：复用 `runewidth.RuneWidth`，与 InputDialog / SelectDialog 一致。
- **不在双宽字符中间断开**：检测到 `currentWidth + rWidth > width` 且当前行非空时，先结束当前行，再开始新行。
- **空行保留**：`strings.Split("", "\n")` 返回 `[]string{"")`，空行会被保留。
- **不实现 wordwrap**：单词可以在任意位置断开（与原 softwrap.go 的 wordwrap 模式不同）。MsgDialog 用于展示调用方给定的文本，调用方如果需要 wordwrap，需要预先在文本里加好换行符。

### 6.3 高度

```
totalLines    = len(lines)                 // softwrap 后的总行数
if maxH > 0:
    textH = min(totalLines, maxH)         // 限制显示行数
else:
    textH = totalLines                    // 不限制
contentH      = textH + 2                  // 文本 + 空行 + 按钮行
```

说明：

- `maxH` 控制最多显示多少行文本，超出部分截断。
- `maxH=0` 表示不限制，显示所有 softwrap 后的行。
- 按钮与文本之间隔一个空行（视觉分隔），所以 `contentH = textH + 2`。
- 调用者需要确保 `contentH + 2 <= screen.H - infoBarOffset - 1`（外高度通过 floatFrame 前置检查），否则 `TheFloatFrame.Open` 返回 false。

### 6.4 文本对齐

每行文本按 `align` 对齐：

```
lineWidth = runewidth.StringWidth(line)
if align == AlignLeft:
    startX = area.X
elif align == AlignCenter:
    startX = area.X + (width - lineWidth) / 2
elif align == AlignRight:
    startX = area.X + width - lineWidth
```

说明：

- 居左：文本从内容区左边开始绘制。
- 居中：文本在内容区中央，左右两侧可能有空格填充。
- 居右：文本靠内容区右边对齐。
- 超出 `width` 的部分不会出现（softwrap 已确保 lineWidth <= width）。

### 6.5 按钮位置

按钮在文本下方隔一行，水平居中：

```
btnText     = " Close "                         // 1 空格 + "Close" + 1 空格
btnVisualW  = runewidth.StringWidth(btnText)   // 动态计算（支持 CJK）
btnRow      = area.Y + textH + 1                // 文本 + 空行
btnX        = area.X + (width - btnVisualW) / 2 // 水平居中
btnY        = btnRow
```

## 七、渲染设计

`FloatFrame.Display` 已完成：清外矩形 → 画 4 角 + 上下左右边 → title 嵌入上边框 → `HideCursor()` → 委托 `d.display(contentArea)`。注意：`FloatFrame` 已清空整个 contentArea，display 里再次清行是为了保证对齐正确（从 `startX` 画文本前先清整行，确保底色一致）。

`d.display(contentArea)` 职责：画文本行、空行、按钮、缓存命中区域。MsgDialog **不调** `ShowCursor`（只读，无光标），保留 floatFrame 开头的 `HideCursor`。

```
display(area):
    d.lastArea = area                              // 缓存给鼠标用

    revStyle := config.DefStyle.Reverse(true)
    style    := config.DefStyle

    // 1. 文本行 [0, textH)
    for vi in [0, textH):
        row := area.Y + vi
        line := d.lines[vi]
        lineWidth := runewidth.StringWidth(line)

        // 按 align 计算起始 X
        if d.align == AlignLeft:
            startX = area.X
        elif d.align == AlignCenter:
            startX = area.X + (d.width - lineWidth) / 2
        else: // AlignRight
            startX = area.X + d.width - lineWidth

        // 先清空整行（底色一致）
        for col in [area.X, area.X + d.width):
            SetContent(col, row, ' ', nil, style)

        // 画文本（从 startX 开始）
        col := startX
        for r in line (逐 rune):
            w := runewidth.RuneWidth(r)
            SetContent(col, row, r, nil, style)
            if w > 1:
                for k in [1, w): SetContent(col+k, row, ' ', nil, style)   // 双宽占位
            col += w

    // 2. 空行（文本与按钮的分隔）
    emptyRow := area.Y + textH
    for col in [area.X, area.X + d.width):
        SetContent(col, emptyRow, ' ', nil, style)

    // 3. 按钮行（文本下方 +1 行，水平居中，反白底）
    btnRow := area.Y + textH + 1
    btnText := " Close "
    btnVisualW := runewidth.StringWidth(btnText)
    d.btnX = area.X + (d.width - btnVisualW) / 2
    d.btnY = btnRow
    d.btnW = btnVisualW
    for i, r in btnText: SetContent(d.btnX + i, btnRow, r, nil, revStyle)
```

已知边界与实现要点：

- **不调 ShowCursor**：与 InputDialog 相反。MsgDialog 只读，floatFrame 开头的 `HideCursor` 直接生效，无需覆盖。
- **双宽字符占位**：CJK / emoji 后半格写空格占位（与 InputDialog `display` 同款循环），避免下一字符覆盖前半。
- **组合字符**：`screen.SetContent` 的 `combc` 参数本版传 nil（MsgDialog 不接输入法，文本是调用方给定的完整字符串）。若未来要支持拆分输入法组合字符，再参考 InfoBar `infowindow.go:80-102` 的 `util.DecodeCharacter` 路径。
- **行先清空再画文本**：每行先填空格清空，再从 `startX` 开始画文本，保证对齐正确且底色一致。
- **无滚动指示**：MsgDialog 不支持滚动，超出行截断，无 `▲▼` 指示符。

## 八、交互逻辑（handleEvent）

`d.handleEvent` 接到的是 floatFrame 转发的非 resize 事件（含键盘与鼠标）。分发为 switch，关闭路径走统一 `close()`。

### 8.1 键盘

| 键 | 动作 | 说明 |
|---|---|---|
| `KeyEnter` / `KeySpace` | 关闭 | 激活按钮（按钮恒聚焦，反白即焦点） |
| `KeyEscape` | 关闭 | 取消键，与 InputDialog 一致 |
| 其它任何键 | 吞掉（modal） | 不转发、不回显 |

说明与取舍：

- **不读 `config.Bindings`**：与 SelectDialog 同款，直接 `ev.Key()` switch。MsgDialog 没有编辑动作，「确认/取消」都是固定语义键，用户自定义键位在此无意义；保持简单可预测。
- **Enter 与 Space 等价**：按钮恒聚焦（单按钮），二者都是「激活当前焦点按钮」的标准语义。
- **无滚动键**：MsgDialog 不支持滚动，上下键/PgUp/PgDn/Home/End 都不处理（吞掉）。

### 8.2 鼠标

复用 F2 §2 建立的「display 缓存 contentArea → handleEvent 命中判断」范式：

| 事件 | 动作 | 命中条件 |
|---|---|---|
| 左键按下（`Button1`） | 关闭 | `mx ∈ [btnX, btnX+btnW)` 且 `my == btnY` |
| 其它（点文本区 / 点边框 / 拖动 / 滚轮） | 忽略 | 不关闭、不滚动 |

说明：

- **点文本区不关闭**：避免用户误触（读文本时不小心点击）。只有点按钮才关闭，行为可预测。
- **无滚轮支持**：MsgDialog 不支持滚动，滚轮事件被忽略。
- **命中坐标来自 `d.lastArea`**：`display` 每帧刷新 `btnX/btnY/btnW`。由于 floatFrame.Open 后位置恒定（resize 即关），首次鼠标事件到达时 `display` 至少已跑过一帧（Open → Redraw → 下一帧 Display → 事件），缓存恒有效。

伪代码：

```go
case *tcell.EventMouse:
	mx, my := ev.Position()
	btns := ev.Buttons()
	// 左键点击按钮 → 关闭
	if btns & tcell.Button1 != 0 {
		if mx >= d.btnX && mx < d.btnX+d.btnW && my == d.btnY {
			d.close()
		}
	}
	// 滚轮/其它事件忽略
```

## 九、生命周期与回调顺序

正常开启 → 关闭（按钮 / Enter / Space）：

```
Open(text, ..., width, align, maxH, ...)
  ├ rawLines := strings.Split(text, "\n")
  ├ d.lines = softwrap(rawLines, width)       // 按 width 换行
  ├ d.textH = 计算显示行数（按 maxH 限制）
  ├ d.width = width, d.align = align, d.maxH = maxH
  ├ contentH := d.textH + 2                      // 文本 + 空行 + 按钮行
  ├ spec := FloatOpenSpec{
  │     Anchor, ContentSize: Size{W: width, H: contentH},
  │     Title, FrameColor,
  │     Display: d.display, HandleEvent: d.handleEvent,
  │     OnResize: d.onResize, AutoExpand: true,
  │ }
  └ TheFloatFrame.Open(spec) → true（成功）
  ... 用户读文本 / 点按钮 ...
handleEvent(KeyEnter 或 Button1 命中按钮)
  ├ cb := d.onClose; d.onClose = nil
  ├ TheFloatFrame.Close()
  └ cb()
```

取消（Esc）：

```
handleEvent(KeyEscape)
  ├ cb := d.onClose; d.onClose = nil
  ├ TheFloatFrame.Close()
  └ cb()
```

取消（Resize，由容器拦截）：

```
floatFrame.HandleEvent(EventResize)
  ├ cb := f.onResize          // 容器先存
  ├ f.Close()                 // 清空 onResize
  └ cb()                      // 触发 MsgDialog 的 onResize
        ├ cb2 := d.onClose; d.onClose = nil
        └ cb2()               // 不再 Close（容器已关）
```

`d.onResize` 实现：取出 `onClose`、置 nil、调 `onClose()`。不再调 `TheFloatFrame.Close()`（容器已 Close，重复 Close 是 no-op 但语义上应避免）。与 InputDialog `onResize` 完全对称，只是回调签名无参。

关键不变量：

- `onClose` 恒触发一次：按钮、Enter、Space、Esc、Resize、Open 失败，六条路径都覆盖。
- 触发前置 nil，防重入。
- `onClose` 触发时 floatFrame 已是 `isOpen=false`，调用方可在回调里安全开下一个浮窗。

## 十、与 InputDialog 的差异对比

| 维度 | InputDialog | MsgDialog |
|---|---|---|
| 用途 | 单行文本输入/编辑 | 多行文本只读展示 |
| 编辑 | 有（buffer + cursor 原语） | 无（静态 `[]string`） |
| 底层数据 | `*buffer.Buffer`(BTInfo) + `*buffer.Cursor` | `[]string`（softwrap 后） |
| 内容高度 | 恒 1（`ContentSize.H = 1`） | 可变（`textH + 2`，按 maxH 限制） |
| 宽度计算 | 调用方指定 `width` | 调用方指定 `width` |
| 对齐方式 | 无（左对齐） | 可选（Left/Center/Right） |
| 行数限制 | 无（恒 1 行） | 有（maxH 控制） |
| 换行 | 无 | 有（softwrap） |
| 光标显示 | `ShowCursor` 覆盖 `HideCursor` | 不调 `ShowCursor`（保留 `HideCursor`） |
| 滚动 | 有（`hscroll` 跟随光标） | 无（超出行截断） |
| 键位映射 | 读 `config.Bindings["command"]`（跟随用户自定义） | 直接 `ev.Key()` switch（固定语义键） |
| 鼠标支持 | 首版无 | 有（按钮点击） |
| 退出语义 | Enter=确认 / Esc=取消（二元） | 所有路径等价（无二元） |
| 回调签名 | `func(result string, canceled bool)` | `func()` |
| AutoExpand | `true`（实现值，N1 设计原写 false 已调整） | `true` |

共性（不变的部分）：

- 同一 floatFrame 容器、同一 `Open → ... → Close → 回调` 生命周期。
- 回调顺序一致：先 `TheFloatFrame.Close()`，再触发回调。
- 失败路径一致：Open 返回 false 时不设业务状态，直接回调。
- `onResize` 实现一致：取回调、置 nil、调回调，不再重复 Close。
- 纯结构体形态，不持 BufPane，用完即弃靠 GC。
- 复用 `min`（`select.go:228`）、`config.DefStyle`、`screen.SetContent` / `screen.Redraw`、`util.StringWidth` / `runewidth.RuneWidth`。

## 十一、实现要点

1. **复用现有 `min`**：`select.go:228` 已有包级 `func min(a, b int) int`。MsgDialog 直接复用，不要再加。需要 `max` 时用 `if x > max { x = max }` 内联，不为单点使用加全局函数。
2. **不写日志、不自建 print**：遵循项目 Debug 规则。需要排查时走 `display.DbgLog`（`internal/display/bufwindow_md.go`）。
3. **`util.StringWidth` 的参数**：签名 `StringWidth(b []byte, n, tabsize int) int`，`n` 是字符数（`util.CharacterCountInString(line)`），`tabsize` 读 `config.GetGlobalOption("tabsize").(float64)` 转 int（与 InfoBar / InputDialog 同款读法）。
4. **`strings.Split` 行为**：`strings.Split("", "\n")` 返回 `[""]`（长度 1），空文本也能正常画 1 行空内容 + 按钮，无需特判。
5. **`contentW` 下限**：即便 text 与按钮都极短，`contentW` 至少 `btnVisualW + 2 = 9`，保证按钮不被压扁。
6. **`AutoExpand=true`**：anchor 由调用方给「期望位置」（如光标位、屏幕中央 `{W/2, H/2}`），floatFrame 自动避开屏边。N1 InputDialog 设计原写 `false`，实现已调整为 `true`（`input.go:81`），MsgDialog 对齐实现值。
7. **鼠标命中缓存**：`display` 每帧写 `lastArea/btnX/btnY/btnW`；`handleEvent` 读这四个字段。由于位置恒定，缓存恒有效，无需担心过期。
8. **不接 `config.Bindings`**：MsgDialog 的键（Enter/Esc/方向键）都是固定语义，直接 `ev.Key()` switch，行为可预测、不随用户配置漂移。与 SelectDialog 同款。
9. **代码注释自包含**：不引用本文档文件名/章节号（项目规则）。注释只讲「为什么这么写」。

## 十二、风险与边界

| 风险 | 说明 | 应对 |
|---|---|---|
| width 过小 | `width < 9` 时按钮（" Close " 宽度 7）会溢出内容区 | 调用方需确保 `width >= 9`；文档明确说明最小宽度要求 |
| width 过大 | `width + 2 > screen.W` 时 floatFrame.Open 返回 false | 调用方需确保 width 合理；失败路径直接回调 onClose() |
| maxH 过大 | `textH + 2 > screen.H - infoBarOffset - 1` 时 floatFrame.Open 返回 false | 调用方需确保 maxH 合理；失败路径直接回调 onClose() |
| softwrap 实现 | 需正确处理 CJK 双宽、Tab 展开、双宽字符中间不断行 | 复用 `runewidth.RuneWidth` 和 Tab 展开逻辑，从 config 读 tabsize |
| Tab 宽度不一致 | tabsize 缺失时 Tab 按硬编码值展开，与主编辑器不一致 | 从 `config.GetGlobalOption("tabsize")` 读默认值 |
| 空行保留 | `strings.Split("", "\n")` 返回 `[""]`，空文本应显示 1 行空内容 | softwrap 保持空行，行数组里有空字符串 |
| align 居中计算 | lineWidth 为奇数时居中可能有 1 列偏差 | 使用整数除法，1 列偏差可接受 |
| 文本被截断 | maxH 限制导致部分文本不可见 | 调用方负责设置合理的 maxH；文档明确说明截断行为 |
| softwrap 边界 | `currentWidth + rWidth == width` 时字符留在本行，不提前换行 | 算法逻辑正确（`> width` 才换行），文档明确说明边界行为 |
| 鼠标点文本区误关闭 | 用户读文本时不小心点击 | 仅点按钮关闭，点文本区/边框/拖动都忽略（§8.2） |
| 回调里再 Open 浮窗 | onClose 触发时 floatFrame 已 Close | 安全，已验证顺序（先 Close 后回调） |
| 鼠标事件首帧缓存未就绪 | Open 后第一个事件到达前 display 是否已跑 | Open → Redraw → 下一帧 Display → 事件，display 至少跑过一帧；且位置恒定，缓存不会过期 |
| 插件 `onAnyEvent` | 主循环每事件后跑 `RunPluginFn("onAnyEvent")`（`micro.go:597`） | 与 MsgDialog 无关，不拦截；插件读到的是 floatFrame 开启态，符合预期 |
| 与 SelectDialog / InputDialog 同屏 | floatFrame modal 期间只能开一个，反之亦然 | 由 C1/C3 自然保证，无额外处理 |

## 十三、未来考虑

首版聚焦「展示文本 + 关闭」核心功能，未来可按需增强：

| 功能 | 首版 | 未来考虑 |
|---|---|---|
| 按钮文案自定义 | 硬编码 `" Close "` | 加 `buttonLabel string` 入参（如 "OK" / "Dismiss" / "Got it"）；改动极小 |
| 滚动功能 | 不支持（截断） | 若需要，可恢复滚动模式（加 topIdx、scrollable、▲▼、滚轮支持） |
| 按钮键盘焦点切换 | 单按钮恒聚焦 | MsgDialog 只有一个按钮，无需切换；ConfirmDialog（双按钮）才需要焦点切换 |
| 文本可选择/复制 | 不支持 | 终端层鼠标选择由终端模拟器处理（非应用层）；若需应用内选择，参考 mouse drag 范式 |
| 多按钮（ConfirmDialog） | 不在本设计内 | N0 已规划 ConfirmDialog（OK/Cancel 双按钮），将单独写 N1d 设计文档，复用本文档的渲染/鼠标范式 |

## 十四、实现步骤

落地顺序（供执行）：

1. 新建 `internal/dialog/msg.go`，`package dialog`。
2. 写 `MsgDialog` 结构体 + `NewMsgDialog()`（对齐 `NewSelectDialog` / `NewInputDialog`）。
3. 写 `softwrap(text, width int) []string` 函数：按 `width` 对原始行进行软换行，正确处理 CJK 双宽、Tab 展开、双宽字符中间不断行。`tabsize` 从 `config.GetGlobalOption("tabsize")` 读取。
4. 写 `Open(...)`：`strings.Split` 切行 → `softwrap` 换行 → 按 §6 算 `textH/contentH` → 构造 `FloatOpenSpec`（`AutoExpand:true`、`ContentSize`、`OnResize:d.onResize`）→ 调 `TheFloatFrame.Open`，失败即清理 + `onClose()`。
5. 写 `d.display(area)`：§7 三段（文本行 / 空行 / 按钮行）+ 缓存 `lastArea/btnX/btnY/btnW`。注意按 `align` 计算每行起始 X。
6. 写 `d.handleEvent`：§8 键盘 switch（Enter/Space/Esc → close）+ 鼠标分支（Button1 命中按钮 → close）。
7. 写 `d.close()`：取 `onClose`、置 nil、`TheFloatFrame.Close()`、调 `onClose()`。
8. 写 `d.onResize()`：取 `onClose`、置 nil、调 `onClose()`（不再 Close，容器已关）。
9. `make build-quick` 编译通过；手动构造一个调用点（临时写个 `:msgtest` 命令，或 finder 某个提示路径上接）验证：
   - **三态退出**：Enter / Space / Esc / 点按钮 → 回调触发一次
   - **Open 失败**：先开另一个浮窗再 Open MsgDialog → 直接回调，不展示
   - **多行渲染**：3-5 行短文本布局正确（文本区 + 1 空行分隔 + 按钮行居下）
   - **CJK 双宽**：含中文的行宽度计算正确，不被压扁、不重叠
   - **长行换行**：传入超宽行 → softwrap 换行正确，不超过 width
   - **align 对齐**：Left/Center/Right 三种对齐方式正确
   - **maxH 限制**：设置 maxH=5，传入 8 行文本 → 只显示前 5 行
   - **按钮点击命中**：点按钮区域内 → 关闭；点按钮外（文本区/边框）→ 忽略
   - **光标隐藏**：浮窗期间无可见光标（与 InputDialog 相反）
   - **回调触发次数**：验证 `onClose` 恰好触发一次，无重复
10. 移除临时调用点，提交。
