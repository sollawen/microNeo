# D15：notePane 内切换 receiver（Alt-I）

> 依赖：D13（SelectPane 位置自适应）、D14（notePane 传真实锚点）
> 状态：待实施
> 改动面：`internal/action/notepane.go`（2 个 hook + 1 个 action + 1 行绑定）

---

## 一、动机与交互

**场景**：用户在 notePane 里输入了一些内容，临时想换个 receiver。当前必须 Esc 关掉重开，输入的草稿会丢（`open()` 兑现"打开=全新"承诺，重建空 buffer）。

**新交互**：notePane 开着时按 **Alt-I** → 弹 SelectPane 选 receiver → 选完（Enter）只更新 `selectedReceiver`，notePane 保持开、草稿保留；Esc 取消则什么都不变。

**核心原则**（用户拍板）：调用方只管传锚点坐标，**展开方向交给 D13 自适应**，不强制向上。SelectPane 会根据 `n.x, n.y` 和屏宽自动判方向（receiver 少向下、receiver 多向上，等等）。调用方不关心、也不应该关心展开方向。

---

## 二、前置研究结论

### 2.1 Alt-I 当前空闲

全局无绑定（`runtime/`、`internal/`、`cmd/` 搜索均无 `Alt-i`/`AltI`），notePane binding 树也没有。可用。

### 2.2 关键 gap：SelectPane 只挂主 BufPane，notePane 有独立路由

这是本 D 最核心的技术点。

SelectPane 现有的转发/绘制都挂在**主 BufPane**：
- 事件：`bufpane.go:434-436`（`IsOpen()` 时转发给 SelectPane 并早 return）
- 绘制：`bufpane.go:545-546`（主 BufPane.Display 末尾画 SelectPane）

但 notePane 开着时，顶层路由（`micro.go:547-548`）把事件**直接给 `TheNotePane.HandleEvent`**，主 BufPane 不参与；绘制顺序（`micro.go:499-500`）notePane 也是**最后画、盖在最上层**。

**后果**：直接在 notePane 里调 `SelectPane.Open` → SelectPane **看不见（被 notePane 盖住）、也收不到键**。

**解法**：在 `notepane.go` 补 2 个 hook，镜像主 BufPane 的模式（详见 §三）。

### 2.3 receiver 列表来源

无缓存。`notePaneOpen` 每次 `eabp.Discover()`（本地 socket 查询，快）。switch 时同样实时 discover，v1 同步即可，不引入异步。

### 2.4 buffer / 焦点 / 收尾

- **buffer**：switch 时**不调 `n.open()`**（它会重建空 buffer 丢草稿），只更新 `n.selectedReceiver`
- **焦点**：SelectPane 关闭后事件路由自然回到 notePane（顶层路由看 `TheNotePane.IsOpen()` 仍为 true）
- **标题刷新**：notePane 上边框嵌的 receiver 名字在每次 `Display` 时按 `n.selectedReceiver.Name` 重画（`notepane.go:643` 附近），更新即时

---

## 三、改动清单

全部在 `internal/action/notepane.go`。

### 3.1 hook 1：HandleEvent 转发（镜像 `bufpane.go:434-436`）

`notepane.go:604` 现状：

```go
func (n *NotePane) HandleEvent(event tcell.Event) {
	if _, ok := event.(*tcell.EventResize); ok {
		if n.isOpen {
			n.reposition()
		}
		return
	}
	n.BufPane.HandleEvent(event)
}
```

改后（在 resize 分支之后、`n.BufPane.HandleEvent` 之前插入转发）：

```go
func (n *NotePane) HandleEvent(event tcell.Event) {
	if _, ok := event.(*tcell.EventResize); ok {
		if n.isOpen {
			n.reposition()
		}
		return
	}
	// D15：SelectPane 打开时转发（镜像主 BufPane bufpane.go:434-436）
	if TheSelectPane != nil && TheSelectPane.IsOpen() {
		TheSelectPane.HandleEvent(event)
		return
	}
	n.BufPane.HandleEvent(event)
}
```

**位置说明**：转发放在 resize 分支**之后**，保证 resize 优先级最高（resize 应该总能触发 reposition，不被 SelectPane 截走）。两者逻辑独立、不冲突。

### 3.2 hook 2：Display 叠加（镜像 `bufpane.go:545-546`）

`notepane.go:615` 的 `Display` 末尾，画完 notePane 自己之后追加：

```go
// D15：SelectPane 叠加在 notePane 之上（镜像主 BufPane bufpane.go:545-546）
if TheSelectPane != nil && TheSelectPane.IsOpen() {
	TheSelectPane.Display()
}
```

放在 `Display` 的**最后一行**，确保 SelectPane 画在 notePane 之上（盖住）。

### 3.3 action handler：NotePaneSwitchReceiver

新增 action 函数（放 notePane 已有 action 函数附近，如 `NotePaneOpen`/`NotePaneClose` 旁）：

```go
// NotePaneSwitchReceiver 在 notePane 已开态下弹 SelectPane 切换 receiver。
// 绑定 Alt-I。只更新 selectedReceiver，不重建 buffer（保留草稿）。
func NotePaneSwitchReceiver() bool {
	if TheNotePane == nil || !TheNotePane.IsOpen() {
		return false  // 静默：notePane 没开时 Alt-I 无效
	}
	n := TheNotePane

	// 实时 discover（本地 socket，快；v1 同步）
	receivers, err := eabp.Discover()
	if err != nil {
		InfoBar.Message("✗ discover error: " + err.Error())
		return false
	}
	if len(receivers) == 0 {
		InfoBar.Message("✗ no receiver found")
		return false
	}
	if len(receivers) == 1 && receivers[0].Socket == n.selectedReceiver.Socket {
		InfoBar.Message("· only one receiver, already selected")
		return false  // 只有一个且就是当前的，没必要弹
	}

	names := make([]string, len(receivers))
	for i, r := range receivers {
		names[i] = r.Name
	}

	// 锚点 = notePane 左上角。展开方向交给 D13 自适应。
	ax := n.x
	ay := n.y
	TheSelectPane.Open(names, "Switch Receiver", &ax, &ay, func(s *string) {
		if s == nil {
			// Esc：保留当前 receiver，什么都不变
			return
		}
		// Enter：找到 name 对应的 RegFile，更新 selectedReceiver
		for _, r := range receivers {
			if r.Name == *s {
				n.selectedReceiver = r
				screen.Redraw()  // 触发 notePane 重画，刷新上边框 receiver 名字
				return
			}
		}
		// 防御：items 来自 receivers，理论上到不了这
		InfoBar.Message("✗ internal: selected name not in receivers")
	})
	return true
}
```

**与 `notePaneOpen` 的关键差异**：
| | `notePaneOpen` | `NotePaneSwitchReceiver` |
|---|---|---|
| 触发前提 | 主编辑器有焦点 | notePane 已开（`IsOpen()`） |
| 弹完做什么 | callback 里 `n.open()`（重建 buffer） | callback 里只改 `selectedReceiver`（保留 buffer） |
| case 1 / 缓存命中优化 | 直接赋值不弹 | 单 receiver 时静默 return（不弹） |
| 上下文捕获 | `lowestCursorScreenRow`（notePane 还没开） | 不需要（notePane 已开，`n.x/n.y` 现成） |

### 3.4 绑定

`init()` 里（`notepane.go:213` 附近，`NotePaneBindings` 注册区）：

```go
notePaneMapBinding("Alt-I", "NotePaneSwitchReceiver")
```

> 注意：Alt-I 用大写 I（Alt-Shift-I），避免和可能的小写 i 文本输入冲突。若终端不支持，fallback 到其它组合（实施时验证）。

---

## 四、流程时序

### 4.1 切换 receiver（Enter）

```
notePane 开，焦点在 notePane
→ Alt-I
→ NotePaneSwitchReceiver: discover → SelectPane.Open(anchor=n.x,n.y)
→ 顶层路由：事件 → notePane.HandleEvent → hook1 转发 → SelectPane.HandleEvent
→ 绘制：notePane.Display → hook2 叠加 → SelectPane.Display（盖在上）
→ 用户 Up/Down 选 + Enter
→ SelectPane.Close + onSelect(&name) → 更新 selectedReceiver + Redraw
→ 焦点自动回 notePane（IsOpen 仍 true），上边框 receiver 名字刷新
```

### 4.2 取消（Esc）

```
→ ...同上到 SelectPane 打开
→ Esc
→ SelectPane.Close + onSelect(nil) → 直接 return（selectedReceiver 不变）
→ 焦点回 notePane，原 receiver 保留
```

### 4.3 SelectPane modal 对 notePane 的意义

hook1 转发 + 早 return 保证：SelectPane 开着时，notePane 的 `BufPane.HandleEvent` 收不到键，**用户在 notePane 里的输入不会跑到 SelectPane 选中的 receiver 上**。双向隔离干净。

---

## 五、位置语义说明（不强制，仅记录）

按 §一原则，调用方只传 `(n.x, n.y)`，展开方向由 D13 自适应。实际效果：

- receiver 少（`paneH = min(receivers,10)+2` ≤ notePane 高 7，如 3 个 → paneH=5）：D13 判**向下展开**，SelectPane 顶边 = n.y，整块盖住 notePane（草稿不丢，选完回来）
- receiver 多（paneH > 7）：向下不够，D13 判**向上展开**，盖主编辑器、notePane 保持可见

这是 D13 算法的自然结果，调用方不干预。草稿被盖时不会丢（notePane buffer 不动），选完自动回来。

---

## 六、验收

| # | 场景 | 期望 |
|---|------|------|
| 1 | notePane 开，按 Alt-I | SelectPane 从 notePane 左上角附近弹出，可 Up/Down |
| 2 | 选某 receiver + Enter | SelectPane 关闭，notePane 上边框 receiver 名字刷新，草稿保留 |
| 3 | Esc 取消 | SelectPane 关闭，原 receiver 保留，草稿保留 |
| 4 | 只有一个 receiver 且是当前的 | 不弹，InfoBar 提示 "only one receiver, already selected" |
| 5 | notePane 未开时按 Alt-I | 静默无效（return false，不弹） |
| 6 | SelectPane 打开时在 notePane 里敲字符 | 字符不进 notePane（被 SelectPane 截走） |
| 7 | 终端极矮/极窄 | D13 §4.6 失败路径：`onSelect(nil)`，等价 Esc，notePane 保留 |

编译：`make build-quick` 0 错 0 警。

---

## 七、与 D13/D14 的关系

- **D13**：定义 SelectPane 如何接受锚点自适应展开（不关心调用方）
- **D14**：notePane **打开前**的调用方决策（光标下方 + gutter 之后第 3 列）
- **D15**（本文）：notePane **已开态下**的调用方决策（notePane 左上角）+ notePane 侧补 SelectPane 转发/绘制 hook

D15 之后 notePane 有了"打开前选"（D14）和"开态切换"（D15）两条 SelectPane 调用路径，hook 让两条都能正常工作。SelectPane 本身（D13）零改动。
