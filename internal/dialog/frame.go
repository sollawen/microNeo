// Package dialog provides modal floating window components for microNeo.
//
// The dialog package contains the container (FloatFrame) and concrete dialogs
// (SelectDialog, etc.) that share the same open/close lifecycle, layout, and
// modal behavior. All dialogs are self-contained and do not reference the
// action package.
package dialog

import (
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

// FloatFrame is the unified container for all modal floating windows in microNeo.
//
// The container handles shared concerns: anchor expansion, frame drawing, clearing,
// lifecycle, and event routing. Business logic (list selection, scrolling, dynamic
// refresh, etc.) is entirely delegated to concrete dialogs. A concrete dialog opens
// by passing content size, anchor, title, frame color, display function, and
// handleEvent function to Open; FloatFrame takes over user interaction until Close.
//
// Design constraints: single floating window at a time, no nesting, modal,
// container stays thin.
//
// Layout semantics: Open's contentSize is pure content (excludes border).
// FloatFrame internally derives outerW = max(contentSize.W+2, len(title)+6),
// outerH = contentSize.H+2, then expands anchor to (fx, fy).
// When delegating content, contentArea is always Rect{fx+1, fy+1, contentSize.W, contentSize.H}
// so concrete dialogs don't need to know borders exist.

// Rect is a rectangular area in screen coordinates.
type Rect struct {
	X, Y, W, H int
}

// Pos is a point in screen coordinates (used for anchors).
type Pos struct {
	X, Y int
}

// Size is width and height (used for content dimensions).
type Size struct {
	W, H int
}

// FloatOpenSpec bundles all parameters for opening a floating window.
type FloatOpenSpec struct {
	Anchor      Pos                     // AutoExpand=true: expansion center; false: top-left of outer rect
	ContentSize Size                    // Pure content size (excludes border)
	Title       string                  // Label embedded in top border; empty = plain horizontal line
	FrameColor  tcell.Style             // Frame color; zero = config.DefStyle
	Display     func(contentArea Rect)  // Draw content (area already excludes border)
	HandleEvent func(event tcell.Event) // Handle key events (resize is intercepted by FloatFrame)
	OnResize    func()                  // Called only when container closes due to resize (ESC/Cancel/Close don't trigger it)
	AutoExpand  bool                    // true: anchor自适应展开; false: use Anchor directly as top-left of outer rect
}

// FloatFrame is the container itself.
type FloatFrame struct {
	isOpen bool

	// —— From concrete dialog (pure content semantics, no border) ——
	anchor      Pos         // Anchor in screen coordinates
	contentSize Size        // Pure content size
	title       string      // Label embedded in top border
	frameColor  tcell.Style // Frame drawing color; zero = config.DefStyle
	display     func(contentArea Rect)
	handleEvent func(event tcell.Event)
	onResize    func() // Only triggered on resize; Close() clears to prevent stale callbacks

	// —— Computed by FloatFrame ——
	outerW, outerH int // Total size including border (contentSize + 2, or title width if wider)
	fx, fy         int // Final top-left after anchor expansion
}

// TheFloatFrame is the global singleton container, initialized at package load time.
var TheFloatFrame = NewFloatFrame()

// NewFloatFrame returns an empty FloatFrame.
func NewFloatFrame() *FloatFrame {
	return &FloatFrame{}
}

// Open opens the floating window with the given spec.
//
// Layout branches on AutoExpand:
//   - AutoExpand=true → anchor自适应展开（贴光标弹窗）
//   - AutoExpand=false → use Anchor directly as top-left of outer rect
//
// Returns bool: true = opened successfully; false = rejected (already open / doesn't fit screen).
func (f *FloatFrame) Open(spec FloatOpenSpec) bool {
	if f.isOpen {
		return false
	}

	// Derive outerW/outerH
	outerW := spec.ContentSize.W + 2
	if titleW := len(spec.Title) + 6; titleW > outerW {
		outerW = titleW
	}
	outerH := spec.ContentSize.H + 2

	// Pre-flight check
	w, h := screen.Screen.Size()
	iOffset := config.GetInfoBarOffset()
	bottomLimit := h - iOffset - 1 - 1
	if outerH > bottomLimit+1 || outerW > w {
		return false
	}

	// Store fields
	ax, ay := spec.Anchor.X, spec.Anchor.Y
	if spec.AutoExpand && ay < 0 { // sentinel: only used by SelectDialog to stick to statusLine
		ay = bottomLimit + ay + 1
	}
	fc := spec.FrameColor
	if fc == (tcell.Style{}) {
		fc = config.DefStyle
	}
	f.anchor = Pos{X: ax, Y: ay}
	f.contentSize = spec.ContentSize
	f.title = spec.Title
	f.frameColor = fc
	f.display = spec.Display
	f.handleEvent = spec.HandleEvent
	f.onResize = spec.OnResize
	f.outerW, f.outerH = outerW, outerH

	// Layout branch
	if spec.AutoExpand {
		f.fx, f.fy = expandAnchor(ax, ay, outerW, outerH)
	} else {
		f.fx, f.fy = ax, ay
	}
	f.isOpen = true
	screen.Redraw()
	return true
}

// Close closes the floating window and resets to empty state.
// Callers should Close() before triggering business callbacks so callbacks
// see the container in closed state.
func (f *FloatFrame) Close() {
	if !f.isOpen {
		return
	}
	f.isOpen = false
	f.display = nil
	f.handleEvent = nil
	f.onResize = nil
	screen.Redraw()
}

// IsOpen returns whether the floating window is open.
func (f *FloatFrame) IsOpen() bool { return f.isOpen }

// Display draws the floating window: clear → corners + borders → title → delegate content.
// Must be drawn last (later writes win + modal consistency).
func (f *FloatFrame) Display() {
	if !f.isOpen {
		return
	}

	x, y := f.fx, f.fy
	w, h := f.outerW, f.outerH
	color := f.frameColor

	// Hide cursor from underlying panes. tcell cursor position and cell content are
	// independent: drawing cells doesn't overwrite cursor, so without HideCursor
	// the cursor would float inside the dialog (D15 confirmed bug).
	screen.Screen.HideCursor()

	// 1. Clear entire rectangle (border + content)
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			screen.Screen.SetContent(x+col, y+row, ' ', nil, color)
		}
	}

	// 2. Corners + top/bottom borders
	screen.Screen.SetContent(x, y, '┌', nil, color)
	screen.Screen.SetContent(x+w-1, y, '┐', nil, color)
	screen.Screen.SetContent(x, y+h-1, '└', nil, color)
	screen.Screen.SetContent(x+w-1, y+h-1, '┘', nil, color)
	for i := 1; i < w-1; i++ {
		screen.Screen.SetContent(x+i, y, '─', nil, color)
		screen.Screen.SetContent(x+i, y+h-1, '─', nil, color)
	}

	// 3. Left + right borders
	for row := 0; row < h-2; row++ {
		screenY := y + 1 + row
		screen.Screen.SetContent(x, screenY, '│', nil, color)
		screen.Screen.SetContent(x+w-1, screenY, '│', nil, color)
	}

	// 4. Embed title in top border: "──<title>──...─"
	col := x + 1
	write := func(r rune) {
		if col < x+w-1 {
			screen.Screen.SetContent(col, y, r, nil, color)
			col++
		}
	}
	write('─')
	write('─')
	for _, r := range f.title {
		write(r)
	}
	write('─')
	write('─')
	for col < x+w-1 {
		write('─')
	}

	// 5. Delegate content drawing (contentArea = fx+1, fy+1, contentSize.W, contentSize.H)
	f.display(Rect{X: f.fx + 1, Y: f.fy + 1, W: f.contentSize.W, H: f.contentSize.H})
}

// HandleEvent forwards events to concrete dialog. Resize is intercepted (closes + calls OnResize),
// other events are forwarded as-is.
func (f *FloatFrame) HandleEvent(event tcell.Event) {
	if !f.isOpen {
		return
	}
	if _, ok := event.(*tcell.EventResize); ok {
		cb := f.onResize
		f.Close()
		if cb != nil {
			cb()
		}
		return
	}
	f.handleEvent(event)
}

// expandAnchor computes the final top-left (fx, fy) after anchor expansion.
//
// Called only when AutoExpand=true (SelectDialog sticks to cursor/statusLine path).
// Algorithm (y and x are independent, each prefers right/down, falls back to left/up,
// then to whichever side has more space):
//
//	x direction: prefer right, fallback left, then whichever has more space
//	y direction: prefer down, fallback up, then whichever has more space
//
// Boundary clamping is defensive; normal path is protected by pre-flight check in Open.
func expandAnchor(ax, ay, outerW, outerH int) (fx, fy int) {
	w, h := screen.Screen.Size()
	iOffset := config.GetInfoBarOffset()
	statusLineY := h - iOffset - 1
	bottomLimit := statusLineY - 1

	// y direction
	downSpace := bottomLimit - ay + 1
	upSpace := ay + 1
	switch {
	case downSpace >= outerH:
		fy = ay
	case upSpace >= outerH:
		fy = ay - outerH + 1
	case downSpace >= upSpace:
		fy = ay
	default:
		fy = ay - outerH + 1
	}
	// Defensive boundary clamp
	maxY := bottomLimit - outerH + 1
	if maxY < 0 {
		maxY = 0
	}
	if fy > maxY {
		fy = maxY
	}
	if fy < 0 {
		fy = 0
	}

	// x direction (symmetric with y)
	rightSpace := w - ax
	leftSpace := ax + 1
	switch {
	case rightSpace >= outerW:
		fx = ax
	case leftSpace >= outerW:
		fx = ax - outerW + 1
	case rightSpace >= leftSpace:
		fx = ax
	default:
		fx = ax - outerW + 1
	}
	// Defensive boundary clamp
	maxX := w - outerW
	if maxX < 0 {
		maxX = 0
	}
	if fx > maxX {
		fx = maxX
	}
	if fx < 0 {
		fx = 0
	}

	return fx, fy
}
