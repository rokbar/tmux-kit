package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// fakeWt answers List with whatever rows and error the test has set.
type fakeWt struct {
	rows  []wt.Worktree
	err   error
	lists []string // the folders List was called in
}

func (f *fakeWt) List(dir string) ([]wt.Worktree, error) {
	f.lists = append(f.lists, dir)
	return f.rows, f.err
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

↑/↓ j/k: move   r: refresh   q/esc: quit
`
	if got := m.View().Content; got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
}

// Opened in a worktree, wtui runs wt there, but names the repo by its main clone.
func TestHeaderNamesTheRepoByItsMainClone(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat}}
	m := newModel(f, feat.Path, nil)
	m, _ = update(t, m, run(t, m.Init()))
	if want := []string{feat.Path}; !slices.Equal(f.lists, want) {
		t.Errorf("listed in %q, want %q", f.lists, want)
	}
	if header, _, _ := strings.Cut(m.View().Content, "\n"); header != mainClone.Path {
		t.Errorf("header = %q, want the main clone's path %q", header, mainClone.Path)
	}
}

// key is a key press: a printable key by its text, else "up", "down" or "esc".
func key(name string) tea.KeyPressMsg {
	switch name {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	}
	return tea.KeyPressMsg{Code: []rune(name)[0], Text: name}
}

// started is wtui on repoDir once its first list, from f, has arrived.
func started(t *testing.T, f *fakeWt) model {
	t.Helper()
	m := newModel(f, repoDir, nil)
	m, _ = update(t, m, run(t, m.Init()))
	return m
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
			m := newModel(f, repoDir, c.findErr)
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
	m := newModel(f, "", fmt.Errorf("%w: /tmp/x", errNotInRepo))
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

// The terminal would cut long lines off at its edge, losing part of the message.
func TestStartupMessageWrapsToTheWindow(t *testing.T) {
	long := "/a/rather/long/path/to/a/folder/that/is/in/no/repo"
	m := newModel(&fakeWt{}, "", fmt.Errorf("%w: %s", errNotInRepo, long))
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 30, Height: 10})
	view := m.View().Content
	for _, line := range strings.Split(view, "\n") {
		if w := len([]rune(line)); w > 30 {
			t.Errorf("line %q is %d wide, want at most 30", line, w)
		}
	}
	if !strings.Contains(strings.ReplaceAll(view, "\n", ""), long) {
		t.Errorf("view lost part of the path:\n%s", view)
	}
}
