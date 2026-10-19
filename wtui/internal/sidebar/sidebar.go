// Package sidebar is the sidebar's model: the Bubble Tea program on the left
// of a host. It shows the tree of projects, worktrees, sessions and windows,
// keeps the cursor and the folds by identity across rebuilds, and shows a
// session or window on the right when asked to, opening a worktree's session
// through wt first if it has none.
package sidebar

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
	"github.com/rokbar/tmux-kit/wtui/internal/tree"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// Host is what the sidebar asks of the host it runs in.
type Host interface {
	// FocusRight moves focus to the right side.
	FocusRight() error
	// PrefixRight moves focus to the right side and passes tmux's prefix on.
	PrefixRight() error
	// Hide hides the sidebar, giving the right side the whole window and
	// focus, until the sidebar gets focus again.
	Hide() error
}

// Normal is what the sidebar asks of the normal tmux server.
type Normal interface {
	// Snapshot is its sessions and their windows.
	Snapshot() (tmux.Snapshot, error)
	// ActiveSession is the session the right side shows (an id), "" for
	// none.
	ActiveSession() (string, error)
	// Show shows session (an id) on the right side, at window (an id)
	// unless that's "".
	Show(session, window string) error
}

// Repos finds which git repo a folder is in.
type Repos interface {
	// MainClone is the main clone of the repo dir is in, "" outside git.
	MainClone(dir string) (string, error)
}

// Wt is what the sidebar asks of wt.
type Wt interface {
	// List is the main clone, then every worktree, of the git project whose
	// main clone is mainClone, in git's order.
	List(mainClone string) ([]wt.Worktree, error)
	// Open opens the session of the main clone or worktree at path, in the
	// project whose main clone is mainClone, without showing it: one with
	// wt's default name and usual setup, unless there's one already.
	Open(mainClone, path string) error
}

// Clients are what the sidebar talks to.
type Clients struct {
	Host   Host
	Normal Normal
	Repos  Repos
	Wt     Wt
}

// Model is the sidebar.
type Model struct {
	c   Clients
	key string // the wtui key, as tmux names it

	// src is what the tree was last built from, so that part of it (e.g. a
	// session's windows) can change without a new snapshot.
	src  tree.Sources
	tree *tree.Tree
	// clones are the start folders' main clones looked up so far: one git
	// lookup per folder for the launch, until r.
	clones map[string]string
	// building is whether a build is running; again whether another was
	// asked for meanwhile, and fresh whether that one looks every folder up
	// again.
	building, again, fresh bool

	active string // the active session, by id
	// hand has the folds made by hand, by node: unfolded or not. They stick
	// for the launch; other nodes fold by default (see open).
	hand map[tree.ID]bool

	cursor tree.ID // the row the cursor is on
	// focused is whether the sidebar has focus, as focus events say. It
	// starts with focus, which the host gives it, and tmux says nothing until
	// focus moves.
	focused bool
	// seek is whether the cursor goes to the active session as soon as the
	// tree has it: at launch, and on getting focus.
	seek bool

	width, height int
	top           int // the first row the view shows
	status        status

	// refreshing has the git projects (by main clone) whose worktrees are
	// being listed again for their markers, true for those to be listed
	// once more after that (see refresh).
	refreshing map[string]bool
	// unlisted has why wt couldn't list git projects' worktrees (by main
	// clone), as their last listings, a build's or a refresh's, said.
	unlisted map[string]error
	// after is a command that gives msg once d has passed (a fake in
	// tests).
	after func(d time.Duration, msg tea.Msg) tea.Cmd
}

// New is the sidebar of a host, talking to clients, where key is the wtui
// key.
func New(c Clients, key string) Model {
	return Model{c: c, key: key, hand: map[tree.ID]bool{}, focused: true, seek: true,
		refreshing: map[string]bool{}, unlisted: map[string]error{}, after: after}
}

// Rebuild asks the sidebar to build its tree again, in the background, from
// a new snapshot.
type Rebuild struct{}

// Active says which session the right side shows (an id, "" for none), or
// why that couldn't be read.
type Active struct {
	Session string
	Err     error
}

// built is a build's result: the sources for the new tree, unless the
// snapshot failed, and what went wrong on the way (the tree is still built
// from what was found): why wt couldn't list a git project's worktrees, by
// main clone, in unlisted, and any other failure in err.
type built struct {
	src      *tree.Sources
	unlisted map[string]error
	err      error
}

// shown is the right side showing session, once asked to, or why it
// doesn't.
type shown struct {
	session string
	err     error
}

type hostDone struct{ err error }

func (m Model) Init() tea.Cmd {
	m, build := m.rebuild()
	return tea.Batch(m.readActive, build)
}

func (m Model) readActive() tea.Msg {
	id, err := m.c.Normal.ActiveSession()
	return Active{id, err}
}

// ask runs a host request in the background.
func ask(f func() error) tea.Cmd { return func() tea.Msg { return hostDone{f()} } }

// rebuild starts a build, unless one is running, in which case another
// follows it.
func (m Model) rebuild() (Model, tea.Cmd) {
	if m.building {
		m.again = true
		return m, nil
	}
	m.building = true
	known := m.clones
	if m.fresh {
		known, m.fresh = nil, false
	}
	normal, repos, w := m.c.Normal, m.c.Repos, m.c.Wt
	return m, func() tea.Msg { return build(normal, repos, w, known) }
}

// build takes a snapshot, looks up the start folders not in known, and has
// wt list each git project's worktrees.
func build(normal Normal, repos Repos, w Wt, known map[string]string) built {
	snap, err := normal.Snapshot()
	if err != nil {
		return built{err: err} // the last tree stays
	}
	clones := maps.Clone(known)
	if clones == nil {
		clones = map[string]string{}
	}
	var errs []error
	for _, s := range snap.Sessions {
		if _, ok := clones[s.Path]; ok {
			continue
		}
		main, err := repos.MainClone(s.Path)
		if err != nil {
			errs = append(errs, err) // looked up again next time
			continue
		}
		clones[s.Path] = main
	}
	lists := map[string][]wt.Worktree{}
	unlisted := map[string]error{}
	for _, s := range snap.Sessions {
		main := clones[s.Path]
		if _, ok := lists[main]; ok || main == "" {
			continue
		}
		rows, err := w.List(main)
		if err != nil {
			unlisted[main] = err // the project shows as tmux sees it
		}
		lists[main] = rows
	}
	src := &tree.Sources{Snapshot: snap, MainClones: clones, Worktrees: lists}
	return built{src, unlisted, errors.Join(distinct(errs)...)}
}

// listFailure is what the status line says when wt can't list a project's
// worktrees.
func listFailure(err error) error {
	if errors.Is(err, wt.ErrTooOld) {
		return fmt.Errorf("wt is too old for wtui (update tmux-kit): %w", err)
	}
	return err
}

// distinct is errs without those that say what one before them says (e.g.
// that there's no wt, once for each git project).
func distinct(errs []error) []error {
	var out []error
	said := map[string]bool{}
	for _, err := range errs {
		if !said[err.Error()] {
			said[err.Error()] = true
			out = append(out, err)
		}
	}
	return out
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := m.update(msg)
	return m.scrolled(), cmd
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.FocusMsg:
		m.focused, m.seek = true, true
		return m.toActive(), nil
	case tea.BlurMsg:
		m.focused = false
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			m.seek = false
			return m.clickOn(msg.Y), nil
		}
	case tea.KeyPressMsg:
		m.seek = false // the cursor is the user's now
		if a, ok := keyed[msg.String()]; ok {
			return a.do(m)
		}
	case Rebuild, tmux.SessionsChanged:
		return m.rebuild()
	case tmux.WindowsChanged:
		return m.withWindows(msg.Session, msg.Windows), nil
	case built:
		m.building = false
		m.status.build = errText(msg.err)
		var tick tea.Cmd
		if msg.src != nil {
			if m.tree == nil {
				tick = m.tick() // the first tree: its markers refresh from now on
			}
			m.unlisted = msg.unlisted
			m = m.withSources(*msg.src).toActive().sayUnlisted()
		}
		if m.again {
			m.again = false
			var build tea.Cmd
			m, build = m.rebuild()
			return m, tea.Batch(tick, build)
		}
		return m, tick
	case refreshDue:
		m, refresh := m.refreshAll(false)
		return m, tea.Batch(refresh, m.tick())
	case refreshed:
		return m.withRefreshed(msg)
	case Active:
		m.status.active = errText(msg.Err)
		if msg.Err == nil {
			m.active = msg.Session
			m = m.inSight().toActive()
		}
	case tmux.ActiveSessionChanged:
		return m.update(Active{Session: msg.Session})
	case shown:
		m.status.action = errText(msg.err)
		if msg.session != "" {
			// at once, before tmux says so (ActiveSessionChanged); a window
			// shown becomes its session's current one when tmux says that
			// (WindowsChanged)
			m.active = msg.session
			m = m.inSight()
		}
		return m.afterAction()
	case hostDone:
		m.status.action = errText(msg.err)
	}
	return m, nil
}

// withSources is the sidebar with its tree built from src, the cursor on the
// same row as before, or the nearest one if that's gone.
func (m Model) withSources(src tree.Sources) Model {
	old := m.tree
	m.src, m.clones = src, src.MainClones
	m.tree = tree.Build(src)
	m.cursor = m.tree.Nearest(old, m.cursor)
	return m.inSight()
}

// withWindows is the sidebar with session's windows now windows, and the
// rest of the tree as it was built. A session it doesn't have yet is in the
// build its creation asked for. A build running now may have taken its
// snapshot before the windows changed, so another follows it.
func (m Model) withWindows(session string, windows []tmux.Window) Model {
	if m.building {
		m.again = true
	}
	sessions := slices.Clone(m.src.Snapshot.Sessions)
	i := slices.IndexFunc(sessions, func(s tmux.Session) bool { return s.ID == session })
	if i < 0 {
		return m
	}
	sessions[i].Windows = windows
	src := m.src
	src.Snapshot.Sessions = sessions
	return m.withSources(src)
}

// inSight is the sidebar with the cursor on a row that shows: if its row is
// folded away, the row it's folded into.
func (m Model) inSight() Model {
	var path []*tree.Node // from the top down to the cursor's row
	for n := m.tree.Node(m.cursor); n != nil; n = n.Parent {
		path = append([]*tree.Node{n}, path...)
	}
	for _, n := range path {
		if !m.open(n) {
			m.cursor = n.ID
			break
		}
	}
	return m
}

// clickOn is the sidebar with the cursor on the row the view shows on line
// y, if any.
func (m Model) clickOn(y int) Model {
	rows, room := m.rows(), m.room()
	i := m.top + y - 1 // rows start under the header
	if y >= 1 && (room < 0 || y <= room) && i < len(rows) {
		m.cursor = rows[i].ID
	}
	return m
}

// scrolled is the sidebar scrolled as little as keeps the cursor's row in
// sight.
func (m Model) scrolled() Model {
	rows, room := m.rows(), m.room()
	if room <= 0 {
		m.top = 0
		return m
	}
	at := m.at(rows)
	m.top = max(0, min(m.top, at, len(rows)-room))
	if at >= m.top+room {
		m.top = at - room + 1
	}
	return m
}

// toActive is the sidebar with the cursor on the active session, its
// project unfolded if it was folded, if it's seeking it and the tree has it.
func (m Model) toActive() Model {
	n := m.tree.Node(tree.ID{Kind: tree.Session, Key: m.active})
	if !m.seek || n == nil {
		return m
	}
	m.seek = false
	for p := n.Parent; p != nil; p = p.Parent {
		if !m.open(p) {
			m.hand[p.ID] = true
		}
	}
	m.cursor = n.ID
	return m
}

// open says whether node n shows its children: as folded or unfolded by
// hand, else by default, projects and worktrees unfolded and sessions
// folded, but for the active session.
func (m Model) open(n *tree.Node) bool {
	if o, ok := m.hand[n.ID]; ok {
		return o
	}
	switch n.ID.Kind {
	case tree.Project, tree.Worktree:
		return true
	case tree.Session:
		return n.Session == m.active
	}
	return false
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
