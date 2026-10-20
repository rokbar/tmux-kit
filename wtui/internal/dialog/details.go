package dialog

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rokbar/tmux-kit/wtui/internal/detail"
)

// Reader reads what git says about the main clone or a worktree: a
// detail.Reader, or a fake in tests.
type Reader interface {
	Read(path string) (detail.Details, error)
}

// Details is the details dialog of the main clone or worktree at path: its
// branch ("" if it has none: a detached HEAD) and dev URL ("" for none), as
// wt listed them, and what git, through reader, reads in its folder: where it
// stands against its upstream, its changed files and its last commits. Any
// key closes it.
func Details(reader Reader, path, branch, devURL string) tea.Model {
	return details{reader: reader, path: path, branch: branch, devURL: devURL}
}

type details struct {
	reader               Reader
	path, branch, devURL string
	// read is what git read, once it has, or why it couldn't
	read          *detailsRead
	width, height int
}

type detailsRead struct {
	details detail.Details
	err     error
}

func (d details) Init() tea.Cmd {
	reader, path := d.reader, d.path
	return func() tea.Msg {
		got, err := reader.Read(path)
		return detailsRead{got, err}
	}
}

func (d details) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width, d.height = msg.Width, msg.Height
	case detailsRead:
		d.read = &msg
	case tea.KeyPressMsg:
		return d, tea.Quit
	}
	return d, nil
}

func (d details) View() tea.View {
	branch := "branch " + d.branch
	if d.branch == "" {
		branch = "no branch (detached HEAD)"
	}
	devURL := "dev URL " + d.devURL
	if d.devURL == "" {
		devURL = "no dev URL"
	}
	room := -1 // for the lines under the branch, any number until the height is known
	if d.height > 0 {
		room = max(0, d.height-2)
	}
	lines := []string{branch}
	switch {
	case d.read == nil:
		lines = append(lines, "reading the details…")
	case d.read.err != nil:
		// wrapped, since it's the whole story
		wrapped := ansi.Wrap(d.read.err.Error(), max(1, d.width-2), "")
		lines = append(lines, AtMost(room, strings.Split(wrapped, "\n"))...)
	default:
		g := d.read.details
		lines = append(lines, Fit(room, Section{Heading: devURL}, Section{Heading: Upstream(g)},
			List("changes", g.Changes), List("commits", g.Commits))...)
	}
	return view(lines, d.width, d.height)
}

// Upstream says how far a worktree is ahead of and behind its upstream, or
// that it has none, so isn't pushed.
func Upstream(d detail.Details) string {
	switch {
	case d.Upstream == "":
		return "not pushed"
	case d.Ahead > 0 && d.Behind > 0:
		return fmt.Sprintf("%d ahead, %d behind %s", d.Ahead, d.Behind, d.Upstream)
	case d.Ahead > 0:
		return fmt.Sprintf("%d ahead of %s", d.Ahead, d.Upstream)
	case d.Behind > 0:
		return fmt.Sprintf("%d behind %s", d.Behind, d.Upstream)
	}
	return "up to date with " + d.Upstream
}

// Section is a part of what's shown about a worktree: a heading, and items
// under it.
type Section struct {
	Heading string
	Items   []string
}

// List is items under a heading saying what they are, or just a heading
// saying there are none.
func List(what string, items []string) Section {
	if len(items) == 0 {
		return Section{Heading: "no " + what}
	}
	return Section{Heading: what + ":", Items: items}
}

// Fit lays sections out in at most room lines (any number if room is
// negative): each heading, with as many of its items as there's room for
// indented under it. The room under the headings is shared out a line at a
// time to each section in turn, so a short one leaves the rest to longer ones.
// A section that's cut ends with a line saying how many more items it has: its
// heading, if it has no room for any.
func Fit(room int, sections ...Section) []string {
	shares := make([]int, len(sections))
	left := room - len(sections) // the lines under the headings
	for more := true; more; {
		more = false
		for i, s := range sections {
			if shares[i] < len(s.Items) && (room < 0 || left > 0) {
				shares[i]++
				left--
				more = true
			}
		}
	}
	var lines []string
	for i, s := range sections {
		heading, shown := s.Heading, s.Items[:shares[i]]
		if more := len(s.Items) - shares[i]; more > 0 && shares[i] == 0 {
			heading += fmt.Sprintf(" … %d more", more)
		} else if more > 0 {
			shown = append(slices.Clone(shown[:shares[i]-1]), fmt.Sprintf("… %d more", more+1))
		}
		lines = append(lines, heading)
		for _, item := range shown {
			lines = append(lines, "  "+item)
		}
	}
	return AtMost(room, lines)
}
