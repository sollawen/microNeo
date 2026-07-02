# D27 · aibp-claude —— microNeo → Claude Code 的代码递送通道

> **状态**：方案，待实施。
> **目标**：让 microNeo 通过 AIBP 协议把选区/消息递送到当前运行的 Claude Code session。
>
> **前置阅读**：D17（协议）、D19（aibp-opencode 设计）、`aibp-agents/pi/index.ts`、`aibp-agents/opencode/index.tsx`（共享的协议层代码）。本文件**只讲差异**——和 aibp-pi/aibp-opencode 同质的部分（名字池、registryDir、formatText、分帧、版本校验）请直接参照那两份代码。

---

## 一、一句话总结

**Claude Code Channel 插件**（MCP server over stdio），让 microNeo 通过 Unix socket 推过来的选区/消息，在 Claude Code session 里以 `<channel source="aibp">` 形式注入。

```
microNeo  ──Unix socket──▶  Channel MCP Server (aibp-channel)
                                  │
                                  │ MCP stdio
                                  ▼
                              Claude Code
                            （<channel> 注入上下文）
```

---

## 二、和 aibp-pi / aibp-opencode 的根本差异

| 维度 | aibp-pi / aibp-opencode | aibp-claude |
|------|------------------------|-------------|
| 扩展机制 | pi / opencode **自身进程的扩展**（API plugin） | **Claude Code spawn 子进程**（MCP server over stdio） |
| 装载方式 | pi 用 `pi install`；opencode 用 `opencode plugin` | `claude plugin install` / `claude --plugin-dir` |
| 进程模型 | 一个进程 = plugin（无 stdio 概念） | **双接口进程**：MCP（stdio）把消息通知 Claude；Unix socket 接 microNeo 的消息 |
| 协议层 | AIBP | AIBP（完全沿用） |
| 名字池 / registry | 复用 | **完全复用**（`~/.config/aibp/aibp-names.json`、`$XDG_RUNTIME_DIR/aibp-<uid>/`） |
| 报文格式 | JSON + \n | **完全沿用** |
| 递送语义 | 全部直接送达 LLM（不区分 `p.message` 有无） | **`mcp.notification({method: 'notifications/claude/channel', ...})`** |
| 退出清理 | 自己清 footer / 自己清 registry | `mcp.close()` 触发 Claude 端清理；我们清 registry 文件 |

**关键结论**：协议层、registry、名字池、formatText、分帧 **全部沿用**。差异只在最末端的递送 API。

### 2.1 形态选型理由

| 选项 | 评估 |
|------|------|
| **Channel（MCP server）** | ✅ 唯一支持"从外部主动推内容进 session"的官方机制；stdin/stdout 通道天然契合 |
| Skills / slash command | ❌ Skills 是文本触发器（Claude 读 SKILL.md 决定何时调），**不能**用于把外部内容塞进 session |
| Hooks | ❌ Hooks 触发方是 Claude 自己（PostToolUse 等），不能反过来推 |
| Plugin + `.mcp.json` 包装 Channel | ✅ 这个就对了——Claude Code 的 plugin 形态天然支持 `.mcp.json`，Channel server 就定义在 `.mcp.json` 里 |

所以 `.claude-plugin/plugin.json` 用 `channels` 字段挂载 MCP server，是最直接的形态。**不是裸 MCP server 单独跑**——而是按 Claude 插件规范打包成可分发单元。

> ⚠️ **Channels 在 research preview**（v2.1.80+）。开发期需 `claude --dangerously-load-development-channels server:aibp-channel`（或 `plugin:aibp-claude`）。生产期需要上 Anthropic 的 allowlist 或自己托管 marketplace（**v1 不涉及发布，详见 §8.1/8.2**）。

---

## 三、目录结构

```
aibp-claude/
├── .claude-plugin/
│   └── plugin.json               # 插件清单（声明 name、version、channels 挂载）
├── .mcp.json                     # MCP server 配置（一个 channel 服务）
├── aibp-channel.ts               # ⭐ Channel MCP server：stdio（MCP）→ 通知 Claude；unix socket ← 接 microNeo
├── package.json                  # npm 包元信息（与 aibp-pi / aibp-opencode 同构）
├── ensure_claude-plan.md         # 给未来 Go 端 ensure_claude.go 留的设计笔记（可选）
└── README.md                     # 安装 / 验证 / 卸载
```

**单文件实现原则**：和 aibp-pi（95 行）/ aibp-opencode（430 行）的惯例一致——把所有逻辑塞进 `aibp-channel.ts` 一个 default export。

**不用 TypeScript JSX**：和 opencode 用 `@opentui/solid` 不同，Claude 的 channel server 是**纯 MCP**——没有 UI、没有 Solid 渲染。纯 TS + `@modelcontextprotocol/sdk` + node:net/fs。

---

## 四、关键文件规格

### 4.1 `.claude-plugin/plugin.json`

> Claude Code 的 plugins-reference（plugin manifest schema）规定，`channels` 字段引用 `.mcp.json` 里的 server。

```jsonc
{
  "name": "aibp-claude",
  "displayName": "AIBP Claude Receiver",
  "version": "1.0.0",
  "description": "AIBP (AI Bridge Protocol) receiver plugin for Claude Code — receive editor context from microNeo",
  "license": "MIT",
  "keywords": ["claude-plugin", "microNeo", "aibp"],
  // 把 aibp-channel 注册为 channel；server 字段匹配 .mcp.json 里的 key
  "channels": [
    {
      "server": "aibp-channel",
      "userConfig": {}
    }
  ]
}
```

**注意**：
- `name` 是 kebab-case（与 aibp-pi / aibp-opencode 同款，便于出现在 `/plugin` 列表时统一）
- `channels[].server` 引用 `.mcp.json` 里 `mcpServers` 的 key
- 不需要 `mcpServers` 字段在 plugin.json 里——已经按默认约定从 `.mcp.json` 读

### 4.2 `.mcp.json`

```jsonc
{
  "mcpServers": {
    "aibp-channel": {
      // Bun 跑 TS，无需预编译；运行时擦除 import type
      "command": "bun",
      "args": ["${CLAUDE_PLUGIN_ROOT}/aibp-channel.ts"],
      // 透传环境变量给 MCP 子进程
      "env": {}
    }
  }
}
```

**`${CLAUDE_PLUGIN_ROOT}`** 是 Claude Code 提供的环境变量（plugins-reference §Environment variables），自动解析为插件安装目录的绝对路径——开发期指向源码目录，安装期指向 cache 目录。

> 不写绝对路径——发布后路径会变（marketplace 安装会拷到 `~/.claude/plugins/cache/aibp-claude/<sha>/`）。

### 4.3 `package.json`

```jsonc
{
  "name": "aibp-claude",
  "version": "1.0.0",
  "aibp": { "protocol": "aibp-2.0" },     // ⭐ 与 pi/opencode 一致
  "description": "AIBP (AI Bridge Protocol) receiver plugin for Claude Code",
  "license": "MIT",
  "type": "module",
  "bin": { "aibp-channel": "./aibp-channel.ts" },  // 可选：让用户能 bun 直接跑
  "keywords": ["claude-plugin", "microNeo", "aibp"],
  "peerDependencies": {
    "@modelcontextprotocol/sdk": ">=1.0.0"
  },
  "files": [
    ".claude-plugin/plugin.json",
    ".mcp.json",
    "aibp-channel.ts",
    "package.json",
    "README.md"
  ]
}
```

**关键约束**（参考 D19 里 opencode 版的教训）：
- `aibp.protocol` 必须是 `"aibp-2.0"`——与发送端常量 `internal/aibp.Protocol` 字面对齐
- `type: "module"`（ESM 必须，跑 Bun）
- 没有 `main` 字段（Claude Code 不像 opencode 那样有 main→server 的误判，但养成一致习惯）
- `files` 必须含上述 5 个文件

### 4.4 `aibp-channel.ts` ⭐（核心实现）

结构分四段：

```
aibp-channel.ts
├── 段 A：常量与协议层           ← 复制自 aibp-pi/index.ts + aibp-opencode/index.tsx
│   ├── PROTOCOL / PROTOCOL_MAJOR 读 package.json 的 aibp.protocol
│   ├── DEFAULT_NAMES_STR          NATO A–O
│   ├── normalizeNames / loadNamePool / allocateName / registryDir
│   └── formatText                 与 aibp-pi/opencode 完全一致（含尖括号标签分隔）
│
├── 段 B：MCP server 构造           ← channels-reference 模式
│   ├── capabilities.experimental['claude/channel'] = {}    注册 channel
│   ├── instructions 文字          告诉 Claude <channel source="aibp"> 是啥
│   ├── mcp.connect(StdioServerTransport)
│
├── 段 C：Unix socket listener      ← 起在 MCP connect 之后，与 aibp-pi/opencode 同款
│   ├── loadNamePool → allocateName → listen unix socket
│   ├── 写 registry 文件（ai-<name>.json，agent 字段写 "claude"）
│   ├── connection handler：分帧 + JSON.parse + 版本校验 + 调段 D 的投递
│   └── cleanup on mcp close (process.on('SIGTERM') / process.exit)
│
└── 段 D：投递 — channel notification
    ├── handleLine(parsed env, ctx)
    ├── formatText(env.payload) → text
    ├── mcp.notification({
    │     method: 'notifications/claude/channel',
    │     params: {
    │       content: text,                              // 选区 + 用户消息
    │       meta: {                                     // ← meta 走 <channel> 标签属性
    │         path: env.payload.path,
    │         lines: `${start.line}-${end.line}`,
    │         // meta 标记（informational only）：是否有用户消息。供 Claude 路由/优先级使用
    │         // ——本投递策略对有/无 message 一视同仁（见 §5.1 决策）
    │         with_message: p.message ? 'true' : 'false',
    │       },
    │     },
    │   })
    └── 无 message 也照样发（统一送达 LLM；见 §5.1 决策）
```

#### 4.4.1 段 A 复制清单

直接抄的"协议层"代码（与 D11/D17 一致）：

| 函数 | 来源 | 大致行数 |
|------|------|---------|
| `normalizeNames` | aibp-pi/index.ts:32-49 | 18 |
| `loadNamePool` | aibp-pi/index.ts:55-83 | 28 |
| `registryDir` | aibp-pi/index.ts:190-196 | 7 |
| `allocateName` | aibp-pi/index.ts:90-186 | 95 |
| `formatText` | aibp-pi/index.ts:241-256（D10 决策 4 版） | 16 |

**总 ~165 行复制**——和 aibp-opencode 抄的一样（opencode 在 §「⟨复制 aibp-pi⟩」 块下复制 ~165 行）。

> 为什么不抽成共享 npm 包？与 opencode 同答：**协议层代码很少、抽包本身要发包+维护，反而麻烦**。先把代码搬一遍；等真出现 ≥3 个 agent 再抽（参见 `说明-架构设计 §三`）。

#### 4.4.2 段 B：MCP server 关键片段

> 来自 channels-reference §"Example: build a webhook receiver"，最小改动适配。

```typescript
import { Server } from '@modelcontextprotocol/sdk/server/index.js'
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js'

const mcp = new Server(
  { name: 'aibp-channel', version: pkg.version },
  {
    capabilities: {
      // ⭐ channel 必需的能力——注册 notification listener；与可选的 tools 并存即变双向
      experimental: { 'claude/channel': {} },
      // v1：不开 tools（单向：microNeo → Claude 已满足需求）
      // 不开 channel/permission（用不到权限中继——那是聊天桥场景）
    },
    // ⭐ 写好 instructions；Claude 在 system prompt 里会看到这些
    instructions:
      'Events from the AIBP channel arrive as <channel source="aibp" path="..." lines="...">. ' +
      'They contain a code selection from the microNeo editor (inline as <selection>...</selection>) ' +
      'optionally followed by a user message (wrapped in <user-input>...</user-input>). ' +
      'Act on them directly when the user message asks you to do something; treat selection-only ' +
      'events as background context for your next user turn.',
  },
)

await mcp.connect(new StdioServerTransport())
```

**设计要点**：
- `name: 'aibp-channel'` 对应 `.mcp.json` 里的 server 名（也对应 `<channel source="aibp-channel">`——其实 source 是 server name，但下面 §5.3 我会建议 source 用 `"aibp"` 更稳定）
- `instructions` 写三件事：① 内容长啥样 ② 有 message / 无 message 的解读差异 ③ 是否需要 reply（不需要——所以不暴露 reply tool）

#### 4.4.3 段 C：Unix socket 与 connection handler

几乎逐行复制 aibp-pi：

```typescript
const connectionHandler = (conn: net.Socket) => {
  let buf = ''
  conn.on('data', chunk => {
    buf += chunk.toString('utf8')
    let nl
    while ((nl = buf.indexOf('\n')) >= 0) {
      handleLine(buf.slice(0, nl))   // ← 注意不带 ctx（MCP 没有 ctx 概念）
      buf = buf.slice(nl + 1)
    }
  })
}

await allocateName(names, connectionHandler)  // listen 抢锁 → server 闭包持有
// 写 registry 文件
fs.writeFileSync(regFile, JSON.stringify({
  name,
  agent: 'claude',      // ⭐ 与 pi/opencode 区别；方便 aibp 别处按 agent 过滤
  pid: process.pid,
  transport: 'unix',
  socket: socketPath,
  protocol: PROTOCOL,
  startedAt: Math.floor(Date.now()/1000),
  cwd: process.cwd(),
  labels: ['default'],
}))
```

**注册文件 schema 与现有 §"RegFile"` 完全一致**（`internal/aibp/registry.go`），只多一个 `agent` 字段。目前发送端不读这个字段，但**为未来扩展预留**（例如「只往 pi 发」过滤）。

#### 4.4.4 段 D：投递——`mcp.notification`

```typescript
function handleLine(line: string) {
  let env: any
  try { env = JSON.parse(line) } catch { return }
  if (env.v !== PROTOCOL_MAJOR || env.type !== 'context') return

  void onMessage(env)   // fire-and-forget；notification 失败只 toast，不阻塞 socket
}

async function onMessage(env: any) {
  const p = env.payload
  const text = formatText(p)            // 共用 §A 的 formatText

  try {
    await mcp.notification({
      method: 'notifications/claude/channel',
      params: {
        content: text,
        meta: {
          // 给 Claude 的 <channel> 标签做路由属性（Claude 可以按 path/with_message 过滤）
          path: p.path,
          ...(p.selection && {
            lines: `${p.selection.start.line}-${p.selection.end.line}`,
          }),
          // 标识是否有用户消息——"with_message=true" 的 channel 优先级应该更高
          with_message: p.message ? 'true' : 'false',
        },
      },
    })
  } catch (e) {
    // notification 失败只 warn，不抛——socket 是 fire-and-forget
    console.warn(`[aibp-claude] notification failed: ${(e as Error).message}`)
  }
}
```

**`meta` key 约束（channels-reference §Notification format）**：
> Keys must be identifiers: letters, digits, and underscores only. Keys containing hyphens or other characters are silently dropped.

→ 所以 `with_message`（带下划线）✅，`lines` ✅，`path` ✅。

### 4.5 `README.md`

参照 aibp-opencode 的 README 结构：

1. **安装**——`claude plugin install /path/to/microNeo/.../claude`（开发期），后续上 npm
2. **首次启用**——`claude --dangerously-load-development-channels plugin:aibp-claude`（research preview flag）
3. **⚠️ 注意事项**——开发期 flag；onboarding 的 trust dialog；`<channel>` 内的内容写法
4. **卸载**——`claude plugin uninstall aibp-claude`（Claude CLI 自带，不用像 opencode 那样手动 jqx）
5. **验证**——`ls $XDG_RUNTIME_DIR/aibp-$(id -u)/` 应见 `ai-<名字>.json` + `.sock`

---

## 五、行为细节 & 决策

### 5.1 设计决策：统一送达 LLM（本阶段不做 input-box 分支）

**决策**：aibp-claude 不区分 `p.message` 有无，所有报文都通过 `mcp.notification(channel)` 送达 LLM。

**为什么**：

- Claude Code 的 channels 架构本身不支持"放输入框但不触发对话"——channels 是 "events for Claude to react to" 的机制（channels-reference），所有推送都会被 Claude 看见并处理
- 真要"填输入框"语义，应该设计独立的途径（如独立 MCP tool / slash 命令），不属于 channel 的职责
- 本阶段不考虑引入这个区分

### 5.2 source 字段：`"aibp-channel"` vs `"aibp"`

`mcp.notification({...})` 发出去的 event 会被 Claude 包成：
```
<channel source="aibp-channel" path="..." lines="12-14">...</channel>
```

`source` 默认是 server 的 `name`（即 `.mcp.json` 里的 key）。**有两个选择**：

| 选 | 优 | 劣 |
|----|----|----|
| `source = "aibp-channel"` | 与 server key 自然绑定，无需额外配置 | plugin 改名会让旧 session 里已有引用（`<channel source="aibp-channel">`）看起来奇怪；不如直接固定一个对人类友好的名字 |
| `source = "aibp"`（固定） | 一个名字，看 `<channel source="aibp">` 立刻知道是 microNeo 来的 | 需要在 capabilities 里 `name:` 字段填 `aibp`（不直白） |

**推荐：固定 `"aibp"`**——`source` 与具体 plugin/agent 名解耦（plugin 改名、`aibp-channel` 改名、`aibp-claude` 改名都不会让旧 session 里的 channel 引用失效），从 microNeo 角度叫它"aibp"也最自然。实现方法：`new Server({name: 'aibp', version: pkg.version}, ...)`，让 server 本身就是 `aibp`。

### 5.3 子进程生命周期 ≠ MCP server 生命周期

**关键洞察**：Channel server 是 Claude spawn 出来的。

| 时机 | 谁的事件 |
|------|---------|
| 用户 `claude plugin enable aibp-claude` | Claude 把 `aibp-channel.ts` 起为子进程 |
| 子进程 `mcp.connect(StdioServerTransport)` | 与 Claude 建 MCP 会话 |
| Claude Code 退出 | 关 stdin → 子进程 SIGPIPE → 子进程清理 → close socket + unlink registry |
| 子进程崩溃 | Claude 检测到 MCP 断开，自动标 channel 失效 |

**清理逻辑复制 aibp-pi 的 `session_shutdown` handler**，但触发器换成 `process.on('beforeExit')` / stdio close 事件。

### 5.4 诊断日志（与 opencode 同款）

```typescript
const DEBUG = false  // ⚠️ 发布前必改 false（用户在 /tmp 有选区明文）
const LOG_FILE = '/tmp/aibp-claude.log'
```

注意 README 的 ⚠️ 注意事项 1 与 aibp-opencode 一字不差——`/tmp/aibp-opencode.log` 改名即可。

---

## 六、与发送端约定的兼容性

发送端（microNeo 内 `internal/aibp`）是 **agent-agnostic**——只发 socket 行、不关心接收端是 pi/opencode/claude。`Discover()` 扫所有 `ai-*.json` → 并发往每个 socket 投消息。✓ 完全不变。

`Discover()` 在 `internal/aibp/registry.go:25-50` 已经做了 connect 试探 + 主版本校验 + PID 存活 + 僵尸 GC——aibp-claude 注册文件复用 `RegFile` schema 就够。✓ 零改 Go 端。

`formatText`（D10 决策 4）的尖括号标签 `<selection>` / `<user-input>` 是接收端自己跑（aibp-pi 第 241 行起）——aibp-claude 的 `formatText` **逐字复制** aibp-pi 已验证的实现。

---

## 七、`AgentEnsurer` 集成（ensure_claude.go 设计）

> **本节是给 microNeo 后续 `:check-agent` 扩展用的设计笔记**。不是本次 aibp-claude 实现的必需部分——但既然其它两个 agent 都接了，aibp-claude 也必须有，否则 `:check-agent` 漏掉 Claude。
>
> **是否本次一并交付**：按用户偏好。先做 plugin 实现；ensurer 单独 PR 跟踪。

### 7.1 文件：`internal/aibp/ensure_agents/ensure_claude.go`

参照 `ensure_opencode.go`，五个方法：

| 方法 | 实现 |
|------|------|
| `AgentName()` | `"claude"` |
| `HasAgent()` | `exec.LookPath("claude")` |
| `HasAIBP()` | 读 `~/.claude/plugins/installed.json` 或 `~/.claude/settings.json` 的 `enabledPlugins` 字段，匹配 `"aibp-claude"` |
| `AIBPVersion()` | 读 `<pluginCacheDir>/aibp-claude/<sha>/package.json` 的 `aibp.protocol`（marketplace 安装版） |
| `InstallAIBP()` | ① 先确认 channels 启用政策（org policy / enable flag）；② `claude plugin install aibp-claude` |
| `UpdateAIBP()` | 与 Install 同路径（Claude 无原生 update） |

**路径解析细节**（与 opencode 的"不能用 `os.UserCacheDir()`"同款）：
- 插件 cache：`$XDG_CACHE_HOME/claude/plugins/...`（macOS: `~/Library/Caches`，需走 xdg-basedir 镜像）
- 插件 manifest：`~/.claude/plugins/installed.json` / `~/.claude/settings.json` → `enabledPlugins`
- 频道 enable 状态：`~/.claude/settings.json` → `channelsEnabled`（org-level）/ `enabledChannels`

> ⚠️ **本节是预研级**，具体 schema 以 Claude Code 实际 release 时为准（v2.1.80+ 起 channels 才稳定）。实际实现前**先看一份本机 Claude 实例的 settings.json 结构**，按真实字段写。Ensurator 与 plugin 实现并行无依赖。

### 7.2 `ensure.go` 追加

```go
var allEnsurers = []AgentEnsurer{
    PiEnsurer{},
    OpencodeEnsurer{},
    ClaudeEnsurer{},   // ⭐ +1 行
}
```

`command_neo.go` / 后续 CLI 的 `EnsureAll` 自动遍历——零改命令侧。

---

## 八、实施计划（Phase 0 → 3）

| Phase | 交付物 | Build gate |
|-------|--------|-----------|
| **Phase 0** | 前置基线：`make build` 通过；当前 AIBP 链路正常；Bun ≥ 1.0 本机可用；Claude Code ≥ 2.1.80（看 `claude --version`） | 全部 ✅ |
| **Phase 1** | `aibp-claude/` 完整实现：`plugin.json` + `.mcp.json` + `aibp-channel.ts` + `package.json` + `README.md` | `claude --plugin-dir ./internal/aibp/aibp-agents/claude --dangerously-load-development-channels plugin:aibp-claude` 启动不报错；左下角 footer（或 `/mcp`）显示 `aibp-channel` 连接成功；注册文件写出 |
| **Phase 2** | 端到端：microNeo Alt-Enter → Claude Code session 看见 `<channel source="aibp">` → Claude 行动 | 选区+消息能正确递交；连接断开不影响 microNeo 其它 receiver（多 receiver 并存） |
| **Phase 3** | `ensure_claude.go`（Go 端自举，参考 `ensure_opencode.go` 的 9 个测试用例） | `go test ./internal/aibp/ensure_agents` 全绿；`--check-agent` 走到 Claude |

**发布策略**：v1 范围内**只做开发调试**，不涉及正式发布。Phase 2 后只验证 Phase 2 build gate（端到端），不进入对外分发流程。详细分发策略作为"未来参考"留档（§8.2），但 v1 不执行。

### 8.1 开发期加载方式（v1 唯一交付目标）

```bash
# Claude Code v2.1.80+ 用 --dangerously-load-development-channels 加载未在 allowlist 的 plugin
claude --plugin-dir ./internal/aibp/aibp-agents/claude \
       --dangerously-load-development-channels plugin:aibp-claude
```

**优势**：
- ✅ 改源码立即生效（无需 `npm publish` + 重装）
- ✅ 不需要 marketplace 注册
- ✅ 不需要 Anthropic allowlist
- ✅ 与 aibp-pi / aibp-opencode 开发期惯例一致

**要求**：
- Bun ≥ 1.0（已验证本机 1.3.6）
- Claude Code ≥ 2.1.80（已验证本机 2.1.132）
- DEBUG 默认 `true`（开发期需要看 `/tmp/aibp-claude.log`；发布前才改 `false`——本节范围不涉及）

### 8.2 未来发布参考（v1 不执行）

> 📌 **本节仅为未来参考，不在 v1 范围**。当 v1 稳定后想公开发布时再启动。
> 核心原则：**不依赖 Anthropic 官方 marketplace 即可分发**——走 GitHub / npmjs.com 的"自助式分发"完全够用。Anthropic 官方 allowlist 是**可选锦上添花**，不是必需。

```bash
# 1. DEBUG = false（README 也加一遍 ⚠️ 提示）

# 2. npm publish（用于 npm source marketplace，与 aibp-pi / aibp-opencode 工作流对齐）
#    路径：internal/aibp/aibp-agents/claude/ → npm publish

# 3. 渠道分发（按优先级，可全做可只做 1-2）：
#    a. GitHub marketplace（默认走这条，零审批）
#       - microNeo 主仓库的 .claude/marketplace.json 注册 aibp-claude 条目
#       - source 指向 GitHub raw 或 git URL（github source）
#       - 用户一行装：claude plugin install aibp-claude@microNeo-marketplace
#    b. npmjs.com publish + npm source marketplace（兼容 aibp 系列工作流）
#       - npm publish 后在自家 marketplace.json 用 "source": "npm" 引用 aibp-claude 包
#    c. 内网/自家 marketplace（GitLab / Gitea + url source）
#       - 内网部署场景，与公网完全隔离
#    d. （可选）申请 Anthropic 官方 allowlist——非必需，仅追求最大公网曝光时考虑
```

**为什么这样设计**：分发自助化意味着 **Anthropic 政策变化 / allowlist 申请被拒都不影响 microNeo 用户正常使用 aibp-claude**。三条主路径（a/b/c）都不需要任何审批。

---

## 九、测试矩阵

按 aibp-pi/opencode 的 Tn-惯例补全（重点是新增的、Cloude 特有的）：

| # | 场景 | 期望 |
|---|------|------|
| T1 | Claude 未装 | Claude plugin exit early、toast "claude not found" |
| T2 | Claude 已装但 channels flag 没加 | 启动 Claude 报 "channel not registered"，不加载 aibp-channel |
| T3 | Claude 已装 + flag 在 + 插件未装 | 装 source 版 → `claude plugin enable aibp-claude` → 重启 Claude → 左下角 / `/mcp` 应显示 `aibp-channel ● connected` |
| T4 | 全新环境触发 `:check-agent` | 安装 + 注册名 + 写 registry 文件 |
| T5 | 有选区 + 有 message | `<channel source="aibp" path="..." lines="A-B" with_message="true">` + 内联 `<selection>` + `<user-input>` |
| T6 | 有选区 + 无 message | Claude 收到带 selection 的 channel（无 message 也照送达）；`with_message="false"`（meta 标记，不影响投递行为） |
| T7 | 无选区 + 有 message | `@path :lineX` 引用 + message |
| T8 | pi / opencode / claude 三 receiver 并存 | microNeo 端发给三个，AllEnsurers 各发各；Claude 名字是 `Charlie` 之类 |
| T9 | connection 断开重连 | 子进程不退出；microNeo 重连 retry；registry 文件不删 |
| T10 | 版本不匹配（microNeo `aibp-2.0` vs 扩展 `aibp-1.x`） | 静默丢弃（与 pi/opencode 同）；Discover 时被主版本校验过滤掉 |
| T11 | 名字池耗尽（15 个名字被占满） | `toast` 警告，不注册 |
| T12 | DEBUG log 开关 | `true` 时 `/tmp/aibp-claude.log` 有 `[<名字>]` 标签；`false` 时静默 |
| T13 | CLI `/mcp` 状态 | 显示 `aibp-channel` server、状态、capabilities |
| T14 | Claude 重启（kill -9 恢复） | mcp 子进程退出 → registry 文件清理 → 重启后再次自举 |
| T15 | org policy 拦截 channels | 启动时报 "blocked by org policy"，aibp-channel 加载失败；aibp-claude plugin 不报错 |

---

## 十、风险 & 已知限制

| # | 风险 / 限制 | 应对 |
|---|------------|------|
| R1 | **Channels 是 research preview**：生产期需 `--dangerously-load-development-channels` flag；Anthropic 官方 marketplace 走 allowlist 机制 | 文档明示；**v1 只做开发调试，用 `--plugin-dir` 加载（§8.1），不涉及发布**；未来发布时优先走 GitHub / npm 自助分发（§8.2 a/b/c），不依赖官方 allowlist；企业用户用 `allowedChannelPlugins` 私有 allowlist；只有追求 Anthropic 官方 marketplace 曝光时才联系 partner |
| R2 | **org policy 可能禁 channels**（`channelsEnabled=false`） | README 给出 IT admin 操作指引；T15 测 |
| R3 | **没有"只填输入框"语义**（§5.1） | README 标注差异化；不算 bug |
| R4 | **meta key 强约束**（letters/digits/underscores only） | 不能传 `lines: "12-14"` 给 meta（带连字符）——我已改为单 key 不带连字符的 `lines_a` 与 `lines_b`（或干脆不传 meta、用 content 表达） |
| R5 | **`<selection>` / `<user-input>` 是纯文本片段而非结构化标签**（是否被 Claude 正确理解为语义分隔符，取决于 prompt 工程实际效果；可能需要根据反馈微调表述） | 靠 tunning + 测试矩阵 |
| R6 | **依赖 Bun 运行时**（Bun 直接跑 TypeScript、零编译；只需本机装 Bun ≥ 1.0） | README 明示安装需求；如需兼容 Node，可把 `.ts` 编译为 `.js` |
| R7 | **`name: 'aibp'` 与 `.mcp.json` key 名可能冲突** | `.mcp.json` 的 key 也叫 `aibp-channel`（不要叫 `aibp`）——避免混淆但 source 还是取 server.name 字段 |
| R8 | **v1 单向**：不暴露 `tools`，Claude 无法主动调 reply tool（**注：Channel 机制本身支持双向**——`claude/channel` capability 与 `tools` 可并存，v1 是设计选择非机制限制；见 channels-reference.md §Expose a reply tool） | v1 满足需求 |

---

## 十一、和历史 D 系列的关系

| 历史 / 关联文档 | 关系 |
|----------------|------|
| D11（名字池） | 直接沿用；aibp-claude 的 `allocateName` 函数逐字复制 |
| D17（aibp-pi 协议） | 同左——aibp-claude 的协议层、报文格式、formatText 全部沿用 |
| D19（aibp-opencode 设计） | 源码结构、npm 发版、README 套路、cache 处理 直接借鉴 |
| D22（aibp-opencode 初始化） | ensure_claude.go 的设计原型 |
| D25（v1.1 更新） | 不直接相关（那是 npm 包本身 v1.0 → v1.1 升级流程） |
| D26（`:check-agent` 迁 CLI） | **受益**——确保 `Ensure(ClaudeEnsurer{}, reporter)` 在 CLI 模式下自动遍历 |

## 十二、下一步可执行清单

按用户许可后实施 Phase 1：

1. ✍️ 写 `aibp-claude/.claude-plugin/plugin.json`（§4.1）
2. ✍️ 写 `aibp-claude/.mcp.json`（§4.2）
3. ✍️ 写 `aibp-claude/aibp-channel.ts`（§4.4，分四段 165 + 60 + 50 + 30 ≈ 305 行）
4. ✍️ 写 `aibp-claude/package.json`（§4.3）
5. ✍️ 写 `aibp-claude/README.md`（§4.5）
6. 🧪 跑 `claude --plugin-dir ./.../claude --dangerously-load-development-channels plugin:aibp-claude` 验证 T1–T15
7. 🧪 跑 microNeo → Alt-Enter → 选区+消息 → 确认 Claude session 出现 `<channel source="aibp">`
8. 📝 提 PR / 文档更新（如有 `docs/agent-comm/` 新增 D28 则补，否则就把本文件当 D27）

---

**变更日志**

| 日期 | 事件 |
|------|------|
| 2026-07-01 | 初稿：方案设计，待用户批准后实施 Phase 1 |
