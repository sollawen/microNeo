# N1a — Dialog 包迁移计划

把 `SelectDialog`（连带其容器 `FloatFrame`）从 `internal/action/` 迁出，落到新建的 `internal/dialog/` 包，为 N1（InputDialog）以及后续 ConfirmDialog 等同族组件铺路。

本计划只做「搬家 + 接线」，不改任何运行时行为，不实现新功能。

---

## 1. 目标与动机

**为什么要迁**

- `SelectDialog` 是一个 modal 组件，和它赖以运行的容器 `FloatFrame`、以及两者共享的 `Rect / Pos / Size / FloatOpenSpec` 契约，在概念上是一族东西（容器 + 具体浮窗）。目前它们散落在 `action` 包里，与 `action` 的本职（BufPane / 命令分发 / 键位树）混在一起。
- N0 已规划同族扩充：`InputDialog`、`ConfirmDialog`。N1 的 InputDialog 设计当前假设「InputDialog 写在 package action」（N1 §6.2、§10）。如果照此落地，`action` 包会再累积一个与主控无关的 modal 组件，愈发臃肿。
- 趁 InputDialog 还没动工，先把 dialog 这条线独立成包，让后续同族组件有统一的归属，也避免「先写进 action 再搬家」的二次成本。

**本计划的边界**

- 只迁 `SelectDialog` + `FloatFrame`，不改它们的逻辑。
- 不实现 InputDialog / ConfirmDialog（那是 N1 及以后的事）。
- 不动 finder（finder 有自己的 `finder.Rect`，与本次无关）。

**成功标准**

- `make build` 通过，行为与迁移前完全一致。
- `:theme` 选择器、NotePane 的 Receiver 选择器两条路径手测无回归。
- `internal/dialog/` 成为不引用 `action` 的叶子包。

---

## 2. 现状盘点（依赖图）

### 2.1 被迁代码

| 源文件 | 行数 | 内容 | action 包内依赖 |
|---|---|---|---|
| `internal/action/selectdialog.go` | ~235 | `SelectDialog` + `NewSelectDialog` + `Open/display/handleEvent/ensureVisible/effMaxVisible` + 包级 `maxItemLen`、`min` | `TheFloatFrame`、`Rect/Pos/Size/FloatOpenSpec`（均来自 floatframe.go） |
| `internal/action/floatframe.go` | ~290 | `FloatFrame` + `TheFloatFrame` + `NewFloatFrame` + `Rect/Pos/Size/FloatOpenSpec` + `expandAnchor` | 仅 `config`、`screen`、`tcell`（外部包） |

注：`Rect/Pos/Size/FloatOpenSpec` 迁入 dialog 后，SelectDialog 在 dialog 包内直接引用，无跨包依赖。

关键观察（已用 rg 核实）：

- **`SelectDialog` 不调用 `keyEvent`**。它的 `handleEvent` 直接 `switch ev.Key()` 走 tcell 键码，不经 action 的键名解析。这是本次迁移能干净完成的前提——SelectDialog 不依赖任何 action 包内私有函数。
- **`min` 函数仅 `selectdialog.go` 内使用**（Go 1.19 无 builtin min/max，`go.mod` 确认）。可整块随 `selectdialog.go` 搬走，action 包不会因此缺符号。
- `FloatFrame` 本身只依赖 `config` / `screen` / `tcell`，不碰 action 任何包内符号。

### 2.2 调用方（迁移后需要改引用的地方）

| 位置 | 现状 | 迁移后 |
|---|---|---|
| `internal/action/notepane.go:167` | `NewSelectDialog().Open(... Pos{...} ...)` | `dialog.NewSelectDialog().Open(... dialog.Pos{...} ...)` |
| `internal/action/notepane.go:247` | 同上 | 同上 |
| `internal/action/command_neo.go:57` | `Pos{X:0, Y:-1}` + `NewSelectDialog().Open(...)` | `dialog.Pos{...}` + `dialog.NewSelectDialog().Open(...)` |
| `internal/action/globals.go:17` | `TheFloatFrame = NewFloatFrame()` | 删除（改由 dialog 包自初始化，见 §4.4） |
| `cmd/micro/micro.go:535-536, 583-587` | `action.TheFloatFrame.IsOpen/Display/HandleEvent` | `dialog.TheFloatFrame.*`（6 处） |

### 2.3 不受影响

- `internal/finder/session.go` 有自己的 `finder.Rect`，与 action 的 `Rect` 是两套类型。`fileops.go:30` 用的是 `finder.Rect`，本次不涉及。
- `events.go` 的 `keyEvent` / `KeyEvent` 不被 SelectDialog 使用，本次不迁、不改可见性（见 §4.3 / §7）。

---

## 3. 结构设计：新包文件结构

```
internal/dialog/
├── doc.go          # 包注释（一句话说明 dialog 是 modal 浮窗族：容器 + 具体浮窗）
├── frame.go        # 由 action/floatframe.go 原样迁入
└── select.go       # 由 action/selectdialog.go 原样迁入
```

约定：

- 文件改名但不改内容逻辑。`floatframe.go → frame.go`、`selectdialog.go → select.go`（包内文件名更短，且与未来 `input.go`、`confirm.go` 对齐）。
- 包名 `dialog`。所有导出符号名保持不变（`FloatFrame`、`TheFloatFrame`、`NewFloatFrame`、`SelectDialog`、`NewSelectDialog`、`Rect`、`Pos`、`Size`、`FloatOpenSpec`），仅 `package action → package dialog`。
- `doc.go` 只放包文档注释，不放代码，给后续同族组件留索引。

未来（不在本计划内）追加：

```
internal/dialog/input.go      # InputDialog（N1）
internal/dialog/confirm.go    # ConfirmDialog（N0 规划）
```

---

## 4. 依赖处理决策

### 4.1 【核心决策】floatFrame 迁到 dialog，不留在 action

这是本次唯一有设计含量的决策。结论：**floatFrame 连同 SelectDialog 一起迁到 dialog**。

**理由（循环依赖是硬约束）**

Go 不允许循环 import。把两条边摆出来看：

- 边 A：`action → dialog`。SelectDialog 的调用方在 action（notepane.go、command_neo.go），迁移后它们必须写 `dialog.NewSelectDialog()`，所以 action 必然引用 dialog。
- 边 B：`dialog → action`。SelectDialog 运行时要调 `TheFloatFrame.Open / Close`、要用 `Rect/Pos/Size/FloatOpenSpec`。如果这些东西留在 action，dialog 就必须引用 action。

两条边同时存在 = 循环 import，编译直接失败。**只要 SelectDialog 的调用方还在 action，floatFrame 就不能留在 action**——这是结构性的，不是风格选择。

把 floatFrame 一起迁走后，边 B 消失：dialog 只引用 `config / screen / tcell`，成为叶子包；只剩边 A（action → dialog），DAG 成立。

**已评估并否决的备选**

- 「floatFrame 留 action + SelectDialog 迁 dialog + 用 registry/interface 注入打破环」：即 action 定义 `var newSelectDialogFactory func(...)`，dialog 在 `init()` 里注册实现。能编译通过，但为一个全局单例引入注册表是过度设计，违背「如非必要勿增实体」。而且 TheFloatFrame 仍得留在 action（否则 dialog 连容器都够不着），等于「容器留 action、组件去 dialog」，一族东西被劈成两半，归属更乱。否决。
- 「只迁 SelectDialog、不迁 floatFrame，把 SelectDialog 的调用方也搬出 action」：调用方（notePaneOpen、ThemeCmd）深度依赖 action 内部（MainTab、InfoBar、n.selectedReceiver、SetGlobalOption……），搬出代价远大于收益。否决。

### 4.2 Rect / Pos / Size / FloatOpenSpec 随 floatFrame 走

这四个类型是 `FloatFrame` 与具体浮窗之间的契约，定义在 `action/floatframe.go` 里。它们和 floatFrame 是一体的，随 §4.1 一起迁入 `dialog/frame.go`。

迁移后引用方式：

- dialog 包内（frame.go / select.go）：裸用 `Rect`、`Pos`、`Size`、`FloatOpenSpec`，无前缀。
- action 包内调用方（notepane.go、command_neo.go）：写 `dialog.Pos{...}`。
- 不存在与 `finder.Rect` 的冲突——两套类型分属不同包，引用时都带包前缀，不会撞名。

### 4.3 keyEvent / infodefaults 可见性：本计划不动

需要明确区分两件事：

- **SelectDialog 不依赖 `keyEvent`**（已核实，§2.1）。所以 SelectDialog 迁移到 dialog 后，dialog 包当前**完全不引用 action**，keyEvent 的可见性对本次迁移没有任何影响。
- **InputDialog（N1）会依赖 `keyEvent` 和 `infodefaults`**——N1 §6.2 的 `lookupAction` 需要调 `keyEvent(event).Name()` 再查 `infodefaults`。这俩目前都是 action 包内私有符号。

因此 keyEvent 的可见性问题**属于 N1 的实现范畴，不在本计划内解决**。但因为它会反过来影响 dialog 包的纯净度，本计划在 §7 给出明确建议方向，供 N1 实施时直接采用，避免届时再返工。

### 4.4 action 如何引用 dialog

**全局单例的初始化**

现状：`action/globals.go` 的 `InitGlobals()` 里有一行 `TheFloatFrame = NewFloatFrame()`。`NewFloatFrame()` 只做 `&FloatFrame{}`，不碰任何外部状态（screen 在 Display/Open 时才用），因此可以改成包级变量直接初始化。

迁移后（推荐）：

```go
// internal/dialog/frame.go
var TheFloatFrame = NewFloatFrame()
```

`action/globals.go` 删掉 `TheFloatFrame = NewFloatFrame()` 这一行。`NewFloatFrame()` 保留导出（测试或未来重置可能用）。

这样 dialog 包 import 即就绪，无需 action 显式 Init，也无需改 micro.go 的启动顺序。

**cmd/micro 的引用**

`cmd/micro/micro.go` 有 4 处 `action.TheFloatFrame`（535、536、583、584、586、587 行——共 6 个引用点，分布在 4 个语句里）。全部改成 `dialog.TheFloatFrame`，并在 micro.go 的 import 块加 `"github.com/micro-editor/micro/v2/internal/dialog"`。

不采用「action 保留一个 `TheFloatFrame` 转发变量」的写法——转发变量在重新赋值时有指针陈旧风险，且让全局单例出现两个入口，更难追踪。直接改引用点更干净，总共就 4 个语句。

---

## 5. 迁移步骤（按顺序执行）

前置：确认工作区干净（`git status`），便于事后 diff 核对。

1. **新建 dialog 包骨架**
   - `mkdir internal/dialog`
   - 写 `internal/dialog/doc.go`：`package dialog` + 一行包注释。

2. **迁入 frame.go**
   - 把 `internal/action/floatframe.go` 整文件内容复制到 `internal/dialog/frame.go`。
   - 改 `package action` → `package dialog`。
   - 在文件末尾把 `var TheFloatFrame *FloatFrame` 改为 `var TheFloatFrame = NewFloatFrame()`（合并声明 + 初始化）。
   - 其余逻辑、注释、import 原样保留（import 已经只有 config/screen/tcell，无需改）。

3. **迁入 select.go**
   - 把 `internal/action/selectdialog.go` 整文件内容复制到 `internal/dialog/select.go`。
   - 改 `package action` → `package dialog`。
   - 其余原样（裸用的 `TheFloatFrame`、`Rect/Pos/Size/FloatOpenSpec`、`min`、`maxItemLen` 在 dialog 包内仍然可见，无需改前缀）。

4. **删除 action 包内的两个旧文件**
   - `rm internal/action/floatframe.go internal/action/selectdialog.go`
   - 删前确认 §2.2 列出的引用点都已迁完（步骤 5-7）。

5. **改 action/globals.go**
   - 从 `InitGlobals()` 函数体内删除 `TheFloatFrame = NewFloatFrame()` 这一行（连同行尾注释），保留其余初始化语句（`InfoBar`、`TheNotePane`）不变。

6. **改 action 包内两处调用方**
   - `internal/action/notepane.go`：文件头 import 加 `dialog "github.com/micro-editor/micro/v2/internal/dialog"`；两处 `NewSelectDialog()` → `dialog.NewSelectDialog()`；两处 `Pos{...}` → `dialog.Pos{...}`。
   - `internal/action/command_neo.go`：同样加 import；`Pos{X:0, Y:-1}` → `dialog.Pos{X:0, Y:-1}`；`NewSelectDialog()` → `dialog.NewSelectDialog()`。

7. **改 cmd/micro/micro.go**
   - import 块加 `"github.com/micro-editor/micro/v2/internal/dialog"`。
   - 6 个 `action.TheFloatFrame` 引用点（535、536、583、584、586、587）全部改成 `dialog.TheFloatFrame`。

8. **编译**
   - `make build`（必须走 Makefile，不用 `go build`）。
   - 若提示循环 import：说明 §4.1 的决策被违反——检查 dialog 包内是否误引了 action（rg `"internal/action"` internal/dialog/）。
   - 若提示 `min` redeclared：说明 action 包别处也有人定义了 `min`——回到 §2.1 的核实，确认 `min` 是否真的只在 selectdialog.go 用（目前已核实仅此一处）。

9. **回归手测**（见 §6）。

---

## 6. 风险与测试要点

### 6.1 循环依赖（最大风险，已有对策）

- 表现：`make build` 报 `import cycle not allowed: internal/action -> internal/dialog -> internal/action`。
- 对策：dialog 包**只允许** import `config / screen / tcell / tcell下的子包`。迁完后跑一次 `rg '^import' -A 10 internal/dialog/*.go` 人工核对 import 列表，确认没有 `internal/action`。
- keyEvent / infodefaults 是未来 InputDialog 引入 action 引用的潜在入口（§7），本计划不引入。

### 6.2 全局单例初始化时机

- 改成包级 `var TheFloatFrame = NewFloatFrame()` 后，TheFloatFrame 在 dialog 包被首次 import 时即就绪，早于 `InitGlobals()` 调用。`NewFloatFrame()` 不碰 screen，无就绪顺序问题。
- 若担心隐式初始化不符合项目风格，备选是保留显式初始化：在 dialog 包加 `func Init() { TheFloatFrame = NewFloatFrame() }`，由 `action.InitGlobals()` 调 `dialog.Init()`。两种都可，推荐包级变量（少一个函数、少一次跨包调用）。

### 6.3 回归手测清单

迁移不改逻辑，回归面很小，聚焦两条 SelectDialog 路径 + 容器本身：

- `:theme`（无参数）→ 弹出主题选择器；↑↓ 滚动、Enter 切换并持久化、Esc 取消、列表超出视口时 ▲▼ 指示符正常。对应 `command_neo.go:ThemeCmd`。
- NotePane Receiver 选择器（多 receiver 时）→ 弹出选择器；选中后 notePane 正确打开；Esc 取消。对应 `notepane.go:notePaneOpen / notePaneSelect`。
- 屏幕 resize 时浮窗自动关闭且回调收到取消（`OnResize` 路径）：在浮窗打开状态下缩放终端窗口，确认浮窗消失、调用方未崩溃。
- 浮窗打开时其它键被吞掉（modal）：在浮窗打开状态下敲字符，确认不泄漏到主编辑器。

### 6.4 低风险项（已排查，列出备查）

- `min` 重复定义：已 rg 确认 action 包内 `min` 仅 selectdialog.go 使用，随迁不冲突。
- `Rect` 与 `finder.Rect` 撞名：分属不同包，带前缀引用，不冲突。
- 注释里出现「参见 docs/...」：旧 selectdialog.go / floatframe.go 的注释引用了设计文档路径。按项目规则「代码注释不引用文档」，迁入时顺手把这些引用删掉，注释改写成自包含散文（一两句讲清为什么）。这一步合并在步骤 2/3 里做，不算额外工作。

---

## 7. 后续：InputDialog 落位与 keyEvent 可见性（ownerPane 注入）

本计划把 dialog 包立起来、把 SelectDialog 迁进去。N1（InputDialog）实施时，InputDialog 直接写进 `internal/dialog/input.go`，形态与 SelectDialog 对称。

### 7.1 未来的 Dialog 分析

| Dialog | 是否需要用户键位配置 | 说明 |
|---|---|---|
| **SelectDialog** | 否 | 简单键盘操作（↑↓/Enter/ESC），直接 `switch ev.Key()` |
| **InputDialog** | **是** | 需把 `tcell.EventKey` 解析成键名，再查 `config.Bindings["command"]` |
| **ConfirmDialog** | 否 | OK/Cancel 按键，直接 `switch ev.Key()` |
| **MsgDialog** | 否 | 滚动 + Close 按键，直接 `switch ev.Key()` |

**结论**：只有 InputDialog 需要访问用户键位配置，其他 Dialog 都不需要。

### 7.2 选定方案：ownerPane 注入 keyResolver + dialog 直接读 config

基于两个代码事实，方案得以简化：

**事实 1：`config.Bindings["command"]` 已是合并后的完整键位映射。**

`action.InitBindings()`（`bindings.go:38`）启动时先把 `infodefaults`（`defaults_other.go`）逐条写入 `config.Bindings["command"]`，再叠上用户 `bindings.json` 的覆盖。也就是说 `config.Bindings["command"]` 本身就包含了默认值 + 用户自定义，**不需要再单独传 `infodefaults`**。而 `config` 是叶子包（只依赖 util/lua/runtime），dialog 可以直接 import、自己读。

**事实 2：finder 由 ownerPane 创建/open，ownerPane 在 action 包内。**

finder 的生命周期由 ownerPane（BufPane，action 包内）掌控：`bufpane.go:282` 的 `finder.NewSession()` 创建、`fileops.go:30` 的 `h.finder.Open(...)` 打开。ownerPane 在 action 包内，能直接访问 `keyEvent` 这个私有函数。因此 ownerPane 可以在 `NewSession` 时把键位解析能力作为函数值注入给 finder——这与 finder 现有的 `onClose func(Result)` 注入模式完全对称（`fileops.go:30` 的 `h.onFinderClose`）。

**设计**：

1. dialog 定义 `KeyResolver` 函数类型，`Open` 时接收一个 `keyResolver` 参数。
2. dialog 自己 import `config`，直接读 `config.Bindings["command"]` 查动作。
3. ownerPane 在 `NewSession` 时注入 `keyResolver` 闭包（闭包捕获 action 包内私有 `keyEvent`）。
4. finder 持有 `keyResolver`，打开 InputDialog 时透传。

```go
// internal/dialog/input.go
import "github.com/micro-editor/micro/v2/internal/config"

type KeyResolver func(event tcell.Event) string

type InputDialog struct {
    // ...
    keyResolver KeyResolver
}

func (d *InputDialog) Open(..., keyResolver KeyResolver) {
    d.keyResolver = keyResolver
    // ...
}

func (d *InputDialog) lookupAction(event tcell.Event) string {
    keyName := d.keyResolver(event)  // 注入的闭包解析键名
    if action, ok := config.Bindings["command"][keyName]; ok {
        return action
    }
    return ""
}
```

```go
// internal/finder/session.go（finder 持有 keyResolver，透传给 InputDialog）
type Session struct {
    // ...
    keyResolver func(tcell.Event) string  // 由 ownerPane 注入
}

func NewSession(keyResolver func(tcell.Event) string) *Session {
    return &Session{keyResolver: keyResolver}
}

// 重命名时：finder 透传 keyResolver 给 InputDialog
dlg := dialog.NewInputDialog()
dlg.Open(initial, "Rename", anchor, 40, style, onResult, fm.keyResolver)
```

```go
// internal/action/bufpane.go:282（ownerPane，在 action 包内，闭包捕获私有 keyEvent）
h.finder = finder.NewSession(func(e tcell.Event) string {
    if k, ok := e.(*tcell.EventKey); ok {
        return keyEvent(k).Name()  // action 私有，直接用
    }
    return ""
})
```

**依赖图（无环）**：
```
finder → dialog → config（叶子）
action → finder
action → dialog
```
（注：buffer 依赖为 N1 InputDialog 预留，当前 dialog 包不引用。）

**优点**：
- 不新建任何包，不改 `events.go`（现有代码零改动）。
- 注入模式和 finder 现有的 `onClose` 对称，finder 代码风格一致。
- `config.Bindings["command"]` 已含默认值 + 用户配置，dialog 直接读，只需注入 `keyResolver`（1 个参数，不是 2 个）。

**代价**：
- `InputDialog.Open` 多 1 个 `keyResolver` 参数。
- `finder.NewSession` 多 1 个参数、`Session` 多 1 个字段。

### 7.3 为什么不抽 keyevent 共享包（方案 A）

方案 A（新建 `internal/keyevent` 包，从 `events.go` 迁入 `KeyEvent`/`Parse`/`Name()`）被否决，理由：

- 违反「如非必要勿增实体」：只有 InputDialog 需要键名解析，新建一个包只为一个组件服务。
- 需要改 `action/events.go`（alias + wrapper），碰核心文件。
- ownerPane 注入方案（§7.2）不碰现有代码、不新建包，更简洁。

keyevent 共享包可以作为未来储备：如果多个 Dialog 都需要键名解析，或者 finder 以外的叶子包也需要，再考虑抽出。

### 7.4 ownerPane / finder 需要的改动

**ownerPane（`action/bufpane.go`）**：`NewSession()` 调用点加一个 keyResolver 闭包。

```go
// bufpane.go:282 改动
h.finder = finder.NewSession(func(e tcell.Event) string {
    if k, ok := e.(*tcell.EventKey); ok {
        return keyEvent(k).Name()  // action 私有，直接用
    }
    return ""
})
```

**finder（`internal/finder/session.go`）**：`Session` 加一个字段，`NewSession` 加一个参数，重命名动作里透传给 InputDialog。

这两个改动都不碰 action 现有代码的核心逻辑，只是在构造点新增一个闭包参数（和已有的 `onClose` 注入对称）。`action/events.go` 完全不动。

---

## 附：Files to Modify / New Files 速查

**New Files**

- `internal/dialog/doc.go` — 包注释。
- `internal/dialog/frame.go` — 由 `action/floatframe.go` 迁入，`TheFloatFrame` 改包级初始化。
- `internal/dialog/select.go` — 由 `action/selectdialog.go` 迁入。

**Files to Modify**

- `internal/action/floatframe.go` — 删除（内容迁入 dialog/frame.go）。
- `internal/action/selectdialog.go` — 删除（内容迁入 dialog/select.go）。
- `internal/action/globals.go` — 删 `TheFloatFrame = NewFloatFrame()` 一行。
- `internal/action/bufpane.go` — §7.4：`NewSession()` 调用点加 keyResolver 闭包（N1 阶段）。
- `internal/finder/session.go` — §7.4：`Session` 加字段、`NewSession` 加参数（N1 阶段）。
- `internal/action/notepane.go` — 加 dialog import；2 处 `NewSelectDialog()` / `Pos{}` 加 `dialog.` 前缀。
- `internal/action/command_neo.go` — 加 dialog import；`Pos{}` / `NewSelectDialog()` 加 `dialog.` 前缀。
- `cmd/micro/micro.go` — 加 dialog import；6 处 `action.TheFloatFrame` → `dialog.TheFloatFrame`。
