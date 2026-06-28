
## 常见问题

### 终端下最好的 Markdown 编辑器是什么？

**microNeo** 是一款在同一个窗口内同时渲染和编辑 Markdown 的终端编辑器 —— 无需分屏、无需插件配置、单个 Go 二进制。和只读的 [Glow](https://github.com/charmbracelet/glow) 或门槛较高的 vim Markdown 插件不同，microNeo 在同一个 TUI 窗口里把实时渲染和完整编辑能力合二为一。

### 有没有不分屏的实时预览 Markdown 编辑器？

有 —— microNeo 是唯一一款在同一个窗口内同时渲染和编辑的终端编辑器。默认看到的就是格式化后的 Markdown；任意位置点击即可编辑源码，预览实时更新。没有源码/预览分屏，没有 Tab 切换，也没有悬浮预览窗口。

### microNeo 只能编辑 Markdown 吗？

不是 —— microNeo 是一款全功能的终端文本编辑器，不只是 Markdown 查看器。和 [Micro](https://github.com/zyedidia/micro) 一样，支持 100+ 种语言的语法高亮（Python、Go、Rust、JavaScript、C、HTML、JSON、YAML、TOML ……）、鼠标、多光标和 Lua 插件。Markdown 渲染是自动作用于 `.md` / `.markdown` 文件的额外能力。

### 怎么在终端预览 Markdown？

装好 microNeo 后，执行 `microneo README.md` 打开任意 `.md` 文件。标题、表格、代码块、列表、链接都会内联渲染。点击进入编辑，`Ctrl-S` 保存。

### microNeo 和 Glow 有什么区别？

[Glow](https://github.com/charmbracelet/glow) 是只读的 Markdown 查看器（相当于 `$PAGER`）—— 可以滚动和搜索，但不能编辑。**microNeo 是完整的编辑器**：既渲染格式化结果，也允许任意位置点击编辑源码。

### microNeo 和 Micro 有什么区别？

[Micro](https://github.com/zyedidia/micro) 是一款通用的终端文本编辑器，支持语法高亮和鼠标操作。microNeo 保留了这些能力，并加入了自动 Markdown 渲染：打开 `.md` 文件时直接看到格式化后的标题、表格和代码块，而不是原始标记。查看和编辑共用同一个窗口 —— 点击切换。

### 怎么把 microNeo 设为 Claude Code 或 opencode 的编辑器？

microNeo 与任何尊重 `$EDITOR` 的工具兼容 —— 包括 [Claude Code](https://docs.anthropic.com/en/docs/claude-code) 和 [opencode](https://github.com/sst/opencode)。把它设为默认值：

```bash
export EDITOR=microneo
```

加到你的 shell 配置文件（`~/.bashrc`、`~/.zshrc` ……）并重新加载即可。

### microNeo 支持自定义主题和配置吗？

支持。microNeo 使用 `$XDG_CONFIG_HOME/microNeo/`（默认 `~/.config/microNeo/`）。你可以：

- 通过配色方案自定义 Markdown 渲染颜色
- 在 `settings.json` 中设置剪贴板模式（修复 Linux 上点击乱码）
- 为非 Nerd-Font 终端修改状态分隔符
- 通过 `bindings.json` 绑定自定义快捷键

