package md

import (
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
)

// heading 是 CollectHeadings 的中间产物，不导出。
type heading struct {
	line int    // buffer 内 0 基行号
	text string // 条目文本：缩进 + ◇ 前缀 + 标题原文
}

// CollectHeadings 扫描 buffer，返回可直接显示的 Outline 条目串。
// 每条 = 缩进与 ◇ 前缀 + 标题原文 + 引导线填充 + 右对齐行号，行号在串的最右端。
// width = 调用方算好的条目文本可用宽（= 内容区宽 - 1，指示器那列已扣掉），
// CollectHeadings 不再自己扣指示器列，直接按 width 拼装，
// 所有条目串的显示宽度都等于 width。
// 行号用 1 基（跟 pane 的 gutter 一致），buffer 内部行号 +1 后输出。
func CollectHeadings(buf BufferReader, width int) []string {
	hits := []heading{}
	for _, seg := range DetectSegments(buf, 0, buf.LinesNum()-1) {
		if seg.BufStartLine != seg.BufEndLine {
			continue // 多行结构（blockquote/table/list/codeblock）不是标题
		}
		line := strings.TrimSpace(string(buf.LineBytes(seg.BufStartLine)))
		if !strings.HasPrefix(line, "#") {
			continue
		}
		// tab 一律换成空格：runewidth 记 0 宽而 tcell 实画 1 格，口径不一
		line = strings.ReplaceAll(line, "\t", " ")
		// 前导 '#' 串换成「缩进 + ◇」：n 个 '#' → n-1 个空格 + ◇，标题原文其余部分不动
		level := 0
		for level < len(line) && line[level] == '#' {
			level++
		}
		text := strings.Repeat(" ", level-1) + "◇" + line[level:]
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
		return nil // 连「标题 1 字 + 分隔符 + 行号」都放不下
	}
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, buildEntry(h.text, h.line+1, width))
	}
	return out
}

// buildEntry 拼一条条目串：标题按显示宽度从右截断（至少留 1 个字符），
// 用引导线补齐，行号不带前导空格放在串尾。
// 结果的显示宽度恒等于 width，且标题与行号之间至少有 1 个分隔符。
func buildEntry(text string, line, width int) string {
	num := strconv.Itoa(line)
	avail := width - runewidth.StringWidth(num) - 1
	t := truncateByWidth(text, avail)
	fill := width - runewidth.StringWidth(num) - runewidth.StringWidth(t)
	return t + strings.Repeat("･", fill) + num
}

// truncateByWidth 按显示宽度截断，保留完整 rune，不把宽字符切成两半。
func truncateByWidth(s string, avail int) string {
	if avail <= 0 {
		return ""
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if w+rw > avail {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String()
}

// digits 返回 n 的十进制位数（n >= 0）。
func digits(n int) int {
	c := 1
	for n >= 10 {
		n /= 10
		c++
	}
	return c
}
