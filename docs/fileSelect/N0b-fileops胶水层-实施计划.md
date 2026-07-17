# N0b — fileops 胶水层统一（实施计划）

本文是 `N0-文件管理器独立成包讨论.md` 的**第二阶段**实施计划，在 `N0a-finder独立成包-实施计划.md` 落地之后做。

N0a 已经把 `internal/finder/` 造成一个自洽的、零 action 依赖的包，公共契约（`Open(rect, cwd, file, isQuit, onClose)` + `Result{Reason, Cwd, File, IsQuit}` + `IsOpen`/`HandleEvent`/`Display`）已固定。但 N0a **完全没有动 action**——action 仍跑旧的 `FileSelector`（寄生 `TheFloatFrame`），finder 包是 dead code。

N0b 就是把 owner（BufPane）侧**全部联接到 finder 上**并清理旧代码：给 `BufPane` 接两条转发线、把 `filemanager.go` 改名 `fileops.go` 写成最终态（单一入口 `OpenFinder` + 统一 `onFinderClose`）、birth trigger 收敛到 `finishInitialize` 一行 hook、删 6 个 spawn wrapper、接通 cwd 链路、删掉 action 里的旧 `FileSelector` 实现。N0b 全程在 `internal/action/` + `cmd/micro/micro.go` 内做，**不再动 finder 包**。

---

## 1. 目标

把 owner（BufPane）侧与 finder 的接线层，从现状的「旧 FileSelector 寄生 FloatFrame + 三 trigger + 6 wrapper + 启动段」杂乱态，收敛成讨论 §B 描述的稳定形态：

- **owner 两条转发线**：`BufPane` 持有 `finder *finder.Session` 字段；`HandleEvent` 头部转发（modal 屏蔽）、`Display()` 末尾追加 `finder.Display()`。
- **单一入口** `OpenFinder(isQuit)`：三个 caller（`FileCmd` / `QuitNeo` / `finishInitialize` hook）全部经它进 finder。
- **单一 handler** `onFinderClose(Result)`：按 `Result.Reason` 一维分发（`IsQuit` 入参由 finder 透传到 `Result.IsQuit`；预检放不下走 Open 返回 false 分支，不进 onClose）。
- **birth 自动化**：`BufPane.finishInitialize` 末尾一行 hook 取代 6 个 spawn wrapper + 启动段调用。
- **cwd 链路接通**：`Result.Cwd` → `doQuit` 暂存 → `lastWorkingDir` 优先读，让 shell 的 `m()` 能跟到 finder 里导航到的目录。
- **删旧实现**：删 action 里的 `fileselector.go` / `fileselector_git.go` / 旧测试，消除 N0a 留下的重复代码。
- **删 R7 nil 防御**：`h.Buf` 永不为 nil 是事实，`onFinderClose` 不写 `if h.Buf == nil`。

N0b 落地后用户可见行为与现状基本一致（除下文 §6 标注的微小回归）；变的是代码结构、一条 cwd 同步增强、以及 finder 接管了渲染与事件路由（脱离 FloatFrame）。

---

## 2. 前置条件

- N0a 已合入：`internal/finder/` 包存在、`finder.Session` 契约固定、可编译可单测。finder 包此时尚未被调用。
- N0b 不再改 finder 包任何签名——只用 N0a 已暴露的 `NewSession` / `Open` / `IsOpen` / `HandleEvent` / `Display` / `NotifyBlur` / `Result` / `CloseReason`（五个常量）。若发现需要改 finder 签名，说明 N0a 契约没定够，应回到 N0a 补，而不是在 N0b 里临时改 finder。

---

## 3. 现状关键事实（N0a 落地后）

| 事实 | 位置 |
|---|---|
| finder 包已就绪但未被调用（dead code） | `internal/finder/` |
| 旧 `FileSelector` 仍在跑，寄生 `TheFloatFrame` | `internal/action/fileselector.go` / `fileselector_git.go` |
| 三 trigger：`openBrowseSelector` / `openQuitSelector` / `OpenBirthSelector`，各写一份 close 回调，内部仍 `new FileSelector` + `TheFloatFrame.Open` | `internal/action/filemanager.go` |
| helper：`isNoNameBuf` / `birthDir` / `startDirOf` | `filemanager.go` |
| 6 spawn wrapper：`neoAddTabAction` / `neoVSplitAction` / `neoHSplitAction` / `neoNewTabCmd` / `neoVSplitCmd` / `neoHSplitCmd`，各自「捕 dir → 调原生 spawn → `OpenBirthSelector(np, dir)`」 | `command_neo.go:250-303` |
| `InitNeoCommands` 覆写：`BufKeyActions["AddTab"]` / `BufKeyActions["VSplit"]` + `commands["tab"/"vsplit"/"hsplit"]` 三行；`bufpane.go` 包级 map `"HSplit"` 指 `neoHSplitAction` | `command_neo.go:34-38`；`bufpane.go:859` |
| 启动段：`action.OpenBirthSelectors(action.MainTab().CurPane(), "")` | `cmd/micro/micro.go:513` |
| `isNoName` 字段 + `QuitNeo` 的 `if !h.isNoName { return h.Quit() }` 分支 | `bufpane.go:260`；`command_neo.go:100` |
| `BufPane` 嵌入 `display.BWindow`（`BufPane.Display()` 是提升方法 `BufWindow.Display()`）；**尚未持有 finder 字段** | `bufpane.go:208` |
| `BufPane.HandleEvent` 头部是 ExternallyModified reload 检查（**尚未转发 finder**） | `bufpane.go:436-454` |
| `BufPane.finishInitialize`：`initialRelocate` + `initialized=true` + `onBufPaneOpen` plugin 回调 | `bufpane.go:297-305` |
| `BufPane.Resize` 末尾 `if !h.initialized { h.finishInitialize() }`（首次 Resize 才触发） | `bufpane.go:310-312` |
| `Tab.HandleEvent` 末尾 `t.Panes[t.active].HandleEvent(event)`：所有非鼠标事件（含 EventResize）都路由到活动 pane | `tab.go:337` |
| `Tab.HandleEvent` 对鼠标事件**先于** pane 分发做自身路由：Button1 释放时 `SetActive(被点 pane)`、按下分割条时拖拽 `t.resizing`、滚轮走光标下方的 pane | `tab.go:290-336` |
| 现状 FloatFrame 是**主循环级全局 modal**（`micro.go:585` `else if TheFloatFrame.IsOpen()`），开着时 `Tabs.HandleEvent` 根本不跑——所以旧 FileSelector 期间鼠标路由全跳过、owner 焦点不会移走 | `cmd/micro/micro.go:583-588` |
| `BufPane.SetActive(b bool)` **已存在**（影子化 `BWindow.SetActive`）：有 `if h.IsActive()==b` 早退守卫；`b==true` 那支做 gutter + `onSetActive` 插件钩子，**`b==false` 那支空着**——正是挂失焦通知的位置，无需新增方法 | `bufpane.go:717-729` |
| `SmallPane` 场景 4（单 pane + 无其他 tab）走 `h.neoHSplitAction()` | `command_neo.go:148` |
| `lastWorkingDir()` 读 `MainTab().CurPane().Buf.AbsPath` 的父目录；`exit()` 写 `--cwd-file` | `cmd/micro/micro.go:285-299` |
| 原生 `BufPane.Quit()`（自带 modified 检查 + 关 pane） | `actions.go` |

最后两条合起来是 owner 接线的关键不变式：**finder 脱离 FloatFrame 后，EventResize 经 `Tabs.HandleEvent → 活动 pane.HandleEvent` 仍然到达 owner 头部转发**——resize 不需要单独的通知通道，它和按键走同一通道进 `HandleEvent`，finder 在里面拦截后自关。但鼠标是另一回事：`Tab.HandleEvent` 的鼠标路由（切焦点 / 拖分割条 / 滚轮）在 owner 头部转发**之上**，owner 的 modal `return` 拦不住——点别的 pane 会让 owner `SetActive(false)`。这条由已有的 `BufPane.SetActive`（`bufpane.go:717`）失焦分支 + fileops.go 的 `onOwnerBlur` + finder 的 `NotifyBlur`（N0a §1）共同兜住：失焦 → finder 自决 `close(Esc)`。详见步骤 1。

`h.Buf = nil` 全库无赋值点（讨论 §B.3 已核实）：R7 触发条件为空集，N0b 整段删掉。

---


## 4. 实施步骤

每步都是一个可编译、可手测的提交点。

### 步骤 1 — 打好基础（fileops.go + BufPane finder 字段 + 转发 + 失焦取消）

把 `filemanager.go` 改名 `fileops.go`，写好 `OpenFinder` / `onFinderClose` / `doQuit` / `LastFinderCwd` / `onOwnerBlur`；给 `BufPane` 加 `finder` 字段 + `HandleEvent` / `Display` 转发线；在已有的 `SetActive`（`bufpane.go:717`）失焦分支调 `h.onOwnerBlur()`（收口 finder 调用）。本步纯增量，不动任何调用点。

**owner 两条转发线 + 字段**：

```go
package action

import "github.com/micro-editor/micro/v2/internal/finder"

type BufPane struct {
    display.BWindow
    // ... 原字段
    finder *finder.Session
}

func newBufPane(/* ... */) *BufPane {
    h := &BufPane{/* ... */}
    h.finder = finder.NewSession()
    return h
}

// HandleEvent：头部转发（必须在 reload 检查之前，§A.4）
func (h *BufPane) HandleEvent(event tcell.Event) {
    if h.finder.IsOpen() {
        h.finder.HandleEvent(event)
        return // modal：owner 整段全不跑
    }
    // 原 ExternallyModified reload 检查 + switch ...（不动）
}

// Display：shadow BWindow.Display()，末尾追加 finder
func (h *BufPane) Display() {
    h.BWindow.Display()
    if h.finder.IsOpen() {
        h.finder.Display()
    }
}

// SetActive：失焦时经 fileops 取消 finder——Tab 鼠标路由在 modal return 之上，点别的 pane 拦不住
func (h *BufPane) SetActive(b bool) {
    if h.IsActive() == b {
        return
    }
    h.BWindow.SetActive(b)
    if b {
        // 原 gutter + onSetActive 钩子（不动）
    } else {
        h.onOwnerBlur()
    }
}
```

**fileops.go（由 filemanager.go 改名）**：新增 `OpenFinder` / `onFinderClose` / `doQuit` / `LastFinderCwd` / `onOwnerBlur`；旧的 `openBrowseSelector` / `openQuitSelector` / `OpenBirthSelector` / `isNoNameBuf` / `birthDir` / `startDirOf` 暂留（步骤 2 删）。

```go
// fileops.go (action 包) —— BufPane 与 finder 之间的接线层

// OpenFinder —— 启动 finder 会话（三个 caller 的唯一对外接口）
// caller 契约: h.Buf != nil 且 h.BWindow 已拿到真尺寸
func (h *BufPane) OpenFinder(isQuit bool) {
    var dir, file string
    if abs := h.Buf.AbsPath; abs != "" {
        dir  = filepath.Dir(abs)  // cwd：当前文件所在目录（全路径、不含文件名）
        file = filepath.Base(abs) // 纯文件名（不含路径）
    } else {
        dir, _ = os.Getwd() // noName / 启动段 fallback 到 cwd
    }
    v := h.BWindow.GetView()
    if !h.finder.Open(finder.Rect{v.X, v.Y, v.Width, v.Height}, dir, file, isQuit, h.onFinderClose) {
        // 预检放不下: finder 没开会话、不触发 onClose（owner 自己处理）
        if isQuit {
            h.Quit() // quit 入口窄窗直接退（无 Cwd，不写 LastFinderCwd）
        }
        return
    }
}

// onFinderClose —— 共享 onClose：按 Result.Reason 一维分发（见决策表）
func (h *BufPane) onFinderClose(r finder.Result) {
    switch r.Reason {
    case finder.Picked:
        if filepath.Join(r.Cwd, r.File) == h.Buf.AbsPath {
            return // 同文件 no-op
        }
        h.OpenCmd([]string{filepath.Join(r.Cwd, r.File)})
    case finder.Quit:
        h.doQuit(r)
    case finder.Resize, finder.Esc:
        // no-op（预检放不下走 Open 返回 false 分支，不进 onClose）
    }
}

// doQuit —— 先暂存 r.Cwd 再调原生 h.Quit()；顺序不能反（Quit 后 pane 没了）
func (h *BufPane) doQuit(r finder.Result) {
    LastFinderCwd = r.Cwd
    h.Quit()
}

// LastFinderCwd —— 包级暂存变量（导出）；doQuit 写 / lastWorkingDir 在 exit 时读
var LastFinderCwd string

// onOwnerBlur —— owner 失焦时取消 finder 会话（若开着）；由 BufPane.SetActive(false) 调
func (h *BufPane) onOwnerBlur() {
    if h.finder != nil && h.finder.IsOpen() {
        h.finder.NotifyBlur()
    }
}
```

`onFinderClose` 是 `(*BufPane)` 的 method（不是包级函数）：回调体要调 `h.OpenCmd` / `h.Quit`，而三个 caller 的 `h` 是不同 `*BufPane` 实例。`OpenFinder` 传 `h.onFinderClose`（Go 绑定方法值），当次的 `h` 随回调带走。详见讨论 §B.3「为什么 onFinderClose 是 method」。

**验收**：`make build` 通过；`go test ./internal/finder/...` 通过（finder 包正常）；finder 包尚未被调用（dead code，等待步骤 2 接线）；`BufPane.SetActive` 的失焦分支已就位（步骤 2 接通 finder 后才真正生效）。

---

### 步骤 2 — 一次性切换所有调用点

所有调用点一次性切到新入口：browse/quit → `OpenFinder`；birth → `finishInitialize` hook；删掉旧的 trigger / wrapper / 调用点。

**两个 binding 改调 OpenFinder**：

```go
func (h *BufPane) FileCmd(args []string) { h.OpenFinder(false) }

func (h *BufPane) QuitNeo() bool {
    if !h.isNoName {
        return h.Quit() // file-born：完全等价原生 Quit
    }
    h.OpenFinder(true) // noName → quit 入口
    return true
}
```

**finishInitialize 加 birth hook**（`bufpane.go`）：

```go
func (h *BufPane) finishInitialize() {
    h.initialRelocate()
    h.initialized = true

    err := config.RunPluginFn("onBufPaneOpen", luar.New(ulua.L, h))
    if err != nil {
        screen.TermMessage(err)
    }

    // birth trigger —— noName 三条件全满足时置 isNoName=true 并开 finder；
    // 否则显式置 false。isNoName 是 sticky 出身标记，一次性写入。
    if h.Buf != nil
        && h.Buf.AbsPath == ""
        && h.Buf.Type == buffer.BTDefault
        && h.Buf.Size() == 0 {
        h.isNoName = true
        h.OpenFinder(false)
    } else {
        h.isNoName = false
    }
}
```

**为什么是这个时点**（讨论 §B.4）：`BufPane.Resize` 末尾 `if !h.initialized { h.finishInitialize() }` 保证 hook 只在首次 Resize 触发；此时 `h.BWindow` 的 X/Y/W/H 已由 `h.BWindow.Resize(w, h)` 设为真值（`newBufPane` 时是 `(0,0,0,0)` 占位，真尺寸由 split tree 算出后填）。`initialRelocate` 已跑、cursor 已校准。与既有 `onBufPaneOpen` plugin 回调平级。

file-born pane（`:tab foo` / `:vsplit foo` / `:hsplit foo`）也走同一 `finishInitialize`，但 buffer 经 `NewBufferFromFile` 构建、`AbsPath` 非空，**天然不满足三条件**，自动跳过。

`isNoName` 字段保留（sticky 语义不变），赋值点在 hook 的 if-else 两路（满足→true，否则→false；显式赋值胜过依赖零值）。`QuitNeo` 的 `if !h.isNoName` 分支（`command_neo.go:100`）**不动**。详见讨论 §B.4「isNoName 字段保留」。

**与失焦关闭的关系（不会自取消）**：spawn 路径（`VSplitIndex`/`HSplitIndex`，`bufpane.go:661/675`）的时序是 `Resize()`（触发 finishInitialize → `OpenFinder`）**先于** `SetActive(currentPaneIdx)`——后者把**新 pane 设为 active**（`SetActive(true)`），而失焦分支只在 `SetActive(false)` 时触发，故 birth 的 finder 不会被这条机制误关。birth pane 的 finder 只会在用户随后真的点别处（`SetActive(false)`）时按 Esc 取消，符合预期。启动段首个 pane（`NewTabFromBuffer` → 唯一 pane）天然 active，同理安全。

**删旧代码**：

| 文件 / 位置 | 操作 |
|---|---|
| `fileops.go` | 删 `openBrowseSelector` / `openQuitSelector` / `OpenBirthSelector` / `isNoNameBuf` / `birthDir` / `startDirOf` |
| `command_neo.go:250-303` | 删 6 个 wrapper（`neoAddTabAction` / `neoVSplitAction` / `neoHSplitAction` / `neoNewTabCmd` / `neoVSplitCmd` / `neoHSplitCmd`） |
| `command_neo.go:34-38` | 删除 `InitNeoCommands` 覆写（AddTab/VSplit/tab/vsplit/hsplit） |
| `bufpane.go:859` | `"HSplit": (*BufPane).neoHSplitAction,` 改回 `(*BufPane).HSplitAction` |
| `cmd/micro/micro.go:513` | 删除启动段 `OpenBirthSelector` 调用 |
| `command_neo.go:148` | `SmallPane` 场景 4 改 `return h.HSplitAction()` |
| `cmd/micro/micro.go::lastWorkingDir` | 改为优先读 `LastFinderCwd` |

**lastWorkingDir 改法**（`cmd/micro/micro.go`）：

```go
func lastWorkingDir() string {
    if action.LastFinderCwd != "" { // finder 会话的导航成果优先
        return action.LastFinderCwd
    }
    if t := action.MainTab(); t != nil {
        if pane := t.CurPane(); pane != nil && pane.Buf != nil {
            if ap := pane.Buf.AbsPath; ap != "" {
                return filepath.Dir(ap)
            }
        }
    }
    return ""
}
```

**验收**：
- `make build` 通过
- 三入口（Ctrl-o / Ctrl-q noName / birth + 启动段）手测行为与改前逐项一致
- finder 开着时 resize → finder 自关、回编辑态
- finder 开着时点兄弟 pane → finder 按 Esc 自关、焦点落到被点 pane（不孤儿化）
- finder 开着时拖分割条 / 滚轮 → 不崩；若焦点因此切走则按上一条 Esc 自关
- 极窄 pane 开 finder → 拒开 + InfoBar 提示
- `grep openBrowseSelector\|openQuitSelector\|OpenBirthSelector\|isNoNameBuf\|birthDir\|startDirOf internal/action/` 无命中
- `grep neoAddTabAction\|neoVSplitAction\|neoHSplitAction\|neoNewTabCmd\|neoVSplitCmd\|neoHSplitCmd internal/action/` 无命中
- `m` 在 A 启动 → noName → Ctrl-q → finder 导航到 B → Ctrl-q Quit → shell `m()` 落在 B（cwd 同步）

---

### 步骤 3 — 删旧实现 + 收尾

N0a 留下的旧 `FileSelector` 实现（已被 finder 包取代、不再有任何调用方）整段删除，消除重复代码。

| 文件 | 操作 |
|---|---|
| `internal/action/fileselector.go` | **删除**（`FileSelector` 类型 + state + display + handleEvent + 纯工具 + 数据模型，全部已迁入 finder） |
| `internal/action/fileselector_git.go` | **删除**（已迁入 `finder/git.go`） |
| `internal/action/fileselector_test.go` / `fileselector_git_test.go` | **删除**（已迁入 finder） |

收尾检查：

- `grep TheFloatFrame internal/action/fileops.go` 无命中（finder 已脱离 FloatFrame，fileops 只调 finder）
- `grep "h.Buf == nil" internal/action/fileops.go` 无命中（R7 已删）
- `grep FileSelector internal/action/` 无命中（旧类型彻底消失）
- SelectPane（`:theme`）仍正常——FloatFrame 路径未动

**验收**：
- `make build` 通过
- `go test ./internal/finder/...` 通过
- 三入口 + resize + 窄窗 + cwd 同步手测全部正常

---

## 5. 涉及文件（N0b）

| 文件 | 改动 |
|---|---|
| `internal/action/fileops.go` | **由 `filemanager.go` 改名**；删三 trigger + `OpenBirthSelector` + `isNoNameBuf` / `birthDir` / `startDirOf`；新增 `OpenFinder` / `onFinderClose` / `doQuit` / `LastFinderCwd` / `onOwnerBlur` |
| `internal/action/bufpane.go` | `BufPane` 加 `finder *finder.Session` 字段（`newBufPane` 构造）；`HandleEvent` 头部转发；新增 `Display()` method；已有的 `SetActive`（`bufpane.go:717`）失焦分支调 `h.onOwnerBlur()`；`finishInitialize` 末尾加 birth hook（if-else 两路）；包级 map `"HSplit"` 改回 `HSplitAction` |
| `internal/action/command_neo.go` | 删 6 wrapper；`InitNeoCommands` 删 AddTab/VSplit 覆写 + tab/vsplit/hsplit 命令覆写；`FileCmd`→`OpenFinder(false)`；`QuitNeo`→`OpenFinder(true)`；`SmallPane` 场景 4 改 `HSplitAction` |
| `internal/action/fileselector.go` / `fileselector_git.go` / 对应测试 | **删除**（旧 `FileSelector` 实现已被 finder 包取代，消除 N0a 留下的重复代码） |
| `cmd/micro/micro.go` | 删 `:513` 启动段 `OpenBirthSelector` 调用；`lastWorkingDir` 优先读 `LastFinderCwd` |
| `internal/finder/` | **不动**（N0a 已定型） |

---

## 6. 验证清单

单测（finder 包已迁，直接跑 finder 包测试）：

- `go test ./internal/finder/...`：git 解析 / 纯工具（原 fileselector 测试集）。

手测（三入口 + resize + cwd 同步）：

- browse：`:open` / Ctrl-o 在 file-born pane → 选文件后打开；同文件 noop；resize 自关；窄窗拒开 + InfoBar 提示。
- quit noName：`:open` / Ctrl-o 在 noName pane → 选文件后打开；Ctrl-q → finder → 选文件后打开；Ctrl-q → Quit；窄窗拒开 + Ctrl-q 逃路（InfoBar 提示 + 退回 finder）。
- birth：`:tab` / `:vsplit` / `:hsplit`（无参）→ 新 pane 自动开 finder；带文件参数（`:tab foo`）→ 不开；启动 noName → 自动开 finder。
- 失焦取消（新增）：split 两 pane，A 开 finder（Ctrl-o）→ 点 B → A 的 finder 按 Esc 自关、焦点到 B，A 不留孤儿 finder；birth 新 pane 开 finder 后点回老 pane → 新 pane finder 自关、老 pane 获焦（新 pane 的 `isNoName` 仍 true，后续 Ctrl-q 仍走 quit selector）。
- cwd：`m` 在 A 启动 → noName → Ctrl-q → 导航到 B → Ctrl-q Quit → shell `m()` 落在 B（`lastWorkingDir` 读 `LastFinderCwd`）。
- SelectPane（`:theme`）仍正常——FloatFrame 路径未动。

---

## 7. 风险与约束

- **finder 不能 import action**：N0a 已保证 finder 包不 import action（`grep "github.com/micro-editor/micro/v2/internal/action" ./internal/finder` 无命中）。N0b 的 `action → finder` import 方向合法。
- **resize 路由依赖**：finder 的 `HandleEvent` 能收到 `EventResize` 依赖 `tab.go:337` 的默认路由（`t.Panes[t.active].HandleEvent(event)`）。未来若有人改 tab.go 的 resize 分发，需同步保证 owner 仍能收到 resize。
- **birth 起始目录**：现状 `OpenBirthSelector(pane, dir)` 显式收 spawn 时捕的父目录；`:cd ~/other` 后 HSplit，新 pane finder 起始 = `dir(当前文件)`。N0b birth 不传 dir、新 pane 无文件 → fallback cwd。**仅当 micro 的 cwd ≠ dir(当前文件) 时有差**。绝大多数使用下 cwd == dir(当前文件)，用户进 finder 也能跳转，简化换来的微小回归。
- **FloatFrame 不动**：SelectPane（`:theme`）仍用 FloatFrame，N0b 不删 `floatframe.go`。

---

## 8. 与 N0a 的衔接

N0a 交付的**成品**是 `internal/finder/` 包本身 + 它的公共契约（`Open` 签名 + `Result`/`CloseReason` + `IsOpen`/`HandleEvent`/`Display`）。N0b 在这个契约之上做 owner 侧的全部联接工作，不再改 finder 包任何签名。

N0a 把「造一个自洽的包」这个高风险变更（断 FloatFrame、改契约、自画 overlay）单独隔离——它不依赖 action 任何改动、不影响 app 行为，可以独立编译、独立单测、独立 review。N0b 在「包已就绪」的稳态上做 owner 侧的联接与清理。

---

## 9. 行为变更总结

N0b 整体接近零行为变更，两处需要明说：

- **birth 起始目录**（可接受的微小回归，讨论 §B.8 已评估）：现状 `OpenBirthSelector(pane, dir)` 显式收 spawn 时捕的父目录；`:cd ~/other` 后 HSplit，新 pane finder 起始 = `dir(当前文件)`。N0b birth 不传 dir、新 pane 无文件 → fallback cwd。**仅当 micro 的 cwd ≠ dir(当前文件) 时有差**。绝大多数使用下 cwd == dir(当前文件)，用户进 finder 也能跳转，简化换来的微小回归。
- **点别的 pane 取消 finder**（有意的行为变更）：旧 FloatFrame 是主循环级全局 modal，finder 开着时鼠标被整体吞掉、点不到别的 pane。N0b 改 owner-local 后，点兄弟 pane 会让 owner 失焦，经 `onOwnerBlur → NotifyBlur → close(Esc)` 取消、焦点落到被点 pane。语义等价「点框外取消」，是更自然的 modal UX，不再是「鼠标被锁死」。

其余均为结构清理或新增能力（cwd 同步、同文件 noop、窄窗 quit 逃路语义统一）。
