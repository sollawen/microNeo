# F4b · 实施方案（isNoName per-pane 模型）

对应设计：`F4a-isNoName模型.md`（final-reviewed 版）。
本文件把 F4a 翻译成**可逐字执行**的改动步骤，分 3 个阶段，每阶段独立编译、独立验证。

设计原则不变：对 micro 原生代码侵入越小越好；所有 neo 逻辑隔离到 `filemanager.go` / `command_neo.go`。

**核心架构决策（v2）**：不在 `Resize` / `finishInitialize` 里放任何 neo 逻辑——Resize 只管几何，职责保持纯净。birth selector 改在 **spawn 包装里、调完原生 spawn 之后**开（新 pane 此时已 active 且有几何）；启动场景在 micro.go 的首个 resize 事件之后显式开一次。目录作为参数显式传递，**不用包级全局**。

---

## 0. 目标

用 per-pane 的 `BufPane.isNoName`（诞生时赋值、终身不变）替换全局 `WelcomeMode`，统一「启动 / split / tab / 关文件」四条路径下的 selector 行为。详见 F4a。

---

## 1. 文件布局决策

`welcome_md.go` 整个围绕已退役的 welcome 概念，retire 后留着只会误导。决定：

- **删** `internal/action/welcome_md.go`（含 `WelcomeMode`/`DetectWelcomeMode`/`EnterWelcome`/`gotoWelcome`/`openWelcomeSelector`/`exitProgram`/旧 `QuitNeo`/旧 `InitNeoBindings`）。
- **新建** `internal/action/filemanager.go`，microNeo 的文件导航/管理模块。本任务承载：`isNoNameBuf`/`birthDir`/`OpenBirthSelector`（核心 helper）/新 `QuitNeo`/新 `InitNeoBindings`/6 个 spawn 包装；后续删除/改名/新建等操作也归此文件。与 `fileselector.go` 配对：后者是选文件 UI 控件，前者是文件管理逻辑。
- 原生 struct 的**唯一**一行侵入：`BufPane` 加 `isNoName bool` 字段（Go 不允许跨文件加 struct 字段，无法回避）。

**与 v1（finishInitialize 挂点）的关键差异**：
- `bufpane.go::finishInitialize` **完全不动**（v1 在这里加一行调用，v2 删掉）。
- 删掉包级全局 `pendingBirthDir`（v1 用它传目录，v2 改成函数参数显式传）。
- InfoPane/RawPane/NotePane **完全不进** neo 逻辑（v1 经 finishInitialize 进了再 bail，v2 它们不经 spawn 包装，压根不调 `OpenBirthSelector`）。

调用方（`micro.go::InitNeoBindings()` 等）按包级函数调用，文件搬移不影响。

---

## 2. 分阶段实施

### Phase 1 — fileselector 改名 `isWelcome`→`isQuit`（行为零变化）

把 selector 层的 welcome 命名先退役。**纯改名 + birth 语义翻转的铺垫，但 birth 调用点此刻还不存在**（要等 Phase 2），所以现存两个调用点（browse=false、welcome=true）改名后真值不变，行为完全一致。这一步单独成 commit，隔离对 `fileselector.go` 的触碰。

**文件：`internal/action/fileselector.go`**

1. `FileSelector` struct 字段 `isWelcome bool` → `isQuit bool`（含注释更新：true=quit 态「不收 Esc、收 Ctrl-q」，false=browse/birth 态「收 Esc、不收 Ctrl-q」）。
2. `Open` 签名参数 `isWelcome bool` → `isQuit bool`；函数体 `fs.isWelcome = isWelcome` → `fs.isQuit = isQuit`；doc 注释同步。
3. `handleEvent` 两处分支，**结构不变**，仅改字段名：
   - `case KeyEscape: if !fs.isWelcome {` → `if !fs.isQuit {`
   - `case KeyCtrlQ: if fs.isWelcome {` → `if fs.isQuit {`

**文件：`internal/action/welcome_md.go`**（过渡，Phase 2 整体删除）

4. `openWelcomeSelector` 里 `fs.Open(..., true)` 实参不变（仍是 `true`，语义 = welcome 态 Esc no-op / Ctrl-q 退出，与旧一致）。

**验证（Phase 1）**
- `make build`。
- `microNeo a.md` → Ctrl-o 开 browse selector：Esc 关、Ctrl-q 吞。行为同改前。
- `microNeo`（无参）→ welcome selector（旧路径仍走 EnterWelcome）：Esc no-op、Ctrl-q 退程序。行为同改前。
- 无新增行为，确认纯改名。

---

### Phase 2 — isNoName 模型核心（本任务主体）

#### 2.1 原生 struct 加字段：`internal/action/bufpane.go`

`BufPane` struct 末尾（`initialized bool` 之后）加一行：

```go
	// The pane may not yet be fully initialized after its creation
	// since we may not know the window geometry yet. In such case we finish
	// its initialization a bit later, after the initial resize.
	initialized bool

	// microNeo: 本 pane 诞生时是否 noName（空 buffer）。诞生时赋值一次、终身不变。
	// 决定 Ctrl-q 路由与 birth selector 行为（见 F4a §2）。
	isNoName bool
}
```

**注**：v1 曾在 `finishInitialize` 里加一行调用——v2 **删掉这个挂点**，`finishInitialize` 保持原生原样、零 neo 代码。

#### 2.2 新建核心文件：`internal/action/filemanager.go`

```go
package action

import (
	"os"
	"path/filepath"

	"github.com/micro-editor/micro/v2/internal/buffer"
)

// isNoNameBuf 判定 buffer 是否「noName」：无路径 + 默认类型 + 空内容。
// OpenBirthSelector 用它作门控：true 才置 pane.isNoName=true 并开 birth selector；
// false（file-born / info / raw / note / 管道内容）直接 bail，不介入。
//   - NewBufferFromString("", "", BTDefault) → true
//   - 文件 buffer（AbsPath!=""）→ false
//   - 管道内容（Size>0）→ false（等价旧 isatty 效果，F4a §11）
//   - Help/Log/Raw/Scratch/Info（Type!=BTDefault）→ false
//   - Size()==0 精确语义：buffer.go:726 Size() 逐行字节求和、末行不加分隔符，
//     故 NewBufferFromString("","") 的单行空 buffer Size==0（满足）；用户敲过字
//     （含回车产生第二行）的空 buffer Size≥1（不满足→非 noName）。勿改成 len 判空。
func isNoNameBuf(buf *buffer.Buffer) bool {
	if buf == nil {
		return false
	}
	return buf.AbsPath == "" && buf.Type == buffer.BTDefault && buf.Size() == 0
}

// birthDir 返回 pane 当前 buffer 所在目录；空（noName / 无 buf）→ ""（调用方回退 cwd）。
func birthDir(h *BufPane) string {
	if h.Buf == nil {
		return ""
	}
	if p := h.Buf.AbsPath; p != "" {
		return filepath.Dir(p)
	}
	return ""
}

// OpenBirthSelector 对刚诞生的 pane 开 birth selector（若它是 noName）。
// 调用方：6 个 spawn 包装（包内）+ micro.go 启动段（跨包，故导出大写）。
//   - dir = 父 pane 目录（spawn 包装传入）或 ""（启动 → 回退 cwd）。
//   - noName pane → 置 isNoName=true（一次性、终身不变）+ 开 birth selector（isQuit=false）。
//   - file-born pane（如 :vsplit foo）→ 直接 return，isNoName 保持零值 false，不开。
//
// 为什么能在 spawn 之后立刻开：三种 spawn（VSplitIndex/HSplitIndex/AddTab 及对应 *Cmd）
// 末尾都同步调 SetActive + Resize，返回时新 pane 已是 active（MainTab().CurPane() 即它）、
// 且 BWindow 已有真实几何（computeLayout 预检不会 0×0 误判）。详见 F4a §6.1 时序验证。
func OpenBirthSelector(pane *BufPane, dir string) {
	if !isNoNameBuf(pane.Buf) {
		return
	}
	pane.isNoName = true
	if dir == "" {
		dir, _ = os.Getwd()
	}
	NewFileSelector().Open(pane, dir, func(r SelectResult) {
		if r.Kind != Picked {
			return // ReasonEsc → no-op，继续编辑当前（空）buffer
		}
		if pane.Buf == nil { // R7 防御：pane 在打开期间被关
			return
		}
		b, err := buffer.NewBufferFromFile(r.Path, buffer.BTDefault)
		if err != nil {
			InfoBar.Error(err)
			return
		}
		pane.OpenBuffer(b)
	}, false) // isQuit=false：birth 态 Esc 可关（→继续编辑空 buffer），Ctrl-q 不收
}

// QuitNeo 是 microNeo 的 Ctrl-q / :quit 路由（重写，替换 welcome_md.go 旧版）。
//   - file-born pane（isNoName=false）→ 直接 h.Quit()（原生自带存盘提示；最后→退程序）。
//   - noName-born pane → 开 quit selector（isQuit=true）：
//       Enter 选文件 → 原地换入；selector 内 Ctrl-q（ReasonQuit）→ h.Quit()。
func (h *BufPane) QuitNeo() bool {
	if !h.isNoName {
		return h.Quit() // file-born：完全等价原生 Quit，零行为变化
	}
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
				return
			}
			// Closed
			if r.Reason == ReasonQuit {
				h.Quit() // → ForceQuit：非最后 pane 关本 pane；最后一个 runtime.Goexit 退程序
			}
			// ReasonResize：透明（FloatFrame 已关，不重开 quit selector）
		}, true) // isQuit=true：不收 Esc，只收 Ctrl-q
	}
	if h.Buf.Modified() && !h.Buf.Shared() {
		h.closePrompt("Close", proceed) // y/n → proceed（开 selector）；esc → 取消留编辑
		return true
	}
	proceed()
	return true
}

// InitNeoBindings 注册 microNeo 的键位覆盖。
// 必须在 action.InitBindings() 之后调用（micro.go:408）。
// Ctrl-q 始终绑 QuitNeo：运行时按 pane.isNoName 分流，file-born 等价原生 Quit。
func InitNeoBindings() {
	BufKeyActions["QuitNeo"] = (*BufPane).QuitNeo
	BindKey("Ctrl-q", "QuitNeo", Binder["buffer"])
	// F4 / F10 留原生（整体移除 F 键默认绑定属独立清理任务，非本任务）。
}

// ---- spawn 包装：捕获父目录 → 调原生 spawn → 对新 pane 开 birth selector ----
// 覆盖 split/tab 的 3 个 key action 与 3 个 command（见 command_neo.go::InitNeoCommands）。
// 关键时序事实：三种 spawn 末尾都同步 SetActive+Resize，返回时新 pane = MainTab().CurPane()、
// 已有真实几何，故 OpenBirthSelector 可立即开（不在 Resize 里搞，Resize 保持纯净）。
// 带文件参数的 :vsplit foo / :tab foo 开 file-born pane（isNoNameBuf=false），OpenBirthSelector 直接 bail。

func (h *BufPane) neoAddTabAction() bool {
	dir := birthDir(h)
	r := h.AddTab()
	OpenBirthSelector(MainTab().CurPane(), dir)
	return r
}
func (h *BufPane) neoVSplitAction() bool {
	dir := birthDir(h)
	r := h.VSplitAction()
	OpenBirthSelector(MainTab().CurPane(), dir)
	return r
}
func (h *BufPane) neoHSplitAction() bool {
	dir := birthDir(h)
	r := h.HSplitAction()
	OpenBirthSelector(MainTab().CurPane(), dir)
	return r
}

func (h *BufPane) neoNewTabCmd(args []string) {
	dir := birthDir(h)
	h.NewTabCmd(args)
	OpenBirthSelector(MainTab().CurPane(), dir)
}
func (h *BufPane) neoVSplitCmd(args []string) {
	dir := birthDir(h)
	h.VSplitCmd(args)
	OpenBirthSelector(MainTab().CurPane(), dir)
}
func (h *BufPane) neoHSplitCmd(args []string) {
	dir := birthDir(h)
	h.HSplitCmd(args)
	OpenBirthSelector(MainTab().CurPane(), dir)
}
```

#### 2.3 spawn 包装注册：`internal/action/command_neo.go`::`InitNeoCommands`

在 `InitNeoCommands` 末尾追加 6 个 map 覆盖。**必须在此处**：`InitCommands()`（command.go:33）整表重赋值会 clobber 早于它的 insert；启动序 `InitCommands`(409) → `InitNeoCommands`(410)，放这里才安全（F4a §6.1(3)）。

```go
func InitNeoCommands() {
	MakeCommand("theme", (*BufPane).ThemeCmd, nil)
	MakeCommand("file", (*BufPane).FileCmd, nil)
	MakeCommand("quit", (*BufPane).QuitNeoCmd, nil) // QuitNeoCmd 已存在，包 QuitNeo，不改

	// microNeo: spawn 包装覆盖（捕获父目录 + 开 birth selector）。
	// BufKeyActions 是包级 var（永不被 clobber）；commands 须在此处覆盖（InitCommands 整表重赋值之后）。
	BufKeyActions["AddTab"] = (*BufPane).neoAddTabAction
	BufKeyActions["VSplit"] = (*BufPane).neoVSplitAction
	BufKeyActions["HSplit"] = (*BufPane).neoHSplitAction
	commands["tab"]   = Command{(*BufPane).neoNewTabCmd, buffer.FileComplete}
	commands["vsplit"] = Command{(*BufPane).neoVSplitCmd, buffer.FileComplete}
	commands["hsplit"] = Command{(*BufPane).neoHSplitCmd, buffer.FileComplete}
}
```

`QuitNeoCmd`（command_neo.go，`:quit` 命令的 action）已存在、包 `QuitNeo()`，重写后自动生效，**不改**。

#### 2.4 删 `internal/action/welcome_md.go`（整文件）

整文件删除。其承载的符号去向：
- `WelcomeMode`/`DetectWelcomeMode`/`EnterWelcome`/`gotoWelcome`/`openWelcomeSelector`/`exitProgram`/旧 `QuitNeo`/旧 `InitNeoBindings` → **全部删除**（isNoName 模型取代）。
- 新 `QuitNeo`/新 `InitNeoBindings`/`OpenBirthSelector` → 已在 `filemanager.go`（2.2）。

#### 2.5 `cmd/micro/micro.go` 去 welcome 触点

| 行 | 现状 | 改动 |
|---|---|---|
| 407 | `action.DetectWelcomeMode(flag.Args()) // neo：…` | **删** |
| 408 | `action.InitNeoBindings() // neo：按 WelcomeMode…` | **留**（注释更新为「Ctrl-q→QuitNeo 始终绑定」） |
| 485-487 | `// neo：…` + `if action.WelcomeMode {` + `action.EnterWelcome()` + `}` | **替换**为 `action.OpenBirthSelector(action.MainTab().CurPane(), "")`（注释：启动 pane 若 noName 则开 birth selector；此时 resize 事件已处理、启动 pane 已有几何） |

替换后启动段：

```go
	// wait for initial resize event
	select {
	case event := <-screen.Events:
		action.Tabs.HandleEvent(event)
	case <-time.After(10 * time.Millisecond):
	}

	// microNeo: 启动 pane 若 noName 则开 birth selector
	// （resize 事件已在上面的 HandleEvent 里同步填好真实几何，此时开不会 0×0）
	action.OpenBirthSelector(action.MainTab().CurPane(), "")

	for {
		DoEvent()
	}
```

**验证（Phase 2）** — 见 §4 完整清单。关键冒烟：
- `make build`。
- `microNeo`（无参）→ birth selector 自动弹出（cwd）；Esc → 空 buffer；Ctrl-q → quit selector；selector 内 Ctrl-q → 退程序。
- `microNeo a.md` → 直接编辑；Ctrl-q → 原生存盘提示（若 modified）→ 退程序。**file-born 零变化**。
- Ctrl-o（任意 pane）→ browse selector：Esc 回当前文件。
- vsplit/hsplit/Ctrl-t → 新 pane 自动弹 birth selector，起始目录 = 父 pane 文件目录。

---

### Phase 3 — 收尾与文档

#### 3.1 修正 stale help：`runtime/help/keybindings.md:566`

```
"Ctrl-o":         "OpenFile",
```
→ 改为 `command:file`（实际绑定，F4a §3.3 注）。非阻塞，顺手修。

#### 3.2 跑 §4 完整验证清单

逐条手测；全绿即收。

#### 3.3 CHANGELOG

按 `.agents/skills/changelog` 记一条：移除 WelcomeMode、引入 isNoName per-pane 模型、split/tab 自动开 selector。

---

## 3. 改动清单总表

| # | 文件 | 改动 | 侵入度 |
|---|---|---|---|
| 1 | `internal/action/bufpane.go` | struct 加 `isNoName bool`（1 行+注释）。**finishInitialize 不动** | 极小（唯一原生 struct 字段） |
| 2 | `internal/action/filemanager.go`（**新**） | 文件导航/管理模块：`isNoNameBuf`/`birthDir`/`OpenBirthSelector`/新 `QuitNeo`/`InitNeoBindings`/6 spawn 包装 | 新文件，零原生侵入 |
| 3 | `internal/action/fileselector.go` | `isWelcome`→`isQuit` 改名（字段+Open 参数+handleEvent 两处分支名），逻辑结构不变 | neo 自己的文件 |
| 4 | `internal/action/command_neo.go` | `InitNeoCommands` 末尾追加 6 个 map 覆盖 | neo 自己的文件 |
| 5 | `internal/action/welcome_md.go` | **整文件删除** | 删 neo 文件 |
| 6 | `cmd/micro/micro.go` | 删 `DetectWelcomeMode`(407)；`EnterWelcome`(487) 换成 `OpenBirthSelector(MainTab().CurPane(), "")`；InitNeoBindings 注释更新 | 极小（删/换 neo 调用） |
| 7 | `runtime/help/keybindings.md` | Ctrl-o 绑定 stale 修正（可选） | 文档 |

原生代码唯一触碰：`bufpane.go` 的 1 个 struct 字段，`micro.go` 删 1 行 + 换 1 行 neo 调用。**`finishInitialize` / `Resize` 零触碰**。其余全在 neo 文件内。

---

## 4. 验证清单（= F4a §13，附操作）

**A. 启动 noName pane**
- [ ] `microNeo`（无参）→ birth selector 弹出（isQuit=false），起始目录 = cwd。
- [ ] birth selector 按 Esc → 编辑空 buffer；再 Ctrl-q → quit selector（isQuit=true）；selector 内 Ctrl-q → 关 pane → 退程序。
- [ ] birth selector 按 Enter 选文件 → 编辑该文件；此后 Ctrl-q → quit selector（混合态，isNoName 仍 true，按 noName 走）。

**B. 启动 file pane（零变化基线）**
- [ ] `microNeo a.md` → 直接编辑，无 selector。
- [ ] Ctrl-q（未改）→ 关 pane → 退程序。
- [ ] 改文件后 Ctrl-q → 原生存盘提示（y/n/esc）。

**C. Ctrl-o（browse，两种 pane 通用）**
- [ ] 任意 pane Ctrl-o → browse selector（isQuit=false）：Esc → 回当前文件；Enter → 原地换文件。
- [ ] isNoName 不变（sticky）：noName pane 上 Ctrl-o 选文件后，Ctrl-q 仍走 quit selector。

**D. split / tab 自动开 birth selector（本次新需求核心）**
- [ ] `microNeo a.md` 编辑中 vsplit → 新 pane 弹 birth selector，起始目录 = `a.md` 所在目录。
- [ ] hsplit → 同上。
- [ ] Ctrl-t（新 tab）→ 新 tab 的 pane 弹 birth selector。
- [ ] `:vsplit` / `:hsplit` / `:tab`（无参）→ 同上。
- [ ] `:vsplit foo.md`（带文件）→ 新 pane 直接编辑 foo.md，**不弹** selector（file-born）。

**E. noName pane 存盘提示**
- [ ] noName pane 选文件后改文件、Ctrl-q → 存盘提示（y/n/esc）→ quit selector。

**F. 优雅降级**
- [ ] 窄 pane（宽 < 20 或高 < 10）开 selector → `computeLayout` 失败 → 不开、留空 buffer，不崩、不循环。

**G. 回归（原生行为未动）**
- [ ] `:quit` 命令 = Ctrl-q（QuitNeoCmd 包 QuitNeo，file-born→原生 Quit）。
- [ ] F4 / F10 仍原生 Quit（未重绑）。
- [ ] **用户拖终端 resize** → 不触发任何 selector / neo 逻辑（finishInitialize/Resize 未被触碰）。

---

## 5. 风险与回退

| 风险 | 说明 | 应对 |
|---|---|---|
| **selector 内 ReasonQuit → `h.Quit()` 不能干净退出** | 旧路径用 `exitProgram()`（screen.Fini+Goexit），新路径靠原生 `ForceQuit`→Goexit。最后一个 pane 时应等价，但 selector 内调用栈不同 | F4a §5 已预留：若实测不能干净退出，回退该分支到 `exitProgram()`（保留 `screen`/`runtime` import）。验证 A 项「quit selector 内 Ctrl-q → 退程序」重点测 |
| **`MainTab().CurPane()` 不等于新 pane** | 6 个包装都靠「spawn 后新 pane 是 active」这个假设取 pane。已核实三条 spawn 末尾都 SetActive，但若某条命令路径（如带多文件参数的 `:vsplit a b c`）末态异常 | 已验证：VSplitIndex/HSplitIndex/AddTab/NewTabCmd 末尾都 SetActive。多文件参数创建 file-born pane（OpenBirthSelector bail），不影响。若发现新 pane 没开 selector，第一查 `MainTab().CurPane()` 是否真是新 pane |
| **`isatty` 退役** | 旧 `DetectWelcomeMode` 用 isatty 挡管道；新靠 `isNoNameBuf` 的 `Size()==0` | 管道内容 buffer Size>0 → isNoName=false，等价效果。F 项验证 |
| **回退** | 若整体需回退 | git revert 本任务 commit；welcome_md.go 可从历史恢复。原生侵入仅 bufpane.go 1 行 + micro.go 删/换 2 行，回退成本极低 |

> v1 曾有的「startup 在 resize 事件处理栈内开 FloatFrame」风险，v2 **已消除**——birth selector 现在在 spawn 包装（按键事件处理）或 micro.go 启动段（resize 事件之后）开，都不在 Resize 调用栈内。这是 v2 架构的核心收益。

---

## 6. 复用纪律自检

- `OpenBirthSelector`：birth/启动/（quit 复用同款 `NewFileSelector().Open`）三场景的共用入口；Picked 回调照搬现有 `openSelector`（command_neo.go:84）同款（含 R7 `h.Buf==nil` 防御），**不新增包装函数**。
- `QuitNeo` 的存盘提示：**复用** 原生 `closePrompt`（actions.go:1916，签名 `closePrompt(action string, callback func())`），对齐 `FileCmd` 风格。
- `:quit` / `QuitNeoCmd`：**复用** 已有 command_neo.go，不改。
- `Ctrl-o`→`:file`：**已存在**（defaults_*.go:39/42），零新代码。
- 退出路径：**复用** 原生 `h.Quit()`→`ForceQuit`→`Goexit`，删自写的 `exitProgram`。
- spawn 包装：仿现有 `BufKeyActions["QuitNeo"]` 先例（map 覆盖）与 `MakeCommand`（insert 机制），不新建抽象。
- 目录传递：v1 用包级全局 `pendingBirthDir`，v2 改成函数参数（`birthDir(h)` → `OpenBirthSelector(pane, dir)`），**删掉一个全局**。

新增的只有 `filemanager.go` 一个文件；其内全是薄 helper + 薄包装 + 一个薄重写（QuitNeo），无新抽象、无新组件、无包级全局。
