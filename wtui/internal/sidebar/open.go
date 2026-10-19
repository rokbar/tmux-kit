package sidebar

import (
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/tree"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// Opener opens something in a window of its own: the editor on a folder, or
// the browser on a URL. It returns once that has started, or failed to.
type Opener interface {
	Open(string) error
}

// worktreeOf is the main clone or worktree that row n is in, as wt lists it:
// a git project's row is its main clone's. Nil for a row in none (e.g. in a
// non-git project).
func worktreeOf(n *tree.Node) *wt.Worktree {
	if n.ID.Kind == tree.Project {
		for _, c := range n.Children {
			if c.Worktree != nil && c.Worktree.Path == n.Path {
				return c.Worktree
			}
		}
		return nil
	}
	for ; n.ID.Kind != tree.Project; n = n.Parent {
		if n.Worktree != nil {
			return n.Worktree
		}
	}
	return nil
}

// folderOf is the folder of row n: that of the worktree it's in, else its
// own (a project's, or a session's start folder; a window's session's). In a
// missing worktree, there's none.
func folderOf(n *tree.Node) (string, hint) {
	w := worktreeOf(n)
	switch {
	case w == nil:
		return sessionOf(n).Path, ""
	case w.State == wt.Missing:
		return "", isMissing(w)
	}
	return w.Path, ""
}

// devURLOf is the dev URL of the worktree row n is in, if it has one. A row
// in none (e.g. in a non-git project) has none.
func devURLOf(n *tree.Node) (string, hint) {
	w := worktreeOf(n)
	switch {
	case w == nil:
		return "", hint(sessionOf(n).Name + " has no dev URL")
	case w.State == wt.Missing:
		return "", isMissing(w)
	case w.DevURL == "":
		return "", hint(filepath.Base(w.Path) + " has no dev URL")
	}
	return w.DevURL, ""
}

// sessionOf is a window's session, else n.
func sessionOf(n *tree.Node) *tree.Node {
	if n.ID.Kind == tree.Window {
		return n.Parent
	}
	return n
}

// isMissing is the hint for a key that needs worktree w, which is missing.
func isMissing(w *wt.Worktree) hint { return hint(filepath.Base(w.Path) + " is missing") }

// edit opens the editor on folder.
func (m Model) edit(folder string) (Model, tea.Cmd) {
	editor := m.c.Editor
	return m, act(func() error { return editor.Open(folder) })
}

// browse opens url in the browser.
func (m Model) browse(url string) (Model, tea.Cmd) {
	browser := m.c.Browser
	return m, act(func() error { return browser.Open(url) })
}
