package finder

import (
	"testing"

	"github.com/micro-editor/tcell/v2"
)

// ---- listRowAt 测试 ----

func TestListRowAt_Breadcrumb(t *testing.T) {
	l := &fileList{rect: Rect{X: 5, Y: 11, W: 39, H: 20}, showEntries: []*entry{{name: "a"}, {name: "b"}}, listH: 18, topIdx: 0}

	cursor, ok := l.listRowAt(11) // rect.Y = breadcrumb
	if !ok || cursor != 0 {
		t.Errorf("breadcrumb: got (%d, %v), want (0, true)", cursor, ok)
	}
}

func TestListRowAt_FirstEntry(t *testing.T) {
	l := &fileList{rect: Rect{X: 5, Y: 11, W: 39, H: 20}, showEntries: []*entry{{name: "a"}, {name: "b"}}, listH: 18, topIdx: 0}

	cursor, ok := l.listRowAt(12) // rect.Y+1
	if !ok || cursor != 1 {
		t.Errorf("first entry: got (%d, %v), want (1, true)", cursor, ok)
	}
}

func TestListRowAt_KthEntry(t *testing.T) {
	l := &fileList{rect: Rect{X: 5, Y: 11, W: 39, H: 20}, showEntries: []*entry{{name: "a"}, {name: "b"}, {name: "c"}, {name: "d"}}, listH: 18, topIdx: 2}

	for k, want := range []int{3, 4} {
		cursor, ok := l.listRowAt(12 + k)
		if !ok || cursor != want {
			t.Errorf("k=%d: got (%d, %v), want (%d, true)", k, cursor, ok, want)
		}
	}
}

func TestListRowAt_Scrolled(t *testing.T) {
	l := &fileList{rect: Rect{X: 5, Y: 11, W: 39, H: 20}, showEntries: []*entry{{name: "a"}, {name: "b"}, {name: "c"}}, listH: 2, topIdx: 1}

	cursor, ok := l.listRowAt(12)
	if !ok || cursor != 2 {
		t.Errorf("scrolled first entry: got (%d, %v), want (2, true)", cursor, ok)
	}
}

func TestListRowAt_BlankRow(t *testing.T) {
	l := &fileList{rect: Rect{X: 5, Y: 11, W: 39, H: 20}, showEntries: []*entry{{name: "a"}}, listH: 18, topIdx: 0}

	_, ok := l.listRowAt(13) // rect.Y+2：条目只有 1 个，后面空白
	if ok {
		t.Errorf("blank row: got ok=true, want ok=false")
	}
}

func TestListRowAt_Outside(t *testing.T) {
	l := &fileList{rect: Rect{X: 5, Y: 11, W: 39, H: 20}, showEntries: []*entry{{name: "a"}}, listH: 5, topIdx: 0}

	cases := []struct {
		name string
		my   int
	}{
		{"top border", 10},              // above rect.Y
		{"hint row", 11 + 5 + 1},        // rect.Y + listH + 1
		{"bottom border", 11 + 20 - 1},  // rect.Y + H - 1
		{"way below", 100},
	}
	for _, c := range cases {
		_, ok := l.listRowAt(c.my)
		if ok {
			t.Errorf("%s: got ok=true, want ok=false", c.name)
		}
	}
}

func TestListRowAt_MoreEntriesThanView(t *testing.T) {
	l := &fileList{rect: Rect{X: 5, Y: 11, W: 39, H: 20}, showEntries: []*entry{{name: "a"}, {name: "b"}, {name: "c"}, {name: "d"}, {name: "e"}}, listH: 3, topIdx: 0}

	for i := 0; i < 3; i++ {
		_, ok := l.listRowAt(12 + i)
		if !ok {
			t.Errorf("visible row %d: got ok=false, want ok=true", i)
		}
	}
	_, ok := l.listRowAt(12 + 3)
	if ok {
		t.Errorf("beyond visible: got ok=true, want ok=false")
	}
}

// ---- hitTest 测试 ----

type testRegion struct {
	r Rect
}

func (tr testRegion) Rect() Rect            { return tr.r }
func (tr testRegion) Display()              {}
func (tr testRegion) HandleMouse(_ *tcell.EventMouse) bool { return false }

func TestHitTest_Hit(t *testing.T) {
	regions := []testRegion{
		{r: Rect{X: 5, Y: 10, W: 10, H: 5}},
		{r: Rect{X: 20, Y: 10, W: 10, H: 5}},
	}
	ev := tcell.NewEventMouse(7, 12, tcell.Button1, 0, "")
	idx := hitTest(regions, ev)
	if idx != 0 {
		t.Errorf("hit first region: got %d, want 0", idx)
	}
}

func TestHitTest_HitSecond(t *testing.T) {
	regions := []testRegion{
		{r: Rect{X: 5, Y: 10, W: 10, H: 5}},
		{r: Rect{X: 20, Y: 10, W: 10, H: 5}},
	}
	ev := tcell.NewEventMouse(22, 12, tcell.Button1, 0, "")
	idx := hitTest(regions, ev)
	if idx != 1 {
		t.Errorf("hit second region: got %d, want 1", idx)
	}
}

func TestHitTest_Miss(t *testing.T) {
	regions := []testRegion{
		{r: Rect{X: 5, Y: 10, W: 10, H: 5}},
	}
	ev := tcell.NewEventMouse(0, 0, tcell.Button1, 0, "")
	idx := hitTest(regions, ev)
	if idx != -1 {
		t.Errorf("miss: got %d, want -1", idx)
	}
}

func TestHitTest_GapBetweenRegions(t *testing.T) {
	regions := []testRegion{
		{r: Rect{X: 5, Y: 10, W: 10, H: 5}},
		{r: Rect{X: 20, Y: 10, W: 10, H: 5}},
	}
	// 点 gap 列 (15..19) 应返回 -1
	ev := tcell.NewEventMouse(16, 12, tcell.Button1, 0, "")
	idx := hitTest(regions, ev)
	if idx != -1 {
		t.Errorf("gap: got %d, want -1", idx)
	}
}

func TestHitTest_EmptySlice(t *testing.T) {
	regions := []testRegion{}
	ev := tcell.NewEventMouse(5, 5, tcell.Button1, 0, "")
	idx := hitTest(regions, ev)
	if idx != -1 {
		t.Errorf("empty slice: got %d, want -1", idx)
	}
}

func TestHitTest_RightEdgeExclusive(t *testing.T) {
	regions := []testRegion{
		{r: Rect{X: 5, Y: 10, W: 10, H: 5}},
	}
	// X+W = 15，点 15 应返回 -1（右边界 exclusive）
	ev := tcell.NewEventMouse(15, 12, tcell.Button1, 0, "")
	idx := hitTest(regions, ev)
	if idx != -1 {
		t.Errorf("right edge: got %d, want -1", idx)
	}
	// X+W-1 = 14 应命中
	ev2 := tcell.NewEventMouse(14, 12, tcell.Button1, 0, "")
	idx2 := hitTest(regions, ev2)
	if idx2 != 0 {
		t.Errorf("rightmost col: got %d, want 0", idx2)
	}
}

func TestHitTest_BottomEdgeExclusive(t *testing.T) {
	regions := []testRegion{
		{r: Rect{X: 5, Y: 10, W: 10, H: 5}},
	}
	// Y+H = 15，点 15 应返回 -1
	ev := tcell.NewEventMouse(7, 15, tcell.Button1, 0, "")
	idx := hitTest(regions, ev)
	if idx != -1 {
		t.Errorf("bottom edge: got %d, want -1", idx)
	}
}