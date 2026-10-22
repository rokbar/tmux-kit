package sidebar

import (
	"slices"
	"testing"

	"github.com/rokbar/tmux-kit/wtui/internal/host"
)

// closed says whether done is closed.
func closed(done chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

// The host has the sidebar press some keys itself (see Keys): the key does
// what it does when typed, and Done closes once it has done all of it. For a
// jump, that's once the session is shown and focus is on the right, so the
// keys typed after it land there.
func TestAKeyTheHostPressesSaysWhenItsDone(t *testing.T) {
	f := fixture()
	m := focused(t, f)
	done := make(chan struct{})
	m, cmd := update(t, m, Press{Key: "1", Done: done})
	if closed(done) {
		t.Fatal("1 was done before it showed a session")
	}
	m, _ = update(t, m, run(t, cmd))
	if !closed(done) {
		t.Fatal("1 isn't done once it has shown session 1")
	}
	if want := []string{"$1"}; !slices.Equal(f.normal.shown, want) {
		t.Errorf("1 showed %q, want %q", f.normal.shown, want)
	}
	if want := []string{"focus right"}; !slices.Equal(f.host.asked, want) {
		t.Errorf("1 asked the host %q, want %q", f.host.asked, want)
	}
	if got := styles(m)["1   ▾ api"]; got != "1;36" {
		t.Errorf("once 1 is done, api is styled %q, want it the active session", got)
	}
}

// A key the host presses that has nothing more to do (here, enter on a
// project, which folds it) is done at once.
func TestAKeyTheHostPressesThatDoesNoMoreIsDoneAtOnce(t *testing.T) {
	m := keys(t, focused(t, fixture()), press("k"), press("k"), press("k")) // api
	done := make(chan struct{})
	m, cmd := update(t, m, Press{Key: "enter", Done: done})
	if cmd != nil || !closed(done) {
		t.Errorf("enter on a project gave %v, and done is closed: %v; want nothing more, done", cmd, closed(done))
	}
	if got := cursor(t, m); got != "  ▸ api" {
		t.Errorf("after enter on api, the cursor is on %q, want api, folded", got)
	}
}

// The host binds every sidebar key after the wtui key, by its name in tmux,
// so a key added to the sidebar is bound too. The keys after which focus can
// go right are held: the host has the sidebar press them, and holds the keys
// typed after them until they're done. ] and [ repeat.
func TestTheHostBindsEverySidebarKey(t *testing.T) {
	got := map[string]host.Key{}
	for _, k := range Keys() {
		got[k.Name] = k
	}
	for _, a := range actions {
		for _, k := range a.keys {
			if got[k].Tmux == "" {
				t.Errorf("sidebar key %q has no tmux name: %+v", k, got[k])
			}
		}
	}
	for _, want := range []host.Key{
		{Tmux: "j", Name: "j"},
		{Tmux: "Down", Name: "down"},
		{Tmux: "Space", Name: "space"},
		{Tmux: "q", Name: "q"},
		{Tmux: "r", Name: "r"},
		{Tmux: "Enter", Name: "enter", Held: true},
		{Tmux: "1", Name: "1", Held: true},
		{Tmux: "9", Name: "9", Held: true},
		{Tmux: "Tab", Name: "tab", Held: true},
		{Tmux: "]", Name: "]", Held: true, Repeats: true},
		{Tmux: "[", Name: "[", Held: true, Repeats: true},
		{Tmux: "Escape", Name: "esc", Held: true},
		{Tmux: "C-b", Name: "ctrl+b", Held: true},
		{Tmux: "z", Name: "z", Held: true},
	} {
		if got[want.Name] != want {
			t.Errorf("the host binds %q as %+v, want %+v", want.Name, got[want.Name], want)
		}
	}
}
