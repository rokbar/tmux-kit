package dialog

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// Remover removes worktrees: a *wt.Client, or a fake in tests. It returns
// what wt said, or wt's failure, a safety refusal being of the kind
// wt.ErrSafetyRefusal.
type Remover interface {
	Remove(dir, path string, force bool) (string, error)
}

// Remove is the remove dialog of the worktree at path: it asks question,
// its lines (see Ask), and on y has wt remove the worktree, running in dir
// (its main clone). If wt refuses for safety, it shows wt's reasons and asks
// whether to force; any other failure it shows until a key closes it. n, q,
// esc and ctrl+c cancel, but not while wt runs. What wt said, or its
// failure, is what the dialog says once it has closed (Said).
func Remove(w Remover, dir, path string, question []string) Removing {
	return Removing{wt: w, dir: dir, path: path, question: question, ask: Ask(question, "remove")}
}

// Removing is the remove dialog (see Remove).
type Removing struct {
	wt        Remover
	dir, path string
	question  []string
	ask       tea.Model // the question, until it's answered
	mode      removeMode
	forced    bool   // whether the removal running, or last run, forces it
	refusal   string // why wt refused to remove the worktree, once it has
	top       int    // the first line of the refusal on screen, when it doesn't all fit
	failure   string // wt's failure, once it has failed otherwise
	said      string
	width     int
	height    int
}

// removeMode is what the remove dialog shows, and so what keys do.
type removeMode int

const (
	askingMode   removeMode = iota // whether to remove the worktree
	removingMode                   // wt is removing it
	refusedMode                    // wt refused to for safety: why, and whether to force
	failedMode                     // wt failed otherwise: why, until any key
)

// removed is wt having removed the worktree, which it said, or failed to.
type removed struct {
	said string
	err  error
}

// Said is what the dialog says for the status line, once it has closed:
// what wt said it did, or its failure; nothing if it was cancelled.
func (d Removing) Said() string { return d.said }

func (d Removing) Init() tea.Cmd { return nil }

func (d Removing) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width, d.height = msg.Width, msg.Height
		d.ask, _ = d.ask.Update(msg)
	case removed:
		return d.withRemoved(msg)
	case tea.KeyPressMsg:
		return d.press(msg)
	}
	return d, nil
}

// press is the dialog once key is pressed. While wt runs, keys do nothing:
// stopping it halfway would leave the worktree half removed.
func (d Removing) press(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch d.mode {
	case askingMode:
		var answer tea.Cmd
		d.ask, answer = d.ask.Update(key)
		switch {
		case !closes(answer):
			return d, answer
		case Yes(d.ask):
			return d.remove(false)
		}
		return d, tea.Quit
	case removingMode:
		return d, nil
	case failedMode:
		return d, tea.Quit
	}
	switch key.String() {
	case "y":
		return d.remove(true)
	case "n", "q", "esc", "ctrl+c": // raw mode turns ctrl+c into a key
		return d, tea.Quit
	case "up", "k":
		d = d.scroll(-1)
	case "down", "j":
		d = d.scroll(1)
	}
	return d, nil
}

// closes says whether cmd closes a dialog: what Ask does once answered.
func closes(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// remove has wt remove the worktree, forcing it if force.
func (d Removing) remove(force bool) (tea.Model, tea.Cmd) {
	d.mode, d.forced = removingMode, force
	w, dir, path := d.wt, d.dir, d.path
	return d, func() tea.Msg {
		said, err := w.Remove(dir, path, force)
		return removed{said, err}
	}
}

// withRemoved is the dialog once wt has removed the worktree, which closes
// it, or failed to. A safety refusal gets wt's reasons and the offer to
// force; any other failure, wt's message.
func (d Removing) withRemoved(r removed) (tea.Model, tea.Cmd) {
	switch {
	case r.err == nil:
		d.said = r.said
		return d, tea.Quit
	case errors.Is(r.err, wt.ErrSafetyRefusal) && !d.forced:
		d.mode, d.refusal, d.top = refusedMode, r.err.Error(), 0
	default:
		d.mode, d.failure, d.said = failedMode, r.err.Error(), r.err.Error()
	}
	return d, nil
}

// What the dialog shows under what it says, after a blank line, as Ask
// shows its keys.
var (
	removingNow = []string{"removing…"}
	forceKeys   = []string{"force?", "y: force   n/q/esc: cancel"}
	failedKeys  = []string{closing}
)

func (d Removing) View() tea.View {
	switch d.mode {
	case askingMode:
		return d.ask.View()
	case removingMode:
		var lines []string
		for _, l := range d.question {
			lines = append(lines, wrapTo(d.width, l, "")...)
		}
		return prompt(d.width, d.height, lines, removingNow)
	case refusedMode:
		lines, fit := d.reasons()
		if fit < len(lines) {
			top := min(d.top, len(lines)-fit) // the popup may have grown
			lines = append(lines[top:top+fit:top+fit],
				fmt.Sprintf("↑/↓ j/k: scroll (lines %d–%d of %d)", top+1, top+fit, len(lines)))
		}
		return prompt(d.width, d.height, lines, forceKeys)
	}
	return prompt(d.width, d.height, wrapTo(d.width, d.failure, "/"), failedKeys)
}

// reasons is wt's reasons for refusing, wrapped to the popup, and how many
// of their lines fit above a blank line and the offer to force: all of
// them, or, if they don't all fit, one less than that, for a line saying
// which are shown.
func (d Removing) reasons() (lines []string, fit int) {
	lines = wrapTo(d.width, d.refusal, "/")
	room := d.height - len(forceKeys) - 1
	if d.height <= 0 || len(lines) <= room {
		return lines, len(lines)
	}
	return lines, max(room-1, 1)
}

// scroll moves which of wt's reasons for refusing are on screen by lines,
// within them.
func (d Removing) scroll(by int) Removing {
	lines, fit := d.reasons()
	d.top = max(0, min(len(lines)-fit, d.top+by))
	return d
}
