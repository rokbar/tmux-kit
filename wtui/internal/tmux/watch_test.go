package tmux_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// watch starts watching the normal server from nested, the nested client,
// until the test ends.
func watch(t *testing.T, nested *tmux.Client) *tmux.Watch {
	t.Helper()
	w := nested.Watch()
	t.Cleanup(func() { w.Close() })
	return w
}

// next is the first event from w that want takes, skipping the others; the
// test fails if none comes within 5 s.
func next[E tmux.Event](t *testing.T, w *tmux.Watch, what string, want func(E) bool) E {
	t.Helper()
	timeout := time.After(5 * time.Second)
	var seen []string
	for {
		select {
		case ev, ok := <-w.Events():
			if !ok {
				t.Fatalf("the events ended before %s; saw %s", what, strings.Join(seen, ", "))
			}
			seen = append(seen, fmt.Sprintf("%#v", ev))
			if e, ok := ev.(E); ok && want(e) {
				return e
			}
		case <-timeout:
			t.Fatalf("timed out waiting for %s; saw %s", what, strings.Join(seen, ", "))
		}
	}
}

// id is the id of the session named name.
func id(t *testing.T, name string) string {
	t.Helper()
	return tmuxRun(t, "display-message", "-p", "-t", "="+name+":", "#{session_id}")
}

// controlClient is the session the control-mode client is on and its
// flags, or "" while there's none; the test fails if there are several.
func controlClient(t *testing.T) (session, flags string) {
	t.Helper()
	var on []string
	for _, line := range strings.Split(tmuxRun(t, "list-clients", "-F", "#{client_control_mode} #{session_name} #{client_flags}"), "\n") {
		if rest, ok := strings.CutPrefix(line, "1 "); ok {
			on = append(on, rest)
		}
	}
	if len(on) > 1 {
		t.Fatalf("there are %d control clients: %q", len(on), on)
	}
	if len(on) == 0 {
		return "", ""
	}
	session, flags, _ = strings.Cut(on[0], " ")
	return session, flags
}

func onSession(t *testing.T, name string) func() bool {
	return func() bool { s, _ := controlClient(t); return s == name }
}

// The control client attaches to the session the nested client shows, then
// follows it wherever it goes; each move is an event.
func TestWatchFollowsTheNestedClient(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "a")
	tmuxRun(t, "new-session", "-d", "-s", "b")
	nested := nestedClient(t, "a")
	w := watch(t, nested)

	next(t, w, "a to be active", func(e tmux.ActiveSessionChanged) bool { return e.Session == id(t, "a") })
	waitFor(t, "the control client to be on a", onSession(t, "a"))
	_, flags := controlClient(t)
	for _, f := range []string{"ignore-size", "no-output"} {
		if !strings.Contains(flags, f) {
			t.Errorf("the control client's flags are %s, want %s among them", flags, f)
		}
	}

	tmuxRun(t, "switch-client", "-c", nested.Nested, "-t", "=b")
	e := next(t, w, "b to be active", func(e tmux.ActiveSessionChanged) bool { return e.Session == id(t, "b") })
	if e.Ended {
		t.Error("switching the nested client to b says a ended, which it didn't")
	}
	waitFor(t, "the control client to follow the nested client to b", onSession(t, "b"))
}

// started is a Watch from a nested client on a, once it's attached.
func started(t *testing.T, a string) *tmux.Watch {
	t.Helper()
	w := watch(t, nestedClient(t, a))
	next(t, w, a+" to be active", func(e tmux.ActiveSessionChanged) bool { return e.Session == id(t, a) })
	return w
}

func always[E tmux.Event](E) bool { return true }

// Sessions created, renamed and killed from outside are events, whichever
// session the control client is on.
func TestWatchSeesSessionsChange(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "a")
	w := started(t, "a")
	for _, args := range [][]string{
		{"new-session", "-d", "-s", "b"},
		{"rename-session", "-t", "=b", "b two"},
		{"kill-session", "-t", "=b two"},
	} {
		tmuxRun(t, args...)
		next(t, w, strings.Join(args, " ")+" to be an event", always[tmux.SessionsChanged])
	}
}

// windows are a WindowsChanged's windows as "index name", the active one
// marked *.
func windows(e tmux.WindowsChanged) string {
	var ws []string
	for _, w := range e.Windows {
		mark := ""
		if w.Active {
			mark = "*"
		}
		ws = append(ws, fmt.Sprintf("%d %s%s", w.Index, w.Name, mark))
	}
	return strings.Join(ws, ", ")
}

// Windows added, renamed, made active and killed from outside are events
// that say which session's windows changed and what they are now, whether
// that's the control client's session or another one. Window names may have
// spaces.
func TestWatchSeesWindowsChange(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "a", "-n", "one")
	tmuxRun(t, "new-session", "-d", "-s", "b", "-n", "one")
	w := started(t, "a")
	for _, c := range []struct {
		args          []string
		session, want string
	}{
		{[]string{"new-window", "-d", "-t", "=b:", "-n", "two"}, "b", "0 one*, 1 two"},
		{[]string{"rename-window", "-t", "=b:1", "dev server"}, "b", "0 one*, 1 dev server"},
		{[]string{"select-window", "-t", "=b:1"}, "b", "0 one, 1 dev server*"},
		{[]string{"kill-window", "-t", "=b:0"}, "b", "1 dev server*"},
		{[]string{"new-window", "-d", "-t", "=a:", "-n", "two"}, "a", "0 one*, 1 two"},
		{[]string{"rename-window", "-t", "=a:1", "logs"}, "a", "0 one*, 1 logs"},
		{[]string{"kill-window", "-t", "=a:1"}, "a", "0 one*"},
	} {
		tmuxRun(t, c.args...)
		session := id(t, c.session)
		next(t, w, fmt.Sprintf("%s to make %s's windows %s", strings.Join(c.args, " "), c.session, c.want),
			func(e tmux.WindowsChanged) bool { return e.Session == session && windows(e) == c.want })
	}
}

// needTmux skips the test on a tmux older than min.
func needTmux(t *testing.T, min string) {
	t.Helper()
	if v := strings.TrimPrefix(tmuxRun(t, "-V"), "tmux "); tmux.Older(v, min) {
		t.Skipf("this test needs tmux %s or newer, and this is %s", min, v)
	}
}

// When the active session ends, tmux moves the nested client to the session
// with the most recent activity (no-detach-on-destroy), and the control
// client follows it there. The event says the session it was on ended.
func TestWatchFollowsTheNestedClientWhenItsSessionEnds(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	needTmux(t, "3.6")
	tmuxRun(t, "new-session", "-d", "-s", "a")
	tmuxRun(t, "new-session", "-d", "-s", "b")
	nested := nestedClient(t, "a")
	w := watch(t, nested)
	next(t, w, "a to be active", func(e tmux.ActiveSessionChanged) bool { return e.Session == id(t, "a") })
	tmuxRun(t, "switch-client", "-c", nested.Nested, "-t", "=b")
	next(t, w, "b to be active", func(e tmux.ActiveSessionChanged) bool { return e.Session == id(t, "b") })
	waitFor(t, "the control client to be on b", onSession(t, "b"))
	time.Sleep(1100 * time.Millisecond) // c is the most recent
	tmuxRun(t, "new-session", "-d", "-s", "c")
	c := id(t, "c")
	tmuxRun(t, "kill-session", "-t", "=b")
	e := next(t, w, "c to be active", func(e tmux.ActiveSessionChanged) bool { return e.Session == c })
	if !e.Ended {
		t.Error("moving on from b, which ended, doesn't say b ended")
	}
	waitFor(t, "the control client to be on c", onSession(t, "c"))
}

// Started before the nested client shows a session (as at launch), the
// Watch waits for it.
func TestWatchWaitsForTheNestedClient(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "a")
	tmuxRun(t, "new-session", "-d", "-s", "term", "-x", "100", "-y", "30",
		"sh", "-c", "sleep 1; exec env -u TMUX tmux attach -t =a")
	tty := tmuxRun(t, "display-message", "-p", "-t", "=term:", "#{pane_tty}")
	w := watch(t, &tmux.Client{Nested: tty})
	next(t, w, "a to be active", func(e tmux.ActiveSessionChanged) bool { return e.Session == id(t, "a") })
	waitFor(t, "the control client to be on a", onSession(t, "a"))
}

// If the control client ends (here, detached from outside), the sessions
// may have changed meanwhile; once there's a session to attach to again, it
// does, and says which is active.
func TestWatchComesBackIfItsClientEnds(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "a")
	w := started(t, "a")
	waitFor(t, "the control client to be on a", onSession(t, "a"))
	var name string
	for _, line := range strings.Split(tmuxRun(t, "list-clients", "-F", "#{client_control_mode} #{client_name}"), "\n") {
		if n, ok := strings.CutPrefix(line, "1 "); ok {
			name = n
		}
	}
	tmuxRun(t, "detach-client", "-t", name)
	next(t, w, "the sessions to have changed", always[tmux.SessionsChanged])
	next(t, w, "a to be active again", func(e tmux.ActiveSessionChanged) bool { return e.Session == id(t, "a") })
	waitFor(t, "the control client to be back on a", onSession(t, "a"))
}

// When the server ends, so does the control client, with nowhere to attach
// again: the Watch says the sessions changed (they're gone) and the nested
// client shows none, and waits.
func TestWatchSaysSoWhenTheServerEnds(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "a")
	w := started(t, "a")
	waitFor(t, "the control client to be on a", onSession(t, "a"))
	tmuxRun(t, "kill-server")
	next(t, w, "the sessions to have changed", always[tmux.SessionsChanged])
	next(t, w, "no session to be active", func(e tmux.ActiveSessionChanged) bool { return e.Session == "" })
}

// So it does while the server is on its way out, before it has exited.
func TestWatchSaysSoAsTheServerEnds(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "a")
	w := started(t, "a")
	waitFor(t, "the control client to be on a", onSession(t, "a"))
	killSlowly(t)
	next(t, w, "the sessions to have changed", always[tmux.SessionsChanged])
	next(t, w, "no session to be active", func(e tmux.ActiveSessionChanged) bool { return e.Session == "" })
}

// And so it does when the last session ends but the server stays, with no
// session: until it exits, or for good with exit-empty off.
func TestWatchSaysSoWhenTheLastSessionEnds(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "a")
	w := started(t, "a")
	waitFor(t, "the control client to be on a", onSession(t, "a"))
	tmuxRun(t, "set-option", "-g", "exit-empty", "off")
	tmuxRun(t, "kill-session", "-t", "=term") // the nested client's terminal
	tmuxRun(t, "kill-session", "-t", "=a")
	next(t, w, "the sessions to have changed", always[tmux.SessionsChanged])
	next(t, w, "no session to be active", func(e tmux.ActiveSessionChanged) bool { return e.Session == "" })
}

// A subscription's format is about the whole server, not only the control
// client's session: its value comes at once, and again whenever it changes
// (tmux looks once a second).
func TestWatchSubscriptions(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmux.Subscribe(t, "names", "#{S:#{session_name};}")
	tmuxRun(t, "new-session", "-d", "-s", "a")
	w := started(t, "a")
	// the sessions, in whatever order tmux walks them
	names := func(want ...string) func(tmux.Subscribed) bool {
		return func(e tmux.Subscribed) bool {
			got := strings.Split(strings.TrimSuffix(e.Value, ";"), ";")
			slices.Sort(got)
			return e.Name == "names" && slices.Equal(got, want)
		}
	}
	next(t, w, "the names", names("a", "term"))
	tmuxRun(t, "new-session", "-d", "-s", "b c")
	next(t, w, "the names with b c", names("a", "b c", "term"))
}

// Once closed, the control client has gone, and so have the events.
func TestWatchEndsOnClose(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "a")
	w := started(t, "a")
	waitFor(t, "the control client to be on a", onSession(t, "a"))
	w.Close()
	if session, _ := controlClient(t); session != "" {
		t.Errorf("after Close, the control client is still on %s", session)
	}
	for ev := range w.Events() {
		t.Errorf("after Close, an event: %#v", ev)
	}
}

// recency is session name's last activity and last attach, as tmux has them
// (to the second).
func recency(t *testing.T, name string) string {
	t.Helper()
	return tmuxRun(t, "display-message", "-p", "-t", "="+name+":", "#{session_activity} #{session_last_attached}")
}

// Following the nested client leaves no trace on other sessions: their
// activity and last attach stay as they were, so what's most recent stays
// so, and no other session has the control client attached.
func TestWatchLeavesOtherSessionsRecencyAlone(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "a")
	tmuxRun(t, "new-session", "-d", "-s", "b")
	tmuxRun(t, "new-session", "-d", "-s", "c")
	nested := nestedClient(t, "a")
	time.Sleep(1100 * time.Millisecond) // so that a bump would show
	c := recency(t, "c")

	w := watch(t, nested)
	next(t, w, "a to be active", func(e tmux.ActiveSessionChanged) bool { return e.Session == id(t, "a") })
	waitFor(t, "the control client to be on a", onSession(t, "a"))
	tmuxRun(t, "switch-client", "-c", nested.Nested, "-t", "=b")
	waitFor(t, "the control client to follow the nested client to b", onSession(t, "b"))
	a := recency(t, "a")
	time.Sleep(1100 * time.Millisecond)

	if got := recency(t, "c"); got != c {
		t.Errorf("c's activity and last attach went from %s to %s", c, got)
	}
	if got := recency(t, "a"); got != a {
		t.Errorf("once the nested client left a, its activity and last attach went from %s to %s", a, got)
	}
	attached := tmuxRun(t, "list-sessions", "-F", "#{session_name}:#{session_attached}")
	if want := "a:0\nb:2\nc:0\nterm:0"; attached != want {
		t.Errorf("the sessions' attached clients are\n%s\nwant\n%s", attached, want)
	}
}
