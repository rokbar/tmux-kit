package sidebar

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rokbar/tmux-kit/wtui/internal/tree"
)

// status is what the status line shows: the last failure of an action, of a
// build and of reading the active session, whichever there are.
type status struct{ action, build, active string }

func (s status) lines() []string {
	var lines []string
	for _, l := range []string{s.action, s.build, s.active} {
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
		if m.focused && r.ID == m.cursor {
			params = append(params, reverse)
		}
		params = append(params, m.highlight(r.Node)...)
		lines = append(lines, styled(m.fit(m.row(r)), params...))
	}
	status := m.statusLines()
	for room >= 0 && len(lines) < m.height-len(status) {
		lines = append(lines, "") // so the status line is at the bottom
	}
	lines = append(lines, status...)
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

// room is how many rows the view has room for under the header, or -1 for
// any number while the height isn't known.
func (m Model) room() int {
	if m.height <= 0 {
		return -1
	}
	return max(0, m.height-1-len(m.statusLines()))
}

// fit is line cut short or padded to the sidebar's width, once that's known.
func (m Model) fit(line string) string {
	if m.width <= 0 {
		return line
	}
	line = ansi.Truncate(line, m.width, "…")
	return line + strings.Repeat(" ", m.width-ansi.StringWidth(line))
}

// highlight is how node n stands out: the active session in bold cyan, and
// the rows above it (its project) in cyan; a session's active window in
// bold.
func (m Model) highlight(n *tree.Node) []string {
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

// row is how row r shows: after a two-column margin (where session numbers
// go), indented by its depth, a fold mark if it has children, and its name.
func (m Model) row(r tree.Row) string {
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
	return "  " + strings.Repeat("  ", r.Depth) + mark + " " + name
}
