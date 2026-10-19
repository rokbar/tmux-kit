package sidebar

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// A worktree's row shows ± after its name if it's dirty, then its dev URL's
// port: on the worktree's own row, or on its one session's. Sessions under a
// worktree's row show none of their own. A dev URL without a port shows
// nothing.
func TestDirtyAndDevPortMarkers(t *testing.T) {
	f := worktrees()
	api := f.wt.lists["/p/api"]
	api[0].State = wt.Dirty                                         // main, with two sessions under it
	api[1].State, api[1].DevURL = wt.Dirty, "http://localhost:3001" // fix-login: login's row
	api[2].DevURL = "http://localhost:3002/app"                     // docs, without a session
	f.wt.lists["/p/Zed"][1].DevURL = "https://agent.test"           // agent's
	m := started(t, f)
	want := ` wtui
  ▾ api
    ▾ main ±
      ▸ api
      ▸ Bugfix
    ▾ login ± :3001
        0 shell
        1 dev
      docs :3002
      api--rebase ±
      api--gone ✗
  ▾ notes
    ▸ notes
  ▾ Zed
    ▸ zed
    ▸ agent`
	if got := plain(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
}

// In a narrow sidebar, a name too long for its row is cut short before its
// markers are, and they before the agent state's column at the right edge.
func TestALongNameIsCutBeforeItsMarkers(t *testing.T) {
	f := worktrees()
	f.normal.snap.Sessions[1].Name = "a-long-session-name" // fix-login's one
	fix := &f.wt.lists["/p/api"][1]
	fix.State, fix.DevURL = wt.Dirty, "http://localhost:3001"
	m, _ := update(t, started(t, f), tea.WindowSizeMsg{Width: 24, Height: 30})
	if got := plain(m); !strings.Contains(got, "\n    ▾ a-long-… ± :3001\n        0 shell\n") {
		t.Errorf("view:\n%s\nwant a-long-session-name cut short, with its markers", got)
	}
}

// clock stands in for the sidebar's timer: it records each tick the sidebar
// asks for (how long from then, and the message it's to give), and none goes
// off by itself. A test gives the sidebar the message when it likes.
type clock struct {
	waits []time.Duration
	msgs  []tea.Msg
}

func (c *clock) after(d time.Duration, msg tea.Msg) tea.Cmd {
	c.waits = append(c.waits, d)
	c.msgs = append(c.msgs, msg)
	return nil
}

// tick is the message of the last tick the sidebar asked for.
func (c *clock) tick(t *testing.T) tea.Msg {
	t.Helper()
	if len(c.msgs) == 0 {
		t.Fatal("the sidebar asked for no tick")
	}
	return c.msgs[len(c.msgs)-1]
}

// ticking is the sidebar, with c as its timer, once it has built its tree and
// read the active session.
func ticking(t *testing.T, f fakes) (Model, *clock) {
	t.Helper()
	c := &clock{}
	m := New(Clients{Host: f.host, Normal: f.normal, Repos: f.repos, Wt: f.wt}, `C-\`)
	m.after = c.after
	return feed(t, m, m.Init()), c
}

// Once the tree is built, every 10 seconds, each git project's worktrees are
// listed again, in the background, and the markers follow: ± and ports
// appear and clear.
func TestMarkersRefreshEveryTenSeconds(t *testing.T) {
	f := worktrees()
	m, c := ticking(t, f)
	every := []time.Duration{10 * time.Second}
	if !slices.Equal(c.waits, every) {
		t.Fatalf("once built, the sidebar asked for ticks in %v, want %v", c.waits, every)
	}
	docs := &f.wt.lists["/p/api"][2]
	docs.State, docs.DevURL = wt.Dirty, "http://localhost:3002"
	f.wt.lists["/p/api"][3].State = wt.Clean // rebase
	f.wt.listed = nil
	m, cmd := update(t, m, c.tick(t))
	if len(f.wt.listed) > 0 {
		t.Fatal("the tick listed worktrees before returning, want it in the background")
	}
	if every = append(every, every[0]); !slices.Equal(c.waits, every) {
		t.Errorf("after the tick, the sidebar asked for ticks in %v, want %v", c.waits, every)
	}
	m = feed(t, m, cmd)
	if want := []string{"/p/api", "/p/Zed"}; !slices.Equal(f.wt.listed, want) {
		t.Errorf("the tick had wt list %q, want %q", f.wt.listed, want)
	}
	if got := plain(m); !strings.Contains(got, "\n      docs ± :3002\n      api--rebase\n") {
		t.Errorf("after the tick, the view is:\n%s\nwant docs dirty, on port 3002, and api--rebase clean", got)
	}
	docs.State, docs.DevURL = wt.Clean, ""
	m, cmd = update(t, m, c.tick(t))
	m = feed(t, m, cmd)
	if got := plain(m); !strings.Contains(got, "\n      docs\n      api--rebase\n") {
		t.Errorf("after the next tick, the view is:\n%s\nwant docs clean, without a port", got)
	}
}

// split is the commands cmd, which must be there, stands for: its batch's,
// or itself. Splitting runs cmd if it's no batch.
func split(t *testing.T, cmd tea.Cmd) []tea.Cmd {
	t.Helper()
	msg := run(t, cmd)
	if batch, ok := msg.(tea.BatchMsg); ok {
		return batch
	}
	return []tea.Cmd{func() tea.Msg { return msg }}
}

// While a project's refresh runs, a tick starts none for it, and the other
// projects refresh as usual. Once it's done, the next tick refreshes it
// again.
func TestATickStartsNoRefreshWhileTheLastOneRuns(t *testing.T) {
	f := worktrees()
	m, c := ticking(t, f)
	m, cmd := update(t, m, c.tick(t))
	refreshes := split(t, cmd)
	if len(refreshes) != 2 {
		t.Fatalf("the tick gave %d commands, want 2: api's refresh and Zed's", len(refreshes))
	}
	m, _ = update(t, m, refreshes[0]()) // api's is done, Zed's runs on
	f.wt.listed = nil
	m, cmd = update(t, m, c.tick(t))
	m = feed(t, m, cmd)
	if want := []string{"/p/api"}; !slices.Equal(f.wt.listed, want) {
		t.Errorf("with Zed's refresh running, the tick had wt list %q, want %q", f.wt.listed, want)
	}
	m, _ = update(t, m, refreshes[1]()) // Zed's is done
	f.wt.listed = nil
	m, cmd = update(t, m, c.tick(t))
	feed(t, m, cmd)
	if want := []string{"/p/api", "/p/Zed"}; !slices.Equal(f.wt.listed, want) {
		t.Errorf("once Zed's refresh was done, the tick had wt list %q, want %q", f.wt.listed, want)
	}
}

// Once an action has finished (here, enter showing a session), every git
// project's worktrees are listed again, and the markers follow.
func TestMarkersRefreshAfterAnAction(t *testing.T) {
	f := worktrees()
	m, _ := ticking(t, f)
	m, _ = update(t, m, tea.FocusMsg{})
	m = keys(t, m, press("k"))               // Bugfix
	f.wt.lists["/p/api"][2].State = wt.Dirty // docs
	f.wt.listed = nil
	m, cmd := update(t, m, enter)
	m, after := update(t, m, run(t, cmd))
	m = feed(t, m, after)
	if want := []string{"/p/api", "/p/Zed"}; !slices.Equal(f.wt.listed, want) {
		t.Errorf("after enter, the sidebar had wt list %q, want %q", f.wt.listed, want)
	}
	if got := plain(m); !strings.Contains(got, "\n      docs ±\n") {
		t.Errorf("after enter, the view is:\n%s\nwant docs dirty", got)
	}
}

// e and o are actions too: once the editor or the browser has opened, every
// git project's worktrees are listed again. Moving focus (esc) is no action
// on worktrees, and lists nothing.
func TestMarkersRefreshAfterEAndO(t *testing.T) {
	f := worktrees()
	f.wt.lists["/p/api"][1].DevURL = "http://localhost:3001" // fix-login, login's
	m, _ := withOpeners(t, f)                                // on login
	for _, k := range []string{"e", "o"} {
		f.wt.listed = nil
		var cmd, after tea.Cmd
		m, cmd = update(t, m, press(k))
		if m, after = update(t, m, run(t, cmd)); after != nil {
			m = feed(t, m, after)
		}
		if want := []string{"/p/api", "/p/Zed"}; !slices.Equal(f.wt.listed, want) {
			t.Errorf("after %s, the sidebar had wt list %q, want %q", k, f.wt.listed, want)
		}
	}
	f.wt.listed = nil
	m, cmd := update(t, m, esc)
	if _, after := update(t, m, run(t, cmd)); after != nil || len(f.wt.listed) > 0 {
		t.Errorf("after esc, the sidebar gave %v and had wt list %q, want nothing", after, f.wt.listed)
	}
}

// When an action finishes while a project's refresh runs, that refresh may
// predate what the action did, so another follows it, once it's done. Here
// the action fails (wt can't open docs, but made it dirty), which counts
// too.
func TestARefreshFollowsTheOneRunningWhenAnActionFinishes(t *testing.T) {
	f := worktrees()
	f.wt.openErr = errors.New("wt: setup failed")
	m, c := ticking(t, f)
	m, _ = update(t, m, tea.FocusMsg{})
	m, cmd := update(t, m, c.tick(t))
	var running []tea.Msg // listed before the action, not yet taken in
	for _, r := range split(t, cmd) {
		running = append(running, r())
	}
	m = keys(t, m, press("j"), press("j"), press("j")) // docs
	f.wt.lists["/p/api"][2].State = wt.Dirty
	m, cmd = update(t, m, enter)
	m, after := update(t, m, run(t, cmd))
	if after != nil {
		t.Error("once the action finished, a refresh started while the last one ran")
	}
	if got := plain(m); strings.Contains(got, "docs ±") {
		t.Fatalf("before any refresh after the action, the view is:\n%s\nwant docs clean, as listed before", got)
	}
	f.wt.listed = nil
	for _, msg := range running {
		var next tea.Cmd
		m, next = update(t, m, msg)
		m = feed(t, m, next)
	}
	if want := []string{"/p/api", "/p/Zed"}; !slices.Equal(f.wt.listed, want) {
		t.Errorf("after the running refreshes, the sidebar had wt list %q, want %q", f.wt.listed, want)
	}
	if got := plain(m); !strings.Contains(got, "\n      docs ±\n") {
		t.Errorf("after the refreshes that followed, the view is:\n%s\nwant docs dirty", got)
	}
}

// r rebuilds everything, which lists every git project's worktrees again,
// and the markers follow.
func TestRRefreshesTheMarkers(t *testing.T) {
	f := worktrees()
	m, _ := ticking(t, f)
	f.wt.lists["/p/api"][2].DevURL = "http://localhost:3002" // docs
	f.wt.listed = nil
	m, cmd := update(t, m, press("r"))
	m = feed(t, m, cmd)
	if want := []string{"/p/api", "/p/Zed"}; !slices.Equal(f.wt.listed, want) {
		t.Errorf("r had wt list %q, want %q", f.wt.listed, want)
	}
	if got := plain(m); !strings.Contains(got, "\n      docs :3002\n") {
		t.Errorf("after r, the view is:\n%s\nwant docs on port 3002", got)
	}
}

// A listing that fails, a refresh's as a build's, lists the project as tmux
// sees it, and the status line says what wt said, once for every project it
// failed for. That lasts until the project's next listing works, whichever
// listed it.
func TestAFailedListingSaysWhyUntilTheNextOneWorks(t *testing.T) {
	f := worktrees()
	f.normal.snap.Sessions = append(f.normal.snap.Sessions[:3:3], f.normal.snap.Sessions[3])
	old := wtFailure{wt.ErrTooOld, "usage: wt [options] <branch>"}
	f.wt.fails = map[string]error{"/p/api": old}
	m, c := ticking(t, f)
	tmuxs := ` wtui
  ▾ api
    ▸ api
    ▾ login
        0 shell
        1 dev
  ▾ notes
    ▸ notes`
	worktrees := ` wtui
  ▾ api
    ▸ api
    ▾ login
        0 shell
        1 dev
      docs
      api--rebase ±
      api--gone ✗
  ▾ notes
    ▸ notes`
	zed := "\n  ▾ Zed\n    ▸ zed\n      agent-1"
	tooOld := "\nwt is too old for wtui (update tmux-kit): usage: wt [options] <branch>"
	if got, want := plain(m), tmuxs+zed+tooOld; got != want {
		t.Fatalf("with wt too old for api, the view is:\n%s\nwant:\n%s", got, want)
	}
	tick := func(what, want string) {
		t.Helper()
		var cmd tea.Cmd
		m, cmd = update(t, m, c.tick(t))
		if m = feed(t, m, cmd); plain(m) != want {
			t.Errorf("after a tick %s, the view is:\n%s\nwant:\n%s", what, plain(m), want)
		}
	}
	f.wt.fails = nil
	tick("that listed api", worktrees+zed)
	gone := wtFailure{wt.ErrNotFound, "wt not found on PATH"}
	f.wt.fails = map[string]error{"/p/api": gone, "/p/Zed": gone}
	tick("without wt", tmuxs+"\n  ▾ Zed\n    ▸ zed\nwt not found on PATH")
	f.wt.fails = nil
	if got, want := plain(rebuilt(t, m, f)), worktrees+zed; got != want {
		t.Errorf("after a build with wt back, the view is:\n%s\nwant:\n%s", got, want)
	}
}
