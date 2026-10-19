package tmux_test

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
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

func TestOlder(t *testing.T) {
	for _, c := range []struct {
		v    string
		want bool
	}{
		{"3.5a", true},
		{"3.4", true},
		{"2.9", true},
		{"3.6", false},
		{"3.6a", false},
		{"3.7c", false},
		{"3.10", false},
		{"4.0", false},
		{"next-3.7", false},
		{"3.6-rc", false},
		{"next-3.5", true},
		// builds that follow tmux's development
		{"master", false},
		{"openbsd-7.6", false},
	} {
		if got := tmux.Older(c.v, "3.6"); got != c.want {
			t.Errorf("Older(%q, 3.6) = %v, want %v", c.v, got, c.want)
		}
	}
}

// With no server running, the version is that of the tmux on PATH, which
// would start one; then the server's.
func TestVersion(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	want := strings.TrimPrefix(tmuxRun(t, "-V"), "tmux ")
	c := &tmux.Client{}
	if got, err := c.Version(); err != nil || got != want {
		t.Errorf("with no server, Version() = %q, %v; want %q", got, err, want)
	}
	tmuxRun(t, "new-session", "-d", "-s", "a")
	if got, err := c.Version(); err != nil || got != want {
		t.Errorf("Version() = %q, %v; want %q", got, err, want)
	}
}

func TestKeyIsTheWtuiKeyOption(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	c := &tmux.Client{}
	if got := c.Key(); got != `C-\` {
		t.Errorf("with no server, Key() = %q, want C-\\", got)
	}
	tmuxRun(t, "new-session", "-d", "-s", "a")
	if got := c.Key(); got != `C-\` {
		t.Errorf("with @wtui-key unset, Key() = %q, want C-\\", got)
	}
	tmuxRun(t, "set-option", "-g", "@wtui-key", "C-]")
	if got := c.Key(); got != "C-]" {
		t.Errorf("with @wtui-key C-], Key() = %q", got)
	}
}

func TestSessions(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	before := time.Now().Add(-time.Second)
	tmuxRun(t, "new-session", "-d", "-s", "my api")
	tmuxRun(t, "new-session", "-d", "-s", "beta")
	got, err := (&tmux.Client{}).Sessions()
	if err != nil {
		t.Fatal(err)
	}
	ids := strings.Fields(tmuxRun(t, "list-sessions", "-F", "#{session_id}"))
	if len(got) != 2 || got[0].Name != "beta" || got[1].Name != "my api" {
		t.Fatalf("Sessions() = %+v, want beta and my api, by name", got)
	}
	for i, sess := range got {
		if sess.ID != ids[i] {
			t.Errorf("%s's id is %q, want %q", sess.Name, sess.ID, ids[i])
		}
		if sess.Activity.Before(before) || sess.Activity.After(time.Now()) {
			t.Errorf("%s's activity is %v, want about now", sess.Name, sess.Activity)
		}
	}
}

// In a host's pane, TMUX names the host; the client still reaches the
// normal server.
func TestReachesTheNormalServerFromInsideAHost(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "a")
	t.Setenv("TMUX", s.Path("host/socket")+",1,0")
	t.Setenv("TMUX_PANE", "%1")
	got, err := (&tmux.Client{}).Sessions()
	if err != nil || len(got) != 1 || got[0].Name != "a" {
		t.Errorf("Sessions() = %+v, %v; want the normal server's a", got, err)
	}
}

// The active session is the one the nested client, by its tty, shows.
func TestActiveSessionIsTheNestedClients(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "my api")
	tmuxRun(t, "new-session", "-d", "-s", "beta")
	// a client attached from the pane of a session standing in for a terminal
	tmuxRun(t, "new-session", "-d", "-s", "term", "-x", "100", "-y", "30",
		"env", "-u", "TMUX", "tmux", "attach", "-t", "=my api")
	tty := tmuxRun(t, "display-message", "-p", "-t", "=term:", "#{pane_tty}")
	nested := &tmux.Client{Nested: tty}
	var got string
	for range 100 {
		if got, _ = nested.ActiveSession(); got != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if got != "my api" {
		t.Errorf("ActiveSession() = %q, want my api", got)
	}
	if got, err := (&tmux.Client{Nested: "/dev/ttys999"}).ActiveSession(); err != nil || got != "" {
		t.Errorf("with no client on its tty, ActiveSession() = %q, %v; want none", got, err)
	}
}
