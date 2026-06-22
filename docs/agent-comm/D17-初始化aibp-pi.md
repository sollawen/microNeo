# D17 · microNeo 启动时自动初始化 aibp-pi

> **回答** [`如何初始化aibp.md`](./如何初始化aibp.md) 提出的三个问题。
>
> **状态**：方案已定，待实施。
>
> **关联**：[`如何初始化aibp-调研结论.md`](./如何初始化aibp-调研结论.md)（pi 扩展机制的调研依据）。

---

## 一、目标

用户下载安装 microNeo 后，打开一个 `.md` 文件、按 Alt-Enter，notePane 就能跟 pi 通信——**不需要用户手动安装任何扩展**。

为此 microNeo 启动时必须自动完成三件事（对应需求的三个问题）：

| 需求问题 | 本方案 |
|---------|--------|
| 如何发现用户装了 pi | `exec.LookPath("pi")` |
| 如何判断 pi 有没有装 aibp-pi | 读 `~/.pi/agent/settings.json` 的 `packages`，看有无指向 aibp-pi 的条目 |
| 没装时怎么装进去 | **把随包的 aibp-pi 源码 copy 到 `$XDG_CONFIG_HOME/aibp/agents/pi/`，再调 `pi install <绝对路径>` 注册** |

**非目标**：发布 npm/git 包、联网查版本、运行时询问用户——这些留作未来演进。

---

## 二、方案总览

### 2.1 核心机制

```
编译期                          运行期（microNeo 每次启动）
─────────                      ──────────────────────────────
aibp-agents/pi/                 ┌─ ① 发现 pi？ LookPath
  index.ts        ─ go:embed ──▶│
  package.json   （打进二进制）  ├─ ② 已就位？ 检测目录+版本+登记项
                                 │     └─ 是 → 结束（常态快路径，~2-3ms）
aibp-agents/embed.go            │
                                 ├─ ③ 没就位？ copy 出 index.ts + package.json
microNeo 二进制 ──────────────────▶│     → $XDG_CONFIG_HOME/aibp/agents/pi/
                                 │     → pi install <abs>  写入 pi settings.json
                                 └─ 全过程静默
```

一句话：**随包 embed + 启动时 copy + 用 pi 的标准 `install` 命令注册**。

---

## 三、系统设计

### 3.1 模块划分

新增一个独立子领域 `internal/startup/`，承担"启动期应用层工作"。依赖方向单向：

```
main → action.globals ──┐
                        ▼
                   internal/startup  ──┐  (顶层消费者：启动期编排 + 各 ensure-*.go)
                        │              │
                        │              ├──▶ aibp-agents (aibpagents：纯数据，embed 出来的字节)
                        │              └──▶ action (用 InfoBar 报错)
                        │
              internal/aibp  (aibp 工具：registry/send/discover —— 与 startup 无关)
```

三个角色的职责边界：

| 模块 | 职责 | 不做什么 |
|------|------|---------|
| `aibp-agents`（包 `aibpagents`） | 纯数据：编译期 embed 出随包源码字节 | 任何运行时副作用 |
| `internal/startup` | 启动期编排 + aibp-pi 自举（写文件、spawn `pi install`） | 不被任何业务包 import（它是顶层消费者） |
| `internal/aibp` | aibp 工具本身（名字池/发送/发现） | 不管"装扩展到 pi"——那是 microNeo 启动期的事 |

**为什么是独立子领域**：未来启动期工作会增多（联网查版本、弹窗询问用户、开 shell 跑命令等会用到 `action` / `shell` / 未来 `version` 等多个包）。散在 `globals.go` 或 `main.go` 会很乱；统一在 `startup/` 才符合 microNeo 现有"独立子领域 = 独立目录"的风格。

### 3.2 启动自举流程（三步，幂等）

```go
func EnsurePiExtension() error {
    // ① 发现 pi：没有就静默放弃（不告警）
    if _, err := exec.LookPath("pi"); err != nil { return nil }

    // ② 检测是否就位
    if !needReinstall(deployedDir()) { return nil }   // 常态快路径

    // ③ 重新部署：copy + install（两步都做、都幂等）
    if err := deploy(deployedDir()); err != nil { return err }
    return exec.Command("pi", "install", abs).Run()  // 失败报错，见 §4 D4
}
```

`needReinstall` 在以下任一情况返回 true：`index.ts` 缺失 / 已部署版本 ≠ 随包版本 / pi 的 `settings.json` 无登记项。一旦需要动作就同时 copy + install——两步幂等，不区分"文件过期"还是"登记丢失"。这也对齐未来 npm 场景的 `reinstall` 心智模型：检测到不一致即整体重装。

### 3.3 目录与部署布局

```
$XDG_CONFIG_HOME/aibp/                 # aibp 命名空间（aibp-names.json 已在此）
└── agents/
    └── pi/                            # ⭐ 运行期写出的 aibp-pi 副本
        ├── index.ts
        └── package.json               # 含 version 字段

~/.pi/agent/settings.json              # pi install 自动写入
└── packages: [ "<abs>/aibp/agents/pi" ]
```

- 部署目录用 `$XDG_CONFIG_HOME/aibp/`（未设置时 fallback `~/.config/aibp/`），**不复用 `config.ConfigDir`**——后者拼了 `microNeo` 后缀，而 aibp 是独立命名空间。
- pi settings.json 路径 v1 写死 `~/.pi/agent/settings.json`（见 §8 R1 的已知限制）。

### 3.4 随包资源打包：就地 `go:embed`

在 `aibp-agents/` 放一个 `embed.go`，对同级的 `pi/index.ts`、`pi/package.json` 做 `//go:embed`。这是 micro 原生 `runtime/runtime.go` 的既有模式——那个 Go 文件就放在 `runtime/` 资源目录里，对同级子目录就地 embed。

> `//go:embed` 路径相对 .go 文件所在目录，不能跨 `..`。把 `embed.go` 放在 `aibp-agents/` 父层（而非 `aibp-agents/pi/`），是因为它要能同时够到未来其它 agent 子目录；而 `aibp-agents/pi/` 是纯源码目录，不被 Go 视为包。

---

## 四、设计决策

| # | 决策 | 理由 |
|---|------|------|
| D1 | **触发时机：microNeo 启动时检查**（非 Alt-Enter 时） | 常态检测 ~2-3ms（LookPath + 读两个小文件），淹没在 microNeo 启动本身的开销里；且避免"用户按 Alt-Enter 期待通信却被打断去装扩展"的体验谷底 |
| D2 | **全过程静默**，只在真出错时走 InfoBar 告警 | 启动期界面只有 infoPane，不宜插入"正在配置…"提示；未装 pi 直接跳过 |
| D3 | **版本过期自动覆盖，无需用户确认** | 装好即就绪，符合"用户无感"目标 |
| D4 | **`pi install` 失败不兜底**，报错 + 提示升级 pi | 常态失败率 <1%，唯一现实风险是 pi 太旧没有 install 子命令；回退方案（copy 到 pi 约定目录 / 自行合并 settings.json）都会留残留或破坏用户配置，更差 |
| D5 | **同步执行**（非异步） | 常态路径不 spawn、仅几 ms；首次/升级 spawn 是偶发的几百 ms，可接受；异步会引入 notePane 竞态 |
| D6 | **版本比较用字符串等值**（非 semver） | 简单可靠；约束随包 `version` 必须是静态字面量 |
| D7 | **自举文件命名 `internal/startup/ensure-<agent>.go`** | 一个 agent 扩展一个文件；未来加 opencode/claude = 复制模板 + globals.go 列表加一项 |
| D8 | **编排器 `RunStartupChecks` 统一报告错误**，各 `ensure-*.go` 只返回 error | 单一职责：业务文件不反向依赖 UI；错误处理点唯一 |

---

## 五、实现规格

### 5.1 文件清单与侵入面

| 文件 | 状态 | 包 | 职责 |
|------|------|---|------|
| `aibp-agents/pi/package.json` | 改动 | — | 补 `"version": "1.0.0"` |
| `aibp-agents/embed.go` | 新增 | `aibpagents` | 静态导出随包源码字节（纯数据，无副作用） |
| `internal/startup/ensure-pi.go` | 新增 | `startup` | aibp-pi 自举：发现 pi、检测、部署、注册（返回 error） |
| `internal/startup/ensure-pi_test.go` | 新增 | `startup` | `needReinstall` 各触发条件 + 路径归一化单测 |
| `internal/startup/startup.go` | 新增 | `startup` | 通用编排框架：`Check` + `RunStartupChecks` |
| `internal/action/globals.go` | 改动 (+2 行 +1 import) | `action` | 挂载点：`InitGlobals()` 末尾调一次编排 |

**微原生侵入**：唯一触碰的微原生文件 `globals.go` 本就是 microNeo 已改过的；本次仅 +2 行函数体 + 1 个 import。其余全为新增。预计 ~150 行 Go，无新依赖（全标准库）。

### 5.2 代码骨架

**① `aibp-agents/embed.go`** —— 就地 embed（§3.4）

```go
package aibpagents

import (
	_ "embed"
	"encoding/json"
)

//go:embed pi/index.ts
var indexTS []byte

//go:embed pi/package.json
var packageJSON []byte

func IndexTS() []byte     { return indexTS }
func PackageJSON() []byte { return packageJSON }

// Version 解析随包 package.json 的 version（D6 约束：必须静态字面量）。
func Version() string {
	var pkg struct{ Version string `json:"version"` }
	_ = json.Unmarshal(packageJSON, &pkg)
	return pkg.Version
}
```

**② `internal/startup/ensure-pi.go`** —— 自举业务（§3.2）

```go
package startup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	aibpagents "github.com/micro-editor/micro/v2/aibp-agents"
)

// aibpBase 对齐 index.ts 的 configBase：$XDG_CONFIG_HOME 或 ~/.config，再拼 aibp。
// 不复用 config.ConfigDir（它多拼了 "microNeo" 后缀）。
func aibpBase() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "aibp")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "aibp")
}

func deployedDir() string { return filepath.Join(aibpBase(), "agents", "pi") }

func piSettingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pi", "agent", "settings.json")
}

// needReinstall：任一不一致即 true（文件缺失 / 版本过期 / 未登记）。
// 不区分原因——检测到不一致就整体 copy + install（两步幂等），对齐未来 npm reinstall 模型。
func needReinstall(deployed string) bool {
	bundledVer := aibpagents.Version()
	deployedVer := readVersion(filepath.Join(deployed, "package.json"))
	fileOk := fileExists(filepath.Join(deployed, "index.ts")) && deployedVer == bundledVer
	installOk := piSettingsHasPackage(deployed)
	return !fileOk || !installOk
}

func readVersion(p string) string {
	b, err := os.ReadFile(p)
	if err != nil { return "" }
	var pkg struct{ Version string `json:"version"` }
	if json.Unmarshal(b, &pkg) != nil { return "" }
	return pkg.Version
}

// piSettingsHasPackage：packages 条目可能是相对路径（相对 settings.json 所在目录），
// 必须 abs 归一化后比，否则 "../../aibp/agents/pi" 这类条目会误判（R4）。
func piSettingsHasPackage(deployed string) bool {
	settings := piSettingsPath()
	b, err := os.ReadFile(settings)
	if err != nil { return false } // 读不到 → 视为未登记（首次安装）
	var s struct{ Packages []string `json:"packages"` }
	if json.Unmarshal(b, &s) != nil { return false }
	absDeployed, _ := filepath.Abs(deployed)
	settingsDir := filepath.Dir(settings)
	for _, entry := range s.Packages {
		abs := entry
		if !filepath.IsAbs(abs) { abs = filepath.Join(settingsDir, entry) }
		abs, _ = filepath.Abs(abs)
		if abs == absDeployed { return true }
	}
	return false
}

func EnsurePiExtension() error {
	if _, err := exec.LookPath("pi"); err != nil { return nil } // 未装 pi 静默放弃

	deployed := deployedDir()
	if !needReinstall(deployed) { return nil } // 常态快路径

	if err := deploy(deployed); err != nil {
		return fmt.Errorf("部署失败: %w", err)
	}
	abs, _ := filepath.Abs(deployed)
	if err := exec.Command("pi", "install", abs).Run(); err != nil {
		return errors.New("注册失败，请升级 pi 后重启 microNeo") // D4：不兜底
	}
	return nil
}

func deploy(deployed string) error {
	if err := os.MkdirAll(deployed, 0o755); err != nil { return err }
	files := map[string][]byte{
		"index.ts":     aibpagents.IndexTS(),
		"package.json": aibpagents.PackageJSON(),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(deployed, name), content, 0o644); err != nil {
			return err
		}
	}
	return nil
}
```

**③ `internal/startup/startup.go`** —— 通用编排框架（D8）

```go
package startup

import "github.com/micro-editor/micro/v2/internal/action"

// Check 描述一个启动检查项。
// v0：Run 返回 error → action.InfoBar 报告；nil 静默通过。
// 未来扩展点（仅接口预留）：加 Priority / Mode（InfoBar→MessageBox 询问）/ Async 字段，
// 改 RunStartupChecks 内部，调用方和 Check 构造方都不动。
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

**④ `internal/action/globals.go`** —— 挂载点（+2 行 +1 import）

```go
import "github.com/micro-editor/micro/v2/internal/startup"

func InitGlobals() {
	// ... 原有初始化
	startup.RunStartupChecks([]startup.Check{
		{Name: "aibp-pi", Run: startup.EnsurePiExtension},
	})
}
```

---

## 六、实施计划

每个 Phase 通过自己的 build gate 才进入下一个。

### Phase 0 — 前置基线

| 检查项 | 验证方法 |
|--------|---------|
| 编译通过 | `make build` 成功 |
| AIBP 基线链路通 | 手动把 `~/.pi/agent/settings.json` 里 aibp 条目指到开发机现有源码路径，开 pi → Alt-Enter → pi 收到消息 |

基线不对则停下，后续验证无意义。回滚靠 git（只读取回，本计划不要求 git 写操作）。

### Phase 1 — 随包资源 + embed 基建

| 改动 | 文件 |
|------|------|
| 补版本号 | `aibp-agents/pi/package.json` 加 `"version": "1.0.0"` |
| 新增 embed | `aibp-agents/embed.go`（见 §5.2①） |

**Build gate**：`make build` 与 `make build-quick` 均过；临时单测验证 `Version()=="1.0.0"`、`IndexTS()` 非空。

### Phase 2 — aibp-pi 自举（可单测，未挂载）

| 改动 | 文件 |
|------|------|
| 新增自举 | `internal/startup/ensure-pi.go`（见 §5.2②） |
| 新增单测 | `internal/startup/ensure-pi_test.go` |

**Build gate**：`make build` 过；`go test ./internal/startup` 全绿（needReinstall 各触发条件：文件缺失 / 版本过期 / 未登记、相对路径 abs 归一化）。此 Phase 不改运行期行为（未挂载到编排器）。

### Phase 3 — startup 编排层 + 接入启动钩子

| 改动 | 文件 |
|------|------|
| 新增编排 | `internal/startup/startup.go`（见 §5.2③） |
| 挂载 | `internal/action/globals.go:InitGlobals()`（见 §5.2④） |

**Build gate**：`make build` 过；首次启动后 `$XDG_CONFIG_HOME/aibp/agents/pi/index.ts` 存在、pi settings.json 多出绝对路径条目、`pi list` 显示 aibp-pi。

### Phase 4 — 测试矩阵执行

跑 §7.1 的 T1–T8，全部通过才算完成。

### Phase 5 — 端到端联调 + 文档回填

全新环境（清空 aibp 目录 + 删 pi 登记项）→ 启动 microNeo → 开 pi → Alt-Enter 发消息 → pi 收到；`time` 实测常态路径耗时增量 < 5ms，回填到 §4 D1。

---

## 七、测试与验收

### 7.1 测试矩阵

**前置**（每次测试前重置）：`rm -rf "${XDG_CONFIG_HOME:-$HOME/.config}/aibp/agents"` + 从 pi settings.json 删 aibp 条目 + `killall pi microneo`。

| # | 场景 | 前置条件 | 步骤 | 期望 |
|---|------|---------|------|------|
| T1 | 未装 pi | PATH 去掉 pi | 启动 microNeo | 静默跳过；不创建目录、不改 settings.json |
| T2 | 首次部署 | 全新环境，pi 在 PATH | 启动 microNeo | `aibp/agents/pi/index.ts` 存在；pi settings.json 含 deployed 绝对路径；`pi list` 显示 aibp-pi |
| T3 | 常态快路径 | T2 后再启动 | `time ./microneo some.md` | 不 copy 不 install；启动耗时增量 < 5ms |
| T4 | 版本升级 | 已部署 version 改 `0.9.0` | 启动 microNeo | 文件被覆盖为随包版本；同时重新 pi install（幂等，条目内容刷新、数量不变） |
| T5 | 登记丢失 | T2 后 `pi remove` 掉 aibp-pi | 启动 microNeo | copy（幂等，覆盖相同文件）+ 重新 pi install；`pi list` 重现 aibp-pi |
| T6 | pi install 失败 | PATH 放一个无 install 子命令的假 pi | 启动 microNeo | InfoBar 提示升级 pi；deployed 文件已 copy |
| T7 | 端到端 | T2 后 | 开 pi → Alt-Enter → 写一行 → Alt-Enter | pi 收到消息（部署的 aibp-pi 真正工作） |
| T8 | 幂等 | T2 后连启两次 | 第二次启动 | 无文件写（copy/install 全 skip） |

### 7.2 验收清单（Definition of Done）

- [ ] Phase 0–5 按序执行，各 Phase build gate 全过
- [ ] `make build` 与 `make build-quick` 均干净通过
- [ ] `go test ./internal/startup` 全绿
- [ ] 测试矩阵 T1–T8 全部通过
- [ ] 端到端：全新环境启动即自动装好 aibp-pi，Alt-Enter 能发消息
- [ ] 常态路径启动耗时增量实测 < 5ms，数字回填到 §4 D1
- [ ] 微原生侵入面：除 `aibp-agents/embed.go` + `internal/startup/ensure-pi.go` + `internal/startup/startup.go`（新增）+ `internal/action/globals.go`（+2 行 +1 import）外，未改任何原生文件（`git diff -- internal/action/` 核对）

---

## 八、风险与已知限制

| # | 风险 | 触发条件 | 应对 |
|---|------|---------|------|
| R1 | **pi settings.json 路径假设** | pi 用自定义 `PI_HOME` / 非 `~/.pi/agent` | v1 写死 `~/.pi/agent/settings.json`，记为已知限制；pi 文档明确后再适配 |
| R2 | **首次启动卡顿** | 首次 spawn `pi install` 耗时几百 ms | 偶发可接受；实测 >1s 再议异步化（D5） |
| R3 | **version 字面量漂移** | 随包 version 写成动态值或忘改 | 约束 `package.json` 的 version 必须是静态字符串；Phase 1 build gate 校验 |
| R4 | **相对路径误判已装** | pi settings.json 里 packages 是相对路径 | `piSettingsHasPackage` 统一 abs 归一化后再比；Phase 2 单测覆盖 |
| R5 | **微原生侵入** | 误改 micro 原生文件 | `globals.go` 仅 +2 行 +1 import；验收用 `git diff` 核对 |

**回滚**：改动的原生文件 `git checkout` 取回；新增文件直接删除。

---

## 附：未来扩展（v0 不实现，仅留接口位）

- `Check` 加 `Mode` 字段（`ModeInfoBar` / `ModeMessageBox`）：把"静默报告"升级为"弹窗询问"
- 加 `func Register(c Check)`：业务包在 init() 自注册，globals.go 不写死列表
- 各 `ensure-*.go` 扩展：`ensure-opencode.go` / `ensure-claude.go` 复制模板，globals.go 列表加一项
- 发布 npm/git：`pi install npm:` / `git:`，本地 copy 作为离线兜底并存

> 本方案已为以上演进预留结构（独立 `startup` 子领域 + 通用 `Check` 编排 + `ensure-<agent>.go` 命名约定），但 v1 实现不写双通道代码。
