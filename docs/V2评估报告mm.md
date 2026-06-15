# 方案 B 评估（2026-06-15）

> 评估对象：`光标滚动的问题V2.md` §三「方案B，彻底重构」
> 参考：`光标滚动-修改总结.md`（v1.0.5）+ 当前代码
> 约束：侵入 micro 原生代码越小越好

---

## 0. 结论

**方案 B 治本，但当前不是时机。** V2 bug 在 V1 框架内能用最小补丁修，不必为它动整个 event 路径。建议先实施方案 C（§4），观察同类问题是否复现，**复现 ≥ 2~3 个场景**再立项方案 B，且**先消歧 §5 的问题再写代码**。

---

## 1. V2 bug 根因（V1 代码追踪）

T0：cursor.line=59 落在 table segment 内，editMode=true → 整段走 `renderSegmentNative`（无装饰行），`viewportRowmap` 1:1。
T1：用户按 ↓，cursor.Y=60。
T2：`relocateVerticalMD` → `LineToScreenRow(60,0)` 失败（map 里 line 60 还是 -2）→ 回退 `relocateVerticalNativeFallback`（1:1 假设）→ 改 StartLine，**不知道新位置会有装饰行，scroll 量算少**。
T3：display 时 table 段已不在 cursor 内 → 改走 `renderSegmentMD`（带装饰行，5→9 行）→ table 视觉变大，cursor 被挤出 viewport。

**修法**：T2 末尾 `LineToScreenRow` 失败时，判断"是不是刚跨出 segment"——是的话干渲染一次刷新 map，再算 scroll。改动局限在 `bufwindow_md.go` 一个函数。

---

## 2. 方案 B 的核心机制

把"渲染+显示"拆成「预渲染到 `bigViewportMap` → 决策 → blit」：
- event 路径上调 `preRender(startLine, showCursor)` 渲整个 viewport 到 map
- scroll 决策和显示都用同一份 map（真值），无时序分裂
- `displayBufferMD` 变成纯 blit

**机制正确**：`bigViewportMap` 是新状态真值，scroll 量算得准。

---

## 3. 方案 B 的 5 个问题

**3.1 时序坑没消除，只是搬家。** V1 的坑是「`Relocate` 时刻的 map 是上一帧的」；方案 B 变成「`Relocate` 必须真渲染，渲染依赖的 MDSegments 必须是新的」。把"Relocate 不渲染"的清爽语义打破了。

**3.2 伪代码自相矛盾。** "光标段原渲染特权"——伪代码说"光标行 native"，文字说"整段 native"。V2 场景下两者结果不同：前者光标行 wrap 数与 MD 不同，`cursorRow` 难精确算；后者和 V1 一样，跨段才切 MD，**V2 bug 仍会复现**。

**3.3 case B（scroll）有歧义。** "直接粗暴估算 startLine" + "preRender 不再校正" = 估算错了就显示错位置（cursor 在视口外）。V1 的"scroll 量不准"在方案 B 里变成"视图可能错"。

**3.4 跨帧状态引入失效管理。** V1 的 `viewportRowmap` 每帧新算，零失效问题；方案 B 的 `bigViewportMap` 跨帧持有，**`undo/redo`、多光标、命令面板、Goto、search match** 等路径都要保证 invalidate 或重算，复杂度剧增。

**3.5 违反"侵入最小"原则。** actions.go 不动（好），但 `bufwindow_md.go` 大改、新增 `preRender` + `bigViewportMap`；渲染从 display 阶段挪到 event 阶段，hot path 每次按键都跑一次完整 render（work 量不变，分布变了）。

---

## 4. 推荐：方案 C（最小补丁）

在 `relocateVerticalMD` 的 `LineToScreenRow` 失败分支加 1 个判断 + 干渲染重试：

```go
if !ok {
    // 检测：光标是否刚跨出 viewport 最后一个 content 行
    lastContent := -1
    for _, v := range w.viewportRowmap { if v.Line >= 0 { lastContent = v.Line } }
    if lastContent >= 0 && c.Line == lastContent + 1 {
        w.dryRender()  // 复用 displayBufferMD 但不画 screen，只更新 viewportRowmap
        cursorRow, ok = w.LineToScreenRow(c.Line, c.Row)
        if ok { /* 走正常 scroll 判定 */ }
    }
    return w.relocateVerticalNativeFallback(c, scrollmargin, height)
}
```

**优势**：只动 `relocateVerticalMD` 一个函数；不改 event 路径；不改数据流；V1 已有路径全保留；只有跨段边界多一次干渲染（按键时）。
**风险**：`dryRender()` 需复用现有渲染但跳过 `screen.SetContent`；或干脆真渲一次（<1ms）。

---

## 5. 方案 B 实施前需消歧的 7 个问题

1. **光标段原渲染特权**：整段 native 还是只光标行 native？
2. **case B 估算错**：preRender 渲估算位置还是校正后位置？
3. **preRender 停止条件**："光标之后再渲 scrollmargin 行"只在 `showCursor=true` 时评估？
4. **bigViewportMap 失效**：MDSegments invalidate 时是否同步？还是要自己校验 staleness？
5. **Resize 后**：bigViewportMap 怎么重算？
6. **多 BufPane / split**：bigViewportMap 是 pane-private 还是 buffer 级？
7. **editMode 切换（ESC）**：case B 里 preRender 起什么作用？

---

## 6. 一句话

方案 B 解决的是"v1 的根本设计缺陷"，但当前 V2 bug 只是"v1 边界 fallback 不够"，**两者不匹配**。先 C 修 bug，B 留作 v2.0 立项。
