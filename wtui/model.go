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
	Open(dir, path string) error
	Remove(dir, path string, force bool) (string, error)
	Pick(dir string, done tea.ExecCallback) tea.Cmd
	GitUI(dir, path string, done tea.ExecCallback) tea.Cmd
}

// repo is one repo's worktrees.
type repo struct {
	// where wt runs, and how the screen names the repo: the top folder of the
	// main clone or worktree wtui was opened in, then the main clone once a
	// list names it. wt never removes that, unlike the worktree.
	dir  string
	rows []wt.Worktree // as wt lists them: the main clone, then each worktree
}

// mode is what wtui is showing, and so what keys do.
type mode int

const (
	listMode    mode = iota // the worktrees; keys act on the selected one
	confirmMode             // asking whether to remove a worktree
	refusedMode             // wt refused to remove it for safety: showing why, asking whether to force
	messageMode             // a start-up problem, shown until a key exits wtui
)

// model is wtui's state.
type model struct {
	client   wtClient
	reader   detailReader
	opener   urlOpener
	watcher  clientWatcher
	mode     mode
	repos    []repo // v1: always one, the working folder's repo
	selected string // the selected row's path, so it stays put when rows move
	loaded   bool   // a list has arrived, so wt failures go to the status line
	width    int    // the terminal's width; 0 until it's known
	height   int    // the terminal's height; 0 until it's known
	message  string // messageMode's text
	status   string // the status line: what wt or the opener said last, a hint, or what went wrong, until r
	handoff  string // the session to take the terminal to once wtui has quit
	// what confirmMode and refusedMode ask about
	removing removal
	pane     detailsRead // what git last read of the selected worktree
	reads    int         // how many times wtui has asked git to read one
}

// removal is the worktree a prompt asks about removing, and its repo's folder.
type removal struct {
	dir      string
	worktree wt.Worktree
	refusal  string // why wt refused to remove it, once it has
	top      int    // the first line of refusal on screen, when it doesn't all fit
}

// question is the prompt asking whether to remove the worktree: it names the
// worktree, and the session that would end with it.
func (r removal) question() string {
	name := filepath.Base(r.worktree.Path)
	if r.worktree.Session == "" {
		return "remove " + name + " (no session)?"
	}
	return "remove " + name + " and end session " + r.worktree.Session + "?"
}

// listed is a list of one repo's worktrees from wt.
type listed struct {
	dir  string
	rows []wt.Worktree
	err  error
}

// opened is wt having opened path's session, or failed to (err), and the list
// of path's repo that followed, which has the session in it.
type opened struct {
	path string
	err  error
	list listed
}

// removed is wt having removed a worktree, which it said, or failed to (err),
// and the list of its repo that followed.
type removed struct {
	removal removal
	said    string
	err     error
	list    listed
}

// newModel is wtui on the repo whose top folder is repoDir, as findRepo
// found it. If findRepo failed, wtui only shows that.
func newModel(client wtClient, reader detailReader, opener urlOpener, watcher clientWatcher, repoDir string, findErr error) model {
	m := model{client: client, reader: reader, opener: opener, watcher: watcher, repos: []repo{{dir: repoDir}}}
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
		cmds = append(cmds, func() tea.Msg { return m.list(r.dir) })
	}
	return tea.Batch(cmds...)
}

// resumeAfter is wtui back on the list once a program it paused for (the
// picker, the git UI) has exited, having failed (err) or not. It lists
// again, since the program may have changed what wt lists. The status line
// says what went wrong, or is cleared: what was there was about something
// before it.
func (m model) resumeAfter(err error) (model, tea.Cmd) {
	m.status = ""
	if err != nil {
		m.status = err.Error()
	}
	return m, m.reload()
}

// list has wt list the worktrees of the repo it runs in at dir.
func (m model) list(dir string) listed {
	rows, err := m.client.List(dir)
	return listed{dir: dir, rows: rows, err: err}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
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
		return m.withList(msg)
	case opened:
		return m.withOpened(msg)
	case removed:
		return m.withRemoved(msg)
	case picked:
		return m.withPicked(msg)
	case gitUIClosed:
		return m.resumeAfter(msg.err)
	case detailsRead:
		return m.withDetails(msg), nil
	case urlOpened:
		return m.withURLOpened(msg), nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" { // raw mode turns ctrl+c into a key, not SIGINT
			return m, tea.Quit
		}
		if m.mode == confirmMode || m.mode == refusedMode {
			return m.answer(msg.String())
		}
		switch msg.String() {
		case "up", "k":
			return m.move(-1)
		case "down", "j":
			return m.move(1)
		case "enter":
			return m, m.open()
		case "n":
			return m, m.pick()
		case "x":
			// wt never removes the main clone
			if r, w, ok := m.selection(); ok && !w.MainClone {
				m.mode, m.removing = confirmMode, removal{dir: r.dir, worktree: w}
			}
		case "o":
			return m.openDevURL()
		case "g":
			return m, m.runGitUI()
		case "r":
			m.status = ""
			return m, m.reload()
		case "q", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

// withList takes in a repo's new list, and reads the selected worktree's
// details again. If wt failed, wtui shows why: at start-up instead of the
// list, later in the status line under the last list.
func (m model) withList(l listed) (model, tea.Cmd) {
	if l.err != nil && !m.loaded {
		m.mode, m.message = messageMode, startupMessage(l.err)
		return m, nil
	}
	if l.err != nil {
		m.status = l.err.Error()
		return m, nil
	}
	m.loaded = true
	for i := range m.repos {
		if m.repos[i].dir == l.dir {
			m.repos[i].rows = l.rows
			if len(l.rows) > 0 && l.rows[0].MainClone {
				m.repos[i].dir = l.rows[0].Path
			}
		}
	}
	if rows := m.rows(); len(rows) > 0 && m.selectedIndex() < 0 {
		m.selected = rows[0].Path
	}
	return m.readDetails()
}

// withOpened takes in the list that followed opening a worktree's session,
// then quits to hand the terminal off to that session. If wt failed, or lists
// no session there, wtui stays and the status line says so.
func (m model) withOpened(o opened) (model, tea.Cmd) {
	m, read := m.withList(o.list)
	if o.err != nil {
		m.status = o.err.Error()
		return m, read
	}
	if o.list.err != nil {
		return m, nil // withList put it in the status line
	}
	// by its folder, as wt finds it: its name may not be the one wt chose
	i := slices.IndexFunc(o.list.rows, at(o.path))
	if i < 0 || o.list.rows[i].Session == "" {
		m.status = "no session started in " + o.path
		return m, read
	}
	m.handoff = o.list.rows[i].Session
	return m, tea.Quit
}

// answer is a key pressed at a prompt: y removes the worktree it asks about,
// forcing it once wt has refused to for safety; n, q and esc cancel. Either
// way, it's back to the list. Up and down scroll wt's reasons for refusing.
func (m model) answer(key string) (model, tea.Cmd) {
	switch key {
	case "y":
		force := m.mode == refusedMode
		m.mode = listMode
		return m, m.remove(force)
	case "n", "q", "esc":
		m.mode = listMode
	case "up", "k":
		m = m.scroll(-1)
	case "down", "j":
		m = m.scroll(1)
	}
	return m, nil
}

// forcePrompt is the question under wt's reasons for refusing, and its keys.
const forcePrompt = "force?\ny: force   n/q/esc: cancel"

// reasons is wt's reasons for refusing, wrapped to the window, and how many
// of their lines fit on screen above a blank line and the prompt: all of
// them, or, if they don't all fit, one less than that, for a line saying
// which are shown.
func (m model) reasons() (lines []string, fit int) {
	lines = strings.Split(m.wrap(m.removing.refusal), "\n")
	room := m.height - strings.Count(m.wrap(forcePrompt), "\n") - 2
	if m.height <= 0 || len(lines) <= room {
		return lines, len(lines)
	}
	return lines, max(room-1, 1)
}

// scroll moves which of wt's reasons for refusing are on screen by lines,
// within them.
func (m model) scroll(by int) model {
	lines, fit := m.reasons()
	m.removing.top = max(0, min(len(lines)-fit, m.removing.top+by))
	return m
}

// remove has wt remove the worktree the prompt asked about, then lists its
// repo again, whether or not that worked.
func (m model) remove(force bool) tea.Cmd {
	r := m.removing
	return func() tea.Msg {
		said, err := m.client.Remove(r.dir, r.worktree.Path, force)
		return removed{removal: r, said: said, err: err, list: m.list(r.dir)}
	}
}

// withRemoved takes in the list that followed a removal. The status line says
// what wt said, that it removed the worktree or why it couldn't, then why the
// list failed, if it did. If wt refused because something would be lost, its
// reasons get a screen of their own instead, which asks whether to force.
func (m model) withRemoved(r removed) (model, tea.Cmd) {
	m, read := m.withList(r.list)
	var status []string
	switch {
	case errors.Is(r.err, wt.ErrSafetyRefusal):
		m.mode, m.removing = refusedMode, r.removal
		m.removing.refusal = r.err.Error()
	case r.err != nil:
		status = append(status, r.err.Error())
	default:
		status = append(status, r.said)
	}
	if r.list.err != nil {
		status = append(status, r.list.err.Error())
	}
	m.status = strings.Join(status, "\n")
	return m, read
}

// open has wt open the selected worktree's session, then lists its repo again,
// whether or not that worked. A missing worktree has no folder to open one in.
func (m model) open() tea.Cmd {
	r, w, ok := m.selectionThere()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		err := m.client.Open(r.dir, w.Path)
		return opened{path: w.Path, err: err, list: m.list(r.dir)}
	}
}

// selection is the selected row and the repo it's in; ok is false if no row
// is selected.
func (m model) selection() (repo, wt.Worktree, bool) {
	for _, r := range m.repos {
		if i := slices.IndexFunc(r.rows, at(m.selected)); i >= 0 {
			return r, r.rows[i], true
		}
	}
	return repo{}, wt.Worktree{}, false
}

// selectionThere is the selected row and the repo it's in, as selection, if
// the worktree's folder is there: ok is false for a missing worktree too,
// which has no folder to act or look in.
func (m model) selectionThere() (repo, wt.Worktree, bool) {
	r, w, ok := m.selection()
	return r, w, ok && w.State != wt.Missing
}

// at matches the row for the worktree at path.
func at(path string) func(wt.Worktree) bool {
	return func(w wt.Worktree) bool { return w.Path == path }
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
	return slices.IndexFunc(m.rows(), at(m.selected))
}

// move selects the row by rows away, stopping at the first and last rows, and
// reads the details of the worktree it selects.
func (m model) move(by int) (model, tea.Cmd) {
	rows := m.rows()
	if len(rows) == 0 {
		return m, nil
	}
	was := m.selected
	m.selected = rows[max(0, min(len(rows)-1, m.selectedIndex()+by))].Path
	if m.selected == was {
		return m, nil
	}
	return m.readDetails()
}

func (m model) View() tea.View {
	if m.mode == messageMode {
		return screen(m.wrap(m.message) + "\n")
	}
	if m.mode == refusedMode {
		lines, fit := m.reasons()
		shown := strings.Join(lines, "\n")
		if fit < len(lines) {
			top := min(m.removing.top, len(lines)-fit) // the window may have grown
			shown = strings.Join(lines[top:top+fit], "\n") +
				fmt.Sprintf("\n↑/↓ j/k: scroll (lines %d–%d of %d)", top+1, top+fit, len(lines))
		}
		return screen(shown + "\n\n" + m.wrap(forcePrompt) + "\n")
	}
	var b strings.Builder
	for _, r := range m.repos {
		fmt.Fprintf(&b, "%s\n\n", r.dir)
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
	bottom := m.bottom()
	room := -1 // no limit while the window's height isn't known
	if m.height > 0 {
		// what the list and the bottom lines leave, less a blank line
		room = max(0, m.height-strings.Count(b.String(), "\n")-strings.Count(bottom, "\n")-1)
	}
	if pane := m.paneLines(room); len(pane) > 0 || room > 0 {
		// filled out, so the bottom lines stay at the bottom of the window
		for len(pane) < room {
			pane = append(pane, "")
		}
		b.WriteString(strings.Join(pane, "\n") + "\n\n")
	}
	return screen(b.String() + bottom)
}

// bottom is what's under the list and the detail pane: the prompt asking
// whether to remove a worktree, or the status line and the keys.
func (m model) bottom() string {
	if m.mode == confirmMode {
		return m.removing.question() + "\ny: remove   n/q/esc: cancel\n"
	}
	var status string
	if m.status != "" {
		status = m.wrap(m.status) + "\n"
	}
	return status + m.keyLine()
}

// listKeys is what the keys do in the list, in the order the key line shows
// them.
var listKeys = []string{"↑/↓ j/k: move", "enter: open", "n: picker", "x: remove", "o: dev URL", "g: git UI", "r: refresh", "q/esc: quit"}

// keyLine is listKeys three spaces apart, on as many lines as the window's
// width needs, once that's known: the screen would cut off what's past its
// edge. A key is never split over two lines.
func (m model) keyLine() string {
	var b strings.Builder
	lineWidth := 0 // of the line so far
	for i, k := range listKeys {
		keyWidth := ansi.StringWidth(k)
		switch {
		case i == 0:
		case m.width > 0 && lineWidth+3+keyWidth > m.width:
			b.WriteString("\n")
			lineWidth = 0
		default:
			b.WriteString("   ")
			lineWidth += 3
		}
		b.WriteString(k)
		lineWidth += keyWidth
	}
	return b.String() + "\n"
}

// wrap wraps text to the window's width, once that's known, breaking paths
// at their slashes rather than anywhere: the screen would cut off what's past
// its edge.
func (m model) wrap(text string) string {
	if m.width <= 0 {
		return text
	}
	return ansi.Wrap(text, m.width, "/")
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
