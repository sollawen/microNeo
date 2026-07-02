# aibp-claude 实施方案（Monitor 方案）

> **前置**：本文基于 `aibp-claude-方案选型.md`(v3) 的结论——用 **plugin Monitor** 把 microNeo 的上下文+消息递送给 Claude。第三方 LLM（ccmm）用户也可用，不受第一方登录墙限制。
>
> **范围（v1）**：**只做 §6.3（带消息 → 立即触发）**，对齐 aibp-opencode 的"总是触发"行为。§6.4（纯上下文 → 不触发）留待后续——opencode 也没做。
>
> **本文产出**：可直接照着写代码的实施方案。含文件清单、代码骨架、配置、激活方式、验证计划、风险项。

---

## 1. 方案总览

### 1.1 单路递送（单 daemon）

```
            aibp socket daemon（= plugin monitor 的 command）
            监听 unix socket，复用现有协议层
                      │
             收到 envelope，校验 v/type
                      │
               formatText → JSON 单行
                      │
                stdout（Monitor 事件流）
                      │
            Claude 收到事件 → 立即插话响应 ✅
               （Monitor 机制，§6.3）
```

v1 对所有进来的报文都触发（与 opencode 行为一致）。每条 aibp 报文 → 一个 stdout 事件 → Claude 响应一次。

### 1.2 激活模型（与 pi/opencode 对齐）

用户运行 `ccmm`（= claude + env）→ claude 经 `--plugin-dir` 加载 aibp-claude 插件 → 插件声明的 monitor **自动 spawn** 为子进程（= daemon）→ daemon 注册 aibp socket → microNeo 发现并连接。**一条命令全自动**，与 pi/opencode 体验一致。

### 1.3 生命周期

| 事件 | 动作 |
|------|------|
| claude session 起 | 插件激活 → monitor(daemon) 自动 spawn → allocateName → 写 registry |
| microNeo 发消息 | daemon → stdout 事件 → Claude 立即响应 |
| claude session 结束 | claude 杀 monitor 子进程 → `process.on('exit')` 清理 registry+socket |

daemon 是 claude 管理的子进程，session 级。无 claude 运行时无需递送（正确行为）。

---

## 2. 文件改动清单

**开发期**（monitor 验证通过前，保留 channel 作对照）：
```
claude/
├── index.ts              【新增】主 daemon：协议层复用 + stdout 事件（主路径）
├── aibp-channel.ts        【暂留】旧 channel 方案，开发期作对照参考
├── .claude-plugin/
│   └── plugin.json        【改】去 channels 声明，加 experimental.monitors
├── .mcp.json              【暂留】旧 channel 的 MCP server 定义；monitor 主路径不读它
├── package.json           【改】bin 指向 index.ts
└── README.md              【改】更新激活/使用说明
```

**完成期**（monitor 验证通过后，删掉整条 channel 链路）：
```
claude/
├── index.ts              # 主 daemon，零运行时外部依赖（仅 node:*）
├── .claude-plugin/
│   └── plugin.json        # 只声明 monitor
├── package.json           # 去掉 @modelcontextprotocol/sdk 依赖
└── README.md
```
删除项：`aibp-channel.ts` + `.mcp.json` + `package.json` 的 `@modelcontextprotocol/sdk` 依赖。

> v1 **不需要 hooks 目录**（§6.4 暂不做，无 pending 文件机制）。
>
> **为何分两阶段**：开发期留 channel 作对照，万一 monitor 走不通能快速回退；monitor 验证通过后整条 channel 链路（channel 代码 + MCP server 定义 + MCP SDK 依赖）一并删除，主程序成为零外部依赖的纯 socket daemon。

---

## 3. 核心代码：index.ts（daemon）

> 基于 `aibp-channel.ts` 改造。**协议层（normalizeNames/loadNamePool/registryDir/allocateName/formatText）逐字复用，零改动**。下面只标注【复用】【删除】【新增】【改动】的差异。

### 3.1 头部【复用 + 小改】

```typescript
import * as net from "node:net"
import * as fs from "node:fs"
import * as path from "node:path"
import * as os from "node:os"
import { fileURLToPath } from "node:url"
// 【删除】不再 import @modelcontextprotocol/sdk —— monitor 模式不走 MCP

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const pkg = JSON.parse(fs.readFileSync(path.join(__dirname, "package.json"), "utf8"))
const PROTOCOL = pkg.aibp.protocol
const PROTOCOL_MAJOR = Number(PROTOCOL.split("-").pop())

// 【复用】诊断日志（写文件，绝不污染 stdout）
const LOG_FILE = process.env.MNAB_LOG || "/tmp/aibp-claude.log"
// ⚠ 关键约束升级：stdout 现在专用于 Monitor 事件流（每行=一个事件），
//   严禁 console.log 污染 stdout，否则会被 claude 当成伪事件。诊断一律写文件。
let LOG_TAG = "boot"
function log(message: string, data?: unknown) { /* 与现有一致，写文件 */ }

const DEFAULT_NAMES_STR = "Alpha Bravo ... Oscar"  // 【复用】
```

### 3.2 协议层【完全复用】

`normalizeNames` / `loadNamePool` / `registryDir` / `allocateName` / `formatText` —— **从 aibp-channel.ts 逐字搬过来，一个字不改**。这是与 aibp-pi/opencode 对齐的协议层。

### 3.3 stdout 事件格式【新增·关键设计】

> Monitor 的 command source 按 **stdout 行（\n 分隔）** 分事件，每行触发一次 claude 响应。formatText 的输出是多行的（选区代码+消息），直接 stdout 会被拆成 N 个事件（灾难）。**必须压成单行**。

**决策：JSON 单行编码。** `JSON.stringify` 自动转义内部换行，天然单行；claude(LLM) 能直接读懂 JSON 字段。

```typescript
// 收到报文 → formatText → JSON 单行 → stdout。v1 对每条报文都触发（对齐 opencode）。
function emitMonitorEvent(p: any) {
  const text = formatText(p)          // 复用：selection 内联 / @ 引用 + message
  const event = {
    type: "aibp-context",             // 固定标记，让 claude 识别来源（aibp 报文）
    content: text,                    // formatText 完整结果：path/lines 已内含在 content 里，不冗余重复
  }
  // ⚠ 单行（JSON.stringify 不产生内部换行）+ 末尾 \n 作为事件分隔。
  //   process.stdout.write 对管道是同步写入，无需显式 flush（Lisa review #3）。
  process.stdout.write(JSON.stringify(event) + "\n")
  log("emitted monitor event", { path: p.path, hasMsg: !!p.message })
}
```

> **替代考量**：也可不套 content，直接 `JSON.stringify(p)`（payload 原样）。但 formatText 已封装了"内联 vs @引用"的选区处理逻辑，复用它更省事，且与 pi/opencode 的呈现一致。

### 3.4 报文处理【改动·核心】

```typescript
// 【改动】onMessage：从"调 mcp.notification"改为"发 stdout 事件"
//   v1 对齐 opencode：不分支，所有报文都触发。
function onMessage(env: any) {
  const p = env.payload
  log("onMessage", { path: p?.path, hasMessage: !!p?.message })
  emitMonitorEvent(p)
}
```

> **与 opencode 一致**：opencode 的 `onMessage` 也没有 `if (p.message)` 分支，总是 `session.prompt`（总是触发）。这里总是 `emitMonitorEvent`（总是触发）。行为等价。
>
> **未来 §6.4 扩展点**：若以后要做"无消息不触发"，在这里加 `if (p.message)` 分支，无 message 那路走 UserPromptSubmit hook + pending 文件（见本文档 git 历史里的旧版 §5）。

### 3.5 启动/监听/清理【复用 + 删除 MCP】

```typescript
// 【复用】loadNamePool / allocateName / connectionHandler / handleLine / 写 registry
//   —— 与 aibp-channel.ts 的"段 C"完全一致，搬过来即可。
//   唯一区别：handleLine → onMessage（见 3.4，不再调 mcp.notification）

// ... allocateName 成功后写 registry（agent 字段填 "claude"）...

// 【删除】末尾不再有：
//   const mcp = new Server(...)
//   await mcp.connect(new StdioServerTransport())
// monitor daemon 不需要 MCP 协议，stdout 直接是事件流。

// 【复用】信号/退出清理（SIGPIPE/SIGTERM/SIGINT/exit → 删 registry + socket）
```

### 3.6 完整骨架对照（差异高亮）

```
aibp-channel.ts (现有·对照)        index.ts (新·主路径)
─────────────────────────           ─────────────────────────
import net/fs/path/os/url            import net/fs/path/os/url          【同】
import MCP Server/Transport          (删除)                            【删】
PROTOCOL/PROTOCOL_MAJOR              PROTOCOL/PROTOCOL_MAJOR           【同】
log() 写文件                          log() 写文件                       【同】
normalizeNames                       normalizeNames                    【同·复用】
loadNamePool                         loadNamePool                      【同·复用】
registryDir                          registryDir                       【同·复用】
allocateName                         allocateName                      【同·复用】
formatText                           formatText                        【同·复用】
const mcp = new Server(...)          (删除)                            【删】
names = loadNamePool(...)            names = loadNamePool(...)         【同】
connectionHandler                    connectionHandler                 【同】
allocateName + 写registry            allocateName + 写registry         【同】
handleLine → onMessage               handleLine → onMessage            【同壳】
onMessage: mcp.notification          onMessage: emitMonitorEvent       【改·3.4】
                                    + emitMonitorEvent (stdout)        【新·3.3】
mcp.connect(StdioServerTransport)    (删除)                            【删】
信号/exit 清理                        信号/exit 清理                     【同】
```

**结论：协议层 100% 复用，只把递送末端从 `mcp.notification` 换成 `stdout 事件`，删 MCP 依赖启动。改动量小、风险低。**

---

## 4. plugin.json 改造

```jsonc
{
  "name": "aibp-claude",
  "version": "1.0.0",
  "description": "AIBP receiver plugin for Claude Code — receive editor context from microNeo",
  "license": "MIT",
  "keywords": ["claude-plugin", "microNeo", "aibp"],

  // 【删除】"channels": [{ "server": "aibp-channel" }] —— monitor 主路径不需要。
  //   开发期 aibp-channel.ts 留在仓库作对照；monitor 验证通过后整条 channel
  //   链路（含本字段、.mcp.json、channel.ts、MCP SDK 依赖）一并删除（见§2 完成期）。

  // 【新增】monitor 主路径：插件激活即自动 spawn daemon
  "experimental": {
    "monitors": [
      {
        "name": "aibp-daemon",
        "command": "bun \"${CLAUDE_PLUGIN_ROOT}/index.ts\"",
        "description": "microNeo editor context & messages (AIBP)"
      }
    ]
  }
}
```

**字段说明**：
- `experimental.monitors[].name="aibp-daemon"`：唯一标识，防 plugin reload 重复 spawn（文档要求）。
- `command`：bun 跑 daemon。支持 `${CLAUDE_PLUGIN_ROOT}` 变量替换。
- `description`：显示在 task panel / 通知摘要里，让用户知道这是 microNeo 的桥。
- `when` 省略 = `"always"`（session 起即自动跑）。

**权限**：Monitor 的 command 走 Bash 权限规则。⚠ `plugin.json` schema **没有** `permissions` 字段（Lisa review #4 确认）——权限必须写在**用户** `~/.claude/settings.json` 里，插件无法自带：
```jsonc
// ~/.claude/settings.json
{ "permissions": { "allow": ["Bash(bun \"${CLAUDE_PLUGIN_ROOT}/index.ts\")"] } }
```
否则每次起 session Claude 都弹权限提示。这条前置配置写进 README 让用户一次性设。

---

## 5. 激活方式（ccmm wrapper）

### 5.1 开发期（最直接）：新建 `ccaibp` wrapper

在 zshrc（ccmm 定义旁边）新增一个 `ccaibp` 函数 = ccmm + `--plugin-dir`。ccmm 本身不动（它只管 env 中转，不带 aibp），想要 aibp 桥时用 `ccaibp`，不需要时还是 `ccmm`：

```bash
# ~/.zshrc，紧跟 ccmm 定义之后
ccaibp() {
  ccmm --plugin-dir /Users/sollawen/pi-dev/microNeo.master/internal/aibp/aibp-agents/claude "$@"
}
```

这样 `ccaibp` 继承 ccmm 的全部 env（ANTHROPIC_BASE_URL/MODEL 等），只是多带一个插件。用法和 ccmm 完全一样，参数透传。

### 5.2 分发期（正式）：marketplace + enabledPlugins

```bash
claude plugin marketplace add <aibp-claude marketplace>
claude plugin install aibp-claude@aibp-marketplace
```
→ 进 `enabledPlugins`，之后任何 `claude`/`ccmm` 调用自动加载，无需 flag。

两种都是**一次性配置，之后永久自动**。开发期用 5.1。

---

## 6. 关键技术决策记录

| 决策 | 选择 | 理由 |
|------|------|------|
| 递送机制 | plugin Monitor | 唯一在 ccmm 下能"立即触发"的公开机制（方案选型 v3 已证） |
| v1 范围 | 只做 §6.3（总触发） | 对齐 opencode；§6.4 暂不做，daemon 极简 |
| stdout 编码 | JSON 单行 | Monitor 按 \n 分事件；多行 formatText 必须压单行，JSON.stringify 天然单行且 claude 可读 |
| channel 去留 | 开发期保留对照，完成后删除 | 验证 monitor 期间能快速回退；通过后整条 channel 链路删干净，主程序零外部依赖 |
| 日志 | 写文件，禁 stdout | stdout 现是 Monitor 事件流，污染=伪事件 |

---

## 7. 验证计划（实施后，需许可执行）

分步、每步可独立验证：

**V1. daemon 独立可跑**（脱离 claude）
- `bun index.ts` 手动起，检查：registry 写入 `/tmp/aibp-<uid>/ai-<name>.json`、socket 监听、日志 `/tmp/aibp-claude.log` 正常
- 用测试脚本往 socket 写一条 `{v:2,type:"context",payload:{path:"x",cursor:{line:1,col:1},selection:{...},message:"hi"}}` → 检查 stdout 输出一行 JSON
- 再写一条无 message 的 → 检查 stdout 仍输出一行（v1 总触发）

**V2. plugin monitor 自动起（高优先级未知项）**
- `ccaibp`（带 --plugin-dir）起 claude → 检查 daemon 子进程自动 spawn（`ps`）、registry 写入、claude task panel 显示 aibp-daemon
- **【关键验证项·作用域存疑】**：plugin-doc 明确写"project-scope 插件 Background monitors do not load；personal-scope 无限制"。`--plugin-dir` 的作用域文档没明说——推断它属"用户显式指定=高信任=personal-scope"，但**必须实测**。若 monitors 不起，改走 marketplace install（§5.2，装入用户 scope）

**V3. 端到端 —— 拆成两项，优先级不同**
- **V3a【命门·核心假设】**：microNeo 选区 + 发消息 → Claude **是否立即生成响应**（开一个 LLM 回合、开始 streaming 打字）？
  - 这是整个方案的命门：Monitor 文档说"interjects"，但"interject"到底是不是真触发 LLM 回合、还是只挂个通知等用户下一轮，文档没保证（Lisa review #14）。**若不立即触发，§6.3 核心目标落空，方案要回炉。**
  - 验证：发消息后观察主对话区是否**主动**开始 streaming（无需用户再按回车）
- **V3b【美观·次要】**：界面是否原样闪现那行 JSON？若碍眼再调 content 格式（不影响功能）

**V4. ccmm 环境验证**
- 用 ccmm（MiniMax 中转）跑 V2-V3，确认 monitor 不被 auth-gate（对比 channel 的 "not currently available"）
- 确认未设 `DISABLE_TELEMETRY`（已查 zshrc/settings.json 均无）

**V5. 边界**
- 多个 claude session 同时跑 → 各抢不同 name（allocateName），互不冲突
- 关闭 claude → daemon 退出、registry+socket 清理
- 池子耗尽 → daemon 退出、日志记录

---

## 8. 风险与待确认项

| 项 | 风险 | 处置 |
|----|------|------|
| **Monitor 是否真触发 LLM 回合**（核心假设） | "interject"未必=开 LLM 回合，可能只通知不生成响应 | **V3a 命门验证**；若不触发，方案回炉（最关键） |
| `--plugin-dir` 的作用域 | plugin-doc 明确 project-scope 插件 monitors 不加载；--plugin-dir 属哪种没明说 | V2 实测；推断属 personal-scope；若不行改 marketplace(§5.2) |
| Monitor stdout 在界面如何呈现 | 原始 JSON 可能原样闪现 | V3b 观察；若碍眼再调格式（仅美观） |
| 并发多事件（连发多条） | Claude 响应慢时事件堆积；是否串行/会否打断自己未知 | 开放问题；V3 连发两条观察行为 |
| Claude 崩溃 → 孤儿 daemon | `process.on('exit')` 不触发，registry 残留 | 低危：allocateName 的 connect 试探 GC 兜底；日志记录异常退出 |
| Monitor 可用性前置条件 | DISABLE_TELEMETRY / 版本<2.1.98 / CI 非交互模式 都致失效 | README 写清：未设 DISABLE_TELEMETRY + 版本达标 + 交互式 CLI（与 ccmm 中转无关，Lisa review #11） |

---

## 9. 协议合规自查

| aibp 协议要求 | 本方案(v1) | 状态 |
|--------------|-----------|------|
| §5.1 信封 v/type 校验 | handleLine 校验 PROTOCOL_MAJOR + type==="context" | ✅ 复用 |
| §4.3 分帧（\n 分隔 JSON） | connectionHandler 按 \n 切行 | ✅ 复用 |
| §五 名字池 allocateName + GC | allocateName 原样复用 | ✅ 复用 |
| §六 注册表 registry 文件 | 写 ai-<name>.json，字段齐全 | ✅ 复用 |
| §6.3 带消息→立即触发 | Monitor stdout 事件 → claude 响应 | ✅ |
| §6.4 纯上下文→不触发 | v1 暂不做（对齐 opencode） | ⏸ 后续 |
| §6.5 单实例覆盖 | allocateName + monitor name 去重 | ✅ |
| 铁律3 能力差异吸收 | 闭源差异用 monitor/stdout 绕开 | ✅ |
| 与 pi/opencode 协议层一致 | 协议层逐字复用 | ✅ |

---

## 10. 落地步骤（建议顺序）

1. **写 index.ts**：复制 aibp-channel.ts → 删 MCP → 加 emitMonitorEvent(§3.3-3.4) → 本地 V1 验证
2. **改 plugin.json**（§4）+ package.json bin 指向 index.ts
3. **新建 ccaibp wrapper**（§5.1）→ V2 验证 monitor 自动起
4. **V3-V5 端到端验证**
5. **更新 README**：激活方式、§6.3 行为说明、DISABLE_TELEMETRY 要求
6. **清理 channel 链路**（monitor 验证通过后）：删 `aibp-channel.ts` + `.mcp.json` + `package.json` 的 `@modelcontextprotocol/sdk` 依赖

---

## 附：与现有 channel 方案的差异速览

| 维度 | channel(现有) | monitor(本方案) |
|------|--------------|----------------|
| claude 公开机制 | MCP channel notification | plugin Monitor (stdout 事件) |
| 第三方 LLM(ccmm) | ❌ auth-gate | ✅ 可用 |
| §6.3 立即触发 | ✅ | ✅ |
| §6.4 不触发 | (channel 总触发) | ⏸ v1 暂不做 |
| 依赖 | MCP SDK | 仅 bun + node net/fs |
| daemon 形态 | MCP server(stdio JSON-RPC) | 纯 stdout 事件流 |
| 协议层 | 复用 pi | 复用 pi（相同） |

---

## 附：未来 §6.4 扩展预留

v1 省略了 §6.4（纯上下文不触发）。若日后要补，在 `onMessage` 加分支：

```typescript
function onMessage(env: any) {
  const p = env.payload
  if (p.message) {
    emitMonitorEvent(p)                          // §6.3：stdout 事件 → 触发
  } else {
    fs.writeFileSync(pendingFile(), formatText(p)) // §6.4：写 pending → 不触发
  }
}
```

并新增 `hooks/aibp-flush.ts`（UserPromptSubmit hook 读 pending → additionalContext）。但这需要先确认 `additionalContext` 的确切输出格式（在线 `/en/hooks`）。

> ⚠ 以上是**简化草图**（Lisa review #10）：真做 §6.4 还需考虑"给用户一个可见提示——有待用上下文"，不止单纯写 pending 文件。**v1 不做，草图仅示意。**
