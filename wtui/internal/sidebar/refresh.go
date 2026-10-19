package sidebar

import (
	"errors"
	"maps"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// refreshEvery is how often the markers (± and the dev URL's port) refresh,
// once the tree is first built: each git project's worktrees are listed
// again.
const refreshEvery = 10 * time.Second

// refreshDue is the tick: time to refresh the markers.
type refreshDue struct{}

// refreshed is a git project's worktrees, listed again: what wt listed, or
// why it couldn't.
type refreshed struct {
	project string // its main clone
	rows    []wt.Worktree
	err     error
}

// tick is the next tick, refreshEvery from now.
func (m Model) tick() tea.Cmd { return m.after(refreshEvery, refreshDue{}) }

// after is a command that gives msg once d has passed.
func after(d time.Duration, msg tea.Msg) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return msg })
}

// afterAction is the sidebar once an action it ran has finished, whatever
// came of it: every git project the tree shows has its worktrees listed
// again, so that the markers catch up with what the action did (e.g. a
// commit made in the git UI clears ±). A listing running for a project may
// predate the action, so another follows it.
//
// Every action's result goes through here: an action's message handler
// returns m.afterAction(), batched with its own commands, if any.
func (m Model) afterAction() (Model, tea.Cmd) { return m.refreshAll(true) }

// refreshAll refreshes every git project the tree shows (see refresh).
func (m Model) refreshAll(follow bool) (Model, tea.Cmd) {
	var cmds []tea.Cmd
	for _, p := range m.gitProjects() {
		var cmd tea.Cmd
		m, cmd = m.refresh(p, follow)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

// refresh has wt list the worktrees of the git project whose main clone is
// project again, in the background. If a listing of it is running already,
// none starts; if follow, another follows that one, once.
func (m Model) refresh(project string, follow bool) (Model, tea.Cmd) {
	if _, running := m.refreshing[project]; running {
		m.refreshing[project] = m.refreshing[project] || follow
		return m, nil
	}
	m.refreshing[project] = false
	w := m.c.Wt
	return m, func() tea.Msg {
		rows, err := w.List(project)
		return refreshed{project, rows, err}
	}
}

// withRefreshed is the sidebar with a project's worktrees as listed again,
// and the listing that was to follow, started. As after a build, a project
// wt fails to list shows as tmux sees it, and the status line says why.
func (m Model) withRefreshed(r refreshed) (Model, tea.Cmd) {
	again := m.refreshing[r.project]
	delete(m.refreshing, r.project)
	src := m.src
	src.Worktrees = maps.Clone(src.Worktrees)
	src.Worktrees[r.project] = r.rows
	if r.err != nil {
		m.unlisted[r.project] = r.err
	} else {
		delete(m.unlisted, r.project)
	}
	m = m.withSources(src).sayUnlisted()
	if again {
		return m.refresh(r.project, false)
	}
	return m, nil
}

// sayUnlisted is the sidebar with the status line saying why wt couldn't
// list the worktrees of the git projects the tree shows, each different
// failure once.
func (m Model) sayUnlisted() Model {
	var errs []error
	for _, p := range m.gitProjects() {
		if err := m.unlisted[p]; err != nil {
			errs = append(errs, listFailure(err))
		}
	}
	m.status.lists = errText(errors.Join(distinct(errs)...))
	return m
}

// gitProjects are the git projects the tree shows, by main clone, in its
// order.
func (m Model) gitProjects() []string {
	if m.tree == nil {
		return nil
	}
	var projects []string
	for _, p := range m.tree.Projects {
		if _, ok := m.src.Worktrees[p.Path]; ok {
			projects = append(projects, p.Path)
		}
	}
	return projects
}
