// Package tmux is wtui's client for the normal tmux server, the one the
// user's sessions are on: the only code that knows tmux's command line for
// it. Each command runs with TMUX unset, so it reaches the normal server even
// from a host's pane, where TMUX names the host.
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

// Client is wtui's client for the normal server.
type Client struct {
	// Nested is the tty of the nested client, the one a host shows on its
	// right side; empty outside a host.
	Nested string
}

// Session is a session on the normal server.
type Session struct {
	ID, Name string
	// Activity is when the session was last used (session_activity), to the
	// second.
	Activity time.Time
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

// Sessions are the normal server's sessions, by name.
func (c *Client) Sessions() ([]Session, error) {
	// space-separated: tmux turns a tab into _ without a UTF-8 locale. Ids
	// ($1) and times have no spaces; names may.
	out, err := c.output("list-sessions", "-F", "#{session_id} #{session_activity} #{session_name}")
	if err != nil {
		return nil, err
	}
	var sessions []Session
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, " ", 3)
		if len(f) < 3 {
			continue
		}
		secs, _ := strconv.ParseInt(f[1], 10, 64)
		sessions = append(sessions, Session{ID: f[0], Name: f[2], Activity: time.Unix(secs, 0)})
	}
	return sessions, nil
}

// ActiveSession is the name of the session the nested client shows, or ""
// while it shows none.
func (c *Client) ActiveSession() (string, error) {
	out, err := c.output("list-clients", "-F", "#{client_tty} #{session_name}")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if name, ok := strings.CutPrefix(line, c.Nested+" "); ok && c.Nested != "" {
			return name, nil
		}
	}
	return "", nil
}

// Attach runs the nested client on this process's terminal: a tmux client
// attached to session (an id, or "" to have tmux choose), and returns once it
// exits, with what it said as it did. detached is whether the user detached
// it (prefix d), rather than its running out of sessions or failing.
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
// is what it said on stderr.
func (c *Client) output(args ...string) (string, error) {
	cmd := c.command(args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if msg := strings.TrimSpace(stderr.String()); err != nil && msg != "" {
		err = errors.New(msg)
	}
	return strings.TrimSpace(string(out)), err
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
