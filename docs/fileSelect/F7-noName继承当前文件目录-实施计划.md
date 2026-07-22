# F7 · noName buffer 记住来源目录：实施计划

**性质**：实施计划。设计已定案（三条共识，见 §1），本文给出精确改动清单与行为表，可直接照着改。

**用户场景**（两个消费点，都不涉及存盘）：

1. 在 `aaa` 启动 microNeo（进程 cwd = `aaa`）→ finder 导航到 `bbb` 打开 `fileb.txt` → Ctrl-T 新 tab → noName buffer2 → birth hook 自动弹 finder → **希望起始目录 = `bbb`**（当前是 `aaa`）
2. 把其它 buffer 都关掉，buffer2 成最后一个 → Ctrl-q → finder 弹出 → Quit 退出 → **希望 shell 的 `m()` cd 到 `bbb`**（当前 cd 到 `aaa` 或不 cd）

共同前提：**noName buffer2 要记住自己是从 `bbb` 来的**。

---

## 1. 设计决策（三条共识）

### 决策 1：`Dir` 是 buffer 的「当前目录」字段，创建和保存时维护

- `Dir` 存着 buffer 的当前目录，是一个**存储字段**（不是计算属性）。命名 buffer 的 `Dir` 跟文件走——**创建**（`NewBuffer`）和**保存**（`save.go`）时设成 `filepath.Dir(AbsPath)`。noName 的 `Dir` 跟继承走——创建时由 `NewBufferNoName(dir)` 传入 `dir`；不传则 `NewBuffer` 给 cwd。
- **消费点一律直接读 `b.Dir`，零分支**。之前每个消费点（OpenFinder / lastWorkingDir）都要 `if HasFilename ? filepath.Dir(AbsPath) : b.Dir`——现在不用了，`Dir` 本身就是那个值。
- 维护点只有 2 处：`NewBuffer` 创建、`save.go` 保存路径赋值。全代码库 `.AbsPath =` 写点就这两个（`internal/buffer/buffer.go:398`、`internal/buffer/save.go:334`，已 grep 验证），不可能漏维护。
- 用户语义四条（来自需求）：
  - `AbsPath = aaa/fileA` → `Dir = aaa`（创建时设）
  - `AbsPath` 从 `aaa/fileA` → `aaa/fileB`（同目录）→ `Dir` 不变（`filepath.Dir` 算出来一样）
  - `AbsPath` 从 `aaa/fileA` → `bbb/fileB`（跨目录）→ `Dir` 同步改成 `bbb`（保存时设）
  - `AbsPath = ""`（noName）→ `Dir` 必须指定：noName 没有文件，`AbsPath` 留空，但 `Dir` 必须由创建者提供（`NewBufferNoName(dir)` 传入，或不传走承重墙的 cwd），否则消费点没值可读。

  case 1-3 由「创建/保存时 `b.Dir = filepath.Dir(absPath)`」满足；case 4 由「`NewBufferNoName` 传 `dir`、不传走承重墙」满足。两个机制互补，覆盖所有 buffer 状态。
- 对命名 buffer：`Dir` 是新字段，但值 = `filepath.Dir(AbsPath)`，跟旧实现在消费点现算同一个值，行为零变化。对 noName：`Dir` 是这次新增的语义（旧实现没维护，消费点靠 `os.Getwd()` 兜底；现在改成创建时指定）。

### 决策 2：noName 判据统一到 `Path == ""`，不用 `AbsPath`

- `Path == ""` 和 `AbsPath == ""` 在当前实现下等价——noName 时 `NewBuffer` 设的 `absPath` 是空（`path` 空 → `util.ResolvePath` 返回空），命名时 `util.ResolvePath` 对非空 `path` 必返回非空绝对路径。所以 `AbsPath == ""` 作为 noName 判据逻辑上成立。
- 但全代码库 7 处用的是 `Path == ""` / `Path != ""`（`GetName` / `SaveCB` / `Backup` / `RemoveBackup` / `serialize` ×2 / `CdCmd`，已 grep 验证），仅 birth hook 一处用了 `AbsPath == ""`，是孤例。本次对齐到 `Path`。
- 新增 `HasFilename()` accessor 把「按 filename 判」这条约定固化在唯一入口。OpenFinder 决定「要不要预选当前文件」也用它。

### 决策 3：原语归原语，策略归策略

- `NewBufferNoName(dir)`（buffer 包）是**唯一需要的原语**：「在 dir 这个目录里造一个 noName」。它不关心 dir 从哪来——继承的、cwd、还是别处指定的都行。将来 `NewBufferNoName("/tmp")`、`NewBufferNoName(projectRoot)` 都能直接用。
- 不需要独立的 F7 策略 helper。决策 1 把 `Dir` 维护成 buffer 的「当前目录」后，F7 创建点只需要 `NewBufferNoName(h.Buf.Dir)`——`h.Buf.Dir` 已经是对的，直接读即可，不再需要 `inheritDir()` 之类的推导函数。
- 不把 `parent *Buffer` 塞进构造器——那会把「从父文件继承」这条 F7 策略焊进一个本该通用的原语，构造器就被绑死在 F7 场景，别的需求用不了。

---

## 2. 改动清单

### 2.1 `internal/buffer/buffer.go`：字段 + accessor + 创建时维护 + 原语

**(a) `SharedBuffer` 加字段**（紧跟 `AbsPath string` 之后）

```go
// Path to the file on disk
Path string
// Absolute path to the file on disk
AbsPath string
// Dir 是 buffer 的「当前目录」。命名 buffer 由 NewBuffer 创建时和 save.go
// 保存时维护成 filepath.Dir(AbsPath)，随文件走；noName 由 NewBufferNoName
// 传入 dir，不传则 NewBuffer 给 cwd（承重墙）。所有 buffer 的 Dir 都被维护
// 成正确值，消费点（OpenFinder / lastWorkingDir）一律直接读
// b.Dir，无需按 HasFilename 分支。
Dir string
```

**(b) 新增 `HasFilename` accessor**（放在 `GetName` 附近）

```go
// HasFilename 报告 buffer 是否绑定磁盘文件；noName 恒为 false。
// 判据用 Path 而非 AbsPath：「有没有文件」与「路径是否解析过」是两件事，
// 前者才是 noName 的本质。全代码库判 noName 应统一走这里。
func (b *Buffer) HasFilename() bool { return b.Path != "" }
```

**(c) `NewBuffer` 创建时设 `Dir`**（`!found` 分支，紧跟 `b.AbsPath = absPath; b.Path = path` 之后）

命名文件设成文件所在目录；noName 和非 BTDefault buffer（Help/Log/Raw/Scratch）设成 cwd（承重墙）：

```go
b.AbsPath = absPath
b.Path = path
if btype == BTDefault && path != "" {
    b.Dir = filepath.Dir(absPath) // 命名文件：文件所在目录
} else {
    wd, _ := os.Getwd()           // noName / Help / Log / Raw / Scratch：cwd（承重墙）
    b.Dir = wd
}
```

注：
- `btype == BTDefault` guard 跟上面 `absPath := path; if btype == BTDefault && path != "" { absPath = ResolvePath(path) }` 对齐。非 BTDefault buffer 即便有 path（Help 的 `"page.md"`、Info 的 page 名）也不是真文件路径，`filepath.Dir` 会算出 `"."`，所以一律走 cwd else 分支——这样从 Help pane Ctrl-T 时起始目录是 cwd 而不是字面量 `"."`。
- `os.Getwd()` 返回 `(string, error)`，不能直接赋给 string 字段，拆两行（fileops.go 旧代码也是 `dir, _ = os.Getwd()`）。error 只在 cwd 被删等极端情况出现，忽略即可。
- `os` 和 `path/filepath` 已在 buffer.go import。

**(d) 新增 `NewBufferNoName(dir)` 原语**（放在 `NewBufferFromString` 附近）

```go
// NewBufferNoName 创建一个位于 dir 目录的 noName buffer（空内容、BTDefault、
// 无文件）。dir 非空时覆盖来源目录；dir 为空则保留 NewBuffer 承重墙设的 cwd。
// 它只负责「在某个目录造一个 noName」，不关心 dir 是继承来的、还是别处指定的。
func NewBufferNoName(dir string) *Buffer {
    b := NewBuffer(strings.NewReader(""), 0, "", BTDefault, emptyCommand)
    if dir != "" {
        b.Dir = dir
    }
    return b
}
```

直接调基座 `NewBuffer`（空 reader），不绕道 `NewBufferFromString`——后者名字暗示「内容来自 string」，但这里根本没有 string。`NewBufferNoName` 自身就是 wrapper，没有 DRY 顾虑。

`if dir != ""` 的 guard 复用承重墙（`NewBuffer` else 分支的 `wd, _ := os.Getwd(); b.Dir = wd`）：dir 非空就覆盖，dir 为空就保留承重墙给的 cwd，cwd 逻辑只此一处。试过两种替代都不如它——无条件 `b.Dir = dir` 会在 `NewBufferNoName("")` 时把承重墙的 cwd 覆盖成空串（不安全）；在函数内自带 cwd 兜底又跟承重墙重复。

承重墙这个依赖不是缺点：那行是 load-bearing 的——启动 noName 不经本函数、也靠它拿 cwd，删了启动场景先炸，所以不可能被误删。NewBufferNoName 依赖一个「必须存在」的不变量，安全。Dir 字段注释已写明「NewBuffer 保证 noName 的 Dir 非空」。

`strings` 已在 buffer.go import；`emptyCommand` 是包级标识符（`NewBufferFromString` 自己也用它）。

**(e) `save.go` 保存时维护 `Dir`**（约 334 行，`b.AbsPath = absFilename` 紧随其后）

保存是创建后唯一会改 `AbsPath` 的地方（Save 和 SaveAs 都走这里：`newPath := b.Path != filename` 之后一并赋值 `b.Path = filename; b.AbsPath = absFilename`）。AbsPath 一变就同步 Dir：

```go
b.Path = filename
b.AbsPath = absFilename
b.Dir = filepath.Dir(absFilename) // ← 新增：AbsPath 变了，Dir 同步
```

这是决策 1 的第二个维护点。用户语义四条里，case 2（同目录 SaveAs Dir 不变）和 case 3（跨目录 SaveAs Dir 同步）靠这一行——save.go 同时是「noName 首次 SaveAs 变命名」这个转换点（AbsPath 从 "" 到实际路径，Dir 从继承/cwd 同步成新文件的父目录）。case 4（noName 创建时设 Dir）是创建机制，不在 save 维护范围内。`path/filepath` 已在 save.go import（同文件 line 289 已在用 `filepath.Dir`）。

### 2.2 `internal/action/bufpane.go`：birth hook 判据对齐（决策 2）

`Display()` 里的 birth 判定（约 343 行），把 `AbsPath == ""` 换成 `HasFilename()`：

```go
// before
h.isNoName = h.Buf != nil &&
    h.Buf.AbsPath == "" &&
    h.Buf.Type == buffer.BTDefault &&
    h.Buf.Size() == 0

// after
h.isNoName = h.Buf != nil &&
    !h.Buf.HasFilename() &&
    h.Buf.Type == buffer.BTDefault &&
    h.Buf.Size() == 0
```

### 2.3 `internal/action/fileops.go`：消费点 1（OpenFinder 起始目录）

决策 1 之后 `Dir` 已维护成正确值，`OpenFinder` 的 dir 直接读，只有「要不要预选当前文件」需要分支：

```go
// before
if abs := h.Buf.AbsPath; abs != "" {
    dir = filepath.Dir(abs)
    file = filepath.Base(abs)
} else {
    dir, _ = os.Getwd()
}

// after
dir = h.Buf.Dir // 已维护：命名 = filepath.Dir(AbsPath)，noName = 继承/cwd
if h.Buf.HasFilename() {
    file = filepath.Base(h.Buf.AbsPath) // 仅命名 buffer 预选当前文件
}
```

`os.Getwd()` 回退删除——`Dir` 永不为空，不需要兜底。

### 2.4 `cmd/micro/micro.go`：消费点 2（lastWorkingDir，含窄窗兜底）

决策 1 之后直接返回 `pane.Buf.Dir`，零分支：

```go
// before
if t := action.MainTab(); t != nil {
    if pane := t.CurPane(); pane != nil && pane.Buf != nil {
        if ap := pane.Buf.AbsPath; ap != "" {
            return filepath.Dir(ap)
        }
    }
}
return ""

// after
if t := action.MainTab(); t != nil {
    if pane := t.CurPane(); pane != nil && pane.Buf != nil {
        return pane.Buf.Dir // 已维护：命名 = Dir(AbsPath)，noName = 继承/cwd
    }
}
return ""
```

正常路径下消费点 2 由消费点 1 传导满足（Ctrl-q 弹的 finder 起始 = `Dir` → Quit → `LastFinderCwd = Dir`）。本处改动只为窄窗预检失败、finder 没开成的 quit 边界兜底。

### 2.5 F7 创建点：直接读 `h.Buf.Dir`

决策 1 把 `Dir` 维护成 buffer 的「当前目录」后，F7 创建点不再需要推导函数，直接读 `h.Buf.Dir` 即是正确值（原 `NewBufferFromString("", "", buffer.BTDefault)` 整体替换）：

`internal/action/actions.go`：

```go
// AddTab（约 1979 行）
b := buffer.NewBufferNoName(h.Buf.Dir)
tp := NewTabFromBuffer(0, 0, width, height-iOffset, b)

// VSplitAction（约 2026 行）
h.VSplitBuf(buffer.NewBufferNoName(h.Buf.Dir))

// HSplitAction（约 2033 行）
h.HSplitBuf(buffer.NewBufferNoName(h.Buf.Dir))
```

`internal/action/command.go`：

```go
// NewTabCmd 无参分支（约 568 行）
b := buffer.NewBufferNoName(h.Buf.Dir)
tp := NewTabFromBuffer(0, 0, width, height-iOffset, b)
```

`:vsplit` / `:hsplit` 无参会汇到 `VSplitAction` / `HSplitAction`（`VSplitCmd` / `HSplitCmd` 在 `len(args)==0` 时转发），已覆盖。

---

## 3. 不需要改的

- **非 F7 的 noName 创建点**：`globals.go` LogBuf、`notepane.go` Scratch×2、`rawpane.go` Raw、`micro.go` 启动 noName×2、`buffer.go:422` 递归兜底——全部不碰。它们要么有自己的 btype（非 `BTDefault`，用不了 `NewBufferNoName`），要么无父文件概念，继续走 `NewBufferFromString`，由 `NewBuffer` 自动给 cwd。
- **命名 buffer**：`Dir` 在创建/保存时维护成 `filepath.Dir(AbsPath)`，消费点也读 `Dir`——值跟旧实现（消费点现算 `filepath.Dir(AbsPath)`）一样，行为零变化。
- **save / backup / serialize / display / lua / config**：完全不碰。决策 1 选独立字段而非复用 `Path`，根本原因就是让现有 `Path` 逻辑一字不动。

---

## 4. 行为表

| 场景 | `Dir` 来源 | finder 起始目录 | 退出 cd |
|---|---|---|---|
| 启动 noName（micro 无参） | NewBuffer else 分支 → cwd | cwd | cwd |
| Ctrl-T 从 `bbb/fileb.txt` | 命名 buffer：NewBuffer 创建时 `Dir` = `filepath.Dir(AbsPath)` = bbb | bbb | bbb |
| 在 `bbb/fileb.txt` 上 F7 → noName1 | F7：`NewBufferNoName(h.Buf.Dir)`，`fileb.txt.Dir` = bbb → noName1.Dir = bbb | bbb | bbb |
| 在 noName1 上再 F7 → noName2 | F7：`NewBufferNoName(h.Buf.Dir)`，`noName1.Dir` = bbb → noName2.Dir = bbb（沿链传播） | bbb | bbb |
| 当前 buffer 是 noName 时 Ctrl-T | OpenFinder 读 `b.Dir`（noName 创建时继承的目录） | `b.Dir` | `b.Dir` |
| TermPane / Help / Log 上 Ctrl-T | 这些 buffer 非 BTDefault，NewBuffer else 分支 → cwd | cwd | cwd |
| Ctrl-q 走 finder 正常路径 | — | `Dir`(bbb) → Quit → `LastFinderCwd` = 导航目录 | 导航目录（用户主动选择优先） |
| Ctrl-q 窄窗预检失败 | — | 不开 finder | `Dir`(bbb)，经 `lastWorkingDir` 兜底 |
| 首次 `SaveAs` 之后 | 保存时 `Dir` 维护成 `filepath.Dir(新 AbsPath)`，跨目录则同步更新 | `filepath.Dir(AbsPath)` | `filepath.Dir(AbsPath)` |
| `:cd` 命令 | 不回填已有 `Dir`；之后新建 buffer 的默认值跟随新 cwd。对我们无影响：消费点读 `b.Dir`，与 cwd 无关 | — | — |

---

## 5. 决策记录（已拍板）

- ✅ **做**。
- ✅ **方案 B**：新增独立 `Dir` 字段，不复用 `Path`。
- ✅ **`Dir` 是维护式字段**——命名 buffer 创建（`NewBuffer`）和保存（`save.go`）时 `b.Dir = filepath.Dir(absPath)`；noName 创建时由 `NewBufferNoName(dir)` 传入或不传走承重墙。**消费点一律直接读 `b.Dir`，零分支**。维护点只有 2 处（全代码库 `AbsPath` 写点就这 2 个，不可能漏）。
- ✅ **消费点删除 `os.Getwd()` 回退**——`Dir` 永不为空（命名有父目录、noName 有承重墙），不需要兜底。
- ✅ **noName 判据统一 `HasFilename()`**（= `Path != ""`），birth hook 从 `AbsPath == ""` 对齐过来。
- ✅ **只有 `NewBufferNoName(dir)` 一个原语**——buffer 包通用，F7 创建点直接 `NewBufferNoName(h.Buf.Dir)`，不需要任何策略 helper。决策 1 之后 `Dir` 已被维护成正确值，创建点只需「读」不需「推导」。
- ✅ **`:cd` 不回填已有 `Dir`**——`Dir` 创建即定死，与 micro「buffer 路径创建时确定」一致。对我们无影响：消费点一律读 `b.Dir`、不读 cwd，而 `:cd` 只改进程 cwd 和 `b.Path` 的相对化，根本不碰 `b.Dir` / `b.AbsPath`。`:cd` 留还是删是独立的产品决定，不在本计划范围。
- ✅ **窄窗兜底（§2.4）顺带做**——消费点 2 本就要改判据，多返回一行 `pane.Buf.Dir` 零额外成本。
- ✅ **`NewBuffer` 的 `Dir` 分支判据加 `btype == BTDefault`**——非 BTDefault buffer（Help 的 `"page.md"`、Info page 名）的 path 不是真文件路径，`filepath.Dir` 会算出 `"."`；判据跟上面 `ResolvePath` 的 `if btype == BTDefault && path != ""` 对齐，非 BTDefault 一律走 cwd。
- ✅ **承重墙拆两行写**（`wd, _ := os.Getwd(); b.Dir = wd`）——`os.Getwd()` 返回 `(string, error)`，不能直接赋给 string 字段。

无遗留待决项。