# D18 · `:check-aibp` 命令的执行过程反馈

> **状态**：方案已定，待实施。基于 D17 已落地的 `:check-aibp` 命令，补上执行过程中的实时 InfoBar 反馈。

---

## 一、目标

用户运行 `:check-aibp` 后，在命令执行的**各个阶段**实时看到 InfoBar 反馈，而不是只在命令结束才看到一行结果。当前实现把 `Ensure()` 当黑盒阻塞调用，`pi install` 阻塞几秒期间屏幕冻结、无任何提示，用户体验像"卡死了"。

目标反馈序列（用户需求原文）：

| 时机 | InfoBar 显示 |
|------|-------------|
| 命令一进入 | `finding ai-agents...` |
| 检测到 pi 存在 | `pi found.` |
| 安装完成（或已就绪） | `AIBP(v1.0.1) installed. pi is ready.` |

其中 `1.0.1` 是 **npm 包版本**（package.json 的 `version` 字段），**不是**协议版本（`aibp.protocol="aibp-1"`）。两者是正交维度（见 D17 §3.2、说明-AIBP §7.1.1）——包版本给用户看（"我装的是哪个版本"），协议版本给程序判兼容性用。

**不做的事情**：异步化命令执行、进度条/百分比、安装过程中的 npm 子步骤透出（pi install 内部跑 npm install 的细节不暴露，保持黑盒）。

---

## 二、问题根因分析（关键）

先回答一个被反复问到的问题：**是不是 micro 的基础框架不支持中途刷屏？**

**不是。框架完全支持，是当前实现的两处"懒"叠加导致的。**

### 2.1 `InfoBar.Message()` 是惰性的

`internal/info/infobuffer.go:80-88`：

```go
func (i *InfoBuf) Message(msg ...any) {
	if !i.HasPrompt {
		displayMessage := fmt.Sprint(msg...)
		i.Msg = displayMessage          // ← 只改字段
		i.HasMessage, i.HasError = true, false
	}
}
```

它**只设置 `Msg` 字段，不重绘**。真正的绘制发生在别处。

### 2.2 真正的重绘在主循环顶部

`cmd/micro/micro.go:491-505`（`DoEvent`）：

```go
// Display everything
screen.Screen.Fill(' ', config.DefStyle)
screen.Screen.HideCursor()
action.Tabs.Display()
for _, ep := range action.MainTab().Panes {
	ep.Display()
}
action.MainTab().Display()
action.InfoBar.Display()        // ← InfoBar 在这里才画
...
screen.Screen.Show()            // ← tcell 刷屏（只发脏行，无闪烁）
// Check for new events
select {
case event = <-screen.Events:
...
}
```

即"先画完一帧 → `Show()` 刷屏 → 阻塞等事件"。一帧 = 一个 `DoEvent`。

### 2.3 命令处理跑在哪、为什么中途 `Message()` 看不见

命令处理在 `select` 拿到事件**之后**的同一轮 `DoEvent` 里执行（命令 bar 的回车事件 → `ExecuteCommand` → `CheckAibpCmd`）。此时本帧的 `Display` + `Show()` 已经过去了。`CheckAibpCmd` 同步阻塞调用 `ensure_agents.Ensure()`，期间：

- `pi install` 阻塞几秒 → 整个主 goroutine 卡住 → 没有任何 `DoEvent` 在跑 → 屏幕冻结在命令执行前的画面。
- 在 `Ensure()` 中途调 `InfoBar.Message(...)` 只是改字段，等命令返回、下一轮 `DoEvent` 才画出来。
- 当前 `CheckAibpCmd` 只在**结尾** `Message` 一次，所以等待那几秒里用户什么反馈都看不到。

### 2.4 框架其实暴露了手动重绘能力

`DoEvent` 自己就示范了怎么做：**调对应 pane 的 `Display()` + `screen.Screen.Show()`**。在命令处理中途手动调这两个，就能立刻把 InfoBar 的新消息画到屏幕上：

```go
InfoBar.Display()        // InfoWindow.Display()：清 InfoBar 那行 + 画 i.Msg
screen.Screen.Show()     // tcell 刷屏（增量，无闪烁）
```

**线程安全**：tcell 的事件轮询（`PollEvent`）在另一线程，但**画图只在主 goroutine**。`DoEvent` 本身不加锁画图（`screen.Lock/Unlock` 是给 poll 线程 + 主线程关闭 screen 时的临界区用的，不锁画图）。命令处理跑在主 goroutine（由主循环的 `select` 分发），所以中途手动重绘和主循环的画图是**同一个 goroutine，天然串行，无竞态**。micro 自己的 `DoEvent` 也没给画图加锁，印证了这一点。

> **结论**：不需要改 micro 的主循环、不需要异步化、不需要新线程。只要在命令处理中途，在每个里程碑之后手动 `InfoBar.Display()` + `screen.Screen.Show()`。

---

## 三、方案选型

**手动重绘 + 进度回调**：`Ensure()` 接受一个 `progress func(string)` 回调，在每个里程碑调它；回调由 `CheckAibpCmd` 提供，内部做 `Message + Display + Show`。

**为什么用回调而非直接在 `Ensure()` 里写刷屏代码**：

- D17 §五 定的 `Ensure()` 契约是"纯逻辑、不依赖 action 包、错误返回给调用方处理交互"。刷屏依赖 `internal/action`（InfoBar）和 `internal/screen`，写进 `Ensure()` 就破坏了这个边界，让 `ensure_agents` 包反向依赖 `action`，形成循环依赖风险（`action` → `ensure_agents` → `action`）。
- 回调把这个依赖反转：`Ensure()` 只声明"我到了某个阶段"，怎么呈现由调用方决定。未来 `Ensure()` 被别处调用（测试、CLI 工具）时可以传 `nil` 或日志回调，不被 InfoBar 绑死。

**为什么不用异步（goroutine + channel）**：

- D17 D4 已定"同步执行"：用户主动运行命令，等几秒合理。异步化引入主循环竞态（goroutine 改 InfoBar 字段时主循环也在画）、生命周期管理（命令结束后 goroutine 还在跑怎么办）、退出时序问题。收益（不卡 UI）用手动重绘就拿到了，异步的复杂度不值得。
- 关键洞察：用户卡的不是"CPU 在算"，是"pi install 在 spawn 子进程等网络"。手动重绘让等待期间有反馈，体感就够了。异步不能让 `pi install` 更快完成。

**版本来源用包版本而非协议版本**（见 §一）：用户要的 "v1.0.1" 是 npm 包版本。需新增一个读 package.json `version` 字段的方法。

---

## 四、系统设计

### 4.1 模块划分与改动面

只动 D17 已建的三个 microNeo 文件，不碰任何 micro 原生文件：

| 文件 | 状态 | 改动 |
|------|------|------|
| `internal/aibp/ensure_agents/ensure.go` | 改动 | `Ensure` 签名加 `progress func(string)` 参数；各里程碑调 `progress(...)` |
| `internal/aibp/ensure_agents/ensure_pi.go` | 改动 | 新增 `AIBPPackageVersion() (string, error)` 方法实现（读 package.json `version`） |
| `internal/aibp/ensure_agents/ensure_pi_test.go` | 改动 | 给 `AIBPPackageVersion` 补单测 |
| `internal/action/command_neo.go` | 改动 | `CheckAibpCmd` 提供 progress 回调（`Message + Display + Show`）；成功消息带包版本 |

**对 micro 原生代码的侵入**：零。`internal/screen`、`internal/info`、`cmd/micro` 都只读不改。

### 4.2 进度回调契约

`progress func(string)` 由调用方传入，`Ensure()` 在里程碑调用它。契约：

- `progress` 可为 `nil`（测试/CLI 调用时），`Ensure()` 内部 nil-guard。
- `progress` 只负责"呈现当前阶段"，**不**影响 `Ensure()` 的返回值/控制流——`Ensure()` 的成功/失败判定逻辑不变。
- `progress` 的调用时机是**确定性的**（不依赖耗时），每个里程碑只调一次。

### 4.3 里程碑序列（对齐用户三条要求）

`Ensure()` 内部的里程碑调用点（在现有编排逻辑 §五 里插入）：

| # | 调用时机（Ensure 内部） | progress 消息 | 对应用户要求 |
|---|----------------------|--------------|-------------|
| M1 | `Ensure()` 一进入 | `finding ai-agents...` | 命令一进入 |
| M2 | `HasAgent()` 返回 true 之后 | `pi found.` | 找到 pi |
| M3 | `HasAIBP()` 返回 false、即将 `InstallAIBP()` | `installing aibp-pi...` | （新增的中间 UX，非用户要求但填补等待空白） |
| M4 | `Ensure()` 返回 `nil`（成功）后，由 **`CheckAibpCmd`** 调用 | `AIBP(v%s) installed. pi is ready.` | 安装完成 |

**为什么 M4 在 `CheckAibpCmd` 而非 `Ensure` 里调**：M4 消息需要包版本，而包版本读取（`AIBPPackageVersion`）是"成功后的展示性动作"，不属于 `Ensure()` 的"检查+安装"职责。`Ensure()` 返回 nil 后，`CheckAibpCmd` 再读包版本拼成功消息。这也避免 `Ensure()` 返回值既要表达成功又要携带版本字符串的别扭。

**fast path（已装且兼容）的消息处理**：M3 跳过（不进 install 分支），M4 仍显示 `AIBP(v1.0.1) installed. pi is ready.`。

> **措辞待定点（D5，见 §五）**：fast path 没有真实"安装"动作，显示 "installed" 语义略不准。两个选项：(a) 统一显示 "installed"（简单，用户感知一致）；(b) 区分 install 路径显示 "installed"、fast path 显示 "ready"。本方案默认 (a)，理由是用户感知目标是"能用了"，措辞统一更省心。

### 4.4 错误路径

错误（未装 pi / `pi install` 失败 / 版本不兼容 / package.json 损坏重装失败）走原有收尾：`InfoBar.Message("aibp-pi: " + err.Error())`。这些错误**不**经过 progress 里程碑的展示性消息，因为：

- 错误是 `Ensure()` 的返回值，由 `CheckAibpCmd` 统一收尾。
- 错误前的进度里程碑（如 M2 `pi found.` 后 M3 失败）已经实时显示过了，用户能看到"卡在哪一步"。

---

## 五、设计决策

| # | 决策 | 理由 |
|---|------|------|
| D1 | **用手动重绘（`Display + Show`）实现过程反馈**，不改主循环、不异步化 | §二证明框架支持、线程安全；异步复杂度不值得（D17 D4 已定同步） |
| D2 | **用 `progress func(string)` 回调注入呈现**，不把刷屏写进 `Ensure()` | 保持 `Ensure()` 纯逻辑、不依赖 action/screen；避免循环依赖；可测试 |
| D3 | **`progress` 可为 nil**，`Ensure()` 内部 nil-guard | 测试/CLI 调用 `Ensure()` 时不需要 InfoBar 反馈 |
| D4 | **包版本（npm `version`）用于展示，协议版本（`aibp.protocol`）用于兼容性判定**，两者正交、各读各的 | 对齐 D17 D8、说明-AIBP §7.1.1；用户看 "v1.0.1"、程序判 "aibp-1"，互不混淆 |
| D5 | **fast path 也显示 "installed" 措辞**（方案 a，统一） | 用户感知目标是"能用了"；区分方案 (b) 作为 open question 留待用户反馈 |
| D6 | **成功消息（M4）由 `CheckAibpCmd` 拼装，不在 `Ensure` 内调 progress** | 包版本读取是展示性动作，非 `Ensure` 职责；避免 `Ensure` 返回值携带版本字符串 |
| D7 | **M3 "installing aibp-pi..." 填补 install 等待空白**，即使非用户明确要求 | 几秒网络等待无反馈是体验黑洞；一行消息成本极低 |
| D8 | **错误路径不经 progress 里程碑**，统一由 `CheckAibpCmd` 用 `Message(err)` 收尾 | 错误是返回值不是阶段；错误前的里程碑已实时显示，用户能看到断点 |

---

## 六、接口契约调整

D17 §五 定义的 `AgentEnsurer` 接口有 5 个方法（`AgentName/HasAgent/HasAIBP/AIBPVersion/InstallAIBP`）。本方案**新增 1 个方法**，变成 6 个：

```go
type AgentEnsurer interface {
	AgentName() string
	HasAgent() bool
	HasAIBP() bool
	AIBPVersion() (string, error)              // 协议版本（兼容性判定）
	AIBPPackageVersion() (string, error)        // ⭐ 新增：包版本（展示用）
	InstallAIBP() error
}
```

**新增方法语义**：`AIBPPackageVersion()` 读已装扩展 package.json 的 `version` 字段（npm 包版本，如 `"1.0.1"`）。读不到（文件缺失/损坏）→ 返回 error，调用方降级显示（见 §七）。

**对 D17 契约的影响**：

- 这是 D17 §五 接口契约的**首次扩展**。由于 D17 的 `Ensure()` 还没有第二个实现（opencode/claude 都是"未来"），现在扩展零迁移成本。
- `AIBPPackageVersion` 与 `AIBPVersion` 读同一个 package.json 文件，只是字段不同（`version` vs `aibp.protocol`）。各 agent 实现里可复用文件读取逻辑。
- 未来 agent 若没有"包版本"概念（如纯配置文件式扩展），可实现为返回固定字符串或 `""`，接口不强制语义。

---

## 七、实现规格

### 7.1 `ensure.go` —— `Ensure` 加 progress 参数 + 里程碑

```go
package ensure_agents

import (
	"errors"
	"fmt"

	"github.com/micro-editor/micro/v2/internal/aibp"
)

var (
	errExtensionOutdated = errors.New("aibp 扩展协议版本过旧，请运行 `pi update npm:aibp-pi` 后重启 pi")
	errMicroNeoOutdated  = errors.New("aibp 扩展协议版本较新，请升级 microNeo")
)

type AgentEnsurer interface {
	AgentName() string
	HasAgent() bool
	HasAIBP() bool
	AIBPVersion() (string, error)
	AIBPPackageVersion() (string, error) // D18：包版本（展示用），与协议版本正交
	InstallAIBP() error
}

// notify 在 progress 非 nil 时调用它。抽出来避免每个里程碑都写 if。
func notify(progress func(string), msg string) {
	if progress != nil {
		progress(msg)
	}
}

// Ensure 是统一编排逻辑。progress 可为 nil（测试/CLI）。
// 返回的 error 由调用方决定如何交互（InfoBar 提示等）——本函数不预设交互模式。
func Ensure(e AgentEnsurer, progress func(string)) error {
	notify(progress, "finding ai-agents...") // M1

	if !e.HasAgent() {
		return fmt.Errorf("未检测到 %s，请先安装", e.AgentName())
	}
	notify(progress, fmt.Sprintf("%s found.", e.AgentName())) // M2（"pi found."）

	if !e.HasAIBP() {
		notify(progress, "installing aibp-pi...") // M3
		return e.InstallAIBP()
	}
	ext, err := e.AIBPVersion()
	if err != nil {
		notify(progress, "installing aibp-pi...") // M3（自愈重装）
		return e.InstallAIBP()
	}
	switch extMajorVersion, mineMajorVersion := aibp.MajorVersion(ext), aibp.MajorVersion(aibp.Protocol); {
	case extMajorVersion == mineMajorVersion:
		return nil // 兼容（fast path，M4 由调用方拼装）
	case extMajorVersion < mineMajorVersion:
		return errExtensionOutdated
	default:
		return errMicroNeoOutdated
	}
}
```

> **改动说明**：相对 D17 原版，仅 (a) 签名加 `progress func(string)`，(b) 加 `notify` helper，(c) 4 处 `notify(...)` 调用。编排逻辑（HasAgent→HasAIBP→AIBPVersion→版本比对）零改动。

### 7.2 `ensure_pi.go` —— 新增 `AIBPPackageVersion`

```go
// AIBPVersion 读协议版本（已有，D17）。AIBPPackageVersion 读包版本（D18 新增）。
// 两者读同一个 package.json，字段不同，语义正交（D4）。
// 读不到 → 返回 error，调用方降级显示。

func (PiEnsurer) AIBPPackageVersion() (string, error) {
	pkgPath := filepath.Join(piAgentDir(), "npm", "node_modules", "aibp-pi", "package.json")
	b, err := os.ReadFile(pkgPath)
	if err != nil {
		return "", err
	}
	var pkg struct{ Version string `json:"version"` }
	if err := json.Unmarshal(b, &pkg); err != nil {
		return "", err
	}
	if pkg.Version == "" {
		return "", fmt.Errorf("package.json 缺少 version 字段")
	}
	return pkg.Version, nil
}
```

### 7.3 `command_neo.go` —— progress 回调 + 成功消息

```go
package action

import (
	"fmt"

	"github.com/micro-editor/micro/v2/internal/aibp/ensure_agents"
	"github.com/micro-editor/micro/v2/internal/screen"
)

// RegisterCommands 不变（D17）。
func RegisterCommands() {
	MakeCommand("check-aibp", (*BufPane).CheckAibpCmd, nil)
}

// CheckAibpCmd：D18 加过程反馈。
func (h *BufPane) CheckAibpCmd(args []string) {
	e := ensure_agents.PiEnsurer{}
	progress := func(msg string) {
		InfoBar.Message(msg)
		InfoBar.Display()       // InfoWindow.Display()：清行 + 画 Msg
		screen.Screen.Show()    // tcell 刷屏（增量，无闪烁）
	}

	if err := ensure_agents.Ensure(e, progress); err != nil {
		InfoBar.Message("aibp-pi: " + err.Error())
		return
	}

	// M4：成功消息。包版本读不到时降级（不阻塞成功判定）。
	ver, verr := e.AIBPPackageVersion()
	if verr != nil {
		InfoBar.Message("aibp-pi: 就绪（版本未知）")
		return
	}
	InfoBar.Message(fmt.Sprintf("AIBP(v%s) installed. pi is ready.", ver))
}
```

> **为什么 progress 回调里不直接判 nil**：`notify` helper（§7.1）已经 nil-guard 了，回调本体只在真正需要时被调用，无需重复判。
>
> **降级显示**（§7.3 末）：`AIBPPackageVersion` 读失败（package.json 损坏等）不应让成功判定变失败——`Ensure` 已经返回 nil 说明装好了，展示性信息缺失走降级文案。

---

## 八、实施计划

### Phase 1 —— 接口扩展 + pi 实现 + 单测

| 改动 | 文件 |
|------|------|
| `AgentEnsurer` 加 `AIBPPackageVersion` + `Ensure` 加 `progress` 参数 + `notify` + 4 处里程碑 | `internal/aibp/ensure_agents/ensure.go` |
| 实现 `AIBPPackageVersion` | `internal/aibp/ensure_agents/ensure_pi.go` |
| `AIBPPackageVersion` 单测（正常/缺失/损坏/无 version 字段）+ `Ensure` 带 progress 的里程碑顺序断言 | `internal/aibp/ensure_agents/ensure_pi_test.go` |

**Build gate**：`make build` 过；`go test ./internal/aibp/ensure_agents` 全绿。

> **`Ensure` 签名变更的连带影响**：D17 落地后只有 `command_neo.go` 一处调用 `Ensure`，Phase 2 会同步改它。Phase 1 阶段 `command_neo.go` 暂时编译不过（签名不匹配），故 Phase 1 不单独验证 `make build`——以 `go test ./internal/aibp/ensure_agents` 为 gate，Phase 2 完成后 `make build` 才会绿。或 Phase 1 临时给 `command_neo.go` 传 `nil` 让 build 过，Phase 2 再换成真回调。**推荐后者**（每个 Phase 都 build-clean）。

### Phase 2 —— 命令挂载过程反馈

| 改动 | 文件 |
|------|------|
| `CheckAibpCmd` 加 progress 回调 + 成功消息拼装 | `internal/action/command_neo.go` |

**Build gate**：`make build` 过；手动跑 §九 T1–T6。

### Phase 3 —— 端到端验收

跑 §九 全部测试，重点验证"等待期间 InfoBar 实时变化"。

---

## 九、测试与验收

**前置**（每次重置）：`pi remove npm:aibp-pi` + settings.json 删 aibp-pi 条目 + `killall pi microneo`。

| # | 场景 | 步骤 | 期望（重点看**过程反馈**） |
|---|------|------|---------------------------|
| T1 | 首次安装 | 全新环境运行 `:check-aibp` | InfoBar 依次实时显示：`finding ai-agents...` → `pi found.` → `installing aibp-pi...`（停留数秒，对应 pi install 耗时）→ `AIBP(v1.0.1) installed. pi is ready.` |
| T2 | 常态快路径 | T1 后再运行 | `finding ai-agents...` → `pi found.` → `AIBP(v1.0.1) installed. pi is ready.`（**跳过** installing，M3 不出现） |
| T3 | 未装 pi | PATH 去掉 pi | `finding ai-agents...` → 停留 → `aibp-pi: 未检测到 pi，请先安装`（M2 不出现，因为 HasAgent 假） |
| T4 | pi install 失败 | PATH 放无 install 子命令的假 pi | `finding ai-agents...` → `pi found.` → `installing aibp-pi...` → `aibp-pi: pi install 失败...` |
| T5 | 自愈重装 | T1 后损坏 package.json 的 aibp.protocol | `finding ai-agents...` → `pi found.` → `installing aibp-pi...`（M3，自愈分支）→ `AIBP(v1.0.1) installed. pi is ready.` |
| T6 | 包版本读不到 | T1 后损坏 package.json 的 version 字段（保留 aibp.protocol） | 成功，但末消息降级为 `aibp-pi: 就绪（版本未知）` |

**单测**（`ensure_pi_test.go`）：

| 测试 | 覆盖 |
|------|------|
| `TestAIBPPackageVersion` | 正常读出 `"1.0.1"` / 文件缺失 error / 损坏 error / 无 version 字段 error |
| `TestEnsureProgress` | 注入收集型 progress 回调，断言各场景下的里程碑**序列**：T1 场景 → `[finding, pi found., installing]`；T2 场景 → `[finding, pi found.]`；T3 场景 → `[finding]`；nil 回调不 panic |

**验收清单**：

- [ ] Phase 1–3 各 build gate 全过
- [ ] `make build` 干净通过
- [ ] `go test ./internal/aibp/ensure_agents` 全绿（含新单测）
- [ ] T1–T6 手动验证，**重点确认等待期间 InfoBar 文字在变**（不是冻结）
- [ ] 零 micro 原生文件改动（`git diff` 核对，仅 `internal/aibp/ensure_agents/*` 和 `internal/action/command_neo.go`）

---

## 十、风险与已知限制

| # | 风险 | 触发条件 | 应对 |
|---|------|---------|------|
| R1 | **手动重绘与主循环竞态** | 理论上若有别的 goroutine 也在画图 | 实测无：画图只在主 goroutine（§2.4）。`DoEvent` 自己也不锁画图，印证安全 |
| R2 | **progress 回调阻塞** | 回调里做重活 | 回调只做 `Message + Display + Show`（毫秒级），不阻塞 |
| R3 | **`screen.Screen` 为 nil** | 命令在 screen 未初始化时运行 | 实际不会：命令必须通过命令 bar 触发，命令 bar 要先有 screen。若担心可加 nil-guard，但属过度防御 |
| R4 | **fast path 措辞 "installed" 语义不准** | 已装且兼容时仍显示 "installed" | D5 open question；用户反馈后再定是否区分 |
| R5 | **`Ensure` 签名变更破坏未来调用方** | opencode/claude 实现接入时 | 现在只有 pi 一个实现，零迁移；接口扩展一次性，后续稳定 |

**回滚**：三个文件 `git checkout` 取回 D17 版本即可，无新增文件、无原生文件改动。

---

## 附：与 D17 的关系

D18 是 D17 的**增量增强**，不推翻 D17 任何决策：

- D17 的编排逻辑（HasAgent→HasAIBP→AIBPVersion→版本比对）、npm 分发、`MakeCommand` 注册、幂等/自愈语义**全部保留**。
- D18 只动两件事：(a) `Ensure` 加 `progress` 参数注入过程反馈；(b) 接口加 `AIBPPackageVersion` 提供展示用版本号。
- D17 的决策表（D1–D10）不受影响；D18 新增自己的决策表（D1–D8），与 D17 编号独立。
- D17 的测试矩阵 T1–T13 中，T1/T2/T3/T9 涉及 `Ensure` 成功路径的，验收时增加"过程反馈序列正确"的检查项（见本文件 §九）。

**读完 D17 再读本文件**：本文件多处引用 D17 的概念（编排逻辑、接口契约、D8 协议版本静态化），不重复定义。
