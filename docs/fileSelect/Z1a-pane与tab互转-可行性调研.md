# pane ↔ tab 互转 —— 可行性调研

需求来源：`Z1-tab切换.md`（提出 `Alt-+` / `Alt-_` 两个语义：场景1 把活动 pane 移出去变新 tab；场景2 把相邻 tab 合并成一个 split tab）。本文先回答「**micro / microNeo 现在能不能**」，再回答「**能不能做、怎么做**」。

---

## 1. 现状（已查证）

### 1.1 micro / microNeo 现成的 tab + split action 全清单

来源：`internal/action/bufpane.go:746` `BufKeyActions` map、`runtime/help/keybindings.md:314-325`。

| Action | 默认键 | 行为 |
|---|---|---|
| `AddTab` | `Ctrl-t` | 开新 tab，里面 1 个 pane |
| `PreviousTab` / `NextTab` / `FirstTab` / `LastTab` | `Alt-,` / `Alt-.` / `CtrlPageUp` / `CtrlPageDown` | tab 间切焦点 |
| `VSplit` / `HSplit` | `> vsplit` / `> hsplit` | 当前 tab 内分屏（左右 / 上下） |
| `NextSplit` / `PreviousSplit` / `FirstSplit` / `LastSplit` | `Ctrl-w` | 当前 tab 内 pane 切焦点 |
| `Unsplit` | 无默认 | 当前 tab 内除活动 pane 外其余全关 |

加 `Ctrl-+` / `Ctrl--` 调控比例是 Z0 已经做了的（`command_neo.go` 里的 `GrowPane` / `ShrinkPane`），不在本调研范围。

### 1.2 Z1 想要的两个功能，现在都没有

| Z1 需求 | 现状 |
|---|---|
| 把活动 pane 从 tab 移出去变新 tab | ❌ 无 `MovePane` / `PromotePane` / `PaneToTab` / `SplitTab` 等任何 action。grep `MovePane\|MoveToTab\|Pane2Tab\|PromotePane\|SplitTab` 在 `internal/` `runtime/` 全部 0 命中。 |
| 把两个 tab 合并成一个 split tab | ❌ 无 `MergeTab` / `JoinTab` / `AbsorbPane` 等任何 action。grep 同上 0 命中。 |

能复用的只有「方向相反」的零碎原语：
- `Unsplit`（actions.go:2039）能把单 tab 内的多 pane 收成一个 pane —— **不是**把 pane 拆到新 tab。
- `AddTab`（actions.go:1976）只能开一个**空白** tab，不能把当前 pane 的内容带过去。
- 手动 `:tab <file>` 能用同一 buffer 重开一个 tab（buffer 共享），效果接近「pane 移出去」，但路径绕一圈、不是直接动作、且新 tab 的几何是 tab-list 默认尺寸不是 pane 原尺寸。

### 1.3 但是底层原语全齐（关键）

| 需求能力 | 现成 API | 位置 |
|---|---|---|
| 把任意 pane 包成新 Tab | `NewTabFromPane(x, y, w, h int, pane Pane) *Tab` | `internal/action/tab.go:259` |
| 给现成 Tab 塞 pane | `(*Tab).AddPane(pane Pane, i int)` | `internal/action/tab.go:353` |
| 从 Tab 拆 pane | `(*Tab).RemovePane(i int)` | `internal/action/tab.go:373` |
| 把 tab 加进 TabList | `(*TabList).AddTab(p *Tab)` | `internal/action/tab.go:52` |
| 把 tab 从 TabList 移除 | `(*TabList).RemoveTab(id uint64)` | `internal/action/tab.go:59`（注意：只跳过空 pane 不报错） |
| 从 split tree 拿 pane 几何 | `tab.GetNode(pane.splitID)`（promoted from embedded `*views.Node`）→ `Node.X/Y/W/H`（View 字段直接读） | `internal/views/splits.go:34, 130` |
| 移除 pane 后收缩 split tree | `Node.Unsplit()`（含 `flatten()` 自动清理单孩子链） | `internal/views/splits.go:467, 488` |
| tab 整体几何调整 | `Tab.Resize()`（遍历 panes 对齐节点几何） | `internal/action/tab.go:380` |
| 把 pane 挂到新 tab | `BufPane.SetTab(t)` / `SetID(id)` | `internal/action/bufpane.go:316, 411` |
| 把 buffer 单独包成新 tab | `NewTabFromBuffer(x, y, w, h, b)` | `internal/action/tab.go:246` |

microNeo 已经在 `command_neo.go:112-117` 和 `:141-146` 为 `AddTab` / `NewTabCmd` 手动补了 `MainTab().Resize()`，说明「tab/pane 几何重建时机」是 microNeo 已知要点，沿同一 pattern 写新 action 不踩陌生坑。

---

## 2. 答「能不能做」—— 能

Z1 两个功能都**可实现**，且都是「已具备底层原语、只缺一个上层 action + 键位」的范围，不需要改 split tree / buffer / display 任何底层模块。

推荐路线：**Go 层加 action**（路线 A）。理由：
- microNeo 已经有 `command_neo.go` 重写 `VSplit` / `HSplit` / `AddTab` 的先例（`command_neo.go:24-27`），模式现成，绑定 + Resize 时机都有参考。
- Lua 插件（路线 B）能跑，但 Resize 时机、跨 pane/tab 的状态机同步比 Go 难控制；microNeo 项目里的 `Linter` 等插件已经吃过 Lua 时序的亏（看 `runtime/plugins/linter/linter.lua` 怎么 defer 调 `BufPane.Resize`），本功能涉及 split tree 改写，更倾向 Go。

---

## 3. 设计 —— 两个 action 的实现思路

### 3.1 场景1（pane → 新 tab）两种语义

Z1 区分了 `Alt-+`（pane 扩大到新 tab）和 `Alt-_`（pane 缩小到新 tab，原 tab 仍活动）。这是**两个 action**，不是带参数的同一个。

#### 3.1.1 共通前置

```
1. 取当前 pane 的实际几何：
   n := h.tab.GetNode(h.splitID)              // embedded *views.Node promoted
   srcX, srcY, srcW, srcH := n.X, n.Y, n.W, n.H
2. 保留 pane 引用：pane := h
```

#### 3.1.2 `Alt-+`（pane 扩大到整个新 tab）

```
a. 把当前 pane 从 src tab 拆出来：
   srcTab := h.tab
   srcTab.RemovePane(srcTab.GetPane(h.splitID))
b. 用 Node.Unsplit() 收 src tab 的 split tree 到只剩 1 个叶子：
   n.Unsplit()                                // 注意：本身就会 flatten 单孩子链
c. 包成新 tab（全屏尺寸 = pane 原尺寸意味着新 tab 就是那一块；建议直接 = src 原尺寸，让用户视觉一致）：
   newTab := NewTabFromPane(srcX, srcY, srcW, srcH, pane)
   Tabs.AddTab(newTab)
d. Tabs 整体 Resize（AddTab 内部已 t.Resize，但这里是 pane「搬家」，src tab 留下的 pane 需要重布局；走 MainTab().Resize() 让所有 tab 重新对齐节点几何）。
e. Tabs.SetActive(新 tab 索引)；若要「原 tab 仍活动」则跳过此步（Alt-_ 用）。
```

**坑点 A**：步骤 b 的 `Node.Unsplit()` 调用顺序 —— 必须在 `RemovePane` **之前**，否则 `RemovePane` 后 `tab.GetNode(h.splitID)` 拿不到节点（split tree 还在但 pane 引用已被移走，逻辑上不匹配）。参考 `actions.go:2039-2047` 原生 `Unsplit` 的顺序（先 `n.Unsplit()` 再 `tab.RemovePane` 再 `tab.Resize`）。**严格照抄这个顺序。**

**坑点 B**：步骤 c 用 `NewTabFromPane`，内部会把 pane 的 `SetID(t.ID())` 设成新 tab 的根 ID（= 1 之类的常量）。但 pane 之前是 src tab 里的某个叶子节点，**它的 splitID 是叶子节点的 id（不是 tab 根 ID）**。搬到新 tab 后 splitID 被覆盖成新 tab 的 ID 也没问题，因为新 tab 只有这 1 个 pane，整个 tree 就这一个叶子，ID = 根 ID，逻辑自洽。**无需额外保存旧 splitID。**

**坑点 C**：步骤 c 的几何如果用「全屏」`(0, 0, screenW, screenH-1)` 而不是 pane 原尺寸 `(srcX, srcY, srcW, srcH)`，会出现「pane 从屏幕中间某处瞬间跳到全屏」的视觉跳变。Z1 的语义是「pane 移出去并扩大」，所以**新 tab 应该是全屏尺寸**，旧 pane 留下的 src tab 也回全屏。这是 Z1 的产品决定，与「保持原位置」是两套语义。本文档按 Z1 语义（新 tab 全屏）写。

#### 3.1.3 `Alt-_`（pane 缩小成新 tab 的一半，原 tab 仍活动）

```
a–b 同上：拆 pane + Unsplit 收 src tab
c. 关键差异 —— pane 的几何「冻结为原 pane 尺寸」，新 tab 尺寸 = pane 原尺寸（视觉无跳变）：
   newTab := NewTabFromPane(srcX, srcY, srcW, srcH, pane)
   Tabs.AddTab(newTab)
d. Tabs.Resize()                              // 这里要让 src tab 也撑满全屏
e. 不动 Tabs.SetActive（Z1 要求 src tab 仍活动）。
```

**坑点 D**：步骤 d 的 `Tabs.Resize()` 会按 `len(t.List)` 决定布局（`tab.go:82-99`）：>1 tab 时每 tab 占 `Y=1, H=h-1-InfoBar`，≤1 tab 占 `Y=0, H=h-InfoBar`。**加完新 tab 后 src tab 的 Y 从 0 变 1、H 从 h 变 h-1-InfoBarOffset**，src tab 留下的那个 pane 需要重 Resize。`Tab.Resize()`（`tab.go:380`）会遍历 panes 对齐节点几何，所以 **srcTab.Resize() 必须显式调一次**，`Tabs.Resize()` 不会帮你做（它只调 `p.Resize()` = tab.Resize）。等等，再看一遍 —— `tab.go:89` 写的是 `p.Resize()`，不是 `p.Node.Resize`，所以**`Tabs.Resize` 已经会触发 srcTab.Resize**。OK 那 d 步骤只需要 `Tabs.Resize()`，不需要显式 `srcTab.Resize()`。

**坑点 E**：Z1 写「pane 缩小到新 tab」，但 `NewTabFromPane` 没有「缩小」概念 —— 它只接 (x,y,w,h)，传什么是什么。所以「缩小」语义其实是「pane 仍按原尺寸渲染在新 tab 内，新 tab 本身只占那一块」。这样 src tab 留下的 pane 反而**自动撑满整个 src tab 全屏**（因为 src tab 全屏、只剩它一个 pane）。这是 Z1 想要的「pane 缩小」含义吗？

- 读 Z1：「`Alt-_` 想把当前 paneA 从当前 tab0 里移出去，**缩小**变成一个新的 tab1，并且当前活跃 tab0 仍然是当前 tab」
- 字面理解：「缩小」指 paneA 变小。新 tab 只占 paneA 原大小，paneA 在新 tab 内仍占满（1 个 pane 时只能占满），所以 paneA 视觉上**不变小**。这跟「缩小」字面冲突。

可能的两种解读，需要在 Z1b（产品细化）或后续讨论里确认：
- 解读1（按字面）：paneA 缩到原尺寸的 50% 再被搬走。但搬走后只剩 1 个 pane，强行 50% 会留空白，UX 怪。
- 解读2（按效果）：新 tab 只占 paneA 原位置那块，src tab 留下的 pane 撑满。**paneA 的内容本身不变**，「缩小」指的是「它占的屏幕区域从全 tab 缩成一小块」（在新 tab 里）。
- 解读3：新增 `Alt-_` 时根本不应该「缩小」，因为语义跑不通。最干净的产品语义是「`Alt-_` = 把 pane 移到新 tab，原 tab 留下其它 pane，**新 tab 尺寸 = pane 原位置尺寸**」。

**本文档不替 Z1 选解读**，把这个语义未定项标记成 `[Z1 待确认]`。建议 Z1b 之前先在产品上对齐。

#### 3.1.4 边界

- 单 pane 时按 `Alt-+` / `Alt-_`：活动 pane 是 tab 里**唯一**的 pane，拆出去后 src tab 空了 → `Tabs.RemoveTab(srcTabID)`。这一步**必须**加，否则 src tab 变空 tab 留在 TabList 里。TabList.RemoveTab 已经能处理空 pane（`tab.go:62` 跳过 `len(p.Panes)==0` 的 tab），但反过来 —— src tab 留个空 tab 在 TabList 里是 bug，所以**主动 `RemoveTab`**。
- Unsplit 后 src tab 留 1 pane：剩余 pane 的 `splitID` 还是它原叶子 ID（`flatten()` 在根只剩 1 子时把子树提升为新根，root id 不变；其它叶子 id 也不变），tab.Panes 仍能 GetPane 找到。OK。

### 3.2 场景2（两个 tab → 一个 split tab）

Z1 写「`Alt-+` 因为当前 tab 只有一个 pane，没法再扩大，所以这个命令不起作用」—— 这是 `len(h.tab.Panes) == 1` 时的 no-op guard。`Alt-_` 则把当前 pane 跟相邻 tab 的 pane 合成上下 split。

#### 3.2.1 `Alt-+`（tab 只有 1 pane 时 no-op）

```
if len(h.tab.Panes) == 1 {
    InfoBar.Message("only 1 pane, no other pane to grow into")
    return false
}
```

多于 1 pane 时的语义 Z1 没写（多 pane 怎么 grow？grow 成全 tab 那就是场景1了）。所以**多于 1 pane 时也可以 no-op**，或干脆 `Alt-+` 在场景2的语义就和场景1的「pane → 新 tab」等价（活动 pane 搬走）。建议 Z1b 确认。

#### 3.2.2 `Alt-_`（合并当前 tab + 相邻 tab 成上下 split）

Z1 明确「上面是 paneA，下面是 tab1 里的那个 pane」—— 即当前 pane 在上、被合并的 tab 的 pane 在下；split 方向 = **HSplit**。

```
a. 找「相邻 tab」。Z1 没明说，但语义上是「和当前 tab 在视觉上接壤的下一个 tab」：
   curIdx := Tabs.Active()
   srcTab := Tabs.List[curIdx]
   // 「下一个」在 TabList 里 = curIdx+1（模 len），但视觉上可能不是接壤。
   // 简单起见：curIdx+1，若 curIdx 是最后一个则 curIdx-1（环回）。
   // 真要做得对，可能需要看 tab list 的视觉位置（tab bar 上相邻），但那是 Z1b 的事。
   otherIdx := curIdx + 1
   if otherIdx >= len(Tabs.List) { otherIdx = curIdx - 1 }
   if otherIdx == curIdx || otherIdx < 0 { /* 唯一 tab */ no-op return }
   otherTab := Tabs.List[otherIdx]

b. 各取一个 pane（多 pane 时选哪个？Z1 假设对方 tab 也只有 1 pane）：
   curPane := srcTab.CurPane()
   otherPane := otherTab.CurPane()

c. 把 otherPane 塞进 srcTab（在 curPane 下面）：
   srcTab.AddPane(otherPane, len(srcTab.Panes))  // 直接 append = 「下面」

d. 从 TabList 摘掉 otherTab：
   Tabs.RemoveTab(otherTab.Panes[0].ID())        // 必须先 c 后 d，否则 otherTab.Panes[0].ID() 拿到的是已搬走的 pane

e. Tabs.Resize()                                 // 重布局；srcTab 现在有 2 pane，需要 split tree + pane resize
```

**坑点 F**：步骤 c 之后 src tab 有 2 个 pane（curPane + otherPane），但 split tree **还是 1 个叶子**（Node 还是 root 就是叶子那种状态）。`Tabs.Resize()` 只触发 `tab.Resize()`，而 `tab.Resize()` 遍历 `tab.Panes` 调 `p.Resize`，但**不会自动建 split tree**。所以**必须在 c 之前或之后显式调 `Node.HSplit(bottom)` / `Node.VSplit(right)`**：

```
// 在 c 之前：
otherPane.splitID = srcTab.GetNode(curPane.splitID).HSplit(bottom=true)
// 在 c 中 AddPane 时用 srcTab.GetPane(otherPane.splitID) 拿正确 idx
```

或者更安全的顺序：**先建 split 节点（HSplit），再 AddPane**。`BufPane.HSplitBuf` 的写法（`bufpane.go:675-682`）就是这顺序的范本：

```
1. srcTab.GetNode(curPane.splitID).HSplit(bottom=true)  // 拆出新叶子
2. otherPane.SetID(<新叶子id>)
3. srcTab.AddPane(otherPane, <新 idx>)
4. Tabs.RemoveTab(otherTab.Panes[0].ID())
5. Tabs.Resize()
```

**坑点 G**：步骤 4 之前 `otherTab.Panes[0].ID()` 还能拿到原 tab 的 pane ID（因为 otherPane 的 splitID 在步骤 2 才改成 src tab 的新叶子 ID）。**必须在步骤 2 之前完成 RemoveTab**。要么用 `otherTab.Panes[0]` 在搬走前先记下原 ID，要么在 AddPane 后调 `RemoveTab(otherPane.OldID)` —— **最稳的做法**是先 `RemoveTab` 再搬 pane（顺序倒过来），因为 `Tabs.RemoveTab(id)` 通过 `p.Panes[0].ID() == id` 找 tab，而搬走前 otherPane 还是 otherTab.Panes[0]，逻辑上对：

```
1. Tabs.RemoveTab(otherTab.Panes[0].ID())      // 摘掉 otherTab，srcTab 不动
2. otherPane.SetTab(srcTab)                    // 切 pane 的 tab 归属
3. newLeafID := srcTab.GetNode(curPane.splitID).HSplit(bottom=true)
4. otherPane.SetID(newLeafID)
5. srcTab.AddPane(otherPane, len(srcTab.Panes))   // append 到末尾 = 下面
6. Tabs.Resize()
```

**坑点 H**：步骤 1 的 `Tabs.RemoveTab` 内部会 `t.SetActive(...)` 调整 active 索引（`tab.go:71`），如果 otherTab 在 curIdx 后面还好；如果在前面，curIdx 会偏移。**操作完后必须 `Tabs.SetActive(curIdx)`**（修正后的索引）。

**坑点 I**：方向。Z1 写「下面是 tab1 里的 pane」，即 HSplit（上下）。如果以后想支持「左边/右边」（VSplit），那 `Alt-=` / `Alt-_` 各自的方向要和场景1对齐，否则按键语义两套规则。**Z1b 一并决定**。

### 3.3 边界 / 风险

1. **microNeo 的「每 tab 最多 2 pane」不变量**：Z0 已经把 `VSplitBuf` / `HSplitBuf` 加了 `len(tab.Panes) >= 2` 主卡口（Z0 §3.1）。场景2 合并完恰好产生 2 pane，**不会撞卡口**（因为用的是 `Node.HSplit` 直接拆叶子，不是 `HSplitBuf`）。场景1 拆走 pane 后 src tab 留下 1 pane，也合规。✅
2. **`Tabs.Resize()` 的全 tab 列表遍历 + tab.Resize + Node.Resize**：`tab.go:82-99` 已实现；本功能的 Resize 时机参考 `command_neo.go:116, 145`（`AddTab` / `NewTabCmd` 后手动补一次 `MainTab().Resize()`）。**新 action 也必须在 AddTab / RemoveTab / 拆叶子之后调一次 `Tabs.Resize()`**。
3. **空 pane / 空 tab**：
   - 场景1 单 pane 拆走 → srcTab 空 → 必须 `Tabs.RemoveTab(srcTab.Panes[0].ID())`（移除空 tab），否则 TabList 里挂空 tab。
   - 场景2 otherTab 摘走 → 没了 → `Tabs.RemoveTab` 已处理（`tab.go:62` 跳过空 pane 那个查找循环，但 otherTab 不空，正常走删除路径）。
4. **pane 共享 buffer**：场景1 把 pane 搬到新 tab，buffer 不复制，pane 还是绑同一 buffer（`pane.Buf` 不变）。这点天然正确，因为 `NewTabFromPane` 不动 buffer。
5. **pane 共享导致「同一 buffer 在两个 tab 可见」**：场景1 搬完 paneA 到 tab1，tab0 没了 paneA，buffer 没复制也没移动，所以 tab1 看的 buffer 和 tab0 之前看的 buffer 是同一个。这正是用户想要的（移动 ≠ 复制），无需特殊处理。
6. **Z1 未定的语义**（需要 Z1b 决定）：
   - `Alt-_` 在场景1 的「缩小」具体是什么（见 3.1.3 解读 1/2/3）。
   - 场景2 的「相邻 tab」怎么选（环回上一个 / 视觉相邻 / 用户命令指定）。
   - 场景2 多 pane 时 `Alt-+` / `Alt-_` 各是什么意思。
   - 场景2 的 split 方向是否锁死 HSplit。

---

## 4. 涉及文件（如果走 Go action 路线）

| 文件 | 改动 |
|---|---|
| `internal/action/command_neo.go` | 加 2 个 `BufKeyAction` 方法：`PromotePaneAction`（场景1 `Alt-+`）、`MovePaneToTabAction`（场景1 `Alt-_`，暂用名）等；加 2 个：`MergeTabNoopAction`（场景2 `Alt-+` no-op）、`MergeTabsAction`（场景2 `Alt-_`）。`InitNeoBindings()` 注册 4 个到 `BufKeyActions` + 绑键。命名待 Z1b 决定。 |
| （无其它文件改动） | 全程复用 §1.3 列出的现成 API；不动 `VSplitBuf` / `HSplitBuf` 主卡口（场景1 拆 pane 走 `Node.Unsplit` 不走 `HSplitBuf`，场景2 合并走 `Node.HSplit` 也不走 `HSplitBuf`，所以 2-pane 不变量不被破坏）。 |
| `runtime/help/keybindings.md` | 在 Tabs / Splits 段落补充 4 个新 action 说明。 |

---

## 5. 不建议走 Lua 插件路线的原因

| 因素 | Go action | Lua 插件 |
|---|---|---|
| 改动位置 | `command_neo.go` 一文件 | 新建 `runtime/plugins/tabpane/*.lua` + README |
| split tree 操作 | 直接调 `Node.Unsplit` / `Node.HSplit` / `Node.VSplit` 等 Go 方法 | Lua 只能调导出的 Lua API，没有直接动 split tree 的能力，得绕 `Pane` / `BufPane` 公开方法 |
| Resize 时机 | 同步调，Go 内 `Tabs.Resize()` 立即生效 | Lua 里调 `p:Resize()` / `tab:Resize()` 需要 defer / 异步，否则事件循环外操作 UI 状态会出竞态 |
| 与 microNeo Resize 体系一致性 | 100% 一致（沿用 `command_neo.go:116` 模式） | 偏离，microNeo 的 Resize 时机约定只在 Go 代码里有 |
| 调试 | 编译期 + Go 调试器 | 纯运行时，stack trace 难 |

唯一适合 Lua 的场景是「不想动 Go 代码、纯配置侧加能力」，但 microNeo 已有 `command_neo.go` 这个专门承接 microNeo 特有行为的口子，**新功能放 Go 更合身**。

---

## 6. 实施顺序（出 PLAN 模式后，前提：Z1b 已敲定未定语义）

1. `command_neo.go`：写 `PromotePaneAction`（场景1 `Alt-+`，按 §3.1.2）→ `make build-quick` → 验证单 tab 多 pane 时按 Alt-+ 拆出新 tab，src tab 留下 pane 自动撑满，Tabs 列表多一项。
2. `command_neo.go`：写 `MovePaneToTabAction`（场景1 `Alt-_`，**先按 §3.1.3 的某一种解读跑通**）→ 验证。
3. `command_neo.go`：写 `MergeTabNoopAction` + `MergeTabsAction`（场景2，**先只支持 otherTab 也是单 pane 的简单情况**，按 §3.2.2 顺序）→ 验证当前 tab + 下一个单 pane tab 合并成上下 split。
4. `InitNeoBindings()`：注册 + 绑 `Alt-+` / `Alt-_` → 验证与默认 `Alt-,` / `Alt-.`（标签页切换）不冲突（它们不是 `+`/`_`）。
5. `runtime/help/keybindings.md`：补 Tabs / Splits 段落说明。
6. 边界回归：
   - 场景1 单 pane 按 Alt-+ → srcTab 变空 → 是否触发空 tab 自动摘除。
   - 场景2 合并后 srcTab 是唯一 tab → `Tabs.Resize` 走「`len==1`」分支（`tab.go:94`）是否正常。
   - 合并后再按 `Unsplit` / `Ctrl-w` / 鼠标拖分隔线 → 行为是否符合预期。
   - 合并后再 `Alt-t` 拆 pane → 是否撞 Z0 主卡口。