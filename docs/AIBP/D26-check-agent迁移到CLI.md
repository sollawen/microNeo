# D26 · `:check-agent` 迁移到 CLI（`microneo --check-agent`）

> **状态**：方案已定，待实施。
> **前置**：D17 / D21 / D22 / D25（现有 `:check-agent` 命令 + `Ensure` 编排 + `InfoBarNow` 机制）。

---

## 一、目标

把 aibp 扩展自举（检测 / 安装 / 升级各 agent 的 aibp-pi、aibp-opencode）从 TUI 命令 `:check-agent` 迁移到 shell 命令 `microneo --check-agent`。删除 TUI 命令，统一为单一 CLI 入口。

---

## 二、现状问题

当前 `:check-agent`（`internal/action/command_neo.go`）有三个结构性问题：

1. **InfoBar 单行反馈不足**：安装/升级是多步骤（checking → installing → ready），InfoBar 只能显示最后一条，过程丢失。
2. **阻塞 TUI 主线程**：`CheckAibpCmd` 在主 goroutine 同步跑 `pi install`（联网数秒），冻结 `DoEvent` 循环。`InfoBarNow`（D21）正是为此打的补丁——在冻结的线程里手动刷 InfoBar。
3. **不可脚本化**：CI / 安装向导无法调用 TUI 命令验证 agent 状态。

根因：aibp 自举是**系统级操作**（检测环境、联网安装），不是编辑器内操作。应走 shell，让 stdout 自由输出全过程。

**可行性**：`ensure_agents.Ensure(e, Reporter)` 已是 UI 无关的——只接受 `Reporter func(string)` 回调。迁移只需把 reporter 从 `InfoBarNow` 换成 `fmt.Println`，单 agent 编排逻辑零改动。且 `ensure_agents` 包对 `config`/`screen`/`plugins` 零依赖，所以 `-check-agent` 能在 `main()` 最早期退出，跳过所有初始化——config 损坏时也能工作（诊断工具应具备的独立性）。

---

## 三、方案

### 3.1 目标架构

```
internal/aibp/
├── registry.go / message.go        运行时：Discover / 协议常量（不变）
├── ensure_agents/                  ⭐ 位置不动（aibp 域能力）
│   ├── ensure.go                   + EnsureAll()（批量编排，与 Ensure 对称）
│   ├── ensure_pi.go                不变
│   └── ensure_opencode.go          不变
└── cmd/discover|send               开发期协议探针（手测 Discover/send 链路），不变

cmd/micro/
├── micro.go                        + flag 声明 + usage + main() 早退调用（唯一改的上游文件）
├── micro_neo.go                    ⭐ microNeo CLI 扩展聚合：DoCheckAgent + ResetSettings
├── clean.go / debug.go / ...        上游原生，不碰
└── reset_settings.go               ❌ 删（内容并入 micro_neo.go）

internal/action/
├── infopane.go                     上游原生，不碰
├── infopane_neo.go                 ⭐ 新建：MessageNow 方法（对齐 bufpane_md.go / command_neo.go 惯例）
├── command_neo.go                  ❌ 删整个文件
├── globals.go                      删 RegisterCommands() 调用
└── notepane.go                     失败提示带 CLI 引导
```

`ensure_agents` 对外三件套，各司其职：

| 符号 | 角色 |
|---|---|
| `Ensure(e, report)` | 单 agent 状态机（检查→版本判断→install/update/ready） |
| `EnsureAll(report)` | 批量编排（遍历 + 过滤 + 调 Ensure + 累积错误） |
| `Reporter` | 进度回调，解耦域与 IO |

注册表 `allEnsurers` 收为包私有——迁移后它唯一的包外引用者（`command_neo.go`）被删，零外部引用。未来若有 `--list-agents` 类需求再重新导出。

### 3.2 域包：新增 `EnsureAll`

`internal/aibp/ensure_agents/ensure.go`：

```go
// EnsureAll 对所有已注册 agent 执行 Ensure 编排（跳过未安装的）。
// report 透传给各 agent 的 Ensure；nil 则静默。
// 返回 hadError：是否有 agent 出错，调用方据此决定退出码。
func EnsureAll(report Reporter) (hadError bool) {
	if report == nil {
		report = func(string) {}
	}
	for _, e := range AllEnsurers {
		if !e.HasAgent() {
			report(e.AgentName() + ": not installed, skipping")
			continue
		}
		if err := Ensure(e, report); err != nil {
			hadError = true
		}
	}
	return
}
```

**行为变化**：TUI 版对未装 agent 静默跳过；CLI 版显式打 "skipping"——CLI 诊断需要完整透明度（让用户看到"pi 没装所以跳过"）。

### 3.3 入口层

**`cmd/micro/micro_neo.go`**（新增，聚合所有 microNeo CLI 扩展；`ResetSettings` 从原 `reset_settings.go` 搬入）：

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/micro-editor/micro/v2/internal/aibp/ensure_agents"
	"github.com/micro-editor/micro/v2/internal/config"
	rt "github.com/micro-editor/micro/v2/runtime"
)

// ResetSettings —— 从原 reset_settings.go 搬入，逻辑不变。
// 复制 embedded runtime/settings.json 到用户 config 目录。
func ResetSettings() {
	// ...（原内容不变）
}

// DoCheckAgent 执行 -check-agent：对已装的 agent 跑 aibp 扩展自举编排，
// 进度打到 stdout。跑完 exit，不进 TUI。
// 放在 config/screen 初始化之前——ensure_agents 对二者零依赖，
// 即便 micro config 损坏也能工作。
func DoCheckAgent() {
	if !*flagCheckAgent {
		return
	}
	hadErr := ensure_agents.EnsureAll(func(msg string) { fmt.Println(msg) })
	if hadErr {
		exit(1)
	}
	exit(0)
}
```

**聚合原则**：`cmd/micro/` 下所有 microNeo 新增的 CLI flag handler 都进 `micro_neo.go`，未来加 `--xxx` 只动这一个文件 + micro.go 加一行调用。上游文件（clean.go / debug.go）永远不碰，merge 上游零冲突。

**`cmd/micro/micro.go`**（三处改动）：

```go
// flag 声明（与其它 flag 并列）
flagCheckAgent = flag.Bool("check-agent", false,
	"Check and self-heal aibp extensions for installed AI agents (pi/opencode), then exit")
```

```
// flag.Usage 加一段（仿 -clean 格式）
-check-agent
    \tCheck aibp extensions for all installed AI agents (pi, opencode).
    \tInstalls missing extensions, updates outdated ones, prints status to stdout, and exits.
    \tDoes not open the editor.
```

```go
// main() 调用，紧跟 InitFlags()（在 config.InitConfigDir 之前）
InitFlags()
DoCheckAgent()   // -check-agent 早退，零 config 依赖
// ...
```

### 3.4 action 层清理

**`internal/action/infopane_neo.go`**（新建，不碰上游 `infopane.go`）：`InfoBarNow` 从 `command_neo.go` 的包级函数，改为上游 `InfoPane` 类型的方法，定义在新文件里（与 `bufpane_md.go` / `command_neo.go` 同构——上游 `<name>.go` + microNeo `<name>_neo.go`）：

```go
// MessageNow 设消息 + 同步刷 InfoBar 到终端。
// 用于主 goroutine 同步阻塞期间需要立即反馈的场景（如 SelectPane 同步加载）。
// 必须在主 goroutine 调用。
func (h *InfoPane) MessageNow(msg string) {
	screen.Screen.HideCursor()
	h.Message(msg)
	h.Display()
	screen.Screen.Show()
}
```

调用方从 `InfoBarNow(msg)` → `InfoBar.MessageNow(msg)`。保留它：是合理的中性工具，有明确场景（主线程阻塞时的即时反馈），成本 8 行。

**`internal/action/command_neo.go`**：删整个文件（`CheckAibpCmd` / `TestInfoCmd` / 包级 `InfoBarNow` / `RegisterCommands`）。`TestInfoCmd` 是为测 `InfoBarNow` 补丁而存在的死代码，一并删——已验证 `:test-info` 在代码中零外部引用（仅 `command_neo.go` 自身注册 + D21 历史文档提及）。

**`internal/action/globals.go`**：删 `InitGlobals()` 末尾的 `RegisterCommands()` 调用。

**`internal/action/notepane.go`**：两处（约 L142、L210）失败提示带引导：

```go
InfoBar.Message(`✗ no receiver — run "microneo --check-agent"`)
```

这是删 `:check-agent` 后的入口可发现性兜底：用户按 Alt-Enter 失败时，直接被告知该敲什么。

---

## 四、设计决策

| # | 决策 | 理由 |
|---|------|------|
| 1 | 只保留 CLI `--check-agent`，删 TUI `:check-agent` | 系统级自举非编辑器操作；TUI 内联网自举冻结主线程是反模式 |
| 2 | 一步到位，不分阶段 | 无运行时风险（同份 `Ensure` 编排）；项目早期，校准架构成本最低 |
| 3 | 批量编排下沉为 `EnsureAll` | 遍历策略是域知识；与 `Ensure`/`allEnsurers` 成三件套 |
| 4 | `ensure_agents` 留在 `internal/aibp/` | 包位置跟领域走不跟调用方走；域内依赖闭环 |
| 5 | microNeo CLI 扩展聚合到 `micro_neo.go`，删 `reset_settings.go` | 同阵营聚合；merge 上游零冲突；新增 flag 只动一个文件 |
| 6 | `MessageNow` 放 `infopane_neo.go`，不碰上游 `infopane.go` | 对齐 `bufpane_md.go` / `command_neo.go` 惯例；上游零侵入 |
| 7 | `InfoBarNow` → `InfoPane.MessageNow` | 能力内聚到类型；保留（有合理场景如 SelectPane），删死代码 `TestInfoCmd` |
| 8 | 未装 agent 显式打 "skipping"；flag 名 `--check-agent` | CLI 诊断需透明度；与原命令名延续，迁移成本低 |

---

## 五、输出示例

```
$ microneo --check-agent
checking pi ...
aibp-pi ready (aibp-2.0)

checking opencode ...
aibp-opencode not installed, installing ...
aibp-opencode installed

$ echo $?
0
```

退出码：0 = 全部 ready/跳过；1 = 至少一个 agent 出错。可脚本化（CI、安装向导）。

---

## 六、实施步骤

| # | 文件 | 改动 | 依赖 |
|---|------|------|------|
| 1 | `internal/aibp/ensure_agents/ensure.go` | 新增 `EnsureAll`；`AllEnsurers` → `allEnsurers`（包私有） | 无 |
| 2 | `cmd/micro/micro_neo.go` | 新建：`DoCheckAgent` + 搬入 `ResetSettings` | 1 |
| 3 | `cmd/micro/reset_settings.go` | ❌ 删（内容已并入 micro_neo.go） | 2 |
| 4 | `cmd/micro/micro.go` | flag 声明 + usage + `main()` 调用 + import | 2 |
| 5 | `internal/action/infopane_neo.go` | 新建：`MessageNow` 方法（上游 InfoPane 类型的扩展） | 无 |
| 6 | `internal/action/command_neo.go` | 删整个文件 | 5 |
| 7 | `internal/action/globals.go` | 删 `RegisterCommands()` 调用 | 6 |
| 8 | `internal/action/notepane.go` | 两处提示文案 | 无 |
| 9 | `README.md` / `CHANGELOG.md` | README 里 `:check-agent` 引用改 `microneo --check-agent`；CHANGELOG 的历史版本记录（v1.0.x 已发布内容）**保留不动**，新增一条迁移说明条目指向 CLI | 1-8 |
| 10 | `D17` / `D21` / `D22` / `D25` | 顶部加迁移标注 | 1-8 |

改动量：~30 行净增 + ~80 行净删（含 reset_settings.go 整文件）+ 2 处文案 + 1 处可见性收敛。每步后 `make build-quick`，全完成后 `make build`。

---

## 七、验收

| # | 场景 | 期望 |
|---|------|------|
| V1 | 常态（已装且兼容） | stdout 各 agent "checking / ready"；退出码 0；不进 TUI |
| V2 | 首装（`pi remove npm:aibp-pi` 后跑） | "installing" → "installed"；退出码 0 |
| V3 | 未装 agent（PATH 去 pi） | "pi: not installed, skipping"；继续 opencode；退出码 0（**未装不算错误**） |
| V4 | install 失败（假 pi：`HasAgent()=true` 但 `InstallAIBP()` 失败） | stdout/stderr 见原因；退出码 1（**仅 `Ensure()` 返回非 nil 才计 `hadError`**） |
| V5 | 不带 flag（`microneo file.md`） | 正常进 TUI |
| V6 | 源码安装（dev 机） | "source install, skipping"；退出码 0 |
| V7 | config 损坏 | 仍正常工作（零 config 依赖） |
| V8 | `:check-agent` | 命令不存在：输入 `:check-agent` 报 unknown command；命令补全列表无此项 |
| V9 | Alt-Enter 无 receiver | InfoBar 显示 `✗ no receiver — run "microneo --check-agent"` |
| V10 | `make build` | 无编译错误、无未用 import |

---

## 八、与历史文档的关系

- **D17**：命令入口被 D26 取代，`Ensure`/`AgentEnsurer` 编排不变。加迁移标注。
- **D21**（`InfoBarNow`）：补丁机制随 `:check-agent` 删除失去原场景；能力以 `MessageNow` 形式保留，脱离 agent 自举语境。加迁移标注。
- **D22 / D25**：编排逻辑不变，入口从 `:command` 变 `-flag`。加迁移标注。
