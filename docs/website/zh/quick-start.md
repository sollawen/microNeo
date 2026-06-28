
## Quick Start

**第一步：一句话安装 microNeo**

```bash
curl -fsSL https://raw.githubusercontent.com/sollawen/microNeo/master/install.sh | sh
```

**第二步：检测有哪些 AI agent**

```bash
microneo --check-agent
```

> 这个检测只需要做一次，microNeo会记录已有哪些AI在电脑里。当然，如果你安装了另一个AI agent的时候，再运行一次这个命令就可以了。

**第三步：用microNeo打开你想和AI讨论的那个文件**

```bash
# 打开任意文件，可以是markdown，也可以是程序代码,json,yaml，任意文本文件
microneo README.md
```

---

## 常用快捷键

| 操作         | 快捷键      | 操作         | 快捷键      |
| ------------ | ---------- | ------------ | ---------- |
| 保存         | `Ctrl-S`   | 撤销         | `Ctrl-Z`   |
| 退出         | `Ctrl-Q`   | 搜索         | `Ctrl-F`   |
| Select | `Shift-up/down` | 给AI发送消息 | `Alt-Enter` |
| 命令模式     | `Ctrl-E`   |              |            |

- 大部份快捷键与VScode相同
- 按 `Ctrl-E` 后输入 `help` 查看更多快捷键和命令。

---

## 设为默认编辑器

```bash
export EDITOR=microneo
```

- 与 `Claude Code`、`Yazi` 等尊重 `$EDITOR` 的工具无缝协作。
- microNeo非常小巧和快速，很适合做为这些工具的默认编辑器

