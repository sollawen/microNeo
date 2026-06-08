package display

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	luar "layeh.com/gopher-luar"

	"github.com/micro-editor/tcell/v2"
	runewidth "github.com/mattn/go-runewidth"
	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	ulua "github.com/micro-editor/micro/v2/internal/lua"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/micro/v2/internal/util"
	lua "github.com/yuin/gopher-lua"
)

// StatusLine represents the information line at the bottom
// of each window
// It gives information such as filename, whether the file has been
// modified, filetype, cursor location
type StatusLine struct {
	Info map[string]func(*buffer.Buffer) string

	win *BufWindow
}

// statusSegment represents a text segment with its own style
type statusSegment struct {
	text  []byte
	style tcell.Style
}

var statusInfo = map[string]func(*buffer.Buffer) string{
	"filename": func(b *buffer.Buffer) string {
		return b.GetName()
	},
	"line": func(b *buffer.Buffer) string {
		return strconv.Itoa(b.GetActiveCursor().Y + 1)
	},
	"col": func(b *buffer.Buffer) string {
		return strconv.Itoa(b.GetActiveCursor().X + 1)
	},
	"modified": func(b *buffer.Buffer) string {
		if b.Modified() {
			return "+ "
		}
		if b.Type.Readonly {
			return "[ro] "
		}
		return ""
	},
	"overwrite": func(b *buffer.Buffer) string {
		if b.OverwriteMode && !b.Type.Readonly {
			return "[ovwr] "
		}
		return ""
	},
	"lines": func(b *buffer.Buffer) string {
		return strconv.Itoa(b.LinesNum())
	},
	"percentage": func(b *buffer.Buffer) string {
		return strconv.Itoa((b.GetActiveCursor().Y + 1) * 100 / b.LinesNum())
	},
	"position": func(b *buffer.Buffer) string {
		cur := b.GetActiveCursor().Y + 1
		total := b.LinesNum()
		if total == 0 {
			return "1/1"
		}
		return fmt.Sprintf("%d/%d", cur, total)
	},
	"brand": func(b *buffer.Buffer) string {
		return "microNeo"
	},
}

func SetStatusInfoFnLua(fn string) {
	luaFn := strings.Split(fn, ".")
	if len(luaFn) <= 1 {
		return
	}
	plName, plFn := luaFn[0], luaFn[1]
	pl := config.FindPlugin(plName)
	if pl == nil {
		return
	}
	statusInfo[fn] = func(b *buffer.Buffer) string {
		if pl == nil || !pl.IsLoaded() {
			return ""
		}
		val, err := pl.Call(plFn, luar.New(ulua.L, b))
		if err == nil {
			if v, ok := val.(lua.LString); !ok {
				screen.TermMessage(plFn, "should return a string")
				return ""
			} else {
				return string(v)
			}
		}
		return ""
	}
}

// NewStatusLine returns a statusline bound to a window
func NewStatusLine(win *BufWindow) *StatusLine {
	s := new(StatusLine)
	s.win = win
	return s
}

// FindOpt finds a given option in the current buffer's settings
func (s *StatusLine) FindOpt(opt string) any {
	if val, ok := s.win.Buf.Settings[opt]; ok {
		return val
	}
	return "null"
}

var tokenParser = regexp.MustCompile(`\$\[([^\]]+)\]|\$\((.+?)\)|\$sep\b`)

// lookupColor looks up a color name in the colorscheme and returns the style.
// It adds "statusline." prefix automatically. Falls back to normalStyle on failure.
func lookupColor(name string, normalStyle tcell.Style) tcell.Style {
	if name == "" {
		return normalStyle
	}
	if style, ok := config.Colorscheme["statusline."+name]; ok {
		return style
	}
	return normalStyle
}

// resolvePlaceholder resolves a placeholder name to its text value.
// Supports opt: and bind: prefixes, as well as statusInfo functions.
func (s *StatusLine) resolvePlaceholder(name string) []byte {
	if strings.HasPrefix(name, "opt:") {
		return fmt.Append(nil, s.FindOpt(name[4:]))
	}
	if strings.HasPrefix(name, "bind:") {
		binding := name[5:]
		for k, v := range config.Bindings["buffer"] {
			if v == binding {
				return []byte(k)
			}
		}
		return []byte("null")
	}
	if fn, ok := statusInfo[name]; ok {
		return []byte(fn(s.win.Buf))
	}
	return []byte{}
}

// parseFormat parses the format string and returns segments with styles.
// Supports $[color] color switching, $(name) placeholders, and $sep separators.
func (s *StatusLine) parseFormat(format string, defaultStyle tcell.Style) []statusSegment {
	normalStyle := lookupColor("normal", defaultStyle)
	currentStyle := normalStyle
	prevStyle := currentStyle
	var segments []statusSegment

	addSegment := func(text []byte, style tcell.Style) {
		if len(text) > 0 {
			segments = append(segments, statusSegment{text, style})
		}
	}

	matches := tokenParser.FindAllStringSubmatchIndex(format, -1)
	pos := 0

	for _, match := range matches {
		// Literal text before this match
		if match[0] > pos {
			addSegment([]byte(format[pos:match[0]]), currentStyle)
		}

		matchStr := format[match[0]:match[1]]
		if len(matchStr) < 2 {
			pos = match[1]
			continue
		}

		switch {
		case matchStr[1] == '[':
			// $[color] — switch color, save prevStyle for $sep
			name := matchStr[2 : len(matchStr)-1]
			prevStyle = currentStyle
			currentStyle = lookupColor(name, normalStyle)

		case matchStr[1] == '(':
			// $(name) — placeholder
			name := matchStr[2 : len(matchStr)-1]
			text := s.resolvePlaceholder(name)
			style := currentStyle
			if name == "brand" {
				style = style.Bold(true)
			}
			addSegment(text, style)

		default:
			// $sep — separator with colors from prevStyle and currentStyle
			sepChar, _ := s.win.Buf.Settings["status-separator"].(string)
			if sepChar == "" {
				sepChar = " "
			}
			_, prevBg, _ := prevStyle.Decompose()
			_, curBg, _ := currentStyle.Decompose()
			sepStyle := tcell.StyleDefault.Foreground(prevBg).Background(curBg)
			addSegment([]byte(sepChar), sepStyle)
		}

		pos = match[1]
	}

	// Remaining literal text
	if pos < len(format) {
		addSegment([]byte(format[pos:]), currentStyle)
	}

	return segments
}

// segReader tracks position in a slice of status segments for rendering.
type segReader struct {
	segs    []statusSegment
	segIdx  int
	byteIdx int
}

// nextRune returns the next rune from the segments and advances the position.
func (r *segReader) nextRune() (rune, []rune, int, bool) {
	if r.segIdx >= len(r.segs) {
		return 0, nil, 0, false
	}
	seg := r.segs[r.segIdx]
	ch, combc, size := util.DecodeCharacter(seg.text[r.byteIdx:])
	r.byteIdx += size
	if r.byteIdx >= len(seg.text) {
		r.segIdx++
		r.byteIdx = 0
	}
	return ch, combc, size, true
}

// currentStyle returns the style of the current segment.
func (r *segReader) currentStyle() tcell.Style {
	if r.segIdx < len(r.segs) {
		return r.segs[r.segIdx].style
	}
	return tcell.StyleDefault
}

// renderSeg renders a single character (or wide character spread) to the screen.
func renderSeg(reader *segReader, isActive bool, activeStyle tcell.Style, winX, y, x int) int {
	style := activeStyle
	if isActive {
		style = reader.currentStyle()
	}
	ch, combc, _, _ := reader.nextRune()
	rw := runewidth.RuneWidth(ch)
	for j := 0; j < rw; j++ {
		c := ch
		if j > 0 {
			c = ' '
			combc = nil
			x++
		}
		screen.SetContent(winX+x, y, c, combc, style)
	}
	return x
}

// Display draws the statusline to the screen
func (s *StatusLine) Display() {
	// We'll draw the line at the lowest line in the window
	y := s.win.Height + s.win.Y - 1

	winX := s.win.X

	b := s.win.Buf
	// autocomplete suggestions (for the buffer, not for the infowindow)
	if b.HasSuggestions && len(b.Suggestions) > 1 {
		statusLineStyle := config.DefStyle.Reverse(true)
		if style, ok := config.Colorscheme["statusline.suggestions"]; ok {
			statusLineStyle = style
		} else if style, ok := config.Colorscheme["statusline"]; ok {
			statusLineStyle = style
		}
		x := 0
		for j, sug := range b.Suggestions {
			style := statusLineStyle
			if b.CurSuggestion == j {
				style = style.Reverse(true)
			}
			for _, r := range sug {
				screen.SetContent(winX+x, y, r, nil, style)
				x++
				if x >= s.win.Width {
					return
				}
			}
			screen.SetContent(winX+x, y, ' ', nil, statusLineStyle)
			x++
			if x >= s.win.Width {
				return
			}
		}

		for x < s.win.Width {
			screen.SetContent(winX+x, y, ' ', nil, statusLineStyle)
			x++
		}
		return
	}

	// Determine default style for statusline
	statusLineStyle := config.DefStyle.Reverse(true)
	if s.win.IsActive() {
		if style, ok := config.Colorscheme["statusline"]; ok {
			statusLineStyle = style
		}
	} else {
		if style, ok := config.Colorscheme["statusline.inactive"]; ok {
			statusLineStyle = style
		} else if style, ok := config.Colorscheme["statusline"]; ok {
			statusLineStyle = style
		}
	}

	// Parse format strings into segments
	leftSegs := s.parseFormat(b.Settings["statusformatl"].(string), statusLineStyle)
	rightSegs := s.parseFormat(b.Settings["statusformatr"].(string), statusLineStyle)

	// Calculate text widths (color info doesn't affect width)
	leftLen := 0
	for _, seg := range leftSegs {
		leftLen += util.StringWidth(seg.text, util.CharacterCount(seg.text), 1)
	}
	rightLen := 0
	for _, seg := range rightSegs {
		rightLen += util.StringWidth(seg.text, util.CharacterCount(seg.text), 1)
	}

	// Use inactive color for inactive windows (ignore segment colors)
	activeStyle := statusLineStyle
	// Determine normal style for middle gap filler
	normalStyle := lookupColor("normal", statusLineStyle)

	isActive := s.win.IsActive()
	left := segReader{segs: leftSegs}
	right := segReader{segs: rightSegs}

	for x := 0; x < s.win.Width; x++ {
		if x < leftLen {
			x = renderSeg(&left, isActive, activeStyle, winX, y, x)
		} else if x >= s.win.Width-rightLen && x < rightLen+s.win.Width-rightLen {
			x = renderSeg(&right, isActive, activeStyle, winX, y, x)
		} else {
			// Middle gap filler - force normalStyle style
			gapStyle := normalStyle
			if !isActive {
				gapStyle = activeStyle // inactive windows still use inactive style
			}
			screen.SetContent(winX+x, y, ' ', nil, gapStyle)
		}
	}
}