package sidebar

import (
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/host"
)

// Keys are every sidebar key, for the host to bind: after the wtui key, and
// those that are held (see action) also while the sidebar has focus.
func Keys() []host.Key {
	var keys []host.Key
	for _, a := range actions {
		for _, k := range a.keys {
			keys = append(keys, host.Key{Tmux: tmuxKey(k), Name: k, Held: a.held, Repeats: a.repeats})
		}
	}
	return keys
}

// tmuxKeys are the keys tmux names differently from Bubble Tea, by Bubble
// Tea's name.
var tmuxKeys = map[string]string{
	"enter": "Enter", "esc": "Escape", "space": "Space", "tab": "Tab", "backspace": "BSpace",
	"up": "Up", "down": "Down", "left": "Left", "right": "Right",
	"home": "Home", "end": "End", "pgup": "PPage", "pgdown": "NPage", "insert": "IC", "delete": "DC",
	"f1": "F1", "f2": "F2", "f3": "F3", "f4": "F4", "f5": "F5", "f6": "F6",
	"f7": "F7", "f8": "F8", "f9": "F9", "f10": "F10", "f11": "F11", "f12": "F12",
	";": `\;`, // alone, a ; separates tmux commands
}

// tmuxMods are Bubble Tea's modifiers, as tmux names them.
var tmuxMods = map[string]string{"ctrl": "C-", "alt": "M-", "shift": "S-"}

// tmuxKey is key, as Bubble Tea names it (ctrl+b, enter, ], ...), as tmux
// does (C-b, Enter, ], ...), or "" for one it has no name for.
func tmuxKey(key string) string {
	if key == "shift+tab" {
		return "BTab"
	}
	parts := strings.Split(key, "+")
	var name strings.Builder
	for _, mod := range parts[:len(parts)-1] {
		if tmuxMods[mod] == "" {
			return ""
		}
		name.WriteString(tmuxMods[mod])
	}
	k := parts[len(parts)-1]
	switch {
	case tmuxKeys[k] != "":
		name.WriteString(tmuxKeys[k])
	case utf8.RuneCountInString(k) == 1:
		name.WriteString(k)
	default:
		return ""
	}
	return name.String()
}

// Press is a key the host has the sidebar press, by its Name in Keys. Done
// closes once the key has done all it does: for a jump, once the session is
// shown and focus is on the right.
type Press struct {
	Key  string
	Done chan<- struct{}
}

// pressed is what a Press's command gave, msg, and that Press's Done, which
// closes once the sidebar has taken msg in.
type pressed struct {
	msg  tea.Msg
	done chan<- struct{}
}

// fromHost is the sidebar after a Press, or a pressed.
func (m Model) fromHost(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case Press:
		m, cmd := m.press(msg.Key)
		if cmd == nil {
			close(msg.Done)
			return m, nil
		}
		return m, func() tea.Msg { return pressed{cmd(), msg.Done} }
	case pressed:
		defer close(msg.done)
		if batch, ok := msg.msg.(tea.BatchMsg); ok {
			// a batch's commands run in the background, so the key is done
			// without them
			return m, tea.Batch(batch...)
		}
		return m.update(msg.msg)
	}
	return m, nil
}
