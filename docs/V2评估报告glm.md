# 光标滚动 V2 方案评估报告

> 评估对象：`docs/光标滚动的问题V2.md` 中的「方案 B（彻底重构）」
> 评估日期：2026-06-15

## 1. 根因诊断：正确 ✅

文档准确抓住了核心矛盾：

> **`viewportRowmap` 是上一帧 `displayBufferMD` 的产物，但 `Relocate()` 要回答的是下一帧光标该在哪。装饰行的有无取决于光标在哪段，所以光标一动，map 就失效。**

当前主循环时序（`cmd/micro/micro.go:DoEvent`）：

```
Display()          ← 用旧 cursor 生成 viewportRowmap
Show()
event = <-Events
HandleEvent()
    MoveCursorDown
    Relocate()     ← 用【上一帧、已失效的】map 判定滚动量 ← V2 bug 诞生地
```

装饰行（表格 frame / 代码块边框）的存在是 cursor 位置的函数，map 永远落后一帧。这是**架构断层**，靠改 `relocateVerticalMD` 内部判定逻辑无法解决——输入数据本身就是错的。

## 2. 方案 B 分两部分看

| 子改动 | 内容 | 价值 | 代价 |
|--------|------|------|------|
| **B1 精神** | `Relocate` 前先 preRender，判定基于新鲜 map | ⭐⭐⭐⭐⭐ 直接根治 | 中 |
| **B2 彻底重构** | `displayBufferMD` 不再 render，只 blit `renderedMap` | ⭐⭐ 省一次渲染、架构"更纯" | ⭐⭐⭐⭐⭐ 巨大 |

**B1 必须做，B2 过度工程。**

### B2 代价被严重低估

文档说得很轻：「直接把 render 后的数据储存在 renderedMap，`displayBufferMD` 直接 blit」。但现状不是这种结构：

- `renderSegmentMD`（`bufwindow_md.go:61`）里 `screen.SetContent` 与 `viewportRowmap[vY]=...` **耦合在同一次循环**。分离要把 `expandLineStyles`、装饰行 `effectiveLine`、`cellBg`/`defBg` 合并、Bold/Italic 叠加……每条 cell 逻辑都改。
- `renderSegmentNative`（`bufwindow_md.go:210`）直接复制了 micro 原生显示特性栈：`matchingBraces`、`showchars`、`cursorline`、`color-column`、`hltrailingws`、selection 高亮、`showcursor`……这些与「屏幕坐标 + `cursors`」深度绑定，无法「渲染完暂存」再 blit——`showcursor` 必须在 blit 时按当前 cursor 位置画。

**当前 render 函数不是 `f(state)→Row[]`，而是 `f(state, screen)→副作用`。** 把它变成前者 = 重写一遍 micro 的显示核心，违反 AGENTS.md「对 micro 原生代码侵入越小越好」。

### B2 的额外收益其实很小

唯一收益是「省一次 render」。但 micro 主循环本来就是**每帧全量重算**，2 次渲染对于 `bufHeight ≤ 终端高度`（一般 ≤ 50）的规模毫无压力。

## 3. 推荐：变体 B'（影子渲染）

保留 micro 每帧全量重渲染节奏，只在一个地方插刀：**`relocateVerticalMD` 入口先做一次「只算 map、不写 screen」的影子渲染**。

```go
func (w *BufWindow) relocateVerticalMD(c SLoc, scrollmargin, height int) bool {
    w.shadowRenderMap(c)   // 基于新 cursor 重算 map（不写 screen）
    cursorRow, ok := w.LineToScreenRow(c.Line, c.Row)
    ...                    // 判定逻辑不变，但 map 新鲜
}
```

**实施要点：**
1. render 函数加 `writeScreen bool` 参数（或拆成 `computeLayout`/`blit`）。侵入仅限 MD 路径，micro 原生 `displayBuffer` 不动。
2. shadowRender 用 `oldStartLine` 起步，渲染到 `2*bufHeight` 这种粗暴上界，避免「鸡生蛋」迭代。
3. 可选：判定完拿新 `StartLine` 后，让 `Display()` 复用同一份 map 对应切片——顺便吃到 B2 部分好处，但不强制。

**好处：**
- 根治 V2：map 反映新 cursor，装饰行展开后再算 scroll，量准确。
- 顺带解决「跨段切换光标消失一帧」（总结文档 §5 已知限制 1、2）。
- 零侵入主循环：30+ 个 `Relocate()` 调用入口不动，刀只落在 `relocateVerticalMD` 一个函数内。

## 4. V2 伪代码里 5 处必须先明确的坑

1. **`preRender` 停止条件有鸡生蛋问题**：scroll down 时新视口底部位置是 Relocate 输出，preRender 时未知。解：直接超量渲染 `2*bufHeight`，丢弃「迭代收敛」。
2. **超大段性能/内存爆炸**：2000 行的代码块/表格段要全渲染。解：设上限（如 `3*bufHeight` 截断 + nativeFallback 兜底），文档未提。
3. **多光标**：`hasCursorInside` 现已支持多 cursor，但方案 B 未提多光标。建议：只看主 cursor，其它不影响 editMode 判定。
4. **editMode 切换时机**：当前 `observeEditModeToggle` 在 `Relocate` 之后，Relocate 用的是旧 editMode。方案 B 必须明确：要么提前 toggle、要么 preRender 内推断。
5. **向上对称问题**：总结文档 §1 提到「按 ↑ 到顶边会偶发多余滚动」，向上分支 `w.Scroll(c, -scrollmargin)` 同样基于 1:1 假设，对 MD 装饰行**也不准**。方案 B 应一并覆盖。

## 5. 结论

| 项 | 结论 |
|----|------|
| 根因诊断 | ✅ 准确 |
| 方向（先渲染后判定） | ✅ 正确 |
| 完全版 B2（Display 改 blit） | ❌ 过度工程，收益小代价大，违反低侵入原则 |
| **推荐变体 B'（影子渲染）** | ✅ 性价比最高，根治 V2 + 顺带修跨段消失一帧 |
| 伪代码落地细节 | ⚠️ 5 处坑需先敲定再动代码 |

**建议：** 推进前先写一份 `docs/plan-光标滚动V2-Bprime.md`，敲定 §3、§4 的实施细节，再动代码。
