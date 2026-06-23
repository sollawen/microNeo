# D18 · `:check-agent` 同步阻塞期间刷新 InfoBar 显示

> **状态**：方案待评审。`D17` 已实施完毕，本方案在其基础上小幅增量。
> **目标**：`pi install` 联网几秒，期间屏幕静止像死机——通过 InfoBar 实时刷出 "AIBP downloading....." 等中间消息。

---

## 一、问题与根因

### 现象
`:check-agent` 走到 `InstallAIBP()`（`pi install npm:aibp-pi` → `npm install` 联网）时阻塞几秒。期间屏幕完全静止，用户以为卡死。

### 根因
`CheckAibpCmd` **同步运行在主 goroutine**上，与主循环 `DoEvent`（`cmd/micro/micro.go:468`）同线程：

```
DoEvent() 顶部：Display 全部 → Show → select 阻塞等事件
   ↑ 收到事件 → InfoBar 回调 → CheckAibpCmd() ──同步阻塞──► pi install 跑完才返回
   │                                                              │
   └───────────── 下一轮 DoEvent 顶部才重新 Display ◄──────────────┘
```

`DoEvent` 的显示阶段在**循环顶部**（`micro.go:478-491`），只有上一轮事件处理完返回后、下一轮才会重新 `Display` + `Show`。`CheckAibpCmd` 一阻塞，期间：
- `InfoBar.Message(...)` 只是把消息写进 `InfoBuf.Msg` 字段（`infobuffer.go:41`），**不触发刷屏**
- 真正的 `InfoBar.Display()`（写 backbuffer）和 `screen.Screen.Show()`（刷终端）都在被阻塞的 `DoEvent` 顶部

所以即便命令中途 `InfoBar.Message("安装中...")`，**这条消息直到命令返回才显示**——用户看不到。**光调用 `InfoBar.Message` 没用，必须额外强制立即刷屏。**

### 同步阻塞的连带后果
不只是显示冻结——期间 `DoEvent` 卡住，**整个编辑器暂停**：键盘输入堆积不响应、自动保存停、后台 Job 回调不执行、定时器全停。`CheckAibpCmd` 不返回，什么都转不动。

---

## 二、方案选型：为什么是 InfoBar 刷新，而非内嵌终端

### 候选方案
- **B（采用）：保持同步 `Ensure`，加 `Reporter` 回调 + `redrawNow` 在阻塞中途刷 InfoBar**
- A（否决）：开 split 用内嵌终端跑 install，实时显示 npm 输出

### 为什么选 B

**install 时长是关键变量**：`aibp-pi` 是很小的包，`npm install` 有缓存 1-3 秒、首次 5-10 秒，极少到几十秒。现实场景下冻结几秒 + 底部有字，完全可接受。

**复杂度 / 风险对比**：
| | B | A |
|---|---|---|
| 改动量 | `ensure.go` 加 `Reporter` 参数 + `command_neo.go` 加 2 行 `redrawNow` ≈ **15 行** | 手搓 micro 没有现成 helper 的"开 split 跑 term"：操作 Node 树、替换 Panes 数组、placeholder buffer 管理、自动关闭时序 |
| 风险 | 复刻 DoEvent 已验证的显示序列，几乎不会出 bug | 深度操控 micro UI 核心数据结构，Node 树/Panes 状态一旦不一致调试成本极高 |
| 资源 | 只多一帧绘制 | 多一个子进程 + PTY + 渲染 goroutine |

A 的优雅是理论上的，B 的简单是实打实的。为"几秒安装能不能看 npm 进度条"承担 A 的工程风险，过度工程。

> **务实路径**：先上 B 低风险立即解决“假死”困惑。若将来真有用户抱怨 install 慢、冻结难受，那时再升级到 A，届时也有真实耗时数据支撑决策。现在 B 足够。

> **方案 A 的权衡**：A 的体验更佳（用户可看 npm 真实输出，且 install 期间编辑器不冻结）。当前不选 A 是因为工程投入与体验收益不成正比：A 需手搓 micro 没有现成 helper 的 split+term（操作 Node 树、替换 Panes 数组、placeholder buffer 管理、自动关闭时序），而 install 通常 1-10 秒，冻结 + 底部有字可接受。若将来证明 install 耗时确实成为问题，A 是清晰的演进方向。

---

## 三、方案

两个独立的小机制组合：

| 机制 | 作用 | 改动位置 |
|------|------|---------|
| **① 进度回调** `Reporter` | 把 `Ensure` 各步骤的进度消息向上传给命令层 | `internal/aibp/ensure_agents/ensure.go` |
| **② 强制刷屏** `redrawNow()` | `InfoBar.Display()` + `screen.Screen.Show()`，立即把 InfoBar 消息刷到终端 | `internal/action/command_neo.go` |

命令层把两者串起来：`report(msg)` → `InfoBar.Message(msg)` + `redrawNow()`。

### 3.1 机制①：`Ensure` 增加 `Reporter` 回调（不破坏纯净性）

`Ensure` 仍**不依赖 action 包**——它只接收一个 `func(string)` 回调，由调用方决定怎么展示（InfoBar+刷屏 / 记日志 / 测试里吞掉）。`Ensure` 自己不知道消息最终去哪。

```go
// ensure.go

// Reporter 接收 Ensure 各步骤的进度消息。
// 由调用方（command_neo.go）决定如何展示；Ensure 自身不依赖 UI。
// 传 nil 表示不需要进度反馈（如测试）。
type Reporter func(msg string)

func Ensure(e AgentEnsurer, report Reporter) error {
    if report == nil {
        report = func(string) {} // nil → 静默
    }

    report("checking " + e.AgentName() + " ...")
    if !e.HasAgent() {
        return fmt.Errorf("%s not found, please install it first", e.AgentName())
    }

    if !e.HasAIBP() {
        report("AIBP downloading.....")
        return e.InstallAIBP()   // ← 阻塞点：网络慢时停留最久
    }

    report("checking protocol version ...")
    ext, err := e.AIBPVersion()
    if err != nil {
        report("reinstalling (corrupted) ...")
        return e.InstallAIBP()
    }
    switch extMajor, mine := aibp.MajorVersion(ext), aibp.MajorVersion(aibp.Protocol); {
    case extMajor == mine:
        return nil // 兼容
    case extMajor < mine:
        return errExtensionOutdated
    default:
        return errMicroNeoOutdated
    }
}
```

关键点：**`InstallAIBP` 阻塞前**先 `report("AIBP downloading.....")` 并由命令层刷屏——这是用户在等待期间唯一能看到的那条消息，**核心价值就在这一句**。

`nil` 兜底：测试和未来不关心进度的调用方传 `nil` 即可，签名只多一个参数。

**对现有测试的影响**：`ensure_pi_test.go` 只测 `HasAIBP` / `AIBPVersion` 两个方法，**没有直接调 `Ensure`**。所以加参数不会破坏现有测试，无需改测试文件。

### 3.2 机制②：`redrawNow` 只刷 InfoBar（两行）

```go
// command_neo.go

import (
    "github.com/micro-editor/micro/v2/internal/aibp/ensure_agents"
    "github.com/micro-editor/micro/v2/internal/screen"
)

// redrawNow 立即强制把 InfoBar 当前消息刷到终端。
// microNeo 自有：同步命令（如 :check-agent 的 pi install）执行期间主循环 DoEvent 被阻塞，
// 其顶部的 Display+Show 不会运行，InfoBar.Message 设置的消息刷不到屏幕，需手动刷新。
// 复刻 DoEvent 顶部的显示阶段（micro.go:478-491），确保 backbuffer 里所有行都有正确内容，
// Show() 做 diff 只把变化的单元格刷到终端。必须在主 goroutine 调用（与 DoEvent 同线程）。
func redrawNow() {
	screen.Screen.Fill(' ', config.DefStyle)
	screen.Screen.HideCursor()
	Tabs.Display()
	for _, ep := range MainTab().Panes {
		ep.Display()
	}
	MainTab().Display()
	InfoBar.Display()
	if TheNotePane != nil && TheNotePane.IsOpen() {
		TheNotePane.Display()
	}
	if TheFloatFrame.IsOpen() {
		TheFloatFrame.Display()
	}
	screen.Screen.Show()
}
```

**为什么只刷 InfoBar、不整屏复制**：
- 同步阻塞期间编辑器冻结，**没有任何代码改动其它 pane 的内容**，backbuffer 里 InfoBar 行以外的东西必然还是上一帧的样子
- `InfoBar.Display()` 把新消息写进 backbuffer 那一行，`Show()` 做 diff 只把变化的单元格刷终端
- 整屏复刻（`Fill` 清屏 + 各 pane `Display` + `HideCursor`）全是多余的：改不了 InfoBar 行以外的东西，`Fill` 清屏反而可能一帧闪烁

### 3.3 串起来：`CheckAibpCmd` 传回调

```go
// command_neo.go

func (h *BufPane) CheckAibpCmd(args []string) {
    // progress 回调：设消息 + 立即刷屏。
    // 主循环 DoEvent 在本命令期间被阻塞，必须 redrawNow 才能让中间消息可见。
    report := func(msg string) {
        InfoBar.Message("aibp-pi: " + msg)
        redrawNow()
    }

    if err := ensure_agents.Ensure(ensure_agents.PiEnsurer{}, report); err != nil {
        InfoBar.Message("aibp-pi: " + err.Error())
        return
    }
    InfoBar.Message("aibp-pi: ready")
    // 最终消息（ready/出错）由命令返回后 DoEvent 下一轮顶部自动刷新，无需手动 redraw
}
```

**最终消息为何不 `redrawNow`**：`CheckAibpCmd` 返回后控制权回到主循环，下一轮 `DoEvent` 顶部第一件事就是 `Display`+`Show`，最终消息立即出现。

---

## 四、运行时观感

| 阶段 | 用户看到（InfoBar） | 耗时 |
|------|--------------------|------|
| 进入命令 | `aibp-pi: checking pi ...` | <1ms |
| 已装且兼容（常态快路径） | `aibp-pi: checking protocol version ...` → `aibp-pi: ready` | 几 ms |
| **首次安装 / 网络慢** | `aibp-pi: AIBP downloading.....` ← **停留数秒** → `aibp-pi: ready` | 阻塞期间有明确反馈 |
| 出错 | `aibp-pi: <原因>`（如 `pi not found, please install it first`） | — |

核心收益：**用户在 "AIBP downloading....." 这段时间看到的是明确进度文字，而不是静止画面**。期间编辑器冻结（输入不响应），但 aibp-pi 是小包，冻结 1-10 秒可接受。

---

## 五、设计决策

| # | 决策 | 理由 |
|---|------|------|
| E1 | **保持同步执行**，不引入异步 Job / 内嵌终端 | install 通常 1-10 秒，冻结几秒 + 底部有字可接受；异步化破坏 `Ensure` 纯同步 API 且引入竞态。详见 §二方案选型 |
| E2 | **进度用回调 `Reporter` 注入 `Ensure`** | `Ensure` 保持"不依赖 action"（D17 §3.1），只收 `func(string)`；展示方式由调用方决定。测试传 `nil` |
| E3 | **`redrawNow` 复刻 DoEvent 顶部的显示阶段**（Fill + HideCursor + Tabs/Panes/InfoBar/NotePane/FloatFrame Display + Show） | 同步阻塞期间 DoEvent 顶部不运行，必须手动复制其显示序列；只刷 InfoBar 会导致其它行被 Fill 清空（backbuffer 中是空格），Show() 会把它们刷成空白 |
| E4 | **不做 spinner / 倒计时 / 流式输出 npm 日志** | ① spinner 需后台 ticker + 停止协调，复杂且 micro 风格不用 ② npm 输出逐行刷进单行 InfoBar 会闪烁噪音 ③ 步骤级文字（"AIBP downloading....."）已足以消除"死机"错觉 |
| E5 | **最终消息（ready/出错）不手动 redraw** | 命令返回后 DoEvent 下一轮顶部立即刷新；手动刷无害但冗余 |

---

## 六、文件清单与侵入面

| 文件 | 状态 | 改动 |
|------|------|------|
| `internal/aibp/ensure_agents/ensure.go` | 改 | `Ensure` 签名加 `report Reporter` 参数 + `nil` 兜底 + 各步骤 `report(...)`（~10 行） |
| `internal/action/command_neo.go` | 改 | 加 `redrawNow()` helper（~15 行，复刻 DoEvent 显示序列）+ import `screen`；`CheckAibpCmd` 构造 `report` 闭包传入（~5 行） |

**对 micro 原生代码的侵入**：**零**。两个文件都是 microNeo 自有（`command_neo.go` 自不待言；`ensure.go` 在 microNeo 自建的 `internal/aibp/` 目录）。不动 `micro.go`、`DoEvent`、`screen.go`、`infobuffer.go`。

**未触及**：`ensure_pi.go`（PiEnsurer 实现无需改）、`ensure_pi_test.go`（不调 `Ensure`）、`registry.go`。

---

## 七、实施计划

单 Phase 即可（改动很小）。

| 步骤 | 文件 | 验证 |
|------|------|------|
| 1 | `ensure.go`：加 `Reporter` 类型 + `Ensure` 加参数 + 各步骤 `report` | `make build-quick` 过；`go test ./internal/aibp/ensure_agents` 全绿（签名变了但测试不调 Ensure，不受影响） |
| 2 | `command_neo.go`：加 `redrawNow` + import；`CheckAibpCmd` 传 `report` | `make build` 过 |
| 3 | 手动验证（见 §八） | 进度消息在网络慢/首次安装时可见 |

---

## 八、测试与验收

| # | 场景 | 步骤 | 期望 |
|---|------|------|------|
| U1 | 常态快路径（已装且兼容） | 运行 `:check-agent` | InfoBar 快速闪过 "checking pi ... → checking protocol version ... → ready"，肉眼几乎只见 "ready" |
| U2 | **首次安装 / 网络慢** | 全新环境（`pi remove npm:aibp-pi` + 删 settings 条目后）运行 `:check-agent` | InfoBar 显示 "aibp-pi: AIBP downloading....." 并**停留可见数秒**（关键验收点：期间不是静止旧画面），安装完变 "ready" |
| U3 | 未装 pi | PATH 去 pi 后运行 | InfoBar 显示 "aibp-pi: pi not found, please install it first" |
| U4 | 扩展损坏自愈 | 删 `<piAgentDir>/npm/node_modules/aibp-pi/package.json` 后运行 | 先看到 "checking protocol version ..." → "reinstalling (corrupted) ..." → 安装 → "ready" |
| U5 | 画面一致性 | U2 期间观察编辑器其它 pane 内容 | 冻结期间各 pane 内容应保持上一帧不变，不撕裂不残留 |
| U6 | **infobar 设置为 off** | `set infobar false` 后运行 `:check-agent`（常态路径或首次安装） | InfoBar.Display() 内部 `if !config.GlobalSettings["infobar"].(bool) { return }` 会直接返回，redrawNow 刷了也白刷，消息不可见 |

**验收清单**：
- [ ] `make build` 干净通过
- [ ] `go test ./internal/aibp/ensure_agents` 全绿
- [ ] U1–U5 通过，尤其 U2："安装中"消息在等待期间确实可见
- [ ] 无 micro 原生文件改动（`git diff` 核对仅两个 microNeo 文件）

---

## 九、风险与回滚

| # | 风险 | 应对 |
|---|------|------|
| F1 | `redrawNow` 与轮询 goroutine 的 `PollEvent` 并发 | 与 `DoEvent` 自身 `Show` 调用同上下文（主 goroutine）；tcell 内部保证 `Show`/`PollEvent` 线程安全。micro 的 `screen.Lock` 只防 `TempFini` 置 nil，正常运行期 `Screen` 稳定非 nil。**注**：`TempFini` 可将 `Screen` 置 nil，但同步命令执行期间主循环未处理事件，不会触发 `TempFini` 路径；风险极低。可选择性在 `redrawNow` 里加 nil 检查。 |
| F2 | `Ensure` 签名变更影响其它调用方 | 全局仅 `command_neo.go` 一处调用 `Ensure`；测试不调。已 grep 确认 |

**回滚**：`git checkout` 这两个文件即可，无新增文件、无配置变更。
