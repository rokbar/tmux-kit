package tmux_test

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// pinned is the nested client of a server of its own, on session mine, pinned
// to that server, and a way to run tmux there. That server's socket is in the
// sandbox's folder pinned; TMUX_TMPDIR then points at the sandbox's own
// server, which has a session other. It never points at a folder that isn't
// there: tmux would look in /tmp, for the user's real server.
func pinned(t *testing.T, s *sandbox.Sandbox) (nested *tmux.Client, on func(args ...string) string) {
	t.Helper()
	if err := os.MkdirAll(s.Path("pinned"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX_TMPDIR", s.Path("pinned"))
	tmuxRun(t, "new-session", "-d", "-s", "mine")
	socket := tmuxRun(t, "display-message", "-p", "#{socket_path}")
	// before the sandbox removes its folder, with the socket still there
	t.Cleanup(func() { exec.Command("tmux", "-S", socket, "kill-server").Run() })
	nested = nestedClient(t, "mine")
	if err := nested.Pin(); err != nil {
		t.Fatalf("Pin: %v", err)
	}
	t.Setenv("TMUX_TMPDIR", s.Dir)
	tmuxRun(t, "new-session", "-d", "-s", "other")
	return nested, func(args ...string) string { return tmuxRun(t, append([]string{"-S", socket}, args...)...) }
}

// controlClients are the sessions control-mode clients are on, on the server
// run reaches.
func controlClients(run func(args ...string) string) []string {
	var on []string
	for _, line := range strings.Split(run("list-clients", "-F", "#{client_control_mode} #{session_name}"), "\n") {
		if session, ok := strings.CutPrefix(line, "1 "); ok {
			on = append(on, session)
		}
	}
	return on
}

// Pinned, a client reaches the normal server it reached then, by its socket,
// wherever TMUX_TMPDIR points since: here, at another server's folder. So does
// its Watch's control client.
func TestAPinnedClientKeepsToItsServer(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	nested, on := pinned(t, s)

	snap, err := nested.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, sess := range snap.Sessions {
		names = append(names, sess.Name)
	}
	if want := []string{"mine", "term"}; !slices.Equal(names, want) {
		t.Errorf("the pinned client's sessions are %q, want its server's %q", names, want)
	}
	mine := on("display-message", "-p", "-t", "=mine:", "#{session_id}")
	if got, err := nested.ActiveSession(); err != nil || got != mine {
		t.Errorf("ActiveSession() = %q, %v; want mine's id, %s", got, err, mine)
	}

	w := watch(t, nested)
	next(t, w, "mine to be active", func(e tmux.ActiveSessionChanged) bool { return e.Session == mine })
	waitFor(t, "the control client to be on mine", func() bool {
		return slices.Equal(controlClients(on), []string{"mine"})
	})
	if got := controlClients(func(args ...string) string { return tmuxRun(t, args...) }); len(got) > 0 {
		t.Errorf("the other server has control clients on %q", got)
	}
}

// Once its server's socket has gone (here, with its folder), a pinned client
// finds no server, as with none running, rather than the one TMUX_TMPDIR
// points at: it lists no sessions there and changes nothing there. Its Watch
// says no session is active, and attaches nowhere.
func TestAPinnedClientFindsNoServerOnceItsSocketHasGone(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	nested, on := pinned(t, s)
	w := watch(t, nested)
	next(t, w, "a session to be active", func(e tmux.ActiveSessionChanged) bool { return e.Session != "" })
	otherPane := tmuxRun(t, "display-message", "-p", "-t", "=other:0.0", "#{pane_id}")
	otherWindow := tmuxRun(t, "display-message", "-p", "-t", "=other:0", "#{window_id}")
	tmuxRun(t, "set-option", "-p", "-t", otherPane, "@agent_state", "done")
	tmuxRun(t, "set-environment", "-g", "TMUX_KIT_EDITOR", "vim")

	on("kill-server")
	if err := os.RemoveAll(s.Path("pinned")); err != nil {
		t.Fatal(err)
	}
	next(t, w, "no session to be active", func(e tmux.ActiveSessionChanged) bool { return e.Session == "" })

	if got, err := nested.Snapshot(); err != nil || len(got.Sessions) != 0 {
		t.Errorf("Snapshot() = %+v, %v; want no sessions", got, err)
	}
	if got, err := nested.ActiveSession(); err != nil || got != "" {
		t.Errorf("ActiveSession() = %q, %v; want none", got, err)
	}
	if got, err := nested.Editor(); err == nil {
		t.Errorf("Editor() = %q, want tmux's error", got)
	}
	if err := nested.ClearDone(otherWindow); err == nil {
		t.Error("ClearDone() worked, want tmux's error")
	}
	if got := tmuxRun(t, "show-options", "-pqv", "-t", otherPane, "@agent_state"); got != "done" {
		t.Errorf("the other server's done is now %q", got)
	}
	time.Sleep(1100 * time.Millisecond) // the Watch looks again every 0.5 s
	if got := controlClients(func(args ...string) string { return tmuxRun(t, args...) }); len(got) > 0 {
		t.Errorf("the other server has control clients on %q", got)
	}
	if _, err := os.Stat(s.Path("pinned")); err == nil {
		t.Error("the pinned server's folder is back")
	}
}
