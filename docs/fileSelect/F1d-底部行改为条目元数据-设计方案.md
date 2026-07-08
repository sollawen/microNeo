# F1d · FileSelector 底部行改为「光标条目元数据」设计方案

**状态**：草案（待评审）
**依据**：F0 §7（底部 hint 行，原静态按键提示）+ 源码 `fileselector.go`（`hintText`/`display`/`entry`/`listDirEntries`）
**前置**：F1b 已落地、`:file` 三段布局（面包屑 / 文件列表 / 末行）已稳定。
**交付**：把末行从静态按键 hint 改为「光标所在条目的元数据」——文件显 `size + mtime`，子目录显 `mtime`。size 用人类可读单位（`405B`/`12.3K`/`3.4M`）。
**原生侵入**：零。改动仅限 microNeo 自有文件 `internal/action/fileselector.go`。

---

## 1. 背景与目标

### 1.1 现状

末行（`display` 行 `area.H-1`）固定画 `hintText()` 的输出：

```
Enter 打开  ·  . 显示隐藏  ·  Esc 取消
```

文案随光标位置（Enter 文案）和 dotfile 显隐（`.` 文案）微调，但本质是**静态按键提示**。

### 1.2 问题

- 按键提示用过一次就内化，常驻 = 视觉噪音，信息密度低。
- 文件选择器场景下，用户真正想知道的是「这个文件多大 / 多新」，尤其是面对一堆同名前缀、只能靠元数据区分的文件时。
- F0 §10.2 当初砍掉 size/mtime **列**是因为 24~32 列窄宽放不下；但末行整行宽度（≥18 列）放一条元数据绰绰有余——把列预算让给末行即可。

### 1.3 目标

末行展示光标条目的实时元数据，让末行从「告诉我按键」升级为「告诉我这个文件是什么」。

---

## 2. 数据来源与性能

### 2.1 来源：按需 `os.Stat`（懒查 + 按条目名缓存）

`entry` 当前只存 `name`+`isDir`，元数据不预存。**新增**：光标落到某条目时，对该条目做一次 `os.Stat(filepath.Join(currentDir, name))`，结果缓存进 state，`display` 直接读缓存。

- 用 `os.Stat`（非 `Lstat`）：跟随符号链接，size/mtime 取**目标**的（对齐 `ls`），即「你打开会得到什么」。
- 断链 / 无权限 → `Stat` 返 error，按 §3.5 降级（显 `—` / 省略）。

**缓存键是条目名**：每个文件最多 stat 一次——光标停在它上面时 stat 一回，之后只要光标没换到别的名字，重复重绘都命中缓存、零 stat（§4.2 的 guard 保证）。

### 2.2 实测性能（本地 SSD）

同机实测单次 `os.Stat`：

| 场景 | 单次 stat | 整目录 stat 一遍 |
|---|---|---|
| 小目录（~25 文件） | ~3µs | ~95µs |
| 大目录（/usr/bin，924 文件） | ~6µs | 5.4ms |

即「光标移动一次 = 单次 stat ≈ 3µs」，对人完全无感；命中缓存时（光标没换文件）连这一次都省了。

### 2.3 选懒查而非批量 stat（对齐 ls -l）的理由

读目录（`os.ReadDir`，等价 `ls` 不带 `-l`）**只返回文件名 + 类型，不含 size/mtime**——这些必须对每个条目额外 `stat` 才能拿到（这正是 `ls -l` 比 `ls` 慢的原因）。所以「批量 stat」= 进入目录时把整个目录 stat 一遍。

两种方案的最坏情况不对称：

| | 懒查（本方案） | 批量 stat（ls -l） |
|---|---|---|
| 单次光标移动 | 1 次 stat（~3µs，命中缓存则 0） | 0 次（已预存） |
| 进入目录 | 0 次 | N 次（= 目录条目数） |
| 最坏情况 | **钉死在单次 stat**，目录再大 / 盘再慢也不卡 | 随目录大小线性增长（1 万文件≈几十 ms；网络盘更糟），进目录那一刻可能「顿」 |

批量在小目录无感，但最坏情况随目录大小发散；懒查的最坏情况恒定。**一个「永远不会卡」的方案优于「通常很快、偶尔卡」的方案**，故选懒查。懒查原本的唯一缺点（要在多个 cursor 变化点分别埋刷新调用）已由 §4.2 的「display 内 guard」消除。

---

## 3. 显示设计

### 3.1 总规则

| 光标位置 | 末行内容 |
|---|---|
| 文件 | `size   mtime`（如 `12.3K   01-15 14:30`） |
| 子目录 | size 段**留空但保留固定宽** + `mtime`（如 `       07-01 09:12`），mtime 与文件行落在同一列 |
| 面包屑行（row 0） | 空白（不显 hint，§3.5） |

### 3.2 size：人类可读单位（1024 进制）

算法（对齐 `ls -lh` / 用户示例 `405B` `12.3K` `3.4M`）：

- `< 1024` → 整数 + `B`：`405B`、`1023B`
- `≥ 1024` → 除到 mantissa < 1024，按 mantissa 分档：
  - mantissa `< 100` → **1 位小数** + 单位：`3.4M`、`12.3K`、`99.9K`
  - mantissa `≥ 100` → **整数** + 单位（舍入后 ≥ 1024 则进位到下一单位）：`123K`、`1023K`、`1023.6K → 1.0M`
- 单位阶 `1024`（二进制 KiB 但标签用 K/M/G，与 `ls -lh` 一致）

**输出宽度恒 ≤ 5 列**（`1023B` / `99.9K` / `1023K` 都是 5；`1.0G` 等 4）——这是定宽对齐的前提。关键是不让 `1023.9K` 这种 7 列值出现：靠「≥ 100 去小数 + ≥ 1024 进位」堵住。目录不显 size（§3.1）。实现时加单测覆盖边界：`0`、`1023`、`1024`、`99.9×1024`、`1023.6×1024`（验进位）、`int64 上限`。

### 3.3 mtime：对齐 `ls -lh` 的「同年省年」格式

- **同年**：`MM-DD HH:MM`（11 列），如 `01-15 14:30`
- **跨年**：`YYYY-MM-DD`（10 列，省时分），如 `2023-07-01`

这是 `ls -lh` 的经典格式，用户零学习成本。整行预算（size + 间隔 + mtime）≤18 列，正好 fit minWidth=20 减边框后的内容宽。

### 3.4 行内布局

```
┌─ area.W (≥18) ─────────────────────────┐
│ 12.3K   01-15 14:30                     │   ← 文件
│        07-01 09:12                      │   ← 目录（无 size，左留空对齐）
└────────────────────────────────────────┘
```

- **size 是固定宽字段**（右对齐，约 5 列），mtime 紧随其后、用 2 空格分隔。关键是**目录行也保留这个字段宽**（留空），让 mtime 在文件行 / 目录行都从同一列开始——列对齐才整齐。
- 尾部用 `config.DefStyle` 填空（复用 `drawString` 的 tail-fill）。
- 样式：建议 size 用默认色、mtime 用**稍 dim**（弱化），让 size 更突出。可用 `config.DefStyle` 的 dim 变体或保持默认；属微调，不阻塞。

### 3.5 降级：面包屑行 / stat 失败

- **光标在面包屑行（row 0）**：该行不是具体文件，无元数据，末行**留空**（不显任何 hint）。Esc / Enter / `.` 等按键不靠末行广告——Esc 是 modal 通用退出约定，Enter 是通用激活，`.` 是 power-user 功能，靠交互自然发现。
- **stat 失败**（断链 / 无权限）：缺失的字段（size / mtime）显 `—` 占位，不报错、不阻断。注意 `—`（U+2014 EM DASH）在显示宽度模型里是 **2 列**（非 1 列），`buildMetaLine` 必须把它**补齐到字段定宽**（size 段 5 列、mtime 段 10/11 列），否则会打破 mtime 列对齐。

### 3.6 决定：末行纯元数据，不留任何按键 hint

末行只显示光标条目的元数据，**完全不显示** Enter / `.` / Esc 等按键提示（含面包屑行也留空）。理由：

- 元数据是高频真需求；按键提示是一次性 onboarding，常驻是视觉噪音、性价比低。
- Esc 是 modal 通用退出约定，Enter 是通用激活，`.` 是 power-user 功能——靠交互和约定自然发现，无需末行广告。
- 要彻底干净的元数据行，就别留半句 hint。

---

## 4. 改动清单

### 4.1 state：按条目名缓存的元数据

`fileSelectorState` 增字段（缓存键 = 条目名）：

```go
metaName  string    // 缓存对应的条目名（""=面包屑/空，表示无缓存或该行无条目）
metaSize  int64     // os.Stat().Size()；目录无意义
metaMtime time.Time // os.Stat().ModTime()
metaOK    bool      // false=stat 失败（断链/无权限），字段降级显 —
metaIsDir bool      // = info.IsDir()（跟随符号链接，见下），决定是否显 size
```

`entry` 结构**不变**（仍 `name`+`isDir`，元数据走 state 缓存，不污染 Model）。

**isDir 的两个来源**：`entry.isDir` 来自 `os.ReadDir`（**不跟随**符号链接，symlink→dir 记为 false）；`metaIsDir` 来自 `os.Stat().IsDir()`（**跟随**符号链接）。末行用 `metaIsDir`——对齐 `ls -l`「你打开会得到什么」的语义，symlink→dir 的条目末行按目录布局（size 留空）。这与列表渲染（用 `entry.isDir`）存在已知边界差异，可接受。

### 4.2 刷新机制：display 内单点 guard（覆盖所有 cursor 变化）

新增 `(s) refreshMetaIfStale()`，在 `display` 画末行**之前**调一次：

```go
func (s *fileSelectorState) refreshMetaIfStale() {
    name := s.cursorEntryName()       // 当前光标条目名；面包屑/空 → ""
    if name == s.metaName {
        return                        // 命中缓存，跳过
    }
    s.metaName = name
    if name == "" {                   // 面包屑行 / 无条目
        s.metaOK = false
        return
    }
    info, err := os.Stat(filepath.Join(s.currentDir, name))
    s.metaOK = err == nil
    if err == nil {
        s.metaSize = info.Size()
        s.metaMtime = info.ModTime()
        s.metaIsDir = info.IsDir()
    }
}
```

**为什么放在 display 里、而不是在每个 cursor 变化点调**：display 是所有渲染的唯一入口，不管光标怎么变（↑↓/chdir/toggleHidden/打开），display 必被调到。在它内部用一个「名字变了才 stat」的 guard，就**一处覆盖全部 cursor 变化路径**，无需在 `moveCursor`/`chdirTo`/`toggleHidden`/`Open` 等 4 个点分别埋调用——消除了「漏埋一个点」的维护风险。

**性能**：guard 命中时（绝大多数重绘）= 一次字符串比较，零 stat；miss 时（光标换了文件）= 1 次 stat（~3µs）。重复 Redraw 不会变成 stat 风暴。

**调用位置**：guard 必须在 `display` 所有渲染路径的最早入口（`s == nil` return 之后、任何渲染之前）调一次，确保每条 display 路径都刷新。

**chdir / toggleHidden 场景**：cursor 重定位后 `cursorEntryName()` 返回的 name 必变，guard 自然 miss 并重新 stat，**无需额外 reset 缓存**——这正是「单点覆盖全部 cursor 变化」的体现。

### 4.3 渲染：`display` 末行分支

替换原 `fs.drawString(..., fs.hintText(), ...)`：

```go
s.refreshMetaIfStale()                       // §4.2：miss 才 stat
text := ""
if s.metaName != "" {                        // 面包屑行 / 空目录 → 留空
    text = buildMetaLine(s)                   // size + mtime，按 metaIsDir/metaOK 组装
}
fs.drawString(area.X, hintRow, area.W, text, config.DefStyle)
```

`buildMetaLine`：按 §3.1/§3.4 拼 `humanSize(size)` + 间隔 + `formatMtime(mtime)`；`metaOK=false` 时对应字段替成定宽 `—`；目录（`metaIsDir`）size 段留空但**保留固定宽**，让 mtime 列对齐。

### 4.4 新增工具函数

- `humanSize(n int64) string`（§3.2）
- `formatMtime(t time.Time) string`（§3.3，同年判定用 `time.Now()`）
- `(s) cursorEntryName() string`：返回当前光标条目名。契约：`cursor==0`（面包屑）或 `idx := cursor-1` 越界（`idx < 0 || idx >= len(entries)`）→ 返回 `""`；**绝不 panic**（必须 bounds-check）。供 §4.2 guard 与 §4.3 判断共用。`metaName == ""` 即等价于「光标不在条目上」。

### 4.5 移除 / 弃用

- `hintText()`：完全移除（末行不再有任何 hint 文案）。`cursorRowKind()` 仍被 `handleEvent`/`activate` 用，**保留**。

### 4.6 不改

- `entry` 结构（仍 `name`+`isDir`，元数据按路径 stat，不污染 Model）。
- `listDirEntries` / 排序 / 过滤逻辑。
- git 状态列、面包屑、文件列表渲染。
- 任何 micro 原生文件。

---

## 5. 验证

`make build` 后手测：

- [ ] 光标在**文件**：末行显 `size mtime`（如 `12.3K  01-15 14:30`），单位正确（B/K/M/G 阶跃在 1024）。
- [ ] 光标在**子目录**：末行 size 段留空、mtime 与文件行**列对齐**。
- [ ] 光标在**面包屑行**：末行**空白**，无 hint。
- [ ] **断链 / 无权限**文件：末行缺失字段显 `—` 占位（补齐定宽，不破坏对齐），不崩、不报错。
- [ ] **列对齐**：文件行、目录行、stat 失败行三者的 mtime 从同一列开始，肉眼无偏移（验 `—` 2 列被正确补齐）。
- [ ] **humanSize 边界**：`0B`、`1023B`、`1024→1.0K`、`99.9K`、`1023K`、`1023.6K→1.0M`、极大值，输出宽度恒 ≤ 5。
- [ ] **跨年文件**：mtime 显 `YYYY-MM-DD`（省时分）；同年显 `MM-DD HH:MM`。
- [ ] **性能**：大目录（数百条目）上下连续移动光标无卡顿（每次仅 1 次 stat）。
- [ ] **窄 pane**（minWidth=20）：末行不溢出、不覆盖边框。
- [ ] **回归**：chdir / toggleHidden / pick / cancel 后末行内容随光标正确刷新；git 列、面包屑、截断（右截断，见上一提交）不受影响。

---

## 6. 范围外（记录，不在本次做）

| # | 项 | 说明 |
|---|----|------|
| 1 | **目录条目数** | 目录行可选显「N items」代替无 size；本次按用户要求只显 mtime，item 计数延后。 |
| 2 | **相对时间**（`3h ago`） | 比 `MM-DD HH:MM` 更口语但会随时间陈旧、需定时刷新；本次用绝对时间，相对时间列后续。 |
| 3 | **末行是否留 hint** | 已在 §3.6 决定：纯元数据，不留任何 hint（含面包屑行也留空）。 |
| 4 | **size 按 1000 进制（KB/MB）** | 用户示例与 `ls -lh` 都是 1024 进制标 K/M，本次从之；1000 进制属另一约定，不改。 |

---

## 附：与 F0 / F1 的对应

| 本方案 | 上游文档 |
|--------|---------|
| §1 末行从静态 hint → 元数据 | F0 §7（底部 hint 行）的演进 |
| §3 size/mtime 格式 | F0 §10.2「砍 size/mtime 列」的窄宽替代（挪到末行） |
| §2 懒查 + 按 name 缓存（display 内 guard，miss 才 stat） | F1 §10.7「重操作才异步」取舍的延续 |
| §4 仅改 microNeo 自有文件 | 项目「原生侵入最小」原则 |
