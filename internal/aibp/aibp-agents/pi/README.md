# aibp-pi

[AIBP (AI Bridge Protocol)](https://github.com/sollawen/microNeo) receiver extension for [pi](https://pi.dev).

让 pi 能接收来自 [microNeo](https://github.com/sollawen/microNeo) 编辑器的上下文通知与消息——在 microNeo 里选中代码按 Alt-Enter，内容会递送到当前运行的 pi 会话。

## 关于 microNeo

microNeo 是基于 [micro](https://github.com/zyedidia/micro) 增强的终端编辑器，专注 Markdown 渲染：打开 `.md` 文件即见完整排版（标题/表格/代码块/列表），而非原始格式字符。内置与 AI 协作的侧边 notePane，可将选中代码、行锚点连同消息一键递送给 pi 等 AI 编程助手。

## 安装

> 📌 **总原则：源码版与 npm 版互斥**。任何切换（npm→源码 或 源码→npm）**都必须先 `pi remove` 旧版再 `pi install` 新版**——`pi install` 只追加不替换，两个版本同时加载会冲突（socket 绑定失败、注册文件互相覆盖）。具体迁移步骤见各方式末尾的两个子节。

### 方式一：本地源码路径（开发推荐）

源码在 `internal/aibp/aibp-agents/pi/`（在 microNeo 仓库内），直接加载 `index.ts`，无需预编译。

```bash
pi install /path/to/microNeo/internal/aibp/aibp-agents/pi
```

会写入 `~/.pi/agent/settings.json` 的 `packages` 字段：

```jsonc
{
  "packages": [
    // ... 已有项 ...
    "/Users/you/path/to/microNeo/internal/aibp/aibp-agents/pi"
  ]
}
```

**迭代开发**：直接编辑 `index.ts`，**重启 pi 即生效**——无需 `npm publish` / `pi install`。如果改了没生效，先确认 `settings.json` 里的路径仍指向你正在编辑的目录。

#### 从 npm 版迁回源码版

如果之前装的是 npm 版（`npm:aibp-pi`），切换路径：

```bash
# 1. 卸掉 npm 版（避免两个版本都加载）
pi remove npm:aibp-pi

# 2. 装源码版
pi install /path/to/microNeo/internal/aibp/aibp-agents/pi
```

#### 从源码版迁回 npm 版

反向同理——`pi install` 只追加不替换，必须先卸源码版：

```bash
# 1. 卸掉源码版（spec 是装时的绝对路径）
pi remove /path/to/microNeo/internal/aibp/aibp-agents/pi

# 2. 装 npm 版
pi install npm:aibp-pi
```

### 方式二：npm 包（发布后）

```bash
pi install npm:aibp-pi
```

**注意**：npm 版的更新需要 `pi update npm:aibp-pi`（详见 microNeo 仓库的 `工作记录0625.md`）。开发期**不要走这条路**——会把本地改动覆盖掉。

microNeo 会在首次启动时自动检测并安装本扩展（若 pi 已就位）——但**只针对 npm 版**。源码版需要手动 `pi install` 一次。

## 功能

通过 Unix socket 接收 microNeo 发来的上下文通知，根据报文内容走两条递送路径：

### 带 message 报文
- 格式化上下文引用（`@<path> :line<focus>`）+ 用户消息
- 调用 `pi.sendUserMessage(text, { deliverAs: "steer" })` 触发 LLM 对话
- streaming 时自动排队，等当前回合工具链结束后插入

### 纯上下文报文（无 message）
- 格式化上下文引用
- 调用 `ctx.ui.setEditorText(text)` 填入输入框
- 用户手动发送后才进入 LLM（不自动触发）

## 协议版本

`aibp-2.0`

## 相关

- 源码与协议详情：[microNeo](https://github.com/sollawen/microNeo)
