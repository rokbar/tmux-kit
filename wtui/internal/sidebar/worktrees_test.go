package sidebar

import (
	"errors"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// fakeWt is wt for the git projects it has lists for, by main clone; fails
// has a project's failure to list instead. A project it has neither for lists
// nothing, so it shows as tmux sees it. Opening a worktree starts a session
// there on normal, named after its branch, else its folder, as wt does. It
// records what it listed and opened.
type fakeWt struct {
	normal  *fakeNormal
	lists   map[string][]wt.Worktree
	fails   map[string]error
	openErr error
	listed  []string // main clones
	opened  []string // "<main clone> <path>"
}

func (w *fakeWt) List(mainClone string) ([]wt.Worktree, error) {
	w.listed = append(w.listed, mainClone)
	if err := w.fails[mainClone]; err != nil {
		return nil, err
	}
	return w.lists[mainClone], nil
}

func (w *fakeWt) Open(mainClone, path string) error {
	w.opened = append(w.opened, mainClone+" "+path)
	if w.openErr != nil {
		return w.openErr
	}
	for _, r := range w.lists[mainClone] {
		if r.Path == path {
			name := r.Branch
			if name == "" {
				name = filepath.Base(path)
			}
			w.normal.snap.Sessions = append(w.normal.snap.Sessions, session(90+len(w.opened), name, path, "shell"))
		}
	}
	return nil
}

// wtFailure is a failure of wt's of kind (e.g. wt.ErrTooOld), with what wt
// said as its text.
type wtFailure struct {
	kind error
	msg  string
}

func (f wtFailure) Error() string { return f.msg }
func (f wtFailure) Unwrap() error { return f.kind }

// The fixture for worktrees: git project api, whose main clone has two
// sessions (api and Bugfix); its worktree fix-login one, login, started in a
// folder in it, and on the right; and worktrees docs (no session), one
// detached mid-rebase (no session) and one missing. The non-git project
// notes. Git project Zed, with a session in its main clone and one in a
// worktree inside the main clone's folder.
func worktrees() fakes {
	n := &fakeNormal{
		snap: tmux.Snapshot{Sessions: []tmux.Session{
			session(1, "api", "/p/api", "shell"),
			session(2, "login", "/p/api--fix-login/web", "shell", "*dev"),
			session(3, "notes", "/home/u/notes", "vim"),
			session(4, "zed", "/p/Zed/src", "shell"),
			session(5, "Bugfix", "/p/api", "shell"),
			session(6, "agent", "/p/Zed/.claude/worktrees/agent", "claude"),
		}},
		active: "$2",
	}
	r := &fakeRepos{clones: map[string]string{
		"/p/api": "/p/api", "/p/api--fix-login/web": "/p/api", "/p/api--docs": "/p/api", "/p/api--rebase": "/p/api",
		"/p/Zed/src": "/p/Zed", "/p/Zed/.claude/worktrees/agent": "/p/Zed",
	}}
	w := &fakeWt{normal: n, lists: map[string][]wt.Worktree{
		"/p/api": {
			{Path: "/p/api", Branch: "main", State: wt.Clean, MainClone: true},
			{Path: "/p/api--fix-login", Branch: "fix-login", State: wt.Clean},
			{Path: "/p/api--docs", Branch: "docs", State: wt.Clean},
			{Path: "/p/api--rebase", State: wt.Dirty},
			{Path: "/p/api--gone", State: wt.Missing},
		},
		"/p/Zed": {
			{Path: "/p/Zed", Branch: "main", State: wt.Clean, MainClone: true},
			{Path: "/p/Zed/.claude/worktrees/agent", Branch: "agent-1", State: wt.Clean},
		},
	}}
	return fakes{&fakeHost{}, n, r, w}
}

// A git project lists its main clone first, then its worktrees in git's
// order. A session belongs to the worktree whose folder holds its start
// folder, the longest such (Zed's agent is in a worktree inside the main
// clone's folder); the main clone counts as one. A worktree with one session
// is that session's row, named after it; with several, a row named after
// its branch, with its sessions under it by name; with none, a row named
// after its branch, else its folder. A missing one is marked ✗.
func TestAGitProjectListsItsMainCloneThenItsWorktrees(t *testing.T) {
	m := started(t, worktrees())
	want := ` wtui
  ▾ api
    ▾ main
      ▸ api
      ▸ Bugfix
    ▾ login
        0 shell
        1 dev
      docs
      api--rebase
      api--gone ✗
  ▾ notes
    ▸ notes
  ▾ Zed
    ▸ zed
    ▸ agent`
	if got := plain(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
}

// Worktrees without a session are dimmed, and so are missing ones. The
// active session's worktree is highlighted, with its project.
func TestWorktreesWithoutASessionAreDimmed(t *testing.T) {
	f := worktrees()
	m, _ := update(t, started(t, f), tea.BlurMsg{}) // no cursor
	want := map[string]string{
		"  ▾ api": "36", "    ▾ main": "", "      ▸ api": "", "      ▸ Bugfix": "",
		"    ▾ login": "1;36", "        0 shell": "", "        1 dev": "1",
		"      docs": "2", "      api--rebase": "2", "      api--gone ✗": "2",
		"  ▾ notes": "", "    ▸ notes": "", "  ▾ Zed": "", "    ▸ zed": "", "    ▸ agent": "",
	}
	if got := styles(m); !maps.Equal(got, want) {
		t.Errorf("the rows' styles are %q, want %q", got, want)
	}
	m, _ = update(t, m, Active{Session: "$5"})
	if got := styles(m); got["  ▾ api"] != "36" || got["    ▾ main"] != "36" || got["      ▾ Bugfix"] != "1;36" {
		t.Errorf("with Bugfix active, api is styled %q, main %q and Bugfix %q, want all highlighted",
			got["  ▾ api"], got["    ▾ main"], got["      ▾ Bugfix"])
	}
}

// enter on a worktree's row with sessions under it folds or unfolds it, as
// on a project, and on a missing worktree does nothing: it has nothing to
// show.
func TestEnterOnAWorktreeWithSessionsFoldsIt(t *testing.T) {
	f := worktrees()
	m := keys(t, focused(t, f), press("k"), press("k"), press("k")) // main
	if got := cursor(t, m); got != "    ▾ main" {
		t.Fatalf("the cursor is on %q, want main", got)
	}
	m, cmd := update(t, m, enter)
	if cmd != nil || !strings.Contains(plain(m), "    ▸ main\n    ▾ login") {
		t.Errorf("after enter on main, the view is:\n%s", plain(m))
	}
	m, _ = update(t, m, enter)
	if !strings.Contains(plain(m), "    ▾ main\n      ▸ api\n      ▸ Bugfix\n    ▾ login") {
		t.Errorf("after enter on main again, the view is:\n%s", plain(m))
	}
	for range 8 {
		m = keys(t, m, press("j")) // to api--gone
	}
	if got := cursor(t, m); got != "      api--gone ✗" {
		t.Fatalf("the cursor is on %q, want api--gone", got)
	}
	before := plain(m)
	if m, cmd = update(t, m, enter); cmd != nil || plain(m) != before {
		t.Errorf("enter on a missing worktree did %v, and the view went from:\n%s\nto:\n%s", cmd, before, plain(m))
	}
	if len(f.normal.shown) > 0 || len(f.wt.opened) > 0 || len(f.host.asked) > 0 {
		t.Errorf("enter showed %q, opened %q and asked the host %q", f.normal.shown, f.wt.opened, f.host.asked)
	}
}

// enter on a worktree without a session has wt open it, in its project's
// main clone, then shows the session wt started there (found by its
// folder) and moves focus right. The cursor stays on the worktree's row,
// now its session's.
func TestEnterOnAWorktreeWithoutASessionOpensItThenShowsIt(t *testing.T) {
	f := worktrees()
	m := keys(t, focused(t, f), press("j"), press("j"), press("j")) // docs
	if got := cursor(t, m); got != "      docs" {
		t.Fatalf("the cursor is on %q, want docs", got)
	}
	m, cmd := update(t, m, enter)
	if len(f.wt.opened) > 0 {
		t.Fatal("enter opened the worktree before returning, want it in the background")
	}
	m, rebuild := update(t, m, run(t, cmd))
	m = feed(t, m, rebuild)
	if want := []string{"/p/api /p/api--docs"}; !slices.Equal(f.wt.opened, want) {
		t.Errorf("enter opened %q, want %q", f.wt.opened, want)
	}
	if want := []string{"$91"}; !slices.Equal(f.normal.shown, want) {
		t.Errorf("enter showed %q, want %q, the session wt opened", f.normal.shown, want)
	}
	if want := []string{"focus right"}; !slices.Equal(f.host.asked, want) {
		t.Errorf("enter asked the host %q, want %q", f.host.asked, want)
	}
	view := plain(m)
	if !strings.Contains(view, "    ▸ login\n    ▾ docs\n        0 shell\n      api--rebase") {
		t.Errorf("view:\n%s\nwant docs's session there, active", view)
	}
	if got := cursor(t, m); got != "    ▾ docs" {
		t.Errorf("the cursor is on %q, want docs's session", got)
	}
}

// If wt fails to open the worktree, the status line says what wt said, and
// nothing is shown.
func TestAFailedOpenSaysWhatWtSaid(t *testing.T) {
	f := worktrees()
	f.wt.openErr = errors.New("wt: /p/api--docs is locked")
	m := keys(t, focused(t, f), press("j"), press("j"), press("j")) // docs
	m, cmd := update(t, m, enter)
	m, _ = update(t, m, run(t, cmd))
	if len(f.normal.shown) > 0 || len(f.host.asked) > 0 {
		t.Errorf("after a failed open, the sidebar showed %q and asked the host %q", f.normal.shown, f.host.asked)
	}
	if got := plain(m); !strings.HasSuffix(got, "\nwt: /p/api--docs is locked") {
		t.Errorf("view:\n%s\nwant it to end with wt's failure", got)
	}
}

// Across rebuilds, the cursor stays on its session or worktree as rows
// change shape: a worktree's one session's row becoming a session under the
// worktree's row, and back; and when a worktree's one session ends, the
// cursor goes to the worktree's row.
func TestTheCursorStaysOnItsSessionOrWorktree(t *testing.T) {
	f := worktrees()
	m := focused(t, f) // login, fix-login's one session
	f.normal.snap.Sessions = append(f.normal.snap.Sessions, session(7, "tests", "/p/api--fix-login", "shell"))
	f.repos.clones["/p/api--fix-login"] = "/p/api"
	m = rebuilt(t, m, f)
	if got := cursor(t, m); got != "      ▾ login" {
		t.Errorf("with a second session in fix-login, the cursor is on %q, want login, now under fix-login:\n%s", got, plain(m))
	}
	m = keys(t, m, press("k")) // fix-login
	if got := cursor(t, m); got != "    ▾ fix-login" {
		t.Fatalf("the cursor is on %q, want fix-login", got)
	}
	f.normal.snap.Sessions = without(f.normal.snap.Sessions, "$2")
	m = rebuilt(t, m, f)
	if got := cursor(t, m); got != "    ▸ tests" {
		t.Errorf("with login gone, the cursor is on %q, want tests, fix-login's one session now:\n%s", got, plain(m))
	}
	f.normal.snap.Sessions = without(f.normal.snap.Sessions, "$7")
	m = rebuilt(t, m, f)
	if got := cursor(t, m); got != "      fix-login" {
		t.Errorf("with tests gone too, the cursor is on %q, want fix-login, without a session now:\n%s", got, plain(m))
	}
}

// With a wt too old to list a project's worktrees, the project is listed as
// tmux sees it, its sessions by name, and the status line says wt is too
// old, with what wt said.
func TestAnOldWtListsTheProjectAsTmuxSeesIt(t *testing.T) {
	f := worktrees()
	f.normal.snap.Sessions = f.normal.snap.Sessions[:3]
	f.wt.fails = map[string]error{"/p/api": wtFailure{wt.ErrTooOld, "usage: wt [options] <branch>"}}
	m := started(t, f)
	want := ` wtui
  ▾ api
    ▸ api
    ▾ login
        0 shell
        1 dev
  ▾ notes
    ▸ notes
wt is too old for wtui (update tmux-kit): usage: wt [options] <branch>`
	if got := plain(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
}

// Without wt, every git project is listed as tmux sees it, and the status
// line says wt isn't there, once.
func TestNoWtSaysSoOnce(t *testing.T) {
	f := worktrees()
	missing := wtFailure{wt.ErrNotFound, "wt not found on PATH"}
	f.wt.fails = map[string]error{"/p/api": missing, "/p/Zed": missing}
	m := started(t, f)
	want := ` wtui
  ▾ api
    ▸ api
    ▸ Bugfix
    ▾ login
        0 shell
        1 dev
  ▾ notes
    ▸ notes
  ▾ Zed
    ▸ agent
    ▸ zed
wt not found on PATH`
	if got := plain(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
}

// A session of a git project in none of the worktrees wt lists (e.g. one
// made since) still has a place: after the worktrees, by name.
func TestASessionInNoListedWorktreeComesAfterThem(t *testing.T) {
	f := worktrees()
	f.normal.snap.Sessions = append(f.normal.snap.Sessions,
		session(7, "spike", "/p/api--spike", "shell"), session(8, "new", "/p/api--new", "shell"))
	f.repos.clones["/p/api--spike"] = "/p/api"
	f.repos.clones["/p/api--new"] = "/p/api"
	m := started(t, f)
	if got := plain(m); !strings.Contains(got, "      api--gone ✗\n    ▸ new\n    ▸ spike\n  ▾ notes") {
		t.Errorf("view:\n%s\nwant new and spike after api's worktrees", got)
	}
}

// Each build has wt list each git project once, in its main clone, however
// many sessions it has; a non-git project isn't wt's.
func TestEachBuildListsEachGitProjectOnce(t *testing.T) {
	f := worktrees()
	m := started(t, f)
	if want := []string{"/p/api", "/p/Zed"}; !slices.Equal(f.wt.listed, want) {
		t.Errorf("the sidebar had wt list %q, want %q", f.wt.listed, want)
	}
	rebuilt(t, m, f)
	if want := []string{"/p/api", "/p/Zed", "/p/api", "/p/Zed"}; !slices.Equal(f.wt.listed, want) {
		t.Errorf("after a rebuild, the sidebar had wt list %q, want %q", f.wt.listed, want)
	}
}
