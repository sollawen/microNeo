# `aibp-claude` 实施 & 测试方案

> 位置：`internal/aibp/aibp-agents/claude/aibp-claude-实施-测试方案.md`
> 上游设计依据：`aibp-claude-方案.md`（设计文档，本文件为落地 checklist）
> 状态：PLAN 模式，未经用户许可不改代码
> 测试栈：**Bun**（`bun:test`，兼容 Jest API）+ Go 标准 `testing`（`ensure_claude.go`）

---

## 0. 背景与目标

### 0.1 为什么需要这份文档

`aibp-claude-方案.md` 已覆盖：
- §一～§三：定位、与 pi/opencode 的差异、目录结构
- §四：5 个关键文件的"规格"（字段级别）
- §五：行为细节（segment 切分、source 选择、生命周期、日志）
- §八：阶段划分（Phase 0 → 3，build gate 级别）
- §九：测试矩阵（T1～T15，**集成/手测场景级别**）

**缺失的部分**：
1. **实施步骤不够细**：原 §八 只到 build gate 级别，每个文件怎么写没列
2. **没有单元测试设计**：T1～T15 都是端到端集成场景，**核心纯函数（normalizeNames / allocateName / formatText 等）** 没单测保护
3. **没有 mock 策略**：Bun 测试如何 mock Claude stdio、文件系统、网络 socket 没规划
4. **没有覆盖率目标**：哪些模块必 100%、哪些 ≥ 80% 没定义

### 0.2 本文档目标

| 目标 | 章节 |
|------|------|
| 把 §八 的"build gate"细化成"代码改动清单" | §3 |
| 设计 aibp-channel.ts 4 段（协议层 / MCP / socket / 投递）的**单元测试** | §4 |
| 设计**集成测试**（mock Claude + 真实 socket） | §5 |
| 把 §九 T1～T15 扩展为 T1～**T30**，覆盖更多边界 | §5.3 |
| 定义**覆盖率目标**和**CI 门禁** | §6.2 |

---

## 1. 改动范围表

| 文件 | 类型 | 测试文件 | 状态 |
|------|------|----------|------|
| `.claude-plugin/plugin.json` | **新建** | — （配置，不测） | §3.1 |
| `.mcp.json` | **新建** | — （配置，不测） | §3.2 |
| `package.json` | **新建** | — （元信息，不测） | §3.3 |
| `aibp-channel.ts` | **新建**（核心） | `aibp-channel.test.ts` | §3.4 + §4 |
| `README.md` | **新建** | — （文档，不测） | §3.5 |
| `internal/aibp/ensure_agents/ensure_claude.go` | **新建**（Go 端自举） | `ensure_claude_test.go` | §3.6 |
| `internal/aibp/ensure_agents/ensure.go` | **不改**（接口稳定） | — | — |

**总计**：6 个新文件 + 2 个测试文件。

---

## 2. 实施阶段（与原 §八 对齐，本节更细）

### Phase 0：环境基线（30 分钟）

| 步骤 | 操作 | 验收 |
|------|------|------|
| 0.1 | `make build` 通过（确认 Go 端 AIBP 链路正常） | 退出码 0，无 panic |
| 0.2 | `claude --version` ≥ 2.1.80 | 输出版本号 |
| 0.3 | `bun --version` ≥ 1.0 | 输出版本号（没有则 `curl -fsSL https://bun.sh/install \| bash`） |
| 0.4 | `which claude` 能找到二进制 | 路径非空 |
| 0.5 | （仅开发期）确认 `--dangerously-load-development-channels` 可用 | `claude --help \| grep dangerously` 有输出 |

### Phase 1：最小可行实现（≈ 305 行 TS + 50 行 README）

| 步骤 | 操作 | 验收 |
|------|------|------|
| 1.1 | 写 `.claude-plugin/plugin.json`（§3.1） | `jq .` 解析通过 |
| 1.2 | 写 `.mcp.json`（§3.2） | 同上 |
| 1.3 | 写 `package.json`（§3.3） | `bun install` 成功（或无需安装——`@modelcontextprotocol/sdk` 是 peerDep） |
| 1.4 | 写 `aibp-channel.ts` 段 A（协议层，~165 行，§3.4.1） | `bun test aibp-channel.test.ts` 全绿（§4.2） |
| 1.5 | 写 `aibp-channel.ts` 段 B（MCP server，~30 行，§3.4.2） | 手测：`bun aibp-channel.ts` 在 MCP 协议握手阶段不崩溃 |
| 1.6 | 写 `aibp-channel.ts` 段 C（Unix socket，~50 行，§3.4.3） | `bun test` socket 单测全绿（§4.4） |
| 1.7 | 写 `aibp-channel.ts` 段 D（投递，~30 行，§3.4.4） | 集成测试 mock Claude 收到正确 notification（§5.1） |
| 1.8 | 写 `README.md`（§3.5） | markdownlint 通过 |
| 1.9 | `claude --plugin-dir ./internal/aibp/aibp-agents/claude --dangerously-load-development-channels plugin:aibp-claude` 启动 | 不报错；左下角 / `/mcp` 显示 `aibp-channel ● connected` |

### Phase 2：端到端集成（手测为主）

| 步骤 | 操作 | 验收 |
|------|------|------|
| 2.1 | microNeo 启动 + Claude Code 启动（独立 terminal） | 两边各就各位 |
| 2.2 | microNeo 打开任一文件，选区，Alt-Enter | Alt-Enter 触发 AIBP 推送（与 pi/opencode 同链路） |
| 2.3 | Claude Code session 出现 `<channel source="aibp">…</channel>` | Claude 立即响应 |
| 2.4 | 关 microNeo，再开；再 Alt-Enter | 重连后仍正常（T9） |
| 2.5 | 同时启动 pi / opencode / claude 三个 receiver | 全部收到推送（T8） |

### Phase 3：Go 端自举（ensure_claude.go）

| 步骤 | 操作 | 验收 |
|------|------|------|
| 3.1 | 写 `ensure_claude.go`（§3.6） | `go build` 通过 |
| 3.2 | 写 `ensure_claude_test.go`（§4.5） | `go test ./internal/aibp/ensure_agents -run TestClaude` 全绿 |
| 3.3 | 在 `ensure.go` 的 `Ensure()` 里加一行 `ClaudeEnsurer{}` 编排 | 不破坏现有 pi/opencode 测试 |
| 3.4 | `:check-agent` 走到 Claude（手测） | infobar 显示 Claude 安装状态 |

---

## 3. 文件级实施清单

### 3.1 `.claude-plugin/plugin.json`

**照搬方案 §4.1**，无新增约束。`jq` 验证 schema：

```bash
jq '.name, .channels[0].server' .claude-plugin/plugin.json
# 期望输出：
#   "aibp-claude"
#   "aibp-channel"
```

### 3.2 `.mcp.json`

**照搬方案 §4.2**，约束：

- `mcpServers.aibp-channel.command` 必须是 `bun`（不写 `node`，避免用户没装 Node）
- `args[0]` 必须用 `${CLAUDE_PLUGIN_ROOT}/aibp-channel.ts`（不写绝对路径——**未来发布时** marketplace 安装路径会变；v1 开发期路径固定）
- `env` 留空 `{}`（v1 不需要透传环境变量）

`jq` 验证：

```bash
jq '.mcpServers["aibp-channel"].command' .mcp.json
# 期望："bun"
```

### 3.3 `package.json`

**照搬方案 §4.3**。**关键字段**：

- `aibp.protocol = "aibp-2.0"`（**字面必须一致**，协议层 §4.2.1 用它做主版本校验）
- `type = "module"`（ESM 必须，跑 Bun）
- `peerDependencies."@modelcontextprotocol/sdk" = ">=1.0.0"`
- `files` 数组必须**包含** `.claude-plugin/plugin.json`、`.mcp.json`、`aibp-channel.ts`、`package.json`、`README.md`（**未来 npm publish 时**漏一个装出来就缺文件；v1 开发期不涉及，但先写对省得发布时改）

**额外约束**（本方案新增）：加 `scripts.test = "bun test"` 方便 `bun run test`。

### 3.4 `aibp-channel.ts` ⭐ 核心实现

#### 3.4.1 段 A：协议层（~165 行）

**照搬** aibp-pi 的下列函数（**逐字复制**，§4.2.1 文档化为"复制清单"）：

| 函数 | 源（aibp-pi/index.ts） | 行数 |
|------|------------------------|------|
| `normalizeNames` | L18-L42 | 25 |
| `loadNamePool` | L52-L92 | 41 |
| `allocateName` | L100-L186 | 87 |
| `registryDir` | L188-L194 | 7 |
| `formatText` | L241-L256 | 16 |

**aibp-claude 特有**：

- `DEFAULT_NAMES_STR` 同 aibp-pi（"Alpha Bravo Charlie … Oscar"）
- `PROTOCOL` 读 `package.json` 的 `aibp.protocol`（不硬编码 "aibp-2.0"——避免版本升级时漏改）
- `PROTOCOL_MAJOR = Number(PROTOCOL.split("-").pop())`

#### 3.4.2 段 B：MCP server（~30 行）

照搬方案 §4.4 段 B 代码块。**关键字段**：

- `name: 'aibp'`（**注意不是 `'aibp-channel'`**——§5.3 决策：source 固定 `"aibp"`，避免 plugin 改名让旧 session 引用失效）
- `capabilities.experimental['claude/channel'] = {}`
- `instructions` 字符串照抄方案 §4.4（**字符级一致**——Claude 的 system prompt 拼接依赖它）

#### 3.4.3 段 C：Unix socket（~50 行）

照搬 aibp-pi 的 connection handler（aibp-pi/index.ts L200-L260）。**关键点**：

- `socketPath = path.join(registryDir(), 'ai-<name>.sock')`（D11 §4.3：name 自带 `-` 会让 `ai-<name>` 视觉歧义——已通过 §4.2.1 `normalizeNames` 步骤 3 过滤）
- 注册文件：`{name, agent: "claude", protocol: "aibp-2.0", socket_path, pid, started_at}`
- 分帧：每行一个 JSON（`\n` 分隔），连接断开清空 buffer
- version check：解析后看 `envelope.v` 是否以 `aibp-<PROTOCOL_MAJOR>.` 开头，否则丢弃并 warn

#### 3.4.4 段 D：投递（~30 行）

```typescript
async function handleLine(envelope: AibpEnvelope, ctx: { mcp: Server }) {
  const text = formatText(envelope.payload)            // 复用段 A
  await ctx.mcp.notification({
    method: 'notifications/claude/channel',
    params: {
      content: text,
      meta: {
        path: envelope.payload.path,
        lines: `${envelope.payload.loc.start.line}-${envelope.payload.loc.end.line}`,
        with_message: envelope.payload.message ? 'true' : 'false',
      },
    },
  })
}
```

**§5.1 决策**（v1 不区分 `p.message` 有无）：有/无 message 一律送达 LLM（统一行为，避免 LLM 学到"无 message 的 channel 可以忽略"）。

### 3.5 `README.md`

**5 段固定结构**（与 aibp-opencode 一致）：

1. **一段话定位**：AIBP receiver for Claude Code via Channel MCP
2. **安装**：`claude plugin install aibp-claude` + 注意事项
3. **开发期加载**：`--dangerously-load-development-channels plugin:aibp-claude`
4. **验证步骤**：开 Claude session + 在 microNeo 选区 + Alt-Enter + 看 `<channel source="aibp">`
5. **卸载**：`claude plugin disable aibp-claude` + 手动清 `~/.config/aibp/aibp-names.json` 的 `Charlie` 等条目

**⚠️ 注意事项 1**（与 aibp-opencode 一字不差）：`/tmp/aibp-claude.log` 改名即可——`aibp-channel.ts` 的 `DEBUG` 常量：**v1 开发期保持 `true`（需要看 `/tmp/aibp-claude.log` 调试）**，**发布前改 `false`**（与 opencode 同款——用户在 `/tmp` 有选区明文）。

### 3.6 `ensure_claude.go`（Go 端自举）

照搬 `ensure_opencode.go` 的 5 个方法（接口契约在 `ensure.go:31`）：

| 方法 | aibp-claude 实现 |
|------|------------------|
| `AgentName()` | `"claude"` |
| `HasAgent()` | `exec.LookPath("claude")` |
| `HasAIBP()` | 读 `~/.claude/plugins/installed.json` 或 `~/.claude/settings.json` 的 `enabledPlugins`，匹配 `"aibp-claude"` |
| `AIBPVersion()` | 读 `<pluginCacheDir>/aibp-claude/<sha>/package.json` 的 `aibp.protocol` |
| `InstallAIBP()` | ① 检查 `channelsEnabled` org policy；② `claude plugin install aibp-claude` |
| `UpdateAIBP()` | 同 `InstallAIBP` 路径（Claude 无原生 update） |

**新增**：本方案建议 `UninstallAIBP()`（与 opencode 对称），但不进接口（与 `ensure_opencode-实施计划.md §2.4` 同款决策）。

---

## 4. 单元测试设计

### 4.1 测试框架与目录

- **运行器**：`bun test`（Bun 内置，零依赖）
- **文件命名**：`*.test.ts`（Bun 默认发现规则）
- **目录**：`aibp-claude/test/`（独立目录，避免污染源码；与 src 通过相对路径 import）

```
aibp-claude/
├── aibp-channel.ts          # 段 A-D 都内联在一个文件
├── package.json
└── test/
    ├── protocol.test.ts     # 段 A 单测
    ├── socket.test.ts       # 段 C 单测
    ├── delivery.test.ts     # 段 D 单测（mock MCP server）
    └── integration.test.ts  # 端到端（mock Claude + 真实 socket）
```

### 4.2 协议层测试（段 A）—— 100% 覆盖目标

**核心函数 × 测试用例矩阵**：

#### 4.2.1 `normalizeNames()`

| # | 输入 | 期望 | 边界类型 |
|---|------|------|---------|
| 1 | `["Alpha", "Bravo", "Charlie"]` | `["Alpha", "Bravo", "Charlie"]` | 正常 |
| 2 | `["VeryLongNameThatExceeds10Chars", "Bravo"]` | `["VeryLongNa", "Bravo"]` | 截断 |
| 3 | `["Alpha", "Alpha", "Bravo"]` | `["Alpha", "Bravo"]` | 去重 |
| 4 | `["A-B", "Bravo", "A B", "A:B", "A/B"]` | `["Bravo"]` | 字符过滤 |
| 5 | `["Alpha", null, 42, undefined]` | `["Alpha"]` | 类型错误容忍 |
| 6 | `["", "Bravo"]` | `["Bravo"]` | 空字符串 |
| 7 | `["Alpha-Bravo"]` | console.warn + `[]` | 全是非法字符 |

#### 4.2.2 `loadNamePool()`

| # | 前置状态 | 期望 | 边界类型 |
|---|---------|------|---------|
| 1 | `aibp-names.json` 不存在 | 写入种子 + 返回 DEFAULT_NAMES | 文件不存在 |
| 2 | `aibp-names.json = []` | 写入种子（不覆盖？**覆盖——见注**）+ 返回 DEFAULT_NAMES | 空数组 |
| 3 | `aibp-names.json = ["Zulu"]` | 返回 `["Zulu"]`（自定义） | 合法非空 |
| 4 | `aibp-names.json = "not json"` | ctx.ui.notify + 返回 `null` | 解析失败 |
| 5 | `aibp-names.json = null`（JSON 字面 null） | 走分支 B | null 值 |
| 6 | `XDG_CONFIG_HOME` 自定义 | 读 `XDG_CONFIG_HOME/aibp/aibp-names.json` | 路径覆盖 |

> **注**：分支 2（"空数组 → 覆盖种子"）与 aibp-pi 行为一致——空数组语义上等同于"想重置"。**保持一致**是减少惊讶的关键。

#### 4.2.3 `allocateName()`

**这是测试重点**——抢占锁 + GC + 池子耗尽三件事都要覆盖。

| # | 前置状态 | 操作 | 期望 | 边界类型 |
|---|---------|------|------|---------|
| 1 | 池子 `["Alpha"]`，无人占用 | allocate | 返回 `{name: "Alpha", socketPath: "/.../ai-Alpha.sock"}` | 正常 |
| 2 | 池子 `["Alpha"]`，`ai-Alpha.json` 存在但 PID 已死 | allocate | GC 僵尸 + 复用 Alpha | 僵尸 GC |
| 3 | 池子 `["Alpha"]`，`ai-Alpha.json` 存在且 PID 活 | allocate | 返回 `null` | 占用冲突 |
| 4 | 池子全部被占（15 个 PID 全活） | allocate | 返回 `null` | 池子耗尽 |
| 5 | 池子第 5 个是僵尸，前 4 个活 | allocate | 返回第 5 个（跳过活的） | 混合 |
| 6 | socket 路径已存在（`ai-Alpha.sock` 文件残留） | allocate | unlink 残留 + listen | socket 残留 |
| 7 | `connectionHandler` 抛错 | allocate | 清理（unlink socket + registry）+ 返回 `null` | handler 错误 |

**Mock 策略**：

- PID 死活：mock `process.kill(pid, 0)` 的返回（不真起进程）
- Socket listen：用真实 `net.createServer()` + `listen(path)`，测完 unlink
- 时间：用 `bun:test` 的 `setSystemTime` mock `Date.now`（如需时间戳断言）

#### 4.2.4 `formatText()`

照搬 D10 决策 4 版。**测试用例**（照 aibp-pi 已有的测试矩阵复制）：

| # | 输入 payload | 期望 output | 边界 |
|---|-------------|-------------|------|
| 1 | `{selection: "foo()", message: "explain"}` | `<selection>foo()</selection>\n<user-input>explain</user-input>` | 有选区有 message |
| 2 | `{selection: "foo()"}` | `<selection>foo()</selection>` | 仅选区 |
| 3 | `{message: "explain"}` | `<user-input>explain</user-input>` | 仅 message |
| 4 | `{selection: "<script>alert(1)</script>", message: "safe?"}` | 内容原样，**不转义** | 注入风险（确认行为） |
| 5 | `{selection: "line1\nline2"}` | 内容含 `\n`（不强制换 `<br>`） | 多行 |
| 6 | `{selection: "", message: ""}` | 两标签都空 | 双空 |

### 4.3 MCP server 测试（段 B）—— 80% 覆盖目标

#### 4.3.1 capabilities 注册

```typescript
test('server registers claude/channel capability', () => {
  const { server } = createTestServer()
  // 断言 capabilities.experimental['claude/channel'] 存在
  expect(server.capabilities.experimental['claude/channel']).toEqual({})
})

test('server does NOT register tools capability (v1 single-direction)', () => {
  const { server } = createTestServer()
  expect(server.capabilities.tools).toBeUndefined()  // ⭐ v1 单向铁律
})
```

> ⚠️ **这条测试是 v1 单向的回归保护**——未来谁手贱加了 `tools: {}` 就立即失败。

#### 4.3.2 name 字段

```typescript
test('server name is "aibp" (fixed source)', () => {
  const { server } = createTestServer()
  expect(server.name).toBe('aibp')  // ⭐ 与方案 §5.3 决策一致
})
```

### 4.4 Unix socket 测试（段 C）—— 100% 覆盖目标

#### 4.4.1 connection handler 分帧

```typescript
test('partial frame buffers until newline', async () => {
  const sock = connectToTestServer()
  sock.write('{"v":"aibp-2.0","pa')        // 半截
  await sleep(10)
  expect(handlerCalls).toEqual([])          // 未触发
  sock.write('yload":{"selection":"x"}}\n') // 补全
  await sleep(10)
  expect(handlerCalls).toEqual([{...}])     // 触发一次
})

test('two complete frames in one write', async () => {
  sock.write('{"v":"aibp-2.0","payload":{"selection":"a"}}\n{"v":"aibp-2.0","payload":{"selection":"b"}}\n')
  expect(handlerCalls.length).toBe(2)
})

test('connection close flushes partial frame (no warning)', async () => {
  sock.write('{"v":"aibp-2.0","pa')
  sock.end()
  await sleep(10)
  // 部分帧丢弃是正确行为，不应 warn
})
```

#### 4.4.2 version check

```typescript
test.each([
  ['aibp-2.0', true],       // 主版本一致
  ['aibp-2.1', true],       // 主版本一致（minor 兼容——v1 决策）
  ['aibp-1.0', false],      // 主版本不一致
  ['aibp-3.0', false],      // 主版本不一致
  ['aibp-2', false],        // 缺 minor
  ['xxx-2.0', false],       // 协议名错
])('', (v, expected) => {
  const env = { v, payload: {...} }
  expect(shouldAcceptEnvelope(env)).toBe(expected)
})
```

### 4.5 delivery 测试（段 D）—— 100% 覆盖目标

#### 4.5.1 mock MCP server

```typescript
function createMockMcpServer() {
  const notifications = []
  return {
    notifications,
    notification: async (args) => { notifications.push(args) },
    // ... 其他 mock 方法
  }
}

test('handleLine sends channel notification with formatted content', async () => {
  const mcp = createMockMcpServer()
  const ctx = { mcp }
  const env = { v: 'aibp-2.0', payload: { selection: 'foo()', message: 'explain', path: '/x.go', loc: {start:{line:1},end:{line:1}} } }
  await handleLine(env, ctx)
  expect(mcp.notifications).toEqual([{
    method: 'notifications/claude/channel',
    params: {
      content: '<selection>foo()</selection>\n<user-input>explain</user-input>',
      meta: { path: '/x.go', lines: '1-1', with_message: 'true' },
    },
  }])
})

test('handleLine with no message still sends (v1 single-direction decision)', async () => {
  // §5.1 决策回归保护
  const env = { v: 'aibp-2.0', payload: { selection: 'foo()', path: '/x.go', loc: {...} } }
  await handleLine(env, ctx)
  expect(mcp.notifications.length).toBe(1)
  expect(mcp.notifications[0].params.meta.with_message).toBe('false')
})

test('handleLine propagates mcp.notification error', async () => {
  const mcp = { notification: async () => { throw new Error('stdio broken') } }
  await expect(handleLine(env, { mcp })).rejects.toThrow('stdio broken')
})
```

### 4.6 Go 端测试（ensure_claude_test.go）

照搬 `ensure_opencode_test.go` 的 `TestOpencodeAIBPVersion` 结构（同一作者、同一接口契约）：

```go
func TestClaudeAIBPVersion(t *testing.T) {
    t.Run("plugin not installed", func(t *testing.T) { ... })
    t.Run("plugin installed via cache (after marketplace install)", func(t *testing.T) { ... })
    t.Run("plugin installed from source", func(t *testing.T) { ... })
    t.Run("installed.json corrupt", func(t *testing.T) { ... })
    t.Run("channels disabled by org policy", func(t *testing.T) { ... })
}
```

**与 opencode 测试差异点**：

- opencode 读 `tui.json`；Claude 读 `installed.json` + `settings.json`
- opencode cache 是 `~/.cache/opencode/packages/...`；Claude cache 是 `~/.claude/plugins/cache/aibp-claude/<sha>/...`

---

## 5. 集成测试与验收矩阵

### 5.1 集成测试（mock Claude + 真实 socket）

**最难但最重要**：验证整条管道（microNeo → unix socket → MCP notification → Claude）真的串起来了。

#### 5.1.1 测试架构

```
┌─────────────────┐       stdin/stdout       ┌──────────────────┐
│ Mock Claude     │◀─────────────────────────▶│ aibp-channel.ts  │
│ (stdin reader)  │                          │ (test instance)  │
└─────────────────┘                          └──────────────────┘
                                                       ▲
                                                       │ unix socket
                                                       ▼
                                                ┌──────────────────┐
                                                │ Test sender      │
                                                │ (模拟 microNeo)  │
                                                └──────────────────┘
```

#### 5.1.2 测试步骤

```typescript
// integration.test.ts
test('end-to-end: sender → socket → mcp → mock Claude', async () => {
  // 1. 起 mock Claude（stdin 收 notification）
  const mockClaude = spawnMockClaude()  // spawn 一个 stdio reader
  
  // 2. 起 aibp-channel.ts（连接 mockClaude + listen unix socket）
  const channel = spawnBunScript('aibp-channel.ts', { stdio: 'pipe' })
  await waitForReady(channel)
  
  // 3. 模拟 microNeo 连 socket 发帧
  const sock = net.createConnection(channelSocketPath)
  sock.write(JSON.stringify({
    v: 'aibp-2.0',
    payload: { selection: 'foo()', message: 'explain', path: '/x.go', loc: {...} },
  }) + '\n')
  
  // 4. 断言 mock Claude 收到了正确格式的 channel notification
  const notification = await mockClaude.waitForNotification(1000)
  expect(notification.method).toBe('notifications/claude/channel')
  expect(notification.params.content).toContain('<selection>foo()</selection>')
  
  // 5. 清理
  channel.kill()
  mockClaude.close()
})
```

#### 5.1.3 何时跑

- **PR 必跑**：集成测试 1 分钟内，CI 必跑
- **本地**：每次改段 D 都跑（notification 字段变了立刻看到）

### 5.2 手测流程（PR 前必走一遍）

```bash
# Terminal 1: 启动 microNeo
make build && ./micro

# Terminal 2: 启动 Claude（开发模式加载插件）
claude --plugin-dir ./internal/aibp/aibp-agents/claude \
       --dangerously-load-development-channels plugin:aibp-claude

# 验证：
# 1. 左下角 footer / `/mcp` 显示 aibp-channel connected
# 2. microNeo 打开 .go 文件，选中一段代码
# 3. Alt-Enter，弹出输入框
# 4. 输入 "explain this"
# 5. Claude session 立即出现：
#    <channel source="aibp" path="..." lines="..." with_message="true">
#    <selection>...</selection>
#    <user-input>explain this</user-input>
#    </channel>
# 6. Claude 立即响应（解释代码）

# Terminal 3: 看日志（如有）
tail -f /tmp/aibp-claude.log  # 仅 DEBUG=true 时有内容
```

### 5.3 验收矩阵（T1-T30）

**继承**方案 §九 T1-T15 + **新增** T16-T30（聚焦本次新增的边界）：

| # | 场景 | 期望 | 类型 |
|---|------|------|------|
| T1-T15 | （继承方案 §九，**不重复**） | — | — |
| **T16** | `normalizeNames` 输入全是非法字符 | 返回 `[]` + console.warn | 单测 |
| **T17** | `allocateName` 池子耗尽时调第二次 | 第二次仍返回 `null`（无副作用） | 单测 |
| **T18** | `formatText` 同时含 `<` 和 `>` 字符 | 原样输出（**不转义**，与 aibp-pi 行为一致） | 单测 |
| **T19** | socket 半截帧 + 客户端断开 | 缓冲丢弃，handler 不触发，无 warn 日志 | 单测 |
| **T20** | socket 写入非 JSON 字符串 | handler 抛错，**连接关闭**（不污染后续帧） | 单测 |
| **T21** | `mcp.notification` 抛错时 handleLine 行为 | promise reject，error 记日志，**不重试**（投递语义） | 单测 |
| **T22** | meta key 含连字符（`lines: "12-14"`） | Claude 静默丢弃该 meta（channels-reference.md §Notification format） | 单测 |
| **T23** | 集成测试：完整端到端流程 | 见 §5.1.2 | 集成 |
| **T24** | 集成测试：socket 写入 100 帧 | mock Claude 按序收到 100 个 notification | 集成/性能 |
| **T25** | 集成测试：microNeo 端用 `Discover()` 扫到 aibp-claude | registry 文件 + PID 校验 + 主版本校验全过 | 集成 |
| **T26** | `--dangerously-load-development-channels` 缺失时启动 Claude | Claude 报 "blocked by org policy" 或类似错误，**aibp-channel 不加载**（不崩溃） | 手测 |
| **T27** | Claude 进程崩溃（kill -9） | aibp-channel.ts 收到 SIGPIPE → 清理 socket + registry → 退出码 0/非 0 均可 | 手测 |
| **T28** | 三个 receiver（pi/opencode/claude）同时在线 | microNeo 的 `Discover()` 给三个都投消息，互不干扰 | 手测 |
| **T29** | 重复 Alt-Enter（同一选区 + 同一 message 多次） | 每次都新生成一条 notification（**不 dedup**——与 pi/opencode 同） | 集成 |
| **T30** | 卸载 aibp-claude（`claude plugin disable`） | registry 文件残留 → microNeo `Discover()` 检测 PID 死亡 → GC → 不影响其它 receiver | 手测 |

---

## 6. 验证清单与覆盖率门禁

### 6.1 PR 前必跑

```bash
# 1. TS 单测
cd internal/aibp/aibp-agents/claude
bun test                      # 全部单测

# 2. TS 集成测试
bun test test/integration.test.ts

# 3. Go 端单测
cd ../../../..                  # 回 microNeo 根
go test ./internal/aibp/ensure_agents -run TestClaude

# 4. Go 端全量（不破坏现有 pi/opencode 测试）
go test ./internal/aibp/...

# 5. microNeo 整体构建
make build
```

### 6.2 覆盖率门禁

| 模块 | 目标 | 工具 |
|------|------|------|
| `normalizeNames` | **100%** | `bun test --coverage` |
| `loadNamePool` | **100%** | 同上 |
| `allocateName` | **100%** | 同上 |
| `formatText` | **100%** | 同上 |
| `handleLine` | **100%** | 同上 |
| connection handler（段 C） | **100%** | 同上 |
| MCP server 构造（段 B） | **80%** | 同上 |
| Go: `ClaudeEnsurer` 各方法 | **80%** | `go test -cover` |
| **整体** | **≥ 85%** | — |

**门禁生效方式**：

- `bun test --coverage` 输出覆盖率数字
- CI 步骤检查整体 ≥ 85%（协议层必须 100%——这是 v1 的质量底线）
- 覆盖率下降的 PR 需在 description 里解释

### 6.3 Lint / 类型检查

```bash
# TypeScript（即使 Bun 跑 TS 无需编译，仍开 tsc 检查类型）
bunx tsc --noEmit aibp-channel.ts

# Go
go vet ./internal/aibp/...
```

---

## 7. 实施顺序（最小可工作单元）

**每步独立可验证**（借鉴 `ensure_opencode-实施计划.md §4` 模式）：

| # | 步骤 | 验证手段 | 时间估 |
|---|------|---------|--------|
| 1 | 写 `package.json`（§3.3） | `bun install --dry-run` 无错 | 5min |
| 2 | 写 `protocol.test.ts`（§4.2，**先写测试**） | `bun test` 全红（还没实现） | 30min |
| 3 | 写 `aibp-channel.ts` 段 A（§3.4.1） | `bun test` 全绿 | 1h |
| 4 | 写 `.claude-plugin/plugin.json`（§3.1） | `jq` 解析通过 | 5min |
| 5 | 写 `.mcp.json`（§3.2） | `jq` 解析通过 | 5min |
| 6 | 写 `aibp-channel.ts` 段 B（§3.4.2） | `bun aibp-channel.ts` 不崩溃（MCP 握手） | 20min |
| 7 | 写 `socket.test.ts`（§4.4） | `bun test` 全红 | 20min |
| 8 | 写 `aibp-channel.ts` 段 C（§3.4.3） | `bun test` 全绿 | 1h |
| 9 | 写 `delivery.test.ts`（§4.5） | `bun test` 全红 | 15min |
| 10 | 写 `aibp-channel.ts` 段 D（§3.4.4） | `bun test` 全绿 | 30min |
| 11 | 写 `integration.test.ts`（§5.1） | `bun test test/integration.test.ts` 全绿 | 1h |
| 12 | 写 `README.md`（§3.5） | markdownlint 通过 | 20min |
| 13 | Phase 1 build gate：启动 Claude 开发模式 | 左下角 `aibp-channel ● connected` | 15min |
| 14 | Phase 2 build gate：端到端手测（T1-T15 + T26-T30） | 全部通过 | 30min |
| 15 | 写 `ensure_claude.go`（§3.6） | `go build` 通过 | 1h |
| 16 | 写 `ensure_claude_test.go`（§4.6） | `go test` 全绿 | 1h |
| 17 | Phase 3 build gate：`:check-agent` 走到 Claude | infobar 显示状态 | 15min |

**总计**：≈ 8.5 小时（不含 review/迭代）

---

## 8. 风险与备选

| 风险 | 应对 | 触发条件 |
|------|------|---------|
| **Bun:test mock stdio 不好写** | 集成测试用 `child_process.spawn` + 真实 stdin/stdout，**不 mock**（§5.1 已经是这个方案） | 永远 |
| **Claude Code API 变化**（channels 还在 research preview） | 固定 Bun + Claude 版本号到 README；测试矩阵 T26 验证 flag 缺失场景 | Claude 大版本升级时 |
| **unix socket 跨平台**（Windows 不支持 AF_UNIX 监听） | aibp-channel.ts 用 `net.createServer().listen(path)` 即可（Node 20+ 已支持 Windows AF_UNIX）；macOS/Linux 优先 | — |
| **覆盖率门禁拖累迭代** | 协议层 100% 是底线；其它 80% 可视情况放宽（但要在 PR 注释说明） | 第一个覆盖率不达标 PR 时 |
| **段 D `mcp.notification` 在 Claude 关闭后不报错** | 集成测试 §5.1 加 T21；channel 子进程不应被未捕获 rejection 干掉 | T21 失败时 |

---

## 9. 实施 checklist（贴 PR description 用）

```markdown
## aibp-claude 实施 PR

### 改动文件
- [ ] `.claude-plugin/plugin.json`（新建）
- [ ] `.mcp.json`（新建）
- [ ] `package.json`（新建）
- [ ] `aibp-channel.ts`（新建，~305 行）
- [ ] `README.md`（新建）
- [ ] `test/protocol.test.ts`（新建，~150 行）
- [ ] `test/socket.test.ts`（新建，~80 行）
- [ ] `test/delivery.test.ts`（新建，~50 行）
- [ ] `test/integration.test.ts`（新建，~80 行）
- [ ] `internal/aibp/ensure_agents/ensure_claude.go`（新建，~150 行）
- [ ] `internal/aibp/ensure_agents/ensure_claude_test.go`（新建，~250 行）

### 自检
- [ ] `bun test` 全绿，覆盖率 ≥ 85%（协议层 100%）
- [ ] `go test ./internal/aibp/...` 全绿
- [ ] `make build` 通过
- [ ] Phase 1 build gate：Claude 开发模式加载 `aibp-channel`
- [ ] Phase 2 build gate：microNeo Alt-Enter → Claude session 收到 `<channel source="aibp">`
- [ ] Phase 3 build gate：`:check-agent` 走到 Claude
- [ ] T1-T30 全过（详见 §5.3）

### 注意事项
- [ ] `aibp-channel.ts` 顶部 `DEBUG = true`（v1 开发期保持 true；发布时改 false）
- [ ] `package.json` 的 `aibp.protocol = "aibp-2.0"` 字面正确
- [ ] `package.json` 的 `files` 数组含 5 个文件
- [ ] README 加 ⚠️ 注意事项 1（`/tmp/aibp-claude.log` 明文）
```

---

## 10. 附录：MCP SDK 已知陷阱（实施时踩坑）

> 这两个陷阱是在实施集成测试（§5.1）时踩的坑，**不限于 aibp-claude**——任何用 `@modelcontextprotocol/sdk` 写 channel / 自定义 notification 的开发者都可能遇到。记录在此避免重复踩。

### M1：MCP server 禁止 `console.log`（stdout 专用于 JSON-RPC） ⭐⭐

**现象**：MCP server 用 `console.log` 打日志后，client 端收不到 notification / 行为异常。

**根因**：
- `StdioServerTransport` 默认绑定 `process.stdout`，用它传输 JSON-RPC（每行一个 `{"jsonrpc":"2.0",...}`）
- `console.log` 也写 `process.stdout`，把**非 JSON-RPC 文本混入消息流**
- client 端 `ReadBuffer` 按行 `JSON.parse`，遇到 `console.log` 的文本 → 抛错 → 触发 `transport.onerror`（server 不崩，但通知受这行噪音干扰）

**正确做法**：
- MCP server 的所有日志一律写 **stderr**：`console.error` / `console.warn`
- stdout 上只能有 SDK 方法（`mcp.notification()` / `mcp.send()` 等）产生的 JSON-RPC

**修复点**：`aibp-channel.ts` 段 C 末尾的 `console.log(listening)` → `console.error`。

### M2：`fallbackNotificationHandler` 必须实例赋值（不能通过 options 传） ⭐⭐⭐

**现象**：通过 `new Client(info, { fallbackNotificationHandler: fn })` 传入的 handler **完全不生效**——所有走 fallback 分支的 notification（即非 SDK 内置 schema 的 method，如 `notifications/claude/channel`）被**静默丢弃**，无任何报错。这是最难调试的那种 bug（server 端一切正常，client 端无报错，notification 凭空消失）。

**根因**（基于 `@modelcontextprotocol/sdk` 源码）：
- `fallbackNotificationHandler` 是 `Protocol` 抽象类的**实例属性**（`shared/protocol.d.ts` 第 265 行，在 class 声明内）
- 它**不在 `ProtocolOptions` type 里**（第 14-53 行只有 `enforceStrictCapabilities` / `debouncedNotificationMethods` / `taskStore` 等）
- `Protocol` constructor（`protocol.js` 第 14 行）只做 `this._options = _options`，**不从 options 提取** `fallbackNotificationHandler`
- `_onnotification`（第 274 行）读 `this.fallbackNotificationHandler`（恒为 `undefined`）→ 找不到 handler → 静默 `return`

**错误写法**（不生效）：

```typescript
const client = new Client(
  { name: 'mock', version: '1.0.0' },
  { capabilities: {}, fallbackNotificationHandler: async (n) => { ... } },  // ❌ 被忽略
)
```

**正确写法**（实例赋值）：

```typescript
const client = new Client(
  { name: 'mock', version: '1.0.0' },
  { capabilities: {} },
)
client.fallbackNotificationHandler = async (n) => { ... }  // ✅
```

**适用场景**：接收 `notifications/claude/channel` 这类**非 SDK 内置 schema** 的 notification 时必须用 fallback——`setNotificationHandler` 需要 zod schema 且内部 `getMethodLiteral` 从 schema metadata 提取 method，对自定义 method 极不便。

**修复点**：`test/integration.test.ts` 的 `beforeEach` 改为实例赋值，注释标注。

### M3：诊断手法——raw MCP 握手（绕过 SDK Client）

当 SDK Client 行为不符合预期、无法定位是 server 端还是 client 端问题时，用以下思路绕过 SDK，直接观察 **server stdout 的原始字节**：

1. 手动 `spawn` 子进程（`bun aibp-channel.ts`），`stdio: ['pipe', 'pipe', 'inherit']`
2. 手写 MCP initialize 请求 JSON（`{jsonrpc,id:1,method:'initialize',params:{protocolVersion,capabilities,clientInfo}}` + `\n`）写到 stdin
3. 手写 `notifications/initialized` 通知
4. 连 unix socket 发 AIBP envelope
5. 监听 `child.stdout` 的 `data` 事件，打印每行原始内容

**判定**：如果 stdout 上能看到 `notifications/claude/channel` 的 JSON 行 → server 端正常，问题在 client 端解析/分发；否则 server 端有问题。

本次正是用这个手法确认 M2：stdout 上 notification 明确存在，从而锁定问题在 client 的 `_onnotification` 分发，而非 server 的 `mcp.notification()` 发送。

---

## 变更日志

| 日期 | 事件 |
|------|------|
| 2026-07-02 | 初稿：基于 `aibp-claude-方案.md`（R8 修正后版本）展开实施 & 测试细节 |
| 2026-07-02 | 追加 §10：集成测试实施时踩出的 MCP SDK 陷阱 M1（stdout 污染）+ M2（fallbackNotificationHandler 实例赋值）+ M3（raw 诊断手法） |