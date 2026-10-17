package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// handOff takes the terminal to session once wtui has quit and the terminal
// is restored. Inside tmux, it switches the client wtui runs in to session.
// Outside tmux, wtui becomes `tmux attach` to it, as wt does.
func handOff(session string) error {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		return err
	}
	target := "=" + session // exactly that name: not a prefix or pattern of another
	if os.Getenv("TMUX") != "" {
		out, err := exec.Command(tmux, "switch-client", "-t", target).CombinedOutput()
		if msg := strings.TrimSpace(string(out)); err != nil && msg != "" {
			err = errors.New(msg) // tmux's reason, e.g. "no current client"
		}
		if err != nil {
			return fmt.Errorf("can't switch to session %s: %w", session, err)
		}
		return nil
	}
	return syscall.Exec(tmux, []string{"tmux", "attach", "-t", target}, os.Environ())
}

// tmuxClient is the tmux client wtui runs in, if it runs in tmux.
type tmuxClient struct{}

// Watch notes the session the tmux client wtui runs in is on, and returns a
// func that says whether that client is on another session now. The client
// is the one tmux takes wtui's to be, as it does for wt's switch-client, and
// it's pinned by name: tmux guesses from recent activity, which switching
// changes. Sessions are compared by id, so a renamed one is the same.
// Outside tmux, or when tmux can't say, the client never moved.
func (tmuxClient) Watch() (moved func() bool) {
	never := func() bool { return false }
	if os.Getenv("TMUX") == "" {
		return never
	}
	client, err := tmuxOutput("display-message", "-p", "#{client_name}")
	if err != nil || client == "" {
		return never
	}
	before, ok := sessionOf(client)
	if !ok {
		return never
	}
	return func() bool {
		now, ok := sessionOf(client)
		return ok && now != before
	}
}

// sessionOf is the id of the session the tmux client named client is on; ok
// is false if tmux can't say.
func sessionOf(client string) (id string, ok bool) {
	// space-separated: tmux turns a tab into _ without a UTF-8 locale. Ids
	// ($1) have no spaces.
	out, err := tmuxOutput("list-clients", "-F", "#{client_name} #{session_id}")
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(out, "\n") {
		if i := strings.LastIndexByte(line, ' '); i >= 0 && line[:i] == client {
			return line[i+1:], true
		}
	}
	return "", false
}

// tmuxOutput is what tmux printed for args, trimmed.
func tmuxOutput(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).Output()
	return strings.TrimSpace(string(out)), err
}
