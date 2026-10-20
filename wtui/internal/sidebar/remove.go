package sidebar

import (
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
	"github.com/rokbar/tmux-kit/wtui/internal/tree"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// removable is a worktree x removes: as wt lists it, and its project's main
// clone, where wt runs.
type removable struct {
	project  string
	worktree *wt.Worktree
}

// removableOf is the worktree row n is in, for x, a missing one included:
// wt unregisters that. wt never removes the main clone, which a project's
// row stands for too, and a row in no worktree (a non-git project, a
// session in no worktree wt lists) has nothing to remove: both get a hint.
func removableOf(n *tree.Node) (removable, hint) {
	w := worktreeOf(n)
	switch {
	case w == nil:
		return removable{}, hint(sessionOf(n).Name + " isn't in a git repo")
	case w.MainClone:
		return removable{}, hint(filepath.Base(w.Path) + " is the main clone, not a worktree")
	}
	p := n
	for p.Parent != nil {
		p = p.Parent
	}
	return removable{p.Path, w}, ""
}

// remove removes r's worktree: the remove dialog, which asks first, then
// has wt remove it. It runs in the main clone, which stays, unlike the
// worktree's folder.
func (m Model) remove(r removable) (Model, tea.Cmd) {
	path := r.worktree.Path
	return m.dialog(tmux.Popup{Title: " remove " + filepath.Base(path) + " ", Dir: r.project,
		Command: []string{"remove", path, m.removeQuestion(r.worktree)}})
}

// removeQuestion is the remove dialog's question for w: whether to remove
// it, naming every session that ends with it; that only its registration
// goes, if it's missing; and the agents that end, by session and window.
// The sessions are those wt ends: each one started in w's folder or a
// folder inside it, whichever worktree's row it's on, in the tree's order.
func (m Model) removeQuestion(w *wt.Worktree) string {
	var ending []*tree.Node
	for _, s := range m.tree.Sessions() {
		if tree.Within(s.Path, w.Path) {
			ending = append(ending, s)
		}
	}
	name := filepath.Base(w.Path)
	var names []string
	for _, s := range ending {
		names = append(names, s.Name)
	}
	var lines []string
	switch len(ending) {
	case 0:
		lines = append(lines, "Remove worktree "+name+" (no session)?")
	case 1:
		lines = append(lines, "Remove worktree "+name+" and end session "+names[0]+"?")
	default:
		last := len(names) - 1
		lines = append(lines, "Remove worktree "+name+" and end sessions "+strings.Join(names[:last], ", ")+" and "+names[last]+"?")
	}
	if w.State == wt.Missing {
		lines = append(lines, "Its folder is gone, so removing it only unregisters it.")
	}
	for _, s := range ending {
		lines = append(lines, m.endingAgents(s.Name, s.Session, s.Children)...)
	}
	return strings.Join(lines, "\n")
}
