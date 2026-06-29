
## Quick Start

**第一步：一句话安装 microNeo**

```bash
curl -fsSL https://raw.githubusercontent.com/sollawen/microNeo/master/tools/install.sh | sh
```

> 在Linux/Mac上面运行完美。Windows需要完整的shell支持，没有实测

**第二步：检测有哪些 AI agent**

```bash
microneo --check-agent
```

- 这个检测只需要做一次，microNeo 会记录已有哪些AI在电脑里。
- 当然，如果你安装了另一个AI agent的时候，需要再运行一次这个命令
- 目前支持 OpenCode and Pi, 对 Claude 的支持正在开发中......


**第三步：用microNeo打开你想和AI讨论的那个文件**

```bash
microneo README.md
```

- 打开任意文件，可以是markdown，也可以是程序代码、json、yaml等任意文本文件
- 和其它编辑器一样对文件进行编辑，快捷键与VScode类似
- `Alt-Enter` 在当前光标处打开与AI的对话框，再按 `Alt-Enter` 发送消息给 AI agent
- 如果你同时运行着多个Ai agent，那么在消息框里面按 `Alt-i` 选择你想对话的那个agent

---

## 常用快捷键

| 操作         | 快捷键      | 操作 | 快捷键 | 操作 | 快捷键 |
| ---- | ----- | ---- | ---- | -----|----- |
| 保存         | Ctrl-S   | 撤销         | Ctrl-Z   | 复制 | Ctrl-C |
| 退出         | Ctrl-Q   | 搜索         | Ctrl-F   | 剪切 | Ctrl-X |
| Select | Shift-Up/Down | 给AI发送消息 | Alt-Enter | 粘贴 | Ctrl-V |
| 命令模式     | Ctrl-E   | Help    |  Ctrl-G     |

- 大部份快捷键与VScode相同
- 按 `Ctrl-G` 查看更多快捷键和命令。

---

## 设为默认编辑器
- microNeo非常小巧和快速，很适合做为这些工具的默认编辑器
- 与 `Claude Code`、`Yazi` 等使用 `$EDITOR` 的工具无缝协作。
- 因为`microNeo`名字比较长，建议在`zshrc` or `bashrc`里面设置alias，方便输入命令

```bash
export EDITOR=microneo
alias edit='microneo'
```

