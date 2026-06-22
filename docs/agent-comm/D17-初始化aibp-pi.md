# D17 · microNeo 启动时自动初始化 aibp-pi

> **回答** [`如何初始化aibp.md`](./如何初始化aibp.md) 提出的三个问题。
>
> **状态**：方案已定，待实施。aibp-pi 已发布到 npm（[`aibp-pi@1.0.1`](https://www.npmjs.com/package/aibp-pi)），端到端链路已验证。

---

## 一、目标

用户下载安装 microNeo 后，打开一个 `.md` 文件、按 Alt-Enter，notePane 就能跟 pi 通信——**不需要用户手动安装任何扩展**。

为此 microNeo 启动时自动完成三件事（对应需求的三个问题）：

| 需求问题 | 本方案 |
|---------|--------|
| 如何发现用户装了 pi | `HasAgent()`：`exec.LookPath("pi")` |
| 如何判断 pi 有没有装 aibp-pi | `HasExtension()`：读 `~/.pi/agent/settings.json` 的 `packages`，找 `npm:aibp-pi` 条目 |
| 没装时怎么装进去 | `Install()`：`pi install npm:aibp-pi`（unpinned，不锁版本） |

**非目标**：git 源安装、运行时询问用户。

---

## 二、方案：npm 标准分发

aibp-pi 作为独立 npm 包发布（`npm:aibp-pi`，已上线）。microNeo 启动时检查 pi 的 `settings.json`，若没有登记 aibp-pi 就调 `pi install` 装上。

```
运行期（microNeo 每次启动）
──────────────────────────────────────────────────────
① HasAgent？     exec.LookPath("pi")              → 没有就静默跳过
② HasExtension？ 读 settings.json 找 npm:aibp-pi   → 有则进入版本校验
③ 否则 Install   exec pi install npm:aibp-pi      → pi 自己 npm install + 写 settings.json
④ 版本校验       读已装扩展的 aibp.protocol        → 主版本不符则触发升级路径
```

**为什么是 npm 而不是本地 copy**：

- **职责解耦**：aibp-pi 改了发 npm 包，microNeo 不用跟着发版
- **机制极简**：pi 自己管 `npm install`、管安装目录（`~/.pi/agent/npm/`）、管 settings.json 登记——microNeo 只调一条命令
- **模型统一**：未来 aibp-opencode / aibp-claude 都是同一个模式——发 npm 包 + 五个接口
- **用户可控**：`pi list` 看得到，`pi remove` 能卸载，`pi update --extensions` 能升级

**不 pin 版本**：`Install()` 用 `npm:aibp-pi`（不带 `@版本号`）。pin 死会让 `pi update --extensions` 跳过升级，与"独立发版、独立升级"初衷相悖。兼容历史遗留的 pinned spec（`npm:aibp-pi@x.y.z`）——见 `HasExtension` 实现备注。

---

## 三、系统设计

### 3.1 模块划分

新增独立子领域 `internal/startup/`，承担"启动期应用层工作"。依赖方向单向：

```
main → action.globals ──┐
                        ▼
                   internal/startup ──┬──▶ action（用 InfoBar 报错）
                        │              ├──▶ aibp（协议版本常量、major()）
                        │              └──▶ 各 agent 的 ensure-*.go
                        │
              internal/aibp（aibp 工具：名字池/发送/发现/协议常量）
```

| 模块 | 职责 |
|------|------|
| `internal/startup` | 启动期编排 + 各 `ensure-<agent>.go` 自举逻辑 |
| `internal/aibp` | 协议常量（`Protocol`）、`major()` 解析器、aibp 工具本身 |

**为什么是独立子领域**：未来启动期工作会增多（联网查版本、弹窗询问、开 shell 跑命令等会用到 `action` / `shell` / 未来 `version` 等多个包）。散在 `globals.go` 或 `main.go` 会很乱；统一在 `startup/` 符合 microNeo 现有"独立子领域 = 独立目录"的风格。

### 3.2 协议版本判定的双向语义

startup 的版本校验解决的是**静态兼容性**问题：aibp-pi 的实现协议和 microNeo 自己实现的协议版本是否兼容。

| 数据点 | 来源 | 形态 |
|--------|------|------|
| **microNeo 期望的协议** | `internal/aibp.Protocol`（已存在的常量 `"aibp-1"`） | 字符串 |
| **已装扩展实现的协议** | 扩展包 `package.json` 的 `aibp.protocol` 字段 | 字符串 |

兼容性判定用**主版本相等**（复用 `internal/aibp/registry.go:major()`）：

| `major(扩展协议)` vs `major(microNeo 协议)` | 含义 | startup 行为 |
|----------------------------------------------|------|-------------|
| 相等 | 兼容 | 快路径，什么都不做 |
| 扩展 < microNeo | 扩展过旧 | `Update()` 升级扩展 |
| 扩展 > microNeo | microNeo 过旧 | InfoBar 提示"请升级 microNeo"（`Update()` 治不了） |

> **协议版本的字段为什么是 `aibp.protocol` 而不是 package.json 的 `version`**：包版本是 npm 迭代用的（`1.0.0` → `1.0.1` → `1.1.0`），不对应协议兼容性。协议可能多个包版本都实现 `aibp-1`；也可能一个包版本实现 `aibp-2`。两者是正交维度。

### 3.3 启动自举流程（幂等）

```go
func EnsurePiExtension() error {
    e := piEnsurer{}                                   // 实现 AgentEnsurer
    if !e.HasAgent()    { return nil }                 // 未装 agent → 静默放弃
    if !e.HasExtension() { return e.Install() }        // 未装扩展 → 装

    ext, mine := e.InstalledProtocolVersion(), aibp.Protocol
    switch cmp := major(ext) - major(mine); {
    case cmp == 0: return nil                          // 兼容：常态快路径
    case cmp < 0:  return e.Update()                   // 扩展过旧：升级扩展（决策 ② 待定）
    default:       return errMicroNeoOutdated          // microNeo 过旧：提示用户升 microNeo
    }
}
```

- **常态快路径**（已装且兼容）：`LookPath` + 读 settings.json + 读已装扩展 package.json = 几 ms，不 spawn
- **首次安装**：spawn `pi install`，pi 自己跑 `npm install`、写 settings.json。偶发，几秒可接受

---

## 四、设计决策

| # | 决策 | 理由 |
|---|------|------|
| D1 | **触发时机：microNeo 启动时检查** | 常态检测几 ms，淹没在启动开销里；避免"用户按 Alt-Enter 期待通信却被打断"的体验谷底 |
| D2 | **全过程静默**，只在真出错时走 InfoBar | 启动期界面只有 infoPane；未装 agent 直接跳过 |
| D3 | **`pi install` 失败不兜底**，报错 + 提示升级 pi | 唯一现实风险是 pi 太旧没有 install 子命令 |
| D4 | **同步执行**（非异步） | 常态路径不 spawn；首次安装偶发几秒可接受；异步引入竞态得不偿失 |
| D5 | **不 pin 版本**（spec 用 `npm:aibp-pi` 不带 `@版本`） | aibp-pi 独立发版，microNeo 不跟随；升级交给 `pi update --extensions` |
| D6 | **自举文件命名 `internal/startup/ensure-<agent>.go`** | 一个 agent 一个文件；未来加 opencode/claude = 复制模板 + globals.go 列表加一项 |
| D7 | **编排器统一报告错误**，各 `ensure-*.go` 只返回 error | 单一职责：业务文件不反向依赖 UI；错误处理点唯一 |
| D8 | **接口契约：每个 `ensure-<agent>.go` 实现五个语义方法** | 详见 §五。不同 agent 扩展机制差异大（pi 用 npm、opencode 可能用 plugin），由各自实现吸收 |
| **D9** | **协议版本静态化**：扩展 `package.json` 的 `aibp.protocol` 是单一事实来源 | startup 静态读它判断兼容性；运行时也读它写注册表。静态/运行时必然一致（详见 [`说明-AIBP §7.1.1`](./说明-AIBP.md)） |

---

## 五、`AgentEnsurer` 接口契约

**每个 agent 自举文件实现以下五个方法**。这是统一契约，吸收各 agent 扩展机制的差异——pi 用 npm、opencode 可能用 plugin、claude 可能用 mcp config，各自的实现细节藏在 `ensure-<agent>.go` 内部。

```go
// internal/startup/startup.go
package startup

// AgentEnsurer 描述一个 agent 的 aibp 扩展自举。
// 不同 agent 的扩展机制差异大，各自实现这五个方法吸收差异。
type AgentEnsurer interface {
    AgentName() string                              // "pi" / "opencode"——日志和报错用

    HasAgent() bool                                  // 本机有没有这个 agent 程序
    HasExtension() bool                              // 该 agent 装没装 aibp 扩展
    InstalledProtocolVersion() (string, error)       // 已装扩展实现的协议（如 "aibp-1"）。
                                                     //   注意：协议版本，非包版本。读静态声明（package.json），不启动 agent
    Install() error                                  // 装 aibp 扩展到该 agent
    Update() error                                   // 升级该 agent 的 aibp 扩展
}

// Ensure 是统一编排逻辑，各 ensure-*.go 共用。
func Ensure(e AgentEnsurer) error {
    if !e.HasAgent()    { return nil }
    if !e.HasExtension() { return e.Install() }

    ext, err := e.InstalledProtocolVersion()
    if err != nil { return err }
    extMajor, mineMajor := major(ext), major(aibp.Protocol)
    switch {
    case extMajor == mineMajor: return nil
    case extMajor < mineMajor:  return e.Update()    // 决策 ② 待定
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
| `internal/startup/ensure-pi.go` | 新增 | `startup` | `piEnsurer` 实现 `AgentEnsurer`（5 个方法） |
| `internal/startup/ensure-pi_test.go` | 新增 | `startup` | `piHasPackage` 匹配逻辑、协议版本解析单测 |
| `internal/startup/startup.go` | 新增 | `startup` | 通用编排：`AgentEnsurer` 接口 + `Ensure` + `RunStartupChecks` |
| `internal/action/globals.go` | 改动 (+2 行 +1 import) | `action` | 挂载点：`InitGlobals()` 末尾调一次编排 |

**微原生侵入**：唯一触碰的微原生文件 `globals.go` 本就是 microNeo 已改过的；本次仅 +2 行函数体 + 1 个 import。其余全为新增。预计 ~90 行 Go，无新依赖（全标准库）。

### 6.2 代码骨架

**① `internal/startup/ensure-pi.go`** —— pi 版 `AgentEnsurer`

```go
package startup

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/micro-editor/micro/v2/internal/aibp"
)

const aibpPiSpec = "npm:aibp-pi" // 不 pin 版本（D5）

type piEnsurer struct{}

func (piEnsurer) AgentName() string { return "pi" }

func (piEnsurer) HasAgent() bool {
	_, err := exec.LookPath("pi")
	return err == nil
}

func piSettingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pi", "agent", "settings.json")
}

// piHasPackage：检查 pi settings.json 的 packages 里是否已登记 aibp-pi。
// 兼容 "npm:aibp-pi"（unpinned，期望形态）和 "npm:aibp-pi@x.y.z"（pinned，历史遗留）。
func (piEnsurer) HasExtension() bool {
	b, err := os.ReadFile(piSettingsPath())
	if err != nil { return false } // 读不到 → 视为未登记（首次安装）
	var s struct{ Packages []string `json:"packages"` }
	if json.Unmarshal(b, &s) != nil { return false }
	for _, p := range s.Packages {
		if p == aibpPiSpec || strings.HasPrefix(p, aibpPiSpec+"@") { return true }
	}
	return false
}

// InstalledProtocolVersion：读已装扩展的 package.json 的 aibp.protocol 字段。
//   扩展位置 = ~/.pi/agent/npm/node_modules/aibp-pi/package.json
//   package.json 损坏/字段缺失 → 返回 error（视为不兼容，触发 Update）
func (piEnsurer) InstalledProtocolVersion() (string, error) {
	home, _ := os.UserHomeDir()
	pkgPath := filepath.Join(home, ".pi", "agent", "npm", "node_modules", "aibp-pi", "package.json")
	b, err := os.ReadFile(pkgPath)
	if err != nil { return "", err }
	var pkg struct{ AIBP struct{ Protocol string `json:"protocol"` } `json:"aibp"` }
	if json.Unmarshal(b, &pkg) != nil { return "", errParseFailed }
	return pkg.AIBP.Protocol, nil
}

func (piEnsurer) Install() error {
	return exec.Command("pi", "install", aibpPiSpec).Run() // 不带 @版本（D5）
}

func (piEnsurer) Update() error {
	return exec.Command("pi", "update", aibpPiSpec).Run() // pi update 升级 unpinned 包
}
```

**② `internal/startup/startup.go`** —— 通用编排框架

```go
package startup

import (
	"errors"

	"github.com/micro-editor/micro/v2/internal/action"
	"github.com/micro-editor/micro/v2/internal/aibp"
)

var (
	errParseFailed       = errors.New("package.json 解析失败")
	errMicroNeoOutdated  = errors.New("aibp 扩展协议版本较新，请升级 microNeo")
)

// AgentEnsurer 描述一个 agent 的 aibp 扩展自举（详见 §五）。
type AgentEnsurer interface {
	AgentName() string
	HasAgent() bool
	HasExtension() bool
	InstalledProtocolVersion() (string, error)
	Install() error
	Update() error
}

// Ensure：统一编排逻辑（§3.3）。各 ensure-*.go 共用。
func Ensure(e AgentEnsurer) error {
	if !e.HasAgent()    { return nil }
	if !e.HasExtension() { return e.Install() }

	ext, err := e.InstalledProtocolVersion()
	if err != nil { return err }
	switch extMajor, mineMajor := major(ext), major(aibp.Protocol); {
	case extMajor == mineMajor: return nil              // 兼容
	case extMajor < mineMajor:  return e.Update()        // 扩展过旧（决策 ② 待定）
	default:                    return errMicroNeoOutdated
	}
}

// major：协议主版本号。复用 internal/aibp 的语义（解析 "aibp-1" → 1）。
func major(protocol string) int { return aibp.Major(protocol) }

// Check 是 v0 的编排入口形态（未来可扩 Priority / Mode 字段）。
type Check struct {
	Name string
	Run  func() error
}

func RunStartupChecks(checks []Check) {
	for _, c := range checks {
		if err := c.Run(); err != nil {
			action.InfoBar.Message(c.Name + ": " + err.Error())
		}
	}
}
```

**③ `internal/action/globals.go`** —— 挂载点

```go
import "github.com/micro-editor/micro/v2/internal/startup"

func InitGlobals() {
	// ... 原有初始化
	startup.RunStartupChecks([]startup.Check{
		{Name: "aibp-pi", Run: func() error { return startup.Ensure(startup.PiEnsurer{}) }},
	})
}
```

### 6.3 待拍决策（实施前必须定）

**① `aibp.Major()` 当前是包内私有（`internal/aibp/registry.go`）**
方案：导出为 `Major()`。改动 1 行（`func major` → `func Major`），属微原生侵入，但在 aibp 子领域内部，不破坏外部接口。

**② `Update()` 触发策略（启动时自动联网升级 vs 只提示）**
当前草案是自动调 `pi update npm:aibp-pi`。隐忧：启动时偷偷联网升级用户环境不够透明，离线会失败。备选是返回 `errExtensionOutdated`，让 InfoBar 提示"请运行 `pi update npm:aibp-pi`"。
**倾向**：只提示——首次 Install 可自动（必要的一次性动作），Update 应让用户知情。

> 决策 ②（扩展比 microNeo 新的反向情况）已并入 §3.2 决策矩阵——返回 `errMicroNeoOutdated` 走 InfoBar，不走 `Update()`。

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
| 导出 `aibp.Major()` | `internal/aibp/registry.go`（`major` → `Major`） |
| 新增接口 + 编排 | `internal/startup/startup.go`（见 §6.2②） |
| 新增 pi 自举 | `internal/startup/ensure-pi.go`（见 §6.2①） |
| 新增单测 | `internal/startup/ensure-pi_test.go` |

**Build gate**：`make build` 过；`go test ./internal/startup` 全绿（`HasExtension` 的 pinned/unpinned 匹配、`InstalledProtocolVersion` 的正常/损坏/缺失路径）。此 Phase 不挂载到 globals，不改运行期。

### Phase 2 — 接入启动钩子

| 改动 | 文件 |
|------|------|
| 挂载 | `internal/action/globals.go:InitGlobals()`（见 §6.2③） |

**Build gate**：`make build` 过；全新环境启动后 `~/.pi/agent/settings.json` 含 `npm:aibp-pi`、`pi list` 显示 aibp-pi。

### Phase 3 — 测试矩阵 + 端到端

跑 §八 全部测试；`time` 实测常态路径耗时增量 < 5ms，回填到 §四 D1。

---

## 八、测试与验收

**前置**（每次测试前重置）：`pi remove npm:aibp-pi` + 从 settings.json 删 aibp-pi 条目 + `killall pi microneo`。

| # | 场景 | 前置条件 | 步骤 | 期望 |
|---|------|---------|------|------|
| T1 | 未装 pi | PATH 去掉 pi | 启动 microNeo | 静默跳过；不改 settings.json |
| T2 | 首次安装 | 全新环境，pi 在 PATH | 启动 microNeo | settings.json 含 `npm:aibp-pi`（unpinned）；`pi list` 显示 aibp-pi |
| T3 | 常态快路径 | T2 后再启动 | `time ./microneo some.md` | 不 spawn install；启动耗时增量 < 5ms |
| T4 | 登记丢失 | T2 后 `pi remove npm:aibp-pi` | 启动 microNeo | 重新 `pi install`；`pi list` 重现 aibp-pi |
| T5 | pi install 失败 | PATH 放一个无 install 子命令的假 pi | 启动 microNeo | InfoBar 提示升级 pi |
| T6 | 扩展协议过旧 | 改已装 package.json 的 `aibp.protocol` 为 `"aibp-0"` | 启动 microNeo | 走 `Update()` 路径（决策 ② 定） |
| T7 | microNeo 协议过旧 | 改已装 package.json 的 `aibp.protocol` 为 `"aibp-2"` | 启动 microNeo | InfoBar 提示升级 microNeo（不 Update） |
| T8 | 端到端 | T2 后 | 开 pi → Alt-Enter → 写一行 → Alt-Enter | pi 收到消息 |
| T9 | 幂等 | T2 后连启两次 | 第二次启动 | 不 spawn install |

**验收清单**：
- [ ] Phase 0–3 按序执行，各 build gate 全过
- [ ] `make build` 干净通过
- [ ] `go test ./internal/startup` 全绿
- [ ] T1–T9 全部通过
- [ ] 端到端：全新环境启动即自动装好 aibp-pi，Alt-Enter 能发消息
- [ ] 常态路径启动耗时增量实测 < 5ms，回填 §四 D1
- [ ] 微原生侵入面：除新增 3 文件 + `globals.go`（+2 行 +1 import）+ `aibp/registry.go`（`major`→`Major` 导出）外，未改任何原生文件

---

## 九、风险与已知限制

| # | 风险 | 触发条件 | 应对 |
|---|------|---------|------|
| R1 | **首次安装需要网络** | `pi install` 触发 `npm install` 需联网 | 用户能装 pi 就有 npm 环境，假设合理；离线时 D3 报错提示 |
| R2 | **pi settings.json 路径假设** | pi 用自定义 `PI_HOME` / 非 `~/.pi/agent` | v1 写死路径，记为已知限制；pi 文档明确后再适配 |
| R3 | **首次启动卡顿** | 首次 `pi install` + `npm install` 耗时几秒 | 偶发一次可接受；实测 >5s 再议异步化（D4） |
| R4 | **微原生侵入** | 误改 micro 原生文件 | `globals.go` 仅 +2 行 +1 import；`registry.go` 仅导出 1 函数；验收用 `git diff` 核对 |
| R5 | **已装扩展 package.json 损坏** | 用户手改 / 文件系统问题 | `InstalledProtocolVersion` 返回 error → 启动期 InfoBar 提示；不静默跑下去 |

**回滚**：改动的原生文件 `git checkout` 取回；新增文件直接删除。

---

## 附：未来扩展

`AgentEnsurer` 接口让每个 agent 扩展的自举高度统一。未来加新 agent = 发 npm 包 + 一个新 ensure 文件：

```go
// ensure-opencode.go（未来）
type opencodeEnsurer struct{}
func (opencodeEnsurer) HasAgent() bool { ... }       // opencode 的发现机制
func (opencodeEnsurer) HasExtension() bool { ... }   // opencode 的扩展登记机制（可能不是 npm）
func (opencodeEnsurer) InstalledProtocolVersion() (string, error) { ... } // 读 opencode 扩展的协议声明
func (opencodeEnsurer) Install() error { ... }
func (opencodeEnsurer) Update() error { ... }
```

globals.go 列表加一项即可。各 agent 独立发版、独立升级，互不耦合——但 startup 编排逻辑（`Ensure` 函数）和接口契约永远不变。
