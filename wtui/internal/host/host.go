// Package host runs wtui's host (ADR 0002): a private tmux server for one
// launch, whose one window holds the sidebar on the left and, on the right, a
// nested tmux client attached to the normal server. It's the only code that
// knows the host server's command line: the launcher, the right side's
// wrapper, and what the sidebar asks of the host.
package host

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// SidebarWidth is the sidebar's width in columns.
const SidebarWidth = 30

// config is the host server's whole config, written for each launch. The
// host has no prefix and binds nothing but the wtui key (bound once the panes
// are known, see Sidebar) and two mouse bindings, so every other key and
// mouse event reaches the side that has focus as it is: a click focuses a
// side, dragging the border resizes the sidebar, and the wheel reaches the
// nested client's own mouse mode. set-clipboard passes the nested client's
// OSC 52 copies on to the terminal. The terminal's title is the right pane's,
// which the nested client sets (the session shown, by default), whichever
// side has focus. The server exits once the terminal, its only client, goes.
const config = `set -g prefix None
set -g prefix2 None
set -g status off
set -g mouse on
set -g set-titles on
set -g set-titles-string '#{P:#{?pane_at_right,#{pane_title},}}'
set -s escape-time 0
set -s set-clipboard on
set -s exit-unattached on
unbind -a -T root
bind -n MouseDown1Pane select-pane -t = \; send -M
bind -n MouseDrag1Border resize-pane -M
`

// ErrInTmux is the launcher's refusal inside tmux.
var ErrInTmux = errors.New("already in tmux, where prefix s and prefix W switch sessions: run wtui in a plain terminal")

// Host is one launch's host server. Its socket and generated config are in a
// folder of their own, which the host removes when it ends (Close, or a
// hangup once the terminal has gone).
type Host struct {
	dir string
	// the sidebar's and the right side's panes, once known
	sidebar, right string
}

func (h *Host) socket() string     { return filepath.Join(h.dir, "socket") }
func (h *Host) configFile() string { return filepath.Join(h.dir, "host.conf") }

// tmux is a tmux command for the host server.
func (h *Host) tmux(args ...string) *exec.Cmd {
	cmd := exec.Command("tmux", append([]string{"-S", h.socket()}, args...)...)
	cmd.Env = tmux.WithoutTmux(os.Environ())
	return cmd
}

func (h *Host) run(args ...string) error {
	out, err := h.tmux(args...).CombinedOutput()
	if msg := strings.TrimSpace(string(out)); err != nil && msg != "" {
		err = errors.New(msg)
	}
	return err
}

// Launch is the launcher, run in a plain terminal: it starts a host for this
// launch and attaches the terminal to it, the sidebar on the left and the
// session with the most recent activity on the right. exe is wtui, which the
// host's panes run. Launch returns once the terminal has left the host. It
// refuses inside tmux (ErrInTmux), and on a tmux older than tmux.MinVersion.
func Launch(exe string, normal *tmux.Client) error {
	if os.Getenv("TMUX") != "" {
		return ErrInTmux
	}
	v, err := normal.Version()
	if err != nil {
		return fmt.Errorf("can't tell tmux's version: %w", err)
	}
	if tmux.Older(v, tmux.MinVersion) {
		return fmt.Errorf("needs tmux %s or newer, and this is tmux %s", tmux.MinVersion, v)
	}

	dir, err := os.MkdirTemp("", "wtui-")
	if err != nil {
		return err
	}
	h := &Host{dir: dir}
	if err := os.WriteFile(h.configFile(), []byte(config), 0o600); err != nil {
		h.cleanup()
		return err
	}
	right := []string{exe, "right"}
	if s := mostRecent(normal); s != "" {
		right = append(right, s)
	}
	// The right side starts first; the sidebar splits off to its left and
	// takes focus. The commands are words, run without a shell.
	args := []string{"-f", h.configFile(), "-T", "sync", "new-session", "-s", "wtui", "-n", "wtui"}
	args = append(append(args, right...), ";", "split-window", "-hb", "-l", strconv.Itoa(SidebarWidth), exe, "sidebar")
	cmd := h.tmux(args...)
	// The terminal's client draws on stdin; only its exit message goes to
	// stdout.
	var out bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, &out, &out
	runErr := cmd.Run()
	said := strings.TrimSpace(out.String())
	if isExitNote(said) {
		return nil // e.g. [server exited] once q has closed the host
	}
	h.cleanup() // the host didn't start, so nothing in it will
	if said == "" {
		return runErr
	}
	return errors.New(said)
}

// isExitNote says whether a tmux client's output is only its one-line note
// on how it ended, e.g. "[exited]" or "[server exited]".
func isExitNote(said string) bool {
	return strings.HasPrefix(said, "[") && strings.HasSuffix(said, "]") && !strings.Contains(said, "\n")
}

// mostRecent is the id of the normal server's session with the most recent
// activity, or "" if it has none (or isn't running). Of sessions used in the
// same second, the first by name.
func mostRecent(normal *tmux.Client) string {
	sessions, _ := normal.Sessions()
	var best *tmux.Session
	for i, s := range sessions {
		if best == nil || s.Activity.After(best.Activity) {
			best = &sessions[i]
		}
	}
	if best == nil {
		return ""
	}
	return best.ID
}

// here is the host this process runs in, from the TMUX its panes get.
func here() (*Host, error) {
	socket, _, _ := strings.Cut(os.Getenv("TMUX"), ",")
	if filepath.Base(socket) != "socket" {
		return nil, errors.New("not in a wtui host")
	}
	return &Host{dir: filepath.Dir(socket)}, nil
}

// Sidebar is the host whose sidebar runs in this process's pane. It learns
// the right side's pane and binds the wtui key, key, in the host's root
// table: from the right it focuses the sidebar, and from the sidebar the
// right. If the host hangs up (the terminal has gone, so the host server
// has exited), it removes the host's folder and exits the process.
func Sidebar(key string) (*Host, error) {
	h, err := here()
	if err != nil {
		return nil, err
	}
	h.sidebar = os.Getenv("TMUX_PANE")
	out, err := h.tmux("list-panes", "-F", "#{pane_id}").Output()
	if err != nil {
		return nil, fmt.Errorf("can't list the host's panes: %w", err)
	}
	for _, id := range strings.Fields(string(out)) {
		if id != h.sidebar {
			h.right = id
		}
	}
	if h.sidebar == "" || h.right == "" {
		return nil, fmt.Errorf("the host has no right side (panes: %s)", strings.Join(strings.Fields(string(out)), " "))
	}

	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		<-hup
		h.cleanup()
		os.Exit(0)
	}()

	err = h.run("bind-key", "-n", key, "if-shell", "-F", "#{==:#{pane_id},"+h.sidebar+"}",
		"select-pane -t "+h.right, "select-pane -t "+h.sidebar)
	if err != nil {
		return nil, fmt.Errorf("can't bind the wtui key %s: %w", key, err)
	}
	return h, nil
}

// RightTTY is the tty of the right side's pane: the nested client's tty on
// the normal server.
func (h *Host) RightTTY() (string, error) {
	out, err := h.tmux("display-message", "-p", "-t", h.right, "#{pane_tty}").Output()
	return strings.TrimSpace(string(out)), err
}

// FocusRight moves focus to the right side.
func (h *Host) FocusRight() error { return h.run("select-pane", "-t", h.right) }

// PrefixRight moves focus to the right side and passes tmux's prefix on to
// the nested client there, so the key typed next completes it.
func (h *Host) PrefixRight() error {
	return h.run("select-pane", "-t", h.right, ";", "send-keys", "-t", h.right, "C-b")
}

// Close ends the host: its server, then its folder. The sessions on the
// normal server live on.
func (h *Host) Close() error {
	// Ending the server hangs up its panes, this process's among them.
	signal.Ignore(syscall.SIGHUP)
	err := h.run("kill-server")
	h.cleanup()
	return err
}

// cleanup removes the host's socket and config, then its folder if that's
// all it held.
func (h *Host) cleanup() {
	os.Remove(h.socket())
	os.Remove(h.configFile())
	os.Remove(h.dir)
}

// Right is the right side's wrapper. It runs the nested client, attached to
// session (an id; "" for the most recent), in this process's pane. Once the
// user detaches it (prefix d), the host closes. If it ends otherwise (no
// session left, or none to attach to), the pane says so, and enter tries the
// most recent session again.
func Right(normal *tmux.Client, session string) error {
	h, err := here()
	if err != nil {
		return err
	}
	in := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("\033[H\033[2J")
		detached, said := normal.Attach(session)
		if detached {
			return h.Close()
		}
		fmt.Printf("%s\r\n\r\nNo session to show. Enter tries again.\r\n", said)
		if _, err := in.ReadString('\n'); err != nil {
			return err // the host has gone
		}
		session = mostRecent(normal)
	}
}
