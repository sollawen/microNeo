# F3 · 退出时同步 shell 工作目录 设计方案

**状态**：草案（待评审）
**依据**：源码 `cmd/micro/micro.go`（`exit()` 约 277–289 行、`InitFlags` 的 `flag` 定义 33–41 行）+ `internal/action/tab.go:223`（`MainTab`）/ `tab.go:395`（`CurPane`）+ `internal/action/command.go:265`（`:cd` → `os.Chdir`）+ 用户 `~/.zshrc` 的 `y()` 函数
**前置**：无。本功能是独立的 shell 集成层，不依赖 FileSelector（F1）、不依赖鼠标支持（F2）。
**交付**：microNeo 退出时把「最后工作目录」写入一个约定路径的临时文件；配套提供 shell 函数（`mn`）在读到该文件后 `cd` 过去，实现「退出 microNeo 后 shell 自动停在 microNeo 最后的目录」。
**原生侵入**：极小。改动仅 `cmd/micro/micro.go` 两处（新增 flag 定义 + `exit()` 内写文件数行），不改任何 `internal/` 逻辑、不改屏幕/事件/渲染管线。

---

## 1. 背景与目标

### 1.1 yazi 的做法

用户 `~/.zshrc` 里的 `y()`：

```zsh
function y() {
    local tmp="$(mktemp -t "yazi-cwd.XXXXXX")" cwd
    command yazi "$@" --cwd-file="$tmp"
    IFS= read -r -d '' cwd < "$tmp"
    [ "$cwd" != "$PWD" ] && [ -d "$cwd" ] && builtin cd -- "$cwd"
    rm -f -- "$tmp"
}
```

效果：在 yazi 里切换目录、退出后，shell 的当前目录自动停在 yazi 最后所在目录。

### 1.2 为什么 microNeo 也想要

microNeo 是编辑器，但用户的真实工作流经常是「编辑器 ⇄ shell」来回切：在 microNeo 里编辑完一个文件，退出后想在 shell 里 `git status` / `ls` / `grep` 这个文件所在目录。如果退出后 shell 还停在启动 microNeo 时的目录，用户得手动 `cd` 到刚编辑的文件所在目录——尤其是用 FileSelector 在目录树里翻找打开的文件，路径往往很深，手动 cd 很烦。

「退出后 shell 自动跟过去」能消除这一摩擦，和 yazi 的体验对齐。

### 1.3 核心约束：子进程不能改父进程的 cwd

这是 OS 基本约束：每个进程的 cwd 是独立的，子进程（microNeo）无法直接修改父进程（shell）的 cwd。任何「退出后 shell 自动 cd」的方案都必须借助某种 IPC，让 shell 在子进程退出后自行执行 `cd`。

yazi 的方案——**临时文件做 IPC**——是最轻、最可移植的做法（无需 shell hook、无需 named pipe、无需 signal）。本方案照搬此机制。

---

## 2. 机制总览

两端协作，shell 端是主动方：

```
shell 端（mn 函数）                     microNeo 端
─────────────────                     ─────────────
1. mktemp 建空临时文件 $tmp
2. microneo --cwd-file="$tmp"
   ──────────────────────────▶        3. flag.Parse() 记下 --cwd-file 路径
                                      4. 正常编辑生命周期……
                                         （:cd 会改进程 cwd；
                                          FileSelector 选文件会换 buffer；
                                          这些都影响「最后目录」语义，见 §3）
                                      5. 用户退出 → 进入 exit(rc)
                                      6. 算出 lastWorkingDir()
                                      7. 写入 $tmp（覆盖空文件）
   ◀──────────────────────────
8. read $tmp 拿到 cwd                 （os.Exit，进程结束）
9. [ cwd != $PWD ] && [ -d cwd ] → cd
10. rm $tmp
```

**契约**：
- shell 与 microNeo 约定的唯一接口是 `--cwd-file <path>` 这一个命令行参数 + 该 path 指向的一个文件。
- 文件内容由 microNeo 在**退出前**写入一行（无换行符也可，shell 用 `read -r -d ''` 读全文件）绝对路径。
- 文件路径由 shell 决定（`mktemp`），microNeo 不关心它在哪、叫什么。

**opt-in 原则**：`--cwd-file` 不传 = 功能完全不启用（`exit()` 里跳过写文件，零开销、零副作用）。直接 `microneo`、`alias edit='microneo'` 的现有用户感知不到此功能存在。

---

## 3. 「最后目录」的语义

**规则**：退出时取最后活跃 pane 的 buffer 所在目录——即 `MainTab().CurPane().Buf.AbsPath` 的父目录（`filepath.Dir`）。只有这一个来源，不做 fallback。

不取其它来源：
- **不取 `os.Getwd()`**：`:cd` 命令改的是进程 cwd，但用户要的是「最后那个 pane 里的目录」，与 `:cd` 无关。
- **不取 FileSelector 的 `currentDir`**：那是短暂浏览状态；用户在 selector 里选中文件换入后，本规则已自动指向该文件目录，无需另取。

**边界**：最后活跃 pane 的 buffer 无 `AbsPath`（无名空 buffer、Help、Log、Scratch 等）时取不到目录 → 不写文件，shell 端 `[ -d "$cwd" ]` 自然 skip、cwd 保持不变。这是安全失败，不兜底。

**触发时机**：仅在 microNeo 进程退出时算一次（§5 的 `exit()` 是唯一汇聚点）。关闭单个 pane / tab / buffer、FileSelector 的 quit selector 选「退出」等中间环节都不触发——只有整个程序真正结束那一次。

**退出程序的两条路径**（microNeo 默认**没有「一键退出整个程序」的操作**）：`Ctrl-q` / `:quit` 在多 pane 时只关闭当前 pane（`ForceQuit` → `Unsplit`），程序继续运行。真正退出整个程序只有两种走法：
1. **逐个关闭**：一路 `Ctrl-q` 把 pane 逐个关掉，当只剩单 pane 单 tab 时，最后一次 `Ctrl-q` 才触发 `runtime.Goexit` → `exit()`。此时 `MainTab().CurPane()` 是最后剩下的那个 pane。
2. **`QuitAll`**（底层 action 存在，但默认未绑快捷键 / 未注册命令，需用户手动绑）：一次性关掉所有 pane/tab → `runtime.Goexit` → `exit()`。此时 `MainTab().CurPane()` 是退出瞬间的焦点 pane。

两条路径下 `lastWorkingDir()` 都成立——`exit()` 时 `MainTab().CurPane()` 总能返回一个有效 pane。

### 3.1 行为表

| 场景 | 最后活跃 pane 的 buffer | 写入的目录 |
|---|---|---|
| 打开 `~/proj/src/a.go` 后退出 | `a.go` | `~/proj/src` |
| 多 pane，逐个 `Ctrl-q` 关到最后 | 最后剩下的 pane | 那个 pane 的 buffer 父目录 |
| 多 pane，`QuitAll` 退出（默认未绑定，焦点在 `b.go`） | `b.go` | b.go 的父目录 |
| FileSelector 选中 `c.go` 换入后退出 | 换入的 `c.go` | c.go 的父目录 |
| FileSelector 浏览后 Esc 取消后退出 | 原 buffer | 原 buffer 的父目录 |
| 启动后没开文件就退出 | 无名空 buffer | 不写（shell cwd 不变） |
| 退出时活跃 pane 是 Help/Log | Help/Log | 不写（shell cwd 不变） |

---

## 4. 实现方案

### 4.1 microNeo 端

#### 4.1.1 新增 flag 定义

`cmd/micro/micro.go` 的 `InitFlags()` 里，与现有 flag 并列：

```go
flagCwdFile = flag.String("cwd-file", "",
    "Write the working directory to this file on exit (for shell integration)")
```

#### 4.1.2 新增 `lastWorkingDir()` 辅助函数

放在 `cmd/micro/micro.go`（`exit` 附近），独立小函数，便于单测：

```go
// lastWorkingDir 返回退出时写回给 shell 的目录：
// 最后活跃 pane 的 buffer 所在目录。取不到（无文件 buffer）返回 ""。
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

（依赖 `path/filepath`，micro.go 已 import，无需新增。）

#### 4.1.3 在 `exit()` 集成写文件

修改 `exit(rc int)`（micro.go 约 277–289 行）。**关键时机**：在 `buffer.OpenBuffers` 的 Fini 循环**之前**算 `lastWorkingDir()`——因为 `b.Fini()` 可能清理 buffer 状态，要在对象还完整时取快照。写文件动作本身放在 `screen.Fini()` 之后、`os.Exit` 之前（写普通文件是裸 syscall，不依赖屏幕）：

```go
func exit(rc int) {
    // 在 buffer Fini 之前快照「最后目录」，此时 buffer.AbsPath 仍完整
    cwdForShell := ""
    if *flagCwdFile != "" {
        cwdForShell = lastWorkingDir()
    }

    for _, b := range buffer.OpenBuffers {
        if !b.Modified() {
            b.Fini()
        }
    }

    if screen.Screen != nil {
        screen.Screen.Fini()
    }

    // shell 集成：把最后目录写回 shell 约定的临时文件
    if *flagCwdFile != "" && cwdForShell != "" {
        _ = os.WriteFile(*flagCwdFile, []byte(cwdForShell), 0600)
    }

    os.Exit(rc)
}
```

要点：
- `*flagCwdFile == ""` 时（未传参）整段跳过，零开销。
- `os.WriteFile` 的错误**静默丢弃**（`_ =`）：退出路径上不该因集成功能报错打断正常退出；shell 端读到空文件会自然 skip（见 §4.2 的 `[ -d "$cwd" ]` 保护）。
- 权限 `0600`：临时文件只属主读写，对齐 `mktemp` 默认权限，不泄露路径给同机其它用户。
- 写入内容**不带换行**：对齐 yazi 的 `read -r -d ''`（读全文件到 NUL），带不带换行都能正确解析。

### 4.2 shell 端

`~/.zshrc` 新增（和现有 `alias edit='microneo'` 并存）：

```zsh
function mn() {
    local tmp="$(mktemp -t "microneo-cwd.XXXXXX")" cwd
    command microneo "$@" --cwd-file="$tmp"
    IFS= read -r -d '' cwd < "$tmp"
    [ "$cwd" != "$PWD" ] && [ -d "$cwd" ] && builtin cd -- "$cwd"
    rm -f -- "$tmp"
}
```

bash 用户把 `mktemp -t "microneo-cwd.XXXXXX"` 换成 `mktemp -t microneo-cwd`（bash/macOS 的 mktemp 模板语法不同，按各自平台写）。

**两道安全闸**（照搬 yazi）：
- `[ "$cwd" != "$PWD" ]`：目录没变就不 cd（省一次无意义 builtin cd）。
- `[ -d "$cwd" ]`：microNeo 写了非法路径（或文件为空、读取失败）时不会 cd 到一个不存在的目录——shell 不会因为本功能报错。

---

## 5. 退出时序与写入时机

为什么把「算目录」和「写文件」拆开两步：

```
exit(rc) 调用 →
  ├─ [1] 算 lastWorkingDir()      ← 此时 OpenBuffers 还没 Fini，AbsPath 完整
  ├─ [2] Fini 各 modified=false 的 buffer
  ├─ [3] screen.Screen.Fini()     ← 屏幕资源释放
  ├─ [4] os.WriteFile(cwdForShell) ← 裸 syscall，不依赖屏幕
  └─ [5] os.Exit(rc)
```

- [1] 必须在 [2] 之前：经验上 `buffer.Fini()` 会重置部分字段，为防 `AbsPath` 被清，先快照。
- [4] 必须在 [3] 之后：写文件是普通 syscall，与屏幕无关，但放在 `screen.Fini` 后更干净（确保终端已恢复正常状态，避免极端情况下写文件时的 stdout/stderr 干扰终端）。
- [4] 在 [5] 之前：`os.Exit` 不跑 deferred，必须显式在它之前完成写文件。

**非 `exit(rc)` 路径？** 全仓库检查确认 `exit()` 是 microNeo 所有退出路径（正常 `defer exit(0)`、`:quit`、`Ctrl-q`、各种错误退出 `exit(N)`）的唯一汇聚点，无第二条 `os.Exit` 调用。集成在此一处即可全覆盖。

---

## 6. 边界与降级

| 情况 | 行为 |
|---|---|
| 启动时未传 `--cwd-file` | 整段逻辑跳过，零副作用（现有用户感知不到） |
| `--cwd-file` 指向不可写路径 | `os.WriteFile` 失败 → 静默丢弃，shell 端读到空文件 → skip cd |
| `lastWorkingDir()` 返回 `""`（最后活跃 pane 无文件 buffer） | 不写文件，shell 端 skip |
| shell 端临时文件读取失败 / 内容不是目录 | `[ -d "$cwd" ]` 闸住，不 cd |
| 多 pane 且活跃 pane 是 Help/Log 等无路径 buffer | 不写文件，shell 端 skip（§3 边界） |
| microNeo 崩溃（panic） | `defer exit(0)` 不执行，文件未写 → shell skip，**不会 cd 到错地方** |
| 用户用 `kill -9` 杀进程 | 同上，文件未写，shell 安全 skip |
| shell 启动 microNeo 后切到别的 shell 进程 | 该 shell 不是父进程，不读文件，无影响 |

崩溃/被杀时「文件未写」是**安全失败**：shell 读到空文件 → `[ -d "" ]` 失败 → 不 cd。绝不会因为本功能把 shell 带去错误目录。

---

## 7. 原生侵入分析

| 文件 | 改动 | 行数 |
|---|---|---|
| `cmd/micro/micro.go` | `InitFlags` 加 1 个 `flag.String` + 新增 `lastWorkingDir()` 函数 + `exit()` 内 3 行（算+写） | ~15 行 |
| `internal/*` | **零改动** | 0 |
| `runtime/*` | **零改动** | 0 |
| `Makefile` / `go.mod` | **零改动** | 0 |

microNeo 自有文件全不碰。`internal/action/tab.go` 的 `MainTab()` / `CurPane()` 是现成导出函数，直接调用。`internal/buffer` 的 `Buffer.AbsPath`、`Buffer.Fini()` 都是现成 API。

**与 F1（FileSelector）的关系**：功能上独立——没有 FileSelector 也能用（只要用户打开过文件，目录就能取到）。但实际体验上互补：FileSelector 让用户在目录树里翻找文件、本功能让退出后 shell 跟到该文件处。两者无代码依赖。

**与 F2（鼠标）的关系**：完全无关。

---

## 8. 测试计划

### 8.1 microNeo 端单测（`cmd/micro/micro_test.go`）

`lastWorkingDir()` 依赖全局 `action.MainTab()`，不便直接单测。拆出只做字符串变换的 `resolveLastDir` 便于注入：

```go
// 可测版本：只做 AbsPath → 父目录的变换
func resolveLastDir(bufAbsPath string) string {
    if bufAbsPath != "" {
        return filepath.Dir(bufAbsPath)
    }
    return ""
}
```

`lastWorkingDir()` 只是 `resolveLastDir(取活跃 buf.AbsPath)` 的薄包装。测 `resolveLastDir`：

| 用例 | 输入 | 期望 |
|---|---|---|
| 有 AbsPath | `"/a/b/c.go"` | `"/a/b"` |
| AbsPath 空 | `""` | `""` |
| 根目录文件 | `"/a.go"` | `"/"` |

### 8.2 `exit()` 集成测试（手动 / e2e）

写一个临时集成测试：临时设 `flagCwdFile` 指向某路径 → 触发 `exit(0)`（子进程方式）→ 断言文件内容。因 `exit` 调 `os.Exit` 难直接测，建议用「编译一个测试 binary + 子进程 + 检查文件」的 e2e 写法，或手工验证（见 §8.3）。

### 8.3 手工验证清单

实现后逐项过：

1. `mn somefile.go` → 编辑 → `:q` 退出 → shell cwd 应停在 `somefile.go` 父目录
2. `mn`（不带文件）→ 直接 `:q` → shell cwd 不变（活跃 pane 是无名空 buffer，不写文件）
3. `mn` → `:cd /tmp`（不改活跃 buffer）→ `:q` → shell cwd 不变（:cd 不影响本功能）
4. `mn a.go` → FileSelector（`Ctrl-o`）选别处 `b.go` → `:q` → shell cwd 应停在 `b.go` 父目录
5. `mn a.go` → FileSelector 浏览后 Esc → `:q` → shell cwd 应停在 `a.go` 父目录（取消不影响）
6. 直接 `microneo`（不带 `--cwd-file`）→ 退出 → shell cwd 不变（opt-in 验证）
7. `mn` → 编辑中 `kill -9` → shell cwd 不变（崩溃安全）

### 8.4 回归

跑 `go test ./cmd/micro/... ./internal/action/...` 确认无回归。因改动集中在 `cmd/micro/micro.go` 的 `exit()`，主要回归面是「退出流程是否仍正常」——重点测 modified buffer 的存盘提示流程没被破坏（顺序：算目录 → Fini → screen.Fini → 写文件 → Exit，存盘提示在 `exit` 之前的 QuitNeo/h.Quit 路径，不在 exit 内，不受影响）。

---

## 9. 未决问题 / 决策点

### 9.1 shell 函数名：`mn` vs 改造 `edit`

用户 `~/.zshrc` 已有 `alias edit='microneo'`。两种集成方式：

| 方案 | 做法 | 优点 | 缺点 |
|---|---|---|---|
| **A（推荐）** | 新增 `mn()` 函数，`edit` alias 不动 | 与 yazi 的 `y()` 心智一致；不破坏不想要 cd 行为的用户；可并存 | 多记一个命令 |
| B | 把 `edit` 改成 `edit()` 函数，内部带 `--cwd-file` | 一个命令通吃 | 改变现有 `edit` 语义，有人可能不想要自动 cd |

**推荐 A**：默认 opt-in，最安全。文档交付时给出 A 的函数体，用户自行决定是否替换 `edit`。

### 9.2 `:cd` 不影响本功能

`:cd` 命令改的是进程 cwd（`os.Chdir`），但本功能只看最后活跃 pane 的 buffer 目录，不读 `os.Getwd()`。所以无论用户是否 `:cd`、`:cd` 到哪，退出时写入的都是 buffer 所在目录。这是 §3 的明确设计选择：「最后那个 pane 里的目录」与 `:cd` 无关。

### 9.3 多 tab 场景下的「活跃」

`MainTab().CurPane()` 取的是当前 tab 的活跃 pane。若用户在 tab1 编辑 `a.go`、tab2 编辑 `b.go`，退出时谁算「最后」？取决于退出瞬间焦点在哪个 tab。这与「最后操作的面板」直觉一致，暂不特殊处理。若日后有「记住所有 tab」的需求再议。

### 9.4 `--cwd-file` 是否写相对路径

`lastWorkingDir()` 返回绝对路径（`filepath.Dir(AbsPath)`，AbsPath 天然绝对）。shell 端 `cd` 绝对路径无歧义。**不写相对路径**，避免歧义。

---

## 10. 命名与约定汇总

| 项 | 约定 | 备注 |
|---|---|---|
| 命令行参数 | `--cwd-file <path>` | 与 yazi 完全同名，降低心智成本 |
| 文件内容 | 一行绝对路径，无换行符要求 | shell `read -r -d ''` 读全文件 |
| 文件权限 | microNeo 写时 `0600`；shell `mktemp` 默认也是 0600 | 防同机用户窥视 |
| shell 函数名 | `mn`（推荐） | 与 `y()` 平行 |
| 临时文件前缀 | `microneo-cwd.XXXXXX` | 便于排查（`ls /tmp` 一眼认出） |
| 目录来源 | 最后活跃 pane 的 buffer 父目录（单一来源，无 fallback） | §3 |
| opt-in 标志 | 不传 `--cwd-file` = 完全不启用 | §2 |

---

## 11. 与其它 F 系列的关系

| 文档 | 关系 |
|---|---|
| F0（产品设计） | 无交集。F0 讲 FileSelector 交互，本功能不涉及 FileSelector 内部 |
| F1（架构设计） | 无代码交集。本功能在 `cmd/micro/`，F1 全在 `internal/action/`。体验上互补：F1 的 FileSelector 让用户翻找文件，本功能让退出后 shell 跟过去 |
| F2（鼠标支持） | 无交集 |

本功能是 microNeo **第一个** shell 集成特性，也是 microNeo **第一个** 跨进程契约（程序 ↔ shell）。若将来还有其它「退出时把状态回传 shell」的需求（如退出码语义、最近文件列表回传），本方案建立的 `--cwd-file` 模式可作为范式复用。
