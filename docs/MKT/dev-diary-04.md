# 开发日记 #4：用 microNeo 替掉 shell 的 cd

cd 是 Linux 里用得最多、也最难用的命令——不必展开。我去年在 shell 里敲了 1874 次 cd，是 git commit 的 6 倍。

试过 `autojump`、`z`、`zoxide`、`fasd`，它们的解法都是「按我去过这个目录的频率做模糊匹配」。但我要的不是这些——我要的是用方向键在庞大的目录树里自由的穿梭，然后当前目录就切换了。

---

### 我的日常工作流

我日常的工作流大概是这样：

```
shell ──► 编辑器 ──► FileSelector ──► 编辑器 ──► shell
  │         │           │             │         │
  │         │           │             │         │
cd        编辑        浏览树状        编辑      git / ls / grep
走老路    a.go       换 b.go       b.go       （想要在 b.go 的目录！）
```

最右那格「git / ls / grep」告诉我失败了——shell 还卡在启动位置，每个文件的目录都要重新敲一次。我装编辑器就是为了不再手动 cd。

---

### 转折：在 yazi 里看到答案

yazi 刚好满足我的需求，特别对症下药。但许多服务器上没装 yazi——我就想，如果 microNeo 也有 yazi 这种切换目录的功能，一台机器只装一个工具就够了。

yazi 早就有这个能力：你随便在 yazi 里切换目录，退出后 shell 自动跟过去。5 行 shell 函数搞定：

```zsh
function y() {
    local tmp="$(mktemp -t "yazi-cwd.XXXXXX")" cwd
    command yazi "$@" --cwd-file="$tmp"
    IFS= read -r -d '' cwd < "$tmp"
    [ "$cwd" != "$PWD" ] && [ -d "$cwd" ] && builtin cd -- "$cwd"
    rm -f -- "$tmp"
}
```

逻辑全在这 5 行里：

1. `mktemp` 建一个临时文件（路径由 shell 拥有）
2. 把临时文件路径通过 `--cwd-file` flag 传给 yazi
3. yazi 退出时往这个文件写最后所在目录
4. shell 函数读完文件、判断合法、就 `cd` 过去
5. `rm` 清理临时文件

**临时文件做 IPC**——shell 是主动方、yazi 是被动方，两端通过一个文件句柄通信。整套机制没 FIFO、没 signal、没 shell hook——就是一个 mktemp 加上一个 `WriteFile`。

这个设计有两条工程上的小巧：

- **轻**：5 行 shell 函数，零运行时依赖，zsh / bash / fish 都能跑。
- **可降级**：yazi 崩了、临时文件被删、内容写废——shell 端 `[ -d "$cwd" ]` 这道闸都会拦截。绝不会因为这个功能把 shell 带去错地方。

把这套机制反过来讲就一句话：**子进程不能改父进程的 cwd——这是 OS 给的硬约束**。任何「程序退出后想改 shell 的目录」的方案，都必须借助 IPC。临时文件是 IPC 里最朴素的那种：文件句柄由 shell 拥有、文件内容由程序填、shell 自己读自己信。不需要新建任何进程外的服务。

yazi 5 行做完最难的设计：IPC 协议、shell 函数时序、错误降级全管完了。

剩下的是搬。

---

### 我做了什么：搬 IPC + 15 行侵入

我的编辑器 microNeo 跟 yazi 不是同一种东西——yazi 是文件管理器，microNeo 是编辑器——但问题结构是一样的：「程序退出时，把一个状态交给 shell」。状态从「最后浏览的目录」换成「最后编辑的文件」。

唯一的设计判断：**「最后目录」是哪个目录**。

看起来这是个傻瓜题。但「我现在在哪」这三个候选是分开的：

- **进程 cwd**（`os.Getwd()`）—— `:cd` 命令改的是这个，但用户要的不是这个。microNeo 里 `:cd` 和「最后编辑的文件」是两件不相关的事。
- **FileSelector 的 currentDir** —— 这是浏览状态，不是编辑状态。用户用 selector 看了一眼翻到某目录、又退出，「最后目录」不该是 selector 看过的那个；该是 selector 选完之后、buffer 落到屏幕上的那个文件所在的目录。
- **最后活跃 pane 的 buffer 的父目录** —— 这是用户语义里的「最后」。我去编辑了它，我就要它的目录。

microNeo 选第三个。规则只有这一条：

| 场景 | 最后活跃 pane 的 buffer | 写入的目录 |
|---|---|---|
| 打开 `~/proj/src/a.go` 后退出 | `a.go` | `~/proj/src` |
| 多 pane，逐个关到只剩一个 | 最后剩下那个 pane | 那个 pane 的 buffer 父目录 |
| FileSelector 选了 `c.go` 换入后退出 | `c.go` | `c.go` 的父目录 |
| FileSelector 浏览后 Esc 取消 | 原 buffer | 原 buffer 的父目录 |
| Help / Log pane 是最后活跃的 | Help / Log | 不写（shell cwd 不变）|
| 退出时活跃 pane 是嵌入式终端 | type assertion 失败返回 nil | 不写 |
| 启动后没开文件就退出 | 无名空 buffer | 不写 |

「Help/Log 不写」这条单独说一下：用户从来没编辑 Help/Log，它是「活动焦点」不等于「我编辑的位置」。**用户语义领先于当前焦点**——这是 microNeo 在这件事上的设计原则。

代码长这样（F3 §4.1 / `cmd/micro/micro.go`，合计 15 行侵入）：

```go
flagCwdFile = flag.String("cwd-file", "",
    "Write the working directory to this file on exit (for shell integration)")
```

```go
// lastWorkingDir returns the directory to report back to the shell:
// the parent directory of the last active pane's buffer. Returns "" when
// no file is open (empty buffer, Help/Log pane, TermPane, etc).
func lastWorkingDir() string {
    if t := action.MainTab(); t != nil {
        if pane := t.CurPane(); pane != nil && pane.Buf != nil {
            if ap := pane.Buf.AbsPath; ap != "" {
                return filepath.Dir(ap)
            }
        }
    }
    return ""
}
```

```go
func exit(rc int) {
    cwdForShell := ""
    if *flagCwdFile != "" {
        cwdForShell = lastWorkingDir()
    }

    // ... 现有 buffer Fini / screen Fini ...

    if *flagCwdFile != "" && cwdForShell != "" {
        _ = os.WriteFile(*flagCwdFile, []byte(cwdForShell), 0600)
    }
    os.Exit(rc)
}
```

总共 15 行。F3 文档算了精确的侵入量：

| 文件 | 改动 | 行数 |
|---|---|---|
| `cmd/micro/micro.go` | `InitFlags` 加 1 个 flag + 新增 `lastWorkingDir()` + `exit()` 内 3 行 | ~15 |
| `internal/*` | **零改动** | 0 |
| `runtime/*` | **零改动** | 0 |
| `Makefile` / `go.mod` | **零改动** | 0 |

`--cwd-file` 不传就整段逻辑跳过。现有 `alias edit='microneo'` 的用户感知不到——**opt-in 原则**：想要这个能力的用户主动装 `m()` 函数，不要的用户继续 `microneo`，两条路并存。

---

### shell 端：照搬 yazi 的 5 行

`~/.zshrc` / `~/.bashrc` 里加：

```zsh
function m() {
    local tmp="$(mktemp -t "microneo-cwd.XXXXXX")" cwd
    command microneo "$@" --cwd-file="$tmp"
    IFS= read -r -d '' cwd < "$tmp"
    [ "$cwd" != "$PWD" ] && [ -d "$cwd" ] && builtin cd -- "$cwd"
    rm -f -- "$tmp"
}
```

跟 yazi 的 `y()` 对得上一对一，复制粘贴改两个名字就行。

那个 `m` 命令不会侵入用户的 `alias edit='microneo'`——想用 cd 集成的用户用 `m`，想用纯净 microNeo 的用户继续 `microneo` / `edit`。**两套并存**，自愿选择。

bash 用户的注意点：`mktemp -t "name.XXXXXX"` 是 BSD / macOS 语法。GNU coreutils 要写 `mktemp -t name`——按各自平台写。这是踩过坑的，**别照抄就跑**。

崩溃降级三类：

- microNeo 被 `kill -9` 杀了 → defer 不跑 → 临时文件不写 → shell 读到空 → `[ -d "" ]` 失败 → 不 cd。
- microNeo panic 同理，文件未写，shell 安全 skip。
- `--cwd-file` 指向不可写路径 → `os.WriteFile` 报错 → 静默丢弃 → shell 端 `[ -d "" ]` 拦截 → 不 cd。

**永远不会因为这个功能把 shell 带去错地方**。这条不是「我们要做到」，是「机制上就做不到」——文件不写 / 写废，shell 端的安全闸都会拦下。

---

### 用一次

我在 `~/playground` 这个两层目录里。敲 `m` 进 microNeo。Ctrl+O 打开 FileSelector，搜 `/Users/sollawen/projects/microNeo/internal/md/render_table.go`，打开它，改一行，`:q` 退出。

shell 提示符跳出来：

```
~/projects/microNeo/internal/md
$
```

从 playground 跳到 5 层深 + FileSelector 翻出来的目录。零中间步骤。

接下来 `git status` 在那个目录跑、`ls` 看兄弟文件、`grep` 找关联代码——我从来没离开过这个文件，shell 也没让我离开过。

---

cd 命令还在。我只是不再敲了。

---

microNeo 是开源的：[github.com/sollawen/microNeo](https://github.com/sollawen/microNeo)。一行命令安装，能当 `$EDITOR` 给 Claude Code、Yazi 之类的工具用。

