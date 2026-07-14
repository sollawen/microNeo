# pane ↔ tab 互转 —— 可行性调研

需求来源：`Z1-tab切换.md`（提出两个语义方向：「PROMOTE」= 把活动 pane 移出去变新 tab 且占满；「ABSORB」= 把活动 pane 移出去变新 tab 且只占原位置大小 / 把相邻 tab 合并成一个 split tab）。本文聚焦「**micro / microNeo 现在是怎么实现 pane 和 tab 切换的**」——只讲底层模型和现有原语，具体怎么实现新功能另开 PLAN 文档。

---

## 1. micro 的 tab / pane 数据模型

micro 把屏幕分成两层：

- **TabList**：一组 Tab 的列表 + 一个 active 索引。一次只显示 active Tab 的内容，Tab 之间互斥可见。
- **Tab + Split tree**：每个 Tab 持有一个独立的 split tree（`internal/views/splits.go` 的 `Node`）。

「Pane」不是一个独立实体，而是「split tree 的叶子 + 挂着的 `BufPane`」。一个 Tab 里多个 pane 就是 split tree 多个叶子；多个 Tab 之间共享同一个 buffer 时是 buffer 引用共享、不是 pane 共享。

```
TabList
└── Tab
    ├── Node (split tree root)
    │   ├── Node (parent)
    │   │   ├── Node (leaf) → BufPane (绑 buffer)
    │   │   └── Node (leaf) → BufPane (绑 buffer)
    │   └── Node (leaf) → BufPane (绑 buffer)
    └── buffer 共享：多个 Tab 看同一 buffer 时共享的是 buffer，不是 pane
```

---

## 2. micro 现成暴露给用户的 action

来源：`internal/action/bufpane.go:746` `BufKeyActions` map、`runtime/help/keybindings.md:314-325`。

### 2.1 现成 action 全清单

| Action | 默认键 | 层级 | 行为 |
|---|---|---|---|
| `AddTab` | `Ctrl-t` | TabList | 开新 Tab，里面 1 个 pane |
| `PreviousTab` / `NextTab` / `FirstTab` / `LastTab` | `Alt-,` / `Alt-.` / `CtrlPageUp` / `CtrlPageDown` | TabList | Tab 间切焦点 |
| `VSplit` / `HSplit` | `> vsplit` / `> hsplit` | Tab 内 | 当前 pane 拆成左右 / 上下两个 pane |
| `NextSplit` / `PreviousSplit` / `FirstSplit` / `LastSplit` | `Ctrl-w` | Tab 内 | pane 间切焦点 |
| `Unsplit` | 无默认 | Tab 内 | 除活动 pane 外其余全关 |

加 `Ctrl-+` / `Ctrl--` 调控比例是 Z0 已经做了的（`command_neo.go` 里的 `GrowPane` / `ShrinkPane`），不在本调研范围。

### 2.2 缺什么

**跨 TabList ↔ Tab 内 split tree 边界的能力，micro 完全没有**：

| 需求动作 | 现状 |
|---|---|
| 把活动 pane 从 Tab 移到新 Tab | ❌ 无 `MovePane` / `PromotePane` / `PaneToTab` 等任何 action |
| 把两个 Tab 合并成一个 split Tab | ❌ 无 `MergeTab` / `JoinTab` / `AbsorbPane` 等任何 action |

`grep -r "MovePane\|MoveToTab\|Pane2Tab\|PromotePane\|SplitTab\|MergeTab\|JoinTab\|AbsorbPane" internal/ runtime/` 全 0 命中。

能复用的零碎原语（**不是**要的能力）：
- `Unsplit`（`actions.go:2039`）：tab 内除活动 pane 外全关 —— 把 pane **收掉**，不是**拆出去**
- `AddTab`（`actions.go:1976`）：只能开**空白** Tab，不能带当前 pane 的内容
- `:tab <file>`：用同一 buffer 重开 Tab（buffer 共享，但路径绕一圈，新 Tab 几何按 tab-list 默认）

---

## 3. 底层原语（关键 —— 全齐）

micro 没暴露 pane↔tab 互转的 action，但底层函数全在。任何实现都是「拼装现成原语」，不需要改 split tree / buffer / display 任何底层模块。

### 3.1 TabList 层（`internal/action/tab.go`）

| API | 行 | 作用 |
|---|---|---|
| `NewTabFromPane(x, y, w, h int, pane Pane) *Tab` | 259 | 把任意 pane 包成新 Tab（带几何） |
| `NewTabFromBuffer(x, y, w, h, b)` | 246 | 把 buffer 单独包成新 Tab |
| `(*TabList).AddTab(p *Tab)` | 52 | 把 Tab 加进 TabList |
| `(*TabList).RemoveTab(id uint64)` | 59 | 从 TabList 摘 Tab（内部会调 `SetActive`） |
| `(*Tab).AddPane(pane Pane, i int)` | 353 | 给现成 Tab 塞 pane |
| `(*Tab).RemovePane(i int)` | 373 | 从 Tab 拆 pane |
| `(*Tab).Resize()` | 380 | 遍历 panes 对齐节点几何 |
| `tab.GetNode(splitID)`（embedded `*views.Node` promoted） | — | 从 split tree 拿节点 → `Node.X/Y/W/H` 直接读 |

### 3.2 Split tree 层（`internal/views/splits.go`）

| API | 行 | 作用 |
|---|---|---|
| `Node.Unsplit()` | 467 | 收 split tree 到只剩 1 个叶子（含 `flatten()` 自动清理单孩子链） |
| `Node.HSplit(bottom bool)` / `Node.VSplit(right bool)` | — | 拆 split tree 出新叶子 |

### 3.3 Pane 层（`internal/action/bufpane.go`）

| API | 行 | 作用 |
|---|---|---|
| `BufPane.SetTab(t)` | 316 | 改 pane 的 Tab 归属 |
| `BufPane.SetID(id)` | 411 | 改 pane 的 splitID（split tree 叶子 ID） |

---

## 4. microNeo 的 Resize 补刀 pattern

microNeo 在 `command_neo.go:112-117`（`AddTab`）和 `:141-146`（`NewTabCmd`）手动补了 `MainTab().Resize()` —— micro 原生 `Tabs.AddTab` 不会重新触发几何对齐，新 Tab 加进去后 src Tab 的 split tree 几何可能滞后。

这是 microNeo 已知要点：**任何 pane↔Tab 互转实现都要在 `AddTab` / `RemoveTab` / 拆叶子之后补一次 `Tabs.Resize()`**。新功能沿同一 pattern 写，不踩陌生坑。

---

## 5. pane ↔ tab 互转的本质

把它落到 micro 的数据结构上看就两句话：

- **pane → Tab**：把 split tree 的叶子节点 + 挂着的 BufPane 一起搬到 TabList（新建一个 Tab 装它），原 Tab 的 split tree 收缩到剩余叶子。
- **Tab → pane**：把另一个 Tab 的 BufPane 从它的 split tree 摘下来，挂进当前 Tab 的 split tree（拆出新叶子），然后从 TabList 删掉那个空 Tab。

整个过程涉及三种数据结构的协同变化：split tree 几何、`Tab.Panes` 列表、`TabList` 列表与 active 索引。micro 不缺任何一个 API，只缺一个把它们拼起来的上层 action + 用户键位绑定。键位选型见独立 PLAN 文档。
