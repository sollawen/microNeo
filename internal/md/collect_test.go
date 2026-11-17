package md

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

// trailingDigits 取串尾连续十进制数字，无则返回空串。
func trailingDigits(s string) string {
	runes := []rune(s)
	i := len(runes)
	for i > 0 && runes[i-1] >= '0' && runes[i-1] <= '9' {
		i--
	}
	return string(runes[i:])
}

func TestCollectHeadingsBasic(t *testing.T) {
	buf := &mockBuffer{lines: []string{
		"# Title",
		"",
		"## Section",
		"text",
		"### Sub",
	}}
	items := CollectHeadings(buf, 30)
	if len(items) != 3 {
		t.Fatalf("want 3 items, got %d: %v", len(items), items)
	}
	wantLine := []string{"1", "3", "5"}
	wantPrefix := []string{"# Title", "## Section", "### Sub"}
	for i, item := range items {
		if !strings.HasPrefix(item, wantPrefix[i]) {
			t.Errorf("item %d: want prefix %q, got %q", i, wantPrefix[i], item)
		}
		if got := trailingDigits(item); got != wantLine[i] {
			t.Errorf("item %d: trailing line want %q, got %q", i, wantLine[i], got)
		}
	}
}

func TestCollectHeadingsCodeblockSkip(t *testing.T) {
	buf := &mockBuffer{lines: []string{
		"# Real",
		"```",
		"# Fake",
		"```",
	}}
	items := CollectHeadings(buf, 30)
	if len(items) != 1 {
		t.Fatalf("want 1 item (codeblock # excluded), got %d: %v", len(items), items)
	}
	if !strings.HasPrefix(items[0], "# Real") {
		t.Errorf("want # Real item, got %q", items[0])
	}
}

func TestCollectHeadingsNone(t *testing.T) {
	buf := &mockBuffer{lines: []string{
		"plain text",
		"more text",
	}}
	if items := CollectHeadings(buf, 30); items != nil {
		t.Errorf("want nil for no headings, got %v", items)
	}
	empty := &mockBuffer{lines: []string{}}
	if items := CollectHeadings(empty, 30); items != nil {
		t.Errorf("want nil for empty buffer, got %v", items)
	}
}

func TestCollectHeadingsTruncate(t *testing.T) {
	long := "# " + strings.Repeat("a", 50)
	buf := &mockBuffer{lines: []string{long}}
	items := CollectHeadings(buf, 20)
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	item := items[0]
	if runewidth.StringWidth(item) != 20 {
		t.Errorf("display width want 20, got %d: %q", runewidth.StringWidth(item), item)
	}
	if !strings.HasPrefix(item, "# ") {
		t.Errorf("truncated item should keep left side, got %q", item)
	}
	// 至少 1 个分隔符 + 行号仍在串尾
	if !strings.Contains(item, "･") || trailingDigits(item) != "1" {
		t.Errorf("want separator and trailing line number, got %q", item)
	}
}

func TestCollectHeadingsWidthTooSmall(t *testing.T) {
	buf := &mockBuffer{lines: []string{"# Title"}}
	if items := CollectHeadings(buf, 2); items != nil {
		t.Errorf("want nil when width too small, got %v", items)
	}
}

func TestCollectHeadingsEqualWidth(t *testing.T) {
	// 第 1 行与第 10 行各一个标题：行号位数不同（1 位 vs 2 位），引导线长度不同，总宽相等
	buf := &mockBuffer{lines: []string{
		"# First",
		"", "", "", "", "", "", "", "",
		"## Tenth",
	}}
	items := CollectHeadings(buf, 24)
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d: %v", len(items), items)
	}
	if runewidth.StringWidth(items[0]) != 24 || runewidth.StringWidth(items[1]) != 24 {
		t.Errorf("display widths want 24, got %d / %d",
			runewidth.StringWidth(items[0]), runewidth.StringWidth(items[1]))
	}
	if trailingDigits(items[0]) != "1" || trailingDigits(items[1]) != "10" {
		t.Errorf("trailing lines want 1 / 10, got %q / %q",
			trailingDigits(items[0]), trailingDigits(items[1]))
	}
	if strings.Count(items[0], "･") <= strings.Count(items[1], "･") {
		t.Errorf("shorter line number should get longer leader: %q vs %q", items[0], items[1])
	}
}

func TestCollectHeadingsCJK(t *testing.T) {
	// CJK 按 2 列算，截断不把宽字符切成两半
	buf := &mockBuffer{lines: []string{"# 中文中文中文中文"}}
	items := CollectHeadings(buf, 10)
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	item := items[0]
	if runewidth.StringWidth(item) != 10 {
		t.Errorf("display width want 10, got %d: %q", runewidth.StringWidth(item), item)
	}
	// avail = 10-1-1 = 8：# + 空格 + 3 个中文（各 2 列）恰好 8，第 4 个中文放不下
	if item != "# 中文中･1" {
		t.Errorf("want complete wide chars, got %q", item)
	}
}

func TestCollectHeadingsHeadingWithDigits(t *testing.T) {
	// 标题自带数字时，尾部数字仍是行号（分隔符保证不粘连）
	buf := &mockBuffer{lines: []string{
		"intro",
		"",
		"# 第2章",
	}}
	items := CollectHeadings(buf, 30)
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d: %v", len(items), items)
	}
	if got := trailingDigits(items[0]); got != "3" {
		t.Errorf("trailing line want 3, got %q: %v", got, items[0])
	}
}

func TestCollectHeadingsMidBuffer(t *testing.T) {
	buf := &mockBuffer{lines: []string{
		"text",
		"text",
		"# Third",
	}}
	items := CollectHeadings(buf, 30)
	if len(items) != 1 || trailingDigits(items[0]) != "3" {
		t.Fatalf("want heading at line 3, got %v", items)
	}
}

func TestCollectHeadingsTab(t *testing.T) {
	buf := &mockBuffer{lines: []string{"#\tTabbed"}}
	items := CollectHeadings(buf, 30)
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if strings.ContainsRune(items[0], '\t') {
		t.Errorf("tab should be replaced by space: %q", items[0])
	}
	if runewidth.StringWidth(items[0]) != 30 {
		t.Errorf("display width want 30, got %d", runewidth.StringWidth(items[0]))
	}
}
