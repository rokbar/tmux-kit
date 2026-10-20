package sidebar

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
	"github.com/rokbar/tmux-kit/wtui/internal/tree"
)

// killable is row n, if d kills it: a session's row (one standing for its
// worktree too) or a window's. Any other row (a project, a worktree with
// several sessions under it, or with none) gets a hint.
func killable(n *tree.Node) (*tree.Node, hint) {
	if n.ID.Kind != tree.Session && n.ID.Kind != tree.Window {
		return nil, hint(n.Name + " isn't a session or a window")
	}
	return n, ""
}

// kill asks whether to kill n, a session's or a window's row, and kills it
// once the user says so: the kill dialog, given n as tmux targets it, by
// ids (session, or session:window), and the question. It talks to the
// normal server itself, by its socket.
func (m Model) kill(n *tree.Node) (Model, tea.Cmd) {
	target := n.Session
	if n.ID.Kind == tree.Window {
		target += ":" + n.Window
	}
	return m.dialog(tmux.Popup{Title: " kill ", Command: m.onNormal("kill", target, m.killQuestion(n))})
}

// killQuestion is the kill dialog's question for n, a session's or a
// window's row: what it kills; that the session ends too, if n is its last
// window; the agents that end with it, by window, most urgent first; and,
// if the session ends, that its worktree stays, if it's in one.
func (m Model) killQuestion(n *tree.Node) string {
	s := sessionOf(n)
	windows := s.Children
	var lines []string
	switch {
	case n.ID.Kind == tree.Window:
		lines = append(lines, fmt.Sprintf("Kill window %d %s of session %s?", n.Index, n.Name, s.Name))
		if len(windows) > 1 {
			windows = []*tree.Node{n}
			break
		}
		lines = append(lines, "It's the session's last window: session "+s.Name+" ends too.")
	case len(windows) == 1:
		lines = append(lines, "Kill session "+s.Name+" and its one window?")
	default:
		lines = append(lines, fmt.Sprintf("Kill session %s and its %d windows?", s.Name, len(windows)))
	}
	lines = append(lines, m.endingAgents("it", s.Session, windows)...)
	if w := worktreeOf(s); len(windows) == len(s.Children) && w != nil && !w.MainClone {
		lines = append(lines, "The worktree "+filepath.Base(w.Path)+" stays.")
	}
	return strings.Join(lines, "\n")
}
