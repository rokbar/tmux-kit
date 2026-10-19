package sidebar

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
	"github.com/rokbar/tmux-kit/wtui/internal/tree"
)

// action is what a sidebar key does.
type action struct {
	// keys are the keys that do it, as Bubble Tea names them (j, down,
	// enter, space, ctrl+b, ...).
	keys []string
	// help says what it does, for lists of keys.
	help string
	do   func(Model) (Model, tea.Cmd)
}

// actions are every sidebar key, in the order a list of keys shows them.
// A key not here does nothing.
var actions = []action{
	{[]string{"j", "down"}, "down", Model.down},
	{[]string{"k", "up"}, "up", Model.up},
	{[]string{"l", "right"}, "unfold", Model.unfold},
	{[]string{"h", "left"}, "fold, or go to the parent", Model.fold},
	{[]string{"space"}, "fold or unfold", Model.toggle},
	{[]string{"enter"}, "show on the right, opening a worktree's session if it has none; fold or unfold a project or worktree", Model.enter},
	{[]string{"r"}, "rebuild the tree", Model.rebuildAll},
	{[]string{"z"}, "hide the sidebar", Model.hide},
	{[]string{"esc"}, "back to the right side", Model.goRight},
	{[]string{"ctrl+b"}, "tmux's prefix, on the right side", Model.prefixRight},
	{[]string{"q"}, "close wtui", Model.quit},
}

// keyed is actions by key.
var keyed = func() map[string]action {
	byKey := map[string]action{}
	for _, a := range actions {
		for _, k := range a.keys {
			byKey[k] = a
		}
	}
	return byKey
}()

func (m Model) down() (Model, tea.Cmd) { return m.move(1), nil }
func (m Model) up() (Model, tea.Cmd)   { return m.move(-1), nil }

// move is the sidebar with the cursor by rows down (up if negative), as far
// as the first or last row.
func (m Model) move(by int) Model {
	rows := m.rows()
	if len(rows) == 0 {
		return m
	}
	i := max(0, min(len(rows)-1, m.at(rows)+by))
	m.cursor = rows[i].ID
	return m
}

// at is where the cursor is in rows, 0 if it's on none of them.
func (m Model) at(rows []tree.Row) int {
	for i, r := range rows {
		if r.ID == m.cursor {
			return i
		}
	}
	return 0
}

// unfold unfolds the cursor's row, or if it's unfolded, goes to its first
// child.
func (m Model) unfold() (Model, tea.Cmd) {
	n := m.tree.Node(m.cursor)
	switch {
	case n == nil || len(n.Children) == 0:
	case !m.open(n):
		m.hand[n.ID] = true
	default:
		m.cursor = n.Children[0].ID
	}
	return m, nil
}

// fold folds the cursor's row, or if it's folded or has nothing to fold,
// goes to its parent.
func (m Model) fold() (Model, tea.Cmd) {
	n := m.tree.Node(m.cursor)
	switch {
	case n == nil:
	case len(n.Children) > 0 && m.open(n):
		m.hand[n.ID] = false
	case n.Parent != nil:
		m.cursor = n.Parent.ID
	}
	return m, nil
}

// toggle folds or unfolds the cursor's row.
func (m Model) toggle() (Model, tea.Cmd) {
	if n := m.tree.Node(m.cursor); n != nil && len(n.Children) > 0 {
		m.hand[n.ID] = !m.open(n)
	}
	return m, nil
}

// enter shows a session, or a window's session at that window, on the right
// and moves focus there; it folds or unfolds a project, or a worktree with
// sessions under it. A worktree without a session, it opens first. A
// missing worktree has nothing to show.
func (m Model) enter() (Model, tea.Cmd) {
	n := m.tree.Node(m.cursor)
	if n == nil {
		return m, nil
	}
	switch {
	case n.ID.Kind == tree.Project || n.ID.Kind == tree.Worktree && len(n.Children) > 0:
		return m.toggle()
	case missing(n):
		return m, nil
	case n.ID.Kind == tree.Worktree:
		return m, m.opening(n)
	}
	normal, host := m.c.Normal, m.c.Host
	session, window := n.Session, n.Window
	return m, func() tea.Msg {
		if err := normal.Show(session, window); err != nil {
			return shown{err: err}
		}
		return shown{session: session, err: host.FocusRight()}
	}
}

// opening has wt open the session of n, a worktree's row, then shows that
// session (the one started in the worktree's folder) on the right and moves
// focus there.
func (m Model) opening(n *tree.Node) tea.Cmd {
	w, normal, host := m.c.Wt, m.c.Normal, m.c.Host
	project, path := n.Parent.Path, n.Path
	return func() tea.Msg {
		if err := w.Open(project, path); err != nil {
			return shown{err: err}
		}
		snap, err := normal.Snapshot()
		if err != nil {
			return shown{err: err}
		}
		i := slices.IndexFunc(snap.Sessions, func(s tmux.Session) bool { return s.Path == path })
		if i < 0 {
			return shown{err: fmt.Errorf("wt opened %s, but no session started there", path)}
		}
		session := snap.Sessions[i].ID
		if err := normal.Show(session, ""); err != nil {
			return shown{err: err}
		}
		return shown{session: session, err: host.FocusRight()}
	}
}

// rebuildAll rebuilds the tree, looking every start folder up again.
func (m Model) rebuildAll() (Model, tea.Cmd) {
	m.fresh = true
	return m.rebuild()
}

func (m Model) goRight() (Model, tea.Cmd)     { return m, ask(m.c.Host.FocusRight) }
func (m Model) prefixRight() (Model, tea.Cmd) { return m, ask(m.c.Host.PrefixRight) }
func (m Model) hide() (Model, tea.Cmd)        { return m, ask(m.c.Host.Hide) }
func (m Model) quit() (Model, tea.Cmd)        { return m, tea.Quit }
