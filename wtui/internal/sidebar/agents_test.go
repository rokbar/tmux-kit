package sidebar

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// Each row shows an agent state at its end: ! needs you, * working, ✓ done.

// agent is an agent in pane of session's window (ids), in state.
func agent(session, window, pane string, state tmux.AgentState) tmux.Agent {
	return tmux.Agent{Session: session, Window: window, Pane: pane, State: state}
}

// withAgents is the worktrees fixture with agents in its panes: in login
// ($2), its shell window with one agent needing you and one done, and its
// dev window (shown) working; in main's api done and Bugfix working; Zed's
// both done.
func withAgents() fakes {
	f := worktrees()
	f.normal.snap.Agents = []tmux.Agent{
		agent("$2", "@20", "%3", tmux.NeedsYou),
		agent("$2", "@20", "%2", tmux.Done),
		agent("$2", "@21", "%1", tmux.Working),
		agent("$1", "@10", "%4", tmux.Done),
		agent("$5", "@50", "%5", tmux.Working),
		agent("$4", "@40", "%6", tmux.Done),
		agent("$6", "@60", "%7", tmux.Done),
	}
	return f
}

// A window shows the most urgent state of its panes' agents, and a
// session, a worktree and a project the most urgent beneath them, folded or
// not: needs you, then working, then done. The snapshot has them, so they
// show at launch.
func TestEachRowShowsTheMostUrgentAgentStateBeneathIt(t *testing.T) {
	m := started(t, withAgents())
	want := ` wtui
  ▾ api !
    ▾ main *
1     ▸ api ✓
2     ▸ Bugfix *
3   ▾ login !
        0 shell !
        1 dev *
      docs
      api--rebase ±
      api--gone ✗
  ▾ notes
4   ▸ notes
  ▾ Zed ✓
5   ▸ zed ✓
6   ▸ agent ✓`
	if got := plain(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
}

// A change of agent states, from tmux's event, shows at once, without a
// snapshot: the rest of the tree stays as it was built.
func TestAgentStatesFollowTheirEvents(t *testing.T) {
	f := withAgents()
	m := started(t, f)
	before := f.normal.snapshots
	f.normal.snap.Sessions = without(f.normal.snap.Sessions, "$3") // unseen
	m, cmd := update(t, m, tmux.AgentsChanged{Agents: []tmux.Agent{
		agent("$3", "@30", "%8", tmux.NeedsYou),
		agent("$1", "@10", "%4", tmux.Working),
	}})
	if cmd != nil || f.normal.snapshots != before {
		t.Errorf("an agent event gave %v and took %d snapshots, want nothing", cmd, f.normal.snapshots-before)
	}
	want := ` wtui
  ▾ api *
    ▾ main *
1     ▸ api *
2     ▸ Bugfix
3   ▾ login
        0 shell
        1 dev
      docs
      api--rebase ±
      api--gone ✗
  ▾ notes !
4   ▸ notes !
  ▾ Zed
5   ▸ zed
6   ▸ agent`
	if got := plain(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
}

// A build that's running when the agent states change took its snapshot
// before they did: the tree keeps the newer states, with no build to follow
// for them. The next build's snapshot is newer again.
func TestAgentStatesNewerThanABuildStay(t *testing.T) {
	f := withAgents()
	m := started(t, f)
	m, build := update(t, m, press("r"))
	old := run(t, build) // its snapshot has the agents as they were
	m, _ = update(t, m, tmux.AgentsChanged{Agents: []tmux.Agent{agent("$3", "@30", "%8", tmux.NeedsYou)}})
	m, again := update(t, m, old)
	if again != nil {
		t.Error("a build follows the one that ran when the agent states changed")
	}
	marked := func(m Model) []string {
		var rows []string
		for _, line := range strings.Split(plain(m), "\n") {
			if strings.HasSuffix(line, " !") || strings.HasSuffix(line, " *") || strings.HasSuffix(line, " ✓") {
				rows = append(rows, line)
			}
		}
		return rows
	}
	if got, want := marked(m), []string{"  ▾ notes !", "4   ▸ notes !"}; !slices.Equal(got, want) {
		t.Errorf("after the build, the rows with agent states are %q, want %q", got, want)
	}
	f.normal.snap.Agents = []tmux.Agent{agent("$4", "@40", "%6", tmux.Done)}
	m = rebuilt(t, m, f)
	if got, want := marked(m), []string{"  ▾ Zed ✓", "5   ▸ zed ✓"}; !slices.Equal(got, want) {
		t.Errorf("after the next build, the rows with agent states are %q, want %q", got, want)
	}
}

// cell is a character of the view, with the SGR parameters it shows in.
type cell struct{ text, params string }

// cellsOf are line's characters, each with its SGR parameters.
func cellsOf(line string) []cell {
	var cells []cell
	params := ""
	for len(line) > 0 {
		if rest, ok := strings.CutPrefix(line, "\x1b["); ok {
			p, after, _ := strings.Cut(rest, "m")
			if params, line = p, after; p == "0" {
				params = ""
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(line)
		cells = append(cells, cell{string(r), params})
		line = line[size:]
	}
	return cells
}

// rowCells are the cells of the view's line for row, the one whose text
// (without the agent column) is row; the test fails if there's none.
func rowCells(t *testing.T, m Model, row string) []cell {
	t.Helper()
	for _, line := range strings.Split(m.View().Content, "\n")[1:] {
		cells := cellsOf(line)
		var text string
		for _, c := range cells[:max(0, len(cells)-2)] {
			text += c.text
		}
		if strings.TrimRight(text, " ") == row {
			return cells
		}
	}
	t.Fatalf("no row %q in the view:\n%s", row, plain(m))
	return nil
}

// The agent state is a column at the right edge of each row, after a space:
// a red !, a yellow *, a dim green ✓. On the cursor's row, it's in reverse
// video, as the rest of the row is.
func TestTheAgentStateIsAColumnAtTheRightEdge(t *testing.T) {
	m := focused(t, withAgents()) // the cursor on login
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 30, Height: 30})
	for _, c := range []struct {
		row, mark, params, gap string
	}{
		{"  ▾ api", "!", "31", ""},
		{"    ▾ main", "*", "33", ""},
		{"1     ▸ api", "✓", "2;32", ""},
		{"3   ▾ login", "!", "7;31", "7"},
		{"  ▾ notes", " ", "", ""},
	} {
		cells := rowCells(t, m, c.row)
		if len(cells) != 30 {
			t.Errorf("row %q is %d columns wide, want the sidebar's 30", c.row, len(cells))
			continue
		}
		if got, gap := cells[29], cells[28]; got != (cell{c.mark, c.params}) || gap != (cell{" ", c.gap}) {
			t.Errorf("row %q ends with %+v, then %+v; want a space styled %q, then %q styled %q", c.row, gap, got, c.gap, c.mark, c.params)
		}
	}
}

// ClearDone clears done in window, as tmux would, and records it.
func (n *fakeNormal) ClearDone(window string) error {
	n.cleared = append(n.cleared, window)
	if n.clearErr != nil {
		return n.clearErr
	}
	n.snap.Agents = slices.DeleteFunc(slices.Clone(n.snap.Agents), func(a tmux.Agent) bool {
		return a.Window == window && a.State == tmux.Done
	})
	return nil
}

// runAll runs cmd, if there's one, and each command of a batch, and returns
// their messages. It doesn't run what the model returns for them.
func runAll(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, c := range batch {
			msgs = append(msgs, runAll(c)...)
		}
		return msgs
	}
	return []tea.Msg{msg}
}

// rowsOf are view's lines that start with one of rows, as they are.
func rowsOf(view string, rows ...string) []string {
	var got []string
	for _, line := range strings.Split(view, "\n") {
		for _, r := range rows {
			if strings.HasPrefix(line, r) {
				got = append(got, line)
			}
		}
	}
	return got
}

// login's windows ($2), with shell (@20) its current one.
var loginAtShell = tmux.WindowsChanged{Session: "$2", Windows: []tmux.Window{
	{ID: "@20", Index: 0, Name: "shell", Active: true},
	{ID: "@21", Index: 1, Name: "dev"},
}}

// "Done" is finished and not seen yet: once its window is shown (the active
// window of the session on the right), tmux is asked to clear it there, and
// the tree shows it cleared at once. That's as done arrives in a window
// shown already, and as a window becomes shown: the session's current
// window changing, or another session shown. Done in a window not shown
// stays.
func TestShowingAWindowClearsItsDone(t *testing.T) {
	f := worktrees()
	f.normal.snap.Agents = []tmux.Agent{
		agent("$2", "@20", "%1", tmux.Done), // login's shell; its dev is shown
		agent("$1", "@10", "%3", tmux.Done), // api's shell
	}
	m := started(t, f)
	rows := []string{"1     ▸ api", "3   ▾ login", "        0 shell", "        1 dev"}
	if got, want := rowsOf(plain(m), rows...), []string{"1     ▸ api ✓", "3   ▾ login ✓", "        0 shell ✓", "        1 dev"}; !slices.Equal(got, want) {
		t.Fatalf("at launch, the rows are %q, want %q", got, want)
	}

	f.normal.snap.Agents = append(f.normal.snap.Agents, agent("$2", "@21", "%2", tmux.Done))
	m, cmd := update(t, m, tmux.AgentsChanged{Agents: f.normal.snap.Agents})
	if got, want := rowsOf(plain(m), rows...), []string{"1     ▸ api ✓", "3   ▾ login ✓", "        0 shell ✓", "        1 dev"}; !slices.Equal(got, want) {
		t.Errorf("with done arriving in dev, shown, the rows are %q, want %q", got, want)
	}
	runAll(cmd)
	if want := []string{"@21"}; !slices.Equal(f.normal.cleared, want) {
		t.Errorf("with done arriving in dev, shown, tmux was asked to clear %q, want %q", f.normal.cleared, want)
	}

	m, cmd = update(t, m, loginAtShell) // prefix p on the right
	runAll(cmd)
	if got, want := rowsOf(plain(m), rows...), []string{"1     ▸ api ✓", "3   ▾ login", "        0 shell", "        1 dev"}; !slices.Equal(got, want) {
		t.Errorf("with login's shell shown, the rows are %q, want %q", got, want)
	}
	m, cmd = update(t, m, tmux.ActiveSessionChanged{Session: "$1"})
	runAll(cmd)
	if got, want := rowsOf(plain(m), "1     ▾ api", "3   ▸ login"), []string{"1     ▾ api", "3   ▸ login"}; !slices.Equal(got, want) {
		t.Errorf("with api shown, the rows are %q, want %q", got, want)
	}
	if want := []string{"@21", "@20", "@10"}; !slices.Equal(f.normal.cleared, want) {
		t.Errorf("tmux was asked to clear %q, want %q", f.normal.cleared, want)
	}
}

// Working and needs you are never cleared, shown or not.
func TestOtherStatesAreNeverCleared(t *testing.T) {
	f := worktrees()
	f.normal.snap.Agents = []tmux.Agent{agent("$2", "@21", "%2", tmux.Working)} // login's dev, shown
	m, cmd := update(t, started(t, f), tmux.AgentsChanged{Agents: []tmux.Agent{agent("$2", "@21", "%2", tmux.NeedsYou)}})
	runAll(cmd)
	m, cmd = update(t, m, loginAtShell)
	runAll(cmd)
	m, cmd = update(t, m, tmux.WindowsChanged{Session: "$2", Windows: f.normal.snap.Sessions[1].Windows}) // dev again
	runAll(cmd)
	if len(f.normal.cleared) > 0 {
		t.Errorf("tmux was asked to clear %q, want nothing", f.normal.cleared)
	}
	if got := rowsOf(plain(m), "        1 dev"); !slices.Equal(got, []string{"        1 dev !"}) {
		t.Errorf("dev's row is %q, want it needing you", got)
	}
}

// enter on a window shows it once tmux says so, not before: the done in
// its session's current window, which isn't shown, stays.
func TestOnlyTheWindowShownClears(t *testing.T) {
	f := worktrees()
	f.normal.snap.Agents = []tmux.Agent{agent("$2", "@21", "%2", tmux.Done)} // login's dev, its current window
	f.normal.active = "$1"
	m := keys(t, focused(t, f), press("j"), press("j"), press("j"), press("l"), press("j"))
	if got := cursor(t, m); got != "        0 shell" {
		t.Fatalf("the cursor is on %q, want login's shell", got)
	}
	// From here on, the windows Show changes are tmux's, not the sidebar's
	// too.
	f.normal.snap.Sessions = slices.Clone(f.normal.snap.Sessions)
	for i, s := range f.normal.snap.Sessions {
		f.normal.snap.Sessions[i].Windows = slices.Clone(s.Windows)
	}
	m, cmd := update(t, m, enter)
	for _, msg := range runAll(cmd) {
		var next tea.Cmd
		m, next = update(t, m, msg) // shown: the right side shows login, not yet its shell
		runAll(next)
	}
	for _, msg := range []tea.Msg{loginAtShell, tmux.ActiveSessionChanged{Session: "$2"}} {
		m, cmd = update(t, m, msg) // tmux says so
		runAll(cmd)
	}
	if len(f.normal.cleared) > 0 {
		t.Errorf("tmux was asked to clear %q, want nothing", f.normal.cleared)
	}
	if got := rowsOf(plain(m), "        1 dev"); !slices.Equal(got, []string{"        1 dev ✓"}) {
		t.Errorf("dev's row is %q, want it done", got)
	}
}

// A build's snapshot may be older than what tmux has said since, e.g. that
// another window is shown: nothing clears from it, only from the build
// that follows it.
func TestNothingClearsFromABuildOlderThanTmuxsWord(t *testing.T) {
	f := worktrees()
	m := started(t, f)
	m, build := update(t, m, press("r"))
	old := run(t, build) // its snapshot has dev shown
	f.normal.snap.Sessions = slices.Clone(f.normal.snap.Sessions)
	f.normal.snap.Sessions[1].Windows = loginAtShell.Windows
	m, _ = update(t, m, loginAtShell)
	f.normal.snap.Agents = []tmux.Agent{agent("$2", "@21", "%2", tmux.Done)}
	m, cmd := update(t, m, tmux.AgentsChanged{Agents: f.normal.snap.Agents}) // dev isn't shown
	runAll(cmd)
	m, again := update(t, m, old)
	for _, msg := range runAll(again) {
		m, cmd = update(t, m, msg)
		runAll(cmd)
	}
	if len(f.normal.cleared) > 0 {
		t.Errorf("tmux was asked to clear %q, want nothing", f.normal.cleared)
	}
	if got := rowsOf(plain(m), "        1 dev"); !slices.Equal(got, []string{"        1 dev ✓"}) {
		t.Errorf("dev's row is %q, want it done", got)
	}
}

// If tmux can't clear a done, the status line says why.
func TestAFailedClearSaysWhy(t *testing.T) {
	f := worktrees()
	f.normal.clearErr = errors.New("can't find pane: %2")
	m, cmd := update(t, started(t, f), tmux.AgentsChanged{Agents: []tmux.Agent{agent("$2", "@21", "%2", tmux.Done)}})
	for _, msg := range runAll(cmd) {
		m, _ = update(t, m, msg)
	}
	if got := plain(m); !strings.HasSuffix(got, "\ncan't find pane: %2") {
		t.Errorf("view:\n%s\nwant it to end with the failure", got)
	}
}
