// Package tree turns what wtui knows about the normal server (its sessions,
// windows and agent states, which git repo each session's start folder is
// in, and each repo's worktrees) into the sidebar's tree: projects, their
// worktrees and sessions, and those sessions' windows, in order, each with
// the most urgent agent state beneath it. It does no I/O, so it's the same
// tree for the same sources.
package tree

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// Sources are what the tree is built from.
type Sources struct {
	Snapshot tmux.Snapshot
	// MainClones has each start folder's main clone, "" for a folder in no
	// git repo. A folder it doesn't have (its lookup failed) counts as one in
	// no repo.
	MainClones map[string]string
	// Worktrees has each git project's main clone and worktrees, as wt lists
	// them, by main clone. A project it has none for (wt failed) is listed
	// as tmux sees it: its sessions, by name.
	Worktrees map[string][]wt.Worktree
}

// Kind is what a node stands for.
type Kind int

const (
	Project Kind = iota
	Worktree
	Session
	Window
)

// ID is a node's identity, which stays the same across builds for as long
// as the thing it stands for exists: a project's folder, a worktree's (or
// main clone's) folder, a session's id (not its name), and a window's id
// within its session.
type ID struct {
	Kind Kind
	Key  string
}

// Node is a row of the tree.
type Node struct {
	ID   ID
	Name string // what the row shows: a folder's, branch's, session's or window's name
	// Path is a project's folder, a worktree's, or a session's start folder.
	Path string
	// Worktree is the main clone or worktree the row stands for, as wt lists
	// it: a worktree's row, or the row of a worktree's one session. Nil for
	// any other row.
	Worktree *wt.Worktree
	// Session is a session's or window's session (an id); Window a window's
	// id, and Index its index.
	Session, Window string
	Index           int
	// Active is whether a window is its session's current window.
	Active bool
	// Agent is the most urgent agent state beneath the row: a window's
	// panes', and any other row's, the rows' under it.
	Agent    tmux.AgentState
	Parent   *Node
	Children []*Node
	// Number is a session's number, 1 to 9 for the first nine sessions in
	// the tree's order, 0 for the rest (and for every other row).
	Number int
}

// Tree is the sidebar's tree. It doesn't change once built.
type Tree struct {
	Projects []*Node
	byID     map[ID]*Node
}

// Build builds the tree. A session belongs to the project of its start
// folder: the main clone if the folder is in a git repo, otherwise the
// folder itself. Projects are labelled by folder name and alphabetical. A
// git project's rows are its main clone and worktrees (see worktrees); in
// any other project, sessions go by name. Windows go by index. The first
// nine sessions in that order are numbered.
func Build(src Sources) *Tree {
	t := &Tree{byID: map[ID]*Node{}}
	projects := map[string]*Node{}
	for _, s := range src.Snapshot.Sessions {
		folder := src.MainClones[s.Path]
		if folder == "" {
			folder = s.Path
		}
		p := projects[folder]
		if p == nil {
			p = &Node{ID: ID{Project, folder}, Name: filepath.Base(folder), Path: folder}
			projects[folder] = p
			t.Projects = append(t.Projects, p)
		}
		sn := &Node{ID: ID{Session, s.ID}, Name: s.Name, Path: s.Path, Session: s.ID, Parent: p}
		for _, w := range s.Windows {
			sn.Children = append(sn.Children, &Node{
				ID: ID{Window, s.ID + ":" + w.ID}, Name: w.Name, Session: s.ID, Window: w.ID,
				Index: w.Index, Active: w.Active, Parent: sn,
			})
		}
		slices.SortStableFunc(sn.Children, func(a, b *Node) int { return cmp.Compare(a.Index, b.Index) })
		p.Children = append(p.Children, sn)
	}
	slices.SortFunc(t.Projects, byName)
	for _, p := range t.Projects {
		slices.SortFunc(p.Children, byName)
		if rows := src.Worktrees[p.Path]; len(rows) > 0 {
			p.Children = t.worktrees(p, rows)
		}
	}
	agents := map[string]tmux.AgentState{} // by window id: its panes' most urgent
	for _, a := range src.Snapshot.Agents {
		agents[a.Window] = max(agents[a.Window], a.State)
	}
	for _, p := range t.Projects {
		rollUp(p, agents)
	}
	t.index(t.Projects)
	for i, s := range t.Sessions() {
		if i < 9 {
			s.Number = i + 1
		}
	}
	return t
}

// rollUp sets the agent state of n and every node under it, from agents
// (each window's), and returns n's.
func rollUp(n *Node, agents map[string]tmux.AgentState) tmux.AgentState {
	if n.ID.Kind == Window {
		n.Agent = agents[n.Window]
	}
	for _, c := range n.Children {
		n.Agent = max(n.Agent, rollUp(c, agents))
	}
	return n.Agent
}

// worktrees are git project p's rows, from its sessions (p's children, by
// name) and its main clone and worktrees: each worktree in wt's order, then
// any session in none of them. A session belongs to the worktree whose
// folder holds its start folder, the longest such. A worktree with one
// session is that session's row, which stands for the worktree too; with
// several, the worktree's row, with its sessions under it; with none, the
// worktree's row alone. A worktree's row is named after its branch, else
// its folder.
func (t *Tree) worktrees(p *Node, rows []wt.Worktree) []*Node {
	in := make([][]*Node, len(rows)) // each worktree's sessions
	var children, elsewhere []*Node
	for _, s := range p.Children {
		if i := holder(rows, s.Path); i >= 0 {
			in[i] = append(in[i], s)
		} else {
			elsewhere = append(elsewhere, s)
		}
	}
	for i := range rows {
		w := rows[i]
		if len(in[i]) == 1 {
			s := in[i][0]
			s.Worktree = &w
			t.byID[ID{Worktree, w.Path}] = s
			children = append(children, s)
			continue
		}
		n := &Node{ID: ID{Worktree, w.Path}, Name: w.Branch, Path: w.Path, Worktree: &w, Parent: p, Children: in[i]}
		if n.Name == "" {
			n.Name = filepath.Base(w.Path)
		}
		for _, s := range in[i] {
			s.Parent = n
		}
		children = append(children, n)
	}
	return append(children, elsewhere...)
}

// holder is the index of the row whose folder holds dir, the longest such,
// or -1 if none does.
func holder(rows []wt.Worktree, dir string) int {
	at := -1
	for i, r := range rows {
		if within(dir, r.Path) && (at < 0 || len(r.Path) > len(rows[at].Path)) {
			at = i
		}
	}
	return at
}

// within says whether dir is folder or inside it.
func within(dir, folder string) bool {
	return dir == folder || strings.HasPrefix(dir, strings.TrimSuffix(folder, "/")+"/")
}

// byName orders nodes alphabetically by name, whatever the case, then by
// identity, so the order never depends on the sources'.
func byName(a, b *Node) int {
	return cmp.Or(
		cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
		cmp.Compare(a.Name, b.Name),
		cmp.Compare(a.ID.Key, b.ID.Key),
	)
}

func (t *Tree) index(nodes []*Node) {
	for _, n := range nodes {
		t.byID[n.ID] = n
		t.index(n.Children)
	}
}

// Node is the node with identity id, or nil if the tree has none. The
// identity of a worktree with one session is that session's row's too.
func (t *Tree) Node(id ID) *Node {
	if t == nil {
		return nil
	}
	return t.byID[id]
}

// Nearest is the identity of t's node that stands where id stood in old, a
// tree t was built after: that of id's node if t has it, or else of the
// worktree id's node stood for too. Otherwise, it's the nearest of id's
// siblings in old that t has, those after it first, or else the same for
// id's parent. It's the zero ID only if t is empty.
func (t *Tree) Nearest(old *Tree, id ID) ID {
	if n := t.Node(id); n != nil {
		return n.ID
	}
	n := old.Node(id)
	if n == nil {
		if t != nil && len(t.Projects) > 0 {
			return t.Projects[0].ID
		}
		return ID{}
	}
	if n.Worktree != nil {
		if w := t.Node(ID{Worktree, n.Worktree.Path}); w != nil {
			return w.ID
		}
	}
	siblings := old.Projects
	if n.Parent != nil {
		siblings = n.Parent.Children
	}
	i := slices.Index(siblings, n)
	for _, s := range siblings[i+1:] {
		if s := t.Node(s.ID); s != nil {
			return s.ID
		}
	}
	for j := i - 1; j >= 0; j-- {
		if s := t.Node(siblings[j].ID); s != nil {
			return s.ID
		}
	}
	if n.Parent != nil {
		return t.Nearest(old, n.Parent.ID)
	}
	return ID{}
}

// Sessions are the tree's sessions in order, top to bottom, folded or not.
func (t *Tree) Sessions() []*Node {
	if t == nil {
		return nil
	}
	return sessionsIn(t.Projects)
}

// sessionsIn are the sessions among nodes and beneath them, in order.
func sessionsIn(nodes []*Node) []*Node {
	var sessions []*Node
	for _, n := range nodes {
		if n.ID.Kind == Session {
			sessions = append(sessions, n)
		} else {
			sessions = append(sessions, sessionsIn(n.Children)...)
		}
	}
	return sessions
}

// Next is the session by places after session (an id) among the sessions of
// its project, in order, going round from the last to the first (before it
// for a negative by). It's nil if the tree hasn't session, or session is its
// project's only one.
func (t *Tree) Next(session string, by int) *Node {
	n := t.Node(ID{Session, session})
	if n == nil {
		return nil
	}
	project := n
	for project.Parent != nil {
		project = project.Parent
	}
	in := sessionsIn([]*Node{project})
	if len(in) < 2 {
		return nil
	}
	i := slices.Index(in, n) + by
	return in[(i%len(in)+len(in))%len(in)]
}

// Row is a node as the sidebar lists it, at its depth: projects are at 0.
type Row struct {
	*Node
	Depth int
}

// Rows are the tree's nodes in order, top to bottom, leaving out the
// children of every node that open says is folded.
func (t *Tree) Rows(open func(*Node) bool) []Row {
	if t == nil {
		return nil
	}
	var rows []Row
	var walk func(nodes []*Node, depth int)
	walk = func(nodes []*Node, depth int) {
		for _, n := range nodes {
			rows = append(rows, Row{n, depth})
			if open(n) {
				walk(n.Children, depth+1)
			}
		}
	}
	walk(t.Projects, 0)
	return rows
}
