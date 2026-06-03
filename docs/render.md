我们来讨论一下detector.go 与 各个renders之间的关系

# 我的想法

## 有哪几种render

- block render, 
	- 典型就是 table.go, codeblock.go, blockquote.go
- line render
	- 典型的是 heading.go, list.go, hr.go, normalLine.go
- inline render
	- 粗体/斜体，行内代码块，link，....

## detect 和 render 应该分开
1，我认为detect/render是完全不同的两件事情，不应该混在一起。
	- 不应该是 table.detect, table.render, 两个合成一个 table.go，应该是分开的两个程序

2，markdown的基础语法非常简单。不值得为每个语法写一个detect。
	- 我认为在 detect.go里面，直接读每一行buffer.line, 根据这一行的字符特征，判断是什么语法，
	- 然后把这一行或是连续的多行，送给对应的那个render去处理就可以了
	- 如果未来我们要做更复杂的markdown判断，比如mermaid，也是由detect识别出mermaid标志，然后把对应的buffer lines送给 mermaid render去处理

```
   detect.go 扫描可见区域：
     line 1-14  → 普通行 → paragraphRenderer
     line 15-20 → 代码块 → codeblockRenderer
     line 21    → 标题   → headingRenderer
     ...

   输出 []Segment，每个 Segment = {起止行号, renderer}

   displayBufferMD() 遍历 Segment 列表：
     segment 1: paragraphRenderer.Render(line 1-14 的内容) → 写到 screen
     segment 2: codeblockRenderer.Render(line 15-20 的内容) → 写到 screen
     segment 3: headingRenderer.Render(line 21 的内容) → 写到 screen
```
	
## renders 的接口定义

- 这里的block renders and line renders, 是对外的，可以被外面程序调用的
- inline render是只能被 block render and line render 调用的。不允许其它程序调用

对外的render的接口
- 入参应该是detect判断出来的这一行或连续多行的buffer.line的内容
- 返回数据应该是render把传入的数据计算转换成需要填入screen的数据结构，还给调用者

