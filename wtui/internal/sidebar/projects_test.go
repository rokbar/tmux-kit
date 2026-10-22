package sidebar

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/host"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// PopupRight records the popup over the right side, on the host, as "popup
// right <title> in <folder>: <command>".
func (h *fakeHost) PopupRight(p tmux.Popup) error {
	h.asked = append(h.asked, "popup right"+p.Title+"in "+p.Dir+": "+strings.Join(p.Command, " "))
	return nil
}

// withRoot is withDialogs with /home/u/Projects for the projects root, and
// the normal server's socket known.
func withRoot(t *testing.T, f fakes) Model {
	t.Helper()
	m := New(Clients{Host: f.host, Normal: f.normal, Repos: f.repos, Wt: f.wt, Wtui: wtui, Socket: socket,
		ProjectsRoot: "/home/u/Projects"}, `C-\`)
	m = feed(t, m, m.Init())
	m, _ = update(t, m, tea.FocusMsg{})
	return m
}

// p opens a project from the projects root, whatever row the cursor is on:
// wtui's open-project dialog, in a popup on the nested client, which shows
// the session it opens.
func TestPOpensAProjectFromTheProjectsRoot(t *testing.T) {
	f := worktrees()
	m := withRoot(t, f)
	for _, row := range []string{"  ▾ api", "      api--gone ✗", "4   ▸ notes"} {
		f.normal.popups = nil
		m, cmd := update(t, toRow(t, m, row), press("p"))
		feed(t, m, cmd)
		want := []tmux.Popup{{Title: " open a project ", Dir: "/home/u/Projects",
			Command: []string{"/bin/wtui", "project", "/home/u/Projects"}}}
		for i := range f.normal.popups {
			f.normal.popups[i].Env = nil // the dialog's own (see dialog.Open)
		}
		if !slices.EqualFunc(f.normal.popups, want, samePopup) {
			t.Errorf("p on %s opened %+v, want %+v", row, f.normal.popups, want)
		}
	}
}

func samePopup(a, b tmux.Popup) bool {
	return a.Title == b.Title && a.Dir == b.Dir && slices.Equal(a.Command, b.Command) && slices.Equal(a.Env, b.Env)
}

// While the right side shows no session (the last one ended), there's no
// nested client to open a popup on: p opens the dialog on the host, over the
// right side, telling it to only open the session (-d), which the right side
// then shows by itself. The host's popup isn't the normal server's, so the
// dialog reaches that by its socket. The tree is empty then, with no row for
// the cursor.
func TestWithNoSessionPOpensTheDialogOnTheHost(t *testing.T) {
	f := worktrees()
	f.normal.snap, f.normal.active = tmux.Snapshot{}, ""
	m := withRoot(t, f)
	m, cmd := update(t, m, press("p"))
	feed(t, m, cmd)
	want := []string{"focus right", "popup right open a project in /home/u/Projects: /bin/wtui -S " + socket + " project -d /home/u/Projects", "focus sidebar"}
	if !slices.Equal(f.host.asked, want) {
		t.Errorf("with no session, p asked the host %q, want %q", f.host.asked, want)
	}
	if len(f.normal.popups) > 0 {
		t.Errorf("with no session, p opened %+v on the nested client", f.normal.popups)
	}
}

// p opens a popup, so it isn't held (see TestPopupKeysArentHeld).
func TestPIsntHeld(t *testing.T) {
	i := slices.IndexFunc(Keys(), func(k host.Key) bool { return k.Name == "p" })
	if i < 0 || Keys()[i].Held || Keys()[i].Tmux != "p" {
		t.Errorf("the host binds p as %+v, want it passed on to the sidebar", Keys())
	}
}
