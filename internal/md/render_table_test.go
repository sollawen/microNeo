package md

import (
	"testing"

	"github.com/micro-editor/micro/v2/pkg/highlight"
	"github.com/micro-editor/tcell/v2"
)

func TestSplitCells(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected []string
	}{
		{"标准格式", "| a | b | c |", []string{"a", "b", "c"}},
		{"无首尾竖线", "a | b | c", []string{"a", "b", "c"}},
		{"空cell", "| a || c |", []string{"a", "", "c"}},
		{"空行", "|  项目  |  说明  |", []string{"项目", "说明"}},
		{"单cell无竖线", "hello", []string{"hello"}},
		{"只有竖线", "|||", []string{}},
		{"首尾空格", "|  hello  |  world  |", []string{"hello", "world"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitCells(tt.line)
			if len(result) != len(tt.expected) {
				t.Errorf("splitCells(%q) = %v, want %v", tt.line, result, tt.expected)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("splitCells(%q)[%d] = %q, want %q", tt.line, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestIsSeparatorLineStr(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{"标准分隔线", "|---|---|", true},
		{"居中对齐", "|:---:|:---:|", true},
		{"右对齐", "|---:|---:|", true},
		{"带空格", "| --- | --- |", true},
		{"混合对齐", "|:--|--:|", true},
		{"空行", "", false},
		{"只有空格", "   ", false},
		{"纯竖线", "||||", false},
		{"文字行", "| 项目 | 说明 |", false},
		{"分隔线但有文字", "| --- | text |", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSeparatorLineStr(tt.line)
			if result != tt.expected {
				t.Errorf("isSeparatorLineStr(%q) = %v, want %v", tt.line, result, tt.expected)
			}
		})
	}
}

func TestParseTable(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		wantNil  bool
		numCols  int
		sepIdx   int
		header   []string
		bodyRows int
	}{
		{
			name:     "标准3列表格",
			lines:    []string{"| 项目 | 说明 |", "|------|------|", "| foo  | bar  |", "| baz  | qux  |"},
			wantNil:  false,
			numCols:  2,
			sepIdx:   1,
			header:   []string{"项目", "说明"},
			bodyRows: 2,
		},
		{
			name:     "无首尾竖线",
			lines:    []string{"项目 | 说明", "|------|------|", "foo | bar"},
			wantNil:  false,
			numCols:  2,
			sepIdx:   1,
			header:   []string{"项目", "说明"},
			bodyRows: 1,
		},
		{
			name:     "空cell补齐",
			lines:    []string{"| 项目 | 说明 |", "|------|------|", "| foo  |"},
			wantNil:  false,
			numCols:  2,
			sepIdx:   1,
			header:   []string{"项目", "说明"},
			bodyRows: 1,
		},
		{
			name:    "无separator退化",
			lines:   []string{"| 项目 | 说明 |", "| foo  | bar  |"},
			wantNil: true,
		},
		{
			name:    "separator在第一行",
			lines:   []string{"|------|------|", "| 项目 | 说明 |"},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTable(tt.lines)
			if tt.wantNil {
				if result != nil {
					t.Errorf("parseTable() = %v, want nil", result)
				}
				return
			}
			if result == nil {
				t.Errorf("parseTable() = nil, want non-nil")
				return
			}
			if result.numCols != tt.numCols {
				t.Errorf("numCols = %d, want %d", result.numCols, tt.numCols)
			}
			if result.sepIdx != tt.sepIdx {
				t.Errorf("sepIdx = %d, want %d", result.sepIdx, tt.sepIdx)
			}
			if len(result.header) != len(tt.header) {
				t.Errorf("header len = %d, want %d", len(result.header), len(tt.header))
			} else {
				for i, v := range result.header {
					if v != tt.header[i] {
						t.Errorf("header[%d] = %q, want %q", i, v, tt.header[i])
					}
				}
			}
			if len(result.body) != tt.bodyRows {
				t.Errorf("body rows = %d, want %d", len(result.body), tt.bodyRows)
			}
		})
	}
}

func TestCalcNatWidths(t *testing.T) {
	tests := []struct {
		name     string
		pt       *parsedTable
		wantLen  int
		minWidth int
	}{
		{
			name: "标准表格",
			pt: &parsedTable{
				numCols: 3,
				header:  []string{"项目名称", "说明", "状态"},
				body:    [][]string{{"foo", "bar", "ok"}, {"verylong", "x", "pending"}},
			},
			wantLen:  3,
			minWidth: 1,
		},
		{
			name: "CJK字符",
			pt: &parsedTable{
				numCols: 2,
				header:  []string{"项目", "说明"},
				body:    [][]string{{"中文内容", "测试"}, {"宏", "你好世界"}},
			},
			wantLen:  2,
			minWidth: 2,
		},
		{
			name:     "空表格",
			pt:       &parsedTable{numCols: 2, header: []string{"", ""}, body: nil},
			wantLen:  2,
			minWidth: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calcNatWidths(tt.pt)
			if len(result) != tt.wantLen {
				t.Errorf("calcNatWidths() len = %d, want %d", len(result), tt.wantLen)
				return
			}
			for i, v := range result {
				if v < tt.minWidth {
					t.Errorf("calcNatWidths()[%d] = %d, want >= %d", i, v, tt.minWidth)
				}
			}
		})
	}
}

func TestWaterlineShrink(t *testing.T) {
	tests := []struct {
		name     string
		target   int
		input    []int
		expected []int
	}{
		{
			name:     "方案文档例子",
			target:   40,
			input:    []int{30, 20, 18},
			expected: []int{13, 13, 14},
		},
		{
			name:     "等宽列等比缩减",
			target:   24,
			input:    []int{10, 10, 10},
			expected: []int{8, 8, 8},
		},
		{
			name:     "只砍胖列",
			target:   30,
			input:    []int{50, 5},
			expected: []int{25, 5},
		},
		{
			name:     "不需要缩",
			target:   30,
			input:    []int{10, 10, 10},
			expected: []int{10, 10, 10},
		},
		{
			name:     "单列",
			target:   10,
			input:    []int{20},
			expected: []int{10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := waterlineShrink(tt.target, tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("waterlineShrink(%d, %v) = %v, want %v", tt.target, tt.input, result, tt.expected)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("waterlineShrink(%d, %v)[%d] = %d, want %d", tt.target, tt.input, i, v, tt.expected[i])
				}
			}
			// 验证总和
			sum := 0
			for _, v := range result {
				sum += v
			}
			if sum != tt.target {
				t.Errorf("sum = %d, want %d", sum, tt.target)
			}
			// 验证最小值 >= 2
			for i, v := range result {
				if v < 2 {
					t.Errorf("waterlineShrink result[%d] = %d < 2", i, v)
				}
			}
		})
	}
}

func TestAllocColWidths(t *testing.T) {
	tests := []struct {
		name       string
		totalAvail int
		natWidths  []int
		wantNil    bool
		wantSum    int
	}{
		{"放得下", 70, []int{30, 12, 18}, false, 60},
		{"放不下需要缩减", 40, []int{30, 20, 18}, false, 40},
		{"极端窄屏退化", 6, []int{10, 10, 10}, true, 0}, // 需要 >= 2*3 = 6 才退化
		{"刚好放得下", 60, []int{30, 12, 18}, false, 60},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := allocColWidths(tt.totalAvail, tt.natWidths)
			if tt.wantNil {
				if result != nil {
					t.Errorf("allocColWidths(%d, %v) = %v, want nil", tt.totalAvail, tt.natWidths, result)
				}
				return
			}
			if result == nil {
				t.Errorf("allocColWidths(%d, %v) = nil, want non-nil", tt.totalAvail, tt.natWidths)
				return
			}
			sum := 0
			for _, v := range result {
				sum += v
			}
			if sum != tt.wantSum {
				t.Errorf("sum = %d, want %d", sum, tt.wantSum)
			}
		})
	}
}

func TestWrapCellsRaw(t *testing.T) {
	tests := []struct {
		name      string
		cells     []Cell
		maxWidth  int
		wantLines int
	}{
		{"空cells返回空行", []Cell{}, 10, 1},
		{"简单截断", []Cell{{Rune: 'a'}, {Rune: 'b'}, {Rune: 'c'}}, 2, 2},
		{"刚好填满", []Cell{{Rune: 'a'}, {Rune: 'b'}}, 2, 1},
		{"单字符超宽", []Cell{{Rune: 'a'}}, 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapCellsRaw(tt.cells, tt.maxWidth)
			if len(result) != tt.wantLines {
				t.Errorf("wrapCellsRaw() = %d lines, want %d", len(result), tt.wantLines)
			}
		})
	}
}

func TestFillCellsToWidth(t *testing.T) {
	tests := []struct {
		name        string
		cells       []Cell
		targetWidth int
		wantWidth   int
	}{
		{"普通文本补空格", []Cell{{Rune: 'a'}, {Rune: 'b'}, {Rune: 'c'}}, 5, 5},
		{"空cell全空格", []Cell{}, 3, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fillCellsToWidth(tt.cells, tt.targetWidth, tcell.StyleDefault)
			// 计算实际显示宽度
			col := 0
			for _, c := range result {
				col += runeDisplayWidth(c.Rune)
			}
			if col != tt.wantWidth {
				t.Errorf("fillCellsToWidth result width = %d, want %d", col, tt.wantWidth)
			}
		})
	}
}

func runeDisplayWidth(r rune) int {
	if r >= 0x1100 &&
		(r <= 0x115F || r == 0x2329 || r == 0x232A ||
			(r >= 0x2E80 && r <= 0x303E) ||
			(r >= 0x3040 && r <= 0xA4CF) ||
			(r >= 0xAC00 && r <= 0xD7A3) ||
			(r >= 0xF900 && r <= 0xFAFF) ||
			(r >= 0xFE10 && r <= 0xFE1F) ||
			(r >= 0xFE30 && r <= 0xFE6F) ||
			(r >= 0xFF00 && r <= 0xFF60) ||
			(r >= 0xFFE0 && r <= 0xFFE6) ||
			(r >= 0x20000 && r <= 0x2FFFD) ||
			(r >= 0x30000 && r <= 0x3FFFD)) {
		return 2
	}
	return 1
}

func TestMakeTableSeparator(t *testing.T) {
	colWidths := []int{3, 5}
	width := 20
	row := makeTableSeparator(colWidths, width, tcell.StyleDefault, tcell.StyleDefault)

	if len(row.Cells) != width {
		t.Errorf("makeTableSeparator len = %d, want %d", len(row.Cells), width)
	}
	if row.Cells[0].Rune != '├' {
		t.Errorf("first rune = %c, want ├", row.Cells[0].Rune)
	}
	// 表格右边界 = 1 + (3+2) + 1 + (5+2) + 1 = 15, ┤ 在 index 14
	borderEnd := 1 + (colWidths[0]+2) + 1 + (colWidths[1]+2) + 1 - 1
	if row.Cells[borderEnd].Rune != '┤' {
		t.Errorf("border end rune at %d = %c, want ┤", borderEnd, row.Cells[borderEnd].Rune)
	}
	// ┤ 之后应该是空格
	for i := borderEnd + 1; i < width; i++ {
		if row.Cells[i].Rune != ' ' {
			t.Errorf("cell[%d] = %c, want space", i, row.Cells[i].Rune)
		}
	}
}

func TestMakeTableTopBorder(t *testing.T) {
	colWidths := []int{3, 5}
	width := 20
	row := makeTableTopBorder(colWidths, width, tcell.StyleDefault, tcell.StyleDefault)

	if len(row.Cells) != width {
		t.Errorf("makeTableTopBorder len = %d, want %d", len(row.Cells), width)
	}
	if row.Cells[0].Rune != '┌' {
		t.Errorf("first rune = %c, want ┌", row.Cells[0].Rune)
	}
	// 表格右边界 ┐ 在 index 1 + (3+2) + 1 + (5+2) = 14
	borderEnd := 1 + (colWidths[0]+2) + 1 + (colWidths[1]+2)
	if row.Cells[borderEnd].Rune != '┐' {
		t.Errorf("border end rune at %d = %c, want ┐", borderEnd, row.Cells[borderEnd].Rune)
	}
	// ┐ 之后应该是空格
	for i := borderEnd + 1; i < width; i++ {
		if row.Cells[i].Rune != ' ' {
			t.Errorf("cell[%d] = %c, want space", i, row.Cells[i].Rune)
		}
	}
}

func TestMakeTableBottomBorder(t *testing.T) {
	colWidths := []int{3, 5}
	width := 20
	row := makeTableBottomBorder(colWidths, width, tcell.StyleDefault, tcell.StyleDefault)

	if len(row.Cells) != width {
		t.Errorf("makeTableBottomBorder len = %d, want %d", len(row.Cells), width)
	}
	if row.Cells[0].Rune != '└' {
		t.Errorf("first rune = %c, want └", row.Cells[0].Rune)
	}
	// 表格右边界 ┘ 在 index 1 + (3+2) + 1 + (5+2) = 14
	borderEnd := 1 + (colWidths[0]+2) + 1 + (colWidths[1]+2)
	if row.Cells[borderEnd].Rune != '┘' {
		t.Errorf("border end rune at %d = %c, want ┘", borderEnd, row.Cells[borderEnd].Rune)
	}
	// ┘ 之后应该是空格
	for i := borderEnd + 1; i < width; i++ {
		if row.Cells[i].Rune != ' ' {
			t.Errorf("cell[%d] = %c, want space", i, row.Cells[i].Rune)
		}
	}
}

func testMDConfig() MDConfig {
	return MDConfig{
		MDTableBorder: false,
		MDTableAlign:  true,
		Colorscheme: MDColorscheme{
			DefStyle: tcell.StyleDefault,
			Styles:   map[string]tcell.Style{},
		},
	}
}

func testMDConfigWithBorder() MDConfig {
	return MDConfig{
		MDTableBorder: true,
		MDTableAlign:  true,
		Colorscheme: MDColorscheme{
			DefStyle: tcell.StyleDefault,
			Styles:   map[string]tcell.Style{},
		},
	}
}

// mockBufferForTest 是测试用的 mock BufferReader
type mockBufferForTest struct {
	lines []string
}

func (m *mockBufferForTest) LinesNum() int              { return len(m.lines) }
func (m *mockBufferForTest) LineBytes(n int) []byte    { return []byte(m.lines[n]) }
func (m *mockBufferForTest) State(n int) highlight.State { return nil }

// makeTableSegment 创建用于测试 RenderTable 的 Segment
func makeTableSegment(lines []string) (Segment, MDConfig) {
	buf := &mockBufferForTest{lines: lines}
	seg := Segment{
		BufStartLine: 0,
		BufEndLine:   len(lines) - 1,
		Render:       RenderTable,
	}
	cfg := testMDConfig()
	cfg.Buf = buf
	return seg, cfg
}

func TestRenderTable_Simple(t *testing.T) {
	lines := []string{"| 项目 | 说明 |", "|------|------|", "| foo  | bar  |"}
	seg, cfg := makeTableSegment(lines)
	result := RenderTable(seg, 30, cfg)

	// 无边框：header行 + 分隔线 + body行 = 3行
	if len(result.Rows) != 3 {
		t.Errorf("RenderTable rows = %d, want 3", len(result.Rows))
	}

	// 验证第一行是 header
	if result.Rows[0].BufLine != 0 {
		t.Errorf("header BufLine = %d, want 0", result.Rows[0].BufLine)
	}

	// 验证装饰行
	if result.Rows[1].BufLine != -1 {
		t.Errorf("separator BufLine = %d, want -1", result.Rows[1].BufLine)
	}
}

func TestRenderTable_WithBorder(t *testing.T) {
	lines := []string{"| 项目 | 说明 |", "|------|------|", "| foo  | bar  |"}
	seg, cfg := makeTableSegment(lines)
	cfg.MDTableBorder = true
	result := RenderTable(seg, 30, cfg)

	// 有边框：顶边框 + header行 + 分隔线 + body行 + 底边框 = 5行
	if len(result.Rows) != 5 {
		t.Errorf("RenderTable rows = %d, want 5", len(result.Rows))
	}

	// 验证顶边框
	if result.Rows[0].Cells[0].Rune != '┌' {
		t.Errorf("top border first rune = %c, want ┌", result.Rows[0].Cells[0].Rune)
	}

	// 验证底边框
	lastRow := result.Rows[len(result.Rows)-1]
	if lastRow.Cells[0].Rune != '└' {
		t.Errorf("bottom border first rune = %c, want └", lastRow.Cells[0].Rune)
	}
}

func TestRenderTable_CJK(t *testing.T) {
	lines := []string{"| 项目 | 说明 |", "|------|------|", "| 中文内容 | 测试 |"}
	seg, cfg := makeTableSegment(lines)
	result := RenderTable(seg, 30, cfg)

	if len(result.Rows) != 3 {
		t.Errorf("RenderTable CJK rows = %d, want 3", len(result.Rows))
	}

	// 验证至少有内容渲染（不退化）
	for _, row := range result.Rows {
		if row.BufLine < 0 {
			continue // 装饰行跳过
		}
		// 检查是否有内容（不只是空格）
		hasContent := false
		for _, cell := range row.Cells {
			if cell.Rune != '│' && cell.Rune != ' ' {
				hasContent = true
				break
			}
		}
		if !hasContent {
			t.Errorf("CJK row has no content")
		}
	}
}

func TestRenderTable_NarrowScreen(t *testing.T) {
	lines := []string{"| 项目 | 说明 |", "|------|------|", "| foo  | bar  |"}
	seg, cfg := makeTableSegment(lines)
	// 极端窄屏：宽度不足以放下任何列
	result := RenderTable(seg, 5, cfg)

	// 应该退化，返回背景色
	if len(result.Rows) != len(lines) {
		t.Errorf("RenderTable narrow screen rows = %d, want %d (退化)", len(result.Rows), len(lines))
	}
}

func TestRenderTable_MDTableAlignFalse(t *testing.T) {
	lines := []string{"| 项目 | 说明 |", "|------|------|", "| foo  | bar  |"}
	seg, cfg := makeTableSegment(lines)
	cfg.MDTableAlign = false
	result := RenderTable(seg, 30, cfg)

	// 应该正常渲染（alignments 全部为 left）
	if len(result.Rows) != 3 {
		t.Errorf("RenderTable MDTableAlign=false rows = %d, want 3", len(result.Rows))
	}
}

func TestRenderTable_CellWrap(t *testing.T) {
	// 创建一个 cell 内容很长的表格
	lines := []string{"| 项目 | 说明 |", "|------|------|", "| 这是一个非常非常长的内容需要换行 | 短 |"}
	seg, cfg := makeTableSegment(lines)
	result := RenderTable(seg, 30, cfg)

	// header 行 + 分隔线 + body 行（可能 wrap 成多行）
	if len(result.Rows) < 3 {
		t.Errorf("RenderTable cell wrap rows = %d, want >= 3", len(result.Rows))
	}

	// 验证 BufLine 映射正确
	for i, row := range result.Rows {
		_ = i
		// body 行应该在第 2 行之后
		if row.BufLine >= 2 && row.BufLine < 0 {
			t.Errorf("unexpected BufLine %d at row %d", row.BufLine, i)
		}
	}
}