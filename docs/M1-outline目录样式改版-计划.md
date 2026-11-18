# M1：Outline 目录条目改版（◇ 前缀 + 层级缩进）

M0 上线的 OutlineDialog，条目直接显示标题原文（含 `#` 号）。M1 把条目改成树状缩进样式。`#` 号不再显示，换成 `◇` 标记，并按标题层级缩进。弹窗几何、键盘交互、Enter 跳转全部不动，只改条目串的拼法。

## 目标显示效果

改版前（现状）：

```
# 1. 总纲･･･････････････････1
## 1.1 需求定义････････････10
## 1.2 名词定义････････････25
# 2. 模块划分･･････････････37
## 2.1 base层模块･･････････59
## 2.2 engine层模块･･･････120
```

改版后（目标）：

```
◇ 1. 总纲･･･････････････････1
 ◇ 1.1 需求定义････････････10
 ◇ 1.2 名词定义････････････25
◇ 2. 模块划分･･････････････37
 ◇ 2.1 base层模块･･････････59
 ◇ 2.2 engine层模块･･･････120
```

原始需求里 `## 2.2 engine层模块` 在目标效果中没写缩进，与同为二级的 1.1、1.2、2.1 矛盾，判定为笔误，按统一规则处理。

## 显示规则

统一规则只有一条，全部条目照此拼装：

- level N 的条目 = N-1 个空格 + `◇` + 去掉 `#` 号的标题文本（`#` 后的那个空格原样保留）
- level 1（`#`）顶格：`◇ 1. 总纲`
- level 2（`##`）缩进 1 格：` ◇ 1.1 需求定义`
- level 3（`###`）缩进 2 格：`  ◇ 小节`
- 引导线与行号维持 M0 的规则不变
- 等宽性质维持 M0 的规则不变

## 改动范围

改动集中在 md 层拼条目串的一处，dialog 层与 action 层零改动：

| 文件 | 动作 | 内容 |
|---|---|---|
| internal/md/collect_headings.go | 修改 | `CollectHeadings` 生成 `◇` 前缀与缩进，`heading.text` 注释同步改 |
| internal/md/collect_headings_test.go | 修改 | 更新既有断言 + 新增层级缩进用例 |

行数估计：
- collect_headings.go 约 10 行
- collect_headings_test.go 约 20 行

## 任务 1：collect_headings.go 生成前缀

现状：`CollectHeadings` 把标题原文（含 `#`）直接存进 `heading.text`，`buildEntry` 只负责截断 + 引导线 + 行号。

改法：前缀在入 `hits` 前拼好，`buildEntry` 与 `truncateByWidth` 一行不改。

改后 `CollectHeadings` 的伪代码（`★` 标记本次新增的行，其余保持原样）：

```
func CollectHeadings(buf BufferReader, width int) []string {
    hits := []heading{}
    for _, seg := range DetectSegments(buf, 0, buf.LinesNum()-1) {
        if seg.BufStartLine != seg.BufEndLine {
            continue                      // 多行结构（blockquote/table/list/codeblock）不是标题
        }
        line := strings.TrimSpace(string(buf.LineBytes(seg.BufStartLine)))
        if !strings.HasPrefix(line, "#") {
            continue                      // 不是标题
        }
        line = strings.ReplaceAll(line, "\t", " ")   // tab 一律换空格，runewidth 与 tcell 口径要对齐

        // ★ 本次改动：把前导 '#' 串换成「缩进 + ◇」，标题原文其余部分照旧
        level := 0
        for level < len(line) && line[level] == '#' {
            level++                      // 数出前导 '#' 的个数 = 标题层级
        }
        text := strings.Repeat(" ", level-1) + "◇" + line[level:]   // line[level:] 含 '#' 后的那个空格

        hits = append(hits, heading{line: seg.BufStartLine, text: text})
    }
    if len(hits) == 0 {
        return nil
    }
    maxLine := 0
    for _, h := range hits {
        if h.line > maxLine {
            maxLine = h.line
        }
    }
    if width-digits(maxLine+1)-1 < 1 {
        return nil                         // 连「标题 1 字 + 分隔符 + 行号」都放不下
    }
    out := make([]string, 0, len(hits))
    for _, h := range hits {
        out = append(out, buildEntry(h.text, h.line+1, width))   // 截断/引导线/行号全在 buildEntry，不改
    }
    return out
}
```

关键几行的语义：
- `level` 就是标题层级，等于前导 `#` 的个数，`#`→1、`##`→2
- `line[level:]` 是剥掉 `#` 号之后的原文，`#` 与标题之间的空格也在里面，原样保留
- `text` 是最终进 `heading.text` 的条目文本，缩进与 `◇` 前缀都在这里拼好
- `buildEntry` 收到的是已经带前缀的 `text`，它只管截断 + 引导线 + 行号

整件事就是一次前缀替换：n 个 `#` 换成 n-1 个空格加一个 `◇`，其余字符不动。实现只用 `strings.Repeat` 一个标准库函数，不新写辅助函数。数 `#` 用一个 for 循环就地数。

要点：
- 等宽不变式不破：前缀计入条目文本后，`buildEntry` 的截断与填充自动适配
    - 每条串的显示宽度仍恒等于 width
- `◇` 的宽度口径安全
    - U+25C7 在 runewidth 记 1 列，tcell 实画 1 格，两边一致
    - 与 M0 已用的 `▲▼`、`･` 同为 Ambiguous 类字符，M0 手测无错位
- 前导缩进空格渲染安全：outline.go 的 display 逐 rune 推进列数，空格按 1 列画
- action 层零改动：`parseTrailingInt` 只认串尾 `･` + 数字，前缀不影响跳转

## 任务 2：更新单测

collect_headings_test.go 的改动明细：
- TestCollectHeadingsBasic：前缀断言逐条改
    - `# Title` → `◇ Title`
    - `## Section` → ` ◇ Section`
    - `### Sub` → `  ◇ Sub`
- TestCollectHeadingsCodeblockSkip：`# Real` → `◇ Real`
- TestCollectHeadingsTruncate：改长标题的断言前缀
    - buffer 里的源行仍是 `# ` + 50 个 a（必须是 `#` 开头才会被当成标题）
    - 断言前缀改 `◇ `
    - 20 列等宽断言不变
- TestCollectHeadingsCJK：期望串改 `◇ 中文中･1`
    - avail=8：◇ 1 列 + 空格 1 列 + 3 个中文 6 列，第 4 个中文放不下
- TestCollectHeadingsTab：只断言无 tab 与等宽，无需改

新增 TestCollectHeadingsIndent：
- 一/二/三级标题各一条
- 断言条目前缀逐级递进
    - 一级 `◇ `
    - 二级 ` ◇ `
    - 三级 `  ◇ `
- 断言 `#` 号不再出现在条目串里

## 任务 3：编译与手测

`make build` 后逐项手测：
- 多级标题显示正确
    - ◇ 一级顶格
    - 二级缩进 1 格
    - 三级缩进 2 格
    - `#` 号不再出现
- 跳转不受影响
    - Enter 各级条目都跳对行
    - 标题带数字的条目（`# 1. 总纲`）不被标题数字干扰
- 等宽与对齐
    - 长标题截断后引导线与行号仍在串尾
    - CJK 标题按 2 列算，缩进后整列仍对齐
- 深层级：`###` 及更深的条目缩进后标题变短，引导线与行号不被挤掉

## 边界行为

这些不特殊处理，写明免得被当成漏做：
- `#Title`（# 后无空格）：条目 `◇Title`，原文照抄不补空格
- `#` 单独一行（空标题）：条目为 `◇`，截断与填充正常走
- 7 个以上 `#`：detect 层只判 `#` 前缀不限制层级，照常收进来，缩进按实际 `#` 个数算
- 层级很深时前缀吃掉的列数多（10 级 = 11 列），标题可能被截得只剩引导线，属于可接受退化

## 风险与验证点

- ◇ 的宽度口径依赖终端
    - Ambiguous 字符在个别终端会被画成 2 格导致错位
    - `▲▼`、`･` 已是同类字符且 M0 手测通过，风险视为已排除
- 缩进 + 前缀占用 3~11 列
    - 窄 pane（textW=12 下限）下深层级标题可读宽度所剩无几
    - 属显示退化而非功能损坏，不做特殊处理
