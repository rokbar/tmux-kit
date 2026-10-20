package sidebar

import (
	"fmt"
	"net/url"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rokbar/tmux-kit/wtui/internal/tree"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// status is what the status line shows: the hint for the last key, if it
// didn't apply, and the last failure of an action, of a build, of listing git
// projects' worktrees and of reading the active session, whichever there are.
type status struct{ hint, action, build, lists, active string }

func (s status) lines() []string {
	var lines []string
	for _, l := range []string{s.hint, s.action, s.build, s.lists, s.active} {
		if l != "" && (len(lines) == 0 || lines[len(lines)-1] != l) {
			lines = append(lines, l)
		}
	}
	return lines
}

// SGR parameters
const (
	bold    = "1"
	dim     = "2"
	reverse = "7"
	red     = "31"
	green   = "32"
	yellow  = "33"
	cyan    = "36"
)

// styled is text in the style of SGR parameters params.
func styled(text string, params ...string) string {
	if len(params) == 0 {
		return text
	}
	return "\x1b[" + strings.Join(params, ";") + "m" + text + "\x1b[0m"
}

func (m Model) View() tea.View {
	// The header shows whether the sidebar has focus: in reverse video with
	// it, dim without.
	head := []string{dim}
	if m.focused {
		head = []string{bold, reverse}
	}
	lines := []string{styled(m.fit(" wtui"), head...)}
	rows, room := m.rows(), m.room()
	if room >= 0 {
		rows = rows[min(m.top, len(rows)):min(m.top+room, len(rows))]
	}
	for _, r := range rows {
		var params []string
		cursor := m.focused && r.ID == m.cursor
		if cursor {
			params = append(params, reverse)
		}
		params = append(params, m.highlight(r.Node)...)
		lines = append(lines, styled(m.fitTo(m.row(r), m.width-agentColumn), params...)+agentMark(r.Agent, cursor))
	}
	status := m.statusLines()
	for room >= 0 && len(lines) < m.height-len(status)-1 {
		lines = append(lines, "") // so the status line is at the bottom
	}
	lines = append(lines, status...)
	if room >= 0 {
		lines = append(lines, styled(m.fit(footer), dim))
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.ReportFocus = true
	v.MouseMode = tea.MouseModeCellMotion // for clicks on rows
	return v
}

// maxStatus is how many lines one failure takes in the status line at most.
const maxStatus = 3

// statusLines are the status line's lines, each failure wrapped to the
// sidebar's width once that's known.
func (m Model) statusLines() []string {
	var lines []string
	for _, s := range m.status.lines() {
		if m.width <= 0 {
			lines = append(lines, s)
			continue
		}
		wrapped := strings.Split(ansi.Wrap(s, m.width, "/"), "\n")
		if len(wrapped) > maxStatus {
			wrapped = append(wrapped[:maxStatus-1], strings.Join(wrapped[maxStatus-1:], " "))
		}
		for _, l := range wrapped {
			lines = append(lines, m.fit(l))
		}
	}
	return lines
}

// footer is the sidebar's last line, once its height is known.
const footer = "? shortcuts"

// room is how many rows the view has room for between the header and the
// status line, above the footer, or -1 for any number while the height isn't
// known.
func (m Model) room() int {
	if m.height <= 0 {
		return -1
	}
	return max(0, m.height-2-len(m.statusLines()))
}

// fit is line cut short or padded to the sidebar's width, once that's known.
func (m Model) fit(line string) string { return m.fitTo(line, m.width) }

// fitTo is line cut short or padded to width columns, once the sidebar's
// width is known.
func (m Model) fitTo(line string, width int) string {
	if m.width <= 0 {
		return line
	}
	width = max(0, width)
	line = ansi.Truncate(line, width, "…")
	return line + strings.Repeat(" ", width-ansi.StringWidth(line))
}

// highlight is how node n stands out: the active session in bold cyan, and
// the rows above it (its worktree and project) in cyan; a session's active
// window in bold. A worktree without a session (a missing one too) is dim.
func (m Model) highlight(n *tree.Node) []string {
	if n.ID.Kind == tree.Worktree && len(n.Children) == 0 {
		return []string{dim}
	}
	switch n.ID.Kind {
	case tree.Window:
		if n.Active {
			return []string{bold}
		}
	case tree.Session:
		if n.Session == m.active {
			return []string{bold, cyan}
		}
	default:
		active := m.tree.Node(tree.ID{Kind: tree.Session, Key: m.active})
		for p := active; p != nil; p = p.Parent {
			if p == n {
				return []string{cyan}
			}
		}
	}
	return nil
}

// rows are the tree's rows that show, top to bottom.
func (m Model) rows() []tree.Row { return m.tree.Rows(m.open) }

// row is how row r shows: after a two-column margin, which has a session's
// number, indented by its depth, a fold mark if it has children, its name,
// and then its markers (see markers). A name too long for the sidebar (short
// of the agent state's column) is cut short before its markers are.
func (m Model) row(r tree.Row) string {
	margin := "  "
	if r.Number > 0 {
		margin = fmt.Sprintf("%d ", r.Number)
	}
	mark := " "
	if len(r.Children) > 0 {
		mark = "▸"
		if m.open(r.Node) {
			mark = "▾"
		}
	}
	name := r.Name
	if r.ID.Kind == tree.Window {
		name = fmt.Sprintf("%d %s", r.Index, r.Name)
	}
	lead, marks := margin+strings.Repeat("  ", r.Depth)+mark+" ", markers(r.Node)
	if m.width > 0 && marks != "" {
		name = ansi.Truncate(name, m.width-agentColumn-ansi.StringWidth(lead+marks), "…")
	}
	return lead + name + marks
}

// markers are what node n's row shows after its name if it stands for a
// worktree: ± if it's dirty, ✗ if it's missing, then its dev URL's port
// (:3001), if the URL has one.
func markers(n *tree.Node) string {
	w := n.Worktree
	if w == nil {
		return ""
	}
	var marks string
	switch w.State {
	case wt.Dirty:
		marks = " ±"
	case wt.Missing:
		marks = " ✗"
	}
	if u, err := url.Parse(w.DevURL); err == nil && u.Port() != "" {
		marks += " :" + u.Port()
	}
	return marks
}

// missing says whether n stands for a missing worktree.
func missing(n *tree.Node) bool { return n.Worktree != nil && n.Worktree.State == wt.Missing }
