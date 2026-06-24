# aibp-opencode

AIBP (AI Bridge Protocol) 接收端插件，让 **opencode** 成为 microNeo 的 AI 接收端：在 microNeo 里选中代码按 Alt-Enter，内容递送到当前运行的 opencode。

## 工作原理

- **形态**：opencode **TUI 插件**（`export default { id, tui }`）。在 opencode 主界面就绪时立即加载（不受 instance bootstrap gating 影响），完成「注册名字 + 显示名字 + 开 socket」。
- **协议**：与 [`aibp-pi`](../pi) 同协议（`aibp-1`），共用同一名字池文件与 registryDir，pi 与 opencode 并存时自动分配不同名字（如 Alpha / Bravo）。
- **递送**：收到 microNeo 消息后，通过 `api.client.tui.*`（`clearPrompt` + `appendPrompt` + `submitPrompt`）填输入框并触发 LLM 对话；纯上下文则只填输入框不提交。

详见 `docs/agent-comm/D19-aibp-opencode.md`。

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

#### 发布前 checklist

```bash
# 1. 把 index.tsx 顶部的 DEBUG 常量改成 false（发布版不能在用户机器写 /tmp 日志）
#    const DEBUG = false
# 2. 打标签 + 发版
npm version patch && npm publish
```

## ⚠️ 注意事项

1. **`package.json` 不能有 `main` 字段**。`opencode plugin` 看 `main` 存在就把包当 server 插件，会同时往 `opencode.json` 里加，启动时报 `must default export an object with server()`。TUI-only 插件只保留 `exports["./tui"]` 一个出口即可。
2. **`opencode plugin` 有本地 cache**（`~/.cache/opencode/packages/aibp-opencode@latest/`），发新版后不重装就还是老版本（症状：明明 `npm view` 看是新版，但启动加载的 manifest 还是旧的）。调试时 `rm -rf` 这个目录再重跑命令。

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

opencode 1.17.9 的 `plugin` 子命令**没有** remove / uninstall，需要手动清理：

```bash
# 1. 从 tui.json 删条目（aibp-opencode 是 TUI 插件，不在 opencode.json 里）
#    用 jq 更安全，或者手动编辑文件把 "aibp-opencode" 从 plugin[] 删掉
jq 'del(.plugin[] | select(. == "aibp-opencode"))' \
   ~/.config/opencode/tui.json > /tmp/tui.json.new \
   && mv /tmp/tui.json.new ~/.config/opencode/tui.json

# 2. 可选：清掉 opencode 的本地包缓存
rm -rf ~/.cache/opencode/packages/aibp-opencode@latest
```

microNeo 侧零改动，协议 agent 无关。
