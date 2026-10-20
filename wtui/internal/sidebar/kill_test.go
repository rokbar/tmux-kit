package sidebar

import (
	"maps"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/host"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// socket is the normal server's socket, in these tests.
const socket = "/tmp/tmux-501/default"

// withKill is the sidebar of f, started with focus, so its cursor is on the
// active session, with wtui at its path and the normal server's socket
// known.
func withKill(t *testing.T, f fakes) Model {
	t.Helper()
	m := New(Clients{Host: f.host, Normal: f.normal, Repos: f.repos, Wt: f.wt, Wtui: wtui, Socket: socket}, `C-\`)
	m = feed(t, m, m.Init())
	m, _ = update(t, m, tea.FocusMsg{})
	return m
}

// d asks, in a popup on the nested client, whether to kill the cursor's
// row, a session or a window: wtui's kill dialog, which reaches the normal
// server by its socket, given the row's session (and window), by id, and
// the question. Any other row gets a hint: a project, a worktree with
// several sessions under it, and a worktree with none (missing or not).
func TestDKillsTheRowsSessionOrWindow(t *testing.T) {
	f := withAgents()
	m := withKill(t, f)
	kill := ": /bin/wtui -S " + socket + " kill "
	want := map[string]string{
		"  ▾ api !":           "api isn't a session or a window",
		"    ▾ main *":        "main isn't a session or a window",
		"1     ▸ api ✓":       kill + "$1 Kill session api and its one window?",
		"2     ▸ Bugfix *":    kill + "$5 Kill session Bugfix and its one window?",
		"3   ▾ login !":       kill + "$2 Kill session login and its 2 windows?",
		"        0 shell !":   kill + "$2:@20 Kill window 0 shell of session login?",
		"        1 dev *":     kill + "$2:@21 Kill window 1 dev of session login?",
		"      docs":          "docs isn't a session or a window",
		"      api--rebase ±": "api--rebase isn't a session or a window",
		"      api--gone ✗":   "api--gone isn't a session or a window",
		"  ▾ notes":           "notes isn't a session or a window",
		"4   ▸ notes":         kill + "$3 Kill session notes and its one window?",
		"  ▾ Zed ✓":           "Zed isn't a session or a window",
		"5   ▸ zed ✓":         kill + "$4 Kill session zed and its one window?",
		"6   ▸ agent ✓":       kill + "$6 Kill session agent and its one window?",
	}
	got := popupOnEveryRow(t, m, press("d"), f)
	for row, opened := range got {
		got[row], _, _ = strings.Cut(opened, "\n") // the question's first line
	}
	if !maps.Equal(got, want) {
		t.Errorf("d opened %q, want %q", got, want)
	}
	for _, p := range f.normal.popups {
		if p.Title != " kill " {
			t.Errorf("the kill dialog is titled %q", p.Title)
		}
	}
	if len(f.normal.shown) > 0 {
		t.Errorf("d showed %q on the right, want nothing shown", f.normal.shown)
	}
}

// question is the question the kill dialog asks, once d is pressed on row,
// after each row in unfold is unfolded.
func question(t *testing.T, f fakes, row string, unfold ...string) string {
	t.Helper()
	m := withKill(t, f)
	for _, r := range unfold {
		m = keys(t, toRow(t, m, r), press("l"))
	}
	m, cmd := update(t, toRow(t, m, row), press("d"))
	n := len(f.normal.popups)
	feed(t, m, cmd)
	if len(f.normal.popups) != n+1 {
		t.Fatalf("d on %q opened %d popups, want one", row, len(f.normal.popups)-n)
	}
	p := f.normal.popups[n]
	return p.Command[len(p.Command)-1]
}

// The question names what d kills, and the agents that end with it, by
// window, most urgent first. Killing a worktree's session leaves the
// worktree, and it says so.
func TestTheKillQuestionNamesTheAgentsInIt(t *testing.T) {
	want := `Kill session login and its 2 windows?
The agents in it end too:
  0 shell: needs you
  0 shell: done
  1 dev: working
The worktree api--fix-login stays.`
	if got := question(t, withAgents(), "3   ▾ login !"); got != want {
		t.Errorf("on login, the question is:\n%s\nwant:\n%s", got, want)
	}
	want = `Kill window 1 dev of session login?
The agent in it ends too:
  1 dev: working`
	linked := withAgents() // dev is linked into another session too, where tmux lists its agent again
	linked.normal.snap.Agents = append(linked.normal.snap.Agents, agent("$9", "@21", "%1", tmux.Working))
	if got := question(t, linked, "        1 dev *"); got != want {
		t.Errorf("on login's dev, the question is:\n%s\nwant:\n%s", got, want)
	}
	// a session in no worktree, and without agents
	if got, want := question(t, withAgents(), "4   ▸ notes"), "Kill session notes and its one window?"; got != want {
		t.Errorf("on notes, the question is:\n%s\nwant:\n%s", got, want)
	}
}

// Killing a session's last window kills the session, and the question says
// so. A session in the main clone has no worktree to stay.
func TestKillingASessionsLastWindowSaysItKillsTheSession(t *testing.T) {
	want := `Kill window 0 shell of session api?
It's the session's last window: session api ends too.
The agent in it ends too:
  0 shell: done`
	if got := question(t, withAgents(), "          0 shell ✓", "1     ▸ api ✓"); got != want {
		t.Errorf("on api's one window, the question is:\n%s\nwant:\n%s", got, want)
	}
	want = `Kill window 0 claude of session agent?
It's the session's last window: session agent ends too.
The agent in it ends too:
  0 claude: done
The worktree agent stays.`
	if got := question(t, withAgents(), "        0 claude ✓", "6   ▸ agent ✓"); got != want {
		t.Errorf("on agent's one window, the question is:\n%s\nwant:\n%s", got, want)
	}
}

// The kill dialog is a popup, so d isn't held: the host would hold the
// keys typed into the popup until d was done, once the popup had closed.
func TestDIsntHeld(t *testing.T) {
	i := slices.IndexFunc(Keys(), func(k host.Key) bool { return k.Name == "d" })
	if i < 0 {
		t.Fatal("the host doesn't bind d")
	}
	if k := Keys()[i]; k.Held || k.Tmux != "d" {
		t.Errorf("the host binds d as %+v, want it passed on to the sidebar", k)
	}
}
