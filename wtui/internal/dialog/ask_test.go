package dialog_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/dialog"
)

var question = []string{
	"Kill session login and its 2 windows?",
	"The agent in it ends too:",
	"  1 dev: working",
}

// Asking shows the question, then the keys that answer it, each line with a
// column either side; a line too long for the popup wraps.
func TestAskShowsTheQuestionThenItsKeys(t *testing.T) {
	m, _ := update(t, dialog.Ask(question, "kill"), tea.WindowSizeMsg{Width: 30, Height: 12})
	want := ` Kill session login and its 2
 windows?
 The agent in it ends too:
   1 dev: working

 y: kill   n/q/esc: cancel`
	if got := view(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
}

// In a popup too short for the question, the keys still show, under as
// much of it as fits.
func TestInAShortPopupTheKeysStillShow(t *testing.T) {
	m, _ := update(t, dialog.Ask(question, "kill"), tea.WindowSizeMsg{Width: 60, Height: 4})
	want := ` Kill session login and its 2 windows?
 The agent in it ends too:

 y: kill   n/q/esc: cancel`
	if got := view(m); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
}

// answer is how the question was answered once key was pressed, and
// whether the dialog closed.
func answer(t *testing.T, key tea.KeyPressMsg) (yes, closed bool) {
	t.Helper()
	m, cmd := update(t, dialog.Ask(question, "kill"), key)
	if cmd != nil {
		_, closed = cmd().(tea.QuitMsg)
	}
	return dialog.Yes(m), closed
}

// y answers yes, and n, q, esc and ctrl+c no; either closes the dialog.
// Any other key does nothing, so that a key pressed by mistake neither
// answers nor closes it.
func TestYAnswersYes(t *testing.T) {
	if yes, closed := answer(t, tea.KeyPressMsg{Code: 'y', Text: "y"}); !yes || !closed {
		t.Errorf("after y, yes is %v and the dialog closed is %v, want both", yes, closed)
	}
	for _, k := range []tea.KeyPressMsg{{Code: 'n', Text: "n"}, {Code: 'q', Text: "q"}, {Code: tea.KeyEscape}, {Code: 'c', Mod: tea.ModCtrl}} {
		if yes, closed := answer(t, k); yes || !closed {
			t.Errorf("after %s, yes is %v and the dialog closed is %v, want no, closed", k, yes, closed)
		}
	}
	for _, k := range []tea.KeyPressMsg{{Code: 'Y', Text: "Y"}, {Code: tea.KeyEnter}, {Code: 'x', Text: "x"}} {
		if yes, closed := answer(t, k); yes || closed {
			t.Errorf("after %s, yes is %v and the dialog closed is %v, want neither", k, yes, closed)
		}
	}
}
