# fileSelector「进入 untracked 目录无 U 标志」bug 修复方案

## 1. 现象与目标行为

一个目录整体是 untracked（目录内零 tracked 文件）时，从**父目录**看它，它该显示 `U`；但一旦 `cd` 进这个目录（或它的任意子目录），里面所有文件 / 子目录的 `U` 标志全部消失。

以真实场景 `webBox/static/manual/`（整个目录 untracked）为例：

| 当前目录 | 目标 | 现状（bug） |
|---|---|---|
| `webBox/static/` | `manual/` 显示 `U` | ✓ 正确 |
| `webBox/static/manual/` | `css/`、`js/`、`manifest.json`、`md_files/` 都显示 `U` | ✗ 全无标志 |
| `webBox/static/manual/md_files/` | `calitools/`、`intro.md`、`teabox-player/` 都显示 `U` | ✗ 全无标志 |

这与已修的「I 标志冒泡」bug（见 `I标志冒泡bug-方案.md`）是**完全对称**的另一面：那次漏掉了 ignored 的「整目录折叠」形态，这次漏掉了 untracked 的「整目录折叠」形态。

## 2. 根因

`internal/action/fileselector_git.go` 的 `getGitStatus` 用的是：

```
git -C <dir> status --porcelain=v1 -z -b --ignored=traditional -- .
```

没有 `-uall`。git 对**整个 untracked 目录**有默认折叠行为：目录内零 tracked 文件时，git 只报**一条折叠记录** `?? <dir>/`，**不展开**内部文件。

而 `parsePorcelain` 对 `??` 记录的处理是「取顶层分量冒泡」：

```go
rel := strings.TrimPrefix(path, prefix)
if i := strings.IndexByte(rel, '/'); i >= 0 {
    name = rel[:i]   // 变更在子目录内 → 取顶层目录名
} else {
    name = rel
}
...
if name == "." || name == "" {
    continue
}
```

问题就出在 cwd 落进 untracked 树内时 `rel` 变成空串：折叠记录的路径正好等于 cwd 的 prefix，剥掉后 `rel == ""` → `name == ""` → 被 `continue` 跳过 → 整个目录无任何标志。

## 3. git 输出的关键规律

实测可复现（临时仓库，`bigdir/` 整个 untracked，含 `bigdir/a.txt`、`bigdir/sub/b.txt`）：

```
$ cd /tmp/gittest && git init -q && git commit -q --allow-empty -m init
$ mkdir -p bigdir/sub && echo a > bigdir/a.txt && echo b > bigdir/sub/b.txt
```

从三个不同 cwd 跑同一条命令 `git status --porcelain=v1 -z -b --ignored=traditional -- .`：

| cwd | prefix（`--show-prefix`） | git 报的 `??` 记录 | 剥 prefix 后 `rel` |
|---|---|---|---|
| 仓库根 | `""` | `?? bigdir/` | `bigdir/` |
| `bigdir/` | `bigdir/` | `?? bigdir/` | `""` |
| `bigdir/sub/` | `bigdir/sub/` | `?? bigdir/sub/` | `""` |

三条规律（均经实测确认）：

1. git 只在「整个目录全 untracked、零 tracked 文件」时才折叠成 `?? dir/`，**不枚举内部文件**；一旦目录里有任何 staged / tracked 文件就改逐个列（见 §7 边界）。
2. **pathspec `-- .` 限定到 cwd**：cwd 落在 untracked 树内（任意深度），git 恒把 **cwd 自己**当作折叠的 untracked 条目来报告，路径 = cwd 相对仓库根 + `/`。所以第三行报的是 `?? bigdir/sub/`（cwd 自己），不是 `?? bigdir/`（树根）。
3. 由 2 直接推出：从 cwd 自己看这条折叠记录，记录路径**恒等于** prefix，剥掉后 `rel` **恒为空串**。

第 3 条正是「进去就全无标志」的机制——也是修复的判据。

## 4. 修复规则（只补 untracked 的整目录折叠；其余 U/A/M/D/R/I 纹丝不动）

`??` 记录设 `rel` = path 剥掉 prefix 后的相对路径：

| `rel` 形态 | 含义 | 动作 |
|---|---|---|
| `rel == ""` | 当前目录**本身**整个 untracked（cwd 落在 untracked 树内） | 置 `allUntracked`，让本目录所有条目都打 `U` |
| 含 `/` | 父视图看一个 untracked 子目录（如 `manual/`） | 取顶层分量冒泡，`chars[name] = 'U'`（**原逻辑不变**） |
| 不含 `/` | 当前目录里单个 untracked 文件 | `chars[rel] = 'U'`（**原逻辑不变**） |

注意「untracked 自身折叠」只有一种特殊形态（`rel == ""`），比 ignored 简单——ignored 还要区分单文件 / 整目录 / 深处三种。所以这里一个判据就够，无需像 ignored 那样写多分支 switch。

把三个现场代入核对（`bigdir/` 整个 untracked）：

| cwd | git 原始记录 | 剥 prefix 后 `rel` | 判定 | 结果 |
|---|---|---|---|---|
| 仓库根 | `?? bigdir/` | `bigdir/` | 含 `/` → 顶层分量冒泡 | `bigdir`=U ✓ |
| `bigdir/` | `?? bigdir/` | `""` | 置 allUntracked | 所有条目=U ✓ |
| `bigdir/sub/` | `?? bigdir/sub/` | `""` | 置 allUntracked | 所有条目=U ✓ |

三项全中。真实场景 `webBox/static/manual/` 同理（`manual`、`manual/md_files` 两个 cwd 都触发 `allUntracked`）。

## 5. 改动点

涉及两个文件、三个位置，全部与 `allIgnored` **对称**。

### 5.1 `fileselector_git.go` — 新增枚举 + `parsePorcelain`

`allIgnored` / `allUntracked` 描述的是「整个当前目录」的状态，互斥且以后还可能冒出别的 allXXX，用一个枚举代替散落的 bool。在 `fileselector_git.go`（紧挨已有的 `statusKind`）定义：

```go
// dirState 表示「当前目录整体」的状态（区别于单文件的 statusKind）。
// parsePorcelain 据折叠记录判定，fetchGit 据它给本目录所有条目统一打标志。
type dirState uint8

const (
    dirNormal      dirState = iota // 正常：按 per-entry chars 映射
    dirAllIgnored                  // 当前目录整个被 ignore（git 折叠成 !! cwd/）
    dirAllUntracked                // 当前目录整个 untracked（git 折叠成 ?? cwd/）
)
```

`parsePorcelain` 用一个 `dirState` 返回值替代原本的 `allIgnored bool`（顺带把 ignored 那行也并进来，不再单列 bool）：

```
parsePorcelain(out, prefix) (chars map[string]rune, branch string, state dirState)
```

ignored（`!!`）记录里 `core == ""` 处，由 `allIgnored = true` 改为 `state = dirAllIgnored`；untracked（`??`）记录于赋 `st = stUntracked` 之前判 `rel == ""`：

```go
case '?':
    if rel == "" {
        state = dirAllUntracked
        // st 保持 stNone：cwd 自身折叠，下面 name=="" 会跳过，不进 agg
    } else {
        st = stUntracked
    }
```

判据放在这里（而非更早）：`rel` 要先由上面的 `TrimPrefix` 算出来；且和 ignored 的 `core == ""` 判定在结构上左右对称，可读性最好。

`state` 只在「cwd 自身被折叠」时偏离 `dirNormal`，此时 `chars` 里不会有任何 per-entry 项（git 只报了折叠目录）。`dirAllIgnored` 与 `dirAllUntracked` 互斥（一个目录不可能既全 ignored 又全 untracked），枚举只被赋值一次，无覆盖风险。

### 5.2 `fileselector_git.go` — `getGitStatus`

透传 `state`（替掉原本的 `allIgnored bool`）：

```
getGitStatus(dir) (isRepo bool, branch string, chars map[string]rune, state dirState)
```

### 5.3 `fileselector.go` — `fetchGit` 合并循环

用一个 `switch state` 收口，`dirNormal` 落到 per-entry：

```go
isRepo, branch, chars, state := getGitStatus(dir)
...
for i := range s.allEntries {
    switch state {
    case dirAllIgnored:
        s.allEntries[i].gitChar = 'I'
        continue
    case dirAllUntracked:
        s.allEntries[i].gitChar = 'U'
        continue
    }
    if ch, ok := chars[s.allEntries[i].name]; ok {
        s.allEntries[i].gitChar = ch
    }
}
```

枚举各值互斥（一个目录不可能既全 ignored 又全 untracked），`switch` 比连串 `if` 更贴语义，也方便以后再加 `dirAllXXX` 分支。

显示层（`gitCharStyle`、画 `U` 的循环）无需改——`'U'` 早已走 `diff-added` 色，标志有无全由上面决定。

## 6. 测试更新（`fileselector_git_test.go`）

`TestParsePorcelain` 的 case 结构把 `allIgnored bool` 字段换成 `state dirState`（零值 `dirNormal` 覆盖绝大多数），调用处接收对应返回值并断言。现有 ignored case 的 `allIgnored: true` 改成 `state: dirAllIgnored`；其余全部行为不变。

补三条覆盖 §4 表格（untracked 整目录折叠）：

| input | prefix | want chars | state |
|---|---|---|---|
| `?? bigdir/\x00` | `""` | `{bigdir: U}` | dirNormal |
| `?? bigdir/\x00` | `bigdir/` | `{}` | dirAllUntracked |
| `?? bigdir/sub/\x00` | `bigdir/sub/` | `{}` | dirAllUntracked |

再补一条验证「untracked 树内深处」从树根视角仍按原逻辑冒泡、不误触 dirAllUntracked：

| input | prefix | want chars | state |
|---|---|---|---|
| `?? bigdir/sub/b.txt\x00` | `""` | `{bigdir: U}` | dirNormal |

（这条验证的是：git 对非折叠 untracked（目录内有 staged 文件所以逐个列）从根视角仍按原逻辑冒泡到顶层目录名 `bigdir`；同时 cwd 在根时 `rel != ""`，不触发 dirAllUntracked，state 保持 `dirNormal`——与「整目录折叠」的 `rel==""` 情形形成对称区分，见 §7 边界讨论。）

注：以上都是 `parsePorcelain` 的**纯解析**单测（手造字节流）。git 默认（无 `-uall`）是否真按「整目录折叠成 `?? dir/`、有 staged 文件就逐个列」输出，需在真实仓库手测验证（本方案调研阶段已用临时仓库实测确认）。

## 7. 不改的边界 / 权衡

- **`dirAllUntracked` 与 per-entry chars 不会冲突**：git 只在目录「全 untracked、零 tracked 文件」时才折叠成 `?? dir/`；一旦目录里有任何 staged / tracked 文件就不折叠，改逐个列（实测：对 `manual/` 做 `git add` 后，cwd=`manual/` 下 git 逐个报 `A  webBox/static/manual/css/manual.css` 等，剥 prefix 后 `rel = css/manual.css`，正确映射 `css`=A、`js`=A、`manifest.json`=A、`md_files`=A，state 保持 `dirNormal`）。所以 `dirAllUntracked`（`rel==""`）触发时 `chars` 必为空，合并循环里该分支不会覆盖任何 M/A/D/R/I。
- **部分 staged 部分还 untracked 的混合目录**天然正确：git 不折叠（有 staged 文件），逐个报 `A` / `??`，cwd 在里面时各自剥 prefix 后映射，staged 的显示 A、未加的显示 U，互不干扰。
- **`dirAllUntracked` 与 `dirAllIgnored` 互斥**：一个目录要么在 `.gitignore` 命中（ignored），要么未跟踪（untracked），不会同时。两者判据分属 `!!` 与 `??`，枚举上各占一个值，互不覆盖。
- M/A/D/R/I 的解析路径**完全不动**，继续走原逻辑。本 bug 只针对 untracked 整目录折叠这一种漏网形态。
- rename 的双路径解析（`R  old\x00new`）是既有行为，与本 bug 无关，不动。
- 与「I 标志冒泡」修复在结构上**完全对称**：那次用 `allIgnored` 处理 `!!` 的 `rel==""`，这次用枚举把两种「整目录折叠」状态统一起来——`dirAllIgnored` 处理 `!!` 的 `rel==""`、`dirAllUntracked` 处理 `??` 的 `rel==""`。判据与改动点一一对应，以后再要 allXXX 只需加一个枚举值 + 一个 switch 分支。

## 8. 对称遗留：ignored 实体深处的 I 标志缺失

上次（§1-§7）修了 untracked 的整目录折叠，但发现 ignored 有一个对称 bug 没有覆盖到。

### 8.1 根因：git 对两种实体的折叠路径行为不同

关键差异已实测确认：

| 实体类型 | cwd 进树内深处时 git 恒报 |
|---|---|
| **untracked** | cwd **自己**（`?? <cwd>/`，path == prefix） |
| **ignored** | ignored **树根**（`!! <树根>/`，prefix 是 path 的祖先） |

实测 1（真实目录 `~/Inception/.ruff_cache/`，整目录 ignored）：
```
cwd=.ruff_cache            prefix=[.ruff_cache/]          git 报: !! .ruff_cache/   → rel=""       → dirAllIgnored ✅
cwd=.ruff_cache/0.12.5     prefix=[.ruff_cache/0.12.5/]   git 报: !! .ruff_cache/   → rel=".ruff_cache/" → 孤儿 ❌
```
实测 2（临时仓库 `/tmp/igtest`，`igroot/` 整 ignored，含 `igroot/mid/deep/f3.txt`）：
```
cwd=igroot/mid/deep        prefix=[igroot/mid/deep/]      git 报: !! igroot/        → 孤儿 ❌
```

untracked 用 `rel == ""` 判据覆盖了（cwd == 折叠路径），但 ignored 进深处时 path 是树根、prefix 比 path 长，`rel` 变成树根路径（如 `.ruff_cache/`），`name` 算成树根名（如 `.ruff_cache`），但 cwd 里没有这个条目——孤儿，导致全不显示 I。

### 8.2 统一「整目录折叠」判据

把两种折叠行为统一为一个判据，放在 per-entity 逻辑之前：

```go
// path 是目录（以 / 结尾）且是 cwd（prefix）的祖先或自身 → cwd 整个落在折叠实体内。
// untracked 时 git 恒报 cwd 自己、ignored 时 git 恒报 ignored 树根，
// 两种情况 path 都是 prefix 的祖先或等于 prefix；git 只回这一条、不报内部文件，
// 所以 cwd 所有条目统一打 dirAllXxx 标志。
if strings.HasSuffix(path, "/") && strings.HasPrefix(prefix, path) {
    if indexSt == '!' && workSt == '!' {
        state = dirAllIgnored
    } else if indexSt == '?' && workSt == '?' {
        state = dirAllUntracked
    }
    continue
}
```

- `path` 以 `/` 结尾排除普通文件（文件名不可能含 `/`）
- `strings.HasPrefix(prefix, path)` 覆盖两种情况：
  - path == prefix（cwd 是 untracked 实体根，untracked 的折叠路径就是 cwd 自己）
  - prefix 是 path 的后代（cwd 在 ignored 实体的深处，ignored 的折叠路径是树根）

这个统一判据**完全兼容**现有所有测试：path == prefix 时 `HasPrefix(prefix, path)` 也为真，所以原来的 `rel==""` / `core==""` 判据被它吸收，可以删掉。

精简后的 per-entity 逻辑：
- ignored：只剩「单层冒泡 / 深处丢弃」
- untracked：只剩 `st = stUntracked`（`rel == ""` 分支被吸收）

### 8.3 新增测试用例

| input | prefix | want chars | state |
|---|---|---|---|
| `!! igroot/\x00` | `igroot/mid/` | `{}` | dirAllIgnored |
| `!! igroot/\x00` | `igroot/mid/deep/` | `{}` | dirAllIgnored |
| `!! .ruff_cache/\x00` | `.ruff_cache/0.12.5/` | `{}` | dirAllIgnored |

同时补齐 §6 遗漏的 untracked case（之前只修了折叠判据本身，测试 case 漏填了）：
| input | prefix | want chars | state |
|---|---|---|---|
| `?? bigdir/\x00` | `""` | `{bigdir: U}` | dirNormal |
| `?? bigdir/\x00` | `bigdir/` | `{}` | dirAllUntracked |
| `?? bigdir/sub/\x00` | `bigdir/sub/` | `{}` | dirAllUntracked |
| `?? bigdir/sub/b.txt\x00` | `""` | `{bigdir: U}` | dirNormal |

