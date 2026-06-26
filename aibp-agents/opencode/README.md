# aibp-opencode

AIBP (AI Bridge Protocol) 接收端插件，让 **opencode** 成为 microNeo 的 AI 接收端：在 microNeo 里选中代码按 Alt-Enter，内容递送到当前运行的 opencode。

## 工作原理

- **形态**：opencode **TUI 插件**（`export default { id, tui }`）。在 opencode 主界面就绪时立即加载（不受 instance bootstrap gating 影响），完成「注册名字 + 显示名字 + 开 socket」。
- **协议**：与 [`aibp-pi`](../pi) 同协议（`aibp-2.0`），共用同一名字池文件与 registryDir，pi 与 opencode 并存时自动分配不同名字（如 Alpha / Bravo）。
- **递送**：收到 microNeo 消息后，通过 `api.client.tui.*`（`clearPrompt` + `appendPrompt` + `submitPrompt`）填输入框并触发 LLM 对话；纯上下文则只填输入框不提交。

详见 `docs/agent-comm/D19-aibp-opencode.md`。

## 安装（本地开发）

> 📌 **总原则：opencode `plugin` 命令按插件名去重，不是只追加**。tui.json 里已有同名条目时，重装只打印 `Already configured` 并**保留旧条目不变**（cache 会刷新，但 tui.json 不改）。因此任何形态/版本切换（npm→源码、源码→npm、或换版本）**都必须先手动从 tui.json 删掉旧条目**，否则新装被静默忽略——症状就是「命令成功执行却没生效」。具体迁移步骤见各方式末尾的两个子节。

源码在 `aibp-agents/opencode/`，Bun 直接加载 `.ts`，无需预编译。

### 方式一：path plugin（推荐，源码就在本地）

```bash
opencode plugin /path/to/microNeo/aibp-agents/opencode -g   # 写入全局 ~/.config/opencode/tui.json
```

> ⚠️ 上面写入的是 **`tui.json`**（TUI 插件登记表），不是 `opencode.json`。本插件是 TUI-only，`opencode.json` 里不会出现它（那里是 server 插件和 MCP 的位置）。

然后重启 opencode。启动后右下角应弹 toast `aibp 已就绪 ● Bravo`（名字视池子分配而定）。

**迭代开发**：直接编辑 `index.tsx`，**重启 opencode 即生效**——无需 `npm publish` / `opencode plugin` 重装。如果改了没生效，先确认 `tui.json` 里的路径仍指向你正在编辑的目录。

#### 从 npm 版迁回源码版

如果之前装的是 npm 版（`aibp-opencode`），切换路径：

```bash
# 1. 从 tui.json 删掉 npm 版条目（startswith 覆盖 "aibp-opencode" 和 "aibp-opencode@1.0.2" 两种写法）
jq 'del(.plugin[] | select(. | startswith("aibp-opencode")))' \
   ~/.config/opencode/tui.json > /tmp/tui.json.new \
   && mv /tmp/tui.json.new ~/.config/opencode/tui.json

# 2. 清掉 npm 包的本地 cache（@* 覆盖所有版本，不然 `opencode plugin add` 还会去那里拉）
rm -rf ~/.cache/opencode/packages/aibp-opencode@*

# 3. 装源码版
opencode plugin /path/to/microNeo/aibp-agents/opencode -g
```

#### 从源码版迁回 npm 版

反向同理——opencode 按插件名去重，不先删源码条目的话 npm 版装不进去（提示 `Already configured`）：

```bash
# 1. 从 tui.json 删掉源码版条目（spec 是装时的绝对路径）
jq 'del(.plugin[] | select(. == "/path/to/microNeo/aibp-agents/opencode"))' \
   ~/.config/opencode/tui.json > /tmp/tui.json.new \
   && mv /tmp/tui.json.new ~/.config/opencode/tui.json

# 2. 装 npm 版
opencode plugin aibp-opencode -g
```

### 方式二：发布到 npm 后

首次安装：

```bash
opencode plugin aibp-opencode -g
```

验证（规范形态）：

```bash
jq '.plugin' ~/.config/opencode/tui.json     # 应是 ["aibp-opencode"]（无版本后缀）
ls ~/.cache/opencode/packages/ | grep aibp    # 应只有 aibp-opencode@latest
```

启动 opencode，左下角应有 `● 名字` 标记。

#### 升级到新版本（清除旧版 → 安装新版）

> ⚠️ **必须先完全退出 opencode TUI**。opencode 运行时持有 cache，不退出就删 cache 会被重建，新版没法真正装上——这是「明明装了新版却还是旧版 / 不生效」最常见的坑。

> 💡 下面用**无版本号安装**（`aibp-opencode`，加载 `@latest`，每次拉 npm 最新），与 microNeo 自动托管（`ensure_opencode.go`）一致。**要锁版本**就把第 3 步换成 `opencode plugin aibp-opencode@<版本号> -g`（tui.json 写成 `aibp-opencode@<版本号>`、cache 目录为 `aibp-opencode@<版本号>`）。

```bash
# 1. 从 tui.json 删除所有 aibp-opencode 条目（与「删除（卸载）」步骤 1 相同）
#    startswith 覆盖带/不带版本号两种写法；|= 只改 plugin 数组，其它键保留。
jq 'if .plugin then .plugin |= map(select(. | startswith("aibp-opencode") | not)) else . end' \
   ~/.config/opencode/tui.json > /tmp/tui.json.new \
   && mv /tmp/tui.json.new ~/.config/opencode/tui.json

# 2. 删除所有版本的插件 cache（@* 清掉所有版本残留，不只是 latest）
rm -rf ~/.cache/opencode/packages/aibp-opencode@*

# 3. 安装最新版（规范形态，无版本后缀）
opencode plugin aibp-opencode -g
```

验证：

```bash
jq '.plugin' ~/.config/opencode/tui.json     # 应是 ["aibp-opencode"]（无版本后缀）
ls ~/.cache/opencode/packages/ | grep aibp    # 应只有 aibp-opencode@latest
```

启动 opencode，左下角应有 `● 名字` 标记。

> 📌 **注册表目录**（`$XDG_RUNTIME_DIR/aibp-<uid>`，fallback `$TMPDIR/aibp-<uid>`）是 opencode 运行时写的状态，**与插件是否安装无关**——升级流程不用管它，opencode 启动时自己管理。

开发期**不要走这条路**——会把本地改动覆盖掉。

#### 发布前 checklist

```bash
# 1. 把 index.tsx 顶部的 DEBUG 常量改成 false（发布版不能在用户机器写 /tmp 日志）
#    const DEBUG = false
# 2. 打标签 + 发版
npm version patch && npm publish
```

## ⚠️ 注意事项

1. **`package.json` 不能有 `main` 字段**。`opencode plugin` 看 `main` 存在就把包当 server 插件，会同时往 `opencode.json` 里加，启动时报 `must default export an object with server()`。TUI-only 插件只保留 `exports["./tui"]` 一个出口即可。
2. **`opencode plugin` 有本地 cache**（`~/.cache/opencode/packages/aibp-opencode@latest/`），发新版后不重装就还是老版本（症状：明明 `npm view` 看是新版，但启动加载的 manifest 还是旧的）。调试时 `rm -rf` 这个目录再重跑命令。这条**只对 npm 版生效**——源码版没有 cache 层，路径直读。

## 验证

```bash
# 1. opencode 启动后，注册文件应已生成（不用先发消息）
ls "$XDG_RUNTIME_DIR/aibp-$(id -u)/"   # 见 ai-Bravo.json
# 2. 名字池（与 aibp-pi 共用）
cat ~/.config/aibp/aibp-names.json
```

然后在 microNeo 里选中代码按 Alt-Enter，opencode 端应：
- 带消息 → 自动发起对话（输入框被填 + 提交）；
- 纯上下文 → 仅填入输入框，等用户编辑后手动发送。

## 删除（卸载）

opencode 的 `plugin` 子命令**没有** remove / uninstall，需要手动清理。下面是经实测验证的完整删除流程（opencode 1.17.11 / aibp-opencode 1.0.2）。

> 💡 删除前建议**完全退出 opencode TUI**——运行时它会持有 cache 和注册文件，退出后清理最干净（否则 opencode 可能把 cache 重建回来）。

```bash
# 1. 从 tui.json 删除所有 aibp-opencode 条目
#    startswith 同时覆盖 "aibp-opencode" 与 "aibp-opencode@<version>" 两种写法；
#    |= 只改 plugin 数组，其它键（$schema / keybinds / …）原样保留。
jq 'if .plugin then .plugin |= map(select(. | startswith("aibp-opencode") | not)) else . end' \
   ~/.config/opencode/tui.json > /tmp/tui.json.new \
   && mv /tmp/tui.json.new ~/.config/opencode/tui.json

# 2. 删除所有版本的插件 cache
#    @* 清掉 @latest 与所有 pinned 版本（@1.0.x）残留——只删 @latest 会泄漏 pinned cache。
rm -rf ~/.cache/opencode/packages/aibp-opencode@*
```

验证删除干净：

```bash
jq '.plugin' ~/.config/opencode/tui.json          # 应是 [] 或不含 aibp 开头的条目
ls ~/.cache/opencode/packages/ | grep aibp         # 应无输出
jq 'keys' ~/.config/opencode/tui.json              # $schema / keybinds 等仍在（未被破坏）
```

> 注册表目录（`$XDG_RUNTIME_DIR/aibp-<uid>`，fallback `$TMPDIR/aibp-<uid>`）是 opencode 运行时写的状态，卸载时**不必管**——它随 opencode 退出自然失效，下次启动按名字池重建。

microNeo 侧零改动，协议 agent 无关。
