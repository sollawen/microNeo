package action

import (
	"os"
	"path/filepath"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

// NotePane is a floating overlay pane for quick notes
type NotePane struct {
	isOpen   bool
	lines []string
	curLine  int
	curCol   int
	x, y     int
	width int
	height int
	noteFile string
	modified bool
}

// TheNotePane is the global NotePane instance
var TheNotePane *NotePane

// NewNotePane creates a new NotePane instance
func NewNotePane() *NotePane {
	n := &NotePane{
		lines:    []string{""},
		curLine:  0,
		curCol:   0,
		height:   5,
		modified: false,
	}

	// Set the notes file path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "/tmp"
	}
	n.noteFile = filepath.Join(homeDir, ".config", "microNeo", "notes.md")

	n.loadFile()
	return n
}

// Toggle opens or closes the NotePane
func (n *NotePane) Toggle() {
	if n.isOpen {
		n.close()
	} else {
		n.open()
	}
}

// IsOpen returns whether the NotePane is currently open
func (n *NotePane) IsOpen() bool {
	return n.isOpen
}

// open opens the NotePane and positions it below the cursor
func (n *NotePane) open() {
	// Get the current active BufPane
	pane := MainTab().CurPane()
	if pane == nil {
		return
	}

	// Get the pane's view (screen coordinates)
	view := pane.BWindow.GetView()

	// Get cursor position in buffer coordinates
	cursor := pane.Buf.GetActiveCursor()
	bufLoc := cursor.Loc

	// Get view's scroll offset to calculate screen position
	startLine := view.StartLine

	// Calculate cursor's screen position
	// Y position: cursor line relative to view start + view Y offset
	cursorScreenY := bufLoc.Y - startLine.Line + view.Y

	// Position the NotePane below the cursor
	n.x = view.X
	n.y = cursorScreenY + 1
	n.width = view.Width

	// If not enough space below, try to position above the cursor
	screenHeight, _ := screen.Screen.Size()
	if n.y+n.height+2 > screenHeight {
		n.y = cursorScreenY - n.height - 2
		if n.y < view.Y {
			n.y = view.Y
		}
	}

	n.isOpen = true
}

// close closes the NotePane and saves the file
func (n *NotePane) close() {
	if n.modified {
		n.saveFile()
	}
	n.isOpen = false
}

// loadFile loads the notes from the file
func (n *NotePane) loadFile() {
	content, err := os.ReadFile(n.noteFile)
	if err != nil {
		// File doesn't exist or can't be read, start with empty content
		n.lines = []string{""}
		return
	}

	// Parse content into lines
	contentStr := string(content)
	if len(contentStr) == 0 {
		n.lines = []string{""}
		return
	}

	// Split by newline, preserving empty lines
	n.lines = []string{}
	start := 0
	for i := 0; i < len(contentStr); i++ {
		if contentStr[i] == '\n' {
			n.lines = append(n.lines, contentStr[start:i])
			start = i + 1
		}
	}
	// Add last line if no trailing newline
	if start < len(contentStr) {
		n.lines = append(n.lines, contentStr[start:])
	}

	// Ensure at least one line
	if len(n.lines) == 0 {
		n.lines = []string{""}
	}

	n.modified = false
}

// saveFile saves the notes to the file
func (n *NotePane) saveFile() {
	// Ensure directory exists
	dir := filepath.Dir(n.noteFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		screen.TermMessage("Error creating directory:", err)
		return
	}

	// Join lines with newline
	content := ""
	for i, line := range n.lines {
		if i > 0 {
			content += "\n"
		}
		content += line
	}

	if err := os.WriteFile(n.noteFile, []byte(content), 0644); err != nil {
		screen.TermMessage("Error saving notes:", err)
		return
	}

	n.modified = false
}

// HandleEvent handles keyboard events for the NotePane
func (n *NotePane) HandleEvent(event tcell.Event) {
	switch e := event.(type) {
	case *tcell.EventKey:
		switch e.Key() {
		case tcell.KeyRune:
			// Insert character at current position
			if e.Modifiers() == 0 {
				r := e.Rune()
				line := n.lines[n.curLine]
				// Insert rune at curCol
				if n.curCol >= len(line) {
					n.lines[n.curLine] = line + string(r)
				} else {
					n.lines[n.curLine] = line[:n.curCol] + string(r) + line[n.curCol:]
				}
				n.curCol++
				n.modified = true
			}

		case tcell.KeyBackspace2:
			// Delete character before cursor
			if n.curCol > 0 {
				line := n.lines[n.curLine]
				n.lines[n.curLine] = line[:n.curCol-1] + line[n.curCol:]
				n.curCol--
				n.modified = true
			} else if n.curLine > 0 {
				// Merge with previous line
				prevLine := n.lines[n.curLine-1]
				currLine := n.lines[n.curLine]
				n.lines[n.curLine-1] = prevLine + currLine
				// Remove current line
				n.lines = append(n.lines[:n.curLine], n.lines[n.curLine+1:]...)
				n.curLine--
				n.curCol = len(prevLine)
				n.modified = true
			}

		case tcell.KeyEnter:
			// Split current line at cursor position
			line := n.lines[n.curLine]
			before := line[:n.curCol]
			after := line[n.curCol:]
			// Insert new line
			n.lines = append(n.lines[:n.curLine+1], n.lines[n.curLine:]...)
			n.lines[n.curLine] = before
			n.lines[n.curLine+1] = after
			n.curLine++
			n.curCol = 0
			n.modified = true

		case tcell.KeyUp:
			// Move up one line
			if n.curLine > 0 {
				n.curLine--
				// Clamp curCol to line length
				if n.curCol > len(n.lines[n.curLine]) {
					n.curCol = len(n.lines[n.curLine])
				}
			}

		case tcell.KeyDown:
			// Move down one line
			if n.curLine < len(n.lines)-1 {
				n.curLine++
				// Clamp curCol to line length
				if n.curCol > len(n.lines[n.curLine]) {
					n.curCol = len(n.lines[n.curLine])
				}
			}

		case tcell.KeyLeft:
			// Move left one column
			if n.curCol > 0 {
				n.curCol--
			} else if n.curLine > 0 {
				// Move to end of previous line
				n.curLine--
				n.curCol = len(n.lines[n.curLine])
			}

		case tcell.KeyRight:
			// Move right one column
			if n.curCol < len(n.lines[n.curLine]) {
				n.curCol++
			} else if n.curLine < len(n.lines)-1 {
				// Move to start of next line
				n.curLine++
				n.curCol = 0
			}

		case tcell.KeyHome:
			// Go to start of line
			n.curCol = 0

		case tcell.KeyEnd:
			// Go to end of line
			n.curCol = len(n.lines[n.curLine])

		default:
			// Consume all other events (don't propagate)
		}

	case *tcell.EventResize:
		// Reposition the NotePane on terminal resize
		if n.isOpen {
			n.open()
		}
	}
}

// Display renders the NotePane on the screen
func (n *NotePane) Display() {
	if !n.isOpen {
		return
	}

	// Box-drawing characters
	topLeft := '┌'
	topRight := '┐'
	bottomLeft := '└'
	bottomRight := '┘'
	horizontal := '─'
	vertical := '│'

	// Draw top border
	screen.Screen.SetContent(n.x, n.y, topLeft, nil, config.DefStyle)
	for i := 1; i < n.width-1; i++ {
		screen.Screen.SetContent(n.x+i, n.y, horizontal, nil, config.DefStyle)
	}
	screen.Screen.SetContent(n.x+n.width-1, n.y, topRight, nil, config.DefStyle)

	// Draw bottom border
	screen.Screen.SetContent(n.x, n.y+n.height+1, bottomLeft, nil, config.DefStyle)
	for i := 1; i < n.width-1; i++ {
		screen.Screen.SetContent(n.x+i, n.y+n.height+1, horizontal, nil, config.DefStyle)
	}
	screen.Screen.SetContent(n.x+n.width-1, n.y+n.height+1, bottomRight, nil, config.DefStyle)

	// Draw side borders and content
	for row := 0; row < n.height; row++ {
		screenY := n.y + 1 + row
		// Left border
		screen.Screen.SetContent(n.x, screenY, vertical, nil, config.DefStyle)
		// Right border
		screen.Screen.SetContent(n.x+n.width-1, screenY, vertical, nil, config.DefStyle)

		// Draw content line
		if row < len(n.lines) {
			line := n.lines[row]
			col := 0
			for _, r := range line {
				if col+1 < n.width-1 {
					screen.Screen.SetContent(n.x+1+col, screenY, r, nil, config.DefStyle)
				}
				col++
				// Stop if line is too long
				if col >= n.width-2 {
					break
				}
			}
			// Fill rest of line with spaces
			for ; col < n.width-2; col++ {
				screen.Screen.SetContent(n.x+1+col, screenY, ' ', nil, config.DefStyle)
			}
		} else {
			// Fill empty line with spaces
			for col := 0; col < n.width-2; col++ {
				screen.Screen.SetContent(n.x+1+col, screenY, ' ', nil, config.DefStyle)
			}
		}
	}

	// Show cursor
	cursorScreenX := n.x + 1 + n.curCol
	cursorScreenY := n.y + 1 + n.curLine
	if cursorScreenX < n.x+n.width-1 {
		screen.Screen.ShowCursor(cursorScreenX, cursorScreenY)
	}
}