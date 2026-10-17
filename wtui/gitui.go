package main

import (
	tea "charm.land/bubbletea/v2"
)

// gitUIClosed is the git UI having exited, and whether wt or the git UI
// failed (err). wtui then resumes the list (resumeAfter): what was done in
// the git UI (a commit, a branch switched) changes what the rows and the pane
// show.
type gitUIClosed struct{ err error }

// runGitUI runs the git UI in the selected worktree's folder, the main
// clone's too, with wtui paused while the git UI has the terminal. A missing
// worktree has no folder to run it in.
func (m model) runGitUI() tea.Cmd {
	r, w, ok := m.selectionThere()
	if !ok {
		return nil
	}
	client := m.client
	return func() tea.Msg {
		// the git UI's own command only asks tea to pause wtui and run it:
		// its message does that once it gets to the program
		return client.GitUI(r.dir, w.Path, func(err error) tea.Msg { return gitUIClosed{err} })()
	}
}
