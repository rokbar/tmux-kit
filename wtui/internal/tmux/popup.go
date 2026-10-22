package tmux

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

// Popup is a program to run in a popup.
type Popup struct {
	// Title is what its border says.
	Title string
	// Dir is the folder it runs in; "" for the server's choice.
	Dir string
	// Command is the program and its arguments, run as they are: no shell
	// reads them, and tmux expands no formats in them.
	Command []string
	// Env has NAME=value pairs added to the environment it runs in, which
	// is the server's global environment, not wtui's.
	Env []string
}

// ClientEnv is the variable a popup on the nested client names that client
// in. wt and t switch the client it names, when it's set: tmux's own pick, for
// a program in a popup, is the client used last, which may be another
// terminal's, or the Watch's control client.
const ClientEnv = "TMUX_KIT_CLIENT"

// Popup opens p in a popup on the nested client, covering all of it (the
// right side, its status line too), and returns once the popup has closed:
// once its program has exited, whatever its exit status. The program learns
// which client it's on from ClientEnv. A popup takes every key typed into its
// client until it closes; one open already on the nested client stays, and
// Popup returns at once.
func (c *Client) Popup(p Popup) error {
	if c.Nested == "" {
		return errors.New("no nested client to open a popup on")
	}
	p.Env = append([]string{ClientEnv + "=" + c.Nested}, p.Env...)
	return OpenPopup(c.command, p, "-c", c.Nested, "-w", "100%", "-h", "100%")
}

// OpenPopup opens p in a popup on the server that command's tmux commands
// (made of their arguments) reach, placed and sized by where (the
// display-popup arguments that do it), and returns once its program has
// exited, whatever its exit status.
func OpenPopup(command func(args ...string) *exec.Cmd, p Popup, where ...string) error {
	args := append(append([]string{"display-popup", "-E"}, where...), "-T", p.Title)
	if p.Dir != "" {
		args = append(args, "-d", p.Dir)
	}
	for _, kv := range p.Env {
		args = append(args, "-e", kv)
	}
	cmd := command(append(append(args, "--"), p.Command...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	// display-popup exits with its program's status, which isn't tmux
	// failing: then it says nothing.
	if err := cmd.Run(); err != nil && strings.TrimSpace(stderr.String()) != "" {
		return errors.New(strings.TrimSpace(stderr.String()))
	}
	return nil
}
