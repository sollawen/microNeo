# 方案A 架构设计评估

> 评估对象：`docs/光标滚动-方案A-架构设计.md`
> 评估日期：2026-06-16
> 评估者：pi (planner)
> 评估依据：
> - 方案A架构设计文档 + 方案A原始思路 + 方案B + 修改总结 + 方案B两份评估报告
> - 当前代码：`internal/display/bufwindow.go`、`internal/display/bufwindow_md.go`、`internal/action/bufpane_md.go`、`internal/buffer/buffer.go`、`internal/md/detect.go`、`internal/md/md.go`、`internal/md/render_table.go`、`cmd/micro/micro.go`

---

## 0. 结论

**方案A方向正确，方案C（`光标滚动-方案A.md`原始思路）已经验证过；本文档的工程化是必要的，但有 4 处需要在实施前澄清/修正。**

| 维度 | 评分 | 评语 |
|------|------|------|
| 根因诊断 | ⭐⭐⭐⭐⭐ | 与代码完全对得上（§9.1/9.2 时序图正确） |
| 触发条件 | ⭐⭐⭐⭐ | "段归属变化" 优于原思路的 "Line +1"，但语义有微妙偏差（见 §1） |
| 架构抽象 | ⭐⭐⭐⭐ | target+dryRun 双参数是合理选择，避免路径漂移 |
| 边界覆盖 | ⭐⭐⭐ | 边界表完整，但 `b.ModifiedThisFrame` 副作用未提及（见 §2） |
| 性能评估 | ⭐⭐⭐ | "段内移动零开销" 表述与当前 Segment 粒度矛盾（见 §3） |
| micro 侵入 | ⭐⭐⭐⭐⭐ | 零侵入主循环 + actions + softwrap，原生路径不动 |
| 与方案B 关系 | ⭐⭐⭐⭐⭐ | "Plan A 是 Plan B 的第一阶段" 论证成立 |

---

## 1. 根因诊断 — 100% 正确

对照 `bufwindow.go:248-251`：

```go
if w.Buf.IsMD {
    ret = w.relocateVerticalMD(c, scrollmargin, height)
} else {
    // ... micro 原生 ...
}
```

`relocateVerticalMD` 读取 `w.viewportRowmap`（`bufwindow_md.go:867-887`），而该 map 由上一帧 `displayBufferMD` 重建（`bufwindow_md.go:718-724`）。光标先 `MoveCursorDown` 再 `Relocate`（`actions.go` 调用序列）的时序坑，方案A 的分析完全准确。

**对称场景**（↑ 进入 table）也是同一个根因：上一帧 cursor 在 line 54，table 段是 MD 渲染（9 row），map 里 line 54 占 1 row；本帧 cursor 到 line 55，table 段仍走 MD 渲染（9 row，但 line 55 现在是 table 起始），map 不变但 `LineToScreenRow(55, 0)` 找的位置不再准确。方案A的 `cursorSegmentChanged` 也会触发 preRender，覆盖此场景。

---

## 2. 必须澄清/修正的问题

### 2.1 【关键】`b.ModifiedThisFrame` 在 preRender 路径中会被清掉

**问题**：`renderSegmentNative`（`bufwindow_md.go:235-242`）开头有：

```go
if b.ModifiedThisFrame {
    if b.Settings["diffgutter"].(bool) {
        b.UpdateDiff()
    }
    b.ModifiedThisFrame = false
}
```

若 preRender 先跑（dryRun=true）→ 触发 `UpdateDiff()` → 清 `ModifiedThisFrame`
随后 Display() 真渲 → 看到 `ModifiedThisFrame == false` → 跳过 `UpdateDiff()`

**影响**：
- `UpdateDiff` 设计上是幂等的，所以结果正确，但**多调一次**
- 若 `UpdateDiff` 后续改为非幂等（例如带缓存或计数器），preRender 会破坏契约
- 真渲路径不再"有副作用"，与"Display 只画屏"的直觉有偏差

**建议**：在 `renderSegmentNative` 头部加 `if !dryRun` 守卫：

```go
if !dryRun && b.ModifiedThisFrame {
    if b.Settings["diffgutter"].(bool) {
        b.UpdateDiff()
    }
    b.ModifiedThisFrame = false
}
```

`renderSegmentMD` 也应做同样审计（我看了一遍，没有类似副作用，但请在实施时再核一遍）。

**文档遗漏**：§12.1 风险表只提到"行为不一致"和"漏触发"，**没有提到 `b.ModifiedThisFrame` 这类状态副作用**。建议在 §6.1 dry-run 不变量清单里明确写出。

---

### 2.2 【关键】"段内移动零开销" 表述与当前 Segment 粒度不符

**问题**：§11.1 说"段内编辑/移动（绝大多数按键）零开销"。

但 `internal/md/detect.go:140-144` 的 Normal 状态：

```go
} else {
    segments = append(segments, Segment{
        BufStartLine: y,
        BufEndLine:   y,
        Render:       RenderNormal,
    })
}
```

**Normal 是单行段**。从 line 60 按 ↓ 到 line 61：
- 上一帧 `lastCursorSegIdx` = `segmentOfLine(60)` = Normal 段 A
- 本帧 `curSeg` = `segmentOfLine(61)` = Normal 段 B
- A ≠ B → `cursorSegmentChanged` = true → **preRender 触发**

也就是说，**在普通文本中按一次 ↓ 就会触发一次 preRender**，并非"零开销"。

**可能影响**：
- 性能上：preRender 一次约 1ms 量级，可接受
- 概念上：preRender 的优化假设（"跨段是低频事件"）不成立
- 进一步推论：**preRender 的真正价值只在于"段渲染放大倍率变化"的场景**（table 5→9、codeblock N→N+2、blockquote/list 等），而不是"任何跨段"

**两种修法路线**：

| 路线 | 改动 | 收益 |
|------|------|------|
| A. **文档修正 + 接受现状** | 把 §11.1 改成"每次按键最多 1 次 preRender，preRender 只算 map 不写屏，开销约 1 次额外 displayBufferMD"；实施时确保 preRender 真的能跑得动 | 文档诚实；实施影响小 |
| B. **改 trigger 条件** | 在 `cursorSegmentChanged` 之上再加一层"两段的 Render 函数是否相同"判断（同一 Render 函数 → 视为段内）；或者更精确：只对渲染放大倍率不同的相邻段（table/codeblock/list/blockquote/heading vs normal）触发 preRender | 真正的"段内零开销"；但需要枚举"放大倍率不同"的段类型对，复杂 |

**我建议路线 A**：单行 Normal 段的 preRender 工作量很小（renderSegmentNative 跑 1 行），不值得为了"省 1ms"增加 trigger 复杂度。文档诚实即可。

如果想兼顾，可以提一个简单优化：`if curSeg.Render == nil || lastSegRender == nil || reflect.ValueOf(curSeg.Render).Pointer() == reflect.ValueOf(lastSegRender).Pointer() { return false }` —— 用 Render 函数指针比较判断"同种段"。但这是锦上添花。

---

### 2.3 【次要】`drawGutterAndLineNumMD` 的 dryRun 传播未展开

**问题**：§6.2 只说"renderSegmentMD 中 `drawGutterAndLineNumMD` 调用同理加守卫"。

`drawGutterAndLineNumMD` 内部直接调 `w.drawGutter` / `w.drawDiffGutter` / `w.drawLineNum`，而这些函数又直接 `screen.SetContent`。

**dryRun 传播方式有两种**：
- (a) `drawGutterAndLineNumMD(vY, bufLine, softwrapped, dryRun bool)` —— 改签名，向下传播
- (b) `w.drawGutterAndLineNumMD` 读 `w.dryRun` 字段 —— 临时状态，侵入大且不线程安全
- (c) `drawGutterAndLineNumMD` 不参与 dryRun，改为 caller 自行在 dryRun 模式下跳过调用

**建议**：方案 (a)，签名增加 `dryRun`。理由：和 `renderSegmentMD` 的 `dryRun` 参数风格一致，没有引入隐藏状态。

**注意**：`renderSegmentNative` 复制了 `drawGutter` / `drawDiffGutter` / `drawLineNum` 的全部调用点（`bufwindow_md.go:300-317` 等），所以**这两条函数的 dryRun 守卫要分别加两遍**（一次给 `renderSegmentMD` 调用，一次给 `renderSegmentNative` 调用）。或者更彻底：把 `w.drawGutter` 等改为 `drawGutter(w, ..., dryRun)`，但这会动 bufwindow.go 的原生路径 —— 违反"零侵入"原则。**不可行**。

所以**最小侵入方案是 (a)**：`drawGutterAndLineNumMD` 加 `dryRun` 参数，renderSegmentMD 和 renderSegmentNative 都加守卫。

---

### 2.4 【次要】`renderSegmentNative` 约 500 行，dryRun 守卫散落有风险

**问题**：`renderSegmentNative` 几乎是 `displayBuffer` 主体循环的复制粘贴。里面所有 `screen.SetContent` / `drawGutter` / `drawDiffGutter` / `drawLineNum` / `showCursor` 调用都需要加 `if !dryRun` 守卫。

**风险**：
- 漏加 → 调试时偶发 dryRun 也写屏的 bug，难复现
- 加错位置 → 性能或行为异常
- 散落 `if !dryRun` 代码可读性差

**建议**：
- **首选**：照 §7.3 的建议，**先复制粘贴主循环**，第一版只对 `renderSegmentMD` 加 dryRun（因为它体量小，125 行），让 `preRenderRowmap` 在第一版只用 `renderSegmentMD` 路径。
- **延后**：`renderSegmentNative` 的 dryRun 在第二版再启用（如果发现 normal 段跨段时也有同类问题）。
- 验证：单测 `TestRenderSegmentMD_DryRunMatchesWetRun` 必加；`renderSegmentNative` 的等价单测可延后。

**对原方案的影响**：原本说"preRender 一次干渲染能完整覆盖"被削弱为"preRender 一次 partial 渲染，对纯 normal 段跨段可能仍走 viewportRowmap 旧值"。但纯 normal 段跨段没有"装饰行"问题，viewportRowmap 也只是 1 行对应 1 行的关系，**实际上没问题**。

这是个好消息：实际需要的 preRender 范围**比方案A自述的更小**。可以借此简化实施。

---

## 3. 设计文档中可改进的表述

### 3.1 §4.1 字段命名 `preRowmap` 易与 `viewportRowmap` 混淆

`preRowmap` 这个名字读起来像"预先渲染出来的 rowmap"，但**不是 viewport 的预渲染结果**，而是"基于新段归属的、给本次 Relocate 用的临时 map"。

**建议改名为 `dryRunRowmap`** 或 `tmpRowmap`。理由：与 `dryRun` 参数同根，grep 友好；语义清晰（"dry-run 渲染出的 map"）。

但这是 cosmetic，无功能影响。

### 3.2 §5.2 字段未明示初始化

`lastCursorLine` 和 `lastCursorSegIdx` 没有在 `NewBufWindow` 显式初始化为 0/-1。Go 的零值正好是所需值（`int` 零值 0 = "上一帧 cursor 在 line 0"，`-1` = "无效"），但**显式写出更易读**：

```go
func NewBufWindow(...) *BufWindow {
    w := new(BufWindow)
    ...
    w.lastCursorSegIdx = -1 // 标记 "无上一帧信息"，首帧必触发
    return w
}
```

### 3.3 §6.3 备选方案 B（抽纯函数）被低估

> "与现有 render 函数有大量重复逻辑（map 构建部分），未来易漂移"

但 `renderSegmentMD` 的 map 构建部分（`bufwindow_md.go:128-143`）约 15 行；与"render 函数"主体（125 行）相比是 12% 占比。**抽纯函数的代价没有 B 选项描述的那么大**。

**反观点**：抽 `computeSegmentRows(seg, vY, target, editMode) (newVY, err)` 纯函数确实可以避免 dryRun 与真实路径漂移，而且测试更容易（不依赖 screen）。**这是一个值得在 Commit 4 重构阶段考虑的方案**，与方案A当前的设计不冲突（甚至可以平滑迁移）。

### 3.4 §15 "Plan A 是 Plan B 的第一阶段" 论证可加强

如果未来要上 Plan B，**有哪些 Plan A 的资产可以复用**？文档没有列出。补充一句会更稳：

- `renderSegmentMD/Native` 的 `target + dryRun` 参数 → Plan B 的"算 map 不画屏"直接可用
- `preRowmap` 字段 → Plan B 升级为 `bigViewportMap`（容量增大、跨帧持有）
- `cursorSegmentChanged` → 改写为 Plan B 的 invalidate 判定
- `relocateVerticalMD` 读 rowmap 的逻辑 → Plan B 不动

---

## 4. 边界场景审计（基于代码现状）

我对 §10 边界表逐条审计了代码：

| 场景 | 审计结果 | 备注 |
|------|----------|------|
| 首帧 / MDSegments 为 nil | ✅ | `cursorSegmentChanged` 第一个 if 拦截 |
| 光标跳出 viewport（大跳转） | ✅ | preRender 仍跑（cursorSegmentChanged=true），`LineToScreenRowIn` 找不到 → fallback |
| cursor 在装饰行 | ✅ | cursor 永远不在装饰行（`c.Line` 是 buffer 行号） |
| 多光标 | ✅ | preRender 用 `b.GetCursors()`，与现状一致 |
| 连续按 ↓ 跨多个段 | ✅ | 每次独立判定 |
| 编辑动作导致段边界变化 | ⚠️ | **MDSegments 的更新依赖 `buffer.go:1032`，而 1032 在 re-highlight 时跑（异步）。如果 preRender 在 re-highlight 完成前跑，preRender 用的就是旧 MDSegments。** 这是一个已存在的 race（Display 也有），但方案A 把它放大了（preRender 多跑一次）。建议记录为"已知限制"，等 re-highlight 主线程化时一并解决。 |
| resize | ✅ | preRender 内部 `if cap < bufHeight` 重新分配 |
| softwrap 开启 | ✅ | `vloc.Y -= w.StartLine.Row` 在 dry-run 路径同样适用 |

**新增发现**：方案A的 preRender 复用 `b.MDSegments`，但 `b.MDSegments` 可能在 re-highlight goroutine 完成后才更新（`buffer.go:462-470`）。如果用户编辑时按 ↓，preRender 跑在旧 MDSegments 上，**新段归属可能仍不准**。这个 race 现有 Display 也有（Display 也是读 `b.MDSegments`），但 Display 是同一帧调用、误差在 1 帧内可接受；preRender 跑在 Relocate（Display 之前）也是 1 帧内，**同样的容忍度**。所以无新增问题，但建议在文档里加一句"preRender 与 Display 共享 MDSegments 的 staleness tolerance"。

---

## 5. 风险矩阵（补充 §12 缺失项）

| 风险 | 概率 | 缓解 | 文档是否提及 |
|------|------|------|--------------|
| `b.ModifiedThisFrame` 被 preRender 提前清掉 | 中 | `if !dryRun` 守卫 | ❌ 需补 |
| `renderSegmentNative` 500 行中漏加 dryRun 守卫 | 中 | 实施时**先只对 renderSegmentMD 加 dryRun**，renderSegmentNative 留到第二版 | ❌ 需补 |
| `drawGutterAndLineNumMD` 内部 screen 调用未守卫 | 高 | 改签名加 `dryRun`，向下传播 | ⚠️ 提了但未展开 |
| `filterSegmentsToVisible` 原地修改 `b.MDSegments[i].VisibleStart/End` 在 dry-run 路径会污染 Display 视图 | 低 | VisibleStart/End 是 viewport 元数据，每帧重算，无累积影响；但建议**在 preRender 中复制一份 segs 再改**（语义更清晰） | ❌ 需补 |
| re-highlight 异步 → MDSegments staleness | 低 | 与 Display 共享，1 帧容忍 | ❌ 需补 |
| 段缓存 `lastCursorSegIdx` 在 buffer 替换时不重置 | 低 | 当前 micro 不会热替换 buffer 关联的 BufWindow；如果未来支持，需要在 `SetBuffer` 里加 reset | ❌ 需补 |

---

## 6. 实施建议（与 §13 commit 计划对照）

整体 commit 切分合理，但建议在 Commit 1 之前加一个 **Commit 0**：

### Commit 0: 修复 1D 测试残留 + 重建 test 脚手架
- 现状（§14）：`bufwindow_md_test.go` 已删除，`go test ./internal/display/` 编译通过但无测试
- 建议：从 `TestRenderSegmentMD_BasicMap`、`TestLineToScreenRow_RoundTrip` 开始建 1-2 个 sanity 测试，确保后续 dryRun 一致性测试有 baseline
- 否则 Commit 1 的 `TestRenderSegmentMD_DryRunMatchesWetRun` 没有"湿路径"基线可比

### Commit 1-4 调整：
- Commit 1：只对 `renderSegmentMD` 加 `target + dryRun`（**不要碰 renderSegmentNative**），减少 75% 改动面
- Commit 2：preRender 只走 `renderSegmentMD` 路径
- Commit 3：接入 Relocate，手测核心场景
- Commit 4：再决定是否把 `renderSegmentNative` 也加 dryRun

**这样 Commit 1-3 总改动 < 200 行**（§14 的"中"规模估计是包含 native 的，含 native 会到 500+ 行）。

---

## 7. 一句话总结

方案A 的**架构方向 100% 正确**，诊断清晰、改动收敛、对原生零侵入、与方案B 兼容。
但作为实施蓝图，有 **4 处需要澄清/修正**（`b.ModifiedThisFrame` 副作用、"段内零开销" 表述与现状不符、`drawGutterAndLineNumMD` dryRun 传播未展开、`renderSegmentNative` 改动面过大）。
建议在落地前补一份 **风险补丁文档**（参考本评估 §2、§5），并把 Commit 1 拆细（先只动 `renderSegmentMD`），可以显著降低实施风险。
