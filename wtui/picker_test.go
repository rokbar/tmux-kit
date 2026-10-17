package main

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// fakeTmux is the tmux client wtui runs in, on session, or no client at all
// (wtui outside tmux) if session is "". The picker takes it to another
// session by changing session.
type fakeTmux struct{ session string }

func (c *fakeTmux) Watch() func() bool {
	before := c.session
	return func() bool { return before != "" && c.session != before }
}

// startedIn is wtui on repoDir, in the tmux client c, once its first list,
// from f, has arrived.
func startedIn(t *testing.T, f *fakeWt, c *fakeTmux) model {
	t.Helper()
	m := newModel(f, &fakeReader{}, &fakeOpener{}, c, repoDir, nil)
	m, _ = update(t, m, run(t, m.Init()))
	return m
}

// n runs the picker in the selected worktree's folder, so wt takes it as the
// invoking worktree: the main clone's, or a worktree's (mid-rebase too). It
// runs from a command, which pauses wtui while the picker has the terminal.
func TestNRunsThePickerInTheSelectedWorktree(t *testing.T) {
	for _, w := range []wt.Worktree{mainClone, feat, rebase} {
		f := &fakeWt{rows: []wt.Worktree{mainClone, feat, rebase}}
		m := selectRow(t, startedIn(t, f, &fakeTmux{session: "api"}), w.Path)
		_, cmd := update(t, m, key("n"))
		if len(f.picks) > 0 {
			t.Fatal("n ran the picker before returning, not from a command")
		}
		run(t, cmd)
		if want := []string{w.Path}; !slices.Equal(f.picks, want) {
			t.Errorf("n on %s ran the picker in %q, want %q", w.Path, f.picks, want)
		}
	}
}

// pickThen presses n on the row for path, then delivers what its command
// reports once the picker has closed. It returns wtui as it is then, and its
// next command.
func pickThen(t *testing.T, m model, path string) (model, tea.Cmd) {
	t.Helper()
	m, cmd := update(t, selectRow(t, m, path), key("n"))
	return update(t, m, run(t, cmd))
}

// The picker opened something, and took the tmux client to its session: wtui
// quits, which closes the popup. It has nothing to hand off (the client is
// already there) and nothing to list.
func TestPickerTakingTheClientToAnotherSessionQuits(t *testing.T) {
	c := &fakeTmux{session: "api"}
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat}, onPick: func() { c.session = "fresh" }}
	m := startedIn(t, f, c)
	lists := len(f.lists)
	m, cmd := pickThen(t, m, feat.Path)
	if !quits(cmd) {
		t.Error("wtui doesn't quit once the picker has taken the client to another session")
	}
	if m.handoff != "" || len(f.lists) != lists {
		t.Errorf("hands off to %q and listed %d more times, want neither", m.handoff, len(f.lists)-lists)
	}
}

// rowFor is the view's line for the worktree at path, or "" if it has none.
func rowFor(m model, path string) string {
	name := filepath.Base(path) + " "
	for _, line := range strings.Split(m.View().Content, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, "> "), name) {
			return line
		}
	}
	return ""
}

// fresh is a worktree the picker made, whose session it started.
var fresh = wt.Worktree{Path: "/p/demo--fresh", Branch: "fresh", State: wt.Clean, Session: "fresh"}

// The picker was cancelled, or opened the session the client was already on:
// the client's session is the same, so wtui stays and lists again, as the
// picker may have changed something (here, made a worktree). The selection
// stays where it was.
func TestPickerLeavingTheClientOnItsSessionReloadsAndStays(t *testing.T) {
	for _, picker := range []struct {
		name   string
		onPick func(*fakeTmux)
	}{
		{"cancelled", func(*fakeTmux) {}},
		{"opened the session the client is on", func(c *fakeTmux) { c.session = "api" }},
	} {
		tmux := &fakeTmux{session: "api"}
		f := &fakeWt{rows: []wt.Worktree{mainClone, feat}}
		f.onPick = func() {
			picker.onPick(tmux)
			f.rows = []wt.Worktree{mainClone, feat, fresh}
		}
		m := startedIn(t, f, tmux)
		lists := len(f.lists)
		m, cmd := pickThen(t, m, feat.Path)
		msg := run(t, cmd)
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Fatalf("%s: wtui quits", picker.name)
		}
		if len(f.lists) != lists+1 {
			t.Fatalf("%s: listed %d more times, want a reload", picker.name, len(f.lists)-lists)
		}
		m, _ = update(t, m, msg)
		if rowFor(m, fresh.Path) == "" {
			t.Errorf("%s: the reloaded list doesn't show the picker's new worktree:\n%s", picker.name, m.View().Content)
		}
		if m.mode != listMode || m.selected != feat.Path {
			t.Errorf("%s: mode %v, selected %q; want the list, on %s still", picker.name, m.mode, m.selected, feat.Path)
		}
	}
}

// Outside tmux there's no client for the picker to take anywhere: whatever it
// did (here, wt attached to the new worktree's session, then the user
// detached), wtui stays and lists again.
func TestOutsideTmuxThePickerReloadsAndStays(t *testing.T) {
	outside := &fakeTmux{}
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat}}
	f.onPick = func() { f.rows = []wt.Worktree{mainClone, feat, fresh} }
	m, cmd := pickThen(t, startedIn(t, f, outside), feat.Path)
	msg := run(t, cmd)
	if _, ok := msg.(tea.QuitMsg); ok {
		t.Fatal("wtui quits outside tmux")
	}
	m, _ = update(t, m, msg)
	if row := rowFor(m, fresh.Path); !slices.Equal(strings.Fields(row), []string{"demo--fresh", "fresh", "clean", "fresh"}) {
		t.Errorf("the reloaded list's row for the picker's new worktree is %q, want it with its session:\n%s", row, m.View().Content)
	}
}

// If the picker fails (e.g. wt can't make the branch), the status line says
// what wt said, and wtui lists again: wt may have got part of the way. Only
// a picker that worked can have opened something to quit to, so that holds
// even if the client is on another session (wt switches it last, so this
// would take something else moving it).
func TestPickerFailureShowsWtsErrorThenReloads(t *testing.T) {
	for _, moved := range []bool{false, true} {
		tmux := &fakeTmux{session: "api"}
		f := &fakeWt{rows: []wt.Worktree{mainClone, feat}, pickErr: errors.New("wt: invalid branch name: a b")}
		if moved {
			f.onPick = func() { tmux.session = "fresh" }
		}
		m := startedIn(t, f, tmux)
		lists := len(f.lists)
		m, cmd := pickThen(t, m, feat.Path)
		msg := run(t, cmd)
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Fatalf("client moved %v: wtui quits after the picker failed", moved)
		}
		if len(f.lists) != lists+1 {
			t.Fatalf("client moved %v: listed %d more times, want a reload", moved, len(f.lists)-lists)
		}
		m, _ = update(t, m, msg)
		if view := m.View().Content; !strings.HasSuffix(view, "\nwt: invalid branch name: a b\n"+wideKeyLine) {
			t.Errorf("client moved %v: view doesn't end with wt's error above the keys:\n%s", moved, view)
		}
	}
}

// The status line is about what wtui did last. Once the picker has closed,
// what was there before no longer applies: a failure replaces it, and so
// does the nothing a cancelled picker says.
func TestPickerReplacesAnEarlierStatusLine(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat}}
	m := startedIn(t, f, &fakeTmux{session: "api"})
	m, _ = update(t, m, key("o")) // the main clone: no dev URL
	if view := m.View().Content; !strings.HasSuffix(view, "\ndemo has no dev URL\n"+wideKeyLine) {
		t.Fatalf("view doesn't end with the hint above the keys:\n%s", view)
	}
	for _, err := range []error{errors.New("wt: invalid branch name: a b"), nil} {
		f.pickErr = err
		next, cmd := pickThen(t, m, feat.Path)
		m, _ = update(t, next, run(t, cmd))
		want := "\n\n" + wideKeyLine // the pane, then no status line
		if err != nil {
			want = "\n" + err.Error() + "\n" + wideKeyLine
		}
		if view := m.View().Content; !strings.HasSuffix(view, want) {
			t.Errorf("with the picker's error %v, view doesn't end with %q:\n%s", err, want, view)
		}
	}
}

// A missing worktree has no folder for the picker to run in: n does nothing
// at all.
func TestNDoesNothingOnAMissingWorktree(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat, gone}}
	m := selectRow(t, startedIn(t, f, &fakeTmux{session: "api"}), gone.Path)
	before := m.View().Content
	m, cmd := update(t, m, key("n"))
	if cmd != nil {
		cmd()
		t.Error("n on a missing worktree issues a command")
	}
	if len(f.picks) > 0 {
		t.Errorf("n on a missing worktree ran the picker in %q", f.picks)
	}
	if view := m.View().Content; view != before {
		t.Errorf("n on a missing worktree changed the view to:\n%s", view)
	}
}
