package finder

import "testing"

// ---- listRowAt 测试 ----

func TestListRowAt_Breadcrumb(t *testing.T) {
	// 面包屑行 (X, Y+1) → cursor=0
	fm := &Session{rect: Rect{X: 5, Y: 10, W: 40, H: 30}}
	fm.state = &finderState{showEntries: []*entry{{name: "a"}, {name: "b"}}, listH: 20, topIdx: 0}
	fm.state.pickerW = 40

	cursor, ok := fm.listRowAt(11) // Y+1 = 10+1
	if !ok || cursor != 0 {
		t.Errorf("breadcrumb: got (%d, %v), want (0, true)", cursor, ok)
	}
}

func TestListRowAt_FirstEntry(t *testing.T) {
	// 首条目行 (X, Y+2) → cursor=topIdx+1
	fm := &Session{rect: Rect{X: 5, Y: 10, W: 40, H: 30}}
	fm.state = &finderState{showEntries: []*entry{{name: "a"}, {name: "b"}}, listH: 20, topIdx: 0}
	fm.state.pickerW = 40

	cursor, ok := fm.listRowAt(12) // Y+2 = 10+2
	if !ok || cursor != 1 {
		t.Errorf("first entry: got (%d, %v), want (1, true)", cursor, ok)
	}
}

func TestListRowAt_KthEntry(t *testing.T) {
	// 第 k 可见条目行 → cursor=topIdx+k+1
	fm := &Session{rect: Rect{X: 5, Y: 10, W: 40, H: 30}}
	fm.state = &finderState{showEntries: []*entry{{name: "a"}, {name: "b"}, {name: "c"}, {name: "d"}}, listH: 20, topIdx: 2}
	fm.state.pickerW = 40

	for k, want := range []int{3, 4} { // topIdx=2, k=0→cursor=3, k=1→cursor=4
		cursor, ok := fm.listRowAt(12 + k) // Y+2+k
		if !ok || cursor != want {
			t.Errorf("k=%d: got (%d, %v), want (%d, true)", k, cursor, ok, want)
		}
	}
}

func TestListRowAt_Scrolled(t *testing.T) {
	// topIdx>0 时首条目映射到 topIdx+1
	fm := &Session{rect: Rect{X: 5, Y: 10, W: 40, H: 30}}
	fm.state = &finderState{showEntries: []*entry{{name: "a"}, {name: "b"}, {name: "c"}}, listH: 2, topIdx: 1}
	fm.state.pickerW = 40

	cursor, ok := fm.listRowAt(12) // Y+2：topIdx=1 时对应 showEntries[1] → cursor=2
	if !ok || cursor != 2 {
		t.Errorf("scrolled first entry: got (%d, %v), want (2, true)", cursor, ok)
	}
}

func TestListRowAt_BlankRow(t *testing.T) {
	// 条目少于视口时，点空白行 → ok=false
	fm := &Session{rect: Rect{X: 5, Y: 10, W: 40, H: 30}}
	fm.state = &finderState{showEntries: []*entry{{name: "a"}}, listH: 20, topIdx: 0}
	fm.state.pickerW = 40

	_, ok := fm.listRowAt(13) // Y+3：条目只有 1 个，后面空白
	if ok {
		t.Errorf("blank row: got ok=true, want ok=false")
	}
}

func TestListRowAt_Outside(t *testing.T) {
	// 点分隔符列 / 上边框 / hint 行 / 下边框 → ok=false
	fm := &Session{rect: Rect{X: 5, Y: 10, W: 40, H: 30}}
	fm.state = &finderState{showEntries: []*entry{{name: "a"}}, listH: 5, topIdx: 0}
	fm.state.pickerW = 40

	cases := []struct {
		name string
		my   int
	}{
		{"top border", 10},              // Y
		{"hint row", 10 + 1 + 5 + 1},    // Y+1+listH+1 = Y+7
		{"bottom border", 10 + 30 - 1},  // Y+H-1
		{"way below", 100},
	}
	for _, c := range cases {
		_, ok := fm.listRowAt(c.my)
		if ok {
			t.Errorf("%s: got ok=true, want ok=false", c.name)
		}
	}
}

func TestListRowAt_MoreEntriesThanView(t *testing.T) {
	// 条目多于 listH：只能点当前可见的 listH 行
	fm := &Session{rect: Rect{X: 5, Y: 10, W: 40, H: 30}}
	fm.state = &finderState{showEntries: []*entry{{name: "a"}, {name: "b"}, {name: "c"}, {name: "d"}, {name: "e"}}, listH: 3, topIdx: 0}
	fm.state.pickerW = 40

	// 可见行 0/1/2 → ok；行 3（超过 listH）→ ok=false
	for i := 0; i < 3; i++ {
		_, ok := fm.listRowAt(12 + i) // Y+2+i
		if !ok {
			t.Errorf("visible row %d: got ok=false, want ok=true", i)
		}
	}
	_, ok := fm.listRowAt(12 + 3) // Y+5：超过 listH=3
	if ok {
		t.Errorf("beyond visible: got ok=true, want ok=false")
	}
}

// ---- whereIsMouse 测试 ----

func TestWhereIsMouse_LeftColumn(t *testing.T) {
	fm := &Session{rect: Rect{X: 5, Y: 10, W: 40, H: 30}}
	fm.state = &finderState{showEntries: []*entry{{name: "a"}}, listH: 20, topIdx: 0, pvRect: Rect{X: 45, Y: 10, W: 30, H: 30}}
	fm.state.pickerW = 40

	cases := []struct {
		name string
		mx, my int
	}{
		{"breadcrumb row", 6, 11},    // X+1, Y+1
		{"first entry row", 6, 12},   // X+1, Y+2
		{"last visible row", 6, 31},  // X+1, Y+1+listH
	}
	for _, c := range cases {
		got := fm.whereIsMouse(c.mx, c.my)
		if got != mouseLeft {
			t.Errorf("%s: got %v, want mouseLeft", c.name, got)
		}
	}
}

func TestWhereIsMouse_LeftColEdgeCases(t *testing.T) {
	fm := &Session{rect: Rect{X: 5, Y: 10, W: 40, H: 30}}
	fm.state = &finderState{showEntries: []*entry{{name: "a"}}, listH: 20, topIdx: 0, pvRect: Rect{X: 45, Y: 10, W: 30, H: 30}}
	fm.state.pickerW = 40

	// 左栏左右边界
	if got := fm.whereIsMouse(5, 12); got != mouseLeft { // X, left edge inclusive
		t.Errorf("left edge X: got %v, want mouseLeft", got)
	}
	if got := fm.whereIsMouse(43, 12); got != mouseLeft { // X+pickerW-2, rightmost content col
		t.Errorf("rightmost content col: got %v, want mouseLeft", got)
	}
	if got := fm.whereIsMouse(44, 12); got != mouseOutside { // X+pickerW-1, separator col
		t.Errorf("separator col: got %v, want mouseOutside", got)
	}
	if got := fm.whereIsMouse(45, 12); got != mouseRight { // X+pickerW, preview left col
		t.Errorf("preview left col: got %v, want mouseRight", got)
	}
}

func TestWhereIsMouse_RightmostContentCol_BugFix(t *testing.T) {
	// Bug fix: 点左栏最右一列内容 (X+pickerW-2) 应该返回 mouseLeft，
	// 修复前旧代码 mx < X+pickerW-1 会错误地把它排到 mouseOutside
	fm := &Session{rect: Rect{X: 5, Y: 10, W: 40, H: 30}}
	fm.state = &finderState{showEntries: []*entry{{name: "a"}}, listH: 20, topIdx: 0, pvRect: Rect{X: 45, Y: 10, W: 30, H: 30}}
	fm.state.pickerW = 40

	// X+pickerW-2 = 43，左栏最右一列内容
	if got := fm.whereIsMouse(43, 12); got != mouseLeft {
		t.Errorf("rightmost content col (X+pickerW-2): got %v, want mouseLeft", got)
	}
}

func TestWhereIsMouse_RightColumn(t *testing.T) {
	fm := &Session{rect: Rect{X: 5, Y: 10, W: 40, H: 30}}
	fm.state = &finderState{showEntries: []*entry{{name: "a"}}, listH: 20, topIdx: 0, pvRect: Rect{X: 45, Y: 10, W: 30, H: 30}}
	fm.state.pickerW = 40

	cases := []struct {
		name string
		mx, my int
	}{
		{"preview top-left", 46, 11},
		{"preview mid", 55, 20},
		{"preview bottom-right", 74, 39},
	}
	for _, c := range cases {
		got := fm.whereIsMouse(c.mx, c.my)
		if got != mouseRight {
			t.Errorf("%s: got %v, want mouseRight", c.name, got)
		}
	}
}

func TestWhereIsMouse_Outside(t *testing.T) {
	fm := &Session{rect: Rect{X: 5, Y: 10, W: 40, H: 30}}
	fm.state = &finderState{showEntries: []*entry{{name: "a"}}, listH: 20, topIdx: 0, pvRect: Rect{X: 45, Y: 10, W: 30, H: 30}}
	fm.state.pickerW = 40

	cases := []struct {
		name string
		mx, my int
	}{
		{"top border", 20, 10},
		{"bottom border", 20, 39},
		{"separator col", 44, 15},   // X+pickerW-1 = 44
		{"above left col", 20, 9},   // my < Y+1
		{"below left col", 20, 32},  // my >= Y+1+listH+1
		{"far left", 0, 15},
		{"far right", 100, 15},
		{"far below", 20, 100},
	}
	for _, c := range cases {
		got := fm.whereIsMouse(c.mx, c.my)
		if got != mouseOutside {
			t.Errorf("%s: got %v, want mouseOutside", c.name, got)
		}
	}
}

func TestWhereIsMouse_LeftTakesPrecedence(t *testing.T) {
	// 左栏和右栏在左上角可能有重叠（preview X 紧接 left W），但横向已互斥：
	// 左栏横向 [X, X+pickerW)，右栏横向 [X+pickerW, ...)，不相交。
	fm := &Session{rect: Rect{X: 5, Y: 10, W: 40, H: 30}}
	fm.state = &finderState{showEntries: []*entry{{name: "a"}}, listH: 20, topIdx: 0, pvRect: Rect{X: 45, Y: 10, W: 30, H: 30}}
	fm.state.pickerW = 40

	// X+pickerW-1 = 44，是分隔符列 → mouseOutside
	if got := fm.whereIsMouse(44, 20); got != mouseOutside {
		t.Errorf("separator col (X+pickerW-1): got %v, want mouseOutside", got)
	}
	// X+pickerW = 45，是右栏最左列
	if got := fm.whereIsMouse(45, 20); got != mouseRight {
		t.Errorf("preview left col (X+pickerW): got %v, want mouseRight", got)
	}
}
