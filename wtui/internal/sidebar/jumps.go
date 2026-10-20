package sidebar

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
	"github.com/rokbar/tmux-kit/wtui/internal/tree"
)

// The jumps: keys that show another session on the right, by its number
// (1–9), the last one shown (tab), or the next or previous one in the active
// session's project (] and [). Every jump ends with focus on the right, even
// one that has nowhere to go (it shows a hint), so the keys typed after it
// land in a session, never in the sidebar.

// number shows the session with the number pressed.
func (m Model) number() (Model, tea.Cmd) {
	n := int(m.pressed[0] - '0')
	for _, s := range m.tree.Sessions() {
		if s.Number == n {
			return m, m.show(s.Session, "")
		}
	}
	return m, m.goNowhere(hint(fmt.Sprintf("no session %d", n)))
}

// last shows the last session: the one shown before, else the most recent
// other one.
func (m Model) last() (Model, tea.Cmd) {
	normal, host := m.c.Normal, m.c.Host
	return m, func() tea.Msg {
		session, err := normal.ShowLast()
		if errors.Is(err, tmux.ErrNoOtherSession) {
			return nowhere{hint("no other session"), host.FocusRight()}
		}
		return onRight(host, session, "", err)
	}
}

func (m Model) next() (Model, tea.Cmd)     { return m.sibling(1) }
func (m Model) previous() (Model, tea.Cmd) { return m.sibling(-1) }

// sibling shows the session by places after the active one in its project
// (before it, for a negative by), going round.
func (m Model) sibling(by int) (Model, tea.Cmd) {
	if s := m.tree.Next(m.active, by); s != nil {
		return m, m.show(s.Session, "")
	}
	project := "this project"
	for p := m.tree.Node(tree.ID{Kind: tree.Session, Key: m.active}); p != nil; p = p.Parent {
		project = p.Name
	}
	return m, m.goNowhere(hint("no other session in " + project))
}

// show shows session (an id) on the right, at its window window (an id)
// unless that's "", and moves focus there. If it can't be shown, focus
// stays, and the status line says why.
func (m Model) show(session, window string) tea.Cmd {
	normal, host := m.c.Normal, m.c.Host
	return func() tea.Msg { return onRight(host, session, "", normal.Show(session, window)) }
}

// onRight is the right side showing session once it was asked to, which
// failed with err if it did: focus moves there too, unless it failed. said
// is what wt said, if it opened the session first.
func onRight(host Host, session, said string, err error) shown {
	if err != nil {
		return shown{said: said, err: err}
	}
	return shown{session: session, said: said, err: host.FocusRight()}
}

// nowhere is a jump with nowhere to go, once focus has moved right all the
// same: hint says why it went nowhere, and err why focus didn't move, if it
// didn't.
type nowhere struct {
	hint hint
	err  error
}

// goNowhere is a jump with nowhere to go: hint h says why, and focus moves
// right all the same.
func (m Model) goNowhere(h hint) tea.Cmd {
	host := m.c.Host
	return func() tea.Msg { return nowhere{h, host.FocusRight()} }
}
