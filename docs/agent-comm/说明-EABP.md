# EABP 协议说明（权威）

> **本文档是 EABP（**E**ditor-**A**gent **B**ridge **P**rotocol）的权威文档**。协议定义、约束、契约、生命周期、开放问题**全部**以本文档为准。
>
> **状态**：v1 协议（`eabp-1`）当前实现状态。
>
> **覆盖范围**：注册表、传输、报文 schema、版本、接收端契约、生命周期边界、安全性、开放问题。
>
> **不覆盖**：
> - microNeo 发送端的 UI/采集/拼报文实现 → `说明-发送端.md`
> - pi 接收端的具体 LLM 递送实现 → `说明-接收端.md`
> - notePane 浮窗本身 → `说明-notepane.md`
> - 总体架构与设计哲学 → `说明-架构设计.md`
> - UI 原意象 → `用户界面-V2.md`
> - 需求源头 → `通信的原始需求.md`
> - opencode 接收端调研（未实现）→ `opencode调研.md`

---

## 〇、文档地图

| 章节 | 内容 |
|------|------|
| 一 | 协议定位 + 三条铁律 |
| 二 | 三个角色（Sender / Receiver / Registry） |
| 三 | 注册表（目录算法、文件 schema、注册/注销、存活检测、GC） |
| 四 | 传输层（Unix socket、连接策略、逐行 JSON 分帧） |
| 五 | 报文协议（信封、类型、payload、关键约定） |
| 六 | 递送语义（**协议最核心**） |
| 七 | 协议版本（版本号、协商、解析代码） |
| 八 | 接收端契约（合规要求） |
| 九 | 生命周期与边界情况（处理矩阵） |
| 十 | 安全性 |
| 十一 | v0 → v1 关键变更 |
| 十二 | 开放问题 |

---

## 一、协议定位

### 1.1 一句话定义

**EABP = LSP 式架构**：microNeo 与各 ai agent 是平等的两端，靠同一份协议对接。任何一方都不感知对方内部实现。

```
┌──────────────────────────────────────────────┐
│  microNeo（与具体 agent 无关）                │
│  发送管线：采集 · 组装 · 发现 · 选择 · 发送   │
└─────────────────────┬────────────────────────┘
                      │  EABP 协议（本文档）
       ┌──────────────┼──────────────┐
       ▼              ▼              ▼
   ┌────────┐    ┌──────────┐   ┌────────┐
   │ pi 扩展 │    │ opencode │   │ XXX    │
   └────────┘    └──────────┘   └────────┘
```

### 1.2 三条铁律（**协议设计必须满足**）

1. **agent 无关**：报文/注册表/传输不出现任何 agent 特定概念。接入新 agent 不改 microNeo。
2. **UI 无关**：协议只规定「拿到 `(消息, 编辑上下文)` 之后怎么处理」。消息和上下文从哪来不在协议范围。
3. **能力差异由接收端吸收**：协议规定「递送什么」，不规定「agent 怎么用」。

> **术语约定**：本文一律用「**递送**（deliver）」描述协议把上下文交给接收端这个动作，**不**用「注入」——「注入」暗示隐式改写消息流，而协议层只递送，是否/如何进入 LLM 上下文由接收端自决。

### 1.3 关键限定

- 通信方向：**单向** microNeo → agent（v1；双向为未来扩展）
- 触发方式：**用户主动、按需、一次性**（不是对光标/选区变化的持续追踪）
- 多对多：单机可有多个 microNeo 进程（编辑不同文件）+ 多个 agent 进程

---

## 二、三个角色

| 角色 | 实体 | 职责 |
|------|------|------|
| **Sender** | microNeo 进程 | 拿到 `(消息, 上下文)` → 查注册表 → 选目标 → 发报文 |
| **Receiver** | agent 内的扩展 | 启动注册 → 监听 socket → 解析报文 → 递送到自己的 LLM 通道 |
| **Registry** | 一个固定目录 | 接收端登记自己；发送端发现接收端 |

- 一个用户机器上：可能有**多个 microNeo 进程**（编辑不同文件）= 多个发送端；可能有**多个 agent 进程**（pi、opencode、或多个 pi session）= 多个接收端
- v1 单向；协议预留双向扩展（agent → microNeo 的 `ack` / `goto`/高亮），v1 不实现

---

## 三、注册表（Registry）

### 3.1 注册表目录算法

跨平台、用户私有、重启不残留：

```
base = $XDG_RUNTIME_DIR ? $TMPDIR ? /tmp       // 优先级回落
dir  = base + "/microneo-agent-bridge-" + $UID
```

解析优先级：

1. `$XDG_RUNTIME_DIR`（Linux `/run/user/$UID`，进程退出自动清理）——首选
2. `$TMPDIR`（macOS 通常是 `/var/folders/.../T/`）——macOS 兜底
3. `/tmp`——最后兜底

**Go 实现**（`internal/eabp/registry.go:RegistryDir()`）：

```go
func RegistryDir() string {
    if d := os.Getenv("MNAB_REG_DIR"); d != "" {  // 调试覆盖
        return d
    }
    base := os.Getenv("XDG_RUNTIME_DIR")
    if base == "" { base = os.Getenv("TMPDIR") }
    if base == "" { base = "/tmp" }
    return filepath.Join(base, fmt.Sprintf("microneo-agent-bridge-%d", os.Getuid()))
}
```

**TS 实现**（`eabp-receivers/eabp-pi/index.ts:registryDir()`）：

```typescript
function registryDir(): string {
  const override = process.env.MNAB_REG_DIR;
  if (override) return override;
  const base = process.env.XDG_RUNTIME_DIR
    || process.env.TMPDIR
    || "/tmp";
  return path.join(base, `microneo-agent-bridge-${process.getuid?.() ?? 0}`);
}
```

**实现约束（必须两端一致）**：发送端和接收端各自用**完全相同的算法**算出该路径。本文固定算法，两端复刻。

**`MNAB_REG_DIR` 调试覆盖**：环境变量可覆盖算法，设了就用其值（跳过算法）。**两端都支持**——调试时指个短路径（如 `/tmp/mnab`）方便手查注册文件；生产也用得上。

**属性**：
- 目录名带 `$UID`，多用户机器天然隔离
- 由首个需要它的进程（发送端或接收端）按 `0700` 权限创建
- socket 文件就放在该目录下，权限 `0600`
- 目录本身不主动删除，靠运行时目录随系统重启自动清理

### 3.2 注册文件

每个接收端在该目录下写一个文件 `receiver-<name>.json`，其中 `<name>` 是该接收端的 name 字段值（如 `pi-12345` → `receiver-pi-12345.json`）。**文件名 = 固定前缀 `receiver-` + name**，去前缀即 name，保证全局唯一。

**完整 schema**（`internal/eabp/registry.go:RegFile`）：

```json
{
  "name": "pi-12345",
  "pid": 12345,
  "transport": "unix",
  "socket": "/tmp/.../microneo-agent-bridge-501/receiver-pi-12345.sock",
  "protocol": "eabp-1",
  "startedAt": 1717000000,
  "cwd": "/Users/me/project",
  "labels": ["default"]
}
```

| 字段 | 必需 | 说明 |
|------|------|------|
| `name` | ✅ | 人类可读唯一名。用户起或自动生成（默认 `<agent>-<pid>`，如 `pi-12345`） |
| `pid` | ✅ | 接收端进程 PID，用于存活检测 |
| `transport` | ✅ | 传输类型，v1 固定 `"unix"` |
| `socket` | ✅ | 该接收端监听的 Unix socket 绝对路径 |
| `protocol` | ✅ | 协议版本号（`eabp-1`）。发送端发现版本不匹配时跳过 |
| `startedAt` | ✅ | 启动时间（Unix 秒）。发送端展示「选择目标」列表时的稳定排序依据 |
| `cwd` | ❌ | 接收端工作目录，发送端可据此判断"是不是同一个项目" |
| `labels` | ❌ | 自由标签数组，发送端可按标签筛选（如 `["default"]`、`["frontend"]`）。**v1 未使用** |

**socket 文件**：`receiver-<name>.sock`，与 json 同目录。

> **为什么加 `receiver-` 前缀**：该目录以后可能混入别的角色文件。前缀即角色命名空间，发送端扫描时按前缀过滤（`receiver-*.json`），未来新增角色不改动现有命名规则。socket 文件同理。两端 strip 前缀的算法必须一致。

### 3.3 注册流程

接收端（agent 扩展）启动时（`session_start`）：

1. 算出注册表目录（§3.1）→ 不存在则 `0700` 创建
2. 决定 name（用户可配，否则默认 `<agent>-<pid>`）
3. 算出 socket 路径 = `<注册表目录>/receiver-<name>.sock`
4. 在该路径**创建并监听** Unix socket（stream 类型）
5. 写注册文件 `receiver-<name>.json`（§3.2 全字段）

> **name 冲突处理**：若 `receiver-<name>.json` 已存在（上次崩溃残留，或真的撞名），接收端应检测：读旧文件的 `pid`，若该 pid 已死 → 覆盖注册；若仍活 → 给自己改名（追加 `pid` 或序号）后再注册。避免两个接收端写同一 socket 路径。**当前实现**：默认 `name = pi-<pid>`，PID 天然唯一，冲突处理暂留。

### 3.4 注销流程

接收端正常退出时（`session_shutdown`）：

1. 删除自己的注册文件 `receiver-<name>.json`
2. 删除 socket 文件 `receiver-<name>.sock`
3. 关闭 socket server（`server.close()` 等所有 conn 关闭）

### 3.5 存活检测与 GC（发送端职责）

发送端在发送前发现接收端时，按优先级判断：

1. **首选——试探连接**：`net.DialTimeout("unix", socket, 200ms)` + 立即关闭。连得上即认为存活（最可靠）
2. **旁证——PID 检测**：`syscall.Kill(pid, 0) == nil`（仅 Unix 平台）
3. **顺手 GC**：连接失败且 PID 不存在 → 由发送端**直接删除**注册文件（防止僵尸注册堆积）

> **connect 为权威判据，PID 为旁证**：进程崩溃后 socket 无人 listen，connect 必失败——这是 GC 的主路径，PID 复用问题根本进不了这条判断链（复用旧 PID 的新进程不会监听旧 socket）。PID 仅在 connect 成功时作为辅助确认。

**实现代码**（`internal/eabp/registry.go:Discover()`）：

```go
func Discover() ([]RegFile, error) {
    dir := RegistryDir()
    entries, err := filepath.Glob(filepath.Join(dir, "receiver-*.json"))
    if err != nil { return nil, err }
    var live []RegFile
    for _, p := range entries {
        var rf RegFile
        if b, e := os.ReadFile(p); e != nil { continue }
        else if json.Unmarshal(b, &rf) != nil { continue }
        if major(rf.Protocol) != 1 { continue }  // 协议主版本不符 → 跳过
        if alive(rf.Socket) { live = append(live, rf); continue }
        if !pidAlive(rf.PID) { _ = os.Remove(p) }  // GC
    }
    return live, nil
}
```

**关键行为**：
- 目录不存在或空 → 返回空 slice，**不报错**
- JSON 解析失败 → 跳过该文件（不报错）
- 协议主版本不符 → 跳过（不计入 live，不 GC——留给 agent 自行处理）
- connect 失败 + PID 死 → **删除注册文件**（GC）

---

## 四、传输层

### 4.1 选型：Unix domain socket

| 方案 | 结论 | 理由 |
|------|------|------|
| **Unix socket（采用）** | ✅ | 本地、零网络暴露、快、跨语言成熟；macOS/Linux 原生支持，Windows 10 1803+ 经 AF_UNIX 也可用 |
| TCP localhost | ❌ | 要管理端口冲突、有网络暴露风险 |
| 文件 + 轮询 | ❌ | 延迟高、磁盘 IO、并发竞争 |
| HTTP | ❌ | 每次 POST 开销大、要起 server |

- **v1 范围**：仅 `unix` transport，覆盖 macOS / Linux。其他平台/传输方式（如 Windows named pipe）由注册表的 `transport` 字段预留，v1 不实现
- **socket 类型**：`SOCK_STREAM`（流式），便于按行分帧

### 4.2 连接策略：每次发送一次"连接 → 写一行 → 关闭"

```
发送端                                接收端
  │                                     │
  │ ── connect(socket) ──────────────► │ accept
  │ ── write(json_line + "\n") ──────► │ read 一行 → 解析 → 处理
  │ ── close ─────────────────────────► │ (conn 关闭)
```

- **为什么不用长连接**：按需发送模式下发送频率极低（人手触发），长连接的状态维护成本大于每次建连的微秒级开销；且无状态设计对 agent 重启天然鲁棒
- **接收端实现**：每个连接读到一个 `\n` 就解析一条报文处理；连接关闭即结束本次。未来若扩展为长连接多报文，分帧逻辑不变

### 4.3 编码：逐行 JSON

- 每条报文是一行 JSON，以 `\n` 结束
- 一条报文一行，**不允许跨行**（JSON 内部不得有裸 `\n`；字符串里的换行必须用 `\n` 转义）
- 接收端按 `\n` 分帧，逐行 `JSON.parse`
- 字符编码：UTF-8
- **「单行」是分帧约束，不是数据量约束**：单行 JSON 可以任意长、任意嵌套（大段选区文本、可见缓冲区、长消息都合法），代价只是人类肉眼可读性差
- 接收端因报文可能很大，需用**不定长 buffer**读（按需增长直到遇到 `\n`），不要用定长 buffer 截断

### 4.4 方向

- v1：**单向 microNeo → agent**
- 协议预留双向扩展（agent 回 `ack`；后续 agent → microNeo 的 `goto`/高亮），v1 不实现

---

## 五、报文协议

### 5.1 公共信封（Envelope）

所有报文共享头部字段：

```json
{
  "v": 1,
  "type": "context",
  "sender": { "pid": 54321, "name": "microNeo", "instance": "default" },
  "ts": 1717000123.456,
  "payload": { ... }
}
```

**Go 定义**（`internal/eabp/message.go`）：

```go
const Protocol = "eabp-1"

type Envelope struct {
    V       int             `json:"v"`       // 主版本，当前=1
    Type    string          `json:"type"`    // "context"（v1仅）/ "bye"(预留)
    Sender  Sender          `json:"sender"`
    TS      float64         `json:"ts"`      // Unix 浮点秒
    Payload json.RawMessage `json:"payload"` // 原样透传；调用方按 Type 自行反序列化
}

type Sender struct {
    PID      int    `json:"pid"`
    Name     string `json:"name"`     // "microNeo"
    Instance string `json:"instance"` // 窗口/实例标识
}
```

| 字段 | 必需 | 说明 |
|------|------|------|
| `v` | ✅ | 协议主版本号（整数）。v1 = `1`。等于注册表里的 `protocol` 的主版本 |
| `type` | ✅ | 报文类型，见 §5.2 |
| `sender` | ✅ | 发送端标识对象 |
| `ts` | ✅ | 发送时刻（Unix 浮点秒，毫秒精度）。用于接收端排序/展示 |
| `payload` | ✅ | 报文负载，类型取决于 `type` |

| sender 字段 | 必需 | 说明 |
|------|------|------|
| `pid` | ✅ | 发送端（microNeo）进程 PID |
| `name` | ✅ | 固定 `"microNeo"`（预留未来其他编辑器） |
| `instance` | ✅ | 本 microNeo 实例标识（用户/系统起的窗口/tab 名）。**v1 MVP 固定 `"default"`**（microNeo 当前无实例/窗口 ID 概念）。多实例时让接收端区分来源 |

### 5.2 报文类型

按需发送模式下，报文类型极少：

| `type` | `payload` | 触发时机 | 方向 | v1 状态 |
|--------|-----------|----------|------|---------|
| `context` | 见 §5.3 | **用户主动触发发送**（核心报文） | microNeo → agent | ✅ 实现 |
| `bye` | `{}` | 发送端实例退出（可选，让 agent 清理对应来源的待用上下文） | microNeo → agent | 📋 预留（v1 未发） |

### 5.3 `context` 报文 payload

```json
{
  "path": "/Users/me/a.md",
  "cursor": { "line": 42, "col": 8 },
  "selection": {
    "start": { "line": 42, "col": 8 },
    "end":   { "line": 45, "col": 0 },
    "text":  "## 标题\n正文..."
  },
  "message": "这段标题感觉怪，帮我换个说法"
}
```

**Go 定义**（`internal/eabp/message.go`）：

```go
type ContextPayload struct {
    Path         string     `json:"path"`
    Cursor       Position   `json:"cursor"`
    Selection    *Selection `json:"selection,omitempty"`    // 无选区则省略（不是 null）
    Message      string     `json:"message,omitempty"`      // 有无决定递送路径（§六）
    VisibleLines string     `json:"visible_lines,omitempty"`
}

type Position struct {
    Line int `json:"line"`
    Col  int `json:"col"`
}

type Selection struct {
    Start Position `json:"start"`
    End   Position `json:"end"`
    Text  string   `json:"text,omitempty"`
}
```

| 字段 | 必需 | 类型 | 说明 |
|------|------|------|------|
| `path` | ✅ | string | 文件绝对路径 |
| `cursor` | ✅ | Position | 光标位置（1-based） |
| `selection` | ❌ | object | 选区对象，无选区则**省略整个字段**（不是 `null`） |
| `selection.start` | ✅* | Position | 选区起点（1-based） |
| `selection.end` | ✅* | Position | 选区终点（1-based） |
| `selection.text` | ❌ | string | 选中文本。超阈值时省略（详见 §5.5） |
| `message` | ❌ | string | **消息：用户附的一段文字**。有则接收端作为一条用户消息递送给 LLM（立即触发对话）；无则递送为"待用上下文"。详见 §六 |
| `visible_lines` | ❌ | string | 可见区域文本（可选，更费，默认关闭）。方便 agent 直接看屏幕上下文。**v1 不发送** |

> **Position 类型**：`{ "line": int, "col": int }`，坐标 **1-based**（行从 1 起、列从 1 起）。被 `cursor`、`selection.start`、`selection.end` 复用。
>
> **合法性约束（协议契约）**：由**发送端保证合法**——`line`/`col` 最小为 1（1-based），`col` 不超出该行长度，`selection.end` 不小于 `selection.start`（不允许反向选区）。**接收端不做校验**，遇非法值行为未定义。这呼应原则③——发送端只递送它已知为真的事实。

> *`selection.start` / `selection.end` 在 `selection` 存在时必需。

### 5.4 完整报文示例

**示例 A：带选区 + 消息（最典型的带消息场景）**

```json
{"v":1,"type":"context","sender":{"pid":54321,"name":"microNeo","instance":"default"},
 "ts":1717000123.5,
 "payload":{
   "path":"/Users/me/a.md",
   "cursor":{"line":43,"col":9},
   "selection":{"start":{"line":43,"col":9},"end":{"line":46,"col":1},"text":"## 标题\n正文..."},
   "message":"这段标题感觉怪，帮我换个说法" }}
```

**示例 B：纯上下文（无选区、无提问）**

```json
{"v":1,"type":"context","sender":{"pid":54321,"name":"microNeo","instance":"default"},
 "ts":1717000123.5,
 "payload":{
   "path":"/Users/me/main.go",
   "cursor":{"line":129,"col":4} }}
```

**示例 C：仅光标位置 + 消息（无选区）**

```json
{"v":1,"type":"context","sender":{"pid":54321,"name":"microNeo","instance":"default"},
 "ts":1717000123.5,
 "payload":{
   "path":"/Users/me/a.md",
   "cursor":{"line":11,"col":1},
   "message":"我现在在看这里，帮我看看这段逻辑对不对" }}
```

> 三个示例的区别只在 payload 字段组合，与消息/上下文的**来源无关**——协议不关心它们由什么 UI/热键产生。

### 5.5 selection.text 的发送策略

协议规定 `selection.text` 字段是**可选的**——是否发送由发送端决定，接收端通过「Text 是否为空」判断如何处理（详见 `说明-接收端.md §七`）。

**当前实现**（发送端，`internal/action/notepane.go:lowestCursorScreenRow`）：

| 条件 | `Selection.Text` |
|------|------------------|
| 无选区 | 整个 `Selection` 字段省略 |
| 选区行数 ≤ `MaxSelectionLines`（=30） | 发：实际文本 |
| 选区行数 > 30 | 不发：`Text` 字段省略（但 `Start/End` 仍发） |

> 选区行数对用户可见（编辑器能数行），字节阈值不可见——这是 D10 决策 2-3 的核心动机。

**为什么是发送端决定**：行数判断在发送端做是**最自然的**——发送端已经知道 buffer 内容、做行数判断几乎零成本。接收端再做一遍是浪费。详见 `说明-发送端.md §3.3`。

### 5.6 MarshalLine

发送端用 `Envelope.MarshalLine()` 序列化：`json.Marshal(env)` 后追加 `\n`——一行 JSON 加换行符。

```go
func (e *Envelope) MarshalLine() ([]byte, error) {
    b, err := json.Marshal(e)
    if err != nil { return nil, err }
    return append(b, '\n'), nil
}
```

---

## 六、递送语义（协议最核心的部分）

> 本节定义"接收端拿到 `context` 报文后该干什么"。**协议层只规定语义分歧点（`message` 有无）与必须满足的行为约束，不规定具体实现机制**——后者因 agent 能力而异，由各接收端自决。

### 6.1 两条递送路径

接收端根据 `payload.message` 是否存在分流：

| 报文形态 | 语义 | 接收端职责（协议要求） | 用户体验 |
|----------|------|----------------------|----------|
| **带消息**（`message` 存在且非空） | 用户明确发起一次协作 | 把 `上下文 + 消息` 格式化为一条用户消息，**递送给 LLM 并触发对话回合** | 在 agent 里直接看到 AI 开始回复 |
| **纯上下文**（`message` 缺省/空字符串） | 用户把编辑器状态"挂"到 agent | 把上下文**递送为待用上下文**（pending）；用户下次发消息时带上，或用户主动粘贴/丢弃 | agent 提示"有来自 microNeo 的上下文待用"，用户决定怎么用 |

### 6.2 为什么是这两条路径（不是别的）

回到需求本质：**用户主动、按需、一次性**推送。

- 因为是按需（不是高频实时），**不存在淹没 LLM 的问题**——每条报文都是用户明确想发的
- 带消息 = 编辑器里直接发起协作，最顺
- 纯上下文 = 把编辑器状态"挂"到 agent，用户决定怎么用，**不强行打断 LLM**
- 两者都很轻量，**不需要**实时方案里那套内存快照持续刷新 / debounce / 周期性注入的复杂机制

### 6.3 "带消息" 的递送：协议层一致

带消息的处理**所有接收端一致**：递送为一条用户消息，触发对话。具体用什么机制把消息送进 LLM 通道，是各接收端的实现职责，不是协议的事。

> **冲突处理（协议建议，非强制）**：若 agent 正在 streaming（上一回合未结束），带消息的报文应**排队等当前回合结束**再触发，不打断。具体排队机制由接收端自决。
>
> **协议层边界**：v1 是单向通信，不传 receiver 状态（是否正在 streaming、队列长度等），发送端无法获知冲突是否发生。因此**冲突的识别与排队完全由接收端自主处理**，协议不协调——这属于原则③「能力差异由接收端吸收」的必然结果。

### 6.4 "纯上下文" 的递送：协议层一致，实现因 agent 而异

**协议要求**（所有接收端必须满足）：
- 收到纯上下文报文后，**不得**自动触发 LLM 对话（用户没问，不该答）
- 应有可见提示告知用户"有来自 microNeo 的上下文待用"
- 应提供手段让用户在下次发言时带上，或主动粘贴/丢弃

> 至于"隐式带上"还是"预填可见"等具体形态，由接收端按自身能力自决，协议不强求统一——这正是协议 agent 无关性的体现。

### 6.5 多来源覆盖

多个 microNeo 实例向同一 agent 发纯上下文时，接收端的 `pendingContext` **被新报文覆盖**（只保留最后一次）。报文带 `sender.instance` 区分来源；覆盖时应在提示里更新来源标注。

> 这是 v1 的简化策略。未来若需保留多份待用上下文，可按 `sender.instance` 做 map，但 v1 不做。

### 6.6 报文格式

接收端把 `context` payload 格式化为**实际递送给 LLM/输入框的文本**——具体格式完全由接收端自决（协议不规定）。当前 pi 实现的格式见 `说明-接收端.md §七`（简述：短选区用「来自 path 第 A-B 行的选中内容：」+ 内联文字；长选区/无选区用 `@path :lineX` 引用）。

---

## 七、协议版本

### 7.1 版本号方案

- 注册文件 `protocol` 字段格式：`eabp-<major>`，如 `eabp-1`（"Editor Agent Bridge Protocol v1"）
- 信封 `v` 字段：主版本整数，如 `1`
- **主版本**：报文结构不兼容变更时 +1（增删必需字段、改语义）
- **次版本/兼容 additions**：新增可选字段、新增报文类型——**不改主版本**。接收端应忽略未知字段、忽略未知 `type`（前向兼容）

### 7.2 版本协商

- 发送端读注册文件 `protocol`（字符串如 `eabp-1`），解析取主版本整数（`1`），与信封 `v` 比较；不匹配 → 跳过该接收端 + 告警提示
- 接收端读信封 `v`（整数），与自身版本的主版本比较；不匹配 → 忽略该报文 + 可选告警
- v1 不做复杂的 capabilities 协商（YAGNI）

### 7.3 解析代码

**Go**（`internal/eabp/registry.go:major()`）：

```go
func major(protocol string) int {
    i := strings.LastIndexByte(protocol, '-')
    if i < 0 { return -1 }
    n, err := strconv.Atoi(protocol[i+1:])
    if err != nil { return -1 }
    return n
}
```

**TS**（`eabp-receivers/eabp-pi/index.ts:handleLine()`）：

```typescript
if (env.v !== 1 || env.type !== "context") return;
```

### 7.4 当前版本

**v1（`eabp-1`）**——本文档定义的全部内容。报文类型仅 `context`（`bye` 预留）。

---

## 八、接收端契约（Receiver Contract）

> 本节定义"一个 agent 想接入 microNeo，必须实现什么"。这是 LSP 式架构的接入规范——协议不只规定报文格式，也规定接收端的行为义务。

### 8.1 协议合规要求（所有接收端必须满足）

一个合规的接收端必须：

1. **启动时注册**：算注册表目录（§3.1）→ 起 Unix socket server（§四）→ 写注册文件（§3.2）
2. **监听并分帧**：accept 连接 → 按 `\n` 分帧 → `JSON.parse` 每行 → 校验信封 `v`/`type`
3. **处理 `context` 报文**：按 §六 分流（`message` 有无）
4. **退出时注销**：删注册文件 + 删 socket + 关 server（§3.4）

> 接入新 agent **不需要改 microNeo 任何代码**——这正是 LSP 式架构的价值。各 agent 按自身扩展机制实现上述四点即可。

### 8.2 抽象分层

接收适配层可以抽 core（agent 无关）+ adapter（agent 特定）两小层：

| 小层 | 职责 | 形态 |
|------|------|------|
| **core** | 注册/分帧/版本校验/分流 | 跨 agent 共享 |
| **adapter** | 把 core 递送物落到 agent 自己的 LLM 通道 | agent 特定 |

> 当前 pi 实现没显式分层（`index.ts` 一个文件）——M2 形态简单，没必要分。如果接第二个 agent 再考虑抽 core。

---

## 九、生命周期与边界情况

| 场景 | 处理 | 详见 |
|------|------|------|
| **agent 崩溃**（没清注册表） | socket 文件残留但连不上；发送端试探连接失败 → 删该注册文件（GC，§3.5）→ 提示重新选择 | §3.5 |
| **microNeo 崩溃** | 无影响。按需模式下没有持续连接，agent 端 `pendingContext` 是用户主动送来的，本来就等用户处理 | — |
| **多 microNeo → 同一 agent** | 报文带 `sender.instance` 区分；agent 的 `pendingContext` 被新报文覆盖（只保留最后一次，§6.5） | §6.5 |
| **agent 正在 streaming** | 接收端自主排队：v1 单向不传状态，冲突的识别与处理全由接收端负责（§6.3） | §6.3 |
| **协议版本不匹配** | 发送端读注册文件 `protocol`，主版本不匹配 → 跳过该接收端 + 告警 | §7.2 |
| **name 冲突**（两个接收端撞名） | §3.3：后启动者检测旧 pid 已死则覆盖，仍活则自改名（**当前实现用 `pid` 天然规避**） | §3.3 |
| **socket 文件被外部删除** | 接收端 accept 失败 / 下次 listen 重建；发送端连接失败 → GC 注册文件 | §3.5 |

---

## 十、安全性

- **只走本地 Unix socket**，无网络监听、无端口暴露
- 注册表目录 `0700`，socket 文件 `0600`，仅本用户可访问
- 报文里 `visible_lines`（可见区域文本）为**可选字段，默认不发送**，降低敏感信息意外进入 agent 上下文的风险
- 未来双向扩展（agent → microNeo，如 `goto line`、高亮某行）必须**显式经用户/配置授权**后才允许执行——v1 不实现，但这是双向化的硬门槛

---

## 十一、关键变更（v0 → v1）

| 维度 | v0 | v1（当前） | commit |
|------|----|------|---|
| line/col 基址 | 0-based | **1-based** | `c95bfb52` |
| payload.Message 字段 | 显式 `"message": null` | **omitempty**（无字段时省略） | `c95bfb52` |
| 选区文本阈值 | 2KB 字节 | **30 行**（`MaxSelectionLines` 常量） | `4b2ec65c` |
| 协议常量名 | 草稿 | `eabp-1` | `c95bfb52` |
| `selection` 字段 | 显式 `null` | **整个字段省略**（用指针 + omitempty） | `c95bfb52` |

**v0 → v1 的原因**：

- **1-based line/col** 与 LLM 工具链（sed -n '12p' / ripgrep / read 工具）天然对齐，不需要做 ±1 转换
- **Message omitempty** 让"无 message 报文"更干净
- **30 行阈值** 替代 2KB 字节——行数对用户可见可预测，字节不可见

---

## 十二、开放问题

下列问题在 v1 暂未解决，记录在此供未来 v2 参考。

### 12.1 协议级

1. **新 agent 接入的标准化程度**：本文档 §八 接收端契约 + §一 三条铁律是否够清晰让社区独立实现？是否需要一份独立的"接收端实现规范"文档？
2. **空 content 发送的语义**：触发发送时 notePane 内容为空，是发纯上下文还是放弃？**当前决策（D10）**：notePane 链路**直接拦截不发**；其他入口（如未来"快速发送选区"热键）可能需要不同决策
3. **多份待用上下文**：v1 多来源覆盖只保留最后一次，未来是否按来源保留多份？
4. **隐私黑名单**：v1 不做。是否应 v1 就做（密钥/凭证文件不应进 agent 上下文）？
5. **双向化的授权模型**：未来 agent → microNeo（`goto`、高亮）怎么授权才安全？v1 完全不做
6. **`bye` 报文是否发送**：v1 预留，发送端**不主动发**。如果接收端需要清理来自某 instance 的待用上下文，是否应补发？
7. **payload 合法性校验**：v1 发送端**保证**合法，接收端**不校验**。未来若允许第三方实现发送端，是否要加 schema 校验？
8. **`sender.instance` 的真实实现**：v1 MVP 固定 `"default"`。microNeo 多实例/多窗口支持需要先做"窗口/tab ID"概念

### 12.2 协议相关但属于发送/接收端实现

9. **Windows 支持**：v1 不实现。技术上 AF_UNIX 自 Win10 1803 可用、Go 原生支持，发送端预期可跑；但接收端（Node）支持需验证，故整体留 v2
10. **长连接 vs 每次新建连接**：v1 每次新建连接。如果未来要支持高频 / streaming 双向通信，需重新评估
11. **注册表目录的可移植性**：`process.getuid?.() ?? 0` 的兜底在 Windows 上 UID 0 是假值。当前 Unix-only，不影响

### 12.3 已明确不做

下列议题**已明确不做**（v1 出界），记录以防被重新提出：

- ❌ 持续追踪光标/选区变化（v1 是按需一次性）
- ❌ 内存快照周期刷新
- ❌ TCP localhost（v1 走 Unix socket）
- ❌ 双向通信
- ❌ Windows 接收端（v1 不实现）

---

## 十三、相关文档

| 文档 | 关系 |
|------|------|
| `说明-发送端.md` | microNeo 端的协议实现（采集 + 拼报文 + 发送）。**本文档的「实现视角」** |
| `说明-接收端.md` | pi 接收端的协议实现（注册 + 监听 + 递送 LLM）。**本文档的「实现视角」** |
| `说明-notepane.md` | notePane 浮窗（消息来源 UI）。与协议层正交 |
| `说明-架构设计.md` | 总体架构与设计哲学。**本文是该文档 §三（协议层）的细化与权威** |
| `用户界面-V2.md` | UI 原意象。**与协议层正交** |
| `通信的原始需求.md` | 需求源头 |
| `opencode调研.md` | opencode 接收端调研（未实现，未来） |
