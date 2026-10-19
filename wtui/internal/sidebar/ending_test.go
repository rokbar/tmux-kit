package sidebar

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// (fakeHost's other requests are in sidebar_test.go.)
func (h *fakeHost) FocusSidebar() error { h.asked = append(h.asked, "focus sidebar"); return nil }

// When the active session ends, tmux moves the right side to the most
// recent session, and the sidebar takes focus, its cursor on that session:
// what the user types next mustn't land in a session they didn't choose.
// Then the tree catches up, without the session that ended.
func TestWhenTheActiveSessionEndsTheSidebarTakesFocus(t *testing.T) {
	f := fixture()
	m := keys(t, focused(t, f), press("j"))                        // the cursor on fix-login's shell
	m, _ = update(t, m, tea.BlurMsg{})                             // focus is on the right
	f.normal.snap.Sessions = without(f.normal.snap.Sessions, "$2") // fix-login ended
	m, cmd := update(t, m, tmux.ActiveSessionChanged{Session: "$3", Ended: true})
	m, _ = update(t, m, run(t, cmd))
	if want := []string{"focus sidebar"}; !slices.Equal(f.host.asked, want) {
		t.Errorf("when the active session ended, the sidebar asked the host %q, want %q", f.host.asked, want)
	}
	m, _ = update(t, m, tea.FocusMsg{}) // as tmux says, once the sidebar has focus
	if got := cursor(t, m); got != "4   ▾ notes" {
		t.Errorf("once the sidebar has focus, the cursor is on %q, want notes, the session shown now", got)
	}
	m, build := update(t, m, tmux.SessionsChanged{})
	m = feed(t, m, build)
	want := ` wtui
  ▾ api
1   ▸ api
2   ▸ Bugfix
  ▾ notes
3   ▾ notes
        0 vim
  ▾ Zed
4   ▸ zed`
	if got := plain(m); got != want {
		t.Errorf("after the tree caught up, the view is:\n%s\nwant:\n%s", got, want)
	}
	if got := cursor(t, m); got != "3   ▾ notes" {
		t.Errorf("after the tree caught up, the cursor is on %q, want notes", got)
	}
}

// If the sidebar has focus already, tmux won't say it got it: the cursor
// goes to the session shown now at once.
func TestWhenTheActiveSessionEndsInTheSidebarTheCursorMovesAtOnce(t *testing.T) {
	f := fixture()
	m := keys(t, focused(t, f), press("j"))
	m, _ = update(t, m, tmux.ActiveSessionChanged{Session: "$3", Ended: true})
	if got := cursor(t, m); got != "4   ▾ notes" {
		t.Errorf("the cursor is on %q, want notes, the session shown now", got)
	}
}

// Once the last session has ended, the right side shows none and the tree
// is empty, with nothing in the status line. When a session comes, the
// right side shows it, with the cursor on it, however the focus, the tree
// and the right side's news came in.
func TestWithNoSessionLeftTheTreeIsEmptyUntilOneComes(t *testing.T) {
	for _, focusFirst := range []bool{true, false} {
		f := fixture()
		m := keys(t, focused(t, f), press("j"), press("j"))
		m, _ = update(t, m, tea.BlurMsg{})
		f.normal.snap = tmux.Snapshot{}
		m, build := update(t, m, tmux.SessionsChanged{})
		if focusFirst { // the right side gave the sidebar focus before the tree caught up
			m, _ = update(t, m, tea.FocusMsg{})
		}
		m, _ = update(t, m, tmux.ActiveSessionChanged{}) // the right side shows none
		m = feed(t, m, build)
		if !focusFirst {
			m, _ = update(t, m, tea.FocusMsg{})
		}
		if got := plain(m); got != " wtui" {
			t.Errorf("focus first: %v; with no session, the view is:\n%s\nwant only the header", focusFirst, got)
		}
		f.normal.snap.Sessions = []tmux.Session{session(9, "delta", "/home/u/delta", "shell")}
		m, build = update(t, m, tmux.SessionsChanged{})
		m, _ = update(t, m, tmux.ActiveSessionChanged{Session: "$9"})
		m = feed(t, m, build)
		if got := cursor(t, m); got != "1   ▾ delta" {
			t.Errorf("focus first: %v; once delta is shown, the cursor is on %q, want delta", focusFirst, got)
		}
	}
}
