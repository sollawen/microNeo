# microNeo 开发日记 #1：把一个 line editor 改造成 render editor

> 状态：中文初稿 v1
> 系列定位：第一篇 / series 起源篇
> 目标渠道：dev.to（英文版）+ 掘金（中文版）
> 目标字数：英文 1500-2500，中文对等

---

## 候选标题（先发后挑，别在标题上耗）

1. How I turned a 25k-star line editor into a render editor
2. What breaks when a line editor stops being linear
3. Micro is a line editor. I needed it to not be.

中文候选：
1. 我把一个 25k star 的 line editor，改造成了 render editor
2. 当一个 line editor 不再是线性的，会塌掉什么
3. Micro 是一个 line editor。而我需要它不是。

---

## 正文

你用过的每一个终端编辑器——vim、nano、Micro、pico——本质上都是同一种东西：line editor。buffer 里有一行，屏幕上就有一行。一个 520 行的渲染函数把每个字符原样搬上屏幕，唯一的变化是颜色。Micro 靠这个简单的假设拿了 25k star。

这是对的。所有程序代码、配置文件，都是这样处理的。几代程序员使用的editor都这样处理文件的。

但是vibe coding来了之后，我发现我越来越少的敲代码了，越来越多的看AI写的markdown的方案了。于是，terminal里面的所有的editor，我都感觉不顺手了。我想要在同一窗口里渲染并编辑 Markdown。那意味着，要打破整个代码库建立其上的这个 1:1 假设。

忍了几个月之后，我忍不了了。于是自己动手，在micro的基础上改造一个新editor出来：把一个 line editor 改造成为一个 rich format editor
 
---

### 1. 什么是 line editor（以及为什么这个假设很值钱）

Micro 原来是怎么工作的。

它的核心渲染函数叫 `displayBuffer()`，在 `bufwindow.go` 里，520 多行。剥掉所有花哨逻辑——括号匹配、空白符显示、选区反显、softwrap——它的骨架只有两层循环：

```
for 每个可见的 buffer 行 {
    画行号
    for 该行的每个字符 {
        取语法高亮颜色
        screen.SetContent(x, y, rune, style)
    }
}
```

**buffer 的第 N 行，就是屏幕的第 N 行。**（softwrap 会让一行变多行，但不破坏"屏幕行总能追溯到唯一一个 buffer 行"这个性质。）

这个假设值钱在哪？

值钱在它让**滚动**这件事变得几乎是免费的。Micro 的滚动系统核心是一个结构叫 `SLoc`，记录"窗口顶部对应 buffer 的哪一行"。滚动一行就是 `SLoc.Line += 1`。光标定位、PageUp/PageDown、鼠标点击——全部建立在这个"buffer 行和屏幕行一一对应"的前提上。

这套设计简洁、快、好维护。Micro 在这上面跑了十年。

---

### 2. 崩溃现场：1:1 是怎么死的

我打开一个 3 行的 Markdown 表格：

```
| name | age |
|------|-----|
| ada  | 36  |
```

我希望我看到的不是这几个傻傻的字符，而是一个真正的表格：有边框线、列对齐、表头加粗。

问题来了。一个 3 行的 buffer，渲染出来要 5 行屏幕（上下边框 + 数据行 + 分隔线）。**buffer 里第 3 行之后，屏幕上多出来两行，这两行在 buffer 里根本不存在。** 我得凭空画出来。

这就是 1:1 死掉的时刻。而且不是只有表格会杀它：

- `**bold**` 渲染成 **bold** —— buffer 里 7 个字符，屏幕上 4 个
- `# 标题` 渲染成大字号标题 —— 那个 `#` 在屏幕上消失了
- 代码块 ```` ``` ```` 围栏 —— 渲染后变成框线，围栏本身不见了

每一项单独看都不难。难的是：**Micro 的滚动、光标、点击、选区——整个交互层——全都假设"屏幕上的第 Y 行，能反查到 buffer 的第 X 行"。** 现在这个反查断了。

更恶心的是表格这种**跨行结构**。你不可能只看表格的一行就知道列该多宽——必须扫完整个表格。这意味着渲染不能再是"逐行独立"，得有一层东西把跨行结构当整体处理。

---

### 3. 核心决定：在 buffer 和 screen 之间，塞一层 Segment

我做的第一个、也是最重要的一个架构判断：**不要去改 buffer，也不要去改 tcell screen。在中间加一层。**

这层我叫它 **Segment（渲染片）**。代码长这样（`internal/md/md.go`，简化版）：

```go
type Segment struct {
    BufStartLine int       // 这片覆盖 buffer 的哪几行
    BufEndLine   int
    Render func(seg Segment, width int, cfg MDConfig) *RenderedSegment
}

type Cell struct {
    Rune         rune
    Style        tcell.Style
    BufLine      int   // 这个屏幕字符对应 buffer 的哪一行；装饰行 = -1
    BufX         int   // 对应 buffer 行内的 rune 偏移
    IsDecorative bool  // true = 装饰字符（边框线），点击忽略
}
```

关键设计有两个，每一个都解决一类问题：

**设计一：每一行 buffer 都属于某个 Segment。**

不是只有表格/代码块才有 Segment。**标题行是 1 行的 Segment，普通段落行也是 1 行的 Segment。** 这样渲染管线完全没有分支：

```
for 每个可见行 {
    找到这行所属的 Segment
    让 Segment 负责输出
}
```

表格 Segment 和段落 Segment 的区别，只是覆盖的 buffer 行数不同、挂的渲染函数不同。管线统一，没有 `if isTable`。

**设计二：把"分类"和"渲染"拆成两个完全独立的阶段。**

这是我踩坑之后才想明白的。一开始我想让渲染函数边渲染边算行高。然后撞上一堵墙：

Micro 的 `Scroll()` 和 `Relocate()` **在 `displayBuffer()` 之前被调用**。它们要算"滚动到这里对不对"，就得知道每一片占几个屏幕行。但行高只有渲染走到那一片才算得出来。

时序死锁。

解法：把"这片是表格、覆盖哪几行、挂哪个渲染函数"——这些**只跟 buffer 内容有关、跟屏宽无关**的信息——提到一个独立的阶段 `DetectSegments()`。它在 buffer 一变化时就跑，结果存起来。渲染阶段只读这个结果。

```
buffer 变化 ──► DetectSegments() ──► 存起来（跟屏宽无关）
                                         │
按键/滚动 ──► Scroll/Relocate（查行高）──► 渲染（按屏宽布局）──► 屏幕
```

detect 不知道屏宽。render 只读 detect 的结果。两个阶段解耦，时序死锁消失。

---

### 4. 有了渲染片，还需要一群渲染器

Segment 只是数据结构——它声明"这段 buffer 是一个整体"，但没说"怎么把这段翻译成屏幕字符"。翻译这件事，交给一群叫 **render** 的普通函数。

每个 render 长这样（真实签名，`internal/md/render_table.go`）：

```go
func RenderTable(seg Segment, width int, cfg MDConfig) *RenderedSegment
```

输入：一个 Segment（知道自己覆盖哪几行 buffer）、当前屏宽、配置。输出：一个 `RenderedSegment`——一串 `RenderedRow`，每行一串 `Cell`。

我设计了七个 render：

| render | 覆盖什么 | 例子 |
|---|---|---|
| `RenderHeading` | 单行 `# 标题` | `# Hello` → 大字号 "Hello" |
| `RenderHR` | 单行 `---` | `---` → 一条横线 |
| `RenderBlockquote` | 连续 `>` 行 | `> quote` → 带竖线前缀的引用块 |
| `RenderList` | 连续 `- ` / `1.` 行 | 列表项 |
| `RenderCodeBlock` | 围栏内的多行 | 带框线的代码块 |
| `RenderTable` | 连续 `\|` 行 | 带边框、列对齐的表格 |
| `RenderParagraph` | 兜底，单行 | 普通段落，处理行内加粗/斜体 |

注意一件事：**这七个 render 互相不认识。** `RenderTable` 不知道 `RenderCodeBlock` 存在。它们也不知道光标在哪、屏幕在滚、用户在不在编辑模式。它们只做一件事：给我 buffer 的某几行和屏宽，我还你一堆 Cell。

它让每个 render 可以单独写、单独测、单独改——加一个 Mermaid 支持，就是新建 `render_mermaid.go`，在 detector 里加一个识别分支，碰都不碰别的 render。

拿最复杂的 `RenderTable` 举例。给它这三行 buffer：

```
| name | age |
|------|-----|
| ada  | 36  |
```

它扫完三行算列宽，输出一个 5 行的 `RenderedSegment`：

```
Row 0: ┌──────┬─────┐    ← BufLine = -1（装饰行，凭空画的）
Row 1: │ name │ age │    ← BufLine = 0（对应 buffer 第 0 行）
Row 2: ├──────┼─────┤    ← BufLine = -1
Row 3: │ ada  │ 36  │    ← BufLine = 2（buffer 第 1 行是分隔线，被吃掉了）
Row 4: └──────┴─────┘    ← BufLine = -1
```

三件事，每一件都是 line editor 做不到的：

1. **5 行屏幕，来自 3 行 buffer。** 多出来的边框，buffer 里没有。
2. **buffer 的第 1 行（`|---|---|`）在屏幕上完全消失。** 它只告诉渲染器"这是表头分隔"，渲染完就被吃掉。
3. **每一个 Cell 都知道自己对应 buffer 的哪里。** 用户点屏幕第 3 行 `ada` 的 `a`，系统查到 `BufLine=2, BufX=2`，光标准确落到 buffer 里 `| ada |` 的 `a` 上。边框点上去，`IsDecorative=true`，点击被忽略或映射到最近的真行。

---

### 5. 管线接通：从 buffer 到屏幕

有了 Segment（§3）和 render（§4），最后一件事是把它们串起来。整条管线长这样：

```
buffer 变化
    ↓
DetectSegments()              ← 集中式分类器，扫全 buffer
    ↓
[]Segment                     ← 每行 buffer 归属一个 Segment，挂着 Render 指针
    ↓
displayBufferMD()             ← 渲染入口，每帧调用
    ↓
遍历可见 Segment → 调用 seg.Render(seg, width, cfg)
    ↓
[]RenderedSegment（全是 Cell）
    ↓
Cells 写到 tcell 屏幕
```

这里有一个关键设计，我在 §3 提过：**detect 和 render 是两个完全独立的阶段。**

- `DetectSegments()` 在 buffer 一变化时就跑。它逐行扫，根据每一行的字符特征——`#` 开头是标题、`|` 开头是表格、连续 `>` 是引用块……——把整篇 buffer 切成一串 Segment。整个分类器不到 80 行。
- detect **完全不知道屏宽、不知道屏幕在滚、不知道有没有光标。** 它只跟 buffer 内容有关。这保证同一个 buffer，不管你在什么尺寸的终端打开，分类结果都一样。
- render 才关心屏宽。`RenderTable(seg, width, ...)` 里那个 `width`，就是当前终端的列数——表格的列宽按它来算。

为什么要拆这么干净？因为 detect 的结果要给两个不同的地方用：渲染要它，**滚动也要它**。Micro 的滚动系统需要知道"这一片占几个屏幕行"才能算 PageUp 滚到哪——而这件事跟屏宽无关、跟 buffer 内容有关。所以 detect 必须独立于 render 存在，让两边都能查。（这件事后来捅出一个大篓子。下一篇讲。）

---

到这里，最基础的架构完整了：

- buffer 不动，tcell 不动，中间加一层 Segment
- 每一行 buffer 归一个 Segment，挂一个 render
- detect 负责分类，render 负责翻译，display 负责往屏幕上写
- 每个 Cell 都带着自己对应 buffer 的坐标，点击、选区、光标都能反查

**渲染完美实现了。** 我打开一个 `.md` 文件，看到的是排版好的标题、表格、代码块，而不是原始简陋的格式字符。

我给这个新的micro，起了个名字“microNeo”。

---

microNeo 是开源的：[github.com/sollawen/microNeo](https://github.com/sollawen/microNeo)。一行命令安装，能当 `$EDITOR` 给 Claude Code、Yazi 之类的工具用。

---

## 写作笔记（定稿前自查用，不发）

### 结构说明（v2，2026-06-19 调整）
**原结构**：§3 Segment → §4 涟漪效应（一次预告 4 篇）→ §5 ASCII 示例 → §6 Takeaway → §7 series 介绍。
**问题**：第一篇还没发就锁死 4 个未来承诺，过度承诺；原 Takeaway 偏说教。
**现结构**：§3 Segment → §4 renders（含表格示例）→ §5 完整管线 → 「渲染完美实现了」→ 轻钩子「欲知后事如何」。
**好处**：第一篇做成一个完整小弧（有胜利时刻），轻承诺不锁死下一篇写什么，给根据 #1 反馈决定 #2 的灵活性。series 钩子从「4 张牌」退为「1 个软钩」。

### 字数
中文初稿约 2300 字（比 v1 略短，砍了 Takeaway 和 4 重预告）。翻成英文预计 1600-1900 词，落在目标区间。

### §4 截图/图待补
表格 ASCII 示意图已嵌入 §4。定稿前两个选择：
- 中文版：保留 ASCII 图即可，轻量、复制粘贴不丢格式
- 英文 dev.to 版：建议换真实 microNeo 截图，视觉冲击更强

### 自检三问（来自 dev-diary-plan §3）
1. **删掉所有 microNeo 名字，这篇文章还有价值吗？** → 有。核心是 line editor 概念 + detect/render 分离架构模式 + 中间层抽象，任何工程师都适用。✅
2. **中级开发者读完能学到什么具体东西？** → (a) line editor 这个概念；(b) 中间层抽象怎么设计（数据结构 + 函数指针）；(c) detect/render 分离。✅
3. **有没有一句话会被人 quote？** → 候选：「buffer 里没有的行，被凭空画出来。」/「这七个 render 互相不认识。」/「渲染完美实现了。」 ✅

### 结尾设计（v3，2026-06-19 修正）
**原方案**：§5 结尾留 cliffhanger（"然后光标消失了"）+ "欲知后事如何，且听下回分解"。
**删除原因**：读者从 dev.to/掘金单篇发现路径进来，**不知道这是 series**。对他而言这是一篇独立开发心得。cliffhanger 在无 series 语境下不是"留人钩子"，是"文章没写完"的负面体验。series 留人应该靠**每篇独立打赢** + 读者日后认出同作者产生的复利，而不是靠第 1 篇埋钩子。
**现结尾**：在"渲染完美实现了"的胜利 closure 处结束，干净自信。
**这也修正了 dev-diary-plan §2**：§2 的"公开承诺（This is post #1 of a series. Next: …）"混淆了两个目标——(a) 对抗作者拖延，(b) 留住读者。(a) 用 tracking 表/deadline 解决，不该污染 (b) 的读者体验。

### 结尾设计（v3，2026-06-19 修正）
**原方案**：§5 结尾留 cliffhanger（"然后光标消失了"）+ "欲知后事如何，且听下回分解" + 软链接段附"这是 microNeo 开发日记的第一篇"。
**删除原因**：读者从 dev.to/掘金单篇发现路径进来，**不知道这是 series**。对他而言这是一篇独立开发心得。任何 series 暗示——cliffhanger、"第一篇"序数词——在无 series 语境下都是噪声。series 认知靠读者日后读过几篇自然形成，不靠作者明示。
**现结尾**：在"渲染完美实现了"的胜利 closure 处结束，软链接段纯产品介绍，无 series 暗示。
**这也修正了 dev-diary-plan §2**：§2 的"公开承诺（This is post #1 of a series. Next: …）"混淆了两个目标——(a) 对抗作者拖延，(b) 留住读者。(a) 用 tracking 表/deadline 解决，不该污染 (b) 的读者体验。

### 风险与待定
- 中文版的 "line editor" 这个词要不要翻成"行编辑器"？判断：**不翻**。中文技术圈 "line editor" 是通用词，翻成"行编辑器"反而显得生分。等用户拍板。
