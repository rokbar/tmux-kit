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

// The sidebar's width is the @wtui-width option, in columns: 30 while it
// isn't set, or isn't a number of columns.
func TestWidthIsTheWtuiWidthOption(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	c := &tmux.Client{}
	if got := c.Width(); got != 30 {
		t.Errorf("with no server, Width() = %d, want 30", got)
	}
	tmuxRun(t, "new-session", "-d", "-s", "a")
	if got := c.Width(); got != 30 {
		t.Errorf("with @wtui-width unset, Width() = %d, want 30", got)
	}
	tmuxRun(t, "set-option", "-g", "@wtui-width", "42")
	if got := c.Width(); got != 42 {
		t.Errorf("with @wtui-width 42, Width() = %d", got)
	}
	for _, bad := range []string{"wide", "0", "-5", "25%", "3.5"} {
		tmuxRun(t, "set-option", "-g", "@wtui-width", bad)
		if got := c.Width(); got != 30 {
			t.Errorf("with @wtui-width %s, Width() = %d, want 30", bad, got)
		}
	}
}

// The editor is the normal server's global TMUX_KIT_EDITOR, the one prefix e
// runs: "" while it isn't set (or is marked for removal).
func TestEditorIsTheGlobalTmuxKitEditor(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	t.Setenv("TMUX_KIT_EDITOR", "")
	os.Unsetenv("TMUX_KIT_EDITOR") // the server would start with it
	c := &tmux.Client{}
	if got, err := c.Editor(); err == nil {
		t.Errorf("with no server, Editor() = %q, want tmux's error", got)
	}
	tmuxRun(t, "new-session", "-d", "-s", "a")
	if got, err := c.Editor(); err != nil || got != "" {
		t.Errorf("with TMUX_KIT_EDITOR unset, Editor() = %q, %v; want \"\"", got, err)
	}
	tmuxRun(t, "set-environment", "-g", "TMUX_KIT_EDITOR", "code -n")
	tmuxRun(t, "set-environment", "-g", "TMUX_KIT_EDITOR_X", "vim")
	tmuxRun(t, "set-environment", "-t", "=a", "TMUX_KIT_EDITOR", "zed")
	if got, err := c.Editor(); err != nil || got != "code -n" {
		t.Errorf("with TMUX_KIT_EDITOR 'code -n', Editor() = %q, %v", got, err)
	}
	tmuxRun(t, "set-environment", "-gr", "TMUX_KIT_EDITOR")
	if got, err := c.Editor(); err != nil || got != "" {
		t.Errorf("with TMUX_KIT_EDITOR marked for removal, Editor() = %q, %v; want \"\"", got, err)
	}
}

// The snapshot has every session, by name, with its windows, by index.
// Names and folders may have spaces, and names may be in any script.
func TestSnapshot(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	before := time.Now().Add(-time.Second)
	s.Write("my projects/api/f", "")
	s.Write("žvėrys/f", "")
	tmuxRun(t, "new-session", "-d", "-s", "my api", "-n", "shell", "-c", s.Path("my projects/api"))
	tmuxRun(t, "new-window", "-d", "-t", "=my api:", "-n", "dev server")
	tmuxRun(t, "new-session", "-d", "-s", "žvėrys", "-c", s.Path("žvėrys"))
	nestedClient(t, "my api")

	got, err := (&tmux.Client{}).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, sess := range got.Sessions {
		names = append(names, sess.Name)
	}
	if want := []string{"my api", "term", "žvėrys"}; !slices.Equal(names, want) {
		t.Fatalf("the snapshot's sessions are %q, want %q", names, want)
	}
	api, zv := got.Sessions[0], got.Sessions[2]
	if want := tmuxRun(t, "display-message", "-p", "-t", "=my api:", "#{session_id}"); api.ID != want {
		t.Errorf("my api's id is %q, want %q", api.ID, want)
	}
	if api.Path != s.Path("my projects/api") || zv.Path != s.Path("žvėrys") {
		t.Errorf("the start folders are %q and %q", api.Path, zv.Path)
	}
	if api.Activity.Before(before) || api.Activity.After(time.Now()) {
		t.Errorf("my api's activity is %v, want about now", api.Activity)
	}
	if api.Attached != 1 || zv.Attached != 0 {
		t.Errorf("my api has %d clients and žvėrys %d, want 1 and 0", api.Attached, zv.Attached)
	}

	wins := strings.Fields(tmuxRun(t, "list-windows", "-t", "=my api", "-F", "#{window_id}"))
	want := []tmux.Window{
		{ID: wins[0], Index: 0, Name: "shell", Active: true},
		{ID: wins[1], Index: 1, Name: "dev server"},
	}
	if !slices.Equal(api.Windows, want) {
		t.Errorf("my api's windows are %+v, want %+v", api.Windows, want)
	}
	if len(zv.Windows) != 1 || !zv.Windows[0].Active {
		t.Errorf("žvėrys's windows are %+v, want its one, active", zv.Windows)
	}
}

// With no server (e.g. once the last session has ended), there's no session,
// and the nested client shows none: that isn't a failure. Neither starts a
// server.
func TestWithNoServerThereIsNoSession(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	c := &tmux.Client{Nested: "/dev/ttys999"}
	if got, err := c.Snapshot(); err != nil || len(got.Sessions) != 0 {
		t.Errorf("with no server, Snapshot() = %+v, %v; want no sessions", got, err)
	}
	if got, err := c.ActiveSession(); err != nil || got != "" {
		t.Errorf("with no server, ActiveSession() = %q, %v; want none", got, err)
	}
	// a server that has gone leaves its socket behind
	tmuxRun(t, "new-session", "-d", "-s", "a")
	tmuxRun(t, "kill-server")
	if got, err := c.Snapshot(); err != nil || len(got.Sessions) != 0 {
		t.Errorf("once the server has gone, Snapshot() = %+v, %v; want no sessions", got, err)
	}
	if out, err := exec.Command("tmux", "list-sessions").CombinedOutput(); err == nil {
		t.Errorf("a server is running, with sessions %s", out)
	}
}

// waitFor polls cond every 0.1 s for up to 10 s.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for range 100 {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// In a host's pane, TMUX names the host; the client still reaches the
// normal server.
func TestReachesTheNormalServerFromInsideAHost(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "a")
	t.Setenv("TMUX", s.Path("host/socket")+",1,0")
	t.Setenv("TMUX_PANE", "%1")
	got, err := (&tmux.Client{}).Snapshot()
	if err != nil || len(got.Sessions) != 1 || got.Sessions[0].Name != "a" {
		t.Errorf("Snapshot() = %+v, %v; want the normal server's a", got, err)
	}
}

// nestedClient attaches a client to session from the pane of a session
// standing in for a terminal, as a host's right side does (no-detach-on-
// destroy, which tmux before 3.6 ignores), and returns the normal server's
// client for it.
func nestedClient(t *testing.T, session string) *tmux.Client {
	t.Helper()
	tmuxRun(t, "new-session", "-d", "-s", "term", "-x", "100", "-y", "30",
		"env", "-u", "TMUX", "tmux", "attach", "-f", "no-detach-on-destroy", "-t", "="+session)
	tty := tmuxRun(t, "display-message", "-p", "-t", "=term:", "#{pane_tty}")
	waitFor(t, "the client to attach", func() bool { return showing(t, tty) != "" })
	return &tmux.Client{Nested: tty}
}

// showing is the session and window the client on tty shows, as "name:index".
func showing(t *testing.T, tty string) string {
	t.Helper()
	for _, line := range strings.Split(tmuxRun(t, "list-clients", "-F", "#{client_tty} #{session_name}:#{window_index}"), "\n") {
		if at, ok := strings.CutPrefix(line, tty+" "); ok {
			return at
		}
	}
	return ""
}

// The active session is the one the nested client, by its tty, shows: its
// id.
func TestActiveSessionIsTheNestedClients(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "my api")
	tmuxRun(t, "new-session", "-d", "-s", "beta")
	nested := nestedClient(t, "my api")
	want := tmuxRun(t, "display-message", "-p", "-t", "=my api:", "#{session_id}")
	if got, err := nested.ActiveSession(); err != nil || got != want {
		t.Errorf("ActiveSession() = %q, %v; want my api's id, %s", got, err, want)
	}
	if got, err := (&tmux.Client{Nested: "/dev/ttys999"}).ActiveSession(); err != nil || got != "" {
		t.Errorf("with no client on its tty, ActiveSession() = %q, %v; want none", got, err)
	}
}

// Showing a session, or one of its windows, switches the nested client to
// it.
func TestShow(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "my api")
	tmuxRun(t, "new-session", "-d", "-s", "beta two")
	tmuxRun(t, "new-window", "-d", "-t", "=beta two:")
	beta := tmuxRun(t, "display-message", "-p", "-t", "=beta two:", "#{session_id}")
	second := tmuxRun(t, "display-message", "-p", "-t", "=beta two:1", "#{window_id}")
	nested := nestedClient(t, "my api")

	if err := nested.Show(beta, ""); err != nil {
		t.Fatal(err)
	}
	if got := showing(t, nested.Nested); got != "beta two:0" {
		t.Errorf("after showing beta two, the client shows %s", got)
	}
	if err := nested.Show(beta, second); err != nil {
		t.Fatal(err)
	}
	if got := showing(t, nested.Nested); got != "beta two:1" {
		t.Errorf("after showing beta two's second window, the client shows %s", got)
	}

	if err := nested.Show("$999", ""); err == nil || !strings.Contains(err.Error(), "$999") {
		t.Errorf("showing a session that's gone: %v, want tmux's error", err)
	}
	if err := (&tmux.Client{}).Show(beta, ""); err == nil {
		t.Error("with no nested client, Show succeeds")
	}
}
