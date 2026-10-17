package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
)

// tmuxRun runs tmux on the sandbox's server and returns what it printed; the
// test fails if tmux does.
func tmuxRun(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// inTmux starts sessions on the sandbox's tmux server, attaches a tmux client
// to the first (see attach), and has wtui's tmux calls go to that server, as
// if wtui ran in it. It returns the client's name.
func inTmux(t *testing.T, s *sandbox.Sandbox, sessions ...string) string {
	t.Helper()
	s.NeedTmux()
	for _, name := range sessions {
		tmuxRun(t, "new-session", "-d", "-s", name, "-x", "80", "-y", "24")
	}
	client := attach(t, "term", sessions[0])
	t.Setenv("TMUX", tmuxRun(t, "display-message", "-p", "#{socket_path},#{pid},0"))
	return client
}

// attach attaches a tmux client to session from the pane of a new session,
// term, which stands in for a terminal, and returns the client's name.
func attach(t *testing.T, term, session string) string {
	t.Helper()
	tmuxRun(t, "new-session", "-d", "-s", term, "-x", "100", "-y", "30",
		"env", "-u", "TMUX", "tmux", "attach", "-t", "="+session)
	tty := tmuxRun(t, "display-message", "-p", "-t", "="+term+":", "#{pane_tty}")
	return waitFor(t, "a tmux client in "+term, func() string {
		for _, line := range strings.Split(tmuxRun(t, "list-clients", "-F", "#{client_tty} #{client_name}"), "\n") {
			if name, ok := strings.CutPrefix(line, tty+" "); ok {
				return name
			}
		}
		return ""
	})
}

// waitFor polls got every 0.1 s until it isn't empty, and returns it. After
// 10 s, it fails the test, naming what.
func waitFor(t *testing.T, what string, got func() string) string {
	t.Helper()
	for range 100 {
		if v := got(); v != "" {
			return v
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("timed out waiting for " + what)
	return ""
}

// The picker takes the client wtui runs in to another session by switching
// it, as here: then the client has moved.
func TestWatchSaysTheClientMovedOnceItsOnAnotherSession(t *testing.T) {
	s := sandbox.New(t)
	client := inTmux(t, s, "api", "fresh")
	moved := tmuxClient{}.Watch()
	if moved() {
		t.Fatal("the client moved before anything switched it")
	}
	tmuxRun(t, "switch-client", "-c", client, "-t", "=fresh")
	if !moved() {
		t.Error("the client switched to another session, but Watch says it didn't move")
	}
}

// With several clients, the one tmux takes wtui's to be (as it does for wt's
// switch-client) is the one keys were last typed in: here, ours. Another
// client switching session isn't ours moving; ours switching is.
func TestWatchKeepsToTheClientWtuiRunsIn(t *testing.T) {
	s := sandbox.New(t)
	other := inTmux(t, s, "elsewhere", "api", "fresh")
	ours := attach(t, "term2", "api")
	tmuxRun(t, "send-keys", "-t", "=term2:", "x")
	waitFor(t, "tmux to take "+ours+" for wtui's client", func() string {
		if tmuxRun(t, "display-message", "-p", "#{client_name}") == ours {
			return ours
		}
		return ""
	})

	moved := tmuxClient{}.Watch()
	tmuxRun(t, "switch-client", "-c", other, "-t", "=fresh")
	if moved() {
		t.Error("another client switched session, but Watch says ours moved")
	}
	tmuxRun(t, "switch-client", "-c", ours, "-t", "=fresh")
	if !moved() {
		t.Error("our client switched to another session, but Watch says it didn't move")
	}
}

// The picker can switch the client to the session it's already on (e.g. it
// opened that worktree). Nor is a session renamed another session.
func TestWatchSaysTheClientStayedOnItsSessionEvenRenamed(t *testing.T) {
	s := sandbox.New(t)
	client := inTmux(t, s, "api", "fresh")
	moved := tmuxClient{}.Watch()
	tmuxRun(t, "switch-client", "-c", client, "-t", "=fresh")
	tmuxRun(t, "switch-client", "-c", client, "-t", "=api")
	tmuxRun(t, "rename-session", "-t", "=api", "renamed")
	if moved() {
		t.Error("the client is back on its session, renamed, but Watch says it moved")
	}
}

// Outside tmux, wtui has no client for the picker to move. A tmux server may
// still be running, but its clients aren't wtui's terminal, so wtui doesn't
// ask it about them: here, a tmux whose one client is on another session
// each time it's asked.
func TestOutsideTmuxTheClientNeverMoves(t *testing.T) {
	s := sandbox.New(t) // TMUX unset
	asked := s.Path("asked")
	s.Script("stubs/tmux", `case "$1" in
display-message) echo /dev/pts/9 ;;
list-clients) echo x >>'`+asked+`'; echo "/dev/pts/9 \$$(wc -l <'`+asked+`' | tr -d ' ')" ;;
esac`)
	t.Setenv("PATH", s.Path("stubs")+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMUX", "/stub,1,0")
	if !(tmuxClient{}).Watch()() {
		t.Fatal("inside tmux, Watch doesn't see the stub's client move: the stub isn't one")
	}

	t.Setenv("TMUX", "")
	if (tmuxClient{}).Watch()() {
		t.Error("outside tmux, Watch says the client moved")
	}
}

// If tmux can't say which session the client is on (there's no server at
// $TMUX, or it has gone since), wtui doesn't take it as having moved: it
// stays, and the user can quit.
func TestWhenTmuxCantSayTheClientNeverMoves(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	t.Setenv("TMUX", s.Path("no-server")+",1,0")
	if (tmuxClient{}).Watch()() {
		t.Error("with no tmux server, Watch says the client moved")
	}

	t.Setenv("TMUX", "") // tmux follows $TMUX to a server
	inTmux(t, s, "api")
	moved := tmuxClient{}.Watch()
	tmuxRun(t, "kill-server")
	if moved() {
		t.Error("with the tmux server gone, Watch says the client moved")
	}
}
