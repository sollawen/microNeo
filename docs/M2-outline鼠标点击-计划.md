# M2：outline 弹窗支持鼠标点击 — 实施计划

**性质**：实施计划。需求一句话可讲清（Alt-o 打开的目录弹窗，现在只能靠 ↑/↓ 移动，要能用鼠标点），事件链路与几何换算已逐行核实，方案落在单个文件内，可直接照着改。

**接续**：outline 弹窗已上线，条目样式为 `◇` 前缀 + 层级缩进。M2 只加鼠标支持，弹窗几何与条目格式全部不动。

## 1. 目标

Alt-o 打开的 MD 目录弹窗（`:outline`）支持鼠标交互：

- 点击某一条标题 → 只把选中高亮移到该条，弹窗保持打开
- 鼠标滚轮 → 上下移动弹窗内的选中高亮
- 确认跳转仍走 Enter，语义与改前完全一致
- 键盘路径保持原样，不受影响：
  - ↑/↓ 移动选中
  - Enter 跳转
  - Esc 关闭

不做的事见 §6。

## 2. 现状机制

### 2.1 入口与调用链

| 环节 | 位置 | 说明 |
|---|---|---|
| 键位 | `internal/action/defaults_darwin.go:40`、`internal/action/defaults_other.go:43` | `Alt-o` → `command:outline` |
| 命令注册 | `internal/action/command_neo.go:29` | `MakeCommand("outline", (*BufPane).OutlineCmd, nil)` |
| action | `internal/action/outline_neo.go:17` | `OutlineCmd` 算几何、收集标题、弹窗 |
| 弹窗 | `internal/dialog/outline.go:15` | `OutlineDialog` 只渲染 + 上下选择 |
| 条目来源 | `internal/md/collect_headings.go:22` | `CollectHeadings` 生成等宽条目串，串尾带 1 基行号 |

`OutlineCmd` 传给弹窗的是「锚点 + 尺寸 + 条目串」，行号信息编码在条目串尾部，由 `internal/action/outline_neo.go:72` 的 `parseTrailingInt` 反解。弹窗层不需要知道行号是什么。

### 2.2 事件链路（已核实，无需改动）

主循环 `cmd/micro/micro.go:585-586`：

```go
} else if dialog.TheFloatFrame.IsOpen() {
    dialog.TheFloatFrame.HandleEvent(event)
} else if action.TheNotePane != nil && action.TheNotePane.IsOpen() {
```

关键事实：这是 `else if` 链。浮窗打开期间，**所有**事件（含鼠标）只进浮窗，`action.Tabs.HandleEvent` 不会被调用。

由此得到三个结论：

- 鼠标事件已经能到达弹窗，路由层零改动
- 弹窗是模态的，点不到底下的正文，不会误挪编辑器光标
- 点击不关窗，press 与随后的 release 都被浮窗吞掉，全程不会漏到 pane

浮窗内部再转发一次：`internal/dialog/frame.go:243` 的 `FloatFrame.HandleEvent` 只拦截 resize，其余原样丢给具体弹窗的 `handleEvent`。

鼠标事件的产生前提是 tcell 开了鼠标上报：`internal/screen/screen.go:231` 依据全局选项 `mouse`（默认 true，`internal/config/settings.go:126`）调 `Screen.EnableMouse()`。

### 2.3 几何换算（已核实）

`FloatFrame.Open`（`internal/dialog/frame.go:92`）里 `AutoExpand=false`，所以 `fx,fy` 就是调用方给的锚点；内容区在 `internal/dialog/frame.go:218` 算出：

```go
f.display(Rect{X: f.fx + 1, Y: f.fy + 1, W: f.contentSize.W, H: f.contentSize.H})
```

内容区 = 外框左上角 + (1,1)，宽高就是调用方给的纯内容尺寸。

`OutlineDialog.display`（`internal/dialog/outline.go:89`）把 `topIdx+vi` 这条画在屏幕行 `area.Y + vi`，可见条数 `visibleH = min(len(items), size.H)`。

所以屏幕行 → 条目下标的映射是纯减法：

```
vi = my - area.Y            // 视口内行号
idx = topIdx + vi           // 绝对条目下标
```

命中条件三条全要满足：x 落在内容区横向范围，y 落在可见行范围，越界一律不命中。

### 2.4 现成的同类实现（可直接抄）

finder 的 history 列表已经有一模一样的「行号 → 下标」映射和左键处理：

- `internal/finder/history.go:183` `HandleMouse`：`Button1` → 移动选中，`WheelUp/WheelDown` → 上下移
- `internal/finder/history.go:207` `rowAt`：把屏坐标行号映射成列表下标，越界返回 false
- `internal/finder/filelist.go:414`、`internal/finder/filelist.go:438`：文件列表的同款实现

命名与结构照抄这两处，microNeo 内部保持一致。

## 3. 方案

### 3.1 行为表

| 操作 | 改前 | 改后 |
|---|---|---|
| ↑ / ↓ | 移动选中高亮 | 不变 |
| Enter | 跳到选中标题 | 不变 |
| Esc | 关闭 | 不变 |
| 左键点条目 | 无反应 | 选中高亮移到该条，弹窗不关 |
| 左键点条目下方的空白区 | 无反应 | 不变（无反应） |
| 左键点右侧 ▲▼ 指示器列 | 无反应 | 不变（不算命中该行） |
| 左键点边框 | 无反应 | 不变（无反应） |
| 滚轮上 / 下（指针在弹窗内容区内） | 无反应 | 上下移动选中高亮 |
| 滚轮上 / 下（指针在内容区外） | 无反应 | 不变（不生效） |

### 3.2 关键设计决定

**点击语义 = 只选中，不跳转**

点击只把选中高亮移到被点的那一条，弹窗保持打开。确认跳转由用户自己按 Enter 决定，与键盘路径完全同源。

这条与 finder 的交互一致（`internal/finder/filelist.go:414` 的左键点击只移动列表光标，激活仍走 Enter）。

好处：

- 点击不关窗，误点不会打断当前浏览，可以继续点别的条目对比
- 「选中」与「跳转」两件事解耦，鼠标与键盘共用同一个选中态，行为没有分叉
- 关窗就会触发 `onSelect` 回调直接跳转，与「点击 = 只选中」的语义冲突

代价：鼠标用户比键盘用户多按一次 Enter。这是刻意的选择——把「看一眼」和「跳过去」分开，避免误触直接跳走。

**点击后必须 `screen.Redraw()`**

选中高亮是反色渲染的（`internal/dialog/outline.go:90` 的 `revStyle`），改 `selected` 后不重绘，高亮不会动。

**命中区域 = 条目文本区，比内容区小一圈**

内容区不是全部可点。实际可点的只有条目文本占的那部分：

- **纵向**切掉下方。弹窗高度默认等于 pane 高度（`internal/action/outline_neo.go:34`，并受 `:36` 的屏幕高度夹取），标题少时条目下方是空白区，属于死区。
- **横向**只切掉右边一列。最后一列固定留给 ▲▼ 指示器（`internal/dialog/outline.go:103` 的 `maxX := area.X + area.W - 1`），条目文本永远不画到那里。左边一列完整可点。

点空白区或指示器列一律不命中，弹窗不关、高亮不动。理由：

- 「点空白关窗」会变成隐式取消，用户想点某行但偏一格就把弹窗关了，不可接受
- 指示器列是滚动提示不是条目，点击它却选中某行会让人困惑
- 与 finder 的**纵向**判定一致：`internal/finder/history.go:207` 的 `rowAt` 同样只认可见行
- 横向与 finder 不同：finder 的指示器列算命中（`internal/finder/filelist.go:438` 的 `listRowAt` 只判 `my` 不判 `mx`），outline 明确排除。这是有意分歧

**选中高亮的取值来自 `topIdx` 视口偏移**

条目可能多于可见行数，屏幕行只是视口内位置，必须加 `topIdx` 才是绝对下标。这一条抄 `internal/finder/history.go:207` 的做法。

**不复制边框宽度知识**

内容区原点是「外框左上角 + (1,1)」，这个 +1 是 `FloatFrame` 的内部约定。若在 `OutlineDialog.Open` 里自己按锚点 +1 算，等于把框架约定抄一份到业务层，框架哪天改边框宽度就会静默错位。改成在 `display(area Rect)` 里把 `area` 存下来，命中判定用的就是绘制时同一个矩形，天然不可能错位。

## 4. 改动清单

### 4.1 核心改动：`internal/dialog/outline.go`

**改动 1：结构体加一个字段**

```go
type OutlineDialog struct {
	items     []string
	selected  int    // 绝对选中下标（0..len-1）
	topIdx    int    // 视口顶部下标
	size      Size   // 纯内容区尺寸（不含边框）
	title     string
	itemStyle tcell.Style
	onSelect  func(*string)
	area      Rect // 最近一次绘制的内容区（屏幕坐标），鼠标命中判定用
}
```

新增行会让 `gofmt` 重排整个结构体的注释对齐列（多出的 `area Rect` 让对齐列左移 2 空格），diff 里会看到三行旧注释跟着移位，属正常噪音。

**改动 2：`display` 里记下内容区**

在 `internal/dialog/outline.go:89` 的 `display(area Rect)` 开头加一行 `o.area = area`。

时序是安全的：主循环每轮先绘制再取事件（`cmd/micro/micro.go` 的 `DoEvent`），`FloatFrame.Open` 触发的重绘在下一次取事件前完成，所以任何鼠标事件到达时 `area` 都已是最新值。

**改动 3：新增命中判定函数**

```go
// rowAt 把屏幕坐标映射成条目下标；落在条目区外返回 false。
func (o *OutlineDialog) rowAt(mx, my int) (idx int, ok bool) {
	if o.area.W == 0 {
		return 0, false // 还没绘制过，无有效内容区
	}
	// 横向：条目文本只占前 W-1 列，最后一列是 ▲▼ 指示器列
	if mx < o.area.X || mx >= o.area.X+o.area.W-1 {
		return 0, false
	}
	// 纵向：条目只占前 visibleH 行，下方是空白区
	visibleH := min(len(o.items), o.size.H)
	if my < o.area.Y || my >= o.area.Y+visibleH {
		return 0, false
	}
	return o.topIdx + (my - o.area.Y), true
}
```

**改动 4：抽出选中移动，键鼠共用**

```go
// moveSelection 上下移动选中项（首尾环绕），并保证选中项可见。
func (o *OutlineDialog) moveSelection(delta int) {
	if len(o.items) == 0 {
		return
	}
	o.selected = (o.selected + delta + len(o.items)) % len(o.items)
	o.ensureVisible()
	screen.Redraw()
}
```

`internal/dialog/outline.go:146` 与 `internal/dialog/outline.go:151` 的 ↑/↓ 分支改成调 `moveSelection(+1)` / `moveSelection(-1)`，行为不变。

**改动 5：`handleEvent` 加鼠标分支**

在 `internal/dialog/outline.go:142` 的 `handleEvent` 里，`case *tcell.EventKey` 之后加：

```go
	case *tcell.EventMouse:
		mx, my := ev.Position()
		btns := ev.Buttons()
		inArea := o.area.W > 0 &&
			mx >= o.area.X && mx < o.area.X+o.area.W &&
			my >= o.area.Y && my < o.area.Y+o.area.H
		switch {
		case btns&tcell.Button1 != 0:
			idx, ok := o.rowAt(mx, my)
			if !ok {
				return
			}
			o.selected = idx
			o.ensureVisible()
			screen.Redraw()
		case btns&tcell.WheelUp != 0 && inArea:
			o.moveSelection(-1)
		case btns&tcell.WheelDown != 0 && inArea:
			o.moveSelection(+1)
		}
```

五点说明：

- `Button1` 分支只改选中态，不关窗、不回调，跳转仍由 Enter 触发
- `screen.Redraw()` 不能省：选中高亮是反色渲染的，不重绘高亮不会动
- 用位掩码（`btns&tcell.Button1 != 0`）而不是 `switch ev.Buttons()` 等值比较，与包内惯例一致（`internal/dialog/confirm.go:225`、`internal/dialog/msg.go:262` 都是这个写法）。tcell 当前每个事件只产出一个 button 值，等值比较也安全，但位掩码对组合事件更稳
- 鼠标 release 事件 `Buttons()` 为 `ButtonNone`，落不到任何分支，自然被吞掉
- `o.ensureVisible()` 显式调一次。当前 `rowAt` 返回的下标必在视口内，调用无害，但把「选中项始终可见」这个不变量写显式

**滚轮范围限定在弹窗内容区内**

滚轮要判坐标，与点击的可点范围保持一致原则：指针不在弹窗内容区（`area` 矩形）内时，滚轮不生效。

理由：

- 弹窗模态，编辑器正文此时收不到任何事件，滚轮全屏生效会让用户在正文上滚滚轮却改了 outline 高亮，很困惑
- finder 的先例如此：`docs/fileSelect/F2-finder鼠标交互-实现方案.md:238` 写明滚轮要求指针在列表栏内

滚轮的范围比点击宽：滚轮只看整个内容区（含空白区与指示器列），因为滚轮是「滚动列表」而不是「选某一条」，不需要精确到条目行。

### 4.2 明确不改的文件

| 文件 | 为什么不改 |
|---|---|
| `cmd/micro/micro.go` | 事件路由已把鼠标交给浮窗，无需加分支 |
| `internal/dialog/frame.go` | 只做转发与 resize 拦截，鼠标由具体弹窗自处理 |
| `internal/action/outline_neo.go` | 回调签名与语义都不变，点击不碰回调路径 |
| `internal/md/collect_headings.go` | 条目串格式不变，行号反解逻辑不动 |
| `internal/finder/*` | 只作为写法参照，不改 |

## 5. 边界与风险

| 风险 | 评估 |
|---|---|
| 误触：想点正文里的标题却点在弹窗上 | 弹窗本就是模态，点不到正文，现状已如此，本次不引入新误触 |
| 连点两次 | 只改选中态，弹窗不关，两次点击各改一次高亮，不会跳走 |
| 点完不按 Enter，弹窗一直开着 | 这是本次的设计选择：确认动作明确交给用户，Esc 随时可关 |
| 点空白区或指示器列无反应 | 死区设计，弹窗不关、高亮不变，行为可预测 |
| 按住左键拖动 | tcell 开了 button-event tracking（模式 `1000+1002+1006`），拖动过程持续产生 `Button1` 事件 → 高亮跟着手指走，最终停在最后经过的那一行。finder 有同样行为（`docs/fileSelect/F2-finder鼠标交互-实现方案.md:239` 列为可接受的副效果） |
| 终端漏报鼠标抬起 | tcell 的 `wasbtn` 去抖会把后续滚轮降级成 `Button1` → 滚轮变成点击。比「滚轮无响应」更隐蔽，列在这里提醒排查 |
| 右键 / 中键点击 | `switch` 无对应分支，事件被吞掉、无反应。与左键死区行为一致 |
| 滚轮事件被终端截断 | 部分终端只报按下与抬起，滚轮无响应，不影响点击主路径 |
| `mouse` 全局选项被关 | 整个编辑器都没有鼠标，弹窗自然也没鼠标，与本次改动无关 |
| MD 阅读模式下跳转 | 与既有 Enter 路径完全同源，行为一致 |
| `items` 为空时取模 | `moveSelection` 里已加空切片保护 |
| 弹窗打开期间窗口 resize | 浮窗 `HandleEvent` 拦截 resize → 关闭并回调 `onSelect(nil)`，属既有行为，本次不变 |

## 6. 验证

### 6.1 编译

```bash
make build
```

### 6.2 手工验收清单

在一个含多级标题的 MD 文件上：

1. 按 Alt-o 打开目录，弹窗出现在 pane 右侧
2. 鼠标悬停到某条目上，高亮无变化（本次不做 hover 效果）
3. 左键点中间某条目 → 高亮移到该条，弹窗保持打开
4. 点完按 Enter → 弹窗关闭，光标落在该标题行
5. 指针移到弹窗内容区内，滚轮上下滚 → 高亮随之移动
6. 滚到底部，滚轮继续下滚 → 高亮环绕回第一条（与 ↑/↓ 行为一致）
7. 指针移到弹窗外（编辑器正文区），滚轮 → 高亮不变
8. 点一条后按 ↑/↓ → 高亮从点击处继续移动，选中态与键盘共用
9. 按 ↑/↓ 移动高亮，行为与改前一致
10. Enter 跳转、Esc 关闭，行为与改前一致
11. 点弹窗边框 → 无反应，弹窗不关
12. 在标题很少的文档（标题数 < 弹窗高度）上点条目下方空白 → 无反应，弹窗不关，高亮不变
13. 点右侧 ▲▼ 指示器列 → 不算命中该行，高亮不变
14. 在长文档（标题数远超弹窗高度）上点最底部可见条目 → 高亮落在正确条目上，不是第 N 条错位
15. 按住左键在条目间拖动 → 高亮跟着手指走，停在最后经过的那行

第 14 条专门验证 `topIdx` 偏移：条目多于可见行数时，屏幕行必须加 `topIdx` 才是绝对下标。

第 12、13 条验证死区边界：可点范围是条目文本区，比内容区小一圈。

第 7 条验证滚轮范围限定在弹窗内容区内。

## 7. 改动面小结

| 文件 | 类型 | 规模 |
|---|---|---|
| `internal/dialog/outline.go` | 改 | 约 +50 / −8 行 |
| 其余全部文件 | 不改 | 0 |

行数含 `gofmt` 重排结构体注释对齐列产生的噪音（新增 `area Rect` 字段会让三行旧注释跟着移位）。

核心逻辑集中在一个文件的一个 `case` 分支里，没有新增类型，没有新增配置项，没有跨包接口变更。
