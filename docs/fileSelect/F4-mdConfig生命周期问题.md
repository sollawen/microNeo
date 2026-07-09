# F4 — mdConfig 生命周期修复（实施方案）

**状态：已实施并验证通过（2026-07-09 实测，`docs/sample.md` 含代码块/标题/列表等场景渲染正常）。**

**发现入口：** 测试 F3 welcome 时，从选择器打开 MD 文件渲染异常；进一步测试确认是通用问题，与 welcome 无关。

---

## 1. 问题

复用一个 pane 换 buffer 时，MD 渲染配置（mdConfig）不刷新，导致换进来的 MD 文件渲染退化。下表既是问题现象，也是修复后的验证基准（见 §6）。

| 启动方式 | 之后在程序内打开 MD 文件 | 修复前 | 修复后 |
|----------|--------------------------|--------|--------|
| `microneo a.md`（命令行直接开 MD） | — | ✅ | ✅（回归保护） |
| `microneo a.go`（命令行开非 MD） | `:open b.md` / `:file` / welcome 选择器 | ❌ | ✅ |
| `microneo`（无参数 → welcome，初始空 buffer） | 选择器打开 MD | ❌ | ✅ |

**坏的表现在**（全部源于 mdConfig 零值，flag 全 false）：

- 代码块：`MDCodeBlock=false` → `render_codeblock.go:141` 退化为纯文本（无边框）
- 标题：`MDHeading=false` → `render_heading.go:16` 退化为普通文本（无下划线）
- 表格：`MDTableBorder=false` → `render_table.go:813/844` 不画顶/底边框（中间分隔不受此 flag 控制，故"中间框线有、顶底没有"）

---

## 2. 根因

`mdConfig` 挂在 `BufWindow` 上，由 `initMDConfig`（`bufpane_md.go`）在 `NewBufPaneFromBuf`（创建 pane）时设置**一次**。该函数第一行是守卫：

```go
func initMDConfig(buf *buffer.Buffer, w *display.BufWindow) {
	if !buf.IsMD { return }   // ← 罪魁
	w.SetMDConfig(...)
	w.SetEditMode(true)
}
```

当 pane 的**第一个** buffer 非 MD（.go、空 buffer）时，守卫直接 return，`SetMDConfig` 从未执行，`mdConfig` 停在 Go 结构体零值（flags 全 false、TabSize=0）。之后换进 MD 文件，`OpenBuffer`（`bufpane.go:351`）不调 `initMDConfig`，mdConfig 仍是零值 → 渲染退化。

关键澄清：这不是"GO buffer 把 mdConfig 留在了正确共享值上"，而是"mdConfig 从头到尾一个值都没拿到过"。

colorscheme 不受影响，是因为 `ensureMDConfigReady`（`bufwindow_md.go:55`）每帧渲染时懒刷新 colorscheme——但这套懒刷新只管 colorscheme，不管 flags，所以现象是"flag 全 false"而非"全黑"。

---

## 3. 方案：去掉 IsMD 守卫，无条件预置

**核心判断：mdConfig 的值是 common/shared settings，所有 buffer 一致。** 既然值不变，pane 创建时一次性设成正确值即可，之后换任何 buffer 都不用再动——`OpenBuffer` 不刷新也无所谓，因为它持有的本就是正确的共享值。

做法：删掉 `initMDConfig` 第一行的 `if !buf.IsMD { return }`，让 mdConfig + editMode 在 pane 创建时**无条件**设置，不论首文件是否 MD。

类型断言安全性已确认：md\* flags 注册在 `defaultCommonSettings`（`config/settings_md.go:5-11`），**任意 buffer**（MD 或非 MD）的 `.Settings` 都有这些 key，去掉守卫后 `buf.Settings["mdcodeblock"].(bool)` 不会 panic。

### 考虑过的替代方案：flags 也并入每帧懒刷新

另一个可行做法是把 7 个 flags + TabSize 也并入 `ensureMDConfigReady`，每帧从 `buffer.Settings` 现刷（和 colorscheme 一样）。它比"去掉守卫"多覆盖两种场景：运行时 `:set mdcodeblock off`、`setlocal` 单独设 flag。但这两种场景实际概率极低，且是 pre-existing 局限（见 §5）。相比之下，去掉守卫的修法：

- 改动最小（一个 `*_md.go` 函数，删一行 + 改注释），零原生侵入。
- 覆盖全部已记录场景。
- 不引入每帧额外开销。

替代方案的可行性（flags 是 common settings、类型断言对所有 buffer 安全、`ensureMDConfigReady` 调用点现成）均已确认，留作未来若需支持"运行时改 flag"时的升级路径，本次不做。

---

## 4. 改动点

**唯一改动文件：`internal/action/bufpane_md.go`（`*_md.go`，无原生侵入）。**

**改前：**

```go
// initMDConfig 在创建 BufWindow 后立刻同步 MD 渲染配置。
// 仅当 buffer 是 MD 文件时才生效，非 MD 文件为 no-op。
// 该函数封装所有 MD 相关逻辑，让 bufpane.go 的 NewBufPaneFromBuf 保持简洁。
func initMDConfig(buf *buffer.Buffer, w *display.BufWindow) {
	if !buf.IsMD { // 单一真源（NewBuffer 算了 IsMarkdownFile）   ← 删除
		return                                                       // ← 删除
	}

	w.SetMDConfig(md.MDConfig{
		...（实参块见下，不变）...
	})
	// Step 1.0：MD 文件默认进入编辑模式（光标所在 segment 回退原生显示）
	w.SetEditMode(true)
}
```

**改后：**

```go
// initMDConfig 在创建 BufWindow 后预置 MD 渲染配置。
// 无条件执行：mdConfig 的 flag/tabsize 来自 common settings（各 buffer 一致），
// editMode 对非 MD 文件为 dead value（displayBufferMD 仅对 MD 调用，observeEditModeToggle
// 开头也有 IsMD 守卫），设了也无害。
// 无条件预置避免了「首个 buffer 非 MD → mdConfig 零值 → 后续换进 MD 渲染退化」的生命周期 bug。
func initMDConfig(buf *buffer.Buffer, w *display.BufWindow) {
	w.SetMDConfig(md.MDConfig{
		...（实参块见下，不变）...
	})
	// Step 1.0：MD 文件默认进入编辑模式（光标所在 segment 回退原生显示）
	w.SetEditMode(true)
}
```

**实参块（改前 / 改后完全相同，只展示一次）：**

```go
		Colorscheme: md.MDColorscheme{
			DefStyle: config.DefStyle,
			Styles:   config.Colorscheme,
		},
		TabSize:        util.IntOpt(buf.Settings["tabsize"]),
		MDTableAlign:   buf.Settings["mdtablealign"].(bool),
		MDTableBorder:  buf.Settings["mdtableborder"].(bool),
		MDBoldItalic:   buf.Settings["mdbolditalic"].(bool),
		MDCodeBlock:    buf.Settings["mdcodeblock"].(bool),
		MDHeading:      buf.Settings["mdheading"].(bool),
		MDList:         buf.Settings["mdlist"].(bool),
		MDLink:         buf.Settings["mdlink"].(bool),
```

delta（改前 → 改后，仅两处）：

1. **重写函数 doc 注释**：原注释称"仅 MD 生效、非 MD 为 no-op"，已不准确。
2. **删除守卫**：函数体首部的 `if !buf.IsMD { return }`（含行尾注释）整体删去。

`SetMDConfig` 实参块、`SetEditMode(true)` 调用及其上方注释：**原样不变**。

**不改动**：`OpenBuffer`、`NewBufPaneFromBuf` 调用点、`SetBuffer`、`ensureMDConfigReady`——全部保持原样。本方案下 `OpenBuffer` 无需补调 `initMDConfig`（值已正确且共享）。

---

## 5. 附带修复 与 不在范围内

### 附带修复：editMode 的生命周期

原疑虑：editMode 也设在 `initMDConfig` 内，是否有同样的生命周期问题？去掉守卫后，每个 pane 创建时 editMode 一律设 true——无论首文件类型。于是"GO 起步、后换进 MD"的 pane，换进来的 MD 正确处于编辑模式（与 `microneo b.md` 直接打开一致），而不是停在零值 false（阅读模式）。该疑虑随之消解，无需额外改动。

### 不在本次范围内（pre-existing 局限，已知接受）

| 场景 | 本次是否覆盖 | 说明 |
|------|--------------|------|
| 首个 buffer 非 MD，后换进 MD | ✅ 覆盖 | 本次目标 |
| 运行时 `:set mdcodeblock off` 改全局 | ❌ 不覆盖 | 需重建 pane 才生效；原代码即如此，非回归 |
| `setlocal` 给某 buffer 单独设 md flag，再换进来 | ❌ 不覆盖 | 同上 |

colorscheme 的运行时切换（`:colorscheme`）仍由 `ensureMDConfigReady` 懒刷新覆盖，本次不动。

---

## 6. 验证

复用 §1 矩阵逐项手测，重点看三个退化判断点：`render_codeblock.go:141`（代码块边框）、`render_heading.go:16`（标题下划线）、`render_table.go:813/844`（表格顶/底边框）。

1. `microneo a.md` → 三项正常。（**回归**：原本就正常，确认没改坏。）
2. `microneo a.go` → `:open b.md` → 三项正常。（核心修复点。）
3. `microneo a.go` → `:file` 选 b.md → 三项正常。
4. `microneo a.go` → welcome 选择器开 MD → 三项正常。
5. `microneo`（无参 → welcome）→ 选择器开 MD → 三项正常。
6. 非 MD 文件（a.go）本身渲染不受影响：普通代码高亮正常，无 MD 装饰泄漏。

---

## 7. 相关代码位置

| 文件:行 | 作用 | 本次是否改 |
|---------|------|-----------|
| `internal/action/bufpane_md.go` | `initMDConfig`（删守卫 + 改注释） | ✅ 改 |
| `internal/action/bufpane.go:280` | `NewBufPaneFromBuf`（调 `initMDConfig`） | 不改 |
| `internal/action/bufpane.go:351` | `OpenBuffer`（换 buffer，本方案下无需补调） | 不改 |
| `internal/display/bufwindow_md.go:44` | `SetMDConfig` | 不改 |
| `internal/display/bufwindow_md.go:55` | `ensureMDConfigReady`（colorscheme 懒刷新，保留） | 不改 |
| `internal/md/render_codeblock.go:141` | `if !cfg.MDCodeBlock` 退化判断（验证用） | 不改 |
| `internal/md/render_heading.go:16` | `if !cfg.MDHeading` 退化判断（验证用） | 不改 |
| `internal/md/render_table.go:813,844` | `if cfg.MDTableBorder` 顶/底边框（验证用） | 不改 |
