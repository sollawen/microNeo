# D15：notePane 内切换 receiver（alt-i）

> **状态**：方案设计（待实施）
>
> **改动面**：仅 `internal/action/notepane.go`（1 个 action 函数 + 3 处注册点），约 30–40 行。
>
> **上游依据**：用户提出的伪代码（见 §一）。
>
> **依赖 / 兼容**：
> - 依赖 **floatFrame 已完成**（DoEvent 集中路由 + FloatFrame 最后画盖在最上层）—— 这是本 D **不需要**碰 notePane 事件路由 / Display 的前提。
> - 与 [D16](./D16-notePane-open参数化.md)（已实施）正交：D16 改 `open(receiver)` 入参；本 D **不调 `open()`**（避免重建 buffer 丢草稿），只改 `selectedReceiver` 字段。

---

## 一、用户需求（伪代码）

```
用户在 notePane 里面的时候，如果想改变 receiver 了，就可以按 alt-i
- 先 discover 看看有几个 receivers
- if 没有，则说明有问题了，在 infoPane 里面提示用户吧
- if 只有一个，则 selectReceiver = 这个唯一的 receiver
- if 2+ 则 selectPaneOpen(receivers)
    - 如果用户选择了一个 receiver，selectReceiver 就是它了
    - 如果用户 esc 没选，那 selectReceiver 不变
```

---

## 二、与 D8（notePaneOpen）的对照

| 维度 | D8 `notePaneOpen`（主编辑器 Alt-Enter） | 本 D `NotePaneSwitchReceiver`（notePane 内 alt-i） |
|---|---|---|
| 触发前提 | 主编辑器有焦点，notePane 未开 | notePane 已开（`IsOpen()`） |
| 缓存命中优化 | ≥2 时查缓存命中跳过 SelectPane | **不做**（切换场景不需要，见 §五） |
| 选完做什么 | `n.open(receiver)`（重建空 buffer） | **只改 `n.selectedReceiver`**（保留草稿，不重建） |
| Esc 行为 | 清零缓存（决策 14） | **不动 `selectedReceiver`**（原 receiver 保留） |

**核心区别**：D8 是「打开 notePane 前选 receiver」，重建 buffer 是期望行为；本 D 是「notePane 开着时换 receiver」，**绝不能丢草稿**——所以只改字段、不调 `open()`。

---

## 三、为什么不需要碰 notePane 的事件路由 / Display

> 这是本 D 相对旧版 D15 最大的简化。旧版 D15 是 floatFrame 完成前写的，需要补 2 个 hook 让 SelectPane 能盖在 notePane 之上、能截走 notePane 的事件。floatFrame 完成后这些 hook 全部不需要了。

当前 `cmd/micro/micro.go` 已经集中路由：

```go
// 事件分发（micro.go:553-556）
} else if action.TheFloatFrame.IsOpen() {        // ← FloatFrame 优先级最高
    action.TheFloatFrame.HandleEvent(event)
} else if action.TheNotePane != nil && action.TheNotePane.IsOpen() {
    action.TheNotePane.HandleEvent(event)
}

// Display 顺序（micro.go:499-503）
if action.TheNotePane != nil && action.TheNotePane.IsOpen() {
    action.TheNotePane.Display()                  // notePane 先画
}
if action.TheFloatFrame.IsOpen() {
    action.TheFloatFrame.Display()                // FloatFrame 后画，盖在最上层
}
```

SelectPane 注册在 FloatFrame 里。因此：

- **事件**：SelectPane 打开时，DoEvent 直接路由到 FloatFrame → SelectPane，notePane 收不到键（天然 modal 隔离，用户在 notePane 里的输入不会跑到 SelectPane 选中的 receiver 上）
- **绘制**：SelectPane 自动画在 notePane 之上（FloatFrame 是 Display 阶段最后一笔）

**结论**：`notepane.go` 的 `HandleEvent` / `Display` **零改动**。本 D 只新增一个 action 函数 + 3 处注册。

---

## 四、改动清单（逐项）

全部在 `internal/action/notepane.go`。

### 4.1 新增 action 函数 `NotePaneSwitchReceiver`

放在 `notePaneClose` / `notePaneOpen` 附近（约行 130 区域）。签名与 `notePaneClose` 一致：`func(h *BufPane) bool`。

```go
// NotePaneSwitchReceiver 在 notePane 已开态下切换 receiver。
// 绑定 alt-i。只更新 selectedReceiver 字段，不调 open()（保留草稿）。
// 守卫：notePane 未开时静默 no-op（alt-i 在主编辑器无意义）。
func NotePaneSwitchReceiver(h *BufPane) bool {
	n := TheNotePane
	if n == nil || !n.IsOpen() {
		return false // 静默：notePane 没开时 alt-i 无效
	}

	// 1. Discover
	receivers, err := eabp.Discover()
	if err != nil {
		InfoBar.Message("✗ discover error: " + err.Error())
		return false
	}

	// 2. case 0：提示用户
	if len(receivers) == 0 {
		InfoBar.Message("✗ no receiver found")
		return false
	}

	// 3. case 1：直接赋值（不做"是否当前"判断 —— 赋值同一个值无副作用）
	if len(receivers) == 1 {
		n.selectedReceiver = receivers[0]
		// 不需要 screen.Redraw()：本函数在 DoEvent 事件回调里执行，
		// 改完字段下一帧 DoEvent 的 Display 阶段会无条件重画整个屏幕，
		// notePane.Display 会读到新的 selectedReceiver.Name。
		return true
	}

	// 4. case 2+：弹 SelectPane
	names := make([]string, len(receivers))
	for i, r := range receivers {
		names[i] = r.Name
	}
	// 锚点 = notePane 左上角。展开方向交给 FloatFrame 自适应（D13）。
	NewSelectPane().Open(names, "Switch Receiver", Pos{X: n.x, Y: n.y}, tcell.Style{}, func(s *string) {
		if s == nil {
			// Esc：selectedReceiver 不变（伪代码明确要求）
			return
		}
		// Enter：找到 name 对应的 RegFile
		for _, r := range receivers {
			if r.Name == *s {
				n.selectedReceiver = r
				// 不需要 screen.Redraw()：本回调在 DoEvent 事件链里执行（SelectPane
				// 的 KeyEnter 经 FloatFrame.HandleEvent 走到这里），下一帧自然重画。
				return
			}
		}
		// 防御：items 来自 receivers，理论上到不了这
		InfoBar.Message("✗ internal: selected name not in receivers")
	})
	return true
}
```

**关键点**：
- **不调 `n.open()`** —— 避免重建 buffer 丢草稿（与 D8 的根本区别）
- **Esc 不动 `selectedReceiver`** —— 严格按伪代码"selectReceiver 不变"
- **不调 `screen.Redraw()`** —— `selectedReceiver` 改了，notePane 上边框嵌的 receiver 名字（`Display` 行 644 读 `n.selectedReceiver.Name`）靠下一帧自然刷新。本函数在 DoEvent 事件链里执行，函数返回后下一帧 Display 阶段无条件重画全屏，不需要主动 Redraw 唤醒 select
- **锚点用 `n.x, n.y`** —— notePane 已开，左上角坐标现成；展开方向交给 FloatFrame（D13）

### 4.2 白名单加 `NotePaneSwitchReceiver`

`allowedNotePaneActions` map（约行 102-103 区域），在 `NotePaneSend` / `NotePaneClose` 旁追加：

```go
"NotePaneSend":          true,
"NotePaneClose":         true,
"NotePaneSwitchReceiver": true,   // ← 新增
```

> 不加白名单的话，`notePaneMapBinding` 会因 `isActionAllowed` 返回 false 而**静默丢弃**这个 binding（见 `notepane.go:275-277`），alt-i 按了没反应。

### 4.3 私有 action map 加 `NotePaneSwitchReceiver`

`notePaneActions` map（约行 117-120），在 `NotePaneSend` / `notePaneClose` 旁追加：

```go
var notePaneActions = map[string]BufKeyAction{
	"NotePaneSend":           NotePaneSend,
	"NotePaneClose":          notePaneClose,
	"NotePaneSwitchReceiver": NotePaneSwitchReceiver,   // ← 新增
}
```

> 这样 action 解析走私有 map 优先（`notePaneRegisterBinding` 行 299），不污染 `bufpane.go` 的 `BufKeyActions`（原生零侵入，D9 既定模式）。

### 4.4 init() 注册 alt-i 绑定

`init()`（约行 202-210），在 `Esc` 绑定后追加：

```go
func init() {
	NotePaneBindings = NewKeyTree()
	notePaneMapDefaults(DefaultBindings("buffer"))
	notePaneMapBinding("Alt-Enter", "NotePaneSend")
	notePaneMapBinding("Esc", "NotePaneClose")
	notePaneMapBinding("Alt-i", "NotePaneSwitchReceiver")   // ← 新增
}
```

> 键名字符串必须用**小写 `i`**（即 `"Alt-i"`）。micro 的绑定解析（`internal/action/bindings.go`）会把 `"Alt-i"` 解析为 `KeyRune + ModAlt + 'i'`，与终端送来的真实事件匹配。若写成大写 `"Alt-I"`，会被解析成 `'I'`（Alt-Shift-I 的结果），与终端送的 `ModAlt+'i'` 不匹配，导致绑定静默失效、按 alt-i 时 fallback 到 rune 插入（在光标处插入一个 `i` 字符）。

---

## 五、与伪代码的偏差说明

用户伪代码很完整，本 D 严格遵循，仅一处需要明确：

### 5.1 2+ 个 receiver 时每次都弹 SelectPane

按伪代码严格实现：discover 出 ≥2 个 receiver，alt-i 每次都弹 SelectPane 让用户从列表里挑，不查缓存、不做「上次用过的那个」优化。

> 「弹 SelectPane 时光标初始预置到当前 receiver 上」是个不错的体验优化（弹出来就在当前项，上下移选别的，Enter 保持），但当前 SelectPane 没有预置光标位置的能力。等以后 SelectPane 加了这个能力再做。

---

## 六、不变项（核对清单）

| 项 | 现状 | 是否改 | 说明 |
|---|---|---|---|
| notePane `HandleEvent` | 转发 resize + `BufPane.HandleEvent` | **不改** | FloatFrame 集中路由已处理 modal（§三） |
| notePane `Display` | 画边框 + 嵌 receiver 名字 + BufWindow | **不改** | FloatFrame 后画盖在上层；`selectedReceiver` 改了靠下一帧自然刷新（不需 Redraw） |
| `selectedReceiver` 字段 | 双重职责：发送目标 + 缓存 | **不改** | 本 D 只写不读，读取端（NotePaneSend 行 470 / Display 行 644）自动看到新值 |
| SelectPane 接口 | `Open(items, title, anchor, frameColor, onSelect)` | **不改** | D13/FloatFrame 已定 |
| SelectPane 锚点展开 | FloatFrame `expandAnchor` 自适应 | **不改** | D13 既定，调用方只传锚点坐标 |
| `open(receiver)` 签名 | D16 已参数化 | **不改** | 本 D 不调 open |
| `notePaneOpen`（D8/D16） | 主编辑器 Alt-Enter 打开流程 | **不改** | 与本 D 正交 |

---

## 七、验收

| # | 场景 | 期望 |
|---|------|------|
| 1 | notePane 开，草稿有内容，按 alt-i，≥2 个 receiver | SelectPane 从 notePane 左上角附近弹出；草稿**保留**（没被清空） |
| 2 | 选某 receiver + Enter | SelectPane 关闭，notePane 上边框 receiver 名字刷新，草稿保留 |
| 3 | Esc 取消 | SelectPane 关闭，原 receiver 保留，草稿保留 |
| 4 | 只有 1 个 receiver，按 alt-i | 不弹 SelectPane，直接切到那个 receiver，上边框名字刷新 |
| 5 | 0 个 receiver，按 alt-i | InfoBar 显示"✗ no receiver found"，notePane 状态不变 |
| 6 | notePane 未开时按 alt-i | 静默无效（return false，不弹、不提示） |
| 7 | SelectPane 打开时在 notePane 里敲字符 | 字符不进 notePane（被 FloatFrame 截走给 SelectPane） |
| 8 | 切换 receiver 后按 Alt-Enter 发送 | 发送到**新切换的** receiver（验证 NotePaneSend 读到新值） |
| 9 | 终端极矮/极窄 | FloatFrame 失败前置检查：`Open` 返回 false → `onSelect(nil)` → 等价 Esc，notePane 保留 |

**编译**：`make build-quick` 0 错 0 警；最终 `make build` 通过。

---

## 八、影响面与回滚

### 8.1 影响面

- **外部可观测行为**：新增 alt-i 功能（notePane 内切换 receiver），其它行为零变化。
- **内部结构**：notePane 加一个 action + 3 处注册。事件路由 / Display 零改动。
- **micro 原生代码**：零侵入（全部改在 `notepane.go`）。

### 8.2 回滚

改动集中在 `notepane.go` 一个文件。回滚 = 撤销对该文件的修改。无中间状态风险。

---

## 九、与历史 D 系列的关系

| 文档 | 关系 |
|------|------|
| [D13（SelectPane 设计）](./D13-SelectPane设计.md) | 不涉及——SelectPane 接口不变 |
| [D14（notePane-select 锚点）](./D14-notePane-select锚点.md) | 不涉及——本 D 锚点用 notePane 左上角（`n.x, n.y`），不是 D14 的光标下方 |
| [D16（open 参数化）](./D16-notePane-open参数化.md) | 正交——D16 改 `open(receiver)` 入参；本 D 不调 open |
| [说明-notepane.md](./说明-notepane.md) | 收尾时同步：§七 键位表加 alt-i 行；§十 关键变更表加 D15 行 |

---

## 附：与旧版 D15 的差异（重写说明）

旧版 D15 是 floatFrame 完成前写的，核心技术点是「补 2 个 hook 让 SelectPane 能盖在 notePane 之上、能截走 notePane 的事件」：

- hook 1：notePane `HandleEvent` 转发给 SelectPane（镜像主 BufPane 旧 forwarding）
- hook 2：notePane `Display` 末尾叠加 SelectPane

floatFrame 完成后，DoEvent 已集中路由（`micro.go:553-556` FloatFrame 优先级最高）、Display 已集中叠加（`micro.go:499-503` FloatFrame 最后画），**这 2 个 hook 完全不需要了**。新 D15 因此大幅简化：只新增一个 action 函数 + 3 处注册，不碰 notePane 的事件路由与 Display。

旧版 D15 的「单 receiver 静默 return」优化也被去掉（§五.1），严格按本次用户伪代码。
