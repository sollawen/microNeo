# aibp-pi

[AIBP (AI Bridge Protocol)](https://github.com/sollawen/microNeo) receiver extension for [pi](https://pi.dev).

让 pi 能接收来自 [microNeo](https://github.com/sollawen/microNeo) 编辑器的上下文通知与消息——在 microNeo 里选中代码按 Alt-Enter，内容会递送到当前运行的 pi 会话。

## 关于 microNeo

microNeo 是基于 [micro](https://github.com/zyedidia/micro) 增强的终端编辑器，专注 Markdown 渲染：打开 `.md` 文件即见完整排版（标题/表格/代码块/列表），而非原始格式字符。内置与 AI 协作的侧边 notePane，可将选中代码、行锚点连同消息一键递送给 pi 等 AI 编程助手。

## 安装

```bash
pi install npm:aibp-pi
```

microNeo 会在首次启动时自动检测并安装本扩展（若 pi 已就位）。手动安装也可用上面的命令。

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

`aibp-1`

## 相关

- 源码与协议详情：[microNeo](https://github.com/sollawen/microNeo)
