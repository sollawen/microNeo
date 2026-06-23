# D17 · microNeo 通过 `:check-agent` 命令初始化 aibp-pi

> **状态**：方案已定，待实施。aibp-pi 已发布到 npm（[`aibp-pi@1.0.1`](https://www.npmjs.com/package/aibp-pi)），端到端链路已验证。

---

## 一、目标

用户下载安装 microNeo 后，运行一次 `:check-agent` 命令完成 aibp-pi 的检查与（必要的）安装，之后按 Alt-Enter 就能让 notePane 跟 pi 通信。

`:check-agent` 一次完成三件事：

| 需求问题 | 本方案 |
|---------|--------|
| 如何发现用户装了 pi | `HasAgent()`：`exec.LookPath("pi")` |
| 如何判断 pi 有没有装 aibp-pi | `HasAIBP()`：读 pi 的 `settings.json`，找 `npm:aibp-pi` 条目 |
| 没装时怎么装进去 | `InstallAIBP()`：`pi install npm:aibp-pi`（不锁版本） |

**核心编排逻辑**（各 agent 共用，完整代码见 §五）：HasAgent 失败 → 提示装 pi；HasAIBP 失败 → InstallAIBP；版本读不到（损坏/缺失）→ 视为没装 → InstallAIBP；版本相等 → 就绪；扩展过旧 → 只提示（D9）；microNeo 过旧 → 提示升级 microNeo。

**用户怎么知道要运行 `:check-agent`**：首次按 Alt-Enter 时，`aibp.Discover()` 找不到 receiver，InfoBar 提示 `✗ no receiver found`，notePane 不打开。这个已有的守卫（`notepane.go:207-208`）就是自然引导信号。用户可据此查找文档或运行 `:check-agent`。

**不做的事情**：启动时自动检查（用户主动跑 `:check-agent`）、git 源安装、运行时弹窗问用户、扩展过旧时自动联网升级（我们只检测 + 提示，更新是用户自己的事，见 D9）。

---

## 二、方案选型

**npm 标准分发**：aibp-pi 作为独立 npm 包发布（`npm:aibp-pi`，已上线）。用户运行 `:check-agent` 时检查 pi 的 `settings.json`，若没有登记 aibp-pi 就调 `pi install` 装上。

**为什么用 npm 分发**：

- **职责解耦**：aibp-pi 改了发 npm 包，microNeo 不用跟着发版
- **机制极简**：pi 自己管 `npm install`、管安装目录、管 settings.json 登记——microNeo 只调一条命令
- **模型统一**：未来 aibp-opencode / aibp-claude 都是同一个模式——发 npm 包 + 五个接口方法
- **用户可控**：`pi list` 看得到，`pi remove` 能卸载，`pi update --extensions` 能升级

**命令触发**：用户主动运行 `:check-agent`，而非启动时自动检查。用户主动触发，避开启动期的 UI 时序问题（主循环未起时 InfoBar 渲染不出来），且给明确反馈天经地义。

**不 pin 版本**：`InstallAIBP()` 用 `npm:aibp-pi`（不带 `@版本号`）。pin 死会让 `pi update --extensions` 跳过升级，与"独立发版、独立升级"初衷相悖。兼容历史遗留的 pinned spec（`npm:aibp-pi@x.y.z`）——见 `HasAIBP` 实现备注。

---

## 三、系统设计

### 3.1 模块划分

只有一层 aibp 域逻辑，外加一个命令注册点：

| 层 | 职责 | 归属 |
|----|------|------|
| **aibp 扩展自举** | `AgentEnsurer` 接口 + `Ensure` 编排 + 各 agent 实现 | `internal/aibp/ensure_agents/` |
| **microNeo 自有命令** | `:check-agent` 等命令方法 + 注册 | `internal/action/command_neo.go`（对应上游 `command.go`） |

```
internal/
├── action/
│   ├── globals.go          ← InitGlobals() 末尾调 RegisterCommands()（+1 行，该文件已被 notePane 改动）
│   └── command_neo.go          ← ⭐ microNeo 自有命令（:check-agent 等）+ 注册，对应上游 command.go
└── aibp/
    ├── registry.go         ← 现有（major → MajorVersion 导出，§6.3）
    ├── message.go          ← 现有（Protocol 常量）
    └── ensure_agents/      ← ⭐ aibp 扩展自举子包
        ├── ensure.go       ← AgentEnsurer 接口 + Ensure 编排 + 错误变量
        ├── ensure_pi.go    ← PiEnsurer 实现（+ 编译期接口断言）
        ├── ensure_pi_test.go
        ├── ensure_opencode.go   (未来)
        └── ensure_claude.go     (未来)
```

**依赖方向（单向、无环）**：

```
cmd/micro ──▶ action ──▶ aibp/ensure_agents ──▶ aibp
```

| 包 | 依赖 |
|----|------|
| `action` | `aibp/ensure_agents`（命令方法调 `Ensure`） |
| `aibp/ensure_agents` | `aibp`（`Protocol` 常量、`MajorVersion()` 解析器） |
| `aibp/ensure_agents` | **不依赖** `action`（纯逻辑，错误返回给调用方处理） |

**为什么用 `MakeCommand` API 而不直接改 `command.go` 的命令 map**：

`command.go` 是纯上游 micro 文件（git 历史确认未受 microNeo 改动）。micro 提供了 `MakeCommand(name, action, completer)` 专门给插件/扩展注册命令，正是为避免侵入 `InitCommands` 的 map。microNeo 调用它注册 `:check-agent`，零修改 `command.go`。

**注册时机**：`cmd/micro/micro.go` 里 `InitCommands()`（407 行）初始化 map，`InitGlobals()`（416 行）在其后。因此 `RegisterCommands()`（内部调 `MakeCommand`）由 `InitGlobals` 调用时 map 已就绪，写入安全。

### 3.2 协议版本判定的双向语义

`Ensure` 的版本校验解决的是**静态兼容性**问题：aibp-pi 的实现协议和 microNeo 自己实现的协议版本是否兼容。

| 数据点 | 来源 | 形态 |
|--------|------|------|
| **microNeo 期望的协议** | `internal/aibp.Protocol`（已存在的常量 `"aibp-1"`） | 字符串 |
| **已装扩展实现的协议** | 扩展包 `package.json` 的 `aibp.protocol` 字段 | 字符串 |

兼容性判定用**主版本相等**（复用 `internal/aibp.MajorVersion()`，§6.3 导出）：

| `MajorVersion(扩展协议)` vs `MajorVersion(microNeo 协议)` | 含义 | `:check-agent` 行为 |
|----------------------------------------------|------|-------------|
| 相等 | 兼容 | InfoBar 提示"aibp-pi: 就绪" |
| 扩展 < microNeo | 扩展过旧 | InfoBar 提示用户运行 `pi update npm:aibp-pi`（**D9：不自动升级**） |
| 扩展 > microNeo | microNeo 过旧 | InfoBar 提示"请升级 microNeo" |

> **协议版本的字段为什么是 `aibp.protocol` 而不是 package.json 的 `version`**：包版本是 npm 迭代用的（`1.0.0` → `1.0.1` → `1.1.0`），不对应协议兼容性。协议可能多个包版本都实现 `aibp-1`；也可能一个包版本实现 `aibp-2`。两者是正交维度。

### 3.3 自举流程（幂等）

- **常态快路径**（已装且兼容）：`LookPath` + 读 settings.json + 读已装扩展 package.json = 几 ms，不 spawn
- **首次安装**：spawn `pi install`，pi 自己跑 `npm install`、写 settings.json。用户主动触发，等几秒可接受
- **幂等**：重复运行 `:check-agent`，已装且兼容则只读不 spawn，提示"就绪"
- **自愈**：package.json 损坏/缺失 → 读不到协议版本 → 视作"没装" → 触发 Install 重装

---

## 四、设计决策

| # | 决策 | 理由 |
|---|------|------|
| D1 | **触发方式：用户运行 `:check-agent` 命令** | 用户主动触发，避开启动期的 UI 时序问题；用户需知道命令，由 Alt-Enter 失败时的 `no receiver found` 提示自然引导 |
| D2 | **命令执行时给明确反馈** | 用户主动触发，给明确反馈是合理的：成功提示"就绪"、失败提示原因、过旧提示升级路径。反馈通过 InfoBar |
| D3 | **`pi install` 失败不兜底**，报错 + 提示升级 pi | 唯一现实风险是 pi 太旧没有 install 子命令 |
| D4 | **同步执行**（非异步） | 用户主动运行命令，等几秒是合理预期，无需异步化的竞态复杂度 |
| D5 | **不 pin 版本**（spec 用 `npm:aibp-pi` 不带 `@版本`） | aibp-pi 独立发版，microNeo 不跟随；升级交给 `pi update --extensions` |
| D6 | **自举文件命名 `internal/aibp/ensure_agents/ensure_<agent>.go`** | 一个 agent 一个文件；未来加 opencode/claude = 复制模板 + `command_neo.go` 加注册行 |
| D7 | **接口契约：每个 `ensure_<agent>.go` 实现五个语义方法** | 详见 §五。不同 agent 扩展机制差异大（pi 用 npm、opencode 可能用 plugin），由各自实现吸收 |
| D8 | **协议版本静态化**：扩展 `package.json` 的 `aibp.protocol` 是单一事实来源 | `:check-agent` 静态读它判断兼容性；运行时也读它写注册表。静态/运行时必然一致（详见 [`说明-AIBP §7.1.1`](./说明-AIBP.md)） |
| D9 | **扩展过旧时只提示，不自动 Update** | 职责边界：Install 是从无到有（必要、可自动）；Update 是改动已有环境（应知情）。我们只负责检测 + 提示，用户怎么更新是他自己的事 |
| D10 | **用 `MakeCommand` API 注册命令，不改 `command.go`** | `command.go` 是纯上游 micro 文件。详见 §3.1 末段（`MakeCommand` 机制 + 注册时机） |

---

## 五、`AgentEnsurer` 接口契约

**每个 agent 自举文件实现以下五个方法**。这是统一契约，吸收各 agent 扩展机制的差异——pi 用 npm、opencode 可能用 plugin、claude 可能用 mcp config，各自的实现细节藏在 `ensure_<agent>.go` 内部。

```go
// internal/aibp/ensure_agents/ensure.go
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

// AgentEnsurer 描述一个 agent 的 aibp 扩展自举。
// 不同 agent 的扩展机制差异大，各自实现这五个方法吸收差异。
type AgentEnsurer interface {
	AgentName() string                              // "pi" / "opencode"——日志和报错用

	HasAgent() bool                             // 本机有没有这个 agent 程序
	HasAIBP() bool                              // 该 agent 装没装 aibp 扩展
	AIBPVersion() (string, error)               // 已装扩展实现的协议（如 "aibp-1"）。
	                                            //   注意：协议版本，非包版本。读静态声明（package.json），不启动 agent
	InstallAIBP() error                         // 装 aibp 扩展到该 agent
}

// Ensure 是统一编排逻辑，各 ensure_<agent>.go 共用。
// 返回的 error 由调用方（action/command_neo.go）决定如何交互（InfoBar 提示等）——
// 本函数不预设交互模式，不依赖 action 包。
func Ensure(e AgentEnsurer) error {
	if !e.HasAgent() {
		return fmt.Errorf("未检测到 %s，请先安装", e.AgentName())
	}
	if !e.HasAIBP() {
		return e.InstallAIBP()
	}
	ext, err := e.AIBPVersion()
	if err != nil { return e.InstallAIBP() }              // 读不到协议 = 没装，重装
	switch extMajorVersion, mineMajorVersion := aibp.MajorVersion(ext), aibp.MajorVersion(aibp.Protocol); {
	case extMajorVersion == mineMajorVersion: return nil              // 兼容
	case extMajorVersion < mineMajorVersion:  return errExtensionOutdated // D9：只提示
	default:                    return errMicroNeoOutdated
	}
}
```

**编排逻辑与各 agent 实现解耦**——`Ensure()` 不知道是 pi 还是 opencode，只调接口。agent 之间的差异（npm install vs plugin manager vs 编辑配置文件）全在各 agent 的方法实现里。

---

## 六、实现规格（pi 版）

### 6.1 文件清单与侵入面

| 文件 | 状态 | 包 | 职责 |
|------|------|---|------|
| `internal/aibp/ensure_agents/ensure.go` | 新增 | `ensure_agents` | `AgentEnsurer` 接口 + `Ensure` 编排 + 错误变量（见 §五） |
| `internal/aibp/ensure_agents/ensure_pi.go` | 新增 | `ensure_agents` | `PiEnsurer` 实现（4 个方法 + 编译期断言） |
| `internal/aibp/ensure_agents/ensure_pi_test.go` | 新增 | `ensure_agents` | `HasAIBP` 匹配逻辑、协议版本解析单测 |
| `internal/action/command_neo.go` | 新增 | `action` | microNeo 自有命令：`CheckAibpCmd` 方法 + `RegisterCommands` 注册函数（对应上游 `command.go`） |
| `internal/action/globals.go` | 改动 (+1 行) | `action` | 挂载点：`InitGlobals()` 末尾调 `RegisterCommands()`（该文件已被 notePane/FloatFrame 改动，非新侵入） |
| `internal/aibp/registry.go` | 改动 (1 行) | `aibp` | `func major` → `func MajorVersion` 导出（§6.3） |

**对 micro 原生代码的侵入**：唯一触碰的原生文件是 `globals.go`（且该文件本就被 microNeo 改过，本次仅 +1 行）。`registry.go` 不是原生文件——它在 `internal/aibp/`，该目录整个是 microNeo 自建（上游 micro 没有这个目录），所以 `major`→`MajorVersion` 导出 + `Discover` 硬编码修复都不算侵入原生。`command.go` 完全不动（用 `MakeCommand` API）。其余全为新增。预计 ~100 行 Go，无新依赖（全标准库）。

### 6.2 代码骨架

**① `internal/aibp/ensure_agents/ensure_pi.go`** —— pi 版 `AgentEnsurer`

```go
package ensure_agents

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const aibpPiSpec = "npm:aibp-pi" // 不 pin 版本（D5）

// PiEnsurer 是 pi 的 aibp 扩展自举实现。
type PiEnsurer struct{}

var _ AgentEnsurer = PiEnsurer{} // 编译期接口断言（同包可用）

func (PiEnsurer) AgentName() string { return "pi" }

func (PiEnsurer) HasAgent() bool {
	_, err := exec.LookPath("pi")
	return err == nil
}

// piAgentDir 返回 pi 的 agent 配置目录（镜像 pi 的 getAgentDir()，见 pi 源码 dist/config.js:412-419）。
// 优先读 PI_CODING_AGENT_DIR 环境变量（pi 实际的覆盖机制，见 config.js:413；变量名由 APP_NAME 派生，
// pi 的 APP_NAME="pi" 所以是 PI_CODING_AGENT_DIR）。
// ~ 展开规则镜像 pi 的 normalizePath（见 pi 源码 dist/utils/paths.js:48-52）："~"→$HOME，"~/x"→$HOME/x。
// 注意：设了环境变量时 pi 直接用展开结果作 agent 目录，不附加 .pi/agent（与未设时的 fallback 不同）。
//
// 派生路径（与 pi 读写一致）：
//   settings.json:                   <agentDir>/settings.json
//   aibp 扩展的 package.json:        <agentDir>/npm/node_modules/aibp-pi/package.json
//                                    （npm: spec 走 agentDir/npm，见 pi package-manager.js:1597）
func piAgentDir() string {
	if env := os.Getenv("PI_CODING_AGENT_DIR"); env != "" {
		if env == "~" {
			home, _ := os.UserHomeDir()
			return home
		}
		if strings.HasPrefix(env, "~/") {
			home, _ := os.UserHomeDir()
			return filepath.Join(home, env[2:])
		}
		return env
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pi", "agent")
}

func piSettingsPath() string {
	return filepath.Join(piAgentDir(), "settings.json")
}

// HasAIBP：检查 pi settings.json 的 packages 里是否已登记 aibp-pi。
// 兼容 "npm:aibp-pi"（unpinned，期望形态）和 "npm:aibp-pi@x.y.z"（pinned，历史遗留）。
func (PiEnsurer) HasAIBP() bool {
	b, err := os.ReadFile(piSettingsPath())
	if err != nil { return false } // 读不到 → 视为未登记（首次安装）
	var s struct{ Packages []string `json:"packages"` }
	if json.Unmarshal(b, &s) != nil { return false }
	for _, p := range s.Packages {
		if p == aibpPiSpec || strings.HasPrefix(p, aibpPiSpec+"@") { return true }
	}
	return false
}

// AIBPVersion：读已装扩展的 package.json 的 aibp.protocol 字段。
//   扩展位置 = <piAgentDir>/npm/node_modules/aibp-pi/package.json（与 settings.json 同源）
//   读不到（文件缺失/损坏/字段缺失）→ 返回 error，由 Ensure 视作「没装」触发 Install。
//   协议版本的单一事实来源是 package.json 的 aibp.protocol——aibp-pi 启动时也读它派生注册表
//   （见 aibp-agents/pi/index.ts:11-12），所以静态检测与运行时必然一致。
func (PiEnsurer) AIBPVersion() (string, error) {
	pkgPath := filepath.Join(piAgentDir(), "npm", "node_modules", "aibp-pi", "package.json")
	b, err := os.ReadFile(pkgPath)
	if err != nil { return "", err }
	var pkg struct{ AIBP struct{ Protocol string `json:"protocol"` } `json:"aibp"` }
	if err := json.Unmarshal(b, &pkg); err != nil { return "", err }
	// 字段缺失（合法 JSON 但无 aibp.protocol）也视为读不到——Ensure() 会据此重装自愈。
	// 否则 MajorVersion("") 返回 -1，Ensure() 会误报"扩展过旧"，用户跑了 pi update 也修复不了。
	if pkg.AIBP.Protocol == "" {
		return "", fmt.Errorf("package.json 缺少 aibp.protocol 字段（视为没装）")
	}
	return pkg.AIBP.Protocol, nil
}

func (PiEnsurer) InstallAIBP() error {
	cmd := exec.Command("pi", "install", aibpPiSpec) // 不带 @版本（D5）
	if err := cmd.Run(); err != nil {
		// D3：pi install 失败不兜底。唯一现实风险是 pi 太旧没有 install 子命令。
		return fmt.Errorf("pi install 失败（可能 pi 过旧，请升级 pi）: %w", err)
	}
	return nil
}
```

**② `internal/action/command_neo.go`** —— microNeo 自有命令文件（对应上游 `command.go`）：`:check-agent` 方法 + 注册

```go
package action

import "github.com/micro-editor/micro/v2/internal/aibp/ensure_agents"

// RegisterCommands 注册 microNeo 所有自己的命令。
// 用 MakeCommand API（micro 为插件/扩展设计的注册入口），避免侵入 command.go 的 InitCommands map。
// 必须在 InitCommands() 之后调用（commands map 已初始化），由 InitGlobals 触发。
// 未来加新命令：在此函数体内追加 MakeCommand 行即可。
func RegisterCommands() {
	MakeCommand("check-agent", (*BufPane).CheckAibpCmd, nil)
}

// CheckAibpCmd 是 :check-agent 命令的处理函数。
// 检查 pi 是否装了 aibp-pi 扩展；没装则安装，装了则校验协议版本兼容性。
// 用户主动运行（非启动自动），可给明确反馈。
func (h *BufPane) CheckAibpCmd(args []string) {
	if err := ensure_agents.Ensure(ensure_agents.PiEnsurer{}); err != nil {
		InfoBar.Message("aibp-pi: " + err.Error())
		return
	}
	InfoBar.Message("aibp-pi: 就绪")
}
```

**③ `internal/action/globals.go`** —— 挂载点（仅 +1 行，该文件已被 notePane/FloatFrame 改动）

```go
func InitGlobals() {
	// ... 原有初始化
	RegisterCommands() // +1 行。同包调用，无需 import
}
```

### 6.3 实现细节：导出 `aibp.MajorVersion()`

`aibp.MajorVersion()` 当前是包内私有 `major()`（`internal/aibp/registry.go`）。导出为 `MajorVersion()`——改动 1 行（`func major` → `func MajorVersion`，大写化）。这不算侵入 micro 原生代码：`internal/aibp/` 整个目录是 microNeo 自己加的（上游 micro 在 github 上根本没有这个目录），改的纯属 microNeo 自己的文件。

**附带**：顺手把 `registry.go` 里 `Discover()` 函数硬编码的 `MajorVersion(rf.Protocol) != 1` 改为 `MajorVersion(rf.Protocol) != MajorVersion(aibp.Protocol)`，让 [`说明-AIBP §7.1.1`](./说明-AIBP.md) 的"单一事实来源"叙事自洽（否则升级到 aibp-2 时这里会漏改）。

---

## 七、实施计划

每个 Phase 通过自己的 build gate 才进入下一个。

### Phase 0 — 前置基线（已通过 ✅）

| 检查项 | 验证方法 | 状态 |
|--------|---------|------|
| 编译通过 | `make build` 成功 | ✅ |
| aibp-pi 在 npm 上可装 | `pi install npm:aibp-pi` 成功 | ✅（[aibp-pi@1.0.1](https://www.npmjs.com/package/aibp-pi)） |
| AIBP 基线链路通 | 开 pi → microNeo Alt-Enter → 发消息 → pi 收到 | ✅（实测） |
| 静态字段 + 运行时同源 | jiti 读 package.json 写注册表 `protocol: aibp-1` | ✅（实测） |

基线全过。回滚靠 git。

### Phase 1 — `AgentEnsurer` 接口 + pi 实现（可单测）

| 改动 | 文件 |
|------|------|
| 导出 `aibp.MajorVersion()` + 顺手修 `Discover()` 硬编码 | `internal/aibp/registry.go`（`major` → `MajorVersion`） |
| 新增接口 + 编排 | `internal/aibp/ensure_agents/ensure.go`（见 §五） |
| 新增 pi 自举 | `internal/aibp/ensure_agents/ensure_pi.go`（见 §6.2①） |
| 新增单测 | `internal/aibp/ensure_agents/ensure_pi_test.go` |

**Build gate**：`make build` 过；`go test ./internal/aibp/ensure_agents` 全绿（`HasAIBP` 的 pinned/unpinned 匹配、`AIBPVersion` 的正常/损坏/缺失路径）。此 Phase 不挂载到 globals，不改运行期。

### Phase 2 — 注册命令

| 改动 | 文件 |
|------|------|
| 新增命令方法 + 注册函数 | `internal/action/command_neo.go`（见 §6.2②） |
| 挂载 | `internal/action/globals.go:InitGlobals()`（见 §6.2③） |

**Build gate**：`make build` 过；全新环境运行 `:check-agent` 后 `~/.pi/agent/settings.json` 含 `npm:aibp-pi`、`pi list` 显示 aibp-pi、Alt-Enter 能打开 notePane。

### Phase 3 — 测试矩阵 + 端到端

跑 §八 全部测试。

---

## 八、测试与验收

**前置**（每次测试前重置）：`pi remove npm:aibp-pi` + 从 settings.json 删 aibp-pi 条目 + `killall pi microneo`。

| # | 场景 | 前置条件 | 步骤 | 期望 |
|---|------|---------|------|------|
| T1 | 未装 pi | PATH 去掉 pi | 运行 `:check-agent` | InfoBar 提示"未检测到 pi，请先安装"；不改 settings.json |
| T2 | 首次安装 | 全新环境，pi 在 PATH | 运行 `:check-agent` | settings.json 含 `npm:aibp-pi`（unpinned）；`pi list` 显示 aibp-pi；InfoBar 提示"就绪" |
| T3 | 常态快路径 | T2 后再运行 | 运行 `:check-agent` | 不 spawn install；InfoBar 提示"就绪" |
| T4 | 登记丢失 | T2 后 `pi remove npm:aibp-pi` | 运行 `:check-agent` | 重新 `pi install`；`pi list` 重现 aibp-pi |
| T5 | pi install 失败 | PATH 放一个无 install 子命令的假 pi | 运行 `:check-agent` | InfoBar 提示升级 pi |
| T6 | 扩展协议过旧 | 改已装 package.json 的 `aibp.protocol` 为 `"aibp-0"` | 运行 `:check-agent` | InfoBar 提示运行 `pi update`（**D9：不自动升级**） |
| T7 | microNeo 协议过旧 | 改已装 package.json 的 `aibp.protocol` 为 `"aibp-2"` | 运行 `:check-agent` | InfoBar 提示升级 microNeo |
| T8 | 端到端 | T2 后 | 开 pi → Alt-Enter → 写一行 → Alt-Enter | notePane 打开；pi 收到消息 |
| T9 | 幂等 | T2 后连运行两次 | 第二次 `:check-agent` | 不 spawn install；都提示"就绪" |
| T10 | Alt-Enter 自然引导 | 未装 aibp-pi | 按 Alt-Enter | InfoBar 提示 `✗ no receiver found`；notePane 不打开 |
| T11 | pinned 历史遗留 | 手动 `pi install npm:aibp-pi@1.0.0` 后改协议为 aibp-0 | 运行 `:check-agent` | InfoBar 提示扩展协议过旧 |
| T12 | 自定义 PI_CODING_AGENT_DIR | 设 `PI_CODING_AGENT_DIR=/tmp/fake_pi`，启动 pi 装好 aibp-pi（装到该路径下） | 运行 `:check-agent` | `HasAIBP` 返回 true（读到 `/tmp/fake_pi/settings.json`）；不重装；InfoBar 提示"就绪" |
| T13 | 已装但 package.json 损坏 | T2 后删除/损坏 `<piAgentDir>/npm/node_modules/aibp-pi/package.json` | 运行 `:check-agent` | 视作"没装"→触发 `Install` 重装；重装后提示"就绪" |

**验收清单**：
- [ ] Phase 0–3 按序执行，各 build gate 全过
- [ ] `make build` 干净通过
- [ ] `go test ./internal/aibp/ensure_agents` 全绿
- [ ] T1–T13 全部通过
- [ ] 端到端：全新环境运行 `:check-agent` 自动装好 aibp-pi，之后 Alt-Enter 能发消息
- [ ] 对 micro 原生代码的侵入：新增 4 文件 + `globals.go`（+1 行，已 microNeo 文件）。`aibp/registry.go` 的两处改动（`major`→`MajorVersion` 导出 + `Discover` 硬编码修复）不算侵入原生——该目录是 microNeo 自建。`command.go` 完全不动

---

## 九、风险与已知限制

| # | 风险 | 触发条件 | 应对 |
|---|------|---------|------|
| R1 | **首次安装需要网络** | `pi install` 触发 `npm install` 需联网 | 用户能装 pi 就有 npm 环境，假设合理；离线时 D3 报错提示 |
| R2 | **对 micro 原生代码的侵入** | 误改 micro 原生文件 | 唯一触碰的原生文件是 `globals.go`（+1 行，已 microNeo 文件）。`registry.go` 在 `internal/aibp/`（microNeo 自建目录）不算原生；`command.go` 完全不动；验收用 `git diff` 核对 |

**回滚**：改动的原生文件 `git checkout` 取回；新增文件直接删除。

---

## 附：未来扩展

### 新 agent 接入（aibp 域）

`AgentEnsurer` 接口让每个 agent 扩展的自举高度统一。未来加新 agent = 发 npm 包 + 一个新 ensure 文件：

```go
// internal/aibp/ensure_agents/ensure_opencode.go（未来）
type OpencodeEnsurer struct{}
var _ AgentEnsurer = OpencodeEnsurer{}

func (OpencodeEnsurer) AgentName() string { return "opencode" }
func (OpencodeEnsurer) HasAgent() bool { ... }       // opencode 的发现机制
func (OpencodeEnsurer) HasAIBP() bool { ... }   // opencode 的扩展登记机制（可能不是 npm）
func (OpencodeEnsurer) AIBPVersion() (string, error) { ... } // 读 opencode 扩展的协议声明
func (OpencodeEnsurer) InstallAIBP() error { ... }
```

`command_neo.go` 注册新命令（或扩展现有命令）：

```go
// 方案 A：每个 agent 一个命令（加进现有 RegisterCommands）
func RegisterCommands() {
	MakeCommand("check-agent",          (*BufPane).CheckAibpCmd,          nil) // 检查 pi
	MakeCommand("check-agent-opencode", (*BufPane).CheckAibpOpencodeCmd, nil) // 未来
}

// 方案 B：一个命令检查所有
func (h *BufPane) CheckAibpCmd(args []string) {
	// 遍历所有 ensure_agents 实现，逐个 Ensure
}
```

各 agent 独立发版、独立升级，互不耦合——但 `Ensure` 编排逻辑和接口契约永远不变。
