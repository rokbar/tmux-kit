package sidebar

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rokbar/tmux-kit/wtui/internal/host"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// x removes the worktree the cursor's row is in, through wtui's remove
// dialog in a popup on the nested client, run in the project's main clone:
// given the worktree's folder, and the question, which names the sessions
// that end. A missing worktree is removed too (wt unregisters it). The main
// clone (a project's row too), a non-git project and a session in no
// worktree wt lists get a hint instead.
func TestXRemovesTheRowsWorktree(t *testing.T) {
	f := worktrees()
	f.normal.snap.Sessions = append(f.normal.snap.Sessions, session(7, "spike", "/p/api--spike/web", "shell"))
	f.repos.clones["/p/api--spike/web"] = "/p/api"
	m := withDialogs(t, f)
	remove := ": /bin/wtui remove "
	login := "/p/api" + remove + "/p/api--fix-login Remove worktree api--fix-login and end session login?"
	want := map[string]string{
		"  ▾ api":             "api is the main clone, not a worktree",
		"    ▾ main":          "api is the main clone, not a worktree",
		"1     ▸ api":         "api is the main clone, not a worktree",
		"2     ▸ Bugfix":      "api is the main clone, not a worktree",
		"3   ▾ login":         login,
		"        0 shell":     login,
		"        1 dev":       login,
		"      docs":          "/p/api" + remove + "/p/api--docs Remove worktree api--docs (no session)?",
		"      api--rebase ±": "/p/api" + remove + "/p/api--rebase Remove worktree api--rebase (no session)?",
		"      api--gone ✗":   "/p/api" + remove + "/p/api--gone Remove worktree api--gone (no session)?",
		"4   ▸ spike":         "spike isn't in a git repo",
		"  ▾ notes":           "notes isn't in a git repo",
		"5   ▸ notes":         "notes isn't in a git repo",
		"  ▾ Zed":             "Zed is the main clone, not a worktree",
		"6   ▸ zed":           "Zed is the main clone, not a worktree",
		"7   ▸ agent":         "/p/Zed" + remove + "/p/Zed/.claude/worktrees/agent Remove worktree agent and end session agent?",
	}
	got := popupOnEveryRow(t, m, press("x"), f)
	for row, opened := range got {
		got[row], _, _ = strings.Cut(opened, "\n") // the question's first line
	}
	if !maps.Equal(got, want) {
		t.Errorf("x opened %q, want %q", got, want)
	}
	if len(f.normal.shown) > 0 {
		t.Errorf("x showed %q on the right, want nothing shown", f.normal.shown)
	}
	for _, p := range f.normal.popups {
		if path := p.Command[2]; p.Title != " remove "+filepath.Base(path)+" " {
			t.Errorf("the dialog removing %s is titled %q", path, p.Title)
		}
	}
}

// asked is the question x has the remove dialog ask, on row.
func asked(t *testing.T, f fakes, row string) string {
	t.Helper()
	m, cmd := update(t, toRow(t, withDialogs(t, f), row), press("x"))
	n := len(f.normal.popups)
	feed(t, m, cmd)
	if len(f.normal.popups) != n+1 {
		t.Fatalf("x on %q opened %d popups, want one", row, len(f.normal.popups)-n)
	}
	p := f.normal.popups[n]
	return p.Command[len(p.Command)-1]
}

// The question names every session that ends with the worktree: wt ends
// each one started in its folder or a folder inside it, whichever row of
// the tree it's on, but none in a folder whose name only starts with the
// worktree's. They're in the tree's order. Then the agents in each, as d's
// question names them: by window, most urgent first.
func TestTheRemoveQuestionNamesEverySessionAndAgentThatEnds(t *testing.T) {
	f := withAgents()
	f.normal.snap.Sessions = append(f.normal.snap.Sessions,
		session(8, "Review", "/p/api--fix-login", "claude"),
		session(9, "beside", "/p/api--fix-login-2", "shell"),
	)
	f.normal.snap.Agents = append(f.normal.snap.Agents, agent("$8", "@80", "%8", tmux.Done), agent("$9", "@90", "%9", tmux.Working))
	f.repos.clones["/p/api--fix-login"], f.repos.clones["/p/api--fix-login-2"] = "/p/api", "/p/api"
	want := `Remove worktree api--fix-login and end sessions login and Review?
The agents in login end too:
  0 shell: needs you
  0 shell: done
  1 dev: working
The agent in Review ends too:
  0 claude: done`
	if got := asked(t, f, "    ▾ fix-login !"); got != want {
		t.Errorf("on fix-login, the question is:\n%s\nwant:\n%s", got, want)
	}
}

// A missing worktree's folder is gone, so wt only unregisters it, and the
// question says so. Its sessions end all the same.
func TestTheRemoveQuestionSaysAMissingWorktreeIsOnlyUnregistered(t *testing.T) {
	f := worktrees()
	f.normal.snap.Sessions = append(f.normal.snap.Sessions, session(7, "gone", "/p/api--gone", "shell"))
	f.repos.clones["/p/api--gone"] = "/p/api"
	want := "Remove worktree api--gone and end session gone?\nIts folder is gone, so removing it only unregisters it."
	if got := asked(t, f, "4   ▸ gone ✗"); got != want {
		t.Errorf("on api--gone, the question is:\n%s\nwant:\n%s", got, want)
	}
}

// Once the dialog has closed, the status line says what wt said, and the
// tree drops the worktree wt removed, from wt's next listing.
func TestAfterXTheStatusLineSaysWhatWtSaid(t *testing.T) {
	f := worktrees()
	m := toRow(t, withDialogs(t, f), "      docs")
	f.normal.onPopup = func(p tmux.Popup) {
		f.wt.lists["/p/api"] = slices.DeleteFunc(slices.Clone(f.wt.lists["/p/api"]), func(w wt.Worktree) bool { return w.Path == "/p/api--docs" })
		says(t, "wt: removed /p/api--docs; branch docs kept (git branch -d docs to delete)")(p)
	}
	m, cmd := update(t, m, press("x"))
	m, refresh := update(t, m, run(t, cmd))
	m = drain(t, m, refresh)
	got := plain(m)
	if strings.Contains(got, "docs\n") || !strings.HasSuffix(got, "\nwt: removed /p/api--docs; branch docs kept (git branch -d docs to delete)") {
		t.Errorf("after x removed docs, the view is:\n%s", got)
	}
}

// Removing the active session's worktree ends that session: tmux moves the
// right side on, and the sidebar takes focus, as when any active session
// ends. wt's message stays in the status line, whether the sidebar has
// focus before the dialog has closed or after.
func TestRemovingTheActiveSessionsWorktreeKeepsWtsMessage(t *testing.T) {
	f := worktrees()
	m := withDialogs(t, f)
	said := "wt: removed /p/api--fix-login; branch fix-login kept (git branch -d fix-login to delete)"
	f.normal.onPopup = func(p tmux.Popup) {
		f.normal.snap.Sessions = without(f.normal.snap.Sessions, "$2")
		f.normal.active = "$1"
		says(t, said)(p)
	}
	m, dialog := update(t, m, press("x"))
	closed := run(t, dialog)
	m, focus := update(t, m, tmux.ActiveSessionChanged{Session: "$1", Ended: true}) // while the dialog closes
	m, refresh := update(t, m, closed)
	m = drain(t, m, refresh)
	m, _ = update(t, m, run(t, focus))
	if got := plain(m); !strings.HasSuffix(got, "\n"+said) {
		t.Errorf("after x removed the active session's worktree, the view is:\n%s", got)
	}
	if got := cursor(t, m); got != "1     ▾ api" {
		t.Errorf("the cursor is on %q, want api, the session shown now", got)
	}
	if want := []string{"focus right", "focus sidebar", "focus sidebar"}; !slices.Equal(f.host.asked, want) {
		t.Errorf("the sidebar asked the host %q, want %q", f.host.asked, want)
	}
}

// x opens a popup, so it isn't held: the host would hold the keys typed
// into the popup until it had closed.
func TestXIsntHeld(t *testing.T) {
	i := slices.IndexFunc(Keys(), func(k host.Key) bool { return k.Name == "x" })
	if i < 0 || Keys()[i].Held || Keys()[i].Tmux != "x" {
		t.Errorf("the host binds x as %+v, want it passed on to the sidebar", Keys())
	}
}
