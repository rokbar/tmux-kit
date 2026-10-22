package sidebar

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Version is the fake's tmux version.
func (n *fakeNormal) Version() (string, error) { return n.version, nil }

// Float records, in its host's log, the command it floats over the right
// side, in folder dir, unless it fails with floatErr.
func (n *fakeNormal) Float(dir string, command []string) error {
	if n.floatErr != nil {
		return n.floatErr
	}
	n.host.asked = append(n.host.asked, "float in "+dir+": "+strings.Join(command, " "))
	return nil
}

// (Popup is in dialogs_test.go.)

// ThenFocusSidebar is command, then "; focus sidebar".
func (h *fakeHost) ThenFocusSidebar(command []string) []string {
	return append(slices.Clone(command), ";", "focus", "sidebar")
}

// GitUICommand is "wt git-ui <path>".
func (w *fakeWt) GitUICommand(path string) ([]string, error) {
	return []string{"wt", "git-ui", path}, nil
}

// wtless is wt that isn't there: it has no git UI to run.
type wtless struct{ *fakeWt }

func (wtless) GitUICommand(string) ([]string, error) {
	return nil, errors.New("wt not found on PATH")
}

// gitUI is the worktrees fixture on the given tmux version, whose host's log
// has what floats or pops up over the right side too.
func gitUI(version string) fakes {
	f := worktrees()
	f.normal.version, f.normal.host = version, f.host
	return f
}

// g runs the git UI, in the background, in the folder of the worktree the
// cursor's row is in: for a session, its worktree's folder (login started
// in a folder inside fix-login). On tmux 3.7 or newer, it floats over the
// right side, with focus there, and once it quits, focus comes back to the
// sidebar.
func TestGFloatsTheGitUIOverTheRightSide(t *testing.T) {
	f := gitUI("3.7")
	m, _ := withOpeners(t, f)
	if got := cursor(t, m); got != "3   ▾ login" {
		t.Fatalf("the cursor is on %q, want login", got)
	}
	m, cmd := update(t, m, press("g"))
	if len(f.host.asked) > 0 {
		t.Fatalf("g asked the host %q before returning, want it in the background", f.host.asked)
	}
	update(t, m, run(t, cmd))
	want := []string{"float in /p/api--fix-login: wt git-ui /p/api--fix-login ; focus sidebar", "focus right"}
	if !slices.Equal(f.host.asked, want) {
		t.Errorf("g asked the host %q, want %q", f.host.asked, want)
	}
	if len(f.normal.shown) > 0 {
		t.Errorf("g showed %q on the right, want it to stay as it is", f.normal.shown)
	}
}

// gOn is what g on the active session's row (login, in fix-login) asked of
// the host, in order, with what floated or popped up over the right side.
func gOn(t *testing.T, f fakes) []string {
	t.Helper()
	m, _ := withOpeners(t, f)
	m, cmd := update(t, m, press("g"))
	update(t, m, run(t, cmd))
	return f.host.asked
}

var (
	// floated is the git UI floating over the right side, in fix-login.
	floated = []string{"float in /p/api--fix-login: wt git-ui /p/api--fix-login ; focus sidebar", "focus right"}
	// poppedUp is the git UI popping up over the right side, in fix-login.
	poppedUp = []string{"focus right", "popup in /p/api--fix-login: wt git-ui /p/api--fix-login", "focus sidebar"}
)

// Floating panes are new in tmux 3.7. On an older tmux, the git UI pops up
// over the right side instead: focus goes there for it, and comes back to
// the sidebar once it has quit (the popup closes with it).
func TestGFloatsOrPopsUpByTmuxVersion(t *testing.T) {
	for _, c := range []struct {
		version string
		want    []string
	}{
		{"3.6", poppedUp},
		{"3.6b", poppedUp},
		{"next-3.6", poppedUp},
		{"3.7", floated},
		{"3.7c", floated},
		{"3.8", floated},
		{"next-3.7", floated},
		{"master", floated}, // follows tmux's development
	} {
		if got := gOn(t, gitUI(c.version)); !slices.Equal(got, c.want) {
			t.Errorf("on tmux %s, g asked the host %q, want %q", c.version, got, c.want)
		}
	}
}

// gOnEveryRow presses g with the cursor on each row that shows, in turn,
// and says what came of it for each, by the row's text: the folder the git
// UI floated in, or what the status line then says.
func gOnEveryRow(t *testing.T, m Model, f fakes) map[string]string {
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
		row, before, n := cursor(t, at), plain(at), len(f.host.asked)
		after, cmd := update(t, at, press("g"))
		if cmd != nil {
			after, _ = update(t, after, run(t, cmd))
		}
		var came []string
		for _, asked := range f.host.asked[n:] {
			if rest, ok := strings.CutPrefix(asked, "float in "); ok {
				folder, command, _ := strings.Cut(rest, ": ")
				if command != "wt git-ui "+folder+" ; focus sidebar" {
					t.Errorf("on %q, g floated %q in %s", row, command, folder)
				}
				came = append(came, folder)
			}
		}
		if said := strings.TrimPrefix(plain(after), before); said != "" {
			came = append(came, strings.TrimPrefix(said, "\n"))
		}
		got[row] = strings.Join(came, ", ")
	}
	return got
}

// g runs the git UI in the row's worktree or main clone, whatever its row:
// a project's row is its main clone's, and a worktree without a session
// (dim) has its own. A window's is its session's. A missing worktree has no
// folder to run it in, and a non-git project no repo, so they show a hint
// instead.
func TestGRunsTheGitUIInTheRowsWorktree(t *testing.T) {
	f := gitUI("3.8")
	m, _ := withOpeners(t, f)
	want := map[string]string{
		"  ▾ api":             "/p/api",
		"    ▾ main":          "/p/api",
		"1     ▸ api":         "/p/api",
		"2     ▸ Bugfix":      "/p/api",
		"3   ▾ login":         "/p/api--fix-login",
		"        0 shell":     "/p/api--fix-login",
		"        1 dev":       "/p/api--fix-login",
		"      docs":          "/p/api--docs",
		"      api--rebase ±": "/p/api--rebase",
		"      api--gone ✗":   "api--gone is missing",
		"  ▾ notes":           "notes isn't in a git repo",
		"4   ▸ notes":         "notes isn't in a git repo",
		"  ▾ Zed":             "/p/Zed",
		"5   ▸ zed":           "/p/Zed",
		"6   ▸ agent":         "/p/Zed/.claude/worktrees/agent",
	}
	if got := gOnEveryRow(t, m, f); !maps.Equal(got, want) {
		t.Errorf("g ran the git UI in %q, want %q", got, want)
	}
	// a window of a session in a non-git project: its session's name
	m, _ = update(t, m, Active{Session: "$3"})
	m, _ = update(t, m, tea.FocusMsg{}) // the cursor on notes
	m = keys(t, m, press("j"))
	if got := cursor(t, m); got != "        0 vim" {
		t.Fatalf("with notes active, the cursor is on %q, want its window", got)
	}
	if m, cmd := update(t, m, press("g")); cmd != nil || !strings.HasSuffix(plain(m), "\nnotes isn't in a git repo") {
		t.Errorf("g on notes's window gave %v, and the view:\n%s", cmd, plain(m))
	}
}

// If the git UI can't run (e.g. there's no wt), or the popup can't open,
// the status line says why, and focus is back on the sidebar.
func TestAGitUIThatFailsShowsInTheStatusLine(t *testing.T) {
	f := gitUI("3.8")
	m := New(Clients{Host: f.host, Normal: f.normal, Repos: f.repos, Wt: wtless{f.wt}}, `C-\`)
	m = feed(t, m, m.Init())
	m, _ = update(t, m, tea.FocusMsg{})
	m, cmd := update(t, m, press("g"))
	m, _ = update(t, m, run(t, cmd))
	if got := plain(m); !strings.HasSuffix(got, "\nwt not found on PATH") {
		t.Errorf("with no wt, after g, the view is:\n%s", got)
	}
	if len(f.host.asked) > 0 {
		t.Errorf("with no wt, g asked the host %q, want nothing", f.host.asked)
	}

	f = gitUI("3.6")
	f.normal.popupErr = errors.New("can't find client: /dev/ttys004")
	m, _ = withOpeners(t, f)
	m, cmd = update(t, m, press("g"))
	m, _ = update(t, m, run(t, cmd))
	if got := plain(m); !strings.HasSuffix(got, "\ncan't find client: /dev/ttys004") {
		t.Errorf("with the popup failing, after g, the view is:\n%s", got)
	}
	if want := []string{"focus right", "focus sidebar"}; !slices.Equal(f.host.asked, want) {
		t.Errorf("with the popup failing, g asked the host %q, want %q", f.host.asked, want)
	}
}

// Once the git UI has floated, or popped up and quit, every git project's
// worktrees are listed again, so that their markers catch up (e.g. a
// commit clears ±).
func TestGRefreshesTheMarkers(t *testing.T) {
	for _, version := range []string{"3.6", "3.8"} {
		f := gitUI(version)
		m, _ := withOpeners(t, f)
		f.wt.listed = nil
		m, cmd := update(t, m, press("g"))
		m, refresh := update(t, m, run(t, cmd))
		feed(t, m, refresh)
		if want := []string{"/p/api", "/p/Zed"}; !slices.Equal(f.wt.listed, want) {
			t.Errorf("on tmux %s, after g, wt listed %q, want %q", version, f.wt.listed, want)
		}
	}
}

// With no session on the right to float over, or if floating fails (e.g. a
// tmux build without floating panes), the git UI pops up instead.
func TestWithNothingToFloatOverGPopsUp(t *testing.T) {
	f := gitUI("3.8")
	f.normal.floatErr = errors.New("unknown command: new-pane")
	if got := gOn(t, f); !slices.Equal(got, poppedUp) {
		t.Errorf("with floating failing, g asked the host %q, want %q", got, poppedUp)
	}

	f = gitUI("3.8")
	m, _ := withOpeners(t, f)
	m, _ = update(t, m, Active{}) // the right side shows no session
	m, cmd := update(t, m, press("g"))
	update(t, m, run(t, cmd))
	if !slices.Equal(f.host.asked, poppedUp) {
		t.Errorf("with no session on the right, g asked the host %q, want %q", f.host.asked, poppedUp)
	}
}
