# `ensure_claude.go` 实施计划

> 位置：本文件与 `ensure_claude.go`（待建）同目录。
> 对照参考：`ensure_pi.go` / `ensure_opencode.go` + `ensure_opencode-实施计划.md`。
> 本计划是**落地 checklist**：改哪个文件、写什么代码、为什么、怎么验。照着做即可。
> 状态：PLAN 模式，未经用户许可不改代码。

---

## 0. 背景

### 0.1 目标

把 claude 接入 `ensure_agents` 自愈编排：`micro -check-agent` 时，对已装的 claude 自动检测/安装/升级 `aibp-claude`（marketplace 形态），与 pi/opencode 行为一致。

### 0.2 关键差异：claude 的安装形态与 pi/opencode 不同

| 维度 | pi | opencode | **claude** |
|---|---|---|---|
| 安装形态 | `npm:aibp-pi` 写进 settings.json `packages[]` | `aibp-opencode` 写进 tui.json `plugin[]` | marketplace install → `installed_plugins.json` + `settings.json.enabledPlugins` |
| 协议来源 | `<agentDir>/npm/node_modules/aibp-pi/package.json` | `<cache>/packages/aibp-opencode@<v>/node_modules/.../package.json` | `<installPath>/package.json`（installPath 由 installed_plugins.json 直接给出） |
| 原生 update 命令 | `pi update` | ❌ 无（要 uninstall+reinstall） | ✅ `claude plugin update`（**比 opencode 简单**） |
| 源码安装可检测 | ✅（配置里有路径串） | ✅（tui.json 里有路径） | ❌（见 §0.3） |

### 0.3 设计决策：claude **不检测源码安装**

claude 的 `--plugin-dir` 是 **session-only 命令行 flag**，磁盘无痕——不像 pi/opencode 把插件列表持久化到配置文件。microNeo 从文件系统**根本无法**知道 claude 是否被以 `--plugin-dir` 启动过。

**决策**：`AIBPVersion()` 不设 `isSource=true` 分支。claude 走纯 marketplace 检测：
- 装了 + enabled + 协议可读 → `(major, minor, false)`
- 否则 → `(0, 0, false)` → 触发 `InstallAIBP`

**取舍**：一个用 `--plugin-dir` 做 aibp-claude 开发的人，若跑 `micro -check-agent`，microNeo 会给他装上 marketplace 版。但因 `--plugin-dir` 在 session 内**优先于**已装版本（官方设计），运行时无害——只是 `installed_plugins.json` 多一条他没主动要的记录。这是可接受的最小代价，文档（README）已说明 `--plugin-dir` 是开发期 escape hatch。

> 本节是本计划与 pi/opencode 计划最大的不同点，评审时重点看这里。

---

## 1. 改动范围

| 文件 | 改动类型 | 说明 |
|---|---|---|
| `ensure_claude.go` | **新建** | §2 全部代码（~130 行） |
| `ensure_claude_test.go` | **新建** | §3：`TestClaudeAIBPVersion` |
| `ensure.go` | **改一行** | `allEnsurers` 追加 `ClaudeEnsurer{}` |

`ensure.go` 的 `AgentEnsurer` 接口**不动**（claude 不需要 `UninstallAIBP`——claude 有原生 `claude plugin uninstall`，未来若加卸载编排再统一提升接口，同 opencode 的处理）。

---

## 2. 代码设计（`ensure_claude.go`）

### 2.1 常量与类型

```go
package ensure_agents

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/micro-editor/micro/v2/internal/aibp"
)

const (
	// 以下三个常量的取值已实测确认（2026-07-03，claude 2.1.199）：
	//   `claude plugin marketplace add sollawen/microNeo-plugins` 后，known_marketplaces.json
	//   存的 key 就是 "microNeo-plugins"；installed_plugins.json 的 plugin key 就是
	//   "aibp-claude@microNeo-plugins"。若未来 marketplace 改名，三常量需同步。
	claudeMarketplaceRepo = "sollawen/microNeo-plugins"                              // owner/repo，喂给 `claude plugin marketplace add`
	claudeMarketplaceName = "microNeo-plugins"                                      // marketplace 名（known_marketplaces.json 的 key）
	claudePluginKey       = "aibp-claude@" + claudeMarketplaceName                    // installed_plugins.json / enabledPlugins / install|enable|update 的 key
)

// ClaudeEnsurer 是 claude 的 aibp 扩展自举实现。
type ClaudeEnsurer struct{}

var _ AgentEnsurer = ClaudeEnsurer{} // 编译期接口断言
```

### 2.2 `AgentName` / `HasAgent`（与 pi/opencode 同构）

```go
func (ClaudeEnsurer) AgentName() string { return "claude" }

func (ClaudeEnsurer) HasAgent() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}
```

### 2.3 配置目录（`claudeConfigDir`）

claude 原生支持 `CLAUDE_CONFIG_DIR` 覆盖——**已隔离测试验证**：`CLAUDE_CONFIG_DIR=<tmp> claude plugin install ...` 后，`installed_plugins.json` / `settings.json` / cache 全部落在 `<tmp>/` 下，布局与默认 `~/.claude` 完全一致：

```
<cfg>/plugins/installed_plugins.json   ← 安装登记表（version + installPath）
<cfg>/plugins/known_marketplaces.json  ← marketplace 登记表
<cfg>/plugins/cache/<market>/<pkg>/<ver>/package.json  ← 协议来源（installPath 直指这里）
<cfg>/settings.json                    ← enabledPlugins
```

```go
// claudeConfigDir 返回 claude 的配置目录。
// claude 原生支持 CLAUDE_CONFIG_DIR 覆盖（已隔离测试：set 后 plugin install/list 全走该目录，
// 布局与默认 ~/.claude 一致）；未设 → ~/.claude。
func claudeConfigDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}
```

> `CLAUDE_CONFIG_DIR` 让测试可以 `t.Setenv` 干净重定向（同 opencode 用 `XDG_CONFIG_HOME`）。

### 2.4 检测（`AIBPVersion`）—— 本计划核心

三步：① installed_plugins.json 有 record → ② settings.json 里 enabled → ③ record.installPath 下 package.json 的 `aibp.protocol` 可读。

```go
// claudeInstalledRecord 读 installed_plugins.json 里 aibp-claude 的条目。
// 返回 (installPath, version, ok)。文件缺失/损坏/无条目 → ok=false。
//
// installed_plugins.json 结构（claude 实际格式，已读真实文件确认）：
//   { "version": 2, "plugins": { "<plugin>@<market>": [ { scope, installPath, version, ... } ] } }
// 每个 key 映射到一个 slice（支持多 scope 同名）；取第一个（microNeo 装的就是 user scope）。
func claudeInstalledRecord() (installPath, version string, ok bool) {
	b, err := os.ReadFile(filepath.Join(claudeConfigDir(), "plugins", "installed_plugins.json"))
	if err != nil {
		return "", "", false
	}
	var doc struct {
		Plugins map[string][]struct {
			InstallPath string `json:"installPath"`
			Version     string `json:"version"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return "", "", false
	}
	entries := doc.Plugins[claudePluginKey]
	if len(entries) == 0 {
		return "", "", false
	}
	return entries[0].InstallPath, entries[0].Version, true
}

// claudeEnabled 读 settings.json 的 enabledPlugins[claudePluginKey]。
// 文件缺失/损坏/无 enabledPlugins 键 → false。
// 注：enabledPlugins 键缺失时 EnabledPlugins 是 nil map，nil map 查询返回零值 false（安全，不 panic）。
func claudeEnabled() bool {
	b, err := os.ReadFile(filepath.Join(claudeConfigDir(), "settings.json"))
	if err != nil {
		return false
	}
	var s struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return false
	}
	return s.EnabledPlugins[claudePluginKey] // nil map 查询安全
}

// AIBPVersion：识别 claude 已装的 aibp（仅 marketplace 形态，见 §0.3）。
//   - installed_plugins.json 有 aibp-claude 条目 + settings.json enabled + installPath 下 package.json 可读
//     → 读 aibp.protocol → (major, minor, false)
//   - 任一不满足 → (0, 0, false)（触发 InstallAIBP）
func (ClaudeEnsurer) AIBPVersion() (int, int, bool) {
	installPath, _, ok := claudeInstalledRecord()
	if !ok {
		return 0, 0, false
	}
	if !claudeEnabled() {
		return 0, 0, false // 装了但被 disable → 视为未就绪，让 InstallAIBP 自愈（install+enable）
	}
	return claudeReadProtocol(installPath)
}

// claudeReadProtocol 读 <installPath>/package.json 的 aibp.protocol。
// 与 piNpmAIBPVersion / opencodeNpmAIBPVersion 同构。
func claudeReadProtocol(installPath string) (int, int, bool) {
	b, err := os.ReadFile(filepath.Join(installPath, "package.json"))
	if err != nil {
		return 0, 0, false
	}
	var pkg struct {
		AIBP struct {
			Protocol string `json:"protocol"`
		} `json:"aibp"`
	}
	if err := json.Unmarshal(b, &pkg); err != nil {
		return 0, 0, false
	}
	if pkg.AIBP.Protocol == "" {
		return 0, 0, false
	}
	maj, min, parseOK := aibp.ParseProtocol(pkg.AIBP.Protocol)
	if !parseOK {
		return 0, 0, false
	}
	return maj, min, false // marketplace 安装：isSource 恒为 false
}
```

**为什么不重构路径、直接用 installPath**：opencode 要从 tui.json 条目推导 cache 子目录名（`@latest` / `@1.0.2`），坑出 D3。claude 的 `installed_plugins.json` **直接给 installPath**（指到 `<cfg>/plugins/cache/microNeo-plugins/aibp-claude/1.0.0`），省掉这层推导，更鲁棒。

### 2.5 安装（`InstallAIBP`）—— 后置校验模式

不信任 `claude plugin install` 的退出码（已装/未装/disabled 行为不明），改为**装完后读 record 权威校验**：

```go
// claudeMarketplaceAdded 读 known_marketplaces.json 判断 marketplace 是否已登记。
func claudeMarketplaceAdded() bool {
	b, err := os.ReadFile(filepath.Join(claudeConfigDir(), "plugins", "known_marketplaces.json"))
	if err != nil {
		return false
	}
	var doc map[string]json.RawMessage
	return json.Unmarshal(b, &doc) == nil && doc[claudeMarketplaceName] != nil
}

func (ClaudeEnsurer) InstallAIBP() error {
	// 1. marketplace 未登记则补登（已登记跳过，避免重复 add 报错）
	if !claudeMarketplaceAdded() {
		if err := exec.Command("claude", "plugin", "marketplace", "add", claudeMarketplaceRepo).Run(); err != nil {
			return fmt.Errorf("claude marketplace add 失败: %w", err)
		}
	}
	// 2. install（对"已装但 disabled"可能是 no-op，下一步 enable 兜底）
	_ = exec.Command("claude", "plugin", "install", claudePluginKey).Run()
	// 3. enable 兜底（install 默认会 enable；这行专治"已装但被 disable"的自愈场景）
	_ = exec.Command("claude", "plugin", "enable", claudePluginKey).Run()
	// 4. 权威校验：装完该能读到 record（不校验 enabled——enable 失败不阻塞，留给下次自愈）
	if _, _, ok := claudeInstalledRecord(); !ok {
		return fmt.Errorf("claude plugin install 失败：install 后 installed_plugins.json 仍无 aibp-claude 记录（可能 claude 过旧或网络问题）")
	}
	return nil
}
```

**为什么 enable 不校验**：`enable` 命令对"已 enabled"插件的退出码未知（可能非 0）。若把 enable 失败当致命，会误报。装上（record 在）是硬指标；enabled 是软指标——即使本次没 enable 成，下次 `-check-agent` 的 `AIBPVersion` 还会因 disabled 触发自愈，最终收敛。

### 2.6 升级（`UpdateAIBP`）—— 用原生命令，比 opencode 简单

claude **有**原生 `claude plugin update`（opencode 没有，所以要 uninstall+reinstall）。直接用：

```go
func (ClaudeEnsurer) UpdateAIBP() error {
	cmd := exec.Command("claude", "plugin", "update", claudePluginKey)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("claude plugin update 失败: %w", err)
	}
	return nil
}
```

> 注：第三方 marketplace 自动更新默认关闭（README 已述），但**手动** `claude plugin update` 能拉最新 npm 版——这正是 `UpdateAIBP` 要的。

### 2.7 注册到编排（`ensure.go` 改一行）

```go
var allEnsurers = []AgentEnsurer{
	PiEnsurer{},
	OpencodeEnsurer{},
	ClaudeEnsurer{}, // ← 新增
}
```

---

## 3. 测试（`ensure_claude_test.go`）

镜像 `ensure_opencode_test.go` 的 `TestOpencodeAIBPVersion` 结构。全部用 `t.Setenv("CLAUDE_CONFIG_DIR", tmpDir)` 隔离。**只测检测逻辑**（`AIBPVersion`），不测编排（`Ensure` 的 trivial 分支，opencode 计划已论证 mock 价值低）。

```go
package ensure_agents

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeAIBPVersion(t *testing.T) {
	// installPath 在 setup 内部算（dir 在其作用域内），用 %s 占位让 setup 把绝对路径
	// 替换进 installedJSON——避免调用方引用 dir（Lisa review Issue #1：闭包拿不到 dir）。
	const installedTpl = `{"version":2,"plugins":{"aibp-claude@microNeo-plugins":[{"scope":"user","installPath":"%s","version":"1.0.0"}]}}`
	const enabledSettings = `{"enabledPlugins":{"aibp-claude@microNeo-plugins":true}}`
	const disabledSettings = `{"enabledPlugins":{"aibp-claude@microNeo-plugins":false}}`

	setup := func(t *testing.T, installedJSON, settingsJSON, pkgProtocol string) string {
		t.Helper()
		dir, _ := os.MkdirTemp("", "claude-test")
		t.Cleanup(func() { os.RemoveAll(dir) })
		t.Setenv("CLAUDE_CONFIG_DIR", dir)

		installPath := filepath.Join(dir, "cache", "microNeo-plugins", "aibp-claude", "1.0.0")

		if installedJSON != "" { // 含 %s → Sprintf 替换成绝对 installPath
			ip := filepath.Join(dir, "plugins", "installed_plugins.json")
			os.MkdirAll(filepath.Dir(ip), 0755)
			os.WriteFile(ip, []byte(fmt.Sprintf(installedJSON, installPath)), 0644)
		}
		if settingsJSON != "" {
			os.WriteFile(filepath.Join(dir, "settings.json"), []byte(settingsJSON), 0644)
		}
		if pkgProtocol != "__omit__" {
			os.MkdirAll(installPath, 0755)
			pkg := `{"name":"aibp-claude","version":"1.0.0","aibp":{"protocol":"` + pkgProtocol + `"}}`
			os.WriteFile(filepath.Join(installPath, "package.json"), []byte(pkg), 0644)
		}
		return dir
	}

	t.Run("marketplace installed + enabled", func(t *testing.T) {
		setup(t, installedTpl, enabledSettings, "aibp-2.0")
		maj, min, isSrc := (ClaudeEnsurer{}).AIBPVersion()
		if maj != 2 || min != 0 || isSrc {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (2,0,false)", maj, min, isSrc)
		}
	})

	// ... 其余用例按下表，均形如 setup(t, <installedTpl 或 "" 或 空json>, <settings>, <protocol>)
}
```

用例清单：

| 用例 | installed_plugins.json | settings enabledPlugins | cache package.json | 期望 `(maj,min,isSrc)` |
|---|---|---|---|---|
| `marketplace installed + enabled` | 有 entry | true | `aibp-2.0` | `(2,0,false)` |
| `not installed` | 无文件 | — | — | `(0,0,false)` |
| `entry missing` | `{}` 空 plugins | — | — | `(0,0,false)` |
| `installed but disabled` | 有 entry | **false** | `aibp-2.0` | `(0,0,false)` |
| `corrupt installed json` | 乱码 | — | — | `(0,0,false)` |
| `cache package missing` | 有 entry（installPath 指向不存在路径）| true | 不建 | `(0,0,false)` |
| `missing protocol field` | 有 entry | true | 无 `aibp` 键 | `(0,0,false)` |

> 不单列「package.json 权限 000（不可读）」用例：它与「cache package missing」走 `claudeReadProtocol` 同一条 `os.ReadFile` 出错分支（都 `err != nil → (0,0,false)`），覆盖率不增；且权限用例在 CI 以 root 执行时会失效（root 能读 000 文件），反而引入 flaky。（Lisa review Issue #3 的取舍。）

`HasAgent` / `InstallAIBP` / `UpdateAIBP` 不写单测（依赖外部 `claude` 进程与网络，靠 §5 手动验证）——与 opencode 同策略。

---

## 4. 实施顺序

每步可单独编译/验证：

1. **写 `ensure_claude.go`**（§2.1–2.6）→ `make build-quick` 通过（接口断言保证签名对）。
2. **注册 `ensure.go`**（§2.7 一行）→ `make build-quick` 通过。
3. **写 `ensure_claude_test.go`**（§3）→ `go test ./internal/aibp/ensure_agents/... -run Claude -v` 全绿。
4. **手动验证**（§5.2/5.3）。
5. `make build`（项目要求，含 generate）通过。

---

## 5. 验证清单

### 5.1 自动化

```bash
go test ./internal/aibp/ensure_agents/... -run Claude -v
```

- [ ] §3 七个用例全过

### 5.2 手动（已装 → ready；卸载 → 自愈装回）

> 前置：本机当前已装 `aibp-claude@microNeo-plugins`（v1.0.0）。

```bash
# A. 已装状态：应报 ready
micro -check-agent 2>&1 | grep claude
# 期望：claude ... aibp-claude ready (aibp-2.0)

# B. 卸载（模拟损坏/丢失）
claude plugin uninstall aibp-claude@microNeo-plugins
micro -check-agent 2>&1 | grep claude
# 期望：claude ... not installed, installing ... → installed

# C. 装回后再验 ready
micro -check-agent 2>&1 | grep claude
# 期望：ready
```

### 5.3 手动（disabled → 自愈 enable）

```bash
claude plugin disable aibp-claude@microNeo-plugins
micro -check-agent 2>&1 | grep claude
# 期望：AIBPVersion 因 disabled 返回 (0,0,false) → InstallAIBP 跑 install+enable → 收敛
# 验：claude plugin list 应再次显示 enabled
```

> §5.3 是 §2.5 "enable 兜底"的命门验证。若 `claude plugin install` 对已装-disabled 插件的行为本身就重新 enable 了，则 enable 那行是冗余但无害；若不是，则靠它兜底。两种情况都该过。

### 5.3b 手动（settings.json 无 enabledPlugins 键 → 不卡死）

Lisa review Issue #4：理论上若 `claude plugin install` 只写 installed_plugins.json、不写 settings.json 的 enabledPlugins 键，会陷入「装了仍报未就绪」的循环。现代 claude（≥2.1.105，含本机 2.1.199）实测会写该键，但补一条验证消除该风险：

```bash
# D. 删掉 settings.json 的 enabledPlugins 键，模拟异常状态
jq 'del(.enabledPlugins)' ~/.claude/settings.json > /tmp/s.json && cp /tmp/s.json ~/.claude/settings.json
micro -check-agent 2>&1 | grep claude          # 期望：触发 install，且本次/下次收敛到 ready
python3 -c "import json;print(json.load(open('/Users/sollawen/.claude/settings.json')).get('enabledPlugins'))"  # 期望：aibp-claude@microNeo-plugins 又被写回 true
```

### 5.4 回归（pi/opencode 不受影响）

```bash
go test ./internal/aibp/ensure_agents/... -v   # 全量绿（含原有 Pi/Opencode 用例）
micro -check-agent                              # 三个 agent 都报 ready/各自状态
```

---

## 6. 风险

| 风险 | 缓解 |
|---|---|
| `claude plugin install` 对"已装"退出码不明 → 误报失败/成功 | §2.5 后置读 record 权威校验，不依赖退出码 |
| `enable` 对"已 enabled"退出码不明 | §2.5 enable 失败不致命，软指标，下次自愈收敛 |
| `marketplace add` 重复执行报错 | §2.5 先查 `known_marketplaces.json`，已登记跳过 |
| `installPath` 是相对路径 / cache 被挪走 | claude 实测写绝对路径；读不到 → (0,0,false) → 触发重装（自愈） |
| `--plugin-dir` 开发者跑 `-check-agent` 被强装 marketplace | §0.3 已述：运行时无害（`--plugin-dir` session 优先）；属可接受代价 |
| enabledPlugins 在 settings.json 缺失（旧版 claude） | `claudeEnabled` 返回 false → 触发自愈；若 install 后仍无 enabledPlugins 键，会陷入"装了仍报未就绪"——**需 §5.2 实测确认现代 claude 必写 enabledPlugins**（当前 2.1.199 已写，已验证） |

---

## 7. 交付定义（DoD）

- [ ] `ensure_claude.go`（§2.1–2.6）落地，`make build-quick` 通过
- [ ] `ensure.go` 注册 `ClaudeEnsurer{}`（§2.7）
- [ ] `ensure_claude_test.go` 七个用例全绿（§5.1）
- [ ] §5.2 手动流程全过（ready → uninstall → 自愈装回 → ready）
- [ ] §5.3 disabled 自愈通过
- [ ] §5.4 pi/opencode 回归不破
- [ ] `make build`（含 generate）通过
