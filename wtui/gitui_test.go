package main

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/detail"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// g runs the git UI in the selected worktree: the main clone, or a worktree
// (mid-rebase too). It runs from a command, which pauses wtui while the git
// UI has the terminal.
func TestGRunsTheGitUIInTheSelectedWorktree(t *testing.T) {
	for _, w := range []wt.Worktree{mainClone, feat, rebase} {
		f := &fakeWt{rows: []wt.Worktree{mainClone, feat, rebase}}
		m := selectRow(t, started(t, f), w.Path)
		_, cmd := update(t, m, key("g"))
		if len(f.gitUIs) > 0 {
			t.Fatal("g ran the git UI before returning, not from a command")
		}
		run(t, cmd)
		if want := []string{w.Path}; !slices.Equal(f.gitUIs, want) {
			t.Errorf("g on %s ran the git UI in %q, want %q", w.Path, f.gitUIs, want)
		}
	}
}

// gitUIThen presses g on the row for path, then delivers what its command
// reports once the git UI has exited. It returns wtui as it is then, and its
// next command.
func gitUIThen(t *testing.T, m model, path string) (model, tea.Cmd) {
	t.Helper()
	m, cmd := update(t, selectRow(t, m, path), key("g"))
	return update(t, m, run(t, cmd))
}

// Once the git UI has exited, wtui is back and lists again, and git reads the
// selected worktree's details again: what was done in the git UI (here, a
// commit of what was changed) shows in its row and the pane. The selection
// stays where it was.
func TestGitUIExitingReloadsTheListAndThePane(t *testing.T) {
	r := &fakeReader{details: map[string]detail.Details{feat.Path: featDetails}}
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat}}
	m := showing(t, f, r, feat.Path)
	committed := feat
	committed.State = wt.Clean
	f.onGitUI = func() {
		f.rows = []wt.Worktree{mainClone, committed}
		r.details = map[string]detail.Details{feat.Path: {Commits: []string{"0f0f0f0 commit from the git UI", "1a2b3c4 work"}}}
	}
	lists := len(f.lists)

	m, cmd := gitUIThen(t, m, feat.Path)
	msg := run(t, cmd)
	if _, ok := msg.(tea.QuitMsg); ok {
		t.Fatal("wtui quits once the git UI has exited")
	}
	if len(f.lists) != lists+1 {
		t.Fatalf("listed %d more times, want a reload", len(f.lists)-lists)
	}
	m = reloaded(t, m, func() tea.Msg { return msg })
	if row := rowFor(m, feat.Path); !slices.Equal(strings.Fields(row), []string{">", "demo--feat", "feat", "clean", "api", "http://localhost:4100"}) {
		t.Errorf("the reloaded row for demo--feat is %q, want it clean and selected:\n%s", row, m.View().Content)
	}
	if view := m.View().Content; !strings.Contains(view, "\ncommits:\n  0f0f0f0 commit from the git UI\n") || strings.Contains(view, "wip.txt") {
		t.Errorf("the pane doesn't show what git reads after the git UI:\n%s", view)
	}
	if m.mode != listMode {
		t.Errorf("mode = %v, want the list", m.mode)
	}
}

// noGitUI is what wt says when no git UI is configured.
const noGitUI = "wt: no git UI configured: set WT_GIT_UI in /home/u/.config/wt/config.sh (e.g. WT_GIT_UI=lazygit)"

// If wt can't run the git UI (here, none is configured), the status line says
// what wt said, and wtui stays and lists again, as after any wt call.
func TestGitUIFailureShowsWtsTextAndStays(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat}, gitUIErr: errors.New(noGitUI)}
	m := started(t, f)
	lists := len(f.lists)
	m, cmd := gitUIThen(t, m, feat.Path)
	msg := run(t, cmd)
	if _, ok := msg.(tea.QuitMsg); ok {
		t.Fatal("wtui quits after the git UI failed")
	}
	if len(f.lists) != lists+1 {
		t.Fatalf("listed %d more times, want a reload", len(f.lists)-lists)
	}
	m, _ = update(t, m, msg)
	if view := m.View().Content; !strings.HasSuffix(view, "\n"+noGitUI+"\n"+wideKeyLine) {
		t.Errorf("view doesn't end with wt's message above the keys:\n%s", view)
	}
}

// The status line is about what wtui did last. Once the git UI has exited,
// what was there before no longer applies: a failure replaces it, and so does
// the nothing a git UI that worked says.
func TestGitUIReplacesAnEarlierStatusLine(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat}}
	m := started(t, f)
	m, _ = update(t, m, key("o")) // the main clone: no dev URL
	if view := m.View().Content; !strings.HasSuffix(view, "\ndemo has no dev URL\n"+wideKeyLine) {
		t.Fatalf("view doesn't end with the hint above the keys:\n%s", view)
	}
	for _, err := range []error{errors.New(noGitUI), nil} {
		f.gitUIErr = err
		next, cmd := gitUIThen(t, m, feat.Path)
		m, _ = update(t, next, run(t, cmd))
		want := "\n\n" + wideKeyLine // the pane, then no status line
		if err != nil {
			want = "\n" + err.Error() + "\n" + wideKeyLine
		}
		if view := m.View().Content; !strings.HasSuffix(view, want) {
			t.Errorf("with the git UI's error %v, view doesn't end with %q:\n%s", err, want, view)
		}
	}
}

// A missing worktree has no folder for the git UI to run in: g does nothing
// at all.
func TestGDoesNothingOnAMissingWorktree(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat, gone}}
	m := selectRow(t, started(t, f), gone.Path)
	before := m.View().Content
	m, cmd := update(t, m, key("g"))
	if cmd != nil {
		cmd()
		t.Error("g on a missing worktree issues a command")
	}
	if len(f.gitUIs) > 0 {
		t.Errorf("g on a missing worktree ran the git UI in %q", f.gitUIs)
	}
	if view := m.View().Content; view != before {
		t.Errorf("g on a missing worktree changed the view to:\n%s", view)
	}
}
