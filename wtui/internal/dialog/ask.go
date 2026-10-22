package dialog

import tea "charm.land/bubbletea/v2"

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

// View is the question, each line wrapped to the popup, then a blank line and
// the keys that answer it (see prompt): as much of the question as leaves
// room for those.
func (a ask) View() tea.View {
	var lines []string
	for _, l := range a.question {
		lines = append(lines, wrapTo(a.width, l, "")...)
	}
	return prompt(a.width, a.height, lines, []string{"y: " + a.yes + "   n/q/esc: cancel"})
}
