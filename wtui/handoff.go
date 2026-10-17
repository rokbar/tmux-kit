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
