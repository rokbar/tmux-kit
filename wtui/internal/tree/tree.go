// Package tree turns what wtui knows about the normal server (its sessions
// and windows, and which git repo each session's start folder is in) into
// the sidebar's tree: projects, their sessions, and those sessions' windows,
// in order. It does no I/O, so it's the same tree for the same sources.
package tree

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// Sources are what the tree is built from.
type Sources struct {
	Snapshot tmux.Snapshot
	// MainClones has each start folder's main clone, "" for a folder in no
	// git repo. A folder it doesn't have (its lookup failed) counts as one in
	// no repo.
	MainClones map[string]string
}

// Kind is what a node stands for.
type Kind int

const (
	Project Kind = iota
	Session
	Window
)

// ID is a node's identity, which stays the same across builds for as long
// as the thing it stands for exists: a project's folder, a session's id
// (not its name), and a window's id within its session.
type ID struct {
	Kind Kind
	Key  string
}

// Node is a row of the tree.
type Node struct {
	ID   ID
	Name string // what the row shows: a folder's, session's or window's name
	// Path is a project's folder, or a session's start folder.
	Path string
	// Session is a session's or window's session (an id); Window a window's
	// id, and Index its index.
	Session, Window string
	Index           int
	// Active is whether a window is its session's current window.
	Active   bool
	Parent   *Node
	Children []*Node
}

// Tree is the sidebar's tree. It doesn't change once built.
type Tree struct {
	Projects []*Node
	byID     map[ID]*Node
}

// Build builds the tree. A session belongs to the project of its start
// folder: the main clone if the folder is in a git repo, otherwise the
// folder itself. Projects are labelled by folder name and alphabetical;
// sessions within a project go by name, and windows by index.
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
	}
	t.index(t.Projects)
	return t
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

// Node is the node with identity id, or nil if the tree has none.
func (t *Tree) Node(id ID) *Node {
	if t == nil {
		return nil
	}
	return t.byID[id]
}

// Nearest is the identity of t's node that stands where id stood in old, a
// tree t was built after: id itself if t has it. Otherwise, it's the nearest
// of id's siblings in old that t has, those after it first, or else the same
// for id's parent. It's the zero ID only if t is empty.
func (t *Tree) Nearest(old *Tree, id ID) ID {
	if t.Node(id) != nil {
		return id
	}
	n := old.Node(id)
	if n == nil {
		if t != nil && len(t.Projects) > 0 {
			return t.Projects[0].ID
		}
		return ID{}
	}
	siblings := old.Projects
	if n.Parent != nil {
		siblings = n.Parent.Children
	}
	i := slices.Index(siblings, n)
	for _, s := range siblings[i+1:] {
		if t.Node(s.ID) != nil {
			return s.ID
		}
	}
	for j := i - 1; j >= 0; j-- {
		if t.Node(siblings[j].ID) != nil {
			return siblings[j].ID
		}
	}
	if n.Parent != nil {
		return t.Nearest(old, n.Parent.ID)
	}
	return ID{}
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
