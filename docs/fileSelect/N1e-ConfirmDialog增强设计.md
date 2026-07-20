# ConfirmDialog 增强设计方案

## 一、目标

N1d 落地的 ConfirmDialog 把交互硬编码为「双按钮 + 默认焦点 OK」，delete 等破坏性操作也走同一条路径，缺乏强制显式按键的安全摩擦。

本次增强把交互形态参数化为两种模式（**是模式切换，不是按钮文案切换**）：

1. **OkCancel 模式**（`KindOkCancel`，默认）：两按钮 `[  OK  ]` / `[Cancel]`，行为等同 N1d。可选 `FocusNone` 做弱安全（Enter / Space 无效，必须先 Tab / 点按钮）。
2. **YesNo 模式**（`KindYesNo`）：单键确认，不画按钮，在 `Open` 阶段把 hint `" [y/n/esc]"` 拼到 text 末尾，跟 text 一起 `softwrap`——短行末尾自然拼接、长行自动换行。强制左对齐（`align` 参数被忽略）。用户必须键入 `y` / `n` / `Esc`。无焦点、无 Enter / Space / Tab / 方向键 / 鼠标点击。

回调签名不变：`func(confirmed bool)`。OkCancel 左 = true / 右 = false；YesNo `y` = true / `n` / Esc = false。

## 二、API

### 类型

```go
type Kind int

const (
	KindOkCancel Kind = iota // 两按钮模式（默认，等同 N1d）
	KindYesNo                // 单键模式（y/n/Esc），无按钮无焦点
)

type Focus int

const (
	FocusOK     Focus = iota // 默认焦点 = 确认位（左按钮）—— 等同 N1d
	FocusCancel              // 默认焦点 = 取消位（右按钮）
	FocusNone                // 无焦点，两按钮初始都不高亮；仅 OkCancel 生效，YesNo 忽略
)

const (
	btnOK = iota // 按钮渲染索引
	btnCancel
)

const yesNoHint = " [y/n/esc]"
```

`Focus` 仅 OkCancel 模式有意义。YesNo 模式下被忽略（不引入 `FocusN/A` 哨值、不报错、不重载签名）——零值 `FocusOK` 在 YesNo 下被忽略无副作用。

### Open 签名

```go
func (d *ConfirmDialog) Open(
	text         string,
	title        string,
	anchor       Pos,
	width        int,            // 内容区宽度（不含边框）；OkCancel 需 >= 18
	align        Align,
	maxH         int,
	frameColor   tcell.Style,
	kind         Kind,           // 新增：交互模式
	defaultFocus Focus,          // 新增：OkCancel 初始焦点（YesNo 忽略）
	onResult     func(confirmed bool),
)
```

破坏性更新，全仓仅 `internal/finder/delete.go:42` 一个调用点。

### 零值兼容

| 调用 | 行为 |
|---|---|
| `KindOkCancel, FocusOK` | 与 N1d 完全一致 |
| `KindOkCancel, FocusCancel` | 两按钮，默认焦点 Cancel，Enter = 取消 |
| `KindOkCancel, FocusNone` | 两按钮初始都不反白，Enter / Space 无效 |
| `KindYesNo, *` | 单键确认，hint 拼到 text 末尾跟 text 一起走；强制左对齐；Focus 被忽略 |

## 三、OkCancel 模式

行为几乎等同 N1d，唯一增量为 `FocusNone` 守卫（写在 `KeyEnter` / `KeyRune(' ')` 两个 case **内部**，不是 handleEvent 顶层的统一守卫点）——漏判会让 `FocusNone` 失效（`FocusNone != FocusOK` 会算成 `close(false)`）。

### 行为表

| `focus` | Tab / ← / → | Enter / Space | Esc | 鼠标点按钮 |
|---|---|---|---|---|
| `FocusOK` | cycle → Cancel | `close(true)` | `close(false)` | 命中即激活 |
| `FocusCancel` | cycle → OK | `close(false)` | `close(false)` | 命中即激活 |
| `FocusNone` | cycle → Cancel | **no-op** | `close(false)` | 命中即激活 |

`FocusNone` 仅作初始态，首次 Tab / ← / → / 鼠标 engage 后进入 `FocusOK ↔ FocusCancel` 二态循环，不再回 None（摩擦一次性）。Esc 三态都 `close(false)`（标准取消语义不锁死）。鼠标点击绕过焦点状态，点哪个激活哪个。

```go
// 在 KeyTab / KeyBacktab / KeyLeft / KeyRight 这四个 case 里调用
func (d *ConfirmDialog) cycleFocus() {
	if d.focus == FocusCancel {
		d.focus = FocusOK
	} else { // FocusOK 或 FocusNone 都翻到 FocusCancel
		d.focus = FocusCancel
	}
	screen.Redraw()
}
```

`FocusNone` 首次 Tab 落点选 `FocusCancel`——避免「首次 Tab → 直接 Enter 即确认」与安全初衷更贴合。

## 四、YesNo 模式

**核心简化**：hint 在 `Open` 阶段拼到 text 末尾，跟 text 一起 `softwrap`。短行末尾自然拼接，长行自动换到下一行——**无任何坐标计算、无 Reverse、无溢出降级分支**。

```go
// Open 阶段
if kind == KindYesNo {
    align = AlignLeft   // 强制左对齐；align 参数被忽略
    text += yesNoHint
}
d.lines = softwrap(text, width)
// ...
// contentH = textH（hint 已在 text 里，不需要预留额外行）
```

`display` 函数是两种模式共用：

```go
func (d *ConfirmDialog) display(contentArea Rect) {
	d.lastArea = contentArea
	drawTextLines(contentArea, d.lines, d.textH, d.width, d.align, config.DefStyle)
	if d.kind == KindOkCancel {
		d.drawButtons(contentArea)
	}
}
```

YesNo 模式下 `drawButtons` 不调用，渲染路径只剩一行 `drawTextLines`。hint 与 text 同色（DefStyle）、同行或自然换行——无任何样式差异，无任何视觉突出。

事件处理极简：

```go
func (d *ConfirmDialog) handleEventYesNo(event tcell.Event) {
	ev, ok := event.(*tcell.EventKey)
	if !ok {
		return // 鼠标 / Resize 一律吞
	}
	switch ev.Key() {
	case tcell.KeyEscape:
		d.close(false)
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'y', 'Y':
			d.close(true)
		case 'n', 'N':
			d.close(false)
		}
	}
}
```

## 五、调用点迁移

`internal/finder/delete.go:42` 改为：

```go
dlg.Open(
	message,
	"Delete",
	anchor,
	fm.state.pickerW-2,
	dialog.AlignCenter,
	0,
	config.DefStyle,
	dialog.KindYesNo,    // delete 是破坏性操作，切单键确认
	dialog.FocusOK,      // YesNo 下被忽略；传零值表达「不关心焦点」
	func(confirmed bool) { /* 删除逻辑 */ },
)
```

为何切 YesNo 而非 OkCancel + FocusNone：YesNo 比 FocusNone 更安全——FocusNone 仍允许鼠标单击直接确认，YesNo 连鼠标都吞掉，必须键入 `y`。

## 六、风险

| 风险 | 应对 |
|---|---|
| **FocusNone 下 Enter 误关闭**：`FocusNone != FocusOK` 会被算成 `close(false)` | Enter / Space 都加 `if d.focus == FocusNone { return }` 守卫在 `close` 之前；code review 必查 |
| **YesNo 多键连击**：用户长按 y 连续触发 EventKey | 首次 `close()` 后 floatFrame 立即 Close 并置 `handleEvent=nil`，后续事件不转发 |
| **破坏性 API 更新**：全仓 1 个调用点未同步则编译失败 | 期望行为，编译期强制迁移 |
| **YesNo IME 拦截**：开中文输入法时键 y 可能被 IME 截获 | tcell 在 IME commit 前不产生 KeyRune；极端情况切回英文 |

回调里再 Open 浮窗：onResult 触发时 floatFrame 已 Close，安全，与 N1d 一致。

## 七、实现步骤

**阶段 0：类型定义**（零行为影响）

0.1 在 `confirm.go` 顶部新增 `Kind` / `Focus` 类型与常量；新增 `kind Kind` 字段；`focus` 字段类型 `int` → `Focus`；保留 `btnOK` / `btnCancel` 用于按钮索引；新增 `yesNoHint` 常量。
0.2 `make build-quick` 通过。

**阶段 1：`Open` 签名扩展 + 零回归迁移**

1.1 `Open` 加 `kind` + `defaultFocus` 两参（`frameColor` 后 / `onResult` 前）。
1.2 `Open` 内 `d.kind = kind`、`d.focus = defaultFocus`；`contentH` 按 kind 分叉——OkCancel = `textH + 2`（与 N1d 一致），YesNo = `textH`（hint 已含在 text 里）。
1.3 delete.go 调用点先补 `KindOkCancel, FocusOK`（零回归）。
1.4 `make build-quick` 通过；运行确认 delete 弹窗与 N1d 一致。

**阶段 2：OkCancel FocusNone 支持**

2.1 `display` 拆出 `drawButtons`（空行 + 双按钮 + 命中缓存）；焦点样式用 `okStyle / cancelStyle` 变量合并三态。
2.2 `handleEvent` 的 `KeyTab` / `KeyBacktab` / `KeyLeft` / `KeyRight` 四个 case 合并为调 `cycleFocus()`；`KeyEnter` / `KeyRune(' ')` 这两个 case 内加 `if d.focus == FocusNone { return }`。
2.3 `make build-quick` 通过；构造临时 `KindOkCancel, FocusNone` 验证。

**阶段 3：YesNo 模式实现**

3.1 `Open` 内 YesNo 分支 `align = AlignLeft`（强制左对齐）+ `text += yesNoHint`，然后正常 `softwrap`。
3.2 `display` 在 `drawTextLines` 之后按 `d.kind` 决定是否调 `drawButtons`（YesNo 不调）。
3.3 `handleEvent` 按 `d.kind` 分叉到 `handleEventOkCancel` / `handleEventYesNo`。
3.4 `make build-quick` 通过；构造临时 `KindYesNo` 验证。

**阶段 4：delete 切 YesNo**

4.1 delete.go 改为 `KindYesNo, FocusOK`。
4.2 实机验证：弹出 `text [y/n/esc]`，`y` 删除 / `n` `Esc` 取消，Enter 与鼠标点击无反应。

**阶段 5：验证清单**

OkCancel + FocusOK 零回归：渲染、Tab / Enter / Space / Esc / 鼠标全部与 N1d 一致；Open 失败 / Resize 路径正确；CJK / softwrap / align / maxH 正常。

OkCancel + FocusNone：初始两按钮都不反白；Enter / Space no-op；Tab / 鼠标 engage 后正常激活；Esc 仍 `close(false)`。

YesNo：hint ` [y/n/esc]` 拼到 text 末尾（同行），强制左对齐；多行时接最后一行；text 超长时 hint 跟 text 一起 softwrap 换到下一行（自然行为，无特殊处理）；`y` / `Y` true，`n` / `N` / `Esc` false；Enter / Space / Tab / 方向键 / 鼠标 全无反应；CapsLock 时 `Y` / `N` 仍生效。

公共函数（`drawTextLines` / `drawButton` / `softwrap` / `runeWidthInLine` / `min`）一律不改。MsgDialog / InputDialog / SelectDialog 一律不动。