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
// A key not here does nothing. A key that acts on something of the cursor's
// row (its folder, its dev URL) finds it with on, which shows a hint for a
// row that has none.
var actions = []action{
	{[]string{"j", "down"}, "down", Model.down},
	{[]string{"k", "up"}, "up", Model.up},
	{[]string{"l", "right"}, "unfold", Model.unfold},
	{[]string{"h", "left"}, "fold, or go to the parent", Model.fold},
	{[]string{"space"}, "fold or unfold", Model.toggle},
	{[]string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}, "show the session with that number on the right", Model.number},
	{[]string{"tab"}, "show the last session on the right", Model.last},
	{[]string{"]"}, "show the next session in the active session's project", Model.next},
	{[]string{"["}, "show the previous session in the active session's project", Model.previous},
	{[]string{"enter"}, "show on the right, opening a worktree's session if it has none; fold or unfold a project or worktree", Model.enter},
	{[]string{"e"}, "the editor on the worktree or folder", on(folderOf, Model.edit)},
	{[]string{"o"}, "the dev URL in the browser", on(devURLOf, Model.browse)},
	{[]string{"g"}, "the git UI in the worktree, over the right side", on(gitFolderOf, Model.gitUI)},
	{[]string{"n"}, "the picker, from the worktree: open a branch, or make one", on(gitWorktreeOf, Model.pick)},
	{[]string{"i"}, "details of the worktree: branch, changes, commits, upstream, dev URL", on(gitWorktreeOf, Model.details)},
	{[]string{"x"}, "remove the worktree and end its sessions, after asking", on(removableOf, Model.remove)},
	{[]string{"d"}, "kill the session or window, after asking (its worktree stays)", on(killable, Model.kill)},
	{[]string{"r"}, "rebuild the tree", Model.rebuildAll},
	{[]string{"p"}, "open a project from the projects root, over the right side", Model.openProject},
	{[]string{"z"}, "hide the sidebar", Model.hide},
	{[]string{"esc"}, "back to the right side", Model.goRight},
	{[]string{"ctrl+b"}, "tmux's prefix, on the right side", Model.prefixRight},
	{[]string{"?"}, "every shortcut", Model.shortcuts},
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

// press is the sidebar once key (as Bubble Tea names it) is pressed, and
// what the key does. Any key clears the last key's hint.
func (m Model) press(key string) (Model, tea.Cmd) {
	m.seek = false // the cursor is the user's now
	m.status.hint = ""
	a, ok := keyed[key]
	if !ok {
		return m, nil
	}
	m.pressed = key
	return a.do(m)
}

// A hint says why a key doesn't apply to the cursor's row, e.g. "notes has
// no dev URL". The status line shows it until the next key, and the key does
// nothing else.
type hint string

// on is an action on what find finds for the cursor's row: its folder, its
// dev URL, ... For a row find has a hint for instead, the key shows that.
func on[T any](find func(*tree.Node) (T, hint), do func(Model, T) (Model, tea.Cmd)) func(Model) (Model, tea.Cmd) {
	return func(m Model) (Model, tea.Cmd) {
		n := m.tree.Node(m.cursor)
		if n == nil {
			return m, nil
		}
		target, h := find(n)
		if h != "" {
			return m.hinted(h)
		}
		return do(m, target)
	}
}

// hinted is the sidebar showing hint h, for a key that doesn't apply to the
// cursor's row.
func (m Model) hinted(h hint) (Model, tea.Cmd) {
	m.status.hint = string(h)
	return m, nil
}

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
// missing worktree has nothing to show, so it gets a hint.
func (m Model) enter() (Model, tea.Cmd) {
	n := m.tree.Node(m.cursor)
	if n == nil {
		return m, nil
	}
	switch {
	case n.ID.Kind == tree.Project || n.ID.Kind == tree.Worktree && len(n.Children) > 0:
		return m.toggle()
	case missing(n):
		return m.hinted(isMissing(n.Worktree))
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
// focus there. The status line says what wt said it did.
func (m Model) opening(n *tree.Node) tea.Cmd {
	w, normal, host := m.c.Wt, m.c.Normal, m.c.Host
	project, path := n.Parent.Path, n.Path
	return func() tea.Msg {
		said, err := w.Open(project, path)
		if err != nil {
			return shown{err: err}
		}
		snap, err := normal.Snapshot()
		if err != nil {
			return shown{said: said, err: err}
		}
		i := slices.IndexFunc(snap.Sessions, func(s tmux.Session) bool { return s.Path == path })
		if i < 0 {
			return shown{said: said, err: fmt.Errorf("wt opened %s, but no session started there", path)}
		}
		session := snap.Sessions[i].ID
		if err := normal.Show(session, ""); err != nil {
			return shown{said: said, err: err}
		}
		return shown{session: session, said: said, err: host.FocusRight()}
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
