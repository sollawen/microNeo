# F4 — mdConfig 生命周期问题（MD 渲染配置未随 buffer 切换刷新）

> 状态：**问题记录**，未定方案。下次讨论。
>
> 发现入口：测试 F3 welcome 时，从选择器打开 MD 文件渲染异常；进一步测试确认是通用问题，与 welcome 无关。

---

## 1. 现象（用户实测）

| 启动方式 | 之后在程序内打开 MD 文件 | 渲染结果 |
|----------|--------------------------|----------|
| `microneo a.md`（命令行直接开 MD） | — | ✅ 正常 |
| `microneo a.go`（命令行开非 MD） | `:open b.md` / `:file` / welcome 选择器 | ❌ 坏 |
| `microneo`（无参数 → welcome，初始 pane 是空 buffer） | 选择器打开 MD | ❌ 坏 |

**坏的表现在**（全部源于 mdConfig 零值，flag 全 false）：

- **代码块**：`MDCodeBlock=false` → `render_codeblock.go:141` 退化为纯文本（无边框），观感上"没渲染"
- **标题**：`MDHeading=false` → `render_heading.go:16` 退化为普通文本（无下划线）
- **表格**：`MDTableBorder=false` → `render_table.go:813/844` 不画顶/底边框；表格本体（单元格 + 中间分隔）不受此 flag 控制 → "中间框线有，顶/底边框没有"

一句话：**pane 创建时第一个文件是什么，决定该 pane 的 mdConfig**。第一个不是 MD，之后换 MD 进来就坏。

---

## 2. 根因（精确机制）

**澄清：microNeo 没有全局 MD 开关。** 现状分三层：

| 状态 | 位置 | 粒度 | 是否正确 |
|------|------|------|----------|
| `buf.IsMD` | `buffer.Buffer` 字段 | per-buffer | ✅ 正确（`NewBuffer` 按扩展名算） |
| MD 渲染触发 | `Display()` 按 `buf.IsMD` 路由到 `displayBufferMD` | per-pane（按当前 buffer） | ✅ 正确 |
| `mdConfig`（flags + colorscheme） | `BufWindow.mdConfig` 字段 | per-BufWindow | ⚠️ **生命周期错误** |

问题在第三层：

- `initMDConfig(buf, w)`（`bufpane_md.go:15`）负责把 `buffer.Settings` 里的 `MDCodeBlock`/`MDHeading`/`MDTableBorder`/`MDTableAlign`/`MDBoldItalic`/`MDList`/`MDLink`/`TabSize` + colorscheme 塞进 `w.mdConfig`，并设 `editMode=true`。
- 它**只在 `NewBufPaneFromBuf`（`bufpane.go:284`）被调用一次**——也就是"创建 pane"那一刻。
- **`OpenBuffer`（`bufpane.go:351`）换 buffer 时不调用 `initMDConfig`**。所以 pane 换了新 buffer 后，`mdConfig` 仍是 pane 初始 buffer 的配置：
  - 初始 buffer 非 MD（如 .go、空 buffer）→ `initMDConfig` 当时是 no-op（`IsMD=false` 直接 return）→ `mdConfig` 从未被设置，**全零值**。
  - 之后换进 MD 文件 → 用全零 `mdConfig` 渲染 → 第 1 节全部症状。

### 一个关键细节：colorscheme 会自愈，flags 不会

`ensureMDConfigReady()`（`bufwindow_md.go:55`）在渲染时按指针比较**懒刷新 colorscheme**——所以 colorscheme 即使初始为空也会自愈，颜色不会全黑。但**这个懒刷新只管 colorscheme，不管 flags**。flags 只在 `initMDConfig` 设一次。

这正是为什么现象是"flag 全 false"而非"全黑"——也说明代码里已经有"渲染时按需刷新"的先例（colorscheme），只是没扩展到 flags。

---

## 3. 影响范围

所有走 `OpenBuffer` **换 buffer** 的路径都受影响：

- welcome 选择器打开文件（`welcome_md.go:119`）— 本 bug 的发现入口
- `:open <file>` 命令（`command.go:312`）
- `:file` 选择器（`command_neo.go:102`）

**不受影响**的路径（走 `NewBufPaneFromBuf` 创建新 pane，会调 `initMDConfig`）：

- 命令行直接开文件
- `vsplit` / `hsplit`
- 新 tab

所以核心规律是：**新建 pane 的路径 OK，复用 pane 换 buffer 的路径坏。**

---

## 4. 设计层面的核心问题

**mdConfig 的初始化与"pane 创建"耦合，而不是与"buffer 赋予 pane"耦合。**

在 pane 复用（buffer swap）场景下，配置不跟随 buffer 更新。这与 microNeo 的正常用法冲突：一个 pane 里先后打开不同类型文件是常态（先看 .go 再看 .md；welcome 会话里反复开关文件）。

### 待讨论的问题（本次不求解，仅列出供下次讨论）

1. **mdConfig 应该挂在哪一层？**
   - 现状：per-BufWindow。
   - 方向 A：仍 per-BufWindow，但每次 buffer 赋予时刷新（给所有换 buffer 路径补 `initMDConfig`）。最小改动，但仍是"靠记得在每个换 buffer 处调一次"，容易再漏。
   - 方向 B：per-buffer（配置跟 buffer 走，渲染时取当前 buffer 的配置）。更符合"每个文件自己一套渲染配置"，但要改 BufWindow 读取 mdConfig 的方式。
   - 方向 C：渲染时按需从 `buffer.Settings` 现算（像 `ensureMDConfigReady` 对 colorscheme 那样），彻底取消"预先 set"模式。

2. 现有的 `ensureMDConfigReady`（colorscheme 懒刷新）是不是该扩展到 flags？还是 colorscheme 走懒刷新、flags 走别的机制——本身就不一致，要不要统一？

3. 若走 per-buffer / 渲染时现算，`buffer.Settings → md.MDConfig` 的转换逻辑放哪？目前 `initMDConfig` 在 action 层（`bufpane_md.go`），依赖 config/buffer/md 三个包；下沉到 buffer 或 md 层会不会引入新的包依赖问题？

4. `editMode`（`BufWindow.editMode`）目前也在 `initMDConfig` 里 set——它的生命周期跟 mdConfig 是同一个问题吗？要不要一起重新考虑？

5. 最小过渡方案（给 `OpenBuffer` 补 `initMDConfig`）能否作为短期止血，同时不阻碍后续重新设计？还是说既然要重新设计，先不动、留着一起改？

---

## 5. 相关代码位置

| 文件:行 | 作用 |
|---------|------|
| `internal/action/bufpane.go:280` | `NewBufPaneFromBuf`（调 `initMDConfig`） |
| `internal/action/bufpane.go:351` | `OpenBuffer`（**未调 `initMDConfig`** ← 缺口） |
| `internal/action/bufpane_md.go:15` | `initMDConfig`（设 mdConfig + editMode） |
| `internal/display/bufwindow.go:71` | `SetBuffer`（只换 buf，不碰 mdConfig） |
| `internal/display/bufwindow_md.go:44` | `SetMDConfig` |
| `internal/display/bufwindow_md.go:55` | `ensureMDConfigReady`（只刷 colorscheme） |
| `internal/md/render_codeblock.go:141` | `if !cfg.MDCodeBlock` 退化判断 |
| `internal/md/render_heading.go:16` | `if !cfg.MDHeading` 退化判断 |
| `internal/md/render_table.go:813,844` | `if cfg.MDTableBorder` 顶/底边框 |
