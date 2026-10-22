package sidebar

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rokbar/tmux-kit/wtui/internal/dialog"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// Popup records the popup, and in its host's log, if it has a host, the
// command it pops up over the right side, in its folder, unless it fails
// with popupErr. Then onPopup, if set, plays the dialog in it; as tmux's
// does, it returns once that has exited.
func (n *fakeNormal) Popup(p tmux.Popup) error {
	n.popups = append(n.popups, p)
	if n.popupErr != nil {
		return n.popupErr
	}
	if n.host != nil {
		n.host.asked = append(n.host.asked, "popup in "+p.Dir+": "+strings.Join(p.Command, " "))
	}
	if n.onPopup != nil {
		n.onPopup(p)
	}
	return nil
}

// Popup records the popup over the whole host, as "popup <width>x<height>
// <title>: <command>".
func (h *fakeHost) Popup(p tmux.Popup, width, height int) error {
	h.asked = append(h.asked, fmt.Sprintf("popup %dx%d %s: %s", width, height, p.Title, strings.Join(p.Command, " ")))
	return nil
}

// says plays a dialog that says text, through what dialog.Say writes to.
func says(t *testing.T, text string) func(tmux.Popup) {
	return func(p tmux.Popup) {
		for _, kv := range p.Env {
			if file, ok := strings.CutPrefix(kv, "WTUI_SAID="); ok {
				if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
		t.Fatalf("the popup %+v has nowhere for its dialog to say anything", p)
	}
}

// drain runs cmd and gives the model what it yields: its message, or each
// message of a batch, batches in it too. It doesn't run what the model
// returns for them.
func drain(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = drain(t, m, c)
		}
		return m
	}
	m, _ = update(t, m, msg)
	return m
}

// wtui is the wtui the dialogs are subcommands of, in these tests.
const wtui = "/bin/wtui"

// withDialogs is the sidebar of f, started with focus, so its cursor is on
// the active session, with wtui at its path.
func withDialogs(t *testing.T, f fakes) Model {
	t.Helper()
	m := New(Clients{Host: f.host, Normal: f.normal, Repos: f.repos, Wt: f.wt, Wtui: wtui}, `C-\`)
	m = feed(t, m, m.Init())
	m, _ = update(t, m, tea.FocusMsg{})
	return m
}

// popupOnEveryRow presses key with the cursor on each row that shows, in
// turn, and says what came of it for each, by the row's text: the popup it
// opened on the nested client, as "<folder>: <command>", or the hint the
// status line shows.
func popupOnEveryRow(t *testing.T, m Model, key tea.KeyPressMsg, f fakes) map[string]string {
	t.Helper()
	got := map[string]string{}
	top := m
	for range len(m.rows()) {
		top = keys(t, top, press("k"))
	}
	for i := range len(m.rows()) {
		at := top
		for range i {
			at = keys(t, at, press("j"))
		}
		row := cursor(t, at)
		before, n := plain(at), len(f.normal.popups)
		after, cmd := update(t, at, key)
		if cmd != nil {
			after, _ = update(t, after, run(t, cmd))
		}
		var opened []string
		for _, p := range f.normal.popups[n:] {
			opened = append(opened, p.Dir+": "+strings.Join(p.Command, " "))
		}
		if said := strings.TrimPrefix(plain(after), before); said != "" {
			opened = append(opened, strings.TrimPrefix(said, "\n"))
		}
		got[row] = strings.Join(opened, ", ")
	}
	return got
}

// i shows the details of the worktree the cursor's row is in, in a popup on
// the nested client: wtui's details dialog, given the worktree's branch and
// dev URL as wt listed them. A project's row is its main clone's. A missing
// worktree, and a row in none (a non-git project, a session in no worktree wt
// lists), get a hint.
func TestIShowsTheRowsDetails(t *testing.T) {
	f := devURLs()
	f.normal.snap.Sessions = append(f.normal.snap.Sessions, session(7, "spike", "/p/api--spike/web", "shell"))
	f.repos.clones["/p/api--spike/web"] = "/p/api"
	m := withDialogs(t, f)
	api := "/p/api: /bin/wtui details /p/api main "
	login := "/p/api--fix-login: /bin/wtui details /p/api--fix-login fix-login http://localhost:3001"
	zed := "/p/Zed: /bin/wtui details /p/Zed main http://localhost:3000"
	want := map[string]string{
		"  ▾ api":             api,
		"    ▾ main":          api,
		"1     ▸ api":         api,
		"2     ▸ Bugfix":      api,
		"3   ▾ login :3001":   login,
		"        0 shell":     login,
		"        1 dev":       login,
		"      docs :3002":    "/p/api--docs: /bin/wtui details /p/api--docs docs http://localhost:3002",
		"      api--rebase ±": "/p/api--rebase: /bin/wtui details /p/api--rebase  ",
		"      api--gone ✗":   "api--gone is missing",
		"4   ▸ spike":         "spike isn't in a git repo",
		"  ▾ notes":           "notes isn't in a git repo",
		"5   ▸ notes":         "notes isn't in a git repo",
		"  ▾ Zed":             zed,
		"6   ▸ zed :3000":     zed,
		"7   ▸ agent :3005":   "/p/Zed/.claude/worktrees/agent: /bin/wtui details /p/Zed/.claude/worktrees/agent agent-1 http://localhost:3005/app",
	}
	if got := popupOnEveryRow(t, m, press("i"), f); !maps.Equal(got, want) {
		t.Errorf("i opened %q, want %q", got, want)
	}
	if len(f.normal.shown) > 0 {
		t.Errorf("i showed %q on the right, want nothing shown", f.normal.shown)
	}
	for _, p := range f.normal.popups {
		if p.Title != " details of "+p.Dir[strings.LastIndex(p.Dir, "/")+1:]+" " {
			t.Errorf("the details of %s are titled %q", p.Dir, p.Title)
		}
	}
}

// n opens the picker in a popup on the nested client, in the folder of the
// worktree the cursor's row is in, which wt takes as the invoking worktree.
// The same rows as i's get a hint.
func TestNRunsThePickerInTheRowsWorktree(t *testing.T) {
	f := worktrees()
	f.normal.snap.Sessions = append(f.normal.snap.Sessions, session(7, "spike", "/p/api--spike/web", "shell"))
	f.repos.clones["/p/api--spike/web"] = "/p/api"
	m := withDialogs(t, f)
	api, login, zed := "/p/api: /bin/wtui pick", "/p/api--fix-login: /bin/wtui pick", "/p/Zed: /bin/wtui pick"
	want := map[string]string{
		"  ▾ api":             api,
		"    ▾ main":          api,
		"1     ▸ api":         api,
		"2     ▸ Bugfix":      api,
		"3   ▾ login":         login,
		"        0 shell":     login,
		"        1 dev":       login,
		"      docs":          "/p/api--docs: /bin/wtui pick",
		"      api--rebase ±": "/p/api--rebase: /bin/wtui pick",
		"      api--gone ✗":   "api--gone is missing",
		"4   ▸ spike":         "spike isn't in a git repo",
		"  ▾ notes":           "notes isn't in a git repo",
		"5   ▸ notes":         "notes isn't in a git repo",
		"  ▾ Zed":             zed,
		"6   ▸ zed":           zed,
		"7   ▸ agent":         "/p/Zed/.claude/worktrees/agent: /bin/wtui pick",
	}
	if got := popupOnEveryRow(t, m, press("n"), f); !maps.Equal(got, want) {
		t.Errorf("n opened %q, want %q", got, want)
	}
	for _, p := range f.normal.popups {
		if p.Title != " picker " {
			t.Errorf("the picker is titled %q", p.Title)
		}
	}
}

// A dialog runs in the background: focus goes right, to the popup on the
// nested client, which takes the keys typed until it closes; then focus
// comes back to the sidebar. The sidebar stays responsive meanwhile. What the
// dialog said shows in the status line (here, wt's failure in the picker),
// and the markers refresh, as after any action.
func TestADialogOpensOnTheRightAndFocusComesBack(t *testing.T) {
	f := worktrees()
	m := withDialogs(t, f)
	var askedThen []string
	f.normal.onPopup = func(p tmux.Popup) {
		askedThen = slices.Clone(f.host.asked)
		says(t, "wt: fatal: 'a b' is not a valid branch name")(p)
	}
	m, cmd := update(t, m, press("n"))
	if len(f.normal.popups) > 0 || len(f.host.asked) > 0 {
		t.Fatalf("n opened %d popups and asked the host %q before returning, want it all in the background", len(f.normal.popups), f.host.asked)
	}
	m = keys(t, m, press("j")) // while it's open
	m, refresh := update(t, m, run(t, cmd))
	listed := len(f.wt.listed)
	drain(t, m, refresh)
	if want := []string{"focus right"}; !slices.Equal(askedThen, want) {
		t.Errorf("while the popup was open, the sidebar had asked the host %q, want %q", askedThen, want)
	}
	if want := []string{"focus right", "focus sidebar"}; !slices.Equal(f.host.asked, want) {
		t.Errorf("the sidebar asked the host %q, want %q", f.host.asked, want)
	}
	if got := plain(m); !strings.HasSuffix(got, "\nwt: fatal: 'a b' is not a valid branch name") {
		t.Errorf("after the picker failed, the view is:\n%s", got)
	}
	if len(f.wt.listed) == listed {
		t.Error("the markers didn't refresh after the picker")
	}

	// cancelled: the picker says nothing, so the status line clears
	f.normal.onPopup = nil
	m, cmd = update(t, m, press("n"))
	if m = feed(t, m, cmd); strings.Contains(plain(m), "fatal") {
		t.Errorf("after the picker was cancelled, the view is:\n%s", plain(m))
	}
}

// What a dialog opens (here, the picker, a new session) is on the right once
// it closes. The sidebar gets focus back then, which can reach it before
// tmux says what the right side shows: either way, the cursor ends on that
// session.
func TestAfterADialogTheCursorIsOnWhatTheRightSideShows(t *testing.T) {
	f := worktrees()
	m := withDialogs(t, f)
	m, cmd := update(t, m, press("n"))
	msg := run(t, cmd)
	f.normal.active = "$1" // the picker showed api
	m, _ = update(t, m, tea.BlurMsg{})
	m, _ = update(t, m, tea.FocusMsg{}) // before tmux says so
	m, next := update(t, m, msg)
	if next != nil {
		m = drain(t, m, next)
	}
	if got := cursor(t, m); got != "1     ▾ api" {
		t.Errorf("after the picker showed api, the cursor is on %q, want api", got)
	}
}

// If the popup can't open, the status line says why, and focus still comes
// back to the sidebar.
func TestADialogThatCantOpenSaysWhy(t *testing.T) {
	f := worktrees()
	m := withDialogs(t, f)
	f.normal.popupErr = errors.New("can't find client: /dev/ttys999")
	m, cmd := update(t, m, press("i"))
	m = feed(t, m, cmd)
	if got := plain(m); !strings.HasSuffix(got, "\ncan't find client: /dev/ttys999") {
		t.Errorf("the view is:\n%s", got)
	}
	if want := []string{"focus right", "focus sidebar"}; !slices.Equal(f.host.asked, want) {
		t.Errorf("the sidebar asked the host %q, want %q", f.host.asked, want)
	}
}

// ? shows every shortcut in a popup over the whole host, sized to fit them,
// where focus stays: wtui's page dialog.
func TestQuestionMarkShowsEveryShortcut(t *testing.T) {
	f := fixture()
	m := withDialogs(t, f)
	m, cmd := update(t, m, press("?"))
	feed(t, m, cmd)
	w, h := dialog.PageSize(Shortcuts(`C-\`))
	if want := []string{fmt.Sprintf(`popup %dx%d  shortcuts : /bin/wtui shortcuts C-\`, w, h)}; !slices.Equal(f.host.asked, want) {
		t.Errorf("? asked the host %q, want %q", f.host.asked, want)
	}
	if len(f.normal.popups) > 0 {
		t.Errorf("? opened a popup on the nested client too")
	}
}

// The shortcuts are every sidebar key, from the same list the host binds
// them from, each with what it does: alone in the sidebar, and after the
// wtui key in a session. The wtui key comes first.
func TestTheShortcutsAreEverySidebarKey(t *testing.T) {
	lines := Shortcuts(`C-\`)
	panel := strings.Join(lines, "\n")
	for _, want := range []string{
		"In the sidebar, press a key alone. In a session, press C-\\",
		"\nC-\\           switch sides, showing a hidden sidebar\n",
		"\nj down        down\n",
		"\n1-9           show the session with that number on the right\n",
		"\nC-b           tmux's prefix, on the right side\n",
		"\nenter         show on the right, opening a worktree's session if it\n              has none; fold or unfold a project or worktree\n",
		"\n?             every shortcut\n",
	} {
		if !strings.Contains(panel, want) {
			t.Errorf("the shortcuts don't have %q:\n%s", want, panel)
		}
	}
	for _, a := range actions {
		label := keyLabel(a.keys)
		if !slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, label+" ") }) {
			t.Errorf("no line for %s (%s):\n%s", label, a.help, panel)
		}
	}
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > shortcutsWidth {
			t.Errorf("%q is %d columns wide, more than %d", l, w, shortcutsWidth)
		}
	}
}

// The keys that open popups aren't held: the host would hold the keys typed
// into the popup until the key was done, which is once the popup has closed.
func TestPopupKeysArentHeld(t *testing.T) {
	found := 0
	for _, k := range Keys() {
		if slices.Contains([]string{"i", "n", "?"}, k.Name) {
			found++
			if k.Held || k.Tmux != k.Name {
				t.Errorf("the host binds %q as %+v, want it passed on to the sidebar", k.Name, k)
			}
		}
	}
	if found != 3 {
		t.Errorf("the host binds %d of i, n and ?", found)
	}
}

// Once the sidebar knows its height, its last line says ? shows the
// shortcuts, under the status line.
func TestTheLastLineSaysQuestionMarkShowsTheShortcuts(t *testing.T) {
	f := worktrees()
	m := withDialogs(t, f)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 30, Height: 20})
	lines := strings.Split(plain(m), "\n")
	if len(lines) != 20 || lines[19] != "? shortcuts" {
		t.Errorf("the view's %d lines end %q, want 20 ending with ? shortcuts", len(lines), lines[len(lines)-1])
	}
	m = keys(t, toRow(t, m, "      api--gone ✗"), press("i"))
	lines = strings.Split(plain(m), "\n")
	if got := lines[len(lines)-2:]; !slices.Equal(got, []string{"api--gone is missing", "? shortcuts"}) {
		t.Errorf("with a hint, the view ends %q", got)
	}
}
