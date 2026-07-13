# fileSelector「I 标志冒泡」bug 修复方案

## 1. 现象与目标行为

被 gitignore 的实体，`I` 标志应**只贴在被 ignore 的那个条目本身**，不向上传染给任何祖先目录。

### 情形 A：单个文件被 ignore（如 `aaa/bbb/ccc/file.md`）

| 当前目录 | 目标 | 现状（bug） |
|---|---|---|
| `ccc` | `file.md` 显示 `I` | ✓ 正确 |
| `bbb` | `ccc/` **不**显示 `I` | ✗ 错误地显示 `I` |
| `aaa` | `bbb/` **不**显示 `I` | ✗ 错误地显示 `I` |

### 情形 B：整个目录被 ignore（如 `ccc/` 在 `.gitignore` 里）

| 当前目录 | 目标 | 现状（bug） |
|---|---|---|
| `ccc` 内部 | `ccc` 里每个文件都显示 `I` | ✗ 全无标志 |
| `bbb` | `ccc/` 显示 `I` | ✓ 正确 |
| `aaa` | `bbb/` **不**显示 `I` | ✗ 错误地显示 `I` |

## 2. 根因

`internal/action/fileselector_git.go` 的 `parsePorcelain` 对**所有**状态（M/U/A/D/R/I）用同一条规则——取路径顶层分量冒泡到顶层目录名。

```go
rel := strings.TrimPrefix(path, prefix)
if i := strings.IndexByte(rel, '/'); i >= 0 {
    name = rel[:i]   // 变更在子目录内 → 取顶层目录名
} else {
    name = rel
}
```

M/U/A/D/R 冒泡**无歧义**：git 里目录本身不可能「被修改」，`bbb/ M` 只能读成「bbb 子树里有修改」。

唯独 `I` 有歧义——**目录本身可以就是被 ignore 的实体**（`.gitignore` 写 `bbb/`）。`bbb/ I` 既能理解成「bbb 自己被 ignore」，也能理解成「bbb 子树里有 ignored 文件」，用户分不清。本 bug 就是后者被误读成前者。

## 3. git 输出的关键规律

实测 `git status --porcelain=v1 -z -b --ignored=traditional`（剥掉 prefix 后看 `rel`）：

```
!! node_modules/      ← 整目录被 ignore：折叠成目录，路径以 / 结尾
!! src/sub/           ← 子树全被 ignore：折叠成目录，以 / 结尾
!! src/debug.log      ← 混合目录里的单个 ignored 文件：普通文件路径
!! debug.log          ← 当前目录里的单个 ignored 文件：无 /
```

三条规律（都经实测确认）：

1. git 只在「整个目录都被 ignore」时才折叠成目录条目，且路径**以 `/` 结尾**；目录里只要有任何 tracked 文件，就**逐个列**被 ignore 的文件（普通路径，不带结尾 `/`）。这给了「整目录 ignore vs 单文件 ignore」一个可靠判据。
2. **从任何层级看同一个 ignored 实体，git 报的路径都一样**（都是仓库根相对的完整路径）。比如 `ccc` 整目录被 ignore，不管你在 `aaa` / `bbb` / `ccc` 哪一层，git 恒报 `!! aaa/bbb/ccc/`。差异完全由剥 prefix 后的 `rel` 体现。
3. **浏览进被 ignore 的目录内部时**（在 `ccc` 里看 `ccc` 自己），git 仍只报折叠的目录条目 `!! aaa/bbb/ccc/`，剥掉 prefix 后 `rel == ""`（不会枚举里面的文件）。这是情形 B 第一行的难点：单靠 `git status` 拿不到「每个文件」的粒度。

## 4. 修复规则（只改 ignored；M/U/A/D/R 纹丝不动）

只对 `!!`（ignored）记录特殊处理。设 `rel` = path 剥掉 prefix 后的相对路径：

| `rel` 形态 | 含义 | 动作 |
|---|---|---|
| `rel == ""` | 当前目录**本身**被 ignore（情形 B 第 1 行） | 置 `allIgnored`，让本目录所有条目都打 `I` |
| 不含 `/` | 当前目录里的单个 ignored 文件（情形 A 第 1 行） | `chars[rel] = 'I'` |
| 形如 `name/`（唯一一个 `/` 在末尾） | 直接子目录被 ignore（情形 B 第 2 行） | `chars[name] = 'I'` |
| 其它（含 `/` 但不是单层目录结尾） | ignored 实体在更深处 | **丢弃，不冒泡**（修掉情形 A/B 第 3 行） |

「单层目录结尾」的判据：`strings.HasSuffix(rel, "/")` 且 `rel` 里只有一个 `/`（等价于去掉末尾 `/` 后不含 `/`）。

把 6 个现场代入核对（前 3 个 = 情形 A 单文件 `file.md`；后 3 个 = 情形 B 整目录 `ccc`）：

| 当前目录 | git 原始记录 | 剥 prefix 后 `rel` | 判定 | 结果 |
|---|---|---|---|---|
| `ccc` | `…/ccc/file.md` | `file.md` | 无 `/` | `file.md`=I ✓ |
| `bbb` | `…/ccc/file.md` | `ccc/file.md` | 深处 → 丢弃 | `ccc/` 无标志 ✓ |
| `aaa` | `…/ccc/file.md` | `bbb/ccc/file.md` | 深处 → 丢弃 | `bbb/` 无标志 ✓ |
| `ccc` | `…/ccc/` | `""` | 置 allIgnored | 所有文件=I ✓ |
| `bbb` | `…/ccc/` | `ccc/` | 单层目录 | `ccc`=I ✓ |
| `aaa` | `…/ccc/` | `bbb/ccc/` | 多层 → 丢弃 | `bbb/` 无标志 ✓ |

六项全中。

## 5. 改动点

涉及两个文件、三个位置。

### 5.1 `fileselector_git.go` — `parsePorcelain`

签名加一个返回值：

```
parsePorcelain(out, prefix) (chars map[string]rune, branch string, allIgnored bool)
```

在解析出 `st == stIgnored` 之后、聚合进 `agg[name]` 之前，插入 ignored 专用分支（按 §4 表格四条判据）。注意 `allIgnored` 只在 `rel == ""` 的 ignored 记录上置真，且这种情况下 `chars` 里不会有任何 per-entry 项（git 只报了折叠目录）。

M/U/A/D/R 的解析路径**完全不动**，继续走原来的「取顶层分量冒泡」逻辑。

### 5.2 `fileselector_git.go` — `getGitStatus`

透传 `allIgnored`：

```
getGitStatus(dir) (isRepo bool, branch string, chars map[string]rune, allIgnored bool)
```

### 5.3 `fileselector.go` — `fetchGit` 合并循环

```
isRepo, branch, chars, allIgnored := getGitStatus(dir)
...
for i := range s.allEntries {
    if allIgnored {
        s.allEntries[i].gitChar = 'I'
        continue
    }
    if ch, ok := chars[s.allEntries[i].name]; ok {
        s.allEntries[i].gitChar = ch
    }
}
```

`allIgnored` 优先（当前目录整目录被 ignore 时，git 不会报任何 per-entry 项，两者实际互斥，但优先级这样写最稳）。

显示层（`gitCharStyle`、画 `I` 的循环）无需改——它只渲染 `entry.gitChar`，标志有无全由上面决定。

## 6. 测试更新（`fileselector_git_test.go`）

`TestParsePorcelain` 的 case 结构加 `allIgnored bool` 字段（零值 false 覆盖绝大多数），调用处接收第 3 个返回值并断言。

现有这条 case 断言了 bug 行为，要改：

```
name:  "ignored file → I",
input: []byte("!! build/artifact.o\x00"),
want:  map[string]rune{"build": 'I'},   // 旧（buggy）
```
→
```
name:  "ignored file in subdir → no bubble (fix)",
input: []byte("!! build/artifact.o\x00"),
want:  map[string]rune{},               // 深处单文件，不冒泡
```

补 6 条覆盖 §4 表格：

| input | prefix | want chars | allIgnored |
|---|---|---|---|
| `!! secret.md\x00` | `""` | `{secret.md: I}` | false |
| `!! node_modules/\x00` | `""` | `{node_modules: I}` | false |
| `!! aaa/bbb/ccc/\x00` | `aaa/bbb/` | `{ccc: I}` | false |
| `!! aaa/bbb/ccc/\x00` | `aaa/` | `{}` | false |
| `!! aaa/bbb/ccc/\x00` | `aaa/bbb/ccc/` | `{}` | true |
| ` M x/mod.go\x00!! x/ignored.log\x00` | `""` | `{x: M}` | false |

（最后一条验证：ignored 单文件在子目录里被丢弃，不污染 M 冒泡。）

再补一条情形 A 的核心场景（深处单文件不冒泡）：

| input | prefix | want chars | allIgnored |
|---|---|---|---|
| `!! aaa/bbb/ccc/secret.md\x00` | `aaa/` | `{}` | false |
| `!! aaa/bbb/ccc/secret.md\x00` | `aaa/bbb/ccc/` | `{secret.md: I}` | false |

注：以上都是 `parsePorcelain` 的**纯解析**单测（手造字节流）。git `--ignored=traditional` 是否真按「整目录折叠带结尾 `/`、混合目录逐文件」输出，需在真实仓库手测验证（已在本方案调研阶段用临时仓库实测确认）。

## 7. 不改的边界 / 权衡

- **`allIgnored` 与 per-entry chars 不会冲突**：实测确认 git 只在目录「全 untracked+ignored、零 tracked 文件」时才折叠成 `!! dir/`；只要目录里有 tracked 文件就不折叠，改逐个列 ignored 文件。所以 `allIgnored`（`rel==""`）触发时 `chars` 必为空，合并循环里 `if allIgnored { gitChar='I'; continue }` 不会覆盖任何 M/U/A/D/R。
- **tracked 文件在 ignored 目录里的正确语义**：若某目录被 ignore 但里面仍有 tracked 文件（先 track 后加规则），git 不折叠、走逐文件路径，tracked 的显示干净、ignored 的显示 I。这比「整目录一律全标 I」更准，实现天然如此，无需额外处理。
- M/U/A/D/R 的冒泡语义不变。它们无歧义（目录不可能是「被修改」的实体），且本 bug 只针对 ignored。
- 「整目录 ignore 且嵌套深」时（根目录看 `aaa/bbb/ccc/` 整个 ignore），`bbb/`、`aaa/` 都不再显示 `I`——这正符合用户要求（不向上传染）。只有 ignored 实体的**直接父目录**里、它自己的那个条目才显示 `I`；ignored 实体本身的内部则全显示 `I`。
- rename 的双路径解析（`R  old\x00new`）是既有行为，与本 bug 无关，不动。
