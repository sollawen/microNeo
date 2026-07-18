抱歉，直接讲。

## 命令 1：`git -C X status --porcelain=v1 -z -- .`

这条是**真正拿 git 状态数据**的。逐段拆：

| 片段 | 含义 |
|------|------|
| `git -C X` | 「假装我先 `cd X` 再跑 git」。`-C X` 等价于切到 X 目录执行，但不开新 shell。 |
| `status` | 查工作区状态：哪些文件改了 / 新增了 / 删了 / 没被 git 跟踪。 |
| `--porcelain=v1` | 输出成**机器好解析**的固定格式 v1。每条是「2 个字符状态码 + 空格 + 路径」，比如 ` M internal/a.go`（`空格M` = 已修改但没 add）。 |
| `-z` | 每条记录用 NUL 字节(`\0`)分隔，不用换行。好处：文件名里有空格、换行、特殊字符也**原样**输出，不会被引号转义搞乱。 |
| `-- .` | pathspec：只报告**当前目录(X)**子树下的变更。大仓库不会扫整个仓库，只扫 X 这棵子树。 |

输出长这样（`\0` 是 NUL）：

```
 M internal/action/a.go\0 M internal/display/d.go\0
```

这就是我们要的「哪些文件有变更」的原始数据。

## 命令 2：`git -C X rev-parse --show-prefix`

这条**不查状态**，只问一个问题：「X 这个目录在仓库树里处在什么位置？」具体是打印**X 相对仓库根的路径**，带尾斜杠：

| 你在 X = | 它输出 |
|----------|--------|
| 仓库根 | `""`（空） |
| `repo/internal/` | `internal/` |
| `repo/internal/action/` | `internal/action/` |

**为什么要它？** 因为命令 1 的状态数据里，路径是**相对仓库根**的（恒如此，我们实测过）。比如在 `internal/` 里，命令 1 给的是 `internal/action/a.go`，不是 `action/a.go`。但列表里可见的条目是 `action`、`flat.go` 这种**相对 X 的**名字。

命令 2 给的 `internal/` 就是「该剥掉的前缀」：从 `internal/action/a.go` 剥掉 `internal/` → `action/a.go` → 取顶层 `action` → 列表里 `action/` 子目录亮状态。

**所以两条命令分工**：
- 命令 1 = 拿「哪些文件有变更」的数据；
- 命令 2 = 拿「X 相对仓库根的位置」，用来把命令 1 的仓库根相对路径，换算成相对 X 的路径。

两者分开是因为 git 没法在一条 status 命令里同时告诉你「数据和参照系」——status 只给数据，参照系得单独问。`rev-parse` 极轻量（亚毫秒，只读 `.git` 位置，不碰 index、不扫文件），所以多一条进程几乎不花时间。

### 我不是很理解这个命令的作用

举例：
- 项目根目录是 ~/a1/a2/aaa
- 当前目录是 aaa/bbb/ccc, ccc下面还有子树 ccc/ddd/eee
- fileSelector肯定知道当前目录是ccc，且全路径是 ~/a1/a2/aaa/bbb/ccc
- 这时在当前目录里执行 "git -C X status --porcelain=v1 -z -- ." 于是返回了当前目录ccc及子树里面的2个文件的git状态
```
    M bbb/ccc/ddd/eee/f.txt      ← 你期望是 ddd/eee/f.txt
    M bbb/ccc/x.go               ← 你期望是 x.go
```
- 然后不就很简单了吗？
	- micro当前目录的全路径是 ~/a1/a2/aaa/bbb/ccc
	- 把读取来的每行里面，都是从项目根目录开始的路径 bbb/ccc/something
	有重叠的"bbb/ccc/都去掉，
	- 不就得到了fileSelector需要的结果了吗？


这不是挺好吗？为什么还需要 git rev-parse -show-prefix 来剥掉前缀？剥掉前缀还需要用git吗？microNeo肯定知道当前目录ccc的完整的全路径啊，直接字符串计算就行了，为什么还需要运行git命令？

而且fileSelector是想显示子树的git标志，拿到了第一步的结果后，把每行里面"当前目录ccc"前面的字符都是删掉掉，不就ok了？根本不需要关心前面是bbb还是abc还是aaeeff啊，fileSelect为什么需要知道仓库的根目录在哪里？

---

## 已修复的 git 状态显示 Bug

### Bug 1：I 标志冒泡问题（已修复）

**现象**：被 gitignore 的文件/目录，`I` 标志会错误地向上传染给祖先目录。

- 单个被 ignore 的文件（如 `aaa/bbb/ccc/file.md`），从 `bbb` 或 `aaa` 看，错误地显示 `bbb/` 或 `aaa/` 有 `I` 标志
- 整目录被 ignore（如 `.gitignore` 写 `ccc/`），从 `aaa` 看，错误地显示 `bbb/` 有 `I` 标志

**根因**：早期实现对所有状态（M/U/A/D/R/I）统一取路径顶层分量冒泡到目录名，但 `I` 有歧义——`bbb/ I` 既能理解为「bbb 自己被 ignore」，也能理解为「bbb 子树里有被 ignore 的文件」，用户分不清。

**修复**：只对 `!!`（ignored）记录特殊处理。判据规则：

| `rel` 形态（剥 prefix 后的相对路径） | 含义 | 动作 |
|---|---|---|
| `rel == ""` | 当前目录本身被 ignore | 置 `dirAllIgnored`，所有条目打 `I` |
| 不含 `/` | 当前目录里的单个 ignored 文件 | `chars[rel] = 'I'` |
| 形如 `name/`（唯一一个 `/` 在末尾） | 直接子目录被 ignore | `chars[name] = 'I'` |
| 其它（含 `/` 但不是单层目录结尾） | ignored 实体在更深处 | **丢弃，不冒泡** |

核心思想：**`I` 标志只贴在被 ignore 的实体本身，不向上传染**。

### Bug 2：进入 untracked 目录无 U 标志（已修复）

**现象**：一个目录整体是 untracked（目录内零 tracked 文件）时，从父目录看它显示 `U`，但一旦 `cd` 进这个目录或它的任意子目录，里面所有文件的 `U` 标志全部消失。

**根因**：git 对**整个 untracked 目录**有默认折叠行为——只报一条折叠记录 `?? <dir>/`，不展开内部文件。当 cwd 落在 untracked 树内时，折叠记录的路径正好等于 cwd 的 prefix，剥掉后 `rel == ""`，被跳过导致整个目录无标志。

**修复**：对 `??`（untracked）记录增加整目录折叠判据：

| `rel` 形态 | 含义 | 动作 |
|---|---|---|
| `rel == ""` | 当前目录本身整个 untracked | 置 `dirAllUntracked`，所有条目打 `U` |
| 含 `/` | 父视图看一个 untracked 子目录 | 取顶层分量冒泡（原逻辑） |
| 不含 `/` | 当前目录里单个 untracked 文件 | `chars[rel] = 'U'`（原逻辑） |

### 统一的「整目录折叠」判据

两个 bug 修复后，用统一判据处理 git 的整目录折叠行为（`internal/finder/git.go`）:

```go
// git 对 untracked（??）和 ignored（!!）整目录折叠的路径行为不同：
//   untracked：cwd 在树内任意层，git 恒报 cwd 自己（"?? <cwd>/"，path==prefix）
//   ignored：  cwd 在实体内任意层，git 恒报 ignored 树根（"!! <树根>/"，prefix 是 path 的祖先）
// 两种情况 path 都是 prefix 的祖先或等于 prefix；git 只回这一条、不报内部文件，
// 所以 cwd 整个落在折叠实体内——统一打 dirAllXxx 标志。
if strings.HasSuffix(path, "/") && strings.HasPrefix(prefix, path) {
    if indexSt == '!' && workSt == '!' {
        state = dirAllIgnored
    } else if indexSt == '?' && workSt == '?' {
        state = dirAllUntracked
    }
    continue
}
```

这个判据同时覆盖：
- cwd 在 untracked 目录根
- cwd 在 untracked 目录深处（如 `bigdir/sub/`）
- cwd 在 ignored 目录根
- cwd 在 ignored 目录深处（如 `.ruff_cache/0.12.5/`）

### 状态映射总结

| git 折叠类型 | 状态码 | `state` 值 | 显示效果 |
|---|---|---|---|
| 当前目录本身被 ignore | `!! <cwd>/` | `dirAllIgnored` | 所有条目显示 `I` |
| 当前目录本身 untracked | `?? <cwd>/` | `dirAllUntracked` | 所有条目显示 `U` |
| 正常（无折叠） | 各种记录 | `dirNormal` | 按 per-entry chars 映射 |

代码位置：`internal/finder/git.go`（`parsePorcelain` 函数 + `dirState` 枚举）、`internal/finder/session.go`（`switch state` 合并循环）。
