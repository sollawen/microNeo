# Bug #7 — 连续 ↓ 跨入表格时误判 case C，StartLine 暴跳

> **定位**：`internal/display/bufwindow_md.go` `relocateVerticalMD` 的 case A/C 判定
> **类别**：case A/C 判定未考虑 blit 偏移（scrollup 后 startVY>0）
> **关系**：是 `方案B-实施总结.md` §8 Bug #2 修复的**不完整回归**——Bug #2 的修复对 goto 场景有效
> （fresh render，startVY=0），但对"scrollup 后继续 ↓"场景失效（startVY>0）。
> **摘要已并入** `方案B-实施总结.md` §8 Bug #7，本文保留完整调试 trace 供回溯。

---

## 1. 复现

- 文件：`docs/sample.md`
- 起始：光标在 line 54（1-indexed，即 buffer L53），正好处于 scrollMargin（屏幕倒数第 3 行）
- 操作：按一次 ↓
- 现象：光标准确移到 line 55（buffer L54，进入小表格段、editMode 正确切换）；
  **但整个内容大幅 scroll up**，StartLine 从 L23 直接跳到 L51（1-indexed line 24→52，跳了 28 行）
- 期望：只 scroll up 1 行，StartLine 应为 L24（1-indexed line 25）

---

## 2. 日志铁证

连续 7 次 ↓ 的关键 trace：

| # | cursor | 旧 StartLine | 新 StartLine | blit startVY | case | 正确? |
|---|--------|--------------|--------------|--------------|------|-------|
| 1 | L48 | L19 | L19 | 0 | A | ✓ |
| 2 | L49 | L19 | L19 | 0 | A | ✓ |
| 3 | L50 | L19 | L19 | 0 | A | ✓ |
| 4 | L51 | L19 | **L20** | 0→1 | A (scrollup 1) | ✓ |
| 5 | L52 | L20 | **L21** | 1 | A (scrollup 1) | ✓ |
| 6 | L53 | L21 | **L23** | 1→2 | A (scrollup 2) | ✓ |
| 7 | **L54** | L23 | **L51** | 2 | **C** | ✗ BUG |

关键行（bug 帧）：
```
>>> relocateVerticalMD ENTER c={L:54,R:0} curStartLine={L:23,R:0} scrollmargin=3 height=37
    relocate: case=C displayStart={L:51,R:0}     ← 应为 case A
    relocate: cursorRow=4 botMarginRow=33 (sbRows=74)
<<< relocateVerticalMD EXIT nochange (case=C cursorRow=4 in [3,33] startLine={L:51,R:0})
```

---

## 3. 根因分析

### 3.1 现象：误判为 case C

case C 把 `displayStart = c.Line - scrollmargin = 54 - 3 = L51`，于是 StartLine 被推到 L51。
case C 本是"cursor 跳到远处"的兜底——重新从 cursor 位置估算 StartLine。但这里 cursor 只下移了
1 行（L53→L54），明明是连续导航，却被误判成"跳远"。

### 3.2 旧 sb 中 L54 的位置

bug 帧的旧 sb 是 step 6 渲染的，`sb.startLine=L21`，关键迭代：

```
[iter16] top vY=35 seg=[L53..L53] ... renderSegmentNative → vY 35→36 (Δ=1)   ← L53 在 vY=35
[iter17] top vY=36 seg=[L54..L58] ... renderSegmentMD → vY 36→45 (Δ=9)       ← 表格段
```

L53 是单行 native 段，占 vY=35 一行。
L54..L58 是表格段，cursor 在 L53 时按 **MD** 渲染（Δ=9 行：上框 + 表头 + 分隔 + 数据行 + 下框）。
其中 **L54（表头）落在 vY=37**（vY=36 是表格顶部装饰边框，line=-1）。

所以 `rowIndexOf({L54, R:0})` = **curRow=37**。

### 3.3 致命的判定式

```go
if w.sb != nil && w.sb.coversLine(c.Line) {
    curRow, ok := w.sb.rowIndexOf(c)
    if ok && curRow < height {                    // ← BUG 在这里
        displayStart = w.StartLine                 // case A：cursor 在第一屏内
    } else {
        displayStart = SLoc{Line: c.Line - scrollmargin, Row: 0}
        caseLabel = "C"
        w.StartLine = displayStart
    }
}
```

判定式 `curRow < height` 用的是 cursor 在 sb 中的**绝对 vY**，隐含假设是"sb 第一屏 = [0, height)"。
但这个假设**只在 blit startVY=0 时成立**。

代入 bug 帧：
- `curRow = 37`（L54 在旧 sb 的 vY）
- `height = 37`
- `curRow < height` = `37 < 37` = **false** → 走 case C ✗

### 3.4 为什么 startVY>0 时假设失效

step 4/5/6 各做了一次 scrollup（cursor 触底 → 推 StartLine）：
- step 4：StartLine L19→L20，但 sb.startLine 还是 L19（case A 没重渲染 sb），所以 blit startVY 从 0→1
- step 5：sb.startLine 变 L20（新 case A 重渲染），StartLine L20→L21，blit startVY 仍 1
- step 6：sb.startLine 变 L21，StartLine L21→L23，blit startVY 从 1→2

**scrollup 推 StartLine，但 StartLine 推进的是 sb 内部的 blit 偏移**，sb 本身不重画。
所以 step 7 进入时 blit startVY=2，旧 sb 的**可见窗口是 [2, 2+37) = [2, 39)**，而不是 [0, 37)。

L54 在旧 sb 的 curRow=37，**完全落在可见窗口 [2, 39) 内**——cursor 根本没离开屏幕，
理应走 case A（保留 StartLine 连续性），却被 `[0, height)` 的错误窗口判定踢到 case C。

### 3.5 与 Bug #2 的关系

§8 Bug #2 的修复正是加了这条 `curRow < height` 检查，目的是防 goto 跳远时误用旧 StartLine：

> jump 到远处时，旧 sb 虽覆盖 cursor 行，但 cursor 已在 sb 的**第二屏**（`curRow >= height`）

这个修复对 **goto 场景**有效：goto 触发 fresh render，blit startVY=0，"第二屏"=[height, 2·height)，
`curRow >= height` 确实代表跳到第二屏。

但对 **scrollup 后继续 ↓** 场景失效：blit startVY>0 后，"第一屏"[0, height) 不再等于可见窗口。
本 bug 正是这条修复的盲区。

---

## 4. 修复方案

把判定从"sb 绝对第一屏"改为"sb 当前可见视口"：

```go
if w.sb != nil && w.sb.coversLine(c.Line) {
    curRow, ok := w.sb.rowIndexOf(c)
    // ★ 用可见视口 [startVY, startVY+height) 判断，而非 sb 绝对第一屏 [0, height)。
    //   scrollup 后 StartLine 在 sb 内部推进，blit startVY>0，
    //   可见窗口随之上移，[0, height) 不再代表可见区。
    startVY, startOk := w.sb.rowIndexOf(w.StartLine)
    if ok && startOk && curRow >= startVY && curRow < startVY+height {
        displayStart = w.StartLine // case A：cursor 在可见视口内
    } else {
        displayStart = SLoc{Line: c.Line - scrollmargin, Row: 0}
        caseLabel = "C"
        w.StartLine = displayStart
    }
} else {
    displayStart = SLoc{Line: c.Line - scrollmargin, Row: 0}
    caseLabel = "C"
    w.StartLine = displayStart
}
```

### 4.1 为什么 `rowIndexOf(w.StartLine)` 安全

`StartLine` 在上一帧 relocate 末尾被设置为以下之一：
- **case A 舒适区分支**：`StartLine` 不变，仍是上上帧 displayToBuffer 的起点 → 在旧 sb 范围内
- **case A SCROLLUP 分支**：`StartLine = slocAt(delta)` 或 decor-skip 后的内容行 → 是旧 sb 中某个
  实际写入的 (line, segRow) 二元组 → `rowIndexOf` 必中
- **case C 分支**：`StartLine = displayStart`，紧接着 `displayToBuffer(displayStart)` 把 sb.startLine
  设为同一值 → 下一帧 `rowIndexOf(StartLine)` 至少命中 sb.startLine 行（除非该行被可见性过滤）

唯一需要兜底的是 `startOk=false`（理论上不该发生，但防御性编程）：直接退化为 case C，行为安全。

### 4.2 验证（手算预期）

修复后 bug 帧应走 case A：
- `displayStart = StartLine = L23`
- `displayToBuffer(L23, true)` 重渲染
- 新 sb 中 L54（此时是 cursor 段，**native** 渲染，5 行而非 9 行）位置 ≈ vY=34
- `cursorRow=34, botMarginRow=33` → `delta=1`
- `slocAt(1)`：vY=1 是 `# Tables` 标题下划线装饰（line=-1）→ decor-skip 到 vY=2（L24）
- `StartLine = L24` ✓（scrollup 恰好 1 行，符合用户预期）

### 4.3 对其它场景的影响

| 场景 | blit startVY | 旧判定 | 新判定 | 影响 |
|------|--------------|--------|--------|------|
| 首帧 / 刚打开文件 | 0 | `curRow<height` | `curRow∈[0,height)` | 等价，无影响 |
| goto 跳远（Bug #2 场景） | 0 | `curRow≥height` → C | `curRow≥height` → C | 等价，保留 Bug #2 修复 |
| 连续 ↓ 未触底 | 0 或小 | A | A | 等价 |
| **scrollup 后继续 ↓（本 bug）** | >0 | **误判 C** | **正确判 A** | **修复** |
| scrollup 后 cursor 仍可见但靠底 | >0 | 可能误判 C | 正确判 A + SCROLLUP 微调 | 修复 |

新判定是旧判定的**严格细化**：在 startVY=0 时两者完全等价，在 startVY>0 时新判定才是正确的。
所以不会破坏现有通过的场景。

---

## 5. 教训（已并入 `方案B-实施总结.md` §9.2）

| 陷阱 | 表现 | 防范 |
|------|------|------|
| **"第一屏"假设依赖 blit 偏移=0** | 用 `[0, height)` 判断可见性，但 scrollup 后 blit startVY>0，可见窗口实际是 `[startVY, startVY+height)` | 任何"屏幕/可见"判断都要先算出当前 blit 偏移（`rowIndexOf(StartLine)`），用相对窗口而非绝对第一屏 |
| **修复的盲区**：单场景验证通过 ≠ 所有场景通过 | Bug #2 用 goto 验证 `curRow<height` 修复有效，但没测"scrollup 后继续 ↓" | 修复后要枚举**所有触发 scrollup 的路径**（连续 ↓、鼠标滚轮、search）回归测试 |
