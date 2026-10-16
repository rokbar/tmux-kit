package main

import tea "charm.land/bubbletea/v2"

// model is a placeholder screen until wtui lists the repo's worktrees.
type model struct{}

func newModel() model { return model{} }

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() tea.View {
	return tea.NewView("wtui: worktree dashboard for wt (nothing to show yet)\n\nq/esc: quit\n")
}
