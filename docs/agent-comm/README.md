# agent-comm

microNeo ↔ ai agent 通信系统的设计/实现文档。

## 是什么

让用户在 microNeo 编辑文件时，按需、一次性地把光标/选区 + 用户消息推送给 ai agent（pi、opencode…）。**v1 单向、用户主动、不做持续追踪**。

## 文档结构

| 文档 | 角色 | 何时读 |
|------|------|--------|
| [`说明-架构设计`](./说明-架构设计.md) | **总纲** — 分层 / 原则 / 决策 / 边界 / 状态 | **先读** |
| [`说明-EABP`](./说明-EABP.md) | **协议层权威** — 注册表 / 报文 / 契约 | 协议相关必读 |
| [`说明-发送端`](./说明-发送端.md) | microNeo 端实现（采集 + 拼报文 + 发送） | 改发送端 |
| [`说明-接收端`](./说明-接收端.md) | pi 端实现（注册 + 监听 + 递送 LLM） | 改 pi 端 |
| [`说明-notepane`](./说明-notepane.md) | notePane 浮窗（buffer / binding / 键位） | 改 notePane |
| [`通信的原始需求`](./通信的原始需求.md) | 需求源头 | 理解"为什么" |
| [`用户界面-V2`](./用户界面-V2.md) | notePane UI 原意象 | 理解产品意图 |
| [`opencode调研`](./opencode调研.md) | opencode 接收端调研（**未实现**） | 接 opencode 时 |

## 关键架构事实

- **LSP 式**：microNeo 只定义协议，agent 按协议实现接收端即可接入。**接新 agent 不改 microNeo 任何代码**。
- **零 Lua 钩子**：发送端全 Go 写死，符合 micro 原生零侵入。
- **注册表 = 文件系统目录**：`$XDG_RUNTIME_DIR/microneo-agent-bridge-$UID/receiver-*.json`。无独立进程。
- **传输 = Unix socket + 逐行 JSON**：每次发送 connect → write 一行 → close。**无状态管理**。
- **line/col 1-based**：与 LLM 工具链（sed/ripgrep/read）天然对齐。
- **`message` 有无决定递送路径**：
  - 有 `message` → 立即/排队触发 LLM 对话
  - 无 `message` → 递送为待用上下文，不触发对话

## 当前实现状态

| 阶段 | 状态 |
|------|------|
| Phase 0 协议定稿 | ✅ |
| Phase 1 最小闭环（单 pi、纯上下文暂存） | ✅ |
| Phase 2 带 message + 选区 + 空拦截 | ✅ |
| Phase 3 opencode 接收端 | 📋 调研完成，**未实现** |
| Phase 4 扩展（长连接 / 双向 / Windows / 多 receiver 选择 UI / 隐私黑名单） | 📋 留 v2 |

**代码位置**：
- microNeo：`internal/eabp/` + `internal/action/notepane.go`
- pi：`eabp-receivers/eabp-pi/index.ts`（~95 行 TS）

## 改东西时

| 任务 | 先读 |
|------|------|
| 改协议字段 / 报文 schema | `说明-EABP.md` §五（`internal/eabp/message.go` 单份，无镜像） |
| 改 notePane 行为 | `说明-notepane.md`（单文件 `internal/action/notepane.go`） |
| 改 pi 端 LLM 递送 | `说明-接收端.md`（单文件 `eabp-receivers/eabp-pi/index.ts`） |
| 加新 agent 接收端 | `说明-EABP.md` §八（接收端契约）+ 调研 `opencode调研.md` 作为参考 |
| 理解"为什么这样设计" | `说明-架构设计.md` §四（5 条原则）+ §六（14 条决策） |

## 注意事项

- **调试工具在 `internal/eabp/cmd/`（discover / send）**：与协议代码同 module，直接 import `internal/eabp`，无镜像维护负担。`make build` 不编译它们，需单独 `go build ./internal/eabp/cmd/<tool>`。
- **notePane buffer 生命周期**：`open()` 总是 Close 旧 + 新建（不是 close 时销毁）——避免 BufPane.HandleEvent 内 nil 访问 panic（v1 → v2 修复历史）。
- **不发送 `visible_lines`**：v1 不发送可见区域文本（隐私考虑）。
- **`MNAB_REG_DIR` 调试覆盖**：两端都支持，调试时设个短路径方便手查注册文件。
- **D0-D10、notePane实施计划已清理**：详细历史可查 git log + commit message。

## 开放问题（高层）

1. opencode 接收端何时实现？
2. 双向通信的授权模型？
3. Windows 支持？
4. 多 receiver 选择 UI？
5. 隐私黑名单？

详细见 `说明-架构设计.md` §九 + `说明-EABP.md` §十二。
