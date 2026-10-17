package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// wtClient is what the model needs from wt: a *wt.Client, or a fake in tests.
type wtClient interface {
	List(dir string) ([]wt.Worktree, error)
}

// repo is one repo's worktrees.
type repo struct {
	dir  string        // where wt runs: the top folder of the main clone or worktree wtui was opened in
	rows []wt.Worktree // as wt lists them: the main clone, then each worktree
}

// name is how the screen names the repo: its main clone's path, once listed.
func (r repo) name() string {
	if len(r.rows) > 0 && r.rows[0].MainClone {
		return r.rows[0].Path
	}
	return r.dir
}

// mode is what wtui is showing, and so what keys do.
type mode int

const (
	listMode    mode = iota // the worktrees; keys act on the selected one
	messageMode             // a start-up problem, shown until a key exits wtui
)

// model is wtui's state.
type model struct {
	client   wtClient
	mode     mode
	repos    []repo // v1: always one, the working folder's repo
	selected string // the selected row's path, so it stays put when rows move
	loaded   bool   // a list has arrived, so wt failures go to the status line
	width    int    // the terminal's width; 0 until it's known
	message  string // messageMode's text
	status   string // the status line: the last wt failure since r
}

// listed is a list of one repo's worktrees from wt.
type listed struct {
	dir  string
	rows []wt.Worktree
	err  error
}

// newModel is wtui on the repo whose top folder is repoDir, as findRepo
// found it. If findRepo failed, wtui only shows that.
func newModel(client wtClient, repoDir string, findErr error) model {
	m := model{client: client, repos: []repo{{dir: repoDir}}}
	if findErr != nil {
		m.mode, m.message = messageMode, startupMessage(findErr)
	}
	return m
}

// startupMessage is what wtui shows when it can't list the worktrees at all.
func startupMessage(err error) string {
	msg := err.Error()
	if errors.Is(err, wt.ErrTooOld) {
		msg = "wt is too old for wtui: update tmux-kit (git pull in its folder).\n\n" + msg
	}
	return msg + "\n\npress any key to exit"
}

func (m model) Init() tea.Cmd {
	if m.mode == messageMode {
		return nil
	}
	return m.reload()
}

// reload lists every repo's worktrees again.
func (m model) reload() tea.Cmd {
	var cmds []tea.Cmd
	for _, r := range m.repos {
		cmds = append(cmds, func() tea.Msg {
			rows, err := m.client.List(r.dir)
			return listed{dir: r.dir, rows: rows, err: err}
		})
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		return m, nil
	}
	if m.mode == messageMode {
		if _, ok := msg.(tea.KeyPressMsg); ok {
			return m, tea.Quit
		}
		return m, nil
	}
	switch msg := msg.(type) {
	case listed:
		return m.withList(msg), nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			m = m.move(-1)
		case "down", "j":
			m = m.move(1)
		case "r":
			m.status = ""
			return m, m.reload()
		case "q", "esc", "ctrl+c": // raw mode turns ctrl+c into a key, not SIGINT
			return m, tea.Quit
		}
	}
	return m, nil
}

// withList takes in a repo's new list. If wt failed, wtui shows why: at
// start-up instead of the list, later in the status line under the last list.
func (m model) withList(l listed) model {
	if l.err != nil && !m.loaded {
		m.mode, m.message = messageMode, startupMessage(l.err)
		return m
	}
	if l.err != nil {
		m.status = l.err.Error()
		return m
	}
	m.loaded = true
	for i := range m.repos {
		if m.repos[i].dir == l.dir {
			m.repos[i].rows = l.rows
		}
	}
	if rows := m.rows(); len(rows) > 0 && m.selectedIndex() < 0 {
		m.selected = rows[0].Path
	}
	return m
}

// rows is every repo's rows, in the order they're shown.
func (m model) rows() []wt.Worktree {
	var rows []wt.Worktree
	for _, r := range m.repos {
		rows = append(rows, r.rows...)
	}
	return rows
}

// selectedIndex is the selected row's index in rows, or -1 if none is.
func (m model) selectedIndex() int {
	return slices.IndexFunc(m.rows(), func(w wt.Worktree) bool { return w.Path == m.selected })
}

// move selects the row by rows away, stopping at the first and last rows.
func (m model) move(by int) model {
	if rows := m.rows(); len(rows) > 0 {
		m.selected = rows[max(0, min(len(rows)-1, m.selectedIndex()+by))].Path
	}
	return m
}

func (m model) View() tea.View {
	if m.mode == messageMode {
		msg := m.message
		if m.width > 0 {
			// break paths at their slashes rather than anywhere
			msg = ansi.Wrap(msg, m.width, "/")
		}
		return screen(msg + "\n")
	}
	var b strings.Builder
	for _, r := range m.repos {
		fmt.Fprintf(&b, "%s\n\n", r.name())
		cells := make([][]string, len(r.rows))
		for i, w := range r.rows {
			cells[i] = rowCells(w)
		}
		for i, line := range columns(cells) {
			mark := "  "
			if r.rows[i].Path == m.selected {
				mark = "> "
			}
			b.WriteString(strings.TrimRight(mark+line, " ") + "\n")
		}
	}
	b.WriteString("\n")
	if m.status != "" {
		b.WriteString(m.status + "\n")
	}
	b.WriteString("↑/↓ j/k: move   r: refresh   q/esc: quit\n")
	return screen(b.String())
}

// screen is a full-screen view of content, so the terminal is left as it was
// when wtui exits.
func screen(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// rowCells is how a row shows a worktree: its folder, branch, state, session
// and dev URL.
func rowCells(w wt.Worktree) []string {
	name := filepath.Base(w.Path)
	if w.MainClone {
		name += " (main clone)"
	}
	branch := w.Branch
	if branch == "" && w.State != wt.Missing {
		branch = "detached"
	}
	session := w.Session
	if session == "" {
		session = "no session"
	}
	return []string{name, branch, string(w.State), session, w.DevURL}
}

// columns lines up cells: each column as wide on screen as its widest cell,
// two spaces apart.
func columns(cells [][]string) []string {
	var widths []int
	for _, row := range cells {
		for i, c := range row {
			if i == len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], ansi.StringWidth(c))
		}
	}
	lines := make([]string, len(cells))
	for i, row := range cells {
		for j, c := range row {
			lines[i] += c + strings.Repeat(" ", widths[j]-ansi.StringWidth(c)+2)
		}
	}
	return lines
}
