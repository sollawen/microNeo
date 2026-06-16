# 方案 B 评估报告（修正版）

> 评估对象：`docs/光标滚动-方案B.md`（displayToBuffer + screenBuffer + 时序统一）
> 关联：`docs/光标滚动-方案A.md`、`docs/方案A架构设计-很臭的方案.md`、`docs/光标滚动-修改总结.md`
> 背景：原以为 A 简单、分析后发现 A 也麻烦 → 倾向 B。本报告回答「B 到底有多复杂」。
> 修订：2026-06-16 — 经用户七轮反驳后修正（渲染层成本、2x cap、分发点位置、dirty 标记是否存在、screenRow 同源元数据、case C 无迭代、showCursor 按 intent 分类 七处原评估有误，已更正）

---

## 0. 一句话结论（修正后）

**方案 B 可行，架构上比 A 干净（单一数据源），且经过七轮澄清后无任何架构性难点。实际代码量是 A 的 ~1.4 倍（~150 行 vs ~105 行），但每帧渲染次数比 A 更少、无 shadow map 同步负担。两条路都走得通，§6 给出最终取舍。**

本报告初版判定「B 复杂 5～7 倍、3 个致命缺陷」，经用户七轮反驳后**全部推翻**。修正过程留在 §5，供复盘。

---

## 1. 用户的三轮反驳（修正依据）

### 1.1 反驳①：screenBuffer = tcell SimulationScreen，渲染层迁移几乎免费

**原评估**：screenBuffer 要新设计 4000 cell/帧的结构，renderSegmentNative 改写入目标 ~80 行。

**事实**：`internal/screen.Screen` 是 `tcell.Screen` **interface** 类型的全局变量（`screen.go:19`），`screen.SetContent` 就是转发到 `Screen.SetContent`。tcell 自带 `simulation.go`（`NewSimulationScreen`）——现成的内存版 Screen 实现。

→ renderSegmentNative 里 4 处 `screen.SetContent(...)` 改成 `sb.SetContent(...)`（sb 作为参数传入）是**机械替换，~8 行**。renderer 逻辑零改动。

### 1.2 反驳②：2x viewport cap 解决 screenBuffer 尺寸问题

**原评估**（缺陷①，曾判为「致命未解」）：case A 用 `oldStartLine` 渲染的 screenBuffer 只覆盖到 `oldStartLine + bufHeight`，Relocate 算出 `newStartLine > oldStartLine` 时 ShowBuffer 越界。

**事实推演**（典型 bug 场景，sample.md line55-59 table）：
- cursor 在 line59（table native 渲染，占 5 行），按 ↓ 到 line60
- table 切换为渲染模式，凭空多 4 个装饰行
- old map 里 cursor 屏行 ≈ bufHeight 附近；new map 里因上方 table 涨了 4 行，cursor 屏行 ≈ bufHeight+4
- `newStartLine - oldStartLine` ≈ 1 + scrollmargin（个位数），远小于 bufHeight

→ **screenBuffer 渲染 `2 * bufHeight`，留一整个 bufHeight 的余量，覆盖这种「旧段重展开」的增长绰绰有余**。极端 case（Goto 跳很远）超出 2x 才触发，退化成 fallback。

### 1.3 反驳③：分发点已在 MD 代码里，根本不用改 micro.go / actions.go

**原评估**（缺陷②，曾判为「原则级违规」）：B 的 case A/B/C 分发要改 DoEvent 主循环或 76 处 Relocate。

**事实**：代码核查后发现，micro 已经有两个 MD 分发点（现状就是 microNeo 的接入缝）：

```
bufwindow.go:249  Relocate()   if w.Buf.IsMD → relocateVerticalMD()
bufwindow.go:940  Display()    if w.Buf.IsMD → displayBufferMD()
```

方案 B 只需把这两个 MD 分支的**实现**换掉：
- `displayBufferMD` → 改名/改造成 **showBuffer**（blit screenBuffer，不再 render）
- `relocateVerticalMD` 入口 → 加 **displayToBuffer**（render 到 screenBuffer）

走一遍时序（DoEvent / actions.go 完全不改）：

```
帧N-1 handleEvent → Relocate() → relocateVerticalMD()
    └─ displayToBuffer(oldStartLine, showCursor)   ← render 到 screenBuffer
    └─ 用 fresh screenBuffer 算 StartLine=L1
帧N   Display()    → displayBufferMD() → showBuffer(L1)   ← cheap blit
帧N   screen.Show()  用户看到
帧N   handleEvent → Relocate() → displayToBuffer(...) → StartLine=L2
帧N+1 Display → showBuffer(L2)
```

→ **micro.go DoEvent 零改动；actions.go 的 76 处 Relocate 零改动；softwrap.go / buffer.go / md/* 零改动**。全部改动落在 `bufwindow.go`（2 行分发调用名）+ `bufwindow_md.go`。**侵入面与方案 A 完全等同**。

---

## 2. 方案 B 的核心机制（修正后）

### 2.1 数据结构

screenBuffer 自建轻量结构（**不用 tcell SimulationScreen**——见下）。核心：把 `line` 作为 cell 的元数据，与 cells 同源生成。

```go
type mdCell struct {
    r     rune
    combc []rune
    style tcell.Style
}
type screenRow struct {
    line  int       // 对应的 buffer line（索引，长期保留，供 LineToScreenRow 查询）
    row   int       // softwrap row（同一 buffer line 的第几段，长期保留）
    cells []mdCell  // 显示数据（临时：showBuffer blit 后置 nil 释放）
}
type screenBuffer struct {
    rows      []screenRow  // 长度动态，≤ 2*bufHeight
    startLine SLoc         // 渲染时的起点（case B 范围检查用）
}
```

**为什么不用 tcell SimulationScreen**（修正反驳①的部分推论）：SimulationScreen 是 `[y][x]` 的纯 cell 网格，没有地方放 `line` 元数据。screenRow 把「line 索引」和「cell 显示数据」合并在同一个 row 里——displayToBuffer 一次渲染**同源产出两者**，永远不会失配。line 填充零额外成本：renderer 内部本就有 line 信息（renderSegmentMD 的 `row.BufLine`、renderSegmentNative 的 `bloc.Y`）。

**cells 用完即弃**：showBuffer 把某 row 的 cells blit 到真屏后，`row.cells = nil` 释放。blit 之后到下一次 displayToBuffer 之间，screenBuffer 只剩轻量的 line/row 索引，内存占用极小。

> 内存澄清：真屏（tcell Screen）内部仍存一份 cell，所以这是避免「screenBuffer + 真屏」双倍冗余，不是凭空省内存。但 screenBuffer 高度是 2×bufHeight（比真屏的 bufHeight 大），释放后收益明显。

**LineToScreenRow 查询**：扫 `screenBuffer.rows` 找 line/row 匹配，O(行数)。与现状扫 viewportRowmap 同复杂度，但数据源从「shadow map」变成「screenRow.line 元数据」。

### 2.2 displayToBuffer（render 到 screenBuffer）

```
function displayToBuffer(startLine, showCursor bool):
    清空 screenBuffer
    vY = 0
    for seg in segmentsFrom(startLine):
        if editMode and showCursor and cursorIn(seg):
            vY = renderSegmentNative(seg, vY, target=screenBuffer)
        else:
            vY = renderSegmentMD(seg, vY, target=screenBuffer)
        # 停止条件（目的驱动，无魔法数）：
        if cursorLine 已渲染 + scrollmargin 余量 + viewport 填满: break
        if vY >= 2*bufHeight: break   # 兜底（反驳②）
```

### 2.3 showBuffer（blit 到真屏）

showBuffer 职责单一：**从指定起点 copy screenBuffer 到真屏**，不做 dirty 判定。

```
function showBuffer(startLine):
    # case B 范围检查（纯算术，非状态机）：startLine 超出 screenBuffer 覆盖区间 → 补渲染
    if startLine < screenBuffer.startLine or startLine > screenBuffer.startLine + 2*bufHeight:
        displayToBuffer(startLine, showCursor=false)   # case B 自愈路径，固定 false（滚动不关心 cursor 行）
    找到 startLine 在 screenBuffer 中的 vY 起点
    blit screenBuffer[vY..vY+bufHeight] → screen
    处理边界（装饰行 / 尾部不足 / 空白填充）
```

**关键澄清**：case A 不需要任何检查（详见 §4.1）。screenBuffer 在 handleEvent 的 Relocate 里刚被 displayToBuffer 刷新过，到下一帧 showBuffer 时必然是最新——showBuffer 只是换 blit 起点，一次 copy。

### 2.4 relocateVerticalMD（入口加 displayToBuffer）

case A 与 case C 走**同一个 relocate 逻辑**，唯一区别是传给 displayToBuffer 的渲染起点不同：cursor 在视野内用 `w.StartLine`（case A），cursor 跳远则按目标行 1:1 估算（case C）。**都只渲染一次**（详见 §4.3）。

```
function relocateVerticalMD(c, scrollmargin, height):
    # 估算本次渲染起点（cheap 预判，渲染前）
    if c.Line 在上一帧 screenBuffer 覆盖区间内:   # case A：cursor 在视野附近
        displayStart = w.StartLine
    else:                                          # case C：cursor 跳远（goto/search/pageup/pagedown）
        displayStart = c.Line - desiredMargin      # 1:1 buffer 算术，让 cursor 落在期望 margin
    displayToBuffer(displayStart, showCursor=true)   # 导航固定 true；editMode 内部 gate 是否真 native 渲染
    # fine-tune（查询，非渲染）：用 fresh screenBuffer 精确调整，让 cursor 落在期望 margin
    cursorRow = 在 screenBuffer 中查 c.Line 所在屏行
    newStartLine = screenBuffer[cursorRow - desiredMargin] 对应的 SLoc
    ...
```

---

## 3. 修正后的复杂度对比

| 维度 | 方案 A | 方案 B（修正后） |
|------|--------|--------|
| 渲染层迁移 | `renderSegmentMD` 加 `dryRun bool` ~20 行 + 新增 `renderSegmentNativeDryRun` 复用 `getRowCount` ~15 行 | screenBuffer 提供 `SetContent` 兼容方法 ~15 行 + renderer 替换 `screen.SetContent` → `sb.SetContent` ~8 行 |
| 查询地图 | 保留 `viewportRowmap []SLoc`（shadow map） | **删除** viewportRowmap；`screenRow.line` 作为 cell 元数据同源生成 |
| 核心新增函数 | `preRenderAtCursor` ~50 + `findSegmentContaining` ~10 | `displayToBuffer` ~80 + `showBuffer` ~40 |
| Relocate 改动 | 入口加触发判定 ~8 行 | 入口加 `displayToBuffer` + 查询源替换 ~20 行 |
| displayBufferMD 改造 | 零（主循环不动） | 改造成 showBuffer（删除 render，加 blit + 范围检查）|
| **每帧渲染次数** | 1 次全量（Display）+ 1 次部分（preRender，写 map 不写屏） | **1 次全量（displayToBuffer）+ 1 次 cheap blit（showBuffer）** ✅ 更省 |
| 主循环 / actions.go | 零改动 | **零改动** |
| softwrap / buffer / md 包 | 零改动 | 零改动 |
| 侵入文件 | display 包 2 个 | display 包 2 个（**等同**）|
| 总代码量 | **~105 行** | **~150 行**（删 viewportRowmap 抵消一部分新增） |
| 架构性质 | shadow map 补丁（screen + 影子 map 双份账，需保证同步） | **单一数据源**（screenRow：line 元数据 + cells 显示数据同源生成；cells blit 后释放，line 留存供查询）|

**修正结论**：原评「B 是 A 的 5～7 倍」**错误**。实际是 **~1.4 倍**，且 B 每帧渲染更少、架构更干净（无 shadow map）。

---

## 4. B 真正剩余的风险与待定设计

侵入度问题被三轮反驳消解后，原列 4 个待定已**全部澄清**：§4.1（dirty 标记）删除、§4.2（查询地图）已解决、§4.3（case C 迭代）已解决、§4.4（showCursor 参数）已解决。**真正剩余的只有 §4.5 与 A 同级的共性 blit 边界**（非 B 新增）：

### 4.1 ~~待定①：showBuffer 的「自愈」与 dirty 标记~~（已澄清，删除）

**初版错误**：曾担心 case A 下 showBuffer 会「多余重渲染一次」，提出 dirty 标记解决方案。

**澄清**：这是把两个不同的「重渲染」混淆了，dirty 标记是凭空臆造的概念，**B 根本不需要**。

完整时序核查（DoEvent 不改）：

```
帧N-1 handleEvent → Relocate → relocateVerticalMD
    └─ displayToBuffer(oldStartLine)   ← 渲染，screenBuffer 数据 = S2
    └─ 算出 newStartLine = L2
帧N   Display → showBuffer(L2)          ← L2 在 S2 内，直接 blit，一次 copy
```

showBuffer(L2) 拿到的 screenBuffer **就是上一帧 Relocate 里刚渲染好的 S2**，L2 只是从 S2 里挑一个起点 blit。**没有任何重渲染，也不需要 dirty 判定**——showBuffer 职责单一：从指定起点 copy。

不同场景下 screenBuffer 的来源与 showBuffer 动作：

| 场景 | screenBuffer 来源 | showBuffer 动作 |
|------|------------------|----------------|
| case A（光标移动） | 上一帧 Relocate 里 displayToBuffer 刚渲染的 S2 | **直接 blit，无任何检查** |
| case B（ScrollUp/Down） | 更早的帧渲染的，StartLine 已被 actions.go 直接改 | **范围检查**（纯算术，见下）；超出则 displayToBuffer 补一次 |
| case C（Goto） | 上一帧 Relocate 里渲染的，cursor 可能超出 2x | Relocate 内部已迭代处理（§4.3），到 showBuffer 时 screenBuffer 必然覆盖 cursor |

**case B 的「范围检查」是纯算术，不是状态机**：

```
if screenBuffer.startLine <= newStartLine <= screenBuffer.startLine + 2*bufHeight:
    blit                # 在覆盖范围内
else:
    displayToBuffer(newStartLine)   # 超出（连续滚了很多次累计越界），补渲染
```

关键区别：
- **范围检查**（case B 需要）：`startLine` 是否落在 screenBuffer 覆盖区间，算术比较
- **dirty 标记**（曾误提）：跟踪 screenBuffer 是否被改过，状态机——**不需要**

→ §4 待定数从 4 降到 3。本节保留作为错误修正记录。

### 4.2 ~~待定②：screenBuffer 作为查询地图的查询效率~~（已解决）

**原担忧**：删 viewportRowmap 后，LineToScreenRow 怎么查？曾提两个选项（GetContent 反推 / 外挂 `[]SLoc` 索引），判「单一数据源打折扣」。

**已解决**（用户设计，见 §2.1）：screenRow 结构把 `line` 作为 cell 的**元数据字段**，与 cells **同源生成**（displayToBuffer 一次渲染产出两者）：
- `cells`：显示数据，showBuffer blit 后释放（临时）
- `line/row`：索引数据，长期保留供 LineToScreenRow 查询（持久）

→ **不是「viewportRowmap 换个写入时机」，而是 line 本来就是 screenRow 的一部分**。

对比 A 与 B 的查询数据来源：

| | 方案 A | 方案 B |
|--|--------|--------|
| 查询数据 | `viewportRowmap []SLoc` | `screenRow.line` |
| 与显示数据的关系 | **并列维护**的 shadow（两套数据，须保证同步） | **同源生成**的元数据（一套数据，渲染时顺便产出） |
| 失配风险 | 有（preRender 刷新时机错就失配，§4.1 原担忧的根源） | 无（line 和 cells 同一次渲染产出） |

「单一数据源不打折扣」成立，且比 A 的 shadow map **更内聚**。

**唯一代价**：renderer 不能用 `sb.SetContent(x,y,...)` 直接机械替换（那是 SimulationScreen 的接口）。screenBuffer 需提供兼容方法（内部映射到 `rows[y].cells[x]`，~15 行），renderer 仍调 `sb.SetContent`、与 `screen.SetContent` 同构。line 字段由 displayToBuffer 在 segment 循环中根据 renderer 已有的 `row.BufLine`/`bloc.Y` 回填。

### 4.3 ~~待定③：case C（Goto/Search 大跨度）的迭代~~（已澄清，无迭代）

**初版错误**：担心 `displayToBuffer(oldStartLine)` 渲染后 cursor 不在 2x screenBuffer 内，提出「迭代渲染」方案，判「最坏 2 次 render，需 fallback」。

**澄清**：这是把 case C 想成了「先渲染旧起点、发现 cursor 不在、再补渲染」的笨路子。但 case C（goto/search/pageup/pagedown）的本质是——**目标行 c.Line 已知**（action 在调 Relocate 前就设好了新 cursor 位置）。所以应该用目标行**直接估算渲染起点**，一次到位：

```
displayStart = c.Line - desiredMargin     # 1:1 buffer 算术，cursor 落在期望 margin
displayToBuffer(displayStart)              # 唯一一次 render：渲染目标行附近的 2x bufHeight
relocate fine-tune                          # 查询 fresh screenBuffer，精确调整 startLine
```

为什么一次就够（不迭代、不 fallback）：
- 估算时 cursor 在 `displayStart + desiredMargin` 处（buffer 空间 1:1）
- 实际渲染后，因 segment 展开/softwrap，cursor 的屏行 R' 可能偏移 ±Δ
- 但 screenBuffer 是 2x bufHeight，`desiredMargin` 在 bufHeight 内，Δ 被 2x 余量吸收 → cursor 必在 screenBuffer 内
- relocate 只是**查询** cursor 实际屏行 R'，算出精确 startLine（查询不是渲染）
- showBuffer 直接从已渲染的 screenBuffer blit，不重渲染

**关键结论**：case C 与 case A 走**同一个 relocate 逻辑**（见 §2.4），区别仅在渲染起点的选择——case A 用 `w.StartLine`（cursor 在视野内），case C 用 `c.Line - desiredMargin`（cursor 跳远）。两者都**只渲染一次**。

→ 无迭代、无 2 次 render、无特殊 fallback。唯一边角 case：单个 segment 本身 > 2x bufHeight（如几百行的 codeblock），cursor 无法完整显示——这是现状就有的限制（viewport 装不下），走 native 兜底，非 B 新增。

### 4.4 ~~待定④：showCursor 参数是否过度设计~~（已澄清，参数必要，按操作类固定取值）

**初版假设**：`showCursor = editMode`（恒等式），参数冗余，可简化为无参内部读 editMode。

**澄清**：showCursor ≠ editMode，两者**正交**：
- **editMode**：渲染模式（cursor 段是否 native 可编辑）
- **showCursor**：操作意图（本次操作是否希望 cursor 行进 viewport）

showCursor 按操作类**固定取值**，不需要 caller 传变量——它在两个 call site 就钉死了：

| 触发路径 | showCursor | 操作类 | 理由 |
|---------|-----------|--------|------|
| relocate（case A 方向键 / case C goto·search·pageup·pagedown）| **true** | 导航 | 用户主动移动，希望看到 cursor 行 |
| showBuffer 自愈（case B 鼠标滚动 / ScrollUp·Down）| **false** | 滚动 | 用户在阅读，cursor 移出 viewport 无所谓 |

**反例证明 showCursor ≠ editMode**：editMode=true + 鼠标滚动 → editMode 为 true 但 showCursor=false（cursor 滚出视野，其段无需 native 渲染）。参数**不冗余**，承载 editMode 无法表达的信息。

**纯渲染模式（editMode=false）**：`editMode and showCursor` 短路为 false，恒 MD 渲染，与 showCursor 取值无关。即便 goto 也只是把目标行滚进 viewport、仍 MD 渲染（阅读不编辑）。

两层效果（showCursor 一次取值同时驱动两件事）：
1. **滚动行为**：true → relocate 把 cursor 行拉进 viewport；false → 不强制（用户滚到哪算哪）
2. **渲染优化**：false → cursor 段也不必 native（反正不显示/不关心），全 MD

**为什么不需要 caller 传操作类型**：showCursor 的两个取值恰好与两个 call site一一对应（relocate 恒 true / showBuffer 自愈恒 false），call site 本身就决定了取值，无需把「当前是什么操作」沿着调用栈传下来。editMode 内部 gate native 渲染决策。逻辑清晰无歧义。

### 4.5 共性风险：blit 边界（与 A 同级，非新增）

showBuffer 要处理 newStartLine 落装饰行 / screenBuffer 尾部不足 / 空白填充 / softwrap Row 偏移。这些与现状 `relocateVerticalMD` 的「delta 落装饰行」是同类问题（修改总结 §3.3 已踩过），不算新增风险等级。

---

## 5. 原评估的错误复盘

初版报告判定「B 复杂 5～7 倍、3 个致命缺陷」，经用户反驳后大部分推翻。错误来源：

| 初版错误结论 | 错误根源 | 修正 |
|------------|---------|------|
| 「screenBuffer 要新设计 4000 cell/帧结构」 | 没查 `screen.Screen` 是 interface、不知道 tcell 有 SimulationScreen | 渲染层迁移 ~8 行，几乎免费 |
| 「缺陷①致命未解：screenBuffer 尺寸不够」 | 没推演典型 bug 场景下 `newStartLine - oldStartLine` 的实际量级 | 2x cap 覆盖绰绰有余，极端 case 走 fallback |
| 「缺陷②原则级违规：要改 micro.go / 76 处 Relocate」 | 没注意到 `bufwindow.go:249/940` 已有 MD 分发点 | 分发点已在 MD 代码里，零改主循环 |
| 「总代码量 500～700 行」 | 上述三项叠加放大 | 实际 ~150 行 |
| 「B 需要 dirty 标记 / 自愈状态机」 | 把 case B 的「范围检查」误推广为状态机，又错误套到 case A | **不需要**。case A 的 screenBuffer 必为最新（上一帧 Relocate 刚渲染）；case B 只需纯算术范围检查 |
| 「B 的单一数据源会打折扣（仍需 viewportRowmap）」 | 只想到 SimulationScreen（无 line 字段）或外挂索引（变回 shadow）两种选项 | screenRow 自建结构：line 作为 cell 元数据同源生成，cells blit 后释放、line 留存查询。比 A 的 shadow map 更内聚 |
| 「case C 需要 2 次渲染 + fallback 迭代」 | 把 case C 想成「先渲染旧起点、发现 cursor 不在、再补渲染」的笨路子 | 目标行已知，直接用它估算渲染起点（1:1 算术）→ 渲染一次 → relocate 查询微调。case C 与 case A 同一逻辑，都只渲染一次 |
| 「showCursor = editMode 恒等式，参数冗余可删」 | 把「渲染模式」与「操作意图」混为一谈 | showCursor ≠ editMode，两者正交。showCursor 按 call site 固定取值（relocate 恒 true / showBuffer 自愈恒 false），承载操作意图，不冗余 |

**教训**：评估「重构方案」时容易把「文档没写的实现细节」都算作「方案要解决的问题」。实际上：
- screenBuffer 的实现成本被 tcell 既有设施吸收了（反驳①）
- 尺寸补救有便宜的启发式解（反驳②）
- 分发位置现状已存在（反驳③）
- showBuffer 是纯 copy，没有状态机（第四轮澄清）
- screenRow 让 line 与 cells 同源，单一数据源不打折扣（第五轮澄清）
- case C 目标行已知，估算起点一次到位，无需迭代（第六轮澄清）
- showCursor 按 call site 固定取值，承载操作意图，非 editMode 恒等式（第七轮澄清）

**真正的风险**：无架构性难点。§4 四个待定全部澄清，只剩 §4.5 与 A 同级的共性 blit 边界（装饰行/尾部不足/softwrap Row 偏移，非 B 新增）。B 可直接进入实施设计。

---

## 6. A vs B 的真实取舍（修正后）

两条路都可行，侵入面等同（display 包 2 个文件）。**B 实际只需新增 2 个机制**（displayToBuffer / showBuffer），无状态机、无 dirty 标记、无 case C 迭代、无操作类型透传。§4 四个待定全部澄清后，两者差异已不再是「B 要多设计复杂机制」，而是代码量与架构净度的权衡：

| 维度 | 方案 A | 方案 B |
|------|--------|--------|
| 代码量 | ~105 行 | ~150 行（+1.4x）|
| 每帧渲染 | 2 次（全量 + preRender 写 map） | **1 次全量 + 1 次 cheap blit** |
| 架构性质 | shadow map 补丁（screen + 影子 map 双份账，须保证同步） | **单一数据源**（screenRow：line 元数据 + cells 同源生成；cells blit 后释放、line 留存查询）|
| 新增机制 | 3 个（preRender / findSegment / dryRun） | 2 个（displayToBuffer / showBuffer）；**无 dirty 标记、无 case C 迭代、无操作类型透传** |
| 心智负担 | 触发条件判定（何时该 preRender）+ shadow map 同步问题 | case B 范围检查（纯算术越界补渲染）；无 shadow map 同步问题（line 与 cells 同源） |
| 与现状差异 | 小（保留 viewportRowmap，加 dryRun 旁路） | 中（displayBufferMD 从「渲染」变「blit」，语义翻转）|
| 剩余风险 | 无架构性难点（§4 全清）| 无架构性难点（§4 全清）|

两者都能根治 V2 bug（跨段时光标飞出 viewport）和修跨段消失帧，这点无差异。

### 选择建议（修订后）

经七轮澄清，原建议「B 要多设计 4 个生命周期机制、B 更复杂」的判断已不成立。差异收敛为：
- **方案 A**：~105 行，最小改动，保留 viewportRowmap 加 dryRun 旁路，与现状最接近。**适合追求最小改动量、最快出活、对 shadow map 同步负担不在意**。
- **方案 B**：~150 行（+45 行），架构更干净（单一数据源、每帧少一次渲染、无 shadow map），但 displayBufferMD 语义翻转（渲染 → blit）需重新理解。**适合看重架构净度、愿意接受语义翻转**。

**推荐**：若团队接受 displayBufferMD 的语义翻转，**选 B**——多出 45 行换单一数据源 + 每帧少一次渲染 + 无同步负担，性价比高。若追求最快出活且对现状熟愁，选 A。

### 选 B 后的下一步

B 无架构性难点，可直接进入实施设计：
1. screenRow / mdCell / screenBuffer 结构定义（§2.1）
2. displayToBuffer 伪码实现（§2.2）
3. showBuffer blit + 范围检查（§2.3）
4. relocateVerticalMD 入口改造（§2.4，统一 case A/C）
5. renderer 的 SetContent retarget（从 `screen` 换成 `sb` 兼容方法）
6. §4.5 blit 边界边角 case 处理（与 A 同级）

---

## 7. 关键代码事实（评估依据）

| 事实 | 位置 | 对评估的意义 |
|------|------|------------|
| `screen.Screen` 是 `tcell.Screen` interface | `internal/screen/screen.go:19` | screenBuffer 不用 SimulationScreen（缺 line 字段），改用 screenRow 自建结构（§2.1）|
| `screen.SetContent` 是转发包装 | `internal/screen/screen.go:116` | renderer 可机械 retarget 到 screenBuffer 的 SetContent 兼容方法（§2.1 + §3 渲染层迁移）|
| `Relocate()` 已有 `if w.Buf.IsMD` 分发 | `bufwindow.go:249` | B 的 displayToBuffer 钩子挂这里，零改 actions.go（反驳③） |
| `Display()` 已有 `if w.Buf.IsMD` 分发 | `bufwindow.go:940` | B 的 showBuffer 钩子挂这里，零改 micro.go（反驳③） |
| actions.go 有 76 处 `h.Relocate()` | `internal/action/actions.go` | **B 零改动**（分发在 BufWindow 层，action 层无感）|
| ScrollUp/Down 直接改 StartLine 不调 Relocate | `actions.go:25-37` | case B 靠 showBuffer 范围检查自愈（纯算术，非状态机，§4.1）|
| renderSegmentMD 内部有 `row.BufLine` | `bufwindow_md.go:99-154` | screenRow.line 填充零额外成本，同源生成（反驳⑤） |
| renderSegmentNative 内部有 `bloc.Y` | `bufwindow_md.go:317+` | screenRow.line 填充零额外成本（反驳⑤） |
| renderSegmentMD ~160 行 | `bufwindow_md.go:54+` | B 机械 retarget（~8 行 SetContent）；A 加 dryRun 参数 |
| renderSegmentNative ~370 行 | `bufwindow_md.go:296+` | B 机械 retarget；A 零改动 + 新增 dryRun ~15 行 |
| getRowCount 封装 softwrap 几何 | `softwrap.go:226` | A 的 dryRun 路线 C 复用；B 不需要（直接跑完整 renderer 到 screenBuffer）|
| relocateVerticalMD 现有五分支 | `bufwindow_md.go:867+` | A 入口加钩子；B 入口统一 case A/C（displayStart 预判 + 唯一渲染 + 查询微调，§2.4）|
| cursor 位置在 action 调 Relocate 前已设好 | `actions.go` goto/search/pageup/pagedown | case C 目标行已知，估算起点一次到位，无迭代（反驳⑥）|
