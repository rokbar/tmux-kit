// Package sidebar is the sidebar's model: the Bubble Tea program on the left
// of a host. For now it's a placeholder that names the active session and
// lists the keys that work; the tree comes later.
package sidebar

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Host is what the sidebar asks of the host it runs in.
type Host interface {
	// FocusRight moves focus to the right side.
	FocusRight() error
	// PrefixRight moves focus to the right side and passes tmux's prefix on.
	PrefixRight() error
}

// Normal is what the sidebar asks of the normal tmux server.
type Normal interface {
	// ActiveSession is the session the right side shows, "" for none.
	ActiveSession() (string, error)
}

// poll is how often the sidebar asks which session is active.
const poll = time.Second

// Model is the sidebar.
type Model struct {
	host   Host
	normal Normal
	key    string // the wtui key, as tmux names it
	active string
	status string // the last failure, if any
	width  int
}

// New is the sidebar of host, over the normal server, where key is the wtui
// key.
func New(host Host, normal Normal, key string) Model {
	return Model{host: host, normal: normal, key: key}
}

type activeRead struct {
	name string
	err  error
}
type pollDue struct{}
type hostDone struct{ err error }

func (m Model) Init() tea.Cmd { return m.readActive }

func (m Model) readActive() tea.Msg {
	name, err := m.normal.ActiveSession()
	return activeRead{name, err}
}

// ask runs a host request in the background.
func ask(f func() error) tea.Cmd { return func() tea.Msg { return hostDone{f()} } }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "esc":
			return m, ask(m.host.FocusRight)
		case "ctrl+b":
			return m, ask(m.host.PrefixRight)
		}
	case activeRead:
		m.active, m.status = msg.name, ""
		if msg.err != nil {
			m.status = msg.err.Error()
		}
		return m, tea.Tick(poll, func(time.Time) tea.Msg { return pollDue{} })
	case pollDue:
		return m, m.readActive
	case hostDone:
		m.status = ""
		if msg.err != nil {
			m.status = msg.err.Error()
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	active := m.active
	if active == "" {
		active = "no session"
	}
	lines := []string{"wtui", "", "on the right: " + active, ""}
	keys := [][2]string{
		{m.key + ", esc", "to the right"},
		{"C-b", "tmux's prefix there"},
		{"q", "close wtui"},
	}
	w := 0
	for _, k := range keys {
		w = max(w, ansi.StringWidth(k[0]))
	}
	for _, k := range keys {
		lines = append(lines, k[0]+strings.Repeat(" ", w-ansi.StringWidth(k[0])+2)+k[1])
	}
	if m.status != "" {
		lines = append(lines, "", m.status)
	}
	if m.width > 0 {
		for i, l := range lines {
			lines[i] = ansi.Truncate(l, m.width, "…")
		}
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}
