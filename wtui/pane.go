package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/detail"
	"github.com/rokbar/tmux-kit/wtui/internal/dialog"
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
		lines = dialog.AtMost(room, strings.Split(p.err.Error(), "\n"))
	} else {
		d := p.details
		lines = dialog.Fit(room, dialog.Section{Heading: dialog.Upstream(d)}, dialog.List("changes", d.Changes), dialog.List("commits", d.Commits))
	}
	for i := range lines {
		lines[i] = dialog.OneLine(lines[i], m.width)
	}
	return lines
}
