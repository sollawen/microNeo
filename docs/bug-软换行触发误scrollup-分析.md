# Bug — 在 last line 输入触发 softwrap 时误判 case C，导致 viewport 暴跳

## 现象

- 光标停在文件的 last line（最后一行），且光标处于 viewport 中部（下方还有大片空白）。
- 持续输入字符，last line 变长；当 softwrap 触发（一行变两行视觉行）的瞬间：
  - 程序误以为位置不够，触发一次"scrollup"。
  - 但不是 scroll up 1 行，而是**重算 StartLine**，StartLine 直接跳到 `c.Line - scrollmargin`。
  - 用户视觉上感觉"viewport 像只能容纳 ~6 行"，原本中部的光标被挤到 viewport 顶部附近。

## 根因定位

代码位置：`internal/display/bufwindow_md.go` → `relocateVerticalMD` → caseJudge（约 1148-1190 行）。

### caseJudge 现有逻辑

```go
if w.sb != nil && w.sb.coversLine(c.Line) {
    curRow, ok := w.sb.rowIndexOf(c)          // ① 精确查 (line, segRow)
    // ★ Bug fix 1：回车新增行 / 连续 ↓ 场景
    if !ok && w.sb.lastLine >= 0 && c.Line == w.sb.lastLine+1 {
        if lastRow, lastOk := w.sb.rowIndexOf(SLoc{Line: w.sb.lastLine, Row: 0}); lastOk {
            curRow = lastRow + 1
            ok = true
        }
    }
    startVY, startOk := w.sb.rowIndexOf(w.StartLine)
    if ok && startOk && curRow >= startVY && curRow <= startVY+height {
        displayStart = w.StartLine            // case A：上帧 StartLine 仍有效
    } else {
        displayStart = SLoc{Line: c.Line - scrollmargin, Row: 0}  // case C：跳远
        w.StartLine = displayStart
    }
}
```

### 触发链（本 bug）

帧 N：last line 长度 80 字符（1 视觉行），光标在末尾 → `c = {lastLine, Row: 0}`。
- old sb 中存在 `{lastLine, 0}`（last line 只占 1 行）。
- `rowIndexOf({lastLine, 0})` 命中 → case A → StartLine 保持 `lastLine - 15`（假设光标在 viewport 第 15 行）。

帧 N+k（用户又输入若干字符）：last line 长度越过 bufWidth → softwrap → last line 变 2 视觉行。
- 光标在行末，自动随 wrap 落到第 2 视觉行 → `c = {lastLine, Row: 1}`。
- old sb（上一帧）只有 `{lastLine, 0}`，**`rowIndexOf({lastLine, 1})` 失败 → `ok = false`**。
- Bug fix 1 的条件 `c.Line == w.sb.lastLine+1` 不成立（`c.Line == lastLine == sb.lastLine`，不是 +1）。
  fix 1 是为"回车新增 buffer 行"设计的，**不是为"同 buffer 行 wrap 多了一行"设计的**。
- `ok` 保持 false → 走 else → `caseLabel = "C"` → `displayStart = c.Line - scrollmargin = lastLine - 3`。

### "viewport 只显示 6 行"的来源

case C 后 `displayToBuffer` 从 `lastLine - 3` 渲染：
- sb.rows[0] = {lastLine-3, 0}
- sb.rows[1] = {lastLine-2, 0}
- sb.rows[2] = {lastLine-1, 0}
- sb.rows[3] = {lastLine, 0}
- sb.rows[4] = {lastLine, 1}   ← 光标在这里
- cursorRow = 4，botMarginRow = height-1-scrollmargin ≈ 26 → 不再 scrollup。

但 **StartLine 已经从 `lastLine - 15` 跳到 `lastLine - 3`**，光标从 viewport 第 15 行跳到第 4 行。
视觉上：viewport 顶部被推到 `lastLine - 3`，光标上方只剩 3 行内容（再往上和往下都是文件尾空白）。
对用户而言就是"屏幕猛地向上挤了一下，光标被推到顶部附近，下方大量空白"，类似 viewport 缩成几行。

## 修复方案

在 caseJudge 增加**第 3 个兜底分支**：cursor 的 `c.Line` 在 old sb 内有记录，但 `c.Row` 超出记录的最大 segRow（说明是该 buffer 行变长后 wrap 出了新视觉行）→ 这是**连续编辑小位移**，应走 case A，不能误判 case C。

### 改动 1：caseJudge 增加分支

`internal/display/bufwindow_md.go`，紧跟现有 Bug fix 1 之后：

```go
// ★ Bug fix 2（本 bug）：行内编辑导致 softwrap 行数增加（如 last line 输入变长 wrap 多一行）。
//   cursor 还在同一 buffer line，但 Row 比 old sb 记录的最大 segRow 大。
//   这是连续编辑（视觉位移通常仅 1 行），应走 case A，绝不能误判 case C 跳远。
//   估算 curRow = old sb 中 c.Line 最大 segRow 的位置 + (新 Row - 最大 segRow)。
if !ok {
    if maxIdx, maxRow, found := w.sb.maxSegRowIndexOf(c.Line); found && c.Row > maxRow {
        curRow = maxIdx + (c.Row - maxRow)
        ok = true
    }
}
```

### 改动 2：新增 helper

```go
// maxSegRowIndexOf 返回 sb 中指定 line 的最大 segRow 的 (索引, segRow 值)。
// 用于行内编辑导致 wrap 行数变化时估算 cursor 在 old sb 中的位置：
//   old sb 只记录了 {line, 0..maxRow}，新 c.Row > maxRow → 用最大记录位置 + 差值估算。
func (s *screenBuffer) maxSegRowIndexOf(line int) (idx int, maxRow int, found bool) {
    if s == nil {
        return 0, 0, false
    }
    maxIdx := -1
    maxR := -1
    for i, r := range s.rows {
        if r.line == line && r.segRow > maxR {
            maxR = r.segRow
            maxIdx = i
        }
    }
    if maxIdx < 0 {
        return 0, 0, false
    }
    return maxIdx, maxR, true
}
```

### 行为验证（修复后）

帧 N+k（last line wrap 瞬间）：
- `rowIndexOf({lastLine, 1})` 仍失败。
- Bug fix 1 不命中。
- **Bug fix 2 命中**：old sb 中 lastLine 最大 segRow=0，索引=15；c.Row=1 > 0 → `curRow = 15 + 1 = 16`，`ok = true`。
- startVY = 0，16 ∈ [0, 0+height] → **case A** → `displayStart = w.StartLine`（保持 `lastLine - 15`，不变）。
- displayToBuffer 从 `lastLine - 15` 渲染：sb.rows[15] = {lastLine, 0}，sb.rows[16] = {lastLine, 1}。
- `cursorRow = 16`，botMarginRow ≈ 26 → 16 < 26，不 scrollup。
- StartLine 不变，光标从 viewport 第 15 行自然过渡到第 16 行（仅下移 1 行），viewport 其余区域纹丝不动。✓

### 边界与回归

| 场景 | 是否受影响 | 说明 |
|------|----------|------|
| 真·case C（goto/search 跳远） | 不受影响 | old sb 中根本找不到 c.Line → `maxSegRowIndexOf` 返回 `found=false` → 维持 case C |
| 普通方向键导航（同 line，Row 不变或变小） | 不受影响 | `rowIndexOf(c)` 命中 → Bug fix 2 不触发 |
| 回车新增行（cursor 行不在 old sb） | 不受影响 | 走 Bug fix 1（c.Line == sb.lastLine+1）|
| **行变长 wrap 多一行**（本 bug） | **修复** | Bug fix 2 命中 → case A |
| 行变长 wrap 多 N 行（极端，一帧内 wrap 数行） | 受影响但正确 | curRow 按 maxIdx+(c.Row-maxRow) 估算；若结果仍在可见窗口内 → case A；若已超出 → case C 修正。无论哪条路径，displayToBuffer 后的 cursorRow/botMarginRow 二次校验仍是最终把关 |
| 删除字符 wrap 行数减少 | 不受影响 | `c.Row` 只会变小或不变，`rowIndexOf` 命中或 `c.Row > maxRow` 不成立 |

---

## 验证步骤（等用户复现 + debug log）

由于当前是 PLAN 模式，先请用户做一次 debug 复现，确认根因后再落地代码改动。

1. `make build-dbg` 重新编译（开 `util.Debug=ON`）。
2. `rm -f /tmp/microNeo_debug.log` 清空日志。
3. 打开一个 .md 文件，光标放到 last line 末尾、viewport 中部。
4. 持续输入字符直到 softwrap 触发（肉眼看到"屏幕猛地向上挤"的瞬间）。
5. 立即停止操作，把 `/tmp/microNeo_debug.log` 提供给我。

我会在日志中重点核对最后一条 `relocateVerticalMD` 记录：
- `>>> relocateVerticalMD ENTER c={L:?,R:?}` — 预期 `c.Row=1`（wrap 后），但 `curStartLine.Row` 是之前的位置。
- `relocate: caseJudge curRow=? startVY=? ok=?` — 预期看到 `curRow=0, ok=false`（即 `rowIndexOf({lastLine,1})` 失败）。
- `relocate: case=C displayStart={L:?,R:0}` — 预期 case C，displayStart 跳到 `lastLine - 3`。

只要这条 trace 与上述预测吻合，即可确认根因并落地修复。
