package tmux_test

import (
	"strings"
	"testing"

	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// sessions are the server's sessions, each with its windows' names, as
// "name: window window", one per line.
func sessions(t *testing.T) string {
	t.Helper()
	var lines []string
	for _, name := range strings.Split(tmuxRun(t, "list-sessions", "-F", "#{session_name}"), "\n") {
		lines = append(lines, name+": "+strings.ReplaceAll(tmuxRun(t, "list-windows", "-t", "="+name+":", "-F", "#{window_name}"), "\n", " "))
	}
	return strings.Join(lines, "\n")
}

// window is the id of session name's window at index.
func window(t *testing.T, name, index string) string {
	t.Helper()
	return tmuxRun(t, "display-message", "-p", "-t", "="+name+":"+index, "#{window_id}")
}

// Killing a session ends it, its windows with it; the others stay.
func TestKillSession(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "my api", "-n", "shell")
	tmuxRun(t, "new-window", "-d", "-t", "=my api:", "-n", "dev")
	tmuxRun(t, "new-session", "-d", "-s", "beta", "-n", "vim")
	c := &tmux.Client{}
	if err := c.KillSession(id(t, "my api")); err != nil {
		t.Fatal(err)
	}
	if got := sessions(t); got != "beta: vim" {
		t.Errorf("after killing my api, the sessions are:\n%s\nwant beta alone", got)
	}
	if err := c.KillSession("$999"); err == nil || !strings.Contains(err.Error(), "$999") {
		t.Errorf("killing a session that's gone: %v, want tmux's error", err)
	}
}

// Killing a window ends that window of its session; its session's other
// windows, and other sessions, stay. Killing a session's last window ends
// the session too.
func TestKillWindow(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "my api", "-n", "shell")
	tmuxRun(t, "new-window", "-d", "-t", "=my api:", "-n", "dev")
	tmuxRun(t, "new-session", "-d", "-s", "beta", "-n", "vim")
	c := &tmux.Client{}
	api, shell, dev := id(t, "my api"), window(t, "my api", "0"), window(t, "my api", "1")
	if err := c.KillWindow(api, dev); err != nil {
		t.Fatal(err)
	}
	if got, want := sessions(t), "beta: vim\nmy api: shell"; got != want {
		t.Errorf("after killing my api's dev, the sessions are:\n%s\nwant:\n%s", got, want)
	}
	if err := c.KillWindow(api, shell); err != nil {
		t.Fatal(err)
	}
	if got := sessions(t); got != "beta: vim" {
		t.Errorf("after killing my api's last window, the sessions are:\n%s\nwant beta alone", got)
	}
	if err := c.KillWindow(id(t, "beta"), dev); err == nil || !strings.Contains(err.Error(), dev) {
		t.Errorf("killing a window that's gone: %v, want tmux's error", err)
	}
}

// Without a session (or window) to kill, nothing is killed: tmux would
// take an empty target for the current session, or its current window.
func TestKillNeedsATarget(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "alpha", "-n", "shell")
	tmuxRun(t, "new-window", "-d", "-t", "=alpha:", "-n", "dev")
	c := &tmux.Client{}
	if err := c.KillSession(""); err == nil {
		t.Error("KillSession(\"\") works")
	}
	if err := c.KillWindow(id(t, "alpha"), ""); err == nil {
		t.Error("KillWindow(alpha, \"\") works")
	}
	if err := c.KillWindow("", window(t, "alpha", "1")); err == nil {
		t.Error("KillWindow(\"\", dev) works")
	}
	if got := sessions(t); got != "alpha: shell dev" {
		t.Errorf("the sessions are:\n%s\nwant alpha with both its windows", got)
	}
}
