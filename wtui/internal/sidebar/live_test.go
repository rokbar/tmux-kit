package sidebar

import (
	"maps"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// The tree keeps itself up to date from the normal server's events (the
// tmux client's Watch).

// A session created, closed or renamed anywhere rebuilds the whole tree, in
// the background.
func TestASessionEventRebuildsTheTree(t *testing.T) {
	f := fixture()
	m := started(t, f)
	before := f.normal.snapshots
	f.normal.snap.Sessions = append(without(f.normal.snap.Sessions, "$3"), session(6, "docs", "/p/docs", "shell"))
	m, cmd := update(t, m, tmux.SessionsChanged{})
	if f.normal.snapshots != before {
		t.Fatal("a session event took a snapshot before returning")
	}
	m = feed(t, m, cmd)
	view := plain(m)
	if !strings.Contains(view, "  ▾ docs\n    ▸ docs\n") || strings.Contains(view, "notes") {
		t.Errorf("after notes went and docs came, the view is:\n%s", view)
	}
}

// fixLogin is a WindowsChanged for fix-login ($2): its shell (@20), its dev
// window (@21) renamed server, and a new window logs (@22), the active one.
var fixLogin = tmux.WindowsChanged{Session: "$2", Windows: []tmux.Window{
	{ID: "@20", Index: 0, Name: "shell"},
	{ID: "@21", Index: 1, Name: "server"},
	{ID: "@22", Index: 2, Name: "logs", Active: true},
}}

// A window event changes only that session's windows, from what the event
// says, without a snapshot: the rest of the tree stays as it was built. The
// cursor stays on its window, whatever it's called now.
func TestAWindowEventUpdatesOnlyThatSessionsWindows(t *testing.T) {
	f := fixture()
	m := keys(t, focused(t, f), press("j"), press("j")) // fix-login's dev
	before := f.normal.snapshots
	f.normal.snap.Sessions = without(f.normal.snap.Sessions, "$3") // unseen
	m, cmd := update(t, m, fixLogin)
	if cmd != nil || f.normal.snapshots != before {
		t.Errorf("a window event gave %v and took %d snapshots, want nothing", cmd, f.normal.snapshots-before)
	}
	want := ` wtui
  ▾ api
    ▸ api
    ▸ Bugfix
    ▾ fix-login
        0 shell
        1 server
        2 logs
  ▾ notes
    ▸ notes
  ▾ Zed
    ▸ zed`
	if got := plain(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
	if got := cursor(t, m); got != "        1 server" {
		t.Errorf("the cursor is on %q, want dev, now server", got)
	}
	if got := styles(m)["        2 logs"]; !hasParam(got, "1") {
		t.Errorf("logs, now fix-login's active window, is styled %q, want bold", got)
	}
}

// A window event for a session the tree doesn't have yet changes nothing:
// the session's own event builds it.
func TestAWindowEventForANewSessionWaitsForItsBuild(t *testing.T) {
	m := started(t, fixture())
	before := plain(m)
	m, cmd := update(t, m, tmux.WindowsChanged{Session: "$9", Windows: []tmux.Window{{ID: "@90", Name: "shell", Active: true}}})
	if got := plain(m); cmd != nil || got != before {
		t.Errorf("after a window event for a new session, the view is:\n%s\nwant it as it was:\n%s", got, before)
	}
}

// When the right side changes session by other means (prefix s there, its
// session ending), the event says so: the highlight moves, the new active
// session unfolds and the old one folds, and on getting focus, the cursor
// goes to the new one. Nothing asks tmux again which session is active.
func TestTheActiveSessionFollowsTheRightSide(t *testing.T) {
	f := fixture()
	m := started(t, f)
	m, poll := update(t, m, Active{Session: "$2"})
	if poll != nil {
		t.Error("the sidebar asks again which session is active, want it told")
	}
	m, _ = update(t, m, tea.BlurMsg{}) // focus is on the right
	m, cmd := update(t, m, tmux.ActiveSessionChanged{Session: "$3"})
	if cmd != nil {
		t.Errorf("a change of active session gave %v, want nothing", cmd)
	}
	want := map[string]string{
		"  ▾ api": "", "    ▸ api": "", "    ▸ Bugfix": "", "    ▸ fix-login": "",
		"  ▾ notes": "36", "    ▾ notes": "1;36", "        0 vim": "1", "  ▾ Zed": "", "    ▸ zed": "",
	}
	if got := styles(m); !maps.Equal(got, want) {
		t.Errorf("with notes active, the rows' styles are %q, want %q", got, want)
	}
	m, _ = update(t, m, tea.FocusMsg{})
	if got := cursor(t, m); got != "    ▾ notes" {
		t.Errorf("on getting focus, the cursor is on %q, want notes, the active session", got)
	}
}

// A build that's running when a window event comes may have taken its
// snapshot before the windows changed, so another build follows it.
func TestAWindowEventDuringABuildHasAnotherFollowIt(t *testing.T) {
	f := fixture()
	m := started(t, f)
	m, build := update(t, m, press("r"))
	old := run(t, build) // its snapshot is from before the change
	sessions := without(f.normal.snap.Sessions, "$2")
	f.normal.snap.Sessions = append(sessions, tmux.Session{ID: "$2", Name: "fix-login", Path: "/p/api--fix-login", Windows: fixLogin.Windows})
	m, _ = update(t, m, fixLogin)
	m, again := update(t, m, old)
	if again == nil {
		t.Fatal("no build follows the one running when the windows changed")
	}
	m = feed(t, m, again)
	if got := plain(m); !strings.Contains(got, "        2 logs\n") {
		t.Errorf("after the following build, the view is:\n%s\nwant fix-login's logs", got)
	}
}
