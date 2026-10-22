package sidebar

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// Jumping between sessions: by number, to the last session, and to the
// next or previous one in the active session's project.

// crowded is the fixture with six more sessions in notes, n1 to n6 ($6 to
// $11): eleven sessions in all.
func crowded() fakes {
	f := fixture()
	for i := 1; i <= 6; i++ {
		f.normal.snap.Sessions = append(f.normal.snap.Sessions, session(5+i, fmt.Sprintf("n%d", i), "/home/u/notes", "shell"))
	}
	return f
}

// Sessions are numbered 1–9 in the margin, in tree order, and the numbers
// stay as they are when a project folds; the sessions after the ninth have
// none.
func TestSessionsAreNumberedInTreeOrder(t *testing.T) {
	m := focused(t, crowded())
	want := ` wtui
  ▾ api
1   ▸ api
2   ▸ Bugfix
3   ▾ fix-login
        0 shell
        1 dev
  ▾ notes
4   ▸ n1
5   ▸ n2
6   ▸ n3
7   ▸ n4
8   ▸ n5
9   ▸ n6
    ▸ notes
  ▾ Zed
    ▸ zed`
	if got := plain(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
	m = keys(t, m, press("h"), press("h"), press("h")) // fold api
	want = ` wtui
  ▸ api
  ▾ notes
4   ▸ n1`
	if got := plain(m); !strings.HasPrefix(got, want) {
		t.Errorf("with api folded, the view is:\n%s\nwant it to start:\n%s", got, want)
	}
}

// A number shows that session on the right, wherever the cursor is, and
// moves focus there.
func TestANumberShowsThatSession(t *testing.T) {
	f := crowded()
	m := keys(t, focused(t, f), press("j"), press("j"), press("j")) // notes
	m, cmd := update(t, m, press("9"))
	m, _ = update(t, m, run(t, cmd))
	if want := []string{"$11"}; !slices.Equal(f.normal.shown, want) {
		t.Errorf("9 showed %q, want %q, n6", f.normal.shown, want)
	}
	if want := []string{"focus right"}; !slices.Equal(f.host.asked, want) {
		t.Errorf("9 asked the host %q, want %q", f.host.asked, want)
	}
	if got := styles(m)["9   ▾ n6"]; got != "1;36" {
		t.Errorf("after 9, n6 is styled %q, want it highlighted as the active session:\n%s", got, plain(m))
	}
}

// A number no session has shows a hint instead, until the next key; focus
// goes right all the same, as after any jump, so what's typed next lands in
// the session there.
func TestANumberNoSessionHasShowsAHint(t *testing.T) {
	f := fixture()
	m := focused(t, f)
	m, cmd := update(t, m, press("7"))
	m, _ = update(t, m, run(t, cmd))
	if len(f.normal.shown) > 0 {
		t.Errorf("7, with five sessions, showed %q", f.normal.shown)
	}
	if want := []string{"focus right"}; !slices.Equal(f.host.asked, want) {
		t.Errorf("7 asked the host %q, want %q", f.host.asked, want)
	}
	if got := plain(m); !strings.HasSuffix(got, "\nno session 7") {
		t.Errorf("view:\n%s\nwant it to end with a hint", got)
	}
	if m = keys(t, m, press("k")); strings.Contains(plain(m), "no session 7") {
		t.Errorf("after the next key, the view still has the hint:\n%s", plain(m))
	}
}

// jumped is the sidebar after key, once the command it gave has run.
func jumped(t *testing.T, m Model, key tea.KeyPressMsg) Model {
	t.Helper()
	m, cmd := update(t, m, key)
	m, _ = update(t, m, run(t, cmd))
	return m
}

// ] and [ show the next and the previous session in the active session's
// project, in tree order, wrapping round; then focus goes right. Where the
// cursor is doesn't matter.
func TestBracketsWalkTheActiveSessionsProject(t *testing.T) {
	// api's sessions: api ($1), Bugfix ($5), fix-login ($2), on the right
	f := fixture()
	m := keys(t, focused(t, f), press("j"), press("j"), press("j")) // notes
	for _, key := range []string{"]", "]", "[", "[", "["} {
		m = jumped(t, m, press(key))
	}
	if want := []string{"$1", "$5", "$1", "$2", "$5"}; !slices.Equal(f.normal.shown, want) {
		t.Errorf("] ] [ [ [ from fix-login showed %q, want %q", f.normal.shown, want)
	}
	if want := slices.Repeat([]string{"focus right"}, 5); !slices.Equal(f.host.asked, want) {
		t.Errorf("] ] [ [ [ asked the host %q, want %q", f.host.asked, want)
	}
}

// A project's sessions are its worktrees' too, wherever they are in it.
func TestBracketsWalkAProjectsWorktrees(t *testing.T) {
	f := worktrees() // api's sessions: main's api ($1) and Bugfix ($5), fix-login's login ($2)
	f.normal.active = "$5"
	m := focused(t, f)
	for _, key := range []string{"]", "]", "]"} {
		m = jumped(t, m, press(key))
	}
	if want := []string{"$2", "$1", "$5"}; !slices.Equal(f.normal.shown, want) {
		t.Errorf("] ] ] from Bugfix showed %q, want %q", f.normal.shown, want)
	}
}

// In a project with one session, ] and [ have nowhere to go: they show a
// hint, and focus goes right all the same.
func TestBracketsInAProjectWithOneSessionShowAHint(t *testing.T) {
	for _, key := range []string{"]", "["} {
		f := fixture()
		f.normal.active = "$3" // notes, alone in notes
		m := jumped(t, focused(t, f), press(key))
		if len(f.normal.shown) > 0 {
			t.Errorf("%s showed %q", key, f.normal.shown)
		}
		if want := []string{"focus right"}; !slices.Equal(f.host.asked, want) {
			t.Errorf("%s asked the host %q, want %q", key, f.host.asked, want)
		}
		if got := plain(m); !strings.HasSuffix(got, "\nno other session in notes") {
			t.Errorf("after %s, the view is:\n%s\nwant it to end with a hint", key, got)
		}
	}
}

var tab = tea.KeyPressMsg{Code: tea.KeyTab}

// tab shows the last session, as the tmux client finds it (the one shown
// before, else the most recent other one), and focus goes right.
func TestTabShowsTheLastSession(t *testing.T) {
	f := fixture()
	f.normal.last = "$3"
	m := jumped(t, focused(t, f), tab)
	if want := []string{"last"}; !slices.Equal(f.normal.shown, want) {
		t.Errorf("tab showed %q, want %q", f.normal.shown, want)
	}
	if want := []string{"focus right"}; !slices.Equal(f.host.asked, want) {
		t.Errorf("tab asked the host %q, want %q", f.host.asked, want)
	}
	if got := styles(m)["4   ▾ notes"]; got != "1;36" {
		t.Errorf("after tab, notes is styled %q, want it highlighted as the active session:\n%s", got, plain(m))
	}
}

// With no other session, tab shows a hint, and focus goes right all the
// same.
func TestTabWithNoOtherSessionShowsAHint(t *testing.T) {
	f := fixture()
	f.normal.lastErr = tmux.ErrNoOtherSession
	m := jumped(t, focused(t, f), tab)
	if want := []string{"focus right"}; !slices.Equal(f.host.asked, want) {
		t.Errorf("tab asked the host %q, want %q", f.host.asked, want)
	}
	if got := plain(m); !strings.HasSuffix(got, "\nno other session") {
		t.Errorf("view:\n%s\nwant it to end with a hint", got)
	}
}
