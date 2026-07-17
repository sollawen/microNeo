# N0a — finder 包独立成包（实施计划）

本文是 `N0-文件管理器独立成包讨论.md` 的**第一阶段**实施计划。姊妹计划：`N0b-fileops胶水层-实施计划.md`（owner 侧联接与清理，在本阶段之后做）。

两阶段按「**先把 finder 这个包完全写好**（N0a，本文），**再把 fileops.go 完全写好、所有地方联接到一起**（N0b）」切开。N0a **只新增 `internal/finder/`，完全不动 `internal/action/`**——action 仍跑旧的 `FileSelector` 实现，app 行为零变化。N0a 落地后 finder 是一个自包含、可编译、可单测、零 action 依赖的独立包，公共契约（纯值入参 + 单一 Result）已固定；它此时**尚未被任何代码调用**（有意的中间态），N0b 负责把 owner 切换到 finder 并清理旧代码。

---

## 1. 目标

把现在散在 `internal/action/fileselector.go` / `fileselector_git.go` 的 `FileSelector`（类型 + state + display + handleEvent）+ git 解析 + 纯工具，**复制**进新包 `internal/finder/`，类型改名 `Session`，并把它从「寄生在 `TheFloatFrame` 上的浮窗」升格为「自洽的 pane 级 overlay」：

- finder 拥有自己那一次会话的全部状态（目录、条目、光标、git 标志、显示模式）。
- finder **不引用** `action` 包、`*BufPane`、`TheFloatFrame`、`InfoBar`。只 import `screen` / `config` / `tcell` / `runewidth`（都比 action 底层，不构成环）。
- finder 自己画边框、自己拦截 resize、自己做尺寸预检（照搬 `floatframe.go` 那三块机械代码）。
- 公共契约固定为**纯值进、单一结果出**：

```go
package finder

type Rect struct{ X, Y, W, H int } // owner pane 绝对屏幕矩形

type CloseReason uint8
const (
    Picked   CloseReason = iota // Enter 选中文件
    Esc                         // 用户按 Esc
    Quit                        // 用户按 Ctrl-q（或 q）
    Resize                      // 运行中终端 resize，已显示后被打断
    TooSmall                    // 打开时窗口过小，finder 从未显示
)

type Result struct {
    Reason CloseReason
    Cwd    string // 关闭时所在目录（始终填，纯目录、不带文件名）
    File   string // Reason==Picked 时：选中的文件名（不含路径）；其余为空
    IsQuit bool   // 原样回显 Open 入参 isQuit；finder 全程不读它
}

func NewSession() *Session
func (fm *Session) Open(rect Rect, cwd, file string, isQuit bool, onClose func(Result))
func (fm *Session) IsOpen() bool
func (fm *Session) HandleEvent(event tcell.Event)
func (fm *Session) Display()
```

类型名 `Session`：强调「一次数据进、数据出的短暂模态会话」（与讨论 §A.1「自己那一次短暂的会话」对齐）。两步式调用：`fm := finder.NewSession()` + `fm.Open(...)`。

**N0a 的边界**：只新增 `internal/finder/`，不动 action 一个字符。action 里的 `fileselector.go` / `fileselector_git.go` 原样保留——所以 N0a 期间 git 解析、字符串工具在两个包里各有一份（迁移中间态的必然，N0b 切换时删掉 action 那份）。finder 包完成后**没有任何调用方**，是 dead code——这是有意的，让「造包」和「联接」彻底分离、各自独立验证。

---

## 2. 范围与边界

**N0a 做**（本文）：

1. 新建 `internal/finder/`，复制 git 解析 + 纯工具 + 数据模型 + 对应测试进新包（零行为变化，action 不动）。
2. `FileSelector` → `finder.Session`，`Open` 改纯值入参，定下 `Result` / `CloseReason` 契约；升格 pane 级 overlay：自己画框 / 拦 resize / 预检，**断绝 FloatFrame**。

**N0a 不做**（全留给 N0b）：

- owner（BufPane）侧任何接线（`finder` 字段、`HandleEvent` 转发、`Display` shadow）。
- 三个 trigger 改调 finder、单一入口 `OpenFinder`、统一 `onFinderClose`。
- `finishInitialize` birth hook、删 6 个 spawn wrapper 与启动段调用。
- cwd 链路（`Result.Cwd` → `lastWorkingDir`）。
- 删 action 里的旧 `fileselector.go` / `fileselector_git.go` / 旧测试。
- 删 R7 `if h.Buf == nil` 防御。

N0a 必须自成可编译、可单测、可发布的一个 PR；落地后 app 行为零变化（因为根本没接 finder）。

---

## 3. 现状关键事实（已核实）

| 事实 | 位置 |
|---|---|
| `FileSelector` 类型 + state + display + handleEvent | `internal/action/fileselector.go` |
| git 解析（`getGitStatus` / `parsePorcelain` / `statusKind` / `dirState`） | `internal/action/fileselector_git.go`（import `config` 读 diffgutter 开关） |
| 纯工具：`truncateNameKeepExt` / `humanSize` / `fitMeta` / `truncateLeftPath` / `formatMtime` / `runeWidth` / `stringWidth` / `headByWidth` / `tailByWidth` / `isHiddenName` | `internal/action/fileselector.go` |
| 数据模型：`entry` / `readDirEntries` / `sortEntries` | `internal/action/fileselector.go` |
| `FileSelector.Open(pane *BufPane, startDir, onSelect)`（**强耦合 `*BufPane`**） | `fileselector.go:154` |
| `computeLayout` 调 `pane.BWindow.GetView()` 取 rect，并在窄窗时**直接调 `InfoBar.Message`** | `fileselector.go:362/365/369` |
| `buildSpec` 把 display/handleEvent 塞进 `FloatOpenSpec`，经 `TheFloatFrame.Open` 代画代路由 | `fileselector.go:390` |
| `finish` 收尾：`TheFloatFrame.Close()` + 回调 | `fileselector.go:838` |
| 边框/清屏/HideCursor/title 嵌入代码 | `internal/action/floatframe.go` `Display()`（约 60 行） |

关键约束：`TheFloatFrame` 在 action 包里，finder 一旦搬成独立包就**物理上够不到它**——`finder → action` 这条 import 边会让编译成环。所以 finder 的 `Session` 从搬进新包的第一刻起就不能再寄生 FloatFrame，必须自己画框、自己预检、自己拦 resize。这一条决定了 N0a 把「断 FloatFrame」和「搬 Session」放在同一步（见步骤 2），不存在「Session 还暂经 FloatFrame」的中间提交。

---

## 4. 实施步骤

每步都是一个可编译、可单测的提交点。N0a 不动 action，所以「手测 app 行为」留到 N0b——N0a 的验收在**包层面**（编译、单测、依赖检查）。

### 步骤 1 — 建包 + 平移纯函数与 git 逻辑（零行为变化）

新建 `internal/finder/`，把不依赖 `screen` 的纯函数与数据模型复制进新包（零逻辑改动，只换 `package`）。action 里的原件**原样保留**——本步纯增量，不动 action 调用点。

**搬运清单**（源 → 目标，逐字复制）：

- `action/fileselector_git.go` → `finder/git.go`：`getGitStatus` / `parsePorcelain` / `statusKind` / `dirState`
- `action/fileselector.go` 纯叶子工具 → `finder/strutil.go`：`truncateNameKeepExt` / `humanSize` / `fitMeta` / `truncateLeftPath` / `formatMtime` / `runeWidth` / `stringWidth` / `headByWidth` / `tailByWidth` / `isHiddenName`
- `action/fileselector.go` 数据模型 → `finder/model.go`：`entry` / `readDirEntries` / `sortEntries` / `sortMode`
- 测试：`fileselector_git_test.go` → `finder/git_test.go`；`fileselector_test.go` 纯函数用例 → `finder/strutil_test.go`

这批函数只依赖 `os` / `sort` / `strings` / `time` / `runewidth`，复制进新包即可编译、即可跑通原测试。action 侧原文件不动，期间 git 解析与工具在两包各存一份——这是迁移中间态，N0b 切换时删掉 action 那份。

**验收**：`make build` 通过；`go test ./internal/finder/...` 通过。

### 步骤 2 — `FileSelector` → `finder.Session` + 契约 + 自画 overlay + 断 FloatFrame

把 `FileSelector`（类型 + state + display + handleEvent）复制进 `internal/finder/`，改名 `Session`，定下对外契约，并一次性升格成 pane 级 overlay——自己预检、自己画框、自己拦 resize。finder 从此不再触碰 `TheFloatFrame`。

**新增契约**（finder 包对外固定）：`Rect` / `CloseReason` / `Result` / 五个公开方法（见 §1）。

**与旧实现的关键差异**（都是讨论 §A 已定）：

- `Open` 改纯值入参：不再收 `*BufPane`，改收 `rect` + `cwd` + `file` + `isQuit` + `onClose`。原 `pane.BWindow.GetView()` 取 rect、`pane.Buf.AbsPath` 取 file 的逻辑，全部外移到调用方（N0b 的 `OpenFinder`）。
- 关闭路径统一成一个 `close(reason)`：集中填 `Cwd` / `IsQuit`，5 条路径只换 `Reason`。
- 自己预检 + 自画框 + 自己拦 resize：照搬 `floatframe.go::Display` 那三块机械代码（4 角 `┌┐└┘` + 上下 `─` + 左右 `│` + clear + HideCursor + title 嵌入）。
- finder **不碰 `InfoBar`**：原 `computeLayout` 在窄窗时直接调 `InfoBar.Message`（`fileselector.go:365/369`），搬包后够不到。finder 只通过 `close(TooSmall)` 上报，提示由 owner 侧（N0b 的 `onFinderClose`）自己调 InfoBar（讨论 §A.2）。

**结构变化对比**（`buildSpec` → 直接调用）：

| 旧（via FloatOpenSpec） | 新（直接） |
|---|---|
| `spec.Display = fs.display` | `fm.Display()` 由 owner 调（N0b 步骤 1 的 `BufPane.Display()`） |
| `spec.HandleEvent = fs.handleEvent` | `fm.HandleEvent()` 由 owner 调（N0b 步骤 1 的 `BufPane.HandleEvent()`） |
| `spec.OnResize = func() { fs.finish(ReasonResize) }` | `HandleEvent` 拦截 `EventResize` → `fm.close(Resize)` |

Owner 调用改为 `h.finder.Open(...)` + `h.finder.HandleEvent()` / `h.finder.Display()`，不再通过 `FloatOpenSpec` 中转。

**常量复制**：`fsMinWidth` / `fsMinHeight`（`fileselector.go:34-35`）随 `Open` 方法一起复制进 `session.go`，用于尺寸预检。

**删 finder 内所有 FloatFrame 引用**：`buildSpec` / `TheFloatFrame.Open` / `TheFloatFrame.Close` / `FloatOpenSpec`。验证：`grep TheFloatFrame ./internal/finder` 应无命中。

```go
package finder

type Rect struct{ X, Y, W, H int } // owner pane 绝对屏幕矩形

type CloseReason uint8
const (
    Picked   CloseReason = iota // Enter 选中文件
    Esc                         // 用户按 Esc
    Quit                        // 用户按 Ctrl-q（或 q）
    Resize                      // 运行中 resize，已显示后被打断
    TooSmall                    // 打开时过小，finder 从未显示
)

type Result struct {
    Reason CloseReason
    Cwd    string // 关闭时所在目录（始终填）
    File   string // 仅 Picked 时：文件名（不含路径）
    IsQuit bool   // 原样回显 Open 入参 isQuit；finder 全程不读
}

func NewSession() *Session
func (fm *Session) Open(rect Rect, cwd, file string, isQuit bool, onClose func(Result))
func (fm *Session) IsOpen() bool
func (fm *Session) HandleEvent(event tcell.Event)
func (fm *Session) Display()
```

**旧契约（action 包 `SelectResult`，N0b 步骤 5 删）**：

```go
// 旧定义在 action/fileselector.go:120-150，N0a 期间不动

type ResultKind uint8
const (
    Closed ResultKind = iota
    Picked
)

type CloseReason uint8
const (
    ReasonEsc    CloseReason = iota
    ReasonQuit
    ReasonSize // 窄/矮窗拒开
    ReasonResize
)

type SelectResult struct {
    Kind   ResultKind
    Path   string // 仅 Picked 时：绝对路径
    Reason CloseReason // 仅 Closed 时
}
```

**语义映射**（旧 → 新）：

- `SelectResult{Kind: Closed, Reason: ReasonSize}` → `Result{Reason: TooSmall}`
- `SelectResult{Kind: Closed, Reason: ReasonResize}` → `Result{Reason: Resize}`
- `SelectResult{Kind: Closed, Reason: ReasonEsc}` → `Result{Reason: Esc}`
- `SelectResult{Kind: Closed, Reason: ReasonQuit}` → `Result{Reason: Quit}`
- `SelectResult{Kind: Picked, Path: <absolute>}` → `Result{Reason: Picked, Cwd: <dir>, File: <basename>}`

N0a 期间两套类型并存（action 的旧类型 + finder 的新类型），互不干扰。N0b 步骤 5 删除旧类型。

```go
// Open：纯值入参 + 自己预检
func (fm *Session) Open(rect Rect, cwd, file string, isQuit bool, onClose func(Result)) {
    fm.onClose = onClose
    fm.isQuit  = isQuit              // 全程不读（§A.6）

    if rect.W < fsMinWidth || rect.H < fsMinHeight {
        fm.close(TooSmall)           // 不够大就直接回调，不画
        return
    }
    fm.state  = newState(cwd, file)  // file=光标起始定位
    fm.rect   = rect
    fm.isOpen = true
    go fm.fetchGit(fm.state.currentDir)
}

// 关闭路径：before → after
// before:
//   func (fs *FileSelector) finish(r SelectResult) {
//       cb := fs.onSelect; TheFloatFrame.Close(); if cb != nil { cb(r) }
//   }
// after: 统一 close，5 条路径只换 Reason
func (fm *Session) close(reason CloseReason) {
    r := Result{Reason: reason, Cwd: fm.state.currentDir, IsQuit: fm.isQuit}
    if reason == Picked {
        r.File = fm.pickedName()
    }
    cb := fm.onClose
    fm.reset()
    if cb != nil {
        cb(r)
    }
}

// 五条调用点，只换 Reason
func (fm *Session) pick()      { fm.close(Picked) } // Enter on 文件
func (fm *Session) onEscKey()  { fm.close(Esc) }
func (fm *Session) onQuitKey() { fm.close(Quit) }   // Ctrl-q / q
// EventResize 命中: fm.close(Resize)
// TooSmall 由 Open 预检产生
```

**所有 `fs.finish(SelectResult{...})` 调用点**在旧 `FileSelector` 中有 8 处（`fileselector.go:188/199/388/764/767/777/826/831`），搬进 finder 后全部替换成 `fm.close(<new-constant>)`。Pane mutation（`OpenCmd` / `Quit`）由旧回调（`onSelect` 在 action 的 openBrowseSelector 等函数里定义）迁移到 N0b 的 `onFinderClose`。步骤 2 只负责把新契约在 finder 包里定好，不涉及 action 侧的回调实现。

```go
// Display：自己画框（照搬 floatframe.go::Display）
func (fm *Session) Display() {
    if !fm.isOpen {
        return
    }
    screen.Screen.HideCursor()
    fm.drawBorder()
    fm.drawContent()
}

// drawBorder：画方框边框（照搬 floatframe.go 三块机械代码）
func (fm *Session) drawBorder() {
    x, y, w, h := fm.rect.X, fm.rect.Y, fm.rect.W, fm.rect.H
    color := config.DefStyle

    // 1. clear 整矩形
    for row := 0; row < h; row++ {
        for col := 0; col < w; col++ {
            screen.Screen.SetContent(x+col, y+row, ' ', nil, color)
        }
    }
    // 2. 4 角 + 上下 ─
    screen.Screen.SetContent(x, y, '┌', nil, color)
    screen.Screen.SetContent(x+w-1, y, '┐', nil, color)
    screen.Screen.SetContent(x, y+h-1, '└', nil, color)
    screen.Screen.SetContent(x+w-1, y+h-1, '┘', nil, color)
    for i := 1; i < w-1; i++ {
        screen.Screen.SetContent(x+i, y, '─', nil, color)
        screen.Screen.SetContent(x+i, y+h-1, '─', nil, color)
    }
    // 3. 左右 │
    for row := 1; row < h-1; row++ {
        screen.Screen.SetContent(x, y+row, '│', nil, color)
        screen.Screen.SetContent(x+w-1, y+row, '│', nil, color)
    }
    // 4. 上边框嵌 ──<title>──...─（照搬 floatframe.go 同段）
}

// drawContent：画内容（内缩 1）
func (fm *Session) drawContent() {
    // 原 fs.display(contentArea) 逻辑照搬，rect 现在是内缩 1 的内容区
}

// HandleEvent：头部拦 resize
func (fm *Session) HandleEvent(event tcell.Event) {
    if !fm.isOpen {
        return
    }
    if _, ok := event.(*tcell.EventResize); ok {
        fm.close(Resize) // 不转发
        return
    }
    fm.handleKey(event)
}
```

`gitCharStyle`（display 层，用 `config.GetColor`）跟着 display 一起进 finder——`finder → config` 合法。

**验收**：`make build` 通过；`go test ./internal/finder/...` 通过；`grep TheFloatFrame ./internal/finder` 无命中；`go vet ./internal/finder/...` 无环（finder 不 import action）。

---

## 5. 涉及文件（N0a）

| 文件 | 改动 |
|---|---|
| `internal/finder/git.go` | **新增**（复制自 `fileselector_git.go`） |
| `internal/finder/strutil.go` | **新增**（纯叶子工具复制） |
| `internal/finder/model.go` | **新增**（`entry` / `readDirEntries` / `sortEntries` 复制） |
| `internal/finder/session.go` | **新增**（`FileSelector` → `Session` + state + display + handleEvent；`Rect`/`CloseReason`/`Result`；纯值 `Open`；自画 overlay + resize 拦截 + 预检；`NewSession`） |
| `internal/finder/git_test.go` | **新增**（复制自 `fileselector_git_test.go`） |
| `internal/finder/strutil_test.go` | **新增**（复制自 `fileselector_test.go` 纯函数用例） |
| `internal/action/*` | **不动**（旧 `FileSelector` 实现原样保留，N0b 切换时删；`filemanager.go` 的三 trigger 原样保留，N0b 步骤 1 改名 `fileops.go`） |
| `internal/action/floatframe.go` | **不动**（边框代码照搬进 finder，FloatFrame 本体继续服务 SelectPane；N0a/N0b 都不改 FloatFrame） |
| `cmd/micro/micro.go` | **不动** |

---

## 6. 验证清单

单测（纯函数，复制后原样跑通）：

- `parsePorcelain`：分支头 / untracked / ignored 折叠 / 优先级聚合 / prefix 剥离。
- `truncateNameKeepExt` / `humanSize` / `fitMeta` / `truncateLeftPath`：含 CJK 列宽用例。

包层面检查：

- `make build` 通过（finder 包编译过、不拖累 action）。
- `grep TheFloatFrame ./internal/finder` 无命中。
- `go vet ./internal/finder/...` 无环（finder 不 import action）。
- `grep "internal/finder" ./internal/action/` 无命中——确认 N0a 没误碰 action。

app 行为：**零变化**。action 仍跑旧 `FileSelector`，finder 包未被调用（dead code，有意为之）。

回归：

- SelectPane（`:theme`）仍走 FloatFrame——N0a 完全不碰 FloatFrame。

---

## 7. 风险与约束

- **import 环**：finder 一旦成包就物理上够不到 action 里的 `TheFloatFrame`，断 FloatFrame 是硬约束、非可选（讨论 §A.5）。步骤 2 必须做干净，`grep TheFloatFrame ./internal/finder` 应无命中。正因如此，N0a 不存在「Session 还暂经 FloatFrame」的中间提交——Session 从进新包第一刻就自画。
- **resize 路由依赖**：finder 的 `HandleEvent` 能收到 `EventResize` 依赖 `tab.go:337` 的默认路由（`t.Panes[t.active].HandleEvent(event)`）。这条路径 N0a 不改、N0b 步骤 1 的 `BufPane.HandleEvent` 转发依赖它。未来若有人改 tab.go 的 resize 分发，需同步保证 owner 仍能收到 resize。
- **dead code**：N0a 完成后 finder 包没有任何调用方。这是有意的中间态：让「造包」和「联接」彻底分离。代价是 git 解析、字符串工具在 finder / action 两包各存一份，直到 N0b 切换时删掉 action 那份。N0a 的验收因此只在包层面（编译 / 单测 / 依赖），真正的手测留到 N0b 接通 owner 之后。
- **不改 FloatFrame**：FloatFrame 去留与 finder 无关，继续服务 SelectPane。N0a 把 `floatframe.go` 的边框代码**照搬**（复制，不是移动）进 finder，FloatFrame 本体一个字不改。不要把 finder 断 FloatFrame 和任何「per-owner FloatFrame 改造」搅在一起（讨论 §A.5 / §D.3）。
- **类型名**：`Session`（已定）。两步式：`fm := finder.NewSession()` + `fm.Open(...)`。
- **回调签名迁移**：步骤 2 的核心重构是把回调签名从旧 `func(SelectResult)` 改成新 `func(Result)`。旧回调（`onSelect` 在 action 的 openBrowseSelector 等函数里定义）调用 `h.OpenCmd(...)` / `h.Quit()`，新回调（N0b 的 `onFinderClose`）接收 `Result` 后按 `Reason` 分发。步骤 2 只负责把新契约在 finder 包里定好，不涉及 action 侧的回调实现。

---

## 8. 与 N0b 的衔接

N0a 交付的**成品**是 `internal/finder/` 包本身 + 它的公共契约（`Open` 签名 + `Result`/`CloseReason` + `IsOpen`/`HandleEvent`/`Display`）。这就是 N0b 的全部输入。

N0b 在这个契约之上做 owner 侧的全部联接工作：

- 给 `BufPane` 接两条转发线（`HandleEvent` 头部转发 + `Display` shadow），加 `finder` 字段；
- 把 `filemanager.go` 改名 `fileops.go`，删旧三 trigger（`openBrowseSelector` / `openQuitSelector` / `OpenBirthSelector`），新建单一入口 `OpenFinder(isQuit)` + 统一 `onFinderClose(Result)`；
- `finishInitialize` birth hook 接管 birth，删 6 spawn wrapper + 启动段调用；
- 接 cwd 链路（`Result.Cwd` → `lastWorkingDir`）；
- 删 action 里的旧 `fileselector.go` / `fileselector_git.go` / 旧测试，消除重复代码。

**import 方向说明**：N0a 刻意避免 `action → finder` 的 import（finder 不 import action）。N0b 步骤 1 会立即引入这个 import 方向（`internal/action/fileops.go` 加 `import finder`，`BufPane` 加 `finder *finder.Session` 字段）——这是正常的，因为 fileops（owner 侧）必须调用 finder（modal component）。两阶段的约束边界在 N0a 结束、N0b 开始处切换。

N0a 把「造一个自洽的包」这个高风险变更（断 FloatFrame、改契约、自画 overlay）单独隔离——它不依赖 action 任何改动、不影响 app 行为，可以独立编译、独立单测、独立 review。N0b 在「包已就绪」的稳态上做 owner 侧的联接与清理。
