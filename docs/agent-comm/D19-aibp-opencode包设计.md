# D19 · aibp-opencode npm 包设计

> **状态**：方案设计。模仿 `aibp-pi`，把 AIBP 接收端落到 opencode 上，发成独立 npm 包。
>
> **一句话结论**：用 **server 插件**（不是调研报告 §6.5 定的 TUI 插件）——因为调研反转结论时 overlooked 了 `client.tui.appendPrompt`/`submitPrompt`，它让 server 插件能直接「填输入框 + 提交」，与 aibp-pi 的 `setEditorText`/`sendUserMessage` 一一对应。详见 §二。

---

## 一、目标

写一个 `aibp-opencode` npm 包，让 opencode 成为 AIBP 接收端：microNeo 里选中代码按 Alt-Enter，内容递送到当前运行的 opencode 会话。

**模仿 aibp-pi 的形态**（`aibp-agents/pi/`，~120 行单文件 TS）：

| 维度 | aibp-pi | aibp-opencode（本包） |
|------|---------|----------------------|
| 语言 | TS（jiti 直接加载） | TS（Bun 直接加载，无需预编译） |
| 形态 | 单文件 default export 函数 | 单文件 default export 函数 |
| 入口 | `pi.on("session_start", …)` | 插件函数体顶层（opencode server 插件加载即跑） |
| 发对话 | `pi.sendUserMessage(text, {deliverAs:"steer"})` | `client.tui.appendPrompt` + `submitPrompt` |
| 填输入框 | `ctx.ui.setEditorText(text)` | `client.tui.appendPrompt`（不 submit） |
| 提示 | `ctx.ui.notify(…)` | `client.tui.showToast(…)` |
| 协议/注册表/名字池/formatText | inline | **inline（逐字复用 aibp-pi）** |

**职责边界**：本包只做 opencode 接收端。microNeo 端零改动（协议 agent 无关，`说明-架构设计.md` 原则 1）。`ensure_opencode.go`（`:check-agent` 装 opencode 端）是 D17 §附的未来工作，本文 §七衔接。

---

## 二、选型重估：server 插件 vs TUI 插件（**核心结论**）

### 2.1 调研报告的原结论与它的盲点

`opencode调研.md` §6.5 / §6.6 最终结论是「**用 TUI 插件，不做 server 插件**」。理由链：

1. 需求 #5（纯上下文 → 待用）只有 server 侧 `experimental.chat.messages.transform` 能做（隐式注入）；
2. 但「注入 = 隐式改写消息流」不好，于是改走 TUI 插件的 `TuiPromptRef.set()`；
3. `opencode serve` 无头模式是伪需求 → 不做 server 插件。

**这个推理有一个未被质疑的盲点**：server 插件拿到的 `client`（`PluginInput.client`）里有 **`client.tui.appendPrompt` / `submitPrompt` / `clearPrompt` / `showToast`**（已确认，见 `@opencode-ai/sdk/dist/gen/sdk.gen.d.ts:399` 的 `tui: Tui` 命名空间）。它们直接覆盖需求 #4/#5/#6：

| 需求 | server 插件用 `client.tui.*` 怎么做 |
|------|--------------------------------------|
| #4 带 prompt 触发对话 | `appendPrompt({body:{text}})` + `submitPrompt()` |
| #5 纯上下文待用 | `appendPrompt({body:{text}})`（不 submit）→ 预填输入框 |
| #6 UI 反馈 | `showToast({body:{message,variant}})` |

调研报告自己在 §2.6 / §6 矩阵 #6 行**列出过** `client.tui.appendPrompt`，但**只把它当「toast 式 UI 反馈」**，没意识到它**同时**就是需求 #5 的解法。于是 §6.5 错误地判定「需求 #5 server 侧只能靠 messages.transform 隐式注入」，进而反转到 TUI 插件。

### 2.2 关键事实：`client.tui.*` 在 TUI 模式下进程内可达

调研 §5.1 实测铁证：日常 TUI 模式的 opencode **是单进程**，server 与 TUI 是进程内逻辑分离，`PluginInput.client` 是**进程内 client**（`serverUrl` 指向 in-process，非独立监听端口）。因此：

- server 插件虽叫「server」，**在 TUI 模式下照样加载**（server 进程即在），`client.tui.*` 的 `/tui/*` 端点是进程内调用，**直达 TUI 的输入框**。
- 不依赖任何对外监听端口——§5.1「opencode 不监听任何 TCP」与此**完全不冲突**，因为是 in-process。

> 调研 §6.6「只做 TUI 插件不做 server 插件」其实**把「server 插件」错误等同于「为无头 serve 模式做的插件」**。事实是：server 插件在 **TUI 模式和 serve 模式都加载**；我们用它在 TUI 模式下经 `client.tui.*` 控制 TUI，根本不碰 serve 无头场景。§6.6 的「伪需求」论证不成立。

### 2.3 server 插件 vs TUI 插件 对照

| 维度 | **server 插件（推荐）** | TUI 插件（调研原结论） |
|------|------------------------|----------------------|
| 入口 | `export default async (input) => ({...Hooks})` | `export { tui: async (api) => {...} }` |
| 拿输入框通道 | `client.tui.appendPrompt`（隐式「当前 prompt」） | 渲染 `<Slot name="session_prompt" ref=…/>` 拿 `TuiPromptRef` |
| 多 session 找 ref | **无需**——`appendPrompt` 自动路由到活动会话(已验证,V1-验证完成报告) | **需解决**「当前活动 session 的 ref 哪来 / home 页无 session 怎么办」（调研 §8 待验证 #4） |
| 依赖 | 纯 `node:*`，可选 `import type` 自 SDK | 需 `@opentui/solid`（JSX/SolidJS 渲染） |
| widget / slash 命令 | 不做（v1 不需要） | 原生强（`api.ui.Slot` / `api.command.register`），但 v1 用不上 |
| 与 aibp-pi 同构度 | **高**：单函数、无 JSX、delivery 调一个 API | 低：要写 SolidJS 组件塞插槽 |
| 待验证项 | **0 项**(V1/V4/V7已验证) | **3 项**（调研 §8 #3/#4/#5，含「TUI 方案命脉」） |
| 复杂度 | 低（~120 行，与 aibp-pi 相当） | 高（JSX + 插槽生命周期 + ref 绑定） |

### 2.4 结论与回退预案

**用 server 插件**。理由：

1. **与 aibp-pi 同构**（用户原话「模仿 aibp-pi」）：单 default-export 函数，delivery 层一个 API 调用，无 JSX。
2. **更简单更稳**：消解调研 §8 的 3 个「命脉级」待验证项（#3/#4/#5）——TUI 插件的 `ref.set+submit` 要验、多 session ref 绑定要解决、预填可见性要验；server 插件**所有核心行为已验证**(V1 appendPrompt+submitPrompt ✅ / V4 多session路由 ✅ / V7 showToast ✅)。
3. **`client.tui.*` 在 TUI 模式进程内可达**（§2.2），不是「server 插件够不着 TUI」。
4. v1 用不上 TUI 插件独有的 widget/slash 能力（`说明-接收端.md §八` 明确不做 widget/slash/pending）。

**回退预案**：若 §八 验证发现 `appendPrompt+submitPrompt` 在 TUI 模式下不能触发 LLM（极不可能，但要验），退回 TUI 插件方案（调研 §7.1 骨架仍有效）。两方案**协议层零差异**（同样的 socket/注册表/报文），只是 delivery 落点不同，切换成本仅限插件内部 ~30 行。

---

## 三、系统设计

### 3.1 模块结构

```
aibp-agents/opencode/
├── package.json     # npm 包自描述（aibp.protocol 单一事实来源）
├── index.ts         # 接收端实现（全部逻辑，~120 行）
└── README.md        # 安装与使用
```

**与 aibp-pi 完全对称**的布局。无编译产物（Bun 直接跑 `.ts`，见 §6.1）。

### 3.2 协议常量（与 aibp-pi 逐字一致）

协议版本单一事实来源 = `package.json` 的 `aibp.protocol`（`说明-AIBP §7.1.1`）：

```typescript
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const pkg = JSON.parse(fs.readFileSync(path.join(__dirname, "package.json"), "utf8"));
const PROTOCOL = pkg.aibp.protocol;                       // "aibp-1"，写注册表
const PROTOCOL_MAJOR = Number(PROTOCOL.split("-").pop()); // 1，校验信封
```

不硬编码 `"aibp-1"`——静态检测（D17 ensure 读 package.json）与运行时（写注册表）同源。

### 3.3 `registryDir()`（与 Go 端 / aibp-pi 逐字符一致）

```typescript
function registryDir(): string {
  const override = process.env.MNAB_REG_DIR;
  if (override) return override;
  const base = process.env.XDG_RUNTIME_DIR || process.env.TMPDIR || "/tmp";
  return path.join(base, `microneo-agent-bridge-${process.getuid?.() ?? 0}`);
}
```

**必须与 `internal/aibp/registry.go:RegistryDir()` 和 `aibp-agents/pi/index.ts:registryDir()` 三端逐字符一致**（`说明-AIBP §3.1` 实现约束）。pi 与 opencode 共用同一注册表目录，故名字池协调靠扫描同一目录（§3.5）。

### 3.4 生命周期：插件函数体顶层 = 「session_start」

**opencode server 插件没有 per-session 的 start/stop 钩子**——插件函数在 opencode 启动时**加载一次**，返回 Hooks 后常驻。opencode（TUI 模式）是单进程，server 插件即随进程常驻。这正好对应「一个 opencode 进程 = 一个接收端」：

```typescript
export default async function plugin(input: PluginInput) {
  const client = input.client;
  const directory = input.directory;

  // —— 等价于 aibp-pi 的 session_start 主体 ——
  const names = loadNamePool(client);
  if (names === null) return {};                     // 池子文件坏，notify 已发，放弃注册

  let server: net.Server | null = null;
  let name = "", socketPath = "", regFile = "";

  const got = await allocateName(names, (conn) => {  // connectionHandler 闭包
    let buf = "";
    conn.on("data", (chunk) => {
      buf += chunk.toString("utf8");
      let nl;
      while ((nl = buf.indexOf("\n")) >= 0) {
        handleLine(buf.slice(0, nl), client);
        buf = buf.slice(nl + 1);
      }
    });
  }, (s) => { server = s; });                        // listen 抢锁成功的 server 回交外层
  if (got === null) {
    await toast(client, "⚠ aibp 名字池已满，本次不接收消息", "warning");
    return {};
  }
  name = got.name; socketPath = got.socketPath;

  regFile = path.join(registryDir(), `ai-${name}.json`);
  fs.writeFileSync(regFile, JSON.stringify({
    name, pid: process.pid, transport: "unix",
    socket: socketPath, protocol: PROTOCOL,
    startedAt: Math.floor(Date.now() / 1000), cwd: directory, labels: ["default"],
  }));

  // —— 退出清理（best-effort；兜底靠发送端 GC，说明-AIBP §3.5/§九）——
  const cleanup = () => {
    try { server?.close(); } catch {}
    try { fs.unlinkSync(regFile); } catch {}
    try { fs.unlinkSync(socketPath); } catch {}
  };
  process.once("beforeExit", cleanup);
  process.once("exit", cleanup);

  function handleLine(line: string, client: any) { /* 见 §3.6 */ }

  return {};   // v1 不需要任何 Hook
}
```

**与 aibp-pi 的差异**：
- aibp-pi 在 `pi.on("session_start")` 里跑——pi 每 `/new` 一个 session 重注册。
- opencode 在插件函数顶层跑一次——一个 opencode 进程一个接收端（多 session/tab 共用同一 socket）。**符合「接收端 = agent 进程」语义**：microNeo 推上下文给「这个 opencode 进程」，由 `appendPrompt` 路由到用户当前看的活动会话。

**清理**：opencode 无显式 shutdown 钩子。用 `process.once("beforeExit"/"exit")` best-effort 清理；进程被 kill -9 时清不掉，靠**发送端 GC**（`说明-AIBP §3.5`：connect 失败 + PID 死 → 删注册文件）兜底——协议本就为此鲁棒设计。

### 3.5 名字池（D11，与 aibp-pi 逐字一致）

`loadNamePool` / `normalizeNames` / `allocateName` 三个函数**从 `aibp-agents/pi/index.ts` 整段复制**，一个字符不改：

- 池子文件 `$XDG_CONFIG_HOME/aibp/aibp-names.json`（fallback `~/.config/aibp/`）——**pi 与 opencode 共用同一文件**。
- 三步规范化（截断 10 → 保留首现去重 → 过滤 `/ \0 : 空格 -`）。
- `allocateName`：listen 原子抢锁 + occupied 集合（扫 `ai-*.json` + `process.kill(pid,0)` 探活 + 顺手 GC 僵尸）+ connect 试探区分真活/僵尸。

**协调保证**：pi 与 opencode 都扫同一 `registryDir`、用同一池子文件，故 pi 占了 `Alpha` 时 opencode 启动自动拿 `Bravo`，永不撞名。这是复用 D11 机制的核心收益——**无需为多 agent 协调新发明任何东西**。

> **唯一适配点**：aibp-pi 的 `loadNamePool`/`allocateName` 里用 `ctx.ui.notify` 报错；opencode 没有 `ctx.ui`，改用 `client.tui.showToast`。把「报错回调」抽成参数传入，或直接在函数内 `await client.tui.showToast(...)`（client 经闭包可见）。见 §五代码骨架。

### 3.6 报文解析（与 aibp-pi 一致）

```typescript
function handleLine(line: string, client: any) {
  let env: any;
  try { env = JSON.parse(line); } catch { return; }
  if (env.v !== PROTOCOL_MAJOR || env.type !== "context") return;   // 静默忽略
  onMessage(env, client);
}
```

校验规则逐字同 aibp-pi（`说明-接收端.md §5.2`）：JSON 解析失败 / 主版本不符 / 未知 type → 静默 return（前向兼容，不告警）。

### 3.7 递送语义（**唯一与 aibp-pi 实质不同的部分**）

```typescript
async function onMessage(env: any, client: any) {
  const p = env.payload;
  const text = formatText(p);                // 与 aibp-pi 逐字一致（§3.8）

  if (p.message) {
    // 带 message → 填输入框 + 提交（≈ aibp-pi sendUserMessage）
    await client.tui.clearPrompt();
    await client.tui.appendPrompt({ body: { text } });
    await client.tui.submitPrompt();
    await toast(client, "microNeo: 已发起对话", "info");
  } else {
    // 纯上下文 → 仅填输入框，不提交（≈ aibp-pi setEditorText）
    await client.tui.clearPrompt();
    await client.tui.appendPrompt({ body: { text } });
    await toast(client, "📌 已放入输入框，可编辑后发送", "info");
  }
}

async function toast(client: any, message: string, variant: "info"|"warning" = "info") {
  try { await client.tui.showToast({ body: { message, variant } }); } catch {}
}
```

**两条路径**（`说明-AIBP §六`）：

| `p.message` | 递送 | 用户体验 | 对应 aibp-pi |
|-------------|------|---------|-------------|
| 有 | `clearPrompt` + `appendPrompt` + **`submitPrompt`** | 触发 LLM 对话 | `sendUserMessage(text,{deliverAs:"steer"})` |
| 无 | `clearPrompt` + `appendPrompt`（**不** submit） | 预填输入框，用户编辑后手发 | `setEditorText(text)` |

**为什么 `clearPrompt` 再 `appendPrompt`**：`appendPrompt` 语义是「追加」（`/tui/append-prompt`，body `{text}`）。aibp-pi 的 `setEditorText` 是「替换」。为行为对齐（纯上下文时用户看到的就是推送内容，不与已输入文字混在一起），先 `clearPrompt` 清空再 append。「带 message」路径因立即 submit，clear 与否影响不大，但统一先 clear 更干净。

> **streaming 冲突**（`说明-AIBP §6.3`）：若 agent 正在回复，`submitPrompt` 的行为由 opencode 自决（很可能等同用户在 streaming 时按回车 = 排队 follow-up，opencode 原生处理）。v1 单向不传 receiver 状态，冲突识别与排队全由接收端吸收——这正是协议原则③。**待验证**（§八 V2）。

### 3.8 `formatText`（与 aibp-pi 逐字一致）

```typescript
function formatText(p: any): string {
  const sel = p.selection;
  const selText = sel?.text && sel.text.length > 0 ? sel.text : "";
  if (sel && selText) {
    const header = `<selection: ${p.path} lines ${sel.start.line}-${sel.end.line}>`;
    return p.message ? `${header}\n\n${selText}\n\n<user input>\n\n${p.message}` : `${header}\n\n${selText}`;
  }
  const focus = sel ? `line${sel.start.line}-line${sel.end.line}` : `${p.cursor.line}`;
  const base = `@${p.path} :line${focus}`;
  return p.message ? `${base}\n\n${p.message}` : base;
}
```

**整段复制自 `aibp-agents/pi/index.ts`**。`formatText` 是 agent 无关的纯文本格式化（`说明-接收端.md §七` D10 决策 4：纯内联 + 尖括号标签，不放 `@` 引用以免触发 LLM 读文件双倍消耗）。opencode 的 LLM 同样认 `@path :lineX` 和 `<selection>` 标签（通用 LLM 约定，非 pi 专属）。

---

## 四、代码复用策略：复制 vs 抽 core

调研 §7.3 建议「把接收端通用逻辑抽成纯 TS 模块，pi/opencode 各包一层 adapter」。但 aibp-pi 已发布（`aibp-pi@1.0.1`）且是单文件 inline。

| 方案 | 优点 | 缺点 |
|------|------|------|
| **A. 复制（推荐 v1）** | aibp-pi 不动、各自独立发包、零耦合；协议 aibp-1 已稳定，漂移风险低 | ~80 行重复（registryDir/名字池/formatText/分帧/版本校验） |
| B. 抽 `aibp-core` 包 | DRY、单一来源 | 需重发 aibp-pi；两包版本协调；D17 ensure 读各 agent 自己的 package.json，core 抽出后协议字段归属变复杂 |

**v1 选 A（复制）**，理由：
1. **「模仿 aibp-pi」= 同构单文件**，复制最贴合。
2. AGENTS.md 精神「侵入越小越好」——不动已上线且工作的 aibp-pi。
3. 协议 aibp-1 稳定（主版本不变=兼容），重复代码漂移风险低；真到 aibp-2 再抽 core 不迟。
4. 复用的 5 块逻辑（registryDir/名字池/formatText/分帧/版本校验）各自独立、边界清晰，复制不引入隐晦耦合。

> **未来**：若接第三个 agent（claude）时重复达 3 份，再抽 `aibp-core`，三端统一依赖。届时 aibp-pi 跟随升级一次。

---

## 五、`index.ts` 完整骨架

> 标「⟨复制 aibp-pi⟩」的函数从 `aibp-agents/pi/index.ts` 整段搬，仅把 `ctx.ui.notify` 换成 `client.tui.showToast`。

```typescript
import type { PluginInput } from "@opencode-ai/plugin";   // 仅类型，Bun 运行时擦除
import * as net from "node:net";
import * as fs from "node:fs";
import * as path from "node:path";
import * as os from "node:os";
import { fileURLToPath } from "node:url";

// —— 协议常量（单一事实来源：package.json）——
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const pkg = JSON.parse(fs.readFileSync(path.join(__dirname, "package.json"), "utf8"));
const PROTOCOL = pkg.aibp.protocol;
const PROTOCOL_MAJOR = Number(PROTOCOL.split("-").pop());

const DEFAULT_NAMES_STR =
  "Alpha Bravo Charlie Delta Echo Foxtrot Golf Hotel India Juliet Kilo Lima Mike November Oscar";

export default async function plugin(input: PluginInput) {
  const client = input.client;
  const directory = input.directory;

  let server: net.Server | null = null;
  let name = "", socketPath = "", regFile = "";

  const names = loadNamePool(client);
  if (names === null) return {};                        // 池子坏，已 toast，放弃

  const connectionHandler = (conn: net.Socket) => {
    let buf = "";
    conn.on("data", (chunk) => {
      buf += chunk.toString("utf8");
      let nl;
      while ((nl = buf.indexOf("\n")) >= 0) {
        handleLine(buf.slice(0, nl), client);
        buf = buf.slice(nl + 1);
      }
    });
  };

  const got = await allocateName(names, connectionHandler, (s) => { server = s; });
  if (got === null) {
    await toast(client, "⚠ aibp 名字池已满，本次不接收消息", "warning");
    return {};
  }
  name = got.name; socketPath = got.socketPath;

  regFile = path.join(registryDir(), `ai-${name}.json`);
  fs.writeFileSync(regFile, JSON.stringify({
    name, pid: process.pid, transport: "unix",
    socket: socketPath, protocol: PROTOCOL,
    startedAt: Math.floor(Date.now() / 1000), cwd: directory, labels: ["default"],
  }));

  const cleanup = () => {
    try { server?.close(); } catch {}
    try { fs.unlinkSync(regFile); } catch {}
    try { fs.unlinkSync(socketPath); } catch {}
  };
  process.once("beforeExit", cleanup);
  process.once("exit", cleanup);

  // —— 报文处理（闭包捕获 client）——
  function handleLine(line: string, client: any) {
    let env: any;
    try { env = JSON.parse(line); } catch { return; }
    if (env.v !== PROTOCOL_MAJOR || env.type !== "context") return;
    void onMessage(env, client);                        // 不阻塞分帧
  }

  return {};                                            // v1 无 Hook
}

// —— 递送（唯一与 aibp-pi 实质不同处）——
async function onMessage(env: any, client: any) {
  const p = env.payload;
  const text = formatText(p);
  try {
    if (p.message) {
      await client.tui.clearPrompt();
      await client.tui.appendPrompt({ body: { text } });
      await client.tui.submitPrompt();
      await toast(client, "microNeo: 已发起对话", "info");
    } else {
      await client.tui.clearPrompt();
      await client.tui.appendPrompt({ body: { text } });
      await toast(client, "📌 已放入输入框，可编辑后发送", "info");
    }
  } catch (e) {
    await toast(client, `⚠ aibp 递送失败: ${(e as Error).message}`, "warning");
  }
}

async function toast(client: any, message: string, variant: "info" | "warning" = "info") {
  try { await client.tui.showToast({ body: { message, variant } }); } catch {}
}

// ===== 以下 ⟨复制 aibp-pi⟩，仅 notify→toast =====
function registryDir(): string { /* §3.3，逐字同 aibp-pi */ }
function normalizeNames(raw: string[]): string[] { /* ⟨复制⟩ */ }
function loadNamePool(client: any): string[] | null { /* ⟨复制⟩，notify→toast */ }
function allocateName(
  names: string[],
  handler: (conn: net.Socket) => void,
  onListen: (s: net.Server) => void,
): Promise<{ name: string; socketPath: string } | null> { /* ⟨复制⟩，server 经 onListen 回交 */ }
function formatText(p: any): string { /* §3.8，逐字同 aibp-pi */ }
```

**与 aibp-pi 的结构差异小结**：
1. `pi.on("session_start", …)` → 插件函数顶层（无 per-session 钩子）。
2. `allocateName` 的 server 赋值：aibp-pi 靠闭包直赋外层 `server`；这里因 `allocateName` 提到模块级会丢失闭包，改用 `onListen` 回调回交（或把 `allocateName` 定义在 plugin 函数内闭包直赋——二选一，实现时择简）。
3. delivery：`sendUserMessage`/`setEditorText` → `client.tui.*`。
4. 清理：`session_shutdown` → `process.once("beforeExit"/"exit")` + 发送端 GC 兜底。

---

## 六、`package.json` 与发布

### 6.1 package.json

```json
{
  "name": "aibp-opencode",
  "version": "0.1.0",
  "aibp": { "protocol": "aibp-1" },
  "description": "AIBP (AI Bridge Protocol) receiver plugin for opencode",
  "license": "MIT",
  "type": "module",
  "main": "./index.ts",
  "exports": { ".": "./index.ts" },
  "keywords": ["opencode", "opencode-plugin", "microNeo", "aibp"],
  "files": ["index.ts", "README.md"],
  "devDependencies": {
    "@opencode-ai/plugin": "^1.4.10",
    "@opencode-ai/sdk": "^1.4.10"
  }
}
```

**字段决策**：

| 字段 | 值 | 理由 |
|------|---|------|
| `aibp.protocol` | `"aibp-1"` | 协议单一事实来源（`说明-AIBP §7.1.1`）；D17 ensure 与运行时同源 |
| `type` | `"module"` | ESM；opencode 经 Bun 加载 ESM/TS |
| `main`/`exports` | `./index.ts` | opencode 按 `main`/`exports` 解析插件入口；Bun 原生跑 `.ts`，**无需预编译**（与 aibp-pi 一致，纯 `node:*` 无重依赖） |
| `devDeps` | plugin/sdk | 仅 `import type` 用，Bun 运行时擦除、不强制安装；声明为 devDep 供编辑器/CI 类型检查。opencode 安装本包到 `~/.cache/opencode/node_modules/`，旁边即有 `@opencode-ai/plugin`，类型可解析 |
| `keywords` | 去 `pi-package` | 那是 pi 专属；opencode 用 `opencode-plugin` |
| `files` | `index.ts`,`README.md` | 单文件，无 dist |

> **无 `pi.extensions` 字段**：那是 pi 专属加载声明。opencode 靠 `opencode.json` 的 `plugin` 数组 + 包的 `main` 加载，无需特殊 manifest。

> **为何不发预编译 JS**：aibp-pi 发 `.ts`（pi jiti 跑）；opencode Bun 原生跑 `.ts`。两端都无需 build 步骤，发布 = `npm publish` 源码。若日后加重依赖（如抽 core），再加 `bun build` 出 dist。

### 6.2 加载方式（两种，opencode 原生）

1. **npm 包**（推荐，发布后）：在 `opencode.json` 声明
   ```json
   { "plugin": ["aibp-opencode"] }
   ```
   或命令装：`opencode plugin aibp-opencode -g`（`-g` 写全局 `~/.config/opencode/opencode.json`）。opencode 启动时用 Bun 自动 `npm install` 到 `~/.cache/opencode/node_modules/`。
2. **本地文件**（开发期）：丢 `~/.config/opencode/plugins/aibp-opencode.ts`，启动自动加载，改完重启。

### 6.3 模块导出检测（server vs tui）

opencode 解析插件模块的导出形态（`@opencode-ai/plugin/dist/index.d.ts`）：
- 导出 `{ tui: fn }` → TUI 插件；
- 导出 `{ server: fn }` 或 **bare default/命名函数** → server 插件。

`opencode-browser` 用 `export default plugin`（bare default）即被识别为 server 插件。**本包用 `export default async function plugin(input) {...}`**，同形态，零歧义。

---

## 七、衔接 D17：`ensure_opencode.go`（未来）

本包发布后，microNeo 的 `:check-agent` 应能像检测 aibp-pi 那样检测/安装 aibp-opencode。这是 D17 §附的 `OpencodeEnsurer`，**本文只给契约，不实现**（属 D17 后续）：

```go
// internal/aibp/ensure_agents/ensure_opencode.go（未来）
type OpencodeEnsurer struct{}
var _ AgentEnsurer = OpencodeEnsurer{}

func (OpencodeEnsurer) AgentName() string { return "opencode" }
func (OpencodeEnsurer) HasAgent() bool    { _, err := exec.LookPath("opencode"); return err == nil }

// HasAIBP：读 ~/.config/opencode/opencode.json 的 plugin 数组是否含 "aibp-opencode"
func (OpencodeEnsurer) HasAIBP() bool { /* 解析 opencode.json plugin[] */ }

// AIBPVersion：读 ~/.cache/opencode/node_modules/aibp-opencode/package.json 的 aibp.protocol
func (OpencodeEnsurer) AIBPVersion() (string, error) { /* 同 D17 §6.2① AIBPVersion 模式 */ }

// InstallAIBP：opencode plugin aibp-opencode -g  （-g 写全局配置）
func (OpencodeEnsurer) InstallAIBP() error { /* exec.Command("opencode","plugin","aibp-opencode","-g") */ }
```

**关键路径**（实现时镜像 opencode 实际行为，以源码/文档为准）：

| 项 | 路径 / 命令 | 依据 |
|----|------------|------|
| opencode 全局配置 | `~/.config/opencode/opencode.json` | 用户实际文件在此；opencode 标准全局配置位（可能支持 `$XDG_CONFIG_HOME/opencode`，实现时确认） |
| 已装插件 package.json | `~/.cache/opencode/node_modules/aibp-opencode/package.json` | 官方文档：「Packages cached in `~/.cache/opencode/node_modules/`」 |
| 安装命令 | `opencode plugin aibp-opencode -g` | `opencode plugin --help`：`-g/--global install in global config` |
| 配置登记形态 | `plugin` 数组追加 `"aibp-opencode"` | 同 `@different-ai/opencode-browser` 现状 |

**与 D17 的关系**：D17 的 `Ensure()` 编排逻辑（HasAgent → HasAIBP → AIBPVersion → InstallAIBP）完全复用，`OpencodeEnsurer` 只是第四个实现。`command_neo.go` 加一个 `:check-agent-opencode` 或把 `:check-agent` 扩成遍历所有 ensurer（D17 §附 方案 A/B）。

---

## 八、验证状态

> 协议层零风险（与 aibp-pi 同协议，已端到端验证）。所有核心行为已验证通过。

| # | 待验证 | 状态 | 怎么验 | 结果 | 失败 fallback |
|---|--------|------|--------|------|--------------|
| V1 | **`appendPrompt + submitPrompt` 是否等同「用户回车」触发 LLM**（核心） | ✅ **已通过** | 写最小 server 插件，appendPrompt 一段文本后 submitPrompt，看 opencode 是否开腔回复 | 成功触发 LLM 回复,消息落进当前活动会话 | 退 TUI 插件 `ref.set+submit`（调研 §7.1） |
| V2 | **streaming 中 submitPrompt 行为**（是否排队/报错） | ⏸️ 待验证 | 在 agent 回复过程中触发 submitPrompt | 若报错，catch 后 toast「agent 忙，稍后再发」；若排队则符合预期 | - |
| V3 | **`clearPrompt + appendPrompt` 是否真能「替换」输入框** | ⏸️ 待验证 | 设输入框有字，clear+append，看是否只剩新内容 | 若 append 是追加语义，纯上下文路径可接受追加（或改用 `set` 等价组合，实现时确认） | - |
| V4 | **多 session 时 appendPrompt 路由到哪个会话** | ✅ **已通过** | 开两个 session，从 microNeo 发，看落点 | 落进当前活动会话(ID不变),符合预期 | 需补 session 定位（调研 §8 #4，但 server 插件大概率无需） |
| V5 | **`.ts` 直接被 opencode Bun 加载**（无需预编译） | ✅ **已通过** | 发包后 `opencode plugin aibp-opencode -g` + 重启，看插件是否加载、socket 是否起 | Bun 直接加载 `.ts` 成功 | 改发预编译 JS（加 `bun build` 出 dist，改 `main`） |
| V6 | **`process.once("exit")` 清理在 opencode 退出时是否触发** | ⏸️ 待验证 | 启动后 `kill` opencode，查注册文件是否删 | 不触发也无妨——发送端 GC（§3.4）兜底 | - |
| V7 | **`client.tui.showToast()` 是否可用** | ✅ **已通过** | 在 event 回调里调用 showToast,看是否弹出提示 | 成功弹出 toast,触发 `tui.toast.show` 事件 | 不影响核心功能,可作为可选的 UI 反馈 |

**V1 是命脉**(已通过)。其余都是边缘行为/可选清理。全部不构成协议层改动。详细验证报告见 `V1-验证完成报告.md`。

**V1 是命脉**（类比调研 §8 #3 之于 TUI 方案）。其余都是边缘行为/可选清理。全部不构成协议层改动。

---

## 九、实施计划

### Phase 0 — 前置验证 ✅ **已完成**

已通过探针插件验证:
- V1 ✅ `appendPrompt+submitPrompt` 成功触发 LLM
- V4 ✅ 多 session 路由正确,落进当前活动会话
- V5 ✅ `.ts` 被 Bun 直接加载
- V7 ✅ `showToast()` 可用,可提供 UI 反馈

详细报告:`V1-验证完成报告.md`

**Gate**:已通过。可直接进入 Phase 1 实现阶段。

### Phase 1 — 实现 aibp-opencode 包

| 改动 | 文件 |
|------|------|
| 实现接收端（§五骨架） | `aibp-agents/opencode/index.ts` |
| 包描述（§6.1） | `aibp-agents/opencode/package.json` |
| 安装/使用说明 | `aibp-agents/opencode/README.md` |

**Gate**：本地 `opencode plugin ./aibp-agents/opencode -g`（或文件式丢 plugins/）加载成功；`ls $XDG_RUNTIME_DIR/microneo-agent-bridge-*/` 见 `ai-Bravo.json`（假设 pi 已占 Alpha）；microNeo Alt-Enter 发消息，opencode 开腔回复（带 message）/输入框被填（纯上下文）。

### Phase 2 — 发布 npm

```bash
cd aibp-agents/opencode
npm publish --access public     # → aibp-opencode@0.1.0
```

**Gate**：`opencode plugin aibp-opencode -g`（从 npm 装）成功；行为同 Phase 1。

### Phase 3 — 端到端 + 与 aibp-pi 共存

- pi（Alpha）+ opencode（Bravo）同开 → microNeo 发 → 两个都收到（广播）。
- D12 多 receiver 选择（已有）能列出 Alpha/Bravo 两个目标。
- §八 V2–V6 逐项验。

### Phase 4（未来，本文不含） — `ensure_opencode.go`

D17 §附：实现 `OpencodeEnsurer` + `:check-agent` 扩展，让 microNeo 自动检测/安装 aibp-opencode。

---

## 十、风险与已知限制

| # | 风险 | 触发 | 应对 |
|---|------|------|------|
| R1 | **V1 不通过**（appendPrompt+submit 不触发 LLM） | 极不可能（SDK 契约 + §5.1 in-process） | §八 fallback：退 TUI 插件，协议层零改 |
| R2 | **streaming 时递送丢失/报错** | agent 正在回复时 microNeo 发 | V2 验证；catch 后 toast 提示（不丢数据，用户重发） |
| R3 | **opencode 配置目录非标准**（`$XDG_CONFIG_HOME` 自定义） | ensure_opencode.go 阶段 | 实现时镜像 opencode 配置解析（§七） |
| R4 | **名字池与 pi 协调依赖文件系统扫描** | 极端并发（pi 与 opencode 同毫秒启动抢同名） | D11 的 listen 原子抢锁已覆盖：EADDRINUSE → 换下一个 |
| R5 | **opencode 版本升级改 `client.tui.*` 契约** | opencode 大版本变 | 锁 `devDependencies` 的 plugin/sdk 版本；运行时 `client.tui` 来自 opencode 自身（随其升级），需关注 breaking change |

**回滚**：aibp-opencode 是独立 npm 包，microNeo 零改动。失败只需 `opencode plugin remove`（或从 opencode.json 删条目）+ `npm unpublish`。

---

## 附：与既有文档的关系

| 文档 | 关系 |
|------|------|
| `opencode调研.md` | §6.5/§6.6 结论「用 TUI 插件」**被本文 §二重估推翻**——改用 server 插件（`client.tui.appendPrompt`）。调研 §1-§5（机制事实、§5.1 单进程实测）仍完全有效，是本文的事实基础 |
| `说明-AIBP.md` | 协议权威。本包是其在 opencode 上的第四层实现，协议层零改动 |
| `说明-接收端.md` | pi 接收端实现。本包大量 ⟨复制⟩ 其代码（registryDir/名字池/formatText/分帧） |
| `D17-初始化aibp-pi.md` | §附 `OpencodeEnsurer` = 本包的 microNeo 侧自举（§七衔接，未来实现） |
| `说明-架构设计.md` | §八 Phase 3「opencode 接收端」= 本文档落地 |
