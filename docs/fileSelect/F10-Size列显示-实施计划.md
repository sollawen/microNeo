# F10 · FileSelector size 列显示 实施计划

**状态**：草案（待评审）
**依据**：源码 `internal/action/fileselector.go`（`listDirEntries` / `drawEntry` / `display` / `refreshMetaIfStale` / `buildMetaLine` / `fileSelectorState`）+ F1 §7 / §10 / §10.1
**前置**：F1d 末行元数据已落地、三段布局（面包屑 / 文件列表 / 末行）稳定。
**交付**：（1）打开/chdir 时一次性预取当前目录**所有**条目的 FileInfo 进内存；（2）每个文件行的**最右侧**显示 size（右对齐 5 列）；（3）末行作 `ls -l` 风格详情栏，显 perms+size+mtime。删除光标懒 stat 机制。
**原生侵入**：零。改动仅限 microNeo 自有文件 `internal/action/fileselector.go`，不改 FloatFrame、不改主循环、不改 micro 原生代码。

---

## 1. 背景与目标

### 1.1 现状（要改的痛点）

F1d 把光标条目的 size+mtime 放在末行，靠 `refreshMetaIfStale` 在 `display` 内懒 stat + 按名缓存。问题：

- size 只在末行、只显当前一个文件——要对比两个文件的大小得上下移动光标来回看，不方便。
- 数据来源是"光标走到哪 stat 到哪"，永远只持有当前一个条目的元数据。

### 1.2 目标

- **打开即全读**：Selector 打开、chdir、toggleHidden 时，一次性把当前目录所有可见条目的 size+mtime 读进 `entry`。
- **size 上行**：每个文件行最右侧 5 列显 size（右对齐），所有行同时可见，方便横向对比。
- **mtime 留末行**：末行仍显当前光标条目的 mtime（size 移走后末行只剩它）。
- **删懒加载**：数据已在 entry 里，光标移动零 stat，末行直接读 entry——净删一套缓存机制。

---

## 2. 数据来源：`listDirEntries` 一次性预取

### 2.1 `entry` 加一个 `os.FileInfo` 字段

**数据与显示分离**：entry 存全量 FileInfo，显示哪些字段（size / mtime / mode…）由 UI 层决定，互不耦合。

```go
type entry struct {
	name  string      // 来自 ReadDir
	isDir bool        // = fi.IsDir()（lstat，比 d_type 更可靠）
	info  os.FileInfo // 来自 d.Info()（lstat，不跟随 symlink）；恒非 nil（失败则跳过）
}
```

`info` 恒非 nil——`d.Info()` 失败的条目（竞态删除等）直接跳过不入列表（§2.2）。后续 UI 想显 mode、权限位、owner 等，直接从 `e.info` 取，无需再改数据层。

### 2.2 在 `listDirEntries` 里读 FileInfo

`os.ReadDir` 返回的 `DirEntry` 只带 `d_type`（类型，不含详情）。每个 entry 调一次 `d.Info()` 拿到完整 `os.FileInfo`（底层 lstat，不跟随 symlink，对齐 `ls -l`），整块存进 entry。完整函数（现有代码 + 本方案改的部分）：

```go
func listDirEntries(dir string, showHidden bool) []entry {
	dirEntries, err := os.ReadDir(dir) // 一次 getdents，拿全部条目名 + 类型（d_type），无 size/mtime
	if err != nil {
		return nil
	}
	var dirs, files []entry
	for _, d := range dirEntries {
		name := d.Name()
		if !showHidden && isHiddenName(name) {
			continue
		}
		fi, err := d.Info()
		if err != nil {
			continue // 条目已不存在（竞态删除），不显示
		}
		e := entry{name: name, isDir: fi.IsDir(), info: fi}
		if e.isDir {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}
	// ... 排序逻辑不变（目录优先 + 大小写不敏感字母序）
}
```

排序、过滤逻辑零改动。

### 2.3 为什么用 `d.Info()`（lstat）而不是 `os.Stat`（follow）

| 项 | `d.Info()`（lstat） | `os.Stat`（follow） |
|----|---------------------|---------------------|
| symlink 大小 | 链接自身（对齐 `ls -l`） | 目标文件 |
| symlink→dir | 当文件画（size 显链接大小） | 当目录画（size 留空） |
| 成本 | 复用 ReadDir 内部数据，N 次 lstat | N 次 stat（symlink 多一次跟随） |
| 与 `entry.isDir` 一致性 | 一致（都 lstat） | 不一致（F1 §16 item 3 的二义） |

现末行用的是 `os.Stat`（follow）。换 lstat 是**行为变化**：symlink 显示链接自身大小。收益是对齐 `ls -l`、消除 F1 §16 item 3 的 `isDir` 二义（`entry.isDir` 与 size 的 IsDir 判定同源）。如评审希望保持 follow 语义，把 `d.Info()` 换成 `os.Stat(filepath.Join(dir, name))` 即可（一处）。

**决定点 A**（**已定**：lstat）：symlink 行显链接自身 size，对齐 `ls -l`。

---

## 3. 显示布局：`drawEntry` 右侧腾 5 列给 size

### 3.1 新行结构

```
列:  0        1 ............ (w-7)  (w-6)  (w-5 .......... w-1)
    [marker] [    name    ]  [git]  [        size         ]
```

- `marker`：1 列（目录 `▸` / 文件空格），不变。
- `name`：超长左截断保扩展名（复用 `truncateNameKeepExt`），预算 = `w - 1(marker) - 5(size) - (1 if git)`。
- `git`：1 列（仅 `showGit` 时），位置 = size 区左侧一列。
- `size`：钉在最右 5 列（`[x+w-5, x+w)`），右对齐；复用 `humanSize`（恒 ≤5 列）。

效果示意（pickerW=24，git 开）：

```
▸ internal/                       M       
  fileselector.go                 M 12.3K
  README.md                         405B
  很长的中文名文件.md                1.2K
```

目录行的 size 段**留空 5 列**（与现末行目录处理一致）。

### 3.2 关键改动（`drawEntry`）

行布局：**右边永远预留 `空格(1) + git(若显示) + size(5)`，name 占剩下的固定区——短了用空格填、超长截断**。从左到右 `[marker] [name…] [空格] [git] [size]`，其中“空格”是**永远保留的 1 列**（name 被截断也不贴 git）：

```go
const sizeW = 5
sizeEnd   := x + w
sizeStart := sizeEnd - sizeW          // size 区 [sizeStart, sizeEnd)
gitCol    := sizeStart - 1            // git 列（仅 showGit 时写）

// name + 填充 占 [x+1, fillEnd)
fillEnd := sizeStart
if showGit {
	fillEnd = gitCol                  // 有 git 时填到 git 前
}
nameLimit := fillEnd - (x + 1) - 1    // -1 = 永远留给 gap 的 1 列空格
if nameLimit < 0 {
	nameLimit = 0
}
```

写入顺序：marker → name（截断到 `nameLimit`）+ 空格填到 `fillEnd`（fillStyle，含那 1 列 gap）→ git 写 `gitCol`（若 showGit）→ size 右对齐写入 `[sizeStart, sizeEnd)`。**size 文本 style 同 fillStyle**（selected 时取 `revStyle`），整行反转连续。

因为 name 永远停在 `fillEnd` 前 1 列，填充区**至少 1 列**——name 再长也碰不到 git。

**实现警示**：现有 `drawEntry` 的 `nameEnd` 在 `showGit=false` 时算成 `x+w-1`（会吃掉 size 列）。实现时必须按本节 `fillEnd`/`nameLimit` 重算，**勿沿用旧 `nameEnd` 分支**。

size 段填充规则：

- 文件：`humanSize(e.info.Size())` 右对齐 pad 到 `sizeW`（`e.info` 恒非 nil，无需判空）。
- 目录：size 区用 `fillStyle` 填 5 个空格（reverse 时整行连续，不破高亮条）。

### 3.3 对最小宽度的影响

name 预算比现在少 5 列。以 `fsMinWidth=20` 算：nameLimit = `20-1-5-(0/1)` = 14/13 列，仍可用（截断保扩展名）。典型 pickerW（pane 40%≈30+ 列）name 有 24+ 列，宽裕。**`fsMinWidth` 不动**。

**决定点 B**（默认 size 钉最右、git 退左一列）：满足“size 在最右边”。如希望 git 仍占最右一格、size 在 git 左侧，请评审时指出。

---

## 4. 末行：perms + size + mtime，删懒加载

末行作为“当前光标条目”的详情栏（类 `ls -l` 属性列），显示 3 个字段，**全部从已存的 `e.info` 读，零 stat**：

| 字段 | 来源 | 示例 | 宽 |
|------|------|------|----|
| 权限 | `fi.Mode().String()` | `drwxr-xr-x` | 10 |
| size | `humanSize(fi.Size())` | `12.3K` | 5 |
| mtime | `formatMtime(fi.ModTime())` | `07-11 14:30` | 11 |

size 虽也显在每行右侧，但末行**保留**——以防以后行内右侧改显 mtime 时末行仍有 size 兜底（用户决策）。末行宽 = 选择器宽（终端 40%），3 字段全显约 30 列，多数终端放得下；窄屏按 `权限→size` 砍，**mtime 保底**。

### 4.1 `buildMetaLine`

```go
func (fs *FileSelector) buildMetaLine(w int) string {
	s := fs.state
	idx := s.cursor - 1
	if idx < 0 || idx >= len(s.entries) {
		return ""                  // 面包屑行 / 空目录 / 越界 → 空
	}
	fi := s.entries[idx].info     // 恒非 nil
	perms := fi.Mode().String()   // "drwxr-xr-x"，对齐 ls -l
	size  := humanSize(fi.Size())
	mtime := formatMtime(fi.ModTime())
	return fitMeta(w, perms, size, mtime)
}

// fitMeta 按 w 宽挑能放下的组合；砍字段优先级 权限→size，mtime 保底。
func fitMeta(w int, perms, size, mtime string) string {
	type f struct{ k, v string }
	order := []f{{"p", perms}, {"s", size}, {"m", mtime}}
	keep := map[string]bool{"p": true, "s": true, "m": true}
	drop := []string{"p", "s"}      // m 永不砍
	for {
		var parts []string
		for _, x := range order {
			if keep[x.k] {
				parts = append(parts, x.v)
			}
		}
		line := strings.Join(parts, "  ")
		if stringWidth(line) <= w {
			return line
		}
		killed := false
		for _, d := range drop {
			if keep[d] {
				keep[d] = false
				killed = true
				break
			}
		}
		if !killed {
			return tailByWidth(line, w) // 只剩 mtime 仍超宽（极窄 pane），左截断保底
		}
	}
}
```

`strings` 已在文件内导入；无新依赖（不查 passwd、不引 syscall）。

### 4.2 `display` 内末行段

```go
// 末行：perms + size + mtime（全从 e.info 读，零 stat）
hintRow := area.Y + area.H - 1
text := fs.buildMetaLine(area.W)   // 内部判 cursor==0/越界 → 返回 ""
fs.drawString(area.X, hintRow, area.W, text, config.DefStyle)
```

删掉原来的 `refreshMetaIfStale()` 调用 + `if s.metaName != ""` guard。

### 4.3 删除懒加载机制（净简化）

数据全在 entry，以下全部删除：

| 删除项 | 原位置 |
|--------|--------|
| `fileSelectorState.metaName / metaSize / metaMtime / metaOK / metaIsDir` 五字段 | `fileSelectorState` 定义 |
| `refreshMetaIfStale()` 方法 | 整个方法 |
| `cursorEntryName()` 方法 | 整个方法（唯一调用方是 `refreshMetaIfStale`，删后无引用） |

`humanSize` / `formatMtime` 保留（行内 size + 末行 size/mtime 都用）；末行新增 `fitMeta` 辅助（无新依赖）。

---

## 5. 改动清单

全部在 `internal/action/fileselector.go`，净改动约 **+60 / −80 行**（末行变富使新增增多，整体仍删多于加）。

| # | 位置 | 改动 |
|---|------|------|
| 1 | `entry` 结构 | 加 `info os.FileInfo` 字段（存全量，UI 按需取字段） |
| 2 | `listDirEntries` | 循环内加 `d.Info()` 预取全量 FileInfo；失败则 `continue` 跳过 |
| 3 | `drawEntry` | 重排行布局：右边留 `空格+git+size`、name 占左（永远留 1 列 gap） |
| 4 | `buildMetaLine` | 改签名 `(w int) string`；拼 perms+size+mtime（窄屏砍字段），新增 `fitMeta` 辅助 |
| 5 | `display` 末行段 | 删 `refreshMetaIfStale()` + guard，改调 `buildMetaLine(area.W)` |
| 6 | `fileSelectorState` | 删 `metaName/metaSize/metaMtime/metaOK/metaIsDir` 五字段 |
| 7 | 方法删除 | 删 `refreshMetaIfStale`、`cursorEntryName` |
| 8 | `display` 滚动指示符段 | `rightCol` 改为 gap 列（`fillEnd-1`）避开 size 区；style 跟随所在行 |

### 5.1 不改

- `entry` 的 `name/isDir` 语义、排序 / 过滤、`isHiddenName`。
- 面包屑渲染（`drawBreadcrumb`）、滚动指示符、`truncateNameKeepExt` / `truncateLeftPath`。
- git 子模块（`fileselector_git.go`）—— git 数据仍异步、仍画在 size 左侧一列。
- Controller（`handleEvent` 及全部键位）、`chdirTo` / `toggleHidden`（它们调 `listDirEntries`，自动拿到新数据）。
- FloatFrame、主循环、任何 micro 原生文件。

---

## 6. 设计取舍

| # | 决定 | 理由 |
|---|------|------|
| 1 | **打开/chdir 同步预取全目录** | 数据量级与现 `os.ReadDir` 同阶（都是同步遍历目录），模型不变；换来光标移动零 stat + 全行 size 可见。 |
| 2 | **右边永远留 `空格 + git + size`，name 占左** | 用户要“size 在最右边”，且 name 与 git 之间**永远留 1 列空格**（name 截断也不贴 git）。 |
| 3 | **目录行 size 留空** | 与现末行目录处理一致，最小改动；"N items"列为范围外（对齐 F1 §16 item 4）。 |
| 4 | **lstat（`d.Info`）而非 follow（`os.Stat`）**——已定 | 对齐 `ls -l`（用户最熟悉）、复用 ReadDir 数据、消除 F1 §16 item 3 的 isDir 二义。 |
| 5 | **删懒加载、不保留双路径** | 数据已在 entry，懒 stat + 按名缓存成多余复杂度；一处数据源（entry）比两处（entry + meta 缓存）好维护。 |
| 6 | **`fsMinWidth` 不动** | name 少 5 列后最小 20 仍可用（nameLimit≥13）；min 是拒开阈值非典型宽度。 |
| 7 | **存全量 `os.FileInfo`、不拆字段** | 数据与显示分离：entry 存整块 FileInfo，UI 层决定显什么（size/mtime/mode…）。未来加显示字段零改数据层。 |
| 8 | **`d.Info()` 失败即跳过该条目** | 失败≈文件已不存在（竞态删除），不显示幽灵条目；换来 `e.info` 恒非 nil，UI 删掉所有 nil/`—` 降级分支。 |

---

## 7. 性能影响

| 场景 | 现在 | 改后 |
|------|------|------|
| 打开 / chdir / toggleHidden | `os.ReadDir`（μs 级） | `os.ReadDir` + N×lstat |
| 移动光标（miss） | 1×stat | **0** |
| 重绘（命中缓存） | 0 | 0 |

本地 SSD lstat ≈ 3µs：1000 文件 ≈ 3ms（无感），10000 文件 ≈ 30ms（仍可接受）。NFS / 网络盘有放大，属边缘场景，且现 `os.ReadDir` 本就同步，模型未变。

若将来遇巨型目录卡顿，可降级为"后台 lstat、size 列渐进填充"（套用现 git 状态的异步范式）——YAGNI，本次不做。

---

## 8. 验证

`make build` 后手测：

- [ ] **打开 Selector**：每个文件行右侧显 size（右对齐），目录行 size 段空白。
- [ ] **大小数值正确**：与 `ls -l` 对比一致（lstat 语义）。
- [ ] **末行**：显当前光标条目的 perms+size+mtime（窄屏按 权限→size 砍字段）。
- [ ] **移动光标**：末行 perms/size/mtime 随光标刷新（全从 entry 读，零 stat）。
- [ ] **git 状态列**：显在 size 左侧一列，颜色 / 字符不变。
- [ ] **chdir 进/出目录**：新目录 size 全部刷新（预取生效）。
- [ ] **`.` 切 dotfile**：显隐翻转后 size 重新预取。
- [ ] **窄 pane（minWidth=20）**：name 截断保扩展名，size 仍右对齐不溢出。
- [ ] **CJK 文件名**：name 截断 rune-safe，size 列对齐不受双宽字符影响。
- [ ] **选中行（Reverse）**：name / 填充 / git / size 整行反转连续，无破高亮条。
- [ ] **滚动指示符 ▲▼**：画在 gap 列（`fillEnd-1`），不盖 size 字符；selected 行指示符反转。
- [ ] **symlink**：列表行无 `/` 后缀、显链接自身 size（lstat）；末行 perms 首字符为 `l`（如 `lrwxr-xr-x`）。
- [ ] **空目录**：仅面包屑行，末行空白，不崩。

### 8.1 单测

- `drawEntry` 不直接单测（写屏），布局改动不破现有测试。
- `TestHumanSize` / `TestFormatMtime` 不变（函数保留）。
- 可选新增：`listDirEntries` 在临时目录验证 `entry.info` 非空、`Size()`/`ModTime()` 正确（需 `t.TempDir()` 造文件）。建议加，防回归。

---

## 9. 已知边界与范围外

### 9.1 滚动指示符位置冲突（已定：挪到 gap 列）

现滚动指示符 `▲/▼` 画在 `rightCol = area.X + area.W - 1`（最右列）。size 区占最右 5 列后，指示符会盖掉 size 末字符。

**决定点 C**：指示符改画在 **gap 列**（`fillEnd - 1`，即 name 与 git/size 之间那道永远保留的空格）。该列恒为空格，覆盖零信息损失，git on/off 都成立（git 开时指示符落在 git 左一列、关时落在 size 左一列）。style 跟随所在行（topStyle/botStyle，selected 时取 revStyle）。

曾考虑挪到 `sizeStart - 1`——但 git 开时那列是 git 标志位、会盖掉 git 标志，故改取 gap 列。

### 9.2 范围外

| # | 项 | 说明 |
|---|----|------|
| 1 | **目录行显 "N items"** | 需递归或额外 syscall 计数，成本高；留空已够。F1 §16 item 4 同此结论。 |
| 2 | **异步预取（巨型目录降级）** | 同步预取对常规目录无感；遇性能问题再套 git 异步范式。 |
| 3 | **symlink follow 选项** | lstat/follow 是全局取舍，不暴露配置开关。 |
| 4 | **size 列可配置显隐 / 宽度** | 当前固定 5 列、恒显。窄 pane 场景已有截断兜底，不加开关。 |

---

## 10. 文档同步（实施后）

F1 以下章节会因本方案过期，**实施落地后再更新**（本次按用户要求不改 F1）：

| F1 章节 | 过期点 |
|---------|--------|
| §7 State 结构 | `entry` 加字段、删 `meta*` 五字段 |
| §10 渲染 | 行布局加 size 列、git 退左 |
| §10.1 末行元数据 | 改为 perms+size+mtime 详情栏，删懒 stat 描述 |
| §16 item 3 | symlink isDir 二义消除（lstat 后同源） |
| §16 item 4 | 目录 size 仍留空（结论不变，描述微调） |

---

## 附：与 F0 / F1 的对应

| 本方案 | 上游文档 |
|--------|---------|
| §1.1 末行 size 痛点 | F1 §10.1（末行元数据现状） |
| §2 数据预取进 entry | F1 §7（entry / State 结构） |
| §3 size 列布局 | F1 §10（文件列表渲染格式） |
| §4 末行改 perms+size+mtime | F1 §10.1 |
| §5 仅改 microNeo 自有文件 | 项目"原生侵入最小"原则 |
