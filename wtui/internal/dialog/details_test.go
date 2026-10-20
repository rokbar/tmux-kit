package dialog_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rokbar/tmux-kit/wtui/internal/detail"
	"github.com/rokbar/tmux-kit/wtui/internal/dialog"
)

// fakeReader answers Read with details, or err, and notes the paths it was
// asked about.
type fakeReader struct {
	details detail.Details
	err     error
	reads   []string
}

func (r *fakeReader) Read(path string) (detail.Details, error) {
	r.reads = append(r.reads, path)
	return r.details, r.err
}

func update(t *testing.T, m tea.Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	t.Helper()
	return m.Update(msg)
}

// view is m's view without its styles, each line without trailing spaces.
func view(m tea.Model) string {
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

// read is the details dialog of fix-login once git, through r, has read its
// details, in a popup of width by height.
func read(t *testing.T, r *fakeReader, branch, devURL string, width, height int) tea.Model {
	t.Helper()
	m := dialog.Details(r, "/p/api--fix-login", branch, devURL)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: width, Height: height})
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("the details dialog reads nothing")
	}
	m, _ = update(t, m, cmd())
	return m
}

var loginDetails = detail.Details{
	Changes:  []string{" M f", "?? wip.txt"},
	Commits:  []string{"1a2b3c4 work", "5d6e7f8 init"},
	Upstream: "origin/fix-login", Ahead: 2, Behind: 1,
}

// The details dialog shows the worktree's branch and dev URL, as wt listed
// them, and what git reads in its folder: where it stands against its
// upstream, its changed files and its last commits. Any key closes it, as
// its last line says.
func TestDetailsShowWhatWtAndGitSay(t *testing.T) {
	r := &fakeReader{details: loginDetails}
	m := read(t, r, "fix-login", "http://localhost:3001", 40, 14)
	want := ` branch fix-login
 dev URL http://localhost:3001
 2 ahead, 1 behind origin/fix-login
 changes:
    M f
   ?? wip.txt
 commits:
   1a2b3c4 work
   5d6e7f8 init




 any key closes`
	if got := view(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
	if want := []string{"/p/api--fix-login"}; !slices.Equal(r.reads, want) {
		t.Errorf("git read %q, want %q", r.reads, want)
	}
}

// A detached worktree (e.g. mid-rebase) has no branch, one without a dev
// URL says so, and one whose branch has no upstream isn't pushed.
func TestDetailsSayWhatTheWorktreeLacks(t *testing.T) {
	r := &fakeReader{details: detail.Details{Commits: []string{"9e8d7c6 ours"}}}
	m := read(t, r, "", "", 40, 8)
	want := ` no branch (detached HEAD)
 no dev URL
 not pushed
 no changes
 commits:
   9e8d7c6 ours

 any key closes`
	if got := view(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
}

// Before git has read the details, the dialog says it's reading them; if
// git fails, it says what git said.
func TestDetailsWhileReadingAndWhenGitFails(t *testing.T) {
	r := &fakeReader{err: errors.New("git status: fatal: not a git repository")}
	m := dialog.Details(r, "/p/api--fix-login", "fix-login", "")
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 40, Height: 4})
	if got := view(m); !strings.Contains(got, " reading the details…") {
		t.Errorf("before git has read them, the view is:\n%s", got)
	}
	m, _ = update(t, m, m.Init()())
	want := " branch fix-login\n git status: fatal: not a git\n repository\n any key closes"
	if got := view(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
}

// In a short popup, the changes and the commits share the room left, and a
// list cut short says how many more it has. A line too long for the popup
// is cut at its edge, and control characters in it show as "?".
func TestDetailsFitThePopup(t *testing.T) {
	d := loginDetails
	d.Changes = []string{" M a", " M b", " M c", " M d"}
	d.Commits = []string{"1 one", "2 two", "3 three", "4 four", "5 \x1b[2Jfive and a subject too long to fit"}
	m := read(t, &fakeReader{details: d}, "fix-login", "", 30, 10)
	want := ` branch fix-login
 no dev URL
 2 ahead, 1 behind origin/fi…
 changes:
    M a
   … 3 more
 commits:
   1 one
   … 4 more
 any key closes`
	if got := view(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
	d.Commits = d.Commits[4:]
	m = read(t, &fakeReader{details: d}, "fix-login", "", 30, 30)
	lines := strings.Split(view(m), "\n")
	if got := lines[slices.Index(lines, " commits:")+1]; got != "   5 ?[2Jfive and a subject …" {
		t.Errorf("the long commit shows as %q, want it cut at the popup's edge, its escape as ?", got)
	}
}

// Any key closes a dialog: the details, and a page of text.
func TestAnyKeyClosesADialog(t *testing.T) {
	for _, m := range []tea.Model{
		read(t, &fakeReader{details: loginDetails}, "fix-login", "", 40, 14),
		dialog.Page([]string{"a line"}),
	} {
		for _, k := range []tea.KeyPressMsg{{Code: 'q', Text: "q"}, {Code: tea.KeyEnter}, {Code: tea.KeyEscape}, {Code: 'c', Mod: tea.ModCtrl}} {
			_, cmd := update(t, m, k)
			if cmd == nil {
				t.Fatalf("%T: %s does nothing, want it to close the dialog", m, k)
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Errorf("%T: %s doesn't close the dialog", m, k)
			}
		}
		if _, cmd := update(t, m, tea.FocusMsg{}); cmd != nil {
			t.Errorf("%T: something other than a key gives a command", m)
		}
	}
}

// A page shows its lines, then says any key closes it, at the bottom of its
// popup, once it knows the popup's height. PageSize is the popup that fits it
// exactly, inside its border.
func TestAPageShowsItsLines(t *testing.T) {
	lines := []string{"In the sidebar, a key alone.", "", "j down   down"}
	m := dialog.Page(lines)
	w, h := dialog.PageSize(lines)
	if w != 32 || h != 7 {
		t.Errorf("PageSize() = %dx%d, want 32x7: the lines, a blank line and the closing line, a column either side, and the border", w, h)
	}
	m, _ = update(t, m, tea.WindowSizeMsg{Width: w - 2, Height: h - 2})
	want := " In the sidebar, a key alone.\n\n j down   down\n\n any key closes"
	if got := view(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
}
