# Editor-Agent Bridge：架构设计

## 1. 需求

- 用户同时开多个 microNeo，编辑不同文件
- 每个 microNeo 在光标移动、选区变化时，实时通知 AI Agent
- AI Agent（pi / opencode）知道用户正在看哪个文件、哪一行、选中了什么

## 2. 架构

### 2.1 通信方式：Unix Domain Socket + NDJSON

**为什么是它：**
- 消息小（几百字节）、频率低（每秒几次）、纯本机
- Go 标准库 + Node 内置，零依赖
- 比 TCP 快，比 ZeroMQ 轻量

**为什么不是其他：**
- ZeroMQ：引入外部依赖，本机通信用不上它的消息模式
- WebSocket：需要额外库，本机通信无优势
- gRPC：太重
- stdio pipe：只能一对一

### 2.2 谁当 Server

| 方案 | 适用场景 |
|------|---------|
| Agent 当 Server | Agent 生命周期 ≥ 编辑器；典型用法是 pi 一直开着，microNeo 随开随关 |
| microNeo 当 Server | 编辑器先启动、Agent 后启动 |
| 独立 daemon | 两边都可能随时启停 |

**选 Agent 当 Server。** 理由：
- pi/opencode 通常持续运行，microNeo 随开随关，Agent 生命周期更长
- microNeo 端实现最简单：连上了就发消息，连不上就忽略
- 多 microNeo 连同一个 Agent 是 server-client 自然模型

**风险：Agent 重启时所有 editor 断连。** 对策：microNeo 自动重连，几秒恢复。

**多 Agent 场景（pi + opencode 同时跑）：** 各用不同 socket 文件。microNeo 端配置连哪一个。

### 2.3 整体拓扑

```
microNeo #1 ──┐
microNeo #2 ──┼──→  Unix Socket (Agent) ──→  Agent 的状态表
microNeo #3 ──┘       (NDJSON)                └→ LLM via tool / system prompt
```

- 单向推送为主（editor → agent）
- Socket 是双向的，预留 agent → editor 反向控制空间

## 3. 数据流

### 3.1 editor → agent 的消息

| 消息 | 触发时机 | 内容 |
|------|---------|------|
| `cursor_update` | 光标移动 / 选区变化 | pid, file, cursor(line,col), selection(start,end 或 null) |
| `ping` | 定期（每 30s） | pid |
| `file_saved` | 保存文件时 | pid, file |
| `bye` | 编辑器退出 | pid |

### 3.2 agent 维护的状态

以 pid 为 key 的哈希表，每个条目：

```
pid → { file, cursor, selection, timestamp }
```

- 每次 `cursor_update` 覆盖该 pid 的条目
- 收到 `bye` 或连接断开 → 删除
- Agent 提供"查全部"和"查最近活跃"两种查询方式

### 3.3 agent → LLM 的暴露方式

两种方式，不互斥：

| 方式 | 触发者 | 优点 | 缺点 |
|------|--------|------|------|
| **Tool** | LLM 主动调用 | 按需获取，token 省 | 需要一轮 tool call |
| **System prompt 注入** | 每次 prompt 自动带入 | LLM 无需额外操作 | 每次 prompt 都消耗 token；信息可能过时 |

**Phase 1 先做 Tool。** 验证体验后再决定是否自动注入。

### 3.4 selected_text 的处理

不随 cursor_update 发送。原因：
- 选区可能很大（全选整个文件）
- 每次光标移动都读 buffer 有开销
- 大部分时候 LLM 只需要知道"用户在看哪个文件哪一行"

**如果 LLM 需要选区内容**：它本来就有 `read` tool，知道文件和行号就够了。

## 4. microNeo 端设计

### 4.1 侵入方式

新增 `internal/bridge/` package，在三个点挂载：

| 挂载点 | 时机 | 作用 |
|--------|------|------|
| main() 启动流程 | config 加载后 | 初始化 bridge，读配置，异步连接 socket |
| DoEvent() 主循环末尾 | 每个事件循环结束 | 检查当前光标位置，如有变化则发送 |
| exit() | 编辑器退出 | 发送 bye，关闭连接 |

对 micro 原生代码的总修改：~10 行。bridge package 完全独立。

### 4.2 防抖

每次事件循环都可能触发检查，但不是每次都发：

1. **位置去重**：file + line + col + selection 完全相同 → 不发
2. **时间限频**：距上次发送 < 100ms → 延迟发送（保留最新位置）
3. **不阻塞主循环**：消息放入 channel，由单独 goroutine 发送

### 4.3 连接管理

- 连接失败 → 静默，不影响编辑器
- 后台定期重连（每 3s）
- 写入失败（agent 挂了）→ 标记断开，自动重连
- channel 满时丢弃旧消息，绝不阻塞编辑器

### 4.4 配置

在 microNeo 的 settings.json 中：

| 配置 | 默认值 | 说明 |
|------|--------|------|
| `bridgeEnabled` | false | 总开关，默认关闭 |
| `bridgeSocket` | ~/.microneo/bridge.sock | socket 路径 |
| `bridgeDebounce` | 100 | 防抖间隔(ms) |

> key 命名需确认 micro 的 settings 是否支持点号（`bridge.enabled`）。不支持则用驼峰。

## 5. Agent 端设计

两个 Agent 都用 TypeScript 写扩展，能力基本对等，但扩展 API 不同。

### 5.1 两个 Agent 的扩展能力对比

| 能力 | pi | opencode |
|------|----|----------|
| 扩展机制 | Extension（`.ts` 文件放 `~/.pi/agent/extensions/`） | Plugin（npm 包，`opencode plugin install`） |
| 语言 / 运行时 | TypeScript / Node.js | TypeScript / Bun |
| 注册 Tool | `pi.registerTool()` | `tool()` 函数挂在 `Hooks.tool` 上 |
| 注入 System Prompt | `before_agent_start` 事件 | `"experimental.chat.system.transform"` hook |
| 生命周期 | `session_start` / `session_shutdown` | `event` hook |
| Schema 库 | typebox | zod |
| 内置 HTTP Server | 无（需自己写） | 有（`opencode serve` / `opencode acp`） |

### 5.2 共性部分：两个 Agent 做的事完全一样

1. 启动时创建 Unix socket server，监听连接
2. 维护 pid → 编辑器状态的映射
3. 向 LLM 暴露 `get_editor_context` tool
4. 连接断开时清理对应 pid 的状态
5. 退出时关闭 server，清理 socket 文件

**只有注册 tool 和 hook 的 API 语法不同。** 协议、socket server、状态管理逻辑可以完全复用。

### 5.3 差异点

**opencode 有内置 HTTP server**（`opencode serve`），理论上 microNeo 可以走 HTTP 而不是 Unix Socket。但不走这条路——原因是 pi 没有内置 server，如果走 HTTP 就没法给 pi 用了。统一用 Unix Socket，两边都能跑。

**opencode 的 plugin 是 npm 包**，需要 `opencode plugin install` 安装，比 pi 的"放个 .ts 文件就行"重一些。但不影响功能。

### 5.4 socket 文件竞争

Agent 启动时如果 socket 文件已存在：
- 旧 agent 崩溃残留 → unlink 后重建
- 另一个 agent 正在运行 → listen 失败 → 报错提示用户

### 5.5 状态过期

长时间无更新的实例可能已经崩溃但没发 bye（进程被 kill -9）。

策略：收到 `ping` 或 `cursor_update` 时更新 timestamp。超过 N 分钟（如 5 分钟）无任何消息的条目视为离线，标记或清理。

## 6. 实施阶段

### Phase 1 — 能用

microNeo: bridge package + 三个挂载点 + 配置项  
Agent: socket server + get_editor_context tool  
手动测试：nc 模拟 server 验证 editor 端发送；真实 pi + microNeo 端到端验证

### Phase 2 — 好用

- system prompt 自动注入
- file_saved 事件
- agent → editor 反向控制（open_file）
- 状态过期机制

### Phase 3 — 生态

- 多 socket / 多 agent 支持（pi 和 opencode 各监听不同 socket，microNeo 配置连哪个）
- 如有需要，迁移到独立 daemon

## 7. 开放问题

1. **settings key 命名**：micro 的 settings 是否支持 `bridge.enabled` 带点的 key？需验证。
2. **选区方向**：micro 的 CurSelection[0] 和 [1] 不保证有序，bridge 端需要排序后再发。
3. **col 语义**：micro 的 Loc.X 是 rune offset（字符偏移），不是 visual column，也不是 byte offset。Agent 端展示给 LLM 时用"第 N 行第 M 列"（1-indexed）更自然。
4. **多个 microNeo 连同一个 agent 时的活跃度排序**：用 timestamp 最新的作为"当前活跃"实例是否合理？还是应该用最后收到事件的？
