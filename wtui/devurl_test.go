package main

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// fakeOpener answers Open with err, and notes each URL it was asked to open.
type fakeOpener struct {
	err    error
	opened []string // the URLs Open was called for
}

func (o *fakeOpener) Open(url string) error {
	o.opened = append(o.opened, url)
	return o.err
}

// wideKeyLine is the keys under the list, on one line, as in a window wide
// enough for them.
const wideKeyLine = "↑/↓ j/k: move   enter: open   n: picker   x: remove   o: dev URL   g: git UI   r: refresh   q/esc: quit\n"

// startedOpening is wtui on repoDir, opening dev URLs with o, once its first
// list, from f, has arrived.
func startedOpening(t *testing.T, f *fakeWt, o *fakeOpener) model {
	t.Helper()
	m := newModel(f, &fakeReader{}, o, &fakeTmux{}, repoDir, nil)
	m, _ = update(t, m, run(t, m.Init()))
	return m
}

// o opens the selected row's dev URL, exactly as wt listed it, from a
// command: keys never wait for the browser. The status line says it's open,
// and wtui stays open and runs no wt. The main clone's opens too.
func TestOOpensTheSelectedWorktreesDevURL(t *testing.T) {
	mainWithURL := mainClone
	mainWithURL.DevURL = "https://localhost:3000/?a=1&b=two words"
	for _, w := range []wt.Worktree{mainWithURL, feat} {
		f := &fakeWt{rows: []wt.Worktree{mainWithURL, feat}}
		o := &fakeOpener{}
		m := selectRow(t, startedOpening(t, f, o), w.Path)
		lists := len(f.lists)

		m, cmd := update(t, m, key("o"))
		if len(o.opened) > 0 {
			t.Fatal("o opened the URL before returning, so keys wait for the browser")
		}
		m, cmd = update(t, m, run(t, cmd))
		if want := []string{w.DevURL}; !slices.Equal(o.opened, want) {
			t.Errorf("o on %s opened %q, want %q", w.Path, o.opened, want)
		}
		if quits(cmd) || m.mode != listMode {
			t.Errorf("o on %s: wtui quits (%v) or left the list (mode %v)", w.Path, quits(cmd), m.mode)
		}
		if len(f.lists) != lists || len(f.opens) > 0 {
			t.Errorf("o on %s ran wt: listed %d more times, opened %q", w.Path, len(f.lists)-lists, f.opens)
		}
		if view := m.View().Content; !strings.HasSuffix(view, "\nopened "+w.DevURL+"\n"+wideKeyLine) {
			t.Errorf("o on %s: view doesn't end with the status line above the keys:\n%s", w.Path, view)
		}
	}
}

// The status line is about what wtui did last, so each o replaces what the
// one before said: the hint, an error, and then that it's open.
func TestOpeningReplacesAnEarlierHintOrError(t *testing.T) {
	o := &fakeOpener{err: errors.New("can't open http://localhost:4100: no xdg-open on PATH")}
	m := startedOpening(t, &fakeWt{rows: []wt.Worktree{mainClone, feat}}, o)
	m, _ = update(t, m, key("o")) // the main clone: no dev URL
	if view := m.View().Content; !strings.HasSuffix(view, "\ndemo has no dev URL\n"+wideKeyLine) {
		t.Fatalf("view doesn't end with the hint above the keys:\n%s", view)
	}
	for _, want := range []string{o.err.Error(), "opened http://localhost:4100"} {
		next, cmd := update(t, selectRow(t, m, feat.Path), key("o"))
		m, _ = update(t, next, run(t, cmd))
		if view := m.View().Content; !strings.HasSuffix(view, "\n"+want+"\n"+wideKeyLine) {
			t.Errorf("view doesn't end with %q alone above the keys:\n%s", want, view)
		}
		o.err = nil
	}
}

// With no dev URL to open, o only says so in the status line: nothing runs.
func TestOWithNoDevURLShowsAHint(t *testing.T) {
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat, rebase}}
	o := &fakeOpener{}
	m := selectRow(t, startedOpening(t, f, o), rebase.Path)
	lists := len(f.lists)

	m, cmd := update(t, m, key("o"))
	if cmd != nil {
		cmd()
		t.Error("o with no dev URL issues a command")
	}
	if len(o.opened) > 0 || len(f.lists) != lists || len(f.opens) > 0 {
		t.Errorf("o with no dev URL opened %q, listed %d more times, opened sessions %q; want nothing",
			o.opened, len(f.lists)-lists, f.opens)
	}
	if view := m.View().Content; !strings.HasSuffix(view, "\ndemo--rebase has no dev URL\n"+wideKeyLine) {
		t.Errorf("view doesn't end with the hint above the keys:\n%s", view)
	}
}

// A missing worktree has no dev server to open: o does nothing at all, even
// with a dev URL listed, and shows no hint.
func TestODoesNothingOnAMissingWorktree(t *testing.T) {
	goneWithURL := gone
	goneWithURL.DevURL = "http://localhost:4200"
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat, goneWithURL}}
	o := &fakeOpener{}
	m := selectRow(t, startedOpening(t, f, o), gone.Path)
	before := m.View().Content

	m, cmd := update(t, m, key("o"))
	if cmd != nil {
		cmd()
		t.Error("o on a missing worktree issues a command")
	}
	if len(o.opened) > 0 {
		t.Errorf("o on a missing worktree opened %q", o.opened)
	}
	if view := m.View().Content; view != before {
		t.Errorf("o on a missing worktree changed the view to:\n%s", view)
	}
}

// If the opener fails (e.g. there's no xdg-open), the status line says why,
// and wtui stays open.
func TestOpenerFailureShowsInTheStatusLine(t *testing.T) {
	o := &fakeOpener{err: errors.New("can't open http://localhost:4100: no xdg-open on PATH")}
	m := selectRow(t, startedOpening(t, &fakeWt{rows: []wt.Worktree{mainClone, feat}}, o), feat.Path)
	m, cmd := update(t, m, key("o"))
	m, cmd = update(t, m, run(t, cmd))
	if quits(cmd) || m.mode != listMode {
		t.Errorf("wtui quits (%v) or left the list (mode %v) after the opener failed", quits(cmd), m.mode)
	}
	if view := m.View().Content; !strings.HasSuffix(view, "\ncan't open http://localhost:4100: no xdg-open on PATH\n"+wideKeyLine) {
		t.Errorf("view doesn't end with the opener's error above the keys:\n%s", view)
	}
}
