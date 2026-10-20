package sidebar

import (
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rokbar/tmux-kit/wtui/internal/dialog"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
	"github.com/rokbar/tmux-kit/wtui/internal/tree"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// gitWorktreeOf is the main clone or worktree, as wt lists it, that row n is
// in, for a key that needs git there (g, i, n). A missing one has no folder,
// and a row in none (a non-git project, a session in no worktree wt lists)
// isn't in git as far as wt says: both get a hint.
func gitWorktreeOf(n *tree.Node) (*wt.Worktree, hint) {
	w := n.InWorktree()
	switch {
	case w == nil:
		return nil, hint(sessionOf(n).Name + " isn't in a git repo")
	case w.State == wt.Missing:
		return nil, isMissing(w)
	}
	return w, ""
}

// details shows w's details: the details dialog.
func (m Model) details(w *wt.Worktree) (Model, tea.Cmd) {
	return m.dialog(tmux.Popup{Title: " details of " + filepath.Base(w.Path) + " ", Dir: w.Path,
		Command: []string{"details", w.Path, w.Branch, w.DevURL}})
}

// pick runs the picker in w, the invoking worktree. Whatever it opens, wt
// shows on the nested client, which the sidebar follows.
func (m Model) pick(w *wt.Worktree) (Model, tea.Cmd) {
	return m.dialog(tmux.Popup{Title: " picker ", Dir: w.Path, Command: []string{"pick"}})
}

// dialog runs p, a dialog whose Command is a wtui subcommand and its
// arguments, in a popup on the nested client, which covers the right side,
// in the background: focus goes right, where the popup takes every key typed
// until it closes; then focus comes back to the sidebar. The sidebar works
// meanwhile (e.g. a click on it). Then the status line says what the dialog
// said, if anything, and afterAction follows.
func (m Model) dialog(p tmux.Popup) (Model, tea.Cmd) { return m.dialogIn(m.c.Normal.Popup, p) }

// dialogIn is dialog, in the popup open opens, e.g. the host's while the
// right side has no nested client.
func (m Model) dialogIn(open func(tmux.Popup) error, p tmux.Popup) (Model, tea.Cmd) {
	p.Command = append([]string{m.c.Wtui}, p.Command...)
	host := m.c.Host
	return m, func() tea.Msg {
		if err := host.FocusRight(); err != nil {
			return dialogClosed{err: err}
		}
		said, err := dialog.Open(func(env []string) error {
			p.Env = append(p.Env, env...)
			return open(p)
		})
		if focus := host.FocusSidebar(); err == nil {
			err = focus
		}
		return dialogClosed{said, err}
	}
}

// onNormal is command, a dialog's wtui subcommand and its arguments, for a
// dialog that talks to the normal server itself: it reaches the server by
// its socket (wtui -S), once that's known.
func (m Model) onNormal(command ...string) []string {
	if m.c.Socket == "" {
		return command
	}
	return append([]string{"-S", m.c.Socket}, command...)
}

// dialogClosed is a dialog's popup closed: what the dialog said, and why
// it failed to open, if it did.
type dialogClosed struct {
	said string
	err  error
}

// withClosed is the sidebar once a dialog has closed, as d says: the status
// line says what the dialog said, and afterAction follows. The dialog may
// have shown another session on the right (e.g. the picker's), and focus is
// back on the sidebar, but tmux's word on either may come in any order. So
// the sidebar reads the active session again, and its cursor goes there.
func (m Model) withClosed(d dialogClosed) (Model, tea.Cmd) {
	m.status.action = d.text()
	m.seek = true
	m, refresh := m.afterAction()
	return m, tea.Batch(refresh, m.readActive)
}

// text is what the status line says once the dialog has closed.
func (d dialogClosed) text() string { return saidText(d.said, d.err) }

// shortcuts shows every shortcut in a popup over the whole host, sized to
// fit them: the page dialog. Focus stays where it is.
func (m Model) shortcuts() (Model, tea.Cmd) {
	host := m.c.Host
	w, h := dialog.PageSize(shortcutLines(m.key))
	p := tmux.Popup{Title: " shortcuts ", Command: []string{m.c.Wtui, "shortcuts", m.key}}
	return m, askHost(func() error { return host.Popup(p, w, h) })
}

// shortcutLines is Shortcuts, for ?'s own action, which can't name it
// directly: Shortcuts lists the actions, so the actions would be
// initialized in terms of themselves.
var shortcutLines func(key string) []string

func init() { shortcutLines = Shortcuts }

// shortcutsWidth is how wide the shortcuts' lines are, at most, and
// keyColumn where what a key does starts on its line.
const (
	shortcutsWidth = 70
	keyColumn      = 14
)

// Shortcuts are the lines that list every shortcut, where key is the wtui
// key: every sidebar key, as the sidebar's own list has them, which work
// alone in the sidebar and after the wtui key in a session, and the keys of
// a session and the mouse.
func Shortcuts(key string) []string {
	lines := wrap("In the sidebar, press a key alone. In a session, press "+key+
		" first, then the key: it acts on the active session.", shortcutsWidth)
	lines = append(lines, "")
	lines = append(lines, keyLines(key, "switch sides, showing a hidden sidebar")...)
	for _, a := range actions {
		lines = append(lines, keyLines(keyLabel(a.keys), a.help)...)
	}
	lines = append(lines, "")
	lines = append(lines, wrap("In a session, tmux's keys work as ever: C-b d closes wtui.", shortcutsWidth)...)
	return append(lines, wrap("Mouse: click a side to focus it, or a row to move the cursor there; "+
		"drag the border to resize the sidebar.", shortcutsWidth)...)
}

// keyLines are label's lines in the shortcuts: label, then help, wrapped
// under the column it starts at.
func keyLines(label, help string) []string {
	lines := wrap(help, shortcutsWidth-keyColumn)
	for i := range lines {
		lead := strings.Repeat(" ", keyColumn)
		if i == 0 {
			lead = label + strings.Repeat(" ", max(1, keyColumn-ansi.StringWidth(label)))
		}
		lines[i] = lead + lines[i]
	}
	return lines
}

// keyLabel is how the shortcuts name keys, Bubble Tea's names for them:
// as tmux names a key with ctrl (C-b), and 1 to 9 as 1-9.
func keyLabel(keys []string) string {
	if slices.Equal(keys, []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}) {
		return "1-9"
	}
	var names []string
	for _, k := range keys {
		if c, ok := strings.CutPrefix(k, "ctrl+"); ok {
			k = "C-" + c
		}
		names = append(names, k)
	}
	return strings.Join(names, " ")
}

// wrap is text in lines width columns wide at most.
func wrap(text string, width int) []string { return strings.Split(ansi.Wrap(text, width, ""), "\n") }
