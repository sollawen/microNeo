# C0：pane 关闭存盘提示改用 ConfirmDialog — 实施计划

## 1. 背景与目标

现状：

- pane 关闭时若 buffer 有未存盘修改，走 `InfoBar.YNPrompt` 在底部 statusLine 显示「Save changes to X before closing? (y,n,esc)」
- 触发关闭的入口：
    - Ctrl-q / :quit
    - :open 换文件
    - 开 shell
- 提示在屏幕最底部一行，与文件内容远离，不显眼

目标：

- 用 ConfirmDialog（KindYesNo 模式）浮窗替代底部提示
- 弹窗水平居中于当前 pane，上边缘落在 pane 垂直中线附近（垂直方向不追求严格居中）
- 弹窗打不开时退回 micro 原装 InfoBar 提示，行为与现状一致
- 三态语义保持，具体为：
    - y 存盘后关闭
    - n 不存盘直接关闭
    - esc 取消不关闭

## 2. 可行性结论

可行，整体机制全部现成：模态事件路由、边框绘制、生命周期、resize 拦截、失败预检都在 `internal/dialog` 里，主循环（cmd/micro/micro.go）已按 FloatFrame 优先路由事件，无需改动。

有两处小扩展必须做，现状机制不够用：

- KindYesNo 回调只有 confirmed bool，n 和 Esc 无法区分 → 新增三值结果类型 ConfirmResult（Yes/No/Esc），含义由调用方决定
- ConfirmDialog.Open 失败时只回调不返回结果，closePrompt 无法判断要不要退回 InfoBar → Open 改为返回 bool，失败时不再触发回调

定位零改动：anchor + width 本来就是调用方传入的参数，closePrompt 直接算 pane 中心当锚点即可，FloatFrame 不用动。

## 3. 现状梳理

### 3.1 触发入口

closePrompt（internal/action/actions.go:1916）是 pane 关闭存盘询问的唯一漏斗，各入口共用：

- Quit（actions.go:1935）：
    - 关闭 pane
    - 入口：Ctrl-q / :quit / ToggleLogCmd
- OpenCmd（command.go:315）：:open 换文件
- TermCmd（command.go:1194）：:term 开 shell，内部调 openTerm
- Ctrl-q 链路：
    - 文件型 pane：Ctrl-q → QuitNeo → h.Quit() → closePrompt
    - noName pane：走 finder（见 §5）

### 3.2 弹窗位置来源

- `h.GetView()` 返回 *View{X, Y, Width, Height}，是 pane 的屏坐标矩形
- tab.go 的 Tab.Resize 里 `pv.X, pv.Y = n.X+offset, n.Y`，位置实时跟随布局
- pane 中心 = (X + Width/2, Y + Height/2)

### 3.3 弹窗现状能力

FloatFrame 已有的能力：

- 锚点展开（AutoExpand，向右下优先 + 兜底 + 边界 clamp）
- 屏内预检（放不下直接拒绝并回调）
- 模态事件路由（IsOpen 时主循环直喂）
- resize 拦截（先 Close 再调 OnResize）
- 边框绘制 + title 嵌入

ConfirmDialog 已有的能力：

- KindOkCancel 双按钮模式（[OK]/[Cancel]，Tab/方向键切焦点）
- KindYesNo 单键模式（y/n/Esc，hint " [y/n/esc]" 自动拼到文本末尾）
- KindYesNo 现有唯一调用方：finder/delete.go

### 3.4 三态缺口

- KindYesNo 的 handleEventYesNo 把 Esc 和 n 都映射到 close(false)
- 回调只有 confirmed bool，区分不出「n=不存盘关闭」和「esc=取消」
- pane 关闭语义必须三态，这是本计划必须补的能力

## 4. 方案设计

### 4.1 定位：调用方算锚点，定位 API 零改动

- ConfirmDialog.Open 的 anchor + width 参数本来就是调用方传入，本方案不加任何定位 API
- closePrompt 从 pane 中心算锚点：anchor = Pos{X: view.X + view.Width/2 - (width+2)/2, Y: view.Y + view.Height/2}
- X 补偿用外框宽 width+2（左右边框各 1 列），水平居中精确到 1 列
- width = view.Width - 4，上限 40，下限 12
- 垂直方向不追求居中：文字通常两三行，上边缘落在 pane 垂直中线附近，由 softwrap 自然决定高度
- AutoExpand 保持 true：
    - 常见情况左上角 = 锚点
    - pane 靠边放不下时 expandAnchor 自动转向
    - 自带边界 clamp

### 4.2 ConfirmDialog 返回三值结果

- 新增导出类型 ConfirmResult，三值 ConfirmYes / ConfirmNo / ConfirmEsc
- KindYesNo 的 onResult 签名从 func(bool) 改为 func(ConfirmResult)

键位映射：
- y → ConfirmYes
- n → ConfirmNo
- Esc → ConfirmEsc

- onResize → ConfirmEsc
- Open 失败：不再触发回调，改为返回 false，由调用方决定兜底
- 语义下沉给调用方：dialog 只报「按了哪个键」，yes/no/esc 各自代表什么由调用方解释

KindOkCancel 顺带统一（当前无调用方，无兼容负担）：
- OK → ConfirmYes
- Cancel → ConfirmEsc
- Esc → ConfirmEsc

Cancel 与 Esc 语义等价：都是「不继续」；ConfirmNo 只留给 KindYesNo 的 n 键。

改动的伪代码：

```go
// 新增类型：三值结果，含义由调用方解释
type ConfirmResult int

const (
	ConfirmYes ConfirmResult = iota
	ConfirmNo
	ConfirmEsc
)
```

```go
// Open 签名：onResult 改 func(ConfirmResult)，同时返回 bool
// 失败时不再触发 onResult，由调用方根据返回值决定兜底
// 失败分支保留状态清理，防止悬挂回调
func (d *ConfirmDialog) Open(...) bool {
	...
	if !TheFloatFrame.Open(spec) {
		d.lines = nil
		d.onResult = nil
		return false
	}
	return true
}

// close 携带结果，替代原 close(bool)
func (d *ConfirmDialog) close(result ConfirmResult) {
	cb := d.onResult
	d.onResult = nil
	TheFloatFrame.Close()
	if cb != nil {
		cb(result)
	}
}

// handleEventYesNo 的键位映射（原 y→true、n 和 Esc 都→false，两条 false 出口拆开）
case Esc      → close(ConfirmEsc)
case 'y', 'Y' → close(ConfirmYes)
case 'n', 'N' → close(ConfirmNo)

// handleEventOkCancel：OK→ConfirmYes；Cancel 按钮 / Esc→ConfirmEsc
// onResize：onResult(ConfirmEsc)
```

### 4.3 closePrompt 改写

- 文本：Save changes to <name> before closing?（hint 由 dialog 自动拼上）
- title：复用 action 参数（"Quit" / "Save"）
- 锚点：pane 中心偏移半个外框宽（view.X + view.Width/2 - (width+2)/2, view.Y + view.Height/2）
- width：view.Width - 4，上限 40，下限 12

回调分发（三值显式 switch，比两个 bool 更直白）：
- ConfirmYes → h.SaveCB(action, callback)
- ConfirmNo → callback()
- ConfirmEsc → 什么都不做

弹窗打不开时的兜底：
- Open 返回 false（屏幕放不下 / 已有弹窗）→ 退回 micro 原装 InfoBar.YNPrompt，行为与现状一致
- 原装代码保留为活代码（兜底分支），不是注释掉的死代码

InfoBar.YNPrompt 保留不动，其它站点仍用它

改动的伪代码：

```go
func (h *BufPane) closePrompt(action string, callback func()) {
	// 定位：pane 中心偏移半个外框宽，高度不参与
	view := h.GetView()
	width := view.Width - 4
	if width > 40 {
		width = 40
	}
	if width < 12 {
		width = 12
	}
	anchor := dialog.Pos{
		X: view.X + view.Width/2 - (width+2)/2,
		Y: view.Y + view.Height/2,
	}

	dlg := dialog.NewConfirmDialog()
	if !dlg.Open(
		"Save changes to "+h.Buf.GetName()+" before closing?",
		action,            // title 复用 action（"Quit" / "Save"）
		anchor,
		width,
		dialog.AlignLeft, // YesNo 强制左对齐，hint 由 Open 自动拼到 text 末尾
		0,                // maxH 不限，文字通常两三行
		config.DefStyle,
		dialog.KindYesNo,
		dialog.FocusOK,   // YesNo 模式忽略
		func(result dialog.ConfirmResult) {
			switch result {
			case dialog.ConfirmYes:
				h.SaveCB(action, callback) // 先存盘再关闭
			case dialog.ConfirmNo:
				callback()                 // 不存盘直接关闭
			case dialog.ConfirmEsc:
				// 什么都不做，保持打开
			}
		},
	) {
		// 兜底：弹窗打不开（屏幕放不下 / 已有弹窗），退回 micro 原装 InfoBar 提示
		InfoBar.YNPrompt("Save changes to "+h.Buf.GetName()+" before closing? (y,n,esc)",
			func(yes, canceled bool) {
				if !canceled && !yes {
					callback()
				} else if !canceled && yes {
					h.SaveCB(action, callback)
				}
			})
	}
}
```

### 4.4 finder/delete.go 适配

- 回调签名改 func(ConfirmResult)，只在 result == ConfirmYes 时执行删除
- Open 改返回 bool，失败时不再触发回调 → 检查返回值，失败直接 return

改动的伪代码：

```go
// 改前：confirmed bool，false 混着 n 和 Esc；失败由 onResult(false) 静默处理
func(confirmed bool) {
	if !confirmed {
		return
	}
	// 删除逻辑
}

// 改后：只有 Yes 才删除；Open 失败返回 false，直接 return
if !dlg.Open(message, "Delete", anchor, l.pickerW-2, dialog.AlignCenter, 0,
	config.DefStyle, dialog.KindYesNo, dialog.FocusOK,
	func(result dialog.ConfirmResult) {
		if result != dialog.ConfirmYes {
			return
		}
		// 删除逻辑不变
	}) {
	return
}
```

## 5. 边界情况

弹窗打开失败（屏幕放不下 / 已有弹窗）：

- Open 返回 false → closePrompt 退回 micro 原装 InfoBar.YNPrompt，行为与现状一致
- finder 删除确认直接 return，等同取消

resize 打断弹窗：

- FloatFrame 拦截 resize，先 Close 再调 OnResize → ConfirmEsc → pane 不关闭
- 提示随之消失，需重新按 Ctrl-q（用户已接受）

pane 过窄：

- 弹窗宽度跟随 pane 收窄（下限 12），正常情况不超过 pane 宽度
- 极端窄 pane 下 expandAnchor 自动转向到屏幕内，功能不受影响

SaveCB 的次级提示仍走 InfoBar，本次不动：
- SaveAs 文件名
- 覆盖确认
- sudo 确认

noName pane：

- Ctrl-q 走 OpenFinder 路由，不经 Quit，不受本次改动影响

## 6. 验收

- 修改 buffer 后 Ctrl-q，弹窗水平居中于 pane，上边缘落在 pane 垂直中线附近

按场景验证：
- y 存盘并关闭
- n 不存盘关闭
- esc 弹窗消失 pane 不关
- 分屏下弹窗在对应 pane 水平居中
- 弹窗打不开（屏幕过小）时退回 InfoBar 提示，行为与现状一致
- finder 删除文件确认仍正常
- make build 通过

## 7. 文件清单

internal/dialog/confirm.go 改动：
- 新增 ConfirmResult 三值类型
- Open 的 onResult 签名改 func(ConfirmResult)，并返回 bool
- handleEventYesNo / handleEventOkCancel 键位映射
- Open 失败分支保留状态清理（d.lines = nil、d.onResult = nil）
- Open 的 doc 注释同步：onResult 描述改三值、返回值改 bool
- 顺带修正锚点注释（「展开中心」→「展开铰点」）：
    - confirm.go:64
    - msg.go:155
    - frame.go:49

internal/action/actions.go 改动：
- closePrompt 改写：弹窗优先，打开失败退回 InfoBar 兜底
- 新增 internal/dialog import

internal/finder/delete.go 改动：
- 回调签名适配 func(ConfirmResult)
- 检查 Open 返回值，失败直接 return
