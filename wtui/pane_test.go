package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/detail"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// fakeReader answers Read with the details set for the path, or err, and
// notes each path it was asked about.
type fakeReader struct {
	details map[string]detail.Details
	err     error
	reads   []string // the paths Read was called for
}

func (r *fakeReader) Read(path string) (detail.Details, error) {
	r.reads = append(r.reads, path)
	return r.details[path], r.err
}

// Moving the selection reads the details of the worktree it moves to, and only
// then: the first row's once the list has arrived, then one per move. Read
// runs in a command, so keys never wait for git.
func TestSelectionChangeReadsTheNewlySelectedWorktree(t *testing.T) {
	r := &fakeReader{}
	m := newModel(&fakeWt{rows: []wt.Worktree{mainClone, feat, rebase}}, r, &fakeOpener{}, &fakeTmux{}, repoDir, nil)
	m, cmd := update(t, m, run(t, m.Init()))
	run(t, cmd)
	for _, k := range []string{"j", "j", "j", "k"} { // the third j is past the last row
		reads := len(r.reads)
		m, cmd = update(t, m, key(k))
		if len(r.reads) != reads {
			t.Fatalf("%s read details before returning, so keys wait for git", k)
		}
		if cmd != nil {
			cmd()
		}
	}
	if want := []string{mainClone.Path, feat.Path, rebase.Path, feat.Path}; !slices.Equal(r.reads, want) {
		t.Errorf("read the details of %q, want %q", r.reads, want)
	}
}

// featDetails is what git says about demo--feat: changed, with commits not
// pushed yet and one it hasn't pulled.
var featDetails = detail.Details{
	Changes:  []string{" M f", "?? wip.txt"},
	Commits:  []string{"1a2b3c4 work", "5d6e7f8 init"},
	Upstream: "origin/feat", Ahead: 2, Behind: 1,
}

// showing is wtui, started on f's rows, with the row for path selected and
// its details, from r, arrived.
func showing(t *testing.T, f *fakeWt, r *fakeReader, path string) model {
	t.Helper()
	m := newModel(f, r, &fakeOpener{}, &fakeTmux{}, repoDir, nil)
	m, read := update(t, m, run(t, m.Init()))
	for range m.rows() {
		if m.selected == path {
			m, _ = update(t, m, run(t, read))
			return m
		}
		m, read = update(t, m, key("j"))
	}
	t.Fatalf("no row for %s to select", path)
	return m
}

// Under the list, the pane shows how far the selected worktree is ahead of
// and behind its upstream, its changed files and its last commits.
func TestPaneShowsTheSelectedWorktreesDetails(t *testing.T) {
	r := &fakeReader{details: map[string]detail.Details{feat.Path: featDetails}}
	m := showing(t, &fakeWt{rows: []wt.Worktree{mainClone, feat}}, r, feat.Path)
	want := `/p/demo

  demo (main clone)  main  clean  no session
> demo--feat         feat  dirty  api         http://localhost:4100

2 ahead, 1 behind origin/feat
changes:
   M f
  ?? wip.txt
commits:
  1a2b3c4 work
  5d6e7f8 init

↑/↓ j/k: move   enter: open   n: picker   x: remove   o: dev URL   g: git UI   r: refresh   q/esc: quit
`
	if got := m.View().Content; got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
}

// The first line says where the worktree stands against its upstream, or
// that it has none: not pushed. A clean worktree has no changes to list.
func TestPaneSaysWhereTheWorktreeStandsAgainstItsUpstream(t *testing.T) {
	for _, c := range []struct {
		upstream      string
		ahead, behind int
		want          string
	}{
		{"", 0, 0, "not pushed"},
		{"origin/feat", 0, 0, "up to date with origin/feat"},
		{"origin/feat", 3, 0, "3 ahead of origin/feat"},
		{"origin/feat", 0, 4, "4 behind origin/feat"},
		{"origin/feat", 2, 1, "2 ahead, 1 behind origin/feat"},
	} {
		d := detail.Details{Commits: []string{"5d6e7f8 init"}, Upstream: c.upstream, Ahead: c.ahead, Behind: c.behind}
		r := &fakeReader{details: map[string]detail.Details{feat.Path: d}}
		m := showing(t, &fakeWt{rows: []wt.Worktree{mainClone, feat}}, r, feat.Path)
		want := "\n\n" + c.want + "\nno changes\ncommits:\n  5d6e7f8 init\n\n"
		if view := m.View().Content; !strings.Contains(view, want) {
			t.Errorf("%+v: view doesn't show %q:\n%s", d, want, view)
		}
	}
}

// rebaseDetails is what git says about demo--rebase, stopped mid-rebase.
var rebaseDetails = detail.Details{
	Changes: []string{"UU f"},
	Commits: []string{"9e8d7c6 ours"},
}

// The selection can move on before git has read the worktree it was on. What
// git read for that one arrives late, and is dropped: it would show one
// worktree's details under another.
func TestLateDetailsOfAnOldSelectionAreDropped(t *testing.T) {
	r := &fakeReader{details: map[string]detail.Details{feat.Path: featDetails, rebase.Path: rebaseDetails}}
	m := newModel(&fakeWt{rows: []wt.Worktree{mainClone, feat, rebase}}, r, &fakeOpener{}, &fakeTmux{}, repoDir, nil)
	m, _ = update(t, m, run(t, m.Init()))
	m, readFeat := update(t, m, key("j"))
	m, readRebase := update(t, m, key("j"))

	m, _ = update(t, m, run(t, readRebase))
	m, _ = update(t, m, run(t, readFeat))
	view := m.View().Content
	if !strings.Contains(view, "\n  UU f\n") {
		t.Errorf("view doesn't show the selected demo--rebase's changes:\n%s", view)
	}
	if strings.Contains(view, "wip.txt") {
		t.Errorf("view shows demo--feat's changes, which arrived late:\n%s", view)
	}
	// and before the selected one's have arrived, nothing
	m, readFeat = update(t, m, key("k"))
	m, _ = update(t, m, key("k"))
	m, _ = update(t, m, run(t, readFeat))
	if view := m.View().Content; strings.Contains(view, "wip.txt") || strings.Contains(view, "UU f") {
		t.Errorf("view shows another worktree's details under the main clone:\n%s", view)
	}
}

// r while a read is under way reads the same worktree again. What the newer
// read found is what the pane shows, even if the older read arrives after it.
func TestOlderDetailsOfTheSameWorktreeDoNotReplaceNewerOnes(t *testing.T) {
	r := &fakeReader{details: map[string]detail.Details{feat.Path: featDetails}}
	m := newModel(&fakeWt{rows: []wt.Worktree{mainClone, feat}}, r, &fakeOpener{}, &fakeTmux{}, repoDir, nil)
	m, _ = update(t, m, run(t, m.Init()))
	m, older := update(t, m, key("j"))
	readBefore := run(t, older) // what git found before the commit below

	r.details = map[string]detail.Details{feat.Path: {Commits: []string{"0f0f0f0 new", "1a2b3c4 work"}}}
	m, cmd := update(t, m, key("r"))
	m, newer := update(t, m, run(t, cmd))
	m, _ = update(t, m, run(t, newer))
	m, _ = update(t, m, readBefore)
	if view := m.View().Content; !strings.Contains(view, "\n  0f0f0f0 new\n") || strings.Contains(view, "wip.txt") {
		t.Errorf("view doesn't show what the newer read found:\n%s", view)
	}
}

// A missing worktree has no folder for git to read: nothing is read, and the
// pane is empty, not left showing the worktree selected before.
func TestMissingWorktreeReadsNothingAndShowsAnEmptyPane(t *testing.T) {
	r := &fakeReader{details: map[string]detail.Details{feat.Path: featDetails}}
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat, gone}}
	m := showing(t, f, r, feat.Path)
	reads := len(r.reads)
	m, cmd := update(t, m, key("j"))
	if m.selected != gone.Path {
		t.Fatalf("selected %q, want the missing worktree", m.selected)
	}
	if cmd != nil {
		cmd()
		t.Error("selecting a missing worktree issues a command")
	}
	// nor does a reload with it selected
	m, cmd = update(t, m, key("r"))
	if m, cmd = update(t, m, run(t, cmd)); cmd != nil {
		cmd()
		t.Error("a reload with a missing worktree selected issues a command after the list")
	}
	if len(r.reads) != reads {
		t.Errorf("read the details of %q, want nothing more", r.reads[reads:])
	}
	want := `/p/demo

  demo (main clone)  main  clean    no session
  demo--feat         feat  dirty    api         http://localhost:4100
> demo--gone               missing  gone

↑/↓ j/k: move   enter: open   n: picker   x: remove   o: dev URL   g: git UI   r: refresh   q/esc: quit
`
	if view := m.View().Content; view != want {
		t.Errorf("view:\n%s\nwant:\n%s", view, want)
	}
}

// If git can't read the worktree, the pane says what it said.
func TestPaneShowsWhyGitCouldNotRead(t *testing.T) {
	r := &fakeReader{err: errors.New("git log: fatal: your current branch 'feat' does not have any commits yet")}
	m := showing(t, &fakeWt{rows: []wt.Worktree{mainClone, feat}}, r, feat.Path)
	want := "> demo--feat         feat  dirty  api         http://localhost:4100\n\n" +
		"git log: fatal: your current branch 'feat' does not have any commits yet\n\n↑/↓"
	if view := m.View().Content; !strings.Contains(view, want) {
		t.Errorf("view doesn't show git's message in the pane:\n%s", view)
	}
}

// git's message can run to several lines, more than the pane has room for.
func TestLongGitMessageIsCutToThePanesRoom(t *testing.T) {
	r := &fakeReader{err: errors.New("git status: fatal: one\nhint: two\nhint: three\nhint: four")}
	m := showing(t, &fakeWt{rows: []wt.Worktree{mainClone, feat}}, r, feat.Path)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 110, Height: 9})
	want := "http://localhost:4100\n\ngit status: fatal: one\nhint: two\n\n↑/↓ j/k: move"
	if view := m.View().Content; !strings.Contains(view, want) || strings.Count(view, "\n") != 9 {
		t.Errorf("view doesn't show the first 2 lines of git's message in 9 lines:\n%s", view)
	}
}

// reloaded delivers what cmd, from a key that has wt act then list again,
// reports, then the details read after that list. It returns wtui as it is
// after them.
func reloaded(t *testing.T, m model, cmd tea.Cmd) model {
	t.Helper()
	m, read := update(t, m, run(t, cmd))
	m, _ = update(t, m, run(t, read))
	return m
}

// Each reload reads the selected worktree's details again, as they may have
// changed: after r, and after each wt call, which reloads too.
func TestEachReloadReadsTheSelectedWorktreeAgain(t *testing.T) {
	r := &fakeReader{details: map[string]detail.Details{feat.Path: featDetails}}
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat, rebase}}
	m := showing(t, f, r, feat.Path)
	reads := len(r.reads)

	// committed elsewhere: r shows it
	r.details = map[string]detail.Details{feat.Path: {Commits: []string{"0f0f0f0 new", "1a2b3c4 work"}}}
	m, cmd := update(t, m, key("r"))
	m = reloaded(t, m, cmd)
	if view := m.View().Content; !strings.Contains(view, "\n  0f0f0f0 new\n") || strings.Contains(view, "wip.txt") {
		t.Errorf("view after r doesn't show the details git reads now:\n%s", view)
	}

	f.openErr = errors.New("wt: tmux session feat already exists")
	m, cmd = update(t, m, key("enter"))
	m = reloaded(t, m, cmd)

	// removing it selects the main clone, whose details are read
	m, _ = update(t, m, key("x"))
	m, cmd = update(t, m, key("y"))
	reloaded(t, m, cmd)

	if want := []string{feat.Path, feat.Path, mainClone.Path}; !slices.Equal(r.reads[reads:], want) {
		t.Errorf("read the details of %q after r, a failed open and a removal; want %q", r.reads[reads:], want)
	}
}

// A worktree whose folder went while it was selected shows as missing after a
// reload: its details are gone with it, and nothing is read.
func TestWorktreeGoneMissingEmptiesThePane(t *testing.T) {
	r := &fakeReader{details: map[string]detail.Details{feat.Path: featDetails}}
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat}}
	m := showing(t, f, r, feat.Path)
	reads := len(r.reads)
	missing := feat
	missing.State = wt.Missing
	f.rows = []wt.Worktree{mainClone, missing}

	m, cmd := update(t, m, key("r"))
	m, cmd = update(t, m, run(t, cmd))
	if cmd != nil || len(r.reads) != reads {
		t.Errorf("the reload read the details of %q, want nothing", r.reads[reads:])
	}
	if view := m.View().Content; strings.Contains(view, "wip.txt") || strings.Contains(view, "origin/feat") {
		t.Errorf("view still shows the details from before it went missing:\n%s", view)
	}
}

// A read can be under way when a reload finds the worktree missing: what it
// read arrives after, and is dropped too.
func TestLateDetailsOfAWorktreeGoneMissingAreDropped(t *testing.T) {
	r := &fakeReader{details: map[string]detail.Details{feat.Path: featDetails}}
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat}}
	m := newModel(f, r, &fakeOpener{}, &fakeTmux{}, repoDir, nil)
	m, _ = update(t, m, run(t, m.Init()))
	m, readFeat := update(t, m, key("j"))
	missing := feat
	missing.State = wt.Missing
	f.rows = []wt.Worktree{mainClone, missing}
	m, cmd := update(t, m, key("r"))
	m, _ = update(t, m, run(t, cmd))

	m, _ = update(t, m, run(t, readFeat))
	if view := m.View().Content; strings.Contains(view, "wip.txt") || strings.Contains(view, "origin/feat") {
		t.Errorf("view shows what was read before the worktree went missing:\n%s", view)
	}
}

// longDetails has more changes and longer lines than a popup has room for.
func longDetails() detail.Details {
	d := detail.Details{Upstream: "origin/feat", Ahead: 1}
	for i := range 40 {
		d.Changes = append(d.Changes, fmt.Sprintf("?? src/generated/%02d/%s.txt", i, strings.Repeat("x", 120)))
	}
	for i := range 10 {
		d.Commits = append(d.Commits, fmt.Sprintf("%07d commit %d, whose subject goes on%s", i, i, strings.Repeat(" and on", 20)))
	}
	return d
}

// The pane gets the lines between the list and the keys, which stay at the
// bottom. Each long section is cut to its share of them, its last line saying
// how many more it has, and long lines are cut at the window's edge rather
// than wrapped onto the lines below.
func TestLongSectionsAreCutToFitTheWindow(t *testing.T) {
	r := &fakeReader{details: map[string]detail.Details{feat.Path: longDetails()}}
	m := showing(t, &fakeWt{rows: []wt.Worktree{mainClone, feat}}, r, feat.Path)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 17})
	want := `/p/demo

  demo (main clone)  main  clean  no session
> demo--feat         feat  dirty  api         http://localhost:4100

1 ahead of origin/feat
changes:
  ?? src/generated/00/xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx…
  ?? src/generated/01/xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx…
  … 38 more
commits:
  0000000 commit 0, whose subject goes on and on and on and on and on and on an…
  0000001 commit 1, whose subject goes on and on and on and on and on and on an…
  … 8 more

↑/↓ j/k: move   enter: open   n: picker   x: remove   o: dev URL   g: git UI
r: refresh   q/esc: quit
`
	if view := m.View().Content; view != want {
		t.Errorf("view:\n%s\nwant:\n%s", view, want)
	}
}

// wtui runs in an 80% × 70% popup: e.g. 128×29 in a 160×42 terminal, or
// smaller. There, every row and the keys stay on screen, and the pane fills
// what's left with as much of each section as fits.
func TestPaneFitsThePopup(t *testing.T) {
	rows := []wt.Worktree{mainClone, feat, gone, rebase}
	for _, size := range []tea.WindowSizeMsg{{Width: 128, Height: 29}, {Width: 80, Height: 24}} {
		r := &fakeReader{details: map[string]detail.Details{feat.Path: longDetails()}}
		m := showing(t, &fakeWt{rows: rows}, r, feat.Path)
		m, _ = update(t, m, size)
		view := m.View().Content
		fitsWidth(t, view, size.Width)
		lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
		if len(lines) != size.Height {
			t.Errorf("%dx%d: view is %d lines, want the window's %d:\n%s", size.Width, size.Height, len(lines), size.Height, view)
		}
		for _, row := range []string{"demo (main clone) ", "> demo--feat ", "demo--gone ", "demo--rebase "} {
			if !strings.Contains(view, row) {
				t.Errorf("%dx%d: view doesn't show the row %q:\n%s", size.Width, size.Height, row, view)
			}
		}
		if last := lines[len(lines)-1]; !strings.HasSuffix(last, "q/esc: quit") || !strings.Contains(view, "\n↑/↓ j/k: move") {
			t.Errorf("%dx%d: the last line is %q, want the keys to end there", size.Width, size.Height, last)
		}
		for _, want := range []string{"1 ahead of origin/feat", "\nchanges:\n", "\ncommits:\n  0000000 commit 0", "more\n"} {
			if !strings.Contains(view, want) {
				t.Errorf("%dx%d: view doesn't show %q:\n%s", size.Width, size.Height, want, view)
			}
		}
	}
}

// git prints a commit's subject as it was written, terminal codes and all,
// which could clear or draw over the list, or make a line wider than it looks.
func TestPaneShowsControlCharactersAsQuestionMarks(t *testing.T) {
	d := detail.Details{Commits: []string{"1a2b3c4 evil\x1b[2J\x1b[Hsubject\twith a tab\r"}}
	r := &fakeReader{details: map[string]detail.Details{feat.Path: d}}
	m := showing(t, &fakeWt{rows: []wt.Worktree{mainClone, feat}}, r, feat.Path)
	if view := m.View().Content; !strings.Contains(view, "\n  1a2b3c4 evil?[2J?[Hsubject?with a tab?\n") {
		t.Errorf("view doesn't show the subject's control characters as ?:\n%q", view)
	}
}

// x's prompt takes the place of the keys under the pane, which makes room
// for it.
func TestRemovePromptStaysOnScreenUnderThePane(t *testing.T) {
	r := &fakeReader{details: map[string]detail.Details{feat.Path: longDetails()}}
	m := showing(t, &fakeWt{rows: []wt.Worktree{mainClone, feat}}, r, feat.Path)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 16})
	m, _ = update(t, m, key("x"))
	view := m.View().Content
	if lines := strings.Split(view, "\n"); len(lines) != 17 { // and the last line's end
		t.Errorf("view is %d lines, want the window's 16:\n%s", len(lines)-1, view)
	}
	if !strings.HasSuffix(view, "\n  … 9 more\n\nremove demo--feat and end session api?\ny: remove   n/q/esc: cancel\n") {
		t.Errorf("view doesn't end with the pane, then the prompt:\n%s", view)
	}
}

// Whatever the pane has to show, or if it has nothing (yet), the keys stay at
// the bottom of the window, where they're expected.
func TestKeysStayAtTheBottom(t *testing.T) {
	short := detail.Details{Commits: []string{"5d6e7f8 init"}}
	r := &fakeReader{details: map[string]detail.Details{mainClone.Path: short}}
	f := &fakeWt{rows: []wt.Worktree{mainClone, feat, gone}}
	m := showing(t, f, r, mainClone.Path)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, step := range []string{"the main clone's details", "feat's not read yet", "the missing worktree"} {
		view := m.View().Content
		lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
		if len(lines) != 24 || !strings.HasSuffix(lines[23], "q/esc: quit") {
			t.Errorf("with %s, the keys aren't on the window's last line:\n%s", step, view)
		}
		m, _ = update(t, m, key("j"))
	}
}

// A long list leaves the pane little room, or none: it shows what fits, a
// section with no room for its lines saying how many it has, and the keys are
// still on screen.
func TestPaneShrinksToTheRoomTheListLeaves(t *testing.T) {
	r := &fakeReader{details: map[string]detail.Details{feat.Path: featDetails}}
	m := showing(t, &fakeWt{rows: []wt.Worktree{mainClone, feat}}, r, feat.Path)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 110, Height: 9})
	want := `/p/demo

  demo (main clone)  main  clean  no session
> demo--feat         feat  dirty  api         http://localhost:4100

2 ahead, 1 behind origin/feat
changes: … 2 more

↑/↓ j/k: move   enter: open   n: picker   x: remove   o: dev URL   g: git UI   r: refresh   q/esc: quit
`
	if view := m.View().Content; view != want {
		t.Errorf("view:\n%s\nwant:\n%s", view, want)
	}
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 110, Height: 6})
	want = `/p/demo

  demo (main clone)  main  clean  no session
> demo--feat         feat  dirty  api         http://localhost:4100

↑/↓ j/k: move   enter: open   n: picker   x: remove   o: dev URL   g: git UI   r: refresh   q/esc: quit
`
	if view := m.View().Content; view != want {
		t.Errorf("view with no room for the pane:\n%s\nwant:\n%s", view, want)
	}
}

// In a popup narrower than the key line (64 columns in an 80×24 terminal),
// the screen would cut the last keys off. They go on to a second line
// instead, each key whole, and the pane makes room for it.
func TestKeyLineBreaksBetweenKeysInANarrowWindow(t *testing.T) {
	r := &fakeReader{details: map[string]detail.Details{feat.Path: longDetails()}}
	m := showing(t, &fakeWt{rows: []wt.Worktree{mainClone, feat}}, r, feat.Path)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 64, Height: 16})
	view := m.View().Content
	keys := "↑/↓ j/k: move   enter: open   n: picker   x: remove   o: dev URL\ng: git UI   r: refresh   q/esc: quit\n"
	if !strings.HasSuffix(view, "\n\n"+keys) {
		t.Errorf("view doesn't end with the keys on two lines:\n%s", view)
	}
	fitsWidth(t, keys, 64)
	if lines := strings.Count(view, "\n"); lines != 16 {
		t.Errorf("view is %d lines, want the window's 16:\n%s", lines, view)
	}
	// exactly as wide as the keys: one line
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 103, Height: 16})
	if view := m.View().Content; !strings.HasSuffix(view, "\n\n"+wideKeyLine) {
		t.Errorf("view at 103 columns doesn't end with the keys on one line:\n%s", view)
	}
}
