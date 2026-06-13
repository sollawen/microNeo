# C4：光标滚动 — row 判定方案

> 关联：根因见 `docs/C0-光标滚动-现状调研.md`；首帧 nil 时序见 `docs/首帧nil崩溃分析.md`。

---

## 1. 问题边界（复述）

MD 渲染文件中，光标停在屏幕底边按 ↓ 不触发上滚、光标飞出视口；按 ↑ 也有对称问题。仅 MD 文档有此问题，非 MD 走 micro 原生路径不受影响。

根因：micro 原生 `Relocate` 全程在"buffer 行空间"运算（`c.Line` vs `StartLine.Line ± scrollmargin`），假设 1 buffer 行 = 1 屏行。MD 装饰行打破这个 1:1 假设。

## 2. 为什么向下动作必须感知装饰行

micro 原生的 `Scroll` 在非 softwrap 下是纯算术（`softwrap.go:292-296`）：

```go
s.Line = util.Clamp(s.Line+n, 0, w.Buf.LinesNum()-1)   // 把 buffer 行号当屏行号加减
```

原生的底边滚动动作（`bufwindow.go` Relocate 内）：
```go
w.StartLine = w.Scroll(c, -height+1+scrollmargin)   // StartLine ≈ 光标上方 20 行
```

它把 `StartLine` 设到光标上方约 `height` 行。光标段原生只保证"**光标段内部**无装饰行"，**不保证**"StartLine 到光标之间无装饰行"。若该区间有 D 个装饰行，光标实际落到屏行 `botMarginRow + D`，**飞出视口**。

后果：连续按 ↓ 时光标持续不可见，直到光标挪进一个"上方 ~20 行都在段内"的够长段，才"追上"显示出来。

→ **向下动作不能用原生，必须感知装饰行。** 向上动作为何可沿用原生，见 §3.3。

## 3. 方案：row 判定 + 非对称动作

### 3.1 核心思路

把判定从"buffer 行空间"挪到"**屏幕行空间**"——屏幕行天然已计入装饰行。

| 方向 | 判定（何时滚） | 动作（怎么滚） |
|------|--------------|--------------|
| 向下 | row-based | **精确**（查 `viewportRowBufLine`） |
| 向上 | row-based | 原生（段内，1:1 精确） |

### 3.2 判定（row-based，两端）

```go
cursorRow, ok := w.bufferLineToScreenOffset(c.Line)   // 光标的真实屏行
botMarginRow := height - 1 - scrollmargin
if cursorRow > botMarginRow { /* 向下滚 */ }
if cursorRow < scrollmargin    { /* 向上滚 */ }
```

`bufferLineToScreenOffset`（`bufwindow_md.go:846`）反向查 `viewportRowBufLine[]`，返回 buffer 行在屏幕上的真实行号。

说明：判定端两处都是 `cursorRow` 与纯整数阈值的比较，与装饰行无关（屏幕行天然已计入装饰行）。

### 3.3 动作

**向下（精确）**：让光标落到底边 margin（屏行 `botMarginRow`）。视口需上移 `delta = cursorRow - botMarginRow` 行。上移 delta 行后，当前屏行 `delta` 的内容变成新的 row 0：

```go
delta := cursorRow - botMarginRow
w.StartLine = SLoc{w.viewportRowBufLine[delta], 0}   // 当前屏行 delta 的 buffer 行 → 新 row 0
```

重新渲染后，原屏行 delta 的内容在 row 0，光标精确落在 `botMarginRow`。

**向上（原生，可用）**：`w.StartLine = w.Scroll(c, -scrollmargin)`，即 `StartLine = c.Line - scrollmargin`。`StartLine` 仅离光标 `scrollmargin`（默认 3）行，**几乎总在光标段内**（原生渲染、无装饰）→ 1:1 精确。偶发短段边界也只是"多滚一点"，光标不会飞出顶部，良性。

### 3.4 关键前提：光标段原生渲染

光标所在段用原生模式渲染（`bufwindow_md.go:727-731`）：

```go
if editMode && hasCursorInside(seg, cursors) {
    vY, rowBufLines = w.renderSegmentNative(seg, vY)   // ★ 光标段 → 原生
} else {
    vY, rowBufLines = w.renderSegmentMD(seg, vY)
}
```

因此 `c.Line` 作为内容行出现在 `viewportRowBufLine` 中，`bufferLineToScreenOffset(c.Line)` **反查必中**。判定端干净。

## 4. 实现代码

### 4.1 修改 `Relocate` 的垂直滚动段（`internal/display/bufwindow.go`）

改成清晰的 if-else 分发：MD 走我们的 row 判定，非 MD 走 micro 原生代码（原样，仅多一层缩进）。

> 注：下面代码里的 `// MicroNeo: MD 走 row 判定...` 注释是**要写进 bufwindow.go 的**（说明架构意图）。

```go
c := w.SLocFromLoc(activeC.Loc)
bStart := SLoc{0, 0}
bEnd := w.SLocFromLoc(b.End())

// MicroNeo: MD 走 row 判定（实质逻辑见 bufwindow_md.go），非 MD 走 micro 原生。
if w.Buf.IsMD && w.mdConfig.MDRender {
	ret = w.relocateVerticalMD(c, scrollmargin, height)
} else {
	if c.LessThan(w.Scroll(w.StartLine, scrollmargin)) && c.GreaterThan(w.Scroll(bStart, scrollmargin-1)) {
		w.StartLine = w.Scroll(c, -scrollmargin)
		ret = true
	} else if c.LessThan(w.StartLine) {
		w.StartLine = c
		ret = true
	}
	if c.GreaterThan(w.Scroll(w.StartLine, height-1-scrollmargin)) && c.LessEqual(w.Scroll(bEnd, -scrollmargin)) {
		w.StartLine = w.Scroll(c, -height+1+scrollmargin)
		ret = true
	} else if c.GreaterThan(w.Scroll(bEnd, -scrollmargin)) && c.GreaterThan(w.Scroll(w.StartLine, height-1)) {
		w.StartLine = w.Scroll(bEnd, -height+1)
		ret = true
	}
}

// horizontal relocation (scrolling)  ← 原样不动
```

### 4.2 新增 `relocateVerticalMD`（`internal/display/bufwindow_md.go`，自包含）

判定基于屏幕行，动作：向下精确、向上沿用原生算术。边界场景（首帧 nil / 光标跳出视口 / 精确动作不可行）此时无可用 `viewportRowBufLine` 数据，走 `relocateVerticalNativeFallback`（1:1 兜底）。

```go
// relocateVerticalMD 是 Relocate 的 MD 垂直滚动分支（自包含）。
// 判定基于屏幕行（bufferLineToScreenOffset），动作：向下精确、向上沿用原生算术。
// 边界场景（首帧 nil / 光标跳出视口 / 精确动作不可行）走 relocateVerticalNativeFallback。
//
// 前提：光标所在段原生渲染（bufwindow_md.go:727），c.Line 作为内容行
// 出现在 viewportRowBufLine 中，反查必中（常规按键场景）。
func (w *BufWindow) relocateVerticalMD(c SLoc, scrollmargin, height int) bool {
	n := len(w.viewportRowBufLine)
	if n == 0 {
		return w.relocateVerticalNativeFallback(c, scrollmargin, height) // 首帧：尚未渲染，1:1 成立
	}
	cursorRow, ok := w.bufferLineToScreenOffset(c.Line)
	if !ok {
		return w.relocateVerticalNativeFallback(c, scrollmargin, height) // 光标跳出视口（goto/搜索）
	}
	botMarginRow := height - 1 - scrollmargin
	if cursorRow > botMarginRow {
		// 向下滚：视口上移 delta 行，新 StartLine = 当前屏行 delta 的 buffer 行。
		delta := cursorRow - botMarginRow
		if delta >= n || w.viewportRowBufLine[delta] < 0 {
			return w.relocateVerticalNativeFallback(c, scrollmargin, height)
		}
		w.StartLine = SLoc{w.viewportRowBufLine[delta], 0} // 精确：光标落到 botMarginRow
		return true
	}
	if cursorRow < scrollmargin {
		w.StartLine = w.Scroll(c, -scrollmargin) // 向上：段内 1:1 精确
		return true
	}
	return true // 光标在 margin 内，无需滚动
}
```

### 4.3 新增 `relocateVerticalNativeFallback`（`internal/display/bufwindow_md.go`）

复刻 micro 原生垂直 Relocate（1:1 假设），仅供 MD 边界场景兜底。

```go
// relocateVerticalNativeFallback 复刻 micro 原生垂直 Relocate（1:1 假设），
// 供 MD 路径边界场景兜底。
//
// ⚠️ 此函数需与 bufwindow.go 中 Relocate 的非 MD 分支保持同步。
// micro 的 Relocate 垂直逻辑历史稳定，但修改其 else 分支时务必同步本函数。
func (w *BufWindow) relocateVerticalNativeFallback(c SLoc, scrollmargin, height int) bool {
	bStart := SLoc{0, 0}
	bEnd := w.SLocFromLoc(w.Buf.End())
	ret := false
	if c.LessThan(w.Scroll(w.StartLine, scrollmargin)) && c.GreaterThan(w.Scroll(bStart, scrollmargin-1)) {
		w.StartLine = w.Scroll(c, -scrollmargin)
		ret = true
	} else if c.LessThan(w.StartLine) {
		w.StartLine = c
		ret = true
	}
	if c.GreaterThan(w.Scroll(w.StartLine, height-1-scrollmargin)) && c.LessEqual(w.Scroll(bEnd, -scrollmargin)) {
		w.StartLine = w.Scroll(c, -height+1+scrollmargin)
		ret = true
	} else if c.GreaterThan(w.Scroll(bEnd, -scrollmargin)) && c.GreaterThan(w.Scroll(w.StartLine, height-1)) {
		w.StartLine = w.Scroll(bEnd, -height+1)
		ret = true
	}
	return ret
}
```

> **设计权衡**：改成纯 if-else 后，MD 分支无法"回退到 else"。故边界场景由 `relocateVerticalNativeFallback` 在 MD 侧自行兜底，导致**原生垂直逻辑存在两份**——`bufwindow.go` 的非 MD 分支（原样）与本函数。
> 这样能让非 MD 分支保持 micro 原生代码原样不动（贴合"最小化原生改动"原则），代价是两份需保持同步（micro 的 `Relocate` 垂直逻辑很稳定，风险低）。
> 备选：把原生逻辑抽成共享方法供两处调用（无重复），但会把原生代码从 `Relocate` 内挪进新方法，对原生改动更大。**本项目取当前方案。**

## 5. 改动文件

| 文件 | 改动 | 说明 |
|------|------|------|
| `bufwindow_md.go` | +新增 `relocateVerticalMD`、`relocateVerticalNativeFallback` | 约 +45 行 |
| `bufwindow.go` | ~改 `Relocate`：垂直段改成 if-else 分发（原生代码包进 else，仅缩进） | 约 +3 / 缩进调整 |

**未涉及的文件**：`softwrap.go`、`actions.go`、渲染管线（`renderSegmentMD`/`renderSegmentNative`）、`viewportRowBufLine` 字段定义与构建逻辑、`bufferLineToScreenOffset`（只读复用）。

## 6. 实施步骤

1. 在 `internal/display/bufwindow_md.go` 新增 `relocateVerticalMD`（§4.2）与 `relocateVerticalNativeFallback`（§4.3）。
2. 在 `internal/display/bufwindow.go` 改 `Relocate`（§4.1）：垂直段改成 if-else 分发，原生代码包进 else（仅缩进）。水平滚动段不动。
3. `make build-quick` 编译。
4. 跑 §8 测试用例。

## 7. 正确性论证

### 7.1 非 MD 零影响
非 MD 时 `w.Buf.IsMD` 为 false → 走 else 分支 → 内部为 micro 原生垂直 Relocate 代码（原样，仅多一层缩进），输入输出与改动前完全一致。

### 7.2 时序正确性
按键时序：`CursorDown`（`actions.go:264-271`）内部先移光标、后调 `Relocate`；外部 `Display()`（`bufwindow.go:936`）在 `CursorDown` 返回后才跑。`Relocate` 读的 `viewportRowBufLine` 是**上一帧**构建的，对应**当前 `StartLine`**（按键期间 `StartLine` 未变），故它准确反映当前视口。判定与向下精确动作都查这个数组，时序有效。

### 7.3 反查必中（前提保证）
光标段原生渲染 → `c.Line` 作为内容行写入 `viewportRowBufLine` → `bufferLineToScreenOffset(c.Line)` 必返回 `(row, true)`。判定端不会误判。

### 7.4 边界与兜底
- **首帧 nil**（首次 Display 前 resize）：`len == 0` → `relocateVerticalNativeFallback`（此刻无 MD 渲染，1:1 成立，正确）。
- **光标跳出视口**（goto-line/搜索）：反查 `!ok` → `relocateVerticalNativeFallback`（1:1 兜底，跳转后下一帧自纠正）。
- **向下精确动作 `delta` 越界 / 落未填充行**：`relocateVerticalNativeFallback`。
- **`delta` 落在装饰行位**（`viewportRowBufLine[delta]` = 绑定的内容行）：重新渲染后光标可能比 `botMarginRow` 偏差 ≤ 装饰行数，小幅且下一次按键自纠正。实测验证。

## 8. 测试用例

| # | 操作 | 预期 |
|---|------|------|
| 1 | MD 文件从 line1 连续按 ↓ 到底 | 光标停在底边 margin（屏行 `height-1-scrollmargin`），**不飞出**，内容随之滚动。偏差 ≤ 装饰行数（§7.4），下一次按键自纠正 |
| 2 | 到底后连续按 ↑ | 光标向上移动，内容向上滚动，光标不飞出顶部 |
| 3 | 光标在 last row 按 ↑ | 光标向上移一行（不再"内容反向滚动"） |
| 4 | 非 MD 文件同样操作 | 行为与原版完全一致 |
| 5 | 短 MD 文件（< 一屏） | 不滚动，光标正常移动 |
| 6 | `./microneo docs/sample.md` 启动 | 不崩溃（首帧 nil 走 fallback） |
| 7 | MD 文件中段 goto-line 跳到远处 | 跳转后视口正确定位光标（走 fallback） |
| 8 | 在装饰密集区（标题/表格/代码块边框）按 ↓ 触发滚动 | 光标偏差 ≤ 装饰行数（§7.4 装饰行位场景），下一次按 ↓ 自纠正到底边 margin |
