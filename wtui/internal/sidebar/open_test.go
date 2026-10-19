package sidebar

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// fakeOpener is the editor or the browser: it records what it opened (a
// folder or a URL), and fails with err if that's set.
type fakeOpener struct {
	opened []string
	err    error
}

func (o *fakeOpener) Open(what string) error {
	o.opened = append(o.opened, what)
	return o.err
}

// openers are the editor and the browser of a sidebar under test.
type openers struct{ editor, browser *fakeOpener }

// withOpeners is the sidebar of f, started with focus, so its cursor is on
// the active session, and with fake openers.
func withOpeners(t *testing.T, f fakes) (Model, openers) {
	t.Helper()
	o := openers{&fakeOpener{}, &fakeOpener{}}
	m := New(Clients{Host: f.host, Normal: f.normal, Repos: f.repos, Wt: f.wt, Editor: o.editor, Browser: o.browser}, `C-\`)
	m = feed(t, m, m.Init())
	m, _ = update(t, m, tea.FocusMsg{})
	return m, o
}

// e opens the editor, in the background, on the folder of the worktree the
// cursor's row is in: for a session, its worktree's folder, not the
// session's start folder (login started in a folder inside fix-login).
func TestEOpensTheEditorOnTheRowsWorktree(t *testing.T) {
	f := worktrees()
	m, o := withOpeners(t, f)
	if got := cursor(t, m); got != "    ▾ login" {
		t.Fatalf("the cursor is on %q, want login", got)
	}
	m, cmd := update(t, m, press("e"))
	if len(o.editor.opened) > 0 {
		t.Fatal("e opened the editor before returning, want it in the background")
	}
	update(t, m, run(t, cmd))
	if want := []string{"/p/api--fix-login"}; !slices.Equal(o.editor.opened, want) {
		t.Errorf("e opened the editor on %q, want %q", o.editor.opened, want)
	}
	if len(o.browser.opened) > 0 || len(f.normal.shown) > 0 || len(f.host.asked) > 0 {
		t.Errorf("e also opened %q in the browser, showed %q and asked the host %q", o.browser.opened, f.normal.shown, f.host.asked)
	}
}

// onEveryRow presses key with the cursor on each row that shows, in turn,
// and says what came of it for each, by the row's text: what opener opened
// ("" for nothing), and then what the status line says, if anything.
func onEveryRow(t *testing.T, m Model, key tea.KeyPressMsg, opener *fakeOpener) map[string]string {
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
		before, n := plain(at), len(opener.opened)
		after, cmd := update(t, at, key)
		if cmd != nil {
			after, _ = update(t, after, run(t, cmd))
		}
		var opened string
		if len(opener.opened) > n {
			opened = strings.Join(opener.opened[n:], ", ")
		}
		if said := strings.TrimPrefix(plain(after), before); said != "" {
			opened += " " + strings.TrimPrefix(said, "\n")
		}
		got[row] = opened
	}
	return got
}

// e opens the editor on the row's worktree or main clone, whatever its
// row: a project's row is its main clone's. In a non-git project, or for a
// session in no worktree wt lists (e.g. one made since), it's the session's
// start folder. A window's is its session's.
func TestEOpensTheEditorOnTheRowsFolder(t *testing.T) {
	f := worktrees()
	f.normal.snap.Sessions = append(f.normal.snap.Sessions, session(7, "spike", "/p/api--spike/web", "shell"))
	f.repos.clones["/p/api--spike/web"] = "/p/api"
	m, o := withOpeners(t, f)
	want := map[string]string{
		"  ▾ api":             "/p/api",
		"    ▾ main":          "/p/api",
		"      ▸ api":         "/p/api",
		"      ▸ Bugfix":      "/p/api",
		"    ▾ login":         "/p/api--fix-login",
		"        0 shell":     "/p/api--fix-login",
		"        1 dev":       "/p/api--fix-login",
		"      docs":          "/p/api--docs",
		"      api--rebase ±": "/p/api--rebase",
		"    ▸ spike":         "/p/api--spike/web",
		"  ▾ notes":           "/home/u/notes",
		"    ▸ notes":         "/home/u/notes",
		"  ▾ Zed":             "/p/Zed",
		"    ▸ zed":           "/p/Zed",
		"    ▸ agent":         "/p/Zed/.claude/worktrees/agent",
	}
	got := onEveryRow(t, m, press("e"), o.editor)
	delete(got, "      api--gone ✗") // missing: see the hints' test
	if !maps.Equal(got, want) {
		t.Errorf("e opened the editor on %q, want %q", got, want)
	}
	// the windows of a session in a worktree's row, and in a non-git
	// project: their session's folder
	for _, c := range []struct{ active, want string }{{"$1", "/p/api"}, {"$3", "/home/u/notes"}} {
		m, _ = update(t, m, Active{Session: c.active})
		m, _ = update(t, m, tea.FocusMsg{}) // the cursor on it
		m = keys(t, m, press("j"))
		if got := strings.TrimSpace(cursor(t, m)); got != "0 shell" && got != "0 vim" {
			t.Fatalf("with %s active, the cursor is on %q, want its first window", c.active, got)
		}
		o.editor.opened = nil
		m2, cmd := update(t, m, press("e"))
		update(t, m2, run(t, cmd))
		if want := []string{c.want}; !slices.Equal(o.editor.opened, want) {
			t.Errorf("e on %s's window opened the editor on %q, want %q", c.active, o.editor.opened, want)
		}
	}
}

// toRow is the sidebar with the cursor moved down to the row that shows as
// row; the test fails if there's none.
func toRow(t *testing.T, m Model, row string) Model {
	t.Helper()
	for range len(m.rows()) {
		m = keys(t, m, press("k"))
	}
	for range len(m.rows()) {
		if cursor(t, m) == row {
			return m
		}
		m = keys(t, m, press("j"))
	}
	t.Fatalf("no row %q in:\n%s", row, plain(m))
	return m
}

// A key that doesn't apply to the cursor's row shows a short hint in the
// status line instead, and does nothing else: e on a missing worktree has no
// folder to open. The next key, or a click, clears the hint.
func TestAKeyThatDoesntApplyShowsAHint(t *testing.T) {
	f := worktrees()
	m, o := withOpeners(t, f)
	m = toRow(t, m, "      api--gone ✗")
	before := plain(m)
	m, cmd := update(t, m, press("e"))
	if cmd != nil || len(o.editor.opened) > 0 || len(f.normal.shown) > 0 || len(f.host.asked) > 0 {
		t.Errorf("e on a missing worktree did %v, opened %q, showed %q and asked the host %q", cmd, o.editor.opened, f.normal.shown, f.host.asked)
	}
	if got, want := plain(m), before+"\napi--gone is missing"; got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
	if m = keys(t, m, press("k")); strings.Contains(plain(m), "is missing") {
		t.Errorf("after the next key, the view still has the hint:\n%s", plain(m))
	}
	m = keys(t, m, press("j"), press("e"))
	if m, _ = update(t, m, tea.MouseClickMsg{Button: tea.MouseLeft, Y: 1}); strings.Contains(plain(m), "is missing") {
		t.Errorf("after a click, the view still has the hint:\n%s", plain(m))
	}
}

// If the editor or the browser fails to open (e.g. there's no such editor),
// the status line says why, until the next action works.
func TestAFailedOpenShowsInTheStatusLine(t *testing.T) {
	f := devURLs()
	m, o := withOpeners(t, f) // on login, which has a dev URL
	for _, c := range []struct {
		key    string
		opener *fakeOpener
		err    error
	}{
		{"e", o.editor, errors.New("can't open the editor: sh: zed: command not found")},
		{"o", o.browser, errors.New("can't open http://localhost:3001: no xdg-open on PATH")},
	} {
		c.opener.err = c.err
		next, cmd := update(t, m, press(c.key))
		next, _ = update(t, next, run(t, cmd))
		if got := plain(next); !strings.HasSuffix(got, "\n"+c.err.Error()) {
			t.Errorf("after a failed %s, the view is:\n%s\nwant it to end with %q", c.key, got, c.err)
		}
		c.opener.err = nil
		next, cmd = update(t, next, press("o"))
		if next, _ = update(t, next, run(t, cmd)); strings.Contains(plain(next), "can't open") {
			t.Errorf("after an o that works, the view still says why %s failed:\n%s", c.key, plain(next))
		}
	}
}

// devURLs is the worktrees fixture with dev URLs: fix-login's, docs's (it
// has no session), and Zed's main clone's and agent's.
func devURLs() fakes {
	f := worktrees()
	api, zed := f.wt.lists["/p/api"], f.wt.lists["/p/Zed"]
	api[1].DevURL = "http://localhost:3001"
	api[2].DevURL = "http://localhost:3002"
	zed[0].DevURL = "http://localhost:3000"
	zed[1].DevURL = "http://localhost:3005/app"
	return f
}

// o opens the dev URL of the row's worktree or main clone in the browser,
// whatever its row: a project's row is its main clone's. A worktree without
// one, and a row in none (in a non-git project, or a session in no worktree
// wt lists), show a hint; so does a missing worktree.
func TestOOpensTheRowsDevURL(t *testing.T) {
	f := devURLs()
	f.normal.snap.Sessions = append(f.normal.snap.Sessions, session(7, "spike", "/p/api--spike/web", "shell"))
	f.repos.clones["/p/api--spike/web"] = "/p/api"
	m, o := withOpeners(t, f)
	want := map[string]string{
		"  ▾ api":             " api has no dev URL",
		"    ▾ main":          " api has no dev URL",
		"      ▸ api":         " api has no dev URL",
		"      ▸ Bugfix":      " api has no dev URL",
		"    ▾ login :3001":   "http://localhost:3001",
		"        0 shell":     "http://localhost:3001",
		"        1 dev":       "http://localhost:3001",
		"      docs :3002":    "http://localhost:3002",
		"      api--rebase ±": " api--rebase has no dev URL",
		"      api--gone ✗":   " api--gone is missing",
		"    ▸ spike":         " spike has no dev URL",
		"  ▾ notes":           " notes has no dev URL",
		"    ▸ notes":         " notes has no dev URL",
		"  ▾ Zed":             "http://localhost:3000",
		"    ▸ zed :3000":     "http://localhost:3000",
		"    ▸ agent :3005":   "http://localhost:3005/app",
	}
	if got := onEveryRow(t, m, press("o"), o.browser); !maps.Equal(got, want) {
		t.Errorf("o opened %q, want %q", got, want)
	}
	if len(o.editor.opened) > 0 || len(f.normal.shown) > 0 || len(f.host.asked) > 0 {
		t.Errorf("o also opened the editor on %q, showed %q and asked the host %q", o.editor.opened, f.normal.shown, f.host.asked)
	}
}
