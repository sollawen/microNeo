# Test Title With Many Characters
## Second Level Title And It's Quite Long With English Too

Plain text, try emoji 👍🌹 Can this display?
Regular paragraph text with `inline code`.
This line has **bold text**.
This line has *italic text*.
This line has ~~strikethrough text~~.
This line has ***bold italic text*** (if terminal supports).
Nested plain text with **bold containing `inline code` inside** various combinations.
Also plain `code` and **bold** and *italic* mixed together on the same line.

# Lists

- List item 1
	- List item 2 with `inline code`
	- List item 3 with **bold text**
- List item 4

1. Ordered list item 1
2. Ordered list item 2

# Task List

- [ ] **Uncompleted** task
- [x] *Completed* task

# Code Blocks
```
This is a plain code block without syntax highlighting
Second line 
```

  ```python
 def function(something: string):
     self.a = 0
     # This is a comment, but it's quite long. Let's test if this comment is quite long.
     return
  ```

```go
package main

import "fmt"

func main() {
    fmt.Println("Hello, World!")
	test1
		test2
}
```

# Tables

| Column A | Column B | Column C |
|----------|----------|----------|
| Data 1   | Data 2   | Data 3 is `quite long` code |
| Data 4   | Data 5   | Data 6 |
| **Bold** | `Code`   | *Italic* |

| 场景 | 修复前（bug） | 修复后 |
|---|---|---|
| list 下一行紧跟 `go` | list 覆盖 codeblock，codeblock 错位/丢失 | list 正常收尾，codeblock 独立成块、顺序正确 |
| blockquote 紧跟 codeblock | 同上 | blockquote 收尾，codeblock 独立 |
| table 紧跟 codeblock | 同上 | table 收尾，codeblock 独立 |
| 中间有空行分隔 | 本就正常 | 仍正常（回归） |

# Blockquotes

> Blockquote first line
> Blockquote `second line`

# Tab Rendering

Normal leading tab (1 tab, should indent to tab stop):
	normal 1 tab
Normal leading tabs (2 tabs):
		normal 2 tabs
Normal mid tab (a TAB b, b should align to next tab stop):
a	b

> Blockquote leading tab (1 tab):
>	quote 1 tab
>
> Blockquote leading tabs (2 tabs):
>		quote 2 tabs
>
> Blockquote mid tab (a TAB b):
> a	b

# Mixed Long Paragraph

On the 4th local time, Russian President Putin stated during a meeting with heads of major international news agencies attending the 29th St. Petersburg International Economic Forum that China is developing strongly and playing an increasingly important role in the *global economy*, **world politics**, and international affairs. The open letter stated that Ukraine has no intention of prolonging the conflict and *proactively proposed a ceasefire negotiation plan*. Regarding meetings and negotiation procedures, Ukraine proposed that both Ukraine and Russia first engage in direct consultations, after which the United States, Europe, and other relevant parties participate in negotiations to provide support for building a subsequent **security guarantee system**.

===

# Links and URLs

This is a [link text](https://example.com) in a sentence.
Bare URL: https://github.com/example/repo
Image: ![Image Description](https://example.com/image.png)


# Special Symbols
Copyright © Registered ® Trademark ™ Ellipsis… Em dash —

---

## List / Blockquote / Table + Codeblock 紧邻验证（修复 #6）

以下四组用于验证「多行结构紧跟 codeblock 时 segment 乱序」修复：前三组（无空行分隔）触发修复前 bug，第四组（有空行分隔）是回归用例。



- list item one
- list item two
```go
const x = 1
```

# Case 2: blockquote 紧跟 codeblock（无空行）

> quote line 1
> quote line 2
```go
fmt.Println("x")
```

# Case 3: table 紧跟 codeblock（无空行）
- list
| col A | col B |
|-------|-------|
| data1 | data2 |
```go
code inside
```

# Case 4: list → 空行 → codeblock（回归，行为不变）

- list item one
- list item two
```go
const y = 2
```

```
```
