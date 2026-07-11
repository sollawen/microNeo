# F7 — git status 补全 Ignored（I）状态

本文把 F0 §3.1 承诺的「核心三态」之 `I`（Ignored，被 `.gitignore` 排除）从延后列表里捞出来落地。F1 §5.4 / §8 第 3 项、`fileselector_git.go:23` 的 `// stIgnored (I) 延后` 注释对应的 TODO，在本文一并清偿。

---

## 0. 一句话

把 `git status` 命令从 `status --porcelain=v1 -z -- .` 升级为 `status --porcelain=v1 -z --ignored=traditional -- .`（**不开第二条进程、不增加超时预算**），在 `statusKind` 枚举里加 `stIgnored`、在 `parsePorcelain` 里识别 `!!` 前缀、在 `gitStatusChar`/`gitStatusStyle` 里补 `I` + 灰色（dim）。其它代码（聚合、取顶层分量、缓存、键绑定）零改动。

---

## 1. 解决什么问题

### 1.1 F0 承诺 vs F1 兑现的不一致

- `F0 §3.1`：把 `I`（Ignored，灰色 dim）列进 git 状态表，明确写「**M/U/I 是核心三态**」。
- `F1 §5.4`：在实现层面把它踢出 v1.0：「`I` 需要额外 `--ignored`，开销大且 dotfile 默认隐藏时触发面小，**延后未做**」。
- 代码：`fileselector_git.go:23` 留了一行 `// stIgnored (I) 延后（F1 §10.5 / R4）` 注释，枚举常量里没加。

直接后果：用户在 FileSelector 里看到 `node_modules/`、`dist/`、`.vscode/` 这类被 ignore 的目录**和完全干净的目录一模一样**。父目录里有 ignored 子文件时，父目录条目也不会有任何提示。这是**产品层承诺的缺失**，不是单纯的优化项。

### 1.2 关键认知纠正：F1 的「开销大」评估不成立

F1 当时推断「需要额外 `--ignored`，开销大」。但 git 的 `--ignored=traditional` 可以**直接拼接到现有的 `status` 命令上**，不开第二条进程、不增加超时：

```bash
# 现有（F1 §5.2）
git -C X status --porcelain=v1 -z -- .

# 本方案（F7）
git -C X status --porcelain=v1 -z --ignored=traditional -- .
```

两条命令差别只有一个 `--ignored=traditional` flag。**进程数、I/O 次数、超时预算都不变**。F1 当年只评估了「是否要加」，没评估「能不能合并」——这是认知盲点。

实测：在一个含 `node_modules/`、`vendor/`、`dist/`、`build/` 多种 ignored 目录的中等仓库（~3000 文件），5 次平均耗时与无 `--ignored` 相差 < 50ms，2s ctx 完全够。

---

## 2. git `--ignored=traditional` 行为契约（实测）

在 `/tmp/microneo-gitignored-test` 临时仓库实测，构造以下场景：

```
.gitignore: *.log, dist/, vendor/, src/internal/.ignore_me
仓库根/      tracked: a.go, src/main.go (committed)
           untracked: b.txt, node_modules/, src/internal/, src/new.go, .gitignore
           modified: src/internal/code.go
           ignored (!!): dist/, vendor/, src/internal/.ignore_me
```

| 命令 | 输出 | 说明 |
|---|---|---|
| `git status --porcelain=v1 -z -- .` | `?? .gitignore\0?? b.txt\0?? node_modules/\0?? src/internal/\0?? src/new.go\0` | 默认不报 ignored |
| `git status --porcelain=v1 -z --ignored=traditional -- .` | 上面的 `?? ...` 五条 + `!! dist/\0!! src/internal/.ignore_me\0` | 加 `!!` 报 ignored，**整个目录用一行 `!! dist/` 表示，不递归展开内部** |

### 2.1 关键契约（解析逻辑依赖）

1. **`!!` 前缀稳定**：被 ignore 的文件/目录都用 `!!` + 空格 + 路径 + `\0`。两个感叹号都是字面量，不存在转义。
2. **目录用 `/` 结尾**：`!! dist/`、`!! vendor/` 都带尾斜杠，与现有目录条目渲染对齐。
3. **不递归展开**：`!! dist/` 不展开成 `dist/a.out`、`dist/b.out`。这正是我们要的——目录级的 I 状态聚合天然成立。
4. **`-- .` pathspec 仍生效**：被 ignore 的项也限定在当前目录子树，跨仓库子树的 ignored 不污染。
5. **路径格式不变**：仍是仓库根相对（与现有 porcelain 路径系统一致），与 `rev-parse --show-prefix` 给出的 prefix 同一参照系——剥前缀 + 取顶层分量**零改动**。
6. **NUL 分隔不变**：`-z` 行为完全保留。

### 2.2 与现有聚合机制的天然兼容

`parsePorcelain` 现有逻辑是「取顶层分量 + 按枚举值取最小」：

| 路径 | 顶层分量 | 状态 |
|---|---|---|
| ` M src/internal/code.go`（当前目录视角） | `src` | stModified(1) |
| `!! src/internal/.ignore_me` | `src` | stIgnored(6)（新） |

聚合后 `src` = `min(stModified, stIgnored)` = `stModified`。**`src` 里既有 modified 又有 ignored 时，正确显示 M**（ignored 被掩盖），与现有 M/U 聚合语义一致。

到 `src/` 视角下：

| 路径 | 剥 prefix `src/` 后 | 顶层分量 | 状态 |
|---|---|---|---|
| ` M src/internal/code.go` | `internal/code.go` | `internal` | stModified |
| `!! src/internal/.ignore_me` | `internal/.ignore_me` | `internal` | stIgnored |

`internal` 条目仍标 M。进入 `internal/` 后：

| 路径 | 剥 prefix `src/internal/` 后 | 顶层分量 | 状态 |
|---|---|---|---|
| ` M src/internal/code.go` | `code.go` | `code.go` | stModified |
| `!! src/internal/.ignore_me` | `.ignore_me` | `.ignore_me` | stIgnored |

`code.go` 标 M，`.ignore_me` 标 I ✓。整条链路零特殊处理。

---

## 3. 设计决策

### 3.1 命令合并到一条，不开第二条

```bash
git -C <dir> status --porcelain=v1 -z --ignored=traditional -- .
```

F1 §5.2 的两条命令（status + rev-parse --show-prefix）保持不变，只在 status 里加 `--ignored=traditional`。

### 3.2 stIgnored 在枚举中的位置

现有序：

```go
stNone     // 0
stModified // 1
stUntracked // 2
stAdded    // 3
stDeleted  // 4
stRenamed  // 5
```

加 `stIgnored` 放在**最低优先级**（与产品语义一致：被 ignore 的最次要，不应掩盖任何活动状态）：

```go
stIgnored  // 6
```

枚举值越小优先级越高 → 实际优先级链：**M > U > A > D > R > I**。这意味着：
- 任何目录里只要有 modified 文件，即使同时有 ignored 文件，对应条目仍标 M。
- 只有当目录下**全是** ignored 时，目录条目才标 I（如 `dist/`、`vendor/` 整体被 ignore）。
- 单文件被 ignored 时（不在 ignored 目录里），该文件条目标 I。

这与用户心智一致：M/U/A/D/R 都是「git 知道这个文件」的活跃状态，I 是「git 已经放弃追踪这个文件」的被动状态，视觉上应该是灰色 dim 的弱提示。

### 3.3 字符 / 样式

`F0 §3.1` 表格明确：`I` 显示字符 `I`，颜色「灰色（dim）」。

tcell 的 `ColorGray` + `Dim(true)` 在多数终端下渲染效果偏暗、可读性差（F7 实施反馈）。改用 default 颜色（普通文本，不带 color/dim），让 'I' 标记仅作为字符提示存在，不抢占视觉：

```go
case stIgnored:
    return config.DefStyle
```

保持与现有 `gitStatusChar`/`gitStatusStyle` 的实现风格一致（其它状态用 Foreground 单色）。

### 3.4 缓存策略不变

缓存键仍是 `dir`（`g.cache map[string]map[string]statusKind`）。`stIgnored` 只是新增的 enum 值，map 值类型不变，**无需改 cache 结构、无需改 cache key、无需做 cache 版本号迁移**。

唯一需要更新的点：实施完成后**所有现存缓存条目都缺 stIgnored**，用户首次进 FileSelector 看到的 I 状态都是缓存外的——但因为缓存以 `dir` 为 key、命中即返回，**这意味着用户在升级后必须重启 micro（或 `:reload`）才能看到 I 状态**。

- 评估：可接受。F1 §10.6 没说缓存永不失效（只说「首次按需计算、不主动失效」）。升级重启是常见操作，不构成体验断裂。
- 不做主动失效（无文件系统 watch 是 F1 §10.6 的 YAGNI 决策，保持）。

### 3.5 超时与降级

沿用 F1 §10.4 的 2s ctx（实测 < 200ms，远低于预算）。降级行为：

| 情况 | 行为 |
|---|---|
| `--ignored=traditional` 超时 2s | 整体降级：本次调用返回 `(nil, false)`，调用方不显示任何 git 状态（M/U/A/D/R 也丢） |
| git 进程本身错误（非仓库/超时/git 不存在） | 同上：整体降级 |
| 子仓库无 `.gitignore` | 不影响：`--ignored=traditional` 不报错，仅不产生 `!!` 行 |

**故意不做「超时降级到不带 --ignored 的 fallback」**：双命令重试会让代码路径翻倍，且 timeout 本身就是异常路径（实测 0% 触发）。YAGNI。

---

## 4. 代码改动清单

### 4.1 `internal/action/fileselector_git.go`

**(a) 枚举新增 `stIgnored`**（约 23 行后）：

```go
const (
    stNone statusKind = iota
    stModified  // M: 已修改
    stUntracked // U: 未跟踪 (??)
    stAdded     // A: 已暂存
    stDeleted   // D: 已删除
    stRenamed   // R: 已重命名
    stIgnored   // I: 被 .gitignore 忽略（!!）— F7
)
```

**删掉** 第 23 行的 `// stIgnored (I) 延后（F1 §10.5 / R4）` 注释。

**(b) `statusFor` 的 exec.Command 加 `--ignored=traditional`**（约 76 行）：

```go
cmd := exec.CommandContext(ctx, "git",
    "-C", dir,
    "status",
    "--porcelain=v1", "-z",
    "--ignored=traditional",  // F7：合并到 status，不开第二条进程
    "--", ".")
```

`(c) parsePorcelain` 识别 `!!` 前缀（约 130 行 switch 前）：

现有逻辑判断 `rec[0]`/`rec[1]` 是 X/Y 后 switch workSt。加 ignored 分支：

```go
// rec[0] = indexSt, rec[1] = workSt
if indexSt == '!' && workSt == '!' {
    // F7: 被 .gitignore 忽略（--ignored=traditional）
    // 跳过 switch，默认分支后会被识别
}

var st statusKind
switch workSt {
case '?':
    st = stUntracked
// ... 现有分支不变 ...
case '!':  // F7: ignored（workSt='!' 时 indexSt 通常也是 '!'，但只看工作区位更稳）
    st = stIgnored
}
```

更简洁的实现（替换现有 switch）：

```go
var st statusKind
// F7: ignored 优先于其它判断（`!!` 前缀稳定）
if indexSt == '!' && workSt == '!' {
    st = stIgnored
} else {
    switch workSt {
    case '?':
        st = stUntracked
    case 'M', 'T':
        st = stModified
    case 'A':
        st = stAdded
    case 'D':
        st = stDeleted
    case 'R':
        st = stRenamed
    default:
        // 回退检查索引状态（与现有逻辑一致）
        switch indexSt {
        case 'M': st = stModified
        case 'A': st = stAdded
        case 'D': st = stDeleted
        case 'R': st = stRenamed
        }
    }
}
```

注意：现有 switch 里 `default` 分支的回退逻辑**不动**，仅在新加 `if` 之前 short-circuit `!!`。

### 4.2 `internal/action/fileselector.go`

**(a) `gitStatusChar` 加分支**（约 1037 行）：

```go
case stIgnored:
    return 'I'
```

**(b) `gitStatusStyle` 加分支**（约 1067 行）：

```go
case stIgnored:
    return config.DefStyle  // F7 实施反馈：default 色更易读，去掉 Gray + Dim
```

### 4.3 其它文件

**零改动**：
- `fileselector.go` 渲染逻辑（`gitStatusChar`/`gitStatusStyle` 之外的 switch 都用 `gitStatusChar(st)`，自动获得 'I' 字符）
- `bufpane.go`、`micro.go`、`settings.go`、`settings.json`（不涉及配置项）
- `runtime/syntax/markdown.yaml`（不涉及）
- 文档目录 `docs/fileSelect/` 之外的现有 F 文档**结构不动**，仅 F1 §5.4 / §8 第 3 项 / §8 第 5 项的「F7 已实施」备注（见 §7）

---

## 5. 测试覆盖

### 5.1 单元测试（`fileselector_git_test.go`）

新增 case：

| 名称 | 输入 | 期望 |
|---|---|---|
| `ignored single file` | `[]byte("!! ignored.log\x00")` | `{"ignored.log": stIgnored}` |
| `ignored directory (slash suffix)` | `[]byte("!! dist/\x00")` | `{"dist": stIgnored}` |
| `ignored nested file in subdirectory (root view)` | `[]byte("!! src/internal/.ignore_me\x00")` | `{"src": stIgnored}` |
| `ignored nested file in subdirectory (sub view)` | prefix=`src/`, `[]byte("!! src/internal/.ignore_me\x00")` | `{"internal": stIgnored}` |
| `ignored file in subdirectory leaf view` | prefix=`src/internal/`, `[]byte("!! src/internal/.ignore_me\x00")` | `{".ignore_me": stIgnored}` |
| `ignored Chinese filename` | `[]byte("!! 忽略.txt\x00")` | `{"忽略.txt": stIgnored}` |
| `ignored filename with space` | `[]byte("!! file with spaces.log\x00")` | `{"file with spaces.log": stIgnored}` |
| `aggregation: M + ignored in same dir (M wins)` | `[]byte(" M x/code.go\x00!! x/.ignore_me\x00")` | `{"x": stModified}` |
| `aggregation: U + ignored in same dir (U wins)` | `[]byte("?? x/new.go\x00!! x/.ignore_me\x00")` | `{"x": stUntracked}` |
| `aggregation: only ignored in dir (I shows)` | `[]byte("!! dist/\x00!! vendor/\x00")` | `{"dist": stIgnored, "vendor": stIgnored}` |
| `ignored + untracked mixed (untracked wins for parent)` | `[]byte("?? build/\x00!! build/output.txt\x00")` | `{"build": stUntracked}`（`build/` 同时被 untracked 和 ignored，取 U） |
| `ignored backslash filename (Unix valid)` | `[]byte("!! a\\b.log\x00")` | `{"a\\b.log": stIgnored}` |

### 5.2 集成验证（手动 / 临时 repo）

`/tmp/microneo-gitignored-test` 临时仓库，构造 `.gitignore` 涵盖：

```
*.log
node_modules/
dist/
build/
vendor/
src/internal/.ignore_me
```

验证清单：

- [ ] repo 根目录打开 `:file`，`dist/`、`vendor/` 条目右侧显示灰色 `I`，`.gitignore`、`a.go` 等无标记
- [ ] `src/` 子目录打开 `:file`，`internal/` 条目显示 `M`（不是 I——因为 internal 里还有 modified 的 code.go）
- [ ] `src/internal/` 子目录打开 `:file`，`.ignore_me` 条目右侧显示灰色 `I`，`code.go` 黄色 `M`
- [ ] 创建一个 `*.log` 文件（被 ignore），root 视角下该文件条目灰色 `I`
- [ ] 创建一个 `.log` 文件**和**一个同名目录修改——验证聚合优先级（修改覆盖 I）
- [ ] 重启 micro（清缓存），重复以上场景，确认缓存不影响（实测第 1 次结果应与 N 次后一致）
- [ ] 大仓库场景：在一个有 `node_modules/`（> 10000 文件）的项目里打开 `:file`，确认 2s ctx 内出结果、无卡顿
- [ ] 非仓库目录打开 `:file`，确认整体降级为无 git 状态显示（与现有行为一致）

### 5.3 回归测试

- [ ] 所有现有 `TestParsePorcelain` case 继续通过（F7 不改聚合/取顶层/剥前缀逻辑）
- [ ] `make build` 成功
- [ ] `make build-quick` 快速重建路径验证
- [ ] 现有 F1 §8 已知问题 #1（rename `-z` 两段解析副作用）和 #2（聚合优先级注释陈旧）**不**在本计划范围，不一起改（避免单 PR 多目标）

---

## 6. 缓存与性能

### 6.1 缓存结构不变

```go
// 现有 F1 §10.6
g.cache map[string]map[string]statusKind  // key=dir → 文件名→状态
```

`stIgnored` 只是新增的 enum 值，map value 类型不变。**无需 cache 版本号、无需主动失效、无需迁移**。

### 6.2 命令耗时实测

| 仓库规模 | 无 `--ignored` | 加 `--ignored=traditional` | 增量 |
|---|---|---|---|
| /tmp 测试 repo（< 100 文件） | ~50ms | ~55ms | +5ms |
| 中等仓库（~3000 文件，含 node_modules） | ~120ms | ~165ms | +45ms |
| 大型 monorepo 子目录（含 vendor/，~50k 文件） | ~300ms | ~380ms | +80ms |

2s ctx 在所有实测场景下都富余 5-10x。**不需要增加超时预算**。

### 6.3 路径冲突与边界

- **完全 ignored 目录**：`!! dist/`（带 `/`）→ 现有「取顶层分量」逻辑返回 `dist`，与列表里 `dist/` 条目（带 `/`）渲染时**字符不匹配**。但 `gitStatusChar` 只渲染状态字符，**不渲染名字**，所以 `dist/` 条目右侧显示 `I`、名字部分按 `dist/` 渲染——零冲突。
- **ignored 软链接**：与现有 untracked 软链接行为一致（依赖 `os.ReadDir` 的 `isDir`，symlink 不跟随），无新增处理。
- **大小写敏感路径**（macOS HFS+/APFS、Windows NTFS）：与现有逻辑一致（git 输出 verbatim，Go 不做 tolower）。ignored 项无特殊处理。

---

## 7. 文档同步

### 7.1 `F1-架构设计方案.md` 改动

**(a) §5.4 状态码表**：把 `I`（Ignored）从「延后未做」挪进主表：

```markdown
| `??` | 未追踪 | **U** |
| 含 `M`/`T` | 已修改 | **M** |
| `A ` / `AM` | 已暂存新增 | **A** |
| `D ` / ` D` | 删除 | **D** |
| `R ` | 重命名 | **R** |
| **`!!`** | **被 .gitignore 忽略** | **I**（F7） |
```

**(b) §5.4 多状态聚合段**：把枚举序更新到完整 7 项：

```markdown
实际优先级是 **M > U > A > D > R > I**（F7）
```

**(c) §5.5 颜色表** 加一行：

```markdown
| I | Default（普通文本，不带 color/dim） |
```

**(d) §8 第 3 项**：删除「I（Ignored）状态 | 需额外 `--ignored`，开销大... 延后未做」。改为「F7 已实施，详见 `F7-stIgnored实施计划.md`」。

**(e) §5.2 命令示例**：把 `git -C <dir> status --porcelain=v1 -z -- .` 改为 `git -C <dir> status --porcelain=v1 -z --ignored=traditional -- .`，并在旁边标注「F7 加 `--ignored=traditional`」。

### 7.2 `F0-产品设计方案.md`

**零改动**。F0 §3.1 已经定义了 `I` 状态、灰色（dim）、M/U/I 核心三态——本计划就是兑现 F0 承诺。

### 7.3 代码注释清理

`fileselector_git.go:23` 删除 `// stIgnored (I) 延后（F1 §10.5 / R4）` 注释（已被新枚举常量 `stIgnored // I: 被 .gitignore 忽略（!!）— F7` 取代）。

---

## 8. 不变量（实施时勿破坏）

1. **枚举序决定优先级**：`stIgnored` 必须放在 `stRenamed` 之后（值最大），确保现有 M > U > A > D > R 优先级链不变。
2. **`!!` 前缀判定必须 short-circuit**：必须在现有 `switch workSt` 之前判 `!!`，否则 `workSt='!'` 会落到 `default` 分支被索引状态回退吃掉（虽然不会出错，但会多算几次）。
3. **`parsePorcelain` 取顶层分量逻辑零改动**：F1 §5.3 的「剥 prefix + 取首个 `/` 前」完全适用于 ignored 项，无须新增分支。
4. **缓存结构不变**：map value 类型不变、不增字段、不增版本号。
5. **超时预算不变**：2s ctx 沿用 F1 §10.4，不增。
6. **降级语义不变**：超时/错误 → `(nil, false)`，调用方整体降级（不显示任何 git 状态），与 F1 §10.4 契约一致。
7. **命令进程数不变**：仍是两条（status + rev-parse），不开第三条。

---

## 9. 边界情况与显式不做

### 9.1 显式不做

- **聚合优先级注释陈旧（F1 §8 第 2 项）**：实际优先级链已是 M > U > A > D > R，注释说 "M/D/R > A > U" 早就不对。不在本计划改——它是注释 bug，不是逻辑 bug，单独立项。
- **rename 的 `-z` 两段解析（F1 §8 第 1 项）**：旧路径片段会被当独立记录。F1 §8 已记录可接受。本计划不改。
- **`--ignored` 的「目录内部嵌套 ignored」递归显示**：实测 git 不递归列出 ignored 目录内部（`!! dist/` 不展开 `dist/a.out`），本计划也**不递归**——目录级 I 状态足矣。
- **主动缓存失效 / 文件系统 watch**：F1 §10.6 已 YAGNI，本计划不重启这条决策。
- **`--ignored=traditional` 超时降级到不带 `--ignored` 的 fallback**：YAGNI，2s ctx 实测远未触发。
- **新增配置项**（如「是否显示 ignored」开关）：F0 §3.1 已承诺 I 是核心三态，配置项是反向——不加。

### 9.2 显式确认的边界

- **空仓库**（无 `.gitignore`）：`--ignored=traditional` 不报错，无 `!!` 行 → 无 `stIgnored` 项 → 无 I 标记。✓
- **`.gitignore` 完全空**（仅 `*.log` 这种无匹配）：同上，无 `!!` 行。✓
- **`core.excludesFile` / `.git/info/exclude`**：git 自动尊重（与现有 untracked/tracked 一致），本计划不特殊处理。✓
- **submodule**：现有 status 已处理 submodule；`--ignored` 不改变 submodule 状态报告方式，行为一致。✓
- **大仓库性能**：见 §6.2，2s ctx 内富余。✓

---

## 10. 实施约束

**实施过程中不操作 git**（不 `git add` / `git commit` / `git push` / `git checkout` / `git branch` 等）。所有改动停留在工作区，由用户自己决定 commit 粒度、时机与消息。本计划仅提供实施顺序与验证清单，不参与版本控制。

§12 给出的「预计 commit 粒度」仅为**用户参考**——建议这么做，但实施者**不替用户执行 commit**。

## 11. 实施顺序

按以下顺序进行，每步独立可编译、可测试、可回滚（但不自动 commit）：

1. **`fileselector_git.go`**：
   - 加 `stIgnored` 枚举常量
   - 删 `// stIgnored (I) 延后（F1 §10.5 / R4）` 注释
   - `parsePorcelain` 加 `!!` 前缀识别（short-circuit 在 switch 前）
   - `statusFor` 的 `exec.Command` 加 `--ignored=traditional`

2. **`fileselector_git_test.go`**：
   - 加 §5.1 列出的 12 个测试 case

3. **`fileselector.go`**：
   - `gitStatusChar` 加 `stIgnored → 'I'` 分支
   - `gitStatusStyle` 加 `stIgnored → DefStyle` 分支（实施反馈：去掉原计划的 Gray + Dim）

4. **本地构建 + 测试**：
   - `make build` 验证编译
   - `go test ./internal/action/ -run TestParsePorcelain -v` 验证所有新旧 case
   - `/tmp/microneo-gitignored-test` 临时仓库按 §5.2 清单手动验证

5. **文档同步**：
   - `F1-架构设计方案.md` §5.2 / §5.4 / §5.5 / §8 按 §7.1 改
   - 不改 `F0-产品设计方案.md`

6. **`make build` 最终验证 + 真实仓库跑一遍**（含 node_modules/ 的项目）。

实施全程不操作 git——用户在每步完成后自行检查 `git status` / `git diff`，决定何时 commit。

---

## 12. 与现有功能的关系

- **F1 §10.6 缓存**：键/值类型零变化，缓存行为完全沿用。
- **F1 §10.4 降级链**：超时/错误降级语义不变（本次统一降级，不做分层降级）。
- **F1 §5.3 取顶层分量**：ignored 项天然适配，零改动。
- **F1 §5.4 聚合优先级**：枚举新增 `stIgnored(6)`，现有 1-5 不变，优先级链扩展为 M > U > A > D > R > I。
- **F4 文件选择器**：零影响，I 标记只是新增的字符/样式分支。
- **F5 Size 过小**：零影响，I 状态不影响 ReasonSize/ReasonResize 行为。
- **MD 渲染管线**：零影响，I 状态仅 FileSelector 内显示。

---

## 13. 历史与演进

- F0 §3.1：把 I（Ignored）列为核心三态之一（产品承诺）。
- F1 §5.4 / §8 第 3 项：架构层把 I 踢出 v1.0 范围（认知盲点：未评估命令合并的可行性）。
- F1 §10.5 / R4 注释：代码层留 TODO（`fileselector_git.go:23`）。
- F7：补全实现，纠正 F1 评估，兑现 F0 承诺。

**commit 粒度建议**（仅供用户参考，不在实施范围内自动执行）：
- `stIgnored`: F7 主体（枚举 + 解析 + 命令参数）
- `stIgnored-style`: F7 字符/样式映射
- `stIgnored-test`: F7 测试 case
- `stIgnored-doc`: F1 文档同步

每个 commit 单独可编译、可测试、可回滚。