package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPlaceholderScreenNamesItself(t *testing.T) {
	if got := newModel().View().Content; !strings.Contains(got, "wtui") {
		t.Errorf("view = %q, want it to name wtui", got)
	}
}

func TestQuitKeys(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		{Code: 'q', Text: "q"},
		{Code: tea.KeyEscape},
		{Code: 'c', Mod: tea.ModCtrl},
	} {
		_, cmd := newModel().Update(key)
		if cmd == nil {
			t.Errorf("%s: no command, want quit", key)
			continue
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%s: command sent %T, want tea.QuitMsg", key, cmd())
		}
	}
}

func TestOtherKeysDoNotQuit(t *testing.T) {
	if _, cmd := newModel().Update(tea.KeyPressMsg{Code: 'x', Text: "x"}); cmd != nil {
		t.Errorf("x: got command sending %T, want none", cmd())
	}
}
