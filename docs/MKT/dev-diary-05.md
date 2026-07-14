# microNeo 开发日记 #5：markdown 渲染管线漏掉了 tab

`runewidth.RuneWidth('\t')` 返回 0。但 tab 是终端里**唯一一个「宽度由上下文决定」的字符**——同一颗 tab，在列 0 后展开成 4 格，在列 2 后展开成 2 格。它的宽度不在自己身上，在它所在的位置上。

microNeo 的 normal / blockquote 渲染路径，就栽在了这件事上。

---

## 1. 现场

我在 sample.md 里随手写了两个测试用例：

```
	normal 1 tab
a	b
```

第一行是行首一个 tab + "normal 1 tab"，第二行是 "a" + tab + "b"。我希望它们渲染出来是这样的：

```
    normal 1 tab     ← 第 4 列起（tabSize=4）
a   b                ← a 在列 0，b 在列 4（不是列 2！）
```

实际上我看到的是：

```
 normal 1 tab        ← tab 之后文字紧跟，tab 自己占了 1 列
ab                   ← a 和 b 紧挨着，tab 完全没起作用
```

第一眼反应：「这不就是 tab 没渲染吗？」重看一遍，发现 bug 更深一层——tab **占了 1 列**。不是 0 列，不是 4 列，是**正好 1 列**。这个 1 列是哪来的？

---

## 2. 根因：三层叠加

normal / blockquote 走的是「逐 rune → wrapCells」渲染路径（区别于 codeblock/list 各自的逐行处理路径）。我顺着这条线往下追，发现 bug 是**三层叠加**出来的，单独看每一层都不致命，叠在一起就废了。

### 第一层：renderInline 把 tab 当「其他字符」

`renderInline`（`internal/md/inline.go:205`）处理一行字符串的逻辑是分支匹配的：行内代码、加粗、斜体、链接……最后落到 `default:  其他字符原样输出`。tab 不是任何标记符，于是落到 default，被作为一个 `Cell{Rune: '\t'}` 塞进 cells 数组。

这一步看起来没问题——tab 字符被保留了，没有丢失。但**关键信息已经丢了**：「这是 tab，需要按上下文展开成 N 个空格」。这是个不可逆的损失：cells 数组里只有 rune 本身，**没有它前面有几个字符**的信息。

### 第二层：wrapCells 用 `runewidth.RuneWidth('\t')` 算宽度

`wrapCells`（`wrap.go:44-46`）处理 word wrap 的核心循环是：

```go
rw := runewidth.RuneWidth(c.Rune)
if curWidth+rw > width { /* 断行 */ }
curCells = append(curCells, c)
curWidth += rw
i++
```

对 tab 来说，`rw = runewidth.RuneWidth('\t') = 0`。

- `curWidth+0 > width`？永远不会，所以 tab **永远不会触发换行**。✅ 这其实是好事。
- `curCells = append(...)`：tab **被收进当前行**，但**不推进 curWidth**。这就是 bug。

行首 tab 进入时 `curWidth = 0`，tab 被收进 cells，`curWidth` 还是 0。然后后面 "normal 1 tab" 的 11 个字符各自 width=1，curWidth 推到 11，写到屏幕上时**显示列是 11，但切片下标是 12**（第 0 列是那个 tab 字符）。

### 第三层：写屏用切片下标当显示列

`bufwindow_md.go:184`：

```go
for col, cell := range row.Cells {
    screen.SetContent(col+x0, y, cell.Rune, ...)
}
```

`col` 是切片下标，**不是显示列**。tab 在切片第 0 个位置，就画在屏幕第 0 列。然后 "n" 在切片第 1 个位置，画在屏幕第 1 列。视觉上 tab 占 1 列、"n" 占 1 列——正好就是 "tab=1 列 + 后续紧跟"。

三层叠加的链条很清楚：

```
tab 没有宽度信息
    ↓
wrapCells 把它当 0 宽字符
    ↓
它没推进 curWidth，但占了一个 cell 槽位
    ↓
写屏时切片下标当显示列
    ↓
tab=1 列 + 后续文字左移贴上 = 渲染错乱
```

每一层都「看起来没问题」，单独看都符合「逐 rune 处理」的预期。问题出在**预期本身**——逐 rune 处理假设每个字符都有确定的显示宽度，tab 是反例。

---

## 3. 正确答案早就存在：codeblock 怎么做的

打开 `render_codeblock.go:180-198`，我看到了完全正确的实现：

```go
col := 0
for _, r := range line {
    if r == '\t' {
        ts := tabSize - (col % tabSize)   // ★ 到下一个 tab stop 的距离
        for j := 0; j < ts; j++ {
            contentCells = append(contentCells, Cell{
                Rune:    ' ',
                Style:   codeStyle,
                BufLine: lineIdx,
                BufX:    runeIdx,         // ★ BufX 全指向 tab 原始位置
            })
        }
        col += ts
    } else {
        rw := runewidth.RuneWidth(r)
        contentCells = append(...)
        col += rw
    }
}
```

两个关键设计：

**第一**，tab 在循环里被**提前识别**，展开成 `ts` 个空格 cell，`ts` = 当前列到下一个 tab stop 的距离。`ts = tabSize - (col % tabSize)`，是这个算法的标准写法——当 `col` 已经在 tab stop 上时（比如 4、8、12），`ts = tabSize`，展开 4 格；当 `col=2` 时，`ts=2`，展开 2 格。这正是 tab 「宽度由上下文决定」的算法实现。

**第二**，所有展开出来的空格，**BufX 全部指向 tab 字符在原 buffer 行里的 rune 偏移**。这是后文会展开的 trick。

打开 `render_list.go:44, 132-141`，list 也有类似的处理（缩进 tab 计 4 列，内容 tab 展开 4 空格）。**codeblock 和 list 都做对了，只有 normal 和 blockquote 漏了**。后两者之前没用过 `cfg.TabSize` 字段——这个字段其实早就存在，只是 normal/blockquote 路径从来没消费过。

到这里问题变简单了：把 codeblock 的做法**复制**到 normal 和 blockquote。

---

## 4. 一个解法解决两个问题：BufX 的 trick

把 codeblock 的循环移植到 `renderInline` 的时候，我意识到这不只是「展开 tab」——它在**顺手解决 BufX 映射问题**。

BufX 是什么：每个屏幕 Cell 都带着自己对应的 buffer 坐标（行号 + 行内 rune 偏移）。光标点击屏幕 → 查 BufLine/BufX → 跳到 buffer 对应位置。这个映射在 markdown 渲染里非常关键（点击表格边框、点击代码块行内字符……都靠它）。

如果我把 tab 展开成 4 个空格，**这 4 个空格的 BufX 应该填什么**？填 tab 后面的字符位置？不行——tab 后面的字符位置是 4 个不同的 rune index，但 4 个空格对应的是**同一个 tab**。填连续递增的 rune index（`runeIdx, runeIdx+1, runeIdx+2, runeIdx+3`）？更不行——buffer 里**没有这 4 个字符**，查询时会查到一个不存在的字符。

代码里的解法（也就是 codeblock 已经验证过的做法）是：**4 个空格全部填 `runeIdx`（tab 自己的位置）**。这样查询的时候，4 个空格都映射回 tab 字符本身——而 tab 在 markdown 语法里**没有特殊着色**（不像 `*` `~~` 这种标记符），查询时拿到的颜色就是 default 色，和周围的空格无视觉差异。

一个写法同时解决了：
- tab 网格对齐（核心需求）
- BufX 颜色查询（零额外成本）

如果 BufX 处理逻辑反过来——比如展开后 rune index 错位——那 BufX 的修复会比 tab 展开本身更费劲。**复用 codeblock 的现成做法等于白送 BufX 正确性**。

这就是为什么方案最终选「在 `renderInline` 内部展开 tab」而不是「在 `linesFromBuf` 字符串层把 tab 替换成空格」——后者改了 buffer 读取层，要重新算 rune index 映射，前功尽弃。

---

## 5. 实施：两个 renderer，各加一段分支

实际改动就是两个分支判断，加上 col 累加。

### renderInline（inline.go）

主循环最开头——必须**先于所有标记判断**——加 tab 分支：

```go
if r == '\t' {
    ts := tabSize - (col % tabSize)
    for j := 0; j < ts; j++ {
        cells = append(cells, Cell{
            Rune:    ' ',
            Style:   baseStyle,
            BufLine: bufLineOffset,
            BufX:    runeIdx,    // 全部指向 tab 原始位置
        })
    }
    col += ts
    runeIdx++
    continue
}
```

tab 必须最先处理——否则会被「其他字符原样输出」的 default 分支吃掉。

然后是 col 累加改造。`renderInline` 有 7+ 个字符出口（行内代码、粗体、斜体、粗斜体、删除线、链接、其他字符），每个出口都要正确累加 `col`。这一段机械但要耐心——漏一个就 tab 网格错位。

### RenderBlockquote（render_blockquote.go）

blockquote 不走 renderInline（因为 highlighter 对 `>` 行是整行匹配，行内规则不生效），它自己有逐 rune 循环。同样的 tab 分支加进去。

### 一个计划之外的偏差

原本的计划文档（`docs/normal与blockquote的tab展开修复.md` §2 末尾）写的是：blockquote 内容区前有 │ 2 列前缀，但那是 wrapCells 之后才拼的。renderInline 看到的内容字符串起始列是 0，所以 tab 网格从 0 算。这个 2 列偏差可接受。

我写计划的时候信了这条——> 行里的 tab 在视觉上跟普通段落里的 tab 对齐就行，没必要跟原生编辑模式对齐。

实施时改主意了。打开 native micro 的代码看了一眼：

```go
// native softwrap.go:97 之类
// tab 在编辑模式里是按整行 col（包括行号 + 前面内容）算的
```

原生 micro 编辑模式下的 `> ` 行，光标在引用内容里时按 Tab，编辑器把光标移到 **整个窗口的第 4 列**，不是「内容区第 4 列」。换言之，blockquote 的 tab 网格**本来就该跟整行对齐**，偏移 2 列反而是 bug。

于是实施时改成了：`col := len(prefixCells)`（= 2），tab 网格从绝对列算起。这样：

- `> \tquote` → │ 之后 2 个空格到列 4，quote 从列 4 起（而不是从列 2 起）
- `> a\tb` → │ + a 到列 3，b 从列 4 起

这是计划文档没写、但实施时改了的细节。

---

## 6. 不抽 helper 的 3 个理由

实施到一半我盯着代码看了一会儿——render_codeblock / render_normal(renderInline) / render_blockquote 三处的 tab 展开逻辑长得**几乎一样**：

```go
ts := tabSize - (col % tabSize)
for j := 0; j < ts; j++ {
    cells = append(cells, Cell{...})
}
col += ts
```

本能反应是抽个 helper：

```go
func expandTab(cells []Cell, col, tabSize, runeIdx, bufLine int, style tcell.Style) ([]Cell, int)
```

想了想没抽。3 个理由：

**第一，签名笨重**。helper 要同时维护两个可变状态——`cells` 要 append（slice 在 Go 里是值传递，返回值赋值），`col` 要累加（也是返回值）。签名会变成 `func(dst, col, tabBufX, bufLine, tabSize, style) ([]Cell, int)`——6 个参数、2 个返回值。调用方写出来是：

```go
cells, col = expandTab(cells, col, runeIdx, bufLineOffset, tabSize, baseStyle)
```

这一行**比内联版本还长**。Go 没有元组返回，多返回值的 helper 调用成本很高。

**第二，与现有代码割裂**。`render_codeblock.go:180-198` 本来就内联写、没抽 helper。如果 normal/blockquote 抽了，会出现「codeblock 内联、normal/blockquote 调 helper」的不一致状态——阅读代码的人会想「为什么这一个抽了、那两个没抽？」比抽之前更难懂。

**第三，`ts := tabSize - (col % tabSize)` 是通用 idiom**。这个写法在 C / Python / Go / Shell 里到处都这么写 tab 展开，是大家都认得的 idiom。一眼可读。封装它等于封装 `i++`——既不必要，又损失可识别度。

**抽 helper 是有成本的**：阅读跳转、调用开销、参数解释。代码重复本身不是病——**抽象收益 < 阅读跳转成本**的时候，就别抽。这次就是这种时候。

这条原则不绝对。如果未来第 4 个、第 5 个 renderer 都要展开 tab，到时候再抽也来得及。**提前抽象**比**延后抽象**危险得多。

---

## 7. 验证：sample.md 端到端

我在 `docs/sample.md` 加了一段 `# Tab Rendering`，覆盖：

| 场景 | 期望（修复后）|
|---|---|
| `\tnormal 1 tab`（行首 1 tab）| normal 1 tab 从列 4 起 |
| `\t\tnormal 2 tabs`（行首 2 tab）| normal 2 tabs 从列 8 起 |
| `a\tb`（行中 tab）| a 在列 0，b 在列 4 |
| `> \tquote`（blockquote 行首 tab）| │ + 2 空格 + quote（quote 从列 4 起）|
| `> a\tb`（blockquote 行中 tab）| │ + a + 2 空格 + b（b 从列 4 起）|

`make build-quick`，打开 sample.md，肉眼对照每个 tab 是否对齐到 tab 网格。同时：

- codeblock / list 行为不变——它们有自己的 tab 处理，本修复没碰
- 非 tab 字符渲染不变——`renderInline` 的 7+ 个字符出口我都加了 col 累加（手数了一遍）
- 现有单测全绿——`go test ./internal/md/... -v`

editMode（按 ESC 切换回原生 micro 渲染）不受影响——native micro 的 tab 处理本来就对，本修复只动 MD 渲染路径。


---

microNeo 是开源的：[github.com/sollawen/microNeo](https://github.com/sollawen/microNeo)。
- 一行命令安装
- 可以和OpenCode、Claude、Pi这类的agent针对文档进行讨论
- 能当 `$EDITOR` 给 Claude Code、Yazi 之类的工具用。
