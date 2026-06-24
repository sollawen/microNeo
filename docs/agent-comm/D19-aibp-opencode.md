# D19 · aibp-opencode —— microNeo → opencode 的代码递送通道

> **状态**：已实现并验证通过。源码 `aibp-agents/opencode/index.tsx`（446 行），npm 包 [`aibp-opencode@1.0.1`](https://www.npmjs.com/package/aibp-opencode)。
>
> 想了解协议细节看 **D17**，想了解安装/卸载/踩过的坑看 **`aibp-agents/opencode/README.md`**，想了解 opencode 插件机制选型看 **`docs/agent-comm/opencode调研.md`**。本文件只做总览。

## 一句话

opencode 的 **TUI 插件**，让 microNeo 能把选中代码（或纯消息）通过 AIBP 协议递送到当前正在运行的 opencode 对话里。aibp-pi 的 opencode 版孪生兄弟。

## 形态选择：为什么是 TUI 插件（不是 server 插件）

| 形态 | 加载时机 | 能否满足"启动即注册名字 + 开 socket" |
|------|---------|-------------------------------------|
| **TUI 插件** | App mount 立即加载 | ✅ |
| server 插件 | 需等用户首条消息触发 instance bootstrap | ❌ 延迟注册会让 microNeo 找不到接收端 |

所以 `index.tsx` 导出的是 `{ id, tui }`，不是 `{ id, server }`。

## 与 aibp-pi 的关系

两者协议层完全一致，是同一个协议的**两个实现**：

| 项 | 取值 | 说明 |
|----|------|------|
| 协议版本 | `aibp-1` | 写在两个包 `package.json` 的 `aibp.protocol` 字段 |
| 名字池 | `~/.config/aibp/aibp-names.json` | NATO phonetic alphabet，两端并存时自动分配不同名字（如 Alpha / Bravo） |
| registryDir | `$XDG_RUNTIME_DIR/microneo-agent-bridge-$UID/` | microNeo 端通过它发现 receiver |
| 报文格式 | 行分隔 JSON | 详见 D17 |

## 源码结构

```
aibp-agents/opencode/
├── index.tsx          # 446 行: tui() 入口 + socket server + 消息格式化 + SDK 调用
├── package.json       # 见下文「关键规则」
└── README.md          # 安装 / 验证 / 卸载 / ⚠️ 注意事项
```

主体逻辑全部在 `index.tsx` 的 `tui(api)` 异步函数里（约 387 行），大致五步：

1. **初始化日志**（写 `/tmp/aibp-opencode.log`）
2. **加载/初始化名字池**（NATO names list from `aibp-names.json`）
3. **分配名字**（读 pool，找第一个未被占用的）
4. **启动 socket server**（unix socket，写 registry 文件）
5. **挂消息接收 handler**：收到消息 → 格式化为 `<selection: file lines X-Y>` 风格 → 调 `client.session.prompt({ sessionID, parts: [...] })`（**v2 SDK 顶层参数风格**）填进当前对话

具体实现看源码，不在本文件展开。

## package.json 关键规则（TUI-only 插件的硬约束）

这几条是 **1.0.0 翻车后总结的**，违反任何一条都会导致 `opencode plugin` 误判形态，启动报 `must default export an object with server()`：

| 规则 | 为什么 |
|------|--------|
| ❌ **不能有 `main` 字段** | opencode plugin 的 `packageTargets()` 把 `main` 存在当作 server 插件的信号，会同时往 `opencode.json` 写条目 |
| ✅ **`exports` 只能有 `"./tui"`** | 不要带 `"."`，那是主入口语义，跟我们无关 |
| ✅ **`type: "module"`** | ESM 必须 |
| ✅ **`peerDependencies`**（不是 `devDependencies`）`@opencode-ai/plugin >= 1.4.0` | 插件跑在 opencode 进程里，不能声明成 dev 依赖 |
| ✅ **`engines.opencode >= 1.4.0"`** | 启动时版本检查 |
| ✅ **`aibp.protocol: "aibp-1"`** | 协议版本声明（与 aibp-pi 一致） |

发版流程：

```bash
cd aibp-agents/opencode
npm version patch && npm publish
```

## opencode plugin 命令（1.17.9）的几个坑

不在 README 重复，只列：

| 坑 | 解法 |
|----|------|
| 本地 cache 在 `~/.cache/opencode/packages/<name>@latest/`，发新版后不刷新 | 调试时 `rm -rf` 后再跑 `opencode plugin <name> -g` |
| TUI 插件**正确**写入位置是 `tui.json`，不是 `opencode.json` | package.json 违规（带 `main`）时会被同时写到 `opencode.json`，需手动回滚 |
| 没有 `plugin remove` 子命令 | 卸载：`jq 'del(.plugin[] \| select(. == "<name>"))' ~/.config/opencode/tui.json` + `rm -rf` cache |

## 版本历史

| 版本 | 关键事件 |
|------|---------|
| `1.0.0` | 首次发布。`package.json` 有 `main` + `exports["."]`，导致 opencode plugin 误判成 server + tui 双形态，启动报 `must default export an object with server()` |
| `1.0.1` | 修复：删 `main` + 删 `exports["."]`，devDeps 改 peerDeps，版本范围放松到 `>=1.4.0`。验证：opencode 启动正常，底部显示分配的 NATO 名字（如 `● Bravo`） |

## 相关文档

- **D17** (`docs/agent-comm/D17-初始化aibp-pi.md`)：协议规范、与 aibp-pi 共用的部分
- **D19b** (`docs/agent-comm/D19b-aibp-opencode-自动建session.md`)：aibp-opencode 在无 session 时自动创建对话的可行性分析
- **`aibp-agents/opencode/README.md`**：安装 / 验证 / 卸载 / ⚠️ 注意事项
- **`docs/agent-comm/opencode调研.md`**：opencode 插件机制（Plugin / MCP / ACP）的选型调研
- **`docs/agent-comm/说明-接收端.md`**：AIBP 接收端的总体说明（aibp-pi 和 aibp-opencode 一起）