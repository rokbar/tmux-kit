// Package tmux is wtui's client for the normal tmux server, the one the
// user's sessions are on: the only code that knows tmux's command line and
// control mode for it. Each command runs with TMUX unset, so it reaches the
// normal server even from a host's pane, where TMUX names the host.
package tmux

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MinVersion is the oldest tmux wtui runs on: the first with the
// no-detach-on-destroy client flag, which the nested client attaches with.
// What else the host uses is older: popups on a given client and format
// subscriptions (3.2), the window-resized hook (3.3).
const MinVersion = "3.6"

// DefaultKey is the wtui key while the normal server's @wtui-key option
// isn't set.
const DefaultKey = `C-\`

// DefaultWidth is the sidebar's width in columns while the normal server's
// @wtui-width option isn't set.
const DefaultWidth = 30

// Client is wtui's client for the normal server.
type Client struct {
	// Nested is the tty of the nested client, the one a host shows on its
	// right side; empty outside a host.
	Nested string
}

// Snapshot is the normal server's sessions and their windows, and its
// panes' agent states, as one moment saw them.
type Snapshot struct {
	Sessions []Session // by name
	Agents   []Agent
}

// Session is a session on the normal server.
type Session struct {
	ID, Name string
	// Path is the session's start folder (session_path).
	Path string
	// Activity is when the session was last used (session_activity), to the
	// second.
	Activity time.Time
	// Attached is how many clients are attached to it.
	Attached int
	Windows  []Window // by index
}

// Window is a session's window.
type Window struct {
	ID    string
	Index int
	Name  string
	// Active is whether it's its session's current window.
	Active bool
}

// Version is the normal server's tmux version (e.g. "3.6a"), or, with no
// server running, that of the tmux on PATH: the one that would start it.
func (c *Client) Version() (string, error) {
	if v, err := c.output("display-message", "-p", "#{version}"); err == nil && v != "" {
		return v, nil
	}
	v, err := c.output("-V")
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(v, "tmux "), nil
}

// tmuxVersion is how tmux names its versions: 3.5a, next-3.7 (a build
// toward 3.7), 3.6-rc.
var tmuxVersion = regexp.MustCompile(`^(?:next-)?(\d+)\.(\d+)([a-z]?)`)

// Older says whether tmux version v is older than min, both named as tmux
// names them. A version it can't read (master, openbsd-7.6) isn't: such
// builds follow tmux's development.
func Older(v, min string) bool {
	a, ok := parseVersion(v)
	b, okMin := parseVersion(min)
	if !ok || !okMin {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// parseVersion is v as major, minor and patch numbers (a = 1, b = 2, ...).
func parseVersion(v string) (n [3]int, ok bool) {
	m := tmuxVersion.FindStringSubmatch(v)
	if m == nil {
		return n, false
	}
	n[0], _ = strconv.Atoi(m[1])
	n[1], _ = strconv.Atoi(m[2])
	if m[3] != "" {
		n[2] = int(m[3][0]-'a') + 1
	}
	return n, true
}

// Key is the wtui key, as tmux names keys: the normal server's @wtui-key
// option, or DefaultKey while it isn't set (or there's no server).
func (c *Client) Key() string {
	if key, err := c.output("show-options", "-gqv", "@wtui-key"); err == nil && key != "" {
		return key
	}
	return DefaultKey
}

// Width is the sidebar's width in columns: the normal server's @wtui-width
// option, or DefaultWidth while it isn't set to a number of columns (or
// there's no server).
func (c *Client) Width() int {
	v, err := c.output("show-options", "-gqv", "@wtui-width")
	if n, convErr := strconv.Atoi(v); err == nil && convErr == nil && n > 0 {
		return n
	}
	return DefaultWidth
}

// Editor is the editor's command line, as the normal server's global
// TMUX_KIT_EDITOR has it (the one tmux-kit's prefix e runs), or "" while
// that isn't set.
func (c *Client) Editor() (string, error) {
	// The whole environment, since asking for one variable that isn't set
	// fails as any other failure would.
	out, err := c.output("show-environment", "-g")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if editor, ok := strings.CutPrefix(line, "TMUX_KIT_EDITOR="); ok {
			return editor, nil
		}
	}
	return "", nil
}

// Snapshot is the normal server's sessions, windows and agent states now,
// from one tmux command. With no server running, there are none.
func (c *Client) Snapshot() (Snapshot, error) {
	// Tab-separated, free text last: tmux escapes tabs in names, and output
	// keeps them (and UTF-8) since output runs tmux with -u.
	out, err := c.output(
		"list-sessions", "-F", "S\t#{session_id}\t#{session_activity}\t#{session_attached}\t#{session_name}\t#{session_path}", ";",
		"list-windows", "-a", "-F", "W\t"+windowFormat, ";",
		"display-message", "-p", "A\t"+agentFormat)
	if noServer(err) {
		return Snapshot{}, nil
	}
	if err != nil {
		return Snapshot{}, err
	}
	var snap Snapshot
	at := map[string]int{} // a session's place in snap.Sessions, by id
	for _, line := range strings.Split(out, "\n") {
		kind, rest, _ := strings.Cut(line, "\t")
		switch kind {
		case "S":
			f := strings.SplitN(rest, "\t", 5)
			if len(f) < 5 {
				continue
			}
			secs, _ := strconv.ParseInt(f[1], 10, 64)
			attached, _ := strconv.Atoi(f[2])
			at[f[0]] = len(snap.Sessions)
			snap.Sessions = append(snap.Sessions, Session{
				ID: f[0], Name: f[3], Path: f[4], Activity: time.Unix(secs, 0), Attached: attached,
			})
		case "W":
			session, w, ok := parseWindow(rest)
			if i, known := at[session]; ok && known {
				snap.Sessions[i].Windows = append(snap.Sessions[i].Windows, w)
			}
		case "A":
			snap.Agents = parseAgents(rest)
		}
	}
	return snap, nil
}

// windowFormat is a window as list-windows -a lists it for parseWindow:
// tab-separated, its name last.
const windowFormat = "#{session_id}\t#{window_id}\t#{window_index}\t#{window_active}\t#{window_name}"

// parseWindow is a line of windowFormat: the window, and its session's id.
func parseWindow(line string) (session string, w Window, ok bool) {
	f := strings.SplitN(line, "\t", 5)
	if len(f) < 5 {
		return "", Window{}, false
	}
	index, _ := strconv.Atoi(f[2])
	return f[0], Window{ID: f[1], Index: index, Name: f[4], Active: f[3] == "1"}, true
}

// ActiveSession is the id of the session the nested client shows, or ""
// while it shows none (or there's no server).
func (c *Client) ActiveSession() (string, error) {
	out, err := c.output("list-clients", "-F", "#{client_tty} #{session_id}")
	if noServer(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if id, ok := strings.CutPrefix(line, c.Nested+" "); ok && c.Nested != "" {
			return id, nil
		}
	}
	return "", nil
}

// Show shows session (an id) on the nested client, at its window window (an
// id) unless that's "".
func (c *Client) Show(session, window string) error {
	if c.Nested == "" {
		return errors.New("no nested client to show a session on")
	}
	target := session
	if window != "" {
		target += ":" + window
	}
	_, err := c.output("switch-client", "-c", c.Nested, "-t", target)
	return err
}

// Attach runs the nested client on this process's terminal: a tmux client
// attached to session (an id or =<name>, or "" to have tmux choose), and
// returns once it exits, with what it said as it did. detached is whether
// the user detached it (prefix d), rather than its running out of sessions
// or failing.
//
// It attaches with the no-detach-on-destroy flag, so when its session ends,
// tmux moves it to the session with the most recent activity, whatever the
// user's detach-on-destroy says; and with the sync feature, so its terminal
// (the host's pane) gets synchronized updates.
func (c *Client) Attach(session string) (detached bool, said string) {
	args := []string{"-T", "sync", "attach-session", "-f", "no-detach-on-destroy"}
	if session != "" {
		args = append(args, "-t", session)
	}
	cmd := c.command(args...)
	// The client draws on its terminal, stdin; only its exit message goes to
	// stdout.
	var out bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, &out, &out
	cmd.Run() // its exit status says no more than its message
	said = strings.TrimSpace(out.String())
	return strings.HasPrefix(said, "[detached"), said
}

// output is what tmux printed for args, trimmed. When tmux fails, the error
// is what it said on stderr. tmux runs with -u, so its output keeps tabs
// and UTF-8 as they are: without a UTF-8 locale, it turns them into _.
func (c *Client) output(args ...string) (string, error) {
	cmd := c.command(append([]string{"-u"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if msg := strings.TrimSpace(stderr.String()); err != nil && msg != "" {
		err = errors.New(msg)
	}
	return strings.TrimSpace(string(out)), err
}

// noServer says whether err is tmux finding no server to talk to: no socket,
// or one a server has left behind.
func noServer(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.HasPrefix(msg, "no server running on ") ||
		strings.HasPrefix(msg, "error connecting to ") && strings.HasSuffix(msg, "(No such file or directory)")
}

func (c *Client) command(args ...string) *exec.Cmd {
	cmd := exec.Command("tmux", args...)
	cmd.Env = WithoutTmux(os.Environ())
	return cmd
}

// WithoutTmux is env without TMUX and TMUX_PANE, the variables that tie a
// program to the tmux server it runs in.
func WithoutTmux(env []string) []string {
	var out []string
	for _, kv := range env {
		if !strings.HasPrefix(kv, "TMUX=") && !strings.HasPrefix(kv, "TMUX_PANE=") {
			out = append(out, kv)
		}
	}
	return out
}
