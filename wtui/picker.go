package main

import (
	tea "charm.land/bubbletea/v2"
)

// clientWatcher is what n needs to tell whether the picker took the tmux
// client wtui runs in to another session: a tmuxClient, or a fake in tests.
type clientWatcher interface {
	// Watch notes the session the client is on, and returns a func that
	// says whether it's on another one now. Outside tmux, it never is.
	Watch() (moved func() bool)
}

// picked is the picker having closed, and whether it failed (err) or took
// the tmux client wtui runs in to another session (moved).
type picked struct {
	err   error
	moved bool
}

// pick runs the picker in the selected worktree's folder, so wt takes it as
// the invoking worktree, with wtui paused while the picker has the terminal.
// A missing worktree has no folder to run it in.
func (m model) pick() tea.Cmd {
	_, w, ok := m.selectionThere()
	if !ok {
		return nil
	}
	client, watcher := m.client, m.watcher
	return func() tea.Msg {
		moved := watcher.Watch() // before the picker can take the client anywhere
		// the picker's own command only asks tea to pause wtui and run it:
		// its message does that once it gets to the program
		return client.Pick(w.Path, func(err error) tea.Msg {
			return picked{err: err, moved: moved()}
		})()
	}
}

// withPicked takes in the picker having closed. If it worked and took the
// tmux client to another session, it opened something there, and wtui
// quits. Otherwise (it was cancelled, opened the session the client was
// already on, or failed) wtui resumes the list (resumeAfter): the picker may
// have made a worktree or started a session, and if it failed, the status
// line says what wt said.
func (m model) withPicked(p picked) (model, tea.Cmd) {
	if p.err == nil && p.moved {
		return m, tea.Quit
	}
	return m.resumeAfter(p.err)
}
