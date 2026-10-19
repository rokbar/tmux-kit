package sidebar

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
	"github.com/rokbar/tmux-kit/wtui/internal/tree"
)

// withAgents is the sidebar with the agent states now agents, and the rest
// of the tree as it was built. They're newer than the snapshot of a build
// running now.
func (m Model) withAgents(agents []tmux.Agent) Model {
	m.agentsNewer = m.agentsNewer || m.building
	src := m.src
	src.Snapshot.Agents = agents
	return m.withSources(src)
}

// newerAgents is src, a build's sources, with the agent states the sidebar
// has if they're newer than its snapshot's.
func (m Model) newerAgents(src tree.Sources) tree.Sources {
	if m.agentsNewer {
		src.Snapshot.Agents = m.src.Snapshot.Agents
	}
	return src
}

// cleared is tmux having cleared done in a window, or why it couldn't.
type cleared struct{ err error }

// clearShown is the sidebar once done is cleared in the window the right
// side shows, if an agent there is done: the tree shows it cleared at once,
// and tmux clears it in the background. That window is the current window
// of the session tmux last said the right side shows. Nothing clears while
// a build runs: one that just ran may be older than what tmux has said
// since (that another window is shown), and the one running follows it.
func (m Model) clearShown() (Model, tea.Cmd) {
	window := m.shownWindow()
	done := func(a tmux.Agent) bool { return a.Window == window && a.State == tmux.Done }
	if m.building || window == "" || !slices.ContainsFunc(m.src.Snapshot.Agents, done) {
		return m, nil
	}
	normal := m.c.Normal
	m = m.withAgents(slices.DeleteFunc(slices.Clone(m.src.Snapshot.Agents), done))
	return m, func() tea.Msg { return cleared{normal.ClearDone(window)} }
}

// shownWindow is the window the right side shows (an id), as far as the
// sidebar knows, or "" for none.
func (m Model) shownWindow() string {
	for _, s := range m.src.Snapshot.Sessions {
		for _, w := range s.Windows {
			if s.ID == m.right && w.Active {
				return w.ID
			}
		}
	}
	return ""
}

// agentColumn is how many columns the agent state takes at the right edge
// of each row: a space, then its mark.
const agentColumn = 2

// agentMarks are how the agent states show: needs you a red !, working a
// yellow *, done a dim green ✓.
var agentMarks = map[tmux.AgentState]struct {
	mark  string
	style []string
}{
	tmux.NeedsYou: {"!", []string{red}},
	tmux.Working:  {"*", []string{yellow}},
	tmux.Done:     {"✓", []string{dim, green}},
}

// agentMark is the agent column of a row whose agent state is state: a
// space, then the state's mark, if it has one. On the cursor's row, both
// are in reverse video too, as the rest of the row is.
func agentMark(state tmux.AgentState, cursor bool) string {
	a, ok := agentMarks[state]
	if !ok {
		a.mark = " "
	}
	var row []string // the row's style that carries on into the column
	if cursor {
		row = []string{reverse}
	}
	return styled(" ", row...) + styled(a.mark, append(row, a.style...)...)
}
