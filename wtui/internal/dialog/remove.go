package dialog

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// Remover removes worktrees: a *wt.Client, or a fake in tests. It returns
// what wt said, or wt's failure, a safety refusal being of the kind
// wt.ErrSafetyRefusal.
type Remover interface {
	Remove(dir, path string, force bool) (string, error)
}

// Removal is a worktree to remove, and what ends with it, for the remove
// dialog to ask about.
type Removal struct {
	// Path is the worktree's folder.
	Path string `json:"path"`
	// Missing is whether the folder is gone already, so that removing the
	// worktree only unregisters it.
	Missing bool `json:"missing,omitempty"`
	// Sessions are those that end with it: every session started in its
	// folder or a folder inside it, as wt ends them.
	Sessions []Ending `json:"sessions,omitempty"`
}

// Ending is a session that ends, by name, and the agents in it.
type Ending struct {
	Session string        `json:"session"`
	Agents  []EndingAgent `json:"agents,omitempty"`
}

// EndingAgent is an agent that ends with its session: the name of its
// window, and its agent state as the sidebar says it (working, needs you,
// done).
type EndingAgent struct {
	Window string `json:"window"`
	State  string `json:"state"`
}

// Arg is r as the remove dialog's argument, for ParseRemoval.
func (r Removal) Arg() string {
	b, _ := json.Marshal(r) // only strings, a bool and slices of them: it can't fail
	return string(b)
}

// ParseRemoval is the Removal that arg, as Arg wrote it, is.
func ParseRemoval(arg string) (Removal, error) {
	var r Removal
	if err := json.Unmarshal([]byte(arg), &r); err != nil || r.Path == "" {
		return Removal{}, fmt.Errorf("not a worktree to remove: %q", arg)
	}
	return r, nil
}

// question is what the dialog asks about removing r: whether to remove the
// worktree, ending its sessions, and what else that does.
func (r Removal) question() []string {
	name := filepath.Base(r.Path)
	var sessions []string
	for _, s := range r.Sessions {
		sessions = append(sessions, s.Session)
	}
	var lines []string
	switch len(sessions) {
	case 0:
		lines = append(lines, "remove "+name+" (no session)?")
	case 1:
		lines = append(lines, "remove "+name+" and end session "+sessions[0]+"?")
	default:
		lines = append(lines, "remove "+name+" and end sessions "+and(sessions)+"?")
	}
	if r.Missing {
		lines = append(lines, "its folder is gone, so removing it only unregisters it")
	}
	for _, s := range r.Sessions {
		var agents []string
		for _, a := range s.Agents {
			agents = append(agents, a.State+" in "+a.Window)
		}
		switch len(agents) {
		case 0:
		case 1:
			lines = append(lines, "the agent in "+s.Session+" ends too: "+agents[0])
		default:
			lines = append(lines, "the agents in "+s.Session+" end too: "+strings.Join(agents, ", "))
		}
	}
	return lines
}

// and is names as a list in a sentence: "a, b and c".
func and(names []string) string {
	last := len(names) - 1
	return strings.Join(names[:last], ", ") + " and " + names[last]
}

// Remove is the remove dialog: it asks whether to remove the worktree r is,
// and on y has wt remove it, running in dir (its main clone). If wt refuses
// for safety, it shows wt's reasons and asks whether to force; any other
// failure it shows until a key closes it. n, q, esc and ctrl+c cancel, but
// not while wt runs. What wt said,
// or its failure, is what the dialog says once it has closed (Said).
func Remove(w Remover, dir string, r Removal) Removing {
	return Removing{wt: w, dir: dir, removal: r}
}

// Removing is the remove dialog (see Remove).
type Removing struct {
	wt      Remover
	dir     string
	removal Removal
	mode    removeMode
	forced  bool   // whether the removal running, or last run, forces it
	refusal string // why wt refused to remove the worktree, once it has
	top     int    // the first line of the refusal on screen, when it doesn't all fit
	failure string // wt's failure, once it has failed otherwise
	said    string
	width   int
	height  int
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
	case removed:
		return d.withRemoved(msg)
	case tea.KeyPressMsg:
		return d.press(msg.String())
	}
	return d, nil
}

// press is the dialog once key is pressed. While wt runs, keys do nothing:
// stopping it halfway would leave the worktree half removed.
func (d Removing) press(key string) (tea.Model, tea.Cmd) {
	switch d.mode {
	case removingMode:
		return d, nil
	case failedMode:
		return d, tea.Quit
	}
	switch key {
	case "y":
		return d.remove(d.mode == refusedMode)
	case "n", "q", "esc", "ctrl+c": // raw mode turns ctrl+c into a key
		return d, tea.Quit
	}
	if d.mode == refusedMode {
		switch key {
		case "up", "k":
			d = d.scroll(-1)
		case "down", "j":
			d = d.scroll(1)
		}
	}
	return d, nil
}

// remove has wt remove the worktree, forcing it if force.
func (d Removing) remove(force bool) (tea.Model, tea.Cmd) {
	d.mode, d.forced = removingMode, force
	w, dir, path := d.wt, d.dir, d.removal.Path
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

// The dialog's last lines, at the bottom of the popup.
var (
	askingKeys  = []string{"y: remove   n/q/esc: cancel"}
	removingNow = []string{"removing…"}
	forceKeys   = []string{"force?", "y: force   n/q/esc: cancel"}
	failedKeys  = []string{closing}
)

func (d Removing) View() tea.View {
	switch d.mode {
	case removingMode:
		return d.prompt(d.question(), removingNow)
	case refusedMode:
		lines, fit := d.reasons()
		if fit < len(lines) {
			top := min(d.top, len(lines)-fit) // the popup may have grown
			lines = append(lines[top:top+fit:top+fit],
				fmt.Sprintf("↑/↓ j/k: scroll (lines %d–%d of %d)", top+1, top+fit, len(lines)))
		}
		return d.prompt(lines, forceKeys)
	case failedMode:
		return d.prompt(d.wrap(d.failure), failedKeys)
	}
	return d.prompt(d.question(), askingKeys)
}

// question is the question's lines, wrapped to the popup.
func (d Removing) question() []string {
	var lines []string
	for _, l := range d.removal.question() {
		lines = append(lines, d.wrap(l)...)
	}
	return lines
}

// reasons is wt's reasons for refusing, wrapped to the popup, and how many
// of their lines fit above a blank line and the offer to force: all of
// them, or, if they don't all fit, one less than that, for a line saying
// which are shown.
func (d Removing) reasons() (lines []string, fit int) {
	lines = d.wrap(d.refusal)
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

// wrap is text's lines, wrapped to the popup less a column either side,
// once its width is known, breaking paths at their slashes rather than
// anywhere.
func (d Removing) wrap(text string) []string {
	if d.width > 2 {
		text = ansi.Wrap(text, d.width-2, "/")
	}
	return strings.Split(text, "\n")
}

// prompt is the dialog's view: lines, then a blank line if there's room,
// then last at the bottom of the popup, its last line dim, each line with a
// column either side.
func (d Removing) prompt(lines, last []string) tea.View {
	if d.height > 0 {
		lines = AtMost(max(0, d.height-len(last)), lines)
	}
	var shown []string
	for _, l := range lines {
		shown = append(shown, " "+OneLine(l, d.width-2))
	}
	for len(shown) < d.height-len(last) || d.height <= 0 && len(shown) < len(lines)+1 {
		shown = append(shown, "")
	}
	for i, l := range last {
		l = OneLine(l, d.width-2)
		if i == len(last)-1 {
			l = "\x1b[2m" + l + "\x1b[0m"
		}
		shown = append(shown, " "+l)
	}
	v := tea.NewView(strings.Join(shown, "\n"))
	v.AltScreen = true
	return v
}
