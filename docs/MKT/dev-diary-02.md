# microNeo 开发日记 #2：表格渲染器 table-render

相关文章：「[把一个 line editor 改造成 render editor](https://zhuanlan.zhihu.com/p/2051803011857528091)」

markdown 原始的表格太难看了——几根破竖线、几根破破折号，看着像 Excel 偷工减料后的产物。我盯着这堆我不认真数字数就根本对不齐的表格，寑食难安。

于是我决定在microNeo的第一件事怀就是：把它们画得好看点。

## 1，表格识别

### 第一个难点：**「哪些行属于这个表格」**。

最朴素的判断：每行第一个字符是 `|`，就是表格行。想一想很简单，上手之后发现还是有点复杂：

首先，这是个**多行结构**——一个表至少 3 行（header、separator、body），body 还可以任意多行。你得用一个状态机扫过整个 buffer：当前行是 `|` 开头，就继续收集；碰到不是 `|` 开头的一行，表就结束了。

状态机大概长这样（`internal/md/detect.go:84-86, 123-130`）：

```
case stateNormal:
  if 第一个字符是 '|':
    state = stateTable
    startLine = y     // 记下表的开始行
case stateTable:
  if 还是 '|':
    // 继续收集
  else:
    输出 Segment{startLine, y-1, Render: RenderTable}
    state = stateNormal
```

### 第二个难点：**配对符号**。

考虑这一行：`| 姓名 \`是 "ada"\` | 36 |`。

表面看，有 3 个 `|`，是个 2 列的表。但其实第一个 `|` 在反引号内、第二个 `|` 在双引号内——它们都**不是分隔符**。这一行其实只有 1 列。

所以判断一行「是不是表格行」不能光看 `|` 的位置，还得**跳过反引号 / 双引号 / 单引号的配对范围**，在范围外找 `|`。这就是为什么我那个 `isTableRow` 函数写了 24 行——不是因为我啰嗦，是因为「表格行」这个概念在 markdown 里就比表面复杂 24 行。

配对符号跳过逻辑（`detect.go:191-214`）：

```go
i := 1
for i < len(s) {
    if s[i] == '|' { return true }       // 在配对外找到了 |
    var close byte
    switch s[i] {
    case '`': close = '`'                  // 跳过反引号配对
    case '"': close = '"'                  // 跳过双引号配对
    case '\'': close = '\''                // 跳过单引号配对
    }
    // 配对内全跳...
}
```

### 第三个难点：**未闭合表格**
——扫到 EOF 的时候表还没结束，要不要算？我选「算」：v1.0.0 的最简版不挑，能渲染就渲染；v1.0.6 之后也保留这个行为，因为「半成品表格」总比「吞掉最后几行」好。

> 24 行判一行属不属于表——听起来夸张，做过 markdown 解析的人会懂。
> 配对符号里的 `|`，和裸 `|`，不是同一种 `|`。

## 2，插入装饰行

上一节讲的是「识别」——扫到一个表，记下 `startLine` 到 `endLine`，输出一个 Segment（**渲染片**）。这一节要讲的是渲染片的具体内容——一个 3 行的表，怎么渲染成 5 行的 screen。

### 什么是渲染片（Segment）

第一篇讲过打破 1:1——buffer 的 1 行可以变成 screen 的 N 行。表格是第一个「真的需要打破 1:1」的 markdown 元素：一个 3 行的表必须有 5 行 screen 才像样——上下边框 + header + separator + body。

这 5 行 screen 整体上是一个 Segment，**但内部不是同质的**——有的屏幕行对应某个具体的 buffer 行，有的不对应任何 buffer 行。

所以每个 screen 行都得回答一个问题：「你属于 buffer 哪一行？」答案写在每个 screen 行的 `BufLine` 字段里——这就是**渲染片的"接口"**：它告诉 display 层「这一行要不要显示行号」「这一行算 buffer 的哪一行」。

### 装饰行的归属：属于整个表，不属于任何 buffer 行

top / separator / bottom 这 3 条装饰线，**没有唯一对应的 buffer 行**——它们是「整个表格」的一部分，不是「某一 buffer 行」的延伸。

v1.0.0 那 574 行图省事，硬给每条装饰行塞了个 buffer 行号——top=0、separator=1、bottom=4。结果出现两个**视觉 bug**：
1. **行号显示错乱**：底部边框显示了「第 5 行的行号」，但 buffer 里根本没第 5 行
2. **bottom border 的 BufLine=4**指向一个不存在的行，display 层读取时拿到一个非法值

```
v1.0.0 错误做法（BufLine 一律硬塞）：
┌──────┬─────┐     BufLine=0    ← 错：装饰行不该归属第 0 行
│ name │ age │     BufLine=0
├──────┼─────┤     BufLine=1    ← separator 实际就是第 1 行（语义算凑巧对）
│ ada  │ 36  │     BufLine=2
└──────┴─────┘     BufLine=4    ← 错：buffer 没有第 4 行
```

### 修复：装饰行 BufLine=-1

修复办法很直接：**装饰行的 BufLine 显式标记为 -1**。`-1` 在 md 包里有一个固定含义——「这一行不属于任何 buffer 行」。display 层看到 -1 就跳过——不显示行号、不参与任何"反查 buffer 行"的逻辑。

```
v1.0.6 正确做法（装饰行 BufLine=-1）：
┌──────┬─────┐     BufLine=-1   ← 装饰行：不显示行号
│ name │ age │     BufLine=0    ← header 算 buffer 第 0 行
├──────┼─────┤     BufLine=-1   ← 装饰行：不显示行号
│ ada  │ 36  │     BufLine=2    ← body 算 buffer 第 2 行
└──────┴─────┘     BufLine=-1   ← 装饰行：不显示行号
```

### 两个关键设计点

1. **`-1` 是 sentinel，不是普通的 buffer 行号**。buffer 行号从 0 开始，-1 不可能合法——只要是 -1 就一定是装饰行。display 层不需要额外查表「这是不是装饰行」，光看 BufLine 就知道。
2. **header 的 BufLine 算 buffer 第 0 行**。这是个微妙的判断：header 是 buffer 第 0 行的"内容延伸"，body 第 1 行是 buffer 第 2 行——header 和 body 之间夹了一个 separator 装饰行，但 **BufLine 序号在内容行之间是连续的**（0、2 中间跳 1 是因为 1 给了 separator 装饰行）。这样在 display 层做"行号显示"或"任何 screen→buffer 的反查"时，反查 BufLine 就能拿到正确的 buffer 行。

> 渲染片是一个抽象。`BufLine` 是这个抽象的"接口"——它告诉 display 层「这一行属于 buffer 哪里」。装饰行不存在于 buffer，所以它的接口值是 -1。

## §4 最小 demo → 完美：演进

第三个 false 难点其实不是「难点」，是「**过程**」——从最小 demo 到完美。

v1.0.0 的 574 行**不是起点，是落点**。在那之前，我做了 3 个调研文档：表格需求文档（486 行）、表格现状分析（225 行）、项目调研笔记（97 行）。**文档先行，代码后写**——这是我做 microNeo 的工作流，每个新功能都这样。

但落到代码这一层，**v1.0.0 真的就是最简版**：
- 列宽固定为字符串自然宽度（窄屏会溢出，列宽算法留给后续版本）
- 不支持 CJK / emoji 宽字符
- 不支持单元格换行

测试倒是到位——563 行，**测试:代码 ≈ 1:1**。这是我另一个工作流：测试先覆盖基础场景。

之后是 3 个关键节点的演进：

| 时间 | 版本 | 关键变化 |
|---|---|---|
| 6/2 | v1.0.0 | 574 行 inline in display 包，静态渲染 |
| 6/11 | v1.0.4 | viewportRowMap 概念首次提出，O(n) → O(1) 优化 |
| 6/16 | v1.0.6 | refactor 到独立 `internal/md/render_table.go`，引入 `IsDecorative` 字段 |

6/16 那天是分水岭——从那之后 `markdown_table.go` 进化成 `internal/md/render_table.go`，从 display 包里的「边角料」变成 md 包的「核心成员」。**6/2 → 6/16 用了 14 天**——14 天从最简版走到生产可用，不算快也不算慢，关键是**每一步都是 shippable**。

我有一个原则：v1.0.0 跑得起来、v1.0.4 跑得更好、v1.0.6 跑得最稳——**不写「未来可能用得上」的代码**。不预设嵌套表格、不预设合并单元格、不预设单元格背景色。每个版本只解决「当前最痛的那个问题」。

> 574 行代码 + 563 行测试：v1.0.0 的全部。
> 做开发的金标准不是「它能跑吗」——是「它坏了我能马上知道吗」。

## §6 Takeaway

3 个 takeaway：

1. **「识别 / 装饰 / 演进」是 3 个互相独立的子问题**——别试图「一步到位」解完。先识别清楚，再插入装饰，最后走版本演进。
2. **「装饰行不存在于 buffer」是 TUI 渲染的通病**——任何边框、连接线、虚线、状态指示器都会遇到这个归属问题。给装饰元素一个显式的 `IsDecorative` 标记 + 一个 `-1` 的占位值，比「硬塞个 buffer 行号」安全得多。
3. **最小 demo → 完美靠版本切割**——每个版本只解「当前最痛的一个问题」，不预设「未来可能用得上」。

---

microNeo 是开源的：[github.com/sollawen/microNeo](https://github.com/sollawen/microNeo)。一行命令安装，能当 `$EDITOR` 给 Claude Code、Yazi 之类的工具用。
