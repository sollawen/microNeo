# Normal / Blockquote 行的 Tab 展开修复

> 状态：**计划中**（PLAN 模式产出，未实现）
> 关联文件：`internal/md/inline.go` / `render_normal.go` / `render_blockquote.go`
> 关联 bug：normal/blockquote 行首（或行中）tab 被当成 0 宽字符，导致缩进错乱

---

## 1. 问题

### 现象

对 buffer 行 `"\thello"`（行首一个 tab + hello），microNeo 在 normal / blockquote 渲染路径下，屏幕表现为「tab = 1 列占位」，整行后续文字只右移 1 列，**不对齐 tab stop 网格**。多个 tab 各占 1 列，完全错乱。

### 根因（已验证）

normal / blockquote 走的是「逐 rune → `wrapCells`」路径，全程没有 tab 展开逻辑：

1. **`renderInline`（inline.go:205-213）**：tab 不匹配任何行内标记符，落到「7. 其他字符：原样输出」，作为一个 `Cell{Rune:'\t'}` 塞进 cells。
2. **`wrapCells`（wrap.go:44-46）**：`runewidth.RuneWidth('\t')` 返回 **0**（实测确认）。tab 既不触发换行，也不推进 `curWidth`，只占一个 cell 槽位。
3. **`appendRow`（wrap.go:188-196）**：tab 占一个 cell，`col += 0`。
4. **写屏（bufwindow_md.go:184）**：`for col, cell := range row.Cells` 用切片下标当屏列，tab 画在列 0，后续文字全部左移贴上。

### 对比：已正确处理 tab 的 renderer

| renderer | tab 处理 | 位置 |
|---|---|---|
| **RenderCodeBlock** | ✅ 读 `cfg.TabSize`，`ts = tabSize - (col % tabSize)` | render_codeblock.go:180-194 |
| **RenderList** | ✅ 缩进 tab 计 4 列，内容 tab 展开 4 空格 | render_list.go:44, 132-141 |
| **RenderNormal** | ❌ 未展开 | inline.go:205-213 |
| **RenderBlockquote** | ❌ 未展开（自己逐 rune，不走 renderInline） | render_blockquote.go:35-50 |

关键参考——**codeblock/list 展开 tab 时，所有展开空格的 `BufX` 都指向原始 tab 的 rune 偏移**：

```go
// render_codeblock.go:194
ts := tabSize - (col % tabSize)
for j := 0; j < ts; j++ {
    contentCells = append(contentCells, Cell{
        ..., BufX: runeIdx,   // ★ 全部指向 tab 的原始位置
    })
}
```

这是 `BufX` 映射的解法：查 `lineStyles[bufLine][BufX]` 时，所有展开空格都拿到 tab 字符本身的颜色（在 markdown 语法里 tab 通常无特殊着色，落到 default 色，正确）。

---

## 2. 方案

### 核心决策：在 `renderInline` 内部展开 tab

候选方案对比：

| 方案 | 做法 | 优点 | 缺点 |
|---|---|---|---|
| A. 字符串层展开 | `linesFromBuf` 后把 `\t` 替换成空格 | 改动小 | ❌ 展开后 rune index 错位，`renderInline` 用 runeIdx 作 BufX 会错，需额外维护映射 |
| **B. renderInline 内部展开** ✅ | 循环开头加 tab 分支 | BufX 天然正确（复用 codeblock 的做法）；只改一处 | renderInline 需新增 `col` 追踪 |
| C. 新 helper | renderer 层调用 expandTab(cells) | 复用 | cells 已含 BufX，再展开要回溯改 BufX，绕 |

**选 B**：和 codeblock/list 完全一致的展开语义，BufX 处理零额外成本。

### `cfg.TabSize` 已就绪

`MDConfig.TabSize`（config.go:43）由 `initMDConfig`（bufpane_md.go:25）从 `buf.Settings["tabsize"]` 填入。renderInline 已接收 `cfg` 参数，直接 `cfg.TabSize` 读取即可——目前只有 codeblock/list 用了这个字段，normal/blockquote 路径完全没消费。

### Blockquote 的 prefix 偏移问题

blockquote 内容区前有 `│ ` 2 列前缀，但那是 `wrapCells` **之后**才拼的（render_blockquote.go:73）。renderInline 看到的内容字符串起始列是 0，所以 tab 网格从 0 算。

这个 2 列偏差**可接受**，理由：
- codeblock 也是内容区从 0 算 tab 网格（render_codeblock.go:194 的 `col` 同样不含前缀），行为一致。
- markdown 的 blockquote 内容本就有缩进语义，内容区独立对齐网格比全局对齐更合理。
- 视觉上几乎无感知。

---

## 3. 详细改动

### 改动 1：`renderInline` 增加 tab 展开分支（inline.go）

**位置**：主循环开头，在「1. 行内代码」判断**之前**插入 tab 分支（tab 必须最先处理，否则会被「其他字符」吃掉）。

**新增 col 追踪 + tabSize 兜底**：在函数开头加（与 render_codeblock.go:180-183 对齐）：

```go
col := 0
tabSize := cfg.TabSize
if tabSize <= 0 {
    tabSize = 4
}
```

并在所有字符出口累加 `runewidth.RuneWidth(r)` 推进 col。

伪代码（插在 `for runeIdx < runeCount` 循环内最前面，tabSize 已在函数开头算好）：

```go
// 0. tab 展开：对齐到 tabSize 网格，全部 BufX 指向 tab 原始位置
if r == '\t' {
    ts := tabSize - (col % tabSize)
    for j := 0; j < ts; j++ {
        cells = append(cells, Cell{
            Rune:    ' ',
            Style:   baseStyle,
            BufLine: bufLineOffset,
            BufX:    runeIdx,
        })
    }
    col += ts
    runeIdx++
    continue
}
```

**col 累加改造点**（renderInline 内所有「输出 Cell」的地方都要加）：

当前 renderInline 用 `for x := ...; x < endIdx; x++` 批量输出标记内容（粗体/斜体/链接等），这些地方 col 累加需要在循环里逐 rune 累加。具体：

- 行内代码块（约 inline.go:46-53）：`col += runewidth.RuneWidth(runes[x])`
- `***` / `**` / `*` / `~~` 内容块：同理
- 链接 `[text](url)` 的 text 输出：同理
- 「7. 其他字符」单字符出口（inline.go:210）：`col += runewidth.RuneWidth(r)`

> ⚠️ 注意：标记符本身（`*`/`~`/`` ` ``/`[`/`]`/`(`/`)`）**不输出 Cell，也不推进 col**（它们被隐藏了），保持现状。只有「输出到 cells 的字符」推进 col。

需要 `import "github.com/mattn/go-runewidth"`（wrap.go 已用，md 包内可直接引用）。

### 改动 2：`RenderBlockquote` 的逐 rune 循环加 tab 分支（render_blockquote.go）

blockquote 没用 renderInline（注释说明 highlight 对 `>` 行整行匹配，行内规则不生效）。所以 tab 展开要在它自己的循环里加。

**位置**：render_blockquote.go:42-51 的 `for _, r := range content` 循环。

伪代码：

```go
col := 0
runeIdx := 0
tabSize := cfg.TabSize
if tabSize <= 0 {
    tabSize = 4
}
cells := make([]Cell, 0, utf8.RuneCountInString(content)+8)
for _, r := range content {
    if r == '\t' {
        ts := tabSize - (col % tabSize)
        for j := 0; j < ts; j++ {
            cells = append(cells, Cell{
                Rune:    ' ',
                Style:   baseStyle,
                BufLine: lineIdx,
                BufX:    runeIdx,
            })
        }
        col += ts
        runeIdx++
        continue
    }
    cells = append(cells, Cell{
        Rune:    r,
        Style:   baseStyle,
        BufLine: lineIdx,
        BufX:    runeIdx,
    })
    col += runewidth.RuneWidth(r)
    runeIdx++
}
```

需要 `import "github.com/mattn/go-runewidth"`。

> 备选：把 blockquote 也改成调 renderInline。但 renderInline 会叠加加粗/斜体/链接等行内属性，改变 blockquote 当前「纯斜体」的行为。**不采纳**，保持最小改动。

### 改动 3：不抽 helper，各 renderer 头部统一兜底 tabSize

改动 1、2 以及现有的 RenderCodeBlock 三处 tab 展开逻辑确实相似（都是 `ts := tabSize - (col % tabSize)` + for 循环 append Cell），但**不抽公共函数**。理由：

1. **签名笨重**：helper 要同时维护两个可变状态（`dst` append + `col` 累加），签名会变成 `func(dst, tabBufX, bufLine, col, tabSize, style) ([]Cell, int)`——6 参 2 返回值，调用方还要重新赋值。抽象收益 < 阅读跳转成本。
2. **与 codeblock 现状不一致**：render_codeblock.go:180-198 本来就内联写、没抽。本次新增的 normal/blockquote 若抽了，会出现「codeblock 内联、normal/blockquote 调 helper」的割裂写法。
3. **`ts := tabSize - (col % tabSize)` 是通用 idiom**：Go 里到处这么写 tab 展开，一眼可读，无需封装。

**统一做法**（与 codeblock 对齐）：各 renderer 在**函数开头**兜底一次 tabSize，循环内直接内联展开。改动 1、2 的伪代码已按此调整。

> 注：RenderList 的 `makeIndentCells`（render_list.go:136）用的是**固定 4 空格**而非 tabSize 网格对齐，语义和 codeblock/normal/blockquote 不同，保持现状，不纳入本次统一。

---

## 4. 验证方式

以 **sample.md 端到端验证**为主（项目惯例：normal/blockquote 渲染本就靠 sample.md 肉眼验证，无单测）。

已在 `docs/sample.md` 补 **# Tab Rendering** 段，覆盖：normal 行首 1/2 tab、行中 tab；blockquote 行首 1/2 tab、行中 tab。`make build-quick` 后打开 sample.md 肉眼对照：

| 场景 | 修复前（bug） | 修复后 |
|---|---|---|
| `\tnormal 1 tab` | tab=1 列，文字紧跟其后 | 对齐到第 4 列（tabSize=4） |
| `a\tb` | a、b 相邻 | a 在列 0，b 在列 4 |
| `> \tquote` | 竖线后 tab=1 列 | 竖线后对齐到内容区第 4 列 |

回归：codeblock / list 的 tab 渲染不受影响（已有正确逻辑，未改动）；非 tab 字符渲染不变。

---

## 5. 风险

### R1：renderInline 的 col 累加漏改（中风险）

renderInline 有 7+ 个字符出口（行内代码、粗体、斜体、粗斜体、删除线、链接、其他字符）。每个出口都要正确累加 col，漏一个就会让后续 tab 的网格对齐算错。

**缓解**：改动 3 的 helper 集中逻辑；测试用例 4（tab 与标记混用）专门覆盖。

### R2：BufX 与 highlight 颜色（低风险）

展开的空格 BufX 指向 tab 原始位置，查 lineStyles 时拿到 tab 字符的颜色。markdown.yaml 里 tab 一般无特殊着色 → default 色 → 视觉上和周围一致。codeblock 已验证此行为正确。

**缓解**：手动验证带语法高亮的 md 文件，确认 tab 区域颜色不突兀。

### R3：wrapCells 与展开后 cells 的交互（低风险）

展开后的 cells 全是空格（runewidth=1），wrapCells 处理空格无特殊逻辑（trimTrailingSpaces 会去尾部空格——但 tab 展开的空格在行首/行中，不受影响）。已分析，无新增风险。

**缓解**：wrap_test.go 现有用例 + 新增用例覆盖。

### R4：editMode 回退路径（低风险）

editMode=true 时光标所在 seg 走 `renderSegmentNative`（原生 micro 渲染），不走 renderInline。原生路径本来就有正确的 tab 处理（softwrap.go:97 等）。本修复**只影响 MD 渲染路径**，editMode 不受影响。

---

## 6. 验收标准

- [ ] `"\thello"` 在 normal 段渲染为「4 空格 + hello」（tabSize=4），而非「1 列 + hello」。
- [ ] blockquote `"> \tx"` 渲染为「│ + 4 空格 + x」。
- [ ] tab 网格对齐：`"a\tb"`（tabSize=4）→ a 在列 0，b 在列 4（不是列 2）。
- [ ] 所有展开空格的 BufX 指向原始 tab 位置（单测断言）。
- [ ] codeblock / list 行为不变（已有 tab 处理，不碰）。
- [ ] `go test ./internal/md/... -v` 全绿。
- [ ] 原生 micro 代码零改动（所有改动在 `*_md.go` / `md/` 内）。

---

## 7. 不做的事

- ❌ 不改 `linesFromBuf`（保持 buffer 读取层纯净）。
- ❌ 不改 codeblock / list 的 tab 处理（它们已正确，且语义独立——codeblock 是硬切网格，list 是固定 4 空格，不应强统一）。
- ❌ 不让 blockquote 改走 renderInline（会改变其纯斜体行为，超出本修复范围）。
- ❌ 不处理「tab 占位符字符」（micro 的 `indentchar` 设置渲染 `→` 等可视 tab 符）——那是编辑模式的可见性增强，阅读模式下应展开为空格，与 micro 默认阅读体验一致。
