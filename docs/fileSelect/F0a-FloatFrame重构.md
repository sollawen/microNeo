# F0a · FloatFrame 重构（Open 签名 / layout 开关 / resize 即关）

**状态**：v1 定稿（重写）。本文把三项相互耦合的 FloatFrame 改造合并成一次过，是这三项的权威设计，自包含。
**范围**：只讲 FloatFrame 容器本身的改造——`Open` 签名、layout 模型、resize 处理。不讲任何具体浮窗（SelectPane / FileSelector）的业务逻辑。
**下游**：F1（FileSelector 架构）把本重构作为**硬前置**；本重构先于 FileSelector 落地，SelectPane 是现成的回归验证对象。

---

## 0. 为什么三项放一起做

三项各自都不大，但都改 `FloatFrame.Open` 的签名、都改 `SelectPane` 的 callsite：

- **A**. `Open` 改为 `FloatOpenSpec`（options 模式）
- **B**. 新增 `AutoExpand` layout 开关
- **C**. resize 上提到 FloatFrame（+ `onCancel` 钩子）

分三次做 = `Open` 签名 churn 三次、SelectPane 回归三次。合并一次过，签名只动一次、回归只做一次。

---

## 1. 背景：现有 FloatFrame 的三个张力

容器本体在 `internal/action/floatframe.go`，单例 `TheFloatFrame`，模态、单浮窗（C1）、画框、清屏、生命周期、事件路由、"后写胜出"（`micro.go:497` 最后画）。容器很薄，但有三处不够用：

### 1.1 张力一：`Open` 签名在膨胀

当前 `Open` 是 6 个 positional 参数：

```
Open(anchor Pos, contentSize Size, title string, frameColor tcell.Style,
     display func(Rect), handleEvent func(tcell.Event)) bool
```

规划中 FileSelector 还需要 `onCancel`（见 C）和 layout 开关（见 B），会到 8 个。Go 到这个数该收手——positional 参数既难读、又每次加能力都要 churn 签名。

### 1.2 张力二：layout 只有"锚点展开"一种模型

`expandAnchor`（`floatframe.go:228`）假设调用者给一个**参考点**，FloatFrame 自适应向上下左右展开——服务 SelectPane 那种"贴光标弹小窗"：

```go
downSpace := bottomLimit - ay + 1   // 锚点下方到屏幕 statusLine 的行数
upSpace   := ay + 1                 // 锚点上方到屏幕顶的行数
switch {
case downSpace >= outerH: fy = ay              // 下方放得下 → 向下（锚点=顶边）
case upSpace >= outerH:   fy = ay - outerH + 1 // 否则向上（锚点=底边）
... // x 对称
}
```

但 FileSelector 要的是"**pane 派生的确定性矩形**"（F1 D2）：左上角和尺寸都由 pane 决定，**不让 FloatFrame 二次决策**。

现状（F1 旧版 D2 的做法）：FileSelector 传一组"恰好让 `expandAnchor` 退化为 no-op"的输入，靠数学不变式保证 `fx==anchor.X && fy==anchor.Y`。意图**隐式**——"可证但脆弱"，后人改宽度公式就可能破坏不变式。这种"靠巧合达成意图"是坏味道。

### 1.3 张力三：resize 处理分散

ADR-9（`docs/弹窗机制/弹窗框架设计.md`）当时判定"resize 因浮窗而异，不是公共能力"，于是 `HandleEvent` 把**所有事件（含 resize）一律转发**给具体浮窗。SelectPane 自己写 `resize → close`（`selectpane.go` 旧 181-188 行）。未来每个浮窗要各写一遍，行为可能不一致。

而实际上：modal 浮窗无珍贵状态（FileSelector 起始目录恒等于 buffer 目录、不记忆上次、无未保存输入），关掉重开几乎零成本——"resize 即关"是所有浮窗都能继承的公共能力。

---

## 2. 决策（三项）

### 2.1 A · `Open` 改为 `FloatOpenSpec`（options 模式）

把 `Open` 的入参收进一个 struct，调用点用命名字段书写，自文档化、未来加能力不 churn 签名。

### 2.2 B · 新增 `AutoExpand` layout 开关

`FloatOpenSpec` 加 `AutoExpand bool` 字段，让调用方**显式声明** layout 模型：

| `AutoExpand` | `Anchor` 语义 | 行为 | 谁用 |
|---|---|---|---|
| `true` | 展开中心点（一个屏幕参考点） | FloatFrame 跑 `expandAnchor`，自适应上下左右 | SelectPane（贴光标弹） |
| `false` | **外矩形左上角（含边框）** | FloatFrame 直接 `fx=Anchor.X, fy=Anchor.Y`，**跳过 `expandAnchor`** | FileSelector（pane 派生） |

这把张力二从"靠巧合让 expandAnchor 变 no-op"变成"显式说别展开"——意图清晰、零脆弱性（F1 的 D2 据此瘦身、原 R1 风险删除）。

**零值注意**：Go 的 bool 零值是 `false`。由于 `FloatOpenSpec` 是命名字段 struct，**两个现存 callsite（SelectPane / FileSelector）都显式写 `AutoExpand`**，不依赖零值默认。

### 2.3 C · resize 上提到 FloatFrame（+ `onCancel` 钩子）

`HandleEvent` 收到 `EventResize` 时，FloatFrame **直接 Close + 调 onCancel，不再转发**给具体浮窗。所有浮窗因此统一"resize 即关"。

关键：FloatFrame 需要 `onCancel` 钩子来通知具体浮窗清理业务回调（否则回调被吞，见 §4）。`onCancel` 经 `FloatOpenSpec` 传入。

---

## 3. `FloatOpenSpec` 契约

```go
// FloatOpenSpec 是打开浮窗的全部入参（options 模式）。
type FloatOpenSpec struct {
    Anchor      Pos         // AutoExpand=true: 展开中心点；AutoExpand=false: 外矩形左上角(含边框)
    ContentSize Size        // 纯内容尺寸(不含边框)；FloatFrame 内部派生 outerW/outerH
    Title       string      // 嵌入上边框的标签；空串=纯横线
    FrameColor  tcell.Style // 边框色；零值 = config.DefStyle
    Display     func(contentArea Rect)         // 画内容(收到的 area 已扣除边框)
    HandleEvent func(event tcell.Event)        // 处理键事件(resize 不会到达这里, FloatFrame 已拦截)
    OnCancel    func()        // FloatFrame 自身被关(resize)时回调; 具体浮窗在此清理业务回调
    AutoExpand  bool         // true: 锚点自适应展开(SelectPane); false: 钉死 Anchor 为左上角(FileSelector)
}

// Open 打开浮窗。返回 false = 没开成(已有浮窗在开 / 屏幕放不下), 调用方应 onSelect(nil) 透明返回。
func (f *FloatFrame) Open(spec FloatOpenSpec) bool
```

派生与检查（两种模式共用）：

```
outerW = max(ContentSize.W + 2, len(Title) + 6)   // title 可能撑宽
outerH = ContentSize.H + 2

失败前置检查(两模式都跑, size-only):
  if outerH > bottomLimit+1 || outerW > screenW → return false

layout(按 AutoExpand 分叉):
  if AutoExpand:  fx, fy = expandAnchor(Anchor.X, Anchor.Y, outerW, outerH)  // 现有逻辑, 含防御性 clamp
  else:           fx, fy = Anchor.X, Anchor.Y                                 // 直接采用, 不二次决策
```

**注**：`AutoExpand=false` 时，**位置合法性（非负、Anchor+outer 不越屏）由调用者保证**——FloatFrame 不替调用者纠偏（这才叫"完全听调用者"）。失败前置检查仍是 size-only 的安全兜底；对 FileSelector 而言它自己的 pane 预检已保证这个检查永远过。

---

## 4. resize 即关 + onCancel 回调契约

本节是原 F0a 的精华，必须想清楚——是本重构最容易埋雷的点。

### 4.1 现状：具体浮窗持有业务回调

`FloatOpenSpec` 之前，`Open` 只收 `display` / `handleEvent` 两个函数值。**FloatFrame 本身不知道 `onSelect` 的存在**。业务回调（如 SelectPane 的 `onSelect`、FileSelector 的 `onSelect`）持在具体浮窗里，由具体浮窗自己负责"关容器 + 调回调"的顺序：

```go
onSelect := s.onSelect        // 先存回调引用
TheFloatFrame.Close()         // 再关容器
if onSelect != nil {          // 再回调
    onSelect(...)
}
```

### 4.2 陷阱：直接 Close 会吞回调

如果 FloatFrame 在 resize 时只调自己的 `Close()`、不通知具体浮窗，那么**业务回调永远不会被触发**——调用方（如 `command_neo.go` 的 `:theme` 回调）不知道 picker 已被取消。

- 今天 `:theme` 的回调对 `nil` 是 no-op，所以"看起来没事"。
- 但这破坏了"**每次 Open 必有恰好一次 onSelect 回调**"的隐含契约；未来某个调用方若依赖该契约（如设置了"等待选择"的标志位），就会**泄漏 / 挂起**。

### 4.3 解法：`onCancel` 钩子

`FloatOpenSpec.OnCancel` 就是"FloatFrame 被自身关闭时，通知具体浮窗清理业务回调"的钩子。具体浮窗把"调 `onSelect(nil)`"接到 `OnCancel` 上：

```go
// SelectPane 的 spec 片段
OnCancel: func() {
    if s.onSelect != nil { s.onSelect(nil) }
},
```

**关键约束（重入安全）**：`OnCancel` 内部**不得再调 `TheFloatFrame.Close()`**——FloatFrame 已经在关，避免重入。`OnCancel` 只做"调业务回调"这一件事。

### 4.4 resize 处理顺序（FloatFrame 侧，对齐现有"先关再回调"约定）

```go
// HandleEvent 内
case *tcell.EventResize:
    cb := spec.OnCancel          // 先存
    f.Close()                    // 关容器(清自身状态, 含 OnCancel)
    if cb != nil { cb() }        // 再触发业务取消回调
    return                       // 不再转发给具体浮窗
```

- 关闭语义等同 Esc：业务回调收到 `onSelect(nil)`，调用方按"用户取消"处理。
- `Close()` 里顺手清空 `OnCancel = nil`，避免旧回调残留。

**备选（不推荐）**：FloatFrame 在 resize 时构造一个"合成 Esc"转发给具体浮窗，让浮窗走自己的取消路径。这能保住回调，但本质还是"转发给浮窗决定"，没拿到上提的好处，且引入"合成事件"的别扭。优先用 `onCancel` 钩子。

---

## 5. 现状对照

| | Before | After（本重构） |
|---|---|---|
| `Open` 签名 | 6 个 positional 参数 | `Open(FloatOpenSpec)` |
| layout 模型 | 只有锚点展开一种 | `AutoExpand` 两模式：展开 / 钉死 |
| FileSelector layout | （规划中）靠 no-op 巧合 | 显式 `AutoExpand=false` |
| resize 到达 FloatFrame | 转发给具体浮窗 | FloatFrame 拦截 → Close + OnCancel，不转发 |
| SelectPane | 自己处理 resize→close | 删掉这段（FloatFrame 已统一处理） |
| 回调契约 | 浮窗自己保证 | FloatFrame 经 OnCancel 保证 |
| 行为 | 各浮窗可能不一致 | 全部统一关闭 |

---

## 6. 改动清单

**`internal/action/floatframe.go`**
- 新增 `FloatOpenSpec` 类型（§3）。
- `Open` 签名改为 `Open(spec FloatOpenSpec) bool`；内部派生 outerW/outerH、失败前置检查、按 `AutoExpand` 分叉 layout（`true` 走 `expandAnchor`，`false` 直接用 `Anchor`）。
- `HandleEvent`：`EventResize` 时按 §4.4 顺序处理（存 cb → Close → 调 cb），不再转发；其余事件照旧转发。
- `Close()`：清空 `OnCancel`（及其它已清字段），幂等。
- 改 `expandAnchor` 上方注释：layout 现在由 `AutoExpand` 选择，`expandAnchor` 只服务 `true` 模式。

**`internal/action/selectpane.go`**
- 删除 `handleEvent` 里的 `case *tcell.EventResize` 分支（FloatFrame 已拦截，不会再转发到这）。
- `SelectPane.Open` 内部把现有入参组装成 `FloatOpenSpec` 再调 `TheFloatFrame.Open(spec)`：`AutoExpand: true`、`OnCancel: func(){ if s.onSelect!=nil { s.onSelect(nil) } }`。**SelectPane.Open 对外的签名不变**。

**调用方**（`command_neo.go` 的 `:theme`、`notepane.go` 的 receiver 选择）
- **无需改动**——它们调的是 `SelectPane.Open(...)`，不是 `FloatFrame.Open(...)`；`SelectPane.Open` 对外签名稳定，翻译逻辑封装在其内部。

**`docs/弹窗机制/弹窗框架设计.md`**
- ADR-9（resize 归具体浮窗）需修订为"resize 由 FloatFrame 统一关闭"。该文档后续会被重写，可低优先处理。

---

## 7. 边界与注意

- **关闭时序**：`Close()` 清空内部状态，下一帧主循环回到无浮窗状态，主编辑器 bufPane 自然重画。
- **重入安全**：`OnCancel` 不得再调 `TheFloatFrame.Close()`（§4.3）；`Close()` 本身幂等（多次调用安全）。
- **notePane 不受影响**：notePane 是常驻 pane（嵌入 BufPane），不是 FloatFrame 管的 modal 浮窗，resize 时它的行为由自身决定，与本重构无关。
- **`AutoExpand=false` 不纠偏位置**：FloatFrame 不替调用者 clamp Anchor（§3）。调用者负责给合法坐标；FileSelector 靠 pane 预检保证。
- **未来例外**：若以后出现"resize 后必须存活"的浮窗（如全屏 modal），再给 FloatFrame 加 opt-in 开关（如 `SurviveResize`）；v1 不做，YAGNI。届时它就是 `FloatOpenSpec` 的又一个字段——这正是 options 模式的红利。

---

## 8. 验证点

实现后确认：

- **签名**：`Open(FloatOpenSpec)` 编译通过；SelectPane / FileSelector（届时）两处 callsite 显式写 `AutoExpand`。
- **resize 即关**：`:theme` 打开 SelectPane → 终端 resize → SelectPane 关闭、回到编辑器、无残留光标 / 重影。
- **回调确实触发**：在 `:theme` 回调里打日志 / 断言，确认 resize 关闭时 `onSelect(nil)` 被**恰好调用一次**（验证 §4.2 契约未破）。
- **AutoExpand=true 回归**：`:theme` 仍按原样贴 statusLine 上方展开（SelectPane 旧行为零变化）。
- **AutoExpand=false（FileSelector 落地后）**：picker 精确落在 pane 左上角、尺寸 = pane 派生值，resize 即关。
- **连续快速 resize / resize 瞬间正好在按键** → 不崩溃，状态干净。
- **resize 关闭后 `:theme` / `:file` 能正常再次打开**（无状态泄漏、无残留 OnCancel）。

---

## 附：与 F1 的对应

| 本文章节 | F1 引用处 |
|---------|----------|
| §2.3 / §4（resize + onCancel） | F1 D5（前置依赖）、§3.2 数据流 |
| §2.2 / §3（AutoExpand） | F1 D2（layout，瘦身版）、§7 Layout 计算 |
| §3（FloatOpenSpec） | F1 §6.1（FloatFrame.Open 契约） |
| §2.1（Open 签名） | F1 §6.1 |
