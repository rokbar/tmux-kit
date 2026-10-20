package dialog

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Ask is a dialog that asks question, its lines, until it's answered: y
// answers yes, which does what yes says (e.g. kill), and n, q, esc and
// ctrl+c answer no. Either closes it. Any other key does nothing, so that a
// key pressed by mistake neither answers nor closes it. Yes says how it was
// answered.
func Ask(question []string, yes string) tea.Model { return ask{question: question, yes: yes} }

// Yes says whether m, an Ask, was answered yes.
func Yes(m tea.Model) bool {
	a, ok := m.(ask)
	return ok && a.answered
}

type ask struct {
	question      []string
	yes           string
	answered      bool // yes
	width, height int
}

func (a ask) Init() tea.Cmd { return nil }

func (a ask) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "y":
			a.answered = true
			return a, tea.Quit
		case "n", "q", "esc", "ctrl+c":
			return a, tea.Quit
		}
	}
	return a, nil
}

// View is the question, each line wrapped to the popup with a column either
// side, then a blank line and the keys that answer it: as much of the
// question as leaves room for those.
func (a ask) View() tea.View {
	var lines []string
	for _, l := range a.question {
		if a.width > 2 {
			l = ansi.Wrap(l, a.width-2, "")
		}
		lines = append(lines, strings.Split(l, "\n")...)
	}
	if a.height > 0 {
		lines = atMost(a.height-2, lines)
	}
	shown := make([]string, 0, len(lines)+2)
	for _, l := range lines {
		shown = append(shown, " "+oneLine(l, a.width-2))
	}
	shown = append(shown, "", " \x1b[2m"+oneLine("y: "+a.yes+"   n/q/esc: cancel", a.width-2)+"\x1b[0m")
	v := tea.NewView(strings.Join(shown, "\n"))
	v.AltScreen = true
	return v
}
