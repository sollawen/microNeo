# F1c · FileSelector 子目录 Git 状态透传 修复方案

**状态**：草案（待评审）
**依据**：F1 §10.2（porcelain 机制）+ F1b T2（git 子模块）+ 源码现状（`fileselector_git.go`）+ git 实测
**前置**：F1b 已落地、`:file` git 状态对**当前目录里的文件**已正常显示。
**交付**：让 FileSelector 列表里的**子目录**也能显示 git 状态——子目录内（任意深度）有变更文件时，该子目录条目亮起对应状态（M/U/A/D/R）。**根目录与任意层子目录都要对。**
**原生侵入**：零。改动仅限 microNeo 自有文件 `fileselector_git.go` + 其单测。

---

## 1. 背景与现象

F1b 落地后，git 状态对**当前目录里的文件**显示正常，但**子目录条目永远空白**。深入排查发现其实是**同一个根因的两种表现**：

| 浏览位置 | 现象 |
|---|---|
| 仓库根 | 文件亮状态（✓），子目录全空（✗） |
| 进了子目录 | 文件**和**子目录都全空（✗） |

用户要求：子目录的子目录的子目录里有个文件 modified，顶层那个子目录也要亮 M——即状态要按目录树**向上透传一层**到当前列表里实际可见的条目。

本方案只修这一个 bug，不重构 git 子模块架构（F1 §10 / F1b T2 的接口、缓存、异步时序、降级链一律不动）。

---

## 2. 根因：`--porcelain` 恒输出「仓库根相对」路径

bug 在 `internal/action/fileselector_git.go:116` 的 `parsePorcelain`：

```go
name := filepath.Base(path)   // 取最末段（文件名）
```

porcelain 输出的路径是**仓库根相对**的，而 `filepath.Base` 取最末段（文件名本身）。当前列表里可见的条目是「相对当前目录」的顶层分量，两者错位 → 状态记到不存在的名字上。

**关键事实（git 2.50.1 实测，铁律）**：`git status --porcelain` 的路径**永远仓库根相对**，与 cwd / `-z` / `-- .` / `status.relativePaths` 全都无关。**只有长格式才遵守 `status.relativePaths`（默认 cwd 相对）。** 实测对照（仓库 `/tmp/g2`，结构 `internal/{action/a.go, display/d.go, flat.go}` + `root.txt` 全 modified）：

```
浏览 internal/ 时：
  porcelain（带或不带 -z、带或不带 -- .、-C 或真 cd）:
      M internal/action/a.go   M internal/display/d.go   M internal/flat.go      ← 永远带 internal/ 前缀
  长格式（默认 git status）:
      modified: action/a.go   modified: flat.go   modified: ../root.txt        ← cwd 相对（但不适合机器解析）

-c status.relativePaths=true 对 porcelain 无效（仍仓库根相对），只对长格式生效。
```

所以「取最末段」在仓库根碰巧对（根的仓库根相对 == cwd 相对），一进子目录就全错——这正是上面两种现象的同一根源。

**`rev-parse --show-prefix` 恰好给出解药**：它返回「当前目录相对仓库根」的路径（带尾斜杠），与 porcelain 共享同一个参照系（都仓库根相对），实测：

```
浏览根              prefix = ""
浏览 internal/      prefix = "internal/"
浏览 internal/action/   prefix = "internal/action/"
```

剥掉这个前缀，porcelain 路径就变成「相对当前目录」，再取顶层分量即为列表里可见的条目名。

---

## 3. 修复方案：剥前缀 + 取顶层分量

### 3.1 算法

1. `statusFor(dir)` cache miss 时，多跑一次 `git -C <dir> rev-parse --show-prefix` 拿 `prefix`（亚毫秒、不碰 index/不扫未跟踪）。
2. `parsePorcelain(out, prefix)`：每条路径先 `strings.TrimPrefix(path, prefix)` → 得到「相对当前目录」的路径；再取其顶层分量（按 `/` 切第一段）作为条目名。

```go
// 剥掉「当前目录相对仓库根」的前缀，得到相对当前目录的路径
rel := strings.TrimPrefix(path, prefix)
// 取顶层分量：含 "/" → 变更在子目录内，取顶层目录名（列表里那个 dir 条目）；
// 否则 → 当前目录内的文件，取整个路径。
var name string
if i := strings.IndexByte(rel, '/'); i >= 0 {
    name = rel[:i]
} else {
    name = rel
}
```

效果（同一组实测数据）：

| 浏览位置 | prefix | porcelain 路径 | 剥后 (rel) | 顶层分量 (name) | 列表条目 |
|---|---|---|---|---|---|
| 根 | `""` | `docs/x.md` | `docs/x.md` | `docs` | `docs/` ✓ |
| 根 | `""` | `internal/a.go` | `internal/a.go` | `internal` | `internal/` ✓ |
| `internal/` | `internal/` | `internal/action/a.go` | `action/a.go` | `action` | `action/` ✓ |
| `internal/` | `internal/` | `internal/flat.go` | `flat.go` | `flat.go` | `flat.go` ✓ |
| `internal/action/` | `internal/action/` | `internal/action/a.go` | `a.go` | `a.go` | `a.go` ✓ |

「子目录与前缀同名」也对：浏览 `internal/` 时若有 `internal/internal/foo.go` → 剥成 `internal/foo.go` → 顶层 `internal`（里面的 `internal/` 子目录）。

### 3.2 为什么前缀剥离确定无疑、无歧义

`--show-prefix` 与 porcelain 路径**同一参照系（都仓库根相对）**，剥的是精确前缀，不靠猜 cwd 相对 / 仓库根相对——两者关系是 git 自己保证的，不存在误判。比「猜测输出形态」稳。

### 3.3 跨平台：只切 `/`

- git porcelain 在**所有平台（含 Windows）都发 `/`**；`rev-parse --show-prefix` 同样发 `/`。剥前缀与切顶层都用 `/`，Windows 一次到位，无 `runtime.GOOS` 分支。
- **不要**用 `\` / `filepath.Separator`：Unix 上 `\` 是合法文件名字符（`-z` 下路径 verbatim），双切会误伤。

### 3.4 目录聚合：复用现有优先级合并

同一顶层目录下多个不同状态文件时，现有合并（`fileselector_git.go:151`，`st < existing` 取枚举值最小 = 最高优先级）已能处理，无需新代码。例如 dir `x` 同时含 A 的 `added.go` 和 M 的 `mod.go` → `x` 显示 M。

### 3.5 性能：大仓库不慢（沿用 F1 §10.4）

- **仍是每个目录单独 fork、`-- .` 只扫当前子树**——F1 §10.4 当初用 pathspec 限定子树（对齐 eza）的初衷完全保留。
- **只多一个 `rev-parse --show-prefix`**：它不碰 index、不做 stat 比对、不扫未跟踪文件，只解析 `.git` 位置，亚毫秒、与仓库大小无关。且只在 cache miss（首次进某目录）时跑，进过的目录命中 cache 不再跑。
- **不选「Open 时全仓库读一次」**：大仓库全量 untracked 扫描会慢，违背 §3.5 初衷，已否决。

---

## 4. 改动清单

### 4.1 代码（`internal/action/fileselector_git.go`）

1. `statusFor(dir)` cache miss 分支：`git -C <dir> status ...` 成功后，再跑 `git -C <dir> rev-parse --show-prefix` 取 prefix（status 失败=非仓库，已在前面 `return (nil,false)`，不会跑到 rev-parse）。把 prefix 传给 parsePorcelain。
   - 两个 git 调用顺序：status 先（它是 F1 §10.2 的可用性黑盒信号），rev-parse 后（成功才需要）。两次 fork 仅在 cache miss 发生。
2. `parsePorcelain(out []byte, prefix string)`：增加 `prefix` 形参；每条路径 `TrimPrefix(path, prefix)` 后再取顶层分量（§3.1）。移除 `filepath.Base` 用法；若 `filepath` 包在文件内别处不再被引用，移除其 import。

### 4.2 测试（`internal/action/fileselector_git_test.go`）

- `parsePorcelain` 加 `prefix` 形参。**除下条要改写的 `file in subdirectory` 外，其余现有 case 均传 `""`（根视角，原期望不变、仍通过）**——已逐条核验：这些 case 的输入路径都不含 `/`（根目录直显文件），取顶层分量 == 取整段路径，与旧 `filepath.Base` 行为完全一致，故仍通过。
- **改写**原 case `"file in subdirectory (base only)"`（行 70-74，锁定的是旧 bug 行为，名字里的「base」即指 `filepath.Base`）：重命名为 `subdirectory propagation (browsing sub/)`，改为带前缀视角——prefix=`"sub/"`、输入 `" M sub/dir/file.go"` → 期望 `{"dir": stModified}`（剥 `sub/` 后 `dir/file.go` → 顶层 `dir`）。
- **新增** case：
  - 根视角三层深：prefix=`""`、`" M a/b/c/deep.go"` → `{"a": stModified}`
  - 子目录视角：prefix=`"internal/"`、`" M internal/action/a.go"` → `{"action": stModified}`
  - 子目录内文件直显：prefix=`"internal/"`、`" M internal/flat.go"` → `{"flat.go": stModified}`
  - 多状态聚合（根视角）：prefix=`""`、`"A  x/added.go\x00 M x/mod.go"` → `{"x": stModified}`（M 胜出）——必须用根视角：剥前缀后两个文件都剩 `x/...`，取顶层分量都得 `x`，才会聚合到同一目录条目上；若用 `prefix="x/"` 会剥成 `added.go` / `mod.go` 两个独立文件，测不到聚合。这与 §3.4 的描述一致。
  - 反斜杠文件名防误切：prefix=`""`、`" M a\\b.go"` → `{"a\\b.go": stModified}`

### 4.3 不改

- F1 §10 / F1b T2 的 `statusFor` 缓存结构（key=dir、value=剥前缀后的 dir 相对 map）、异步时序、降级链——结构不变（value 天然就是 dir 相对，正是 Display 侧 `gitOf(name)` 要的）。
- `fileselector.go` 渲染层（`gitOf`/`drawEntry`）——只按 name 查 map，无感。

---

## 5. 验证

`make build` 后手测（沿用 F1b §5 git 状态小节的环境）：

- [ ] **根目录**：文件仍正确亮状态（回归）；**子目录**亮状态（核心）。
- [ ] **进子目录**：该子目录内的**文件**亮状态、其**子目录**也亮状态（本次重点，之前全空）。
- [ ] **三层深**：`a/b/c/deep.txt` 改了 → 根目录 `a/` 亮 M；进 `a/` → `b/` 亮 M；进 `a/b/` → `c/` 亮 M。
- [ ] **聚合优先级**：子目录同时含 M 和 A 文件 → 子目录亮 M。
- [ ] **回归**：无变更的子项不亮、`diffgutter=false` 整列隐藏、非 git 目录无标志（F1b §5）。
- [ ] `go test ./internal/action/` 通过（含改写 + 新增的 `parsePorcelain(out, prefix)` case）。

---

## 6. 范围外 / 已知问题（记录，不在本次修）

| # | 项 | 说明 |
|---|----|------|
| 1 | **rename 的 `-z` 两段解析** | `-z` 下 rename 是 `XY new\0old\0`，旧路径片段会被当成独立记录。剥前缀 + 取顶层后，旧片段通常映射到 stNone 不入库（副作用大多消失）；dir 聚合会显示 M（workSt 优先），可接受。彻底正确需按记录对解析，单列后续。 |
| 2 | **聚合优先级注释陈旧** | 源码注释「M/D/R > A > U」与实际枚举序（M > U > A > D > R）不符。是否调整优先级是独立产品决策，不在本次。 |
| 3 | **I(Ignored) 状态** | 沿用 F1 §10.5 / R4，延后。 |

---

## 附：与 F1 / F1b 的对应

| 本方案 | 上游文档 |
|--------|---------|
| §2 根因（porcelain 恒仓库根相对） | F1 §10.2 机制的细化（实测补充） |
| §3.1 剥前缀 + 顶层分量 | F1b T2 `parsePorcelain` 职责的修正 |
| §3.1 `rev-parse --show-prefix` | 与 F1 §10.2「git 是黑盒、只消费输出」一致 |
| §3.3 跨平台只切 `/` | git porcelain / rev-parse 协议保证（平台无关） |
| §3.5 per-dir fork + `-- .` | F1 §10.2 / §10.4 性能与降级 |
| §5 验证 | F1b §5 git 状态小节回归项 |
