# FloatFrame 抽取成包 —— 思路文档

**状态**：思路稿，供决策前认真权衡。本文聚焦「把 FloatFrame 从 `internal/action/` 抽到独立包」这一件事，不涉及 file 子系统（那是 N0 的范围）。但本文是 N0 路 B（先抽 float、再抽 file）的前置。
**动机**：FloatFrame 已被 4 个具体浮窗共用（FileSelector / SelectPane / NotePane / InfoPane），事实上是一层共享基础设施，只是恰好寄居在 action 包里。把它独立成包 = 承认它既有的层级身份，并让后续的 file 抽取无需再写 Host adapter。

---

## 1. 为什么现在就值得抽

- **它已经是共享底座**：4 个消费者都直接 `TheFloatFrame.Open/Close`，彼此平级、互不依赖（FileSelector 不调 SelectPane，见 N0 调研）。一个被多处共用的机制，独立成包是顺理成章的，不是「增实体」，是「正名」。
- **抽取本身极廉价**（见 §3）：floatframe.go 已自洽，不需要先解任何耦合。
- **它是 file 抽取的干净前置**：floatframe 独立后，file 包直接 `import floatframe` 用现成的 `FloatOpenSpec`，省掉路 A 的 Host adapter（N0 §5.4）。

---

## 2. 事实依据（已验证）

### 2.1 floatframe.go 已自洽

实测它的依赖：

| 依赖 | 性质 | 抽取后 |
|---|---|---|
| `config` / `screen` / `tcell` | 全外部包 | 照常 import，无环 |
| `SelectPane`（4 处）| **全是注释**，零代码引用 | 抽取时顺手泛化措辞 |

**floatframe.go 对 action 内部符号零代码依赖**。这是最关键的事实——抽取不需要先拆任何死结，是纯机械搬运。

### 2.2 API 契约（导出面 · 入参 / 出参）

抽取后 `floatframe` 包的全部公开 API（调用者标准化使用的依据）。分两层理解：**主循环 / 持有方（action） → 容器**，和 **容器 → 具体浮窗（消费者的回调）**。

**位置与尺寸类型**（都是简单的屏坐标数据）
- `Rect{X, Y, W, H int}` 屏坐标矩形
- `Pos{X, Y int}` 屏坐标点（锚点用）
- `Size{W, H int}` 宽高

**构造**
- `NewFloatFrame() *FloatFrame` —— 返回空壳容器（无入参）

**容器方法（第一层：主循环 / 持有方主动驱动）**

| 方法 | 入参 | 出参 | 语义 |
|---|---|---|---|
| `Open(spec)` | `FloatOpenSpec` | `bool` | 打开浮窗；true=成功，false=没开成（已有浮窗在开 / 屏幕放不下），调用方应透明返回 |
| `Close()` | 无 | 无 | 关闭、清空状态、回空壳 |
| `IsOpen()` | 无 | `bool` | 状态查询 |
| `Display()` | 无 | 无（副作用画屏） | 主渲染循环调用；画框 + title + 委托内容 |
| `HandleEvent(ev)` | `tcell.Event` | 无 | 主事件循环调用；resize 被拦截不外传（见下） |

**`FloatOpenSpec` —— 打开的全部入参（options 模式，`Open` 的唯一入参）**

几何 / 外观（调用方给值）：
- `Anchor Pos` —— AutoExpand=true：展开中心点；false：外矩形左上角（含边框）
- `ContentSize Size` —— 纯内容尺寸（不含边框）；容器内部派生含边框 outerW/outerH
- `Title string` —— 嵌入上边框；空串=纯横线
- `FrameColor tcell.Style` —— 边框色；零值=`config.DefStyle`
- `AutoExpand bool` —— true：锚点自适应展开（贴光标）；false：钉死 Anchor

回调（调用方给函数值，容器在适当时机回调之 —— 第二层：容器 → 具体浮窗）：

| 回调 | 何时被容器调用 | 容器传入 | 具体浮窗应做 |
|---|---|---|---|
| `Display(contentArea Rect)` | 每次渲染（主循环调容器 `Display` 时） | 已扣边框的内容区（左上=fx+1,fy+1） | 在该 Rect 内画自己的内容 |
| `HandleEvent(event)` | 每个**非 resize** 事件 | 原始 `tcell.Event` | 处理键位 |
| `OnCancel()` | resize 触发容器自关时 | 无 | 清理业务回调（如 onSelect(nil)） |

**关键约定（标准化使用必读）**
- 边框对调用者透明：`Display` 回调收到的 Rect 已扣边框，调用者完全无需感知边框存在。
- resize 不外传：resize 永远不会到达调用者的 `HandleEvent`——容器统一拦截 → `Close` + `OnCancel`。
- 关闭顺序：调用方先 `Close()` 再触发业务回调（如 `onSelect`），保证回调读到的是关闭后状态。
- Open 幂等：同一容器已开时再 `Open` 直接返回 false（C1 单浮窗）。

注：`TheFloatFrame` 单例**不在本包**（按 §5.2，实例归 action 持有）；本包只导出类型 + 构造。另：无配套测试文件（`floatframe*_test.go` 不存在）。

### 2.3 消费者改动面（实测引用计数）

| 文件 | 引用处数 | 抽取后处理 |
|---|---|---|
| `globals.go` | 1（InitGlobals 里初始化）| 见 §5.2 |
| `command_neo.go` | 2 | 加 `floatframe.` 前缀 |
| `selectpane.go` | 9 | 加 `floatframe.` 前缀 |
| `notepane.go` | 2 | 加 `floatframe.` 前缀 |
| `fileselector.go` | 23 | 随 file 走，自然 resolve |

留在 action 的消费者合计 **~14 处**，几乎全是类型前缀化（`Pos` → `floatframe.Pos`、`FloatOpenSpec` → `floatframe.FloatOpenSpec`）。`TheFloatFrame.Open(...)` 这类**方法调用是否要改前缀，取决于 §5.2 的单例归属决策**。这些符号没有被 action 以外的包引用，影响完全局部。

---

## 3. 边界：float 包放什么、不放什么

**放（基础设施层）**：

- 容器本体 `FloatFrame`（画框 / 清屏 / 锚点展开 / 事件路由 / resize 拦截 / 生命周期）
- 共享几何与契约类型 `Rect` / `Pos` / `Size` / `FloatOpenSpec`
- 构造 `NewFloatFrame`

**不放（消费者层，各自归域）**：

| 消费者 | 归属 | 理由 |
|---|---|---|
| `SelectPane` | 留 action | 通用列表组件，域中立，被 Themes / NotePane 复用 |
| `NotePane` / `InfoPane` | 留 action | 各自业务 |
| `FileSelector` | → `internal/file/`（N0 范围）| 文件域 |

**规则一句话**：float 包只放「托管任意浮窗都需要的通用机制」；具体弹窗是它的客户，跟着自己的业务域走。把 SelectPane 塞进 float 会开启一个坏先例——同理 FileSelector / NotePane 也该进，于是 float 沦为「所有浮窗杂物间」，lean 基础设施包的意义就没了。（SelectPane 与 FloatFrame 看着亲近，只因它是首个消费者、唯一用 `AutoExpand=true` 的那个；但 `AutoExpand` 是容器的通用能力，SelectPane 只是它的一个客户端，不是共生。）

---

## 4. 改动步骤（机械、可编译引导）

1. 新建 `internal/floatframe/`，把 `floatframe.go` 移过去，`package action` → `package floatframe`。
2. 按 §5.2 决策处理 `TheFloatFrame` 单例声明与初始化。
3. 在 4 个消费者（`globals` / `command_neo` / `selectpane` / `notepane`）里给 `Rect`/`Pos`/`Size`/`FloatOpenSpec` 加 `floatframe.` 前缀；按 §5.2 决定 `TheFloatFrame` 调用是否加前缀。
4. 顺手把 floatframe.go 注释里的「(SelectPane)」「(FileSelector)」泛化成「贴光标弹窗」「固定锚点弹窗」（§5.4）——基础设施不该在注释里点名具体客户。
5. `make build`，编译器逐个报错引导改完。
6. 验证（§7）。

全程**零行为变化**，纯搬运 + 前缀。

---

## 5. 设计决策点（请权衡）

### 5.1 包名

**已定 `internal/floatframe/`**。下文所有类型前缀以 `floatframe.` 为准。

### 5.2 TheFloatFrame 单例归谁

**已定：action 拥有单例。** 原则——`floatframe` 包只放「定义」：类型 + 构造 `NewFloatFrame`（定义「FloatFrame 是什么、怎么造一个」）。而「什么时候造、造几个、实例放哪」是程序业务流程的事，与 floatframe 包无关——这部分归 action。

落地：
- `floatframe` 导出 `type FloatFrame` + `func NewFloatFrame()`；**不含**任何包级实例变量。
- action/globals.go 持有实例：`var TheFloatFrame *floatframe.FloatFrame`，在 `InitGlobals` 里 `TheFloatFrame = floatframe.NewFloatFrame()`（位置与现状一致）。
- 消费者继续写 `TheFloatFrame.Open(...)`——**方法调用零改名**；只有构造 `FloatOpenSpec`/`Pos`/`Size` 处加 `floatframe.` 前缀。

与 N0 给 file 定的原则同构：库零包级状态，实例归 app。

### 5.3 类型随迁

`Rect` / `Pos` / `Size` / `FloatOpenSpec` 整体迁入 floatframe，**单份、无重复**。消费者构造处与 Display 回调签名（`func(area floatframe.Rect)`）加 `floatframe.` 前缀。这正是抽取 floatframe 相对路 A（Host adapter）的核心收益——契约类型不再重复定义。

### 5.4 注释去消费者化

floatframe.go 现有注释把通用特性绑定到具体客户（「AutoExpand=true (SelectPane)」「false (FileSelector)」）。抽取后基础设施不应在注释里「认识」客户，改成行为描述（「贴光标弹窗路径」「固定锚点弹窗」）。纯文档清理，零代码影响。

---

## 6. 与 file 抽取（N0）的关系

- **先抽 floatframe（本文）→ 再抽 file（N0b）**：file 直接 `import floatframe`，用 `floatframe.FloatOpenSpec` / `floatframe.Pos`，**无需 Host adapter**。两步分离、各自独立验证。
- **若不抽 float 直接抽 file**：file 必须自建 Host 接口 + 让 action 做 adapter（N0 路 A），多约 20 行胶水 + 一份重复的 Spec 形状。

即：本文是 N0 路 B 的前置；做不做本文，决定 file 那步走 A 还是 B。但本文**自身也有独立价值**（正名共享底座），不依附于 file 抽取。

---

## 7. 风险与验证

- **导入环**：float → config/screen/tcell，三者均不 import action（已验证）。无环。
- **初始化顺序**：`TheFloatFrame` 在 `InitGlobals` 才赋值，此前为 nil——与现状一致，不引入新风险；赋值语句位置不变。
- **行为**：纯搬运，零逻辑改动。风险近零；编译错误引导改完即可。
- **验证清单**：`make build` 通过；运行 microNeo，开 Ctrl-o 文件选择器、`:theme` 列表、note 的 receiver 选择，确认渲染 / 键位 / resize 即关 全部与改前一致。

---

## 8. 待你决策

| 项 | 选项 | 倾向 |
|---|---|---|
| 抽不抽 | 抽 / 不抽（file 那步改走路 A）| 抽 |
| 时机 | 先于 file 抽取 / 与 file 一起 / 暂不 | 先于 file（独立成一次改动）|

定稿后本文转成可执行的迁移计划（文件移动清单 + 逐文件 diff 要点）。
