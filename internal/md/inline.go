package md

import (
	"unicode/utf8"

	"github.com/micro-editor/tcell/v2"
)

// renderInline 处理行内的 Markdown 标记（加粗、斜体、链接等）。
// 输入一行字符串，输出 []Cell。
// 隐藏标记符号（`**`、`*`、`~~`、`` ` ``、`[]()` 的标记字符不输出为 Cell）。
// 叠加文本属性（Bold/Italic/Strikethrough/Underline），不管颜色。
// BufX 是 rune 偏移（与 highlighter 的 LineMatch map key 一致）。
func renderInline(line string, baseStyle tcell.Style, bufLineOffset int) []Cell {
	cells := make([]Cell, 0, utf8.RuneCountInString(line))

	// 预计算 rune 数组，方便按 rune index 操作
	runes := []rune(line)

	// runeIdx 是当前位置的 rune 偏移
	runeIdx := 0
	runeCount := len(runes)

	for runeIdx < runeCount {
		r := runes[runeIdx]

		// 1. 行内代码：`...`
		if r == '`' {
			// 在 rune 数组中找下一个反引号
			endIdx := -1
			for k := runeIdx + 1; k < runeCount; k++ {
				if runes[k] == '`' {
					endIdx = k
					break
				}
			}
			if endIdx >= 0 {
				// 找到配对的反引号，内容当行内代码输出
				for x := runeIdx + 1; x < endIdx; x++ {
					cells = append(cells, Cell{
						Rune:    runes[x],
						Style:   baseStyle,
						BufLine: bufLineOffset,
						BufX:    x,
					})
				}
				runeIdx = endIdx + 1 // 跳过闭标记的 `
				continue
			}
			// 没找到配对，当普通字符输出
			cells = append(cells, Cell{
				Rune:    r,
				Style:   baseStyle,
				BufLine: bufLineOffset,
				BufX:    runeIdx,
			})
			runeIdx++
			continue
		}

		// 2. *** → Bold+Italic
		if runeIdx+2 < runeCount && runes[runeIdx] == '*' && runes[runeIdx+1] == '*' && runes[runeIdx+2] == '*' {
			endIdx := findRuneSeq(runes, runeIdx+3, runeCount, "***")
			if endIdx >= 0 {
				for x := runeIdx + 3; x < endIdx; x++ {
					cells = append(cells, Cell{
						Rune:    runes[x],
						Style:   baseStyle.Bold(true).Italic(true),
						BufLine: bufLineOffset,
						BufX:    x,
					})
				}
				runeIdx = endIdx + 3
				continue
			}
			// 没找到 ***，尝试 **
			endIdx = findRuneSeq(runes, runeIdx+2, runeCount, "**")
			if endIdx >= 0 {
				for x := runeIdx + 2; x < endIdx; x++ {
					cells = append(cells, Cell{
						Rune:    runes[x],
						Style:   baseStyle.Bold(true),
						BufLine: bufLineOffset,
						BufX:    x,
					})
				}
				runeIdx = endIdx + 2
				continue
			}
			// 没找到 **，尝试 *
			endIdx = findRune(runes, runeIdx+1, runeCount, '*')
			if endIdx >= 0 {
				for x := runeIdx + 1; x < endIdx; x++ {
					cells = append(cells, Cell{
						Rune:    runes[x],
						Style:   baseStyle.Italic(true),
						BufLine: bufLineOffset,
						BufX:    x,
					})
				}
				runeIdx = endIdx + 1
				continue
			}
			// *** 未闭合，当普通字符输出
			cells = append(cells, Cell{
				Rune:    r,
				Style:   baseStyle,
				BufLine: bufLineOffset,
				BufX:    runeIdx,
			})
			runeIdx++
			continue
		}

		// 3. ** → Bold
		if runeIdx+1 < runeCount && runes[runeIdx] == '*' && runes[runeIdx+1] == '*' {
			endIdx := findRuneSeq(runes, runeIdx+2, runeCount, "**")
			if endIdx >= 0 {
				for x := runeIdx + 2; x < endIdx; x++ {
					cells = append(cells, Cell{
						Rune:    runes[x],
						Style:   baseStyle.Bold(true),
						BufLine: bufLineOffset,
						BufX:    x,
					})
				}
				runeIdx = endIdx + 2
				continue
			}
			// 没找到 **，尝试 *
			endIdx = findRune(runes, runeIdx+1, runeCount, '*')
			if endIdx >= 0 {
				for x := runeIdx + 1; x < endIdx; x++ {
					cells = append(cells, Cell{
						Rune:    runes[x],
						Style:   baseStyle.Italic(true),
						BufLine: bufLineOffset,
						BufX:    x,
					})
				}
				runeIdx = endIdx + 1
				continue
			}
			// ** 未闭合，当普通字符输出
			cells = append(cells, Cell{
				Rune:    r,
				Style:   baseStyle,
				BufLine: bufLineOffset,
				BufX:    runeIdx,
			})
			runeIdx++
			continue
		}

		// 4. * → Italic
		if r == '*' {
			endIdx := findRune(runes, runeIdx+1, runeCount, '*')
			if endIdx >= 0 {
				for x := runeIdx + 1; x < endIdx; x++ {
					cells = append(cells, Cell{
						Rune:    runes[x],
						Style:   baseStyle.Italic(true),
						BufLine: bufLineOffset,
						BufX:    x,
					})
				}
				runeIdx = endIdx + 1
				continue
			}
			// 未闭合，当普通字符输出
			cells = append(cells, Cell{
				Rune:    r,
				Style:   baseStyle,
				BufLine: bufLineOffset,
				BufX:    runeIdx,
			})
			runeIdx++
			continue
		}

		// 5. ~~ → Strikethrough
		if runeIdx+1 < runeCount && runes[runeIdx] == '~' && runes[runeIdx+1] == '~' {
			endIdx := findRuneSeq(runes, runeIdx+2, runeCount, "~~")
			if endIdx >= 0 {
				for x := runeIdx + 2; x < endIdx; x++ {
					cells = append(cells, Cell{
						Rune:    runes[x],
						Style:   baseStyle.StrikeThrough(true),
						BufLine: bufLineOffset,
						BufX:    x,
					})
				}
				runeIdx = endIdx + 2
				continue
			}
			// 未闭合，当普通字符输出
			cells = append(cells, Cell{
				Rune:    r,
				Style:   baseStyle,
				BufLine: bufLineOffset,
				BufX:    runeIdx,
			})
			runeIdx++
			continue
		}

		// 6. [ → 尝试匹配 [text](url) 模式
		if r == '[' {
			// 在 rune 数组中找 ]
			closeBracketIdx := -1
			for k := runeIdx + 1; k < runeCount; k++ {
				if runes[k] == ']' {
					closeBracketIdx = k
					break
				}
			}
			if closeBracketIdx >= 0 && closeBracketIdx+1 < runeCount && runes[closeBracketIdx+1] == '(' {
				// 找 )
				closeParenIdx := -1
				for k := closeBracketIdx + 2; k < runeCount; k++ {
					if runes[k] == ')' {
						closeParenIdx = k
						break
					}
				}
				if closeParenIdx >= 0 {
					// 成功匹配 [text](url)，只输出 text，叠加 Underline
					for x := runeIdx + 1; x < closeBracketIdx; x++ {
						cells = append(cells, Cell{
							Rune:    runes[x],
							Style:   baseStyle.Underline(true),
							BufLine: bufLineOffset,
							BufX:    x,
						})
					}
					runeIdx = closeParenIdx + 1
					continue
				}
			}
			// 不是有效的链接格式，当普通字符输出
			cells = append(cells, Cell{
				Rune:    r,
				Style:   baseStyle,
				BufLine: bufLineOffset,
				BufX:    runeIdx,
			})
			runeIdx++
			continue
		}

		// 7. 其他字符：原样输出
		cells = append(cells, Cell{
			Rune:    r,
			Style:   baseStyle,
			BufLine: bufLineOffset,
			BufX:    runeIdx,
		})
		runeIdx++
	}

	return cells
}

// findRune 在 runes[start:end] 中查找第一个等于 target 的位置，返回其 index。
// 没找到返回 -1。
func findRune(runes []rune, start, end int, target rune) int {
	for i := start; i < end; i++ {
		if runes[i] == target {
			return i
		}
	}
	return -1
}

// findRuneSeq 在 runes[start:end] 中查找第一个等于 seq 的位置，返回 seq 起始 index。
// 没找到返回 -1。
func findRuneSeq(runes []rune, start, end int, seq string) int {
	seqRunes := []rune(seq)
	seqLen := len(seqRunes)
	for i := start; i <= end-seqLen; i++ {
		match := true
		for j := 0; j < seqLen; j++ {
			if runes[i+j] != seqRunes[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
