# F1a · FloatFrame 重构 — 实施方案

**状态**：草案（待评审）
**依据**：F0a（设计，v1 定稿）+ 源码现状（`floatframe.go` / `selectpane.go`）
**前置**：无代码前置（本方案是 FileSelector 整条线的第一步）。设计上依赖 F0a 已定稿。
**交付**：`FloatOpenSpec` options 入参 + `AutoExpand` layout 开关 + `onCancel`/resize 即关，**一次过合入**，SelectPane（`:theme`）行为零回归。
**原生侵入**：**零**。`floatframe.go` / `selectpane.go` 均为 microNeo 自有代码（非原生 micro）。

---

## 1. 目标与范围

把 F0a 的三项耦合改造（A: Open 签名 options 化、B: AutoExpand layout 开关、C: resize 上提 + onCancel）**在一个提交里落地**，交付给 F1b 一个"FileSelector 可直接用"的 FloatFrame。

本方案只讲 **怎么改、按什么顺序、怎么验**——设计论证见 F0a，不重复。

改动落在两个文件：
- `internal/action/floatframe.go`（容器本体，287 行）
- `internal/action/selectpane.go`（唯一现存 callsite，228 行）

---

## 2. 前置与交付物

| 项 | 说明 |
|----|------|
| 设计前置 | F0a（已定稿，自包含） |
| 代码前置 | 无 |
| 交付能力 | `Open(FloatOpenSpec)`、`AutoExpand` 两模式、resize 即关 + `OnCancel` 钩子、`Close()` 清 `OnCancel` |
| 回归对象 | SelectPane（`:theme` 命令）—— 现成、可一键验证 |
| 不在本方案 | FileSelector 本体（→ F1b）、ADR-9 文档修订（低优先，F0a §6 已记） |

---

## 3. 任务分解

三个实现任务，严格按序。**T2 与 T3 编译耦合，必须同提交**（Open 签名一改，callsite 不跟着改就编不过）。

### T1 · 定义 `FloatOpenSpec` + 扩展 `FloatFrame` 字段（`floatframe.go`）

**目标**：引入新类型、扩展 struct，**不破坏现有编译**（Open 仍是旧签名，能编过）。

**改动**：

1. 新增类型（字段与语义照搬 F0a §3）：
```go
// FloatOpenSpec 是打开浮窗的全部入参（options 模式）。
type FloatOpenSpec struct {
    Anchor      Pos                             // AutoExpand=true: 展开中心点; false: 外矩形左上角(含边框)
    ContentSize Size                            // 纯内容尺寸(不含边框); FloatFrame 内部派生 outerW/outerH
    Title       string                          // 嵌入上边框的标签; 空串=纯横线
    FrameColor  tcell.Style                     // 边框色; 零值 = config.DefStyle
    Display     func(contentArea Rect)          // 画内容(收到的 area 已扣除边框)
    HandleEvent func(event tcell.Event)         // 处理键事件(resize 不会到达, FloatFrame 已拦截)
    OnCancel    func()                          // FloatFrame 自身被关(resize)时回调; 具体浮窗在此清理业务回调
    AutoExpand  bool                            // true: 锚点自适应展开(SelectPane); false: 钉死 Anchor(FileSelector)
}
```

2. `FloatFrame` struct（现 `floatframe.go:39`）追加两个字段：
```go
    onCancel   func() // FloatFrame 自身关闭时回调(F0a §4); Close() 清空
    autoExpand bool   // layout 模式(F0a §2.2)
```

**验收**：`make build-quick` 通过；此时尚未切换 callsite，行为无变化。

---

### T2 · 改造 `Open` 签名 + `HandleEvent` 拦截 resize + `Close` 清字段（`floatframe.go`）

**目标**：Open 改为 options 入参、layout 按 `AutoExpand` 分叉、resize 在容器层拦截。**此步导致 `selectpane.go` 编译失败**（见 T3 修复）。

**改动**：

1. **`Open` 签名与 layout 分叉**（现 `floatframe.go:75`）。整体结构保留（C1 检查 → 派生 outerW/outerH → 失败前置检查 → 存字段），只改入参形态和最后一步 layout：

```go
func (f *FloatFrame) Open(spec FloatOpenSpec) bool {
    if f.isOpen { return false }                       // C1 不变

    // —— 派生 outerW/outerH（不变）——
    outerW := spec.ContentSize.W + 2
    if titleW := len(spec.Title) + 6; titleW > outerW { outerW = titleW }
    outerH := spec.ContentSize.H + 2

    // —— 失败前置检查（不变, size-only 安全兜底）——
    w, h := screen.Screen.Size()
    iOffset := config.GetInfoBarOffset()
    bottomLimit := h - iOffset - 1 - 1
    if outerH > bottomLimit+1 || outerW > w { return false }

    // —— 存字段 ——
    ax, ay := spec.Anchor.X, spec.Anchor.Y
    if f.autoExpand && ay < 0 {                        // sentinel: 仅 SelectPane(autoExpand) 用, 贴 statusLine
        ay = bottomLimit + ay + 1                      //   放在 f.anchor 赋值前, 与旧代码结构一致
    }
    fc := spec.FrameColor
    if fc == (tcell.Style{}) { fc = config.DefStyle }
    f.anchor = Pos{X: ax, Y: ay}                       // 变换后(autoExpand) / 原值(false), 与旧代码等价
    f.contentSize = spec.ContentSize
    f.title = spec.Title
    f.frameColor = fc
    f.display = spec.Display
    f.handleEvent = spec.HandleEvent
    f.onCancel = spec.OnCancel
    f.autoExpand = spec.AutoExpand
    f.outerW, f.outerH = outerW, outerH

    // —— layout 分叉(F0a §3) ——
    if f.autoExpand {
        f.fx, f.fy = expandAnchor(ax, ay, outerW, outerH)
    } else {
        f.fx, f.fy = ax, ay                            // FileSelector: 直接采用, 不二次决策, 不 clamp(F0a §7: 调用者保证非负)
    }
    f.isOpen = true
    screen.Redraw()
    return true
}
```

   **关键决策**：sentinel（`ay<0`）是 SelectPane 的约定，**条件化为 `if f.autoExpand && ay<0`、放在 `f.anchor` 赋值之前**——既让 SelectPane 路径与旧代码逐字节等价（旧代码 sentinel 也在 `f.anchor` 赋值前），又让 FileSelector 走 `false` 时完全不经过 sentinel（anchor 恒正、pane 左上角）。

2. **`HandleEvent` 拦截 resize**（现 `floatframe.go:215`，当前是无条件转发）。加 resize 分支：

```go
func (f *FloatFrame) HandleEvent(event tcell.Event) {
    if !f.isOpen { return }
    if _, ok := event.(*tcell.EventResize); ok {
        cb := f.onCancel          // 先存(F0a §4.4 顺序)
        f.Close()                 // 关容器(内部已清 onCancel)
        if cb != nil { cb() }     // 再触发业务取消回调
        return                    // 不再转发给具体浮窗
    }
    f.handleEvent(event)
}
```

3. **`Close` 清 `onCancel`**（现 `floatframe.go:133`）。在现有清字段处加一行：
```go
    f.onCancel = nil   // 避免旧回调残留(F0a §4.4)
```
   **清理策略（仿现有惯例）**：Close 只清**引用类型**字段（`display`/`handleEvent`/`onCancel`——断 GC 引用）；值类型字段（`autoExpand`/`anchor`/`title`/`contentSize` 等）不清——下次 `Open` 必覆盖，且两处 callsite 都显式写 `AutoExpand`（F0a §2.2 零值契约），无残留风险。

4. **`expandAnchor` 注释**（现 `floatframe.go:230` 上方 doc）补一句：layout 现由 `AutoExpand` 选择，`expandAnchor` 只服务 `true` 模式。

**验收（独立看 T2）**：逻辑与旧版**逐字节等价**——`AutoExpand:true` 路径的 sentinel → `f.anchor` 赋值 → `expandAnchor` 三步顺序与旧代码完全一致（含 `f.anchor` 存变换后值，而非原始 `spec.Anchor`）。

---

### T3 · 适配 SelectPane（`selectpane.go`）— **与 T2 同提交**

**目标**：把唯一 callsite 切到新签名、删掉自写的 resize 分支。恢复编译。

**改动**：

1. **`Open` callsite**（现 `selectpane.go:74`）组装 `FloatOpenSpec`：
```go
spec := FloatOpenSpec{
    Anchor:      anchor,
    ContentSize: contentSize,
    Title:       title,
    FrameColor:  frameColor,
    Display:     s.display,
    HandleEvent: s.handleEvent,
    AutoExpand:  true,                              // SelectPane: 贴光标/贴 statusLine 展开(旧行为)
    OnCancel:    func() {                           // resize 即关时清理业务回调
        if s.onSelect != nil { s.onSelect(nil) }
    },
}
if !TheFloatFrame.Open(spec) {
    s.items = nil
    s.onSelect = nil
    if onSelect != nil { onSelect(nil) }
}
```
   `SelectPane.Open` **对外签名不变**（仍是 items/title/anchor/.../onSelect），翻译封装在内部——所以 `command_neo.go` 的 `:theme` 调用方零改动。

2. **删 resize 分支**（现 `selectpane.go:181-188` 的 `case *tcell.EventResize`）——FloatFrame 已拦截，resize 不再转发到这里。删除整段。Esc/Enter 分支保留（那两条是"用户主动操作"，仍由 SelectPane 自己处理；只有 resize 是"容器层统一关闭"）。

   **注意区分**：Esc（`selectpane.go` 保留）是键事件、走 SelectPane 自己的 `Close()+onSelect(nil)`；resize（删除）是容器层关闭、走 FloatFrame 的 `Close()+OnCancel`。两者最终都触发 `onSelect(nil)`，但触发方不同——删的是 resize 这条，不是 Esc。

**验收**：`make build-quick` 通过；`:theme` 行为与改前一致。

---

## 4. 执行流程与提交策略

### 4.1 执行流程（过程中一律不 commit）

执行过程中**不 commit**。端到端流程与角色分工：

1. **实施**（agent）：按 T1 → T2 → T3 顺序改代码，每步都不 commit。
2. **编译**（agent）：`make build`（完整，含 generate）通过。
3. **回归 + 人工验证**（用户）：用户手工运行 microNeo，逐项过 §5 清单（`:theme` 展开 / resize 即关 / 回调恰好一次 / Esc+Enter / 再打开），确认视觉零差异、无残留、无崩溃。
4. **commit**（agent，须用户确认）：**用户确认 OK 后**，才执行单次 commit。

任一步未过 → 回到步骤 1 排查，绝不带着问题 commit。

### 4.2 提交粒度

**推荐：单提交**（T1+T2+T3 一起）。F0a §0 已论证"三项放一起做，签名只 churn 一次"。整个重构是一次原子改动，单提交最清晰、回退也干净。

若想拆分便于 review：可拆 **T1（引入类型，编译不断）** + **T2+T3（切签名+适配，编译通过、行为等价）** 两个提交。**绝不能**让 T2 单独成一个提交（会留下编译断点）。

---

## 5. 验证与回归清单

照 F0a §8 逐项过。`make build`（完整，含 generate）确认无回归：

- [ ] `make build` 通过（`Open(FloatOpenSpec)` 编译通过）。
- [ ] **AutoExpand=true 回归**：`:theme` 打开 SelectPane，仍按原样贴 statusLine 上方展开——视觉与改前零差异。
- [ ] **resize 即关**：`:theme` 打开后终端 resize（拖窗口）→ SelectPane 关闭、回到编辑器、**无残留光标 / 重影**。
- [ ] **回调恰好一次**：在 `:theme` 的 onSelect 回调里打日志/断言，确认 resize 关闭时 `onSelect(nil)` 被**恰好调用一次**（验证 F0a §4.2 契约未破——这是本重构最容易埋雷的点）。
- [ ] **连续快速 resize / resize 瞬间正好在按键** → 不崩溃，状态干净。
- [ ] **resize 关闭后 `:theme` 能正常再次打开**（验证 `Close()` 清 `onCancel` 生效、无状态泄漏）。
- [ ] Esc / Enter 选中仍走 SelectPane 自己的路径（这两条没动）。

---

## 6. 风险与注意

| # | 项 | 应对 |
|---|----|------|
| 1 | **onSelect 漏调 / 重复调** | 严格按 §4.4 顺序（存 cb → Close → 调 cb）；`OnCancel` 内**不得再调 `TheFloatFrame.Close()`**（重入，F0a §4.3）；`Close()` 幂等 |
| 2 | **sentinel（ay<0）误作用到 false 路径** | 条件化为 `if f.autoExpand && ay<0`、放在 `f.anchor` 赋值前（T2 §1）；`false` 路径完全不经过 sentinel——FileSelector anchor 恒正 |
| 3 | **误删 Esc/Enter 分支** | 只删 `case *tcell.EventResize`；Esc/Enter 是用户主动操作、保留 |
| 4 | **`OnCancel` 零值** | 两处 callsite 都**显式写** `OnCancel`/`AutoExpand`，不依赖零值（F0a §2.2） |
| 5 | **notePane 误伤** | notePane 不走 FloatFrame（嵌入式 overlay），本重构与它无关 |

---

## 7. 工时估算

**0.5–1 天**。范围小（两个文件）、设计明确（F0a 已逐行写清）、回归对象现成（`:theme` 一键验）。主要时间在"回调恰好一次"的手工验证上。

---

## 附：与 F0a 的对应

| 本方案任务 | F0a 章节 |
|-----------|---------|
| T1（FloatOpenSpec 类型） | F0a §3 |
| T2 §1（Open 分叉） | F0a §2.2 / §3 |
| T2 §2（HandleEvent 拦截） | F0a §2.3 / §4.4 |
| T2 §3（Close 清字段） | F0a §4.4 |
| T3（SelectPane 适配） | F0a §6（改动清单） |
| §5 回归清单 | F0a §8（验证点） |
