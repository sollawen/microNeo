# aibp-claude

AIBP (AI Bridge Protocol) 接收端插件，让 **Claude Code** 成为 microNeo 的 AI 接收端：在 microNeo 里选中代码按 Alt-Enter（或带消息发送），内容递送到当前运行的 Claude 会话并**立即触发 Claude 响应**。

## 工作原理

- **形态**：Claude Code 的 **plugin Monitor**（background monitor）。`plugin.json` 声明一个 monitor，Claude 启动时自动 spawn `index.ts`（daemon）作为子进程，session 结束时终止。
- **daemon**：分配 AIBP 名字（与 pi/opencode 共用名字池）、监听 Unix socket、注册到 `$XDG_RUNTIME_DIR/aibp-<uid>/`（fallback `$TMPDIR/aibp-<uid>/`）。
- **递送**：microNeo 经 socket 发来报文 → daemon 格式化（选区内联 / `@path` 引用 + 消息）→ 写一行 JSON 到 stdout → Claude 当作 monitor 事件接收 → **立即触发一次 LLM 回合**（Claude 主动响应，无需用户按键）。
- **协议**：与 [`aibp-pi`](../pi) / [`aibp-opencode`](../opencode) 同协议（`aibp-2.0`），共用名字池与 registryDir，并存时自动分配不同名字（Alpha / Bravo / …）。

> **为什么不用 MCP channel**：channel 受 Claude 认证限制，第三方 LLM 用户（经 `ANTHROPIC_BASE_URL` 走 MiniMax / GLM / DeepSeek 等中转）不可用。Monitor 不受此限制——这是本插件能服务第三方 LLM 用户的关键。

## 前置条件

- **Claude Code ≥ 2.1.105**（plugin Monitor 支持）
- **交互式 CLI 会话**（非 CI / 脚本 / headless 模式）
- **未设 `DISABLE_TELEMETRY` 与 `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC`**（任一设置都会完全禁用 Monitor）
- **bun** 在 `PATH` 上（运行 `index.ts`）

## 激活

通过 `claude --plugin-dir` 加载本插件目录。源码在 `internal/aibp/aibp-agents/claude/`（microNeo 仓库内），bun 直接加载 `.ts`，无需预编译、无需 `npm install`（零外部依赖）。

### 方式一：直连 Anthropic

```bash
claude --plugin-dir /path/to/microNeo/internal/aibp/aibp-agents/claude
```

### 方式二：第三方 LLM 中转（ccmm 风格）

把中转 env 和 `--plugin-dir` 一起包成 shell 函数：

```bash
ccaibp(){
    ANTHROPIC_BASE_URL="https://your-relay.example/anthropic" \
    ANTHROPIC_AUTH_TOKEN="your-token" \
    ANTHROPIC_MODEL="your-model" \
    claude --plugin-dir /path/to/microNeo/internal/aibp/aibp-agents/claude "$@"
}
```

然后 `ccaibp` 起会话即可。`"$@"` 透传额外参数。

### Bash 权限（视情况）

monitor 命令是 `bun "${CLAUDE_PLUGIN_ROOT}/index.ts"`。若启动时 Claude 弹 Bash 权限提示，在 `~/.claude/settings.json` 加 allow 规则（plugin 声明的 monitor 通常预信任，多数情况不会弹）：

```jsonc
{ "permissions": { "allow": ["Bash(bun \"${CLAUDE_PLUGIN_ROOT}/index.ts\")"] } }
```

## 行为（v1）

**§6.3：始终触发** —— 收到任何报文都立即让 Claude 响应（对齐 aibp-opencode）：

- 带选区 + 消息 → Claude 收到 `<selection path=... lines=...>…</selection>` + `<user-input>消息</user-input>`，立即响应
- 带选区无消息 → 选区作为上下文，仍触发响应
- 无选区 → `@path :lineN` 引用，仍触发响应

界面呈现为 `Monitor event: "microNeo editor context & messages (AIBP)"` 后接 Claude 的响应。

> §6.4（纯上下文不触发、仅填输入框待用户编辑）暂未实现，与 aibp-opencode 一致。

## 协议版本

`aibp-2.0`

## 注意事项

1. **退出残留（预期）**：Claude 退出时强制终止 monitor 子进程，daemon 的 `process.on('exit')` 清理跑不到，registry 会短暂残留僵尸 `ai-<name>.json` + `.sock`。**下次任一 AIBP agent 启动时，`allocateName` 的 GC 会自动清掉**（探测 pid 存活，死的就 unlink），无害。这与 pi/opencode 是同一套容错设计。
2. **日志**：`/tmp/aibp-claude.log`（可用 `MNAB_LOG` 环境变量覆盖路径）。
3. **多 session**：同时开多个 Claude 会话，各抢不同名字，互不冲突。
4. **stdout 专用**：daemon 的 stdout 是 Monitor 事件流，**严禁 `console.log`**（会污染成伪事件）；诊断一律走日志文件。

## 验证

```bash
# Claude 起来后，注册文件应已生成
ls "$TMPDIR/aibp-$(id -u)/"        # 见 ai-Bravo.json 等
# Claude task panel / monitor 列表应显示 aibp-daemon
```

然后在 microNeo 里选中代码按 Alt-Enter 发消息，Claude 应立即开始 streaming 响应。

## 相关

- 设计与选型：`aibp-claude-实施方案.md`
- 源码与协议详情：[microNeo](https://github.com/sollawen/microNeo)
