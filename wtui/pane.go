package main

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rokbar/tmux-kit/wtui/internal/detail"
)

// detailReader is what the detail pane needs from git: a detail.Reader, or a
// fake in tests.
type detailReader interface {
	Read(path string) (detail.Details, error)
}

// detailsRead is git having read the details of the worktree at path, or
// failed to (err), for the nth read wtui asked for.
type detailsRead struct {
	n       int
	path    string
	details detail.Details
	err     error
}

// readDetails has git read the selected worktree's details, without waiting
// for it. A missing worktree has no folder to read: its pane is empty.
func (m model) readDetails() (model, tea.Cmd) {
	_, w, ok := m.selectionThere()
	if !ok {
		m.pane = detailsRead{}
		return m, nil
	}
	m.reads++
	n, reader := m.reads, m.reader
	return m, func() tea.Msg {
		d, err := reader.Read(w.Path)
		return detailsRead{n: n, path: w.Path, details: d, err: err}
	}
}

// withDetails takes in what git read, if it's the selected worktree's: the
// selection may have moved on since. Reads run side by side, so an older one
// can arrive after a newer one; only the newest counts.
func (m model) withDetails(r detailsRead) model {
	if _, w, ok := m.selectionThere(); ok && w.Path == r.path && r.n == m.reads {
		m.pane = r
	}
	return m
}

// paneLines is the detail pane in at most room lines (any number if room is
// negative): the selected worktree's details once git has read them, or why
// it couldn't. A missing worktree has none.
func (m model) paneLines(room int) []string {
	p := m.pane
	if p.path == "" || p.path != m.selected {
		return nil // not read yet
	}
	var lines []string
	if p.err != nil {
		lines = atMost(room, strings.Split(p.err.Error(), "\n"))
	} else {
		d := p.details
		lines = fit(room, section{heading: upstreamLine(d)}, listOf("changes", d.Changes), listOf("commits", d.Commits))
	}
	for i := range lines {
		lines[i] = m.oneLine(lines[i])
	}
	return lines
}

// upstreamLine says how far a worktree is ahead of and behind its upstream,
// or that it has none, so isn't pushed.
func upstreamLine(d detail.Details) string {
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

// section is a part of the detail pane: a heading, and items under it.
type section struct {
	heading string
	items   []string
}

// listOf is items under a heading saying what they are, or just a heading
// saying there are none.
func listOf(what string, items []string) section {
	if len(items) == 0 {
		return section{heading: "no " + what}
	}
	return section{heading: what + ":", items: items}
}

// fit lays sections out in at most room lines (any number if room is
// negative): each heading, with as many of its items as there's room for
// indented under it. The room under the headings is shared out a line at a
// time to each section in turn, so a short one leaves the rest to longer ones.
// A section that's cut ends with a line saying how many more items it has: its
// heading, if it has no room for any.
func fit(room int, sections ...section) []string {
	shares := make([]int, len(sections))
	left := room - len(sections) // the lines under the headings
	for more := true; more; {
		more = false
		for i, s := range sections {
			if shares[i] < len(s.items) && (room < 0 || left > 0) {
				shares[i]++
				left--
				more = true
			}
		}
	}
	var lines []string
	for i, s := range sections {
		heading, shown := s.heading, s.items[:shares[i]]
		if more := len(s.items) - shares[i]; more > 0 && shares[i] == 0 {
			heading += fmt.Sprintf(" … %d more", more)
		} else if more > 0 {
			shown = append(slices.Clone(shown[:shares[i]-1]), fmt.Sprintf("… %d more", more+1))
		}
		lines = append(lines, heading)
		for _, item := range shown {
			lines = append(lines, "  "+item)
		}
	}
	return atMost(room, lines)
}

// atMost is the first room lines (all of them if room is negative).
func atMost(room int, lines []string) []string {
	if room < 0 {
		return lines
	}
	return lines[:min(room, len(lines))]
}

// oneLine is line as one line of the window: cut off at its edge, once its width
// is known, rather than run on to the next. Control characters, which git
// prints as they are in a commit's subject, are shown as "?": they could clear
// or draw over the list, or make the line wider than it looks.
func (m model) oneLine(line string) string {
	line = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, line)
	if m.width <= 0 {
		return line
	}
	return ansi.Truncate(line, m.width, "…")
}
