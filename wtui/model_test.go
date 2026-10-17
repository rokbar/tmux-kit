package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// fakeWt answers List with whatever rows and error the test has set, Open
// with openErr, and Remove as wt would: removeErr if set, else a safety
// refusal with the text refusal if that's set and Remove doesn't force, else
// the worktree's row and folder are gone (so wt can't run there) and wt says so.
type fakeWt struct {
	rows      []wt.Worktree
	err       error
	lists     []string // the folders List was called in
	opens     []string // the paths Open was called for
	openErr   error
	afterOpen []wt.Worktree // if set, the rows once an Open has worked, as wt started a session
	removes   []string      // the paths Remove was called for, as "-f <path>" when forced
	removeErr error
	refusal   string
	moved     []string // the folders Remove has moved away
}

func (f *fakeWt) List(dir string) ([]wt.Worktree, error) {
	f.lists = append(f.lists, dir)
	if slices.Contains(f.moved, dir) {
		return nil, errors.New("chdir " + dir + ": no such file or directory")
	}
	return f.rows, f.err
}

func (f *fakeWt) Open(dir, path string) error {
	f.opens = append(f.opens, path)
	if f.openErr == nil && f.afterOpen != nil {
		f.rows = f.afterOpen
	}
	return f.openErr
}

func (f *fakeWt) Remove(dir, path string, force bool) (string, error) {
	call := path
	if force {
		call = "-f " + path
	}
	f.removes = append(f.removes, call)
	if f.removeErr != nil {
		return "", f.removeErr
	}
	if f.refusal != "" && !force {
		return "", wtFailure{wt.ErrSafetyRefusal, f.refusal}
	}
	f.rows = slices.DeleteFunc(slices.Clone(f.rows), at(path))
	f.moved = append(f.moved, path)
	return "wt: removed " + path, nil
}

const repoDir = "/p/demo"

var (
	mainClone = wt.Worktree{Path: "/p/demo", Branch: "main", State: wt.Clean, MainClone: true}
	feat      = wt.Worktree{Path: "/p/demo--feat", Branch: "feat", State: wt.Dirty, Session: "api", DevURL: "http://localhost:4100"}
	gone      = wt.Worktree{Path: "/p/demo--gone", State: wt.Missing, Session: "gone"}
	rebase    = wt.Worktree{Path: "/p/demo--rebase", State: wt.Dirty}
)

// update sends msg to m.
func update(t *testing.T, m model, msg tea.Msg) (model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(model), cmd
}

// run runs cmd, which must be there, and returns its message.
func run(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command, want one")
	}
	return cmd()
}

func TestStartListsTheRepoAndShowsEachWorktree(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat, gone, rebase}}
	m := started(t, f)
	if want := []string{repoDir}; !slices.Equal(f.lists, want) {
		t.Errorf("listed in %q, want %q", f.lists, want)
	}
	want := `/p/demo

> demo (main clone)  main      clean    no session
  demo--feat         feat      dirty    api         http://localhost:4100
  demo--gone                   missing  gone
  demo--rebase       detached  dirty    no session

↑/↓ j/k: move   enter: open   x: remove   o: dev URL   r: refresh   q/esc: quit
`
	if got := m.View().Content; got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
}

// Opened in a worktree, wtui runs wt there, but names the repo by its main clone.
func TestHeaderNamesTheRepoByItsMainClone(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat}}
	m := newModel(f, &fakeReader{}, &fakeOpener{}, feat.Path, nil)
	m, _ = update(t, m, run(t, m.Init()))
	if want := []string{feat.Path}; !slices.Equal(f.lists, want) {
		t.Errorf("listed in %q, want %q", f.lists, want)
	}
	if header, _, _ := strings.Cut(m.View().Content, "\n"); header != mainClone.Path {
		t.Errorf("header = %q, want the main clone's path %q", header, mainClone.Path)
	}
}

// key is a key press: a printable key by its text, else "up", "down", "esc"
// or "enter".
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
	}
	return tea.KeyPressMsg{Code: []rune(name)[0], Text: name}
}

// started is wtui on repoDir once its first list, from f, has arrived.
func started(t *testing.T, f *fakeWt) model {
	t.Helper()
	return startedOpening(t, f, &fakeOpener{})
}

// selectedRow is the view's line for the selected row.
func selectedRow(t *testing.T, m model) string {
	t.Helper()
	for _, line := range strings.Split(m.View().Content, "\n") {
		if strings.HasPrefix(line, "> ") {
			return line
		}
	}
	t.Fatalf("no selected row in view:\n%s", m.View().Content)
	return ""
}

func TestUpDownAndJKMoveTheSelection(t *testing.T) {
	m := started(t, &fakeWt{rows: []wt.Worktree{mainClone, feat, gone}})
	if m.selected != mainClone.Path {
		t.Fatalf("selected %q at start, want the main clone", m.selected)
	}
	for _, step := range []struct{ key, want string }{
		{"down", feat.Path},
		{"j", gone.Path},
		{"down", gone.Path}, // already the last row
		{"up", feat.Path},
		{"k", mainClone.Path},
		{"k", mainClone.Path}, // already the first row
		{"j", feat.Path},
	} {
		m, _ = update(t, m, key(step.key))
		if m.selected != step.want {
			t.Errorf("after %s: selected %q, want %q", step.key, m.selected, step.want)
		}
	}
	if row := selectedRow(t, m); !strings.Contains(row, "demo--feat ") {
		t.Errorf("selected row on screen is %q, want demo--feat's", row)
	}
}

// refresh presses r and delivers the list it asks for.
func refresh(t *testing.T, m model) model {
	t.Helper()
	m, cmd := update(t, m, key("r"))
	m, _ = update(t, m, run(t, cmd))
	return m
}

func TestRRefreshesTheList(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone}}
	m := started(t, f)
	f.rows = []wt.Worktree{mainClone, feat}
	m = refresh(t, m)
	if want := []string{repoDir, repoDir}; !slices.Equal(f.lists, want) {
		t.Errorf("listed in %q, want %q", f.lists, want)
	}
	if view := m.View().Content; !strings.Contains(view, "demo--feat") {
		t.Errorf("view after r doesn't show the new worktree:\n%s", view)
	}
}

func TestSelectionStaysOnTheSameWorktreeWhenRowsMove(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat, rebase}}
	m := started(t, f)
	m, _ = update(t, m, key("j"))
	// a new worktree comes before feat, so feat's row moves down
	bugfix := wt.Worktree{Path: "/p/demo--bugfix", Branch: "bugfix", State: wt.Clean}
	f.rows = []wt.Worktree{mainClone, bugfix, feat, rebase}
	m = refresh(t, m)
	if m.selected != feat.Path {
		t.Errorf("selected %q after the reload, want %q", m.selected, feat.Path)
	}
	if row := selectedRow(t, m); !strings.Contains(row, "demo--feat ") {
		t.Errorf("selected row on screen is %q, want demo--feat's", row)
	}
}

func TestSelectionFallsBackToTheFirstRowWhenItsWorktreeIsGone(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat, rebase}}
	m := started(t, f)
	m, _ = update(t, m, key("j"))
	m, _ = update(t, m, key("j"))
	f.rows = []wt.Worktree{mainClone, feat}
	m = refresh(t, m)
	if m.selected != mainClone.Path {
		t.Errorf("selected %q after rebase's worktree went, want the main clone", m.selected)
	}
}

// quits reports whether cmd quits wtui.
func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestQAndEscQuit(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{key("q"), key("esc"), {Code: 'c', Mod: tea.ModCtrl}} {
		m := started(t, &fakeWt{rows: []wt.Worktree{mainClone}})
		if _, cmd := update(t, m, k); !quits(cmd) {
			t.Errorf("%s doesn't quit", k)
		}
	}
	m := started(t, &fakeWt{rows: []wt.Worktree{mainClone}})
	for _, k := range []string{"x", "j", "k"} {
		if _, cmd := update(t, m, key(k)); quits(cmd) {
			t.Errorf("%s quits", k)
		}
	}
}

func TestStartupProblemsShowAMessageUntilAKeyIsPressed(t *testing.T) {
	for _, c := range []struct {
		name             string
		findErr, listErr error
		want             string
	}{
		{"not in a git repo", fmt.Errorf("%w: /tmp/x", errNotInRepo), nil, "not in a git repo: /tmp/x"},
		{"wt too old", nil, fmt.Errorf("%w: usage: wt [options] <branch>", wt.ErrTooOld), "update tmux-kit"},
		{"wt not found", nil, fmt.Errorf("%w: not on PATH", wt.ErrNotFound), "wt not found"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeWt{err: c.listErr}
			m := newModel(f, &fakeReader{}, &fakeOpener{}, repoDir, c.findErr)
			if cmd := m.Init(); cmd != nil {
				m, _ = update(t, m, cmd())
			}
			if m.mode != messageMode {
				t.Errorf("mode = %v, want messageMode", m.mode)
			}
			if view := m.View().Content; !strings.Contains(view, c.want) || !strings.Contains(view, "press any key") {
				t.Errorf("view doesn't say %q and to press a key:\n%s", c.want, view)
			}
			// any key exits, even one the list uses
			if _, cmd := update(t, m, key("j")); !quits(cmd) {
				t.Error("j doesn't quit")
			}
		})
	}
}

// Outside a repo, wt would fail and look too old.
func TestNotInARepoRunsNoWt(t *testing.T) {
	f := &fakeWt{}
	m := newModel(f, &fakeReader{}, &fakeOpener{}, "", fmt.Errorf("%w: /tmp/x", errNotInRepo))
	if cmd := m.Init(); cmd != nil {
		update(t, m, cmd())
	}
	if len(f.lists) > 0 {
		t.Errorf("wt ls ran in %q", f.lists)
	}
}

// wtFailure is a failed wt call as the client reports it: of a kind (e.g.
// wt.ErrTooOld, as every failed `wt ls` is), with wt's message as its text.
type wtFailure struct {
	kind error
	msg  string
}

func (e wtFailure) Error() string { return e.msg }
func (e wtFailure) Unwrap() error { return e.kind }

func TestFailedRefreshShowsWtsErrorAndKeepsTheList(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat}}
	m := started(t, f)
	m, _ = update(t, m, key("j"))
	f.rows, f.err = nil, wtFailure{wt.ErrTooOld, "wt: not in a git repo"}
	m = refresh(t, m)
	if m.mode != listMode {
		t.Errorf("mode = %v, want listMode", m.mode)
	}
	view := m.View().Content
	if !strings.Contains(view, "\nwt: not in a git repo\n") {
		t.Errorf("view has no status line with wt's error:\n%s", view)
	}
	if row := selectedRow(t, m); !strings.Contains(row, "demo--feat ") {
		t.Errorf("selected row on screen is %q, want demo--feat's still", row)
	}

	// the next refresh that works clears it
	f.rows, f.err = []wt.Worktree{mainClone, feat}, nil
	m = refresh(t, m)
	if view := m.View().Content; strings.Contains(view, "not in a git repo") {
		t.Errorf("status line still there after a refresh that worked:\n%s", view)
	}
}

// fitsWidth fails the test for each line of text wider than width.
func fitsWidth(t *testing.T, text string, width int) {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if w := len([]rune(line)); w > width {
			t.Errorf("line %q is %d wide, want at most %d", line, w, width)
		}
	}
}

// unwrapped is text without its spaces and line breaks, which wrapping moves
// (it breaks lines at spaces, or after a slash), so wrapped text can be
// compared with what was wrapped.
func unwrapped(text string) string { return strings.Join(strings.Fields(text), "") }

// The terminal would cut long lines off at its edge, losing part of the message.
func TestStartupMessageWrapsToTheWindow(t *testing.T) {
	long := "/a/rather/long/path/to/a/folder/that/is/in/no/repo"
	m := newModel(&fakeWt{}, &fakeReader{}, &fakeOpener{}, "", fmt.Errorf("%w: %s", errNotInRepo, long))
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 30, Height: 10})
	view := m.View().Content
	fitsWidth(t, view, 30)
	if !strings.Contains(strings.ReplaceAll(view, "\n", ""), long) {
		t.Errorf("view lost part of the path:\n%s", view)
	}
}

// enter opens the selected worktree's session through wt: the main clone, a
// worktree with a session, and one mid-rebase with none.
func TestEnterOpensTheSelectedWorktree(t *testing.T) {
	for i, w := range []wt.Worktree{mainClone, feat, rebase} {
		f := &fakeWt{rows: []wt.Worktree{mainClone, feat, rebase}}
		m := started(t, f)
		for range i {
			m, _ = update(t, m, key("j"))
		}
		_, cmd := update(t, m, key("enter"))
		run(t, cmd)
		if want := []string{w.Path}; !slices.Equal(f.opens, want) {
			t.Errorf("enter on %s opened %q, want %q", w.Path, f.opens, want)
		}
	}
}

// A missing worktree has no folder to open a session in.
func TestEnterOnAMissingWorktreeDoesNothing(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, gone}}
	m := started(t, f)
	m, _ = update(t, m, key("j"))
	if _, cmd := update(t, m, key("enter")); cmd != nil {
		cmd()
		t.Error("enter on a missing worktree issues a command")
	}
	if len(f.opens) > 0 {
		t.Errorf("enter on a missing worktree opened %q", f.opens)
	}
}

// After wt opens a worktree, wtui lists again and hands off to the session
// started in that worktree's folder, whatever it's called, then quits.
func TestOpenHandsOffToTheSessionFoundByPath(t *testing.T) {
	// another worktree's session has bugfix's branch name, so wt names
	// bugfix's new session after its folder
	bugfix := wt.Worktree{Path: "/p/demo--bugfix", Branch: "bugfix", State: wt.Clean}
	other := wt.Worktree{Path: "/p/demo--other", Branch: "other", State: wt.Clean, Session: "bugfix"}
	f := &fakeWt{rows: []wt.Worktree{mainClone, bugfix, other}}
	m := started(t, f)
	m, _ = update(t, m, key("j"))
	withSession := bugfix
	withSession.Session = "demo--bugfix"
	f.afterOpen = []wt.Worktree{mainClone, withSession, other}

	m, cmd := update(t, m, key("enter"))
	m, cmd = update(t, m, run(t, cmd))
	if want := []string{repoDir, repoDir}; !slices.Equal(f.lists, want) {
		t.Errorf("listed in %q, want a fresh list after the open: %q", f.lists, want)
	}
	if !quits(cmd) {
		t.Error("wtui doesn't quit after the open")
	}
	if m.handoff != "demo--bugfix" {
		t.Errorf("hands off to session %q, want demo--bugfix", m.handoff)
	}
}

func TestFailedOpenShowsWtsErrorAndStays(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat}}
	m := started(t, f)
	m, _ = update(t, m, key("j"))
	f.openErr = errors.New("wt: tmux session feat already exists")

	m, cmd := update(t, m, key("enter"))
	m, cmd = update(t, m, run(t, cmd))
	if quits(cmd) {
		t.Error("wtui quits after a failed open")
	}
	// it reloads after every wt call, failed or not
	if want := []string{repoDir, repoDir}; !slices.Equal(f.lists, want) {
		t.Errorf("listed in %q, want a reload after the failed open: %q", f.lists, want)
	}
	if m.mode != listMode || m.handoff != "" {
		t.Errorf("mode = %v, handoff = %q; want listMode and none", m.mode, m.handoff)
	}
	view := m.View().Content
	if !strings.Contains(view, "\nwt: tmux session feat already exists\n") {
		t.Errorf("view has no status line with wt's error:\n%s", view)
	}
	if row := selectedRow(t, m); !strings.Contains(row, "demo--feat ") {
		t.Errorf("selected row on screen is %q, want demo--feat's still", row)
	}
}

// wt said it opened the session, but by the time wtui lists again none is
// started there (e.g. it ended at once): there's nothing to hand off to.
func TestOpenWithNoSessionListedThereStays(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, rebase}}
	m := started(t, f)
	m, _ = update(t, m, key("j"))

	m, cmd := update(t, m, key("enter"))
	m, cmd = update(t, m, run(t, cmd))
	if quits(cmd) || m.handoff != "" {
		t.Errorf("wtui quits (%v) to hand off to %q, want it to stay", quits(cmd), m.handoff)
	}
	if view := m.View().Content; !strings.Contains(view, "\nno session started in /p/demo--rebase\n") {
		t.Errorf("view has no status line saying there's no session:\n%s", view)
	}
}

// selectRow moves the selection down to the row for path.
func selectRow(t *testing.T, m model, path string) model {
	t.Helper()
	for range m.rows() {
		if m.selected == path {
			return m
		}
		m, _ = update(t, m, key("j"))
	}
	if m.selected != path {
		t.Fatalf("no row for %s to select", path)
	}
	return m
}

// x asks first, naming the worktree and the session removing it would end,
// and runs nothing until it's answered.
func TestXAsksBeforeRemovingNamingTheSessionItEnds(t *testing.T) {
	for _, c := range []struct {
		w    wt.Worktree
		want string
	}{
		{feat, "remove demo--feat and end session api?"},
		{rebase, "remove demo--rebase (no session)?"},
		{gone, "remove demo--gone and end session gone?"},
	} {
		f := &fakeWt{rows: []wt.Worktree{mainClone, feat, gone, rebase}}
		m := selectRow(t, started(t, f), c.w.Path)
		m, cmd := update(t, m, key("x"))
		if cmd != nil {
			cmd()
			t.Errorf("x on %s issues a command before it's answered", c.w.Path)
		}
		if len(f.removes) > 0 {
			t.Errorf("x on %s removed %q before it's answered", c.w.Path, f.removes)
		}
		if m.mode != confirmMode {
			t.Errorf("x on %s: mode = %v, want confirmMode", c.w.Path, m.mode)
		}
		view := m.View().Content
		if !strings.HasSuffix(view, "\n"+c.want+"\ny: remove   n/q/esc: cancel\n") {
			t.Errorf("x on %s: view doesn't end with %q and the prompt's keys:\n%s", c.w.Path, c.want, view)
		}
		if strings.Contains(view, "enter: open") {
			t.Errorf("x on %s: the prompt shows the list's keys:\n%s", c.w.Path, view)
		}
	}
}

// wt never removes the main clone, so x doesn't even ask.
func TestXDoesNothingOnTheMainClone(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat}}
	m := started(t, f)
	before := m.View().Content
	m, cmd := update(t, m, key("x"))
	if cmd != nil {
		cmd()
		t.Error("x on the main clone issues a command")
	}
	if m.mode != listMode || len(f.removes) > 0 {
		t.Errorf("x on the main clone: mode = %v, removed %q; want listMode and nothing", m.mode, f.removes)
	}
	if view := m.View().Content; view != before {
		t.Errorf("x on the main clone changed the view to:\n%s", view)
	}
}

// pressXThen presses x on the row for path, then answers its prompt(s) with keys,
// delivering what each answer's command reports. It returns wtui as it is
// after the last answer.
func pressXThen(t *testing.T, m model, path string, keys ...string) model {
	t.Helper()
	m, _ = update(t, selectRow(t, m, path), key("x"))
	for _, k := range keys {
		var cmd tea.Cmd
		m, cmd = update(t, m, key(k))
		if cmd != nil {
			m, _ = update(t, m, cmd())
		}
	}
	return m
}

// After wt removes a worktree, its message (e.g. that it kept the branch) is
// in the status line, under a list that no longer has the worktree.
func TestYRemovesTheWorktreeAndShowsWtsMessage(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat, rebase}}
	m := pressXThen(t, started(t, f), feat.Path, "y")
	if want := []string{feat.Path}; !slices.Equal(f.removes, want) {
		t.Errorf("removed %q, want %q", f.removes, want)
	}
	if want := []string{repoDir, repoDir}; !slices.Equal(f.lists, want) {
		t.Errorf("listed in %q, want a reload after the removal: %q", f.lists, want)
	}
	if m.mode != listMode {
		t.Errorf("mode = %v, want listMode", m.mode)
	}
	view := m.View().Content
	if !strings.Contains(view, "\nwt: removed /p/demo--feat\n↑/↓") {
		t.Errorf("view has no status line with wt's message above the keys:\n%s", view)
	}
	if strings.Contains(view, "demo--feat ") {
		t.Errorf("the removed worktree is still listed:\n%s", view)
	}
}

// Forcing can't get past a failure that isn't a safety refusal (e.g. a lock),
// so wtui shows what wt said and doesn't offer to.
func TestOtherRemoveFailureShowsWtsMessageWithNoForceOffer(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat}}
	f.removeErr = errors.New("wt: /p/demo--feat is locked (git worktree unlock it first)")
	m := pressXThen(t, started(t, f), feat.Path, "y")
	if want := []string{feat.Path}; !slices.Equal(f.removes, want) {
		t.Errorf("removed %q, want %q", f.removes, want)
	}
	if want := []string{repoDir, repoDir}; !slices.Equal(f.lists, want) {
		t.Errorf("listed in %q, want a reload after the failed removal: %q", f.lists, want)
	}
	if m.mode != listMode {
		t.Errorf("mode = %v, want listMode", m.mode)
	}
	view := m.View().Content
	if !strings.Contains(view, "\nwt: /p/demo--feat is locked (git worktree unlock it first)\n↑/↓") {
		t.Errorf("view has no status line with wt's message above the keys:\n%s", view)
	}
	if strings.Contains(view, "force") {
		t.Errorf("view offers to force:\n%s", view)
	}
	// y now means nothing: there's no prompt
	if _, cmd := update(t, m, key("y")); cmd != nil {
		cmd()
		t.Errorf("y after the failure issues a command; removed %q", f.removes)
	}
}

// featRefusal is how wt refuses to remove demo--feat, which has uncommitted
// changes.
const featRefusal = "wt: not removing /p/demo--feat:\nuncommitted changes:\n?? wip.txt\n(wt rm -f to force)"

// When wt refuses because something would be lost, wtui shows wt's reasons and
// offers to force; forcing is a removal like any other.
func TestSafetyRefusalOffersToForce(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat}, refusal: featRefusal}
	m := pressXThen(t, started(t, f), feat.Path, "y")
	if want := []string{repoDir, repoDir}; !slices.Equal(f.lists, want) {
		t.Errorf("listed in %q, want a reload after the refusal: %q", f.lists, want)
	}
	if m.mode != refusedMode {
		t.Fatalf("mode = %v after the refusal, want refusedMode", m.mode)
	}
	if view, want := m.View().Content, featRefusal+"\n\nforce?\ny: force   n/q/esc: cancel\n"; view != want {
		t.Errorf("view:\n%s\nwant:\n%s", view, want)
	}

	m, cmd := update(t, m, key("y"))
	m, _ = update(t, m, run(t, cmd))
	if want := []string{feat.Path, "-f " + feat.Path}; !slices.Equal(f.removes, want) {
		t.Errorf("removed %q, want %q", f.removes, want)
	}
	if want := []string{repoDir, repoDir, repoDir}; !slices.Equal(f.lists, want) {
		t.Errorf("listed in %q, want a reload after the forced removal: %q", f.lists, want)
	}
	if m.mode != listMode {
		t.Errorf("mode = %v after forcing, want listMode", m.mode)
	}
	view := m.View().Content
	if !strings.Contains(view, "\nwt: removed /p/demo--feat\n↑/↓") {
		t.Errorf("view has no status line with wt's message above the keys:\n%s", view)
	}
	if strings.Contains(view, "demo--feat ") {
		t.Errorf("the removed worktree is still listed:\n%s", view)
	}
}

// n, q and esc cancel either prompt, back to the list: nothing runs, and wtui
// stays open. Other keys leave the prompt up.
func TestCancellingAPromptRunsNothing(t *testing.T) {
	for _, prompt := range []struct {
		name    string
		answers []string // what gets to the prompt, after x
		removes []string // what wt has removed by then
	}{
		{"confirm", nil, nil},
		{"force", []string{"y"}, []string{feat.Path}},
	} {
		for _, k := range []string{"n", "q", "esc"} {
			f := &fakeWt{rows: []wt.Worktree{mainClone, feat}, refusal: featRefusal}
			m := pressXThen(t, started(t, f), feat.Path, prompt.answers...)
			lists := len(f.lists)
			// keys the list uses do nothing at a prompt
			for _, other := range []string{"j", "x", "enter", "r"} {
				var cmd tea.Cmd
				if m, cmd = update(t, m, key(other)); cmd != nil {
					cmd()
					t.Errorf("%s at the %s prompt issues a command", other, prompt.name)
				}
			}
			if m.mode == listMode {
				t.Fatalf("the %s prompt is gone before %s", prompt.name, k)
			}

			m, cmd := update(t, m, key(k))
			if cmd != nil {
				cmd()
				t.Errorf("%s at the %s prompt issues a command", k, prompt.name)
			}
			if m.mode != listMode {
				t.Errorf("%s at the %s prompt: mode = %v, want listMode", k, prompt.name, m.mode)
			}
			if !slices.Equal(f.removes, prompt.removes) || len(f.lists) != lists {
				t.Errorf("%s at the %s prompt: removed %q and listed %d more times, want nothing run",
					k, prompt.name, f.removes, len(f.lists)-lists)
			}
			if view := m.View().Content; !strings.Contains(view, "> demo--feat ") || !strings.HasSuffix(view, "q/esc: quit\n") {
				t.Errorf("%s at the %s prompt: view isn't the list with feat selected:\n%s", k, prompt.name, view)
			}
		}
	}
}

// Unlike enter, x works on a missing worktree: wt unregisters it, and its row
// goes.
func TestXOnAMissingWorktreeUnregistersIt(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, gone, feat}}
	m := pressXThen(t, started(t, f), gone.Path, "y")
	if want := []string{gone.Path}; !slices.Equal(f.removes, want) {
		t.Errorf("removed %q, want %q", f.removes, want)
	}
	if view := m.View().Content; strings.Contains(view, "demo--gone ") {
		t.Errorf("the missing worktree is still listed:\n%s", view)
	}
	if m.selected != mainClone.Path {
		t.Errorf("selected %q once its row went, want the first row", m.selected)
	}
}

// ctrl+c quits from anywhere, a prompt included, and runs nothing.
func TestCtrlCQuitsFromAPrompt(t *testing.T) {
	for _, answers := range [][]string{nil, {"y"}} {
		f := &fakeWt{rows: []wt.Worktree{mainClone, feat}, refusal: featRefusal}
		m := pressXThen(t, started(t, f), feat.Path, answers...)
		removes := len(f.removes)
		if _, cmd := update(t, m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); !quits(cmd) {
			t.Errorf("ctrl+c at mode %v doesn't quit", m.mode)
		}
		if len(f.removes) != removes {
			t.Errorf("ctrl+c at mode %v removed %q", m.mode, f.removes[removes:])
		}
	}
}

// wt lists every file that would be lost, so its reasons can be longer than
// the window, which would cut off what's past its bottom edge: the prompt, or
// reasons further down (e.g. commits). wtui shows the reasons that fit above
// the prompt, and the rest scroll into view, so all of them can be read
// before forcing.
func TestLongSafetyRefusalScrolls(t *testing.T) {
	refusal := "wt: not removing /p/demo--feat:\nuncommitted changes:\n"
	for i := range 30 {
		refusal += fmt.Sprintf("?? new-%02d.txt\n", i)
	}
	refusal += "(wt rm -f to force)"
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat}, refusal: refusal}
	m := started(t, f)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 40, Height: 12})
	m = pressXThen(t, m, feat.Path, "y")

	top := `wt: not removing /p/demo--feat:
uncommitted changes:
?? new-00.txt
?? new-01.txt
?? new-02.txt
?? new-03.txt
?? new-04.txt
?? new-05.txt
↑/↓ j/k: scroll (lines 1–8 of 33)

force?
y: force   n/q/esc: cancel
`
	if view := m.View().Content; view != top {
		t.Errorf("view:\n%s\nwant:\n%s", view, top)
	}
	for range 40 { // past the end
		m, _ = update(t, m, key("j"))
	}
	bottom := `?? new-23.txt
?? new-24.txt
?? new-25.txt
?? new-26.txt
?? new-27.txt
?? new-28.txt
?? new-29.txt
(wt rm -f to force)
↑/↓ j/k: scroll (lines 26–33 of 33)

force?
y: force   n/q/esc: cancel
`
	if view := m.View().Content; view != bottom {
		t.Errorf("view after scrolling down:\n%s\nwant:\n%s", view, bottom)
	}
	for _, k := range []string{"k", "up"} {
		m, _ = update(t, m, key(k))
	}
	if view := m.View().Content; !strings.HasPrefix(view, "?? new-21.txt\n") || !strings.Contains(view, "(lines 24–31 of 33)") {
		t.Errorf("view after scrolling up 2 lines:\n%s", view)
	}
	for range 40 { // past the start
		m, _ = update(t, m, key("k"))
	}
	if view := m.View().Content; view != top {
		t.Errorf("view after scrolling back up:\n%s\nwant:\n%s", view, top)
	}

	m, cmd := update(t, m, key("y"))
	update(t, m, run(t, cmd))
	if want := []string{feat.Path, "-f " + feat.Path}; !slices.Equal(f.removes, want) {
		t.Errorf("removed %q, want %q", f.removes, want)
	}
}

// The screen would cut off what's past its right edge, so wtui wraps wt's
// reasons, which name files and folders, to the window's width.
func TestSafetyRefusalWrapsToTheWindow(t *testing.T) {
	refusal := "wt: not removing /p/demo--feat:\n" +
		".env is a real file/dir, not a link to the main clone (gitignored, would be lost): merge into /p/demo/.env\n" +
		"(wt rm -f to force)"
	m := started(t, &fakeWt{rows: []wt.Worktree{mainClone, feat}, refusal: refusal})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 30, Height: 30})
	m = pressXThen(t, m, feat.Path, "y")

	view := m.View().Content
	fitsWidth(t, view, 30)
	if got, want := unwrapped(view), unwrapped(refusal+"force?y: force n/q/esc: cancel"); got != want {
		t.Errorf("view lost or changed some of wt's reasons:\n%s", view)
	}
	if !strings.HasSuffix(view, "\n\nforce?\ny: force   n/q/esc: cancel\n") {
		t.Errorf("view doesn't end with the prompt:\n%s", view)
	}
}

// wt's messages name folders by their full path, so the status line can be
// wider than the window, which would cut it off. wtui wraps it.
func TestStatusLineWrapsToTheWindow(t *testing.T) {
	long := wt.Worktree{Path: "/a/rather/long/path/to/the/projects/demo--feat", Branch: "feat", State: wt.Clean}
	m := started(t, &fakeWt{rows: []wt.Worktree{mainClone, long}})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 40, Height: 30})
	m = pressXThen(t, m, long.Path, "y")

	view := m.View().Content
	_, status, ok := strings.Cut(view, "\nwt: removed")
	if !ok {
		t.Fatalf("view has no status line with wt's message:\n%s", view)
	}
	status, _, _ = strings.Cut("wt: removed"+status, "↑/↓")
	if got, want := unwrapped(status), unwrapped("wt: removed "+long.Path); got != want {
		t.Errorf("status line says %q, want wt's message %q", status, "wt: removed "+long.Path)
	}
	fitsWidth(t, status, 40)
}

// Opened in a worktree (e.g. the popup from its session's pane), wtui can
// remove that worktree, folder and all, and still list the repo after.
func TestRemovingTheWorktreeWtuiWasOpenedInStillReloads(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat, rebase}}
	m := newModel(f, &fakeReader{}, &fakeOpener{}, feat.Path, nil)
	m, _ = update(t, m, run(t, m.Init()))
	m = pressXThen(t, m, feat.Path, "y")
	if want := []string{feat.Path}; !slices.Equal(f.removes, want) {
		t.Errorf("removed %q, want %q", f.removes, want)
	}
	view := m.View().Content
	if !strings.Contains(view, "\nwt: removed /p/demo--feat\n↑/↓") {
		t.Errorf("view has no status line with wt's message above the keys:\n%s", view)
	}
	if strings.Contains(view, "demo--feat ") {
		t.Errorf("the removed worktree is still listed:\n%s", view)
	}
	// and later reloads work too
	m = refresh(t, m)
	if view := m.View().Content; !strings.Contains(view, "demo--rebase") || strings.Contains(view, "no such file") {
		t.Errorf("view after r:\n%s", view)
	}
}

// The status line is what wt last said. A refusal is shown on its own screen,
// so it clears what an earlier action left there, which no longer applies.
func TestSafetyRefusalClearsTheStatusLine(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat, rebase}}
	m := pressXThen(t, started(t, f), rebase.Path, "y")
	if view := m.View().Content; !strings.Contains(view, "\nwt: removed /p/demo--rebase\n") {
		t.Fatalf("view has no status line from the first removal:\n%s", view)
	}
	f.refusal = featRefusal
	m = pressXThen(t, m, feat.Path, "y", "n")
	if view := m.View().Content; strings.Contains(view, "wt: removed") {
		t.Errorf("the first removal's status line is still there after the refusal:\n%s", view)
	}
}

// If the list after a removal fails, the status line says both: what wt did,
// and why the list (still showing the worktree) is old.
func TestFailedListAfterARemovalShowsBoth(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat}}
	m := started(t, f)
	f.err = wtFailure{wt.ErrTooOld, "wt: not in a git repo"}
	m = pressXThen(t, m, feat.Path, "y")
	if view := m.View().Content; !strings.Contains(view, "\nwt: removed /p/demo--feat\nwt: not in a git repo\n↑/↓") {
		t.Errorf("view doesn't show wt's message, then the list's error, above the keys:\n%s", view)
	}
}
