# ConfirmDialog 设计方案

| 项 | 内容 |
|---|---|
| 组件类别 | Dialog（modal，独占交互，数据出 = 布尔选择） |
| 容器 | floatFrame（主循环级，全局单实例） |
| 参考实现 | MsgDialog（`internal/dialog/msg.go`）、InputDialog（`internal/dialog/input.go`）、SelectDialog（`internal/dialog/select.go`） |
| 关联文档 | `docs/fileSelect/N0-组件层架构讨论.md`、`docs/fileSelect/N1c-MsgDialog设计.md`、`docs/fileSelect/N1a-Dialog包迁移计划.md`、`docs/fileSelect/F2-鼠标支持-设计方案.md` |

## 一、背景与目标

ConfirmDialog 是一个**带二选一按钮的多行文本展示浮窗**。需求要点：

- 输入参数：一段多行文本（`\n` 分隔）、锚点坐标（屏坐标）。
- 渲染：在 floatFrame 容器内画带边框的多行文本 + 两个按钮 `[  OK  ] [Cancel]`。
- 交互：只读，无编辑。用户读完文本后选择 OK 或 Cancel，把选择结果回传给调用方。
- 退出语义：
  - **OK**：点击 OK 按钮 / Enter（焦点在 OK 时） → `onResult(true)`。
  - **Cancel**：点击 Cancel 按钮 / Esc / Enter（焦点在 Cancel 时） → `onResult(false)`。
  - Resize → `onResult(false)`（由容器拦截，等同 Cancel）。
  - Open 失败（已有浮窗 / 屏幕放不下） → 直接回调 `onResult(false)`，不展示。
- 回调：`func(confirmed bool)`，恒触发一次。

典型应用场景：finder 删除文件/目录前的二次确认（「Delete foo.txt? This cannot be undone.」）、覆盖文件前的确认、退出未保存缓冲区前的确认。

## 二、设计约束

与 MsgDialog 完全一致，直接继承自 floatFrame 契约：

1. **C1 单浮窗**：全局同时最多一个浮窗。
2. **C3 模态**：floatFrame 开启期间，主循环把所有非 resize 事件都路由给 `TheFloatFrame.HandleEvent`。
3. **resize 由容器拦截**：floatFrame 拦截 `EventResize`，先 `Close()` 再触发 `OnResize` 回调。
4. **Display 最后画胜出**：floatFrame 最后画，覆盖一切；且开头会 `HideCursor()`。
5. **contentArea 语义**：传入的 `Rect` 已扣除边框，恒为 `{fx+1, fy+1, contentSize.W, contentSize.H}`。
6. **失败前置检查**：`Open` 在尺寸超限时返回 false。
7. **鼠标事件已到达**：floatFrame 只拦截 `EventResize`，其余事件（含鼠标）原样转发。
8. **复用优先**：能复用已有机制的不新写。

## 三、总体结构

ConfirmDialog 是 `dialog` 包内的一个纯结构体（不带 buffer / Cursor / BufPane），形态与 MsgDialog 完全对称，仅多一个「焦点按钮」状态：

```
调用方                         floatFrame                    ConfirmDialog
  │  Open(...)                    │                              │
  │──────────────────────────────▶│                              │
  │                               │  存 Display/HandleEvent/     │
  │                               │       OnResize 函数值         │
  │                               │  isOpen=true, Redraw         │
  │                               │                              │
  │              主循环每帧        │  Display() → d.display(area) │──画文本 + 空行 + 双按钮
  │              每个事件          │  HandleEvent(ev)             │
  │                               │     ├ resize → Close+OnResize│
  │                               │     └ else → d.handleEvent   │──Tab/←/→ 切焦点；Enter/Space/Esc/点击 → close
  │  onResult(true/false)         │                              │
  │◀──────────────────────────────│◀─────────────────────────────│
```

## 四、数据结构

```go
// 按钮索引（focus 字段取值）
const (
	btnOK = iota  // 0：OK 按钮，默认焦点
	btnCancel     // 1：Cancel 按钮
)

// ConfirmDialog 是带 OK/Cancel 双按钮的多行文本确认浮窗（modal，走 floatFrame）。
type ConfirmDialog struct {
	lines    []string // softwrap 后的文本行（复用 msg.go 的 softwrap）
	width    int      // 内容区宽度（字符列，不含边框），由调用者传入
	align    Align    // 文本对齐方式（复用 msg.go 的 Align）
	maxH     int      // 最大显示行数（0=不限制）
	textH    int      // 实际显示的文本行数
	title    string
	focus    int      // 当前焦点按钮：btnOK(0) 默认 / btnCancel(1)
	onResult func(confirmed bool) // 关闭回调（一次性）

	// 鼠标命中判断用：display 每帧刷新
	lastArea Rect       // 最近一次 display 收到的 contentArea
	btnRects [2]Rect    // [btnOK, btnCancel] 的屏坐标命中矩形（H 恒为 1）
}
```

字段说明：

- `lines`：`Open` 时复用 `msg.go` 的 `softwrap(text, width)` 切行换行。
- `width` / `align` / `maxH` / `textH`：语义与 MsgDialog 完全一致。
- `focus`：当前焦点按钮索引，`btnOK=0`（默认）/ `btnCancel=1`。默认值 `btnOK` 让「直接按 Enter」等价于 OK。
- `onResult`：结果回调，一次性触发，触发后置 nil。
- `btnRects`：两个按钮的屏坐标命中矩形，位置恒定（resize 即关），缓存安全。

## 五、API 设计

### 5.1 构造

```go
// NewConfirmDialog 返回空 ConfirmDialog（未打开状态）。
func NewConfirmDialog() *ConfirmDialog
```

### 5.2 打开

```go
func (d *ConfirmDialog) Open(
	text       string,      // 多行文本（\n 分隔）
	title      string,      // 上边框标签
	anchor     Pos,         // 锚点屏坐标
	width      int,         // 内容区宽度（字符列，不含边框），需 >= 18
	align      Align,       // 文本对齐方式（Left/Center/Right）
	maxH       int,         // 最大显示行数（0=不限制）
	frameColor tcell.Style, // 边框色；零值 = config.DefStyle
	onResult   func(confirmed bool), // 结果回调（恒触发一次）
)
```

行为约定：

- `width` 需满足 `width >= 18`（双按钮组最小宽度）。
- `onResult(true)`：OK 路径
- `onResult(false)`：Cancel / Esc / Resize / Open 失败路径
- 回调顺序：先 `TheFloatFrame.Close()`，再触发 `onResult`

### 5.3 回调签名选型

| Dialog | 回调签名 | 语义 |
|---|---|---|
| SelectDialog | `onSelect func(*string)` | 选择：nil = 取消，非 nil = 选中项 |
| InputDialog | `onResult func(result string, canceled bool)` | 编辑：result 有效当且仅当 canceled=false |
| MsgDialog | `onClose func()` | 展示：无数据出，所有关闭路径等价 |
| **ConfirmDialog** | `onResult func(confirmed bool)` | 确认：true = OK，false = Cancel |

ConfirmDialog 选 `func(confirmed bool)` 的理由：组件名即语义，调用点 `if confirmed { delete() }` 最自然。

### 5.4 内部 display / handleEvent

```go
d.display(contentArea Rect)        // 画文本 + 空行 + 双按钮（焦点反白），并缓存命中矩形
d.handleEvent(event tcell.Event)   // 键盘（切焦点 / 激活 / 取消）+ 鼠标（点击按钮）
```

## 六、布局与尺寸计算

ConfirmDialog 与 MsgDialog 共享文本区布局，差异只在**底部按钮行**。

### 6.1 宽度

```
width = 调用者传入（内容区宽度，不含边框）
```

调用者需确保 `width >= 18`（双按钮组宽度）。

### 6.2 高度

```
totalLines    = len(lines)                 // softwrap 后的总行数
if maxH > 0:
    textH = min(totalLines, maxH)         // 限制显示行数
else:
    textH = totalLines                    // 不限制
contentH      = textH + 2                  // 文本 + 空行 + 按钮行
```

### 6.3 双按钮组布局

```
okText      = "[  OK  ]"      // 方括号 + 2空格 + OK + 2空格 = 可视宽 8
cancelText  = "[Cancel]"      // 方括号 + Cancel = 可视宽 8
gap         = 2               // 两按钮之间的空格数
groupW      = 8 + 2 + 8 = 18  // 双按钮组整体可视宽

btnRow      = area.Y + textH + 1                 // 按钮行
okX         = area.X + (width - groupW) / 2      // 整组水平居中
cancelX     = okX + 8 + gap                      // OK 右侧隔 gap 列
okY = cancelY = btnRow
```

命中矩形（display 每帧写入 `btnRects`）：

```
btnRects[btnOK]     = Rect{X: okX,     Y: btnRow, W: 8, H: 1}
btnRects[btnCancel] = Rect{X: cancelX, Y: btnRow, W: 8, H: 1}
```

说明：

- **整组居中**：两个按钮作为一个视觉单元居中。
- **`groupW` 用 `runewidth.StringWidth` 动态算**：写实现时不要硬编码 18。
- **最小宽度**：`width < 18` 时按钮组溢出内容区。

### 6.4 焦点高亮

焦点按钮反白（`Reverse(true)`），非焦点按钮正常底（`config.DefStyle`）。

```
for i in {btnOK, btnCancel}:
    style = config.DefStyle
    if i == d.focus:
        style = config.DefStyle.Reverse(true)
    画 btnRects[i] 范围内的按钮文案，用 style
```

## 七、渲染设计

`d.display(contentArea)` 职责：画文本行、空行、双按钮行、缓存命中矩形。

```
display(area):
    d.lastArea = area
    d.btnRects[btnOK].Y    = area.Y + d.textH + 1
    d.btnRects[btnCancel].Y = area.Y + d.textH + 1

    revStyle := config.DefStyle.Reverse(true)
    style    := config.DefStyle

    // 1. 文本行 [0, textH) —— 调公共函数（阶段 0 已抽取）
    drawTextLines(area, d.lines, d.textH, d.width, d.align, style)

    // 2. 空行（文本与按钮的分隔）
    emptyRow := area.Y + d.textH
    清空整行 [area.X, area.X+d.width) 用 style

    // 3. 双按钮行
    okText     := "[  OK  ]"
    cancelText := "[Cancel]"
    gap := 2
    okW := runewidth.StringWidth(okText)        // = 8
    cancelW := runewidth.StringWidth(cancelText) // = 8
    groupW := okW + gap + cancelW               // = 18

    btnRow := area.Y + d.textH + 1
    okX := area.X + (d.width - groupW) / 2
    cancelX := okX + okW + gap

    d.btnRects[btnOK]     = Rect{X: okX,     Y: btnRow, W: okW,     H: 1}
    d.btnRects[btnCancel] = Rect{X: cancelX, Y: btnRow, W: cancelW, H: 1}

    // 焦点按钮反白，非焦点正常（调公共函数）
    okStyle     := btnOK     == d.focus ? revStyle : style
    cancelStyle := btnCancel == d.focus ? revStyle : style
    drawButton(d.btnRects[btnOK],     okText,     okStyle)
    drawButton(d.btnRects[btnCancel], cancelText, cancelStyle)
```

## 八、交互逻辑（handleEvent）

### 8.1 键盘

| 键 | 动作 | 说明 |
|---|---|---|
| `KeyTab` / `KeyBacktab` / `KeyLeft` / `KeyRight` | 切换焦点 | `d.focus = 1 - d.focus`，`screen.Redraw()` |
| `KeyEnter` / `KeySpace` | 激活焦点按钮 | `d.close(d.focus == btnOK)` |
| `KeyEscape` | 取消 | `d.close(false)`（无论焦点在哪） |
| 其它任何键 | 吞掉（modal） | 不转发、不回显 |

说明：

- **「Enter = OK」的满足方式**：默认焦点是 `btnOK`，故用户直接按 Enter 时 `d.focus==btnOK` 成立 → `confirmed=true`。
- **Esc 恒取消**：与 InputDialog / MsgDialog / SelectDialog 的 Esc 语义一致。
- **Space 与 Enter 等价**：都是「激活当前焦点按钮」的标准语义。
- **不读 `config.Bindings`**：直接 `ev.Key()` switch，保持简单可预测。

### 8.2 鼠标

| 事件 | 动作 | 命中条件 |
|---|---|---|
| 左键按下（`Button1`）点中 OK | `close(true)` | `(mx,my)` 落在 `btnRects[btnOK]` |
| 左键按下（`Button1`）点中 Cancel | `close(false)` | `(mx,my)` 落在 `btnRects[btnCancel]` |
| 其它（点文本区 / 边框 / 间隙） | 忽略 | 不关闭、不切焦点 |

说明：

- 点击即激活，不改焦点再激活。
- 点文本区 / 边框 / 两按钮间隙都不关闭（避免误触）。
- 鼠标拖拽场景：用户在按钮上按下、拖到别处、松开——此时仍在按钮上会触发，拖到按钮外则忽略。这是预期行为（简单可预测）。

伪代码：

```go
case *tcell.EventMouse:
	mx, my := ev.Position()
	if ev.Buttons() & tcell.Button1 != 0 {
		for i, r := range d.btnRects {
			if mx >= r.X && mx < r.X+r.W && my == r.Y {
				d.close(i == btnOK)
				return
			}
		}
	}
```

## 九、生命周期与回调顺序

正常开启 → OK：

```
Open(text, ..., width, align, maxH, ...)
  ├ d.focus = btnOK
  ├ d.lines = softwrap(text, width)              // 复用 msg.go
  ├ d.textH = 计算显示行数
  ├ contentH := d.textH + 2
  └ TheFloatFrame.Open(spec) → true
  ... 用户读文本 / Tab 切焦点 / 点 OK ...
handleEvent(Enter 或 Space 且 focus==btnOK，或 Button1 命中 OK)
  ├ cb := d.onResult; d.onResult = nil
  ├ TheFloatFrame.Close()
  └ cb(true)
```

Cancel（Esc / 点 Cancel / Enter 焦点在 Cancel）：

```
handleEvent(Esc 或 Enter 且 focus==btnCancel 或 Button1 命中 Cancel)
  ├ cb := d.onResult; d.onResult = nil
  ├ TheFloatFrame.Close()
  └ cb(false)
```

Resize（由容器拦截）：

```
floatFrame.HandleEvent(EventResize)
  ├ f.Close()                 // 清空 onResize
  └ f.onResize()              // 触发 ConfirmDialog 的 onResize
        ├ cb := d.onResult; d.onResult = nil
        └ cb(false)            // 不再 Close（容器已关）
```

## 十、与 MsgDialog 的差异对比

| 维度 | MsgDialog | ConfirmDialog |
|---|---|---|
| 用途 | 只读多行文本展示 | 带二元选择的文本确认 |
| 按钮数 | 1（Close） | 2（OK / Cancel） |
| 焦点状态 | 无（单按钮恒反白） | 有（`focus` 字段，默认 OK） |
| 退出语义 | 所有路径等价（无二元） | 二元：OK=true / Cancel=false |
| Enter | 关闭（无差别） | 激活焦点按钮（默认=OK，结果等价于 MsgDialog） |
| Esc | 关闭（无差别） | 恒取消（false） |
| Tab / 方向键 | 不处理（吞掉） | 切换焦点 + Redraw |
| Space | 关闭 | 激活焦点按钮 |
| 鼠标点击 | 点按钮 → 关闭 | 点 OK → true；点 Cancel → false；点其它 → 忽略 |
| 回调签名 | `func()` | `func(confirmed bool)` |
| 最小内容宽度 | 7（`"[Close]"` 宽 7） | 18（`"[  OK  ]"` + gap2 + `"[Cancel]"` = 8+2+8） |
| Open 失败回调 | `onClose()` | `onResult(false)` |
| Open 参数 | `(text, title, anchor, width, align, maxH, frameColor, onClose)` | 同左，仅末位回调换 `onResult func(bool)` |

共性（不变的部分）：

- 同一 floatFrame 容器、同一生命周期。
- 回调顺序一致：先 `TheFloatFrame.Close()`，再触发回调。
- 失败路径一致：Open 返回 false 时直接回调（MsgDialog 调 `onClose()`，ConfirmDialog 调 `onResult(false)`）。
- 文本区渲染完全一致。
- 纯结构体形态，用完即弃靠 GC。

## 十一、实现要点（复用优先）

1. **`Align` / `softwrap` / `runeWidthInLine`**：`msg.go` 已导出（包级可见），ConfirmDialog 直接复用。
2. **`min`**：`select.go:228` 已有包级函数，直接复用。
3. **`Rect` / `Pos` / `Size` / `FloatOpenSpec` / `TheFloatFrame`**：`frame.go` 已定义，直接用。
4. **文本区绘制**：**阶段 0 已完成**——`drawTextLines` 已在 `msg.go` 抽取，`msg.go` 与 `confirm.go` 共用。
5. **按钮绘制**：**阶段 0 已完成**——`drawButton` 已在 `msg.go` 抽取，MsgDialog 已改调此函数。
6. **反白焦点范式**：沿用 SelectDialog / MsgDialog 的 `config.DefStyle.Reverse(true)`。
7. **Space 键判定**：复用 MsgDialog 的 `ev.Key()==tcell.KeyRune && ev.Rune()==' '` 写法。
8. **鼠标命中范式**：复用 MsgDialog 的 `EventMouse` + `Button1` + 矩形包含判断，扩展为遍历 `btnRects`。
9. **`onResize` 范式**：复用 MsgDialog / InputDialog 的「取回调 / 置 nil / 调回调 / 不重复 Close」四步。
10. **`AutoExpand=true`**：与 MsgDialog / InputDialog 一致。

## 十二、公共函数设计

阶段 0 需抽取两个公共函数，供 `msg.go` 与 `confirm.go` 共用。

### drawTextLines

```go
// drawTextLines 在指定区域绘制多行文本
func drawTextLines(area Rect, lines []string, textH, width int, align Align, style tcell.Style)
```

伪代码：

```go
func drawTextLines(area Rect, lines []string, textH, width int, align Align, style tcell.Style) {
    for vi := 0; vi < textH; vi++ {
        row := area.Y + vi
        line := lines[vi]
        lineWidth := runewidth.StringWidth(line)

        // 按 align 计算起始 X（lineWidth 已用 runewidth.StringWidth 算好，center 对齐直接用）
        var startX int
        switch align {
        case AlignLeft:
            startX = area.X
        case AlignCenter:
            startX = area.X + (width-lineWidth)/2
        case AlignRight:
            startX = area.X + width - lineWidth
        }

        // 先清空整行（底色一致）
        for col := area.X; col < area.X+width; col++ {
            screen.SetContent(col, row, ' ', nil, style)
        }

        // 从 startX 起逐 rune 画文本
        col := startX
        for _, r := range line {
            w := runewidth.RuneWidth(r)
            screen.SetContent(col, row, r, nil, style)
            if w > 1 {
                // 双宽字符后半格写空格占位
                for k := 1; k < w; k++ {
                    screen.SetContent(col+k, row, ' ', nil, style)
                }
            }
            col += w
        }
    }
}
```

### drawButton

```go
// drawButton 在指定矩形绘制单个按钮文案
// 参数：
//   rect  : 按钮命中矩形（X/Y 为左上坐标，W/H 为尺寸，H 恒为 1）
//   text  : 按钮文案（如 "[Close]" / "[  OK  ]" / "[Cancel]"）
//   style : 按钮样式（由调用方根据焦点状态选择：焦点用 Reverse(true)，非焦点用 DefStyle）
func drawButton(rect Rect, text string, style tcell.Style)
```

伪代码：

```go
func drawButton(rect Rect, text string, style tcell.Style) {
    col := rect.X
    row := rect.Y
    for _, r := range text {
        w := runewidth.RuneWidth(r)
        screen.SetContent(col, row, r, nil, style)
        if w > 1 {
            for k := 1; k < w; k++ {
                screen.SetContent(col+k, row, ' ', nil, style)
            }
        }
        col += w
    }
}
```

## 十三、风险与边界

| 风险 | 说明 | 应对 |
|---|---|---|
| width 过小 | `width < 18` 时双按钮组溢出内容区 | 调用方需确保 `width >= 18` |
| maxH 过大 | `textH + 2 > screen.H - infoBarOffset - 1` 时 floatFrame.Open 返回 false | 调用方需确保 maxH 合理 |
| 默认焦点误触 | 默认焦点 OK，用户直接按 Enter 会确认 | 这是标准对话框行为；若需要默认 Cancel，未来加参数 |
| Esc 与焦点冲突 | Tab 到 OK 后按 Esc，预期取消还是确认 OK？ | Esc 恒取消（无论焦点），标准语义 |
| Tab 键被终端截获 | 部分终端把 Tab 转成 `\t` | tcell 已归一化；同时支持 ←/→ 切焦点作为后备 |
| 鼠标点两按钮间隙 | gap 区域（2 列）不属于任何按钮 | 命中判断按矩形严格包含，间隙不触发 |
| 回调里再 Open 浮窗 | onResult 触发时 floatFrame 已 Close | 安全，已验证顺序（先 Close 后回调） |

## 十四、未来考虑

首版聚焦「展示文本 + OK/Cancel 二选一」核心功能，未来可按需增强：

| 功能 | 首版 | 未来考虑 |
|---|---|---|
| 按钮文案自定义 | 硬编码 `"[  OK  ]"` / `"[Cancel]"` | 加 `okLabel` / `cancelLabel` 入参 |
| 默认焦点可选 | 默认 OK | 加 `defaultFocus int` 入参 |
| 三按钮（Yes/No/Cancel） | 不支持 | 需改动焦点切换逻辑和按钮布局 |
| 文本可选择/复制 | 不支持 | 终端层鼠标选择由终端模拟器处理 |

## 十五、实现步骤

**阶段 0：抽取公共函数（前置工作，必须完成）**

0.1 在 `msg.go` 抽取两个公共函数（与 `softwrap` 同文件）：
    - `drawTextLines(area Rect, lines []string, textH, width int, align Align, style tcell.Style)`
    - `drawButton(rect Rect, text string, style tcell.Style)`

0.2 改造 `msg.go` 的 `display` 方法：
    - 文本段改为调用 `drawTextLines(area, d.lines, d.textH, d.width, d.align, style)`
    - 按钮段改为调用 `drawButton(d.btnRect, "[Close]", revStyle)`
    - 按钮文案从 `" Close "`（反白底色）改为 `"[Close]"`（方括号样式，7字符宽）

0.3 验证 MsgDialog 功能不变：
    - 多行文本渲染正确（含对齐、CJK 双宽、换行）
    - 按钮显示为 `[Close]`（方括号样式，7字符宽）且点击正确
    - 所有交互路径（Enter/Space/Esc/鼠标）正常

**阶段 1：实现 ConfirmDialog**

1. 新建 `internal/dialog/confirm.go`，`package dialog`。

2. 写按钮索引常量 `btnOK=0` / `btnCancel=1` 与 `ConfirmDialog` 结构体 + `NewConfirmDialog()`。

3. 写 `Open(...)`：
    - `d.focus=btnOK`
    - `d.lines=softwrap(text,width)`（复用 msg.go）
    - 按 §6 算 `textH/contentH`
    - 构造 `FloatOpenSpec`（`AutoExpand:true`、`OnResize:d.onResize`）
    - 调 `TheFloatFrame.Open`，失败即清理 + `onResult(false)`

4. 写 `d.display(area)`：
    - 调 `drawTextLines(area, d.lines, d.textH, d.width, d.align, style)` 画文本区
    - 清空空行
    - 按 §6.3 算 `okX/cancelX/groupW`，写 `btnRects`
    - 按 `focus` 选样式，分别调 `drawButton` 画两按钮

5. 写 `d.handleEvent`：
    - 键盘 switch：Tab/←/→ → `d.focus=1-d.focus` + Redraw；Enter → `d.close(d.focus==btnOK)`；Esc → `d.close(false)`；Space → `d.close(d.focus==btnOK)`
    - 鼠标分支：Button1 遍历 `btnRects`，命中则 `d.close(i==btnOK)`

6. 写 `d.close(confirmed bool)`：取 `onResult`、置 nil、`TheFloatFrame.Close()`、调 `onResult(confirmed)`。

7. 写 `d.onResize()`：取 `onResult`、置 nil、调 `onResult(false)`（不再 Close）。

8. `make build-quick` 编译通过。

**阶段 2：验证**

9. 手动构造调用点（临时命令或 finder 删除路径）验证：
    - OK 路径：点 OK / Enter（默认焦点） / Space → `true`
    - Cancel 路径：点 Cancel / Esc / Tab+Enter → `false`
    - 焦点切换：Tab / ← / → 切换，高亮正确
    - Esc 恒取消：Tab 到 OK 后按 Esc → 仍 `false`
    - 鼠标命中：点 OK 区 → true；点 Cancel 区 → false；点其它 → 忽略
    - Open 失败：先开浮窗再 Open ConfirmDialog → 直接 `onResult(false)`
    - 多行渲染：文本区 + 空行 + 双按钮行布局正确
    - 最小宽度：`width=18` 时按钮刚好放下
    - CJK 双宽：含中文的行宽度正确
    - 长行换行：softwrap 正确
    - align 对齐：Left/Center/Right 三种正确
    - maxH 限制：设 maxH=5，传 8 行 → 只显示前 5 行
    - 光标隐藏：浮窗期间无可见光标

10. 移除临时调用点，提交。

11. 重新验证 MsgDialog（确认阶段 0 未引入回归）。
