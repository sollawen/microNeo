package action

import "github.com/micro-editor/micro/v2/internal/screen"

// MessageNow sets a message and synchronously flushes the InfoBar line to the terminal.
// Used during synchronous blocking on the main goroutine when immediate feedback is needed
// (e.g., SelectDialog synchronous loading).
// Must be called from the main goroutine.
func (h *InfoPane) MessageNow(msg string) {
	screen.Screen.HideCursor()
	h.Message(msg)
	h.Display()
	screen.Screen.Show()
}
