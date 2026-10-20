package sidebar

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/dialog"
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

// remove removes r's worktree: the remove dialog, which asks first, naming
// every session that ends with it and the agents in them. It runs in the
// main clone, which stays, unlike the worktree's folder.
func (m Model) remove(r removable) (Model, tea.Cmd) {
	removal := dialog.Removal{Path: r.worktree.Path, Missing: r.worktree.State == wt.Missing,
		Sessions: m.endingIn(r.worktree.Path)}
	return m.dialog(tmux.Popup{Title: " remove " + filepath.Base(removal.Path) + " ", Dir: r.project,
		Command: []string{"remove", removal.Arg()}})
}

// endingIn are the sessions that end when wt removes the worktree in
// folder, by name, as wt ends them: each one started in that folder or a
// folder inside it, whichever worktree's row it's on. With each, its agents
// in the order of their windows.
func (m Model) endingIn(folder string) []dialog.Ending {
	var endings []dialog.Ending
	for _, s := range m.src.Snapshot.Sessions {
		if s.Path != folder && !strings.HasPrefix(s.Path, folder+"/") {
			continue
		}
		e := dialog.Ending{Session: s.Name}
		for _, w := range s.Windows {
			for _, a := range m.src.Snapshot.Agents {
				if a.Session == s.ID && a.Window == w.ID {
					e.Agents = append(e.Agents, dialog.EndingAgent{Window: w.Name, State: stateNames[a.State]})
				}
			}
		}
		endings = append(endings, e)
	}
	slices.SortFunc(endings, func(a, b dialog.Ending) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.Session), strings.ToLower(b.Session)), cmp.Compare(a.Session, b.Session))
	})
	return endings
}

// stateNames are the agent states as the dialogs name them.
var stateNames = map[tmux.AgentState]string{tmux.Done: "done", tmux.Working: "working", tmux.NeedsYou: "needs you"}
