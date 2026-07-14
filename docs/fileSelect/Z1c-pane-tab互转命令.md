# :big / :small —— 方案

需求来源：`Z1-tab切换.md`（4 个场景，含新增的场景 4）。底层原语：`Z1a-pane与tab互转-可行性调研.md`（结论：API 全在，只缺上层 action + 键位）。本文是落地设计。

---

## 1. 现状（已查证）

- micro 原生 pane↔tab 边界 action 0 个（Z1a §2.2 已 grep 全 0 命中）。
- microNeocurrently 每个 tab 最多 2 pane（`VSplitBuf`/`HSplitBuf` 主卡口，bufpane.go:689/694）。本方案沿用此不变量，**不会突破 2 pane 上限**。
- `TabList.AddTab(p *Tab)`（tab.go:52）/`RemoveTab(id uint64)`（tab.go:59）/`NewTabFromPane(x,y,w,h,pane Pane)`（tab.go:259）/`Tab.AddPane`/`Tab.RemovePane`（tab.go:353/373）/`Node.Unsplit()`（splits.go:467）/ `BufPane.SetTab`/`SetID`（bufpane.go:316/411）全部现成。
- microNeo 的 Resize 补刀 pattern（command_neo.go:112-117 / :141-146）：`Tabs.AddTab` 内部已调 `t.Resize()`（tab.go:52），补一次 `MainTab().Resize()` 是幂等保险，确保跨 TabList 写后新旧 tab 几何都自洽。本方案在 4 处「跨 TabList 写」的地方都按这个 pattern 补。
- micro 原生 `Tab.Panes` 添加新 pane 时 `AddPane(pane, i)` 把新 pane 插到 `i = 当前 pane idx + 1`（HSplitIndex:678-683），所以**最后一个 pane 是最近打开的**（按加入顺序）。本方案用作 "most recent" 兜底（见 §3.3）。

---

## 2. 目标行为

把 `Z1-tab切换.md` 的 4 个场景统一成两条命令：

| 命令 | 触发条件 | 行为 | 主动 tab |
|---|---|---|---|
| `:big` | 当前 tab ≥ 2 pane（仅场景 1 适用） | 把活动 pane 拆出去，包成新 tab 占满，新 tab 激活 | 新 tab |
| `:big` | 当前tab = 1 pane 时 | no-op + 提示 "already at max" | — |
| `:small` | 当前 tab ≥ 2 pane（场景 1） | 把活动 pane 拆出去，包成新单 pane tab（原 tab 仍 active，新 tab 不 active） | 原 tab |
| `:small` | 当前 tab = 1 pane + 有其他 tab（场景 2/3） | 从另一个 tab 吞一个 pane 进当前 tab（HSplit），该 tab 减 1 pane；空 tab 删除 | 原 tab |
| `:small` | 当前 tab = 1 pane + 无其他 tab（场景 4） | 当前 tab HSplit 出新空 pane（等价原生 `HSplit`） | 原 tab |

`:big` / `:small` 在场景 1 是**同一个动作的两种姿态**——都是「把活动 pane 拆出去变新单 pane tab」，区别只在新 tab 是否激活：
- `:big`：新 tab 激活（paneA 被「promote」成焦点）
- `:small`：原 tab 仍 active（paneA 被「demote」到不 active 的新 tab，"缩小"成非焦点）

`:small` 的其他场景（2/3/4）才是真正的「折叠」语义：把外部 tab 折进当前 tab / 起一个新空 pane，让当前 tab 凑成 2-pane 对照模式。

不变量：
- 每 tab 仍 ≤ 2 pane（与 Z0 一致）
- 单 tab 永远是当前 tab，`:big` 是个例外（切到新 tab）
- 不开 birth selector：场景 1 拆出的 pane 自带 buffer、场景 4/吞入场景的新 pane 是 noName 空 buffer（用户 `:open` 即可）

---

## 3. 设计

### 3.1 设计总览：micro 原子 + microNeo 编排

**核心命题**：`:big` / `:small` 是把 micro 已有原子拼成跨 TabList 动作的编排。**不引入任何新底层原语**（不新加 split-tree / Tab / Pane 任何一个数据结构的方法），但其中 1 个场景（`:small` 场景 2/3 吸收）需要 5-6 个原子按特定顺序串，还附带一段 micro 也没有的业务策略（选哪个 pane 吞）。microNeo 这一层加 3 个编排 helper + 2 个 BufKeyAction + 2 个 command。

#### 分层依赖（按场景）

| 场景 | 调用方式 | 底层原子数 | 有无 microNeo 业务策略 |
|---|---|---|---|
| `:big` 场景 1 | `extractPaneToNewTab(activate=true)` | 3 个（`NewTabFromPane` + `AddTab` + `RemovePane`/`Unsplit`） | 无 |
| `:small` 场景 1 | `extractPaneToNewTab(activate=false)` | 同上 | 无 |
| `:small` 场景 2/3 | `absorbPaneIntoTab` + `pickAbsorbTarget` | **6 个**（`HSplit` + `SetTab`/`SetID` + `AddPane` + `RemovePane` + `Unsplit` + `RemoveTab`） | **有**（`pickAbsorbTarget`：从其他 tab 选哪个 pane 吞） |
| `:small` 场景 4 | `h.HSplitAction()` | 1 个（原生 action） | 无 |

场景 1/4 是**简单声明式组合**，几乎是一行「原子串」。场景 2/3 吸收是**唯一需要复杂编排 + 业务策略**的地方——也是 micro 没有现成原子的唯一地方（micro 不提供「跨 tab 移动 pane」这种动作，因为原生需求里就没有）。

#### micro 原子（直接复用，一行不改，仅在场景 2/3 组合使用）

| 原子 | 出处 | 用途 |
|---|---|---|
| `NewTabFromPane(x,y,w,h,pane)` | `internal/action/tab.go:259` | 把 pane 装进新 tab（内部调 `pane.SetTab`+`pane.SetID`） |
| `NewTabFromBuffer(x,y,w,h,b)` | `internal/action/tab.go:246` | 同上但创 buffer |
| `Tabs.AddTab(p)` | `internal/action/tab.go:52` | TabList 加 tab |
| `Tabs.RemoveTab(id)` | `internal/action/tab.go:59` | TabList 按 pane id 删 tab |
| `TabList.SetActive(a)` | `internal/action/tab.go:152` | 切 active tab（TabList 层，区别于 `Tab.SetActive` tab.go:341）|
| `Tab.AddPane(pane,i)` | `internal/action/tab.go:353` | Tab.Panes 插入 |
| `Tab.RemovePane(i)` | `internal/action/tab.go:373` | Tab.Panes 删除 |
| `Node.HSplit(bottom)` | `internal/views/splits.go:415` | split tree 加叶子（上下） |
| `Node.VSplit(right)` | `internal/views/splits.go:431` | split tree 加叶子（左右） |
| `Node.Unsplit()` | `internal/views/splits.go:467` | split tree 删叶子 |
| `BufPane.SetTab(t)` | `internal/action/bufpane.go:316` | 改 pane 归属 |
| `BufPane.SetID(id)` | `internal/action/bufpane.go:411` | 改 pane splitID |
| `Tab.Resize()` | `internal/action/tab.go:380` | panes 对齐到 split tree 几何 |
| `HSplitAction()` | `internal/action/actions.go:2032` | 场景 4 一行调用 |

`views/splits.go` 的 `Parent()`（splits.go:126）已导出，不需加访问器（Z0 §3.3 加过）。

#### microNeo 新增（仅 command_neo.go 内部）

| 名称 | 性质 | 是否纯编排 / 含业务策略 |
|---|---|---|
| `BigPane` / `SmallPane` | BufKeyAction（顶层路由） | 仅场景分支 |
| `BigCmd` / `SmallCmd` | command 签名包装 | — |
| `extractPaneToNewTab(tab, h, activate)` | 私有 helper（场景 1 共用） | **纯编排**，3 个 micro 原子串起来 |
| `absorbPaneIntoTab(srcPane, srcTab)` | 私有 helper（场景 2/3） | **含业务策略**——指定 5-6 个原子的调用顺序 + delete-empty-tab 边缘处理；micro 没有对应原子 |
| `pickAbsorbTarget()` | 私有 helper（选源 pane） | **纯业务策略**——「从其他 tab 选哪个 pane 吞」micro 根本没这个概念，microNeo 自行决定（取 tab.Panes 末位作为简化） |
| `commands["big"]` / `commands["small"]` | InitNeoCommands 注册 | — |

`absorbPaneIntoTab` + `pickAbsorbTarget` 是本方案中**唯一不能简化为「调单个 micro 原子」的部分**。如果未来 micro 官方加一个 `Tab.MovePaneTo(srcPane, dstTab, position)` 原生 action，本方案的这两 helper 可以直接删掉、改为单行调用。但目前 micro 没这个 action（Z1a §2.2 已 grep 确认），所以本方案就位是必要的。

**0 改动**：`bufpane.go`（Z0 主卡口就位）、`splits.go`（`Parent()` 已导）、`cmd/micro/micro.go`。

#### 跨 TabList 写的 5 步通用数据流

不论 `:big` 还是 `:small`，底层动作都是「把一个 BufPane 从 tab A 的 split tree 摘下来，挂到 tab B（可能是新建 Tab）的 split tree」。每一**步**都是一个或多个上表中的 micro 原子：

```
1) capture  ：在原 tab 记 pane idx 和 split tree node id（纯读取，无副作用）
2) reparent：NewTabFromPane(...) 或手调 SetTab+SetID（调 micro 原子）
3) remove   ：Tab.RemovePane(idx) + Node.Unsplit()（两个 micro 原子）
4) append   ：Tabs.AddTab(新 tab) 或 Tab.AddPane(已存在 tab)（调 micro 原子）
5) resize   ：Tab.Resize() / Tabs.Resize()（调 micro 原子）
```

顺序约束：
- step 1 必须在 step 2 之前：`NewTabFromPane` 调 `pane.SetTab(t)` 和 `pane.SetID(t.ID())`，一旦执行，从原 tab 视角就找不到这个 pane 了（id 已变）。
- **step 3（摘）必须在 step 4（AddTab）之前**：`NewTabFromPane`（step 2）调 `SetID(t.ID())` 把 h.splitID 改成 newTab root id，但 h 此刻仍在原 tab.Panes。若先 AddTab，其内部 `TabList.Resize`（tab.go:54）遍历到原 tab 时 `原tab.GetNode(h.ID())` 返回 nil（原 tab split tree 里没有 newTab root id）→ `Tab.Resize` 里 `n.X`（tab.go:385）nil deref。必须先 RemovePane 把 h 从原 tab.Panes 摘掉、Unsplit 让原 tab root `flatten` 成剩余叶子的 id，再 AddTab。
- step 3 顺序：`Tab.RemovePane(idx)` → `Node.Unsplit()`。两者都只动各自数据结构、无依赖，但**先 RemovePane 再 Unsplit** 让 `Tab.Panes` 始终不出现「指向已删除 node」的脏状态，便于调试。
- step 5 必须在 step 4 之后：`Tabs.AddTab` 内部已调 `t.Resize()`（tab.go:52），但补一次 `MainTab().Resize()` 是幂等保险，确保跨 TabList 写后几何自洽（microNeo 已知 pattern，见 command_neo.go:112-117）。

### 3.2 `:big` —— PromotePane

`:big` 完整代码见 §3.3（与 `:small` 共享 `extractPaneToNewTab(activate=true)`）。这里只讲为什么这么写。

边界：
- **场景 2/3/4**（单 pane）：`len(tab.Panes) < 2` → 提示 + return。这条判断必须**先于** capture，否则单 pane 时 `tab.GetNode(h.splitID)` 拿到的是 root 叶子（`parent == nil`），后续 `Unsplit` 会因 `len(n.parent.children) <= 1`（splits.go:193）返回 false，不崩但不做事，留个 root 单叶子残留——所以早返更干净。
- **剩余 pane 焦点**：从原 tab 摘完 pane 后，原 tab 还剩 1 个 pane。`tab.RemovePane` 后 `tab.active` 若越界显式 `tab.SetActive(0)` 兜底（场景 1 唯一合法的情况）。
- **新 tab 全屏几何**：`screen.Screen.Size() - InfoBarOffset`，跟 `AddTab`（actions.go:1976）的算法一致。`Tab.Resize` 会重写 geometry，无需预判 tab bar 是否显示。

### 3.3 场景 1：`extractPaneToNewTab`（`:big` / `:small` 共享）

场景 1 里 `:big` 和 `:small` 共享 `extractPaneToNewTab`，仅 `activate` 参数不同：

```go
func (h *BufPane) BigPane() bool {
    tab := h.tab
    if len(tab.Panes) < 2 {           // 单 pane 是「已最大」，其它场景都走这里 short-circuit
        InfoBar.Message("pane already at max")
        return false
    }
    return extractPaneToNewTab(tab, h, /* activate = */ true)
}

func (h *BufPane) SmallPane() bool {
    tab := h.tab
    if len(tab.Panes) >= 2 {           // 场景 1：拆 pane → 新单 pane tab
        return extractPaneToNewTab(tab, h, /* activate = */ false)
    }
    if otherPane, otherTab, found := pickAbsorbTarget(); found {   // 场景 2/3
        return absorbPaneIntoTab(otherPane, otherTab)
    }
    return h.neoHSplitAction()        // 场景 4：单 pane + 无其他 tab → 复用 :hsplit 包装（HSplit + OpenBirthSelector）
}

func extractPaneToNewTab(tab *Tab, h *BufPane, activate bool) bool {
    // step 1: capture（原 tab 的 idx 和 splitID，NewTabFromPane 会改写）
    idx       := tab.GetPane(h.splitID)
    oldSplitID := h.splitID

    // step 2: reparent —— 新 tab 全屏几何，NewTabFromPane 调 SetTab/SetID 把 h 归属改到 newTab
    w, height  := screen.Screen.Size()
    iOffset    := config.GetInfoBarOffset()
    newTab     := NewTabFromPane(0, 0, w, height-iOffset, h)

    // step 3: 先从原 tab 摘 h + Unsplit 旧叶子，再 AddTab
    //   顺序硬约束：AddTab 内部调 TabList.Resize 遍历所有 tab 调 Tab.Resize，
    //   若 h 还在原 tab.Panes（且 h.ID 已被 NewTabFromPane 改成 newTab root id），
    //   原 tab.GetNode(h.ID()) 返回 nil → Resize 里 n.X nil deref。
    //   Unsplit 后原 tab root flatten 成剩余叶子的 id，剩余 pane.ID() 仍能 GetNode 命中。
    tab.RemovePane(idx)
    tab.GetNode(oldSplitID).Unsplit()
    tab.SetActive(0)                  // 原 tab 剩 1 pane，激活它

    // step 4: 入 TabList（内部已调 Resize；此时原 tab 已无 h，遍历安全）
    Tabs.AddTab(newTab)

    // step 5: Resize 补刀（幂等保险）
    newTab.Resize()
    tab.Resize()

    if activate {
        Tabs.SetActive(len(Tabs.List) - 1)
    }
    return true
}
```

`:big` 和 `:small` 在场景 1 的差异**只有最后两行**：
- `:big` → `activate=true` → 切到新 tab（paneA 成为焦点，promote）
- `:small` → `activate=false` → 原 tab 仍 active（paneA 被 demote 到不 active 的新 tab）

新 tab 里**只有 paneA 一个 pane**，不开空 companion（与原 Z1 描述对齐；与 `:small` 场景 4 的「开空 pane」是两件事）。

### 3.4 场景 2/3：`absorbPaneIntoTab` + `pickAbsorbTarget`

「move pane 而不是 copy」意味着不调 `HSplitIndex`（它第一行 `NewBufPaneFromBuf` 会造新 BufPane，是 copy 语义），而是退到底层拆开做：split tree 的 `Node.HSplit` 建新叶子 + §3.1 的 `SetTab`/`SetID` 改现有 pane 归属 + `AddPane` 登记。「手动挂 BufPane」就是 `SetTab` 改归属那一对原子，不是另造新方法。

真正棘手的不是缺改归属的原子（micro 现成），而是 micro 没有「跨 tab 移 pane」的单一原子，要把 6 个原子按特定顺序串起来，还夹着「空 src tab 删除」的边缘处理——`Tabs.RemoveTab` 靠 `Panes[0].ID()` 定位（tab.go:59-77），删不掉 Panes 已空的 tab，所以 RemoveTab 的时机是本节最硬的约束。

```go
func (h *BufPane) absorbPaneIntoTab(srcPane *BufPane, srcTab *Tab) bool {
    // step 1: capture src
    srcIdx := srcTab.GetPane(srcPane.splitID)
    srcSplitID := srcPane.splitID

    // step 2: reparent —— 当前 tab split tree 建下叶子，srcPane 挂过去
    //   HSplit(true) 把 root 叶子变 STVert：原 id 保留为上叶子(h 仍在上面)，返回下叶子 newid
    newNodeID := h.tab.GetNode(h.splitID).HSplit(true)
    srcPane.SetTab(h.tab)                                  // 改归属
    srcPane.SetID(newNodeID)                               // 改 splitID 指向新叶子
    h.tab.AddPane(srcPane, h.tab.GetPane(h.splitID)+1)     // 插到 h(idx 0) 之后

    // step 3: 从 src tab 摘
    if len(srcTab.Panes) == 1 {
        // 场景 2：src tab 单 pane，整 tab 删除
        //   必须在 RemovePane 之前 RemoveTab：RemoveTab 靠 Panes[0].ID() 定位，
        //   此时 srcTab.Panes[0] 还是 srcPane(ID=newNodeID)，能匹配；
        //   若先 RemovePane 则 Panes 空，RemoveTab 的 `len==0 continue` 跳过，删不掉
        Tabs.RemoveTab(srcPane.ID())   // = newNodeID
        //   tab 整个删掉，split tree + Panes 随之 GC，无需手动 Unsplit/RemovePane/SetActive
    } else {
        // 场景 3：src tab 2 pane，摘 srcPane 留 tab
        srcTab.RemovePane(srcIdx)
        srcTab.GetNode(srcSplitID).Unsplit()   // 拆 srcPane 旧叶子，root flatten 成单叶子
        srcTab.SetActive(0)
        srcTab.Resize()
    }

    // step 5: Resize 补刀
    h.tab.Resize()
    Tabs.Resize()
    return true
}
```

关键时序约束（逐个原子核对源码后）：

- **`Node.HSplit(true)` 在 root 叶子上**（splits.go:415→vHSplit:343）：root 变 STVert 内部节点，原叶子 id 保留为上叶子 hn1、新 id 返回为下叶子 hn2。所以 `h.splitID` 仍指向叶子 hn1，`GetNode(h.splitID)` 不会返回 nil，`Tab.Resize` 遍历 Panes 时 `GetNode(p.ID())` 对 h 和 srcPane 都命中叶子，不 panic。
- **`Tabs.RemoveTab(id)` 删不掉空 tab**（tab.go:59-77 `if len(p.Panes) == 0 { continue }`）：必须趁 src tab 还有 pane 时调。场景 2 在 RemovePane 之前 RemoveTab，此时 `srcTab.Panes[0]` 还是 srcPane、`ID()==newNodeID` 能匹配自己。
- **无需预存 `dyingTabID`**：`BufPane.ID()` 返回 `h.splitID`（bufpane.go:406），`SetID(newNodeID)` 后 srcPane.ID() 即 newNodeID。直接在 RemovePane 之前调 `RemoveTab(srcPane.ID())`，当前值即匹配值。文档早期方案想预存 dyingTabID，但若在 SetID 之后抓会抓到 newNodeID、RemovePane 之后再 RemoveTab 又遇空 tab 被跳过，两头落空。
- **`Unsplit()` 要求 `n.parent != nil`**（splits.go:469）：场景 3 src tab 2 pane 时 srcSplitID 是非 root 叶子，正常拆；场景 2 单 pane 时 srcSplitID 是 root 叶子（parent==nil），Unsplit 返回 false 是 no-op——但场景 2 走 RemoveTab 整 tab 删，split tree 随 tab 一起 GC，无需 Unsplit。
- **`SetID` 不动 split tree**：场景 3 里 `srcPane.SetID(newNodeID)` 只改 pane 字段，srcTab split tree 里原 srcSplitID 叶子还在，`GetNode(srcSplitID)` 仍按树遍历找到它（splits.go:130），`Unsplit` 才能拆掉。

**`pickAbsorbTarget` —— 选源 pane**

```go
func pickAbsorbTarget() (*BufPane, *Tab, bool) {
    // 取 Tabs.List 末位非当前 tab 的 tab，再取其 Panes 末位 pane（最近打开）。
    // 简化启发式：HSplitIndex 总 append 到末尾，末位 pane = 最近打开。
    // 未来升级为 lastActive 排序（给 BufPane 加 lastActive 字段，SetActive(true) 时更新取 max）。
    for i := len(Tabs.List) - 1; i >= 0; i-- {
        t := Tabs.List[i]
        if t == MainTab() { continue }   // 跳过当前 tab
        if len(t.Panes) == 0 { continue }
        bp, ok := t.Panes[len(t.Panes)-1].(*BufPane)
        if !ok { continue }
        return bp, t, true
    }
    return nil, nil, false
}
```

简化版：用 tab.Panes 的最后一个 pane（HSplitIndex 总是 append 到最后，见 §1 末）。未来升级 `lastActive` 见 §5.5。

### 3.5 场景 4：一行 HSplitAction

直接 `h.HSplitAction()`（原生 actions.go:2032 → HSplitBuf → 主卡口放行后 HSplitIndex 建 pane），零代码。主卡口在 `HSplitBuf` 已经处理了「单 pane 时正常建第 2 pane」的情况，不需要额外 if。

### 3.6 注册：命令 + 可选键位

仿 command_neo.go::InitNeoCommands + InitNeoBindings：

```go
// InitNeoCommands（追加）
BufKeyActions["BigPane"]   = (*BufPane).BigPane
BufKeyActions["SmallPane"] = (*BufPane).SmallPane
commands["big"]   = Command{(*BufPane).BigCmd, nil}
commands["small"] = Command{(*BufPane).SmallCmd, nil}

// BigCmd / SmallCmd 是满足 MakeCommand 签名的薄包装（与 QuitNeoCmd 同模式）
func (h *BufPane) BigCmd(args []string)   { h.BigPane() }
func (h *BufPane) SmallCmd(args []string) { h.SmallPane() }

// InitNeoBindings（追加，若需要默认键位）
// 默认不绑键：用户可自定义。如要绑，候选：
//   Alt-Shift-= → BigPane（"变大"的视觉隐喻；注意 Alt-Shift-+ = Alt-Shift-= 在 tcell 下绑得上）
//   Alt-Shift-- → SmallPane
// 但这两个键 macOS 默认 Option-Shift-= 是 § 符号、Option-Shift-- 是 — 符号，
// 终端行为各异；本方案不强推默认键，由用户在 init.lua 或 settings.json 自行绑。
```

**不绑键的理由**：`:big` / `:small` 是新原语，键位需要和 Ctrl-t、Ctrl-q 等已用键不冲突，候选空间有限，留给用户按习惯配。

### 3.7 命名解释（为什么叫 big/small）

`:big` / `:small` 描述的是**活动 pane 的「显隐状态」**，不是字面尺寸：

- `:big` = pane 被 promote 到 active tab（可见 = 大）
- `:small` = pane 被 demote 到非 active tab（被遮挡 = 小）

场景 1 拆出来的新 tab 永远只含 1 个 pane（paneA），两种命令产出**完全一样的 TabList 拓扑**，唯一区别在 active tab。`:small` 的 2/3/4 场景才是「凑成 2-pane 对照模式」，与命名无关。

---

## 4. 涉及文件

| 文件 | 改动 |
|---|---|
| `internal/action/command_neo.go` | 新增 `BigPane` / `SmallPane` / `BigCmd` / `SmallCmd` 方法 + `extractPaneToNewTab` / `absorbPaneIntoTab` / `pickAbsorbTarget` 私有 helper + `InitNeoCommands` 注册 commands + `InitNeoBindings`（可选，键位由用户配） |
| `internal/action/bufpane.go` | 不动（主卡口在 `VSplitBuf`/`HSplitBuf` 已就位） |
| `internal/views/splits.go` | 不动（`Parent()` 已导出，splits.go:126） |
| （无新文件、无 `cmd/micro/micro.go` 改动） | 复用 `NewTabFromPane`/`AddTab`/`RemoveTab`/`Unsplit`/`AddPane`/`RemovePane`/`SetTab`/`SetID` |

---

## 5. 边界 / 风险

1. **`:big` 在单 pane tab 误触**：场景 2/3/4 入口即被 `len(tab.Panes) < 2` 拦下，提示 "pane already at max"（与 Z0 Alt-= 文案对齐风格）。若未来想用 `:big!` 强制（破坏不变量），需另开方案。

2. **`:small` 场景 4 复用 `:hsplit` 包装**：`return h.neoHSplitAction()`（HSplit + OpenBirthSelector），不是裸 `h.HSplitAction()`。裸 HSplitAction 不开 birth selector、不置 `isNoName`，新 pane 退出时也走不到 quit selector——场景 4 必须和 `:hsplit` 命令走同一条包装路径。**不绕过主卡口**（neoHSplitAction 内 `len>=2` 卡口与 HSplitBuf 一致，允许单 pane → 2 pane）。若主卡口未来升级（例如禁止单 pane tab），`:small` 场景 4 会同步受影响，符合预期。

3. **`tab.SetActive(0)` 兜底**：从原 tab 摘完 pane 后 `tab.active` 可能越界（RemovePane 不维护）。`SetActive(0)` 在 `len(tab.Panes) >= 1` 时合法——本方案所有 SetActive 调用点都保证 `len >= 1`：`extractPaneToNewTab` 摘完剩 1、`absorbPaneIntoTab` 场景 3 摘完剩 1；场景 2 走 `RemoveTab` 整 tab 删除提前 return，不在空 Panes 上调 SetActive。

4. **`pickAbsorbTarget` 用 tab.Panes 末位的简化**：场景 3（tab1 有 2 pane：paneB + paneC）时取末位 = paneC（最近打开）。如果用户先开 paneB、再开 paneC 但实际工作一直在 paneB，按本方案会吞 paneC，与「最近修改」语义不符。**这是已知简化**，对应 §3.3 的兜底逻辑。完美方案是给 BufPane 加 `lastActive time.Time`（Z1-扩展任务），`SetActive(true)` 时更新，`pickAbsorbTarget` 按 `lastActive` 排序取 max——实施成本 ~10 行，不在 Z1c 范围。

5. **多 pane tab（≥ 3）不会出现**：Z0 主卡口禁止。microNeo 不持久化 split 布局（Z0 §5.2），启动期 `InitTabs` 也不会建 ≥3 pane。唯一理论路径是 Lua 插件，但 Lua 也只能调导出的 `HSplitBuf`/`VSplitBuf`，撞主卡口。

6. **`absorbPaneIntoTab` 的 split 方向硬编码 HSplit**：场景 2/3 当前 tab 是 1 pane（无 parent），`n.Parent()` 为 nil，无法 mirror。**统一 HSplit（上下）**——和场景 4 的 HSplit 自然行为一致、和用户原话「上面的是 paneA，下面的是 tab1 里面的那个 paneB」吻合。

7. **删除空 src tab 的副作用**：`Tabs.RemoveTab` 内部会触发 `Tabs.SetActive` 和 `Tabs.Resize`（tab.go:59-77）。如果删除的是 `MainTab()` 自身（不可能：本方案排除「吞自己」），会破坏 invariant。`pickAbsorbTarget` 已 `t == MainTab() { continue }` 排除。

8. **buffer 共享语义**：本方案搬的是 **pane**（BufPane 实例），不是 buffer。原 src tab 的 pane 被整个搬到 dst tab，buffer 跟着 pane 走。src tab 里如果还有其他 pane 共享同一 buffer（少见但可能），不会受影响——本方案不动其他 pane。

9. **Pane.SetID 改 splitID 后，srcTab 内的旧 leaf node 是孤儿**：`SetID` 只改 pane 字段、不动 split tree，`GetNode(oldSplitID)` 仍按树遍历找到原叶子（splits.go:130）。场景 3 该叶子是非 root（`parent != nil`），`Unsplit()` 正常拆掉、root flatten 成单叶自洽。场景 2 单 pane src tab 的旧叶子是 root（`parent == nil`），`Unsplit()` 返回 false 是 no-op——但场景 2 走 `RemoveTab` 整 tab 删除，split tree 随 tab 对象一起 GC，旧叶子不残留。

---

## 6. 实施顺序（出 PLAN 模式后）

1. `command_neo.go`：加 `BigPane` / `SmallPane` / `BigCmd` / `SmallCmd` 4 个方法 + 3 个私有 helper（`extractPaneToNewTab` / `absorbPaneIntoTab` / `pickAbsorbTarget`），**先 stub `pickAbsorbTarget` 返回 last-pane**，保证编译过。
2. `command_neo.go::InitNeoCommands` 注册 `commands["big"]` / `commands["small"]` 和 `BufKeyActions["BigPane"]` / `BufKeyActions["SmallPane"]`。
3. `make build-quick`，冒烟：
   - 场景 4：开 microNeo 进任意文件 → `:small` → 当前 tab 应得上下两 pane，下 pane 是空 noName。
   - 场景 2：`:tab a.txt` 后 `:tab b.txt` 切到 a → `:small` → 当前 tab 应有 a 上、b 下两 pane，b 那个 tab 消失。
   - 场景 1：场景 2 完成后（2 pane）→ `:small` → a 拆出成新 tab（新 tab **只有 a 单 pane**，原 tab 仍 active，内有 b 单 pane）。同一状态再 `:big` → 新 tab 应变 active（paneA 成为焦点），TabList 拓扑不变。
   - 场景 3：`:tab a.txt` 后 `:tab b.txt c.txt` 切到 a → `:small` → 当前 tab 应有 a 上 + c（最近打开）下，b tab 仍存在。
   - 场景 1 + `:big`：从场景 1 状态切到那个 2-pane tab → 选 a → `:big` → a 应变新全屏 tab，新 tab active，原 tab 还剩 b。
   - `:big` 在单 pane tab → 提示 "pane already at max"，无副作用。
4. 验证 multi-pane → multi-tab 双向不变量：
   - 场景 1 + `:big` → 场景 2 + `:small` 来回切几次，几何无残留、`tab.Panes` 与 split tree 一致。
   - 反复 `:small` + `:big` 直到所有 pane 都进单 pane tab（场景 4 状态），再 `:small` 应回到 2 pane。
5. （可选）`:big` / `:small` 加默认键位，写 `runtime/help/keybindings.md` 一段。
6. （未来扩展）`BufPane.lastActive` 跟踪 + `pickAbsorbTarget` 升级：单独立项，本方案不动。
