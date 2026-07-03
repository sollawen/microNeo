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

## 安装

两种方式：**正式安装**（持久，推荐）或**开发期临时加载**（改源码即测，免发布）。

> 共同前置：**bun** 在 `PATH` 上（monitor 用它跑 `index.ts`）。源码零外部依赖，bun 直接加载 `.ts`，无需预编译、无需 `npm install`。

### 方式一：正式安装（npm + marketplace，推荐）

aibp-claude 以 npm 包 `aibp-claude` 发布（与 [`aibp-pi`](../pi) / [`aibp-opencode`](../opencode) 同渠道）。Claude 经 microNeo 自建的 marketplace（`microNeo-plugins`）安装——**装一次，之后任何 `claude` 启动自动加载**，无需每次带 flag、无需 clone microNeo。

```bash
# 1. 登记 marketplace（一次性）
claude plugin marketplace add sollawen/microNeo-plugins

# 2. 安装插件（一次性，默认 user scope → 进 enabledPlugins）
claude plugin install aibp-claude@microNeo-plugins
```

之后直接 `claude` 即可，插件自动起。验证：`/plugin list` 应见 `aibp-claude@microNeo-plugins`。

**更新**：第三方 marketplace 的自动更新默认**关闭**。拿新版两种方式——

- 手动：`claude plugin update aibp-claude@microNeo-plugins`（版本没变就跳过）。
- 自动：交互界面 `/plugin` → Marketplaces → 选 `microNeo-plugins` → `Enable auto-update`（开一次，之后每次启动自动拉新版并提示 `/reload-plugins`）。

### 方式二：开发期临时加载（--plugin-dir）

改 `index.ts` 后**重启 Claude 即生效**，无需 `npm publish`，适合本地迭代：

```bash
claude --plugin-dir /path/to/microNeo/internal/aibp/aibp-agents/claude
```

> 与方式一同名时，`--plugin-dir` 的本地目录在该 session **优先生效**（官方设计：便于测已装插件的改动），无需先卸 npm 版。

### 第三方 LLM 中转（ccmm 风格）

经 `ANTHROPIC_BASE_URL` 走 MiniMax / GLM / DeepSeek 等中转的用户：**先用方式一装好插件**（marketplace 安装与 LLM 来源无关），再把中转 env 包成 shell 函数——**无需** `--plugin-dir`（插件已自动加载）：

```bash
ccaibp(){
    ANTHROPIC_BASE_URL="https://your-relay.example/anthropic" \
    ANTHROPIC_AUTH_TOKEN="your-token" \
    ANTHROPIC_MODEL="your-model" \
    claude "$@"
}
```

然后 `ccaibp` 起会话即可，`"$@"` 透传额外参数。

### Bash 权限（实测：通常不需要）

monitor 是 Claude 按 `plugin.json` 声明自动 spawn 的子进程，不走 LLM 的 Bash 工具门禁——插件一经 install/enable（=用户明确同意）即预信任。实测在 `~/.claude/settings.json` 无任何 `permissions` 段时也能正常 spawn。

极少数情况若仍弹权限提示，往 `~/.claude/settings.json` 顶层加 allow 规则（勿覆盖现有字段）：

```jsonc
{ "permissions": { "allow": ["Bash(bun \"${CLAUDE_PLUGIN_ROOT}/index.ts\")"] } }
```

## 卸载

### 仅卸载插件（保留 marketplace）

```bash
claude plugin uninstall aibp-claude@microNeo-plugins
```

- 从 `enabledPlugins` 与 `installed_plugins.json` 移除；cache 目录标记为 orphan、7 天后自动删（本插件零依赖，`--prune` 无差别）。
- `${CLAUDE_PLUGIN_DATA}` 目录默认随之删除（本插件不使用该目录，无影响）；想保留加 `--keep-data`。
- 卸载后再 `claude`，monitor 不再加载；microNeo 端发送会找不到接收端。

### 彻底移除（连 marketplace）

```bash
claude plugin uninstall    aibp-claude@microNeo-plugins
claude plugin marketplace remove microNeo-plugins
```

npm 包与 GitHub `sollawen/microNeo-plugins` 仓库不受影响，随时可重新 `marketplace add` + `install` 装回。

### 禁用但不卸载（排查用）

保留 cache、仅停止加载：

```bash
claude plugin disable aibp-claude@microNeo-plugins
# 重新启用：
claude plugin enable  aibp-claude@microNeo-plugins
```

### 从 marketplace 版切回开发期 `--plugin-dir`

无需卸载——`--plugin-dir` 在该 session 优先于已装版本（见「安装·方式二」）：

```bash
claude --plugin-dir /path/to/microNeo/internal/aibp/aibp-agents/claude
```

若用 `ccaibp` wrapper，把 `.zshrc` 里 `claude "$@"` 临时改回 `claude --plugin-dir <上述路径> "$@"` 即可。

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

1. **退出清理**：**正常退出**（`:exit` / `/quit` / Ctrl-D）干净无残留——Claude 发 SIGTERM → daemon 的 `process.on('SIGTERM')` → `process.exit(0)` → 同步触发 `process.on('exit')` 跑 `unlinkSync(regFile)` + `unlinkSync(socketPath)`（见 `index.ts` 末尾信号处理）。**仅异常终止**才会残留 `ai-<name>.json` + `.sock`，例如：直接关 terminal 窗口（发 SIGHUP，daemon 未注册该 handler）、`kill -9`、Claude 崩溃、机器硬重启。残留无害：下次任一 AIBP agent 启动时 `allocateName` 的 GC 会探测 pid 存活、死的 unlink。与 pi/opencode 同一套容错设计。
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

- 设计与选型：`aibp-claude-实施方案.md`（microNeo 仓库内）
- 源码与协议详情：[microNeo](https://github.com/sollawen/microNeo)
