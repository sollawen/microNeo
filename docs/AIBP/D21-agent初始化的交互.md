# D21 · `InfoBarNow()` 实施计划

> **状态**：定稿，待批准后实施
> **目标**：为同步阻塞命令（如 `:check-agent`）提供"立即刷 InfoBar"能力。`:check-agent` 改造为 Reporter 模式，所有消息都通过 InfoBar 实时显示。
> **前置**：`Ensure` 签名变更（`Reporter` 参数）

---

## 一、改动总览

| 文件 | 状态 | 改动 |
|---|---|---|
| `internal/action/command_neo.go` | 改 | 加 `InfoBarNow` 函数 + `TestInfoCmd` + 注册 `check-agent` / `test-info` |
| `internal/aibp/ensure_agents/ensure.go` | 改 | `Reporter` 参数 + 内部所有路径 `report` |

**对 micro 原生代码的侵入**：**零**。两个改动文件都是 microNeo 自有。

---

## 二、改 `internal/action/command_neo.go`

### 2.1 完整目标态

```go
package action

import (
	"time"

	"github.com/micro-editor/micro/v2/internal/aibp/ensure_agents"
	"github.com/micro-editor/micro/v2/internal/screen"
)

func RegisterCommands() {
	MakeCommand("check-agent", (*BufPane).CheckAibpCmd, nil)
	MakeCommand("test-info", (*BufPane).TestInfoCmd, nil)
}

// InfoBarNow 设消息 + 同步刷 InfoBar 那 1 行到终端。
//
// 用于同步阻塞命令（如 :check-agent 安装 aibp-pi 联网几秒）执行期间——
// 主 goroutine 卡在命令里，DoEvent 循环的自动 redraw 不运行，
// backbuffer 变化刷不到终端。本函数手动触发 InfoBar 行刷新。
//
// 必须在主 goroutine 调用。签名匹配 ensure_agents.Reporter。
func InfoBarNow(msg string) {
	screen.Screen.HideCursor()
	InfoBar.Message(msg)
	InfoBar.Display()
	screen.Screen.Show()
}

// CheckAibpCmd 是 :check-agent 命令的处理函数。
// 检查 pi 是否装了 aibp-pi 扩展；没装则安装，装了则校验协议版本兼容性。
// 用户主动运行，可给明确反馈。
//
// Ensure 内部所有需要告诉用户的消息都通过 reporter 通知，
// reporter 直接用 InfoBarNow：签名匹配，无需闭包。
func (h *BufPane) CheckAibpCmd(args []string) {
	_ = ensure_agents.Ensure(ensure_agents.PiEnsurer{}, InfoBarNow)
}

// TestInfoCmd 是 :test-info 命令的处理函数。
// microNeo 内部使用，不在用户文档 / README / 帮助里出现。
func (h *BufPane) TestInfoCmd(args []string) {
	msgs := []string{
		"step 1/5: checking",
		"step 2/5: downloading",
		"step 3/5: installing",
		"step 4/5: verifying",
		"step 5/5: done",
	}
	for _, msg := range msgs {
		InfoBarNow(msg)
		time.Sleep(1 * time.Second)
	}
}
```

**为什么只刷 InfoBar**：
- `InfoBar.Display()` 只动 InfoBar 那 1 行（外加可选的 keymenu / suggestions 几行），**不**触发 `Fill` 整屏
- tcell 的 `Show()` 是增量——只把 backbuffer 有变化的 cell 推到终端，没变化的 cell **不**重写

**为什么不复刻 DoEvent 顶部那 20 行**：
- 那是"全屏 Fill + 走全 Display 链"，本场景不需要
- `InfoBar.Display()` + `Show()` 已经够

**InfoBar 显示内容**（aibp 相关消息加 `aibp-<agent>` 前缀以区分多 agent）：
- `checking pi ...`
- `pi not found, please install it first`（或错误描述）
- `aibp-pi downloading.....`
- `aibp-pi installed`
- `checking aibp-pi protocol version ...`
- `aibp-pi reinstalling (corrupted) ...`
- `aibp-pi ready`
- `pi protocol outdated, please upgrade`（或类似）

未来 `:check-agent` 在 opencode 上时，前缀会变成 `aibp-opencode downloading.....` / `aibp-opencode ready` 等——多 agent 共存时用户能清楚看到是哪家的扩展。

---

## 三、`Ensure` 签名变更

**所有需要告诉用户消息的路径都直接调 `report`——`Ensure` 内部统一处理**：

```go
type Reporter func(msg string)

func Ensure(e AgentEnsurer, report Reporter) error {
	if report == nil {
		report = func(string) {}
	}
	prefix := "aibp-" + e.AgentName()
	report("checking " + e.AgentName() + " ...")
	if !e.HasAgent() {
		report(e.AgentName() + " not found, please install it first")
		return errAgentNotFound
	}
	if !e.HasAIBP() {
		report(prefix + " downloading.....")
		if err := e.InstallAIBP(); err != nil {
			report(prefix + " install failed: " + err.Error())
			return err
		}
		report(prefix + " installed")
	}
	report("checking " + prefix + " protocol version ...")
	ext, err := e.AIBPVersion()
	if err != nil {
		report(prefix + " reinstalling (corrupted) ...")
		if err := e.InstallAIBP(); err != nil {
			report(prefix + " install failed: " + err.Error())
			return err
		}
		ext, err = e.AIBPVersion()
		if err != nil {
			report(prefix + " version still invalid after reinstall: " + err.Error())
			return err
		}
	}
	switch extMajor, mine := aibp.MajorVersion(ext), aibp.MajorVersion(aibp.Protocol); {
	case extMajor == mine:
		report(prefix + " ready")
		return nil
	case extMajor < mine:
		report(e.AgentName() + " protocol outdated, please upgrade")
		return errExtensionOutdated
	default:
		report("microNeo protocol outdated, please upgrade")
		return errMicroNeoOutdated
	}
}
```

**设计要点**：
- `report` 在 `Ensure` 内部被频繁调用——每一步业务进展、每一种结果都通过 `report` 通知
- 调用方（`CheckAibpCmd`）**不**做消息处理——`Ensure` 是消息的唯一生产者
- `Ensure` 仍返回 error 让调用方知道 success / failure，但**仅供控制流**（不用于显示）

`ensure_pi_test.go` 不调 `Ensure`，不会破坏现有测试。

---

## 四、验证计划

### 4.1 编译 & 测试

```bash
make build-quick
go test ./internal/aibp/ensure_agents
```

必须无 warning、无 error，测试全绿。

### 4.2 手动验证

| # | 命令 | 场景 | 步骤 | 期望 |
|---|---|---|---|---|
| U1 | `:test-info` | 验证 `InfoBarNow` 机制本身 | 直接运行 | InfoBar 每秒更新一次，连续 5 条，肉眼能清楚看到每条停留 1 秒（**不是**一闪而过） |
| U2 | `:check-agent` | 常态快路径 | 已装 aibp-pi 且版本兼容 | InfoBar 闪过 "checking pi ..." → "checking protocol version ..." → "ready" |
| U3 | `:check-agent` | **首次安装 / 网络慢**（核心验收点） | 全新环境：`pi remove npm:aibp-pi` 后运行 | InfoBar 显示 "aibp-pi downloading....." 并**停留可见数秒** |
| U4 | `:check-agent` | 未装 pi | PATH 去 pi 后运行 | InfoBar 显示 "pi not found, please install it first" |
| U5 | `:check-agent` | 扩展损坏自愈 | 删 `<piAgentDir>/npm/node_modules/aibp-pi/package.json` 后运行 | 看到 "checking aibp-pi protocol version ..." → "aibp-pi reinstalling (corrupted) ..." → 安装 → "aibp-pi ready" |
| U6 | 任意 | 画面一致性 | U3 期间观察编辑器其它 pane | 各 pane 内容应保持上一帧不变，不撕裂不残留 |
| U7 | `:check-agent` | `set infobar false` 后运行 | 先 `set infobar false`，再 `:check-agent` | InfoBar 消息不可见（预期行为），无 crash |

### 4.3 验收清单

- [ ] `make build-quick` 干净通过
- [ ] `go test ./internal/aibp/ensure_agents` 全绿
- [ ] U1 通过：`:test-info` 每秒更新一次，肉眼可见
- [ ] U2-U5 通过：`:check-agent` 中间消息实时显示
- [ ] U7 通过：`:check-agent` 在 `infobar=false` 下不 crash（InfoBar 不可见是预期）
- [ ] `git diff` 核对仅 2 个文件变化（`command_neo.go` 改 + `ensure.go` 改），无 micro 原生文件改动

---

## 五、风险与回滚

| # | 风险 | 应对 |
|---|---|---|
| F1 | `InfoBarNow` 必须在主 goroutine 调用 | 文档说明；目前唯一调用方（`CheckAibpCmd` / `TestInfoCmd`）都在主 goroutine 同步阻塞期间 |
| F2 | `TestInfoCmd` 留在主干 | 保留——microNeo 开发团队内部回归测试用，不在用户文档 / README / 帮助里出现 |

**回滚**：`git checkout internal/action/command_neo.go internal/aibp/ensure_agents/ensure.go` 恢复。

---

## 六、不在本计划范围内

| # | 事项 | 说明 |
|---|---|---|
| OOS1 | `InfoPane.msgNow` 薄封装 | InfoPane 跨界指挥"全屏 redraw"破坏职责清晰度，不推荐 |
| OOS2 | selectPane / notePane / FloatFrame 内的 hint 显示 | 不需要新 helper——主 goroutine 在 DoEvent 循环里，每帧 DoEvent 顶部自动 redraw。直接 `InfoBar.Message(...)` + `screen.Redraw()` 即可 |
| OOS3 | `screen.Redraw()` 改造为同步阻塞场景有效 | 需改 micro 主循环机制，**远超** microNeo "最小侵入"原则 |
| OOS4 | 异步命令的"立即显示消息" | 需 channel 转发到主 goroutine，等真有此场景再加 |

---

## 七、实施步骤

| 步骤 | 文件 | 动作 |
|---|---|---|
| 1 | `command_neo.go` | 按 §2.1 写入完整目标态（`RegisterCommands` + `InfoBarNow` + `CheckAibpCmd` + `TestInfoCmd`） |
| 2 | `ensure.go` | 按 §三 实施 `Reporter` 参数 + 内部所有路径 `report` |
| 3 | 验证 | `make build-quick` + `go test ./internal/aibp/ensure_agents` |
| 4 | 手动验证 | 按 §4.2 跑 U1-U7 |

**预估改动量**：约 50 行（含 `TestInfoCmd`），2 个改动文件，0 个新建文件。
