
## Quick Start

**第一步：一句话安装 microNeo**

```bash
curl -fsSL https://raw.githubusercontent.com/sollawen/microNeo/master/tools/install.sh | sh
```

> 在Linux/Mac上面运行完美。Windows需要完整的shell支持，没有实测

**国内用户**, 如果出现 `raw.githubusercontent.com` is rate-limited (HTTP 429) or unreachable 的问题，可以使用下面的这个镜像来一句话下载。这个问题通常是由于VPN使用的IP地址被GitHub限流了。

```bash
curl -fsSL https://cdn.jsdelivr.net/gh/sollawen/microNeo@master/tools/install.sh | sh
```


**第二步：检测有哪些 AI agent**

```bash
microneo --check-agent
```

- 这个检测只需要做一次，microNeo 会记录已有哪些AI在电脑里。
- 当然，如果你安装了另一个AI agent的时候，需要再运行一次这个命令
- 目前支持 OpenCode、Pi 和 Claude。但是因为 Claude 不是开源的，所以能用但不够完美。


**第三步：开始使用 microNeo**

```bash
# 在当前目录下运行microNeo，
# microNeo 有完整的FileManager，可以方便的在目录树里导航

microneo

# 用microNeo打开指定文件
microneo README.md
```

- 打开任意文件，可以是markdown，也可以是程序代码、json、yaml等任意文本文件
- 和其它编辑器一样对文件进行编辑，快捷键与VScode类似
- `Alt-Enter` 在当前光标处打开与AI的对话框，再按 `Alt-Enter` 发送消息给 AI agent
- 如果你同时运行着多个Ai agent，那么在消息框里面按 `Alt-i` 选择你想对话的那个agent

---

## 编辑时常用操作

### 常用快捷键

| 操作         | 快捷键      | 操作 | 快捷键 | 操作 | 快捷键 |
| ---- | ----- | ---- | ---- | -----|----- |
| 保存         | Ctrl-S   | 撤销         | Ctrl-Z   | 复制 | Ctrl-C |
| 退出         | Ctrl-Q   | 搜索         | Ctrl-F   | 剪切 | Ctrl-X |
| Select | Shift-Up/Down | 给AI发送消息 | Alt-Enter | 粘贴 | Ctrl-V |
| 命令模式     | Ctrl-E   | Help    |  Ctrl-G     |

- 大部份快捷键与VScode相同
- 按 `Ctrl-G` 查看更多快捷键和命令。

### 与AI agent 对话

1. 用microNeo打开一个markdown文档，select你想发表意见的那部份文字
1. 按 `alt-enter` 打开输入框，在里面写下你的意见
1. 写好之后，再次按 `alt-enter` 发送给AI。AI就收到你的意见了。

### 同时与多个AI agent 对话

如果你同时运行多个AI agent，比如1个 `opencode`, 2个 `pi`，这些AI在启动的时候会先给自己起个名字

- 默认的名字是 `Alpha, Bravo, Charlie...` 
- 如果你不喜欢这些默认名字，可以修改配置目录里的 `~/.config/aibp/aibp-names.json` 这个文件，改成你喜欢的名字，比如 `Lisa，Mike，小明，子轩` 之类的

当你在编辑界面里按 `Alt-Enter` 的时候，如果同时有多个AI存在，microNeo会自动打开一个菜单，让你选择一个AI去通信

- 这个选择会被microNeo记住。你再次 `Alt-Enter` 的时候会自动使用上次的名字
- 如果想换一个AI去聊天了，就在对话框里面输入的时候，按 `Alt-i` ，并从菜单里重新挑选AI

### 切换颜色主题 Theme

在microNeo里面，按 `Ctrl-E` 进入最底端的命令行，输入 `:theme` 回车，就可以在菜单里挑选不同的Theme主题了

### 打开新文件 File Open
在microNeo里面，按 `Ctrl-O` 就可以选择新的文件了

- `上下键`，移动光标
- `右键`，进入子目录
- `左键`，进入父目录
- `回车`，打开这个文件


---

## 设为默认编辑器
- microNeo非常小巧和快速，很适合做为这些工具的默认编辑器
- 与 `Claude Code`、`Yazi` 等使用 `$EDITOR` 的工具无缝协作。
- 因为`microNeo`名字比较长，建议在`zshrc` or `bashrc`里面设置alias，方便输入命令

```bash
export EDITOR=microneo
alias edit='microneo'
```

