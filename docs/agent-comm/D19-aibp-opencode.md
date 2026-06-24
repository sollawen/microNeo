# D19: aibp-opencode —— microNeo → opencode 代码递送通道

让 opencode 成为 AIBP 接收端：在 microNeo 里选中代码按 Alt-Enter，内容递送到 opencode 当前运行的对话。

## 功能

- microNeo 用户选中代码 → Alt-Enter → 内容出现在 opencode 当前对话
- 支持纯选区、选区+用户消息、纯消息三种模式
- 自动格式化递送内容（`<selection: file lines X-Y>` 风格）

## 实现方案

### 插件形态：TUI 插件

```typescript
export default {
  id: "aibp-opencode",
  tui: async (api) => { ... }
}
```

**为什么用 TUI 插件**：TUI 插件在 App mount（主界面就绪）时立即加载，不受 instance bootstrap gating 影响。满足"启动即注册名字 + 开 socket"需求。若用 server 插件，需等用户首条消息触发 instance bootstrap，延迟注册会导致 microNeo 端找不到接收端。

### 协议层（与 aibp-pi 共用）

- **协议版本**：`aibp-1`（在 package.json 的 `aibp.protocol` 字段）
- **名字池**：共用 `~/.config/aibp/aibp-names.json`，支持多接收端并存（pi 和 opencode 自动分配不同名字）
- **registryDir**：`$XDG_RUNTIME_DIR/microneo-agent-bridge-$UID/`（与 aibp-pi 一致）
- **分帧**：行分隔的 JSON（与 aibp-pi 一致）

### 递送策略（最简版）

只把消息发到 **TUI 当前正在看的对话**，不创建 session、不选择 agent/model。

```typescript
const route = api.route.current
if (route?.name !== "session" || typeof route?.params?.sessionID !== "string") {
  toast("⚠ 请先在 opencode 打开一个对话", "warning")
  return
}

const sessionID = route.params.sessionID
client.session.prompt({
  sessionID,
  parts: [{ type: "text", text: formattedContent }]
})
```

**为什么最简**：opencode 的 agent/model 选择逻辑复杂（agent 根据工具需求动态切换），让用户在 UI 里手动选择更稳妥。插件只做消息注入器。

## 关键坑点：v2 SDK 调用风格

### 问题

早期实现时，`client.session.prompt` 一直返回 500（err_ref），db 无消息记录，server 日志无痕迹。经诊断，根因是：

**插件拿到的 `api.client` 是 `@opencode-ai/sdk/v2` 的 `OpencodeClient`**，而文档示例和部分源码用的是 v1 风格。

### v1 vs v2 调用风格差异

| | v1 风格（错误） | v2 风格（正确） |
|---|---|---|
| 参数结构 | `{ path: { id: sessionID }, body: { parts: [...] } }` | `{ sessionID, parts: [...] }` 顶层 |
| URL 结果 | `/session/undefined/message` | `/session/{sessionID}/message` |
| 结果 | 500 | ✅ |

**v2 正确签名**（`packages/sdk/js/src/v2/gen/sdk.gen.ts`）：
```typescript
public prompt(parameters: {
  sessionID: string,      // ← 顶层参数
  agent?: string,
  model?: { providerID, modelID },
  parts?: Array<TextPartInput | ...>,
  ...
}, options?)
// url: "/session/{sessionID}/message"
```

**opencode 自己的调用**（`packages/tui/src/component/prompt/index.tsx:1090`）：
```typescript
sdk.client.session.prompt({
  sessionID,
  agent: agent.name,
  model: selectedModel,
  parts: [...],
}, { throwOnError: true })
```

### 修复

改用 v2 风格：
```typescript
client.session.prompt({
  sessionID,
  parts: [{ type: "text", text }]
})
```

## 文件结构

```
aibp-agents/opencode/
├── index.ts           # TUI 插件主体（协议 + socket + 递送）
├── package.json       # exports: { "./tui": "./index.ts" }
└── README.md          # 安装/使用说明
```

## 安装

### 本地开发（推荐）

```bash
cd /path/to/microNeo
opencode plugin ./aibp-agents/opencode -g   # 写入 ~/.config/opencode/opencode.json
```

### 发布后

```bash
opencode plugin aibp-opencode -g
```

## 验证

```bash
# 1. opencode 启动后，注册文件应已生成
ls "$XDG_RUNTIME_DIR/microneo-agent-bridge-$(id -u)/ai-*.json"

# 2. 名字池（与 aibp-pi 共用）
cat ~/.config/aibp/aibp-named.json

# 3. 插件日志
tail -f /tmp/aibp-opencode.log
```

然后在 microNeo 里选中代码按 Alt-Enter，opencode 端应自动发起对话。

## 调试日志

插件写 `/tmp/aibp-opencode.log`（append），记录所有关键节点：
- 启动阶段：module loaded / name pool / allocation / registry / ready
- 报文阶段：connection accepted / line received / envelope parsed / formatText output / prompt calling / resolved

## 设计文档

- D19：本文件（总结）
- 参考 D17：aibp-pi 实现（协议规范）
