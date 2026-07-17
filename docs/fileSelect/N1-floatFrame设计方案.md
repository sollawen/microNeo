# floatFrame 设计方案

**状态**：设计阶段（N0a 序列）。落到代码前需技术架构阶段细化。

**范围**：FloatFrame 抽象、per-owner 化、与 Widget / Pane 的关系、NotePane 边界。

# 用户的最初想法

这部分是讨论起点，作者在动手前对 floatFrame 的产品定位、FileManager 的限制、以及改造方向的原始想法。原文保留，不动。

---

## 问题

现在的 fileManager 是基于 floatFrame 的。但是 floatFrame 有个最大的问题是：它不支持在一个 float 里面再 open 一个 float。这导致我如果想在 fileManager 里面添加 rename, addnew 的时候，是需要输入和修改文件名的。就没办法实现了。

我有些想法：

## floatFrame 的定位

FloatFrame 当初之所以设计成不允许嵌套 open，主要是想提供一个功能比较简单的带边框的区域，可以在里面实现一些简单的交互，

- 比如从一个列表中选择一项，就是现在的 SelectPane
- 比如需要输入或编辑一个 string
- 比如弹出显示一段信息，版本更新说明之类的

总体想法是 floatFrame.open 后，就是所有用户交互都在这里面的，因为是个简单功能。用户确定或是取消后，这个 float 就关闭掉了。所以就决定是不嵌套的了。

## fileManager

如果 fileManager 只是实现在一个文件列表里面去选择文件就结束了，那么使用 floatFrame 算是合适的。

但是如果我们未来要加上更多的操作，比如 rename, move, copy, delete, mkdir, 那势必要在 fileManager 里面加上更多的复杂的用户交互。我估计用 floatFrame 可能就不合适了。

## 我的想法

floatFrame 继续保持现在的定位：一个很薄的层，

- 专门给那些需要一个方框，在里面做一次性的、特别简单的交互的程序使用
- 这个方框是独占的，open 后所有的交互都在里面，关闭之后控制权回到调用者那里
- 没有状态，没有嵌套，没有同时存在多个方框，打开一个之后，必须关闭之后控制权才回来

FileManager 现在还 OK，但是以后多了交互之后，可能就不能使用 floatFrame 了

- fileManager 在 open 之后，要自己管理自己的界面
- 但是在里面 rename, addNew 之类的需要输入编辑的操作，倒时可以使用 floatFrame
    - 比如我们现在的了一个 selectPane 了
    - 未来还可以搞一个 editPane，专门在一个方框里面输入一个单行的 string
    - 还可以搞一个 msgPane，专门用来显示多行的提示文本。只能看，上下滚动，esc 关闭
    - 包括 fileManager、其它 bufPane 在内的程序在需要的时候都可以使用这个 editPane, selectPane, msgPane

## 我们先把 NotePane 放在一边不谈，先讨论 floatFrame。

我当初在 micro 主流程里面插到 Render() and Despatch()，主要是想接入到 micro 的显示流程和事件处理流程里面去。所以找了一个最 root 的地方切入。

但是我现在想，

- floatFrame 一定是在整个实例树里面弹出来的
    - 要么就是在某个 bufPane 里面，
    - 要么就是在 notePane 里面 (selectReceiver)
    - 要么就是在 InfoPane 里面 (:theme 命令)
- 有没有可能这样处理呢？
    - 在这个需要弹出一个方框的 pane 里面，创建一个 floatFrame 的实例
    - 然后由这个 pane 的 handleEvent() 把 event 转送给 floatFrame
    - 由这个 pane 的 display() 来调用这个 floatFrame.display()
    - 这样就不需要修改 micro 最 root 的主流程了

---

# 新的设计方案

经过讨论，作者最初的几个直觉判断（FloatFrame 是薄层、不能嵌套、per-owner 化）全部成立，但中间冒出了一些新的产品规则（Modal 语义、resize 行为、三层架构、NotePane 边界）。这一节把这些收敛后的规则按设计文档的逻辑整理出来。

## 一、核心结论

三句话总结：

1. **FloatFrame 从全局单例改成 per-owner**——每个 owner 持有一个 `floatFrame *FloatFrame` 字段，不再有 `TheFloatFrame` 全局
2. **Widget / Pane 分层**——简单一次性交互（≤300 行）是 Widget，全屏持有（独立运行）是 Pane；FileManager 是 Pane（对标 yazi），FileSelector 是 FileManager 内部细节
3. **NotePane 跟 FloatFrame 没有关系**——NotePane 是 Pane 层的一员，全局单例、modal-style 覆盖当前 pane，独立演进

## 二、产品规则（已收敛）

下面这些是产品层级的承诺，不依赖具体实现。等技术架构阶段拍板的实现选择见 §五。

### 2.1 FloatFrame 定位

FloatFrame 是一个薄容器，只负责五件事：

1. **布局**：根据 anchor 和 content size 算出方框的 `(fx, fy, outerW, outerH)`
2. **画框**：画边框 + title bar
3. **resize**：直接关（跟 GUI / fzf / tmux 一致）
4. **基础交互**：Esc 关闭（事件路由的最低优先级兜底）
5. **owner-local**：**不在 micro 的 root Render / Dispatch 主流程里**——每个 owner 自己持有一个 FloatFrame 实例，由 owner 自己负责渲染和事件转交

FloatFrame **不知道**里面装的是什么 UI（是列表、输入框还是消息）、**不知道** owner 是谁、**不知道** widget 跟 owner 之间传什么数据。内容渲染和事件处理（除了 Esc / resize）全部委托给 widget 自己。

第五条（owner-local）是最关键的架构原则，跟其他四条（布局 / 画框 / resize / Esc）的现状基本一致。**owner-local 的具体架构对比（vs 当前代码）见 §3.6**。

### 2.2 Modal 语义（A1 + A2）

FloatFrame 打开后接管 owner 的事件。owner 失活时 FloatFrame 关闭。拆成两个子规则：

| 规则 | 作用域 | 行为 |
|---|---|---|
| **A1：owner 失活 = close** | 跨 owner | pane A 有 widget → 切到 pane B → A 的 widget 自动 close（focus 转移 = Esc）|
| **A2：owner 内单 widget** | owner 内 | 同一 owner 里 widget 不能叠加，第二个 open 请求被拒绝 |

两个规则联合保证：**全局任何时刻最多只有一个 widget 可见**。

理由：

- FloatFrame 定位是"一次性小交互"，用户失去 focus 关掉它没什么损失
- 跟 stock IDE 的"Esc / 切窗口 = 取消"肌肉记忆一致

#### 验证场景（`:theme` 例子）

`:theme` 是 `(*BufPane).ThemeCmd`（`command_neo.go:26`），打开一个 SelectPane 让用户选 colorscheme。`A1` 规则：用户在 SelectPane 打开时切到别的 pane → SelectPane 自动关闭、colorscheme 未变（用户没选完）——等价于 Esc。跟 stock IDE 的"菜单取消"肌肉记忆一致。

### 2.3 resize 行为

**resize 直接关**——不需要 tryRefit / reposition 之类的额外机制。

理由：

- **跟 GUI 一致**：右键菜单、context menu、dropdown 之类的浮层在 GUI 里 resize 通常直接消失。用户的肌肉记忆是"浮层被 resize 打断 = Esc"
- **跟现有代码一致**：FloatFrame.HandleEvent（`floatframe.go:220`）已经实现 resize → Close + 触发 OnResize 回调，FileSelector / SelectPane 都走这个路径（F1a、F3 §4.1f 已记录）
- **更省事**：不需要 tryRefit、不需要算新坐标、不需要处理"算完还是放不下"的边缘 case
- **widget 是短命的**：modal 组件本来就一次性，关了用户重开就行；跟 fzf / tmux display-popup 行为一致

实施层：FloatFrame.HandleEvent 收到 resize event → Close() 容器 → 触发 spec.OnResize 回调 → widget 内部 cleanup + 通知 owner（带 cancel 信号）。

### 2.4 单实例保证

**全局任何时刻最多一个 widget 可见**——由 A1 + A2 自然保证，不需要额外机制（registry / 全局锁等都不需要）。

### 2.5 嵌套问题

Widget 之间**不嵌套**。理由：

- **产品定位冲突**：Widget 是"一次性小交互"。在 SelectWidget 操作中突然弹出 InputWidget，用户会困惑——"按 Enter 是选了外层还是内层？"
- **A1 + A2 自然保证**：同一时刻全局只有一个 widget 可见，widget 之上的 widget 根本开不起来

但 **Pane → Widget 是允许的**：FileManager（Pane）上叠加 InputWidget 是正常用法。FileManager rename 场景走"关 A → 开 B"序列调用：关当前 widget → 开 InputWidget → 拿到新文件名 → FileManager 继续。

### 2.6 与现有事实的对照

文档早期版写"`:theme` 在 InfoBar 里"——查代码修正：

- `:theme` 注册为 `(*BufPane).ThemeCmd`（`command_neo.go:26`），是 **BufPane 命令**，不是 InfoBar 命令
- `(*BufPane).ThemeCmd` 调 `NewSelectPane().Open(...)`，锚点是 `(0, -1)`（贴 statusLine）
- `(*BufPane).SetCmd`（`command.go:712`）尝试 `SetGlobalOption` 优先，失败才回退 `h.Buf.SetOption`；全局 option 分支 `h` 没用上，只是 dispatch 跳板

`:theme` 是 `:set colorscheme` 的用户友好版，两者 owner 一致——归当前 active BufPane。不为全局命令单独搞新抽象。

## 三、三层架构

### 3.1 FloatFrame / Widget / Pane

```
┌─────────────────────────────────────────────────────────────┐
│  FloatFrame 层：框架 / 容器                                 │
│  - 算坐标、画框、resize 重画或关闭、Esc 关闭                │
│  - 不知道里面装的是什么；只管"有个浮窗"这件事本身          │
└─────────────────────────────────────────────────────────────┘
                          ▲                  ▲
                          │ 被 widget 用     │（不直接用）
                          │                  │
        ┌─────────────────┴────┐    ┌────────┴───────────────────┐
        │  Widget 层            │    │  Pane 层                   │
        │  - 简单，一次性        │    │  - 全屏持有                 │
        │  - ≤ ~300 行           │    │  - 自己的状态机             │
        │  - 数据交换器          │    │  - 自己管 border / resize  │
        │                       │    │  - 像 yazi 一样独立运行     │
        │  SelectWidget         │    │                            │
        │  MsgWidget (future)   │    │  BufPane (buffer)          │
        │  InputWidget (future) │    │  NotePane (notes)          │
        │  ConfirmWidget (future)│    │  TermPane (terminal)       │
        │                       │    │  FileManager (files) ★     │
        └───────────────────────┘    └────────────────────────────┘
```

### 3.2 核心区分：Widget vs Pane

| 维度 | Widget | Pane |
|---|---|---|
| 屏幕面积 | 小，居中或贴某个锚点 | 全屏（或 split tree 里的一个节点）|
| 生命周期 | 一次性，Open/Close | 长期存活，随 tab 一起创建/销毁 |
| 数据流 | 收数据 → 还数据（function-call）| 持续状态机 |
| 复杂度上限 | ≤ ~300 行 | 无上限 |
| 例子 | SelectWidget、InputWidget | BufPane、TermPane、FileManager |
| 类比 | 表单组件（input / select / confirm）| 完整的子应用（VSCode panel、yazi、vim buffer）|

**FileManager 是 Pane，不是 Widget**。对标 yazi：全屏运行、自己的状态机和键绑定、调用外部 previewer，跟 TermPane 平级。

**FileSelector 是 FileManager 的内部实现细节，不暴露给外部**。外部代码只 import FileManager，不 import FileSelector。FileSelector 在 FileManager 实现时变成不导出类型 `fileSelector`（小写）或直接合并进 FileManager——**当前不动**，等 FileManager 实现时一起处理。

### 3.3 Owner 是正交概念

"Owner" 不是一类实体，是一个**能力标签**：任何能在自己 UI 里调用 widget 的实体都是 owner。

- 所有 Pane 都是 Owner：BufPane（`:theme` 弹 SelectWidget）、NotePane（未来加交互）、TermPane（未来加交互）、FileManager（rename 用 InputWidget）
- 非 Pane 也可能是 Owner：边界情况，目前没显式需求

Owner 调用 widget 的方式（function-call 语义）：

```text
widget.Open(
    data: T1,                    // 初始数据（items / prompt / etc.）
    onClose: func(result: T2) {  // 关闭时的回调
        // owner 处理 result
    },
)
```

Widget 是**一次性**的：Open 之后 widget 自己跑，Close 之后 owner 从回调拿结果，中间不持有跨调用状态。

### 3.4 命名约定

| 后缀 | 含义 | 约束 | 例子 |
|---|---|---|---|
| `XxxWidget` | 简单 UI 组件（form） | ≤ ~300 行；一次性；数据交换器 | `SelectWidget`, `MsgWidget`, `InputWidget`, `ConfirmWidget` |
| `XxxPane` | 全屏 Pane（split tree 节点或独立 Pane）| 自己的状态机；全屏 / 占 split 节点 | `BufPane`, `InfoPane`, `NotePane`, `RawPane`, `TermPane`, `FileManager` |
| `FloatFrame` | 容器框架 | — | `FloatFrame` |

FileManager 走 `XxxManager` 后缀没问题——它的角色是 Pane（独立运行的子应用）。FileSelector 是 FileManager 内部细节，不属于这三类之一。

#### Rename 计划

- `SelectPane` → `SelectWidget`：✅ 执行（misnomer 修复；满足 widget 复杂度标准）
- `FileSelector` → `fileSelector`（不导出）或合并进 FileManager：🚧 等 FileManager 实现时一起动
- `FileManager`：保持现有命名（未来实现时遵循 Pane 范式）

### 3.5 Cancel 语义

Widget 关闭至少分三种情况，callback 应该能区分。参考 FileSelector 已经实现的 4 类 CloseReason（`fileselector.go:133-141`）：

| 关闭原因 | 触发条件 | callback 信号 |
|---|---|---|
| **Picked** | 用户 Enter 选中（或其他确认手势）| `result.value` 是选中的值 |
| **ReasonEsc** | 用户按 Esc | `result.cancelled=true, reason=Esc` |
| **ReasonFocus** | focus 转移到别的 owner（A1 规则触发）| `result.cancelled=true, reason=Focus` |
| **ReasonResize** | 窗口 resize（FloatFrame.HandleEvent 主动 Close）| `result.cancelled=true, reason=Resize` |

补充：FileSelector 还定义了 `ReasonQuit`（Ctrl-q）和 `ReasonSize`（打开时窗口过小）——这都是 widget 特有的原因，不属于通用 FloatFrame 的 close 路径。widget 可以根据需要自由扩展 Reason 枚举。

#### 为什么需要区分？

不同取消原因对 owner 来说意味着不同的处理：

| 场景 | Esc | Resize | Focus 转移 |
|---|---|---|---|
| `:theme` 选 colorscheme | 不变 | 不变 | 不变 |
| FileSelector pick file | 不打开 | 不打开 | 不打开 |
| 未来 FileManager rename | 放弃输入 | 保留输入？重开？ | 不可能（Pane 不切换）|
| 未来 confirm dialog | 取消 | 默认值？ | 默认值？ |

**现状**：很多简单场景 owner 不在乎细分原因，nil = 取消够用（SelectPane 就是这样，`selectpane.go:26`）。但接口**应该**支持细分，让有需要的 owner / widget 能用上。

#### 实施层

**当前状态**：

| Widget | 回调签名 | 细分原因 |
|---|---|---|
| SelectPane | `func(*string)` | ❌ nil = 取消（不区分）|
| FileSelector | `func(SelectResult)` | ✅ 4 个 Reason + Picked |

**演进**：

- FloatFrame.Close(cause CloseReason) —— 接收原因参数（待技术架构阶段定义具体接口）
- widget cleanup 时根据 cause 决定行为（释放资源 / 保存状态 / 重新开等）
- SelectPane 升不升级看需求（如果以后 SelectPane 也想区分原因，统一改）
- FileSelector 已经做好（不需动）

接口两种风格：
- 简版：`func(*string)` —— nil = 取消（不在乎原因）
- 细分版：`func(T, CloseReason)` —— 跟 FileSelector 对齐（推荐）

### 3.6 FloatFrame 在 micro 主流程中的位置（owner-local）

§2.1 第 5 条提到 FloatFrame 不在 root 主流程里。这条是改造的**根本架构原则**。具体看下当前代码 vs 目标架构的差别：

| root 阶段 | 当前代码 | 目标架构 |
|---|---|---|
| Render（`micro.go:536-538`）| `if TheFloatFrame.IsOpen() { TheFloatFrame.Display() }` | ❌ 删除 |
| Dispatch（`micro.go:584-588`）| `if TheFloatFrame.IsOpen() { TheFloatFrame.HandleEvent(event) }` | ❌ 删除 |
| 所有者 | `var TheFloatFrame` 全局单例（`globals.go:17`）| ❌ 删除 |
| Render 替代 | — | BufPane.Display() 末尾：`if b.floatFrame.IsOpen() { b.floatFrame.Display() }` |
| Dispatch 替代 | — | BufPane.HandleEvent() 开头：`if b.floatFrame.IsOpen() { b.floatFrame.HandleEvent(event); return }` |

owner-local 的三个具体后果：

1. **Focus 转移语义自然成立**（A1）——切换 owner A → B，A 的 floatFrame 跟着 A 一起失活，不需要在 root 层面做"全局 widget 切换"逻辑
2. **多 pane 场景正确**——pane A 的 widget 不会"穿透"到 pane B 的区域
3. **root main loop 极简**——只有 NotePane（全局 session）+ InfoBar（命令）+ Tabs（默认），没有"widget 全局层"这种抽象

完整改造路径见 §七。

## 四、NotePane 的独立定位

NotePane 跟 FloatFrame **没有关系**，是两个独立演进的实体。

### NotePane 的产品定位

| 维度 | 描述 |
|---|---|
| 架构归属 | Pane 层（和 BufPane、RawPane、TermPane 平级）|
| 出现位置 | 不在 split tree 里 |
| 出现时机 | Alt-Enter 触发，Esc 关掉（modal-style）|
| 视觉位置 | 覆盖当前 pane 的矩形区域，不是覆盖整个 UI |
| 状态归属 | 全局单例（一次只有一个 notes 草稿）|
| 实现 | `TheNotePane *NotePane` 全局变量（`globals.go:16`），`NotePane` 嵌入 `*BufPane`（`notepane.go:22`）|

### 为什么 NotePane 不是 Widget

Widget 定义是"一次性简单数据交换器（≤300 行）"。NotePane 不是——它有持续状态（receiver 选择、mouse 处理、自绘 border），有自己的状态机。是 Pane。

### 为什么 NotePane 是全局单例而不是 per-owner

- 用户心智模型是"一个 notes 草稿区"，全局共享一个会话是合理的
- per-owner 不会增加产品价值（"pane A 的 notes 跟 pane B 不同"不是常见需求）
- 全局单例代码更简单，root dispatch 路径清晰

### NotePane 的 dispatch 优先级

只看 NotePane 这一路（高 → 低）：

1. NotePane（如果开着）
2. InfoBar.HasPrompt（如果有 prompt）
3. Tabs（正常内容）

FloatFrame（widget 弹窗）走另一条路，在 root dispatch 的最高优先级（`micro.go:587`），不在 NotePane 的链路里。两条路不互不干扰。

完整四层 dispatch 优先级（高 → 低，参见 `micro总流程调研.md` §六）：

1. `TheFloatFrame` — 最高（widget 弹窗）
2. `TheNotePane` — 次浮（notes pane-level modal）
3. `InfoBar.HasPrompt` — 底部 prompt（命令行 / 搜索 / y/n 确认 / 保存文件名）
4. `Tabs` — 默认

### 结论

NotePane 现状（全局单例 + root dispatch + 自管 border + 嵌入 BufPane）已经是正确的产品决策。本 FloatFrame per-owner 改造不动 NotePane。两个东西独立演进。

## 五、产品规则 vs 架构实现（划界）

本文档承诺的**全部产品行为**如下：

| # | 产品规则 | 状态 |
|---|---|---|
| 1 | FloatFrame 是薄容器（布局 + 边框 + resize + Esc）| ✅ |
| 2 | Widget 是数据交换器（一次性，≤300 行）| ✅ |
| 3 | Pane 是全屏持有（独立运行，状态机）| ✅ |
| 4 | FileManager = Pane（对标 yazi）| ✅ |
| 5 | FileSelector 是 FileManager 内部细节 | ✅ |
| 6 | Modal A1：跨 owner focus 转移 = close | ✅ |
| 7 | Modal A2：owner 内单 widget | ✅ |
| 8 | Widget 不嵌套（Pane → Widget 可以）| ✅ |
| 9 | resize 直接关（跟 GUI / fzf / tmux 一致）| ✅ |
| 10 | Owner 是正交概念（任何能用 widget 的实体）| ✅ |
| 11 | SelectPane → SelectWidget rename | 🚧 待执行 |
| 12 | FileSelector 改不导出或合并 FileManager | 🚧 等 FileManager 实现 |

**架构层 deferred 到技术架构阶段讨论**：

| # | 架构选择 | 状态 |
|---|---|---|
| A | 数据结构：per-owner 字段？全局 var 加路由表？modal stack？ | ⏳ |
| B | 接口设计：Close 接口怎么传 cancel 信号 | ⏳ |
| C | 字段命名、错误处理、Close 接口参数 | ⏳ |

本节所有 `floatFrame *FloatFrame` 字段、Close 接口升级、`Tab.SetActive` 记录 oldpane 等写法都是**示意图，不是定论**。技术架构阶段可能给出完全不同的实现路径。

## 六、迁移成本估算

```
$ grep -rn "TheFloatFrame" --include="*.go"
cmd/micro/micro.go:536,537,584,585,587,588  ← 6 处，随 root 改动消失
internal/action/floatframe.go:68,70        ← 全局 var + 注释，消失
internal/action/globals.go:17              ← 初始化，消失
internal/action/fileselector.go:27,28,197,838  ← widget 调用，改造
internal/action/selectpane.go:12,16,18,45,64,88,99,181,188  ← widget 调用，改造
```

总计 **~20 处**。`micro.go` 的 6 处 + `globals.go` 的 1 处 + `floatframe.go` 的 2 处 = **~9 处随方案自然消失**，剩下 **~11 处** 是 widget 从 `TheFloatFrame.Open(spec)` 改成 `owner.openFloatFrame(spec)` 的小改造。

**SelectPane 调用点**：3 处

- `command_neo.go:70`（`:theme`）
- `notepane.go:167`（receivers 列表，主路径）
- `notepane.go:247`（receivers 列表，缓存未命中）

## 七、演进路径（提案）

**【状态】这是技术架构阶段的提案，不是定论**。架构层决策（per-owner 字段还是其他方案）要等进技术架构阶段才能拍。

### 第一步：抽象 owner 接口，让 FloatFrame 接受 owner 注入（不改 root）

```text
type FloatFrame struct {
    spec Spec
    // ... 现有布局 / 边框 / 关闭逻辑不变
}

// 新增：owner 通过 OpenSpec 调用
func (f *FloatFrame) OpenSpec(spec Spec) bool { ... }  // 改名，原 Open 等价
```

把 widget 调用从 `TheFloatFrame.Open(spec)` 改为接受一个 `*FloatFrame` 参数（注入式），但 `TheFloatFrame` 这个全局还在，root 暂时不改。

收益：验证 widget 改造工作量、不动 root 风险。

### 第二步：把 owner 引入 FloatFrame 生命周期

- `BufPane` 加 `floatFrame *FloatFrame` 字段（**NotePane 不加**——它是 Pane，跟 FloatFrame 没关系；InfoBar 独立 prompt 体系，也不加）
- 每个 owner 的 HandleEvent 头部加 `if owner.floatFrame.IsOpen() { ... return }`
- 每个 owner 的 Display 末尾加 `if owner.floatFrame.IsOpen() { owner.floatFrame.Display() }`（在画完 owner 内容之后，作为 overlay）
- widget 调用 `owner.floatFrame.OpenSpec(spec)`（不再是全局）
- **focus 转移 = close 实现**：`Tab.SetActive(i)` 切换 pane 时记录 oldpane；如果 oldpane.floatFrame open，调它的 `Close()`（带 cancel 信号，告知 widget 被取消）——目前 `tab.go:341` 的 `Tab.SetActive` 只设 active 不记录 oldpane，需要扩展
- **resize = close 实现**：FloatFrame.HandleEvent 收到 resize event → `Close()` + 触发 `onResize` 回调（已有代码，`floatframe.go:220`，不动）
- **close 接口升级**：FloatFrame.Close(cause CloseReason) 接收原因参数，让 widget 区分 Esc / Focus / Resize 等场景（具体接口设计见 §3.5）

### 第三步：清理 root + 全局

- 删除 root Dispatch 里的 FloatFrame 分支（owner 自己在 HandleEvent 里处理，`micro.go:584-588`）
- 删除 root Render 里的 FloatFrame 分支（owner 自己在 Display 末尾调用，`micro.go:536-538`）
- 删除 `globals.go` 里的 `TheFloatFrame` 初始化（`globals.go:17`）

第三步做完才算完成"root 不需要知道 FloatFrame"这个目标。前两步可以分别提 PR 独立 review。

## 八、当前开放问题

| # | 问题 | 状态 |
|---|---|---|
| 1 | Render 顺序 | ✅ modal 语义保证不需要 root overlay 阶段 |
| 2 | Modal 语义（A1 + A2）| ✅ |
| 3 | 单实例限制 | ✅ 产品层 / ⏳ 架构层 deferred |
| 4 | 嵌套 open | ✅ Widget 不嵌套，A1 + A2 自然保证 |
| 5 | NotePane 一起改 | ✅ 不动，独立演进 |
| 6 | 全局命令的 FloatFrame owner | ✅ 沿用 dispatch 路径（`:theme` 归当前 active BufPane）|
| 7 | EditPane / MsgPane | 🔜 未来再做 |

### Q7：EditPane / MsgPane 何时做？【未来再做】

同意。**等 FloatFrame 接口稳定后做**，复用 owner 模型。当前触发条件：

- FileManager 加了第三种 input 模式（除了 rename 和 new file 之外，比如 mkdir 或 touch）
- FileManager 加了 undo / clipboard 中任一
- 出现"owner 内同时需要两个 inner widget"的真实场景

任一命中，widget 抽象的回报就从"理论值"变成"实际值"。在那之前：

- FileManager 未来实现时**走 Pane 范式**（自管 modal、自己画 border、不依赖 FloatFrame）——参照 NotePane 的做法
- FileSelector 现状**不动**（是 FileManager 的内部实现；FileManager 实现时一起改）
- FloatFrame 本身**不动**，保持薄
- **唯一应该立刻执行的改动**：`SelectPane` → `SelectWidget` 的 rename（misnomer 修复，不涉及架构改动）

## 九、关联文档

- [`fileManager的产品定位.md`](./fileManager的产品定位.md) — FileManager 的产品形态（all-in-one、Finder 镜像等）
- [`micro总流程调研.md`](./micro总流程调研.md) — micro 主流程调研，dispatch / render 顺序
- [`docs/弹窗机制/研究-microPane机制.md`](../弹窗机制/研究-microPane机制.md) — Pane 机制参考
- [`docs/AIBP/说明-notepane.md`](../AIBP/说明-notepane.md) — NotePane 的产品说明

本文档是**架构层**，产品定位文档是**产品层**，各管一段。两者不冲突——用户在 FileManager 里按 F2 看到一个小框输入名字，不管那个小框叫 FloatFrame、editPane、还是 widget，对用户没差。
