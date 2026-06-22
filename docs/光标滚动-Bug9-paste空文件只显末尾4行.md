# Bug #9 — Paste 到空 MD 文件，viewport 只显示末尾 4 行

> **关联文档**：`docs/光标滚动方案B-实施总结.md` §8 踩坑档案（继 Bug #8 之后）
> **状态**：分析完成，待用户许可后实施
> **当前模式**：PLAN（只能改 .md/.txt），代码修改需用户许可

---

## 1. 复现

1. 打开一个新的空 `.md` 文件（buffer 1 行，空）
2. 粘贴一段文本，假设有 N 行，且 **N ≤ viewport 高度**（用户主观判断"应该不需要滚动"）
3. 现象：microNeo 只显示**最后 4 行**，前 N-4 行跑到 viewport 上方外面去了
4. 用户感知："microNeo 认为 viewport 只有 4 行"

> 注：4 = `scrollmargin + 1`（默认 scrollmargin=3）。这个数字是定位根因的关键线索。

---

## 2. 根因定位

### 2.1 触发链路

| 步骤 | 状态 |
|------|------|
| paste 前 | buffer 1 行，`w.sb` 渲染过空 buffer，`sb.startLine=0`, `sb.lastLine=0` |
| paste 后 | buffer N 行，活动光标 `c` 落在最后一行 `c.Line = N-1` |
| `Relocate()` 被触发 | 调用 `relocateVerticalMD(c, scrollmargin=3, height≈H)` |

### 2.2 关键决策点（`internal/display/bufwindow_md.go:1143` `relocateVerticalMD`）

```go
if w.sb != nil && w.sb.coversLine(c.Line) {       // ★ 旧 sb 只覆盖 line 0，c.Line=N-1 不在其中
    ...                                             //   → 整个 if 分支被跳过
} else {
    displayStart = SLoc{Line: c.Line - scrollmargin, Row: 0}   // ★ case C：displayStart = N-1-3 = N-4
    caseLabel = "C"
    w.StartLine = displayStart
}
```

**问题就在这里**：case C 无条件用 `c.Line - scrollmargin` 作为渲染起点。这个估算隐含假设是
"光标远离起点，需要把它放到 `botMarginRow` 让用户看到上下文"——对长文件 jump 场景成立，
但对**短 buffer** 完全错误。

### 2.3 连锁后果

`displayStart = N-4` 之后：

1. `displayToBuffer(N-4, true)` 渲染 → sb 只有 4 行有效内容（`nContent=4`，`lastLine=N-1`）
2. 查询微调阶段：
   - `cursorRow = 3`，`botMarginRow = H-1-3 ≈ 42` → `cursorRow > botMarginRow` ❌ 不成立，**不 SCROLLUP**
   - `cursorRow (3) < scrollmargin (3)` ❌ 不成立，**不 SCROLLDOWN**
   - → StartLine 原地不动，卡在 `N-4`
3. 下一帧 `displayBufferMD`：`coversExtent(StartLine=N-4, H, N)` 检查
   `lastLine(N-1) >= bufferLines-1(N-1)` ✅ → `needRender=false`，直接 blit
4. `showBuffer` 从 `startVY=0` blit，画 4 行内容 + (H-4) 行空白

**最终**：viewport 只看到末尾 4 行，永久卡死（不会自我修复）。

### 2.4 分支结构对比：micro 原生 4 分支 vs microNeo 缺 1 分支

#### 2.4.1 micro 原生 Relocate 垂直部分：**4 个分支**（`bufwindow.go:261-272`）

按"cursor 跑出视口的哪一侧"分两组：

**上半组（cursor 在视口顶之上）— 2 个分支：**

| # | 条件 | 动作 | 语义 |
|---|------|------|------|
| **1** | `c < StartLine+margin` && `c > bStart+margin-1` | `StartLine = c - margin` | cursor 放到距顶 margin 行 |
| **2** | `else if c < StartLine` | `StartLine = c` | 已到 buffer 头，跟随 |

**下半组（cursor 在视口底之下）— 2 个分支：**

| # | 条件 | 动作 | 语义 |
|---|------|------|------|
| **3** | `c > StartLine+H-1-margin` && `c ≤ bEnd-margin` | `StartLine = c - H + 1 + margin` | cursor 放到 botMarginRow（中间区域）|
| **4** | `else if c > bEnd-margin` && `c > StartLine+H-1` | `StartLine = bEnd - H + 1` | ★ **end-pin**：bEnd 钉到视口底（末尾区域）|

**关键**：分支 3 有前置条件 `c ≤ bEnd - margin`——只有 cursor 在"**中间区域**"才走它；
cursor 在"**末尾区域**"（`c > bEnd - margin`）走分支 4。end-pin 分支 4 的代码：

```go
} else if c.GreaterThan(w.Scroll(bEnd, -scrollmargin)) && c.GreaterThan(w.Scroll(w.StartLine, height-1)) {
    w.StartLine = w.Scroll(bEnd, -height+1)        // ★ end-pin：避免顶部留白
}
```

#### 2.4.2 microNeo `relocateVerticalMD`：主路径只覆盖 2 个分支

它的结构是"先选 case → 再微调"，不是 micro 那种平铺分支：

**阶段 1 — 选 case（2 选 1）：**
- **case A**：cursor 在旧 sb 可见视口内 → `displayStart = w.StartLine`（沿用）
- **case C**：else → `displayStart = c.Line - scrollmargin`（**无条件**用分支 3 的公式）

**阶段 3 — 查询微调（3 个分支）：**
- 微调 a：`cursorRow > botMarginRow` → SCROLLUP
- 微调 b：`cursorRow < scrollmargin` → SCROLLDOWN
- 微调 c：else → 不动

（另有兜底 `relocateVerticalNativeFallback`，内部把 micro 原生 4 分支**完整复刻**一遍，
但只在 overflow / rowIndexOf 失败等极端情况触发，不是主路径。）

#### 2.4.3 对应关系：microNeo 主路径**只覆盖了 micro 的分支 1 和 3**

| micro 原生分支 | microNeo 主路径对应 | 状态 |
|---|---|---|
| 分支 1（顶之上 + 距顶 margin） | 微调 b SCROLLDOWN | ✅ 有 |
| 分支 2（顶之上 + 到 buffer 头）`StartLine = c` | 部分由 case A（沿用旧 StartLine，通常已是 0）覆盖 | ⚠️ 隐式覆盖 |
| 分支 3（底之下 + botMarginRow） | case C 公式 + 微调 a SCROLLUP | ✅ 有 |
| **分支 4（底之下 + end-pin）** | **❌ 完全没有** | **🔴 Bug #9 缺的就是这个** |

#### 2.4.4 为什么偏偏缺的是分支 4

看 case C 的公式：

```go
displayStart = SLoc{Line: c.Line - scrollmargin, Row: 0}
```

这**恰好是 micro 分支 3 的动作**（`StartLine = c - H + 1 + margin` 的简化版——
microNeo 把"放到 botMarginRow"的精确定位推迟到阶段 3 的 SCROLLUP 微调去做，case C 只给粗估）。

但 micro 分支 3 有前置条件 `c ≤ bEnd - scrollmargin`，**只有 cursor 在中间区域才允许用这个公式**。
microNeo 的 case C **把这个前置条件扔了**，无条件套用分支 3 的公式。

后果：
- cursor 在中间区域（长文件 jump 到中部）→ 公式正确 ✅
- cursor 在末尾区域（短 buffer / paste / goto 末尾）→ 公式过度上推，本该走分支 4 却走了分支 3 ❌

**所以修复的本质**：在 case C 内部按 cursor 相对 bEnd 的区域分流——中间区走分支 3 公式，
末尾区走分支 4 公式（`StartLine = bEnd - H + 1`），恢复 micro 原生分支 3/4 的二选一判断。
详见 §3。

#### 2.4.5 结构性根因

Bug #9 的根因**不是某行代码写错**，而是 case C 的估算公式漏掉了一类合法场景
（短 buffer / 光标在 bEnd 附近）。这符合方案 B 总结 §8 的共同特征：
"根因都不在出 bug 的那行代码，而在另一个看似无关的子系统"——
这里是 case C 的估算公式不完备，与渲染/查询逻辑本身无关。

> **一句话对比**：micro 原生 **4 分支**（顶 2 + 底 2，其中底部的 end-pin 分支专门处理
> "cursor 在 bEnd 附近"）；microNeo 主路径**只有分支 1 和分支 3 的等价物，分支 4（end-pin）
> 整个缺失**——case C 无条件套用分支 3 的公式，没复制分支 3 的前置条件 `c ≤ bEnd - margin`，
> 导致末尾区域场景失配。这就是 Bug #9 的结构性根因。

---

## 3. 修复方案：case C 内部按分支 3/4 细化

### 3.1 设计思路

**结论**：不在 case C 之后加 clamp，而是**在 case C 内部按 micro 原生的分支 3/4 判断逻辑
重新选择公式**。这样 microNeo 的分支覆盖与 micro 原生**一一对应**，不再有"缺一个分支"的结构性缺陷。

#### 3.1.1 为什么不能照搬原生 4 分支的完整形态

micro 原生的 4 分支**全用 buffer 行（SLoc）算术**，隐含假设是"buffer 行 = 屏幕行（1:1）"。
但 MD 场景下：

- 表格/代码块展开后，**buffer 行 ≠ 屏幕行**：`StartLine + height - 1` 在 buffer 层面算出的行，
  实际可能还在视口中间（表格段展开），或已超出视口（代码块收缩）
- softwrap 也会让行数变化

所以方案 B 的核心架构是 **"渲染后查询"**（总结 §2.2）：`displayToBuffer` 先渲染出 sb，
所有精确判断基于 sb 的屏幕行索引（`cursorRow`）。回到 buffer 行估算就是开倒车。

→ micro 的 4 分支在"**渲染前**"用 buffer 行判断的形态，不能整体照搬。

#### 3.1.2 但分支 3/4 的判断恰好不依赖屏幕行

关键洞察：Bug #9 出在 case C 的**公式选择**，而 micro 分支 3/4 的判断本质**只看 cursor 相对 bEnd
的位置**——`c.Line` 和 `bEnd.Line` 都是 buffer 行，不需要屏幕行信息：

| 区域 | 条件 | 动作 |
|------|------|------|
| 中间区 | `c.Line ≤ bEnd.Line - scrollmargin` | `displayStart = c.Line - scrollmargin`（分支 3）|
| 末尾区 | `c.Line > bEnd.Line - scrollmargin` | `displayStart = bEnd.Line - height + 1`（分支 4，end-pin）|

→ **可以在 case C 内部原样搬进这个二选一判断**，补上缺失的分支 4。

#### 3.1.3 比 clamp 方案更严谨

早先草案提议的是 clamp（`min(c - scrollmargin, bEnd - height + 1)`）。两者在
`height > 2*scrollmargin` 时**数学等价**，但**极端矮视口**（`height ≤ 2*scrollmargin`）时不等价：

| 方案 | height ≤ 2*scrollmargin 时行为 |
|------|-------------------------------|
| **clamp** | 保持 `c - scrollmargin`，可能让 displayStart 超出 bEnd 范围（只有 `< 0` 下界保护，无上界）|
| **分支 3/4 细化** | 永远在 `[0, bEnd - height + 1]` 内，语义清晰 |

而且分支细化**与 micro 原生结构完全对齐**，未来读代码的人能直接对照原生理解，可维护性更好。

### 3.2 代码改动

**文件**：`internal/display/bufwindow_md.go`
**位置**：`relocateVerticalMD` 的 case C 两处（`else` 分支约 1196-1199 行 + 外层 `else` 约 1206-1208 行）

把原来两处 case C 的**无条件公式**：

```go
// 现状（两处一样）：
displayStart = SLoc{Line: c.Line - scrollmargin, Row: 0}
caseLabel = "C"
w.StartLine = displayStart
```

改为**按分支 3/4 细化**：

```go
// case C：jump 到远处，重估 displayStart（对齐 micro 原生分支 3/4）
bEnd := w.SLocFromLoc(w.Buf.End())
if c.Line <= bEnd.Line-scrollmargin {
    // 分支 3：cursor 在中间区域 → 粗估放到 botMarginRow 附近
    //   （精确位置由渲染后微调阶段的 SCROLLUP 校正）
    displayStart = SLoc{Line: c.Line - scrollmargin, Row: 0}
    dbgLog("    relocate: caseC-branch3 displayStart=%d (c=%d bEnd=%d margin=%d)",
        displayStart.Line, c.Line, bEnd.Line, scrollmargin)
} else {
    // 分支 4：cursor 在末尾区域（短 buffer / paste / goto 末尾）
    //   → end-pin：bEnd 钉到视口底，避免顶部留白（Bug #9 修复）
    displayStart = SLoc{Line: bEnd.Line - height + 1, Row: 0}
    dbgLog("    relocate: caseC-branch4 end-pin displayStart=%d (c=%d bEnd=%d height=%d)",
        displayStart.Line, c.Line, bEnd.Line, height)
}
caseLabel = "C"
w.StartLine = displayStart
```

后续的 `if displayStart.Line < 0 { displayStart.Line = 0 }` 下界保护保留不变。

**改动性质**：
- 对 case A（displayStart = w.StartLine）：**零影响**（不进入 case C 块）
- 对 case C 中间区域（原所有 case C 场景的长文件 jump）：行为不变（分支 3 = 原公式）
- 对 case C 末尾区域（本 Bug + goto 末尾 + 短 buffer）：走分支 4，正确收敛
- 不动 `displayToBuffer` / `showBuffer` / `coversExtent`，符合"单一数据源"原则
- 不动 micro 原生代码，全部改动隔离在 `*_md.go`，符合零侵入红线

### 3.3 场景收敛验证

五种场景在分支 3/4 细化下全部正确：

| 场景 | bEnd.Line | height | c.Line | 区域 | 走分支 | displayStart | 正确性 |
|------|-----------|--------|--------|------|--------|--------------|--------|
| **paste 到空文件（本 Bug）** | 29 | 46 | 29 | 末尾区（29>26） | 4 | `29-46+1=0` | ✅ 从头显示全 30 行 |
| paste 巨量内容（超出视口） | 199 | 46 | 199 | 末尾区（199>196） | 4 | `199-46+1=154` | ✅ 末尾一屏，cursor 在底 |
| goto 跳到长文件中部 | 199 | 46 | 100 | 中间区（100≤196） | 3 | `100-3=97` | ✅ 不变，cursor 在 botMarginRow |
| goto 跳到长文件末尾 | 199 | 46 | 199 | 末尾区 | 4 | `154` | ✅ 末尾一屏（顺带修复）|
| buffer 正好填满视口 | 45 | 46 | 45 | 末尾区（45>42） | 4 | `45-46+1=0` | ✅ 从头显示全 46 行 |

### 3.4 改造后的整体分支结构（与 micro 原生一一对应）

```
microNeo relocateVerticalMD（改造后）:
├─ 阶段1 选 case（渲染前，buffer 行）
│   ├─ case A：cursor 在旧 sb 可见视口 → 复用 StartLine（不渲染优化）
│   └─ case C：jump 到远处
│       ├─ 分支 3：cursor 在中间区 (c ≤ bEnd-margin) → c - margin
│       └─ 分支 4：cursor 在末尾区 (c > bEnd-margin) → bEnd - H + 1  ★ 新增
├─ 阶段2 displayToBuffer 渲染
└─ 阶段3 微调（渲染后，屏幕行精确）
    ├─ SCROLLUP：cursorRow > botMarginRow
    ├─ SCROLLDOWN：cursorRow < scrollmargin
    └─ 不动：cursor 在舒适区

micro 原生 Relocate（对照）:
├─ 分支 1：顶之上 + 中间区 → c - margin
├─ 分支 2：顶之上 + 到头 → c
├─ 分支 3：底之下 + 中间区 → c - H + 1 + margin
└─ 分支 4：底之下 + 末尾区 → bEnd - H + 1
```

- **分支 1/2**（上半组）：microNeo 由 case A（复用）+ 微调 SCROLLDOWN 覆盖
- **分支 3/4**（下半组）：microNeo 由 case C 内部细化覆盖（**本次修复补上分支 4**）
- **微调阶段**：处理渲染后才能精确知道的屏幕行偏移（softwrap/装饰行）

这样 microNeo 的分支覆盖与 micro 原生**一一对应**，消除了"缺一个分支"的结构性缺陷。

### 3.5 与现有 Bug 修复的关系

| Bug | 场景 | 与本 Bug 关系 |
|-----|------|---------------|
| #2 | goto 跳远，旧 sb 覆盖 cursor 但在第二屏 | case A/C 判定问题（curRow 与可见窗口关系），本 Bug 是 case C **公式**问题 |
| #7 | 连续 ↓ 跨表格误判 case C | case A/C 判定的"可见视口"细化，本 Bug 在判定之后才触发 |
| #8 | 点击装饰行映射错误 | 点击映射 + case 边界开闭，与本 Bug 无交集 |

本 Bug 是**首次**发现 case C 的 `c.Line - scrollmargin` 公式本身不完备。所有之前的 Bug
都在"case A vs case C 的选择"上，没人质疑过 case C 选对之后的估算。这正是 §9.2 反复踩的陷阱
清单需要补一条的信号。

---

## 4. 验证清单

实施后需回归以下场景（防止修复引入新问题）：

### 4.1 本 Bug 直接验证

- [ ] 空 .md 文件 paste 5 行（< scrollmargin+1）：displayStart 应为 0
- [ ] 空 .md 文件 paste 30 行（< viewport 高度）：从头显示，cursor 在末行
- [ ] 空 .md 文件 paste 46 行（≈ viewport 高度）：从头显示，cursor 在末行
- [ ] 空 .md 文件 paste 200 行（> viewport 高度）：末尾一屏可见，cursor 在底部附近

### 4.2 回归（既有 Bug 不能复现）

- [ ] Bug #2：goto 50 / goto 30 / goto 末尾 → 不错位、光标不消失
- [ ] Bug #3：连续 ↓ 跨表格段 → 光标不错位
- [ ] Bug #5：鼠标滚动 → 不触发原生 fallback
- [ ] Bug #6：鼠标向上滚动 → 不卡住
- [ ] Bug #7：连续 ↓ 跨入表格 → StartLine 不暴跳
- [ ] Bug #8：点击表格上框装饰行 → 光标不跳远

### 4.3 调试方法

按 §7.3 调试循环：
```
make build-dbg    # 日志写 /tmp/microNeo_debug.log
# 清空日志 → 复现 → grep "relocateVerticalMD ENTER\|caseC-branch\|case=\|EXIT"
```

关键 trace：观察末尾区场景（短 buffer / paste / goto 末尾）是否走 `caseC-branch4 end-pin`，
中间区场景（长文件 jump 到中部）是否走 `caseC-branch3`。

---

## 5. 后续（可选）

把本 Bug 作为 **Bug #9** 补进 `docs/光标滚动方案B-实施总结.md` §8 踩坑档案，并在 §9.2
陷阱清单加一条：

> **估算公式要看 cursor 相对 bEnd 的区域**：`c.Line - scrollmargin` 这类"相对 cursor"的
> 估算隐含"cursor 在 buffer 中间区域"假设（等价 micro 原生分支 3 的前置条件 `c ≤ bEnd-margin`）。
> 短 buffer / 光标在 bEnd 附近时（cursor 在末尾区域），必须走分支 4（end-pin：`bEnd - height + 1`）。
> **不要只用 clamp 兄兜底**——直接按区域分流到分支 3/4，与 micro 原生一一对应，语义更清晰、
> 极端矮视口下也更严谨。

---

## 附录：trace 预测（修复前 vs 修复后）

场景：空文件 paste 30 行，viewport height=46，scrollmargin=3，c.Line=29。

**修复前**：
```
>>> relocateVerticalMD ENTER c={L:29,R:0} ... height=46
    relocate: case=C displayStart={L:26,R:0}
>> displayToBuffer ENTER startLine={L:26,R:0} ... cap=92
   ... 只渲染 4 行内容 ...
<<< relocateVerticalMD EXIT nochange (case=C cursorRow=3 in [3,42] startLine={L:26,R:0})
→ showBuffer 从 line 26 blit，只显示 26-29 = 4 行
```

**修复后**：
```
>>> relocateVerticalMD ENTER c={L:29,R:0} ... height=46
    relocate: caseC-branch4 end-pin displayStart=0 (c=29 bEnd=29 height=46)
<<< relocateVerticalMD EXIT nochange (case=C cursorRow=29 in [3,42] startLine={L:0,R:0})
→ showBuffer 从 line 0 blit，显示全 30 行
```
