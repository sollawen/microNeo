# aibp-opencode

AIBP (AI Bridge Protocol) 接收端插件，让 **opencode** 成为 microNeo 的 AI 接收端：在 microNeo 里选中代码按 Alt-Enter，内容递送到当前运行的 opencode。

## 工作原理

- **形态**：opencode **TUI 插件**（`export default { id, tui }`）。在 opencode 主界面就绪时立即加载（不受 instance bootstrap gating 影响），完成「注册名字 + 显示名字 + 开 socket」。
- **协议**：与 [`aibp-pi`](../pi) 同协议（`aibp-1`），共用同一名字池文件与 registryDir，pi 与 opencode 并存时自动分配不同名字（如 Alpha / Bravo）。
- **递送**：收到 microNeo 消息后，通过 `api.client.tui.*`（`clearPrompt` + `appendPrompt` + `submitPrompt`）填输入框并触发 LLM 对话；纯上下文则只填输入框不提交。

详见 `docs/agent-comm/D19b-插件加载时机与形态反转.md`。

## 安装（本地开发）

源码在 `aibp-agents/opencode/`，Bun 直接加载 `.ts`，无需预编译。

### 方式一：path plugin（推荐，源码就在本地）

```bash
cd /path/to/microNeo
opencode plugin ./aibp-agents/opencode -g   # 写入全局 ~/.config/opencode/opencode.json
```

然后重启 opencode。启动后右下角应弹 toast `aibp 已就绪 ● Bravo`（名字视池子分配而定）。

### 方式二：发布到 npm 后

```bash
opencode plugin aibp-opencode -g
```

## 验证

```bash
# 1. opencode 启动后，注册文件应已生成（不用先发消息）
ls "$XDG_RUNTIME_DIR/microneo-agent-bridge-$(id -u)/"   # 见 ai-Bravo.json
# 2. 名字池（与 aibp-pi 共用）
cat ~/.config/aibp/aibp-names.json
```

然后在 microNeo 里选中代码按 Alt-Enter，opencode 端应：
- 带消息 → 自动发起对话（输入框被填 + 提交）；
- 纯上下文 → 仅填入输入框，等用户编辑后手动发送。

## 卸载

```bash
opencode plugin remove aibp-opencode   # 或从 opencode.json 的 plugin[] 删条目
```

microNeo 侧零改动，协议 agent 无关。
