# F7 · noName buffer 记住来源目录：方案调研

**性质**：调研 + 方案选型，不做实施承诺。根因摸排 + 复用现有字段的可行性证伪 + 推荐方案，决策在 §7。

**用户场景**（两个消费点，都不涉及存盘）：

1. 在 `aaa` 启动 microNeo（进程 cwd = `aaa`）
2. 通过 finder 导航到 `bbb` 打开 `fileb.txt`
3. 按 Ctrl-T 创建新 tab → noName buffer2
4. buffer2 创建后 birth hook 自动弹 finder → **希望这个 finder 起始目录 = `bbb`**（当前是 `aaa`）
5. （用户继续操作，最终把其它 buffer 都关掉，buffer2 成为最后一个 buffer）
6. 在 buffer2 上按 Ctrl-q → finder 弹出 → Quit 退出 → **希望 shell 的 `m()` cd 到 `bbb`**（当前 cd 到 `aaa` 或不 cd）

两个消费点的共同前提：**noName buffer2 需要记住自己是从 `bbb` 来的**。

---

## 1. 现状盘点与根因

### 1.1 关键结论：micro 没有「per-buffer cwd」，noName 更没有

全程序只有一个 cwd——Go 进程的 `os.Getwd()`，启动时确定。命名 buffer 还能靠 `AbsPath` 推导父目录；**noName buffer 连 `AbsPath` 都是空的**，所以一切以 buffer 为基准的目录推导对它都失效，只能回退到进程 cwd。

### 1.2 Ctrl-T 创建 noName 的调用链

| 跳 | 位置 | 动作 |
|---|---|---|
| 1 | 键绑定 | `Ctrl-T` → action `AddTab`（`defaults_darwin.go:55` / `defaults_other.go:58`） |
| 2 | `actions.go:1975-1986` `(*BufPane).AddTab()` | `b := buffer.NewBufferFromString("", "", BTDefault)`（`Path=""`、`AbsPath=""`）→ `NewTabFromBuffer` → `AddTab` → `SetActive` |

新 buffer 的 `Path = ""`、`AbsPath = ""`。

### 1.3 两个消费点的现状代码

**消费点 1：noName 创建后 birth hook 弹的 finder 起始目录**

```
bufpane.go:341-348 (birth hook)
  pendingBirth 消费 → isNoName 判定成立 → h.OpenFinder(false)
                                                    ↓
fileops.go:23-28 (OpenFinder)
  if abs := h.Buf.AbsPath; abs != "" {
      dir = filepath.Dir(abs)        // 命名 buffer：父目录
  } else {
      dir, _ = os.Getwd()            // noName：进程 cwd（aaa）  ← 根因
  }
```

**消费点 2：Ctrl-q 退出时 cd 的目录**

```
command_neo.go:93-96 (QuitNeo)
  noName pane 按 Ctrl-q → h.OpenFinder(true)   // quit 入口
                    ↓
fileops.go:46-56 (onFinderClose, Quit 分支)
  h.doQuit(r): LastFinderCwd = r.Cwd; h.Quit()
                    ↓
micro.go:286-298 (lastWorkingDir, exit 时读)
  优先级 1: action.LastFinderCwd         // finder 导航成果（Ctrl-q 走这里）
  优先级 2: filepath.Dir(pane.Buf.AbsPath) // buffer 父目录（noName 时为空 → 返回 ""）
```

### 1.4 关键洞察：消费点 2 是消费点 1 的传导结果

Ctrl-q 的退出链路完整经过 `OpenFinder`：

```
buffer2(noName) Ctrl-q
  → OpenFinder(true)                    // 起始目录 = ?
  → 用户 Quit
  → doQuit(r): LastFinderCwd = r.Cwd    // = finder 当前目录
  → exit() → lastWorkingDir()           // 返回 LastFinderCwd
```

**只要消费点 1 修好**（noName 的 finder 起始目录 = `b.Dir = bbb`），那么：
- Ctrl-q 弹出的 finder 也在 `bbb`
- 用户直接 Quit → `r.Cwd = bbb` → `LastFinderCwd = bbb` → 退出 cd 到 `bbb` ✓
- 用户在 finder 里导航到 `ccc` 再 Quit → `r.Cwd = ccc` → 退出 cd 到 `ccc`（用户主动导航优先，符合预期）✓

**正常路径下，消费点 2 不需要单独修，它跟着消费点 1 走。**

### 1.5 唯一需要单独处理消费点 2 的边界：finder 预检失败

`OpenFinder`（`fileops.go:31-37`）：finder 自带预检，窗口过窄/过矮时 `ok=false`。

```
if !ok {
    if isQuit {
        h.Quit()        // 窄窗 quit 入口：直接退，不走 finder
    }
    return              // LastFinderCwd 未被写入
}
```

此时 Ctrl-q 不经过 finder，`LastFinderCwd` 保持空，`lastWorkingDir()` 走优先级 2：`filepath.Dir(pane.Buf.AbsPath)`。但 noName 的 `AbsPath == ""` → 返回 `""` → shell 不 cd。

这个窄窗边界要修好，需要 `lastWorkingDir()` 在 noName 时也认 `b.Dir`。

### 1.6 为什么进程 cwd 停在 aaa

全代码库 `os.Chdir` 只有两处：`command.go:265`（`:cd` 命令）、`lua.go:308`（lua 暴露）。finder 与 `:open` 打开文件都不 chdir。finder 在 `onFinderClose`（`fileops.go:51`）用 `filepath.Join(r.Cwd, r.File)` 拼绝对路径交给 `OpenCmd`，`AbsPath` 变成 `/abs/bbb/fileb.txt`，但进程 cwd 原地不动。

---

## 2. 能不能复用 Path 字段？（可行性证伪）

**直觉方案**：创建 buffer 时永远传一个 path——`dir/filename` 表示打开文件，`dir`（或 `dir/noname` 占位符）表示 noName。这样 buffer 一出生就锚定到目录，不用新增字段。

**结论：不可行**。根因不在「填什么值」，而在现有代码库把 `Path == ""` 当作 noName 的**唯一标志**，散落各处依赖。只要给 noName 的 `Path` 赋任何非空值（目录名、不存在的文件名、占位符），下面两类共 **20+ 处**全部被破坏。

### 2.1 A 类：`Path == ""` / `Path != ""` / `len(Path) > 0` 判断失效（noName 标志被破坏）

| # | 位置 | 现有判断 | 赋值后的后果 |
|---|---|---|---|
| 1 | `buffer.go:565` `GetName()` | `Path == ""` | tab 显示目录名而非 "No name" |
| 2 | `actions.go:967` `SaveCB` | `Path == ""` | 不弹提示，直接把目录当文件保存 ❌ |
| 3 | `actions.go:1929` `Quit` | `Path != ""` | noName 被 autosave 当命名文件自动保存 |
| 4 | `backup.go:139` `Backup()` | `Path == ""` | 给目录路径做备份 |
| 5 | `backup.go:149` `RemoveBackup()` | `Path == ""` | 删除不存在的备份 |
| 6 | `backup.go:159` | `len(Path) > 0` | noName 触发永久备份 |
| 7 | `serialize.go:28` `Serialize()` | `Path == ""` | 序列化目录路径 |
| 8 | `serialize.go:66` `Unserialize()` | `Path == ""` | 反序列化目录路径 |
| 9 | `command.go:272` `CdCmd` | `len(Path) > 0` | `:cd` 时把这个目录也拿来重算相对路径 |
| 10 | `bufpane.go:344` birth hook | `AbsPath == ""` | 不再认它是 noName，**不再自动弹 finder** ❌ |

特别要注意 #10：birth hook 自己就用 `AbsPath == ""` 判 noName。给 noName 赋非空 `AbsPath`，birth finder 直接不弹——**消费点 1 从源头就被掐断**。

### 2.2 B 类：直接把 `Path` / `AbsPath` 当文件路径用

| # | 位置 | 用法 | 后果 |
|---|---|---|---|
| 11 | `buffer.go:622` | `os.Open(b.Path)` | 打开目录失败 |
| 12 | `save.go:208` | `SaveAs(b.Path)` | 把目录当文件名 SaveAs |
| 13 | `save.go:216` | `saveToFile(b.Path)` | 同上 |
| 14 | `save.go:225` | `SaveAsWithSudo(b.Path)` | 同上 |
| 15 | `backup.go:143` | `writeBackup(b.AbsPath)` | 用目录路径写备份 |
| 16 | `backup.go:152,160` | `DetermineEscapePath(..., AbsPath)` | 备份文件名算错 |
| 17 | `backup.go:166` | `b.Path` 显示在消息里 | 显示目录名 |
| 18 | `serialize.go:46,53,69` | 用 `AbsPath` 算序列化文件 | 路径算错 |
| 19 | `command.go:274` | `filepath.Abs(b.Path)` | `:cd` 重算出错 |
| 20 | `notepane.go:475` | `filePath = AbsPath` | note 路径变目录 |
| 21 | `fileops.go:51` | `Join(Cwd,File) == AbsPath` | finder 比较失败 |

### 2.3 本质

问题不是「用什么值表示路径」，而是现有代码库把 `Path == ""` 作为 noName 唯一标志，到处依赖。想给 noName 带上任何位置信息并塞进 `Path`，就一定撞穿这堵墙，无论那个值长什么样。

因此「位置」和「文件路径」必须是两个独立字段：`Path` 仍表示文件路径、`Path == ""` 仍是 noName 标志；位置单独存一个字段，只在 finder 起始目录和退出 cwd 两处读取。

---

## 3. 候选方案

### 方案 A：打开文件时 `os.Chdir` 到文件父目录（进程 cwd 跟随当前文件）

在 `OpenCmd` / `onFinderClose` / `OpenBuffer` 等入口打开文件后追加 `os.Chdir(filepath.Dir(buf.AbsPath))`。

**优点**：改动行数最少；一刀切修复所有相对路径场景。

**缺点（致命）**：
- **全局副作用**：切 tab 就改 cwd，违反「buffer 路径创建时确定」的直觉
- **破坏 `CdCmd` 的相对路径显示逻辑**（`command.go:266-276`）：这段假设所有 buffer 共享同一进程 cwd 重算 `b.Path`
- **与上游 micro 语义分叉**：upstream 明确不在 open 时 chdir
- 并发场景（save 后台 goroutine）下 cwd 变成「随焦点变化的全局状态」

**结论：不推荐**。副作用面与问题严重不匹配。

### 方案 B：给 Buffer 加一个 `Dir` 字段，创建 noName 时继承当前 buffer 父目录（推荐）

核心思路：noName buffer 没有文件路径，但可以有一个「来源目录」，创建时从当前活动 buffer 继承。详见 §4。

**优点**：
- **精准**：只影响 noName 的 finder 起始目录和退出 cwd，命名 buffer 行为完全不变
- **可预测**：新 tab 的 Dir 创建那一刻就定死，不随后续焦点切换而变
- **向后兼容**：`Dir == ""` 时走原逻辑（`os.Getwd()` / 返回空），所有存量场景零变化
- **与现有约定一致**：`OpenFinder` 命名 buffer 分支和 `lastWorkingDir` 优先级 2 都用 `filepath.Dir(AbsPath)`，本方案是同一思路向 noName 的延伸

**缺点**：新增一个 buffer 字段（但只在 noName 生命周期内有效，首次 `SaveAs` 后 `Path != ""`，`Dir` 再不被读）。

### 方案 C：不新增字段，运行时从「来源 tab」反查（过度工程化）

noName buffer 不存 Dir，每次需要时回溯「我是从哪个 tab 的哪个 buffer 创建的」现场推导。

**缺点**：tab/buffer 的创建关系没有现成记录，要新增来源追踪；切 tab、关 buffer 后来源可能失效；复杂度远高于存一个字符串。

**结论：不推荐**。

---

## 4. 推荐方案 B 细节

### 4.1 对比总表

| 维度 | A（chdir） | **B（Dir 字段）** | C（反查来源） |
|---|---|---|---|
| 精准解决问题 | 否（过宽） | **是** | 是 |
| 副作用面 | 大（全局 cwd） | **小（仅 noName 两处读取）** | 中（来源追踪） |
| 向后兼容 | 否 | **是** | 是 |
| 与现有约定一致 | 否 | **是** | 否 |
| 可预测（不随焦点变） | 否 | **是** | 否 |
| 改动行数 | 极少 | 少（4 处） | 多 |

### 4.2 改动点（4 处）

1. `internal/buffer/buffer.go`：`SharedBuffer` 结构体加字段
   ```go
   // Dir 是 noName buffer 的来源目录（绝对路径），供 finder 起始目录和
   // 退出 cwd 上报使用。命名 buffer 由 AbsPath 推导，不读此字段；
   // 空串回退到进程 cwd（OpenFinder）/ 返回空（lastWorkingDir）。
   Dir string
   ```

2. `internal/action/actions.go` 的 `AddTab` 与 `internal/action/command.go` 的 `NewTabCmd` 无参分支：创建 noName 后填 `Dir`
   ```go
   b := buffer.NewBufferFromString("", "", buffer.BTDefault)
   if h.Buf.AbsPath != "" {
       b.Dir = filepath.Dir(h.Buf.AbsPath)
   }
   ```

3. `internal/action/fileops.go` 的 `OpenFinder`：noName 分支优先用 `b.Dir`（**消费点 1**，birth finder 与 Ctrl-q finder 都经此）
   ```go
   if abs := h.Buf.AbsPath; abs != "" {
       dir = filepath.Dir(abs)
       file = filepath.Base(abs)
   } else if h.Buf.Dir != "" {
       dir = h.Buf.Dir
   } else {
       dir, _ = os.Getwd()
   }
   ```

4. `cmd/micro/micro.go` 的 `lastWorkingDir`：noName 分支优先用 `b.Dir`（**消费点 2 的窄窗边界**）
   ```go
   if ap := pane.Buf.AbsPath; ap != "" {
       return filepath.Dir(ap)
   }
   if pane.Buf.Dir != "" {
       return pane.Buf.Dir
   }
   ```

**不需要改 `saveToFile`**：本需求两个消费点都不涉及存盘。

### 4.3 接口形态：保留「创建时传位置」的偏好

若希望 buffer 创建接口语义统一（一出生就带位置），可包一层 helper，但内部仍把位置存到 `Dir`、`Path` 置空，绕开 §2 的雷区：

```go
// 接口上接受"目录"参数，实现上分离位置与文件路径。
func NewNoNameBuffer(dir string) *Buffer {
    b := NewBufferFromString("", "", BTDefault) // Path 仍为 ""
    b.Dir = dir                                  // 位置单独存
    return b
}
```

形态满足「创建时传位置」，实现不碰 `Path`。本质仍是方案 B——`Path` 和「位置」必须是两个独立字段，因为 `Path == ""` 这个约定不能动。

---

## 5. 边界与退化情形

| 场景 | 行为 |
|---|---|
| 当前 buffer 也是 noName（`AbsPath == ""`） | 创建时 `h.Buf.AbsPath == ""` → `Dir` 留空 → 新 noName 回退进程 cwd（现状） |
| 当前 pane 是 TermPane / Help / Log | `h.Buf.AbsPath` 为空或无意义 → 同上回退 |
| Ctrl-q 走 finder 正常路径 | finder 起始 = `Dir` → Quit → `LastFinderCwd = Dir` → 退出 cd 到 `Dir`（消费点 1 传导，无需改 lastWorkingDir） |
| Ctrl-q 窄窗预检失败 | 不走 finder，`LastFinderCwd` 空 → `lastWorkingDir` 优先级 2 读 `b.Dir`（改动点 4 兜底） |
| 用户在 finder 里导航到别处再 Quit | `r.Cwd` = 导航后的目录，覆盖 `Dir`（用户主动选择优先） |
| 首次 `SaveAs` 之后 | `Path != ""`，buffer 变命名 buffer，`Dir` 不再被读 |
| 分屏 / vsplit 创建 noName | 同样适用（凡是走 `NewBufferFromString("", "", ...)` 的入口都该补 `Dir`） |
| `:cd` 命令 | 改进程 cwd，不影响 `b.Dir`；noName 的 finder 起始仍用 `Dir` |

---

## 6. 影响范围

- `internal/buffer/buffer.go`（加字段）
- `internal/action/fileops.go`（`OpenFinder` noName 分支，消费点 1）
- `cmd/micro/micro.go`（`lastWorkingDir` noName 分支，消费点 2 窄窗兜底）
- `internal/action/actions.go` 与 `internal/action/command.go`（两处 noName 创建点填 `Dir`）
- **不碰** display、finder 内部、lua、config、save
- §2 盘点的 20+ 处现有逻辑**全部不动**（这是选 B 而非复用 Path 的根本原因）

---

## 7. 决策清单（待拍板）

- [ ] **做不做？** 不做 → 文档归档；做 → 进实施
- [ ] **走方案 B？** 还是接受 A 的取舍
- [ ] **`:cd` 是否覆盖 `Dir`？** 建议默认「不覆盖」（`Dir` 创建即定死），保持简单；若期望 `:cd` 是「强制改默认目录」，需在 `CdCmd` 加一行清空逻辑
- [ ] **是否按 §4.3 包一层 `NewNoNameBuffer(dir)` helper？** 取决于是否想统一「buffer 创建必带位置」的接口语义
- [ ] **窄窗兜底（改动点 4）要不要做？** 正常路径消费点 2 已由消费点 1 传导满足；改动点 4 只为窄窗 quit 边界，可选
- [ ] **`NewTabCmd` 带文件名参数的那条分支要不要管？** 不用——那条分支直接 `NewBufferFromFile`，`AbsPath` 非空，是命名 buffer，不读 `Dir`
