package action

var notepanedefaults = map[string]string{
	"Alt-Enter": "NotePaneSend",
	"Alt-i":     "NotePaneSwitchReceiver",
}

var termdefaults = map[string]string{
	"<Ctrl-q><Ctrl-q>": "Exit",
	"<Ctrl-e><Ctrl-e>": "CommandMode",
	"<Ctrl-w><Ctrl-w>": "NextSplit|FirstSplit",
}

// DefaultBindings returns a map containing micro's default keybindings
func DefaultBindings(pane string) map[string]string {
	switch pane {
	case "command":
		return infodefaults
	case "buffer":
		return bufdefaults
	case "terminal":
		return termdefaults
	case "notepane":
		return notepanedefaults
	default:
		return map[string]string{}
	}
}
