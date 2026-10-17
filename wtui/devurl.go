package main

import (
	"path/filepath"

	tea "charm.land/bubbletea/v2"
)

// urlOpener is what o needs to open a dev URL in the browser: a
// browser.Opener, or a fake in tests.
type urlOpener interface {
	Open(url string) error
}

// urlOpened is the opener having opened url in the browser, or failed to
// (err).
type urlOpened struct {
	url string
	err error
}

// openDevURL has the browser open the selected worktree's dev URL, without
// waiting for it. If it has none, the status line says so. A missing
// worktree has no dev server to open.
func (m model) openDevURL() (model, tea.Cmd) {
	_, w, ok := m.selectionThere()
	if !ok {
		return m, nil
	}
	if w.DevURL == "" {
		m.status = filepath.Base(w.Path) + " has no dev URL"
		return m, nil
	}
	url, opener := w.DevURL, m.opener
	return m, func() tea.Msg {
		return urlOpened{url: url, err: opener.Open(url)}
	}
}

// withURLOpened takes in what came of opening a dev URL: the status line says
// it's open, or why the opener failed.
func (m model) withURLOpened(o urlOpened) model {
	if o.err != nil {
		m.status = o.err.Error()
	} else {
		m.status = "opened " + o.url
	}
	return m
}
