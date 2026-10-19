package sidebar

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// fakeHost records what the sidebar asked of its host.
type fakeHost struct{ asked []string }

func (h *fakeHost) FocusRight() error  { h.asked = append(h.asked, "focus right"); return nil }
func (h *fakeHost) PrefixRight() error { h.asked = append(h.asked, "prefix right"); return nil }
func (h *fakeHost) Hide() error        { h.asked = append(h.asked, "hide"); return nil }

// fakeNormal is a normal server with the sessions in snap, whose nested
// client shows active (an id). It records what the sidebar asked of it.
type fakeNormal struct {
	snap      tmux.Snapshot
	snapErr   error
	active    string
	activeErr error
	showErr   error
	snapshots int      // how many times Snapshot ran
	shown     []string // "session" or "session:window", by id
}

func (n *fakeNormal) Snapshot() (tmux.Snapshot, error) {
	n.snapshots++
	return n.snap, n.snapErr
}

func (n *fakeNormal) ActiveSession() (string, error) { return n.active, n.activeErr }

// Show records what it showed and, as tmux does, makes the window it shows
// its session's current window.
func (n *fakeNormal) Show(session, window string) error {
	shown := session
	if window != "" {
		shown += ":" + window
	}
	n.shown = append(n.shown, shown)
	if n.showErr != nil || window == "" {
		return n.showErr
	}
	for _, s := range n.snap.Sessions {
		for i := range s.Windows {
			if s.ID == session {
				s.Windows[i].Active = s.Windows[i].ID == window
			}
		}
	}
	return nil
}

// fakeRepos knows which folders are in git repos: clones maps a folder to
// its main clone, and fails maps a folder to git's failure there. Any other
// folder isn't in a repo. It records the folders it was asked about.
type fakeRepos struct {
	clones map[string]string
	fails  map[string]error
	asked  []string
}

func (r *fakeRepos) MainClone(dir string) (string, error) {
	r.asked = append(r.asked, dir)
	return r.clones[dir], r.fails[dir]
}

// session is a session with id $<n>, named name, started in path, with
// windows named after wins, indexed from 0. The window whose name starts
// with * is the active one (its name without the *), else the first.
func session(n int, name, path string, wins ...string) tmux.Session {
	s := tmux.Session{ID: fmt.Sprintf("$%d", n), Name: name, Path: path}
	active := 0
	for i, w := range wins {
		if after, ok := strings.CutPrefix(w, "*"); ok {
			w, active = after, i
		}
		s.Windows = append(s.Windows, tmux.Window{ID: fmt.Sprintf("@%d%d", n, i), Index: i, Name: w})
	}
	if len(s.Windows) > 0 {
		s.Windows[active].Active = true
	}
	return s
}

// fakes are what a sidebar under test talks to.
type fakes struct {
	host   *fakeHost
	normal *fakeNormal
	repos  *fakeRepos
	wt     *fakeWt
}

// The fixture most tests use: a git project api (main clone /p/api, with a
// worktree /p/api--fix-login), a non-git project notes, and a git project
// Zed. fix-login ($2) is on the right.
func fixture() fakes {
	n := &fakeNormal{
		snap: tmux.Snapshot{Sessions: []tmux.Session{
			session(1, "api", "/p/api", "shell"),
			session(2, "fix-login", "/p/api--fix-login", "shell", "*dev"),
			session(3, "notes", "/home/u/notes", "vim"),
			session(4, "zed", "/p/Zed/src", "shell"),
			session(5, "Bugfix", "/p/api", "shell"),
		}},
		active: "$2",
	}
	r := &fakeRepos{clones: map[string]string{
		"/p/api": "/p/api", "/p/api--fix-login": "/p/api", "/p/Zed/src": "/p/Zed",
	}}
	return fakes{&fakeHost{}, n, r, &fakeWt{normal: n}}
}

func update(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

// run runs cmd, which must be there, and returns its message.
func run(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command, want one")
	}
	return cmd()
}

// feed runs cmd, which must be there, and gives the model what it yields:
// its message, or each message of a batch. It doesn't run what the model
// returns for them.
func feed(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	msg := run(t, cmd)
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				m, _ = update(t, m, c())
			}
		}
		return m
	}
	m, _ = update(t, m, msg)
	return m
}

// started is the sidebar once it has built its tree and read the active
// session.
func started(t *testing.T, f fakes) Model {
	t.Helper()
	m := New(Clients{Host: f.host, Normal: f.normal, Repos: f.repos, Wt: f.wt}, `C-\`)
	return feed(t, m, m.Init())
}

// plain is the view without its styles, each line without trailing spaces.
func plain(m Model) string {
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

// sgr is the SGR parameters line starts with ("" for none), e.g. "1;36".
func sgr(line string) string {
	rest, ok := strings.CutPrefix(line, "\x1b[")
	if !ok {
		return ""
	}
	params, _, _ := strings.Cut(rest, "m")
	return params
}

func hasParam(params, p string) bool { return slices.Contains(strings.Split(params, ";"), p) }

// cursor is the row the cursor is on, as it shows, or "" if none shows: the
// row in reverse video, under the header.
func cursor(t *testing.T, m Model) string {
	t.Helper()
	var on []string
	for _, line := range strings.Split(m.View().Content, "\n")[1:] {
		if hasParam(sgr(line), "7") {
			on = append(on, strings.TrimRight(ansi.Strip(line), " "))
		}
	}
	if len(on) > 1 {
		t.Fatalf("the cursor is on %d rows: %q", len(on), on)
	}
	if len(on) == 0 {
		return ""
	}
	return on[0]
}

// header is the header line as it shows, with its style.
func header(m Model) string { return strings.Split(m.View().Content, "\n")[0] }

func press(text string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: []rune(text)[0], Text: text} }

var (
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	ctrlB = tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl}
)

// Projects are alphabetical, a git project being its main clone's folder
// and a non-git one the session's folder; sessions are by name, and windows
// by index. Projects start unfolded and sessions folded, but for the active
// session, which shows its windows.
func TestTheTreeIsProjectsSessionsAndWindows(t *testing.T) {
	m := started(t, fixture())
	want := ` wtui
  ▾ api
    ▸ api
    ▸ Bugfix
    ▾ fix-login
        0 shell
        1 dev
  ▾ notes
    ▸ notes
  ▾ Zed
    ▸ zed`
	if got := plain(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
}

// Each start folder is looked up in git once, however many sessions started
// there and however often the tree rebuilds; r looks again.
func TestOneGitLookupPerStartFolder(t *testing.T) {
	f := fixture()
	m := started(t, f)
	want := []string{"/p/api", "/p/api--fix-login", "/home/u/notes", "/p/Zed/src"}
	if !slices.Equal(f.repos.asked, want) {
		t.Errorf("the sidebar looked up %q, want %q", f.repos.asked, want)
	}
	f.normal.snap.Sessions = append(f.normal.snap.Sessions, session(6, "docs", "/p/docs"))
	m, cmd := update(t, m, Rebuild{})
	m = feed(t, m, cmd)
	if want := append(want, "/p/docs"); !slices.Equal(f.repos.asked, want) {
		t.Errorf("after a rebuild, the sidebar looked up %q, want %q", f.repos.asked, want)
	}
	f.repos.asked = nil
	m, cmd = update(t, m, press("r"))
	feed(t, m, cmd)
	if want := append(want, "/p/docs"); !slices.Equal(f.repos.asked, want) {
		t.Errorf("after r, the sidebar looked up %q, want every folder again, %q", f.repos.asked, want)
	}
}

// A build runs in the background: a key returns at once. Asked for while
// one runs, another follows it, once, with the tree as it is then.
func TestBuildingRunsInTheBackground(t *testing.T) {
	f := fixture()
	m := started(t, f)
	before := f.normal.snapshots
	m, first := update(t, m, press("r"))
	if f.normal.snapshots != before || first == nil {
		t.Fatalf("r took %d snapshots before returning, and gave the build back as %v", f.normal.snapshots-before, first)
	}
	m, second := update(t, m, press("r"))
	m, third := update(t, m, Rebuild{})
	if second != nil || third != nil {
		t.Fatal("asked for while one runs, a build starts at once")
	}
	f.normal.snap.Sessions = f.normal.snap.Sessions[:1]
	m, next := update(t, m, run(t, first))
	if next == nil {
		t.Fatal("the build asked for while one ran doesn't follow it")
	}
	m, after := update(t, m, run(t, next))
	if after != nil {
		t.Error("two builds follow the one that ran, want one")
	}
	if got := plain(m); got != " wtui\n  ▾ api\n    ▸ api" {
		t.Errorf("after the following build, the view is:\n%s", got)
	}
}

// A start folder git fails on is a project of its own, and the status line
// says what git said; the rest of the tree is as usual.
func TestAFailedGitLookupShowsInTheStatusLine(t *testing.T) {
	f := fixture()
	f.normal.snap.Sessions = f.normal.snap.Sessions[1:3]
	delete(f.repos.clones, "/p/api--fix-login")
	f.repos.fails = map[string]error{"/p/api--fix-login": errors.New("git in /p/api--fix-login: fatal: dubious ownership")}
	m := started(t, f)
	want := ` wtui
  ▾ api--fix-login
    ▾ fix-login
        0 shell
        1 dev
  ▾ notes
    ▸ notes
git in /p/api--fix-login: fatal: dubious ownership`
	if got := plain(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
}

// If the snapshot fails, the tree stays as it was, and the status line says
// why; the next build that works clears it.
func TestAFailedSnapshotKeepsTheTree(t *testing.T) {
	f := fixture()
	f.normal.snap.Sessions = f.normal.snap.Sessions[2:3]
	m := started(t, f)
	f.normal.snapErr = errors.New("no server running on /tmp/tmux-501/default")
	m, cmd := update(t, m, Rebuild{})
	m = feed(t, m, cmd)
	want := " wtui\n  ▾ notes\n    ▸ notes\nno server running on /tmp/tmux-501/default"
	if got := plain(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
	f.normal.snapErr = nil
	m, cmd = update(t, m, Rebuild{})
	if got := plain(feed(t, m, cmd)); got != " wtui\n  ▾ notes\n    ▸ notes" {
		t.Errorf("after a build that works, the view is:\n%s", got)
	}
}

// Reading the active session can fail too; the status line says so once,
// whatever else failed the same way.
func TestAFailedReadOfTheActiveSessionShowsOnce(t *testing.T) {
	f := fixture()
	f.normal.snap.Sessions = f.normal.snap.Sessions[2:3]
	f.normal.snapErr = errors.New("no server running")
	f.normal.activeErr = errors.New("no server running")
	if got := plain(started(t, f)); got != " wtui\nno server running" {
		t.Errorf("view:\n%s", got)
	}
}

// The cursor shows only while the sidebar has focus, and the header shows
// which: in reverse video with focus, dim without. The sidebar starts with
// focus (the host gives it focus, and tmux doesn't say so). Getting focus
// puts the cursor on the active session.
func TestGettingFocusPutsTheCursorOnTheActiveSession(t *testing.T) {
	m := started(t, fixture())
	if got := cursor(t, m); got != "    ▾ fix-login" || !hasParam(sgr(header(m)), "7") {
		t.Errorf("at launch, the header is %q and the cursor on %q, want focus, on the active session, fix-login", header(m), got)
	}
	m, _ = update(t, m, tea.BlurMsg{})
	if got := cursor(t, m); got != "" {
		t.Errorf("without focus, the cursor shows on %q", got)
	}
	if params := sgr(header(m)); !hasParam(params, "2") || hasParam(params, "7") {
		t.Errorf("without focus, the header is %q, want it dim", header(m))
	}
	m, _ = update(t, m, tea.FocusMsg{})
	if got := cursor(t, m); got != "    ▾ fix-login" {
		t.Errorf("with focus, the cursor is on %q, want the active session, fix-login", got)
	}
	if params := sgr(header(m)); !hasParam(params, "7") {
		t.Errorf("with focus, the header is %q, want it in reverse video", header(m))
	}
	m, _ = update(t, m, tea.BlurMsg{})
	if got := cursor(t, m); got != "" {
		t.Errorf("after losing focus, the cursor shows on %q", got)
	}
}

// focused is the sidebar once it has started and got focus, so its cursor is
// on the active session.
func focused(t *testing.T, f fakes) Model {
	t.Helper()
	m, _ := update(t, started(t, f), tea.FocusMsg{})
	return m
}

// keys presses each key in turn and returns the sidebar after the last,
// running none of the commands they return.
func keys(t *testing.T, m Model, ks ...tea.KeyPressMsg) Model {
	t.Helper()
	for _, k := range ks {
		m, _ = update(t, m, k)
	}
	return m
}

var (
	up    = tea.KeyPressMsg{Code: tea.KeyUp}
	down  = tea.KeyPressMsg{Code: tea.KeyDown}
	left  = tea.KeyPressMsg{Code: tea.KeyLeft}
	right = tea.KeyPressMsg{Code: tea.KeyRight}
	space = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
)

// j/k and the arrows move the cursor a row at a time, as far as the first
// and last rows.
func TestCursorMovesUpAndDown(t *testing.T) {
	m := focused(t, fixture())
	for _, c := range []struct {
		key  tea.KeyPressMsg
		want string
	}{
		{press("j"), "        0 shell"},
		{down, "        1 dev"},
		{press("j"), "  ▾ notes"},
		{press("k"), "        1 dev"},
		{up, "        0 shell"},
		{up, "    ▾ fix-login"},
	} {
		m = keys(t, m, c.key)
		if got := cursor(t, m); got != c.want {
			t.Fatalf("after %s, the cursor is on %q, want %q", c.key, got, c.want)
		}
	}
	m = keys(t, m, press("k"), press("k"), press("k"), press("k"), press("k"))
	if got := cursor(t, m); got != "  ▾ api" {
		t.Errorf("past the first row, the cursor is on %q", got)
	}
	for range 20 {
		m = keys(t, m, press("j"))
	}
	if got := cursor(t, m); got != "    ▸ zed" {
		t.Errorf("past the last row, the cursor is on %q", got)
	}
}

// h/← folds a row, or on a folded row (or a window) goes to its parent; l/→
// unfolds a row, or on an unfolded one goes to its first child; space
// toggles. Windows have nothing to fold.
func TestFoldKeys(t *testing.T) {
	m := focused(t, fixture())
	for _, c := range []struct {
		key        tea.KeyPressMsg
		cursor     string
		rows, want string // the rows from the one starting with rows
	}{
		{press("h"), "    ▸ fix-login", "    ▸ fix-login", "    ▸ fix-login\n  ▾ notes"},
		{left, "  ▾ api", "", ""},
		{press("h"), "  ▸ api", "  ▸ api", "  ▸ api\n  ▾ notes"},
		{press("l"), "  ▾ api", "  ▾ api", "  ▾ api\n    ▸ api\n    ▸ Bugfix"},
		{right, "    ▸ api", "", ""},
		{press("l"), "    ▾ api", "    ▾ api", "    ▾ api\n        0 shell\n    ▸ Bugfix"},
		{press("l"), "        0 shell", "", ""},
		{press("l"), "        0 shell", "", ""},
		{space, "        0 shell", "    ▾ api", "    ▾ api\n        0 shell\n    ▸ Bugfix"},
		{press("h"), "    ▾ api", "", ""},
		{space, "    ▸ api", "    ▸ api", "    ▸ api\n    ▸ Bugfix"},
		{space, "    ▾ api", "    ▾ api", "    ▾ api\n        0 shell\n    ▸ Bugfix"},
	} {
		m = keys(t, m, c.key)
		if got := cursor(t, m); got != c.cursor {
			t.Fatalf("after %s, the cursor is on %q, want %q", c.key, got, c.cursor)
		}
		if c.rows == "" {
			continue
		}
		view := plain(m)
		i := strings.Index(view, c.rows)
		if i < 0 || !strings.HasPrefix(view[i:], c.want) {
			t.Fatalf("after %s, the view is:\n%s\nwant it to have:\n%s", c.key, view, c.want)
		}
	}
}

// enter on a project folds or unfolds it.
func TestEnterOnAProjectFoldsIt(t *testing.T) {
	f := fixture()
	m := keys(t, focused(t, f), press("j"), press("j"), press("j"))
	if got := cursor(t, m); got != "  ▾ notes" {
		t.Fatalf("the cursor is on %q, want notes", got)
	}
	m, cmd := update(t, m, enter)
	if cmd != nil || !strings.Contains(plain(m), "  ▸ notes\n  ▾ Zed") {
		t.Errorf("after enter on notes, the view is:\n%s", plain(m))
	}
	m, _ = update(t, m, enter)
	if !strings.Contains(plain(m), "  ▾ notes\n    ▸ notes\n  ▾ Zed") {
		t.Errorf("after enter on notes again, the view is:\n%s", plain(m))
	}
	if len(f.normal.shown) > 0 {
		t.Errorf("enter on a project showed %q", f.normal.shown)
	}
}

// enter on a session shows it on the right, and on a window shows its
// session at that window; then focus goes right.
func TestEnterShowsASessionOrAWindow(t *testing.T) {
	f := fixture()
	m := focused(t, f)
	m = keys(t, m, press("k")) // Bugfix
	_, cmd := update(t, m, enter)
	run(t, cmd)
	m = keys(t, m, press("j"), press("j"), press("j")) // fix-login's dev
	_, cmd = update(t, m, enter)
	run(t, cmd)
	if want := []string{"$5", "$2:@21"}; !slices.Equal(f.normal.shown, want) {
		t.Errorf("enter showed %q, want %q", f.normal.shown, want)
	}
	if want := []string{"focus right", "focus right"}; !slices.Equal(f.host.asked, want) {
		t.Errorf("enter asked the host %q, want %q", f.host.asked, want)
	}
}

// Once a window is shown, tmux says its session's windows changed, and the
// tree marks it as the active window; showing it rebuilds nothing.
func TestAShownWindowBecomesItsSessionsActiveOne(t *testing.T) {
	f := fixture()
	m := keys(t, focused(t, f), press("j")) // fix-login's shell
	m, cmd := update(t, m, enter)
	m, rebuild := update(t, m, run(t, cmd))
	if rebuild != nil {
		t.Error("showing a window rebuilds the tree")
	}
	m, _ = update(t, m, tmux.WindowsChanged{Session: "$2", Windows: f.normal.snap.Sessions[1].Windows})
	m, _ = update(t, m, tea.BlurMsg{}) // focus went right
	if got := styles(m); got["        0 shell"] != "1" || got["        1 dev"] != "" {
		t.Errorf("after showing fix-login's shell, its windows are styled %q and %q, want shell in bold",
			got["        0 shell"], got["        1 dev"])
	}
}

// If showing fails, focus stays, and the status line says why.
func TestAFailedShowStaysAndSaysWhy(t *testing.T) {
	f := fixture()
	f.normal.showErr = errors.New("can't find session: $5")
	m := keys(t, focused(t, f), press("k"))
	m, cmd := update(t, m, enter)
	m, _ = update(t, m, run(t, cmd))
	if len(f.host.asked) > 0 {
		t.Errorf("after a failed show, the sidebar asked the host %q", f.host.asked)
	}
	if got := plain(m); !strings.HasSuffix(got, "\ncan't find session: $5") {
		t.Errorf("view:\n%s\nwant it to end with the failure", got)
	}
}

// styles are the view's rows (under the header), each as its text and the
// SGR parameters it starts with.
func styles(m Model) map[string]string {
	got := map[string]string{}
	for _, line := range strings.Split(m.View().Content, "\n")[1:] {
		got[strings.TrimRight(ansi.Strip(line), " ")] = sgr(line)
	}
	return got
}

// The active session is in bold cyan and its project in cyan; each
// session's active window is in bold. When the active session changes, so
// does the highlight.
func TestTheActiveSessionAndItsProjectAreHighlighted(t *testing.T) {
	m, _ := update(t, started(t, fixture()), tea.BlurMsg{}) // no cursor

	want := map[string]string{
		"  ▾ api": "36", "    ▸ api": "", "    ▸ Bugfix": "",
		"    ▾ fix-login": "1;36", "        0 shell": "", "        1 dev": "1",
		"  ▾ notes": "", "    ▸ notes": "", "  ▾ Zed": "", "    ▸ zed": "",
	}
	if got := styles(m); !maps.Equal(got, want) {
		t.Errorf("the rows' styles are %q, want %q", got, want)
	}
	m, _ = update(t, m, Active{Session: "$3"})
	want = map[string]string{
		"  ▾ api": "", "    ▸ api": "", "    ▸ Bugfix": "", "    ▸ fix-login": "",
		"  ▾ notes": "36", "    ▾ notes": "1;36", "        0 vim": "1", "  ▾ Zed": "", "    ▸ zed": "",
	}
	if got := styles(m); !maps.Equal(got, want) {
		t.Errorf("with notes active, the rows' styles are %q, want %q", got, want)
	}
}

// When the active session changes, the new one unfolds and the old one folds
// again, unless it was unfolded by hand. Folds made by hand stick, through
// changes of active session and rebuilds.
func TestFoldsFollowTheActiveSessionUnlessMadeByHand(t *testing.T) {
	f := fixture()
	m := focused(t, f)
	// Bugfix unfolded by hand, then notes folded by hand
	m = keys(t, m, press("k"), press("l"))
	m = keys(t, m, press("j"), press("j"), press("j"), press("j"), press("j"), press("h"))
	m, _ = update(t, m, Active{Session: "$1"})
	want := ` wtui
  ▾ api
    ▾ api
        0 shell
    ▾ Bugfix
        0 shell
    ▸ fix-login
  ▸ notes
  ▾ Zed
    ▸ zed`
	if got := plain(m); got != want {
		t.Errorf("with api active, the view is:\n%s\nwant:\n%s", got, want)
	}
	m, cmd := update(t, m, Rebuild{})
	m = feed(t, m, cmd)
	m, _ = update(t, m, Active{Session: "$2"})
	want = ` wtui
  ▾ api
    ▸ api
    ▾ Bugfix
        0 shell
    ▾ fix-login
        0 shell
        1 dev
  ▸ notes
  ▾ Zed
    ▸ zed`
	if got := plain(m); got != want {
		t.Errorf("after a rebuild, with fix-login active again, the view is:\n%s\nwant:\n%s", got, want)
	}
}

// The active session folded by hand stays folded, even when it becomes
// active again.
func TestTheActiveSessionFoldedByHandStaysFolded(t *testing.T) {
	m := keys(t, focused(t, fixture()), press("h"))
	m, _ = update(t, m, Active{Session: "$1"})
	m, _ = update(t, m, Active{Session: "$2"})
	if got := plain(m); !strings.Contains(got, "    ▸ fix-login\n  ▾ notes") {
		t.Errorf("view:\n%s\nwant fix-login folded", got)
	}
}

// rebuilt is the sidebar after a build from f's sessions as they are now.
func rebuilt(t *testing.T, m Model, f fakes) Model {
	t.Helper()
	m, cmd := update(t, m, Rebuild{})
	return feed(t, m, cmd)
}

// without is sessions without the one with id.
func without(sessions []tmux.Session, id string) []tmux.Session {
	return slices.DeleteFunc(slices.Clone(sessions), func(s tmux.Session) bool { return s.ID == id })
}

// Across a rebuild, the cursor stays on the same session, window or
// project, wherever it is now and whatever it's called.
func TestTheCursorKeepsItsRowAcrossARebuild(t *testing.T) {
	f := fixture()
	m := keys(t, focused(t, f), press("j"), press("j")) // fix-login's dev
	sessions := f.normal.snap.Sessions
	sessions[1].Name = "a-renamed" // now first in api
	sessions[1].Windows[1].Name = "server"
	f.normal.snap.Sessions = append([]tmux.Session{session(7, "aardvark", "/p/aardvark")}, sessions...)
	m = rebuilt(t, m, f)
	if got := cursor(t, m); got != "        1 server" {
		t.Errorf("after fix-login's dev was renamed and moved, the cursor is on %q, want it there", got)
	}
	m = keys(t, m, press("k"), press("k"))
	m = rebuilt(t, m, f)
	if got := cursor(t, m); got != "    ▾ a-renamed" {
		t.Errorf("after a rebuild, the cursor is on %q, want a-renamed, the session it was on", got)
	}
}

// When the cursor's row is gone, the cursor goes to the row after it among
// its siblings, else the row before, else its parent; for a project, the
// project after it, else before.
func TestTheCursorMovesToTheNearestRowWhenItsRowIsGone(t *testing.T) {
	f := fixture()
	m := keys(t, focused(t, f), press("j"), press("j")) // fix-login's dev
	f.normal.snap.Sessions[1].Windows = f.normal.snap.Sessions[1].Windows[:1]
	f.normal.snap.Sessions[1].Windows[0].Active = true
	m = rebuilt(t, m, f)
	if got := cursor(t, m); got != "        0 shell" {
		t.Errorf("with dev gone, the cursor is on %q, want fix-login's other window", got)
	}
	m = keys(t, m, press("k"), press("k")) // Bugfix
	f.normal.snap.Sessions = without(f.normal.snap.Sessions, "$5")
	m = rebuilt(t, m, f)
	if got := cursor(t, m); got != "    ▾ fix-login" {
		t.Errorf("with Bugfix gone, the cursor is on %q, want the session after it, fix-login", got)
	}
	f.normal.snap.Sessions = without(f.normal.snap.Sessions, "$2")
	m = rebuilt(t, m, f)
	if got := cursor(t, m); got != "    ▸ api" {
		t.Errorf("with fix-login, the last in api, gone, the cursor is on %q, want the one before it, api", got)
	}
	f.normal.snap.Sessions = without(f.normal.snap.Sessions, "$1")
	m = rebuilt(t, m, f)
	if got := cursor(t, m); got != "  ▾ notes" {
		t.Errorf("with project api gone, the cursor is on %q, want the project after it, notes", got)
	}
	m = keys(t, m, press("j"), press("j"), press("j")) // zed
	f.normal.snap.Sessions = without(f.normal.snap.Sessions, "$4")
	m = rebuilt(t, m, f)
	if got := cursor(t, m); got != "  ▾ notes" {
		t.Errorf("with project Zed gone, the cursor is on %q, want the project before it, notes", got)
	}
}

// When the cursor's row folds away under it (here, the windows of a session
// no longer active), the cursor goes to the row it folded into.
func TestTheCursorStaysInSightWhenItsRowFoldsAway(t *testing.T) {
	m := keys(t, focused(t, fixture()), press("j")) // fix-login's shell
	m, _ = update(t, m, Active{Session: "$1"})
	if got := cursor(t, m); got != "    ▸ fix-login" {
		t.Errorf("with fix-login folded, the cursor is on %q, want fix-login", got)
	}
}

// At launch, focus comes before the tree and the active session are known,
// in either order; the cursor lands on the active session once they are.
func TestTheCursorLandsOnTheActiveSessionAtLaunch(t *testing.T) {
	for _, activeFirst := range []bool{true, false} {
		f := fixture()
		m := New(Clients{Host: f.host, Normal: f.normal, Repos: f.repos, Wt: f.wt}, `C-\`)
		m, _ = update(t, m, tea.FocusMsg{})
		batch, ok := run(t, m.Init()).(tea.BatchMsg)
		if !ok || len(batch) != 2 {
			t.Fatalf("Init gives %v, want a batch of two: the active session and the build", batch)
		}
		var msgs []tea.Msg
		for _, c := range batch {
			msgs = append(msgs, c())
		}
		if _, isActive := msgs[0].(Active); isActive != activeFirst {
			msgs[0], msgs[1] = msgs[1], msgs[0]
		}
		for _, msg := range msgs {
			m, _ = update(t, m, msg)
		}
		if got := cursor(t, m); got != "    ▾ fix-login" {
			t.Errorf("with the active session known first: %v, the cursor is on %q, want fix-login", activeFirst, got)
		}
	}
}

// Getting focus again puts the cursor back on the active session, wherever
// it was, unfolding the project if it was folded.
func TestGettingFocusAgainPutsTheCursorBack(t *testing.T) {
	m := keys(t, focused(t, fixture()), press("k"), press("k"), press("k"), press("h")) // api folded
	m, _ = update(t, m, tea.BlurMsg{})
	m, _ = update(t, m, tea.FocusMsg{})
	if got := cursor(t, m); got != "    ▾ fix-login" {
		t.Errorf("after focus came back, the cursor is on %q, want fix-login", got)
	}
}

// If the active session isn't in the tree yet when focus comes (e.g. it's
// new), the cursor goes to it when it shows up, unless a key moved it
// meanwhile.
func TestTheCursorWaitsForANewActiveSessionUnlessMoved(t *testing.T) {
	for _, moved := range []bool{false, true} {
		f := fixture()
		m := started(t, f)
		m, _ = update(t, m, Active{Session: "$7"})
		m, _ = update(t, m, tea.FocusMsg{})
		if moved {
			m = keys(t, m, press("j"))
		}
		f.normal.snap.Sessions = append(f.normal.snap.Sessions, session(7, "new", "/home/u/notes", "shell"))
		m = rebuilt(t, m, f)
		want := "    ▾ new"
		if moved {
			want = "  ▾ notes" // from fix-login, a row down
		}
		if got := cursor(t, m); got != want {
			t.Errorf("moved: %v; once the new session is in the tree, the cursor is on %q, want %q", moved, got, want)
		}
	}
}

// click is a left click on the sidebar's line y (0 is the header).
func click(y int) tea.MouseClickMsg { return tea.MouseClickMsg{X: 5, Y: y, Button: tea.MouseLeft} }

// Clicking a row moves the cursor there, and does nothing else. The
// sidebar asks for mouse events, and clicks off the rows do nothing.
func TestClickingARowMovesTheCursorThere(t *testing.T) {
	f := fixture()
	m := focused(t, f)
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Errorf("the sidebar's mouse mode is %v, want clicks", got)
	}
	m, cmd := update(t, m, click(7))
	if got := cursor(t, m); got != "  ▾ notes" || cmd != nil {
		t.Errorf("after a click on line 7, the cursor is on %q (and the click did %v), want notes", got, cmd)
	}
	for _, y := range []int{0, 11, 40} {
		m, _ = update(t, m, click(y))
		if got := cursor(t, m); got != "  ▾ notes" {
			t.Errorf("after a click on line %d, off the rows, the cursor is on %q", y, got)
		}
	}
	m, _ = update(t, m, tea.MouseClickMsg{X: 5, Y: 2, Button: tea.MouseRight})
	if got := cursor(t, m); got != "  ▾ notes" {
		t.Errorf("after a right click, the cursor is on %q", got)
	}
	if len(f.normal.shown) > 0 {
		t.Errorf("clicking showed %q", f.normal.shown)
	}
}

// The sidebar is narrow: lines that don't fit are cut short.
func TestLinesFitTheSidebar(t *testing.T) {
	f := fixture()
	f.normal.snap.Sessions[2].Name = "a-session-with-a-long-name"
	f.normal.snapErr = nil
	m := started(t, f)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 20, Height: 30})
	for _, line := range strings.Split(plain(m), "\n") {
		if w := ansi.StringWidth(line); w > 20 {
			t.Errorf("line %q is %d columns wide, wider than the sidebar's 20", line, w)
		}
	}
	if !strings.Contains(plain(m), "\n    ▸ a-session-wit…\n") {
		t.Errorf("the session's name isn't cut short:\n%s", plain(m))
	}
}

// In a short pane, the rows scroll to keep the cursor in sight, and the
// status line stays at the bottom. A click is on the row it's on.
func TestTheTreeScrollsInAShortPane(t *testing.T) {
	f := fixture()
	m := focused(t, f)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 30, Height: 6})
	want := " wtui\n  ▾ api\n    ▸ api\n    ▸ Bugfix\n    ▾ fix-login\n        0 shell"
	if got := plain(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
	m = keys(t, m, press("j"), press("j"), press("j")) // project notes
	want = " wtui\n    ▸ Bugfix\n    ▾ fix-login\n        0 shell\n        1 dev\n  ▾ notes"
	if got := plain(m); got != want {
		t.Errorf("with the cursor on notes, the view is:\n%s\nwant:\n%s", got, want)
	}
	m = keys(t, m, press("k"), press("k"), press("k"), press("k"), press("k")) // session api
	m, _ = update(t, m, click(2))
	if got := cursor(t, m); got != "    ▸ Bugfix" {
		t.Errorf("after a click on line 2, the cursor is on %q, want Bugfix", got)
	}
	f.normal.showErr = errors.New("can't find session")
	m, cmd := update(t, m, enter)
	m, _ = update(t, m, run(t, cmd))
	want = " wtui\n    ▸ api\n    ▸ Bugfix\n    ▾ fix-login\n        0 shell\ncan't find session"
	if got := plain(m); got != want {
		t.Errorf("with a status line, the view is:\n%s\nwant:\n%s", got, want)
	}
}

// Moving around the tree, folding and unfolding never changes what's on the
// right.
func TestMovingNeverChangesTheRightSide(t *testing.T) {
	f := fixture()
	m := focused(t, f)
	for _, k := range []tea.KeyPressMsg{press("j"), press("k"), down, up, press("l"), right, press("h"), left, space} {
		var cmd tea.Cmd
		if m, cmd = update(t, m, k); cmd != nil {
			cmd()
		}
	}
	if len(f.normal.shown) > 0 || len(f.host.asked) > 0 {
		t.Errorf("moving showed %q and asked the host %q", f.normal.shown, f.host.asked)
	}
}

func TestQClosesWtui(t *testing.T) {
	m := started(t, fixture())
	_, cmd := update(t, m, press("q"))
	if _, ok := run(t, cmd).(tea.QuitMsg); !ok {
		t.Error("q doesn't quit")
	}
}

func TestEscGoesRight(t *testing.T) {
	f := fixture()
	m := started(t, f)
	_, cmd := update(t, m, esc)
	run(t, cmd)
	if want := []string{"focus right"}; !slices.Equal(f.host.asked, want) {
		t.Errorf("esc asked the host %q, want %q", f.host.asked, want)
	}
}

// z hides the sidebar, which gives the right side the whole window, and
// focus.
func TestZHidesTheSidebar(t *testing.T) {
	f := fixture()
	m := started(t, f)
	_, cmd := update(t, m, press("z"))
	run(t, cmd)
	if want := []string{"hide"}; !slices.Equal(f.host.asked, want) {
		t.Errorf("z asked the host %q, want %q", f.host.asked, want)
	}
}

// C-b goes right and passes the prefix on, so C-b c makes a window there.
func TestTmuxPrefixGoesRightAndOn(t *testing.T) {
	f := fixture()
	m := started(t, f)
	_, cmd := update(t, m, ctrlB)
	run(t, cmd)
	if want := []string{"prefix right"}; !slices.Equal(f.host.asked, want) {
		t.Errorf("C-b asked the host %q, want %q", f.host.asked, want)
	}
}
