package dialog

import (
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

// SelectDialog is a modal list selection dialog (concrete floating window).
//
// v2 scrolling: caller passes maxVisible/wrap; SelectDialog internally manages
// topIdx (viewport offset). Frame, title, clearing, anchor expansion, layout,
// and lifecycle are entirely handled by FloatFrame. This file only contains
// business logic: list state, keyboard mapping, and callback notification.
//
// Lifecycle:
//   - Caller creates a SelectDialog, calls Open(...), passing display/handleEvent to TheFloatFrame.
//   - During execution the SelectDialog object is not referenced by the main loop (D2': GC-eligible).
//   - Close is initiated from handleEvent: first TheFloatFrame.Close(), then onSelect(...).

type SelectDialog struct {
	items      []string
	selected   int   // Absolute selected index (0..len-1)
	topIdx     int   // Viewport top in items
	maxVisible int   // Viewport height limit (<=0 means unlimited)
	wrap       bool  // true: wrap around; false: stop at ends
	title      string
	onSelect   func(*string)
}

// NewSelectDialog returns an empty SelectDialog (closed state).
func NewSelectDialog() *SelectDialog {
	return &SelectDialog{}
}

// Open opens the selection floating window.
//
//	items       list of options
//	title       top border label (e.g. "Receiver")
//	anchor      trigger anchor in screen coordinates (usually from cursor position)
//	frameColor  frame color; tcell.Style{} zero = config.DefStyle
//	maxVisible  viewport height; <=0 falls back to len(items) (no scrolling)
//	wrap        true: wrap around (default); false: stop at ends
//	onSelect    callback: Enter → onSelect(&items[selected]); Esc/resize → onSelect(nil)
//
// Layout, frame, and pre-flight checks are handled by FloatFrame. If FloatFrame.Open
// returns false (rejected or doesn't fit), we call onSelect(nil) directly without
// setting any business state (transparent to caller).
func (s *SelectDialog) Open(
	items []string,
	title string,
	anchor Pos,
	frameColor tcell.Style,
	maxVisible int,
	wrap bool,
	onSelect func(*string),
) {
	s.items = items
	s.selected = 0
	s.topIdx = 0
	s.maxVisible = maxVisible
	s.wrap = wrap
	s.title = title
	s.onSelect = onSelect

	// Compute pure content size (width includes 1 padding each side; height = maxVisible or len(items))
	visibleH := len(s.items)
	if maxVisible > 0 && visibleH > maxVisible {
		visibleH = maxVisible
	}
	contentSize := Size{
		W: maxItemLen(items) + 2,
		H: visibleH,
	}

	spec := FloatOpenSpec{
		Anchor:      anchor,
		ContentSize: contentSize,
		Title:       title,
		FrameColor:  frameColor,
		Display:     s.display,
		HandleEvent: s.handleEvent,
		AutoExpand:  true, // SelectDialog: 贴光标/贴 statusLine 展开
		OnResize: func() {
			if s.onSelect != nil {
				s.onSelect(nil)
			}
		},
	}
	if !TheFloatFrame.Open(spec) {
		s.items = nil
		s.onSelect = nil
		if onSelect != nil {
			onSelect(nil)
		}
	}
}

// display draws only visible items [topIdx, topIdx+visibleH); current selection is reversed.
// Frame and title are handled by FloatFrame.
func (s *SelectDialog) display(area Rect) {
	revStyle := config.DefStyle.Reverse(true)
	visibleH := min(len(s.items), s.effMaxVisible())

	for vi := 0; vi < visibleH; vi++ {
		i := s.topIdx + vi
		if i >= len(s.items) {
			break
		}
		row := area.Y + vi
		style := config.DefStyle
		if i == s.selected {
			style = revStyle
		}
		// Left padding
		screen.Screen.SetContent(area.X, row, ' ', nil, style)
		// Text
		col := area.X + 1
		for _, r := range s.items[i] {
			if col >= area.X+area.W {
				break
			}
			screen.Screen.SetContent(col, row, r, nil, style)
			col++
		}
		// Right padding
		for col < area.X+area.W {
			screen.Screen.SetContent(col, row, ' ', nil, style)
			col++
		}
	}

	// Scroll indicators (only when content overflows)
	if len(s.items) > visibleH {
		rightCol := area.X + area.W - 1
		topStyle := config.DefStyle
		if s.topIdx == s.selected {
			topStyle = revStyle
		}
		botStyle := config.DefStyle
		botIdx := s.topIdx + visibleH - 1
		if botIdx == s.selected {
			botStyle = revStyle
		}
		if s.topIdx > 0 {
			screen.Screen.SetContent(rightCol, area.Y, '▲', nil, topStyle)
		}
		if s.topIdx+visibleH < len(s.items) {
			screen.Screen.SetContent(rightCol, area.Y+visibleH-1, '▼', nil, botStyle)
		}
	}
}

// handleEvent processes keyboard events. Up/Down change selection and scroll viewport;
// Enter/Esc/Resize close container first then callback; other keys are swallowed (modal).
func (s *SelectDialog) handleEvent(event tcell.Event) {
	switch ev := event.(type) {
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyDown:
			if s.wrap {
				s.selected = (s.selected + 1) % len(s.items)
			} else if s.selected < len(s.items)-1 {
				s.selected++
			}
			s.ensureVisible()
			screen.Redraw()
			return
		case tcell.KeyUp:
			if s.wrap {
				s.selected = (s.selected - 1 + len(s.items)) % len(s.items)
			} else if s.selected > 0 {
				s.selected--
			}
			s.ensureVisible()
			screen.Redraw()
			return
		case tcell.KeyEnter:
			picked := s.items[s.selected]
			onSelect := s.onSelect
			TheFloatFrame.Close()
			if onSelect != nil {
				onSelect(&picked)
			}
			return
		case tcell.KeyEscape:
			onSelect := s.onSelect
			TheFloatFrame.Close()
			if onSelect != nil {
				onSelect(nil)
			}
			return
		}
	}
}

// ensureVisible scrolls the viewport to include selected index.
func (s *SelectDialog) ensureVisible() {
	visibleH := min(len(s.items), s.effMaxVisible())
	if s.selected < s.topIdx {
		s.topIdx = s.selected
	}
	if s.selected >= s.topIdx+visibleH {
		s.topIdx = s.selected - visibleH + 1
	}
}

// effMaxVisible returns the effective viewport height limit (maxVisible<=0 means unlimited).
func (s *SelectDialog) effMaxVisible() int {
	if s.maxVisible <= 0 {
		return len(s.items)
	}
	return s.maxVisible
}

// maxItemLen returns the character count of the longest item (rune count).
func maxItemLen(items []string) int {
	m := 0
	for _, it := range items {
		if len(it) > m {
			m = len(it)
		}
	}
	return m
}

// min returns the smaller of a and b.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
