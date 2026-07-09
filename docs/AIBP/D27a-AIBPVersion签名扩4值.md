# D27a · `AIBPVersion()` 签名 3→4：接口声明 + pi / claude 对齐

**状态**：方案已定，待实施。
**前置**：D25（`AgentEnsurer` 接口）、D27（`--update-aibp`，opencode 侧）。
**配套 / 编译耦合**：本文档与 D27 必须一起落地——`AIBPVersion` 是 `AgentEnsurer` 接口方法，任一处返回值变更都会让未对齐的实现 / 调用点编译失败。opencode 的实现 + helper 随 D27 §3.3 改；本文档管接口声明、`Ensure()` 调用点、pi、claude。

---

## 一、目标

给 `AgentEnsurer.AIBPVersion()` 加第四个返回值 `version string`：

- npm / marketplace 形态 → 读 `package.json` 的 `.version`，返回 `"X.Y.Z"`
- 源码安装 / 未装 / 损坏 → 返回 `""`

动机见 D27 §3.3：让 opencode 的 `UpdateToLatest` 一次调用同时拿到 `isSource` 与 `installed`，避免「`AIBPVersion()` 内部已读 `package.json` 却把 `version` 丢掉、外层再绕调内部 helper 重读一遍」的拧巴。

pi / claude 是接口契约的**被动对齐方**——它们的 `UpdateToLatest`（pi 的 no-op；claude 见 D27 §3.4，用 `claudeInstalledRecord` 取 before/after）目前都不消费 `version`，但接口方法签名一改，它们的实现必须跟上，否则不满足 `AgentEnsurer` 接口、编译失败。

---

## 二、现状

接口声明（`ensure.go`）：

```go
AIBPVersion() (major, minor int, isSourceInstall bool)
```

三个实现 + 各自一个内部 helper（均 3 返回值；helper 只被各自的 `AIBPVersion` 调用一次，无其它调用方）：

| agent | 方法 | helper | `.version` 来源（改后） |
|---|---|---|---|
| opencode | `OpencodeEnsurer.AIBPVersion` | `opencodeNpmAIBPVersion` | cache 目录 `package.json`（D27）|
| pi | `PiEnsurer.AIBPVersion` | `piNpmAIBPVersion` | `<agentDir>/npm/node_modules/aibp-pi/package.json` |
| claude | `ClaudeEnsurer.AIBPVersion` | `claudeReadProtocol` | `<installPath>/package.json` |

注：claude 另有 `claudeInstalledRecord()`（读 `installed_plugins.json`，已返回 `version`），但那是 D27 §3.4 `UpdateToLatest` 的 before/after 用的，与本次 `AIBPVersion` 的 `version` **互不干扰**——后者统一从 `package.json` 取，与 opencode / pi 同构（见决策 #4）。

---

## 三、改动

### 3.1 接口声明（`ensure.go`）

```go
// AIBPVersion：识别本 agent 已装的 aibp。
// 返回 (major, minor, isSourceInstall, version)。
//   - 源码路径安装 → isSourceInstall=true，major/minor=0，version=""（不读盘；编排跳过；信任假设）
//   - npm/cache/marketplace 安装 → 读到协议 + 版本号 → isSourceInstall=false，version="X.Y.Z"
//   - 未装 / 损坏 / 读不到协议 → (0, 0, false, "")（编排触发 InstallAIBP）
// 约定：合法协议大号从 1 起；major==0 即「无法识别」；version 仅 npm/marketplace 形态非空。
// version 供 UpdateToLatest（D27）一次调用同时拿 isSource + installed，免得绕调内部 helper 重读 package.json。
AIBPVersion() (major, minor int, isSourceInstall bool, version string)
```

### 3.2 `Ensure()` 调用点（`ensure.go`）

`Ensure` 不消费 `version`，丢弃即可：

```go
// 改前：
major, minor, isSource := e.AIBPVersion()
// 改后：
major, minor, isSource, _ := e.AIBPVersion() // version 仅 UpdateToLatest 用，Ensure 丢弃
```

### 3.3 pi（`ensure_pi.go`）

方法 + helper 各加一个返回值；helper 多读 `.version`：

```go
func (PiEnsurer) AIBPVersion() (int, int, bool, string) {
	for _, entry := range piReadSetting() {
		if strings.Contains(entry, "aibp-agents") {
			return 0, 0, true, "" // 源码路径：不读盘
		}
		if strings.Contains(entry, "npm:aibp-pi") {
			return piNpmAIBPVersion() // npm 包：读版本号
		}
	}
	return 0, 0, false, "" // 没找到 aibp 条目
}

func piNpmAIBPVersion() (int, int, bool, string) {
	pkgPath := filepath.Join(piAgentDir(), "npm", "node_modules", "aibp-pi", "package.json")
	b, err := os.ReadFile(pkgPath)
	if err != nil {
		return 0, 0, false, ""
	}
	var pkg struct {
		AIBP    struct{ Protocol string `json:"protocol"` } `json:"aibp"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(b, &pkg); err != nil {
		return 0, 0, false, ""
	}
	if pkg.AIBP.Protocol == "" {
		return 0, 0, false, ""
	}
	maj, min, ok := aibp.ParseProtocol(pkg.AIBP.Protocol)
	if !ok {
		return 0, 0, false, ""
	}
	return maj, min, false, pkg.Version // npm 安装：isSource 恒 false；version 来自 package.json
}
```

（结构体字段排版按 gofmt 规整。）

### 3.4 claude（`ensure_claude.go`）

同构改法。`claudeReadProtocol` 多读 `.version`，方法把 4 个值透传（结构不变，仅返回字面量加 `""`、末行委托 helper）：

```go
func (ClaudeEnsurer) AIBPVersion() (int, int, bool, string) {
	installPath, _, ok := claudeInstalledRecord()
	if !ok {
		return 0, 0, false, ""
	}
	if !claudeEnabled() {
		return 0, 0, false, ""
	}
	return claudeReadProtocol(installPath)
}

func claudeReadProtocol(installPath string) (int, int, bool, string) {
	b, err := os.ReadFile(filepath.Join(installPath, "package.json"))
	if err != nil {
		return 0, 0, false, ""
	}
	var pkg struct {
		AIBP    struct{ Protocol string `json:"protocol"` } `json:"aibp"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(b, &pkg); err != nil {
		return 0, 0, false, ""
	}
	if pkg.AIBP.Protocol == "" {
		return 0, 0, false, ""
	}
	maj, min, parseOK := aibp.ParseProtocol(pkg.AIBP.Protocol)
	if !parseOK {
		return 0, 0, false, ""
	}
	return maj, min, false, pkg.Version // marketplace 安装：isSource 恒 false；version 来自 package.json
}
```

claude 的 `UpdateToLatest`（D27 §3.4）不消费 `version`，但其首行 `major, _, _ := (ClaudeEnsurer{}).AIBPVersion()` 是 3 值赋值——签名变了须补一个 `_`（已在 D27 §3.4 代码里同步更新）。除此之外 claude 的 `UpdateToLatest` 逻辑不动（before/after 仍走 `claudeInstalledRecord`）。

---

## 四、设计决策

| # | 决策 | 理由 |
|---|------|------|
| 1 | 改 `AIBPVersion` 接口签名（3→4），而非保 3 值 | 保 3 值的代价是 opencode `UpdateToLatest` 必须绕调内部 helper 重读 `package.json`（D27 §3.3 反面论证）；接口只有 3 个实现、调用方都在同 package，改签名成本更低 |
| 2 | `version` 在 source / 未装 / 损坏时一律 `""` | 与 `major==0` 表示「无法识别」的现有约定一致；`UpdateToLatest` 用 `installed != ""` 自然区分「npm 已装」与「未装」 |
| 3 | 三个 helper 同步扩到 4 值，保持同构 | helper 本就「同构」（见现有代码注释）；version 来源统一是 `package.json` `.version`，不引入第二个版本口径 |
| 4 | claude 的 `version` 取 `package.json` 而非 `claudeInstalledRecord` | 与 opencode / pi 同构（都读 `package.json`）；`claudeInstalledRecord` 的 version 留给 `UpdateToLatest` 的 before/after，职责不混 |

---

## 五、实施步骤

| # | 文件 | 改动 | 依赖 |
|---|------|------|------|
| 1 | `internal/aibp/ensure_agents/ensure.go` | `AgentEnsurer.AIBPVersion` 接口声明 3→4 + doc 注释；`Ensure()` 调用点加 `_` | 无 |
| 2 | `ensure_pi.go` | `PiEnsurer.AIBPVersion` 3→4；`piNpmAIBPVersion` 3→4 + 读 `.version` | 1 |
| 3 | `ensure_claude.go` | `ClaudeEnsurer.AIBPVersion` 3→4；`claudeReadProtocol` 3→4 + 读 `.version` | 1 |
| 4 | `ensure_pi_test.go` | 所有 `maj, min, isSource := …AIBPVersion()` 补第四接收值（多数 `_`，版本用例可断 `version`） | 2 |
| 5 | `ensure_claude_test.go` | 同上 | 3 |

与 D27 同批提交。每步后 `make build-quick`，全完成后 `make build`。opencode 的实现 + helper + 测试随 D27 步骤 3 落地。改动量：~15 行（接口 1 行 + 三处方法签名/返回字面量 + 三个 helper 各加 `Version` 字段与返回值 + 测试逐处补 `_`），零新增 helper、零删除。

---

## 六、验收

| # | 场景 | 期望 |
|---|------|------|
| V1 | pi 源码安装 | `AIBPVersion()` → `(0, 0, true, "")` |
| V2 | pi npm 安装（protocol=aibp-2.0, version=1.0.5）| `(2, 0, false, "1.0.5")` |
| V3 | pi 未装 | `(0, 0, false, "")` |
| V4 | claude marketplace 安装 | `(2, 0, false, "1.0.x")` |
| V5 | claude 未装 / 被 disable | `(0, 0, false, "")` |
| V6 | opencode（见 D27 V1-V5）| npm 形态 `version` 非空；source / 未装为 `""` |
| V7 | `make build` | 接口 + 三实现 + `Ensure()` + 所有 `_test.go` 编译通过、无未用 import |
| V8 | 现有 `TestPiAIBPVersion` / `TestClaudeAIBPVersion` / `TestOpencodeAIBPVersion` | 补第四接收值后全绿，原断言值不变（version 可选加断）|

---

## 七、与历史文档的关系

- **D25**（`AgentEnsurer` 接口）：本方案改 `AIBPVersion` 签名（3→4），是 D25 接口的首次签名级变更；`Ensure` 编排逻辑不变（仅调用点多一个 `_`）。
- **D27**（`--update-aibp`）：opencode 的 `UpdateToLatest` 是 `version` 的唯一消费者；pi / claude 是被动对齐方（实现须满足新接口，但各自 `UpdateToLatest` 逻辑不依赖 `version`）。两文档编译耦合，一起落地。
