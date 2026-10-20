package dialog

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// closing is a dialog's last line.
const closing = "any key closes"

// Page is a dialog that shows lines (e.g. the shortcuts), until any key
// closes it.
func Page(lines []string) tea.Model { return page{lines: lines} }

// PageSize is the size of a popup that fits a page of lines exactly: its
// lines, then a blank line and the line saying any key closes it, with a
// column either side, inside the popup's border.
func PageSize(lines []string) (width, height int) {
	width = ansi.StringWidth(closing)
	for _, l := range lines {
		width = max(width, ansi.StringWidth(l))
	}
	return width + 4, len(lines) + 4
}

type page struct {
	lines         []string
	width, height int
}

func (p page) Init() tea.Cmd { return nil }

func (p page) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width, p.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		return p, tea.Quit
	}
	return p, nil
}

func (p page) View() tea.View { return view(p.lines, p.width, p.height) }

// view is a dialog's view, in a popup of width by height (unknown while 0):
// lines, each as one line with a column either side, as many as there's room
// for above the line saying any key closes the dialog, which is the last
// line. A blank line comes before it, if there's room.
func view(lines []string, width, height int) tea.View {
	if height > 0 {
		lines = atMost(height-1, lines)
	}
	shown := make([]string, 0, max(height, len(lines)+2))
	for _, l := range lines {
		shown = append(shown, " "+oneLine(l, width-2))
	}
	for len(shown) < height-1 || height <= 0 && len(shown) < len(lines)+1 {
		shown = append(shown, "")
	}
	shown = append(shown, " \x1b[2m"+oneLine(closing, width-2)+"\x1b[0m")
	v := tea.NewView(strings.Join(shown, "\n"))
	v.AltScreen = true
	return v
}

// atMost is the first room lines (all of them if room is negative).
func atMost(room int, lines []string) []string {
	if room < 0 {
		return lines
	}
	return lines[:max(0, min(room, len(lines)))]
}

// oneLine is line as one line of a window width columns wide: cut off at its
// edge, once its width is known (more than 0), rather than run on to the
// next. Control characters, which git prints as they are in a commit's
// subject, are shown as "?": they could clear or draw over the dialog, or
// make the line wider than it looks.
func oneLine(line string, width int) string {
	line = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, line)
	if width <= 0 {
		return line
	}
	return ansi.Truncate(line, width, "…")
}
