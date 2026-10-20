package dialog_test

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rokbar/tmux-kit/wtui/internal/dialog"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// fakeWt removes worktrees as wt would: with err if set, else with a safety
// refusal whose text is refusal, if that's set and it isn't forced, else it
// says it removed it. It records the removals it was asked for, as
// "<dir> <path>", or "<dir> -f <path>" when forced.
type fakeWt struct {
	err      error
	refusal  string
	removals []string
}

func (f *fakeWt) Remove(dir, path string, force bool) (string, error) {
	call := dir + " " + path
	if force {
		call = dir + " -f " + path
	}
	f.removals = append(f.removals, call)
	switch {
	case f.err != nil:
		return "", f.err
	case f.refusal != "" && !force:
		return "", wtFailure{wt.ErrSafetyRefusal, f.refusal}
	}
	return "wt: removed " + path, nil
}

// wtFailure is a failed wt call as the wt client reports it: of a kind (e.g.
// wt.ErrSafetyRefusal), with wt's message as its text.
type wtFailure struct {
	kind error
	msg  string
}

func (e wtFailure) Error() string { return e.msg }
func (e wtFailure) Unwrap() error { return e.kind }

// key is a key press as Bubble Tea names it: up, down, esc, enter or ctrl+c,
// or a printable key.
func key(name string) tea.KeyPressMsg {
	switch name {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	r := []rune(name)[0]
	return tea.KeyPressMsg{Code: r, Text: name}
}

// quits says whether cmd closes the dialog.
func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// login is removing api--fix-login, which ends session login.
var login = dialog.Removal{Path: "/p/api--fix-login", Sessions: []dialog.Ending{{Session: "login"}}}

// asking is the remove dialog of r, removing through f in the main clone
// /p/api, in a popup of width by height.
func asking(t *testing.T, f *fakeWt, r dialog.Removal, width, height int) tea.Model {
	t.Helper()
	var m tea.Model = dialog.Remove(f, "/p/api", r)
	if cmd := m.Init(); cmd != nil {
		t.Fatal("the remove dialog does something before it's answered")
	}
	m, _ = update(t, m, tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

// answer presses keys in turn, delivering what each one's command reports
// unless it closes the dialog. It returns the dialog after the last key, and
// whether that closed it.
func answer(t *testing.T, m tea.Model, keys ...string) (tea.Model, bool) {
	t.Helper()
	closed := false
	for _, k := range keys {
		var cmd tea.Cmd
		m, cmd = update(t, m, key(k))
		closed = false
		for cmd != nil {
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); ok {
				closed = true
				break
			}
			m, cmd = update(t, m, msg)
		}
	}
	return m, closed
}

// said is what the remove dialog m says for the status line.
func said(m tea.Model) string { return m.(dialog.Removing).Said() }

// The dialog asks before it removes anything, naming the worktree, every
// session that ends with it and the agents in those, by window and state. A
// missing worktree only gets unregistered, which it says too.
func TestRemoveAsksNamingEverySessionAndAgentItEnds(t *testing.T) {
	for _, c := range []struct {
		r    dialog.Removal
		want string // the question's lines
	}{
		{dialog.Removal{Path: "/p/api--docs"}, " remove api--docs (no session)?"},
		{login, " remove api--fix-login and end session login?"},
		{
			dialog.Removal{Path: "/p/api--fix-login", Sessions: []dialog.Ending{{Session: "login"}, {Session: "review"}, {Session: "web"}}},
			" remove api--fix-login and end sessions login, review and web?",
		},
		{
			dialog.Removal{Path: "/p/api--fix-login", Sessions: []dialog.Ending{
				{Session: "login", Agents: []dialog.EndingAgent{{Window: "dev", State: "working"}, {Window: "shell", State: "needs you"}}},
				{Session: "review", Agents: []dialog.EndingAgent{{Window: "claude", State: "done"}}},
				{Session: "web"},
			}},
			" remove api--fix-login and end sessions login, review and web?\n" +
				" the agents in login end too: working in dev, needs you in shell\n" +
				" the agent in review ends too: done in claude",
		},
		{
			dialog.Removal{Path: "/p/api--gone", Missing: true, Sessions: []dialog.Ending{{Session: "gone"}}},
			" remove api--gone and end session gone?\n its folder is gone, so removing it only unregisters it",
		},
		{dialog.Removal{Path: "/p/api--gone", Missing: true}, " remove api--gone (no session)?\n its folder is gone, so removing it only unregisters it"},
	} {
		f := &fakeWt{}
		m := asking(t, f, c.r, 70, 8)
		want := c.want + strings.Repeat("\n", 7-strings.Count(c.want, "\n")) + " y: remove   n/q/esc: cancel"
		if got := view(m); got != want {
			t.Errorf("asking about %+v, the view is:\n%s\nwant:\n%s", c.r, got, want)
		}
		if len(f.removals) > 0 {
			t.Errorf("asking about %s, it removed %q before it's answered", c.r.Path, f.removals)
		}
	}
}

// y has wt remove the worktree, in the main clone, which closes the dialog:
// what wt said is what it says for the status line. A missing worktree is
// removed the same way: wt unregisters it.
func TestYRemovesTheWorktreeAndSaysWhatWtSaid(t *testing.T) {
	for _, r := range []dialog.Removal{login, {Path: "/p/api--gone", Missing: true}} {
		f := &fakeWt{}
		m, closed := answer(t, asking(t, f, r, 70, 8), "y")
		if want := []string{"/p/api " + r.Path}; !slices.Equal(f.removals, want) {
			t.Errorf("y removed %q, want %q", f.removals, want)
		}
		if !closed {
			t.Errorf("after removing %s, the dialog is still open:\n%s", r.Path, view(m))
		}
		if got, want := said(m), "wt: removed "+r.Path; got != want {
			t.Errorf("after removing %s, the dialog says %q, want %q", r.Path, got, want)
		}
	}
}

// While wt runs, the dialog says so, and keys do nothing, ctrl+c included:
// stopping wt halfway would leave the worktree half removed.
func TestKeysDoNothingWhileWtRuns(t *testing.T) {
	f := &fakeWt{}
	m, cmd := update(t, asking(t, f, login, 70, 8), key("y"))
	if cmd == nil {
		t.Fatal("y does nothing")
	}
	if got := view(m); !strings.HasSuffix(got, "\n removing…") || !strings.HasPrefix(got, " remove api--fix-login and end session login?\n") {
		t.Errorf("while wt runs, the view is:\n%s", got)
	}
	for _, k := range []string{"y", "n", "q", "esc", "ctrl+c"} {
		if _, cmd := update(t, m, key(k)); cmd != nil {
			t.Errorf("%s while wt runs gives a command", k)
		}
	}
	if len(f.removals) > 0 {
		t.Errorf("wt ran before y's command did: %q", f.removals)
	}
}

// Forcing can't get past a failure that isn't a safety refusal (e.g. a
// lock), so the dialog shows what wt said, offering no force, until any key
// closes it; then it says it for the status line too.
func TestAnotherFailureShowsWtsMessageWithNoForceOffer(t *testing.T) {
	locked := "wt: /p/api--fix-login is locked (git worktree unlock it first)"
	f := &fakeWt{err: errors.New(locked)}
	m, closed := answer(t, asking(t, f, login, 70, 6), "y")
	if closed {
		t.Fatal("the dialog closed on wt's failure, before showing it")
	}
	want := " wt: /p/api--fix-login is locked (git worktree unlock it first)\n\n\n\n\n any key closes"
	if got := view(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
	if got := said(m); got != locked {
		t.Errorf("the dialog says %q, want wt's failure", got)
	}
	m, closed = answer(t, m, "y")
	if !closed {
		t.Error("y after the failure doesn't close the dialog")
	}
	if want := []string{"/p/api /p/api--fix-login"}; !slices.Equal(f.removals, want) {
		t.Errorf("removed %q, want %q: no force", f.removals, want)
	}
	if got := said(m); got != locked {
		t.Errorf("once closed, the dialog says %q, want wt's failure", got)
	}
}

// loginRefusal is how wt refuses to remove api--fix-login, which has
// uncommitted changes.
const loginRefusal = "wt: not removing /p/api--fix-login:\nuncommitted changes:\n?? wip.txt\n(wt rm -f to force)"

// When wt refuses because something would be lost, the dialog shows wt's
// reasons and offers to force; forcing is a removal like any other.
func TestSafetyRefusalOffersToForce(t *testing.T) {
	f := &fakeWt{refusal: loginRefusal}
	m, closed := answer(t, asking(t, f, login, 70, 10), "y")
	if closed {
		t.Fatal("the dialog closed on the refusal")
	}
	want := " wt: not removing /p/api--fix-login:\n uncommitted changes:\n ?? wip.txt\n (wt rm -f to force)\n\n\n\n\n force?\n y: force   n/q/esc: cancel"
	if got := view(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
	if got := said(m); got != "" {
		t.Errorf("before forcing, the dialog says %q, want nothing", got)
	}
	m, closed = answer(t, m, "y")
	if want := []string{"/p/api /p/api--fix-login", "/p/api -f /p/api--fix-login"}; !slices.Equal(f.removals, want) {
		t.Errorf("removed %q, want %q", f.removals, want)
	}
	if !closed || said(m) != "wt: removed /p/api--fix-login" {
		t.Errorf("after forcing, closed = %v and the dialog says %q, want it closed, saying what wt said", closed, said(m))
	}
}

// If forcing fails too, that's another failure: wt's message, no offer.
func TestAFailedForceShowsWtsMessage(t *testing.T) {
	f := &fakeWt{refusal: loginRefusal}
	m, _ := answer(t, asking(t, f, login, 70, 10), "y")
	f.err = wtFailure{wt.ErrSafetyRefusal, "wt: still not removing it"}
	m, closed := answer(t, m, "y")
	if closed || !strings.HasPrefix(view(m), " wt: still not removing it\n") || strings.Contains(view(m), "force") {
		t.Errorf("after forcing failed, closed = %v and the view is:\n%s", closed, view(m))
	}
	if got := said(m); got != "wt: still not removing it" {
		t.Errorf("the dialog says %q, want wt's failure", got)
	}
}

// n, q, esc and ctrl+c cancel either prompt, closing the dialog: nothing
// runs, and it says nothing. Other keys leave the prompt up.
func TestCancellingRunsNothing(t *testing.T) {
	for _, prompt := range []struct {
		name     string
		answers  []string // what gets to the prompt
		removals int      // how many removals wt has run by then
	}{
		{"remove", nil, 0},
		{"force", []string{"y"}, 1},
	} {
		for _, k := range []string{"n", "q", "esc", "ctrl+c"} {
			f := &fakeWt{refusal: loginRefusal}
			m, _ := answer(t, asking(t, f, login, 70, 10), prompt.answers...)
			before := view(m)
			for _, other := range []string{"x", "enter", "r", "j"} {
				var cmd tea.Cmd
				if m, cmd = update(t, m, key(other)); cmd != nil {
					t.Errorf("%s at the %s prompt gives a command", other, prompt.name)
				}
			}
			if view(m) != before {
				t.Errorf("other keys at the %s prompt changed it:\n%s", prompt.name, view(m))
			}
			_, cmd := update(t, m, key(k))
			if !quits(cmd) {
				t.Errorf("%s at the %s prompt doesn't close the dialog", k, prompt.name)
			}
			if len(f.removals) != prompt.removals || said(m) != "" {
				t.Errorf("%s at the %s prompt: removed %q and says %q, want nothing more", k, prompt.name, f.removals, said(m))
			}
		}
	}
}

// wt lists every file that would be lost, so its reasons can be longer than
// the popup, which would cut off what's past its bottom edge: the offer to
// force, or reasons further down (e.g. commits). The dialog shows the
// reasons that fit above the offer, and the rest scroll into view, so all
// of them can be read before forcing.
func TestALongSafetyRefusalScrolls(t *testing.T) {
	refusal := "wt: not removing /p/api--fix-login:\nuncommitted changes:\n"
	for i := range 30 {
		refusal += fmt.Sprintf("?? new-%02d.txt\n", i)
	}
	refusal += "(wt rm -f to force)"
	f := &fakeWt{refusal: refusal}
	m, _ := answer(t, asking(t, f, login, 40, 12), "y")
	top := ` wt: not removing /p/api--fix-login:
 uncommitted changes:
 ?? new-00.txt
 ?? new-01.txt
 ?? new-02.txt
 ?? new-03.txt
 ?? new-04.txt
 ?? new-05.txt
 ↑/↓ j/k: scroll (lines 1–8 of 33)

 force?
 y: force   n/q/esc: cancel`
	if got := view(m); got != top {
		t.Errorf("view:\n%s\nwant:\n%s", got, top)
	}
	for range 40 { // past the end
		m, _ = update(t, m, key("j"))
	}
	bottom := ` ?? new-23.txt
 ?? new-24.txt
 ?? new-25.txt
 ?? new-26.txt
 ?? new-27.txt
 ?? new-28.txt
 ?? new-29.txt
 (wt rm -f to force)
 ↑/↓ j/k: scroll (lines 26–33 of 33)

 force?
 y: force   n/q/esc: cancel`
	if got := view(m); got != bottom {
		t.Errorf("view after scrolling down:\n%s\nwant:\n%s", got, bottom)
	}
	for _, k := range []string{"k", "up"} {
		m, _ = update(t, m, key(k))
	}
	if got := view(m); !strings.HasPrefix(got, " ?? new-21.txt\n") || !strings.Contains(got, "(lines 24–31 of 33)") {
		t.Errorf("view after scrolling up 2 lines:\n%s", got)
	}
	m, _ = update(t, m, key("down"))
	if got := view(m); !strings.HasPrefix(got, " ?? new-22.txt\n") {
		t.Errorf("view after scrolling down a line:\n%s", got)
	}
	for range 40 { // past the start
		m, _ = update(t, m, key("k"))
	}
	if got := view(m); got != top {
		t.Errorf("view after scrolling back up:\n%s\nwant:\n%s", got, top)
	}
	answer(t, m, "y")
	if want := []string{"/p/api /p/api--fix-login", "/p/api -f /p/api--fix-login"}; !slices.Equal(f.removals, want) {
		t.Errorf("removed %q, want %q", f.removals, want)
	}
}

// The popup would cut off what's past its right edge, so the dialog wraps
// the question and wt's reasons, which name files and folders, to its width.
func TestTheDialogWrapsToThePopup(t *testing.T) {
	refusal := "wt: not removing /p/api--fix-login:\n" +
		".env is a real file/dir, not a link to the main clone (gitignored, would be lost): merge into /p/api/.env\n" +
		"(wt rm -f to force)"
	r := dialog.Removal{Path: login.Path, Sessions: []dialog.Ending{
		{Session: "login", Agents: []dialog.EndingAgent{{Window: "dev", State: "working"}, {Window: "shell", State: "needs you"}}},
	}}
	m := asking(t, &fakeWt{refusal: refusal}, r, 32, 30)
	fitsWidth(t, view(m), 32)
	question := "remove api--fix-login and end session login?the agents in login end too: working in dev, needs you in shell"
	if got := squeezed(view(m)); got != squeezed(question+"y: remove   n/q/esc: cancel") {
		t.Errorf("the question lost or changed some words:\n%s", view(m))
	}
	m, _ = answer(t, m, "y")
	fitsWidth(t, view(m), 32)
	if got := squeezed(view(m)); got != squeezed(refusal+"force?y: force   n/q/esc: cancel") {
		t.Errorf("the view lost or changed some of wt's reasons:\n%s", view(m))
	}
}

// fitsWidth fails the test if a line of text is wider than width.
func fitsWidth(t *testing.T, text string, width int) {
	t.Helper()
	for _, l := range strings.Split(text, "\n") {
		if w := ansi.StringWidth(l); w > width {
			t.Errorf("%q is %d columns wide, more than %d", l, w, width)
		}
	}
}

// squeezed is text without its spaces and line breaks.
func squeezed(text string) string { return strings.Join(strings.Fields(text), "") }

// The sidebar gives the dialog its Removal as one argument, which the dialog
// reads back as it was.
func TestARemovalIsOneArgument(t *testing.T) {
	r := dialog.Removal{Path: "/p/a b/api--fix", Missing: true, Sessions: []dialog.Ending{
		{Session: "lo gin", Agents: []dialog.EndingAgent{{Window: "dev: 1", State: "needs you"}}},
		{Session: "web"},
	}}
	got, err := dialog.ParseRemoval(r.Arg())
	if err != nil || !reflect.DeepEqual(got, r) {
		t.Errorf("ParseRemoval(%s) = %+v, %v; want %+v", r.Arg(), got, err, r)
	}
	for _, arg := range []string{"", "/p/api--fix", "{}", `{"path":`} {
		if _, err := dialog.ParseRemoval(arg); err == nil {
			t.Errorf("ParseRemoval(%q) took it", arg)
		}
	}
}
