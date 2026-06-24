# D22 · microNeo 通过 `:check-agent` 初始化 aibp-opencode

> **状态**：方案已定，待实施。`aibp-opencode@1.0.1` 已发布到 npm（[`aibp-opencode`](https://www.npmjs.com/package/aibp-opencode)）；D17/D21 给 pi 搭的 `:check-agent` + `AgentEnsurer` 接口骨架已经验证。
>
> **核心思路**：复用 D17 §五 的 `AgentEnsurer` 接口契约，给 opencode 加一个 `OpencodeEnsurer` 实现，注册到编排器即可——**接口已经预留**，所有改动按 D17 末尾"附：未来扩展"的路径走。

---

## 一、目标

用户下载安装 microNeo 后，运行一次 `:check-agent` 命令，**对所有已装的 agent（pi / opencode）**完成 aibp 扩展的检查与（必要的）安装。之后按 Alt-Enter 就能让 notePane 跟任意一个 agent 通信。

`:check-agent` 对每个 agent 复用 D17 §五 的三步编排：

| 需求问题 | 本方案 |
|---------|--------|
| 如何发现用户装了 opencode | `HasAgent()`：`exec.LookPath("opencode")` |
| 如何判断 opencode 有没有装 aibp-opencode | `HasAIBP()`：读 opencode 的 `tui.json`，找 `aibp-opencode` 条目 |
| 没装时怎么装进去 | `InstallAIBP()`：`opencode plugin aibp-opencode -g`（**带** `-g` 全局，不锁版本） |

**与 D17 的关系**：D17 给 pi 实现了 `PiEnsurer`；本计划加 `OpencodeEnsurer`，并把 `:check-agent` 从「只查 pi」改成「遍历所有 ensurer」。接口和编排逻辑在 D17 §五 已定，**本计划零新增概念**。

**与 D21 的关系**：D21 把 `Ensure()` 改造成 `Reporter` 回调模式，所有"对用户说的话"通过 `InfoBarNow` 实时刷新。本计划直接复用——`Ensure(OpencodeEnsurer{}, InfoBarNow)` 行为完全一致。

**用户怎么知道要运行 `:check-agent`**：同 D17 §一——首次按 Alt-Enter 时 `no receiver found` 提示自然引导。

**不做的事情**（同 D17 §一）：启动时自动检查；git 源安装；运行时弹窗问用户；扩展过旧时自动升级（只检测 + 提示）。

---

## 二、方案选型

### 2.1 命令设计：扩展 `:check-agent`，不加新命令

D17 §附「未来扩展」预留的扩展方式里，本计划选**方式 B（一个命令检查所有）**——把 `:check-agent` 升级为遍历所有注册的 ensurer，逐个 `Ensure()`。三条理由：

1. **用户认知简单**：用户只关心"我的 aibp 能不能用"，不关心"是 pi 还是 opencode"
2. **多 agent 并存场景天然支持**：用户开两个 opencode / 一个 pi + 一个 opencode，跑一次全搞定
3. **避免静默遗漏**：每个 agent 一个命令的方案下，用户只记得 `:check-agent`，opencode 就漏检了

**Skip 策略**：`Ensure()` 对 `HasAgent() == false` 的 agent 静默跳过——用户没装 opencode 就别去骚扰他。这与 D17 §五 编排器里"未装 agent 返回 error"的语义**不冲突**：错误返回是给单 agent 编排的；遍历层面在外层屏蔽错误。

```
// 伪代码
for _, e := range allEnsurers {
    if !e.HasAgent() { continue }  // 未装 → 跳过，不报错
    ensure_agents.Ensure(e, InfoBarNow)
}
```

**未来加新 agent（如 aibp-claude）**：再写一个 `ClaudeEnsurer` + 加入 `allEnsurers` slice 即可，**零侵入**已实现的代码。

### 2.2 opencode 扩展机制差异（vs pi）

D17 给 pi 走的"npm 标准分发"对 opencode 同样适用，差异在各 agent 的 `ensure_<agent>.go` 内部吸收：

| 维度 | pi (D17 §6.2①) | opencode（本计划） |
|------|---------------|------------------|
| 配置目录 | `$PI_CODING_AGENT_DIR` 或 `~/.pi/agent/` | `~/.config/opencode/`（**无 env 覆盖**） |
| 扩展登记表 | `settings.json` 的 `packages[]` | `tui.json` 的 `plugin[]`（TUI-only 插件） |
| 包安装命令 | `pi install npm:aibp-pi` | `opencode plugin aibp-opencode -g` |
| 包 package.json 位置 | `<piAgentDir>/npm/node_modules/aibp-pi/package.json` | `~/.cache/opencode/packages/aibp-opencode@latest/node_modules/aibp-opencode/package.json` |
| 协议字段位置 | `package.json` 的 `aibp.protocol` | 同 |

**关键约束（D19 已验证）**：

- `opencode plugin` 看 `package.json` 有 `main` 字段会把包当 server 插件处理（启动报 `must default export an object with server()`）。`aibp-opencode@1.0.1` 已修：`package.json` 无 `main`、只 `exports["./tui"]`、TUI-only 插件正确写入 `tui.json`
- 包被 opencode 缓存在 `~/.cache/opencode/packages/aibp-opencode@latest/`，发版后不强制刷新。`InstallAIBP()` 用 `-f`（force）触发覆盖，详见 §四
- TUI 插件卸载无 `opencode plugin remove` 子命令——属于 D9「用户自己管」的范畴，本计划不碰

### 2.3 自举流程（幂等）

与 D17 §3.3 一致：

- **常态快路径**：LookPath + 读 `tui.json` + 读已装包 `package.json`，无 spawn
- **首次安装**：spawn `opencode plugin aibp-opencode -g`，约 1-3 秒（npm install 联网）
- **幂等**：重复运行已装且兼容则只读不 spawn
- **自愈**：`tui.json` 有但 `package.json` 缺失/损坏 → 视作「登记但破损」→ 触发 `-f` 重装

---

## 三、系统设计

### 3.1 模块划分

| 文件 | 状态 | 包 | 职责 |
|------|------|---|------|
| `internal/aibp/ensure_agents/ensure_opencode.go` | 新增 | `ensure_agents` | `OpencodeEnsurer` 实现（4 个方法 + 编译期断言） |
| `internal/aibp/ensure_agents/ensure_opencode_test.go` | 新增 | `ensure_agents` | `HasAIBP` 匹配逻辑、`AIBPVersion` 单测 |
| `internal/action/command_neo.go` | 改 | `action` | `CheckAibpCmd` 改为遍历所有 ensurer；注册 `allEnsurers` slice |

**对 micro 原生代码的侵入**：**零**。所有改动都在 microNeo 自有文件：
- `ensure_opencode.go` / `ensure_opencode_test.go` 新增（D17 同款）
- `command_neo.go` 改的是 D21 已经改过的文件，**复用既有 `MakeCommand` + `InfoBarNow`**

### 3.2 协议版本判定的双向语义

完全复用 D17 §3.2 + `aibp.MajorVersion()`：

| `MajorVersion(扩展协议)` vs `MajorVersion(microNeo 协议)` | 含义 | `:check-agent` 行为 |
|----------------------------------------------|------|-------------|
| 相等 | 兼容 | InfoBar 提示"aibp-opencode: 就绪" |
| 扩展 < microNeo | 扩展过旧 | InfoBar 提示用户运行 `opencode plugin aibp-opencode -f`（**D9：不自动升级**） |
| 扩展 > microNeo | microNeo 过旧 | InfoBar 提示"请升级 microNeo" |

**协议版本单一事实来源**：扩展 `package.json` 的 `aibp.protocol`（`aibp-opencode@1.0.1` → `"aibp-1"`）。`aibp-opencode/index.tsx` 启动时也读同一字段写注册表，静态/运行时一致（见 D19 §源码结构）。

### 3.3 opencode 缓存机制对自举的影响

opencode plugin 把 npm 包缓存在 `~/.cache/opencode/packages/<name>@latest/`：

| 场景 | 缓存行为 | 影响 | 应对 |
|------|---------|------|------|
| 首次安装 | `opencode plugin` 触发 npm install → 写入 cache + tui.json | 正常 | 无 |
| 重装同版本 | cache 命中，不重写 | tui.json 已登记 → `Already configured` | `HasAIBP` 仍返回 true ✓ |
| 升级到新版本（npm 上有新版） | cache 命中，不刷新 | 仍指向旧 package.json | `InstallAIBP()` 用 `-f` 强制覆盖 → `opencode plugin` 删旧 cache + 重装 |
| 卸载（用户手动） | cache 文件残留 | 不影响 `HasAIBP`（读 tui.json），可能误报已装 | 用户卸载走 D9 路径，不在 `Ensure` 编排范畴 |

**结论**：`InstallAIBP()` **始终加 `-f`**。开销可忽略（绝大多数情况 cache 命中只是 metadata check），但确保 cache 不陈旧、协议版本读到的就是最新。

> **为何 `-f` 在 `Ensure` 里就触发而不是提示用户**：D17 D9「扩展过旧只提示不自动升级」是对**协议版本过旧**（语义破坏）的保守；`InstallAIBP` 的触发条件是「没装 / 包损坏」（必要修复），`-f` 是修复手段的一部分，不算"自动升级"。

### 3.4 遍历编排器（`allEnsurers`）

最简实现——一个包内 slice：

```go
// internal/aibp/ensure_agents/ensure.go（同包扩展）
//
// AllEnsurers 注册所有已知 agent 的自举实现。新加 agent 只需追加一行。
var AllEnsurers = []AgentEnsurer{
    PiEnsurer{},
    OpencodeEnsurer{},
}
```

**遍历逻辑**（在 `command_neo.go:CheckAibpCmd`）：

```go
for _, e := range ensure_agents.AllEnsurers {
    if !e.HasAgent() { continue }         // 未装 → 跳过，不报错
    _ = ensure_agents.Ensure(e, InfoBarNow) // 内部错误已通过 reporter 报给 InfoBar
}
```

**设计要点**：
- `Ensure` 返回的 error 不向上冒——`Reporter` 已把每条业务消息实时打到 InfoBar；error 仅控制流，无显示用途
- `HasAgent()` 静默跳过**对所有 ensurer 一致**——未来加 aibp-claude 也走这条规则
- 单 agent 失败**不影响其他**——比如 opencode 装失败，pi 已装的就绪消息照常显示

---

## 四、设计决策

| # | 决策 | 理由 |
|---|------|------|
| D1 | **命令：扩展 `:check-agent` 遍历所有 ensurer**（选 D17 末尾方案 B） | 用户认知简单；多 agent 并存天然支持；未来扩展零侵入 |
| D2 | **遍历时 `HasAgent()==false` 静默跳过** | 用户没装 opencode 别去骚扰；D9「扩展过旧不自动升级」思想延伸 |
| D3 | **`InstallAIBP()` 始终带 `-f`** | 防 opencode 缓存陈旧影响协议版本读数；开销可忽略 |
| D4 | **`opencode plugin` 失败不兜底** | 唯一现实风险是 opencode 太旧没有 plugin 子命令；同 D17 D3 |
| D5 | **不 pin 版本**（spec 用 `aibp-opencode` 不带 `@版本`） | 与 D17 D5 一致；升级由 `opencode plugin aibp-opencode -f` 触发 |
| D6 | **`allEnsurers` 是包内 slice**（不在 `command_neo.go`） | 命令文件只调 `AllEnsurers`；新加 ensurer 不需碰 `command_neo.go` |
| D7 | **错误不向上冒**（`Ensure` error 在 `CheckAibpCmd` 内被忽略） | Reporter 已实时反馈；error 仅控制流 |
| D8 | **配置文件路径硬编码 `~/.config/opencode/`** | opencode 无 `PI_CODING_AGENT_DIR` 类环境变量覆盖机制；用户改配置目录需自管，不在 `Ensure` 编排范畴 |
| D9 | **扩展过旧只提示，不自动 `InstallAIBP()` 升级** | 同 D17 D9；`-f` 是「重装同版本/必要时拿新版」的工具，不是"自动升级到任意新版"的隐式行为——提示保持保守 |
| D10 | **配置登记表读 `tui.json`（不是 `opencode.json`）** | `aibp-opencode` 是 TUI-only 插件（D19 §源码结构）；opencode plugin 自动写入 `tui.json`；读 `opencode.json` 会漏 |

---

## 五、实现规格

### 5.1 `internal/aibp/ensure_agents/ensure_opencode.go`

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

const aibpOpencodeSpec = "aibp-opencode" // 不 pin 版本（D5）

// OpencodeEnsurer 是 opencode 的 aibp 扩展自举实现。
type OpencodeEnsurer struct{}

var _ AgentEnsurer = OpencodeEnsurer{} // 编译期接口断言

func (OpencodeEnsurer) AgentName() string { return "opencode" }

func (OpencodeEnsurer) HasAgent() bool {
	_, err := exec.LookPath("opencode")
	return err == nil
}

// opencodeConfigDir 返回 opencode 的全局配置目录。
//
// 派生路径（与 opencode 1.17.9 的 XDG 行为一致）：
//   tui.json:           <configDir>/tui.json        （TUI 插件登记表；D19 §源码结构）
//   opencode.json:      <configDir>/opencode.json   （server 插件 + MCP；本计划不读）
//   package.json:       <configDir>/../cache/opencode/packages/aibp-opencode@latest/node_modules/aibp-opencode/package.json
//
// 注意：opencode 无 PI_CODING_AGENT_DIR 类环境变量覆盖机制——D8 硬编码。
func opencodeConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "opencode")
}

func opencodeTuiPath() string {
	return filepath.Join(opencodeConfigDir(), "tui.json")
}

// HasAIBP：检查 opencode tui.json 的 plugin[] 里是否已登记 aibp-opencode。
//
// 形态（实测）：
//   - "aibp-opencode"           （unpinned，本计划期望形态）
//   - "aibp-opencode@1.0.1"     （pinned 到具体版本；用户手改或历史遗留）
//
// 兼容两种（D17 D5 同款策略）。
func (OpencodeEnsurer) HasAIBP() bool {
	b, err := os.ReadFile(opencodeTuiPath())
	if err != nil {
		return false // 读不到 → 视为未登记
	}
	var s struct {
		Plugin []string `json:"plugin"`
	}
	if json.Unmarshal(b, &s) != nil {
		return false
	}
	for _, p := range s.Plugin {
		if p == aibpOpencodeSpec || strings.HasPrefix(p, aibpOpencodeSpec+"@") {
			return true
		}
	}
	return false
}

// AIBPVersion：读已装扩展的 package.json 的 aibp.protocol 字段。
//
// 包位置（实测 opencode 1.17.9）：
//   ~/.cache/opencode/packages/aibp-opencode@latest/node_modules/aibp-opencode/package.json
//
// 注意 @latest 缓存 key：D3 通过 InstallAIBP 用 -f 保证 cache 不陈旧。
// 若 cache 文件本身被外部破坏（用户手 rm -rf），Ensure() 会视作「没装」触发 Install 自愈。
func (OpencodeEnsurer) AIBPVersion() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	pkgPath := filepath.Join(
		cache, "opencode", "packages", "aibp-opencode@latest",
		"node_modules", "aibp-opencode", "package.json",
	)
	b, err := os.ReadFile(pkgPath)
	if err != nil {
		return "", err
	}
	var pkg struct {
		AIBP struct {
			Protocol string `json:"protocol"`
		} `json:"aibp"`
	}
	if err := json.Unmarshal(b, &pkg); err != nil {
		return "", err
	}
	if pkg.AIBP.Protocol == "" {
		return "", fmt.Errorf("package.json 缺少 aibp.protocol 字段（视为没装）")
	}
	return pkg.AIBP.Protocol, nil
}

func (OpencodeEnsurer) InstallAIBP() error {
	// -g 全局（写入 ~/.config/opencode/tui.json，D8）
	// -f 强制（覆盖 @latest 缓存，D3）
	cmd := exec.Command("opencode", "plugin", aibpOpencodeSpec, "-g", "-f")
	if err := cmd.Run(); err != nil {
		// D4：opencode plugin 失败不兜底。唯一现实风险是 opencode 太旧没有 plugin 子命令。
		return fmt.Errorf("opencode plugin 失败（可能 opencode 过旧，请升级 opencode）: %w", err)
	}
	return nil
}
```

### 5.2 `internal/aibp/ensure_agents/ensure_opencode_test.go`

镜像 D17 `ensure_pi_test.go` 的结构，覆盖：

| 测试 | 覆盖场景 |
|------|---------|
| `TestHasAIBP/unpinned_spec_matches` | `"aibp-opencode"` 在 `plugin[]` 里 → true |
| `TestHasAIBP/pinned_spec_matches` | `"aibp-opencode@1.0.1"` → true |
| `TestHasAIBP/no_aibp_entry` | plugin[] 里是其他插件 → false |
| `TestHasAIBP/tui_json_missing` | 文件不存在 → false |
| `TestHasAIBP/tui_json_corrupt` | 非 JSON → false |
| `TestAIBPVersion/normal` | cache 里 package.json 有 `aibp.protocol` → 返回协议串 |
| `TestAIBPVersion/package_json_missing` | cache 文件缺失 → error（Ensure 触发重装） |
| `TestAIBPVersion/package_json_corrupt` | 合法 JSON 但非 JSON → error |
| `TestAIBPVersion/protocol_field_missing` | 有 name/version 但无 aibp.protocol → error（Ensure 触发重装） |

`tui.json` 路径在测试里用 `t.Setenv("XDG_CONFIG_HOME", dir)` 覆盖（与 `pi-test` 同款套路）。

### 5.3 `internal/aibp/ensure_agents/ensure.go` —— 加 `AllEnsurers`

```go
// 在 ensure.go 末尾追加（不影响既有 Ensure / Reporter / 错误变量）

// AllEnsurers 注册所有已知 agent 的 aibp 扩展自举实现。
// 新加 agent 只需追加一行：把对应的 <Agent>Ensurer{} 加进 slice。
//
// 注意：AllEnsurers 与 Pi/Opencode/... 文件的相对顺序无所谓，
// 但建议把"主要 / 优先 / 更普及"的 agent 放前面（D11 §4.1 名字池顺序逻辑类似）。
var AllEnsurers = []AgentEnsurer{
	PiEnsurer{},
	OpencodeEnsurer{},
}
```

### 5.4 `internal/action/command_neo.go` —— 改 `CheckAibpCmd`

```go
// CheckAibpCmd 是 :check-agent 命令的处理函数。
//
// 遍历所有已知 agent（pi / opencode / 未来的 aibp-claude...），
// 对每个已装的 agent 运行 ensure 编排（HasAgent → HasAIBP → AIBPVersion → InstallAIBP）。
// 未装的 agent 静默跳过（不要去骚扰没用 opencode 的 pi 用户）。
//
// Ensure 内部所有需要告诉用户的消息都通过 reporter 通知，
// reporter 直接用 InfoBarNow：签名匹配，无需闭包。
// 单 agent 错误不中断后续 agent——错误信息已通过 reporter 实时反馈到 InfoBar。
func (h *BufPane) CheckAibpCmd(args []string) {
	for _, e := range ensure_agents.AllEnsurers {
		if !e.HasAgent() {
			continue // 未装 → 跳过（D2）
		}
		_ = ensure_agents.Ensure(e, InfoBarNow)
	}
}
```

**关键改动点**（vs D21 现状）：
- `_ = ensure_agents.Ensure(ensure_agents.PiEnsurer{}, InfoBarNow)` → 替换为 for-range 遍历 `AllEnsurers`
- 单行改动，但语义从"只查 pi"变成"全查"

`RegisterCommands` / `InfoBarNow` / `TestInfoCmd` 不动。

---

## 六、实施计划

每个 Phase 通过自己的 build gate 才进入下一个。

### Phase 0 — 前置基线（已通过 ✅）

| 检查项 | 验证方法 | 状态 |
|--------|---------|------|
| 编译通过 | `make build` 成功 | ✅ |
| `aibp-opencode` 在 npm 上可装 | `opencode plugin aibp-opencode -g` 成功；opencode 启动正常加载 | ✅（[`aibp-opencode@1.0.1`](https://www.npmjs.com/package/aibp-opencode)） |
| `AgentEnsurer` 接口骨架可用 | D17 Phase 1 已实施并测过 | ✅ |
| `Ensure(e, Reporter)` 实时刷 InfoBar 验证 | D21 U1-U7 全过 | ✅ |

基线全过。回滚靠 git。

### Phase 1 — `OpencodeEnsurer` 实现（可单测）

| 改动 | 文件 |
|------|------|
| 新增 opencode 自举 | `internal/aibp/ensure_agents/ensure_opencode.go`（见 §5.1） |
| 新增单测 | `internal/aibp/ensure_agents/ensure_opencode_test.go`（见 §5.2） |
| 加 `AllEnsurers` slice | `internal/aibp/ensure_agents/ensure.go`（见 §5.3） |

**Build gate**：`make build` 过；`go test ./internal/aibp/ensure_agents` 全绿。此 Phase 不改 `command_neo.go`，运行期行为不变。

### Phase 2 — 改造 `:check-agent` 为遍历

| 改动 | 文件 |
|------|------|
| 改 `CheckAibpCmd` 为遍历 `AllEnsurers` | `internal/action/command_neo.go`（见 §5.4） |

**Build gate**：`make build` 过；既有 pi 路径测试矩阵（D17 T1-T13）仍通过；新增 opencode 测试矩阵（见 §七）。

### Phase 3 — 测试矩阵 + 端到端

跑 §七 全部测试。

---

## 七、测试与验收

**前置**（每次测试前重置）：

| 资源 | 重置方法 |
|------|---------|
| opencode 扩展 | `jq 'del(.plugin[] \| select(. == "aibp-opencode"))' ~/.config/opencode/tui.json > /tmp/x && mv /tmp/x ~/.config/opencode/tui.json` + `rm -rf ~/.cache/opencode/packages/aibp-opencode@latest/` |
| pi 扩展 | `pi remove npm:aibp-pi` + 清 settings.json |
| 进程 | `killall opencode pi microneo` |

### 7.1 opencode 专项（新增）

| # | 场景 | 前置条件 | 步骤 | 期望 |
|---|------|---------|------|------|
| O1 | 未装 opencode | PATH 去掉 opencode | 运行 `:check-agent` | 跳过 opencode（静默）；只查 pi |
| O2 | 首次安装 | tui.json 无 aibp-opencode，cache 无包 | 运行 `:check-agent` | InfoBar 闪 `aibp-opencode downloading....` → `installed` → `ready`；`tui.json` 含 `"aibp-opencode"`；`~/.cache/opencode/packages/aibp-opencode@latest/` 落地 |
| O3 | 常态快路径 | O2 后再运行 | 运行 `:check-agent` | 不 spawn `opencode plugin`；InfoBar 提示"aibp-opencode ready" |
| O4 | 登记丢失 | O2 后从 tui.json 删 aibp-opencode 条目 | 运行 `:check-agent` | 重新 `opencode plugin -g -f`；tui.json 重新登记 |
| O5 | `opencode plugin` 失败 | PATH 放一个无 plugin 子命令的假 opencode | 运行 `:check-agent` | InfoBar 提示升级 opencode |
| O6 | 扩展协议过旧 | 改已装包 `package.json` 的 `aibp.protocol` 为 `"aibp-0"` | 运行 `:check-agent` | InfoBar 提示运行 `opencode plugin aibp-opencode -f`（**D9：不自动升级**） |
| O7 | microNeo 协议过旧 | 改已装包 `package.json` 的 `aibp.protocol` 为 `"aibp-2"` | 运行 `:check-agent` | InfoBar 提示升级 microNeo |
| O8 | 端到端 | O2 后 | 开 opencode → microNeo Alt-Enter → 写一行 → Alt-Enter | notePane 打开；opencode 收到消息（带 message 路径） |
| O9 | 幂等 | O2 后连运行两次 | 第二次 `:check-agent` | 不 spawn install；都提示"aibp-opencode ready" |
| O10 | 自愈：package.json 损坏 | O2 后 `rm ~/.cache/opencode/packages/aibp-opencode@latest/node_modules/aibp-opencode/package.json` | 运行 `:check-agent` | 视作"没装"→触发 `-f` 重装 → 提示"aibp-opencode ready" |
| O11 | pinned 历史遗留 | 手动 `opencode plugin aibp-opencode@1.0.0` 后改协议为 `aibp-0` | 运行 `:check-agent` | InfoBar 提示扩展协议过旧（HasAIBP 兼容 pinned 形态） |
| O12 | 自定义 XDG_CONFIG_HOME | `XDG_CONFIG_HOME=/tmp/fake_xdg`；手工建 `fake_xdg/opencode/tui.json` 登记 aibp-opencode + 装包到 `~/.cache/opencode/packages/aibp-opencode@latest/` | 运行 `:check-agent` | `HasAIBP` 返回 true（读到 `/tmp/fake_xdg/opencode/tui.json`）；不重装；InfoBar 提示"ready" |

### 7.2 多 agent 遍历场景（核心验收）

| # | 场景 | 前置条件 | 步骤 | 期望 |
|---|------|---------|------|------|
| M1 | pi + opencode 双装首次 | 全新环境 | 运行 `:check-agent` | InfoBar 依次显示两套流程：pi 先就绪 → opencode 提示 downloading → installed → ready |
| M2 | 只装 opencode（无 pi） | pi 不在 PATH；opencode 在 | 运行 `:check-agent` | 只跑 opencode 流程；pi 静默跳过 |
| M3 | 只装 pi（无 opencode） | pi 在；opencode 不在 PATH | 运行 `:check-agent` | 只跑 pi 流程；opencode 静默跳过（**关键回归点**） |
| M4 | opencode 装失败 / pi 已就绪 | opencode plugin 失败模拟（O5 条件） | 运行 `:check-agent` | pi ready 消息正常显示；opencode 失败消息显示；两段消息都在 InfoBar 里按序出现，互不干扰 |

### 7.3 pi 专项（回归 D17 测试矩阵）

| # | 场景 | 期望 |
|---|------|------|
| P1-P13 | D17 T1-T13 全部 | 行为不变（M3 等价于 D17 T2「未装 pi 时 HasAgent=false」） |

### 7.4 验收清单

- [ ] Phase 0–3 按序执行，各 build gate 全过
- [ ] `make build` 干净通过
- [ ] `go test ./internal/aibp/ensure_agents` 全绿（含新增 opencode 测试）
- [ ] O1-O12 全部通过
- [ ] M1-M4 全部通过（遍历行为正确，错误隔离）
- [ ] P1-P13 全部通过（pi 路径零回归）
- [ ] 端到端：全新环境运行 `:check-agent`，pi + opencode 双装就绪；之后 Alt-Enter 能往任一 agent 发消息
- [ ] 对 micro 原生代码的侵入：仅 `command_neo.go`（1 函数体改 1 行为 for 循环）。其余改动全在 `ensure_agents/` 子包内（新增 + 既有 ensure.go 末尾追加 5 行 slice）

---

## 八、风险与已知限制

| # | 风险 | 触发条件 | 应对 |
|---|------|---------|------|
| R1 | **首次安装需要网络** | `opencode plugin` 触发 npm install 需联网 | 用户能装 opencode 就有 npm 环境，假设合理；离线时 D4 报错提示 |
| R2 | **`opencode plugin` 没有 uninstall** | 用户想卸载 aibp-opencode | 不在 `Ensure` 编排范畴；D9 思想，用户自己管（见 D19 README §卸载） |
| R3 | **opencode 无 `PI_CODING_AGENT_DIR` 类环境变量覆盖** | 用户改 XDG_CONFIG_HOME 时路径跟踪 | 测试用 `XDG_CONFIG_HOME` 覆盖（O12）；正式场景下 XDG_CONFIG_HOME 本就是规范的环境变量，硬编码 `~/.config` 已经是绝大多数用户的真实情况 |
| R4 | **`@latest` 缓存可能被外部破坏** | 用户手 `rm -rf ~/.cache/opencode/packages/aibp-opencode@latest/` | `Ensure` 走「读不到 package.json → 视作没装 → `-f` 重装」自愈路径（O10） |
| R5 | **遍历时单 agent 失败污染其他 agent 的 InfoBar 输出** | opencode 装失败时 InfoBar 同时显示 pi ready + opencode 失败 | M4 测试矩阵覆盖；设计上 `Ensure` 单次调用的所有 reporter 消息是原子的，不会与另一 `Ensure` 交错（串行 for 循环） |
| R6 | **未来加新 agent 时容易忘记加入 `AllEnsurers`** | 加 aibp-claude 时只写了 `ensure_claude.go`，没加 slice | 编译期 `AllEnsurers` 是显式构造，新 agent 加进去才会被遍历；可考虑加单测断言 slice 长度 ≥ 2，作为 lint 防退化（**不在本计划范围内**） |

**回滚**：
- `command_neo.go`：`git checkout` 恢复 D21 状态（单 agent 版本）
- `ensure_opencode.go` / `ensure_opencode_test.go`：直接删除
- `ensure.go`：去掉追加的 `AllEnsurers` slice

---

## 九、相关文档

- **D17** (`docs/agent-comm/D17-初始化aibp-pi.md`)：`AgentEnsurer` 接口 + `Ensure` 编排的来源；本计划零新增概念，全部沿用
- **D19** (`docs/agent-comm/D19-aibp-opencode.md`)：`aibp-opencode` TUI 插件的设计与踩坑记录；本计划的 InstallAIBP / tui.json 行为在 D19 §源码结构里已描述
- **D21** (`docs/agent-comm/D21-agent初始化的交互.md`)：`Ensure` 的 `Reporter` 模式 + `InfoBarNow` 的实现；本计划直接复用
- **`aibp-agents/opencode/README.md`**：用户视角的安装 / 验证 / 卸载说明（与本计划互补，本计划是 microNeo 内部如何检测安装）

---

## 附：未来扩展（aibp-claude 等）

**新加 agent 只需三步**，零侵入既有代码：

1. 写 `internal/aibp/ensure_agents/ensure_<agent>.go`（4 方法 + 编译期断言）
2. 写 `ensure_<agent>_test.go`（单测覆盖 HasAIBP / AIBPVersion 各路径）
3. 在 `ensure.go` 的 `AllEnsurers` slice 里追加一行 `<Agent>Ensurer{}`

`command_neo.go` 不动（已经在 for-range 遍历里）。`Ensure` 编排器不动。`InfoBarNow` 不动。