# D14：notePane 给 SelectPane 传真实锚点

> 依赖：D13（SelectPane 位置自适应，`Open(items, title, x, y *int, onSelect)`）
> 状态：待实施
> 改动面：`internal/action/notepane.go` 一个调用点 + 几行锚点计算

---

## 一、动机

D13 实施时，notePane 作为唯一调用方暂时传 `nil, nil`，保留 D13 之前的旧行为（左下角、向上展开）。本文把 notePane 升级为传**真实锚点**，让 SelectPane 变成"从主编辑器光标下方下拉"的 receiver 选择器。

**视觉目标**：用户在主编辑器按 Alt-Enter（多 receiver）→ SelectPane 从光标下方弹出（dropdown 风格）→ 选完 receiver → SelectPane 关闭 → notePane 输入框**在同一位置**出现。两个浮窗锚点重合，视觉连贯。

---

## 二、锚点策略（已拍板）

| 维度 | 取值 | 来源 |
|------|------|------|
| `ay`（y 锚点） | `lowestCursorScreenRow + 1`（光标/选区**下一行**） | 决策 2 + 3 |
| `ax`（x 锚点） | `view.X + 2`（gutter 之后第 3 个字符，0-indexed） | 决策 1（+2 / +3 任选，取 +2） |

### 2.1 y 方向的两个细化决策

- **决策 2（多光标）**：取**最下面的光标**的 y。
- **决策 3（有 selection）**：以 selection 的**最下边**为 y。

**这两条恰好就是 notePane 现成的 `lowestCursorScreenRow` 已实现的逻辑**——它遍历所有 cursor，有 selection 的取 `CurSelection[0]/[1]` 里 Y 更大的端点（下边界），无 selection 的取光标本身，再取其中最低的 screenRow。所以 y 锚点**零新逻辑**，直接复用：

```go
lowestRow := n.lowestCursorScreenRow(bw, view)   // 已处理多光标取最低 + selection 取下边界
ay := lowestRow + 1
```

这也意味着 SelectPane 的 y 锚点 = notePane 自己的 `notePaneTopBorder`（两者都是 `lowestRow + 1`），**完全重合**。

### 2.2 x 方向

`view.X` 已含 gutter 偏移（`view.X = w.X + gutterOffset`，见 `bufwindow.go:144`），是文本内容区左边缘。gutter 之后的第 3 个字符：

```go
ax := view.X + 2    // 0-indexed：第1个=view.X+0、第2个=view.X+1、第3个=view.X+2
```

自动适应行号位数变化（99 行 vs 1000 行 gutter 不同宽），无需单独算 gutter 宽度。

---

## 三、为什么这个时机算锚点是对的

SelectPane 是 **modal** 的：`bufpane.go:434-436` 在 `SelectPane.IsOpen()` 时把事件早 return 转发给 SelectPane，主编辑器**冻结**。所以：

- 锚点在 `notePaneOpen` 入口算（`SelectPane.Open` 之前），从 `MainTab().CurPane()` 取主编辑器光标；
- 整个选 receiver 过程中主编辑器光标不变，入口算的锚点稳定有效；
- `lowestCursorScreenRow` 会顺便捕获 `n.fileCursor / n.fileSelection`（和 `reposition` 里再捕获一次幂等，同一冻结状态）。

---

## 四、改动清单

| 文件 | 改动 | 行数 |
|------|------|------|
| `internal/action/notepane.go` | `notePaneOpen` 里 `TheSelectPane.Open(...)` 调用点之前，算 `ax, ay`；把 `nil, nil` 改成 `&ax, &ay` | ~+6 |

**唯一调用点**（`notePaneOpen` case 2+ 缓存未命中分支）：

```go
// 改前
TheSelectPane.Open(names, "Receiver", nil, nil, func(s *string) { ... })

// 改后
view := pane.BWindow.GetView()
bw := pane.BWindow.(*display.BufWindow)
lowestRow := n.lowestCursorScreenRow(bw, view)
ax := view.X + 2
ay := lowestRow + 1
TheSelectPane.Open(names, "Receiver", &ax, &ay, func(s *string) { ... })
```

> `pane` 在 `notePaneOpen` 入口已可取（`MainTab().CurPane()`，与 `reposition` 同源）。`h *BufPane` 参数也可用，但为和 `reposition` 一致用 `MainTab().CurPane()`。

**不动**：
- 其余两个 case（1 receiver / 缓存命中）不调 SelectPane，无需改
- SelectPane 本身（D13 已完成）
- `lowestCursorScreenRow` / `reposition` / `locToScreenRow`（直接复用）

---

## 五、验收

| # | 场景 | 期望 |
|---|------|------|
| 1 | 光标在屏幕中部、单光标无选区 | SelectPane 从光标下一行、内容区第 3 列下拉 |
| 2 | 光标在屏幕底部（下方空间不够 paneH） | D13 §4.3 兜底向上展开（dropup），不溢出 statusLine |
| 3 | 多光标，cursor A 在第 5 行、cursor B 在第 10 行 | SelectPane 锚点 y = 第 10 行 + 1（取最低） |
| 4 | 有 selection 跨第 3-7 行 | SelectPane 锚点 y = 第 7 行 + 1（取选区下边界） |
| 5 | 选完 receiver（Enter） | SelectPane 关闭，notePane 输入框在**同一行**出现 |
| 6 | Esc 取消 | SelectPane 关闭，notePane 不开（D13 §4.6 + 旧行为） |
| 7 | 终端极矮/极窄 | D13 §4.6 失败路径：`onSelect(nil)`，notePane 提示"✗ 已取消" |

编译：`make build-quick` 0 错 0 警。

---

## 六、与 D13 的关系

D13 定义了 SelectPane **如何接受锚点并自适应展开**；本文定义 notePane **作为调用方传什么锚点**。两者职责分离：

- D13：SelectPane 内部算法（不关心调用方传的锚点含义）
- D14：notePane 的调用方决策（光标下方 + gutter 之后第 3 列）

notePane 之后若想换锚点策略（比如跟随光标实际列、或避开某浮窗），只改 notePane，不动 SelectPane。
