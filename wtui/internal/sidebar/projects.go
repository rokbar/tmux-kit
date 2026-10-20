package sidebar

import (
	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// openProject opens a project from the projects root, whatever the cursor's
// row: the open-project dialog, over the right side. It opens the session
// of the folder chosen and shows it on the nested client, whose popup it's
// in. While the right side shows no session, there's no nested client: the
// dialog is the host's, and only opens the session (-d), which the right
// side then shows by itself.
func (m Model) openProject() (Model, tea.Cmd) {
	root := m.c.ProjectsRoot
	p := tmux.Popup{Title: " open a project ", Dir: root, Command: []string{"project", root}}
	if m.right == "" {
		p.Command = []string{"project", "-d", root}
		return m.dialogIn(m.c.Host.PopupRight, p)
	}
	return m.dialog(p)
}
