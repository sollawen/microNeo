# aibp-claude Handoff · 2026-07-02

> **状态**：方向大转弯，从 MCP channel 方案 → 排查 claude 公开注入机制 → 结论：claude 闭源 TUI 没有"立即触发对话"的非-gated 公开接口，channel 是唯一但被 OAuth gate 卡住。
> **明天继续**：从 4 个方向中选一个继续（§六）。
> **接手必读**：§一（背景）+ §二（aibp 协议定位）+ §三（关键发现）+ §六（明天方向）。

---

## 一、背景

### 1.1 目标

实现 `aibp-claude` 插件（TypeScript + Bun + MCP），把 microNeo 编辑器选中的内容推送到 Claude Code 正在运行的会话。

### 1.2 重要性

`aibp-pi`（TS 扩展，~95 行）已实现，`aibp-opencode` 已实现。`aibp-claude` 是 aibp 协议的第三个接收端。

### 1.3 v1 范围

dev/debug only（`--plugin-dir` 加载），不发布 npm，不注册 marketplace。

### 1.4 关键约束

- **ccmm** = zsh 函数，用 MiniMax 中转 API（`ANTHROPIC_BASE_URL=https://api.minimaxi.com/anthropic` + `ANTHROPIC_AUTH_TOKEN`）绕开 Anthropic 官方登录墙
- `ccaibp`（ccaibp 函数）在 ~/.zshrc）= ccmm + `--plugin-dir .../claude "$@"`，目前还加了我们测试用的 `--dangerously-load-development-channels plugin:aibp-claude` flag
- DEBUG = true（v1 不改）
- 代码必须从 aibp-pi 逐字复制（协议层），用 `dependencies`（不是 `peerDependencies`）装 MCP SDK

---

## 二、aibp 协议定位（这次读懂的关键）

### 2.1 协议层（LSP 式）

`说明-AIBP.md` + `说明-架构设计.md`（docs/agent-comm/）是权威。核心三条铁律：

1. **agent 无关**：microNeo 不感知任何 agent 内部
2. **UI 无关**：协议只规定"拿到 (消息, 上下文) 后怎么处理"
3. **能力差异由接收端吸收**：协议规定"递送什么"，不规定"agent 怎么用"

### 2.2 接收端职责（关键）

> 接收端 = agent 内的扩展。**启动注册 → 监听 socket → 解析报文 → 递送到自己的 LLM 通道**（§六 协议层）

具体实现因 agent 而异：
- **pi**：`pi.sendUserMessage(text, {deliverAs: "steer"})`（开源 TUI，扩展能直接调内部 API）
- **opencode**：TUI 插件，`session.prompt()`（开源 TUI）
- **claude**：❓ 这正是 aibp-claude 要解决的核心问题

### 2.3 两条递送路径（§6.1）

| 报文形态 | 语义 | 接收端职责 |
|----------|------|-----------|
| **带 message** | 用户明确发起协作 | 格式化为一条用户消息，**递送给 LLM 并触发对话回合**（立即响应） |
| **纯上下文** | 把编辑器状态"挂"到 agent | 不得自动触发对话；待用户下次发消息时带上 |

### 2.4 aibp 场景本质

> claude 是用户正在长期聊天的运行中 TUI 进程。microNeo 偶尔往里塞一条消息。**这和 opencode/pi 的"注入 TUI 会话"完全对称**。不是"新建一个 headless 进程"（我之前推的 stream-json / -p 完全违背 aibp 场景，已被用户纠正）。

---

## 三、claude 公开注入机制排查结论（最重要的发现）

### 3.1 排查范围

`channels-reference.md`（45KB）+ `claude-mcp-doc.md`（28KB）+ `claude-plugin-doc.md`（79KB）三份本地公开文档，外加 `gh` 查 anthropics 组织仓库（claude-code / claude-agent-sdk-*/ claude-plugins-official）+ `claude --help` + 实测（lsof IPC / stream-json 响应 / ccaibp --debug 日志）。

### 3.2 claude 公开"程序→运行中会话"机制全集

| 机制 | 触发方式 | 注入能力 | aibp 路径 | ccmm 可用 |
|------|----------|----------|-----------|-----------|
| **MCP Channel** | 外部 mcp.notification | 立即注入 `<channel>` 标签 | §6.3 带消息 | ❌ gated |
| **Hooks + additionalContext** | claude 事件触发 | 附加到上下文，**不触发响应** | §6.4 纯上下文 | ✅ 公开 |
| MCP tools | LLM 主动调用 | 反向（LLM→外部） | — | — |
| Agent SDK / `claude -p` | 程序化 query | headless 新建会话 | — | — |
| MCP sampling / elicitation | server→client | 借 LLM / 请求用户 | — | — |
| **TUI 主进程 IPC** | — | — | — | ❌ **lsof 全空，claude TUI 不监听任何 IPC** |

### 3.3 关键证据链

**channel gated 的决定性证据**（ccaibp --debug 日志 `~/.claude/debug/5f953976-...txt` 第 16/69/84/149 行）：

```
[DEBUG] ANTHROPIC_BASE_URL=https://api.minimaxi.com/anthropic is not a first-party Anthropic host
[DEBUG] [Bootstrap] Skipped: no usable OAuth, WIF, or API key
[DEBUG] [claudeai-mcp] Disabled: API-key auth precedence active
[DEBUG] MCP server "plugin:aibp-claude:aibp-channel": Channel notifications skipped: channels feature is not currently available
```

**aibp-channel 代码侧 100% 正确**（capability 声明被识别、plugin 加载成功、notification 发送 resolved OK），唯一阻塞是环境 gate。

### 3.4 hooks 的真实能力边界

`additionalContext` 的语义 = **附加到上下文，不触发立即响应**。证据：
- channels-reference.md 第 145 行 SessionStart hook 输出 `hookSpecificOutput.additionalContext`（superpowers plugin 的 huge additionalContext 就是这个模式，session 启动时注入，不触发对话）
- `FileChanged` hook 虽**外部可主动触发**（写文件就行），但 command 类型输出**无法让 claude 立即响应**——它只是事件通知
- `UserPromptSubmit` hook 能在**用户提交 prompt 时**追加 additionalContext → 对应 aibp §6.4 纯上下文待用路径

### 3.5 claude vs opencode/pi 的根本架构差异

| 维度 | opencode / pi | claude |
|------|---------------|--------|
| TUI 源码 | **开源**，插件同进程 | **闭源**，TUI 黑盒 |
| 扩展能拿 session 对象 | ✅ 能（直接调 `session.prompt()` / `pi.sendUserMessage()`） | ❌ 不能 |
| 外部→会话 接口 | 进程内直接调 | 必须走 channel（gated）或 hooks（仅上下文） |
| 立即触发对话 | ✅ 公开 | ❌ 没有非-gated 公开接口 |

**这是 claude 闭源 TUI 的根本架构限制，不是我们没找到更好的接口。**

---

## 四、已尝试方案

### 4.1 MCP Channel 方案（**已完成代码，gated 失败**）

**做了什么**：
- aibp-channel.ts 374 行（typeScript + Bun + MCP SDK）
- .claude-plugin/plugin.json：`{ name: "aibp-claude", channels: [{ server: "aibp-channel" }] }`
- .mcp.json：`{ mcpServers: { aibp-channel: { command: "bun", args: [aibp-channel.ts] } } }`
- package.json：`aibp.protocol = "aibp-2.0"`
- 12 个文件日志点（写到 `/tmp/aibp-claude.log`，stdout 留给 JSON-RPC）
- 29 单元测试 + 8 集成测试全过
- 真机验证：aibp-channel 启动 + MCP 握手 + capability 识别 + notification 发送全部 OK

**失败原因**：ccmm 下 `channels feature is not currently available`（OAuth + 第一方 host gate）

### 4.2 --dangerously-load-development-channels flag 探索（**已排除**）

测试过的 ref 格式：
- `aibp-channel`、`server:aibp-channel`、`plugin:aibp-claude`、`plugin:aibp-claude@local`、`plugin:aibp-claude@aibp-claude`
- 全部走到 "not logged in"（dummy token）或不报错
- 实际是 flag 本身没激活 channel 功能（gate 在更深层）

ccaibp 当前用的是：`--dangerously-load-development-channels plugin:aibp-claude`（无 marketplace 段，--plugin-dir 无 market）

### 4.3 stream-json / -p / Agent SDK（**方向错误，已否定**）

之前推过这三个"程序发消息"方案，用户指出方向错了：
> claude 是用户长期运行的进程，用户一直在和它聊天。只不过有时候需要从 microNeo 里面给它发个消息而已

这三个都是"新建 headless 会话"或"spawn 新进程"，**违背 aibp 场景**（aibp 是"注入用户已有的运行中会话"）。

实测过的：
- `claude -p --input-format stream-json`（ccmm 下实测成功，3.6s 返回 pong）—— 可用但不是 aibp 场景
- `claude -p --resume <id>`（msg1 记 42，msg2 答 42，1.4-4.4s 冷启动）—— 同上，方向错

---

## 五、当前文件状态

### 5.1 已修改文件

| 路径 | 状态 |
|------|------|
| `/Users/sollawen/.zshrc` | 加了 ccaibp 函数（ccmm + `--dangerously-load-development-channels plugin:aibp-claude --plugin-dir <dir> "$@"`） |
| `/Users/sollawen/pi-dev/microNeo.master/internal/aibp/aibp-agents/claude/.claude-plugin/plugin.json` | `name: "aibp-claude"` + `channels: [{server: "aibp-channel"}]`（**注意**：CHANGELOG 显示 `--channels` 是正确 flag，但我们之前没用它，可能无关） |
| `/Users/sollawen/pi-dev/microNeo.master/internal/aibp/aibp-agents/claude/aibp-channel.ts` | 374 行，12 log 点，capability 正确，代码 100% 正确 |
| `/Users/sollawen/pi-dev/microNeo.master/internal/aibp/aibp-agents/claude/test/protocol.test.ts` | 29 单元测试 |
| `/Users/sollawen/pi-dev/microNeo.master/internal/aibp/aibp-agents/claude/test/integration.test.ts` | 8 集成测试 |
| `/Users/sollawen/pi-dev/microNeo.master/internal/aibp/aibp-agents/claude/package.json` | `aibp.protocol = "aibp-2.0"` |
| `/Users/sollawen/pi-dev/microNeo.master/internal/aibp/aibp-agents/claude/.gitignore` | node_modules 排除 |
| `docs/agent-comm/aibp-跨agent设计问题清单.md` | 加了 3 个 issue（B1 EPERM, I1 opencode log spam, B2 macOS Unix socket） |
| `aibp-claude-方案.md` | D27 方案文档，3 处已纠正 |
| `aibp-claude-实施-测试方案.md` | 实施 + 测试方案，10 章 + §10 附录 |

### 5.2 新增文件（诊断产物，待清理或归档）

| 路径 | 用途 | 建议 |
|------|------|------|
| `/tmp/aibp-claude.log` | aibp-channel 12 点日志 | **保留**（明天继续用） |
| `/tmp/aibp-diag.mjs` | 早期 MCP raw 诊断 | 可删 |
| `/tmp/aibp-raw.mjs` | 早期原始 JSON-RPC 诊断 | 可删 |
| `/tmp/aibp-flagtest.txt` | flag ref 格式测试输出 | 可删 |
| `/tmp/sj-test.txt` | stream-json 实测输出 | 可删 |
| `/tmp/claude-changelog.md` | 下载的 CHANGELOG（用于查 channel 演进） | 可删 |

### 5.3 当前 aibp 进程状态（`/var/folders/66/mvkpj8014gbgj0vwzy2gn/T/aibp-501/`）

- `ai-Alpha.json` → pi（pid 9525）
- `ai-Bravo.json` → claude（pid 28638 之后的某次，ccaibp 重启后会变）

---

## 六、明天方向（4 选项）

### 6.1 选项 A：改用 UserPromptSubmit hook 方案 ⭐ 推荐

**做什么**：
- aibp-claude 不再做 MCP channel server，改为 **常驻 daemon + claude plugin 配 UserPromptSubmit hook**
- aibp-claude daemon 监听 aibp socket（aibp 协议不变）
- 收到 microNeo 消息 → 存 pending（写到约定文件，如 `~/.claude/aibp-pending.json`）
- claude plugin 在 `plugin.json` 的 hooks 配置里注册 `UserPromptSubmit` hook
- 用户在 claude 提交 prompt 时，hook 读 pending → 追加为 additionalContext
- claude 看到 [用户消息 + 来自 microNeo 的上下文] 一起处理

**语义变化**（关键）：
- ❌ 不再是"microNeo 一发，claude 立即开始处理"（这是 channel 路径）
- ✅ 变成"microNeo 一发，claude 在用户下次提交时把上下文带上一起处理"（aibp §6.4 纯上下文路径）

**工作量**：小（aibp-channel.ts 大部分代码可复用为 daemon，plugin 加 hook 配置 + 一个新 TS 脚本）
**体验**：microNeo 发完后**用户要在 claude 里提交一下**才生效
**优势**：公开、ccmm 可用、不需要 OAuth gate、不需要 channel 文档那些复杂配置

### 6.2 选项 B：死磕 channel（等 OAuth 环境）

- 现状不动，等官方登录
- 真正"立即触发"，但要官方账号
- 0 工作量

### 6.3 选项 C：双轨

- aibp-claude 同时支持 hooks + channel，按环境自动选
- 中等工作量，复杂度↑
- 通用但维护成本高

### 6.4 选项 D：重新讨论 aibp 对 claude 的递送语义

- 接受"纯上下文待用"是 claude 闭源架构的天然模型
- 概念层面调整：claude 走 aibp §6.4 路径，不走 §6.3

### 6.5 我的建议

**选项 A**。理由：
1. 公开、ccmm 立即可用
2. 代码量小（复用大部分现有代码）
3. 与 aibp 协议对齐（§6.4 纯上下文）
4. 不依赖未公开/未稳定的 claude 实验功能
5. claude 闭源架构下的务实最优

如果用户特别看重"立即触发"体验，再考虑 B（等官方）或 C（双轨）。

---

## 七、复现命令（明天恢复诊断状态）

### 7.1 启动 ccaibp 看 channel gate 日志

```bash
source ~/.zshrc
ccaibp --debug
```

启动后看 `~/.claude/debug/<session>.txt`，grep：
- `channel`（看 channel skip 原因）
- `first-party` / `API-key` / `OAuth`（看 gate 原因）

### 7.2 测 channel MCP server 加载是否正常

```bash
tail -20 /tmp/aibp-claude.log
```

期望看到完整的诊断链：module loaded → listening → connection accepted → envelope parsed → mcp.notification resolved (OK)

### 7.3 测 stream-json 模式（仅供对照参考）

```bash
# 从 ~/.zshrc 提取 env
TOKEN=$(grep "ANTHROPIC_AUTH_TOKEN=" ~/.zshrc | head -1 | sed 's/.*=["'"'"']//;s/["'"'"'].*//')
BASE=$(grep "ANTHROPIC_BASE_URL=" ~/.zshrc | head -1 | sed 's/.*=["'"'"']//;s/["'"'"'].*//')
MODEL=$(grep "ANTHROPIC_MODEL=" ~/.zshrc | head -1 | sed 's/.*=["'"'"']//;s/["'"'"'].*//')

printf '%s\n' '{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Reply with exactly: pong"}]}}' | \
  ANTHROPIC_BASE_URL="$BASE" ANTHROPIC_AUTH_TOKEN="$TOKEN" ANTHROPIC_MODEL="$MODEL" \
  claude -p --input-format stream-json --output-format stream-json --verbose 2>&1 | tail -3
```

期望看到 `{"type":"result",...,"result":"pong"}`。**注意：此方案违背 aibp 场景，仅供确认 ccmm + claude 通信链通**。

### 7.4 看 aibp 注册表

```bash
ls -la /var/folders/66/mvkpj8014gbgj0vwzy2gn/T/aibp-501/  # macOS
# 或
ls -la "${XDG_RUNTIME_DIR:-/tmp}/aibp-$UID/"  # 通用
```

### 7.5 读 aibp 协议权威文档（明天继续前必读）

- `docs/agent-comm/说明-AIBP.md` §6（递送语义）
- `docs/agent-comm/说明-架构设计.md` §四 原则 4（能力差异由接收端吸收）
- `docs/agent-comm/说明-接收端.md`（pi 接收端实现，对称参考）

---

## 八、关键文件清单

### 8.1 必读（aibp 协议层）

- `docs/agent-comm/说明-AIBP.md`（协议权威）
- `docs/agent-comm/说明-架构设计.md`（架构总纲）
- `docs/agent-comm/说明-接收端.md`（pi 接收端实现，参考价值极高）

### 8.2 必读（claude 公开文档）

- `internal/aibp/aibp-agents/claude/channels-reference.md`（45KB，channel 完整机制）
- `internal/aibp/aibp-agents/claude/claude-mcp-doc.md`（28KB，MCP 双向机制）
- `internal/aibp/aibp-agents/claude/claude-plugin-doc.md`（79KB，hooks + plugin 全集）

### 8.3 aibp-claude 项目文件

- `internal/aibp/aibp-agents/claude/aibp-channel.ts`（374 行，MCP channel server，**代码 100% 正确**）
- `internal/aibp/aibp-agents/claude/.claude-plugin/plugin.json`（channels 声明）
- `internal/aibp/aibp-agents/claude/.mcp.json`（MCP server 配置）
- `internal/aibp/aibp-agents/claude/package.json`（aibp.protocol 声明）
- `internal/aibp/aibp-agents/claude/test/*.test.ts`（37 个测试）

### 8.4 方案文档

- `internal/aibp/aibp-agents/claude/aibp-claude-方案.md`（D27 方案）
- `internal/aibp/aibp-agents/claude/aibp-claude-实施-测试方案.md`（实施 + 测试 + §10 知识附录）

---

## 九、已知 bug / 待办

### 9.1 已知 bug

- **SIGPIPE 处理不当**（aibp-channel.ts）：`process.on('SIGPIPE', () => process.exit(0))` 会导致 claude 关闭连接时 claude 子进程被意外终止。应该标记 transport 死 + 静默丢弃。
- **formatText 无 try/catch**（aibp-channel.ts 的 onMessage）：如果 formatText 抛错，整个报文处理崩溃。应包 try/catch。

### 9.2 待办（按选项 A 推进时）

1. 改 aibp-channel.ts 为 daemon 模式（不再做 MCP server）
2. 写 UserPromptSubmit hook 脚本（读 pending → 输出 additionalContext）
3. 更新 `.claude-plugin/plugin.json` 加 hooks 配置
4. 验证流程：microNeo 发消息 → claude 下次提交时看到 microNeo 上下文
5. 更新 `aibp-claude-实施-测试方案.md` 反映新方案
6. 决定是否保留 channel 方案代码（aibp-channel.ts）作为对照

### 9.3 文档待办

- `aibp-claude-实施-测试方案.md` §10 知识附录应加："claude 闭源 TUI 注入机制限制" + "channel gate 排查证据链"
- aibp-跨agent设计问题清单.md 应加："I2: claude 没有'立即触发'的公开接口，aibp-claude 走 §6.4 纯上下文路径"

---

## 十、关键 quote（防止明天又走偏）

### 10.1 用户的两次纠正

**第一次**（路线 C -p 是扯蛋）：
> claude 是个长期运行的进程，用户一直在和它聊天呢。只不过有时候需要从 microNeo 里面给它发个消息而已

**第二次**（指我去读 docs）：
> 你是不是没搞清楚aibp是怎么回事啊？你好好读读docs/里面的文档吧

### 10.2 aibp 协议定位（`说明-AIBP.md` §六）

> **协议只规定"递送什么"，不规定"agent 怎么用"。** 具体用什么机制把消息送进 LLM 通道，是各接收端的实现职责。

> 协议用「递送」而非「注入」——「注入」暗示隐式改写消息流，而协议层只递送，是否/如何进入 LLM 上下文由接收端自决。

### 10.3 claude 闭源架构事实

> claude 是**闭源 TUI**，不暴露内部 session 对象。opencode/pi 能"插件直接调 session.prompt"是因为它们**开源、插件和 TUI 同进程共享对象**。claude 没有这种能力。

> claude TUI 主进程**不监听任何 IPC**（lsof 实测全空，~/.claude/ 无 sock/pipe），外部→TUI 的唯一"立即注入对话"机制是 channel（gated）。

---

## 十一、明天开工第一件事

读 `docs/agent-comm/说明-接收端.md`（pi 接收端实现细节），然后决定走哪个选项。如果选 A，开工写 UserPromptSubmit hook + 改 daemon。

如果用户问"claude 真的没有更好的接口吗"，直接答：**没有，这是闭源架构限制。channel 是唯一立即触发机制，gated；hooks 是唯一公开机制，但只能纯上下文。**
