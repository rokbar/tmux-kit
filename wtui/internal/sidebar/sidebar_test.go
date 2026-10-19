package sidebar

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// fakeHost records what the sidebar asked of its host.
type fakeHost struct{ asked []string }

func (h *fakeHost) FocusRight() error  { h.asked = append(h.asked, "focus right"); return nil }
func (h *fakeHost) PrefixRight() error { h.asked = append(h.asked, "prefix right"); return nil }

// fakeNormal is a normal server whose nested client shows active, or fails
// with err.
type fakeNormal struct {
	active string
	err    error
}

func (n *fakeNormal) ActiveSession() (string, error) { return n.active, n.err }

func update(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

// run runs cmd, which must be there, and returns its message.
func run(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command, want one")
	}
	return cmd()
}

// started is the sidebar once it has read the active session.
func started(t *testing.T, h *fakeHost, n *fakeNormal) Model {
	t.Helper()
	m := New(h, n, `C-\`)
	m, _ = update(t, m, run(t, m.Init()))
	return m
}

func press(text string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: []rune(text)[0], Text: text} }

var (
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	ctrlB = tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl}
)

func TestNamesTheActiveSessionAndTheKeysThatWork(t *testing.T) {
	m := started(t, &fakeHost{}, &fakeNormal{active: "beta"})
	want := `wtui

on the right: beta

C-\, esc  to the right
C-b       tmux's prefix there
q         close wtui`
	if got := m.View().Content; got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
}

// The right side can change session by itself (prefix s there), so the
// sidebar asks again.
func TestFollowsTheRightSide(t *testing.T) {
	n := &fakeNormal{active: "beta"}
	m := New(&fakeHost{}, n, `C-\`)
	m, again := update(t, m, run(t, m.Init()))
	n.active = "gamma"
	if again == nil {
		t.Fatal("the sidebar doesn't ask again which session is active")
	}
	m, ask := update(t, m, pollDue{}) // the tick, without waiting for it
	m, _ = update(t, m, run(t, ask))
	if got := m.View().Content; !strings.Contains(got, "on the right: gamma") {
		t.Errorf("after the right side moved to gamma, the view is:\n%s", got)
	}
}

func TestSaysWhenNoSessionIsShown(t *testing.T) {
	m := started(t, &fakeHost{}, &fakeNormal{})
	if got := m.View().Content; !strings.Contains(got, "on the right: no session") {
		t.Errorf("with no session on the right, the view is:\n%s", got)
	}
}

func TestQClosesWtui(t *testing.T) {
	m := started(t, &fakeHost{}, &fakeNormal{active: "beta"})
	_, cmd := update(t, m, press("q"))
	if _, ok := run(t, cmd).(tea.QuitMsg); !ok {
		t.Error("q doesn't quit")
	}
}

func TestEscGoesRight(t *testing.T) {
	h := &fakeHost{}
	m := started(t, h, &fakeNormal{active: "beta"})
	_, cmd := update(t, m, esc)
	run(t, cmd)
	if want := []string{"focus right"}; !slices.Equal(h.asked, want) {
		t.Errorf("esc asked the host %q, want %q", h.asked, want)
	}
}

// C-b goes right and passes the prefix on, so C-b c makes a window there.
func TestTmuxPrefixGoesRightAndOn(t *testing.T) {
	h := &fakeHost{}
	m := started(t, h, &fakeNormal{active: "beta"})
	_, cmd := update(t, m, ctrlB)
	run(t, cmd)
	if want := []string{"prefix right"}; !slices.Equal(h.asked, want) {
		t.Errorf("C-b asked the host %q, want %q", h.asked, want)
	}
}

func TestOtherKeysDoNothing(t *testing.T) {
	h := &fakeHost{}
	m := started(t, h, &fakeNormal{active: "beta"})
	for _, k := range []string{"j", "x", "enter"} {
		if _, cmd := update(t, m, press(k)); cmd != nil {
			t.Errorf("%s does something: %v", k, cmd())
		}
	}
}

func TestFailuresShowInTheStatusLine(t *testing.T) {
	m := started(t, &fakeHost{}, &fakeNormal{err: errors.New("no server running")})
	if got := m.View().Content; !strings.HasSuffix(got, "\n\nno server running") {
		t.Errorf("view:\n%s\nwant it to end with the failure", got)
	}
}

// The sidebar is narrow: lines that don't fit are cut short.
func TestLinesFitTheSidebar(t *testing.T) {
	m := started(t, &fakeHost{}, &fakeNormal{active: "a-session-with-a-long-name"})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 20, Height: 30})
	for _, line := range strings.Split(m.View().Content, "\n") {
		if len([]rune(line)) > 20 {
			t.Errorf("line %q is wider than the sidebar's 20 columns", line)
		}
	}
	if !strings.Contains(m.View().Content, "on the right: a-ses…") {
		t.Errorf("the session's name isn't cut short:\n%s", m.View().Content)
	}
}
