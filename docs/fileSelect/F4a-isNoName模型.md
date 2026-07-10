# F4a — isNoName per-pane 模型（替换 WelcomeMode）

## 0. 一句话

用 per-pane 的 `isNoName`（**诞生时赋值、终身不变**）替换全局 `WelcomeMode`。selector 是否自动开、Ctrl-q 怎么走，全由这个不变量决定。原本割裂的「启动 welcome / 新 split / 新 tab / 关文件回 welcome」统一成同一套规则。

---

## 1. 背景：为什么换掉 WelcomeMode

`WelcomeMode` 是**全局**变量、**启动期**靠**命令行参数**判断（`DetectWelcomeMode`：无文件名 + 真终端 → true）。它带来一堆特例：

- 启动要 `EnterWelcome` 显式开 selector（还要解决 0×0 时序）。
- 关最后一个文件要 `gotoWelcome` 回 welcome。
- `openWelcomeSelector` 的回调硬编码 `exitProgram()`。
- `QuitNeo` 用 `WelcomeMode × isLast` 三级判断。
- 新 split / 新 tab 创建空 buffer 时，welcome 机制完全不覆盖（本次新需求的原始痛点）。

换成 per-pane `isNoName` 后，**任何 pane 只要诞生时是 noName，就永远按 noName 规则走**——启动、split、tab、关文件回 welcome 全部自然统一，上面这些特例整体消失。

---

## 2. 核心不变量：`BufPane.isNoName`

- 类型：`bool`，挂在 `BufPane` 上（**不是** buffer 上，因为 buffer 会被换掉，而不变量要 sticky）。
- 赋值时机：pane 创建时（`finishInitialize`，见 §8），根据**初始 buffer** 是否 noName 赋值**一次**。
- 判定：`isNoNameBuf(buf) = buf.AbsPath == "" && buf.Type == BTDefault && 空`（空 = 单行且无内容）。
- **终身不变**：不论用户后来在这个 pane 里换了多少次文件，`isNoName` 都不改。它记录的是「这个 pane 怎么诞生的」。

> 配套（未来，不在本任务）：禁用「pane 内换文件」，用户只能通过 Ctrl-q→selector 切文件。本任务**允许** pane 内换文件，并接受「noName-born 的 pane 后来装了文件」这种混合态——因为 `isNoName` sticky，路由仍按 noName 走，这正是要的。

---

## 3. 完整状态机

### 3.1 noName-born pane（`isNoName = true`）

```
诞生(finishInitialize) ──auto──▶ selector【isQuit=false】（用户被困，只两出口）
                                      ├─ Enter 选文件 ──▶ 【编辑该文件】(原地换入)
                                      └─ Esc 不选     ──▶ 【编辑空 buffer】

【编辑空 buffer】──Ctrl-q──▶ selector【isQuit=true】
【编辑文件】    ──Ctrl-q──▶ (若 modified: 存盘提示) ──▶ selector【isQuit=true】

selector【isQuit=true】（两出口，无取消）
                                      ├─ Enter 选文件 ──▶ 【编辑该文件】(原地换入)
                                      └─ Ctrl-q 不选  ──▶ 关闭本 pane（最后一个→程序退出）
```

### 3.2 file-born pane（`isNoName = false`，如 `microNeo a.md`）

```
诞生 ──▶ 【编辑文件】（无 auto selector）
【编辑文件】──Ctrl-q──▶ 关闭本 pane（最后一个→程序退出）
```

**= micro 原生流程，零行为改动。** `QuitNeo` 对 file-born 直接透传 `h.Quit()`（自带存盘提示、最后→退程序），neo 完全不介入。整个改动的行为面只落在 noName-born pane 上——这是不变量，实施时不得改动 file-born 分支。
（“零改动”指 Ctrl-q/退出路径；Ctrl-o 在 file-born 上也走 neo selector，但那是 `:file` 早有的覆盖、早于 F4a，不计入本任务 delta。）

### 3.3 Ctrl-o（两种 pane 通用，已存在）

编辑态按 **Ctrl-o** → `selector【isQuit=false】`（留守/browse）：Enter 选文件 → 原地换入；Esc → 取消、继续编辑当前文件。**不论 isNoName**，且**不改 isNoName**（sticky）。

**无需新代码**——Ctrl-o 早已绑到 `:file`（`defaults_darwin.go:39` / `defaults_other.go:42` → `command:file`），即 `FileCmd → openSelector(isQuit=false)`，含 modified 存盘提示。本条只是把它显式纳入模型。

> `runtime/help/keybindings.md:566` 仍写 `OpenFile`（原生），stale；实际绑定 `command:file`。可顺手修，非阻塞。

---

## 4. isQuit：selector 的关闭语义

`Open(..., isQuit bool)` 的参数改名（旧名 `isWelcome`，随 welcome 概念一起退役）。语义改为直接描述「这个 selector 怎么关闭」：

- **isQuit=true**（Ctrl-q 进来的 quit selector）：**不收 Esc**，只收 **Ctrl-q** → 关 selector 并退出 pane。
- **isQuit=false**（留守/browse selector）：**不收 Ctrl-q**，只收 **Esc** → 关 selector，继续编辑当前 pane 的文件。诞生自动开 与 Ctrl-o 都用此模式。

| selector | isQuit | Esc | Ctrl-q（selector 内） |
|---|---|---|---|
| 诞生自动开（birth） | `false` | 关 selector → 继续编辑（空 buffer） | 不收 |
| Ctrl-o 开（browse） | `false` | 关 selector → 继续编辑（当前文件） | 不收 |
| Ctrl-q 开（quit） | `true` | 不收 | 关 selector + 退出 pane |

底层 `fileselector.go` 的 `handleEvent` 分支结构（`!flag` / `flag`）**不变**，仅把字段/参数名 `isWelcome` 改 `isQuit`：Esc 分支 `if !fs.isQuit`、Ctrl-q 分支 `if fs.isQuit`。**注意实参不是「真值一一对应」**——birth 调用点的实参从旧 `isWelcome=true`（Esc no-op）翻成新 `isQuit=false`（Esc 关闭），这是有意的行为变更（见 §3.1/§4 表格）；browse 实参不变（均 false）、quit 为新增场景（true）。换言之：底层 handleEvent 代码文本纯改名，但 birth 的**语义**随实参翻转而变。

---

## 5. Ctrl-q 路由（`QuitNeo` 重写）

```
func (h *BufPane) QuitNeo() bool {
    if !h.isNoName {
        return h.Quit() // file-born：直接关（原生自带存盘提示；最后→退程序）
    }
    // noName-born：开 quit selector（isQuit=true）。proceed 闭包去重下方 2 个调用点。
    proceed := func() {
        d := birthDir(h)
        if d == "" {
            d, _ = os.Getwd()
        }
        NewFileSelector().Open(h, d, func(r SelectResult) {
            if r.Kind == Picked {
                if h.Buf == nil { // R7 防御
                    return
                }
                b, err := buffer.NewBufferFromFile(r.Path, buffer.BTDefault)
                if err != nil {
                    InfoBar.Error(err)
                    return
                }
                h.OpenBuffer(b)
            } else { // Closed：ReasonQuit 关 pane / ReasonResize 透明
                h.Quit()
            }
        }, true)
    }
    if h.Buf.Modified() && !h.Buf.Shared() {
        h.closePrompt("Close", proceed) // y/n→proceed，esc→取消留编辑
        return true
    }
    proceed()
    return true
}
```

selector(true) 内再按 Ctrl-q（`ReasonQuit`）的回调 → `h.Quit()`（→`ForceQuit`：非最后 pane 时关本 pane；最后一个 pane/tab 时直接 `runtime.Goexit()` 退程序，原生既有 actions.go:1908-1910）。

> 不再需要 `exitProgram()`（手动 `screen.Fini` + `runtime.Goexit`）。退出统一走原生 `h.Quit()`。若实测 `h.Quit()` 在「selector 内 ReasonQuit」路径上不能干净退出，再回退到 `exitProgram()`。

---

## 6. 三种 selector.open 场景

三个场景直接调 `NewFileSelector().Open(pane, startDir, onSelect, isQuit)`（layout 预检与优雅退化已在 `Open` 内，fileselector.go:170）：

| 场景 | 触发 | isQuit | startDir | callback |
|---|---|---|---|---|
| ① birth | noName pane 诞生（`finishInitialize`） | `false` | 出生时**父 pane** 的文件目录，空→cwd（startup 无父→cwd） | `Picked`→换入文件；`ReasonEsc`→no-op |
| ② quit | noName pane 编辑态按 **Ctrl-q** | `true` | 当前 buffer 目录，空→cwd | `Picked`→换入文件；`ReasonQuit`→`h.Quit()` |
| ③ browse | 任意 pane 按 **Ctrl-o** | `false` | 当前 buffer 目录，空→cwd | `Picked`→换入文件；`ReasonEsc`→no-op（复用 `FileCmd`/`openSelector`） |

- ②③ 的 startDir 直接读 `h.Buf.AbsPath`（h 就是触发 pane，已知）。
- ① 的 startDir 难点见 §6.1。
- 三场景 = 三个 `NewFileSelector().Open(...)` 调用点，**直接内联，不新增包装函数**（仿现有 `openSelector`，command_neo.go:84）：① birth 在 §6.1 `onPaneBirthNeo`、② quit 在 §5 `QuitNeo`、③ browse = 现有 `openSelector`（不改）。
- 原生自身也是每次内联 `NewBufferFromFile`+`OpenBuffer`（command.go:307/516/536/558 共 4 处，从不抽 helper）；neo 从其风格，Picked 回调照搬 `openSelector` 同款（含 R7 `h.Buf==nil` 防御）。

### 6.1 birth selector 的 startDir：捕获机制（实施级）

⚠️ **本节及 §12 已退役**（被 v2 架构取代）：下面描述的 `pendingBirthDir`（包级全局）+ `onPaneBirthNeo`（挂 `finishInitialize`）方案，在 F4b 里已改为——目录作函数参数显式传递、在 spawn 包装内调完原生 spawn 之后开 selector、`finishInitialize` 零触碰。本节仅留作设计演进记录，**实施一律以 F4b 为准**。

**期望规则**：startDir = 出生那一刻「父 pane」当前文件的目录；父 pane 是 noName（空）→ cwd；startup 无父 pane → cwd。

「父 pane」= 触发这次诞生的 active pane。noName 诞生路径穷尽为 6 个入口（3 key + 3 command），全部从 active pane 发起，**没有其它可能**：

| 入口 | 原生映射 | file:line |
|---|---|---|
| `Ctrl-t` 键 | action `AddTab` → `AddTab()` | actions.go:1976 |
| `vsplit` 键 | action `VSplit` → `VSplitAction()` | actions.go:2026 |
| `hsplit` 键 | action `HSplit` → `HSplitAction()` | actions.go:2033 |
| `:tab` | command `tab` → `NewTabCmd()` | command.go:553 |
| `:vsplit` | command `vsplit` → `VSplitCmd()` | command.go:507 |
| `:hsplit` | command `hsplit` → `HSplitCmd()` | command.go:524 |

（`:vsplit foo` 带文件参数 → 开的是 file-born pane，非 noName，不触发 birth selector；只有无参的空 split/tab 才是 noName。`VSplitCmd`/`HSplitCmd` 无参时内部调 `VSplitAction`/`HSplitAction`，command.go:511/531。）

**难点**：selector 必须在 `finishInitialize`（bufpane.go:293）开（解决 0×0），但到那时新 pane 已是 active、buffer 为空——读 active pane 目录只得 cwd（自己）。所以父目录必须在 spawn **包装里、调原生之前**捕获。

**解法**：包级 `pendingBirthDir` + 覆盖上述 6 个 map 入口为 neo 包装；`onPaneBirthNeo` 在 `finishInitialize` 里读并清空。

**（1）状态 + 辅助**
```go
var pendingBirthDir string  // birth startDir；spawn 包装写，onPaneBirthNeo 读+清；""→cwd

func birthDir(h *BufPane) string {  // 父 pane 当前文件目录；空(noName)→""
    if p := h.Buf.AbsPath; p != "" {
        return filepath.Dir(p)
    }
    return ""
}
```

**（2）neo 包装（捕获 + 透传原生）**
```go
// action 包装（覆盖 BufKeyActions，cover Ctrl-t / vsplit键 / hsplit键）
func (h *BufPane) neoAddTabAction() bool  { pendingBirthDir = birthDir(h); return h.AddTab() }
func (h *BufPane) neoVSplitAction() bool  { pendingBirthDir = birthDir(h); return h.VSplitAction() }
func (h *BufPane) neoHSplitAction() bool  { pendingBirthDir = birthDir(h); return h.HSplitAction() }

// command 包装（覆盖 commands，cover :tab / :vsplit / :hsplit）
func (h *BufPane) neoNewTabCmd(args []string) { pendingBirthDir = birthDir(h); h.NewTabCmd(args) }
func (h *BufPane) neoVSplitCmd(args []string) { pendingBirthDir = birthDir(h); h.VSplitCmd(args) }
func (h *BufPane) neoHSplitCmd(args []string) { pendingBirthDir = birthDir(h); h.HSplitCmd(args) }
```

**（3）map 覆盖**（两表都是动态查找：键树存「key→action 名」，派发时才 `BufKeyActions[name]`（bufpane.go:138）；命令同理 `commands[cmd].action`（command.go:1220）。故覆盖 map 条目即生效。但**时序有坑**：`InitCommands()`（command.go:33）是**整表重赋值** `commands = map[...]{}`，会冲掉它之前的一切 insert。启动序 `InitBindings`(406)→`InitNeoBindings`(408)→`InitCommands`(409)→`InitNeoCommands`(410)，故 `commands["tab"/"vsplit"/"hsplit"]` ×3 **必须**放 `InitNeoCommands`(410，整表赋值之后)，逐条 insert（仿 `MakeCommand("quit",...)` 先例 command_neo.go:21）；`BufKeyActions` 是包级 `var`(bufpane.go:729)、永不被 clobber，×3 放 InitNeoBindings 或 InitNeoCommands 皆可。**为避免时序陷阱，建议 6 个覆盖统一放 `InitNeoCommands`。**）
```go
BufKeyActions["AddTab"]  = (*BufPane).neoAddTabAction   // bufpane.go:729，含 :832
BufKeyActions["VSplit"]  = (*BufPane).neoVSplitAction   // :840
BufKeyActions["HSplit"]  = (*BufPane).neoHSplitAction   // :841
commands["tab"]   = Command{(*BufPane).neoNewTabCmd, buffer.FileComplete}   // command.go:34，:53
commands["vsplit"] = Command{(*BufPane).neoVSplitCmd, buffer.FileComplete}  // :51
commands["hsplit"] = Command{(*BufPane).neoHSplitCmd, buffer.FileComplete}  // :52
```

**（4）消费**（`onPaneBirthNeo`，挂在 `finishInitialize`）
```go
func onPaneBirthNeo(h *BufPane) {
    if !isNoNameBuf(h.Buf) {
        return // info/raw/note/file-born：不介入，isNoName 保持零值 false，不碰 pendingBirthDir
    }
    h.isNoName = true
    d := pendingBirthDir
    pendingBirthDir = ""        // 仅 noName pane 消费，语义干净
    if d == "" {
        d, _ = os.Getwd()       // startup 无父 / 父为 noName → cwd
    }
    NewFileSelector().Open(h, d, func(r SelectResult) {
        if r.Kind != Picked {
            return // ReasonEsc→no-op，继续编辑当前 buffer
        }
        if h.Buf == nil { // R7 防御
            return
        }
        b, err := buffer.NewBufferFromFile(r.Path, buffer.BTDefault)
        if err != nil {
            InfoBar.Error(err)
            return
        }
        h.OpenBuffer(b)
    }, false)
}
```

**为什么不 unconditional 清 pendingBirthDir**：finishInitialize 被 InfoPane/RawPane 共用（走 `NewBufPane` 同步路径），无条件清会让「建消息栏」这种无关操作去碰 spawn 变量，逻辑不干净。提到 `isNoNameBuf` 门控后，非 buffer pane（info/raw/note）和 file-born pane 直接 bail、零副作用，只有真正 noName 的 buffer pane 才读清 pendingBirthDir。file-born 的 `isNoName` 靠零值 `false` 天然正确（`new(BufPane)` 即 false），无需显式赋。

**时序验证**
- split 同步：`VSplitAction`→`VSplitBuf`→`VSplitIndex`→`tab.Resize()`（同步）→ 新 pane `Resize`→`finishInitialize`，全在原生 spawn 调用栈内触发。包装在调原生**前**已写 `pendingBirthDir`，故读到。✓
- tab **也同步**（修正早期「异步」误判）：`AddTab`/`NewTabCmd` 自身不调 Resize，但它们调 `Tabs.AddTab`(tab.go:52)，而 `TabList.AddTab` **同步**调 `t.Resize()`(tab.go:54) → 遍历所有 tab（含新 tab）→ `tab.Resize`(tab.go:380) → 新 pane `Resize`（真尺寸：新 tab 的 Node 用 `screen.Size()` 建）→ `finishInitialize`。全在 `AddTab()` 返回前的同一调用栈内完成。`pendingBirthDir` 即写即读，**无被覆盖的异步窗口**（比早期设想更安全）。✓
  - 注：`TabList.Display()`(tab.go:145) **不**调 Resize（仅 UpdateNames + TabWindow.Display）；早期「下一帧 Display→Resize 触发 finishInitialize」的判断错误，已弃。
- startup：无包装 → `pendingBirthDir=""` → cwd。✓

**为什么 h = 父 pane**：key 动作派发时 h=active pane；command 经 `CommandAction`/`HandleCommand` 在 `MainTab().CurPane()`（tab.go:223）上执行，仍是 active pane。两者都在新 pane 创建之前，故 `birthDir(h)`=父目录。`buffer.AbsPath` 见 buffer.go:79。

---

## 7. 删除清单

`internal/action/welcome_md.go`：
- `WelcomeMode` 全局变量
- `DetectWelcomeMode`
- `EnterWelcome`（+ `cmd/micro/micro.go:487` 的调用）
- `gotoWelcome`（整体；「关最后文件回 welcome」不再存在）
- `openWelcomeSelector`
- `exitProgram`（改用 `h.Quit()`，见 §5）

`cmd/micro/micro.go`：
- 删 `action.DetectWelcomeMode(args)` 调用（若有）
- 删 `action.EnterWelcome()`（487 行）

---

## 8. 新增 / 改动清单

- **`BufPane.isNoName` 字段**：neo 字段。在 `finishInitialize` 里赋值。
- **`finishInitialize`（`bufpane.go:293`）加一行 neo 调用**，仿 `initMDConfig` 的挂法（`NewBufPaneFromBuf` 已有先例）：
  ```
  func (h *BufPane) finishInitialize() {
      h.initialRelocate()
      h.initialized = true
      onPaneBirthNeo(h)   // ← 新增 neo 调用
      ...onBufPaneOpen...
  }
  ```
  `onPaneBirthNeo(h)`（neo）：判 isNoName、定 startDir、若 noName 则内联 `NewFileSelector().Open(...)` 开 birth selector。见 §6.1。
  - 为什么挂在 `finishInitialize`：它是「pane 首次拿到真实几何（首个 Resize）」的**一次性**点（`bufpane.go:304` `Resize` 里 `!initialized` 才调）。birth selector 的 `computeLayout` 预检要真实 W/H，在这里开**不会** 0×0 静默失败——正是当年 `EnterWelcome` 要解决的时序问题，这次被挂点天然解掉。
- **selector 打开**：不新增包装函数。browse 复用现有 `openSelector`（command_neo.go:84，不改）；birth/quit 各自内联 `NewFileSelector().Open(...)`（见 §5/§6.1），Picked 回调照搬 `openSelector` 同款。
- **`QuitNeo` 重写**：见 §5。
- **`InitNeoBindings` 改动**：`Ctrl-q → QuitNeo` **始终绑定**（去掉 `if !WelcomeMode { return }` gate）。`QuitNeo` 在运行时按 `isNoName` 分流，file-born 走 `h.Quit()` 等价原生，无行为变化。
  - `F4 / F10` 重绑：旧代码在 welcome 下把它们也绑 `QuitNeo`。新模型暂**留原生**（属独立清理任务，非本任务阻塞）。
- **Ctrl-o → `:file`**：已存在（`defaults_*.go`），**无需新代码**。见 §3.3。
- **birth startDir 捕获（见 §6.1）**：包级 `pendingBirthDir` + `birthDir()`；6 个 neo 包装（3 action + 3 command）；`BufKeyActions` ×3 + `commands` ×3 覆盖（**`commands` ×3 必须放 `InitNeoCommands`，`InitCommands` 整表重赋值会 clobber 早于它的 insert；详见 §6.1(3)**）；`onPaneBirthNeo` 消费。

---

## 9. startDir 汇总

| selector | startDir |
|---|---|
| birth（isQuit=false） | 父 pane 文件目录，空→cwd（详见 §6.1） |
| quit（isQuit=true） | 当前 buffer 目录，空→cwd |
| Ctrl-o（isQuit=false） | 当前 buffer 目录，空→cwd（复用 FileCmd/openSelector） |

---

## 10. 存盘提示

- **file-born** pane Ctrl-q → 原生 `h.Quit()` 自带 Save 提示（micro 原生行为，不动）。
- **noName-born** pane Ctrl-q → 进 quit selector 前，若 `Modified() && !Shared()` → `closePrompt`（对齐现有 `FileCmd` 的 YNPrompt 风格）：`y`=保存→开 selector；`n`=不保存→开 selector；`esc`=取消（留在编辑）。
  - 注：`esc` 取消的是「进 selector」这一步；selector 内**无取消**（见 §4），符合「Ctrl-q = 我想结束这个 pane」的语义。

---

## 11. 边界与已知

- **退出路径统一**：关 pane 走 `h.Quit()`；原生 `ForceQuit`（actions.go:1908）在最后一个 pane/tab 时**直接** `runtime.Goexit()` 退程序（非「主循环检测无 pane」——原生早有此机制，neo 不介入）。无 `exitProgram`。
- **「编辑空 buffer」只在诞生 selector Esc 后可达一次**；之后 Ctrl-q 永远给 isQuit=true selector（无「回空 buffer」路径）。符合 §4 无取消语义。
- **pipe / 非交互启动**：不再有 `isatty` 判断。若启动 buffer 非空（如管道内容）→ `isNoName=false` → 不开 selector；若空 → 试开 selector，非真终端下 `computeLayout` 失败 → graceful degradation（不开，留空 buffer）。等价于旧的 `isatty` 效果。
- **依赖首个 resize 事件触发 `finishInitialize`**（与旧 `EnterWelcome` 同假设）：startup 的 noName pane 须等 micro.go:480-486 `select` 收到首个 `EventResize` → `TabList.Resize` → pane Resize → finishInitialize 才开 birth selector。真实终端总会首发 resize（tcell 启动即发），故实践不触发；极端超时（10ms 无事件）则 `isNoName` 保持零值 false、不开 selector、退化为空 buffer（Ctrl-q 直接退，安全无循环）。新设计比旧 `EnterWelcome` 略稳：finishInitialize 是单点门，resize 晚到也能补开。
- **「三级判断」塌成二级**：删了 `WelcomeMode` 后，不再有 `WelcomeMode × isLast` 三级，只剩 `isLast → 退程序 / !isLast → 关 pane`。
- **vsplit / hsplit / 新 tab 创建空 buffer**：新 pane 是 noName → `finishInitialize` → birth selector。**本次新需求（split/tab 自动开 selector）被这套规则自然覆盖，无需单独挂 4 个入口。**

---

## 12. 代码位置

| 文件 | 改动 |
|---|---|
| `internal/action/filemanager.go`（新） | microNeo 文件导航/管理模块（与 `fileselector.go` 配对：后者是选文件 UI 控件，前者是管理逻辑）。本任务承载：`isNoNameBuf`/`pendingBirthDir`/`birthDir`/`onPaneBirthNeo`（开 birth selector）、重写 `QuitNeo`（Ctrl-q 路由）、`InitNeoBindings`（Ctrl-q→QuitNeo 始终绑定）、6 个 neo spawn 包装**函数**（`neoAddTabAction` 等定义在此；注册在 `command_neo.go`，见下行）。后续删除/改名/新建等文件操作也归此文件。详见 §5/§6/§6.1 |
| `internal/action/welcome_md.go` | **删整文件**（`WelcomeMode`/`DetectWelcomeMode`/`EnterWelcome`/`gotoWelcome`/`openWelcomeSelector`/`exitProgram`/旧 `QuitNeo`/旧 `InitNeoBindings` 全部退役；符号已迁入 `filemanager.go`） |
| `internal/action/command_neo.go` | `InitNeoCommands` 末尾加 6 个 map 覆盖：`BufKeyActions["AddTab"/"VSplit"/"HSplit"]` ×3 + `commands["tab"/"vsplit"/"hsplit"]` ×3（**必须在此处**，`InitCommands` 整表重赋值会 clobber 早于它的 insert；详见 §6.1(3)） |
| `internal/action/bufpane.go` | `finishInitialize`（293）加一行 `onPaneBirthNeo(h)`；`BufPane` struct（206）加 `isNoName bool` 字段（带 `// microNeo:` 注释）。Go 不允许跨文件加 struct 字段，故「挂 neo 文件」不可行——这是本任务对原生 struct 的**唯一一行**数据字段侵入，符合最小侵入原则 |
| `internal/action/fileselector.go` | 字段 + `Open` 参数 `isWelcome` 改名 `isQuit`（`handleEvent` 的 Esc/Ctrl-q 分支结构不变，仅改标识符；birth 调用点实参从 true→false 是有意语义变更，见 §4） |
| `cmd/micro/micro.go` | 删 `DetectWelcomeMode` 调用 + `EnterWelcome()`（487） |

---

## 13. 验证清单

- `microNeo`（无参）→ noName pane → birth selector 弹出（isQuit=false）。
  - Esc → 编辑空 buffer；Ctrl-q → quit selector（isQuit=true）；selector 内 Ctrl-q → 关 pane → 退程序。
  - Enter 选文件 → 编辑该文件；此后 Ctrl-q → quit selector（混合态，仍按 noName 走）。
- `microNeo a.md` → file pane → 直接编辑；Ctrl-q → 关 pane → 退程序。
- **Ctrl-o（任意 pane，不论 isNoName）** → browse selector（isQuit=false）：Esc → 回当前文件；Enter → 换文件；isNoName 不变。
- `microNeo a.md` 编辑后 Ctrl-q → 原生存盘提示。
- vsplit / hsplit / Ctrl-t（新 tab）→ 新 pane 弹 birth selector。
- noName pane 选文件后改文件、Ctrl-q → 存盘提示 → quit selector。
- 窄 pane（宽<20 或高<10）→ `computeLayout` 失败 → 不开 selector、留空 buffer（不崩、不循环）。
