package main

import (
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/dialog"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// runKill is the kill dialog: it asks question, and once the answer is yes,
// kills target on the normal server, which it reaches by socket. target is
// a session, or a session's window (session:window), by ids. Why tmux
// couldn't kill it is for the status line.
func runKill(socket, target, question string) error {
	session, window, isWindow := strings.Cut(target, ":")
	if session == "" || isWindow && window == "" {
		return errors.New("usage: wtui kill <session>[:<window>] <question>")
	}
	final, err := tea.NewProgram(dialog.Ask(strings.Split(question, "\n"), "kill")).Run()
	if err != nil || !dialog.Yes(final) {
		return err
	}
	normal := &tmux.Client{Socket: socket}
	if isWindow {
		err = normal.KillWindow(session, window)
	} else {
		err = normal.KillSession(session)
	}
	if err != nil {
		return dialog.Say(err.Error())
	}
	return nil
}
