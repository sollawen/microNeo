package display

import (
	"testing"

	"github.com/micro-editor/micro/v2/internal/md"
)

func TestScreenOffsetToBufferLine(t *testing.T) {
	tests := []struct {
		name          string
		mdCache       []md.SegmentMeta
		screenOffset  int
		wantLine      int
		wantOK        bool
	}{
		{
			name:          "空缓存",
			mdCache:       nil,
			screenOffset:  0,
			wantLine:      0,
			wantOK:        false,
		},
		{
			name: "内容行直接命中",
			mdCache: []md.SegmentMeta{
				{BufStartLine: 0, BufEndLine: 2, RowBufLines: []int{0, 1, 2}},
			},
			screenOffset: 1,
			wantLine:     1,
			wantOK:       true,
		},
		{
			name: "装饰行往后找",
			mdCache: []md.SegmentMeta{
				{BufStartLine: 0, BufEndLine: 1, RowBufLines: []int{-1, 0, -1, 1}},
			},
			screenOffset: 0, // -1 → 往后找 → 0
			wantLine:     0,
			wantOK:       true,
		},
		{
			name: "装饰行往后找（中间）",
			mdCache: []md.SegmentMeta{
				{BufStartLine: 0, BufEndLine: 2, RowBufLines: []int{-1, 0, -1, 1, -1, 2}},
			},
			screenOffset: 4, // -1 → 往后找 → 2
			wantLine:     2,
			wantOK:       true,
		},
		{
			name: "装饰行后面没有了，用BufEndLine",
			mdCache: []md.SegmentMeta{
				{BufStartLine: 10, BufEndLine: 12, RowBufLines: []int{-1, 10, -1, 11, -1}},
			},
			screenOffset: 4, // -1 → 往后没有了 → BufEndLine=12
			wantLine:     12,
			wantOK:       true,
		},
		{
			name: "跨segment查找",
			mdCache: []md.SegmentMeta{
				{BufStartLine: 0, BufEndLine: 1, RowBufLines: []int{0, 1}},
				{BufStartLine: 5, BufEndLine: 6, RowBufLines: []int{5, 6}},
			},
			screenOffset: 3, // 跳过第一个segment(2行)，落在第二个segment的第1行
			wantLine:     6,
			wantOK:       true,
		},
		{
			name: "超出所有segment范围",
			mdCache: []md.SegmentMeta{
				{BufStartLine: 0, BufEndLine: 1, RowBufLines: []int{0, 1}},
			},
			screenOffset: 5,
			wantLine:     0,
			wantOK:       false,
		},
		{
			name: "表格：顶边框→header→分隔线→body→底边框",
			mdCache: []md.SegmentMeta{
				{BufStartLine: 10, BufEndLine: 12, RowBufLines: []int{-1, 10, -1, 12, -1}},
			},
			screenOffset: 3, // body行=12，直接命中
			wantLine:     12,
			wantOK:       true,
		},
		{
			name: "表格：click底边框，往后没有了",
			mdCache: []md.SegmentMeta{
				{BufStartLine: 10, BufEndLine: 12, RowBufLines: []int{-1, 10, -1, 12, -1}},
			},
			screenOffset: 4, // -1 → 往后没有了 → BufEndLine=12
			wantLine:     12,
			wantOK:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &BufWindow{mdCache: tt.mdCache}
			gotLine, gotOK := w.screenOffsetToBufferLine(tt.screenOffset)
			if gotOK != tt.wantOK {
				t.Errorf("screenOffsetToBufferLine() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotLine != tt.wantLine {
				t.Errorf("screenOffsetToBufferLine() line = %d, want %d", gotLine, tt.wantLine)
			}
		})
	}
}
