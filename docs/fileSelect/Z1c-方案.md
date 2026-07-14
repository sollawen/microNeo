# :big / :small —— 方案

需求来源：`Z1-tab切换.md`（4 个场景，含新增的场景 4）。底层原语：`Z1a-pane与tab互转-可行性调研.md`（结论：API 全在，只缺上层 action + 键位）。本文是落地设计。

---

## 1. 现状（已查证）

- micro 原生 pane↔tab 边界 action 0 个（Z1a §2.2 已 grep 全 0 命中）。
- microNeocurrently 每个 tab 最多 2 pane（`VSplitBuf`/`HSplitBuf` 主卡口，bufpane.go:689/694）。本方案沿用此不变量，**不会突破 2 pane 上限**。
- `TabList.AddTab(p *Tab)`（tab.go:52）/`RemoveTab(id uint64)`（tab.go:59）/`NewTabFromPane(x,y,w,h,pane Pane)`（tab.go:259）/`Tab.AddPane`/`Tab.RemovePane`（tab.go:353/373）/`Node.Unsplit()`（splits.go:467）/ `BufPane.SetTab`/`SetID`（bufpane.go:316/411）全部现成。
- microNeo 的 Resize 补刀 pattern（command_neo.go:112-117 / :141-146）：`Tabs.AddTab` 不会内部 Resize，加完新 Tab 后必须补 `MainTab().Resize()`。本方案在 4 处「跨 TabList 写」的地方都按这个 pattern 补。
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
| `Tabs.SetActive(i)` | `internal/action/tab.go:74` | 切 active tab |
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
- step 3 顺序：`Tab.RemovePane(idx)` → `Node.Unsplit()`。两者都只动各自数据结构、无依赖，但**先 RemovePane 再 Unsplit** 让 `Tab.Panes` 始终不出现「指向已删除 node」的脏状态，便于调试。
- step 5 必须在 step 4 之后：micro 原生 `Tabs.AddTab` 不内部 Resize，必须补（microNeo 已知 pattern，见 command_neo.go:112-117）。

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
    return h.HSplitAction()           // 场景 4：单 pane + 无其他 tab → HSplit 出空 pane
}

func extractPaneToNewTab(tab *Tab, h *BufPane, activate bool) bool {
    // step 1: capture（原 tab 的 idx 和 splitID，NewTabFromPane 会改写）
    idx       := tab.GetPane(h.splitID)
    oldSplitID := h.splitID

    // step 2: reparent —— 新 tab 全屏几何，含 h 一个 pane
    w, height  := screen.Screen.Size()
    iOffset    := config.GetInfoBarOffset()
    newTab     := NewTabFromPane(0, 0, w, height-iOffset, h)

    // step 4: 入 TabList（AddTab 不内部 Resize，下面 step 5 补）
    Tabs.AddTab(newTab)

    // step 3: 从原 tab 摘
    tab.RemovePane(idx)
    tab.GetNode(oldSplitID).Unsplit()
    tab.SetActive(0)                  // 原 tab 剩 1 pane，激活它；新 tab 不动则自然不 active

    // step 5: Resize 补刀（microNeo 已知 pattern，command_neo.go:112-117）
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

把另一个 tab 的一个 pane 整个搬到当前 tab。需要「move pane 而不是 copy」——`HSplitIndex` 内部 `NewBufPaneFromBuf` 会创建新 BufPane，**不是 move 语义**，故走裸路径：直接调 split tree + 手动挂 BufPane。

最棘手。把另一个 tab 的一个 pane 整个搬到当前 tab，需要「move pane 而不是 copy」：

```go
func (h *BufPane) absorbPaneIntoTab(srcPane *BufPane, srcTab *Tab) bool {
    // step 1: capture src
    srcIdx := srcTab.GetPane(srcPane.splitID)
    srcSplitID := srcPane.splitID

    // step 2: reparent —— srcPane.tab 指向当前 tab，srcPane.splitID 改成新 split tree 叶子 id
    //   用 HSplitIndex 但 buffer 复用 srcPane.Buf（不开新 BufPane，move 现有 pane）
    //   HSplitIndex 内部 NewBufPaneFromBuf 会创建新 BufPane，**不是我们想要的 move 语义**。
    //   故走裸路径：直接调 split tree + 手动挂 BufPane：
    newNodeID := h.tab.GetNode(h.splitID).HSplit(true)   // STVert=true → 上下分屏，新 pane 在下
    srcPane.SetTab(h.tab)
    srcPane.SetID(newNodeID)
    h.tab.AddPane(srcPane, h.tab.GetPane(h.splitID)+1)

    // step 3: 从 src tab 摘
    srcTab.RemovePane(srcIdx)
    srcTab.GetNode(srcSplitID).Unsplit()   // ⚠ splitID 已变（srcPane.SetID 后），但 srcTab.GetNode 用旧 ID 还找得到原节点（node 还在）
    if len(srcTab.Panes) == 0 {
        // 场景 2：src tab 空了 → 整 tab 删除
        //   RemoveTab 通过 pane id 删，所以删之前要记一个 src tab 里的 pane id
        Tabs.RemoveTab(srcTab.Panes[0].ID())   // ⚠ 此时 srcTab.Panes 已空，下面修复
    } else {
        srcTab.SetActive(0)
    }

    // step 5: Resize 补刀
    h.tab.Resize()
    srcTab.Resize()

    // 若 src tab 被删，Tabs.Resize 也需要触发
    Tabs.Resize()
    return true
}
```

上面有几个 bug，挑出来逐个修：

1. **`RemoveTab(id)` 接受 pane id 而非 tab id**（tab.go:59-77 实现：扫 `t.List` 找 `Panes[0].ID() == id` 的 tab 删），并且要求传入的 pane 仍在某 tab 的 `Panes[0]` 里。所以必须在 `srcTab.RemovePane(srcIdx)` 之前先抓一个 src tab 的 pane id：
   ```go
   var srcTabID uint64
   if len(srcTab.Panes) == 1 {   // 即将删 src tab，先记
       srcTabID = srcTab.Panes[0].ID()
   }
   ```

2. **`srcPane.SetID(newNodeID)` 之后**，原 `srcSplitID` 在 `srcTab` 的 split tree 里仍然指向一个孤儿 leaf（不在 srcTab.Panes 里了）。`srcTab.GetNode(srcSplitID)` 还能找到，因为 split tree 本身没动。`Unsplit()` 拆掉这个孤儿 leaf，OK。

3. **`srcTab.Panes` 在 `RemovePane` 后会 panic if out of bounds**：若 `srcIdx == len(srcTab.Panes)-1`，`copy(t.Panes[i:], t.Panes[i+1:])` 是空复制；然后 `t.Panes[len-1] = nil; t.Panes = t.Panes[:len-1]`，OK 不 panic。

4. **删除空 src tab 的逻辑放在 step 3 末尾**，先用 `srcTabID` 抓 id，再 RemovePane，再判断 `len(srcTab.Panes) == 0`：
   ```go
   var dyingTabID uint64
   if len(srcTab.Panes) == 1 {
       dyingTabID = srcTab.Panes[0].ID()
   }
   srcTab.RemovePane(srcIdx)
   srcTab.GetNode(srcSplitID).Unsplit()
   srcTab.SetActive(0)
   if dyingTabID != 0 {
       Tabs.RemoveTab(dyingTabID)
   }
   ```

**`pickAbsorbTarget` —— 选源 pane**

```go
func pickAbsorbTarget() (*BufPane, *Tab, bool) {
    // 遍历其他 tab，找「最近被激活的 pane」
    // 兜底（lastActive 未实现或全 0）：用 tab.Panes 的最后一个 pane（最近打开）
    var bestPane *BufPane
    var bestTab *Tab
    var bestTime time.Time
    for _, t := range Tabs.List {
        if t == MainTab() { continue }   // 跳过当前 tab
        for _, p := range t.Panes {
            bp, ok := p.(*BufPane)
            if !ok { continue }
            // 简化版：用 tab.Panes 顺序，最后一个 pane 视为最近；lastActive 全实现后再替换
            if bestPane == nil {
                bestPane = bp
                bestTab = t
                bestTime = time.Now()   // 占位，未来换成 lastActive
            }
        }
    }
    return bestPane, bestTab, bestPane != nil
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

2. **`:small` 场景 4 完全等价 `HSplit`**：直接 `return h.HSplitAction()`，确保「单 pane 无其他 tab 时」行为与原生 Ctrl-t 一致，**不绕过主卡口**（主卡口允许单 pane → 2 pane）。若主卡口未来升级（例如禁止单 pane tab），`:small` 场景 4 会同步受影响，符合预期。

3. **`tab.SetActive(0)` 兜底**：从原 tab 摘完 pane 后 `tab.active` 可能越界（RemovePane 不维护）。`SetActive(0)` 在 `len(tab.Panes) >= 1` 时合法——本方案所有调用点都保证 `len >= 1`（摘完剩 1 / 摘完剩 ≥1）。

4. **`pickAbsorbTarget` 用 tab.Panes 末位的简化**：场景 3（tab1 有 2 pane：paneB + paneC）时取末位 = paneC（最近打开）。如果用户先开 paneB、再开 paneC 但实际工作一直在 paneB，按本方案会吞 paneC，与「最近修改」语义不符。**这是已知简化**，对应 §3.3 的兜底逻辑。完美方案是给 BufPane 加 `lastActive time.Time`（Z1-扩展任务），`SetActive(true)` 时更新，`pickAbsorbTarget` 按 `lastActive` 排序取 max——实施成本 ~10 行，不在 Z1c 范围。

5. **多 pane tab（≥ 3）不会出现**：Z0 主卡口禁止。microNeo 不持久化 split 布局（Z0 §5.2），启动期 `InitTabs` 也不会建 ≥3 pane。唯一理论路径是 Lua 插件，但 Lua 也只能调导出的 `HSplitBuf`/`VSplitBuf`，撞主卡口。

6. **`absorbPaneIntoTab` 的 split 方向硬编码 HSplit**：场景 2/3 当前 tab 是 1 pane（无 parent），`n.Parent()` 为 nil，无法 mirror。**统一 HSplit（上下）**——和场景 4 的 HSplit 自然行为一致、和用户原话「上面的是 paneA，下面的是 tab1 里面的那个 paneB」吻合。

7. **删除空 src tab 的副作用**：`Tabs.RemoveTab` 内部会触发 `Tabs.SetActive` 和 `Tabs.Resize`（tab.go:59-77）。如果删除的是 `MainTab()` 自身（不可能：本方案排除「吞自己」），会破坏 invariant。`pickAbsorbTarget` 已 `t == MainTab() { continue }` 排除。

8. **buffer 共享语义**：本方案搬的是 **pane**（BufPane 实例），不是 buffer。原 src tab 的 pane 被整个搬到 dst tab，buffer 跟着 pane 走。src tab 里如果还有其他 pane 共享同一 buffer（少见但可能），不会受影响——本方案不动其他 pane。

9. **Pane.SetID 改 splitID 后，srcTab 内的旧 leaf node 是孤儿**：`Unsplit()` 拆的是旧 ID 在 split tree 里指向的 leaf。`GetNode(oldSplitID)` 此时仍能找到（split tree 没动），拆完旧 leaf 就消失，split tree 自洽。

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
