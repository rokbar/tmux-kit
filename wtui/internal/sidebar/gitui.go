package sidebar

import (
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
	"github.com/rokbar/tmux-kit/wtui/internal/tree"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// gitFolderOf is the folder of the worktree or main clone row n is in, for
// the git UI. A missing worktree has none, and nor does a row in no
// worktree wt lists (e.g. in a non-git project).
func gitFolderOf(n *tree.Node) (string, hint) {
	w := worktreeOf(n)
	switch {
	case w == nil:
		return "", hint(sessionOf(n).Name + " isn't in a git repo")
	case w.State == wt.Missing:
		return "", isMissing(w)
	}
	return w.Path, ""
}

// gitUI runs the git UI in folder over the right side, whichever session
// shows there. From tmux 3.7, it floats over that session's window, with
// focus in it: unlike a popup, the wtui key works there, and it stays with
// the window. On an older tmux, with no session on the right, or if
// floating fails, it pops up over the right side instead, with focus there
// until it has quit. Either way, focus comes back to the sidebar once the
// git UI quits.
func (m Model) gitUI(folder string) (Model, tea.Cmd) {
	normal, host, w, active := m.c.Normal, m.c.Host, m.c.Wt, m.active
	return m, act(func() error {
		command, err := w.GitUICommand(folder)
		if err != nil {
			return err
		}
		if v, err := normal.Version(); err == nil && active != "" && !tmux.Older(v, tmux.FloatingVersion) &&
			normal.Float(folder, host.ThenFocusSidebar(command)) == nil {
			return host.FocusRight()
		}
		if err := host.FocusRight(); err != nil {
			return err
		}
		return errors.Join(normal.Popup(folder, command), host.FocusSidebar())
	})
}
