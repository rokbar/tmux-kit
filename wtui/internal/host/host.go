// Package host runs wtui's host (ADR 0002): a private tmux server for one
// launch, whose one window holds the sidebar on the left and, on the right, a
// nested tmux client attached to the normal server. It's the only code that
// knows the host server's command line: the launcher, the right side's
// wrapper, what the sidebar asks of the host, and the keys the host has the
// sidebar press.
package host

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// config is the host server's whole config, written for each launch. The
// host has no prefix and binds nothing but the wtui key and the sidebar's
// held keys (bound once the panes are known, see Sidebar), and two mouse
// bindings, so every other key and mouse event reaches the side that has
// focus as it is (the held keys too, on the right side): a click focuses a
// side, dragging the border resizes the sidebar, and the wheel reaches the
// nested client's own mouse mode. set-clipboard passes the nested client's
// OSC 52 copies on to the terminal. The terminal's title is the right pane's,
// which the nested client sets (the session shown, by default), whichever
// side has focus. Focus events reach the panes, so the sidebar knows when it
// gets focus. The server exits once the terminal, its only client, goes.
const config = `set -g prefix None
set -g prefix2 None
set -g status off
set -g mouse on
set -g focus-events on
set -g set-titles on
set -g set-titles-string '#{P:#{?pane_at_right,#{pane_title},}}'
set -s escape-time 0
set -s set-clipboard on
set -s exit-unattached on
unbind -a -T root
bind -n MouseDown1Pane select-pane -t = \; send -M
bind -n MouseDrag1Border resize-pane -M
`

// narrow is the terminal's width, in columns, under which the sidebar hides.
const narrow = 100

// The host holds the sidebar's width and hides it in a narrow terminal with
// two hooks on its window, which keeps three options: @wtui-width, the
// sidebar's width for this launch; @wtui-cols, the window's width when that
// width was last held, 0 while the sidebar is hidden so that it's held again
// once it shows; and @wtui-hidden, set while z hides it. Hiding the sidebar
// is zooming the right side, and anything that focuses the sidebar unzooms
// it. In the commands, SIDEBAR and RIGHT are the panes, and NARROW is narrow.
//
// Commands a hook runs fire no hooks themselves: the hold's own resize isn't
// taken for a drag, and the window-resized hook holds the width itself once
// it has shown the sidebar.
const (
	// zoom hides the sidebar.
	zoom = `resize-pane -Z -t RIGHT
		set -w -t SIDEBAR @wtui-cols 0`
	// hold makes the sidebar @wtui-width wide, or as wide as fits, if it
	// shows and the window's width has changed since it last held.
	hold = `if -F '#{&&:#{!:#{window_zoomed_flag}},#{e|!=:#{window_width},#{@wtui-cols}}}' {
		set -wF -t SIDEBAR @wtui-cols '#{window_width}'
		run-shell -C 'resize-pane -t SIDEBAR -x #{@wtui-width}'
	}`
	// layoutChanged is the window-layout-changed hook. With the sidebar
	// showing and the window as wide as when it last held, the layout
	// changed by a drag on the border, which sets a new width; otherwise it
	// holds the width.
	layoutChanged = `if -F '#{window_zoomed_flag}' {
		set -w -t SIDEBAR @wtui-cols 0
	} {
		set -wqu -t SIDEBAR @wtui-hidden
		if -F '#{e|==:#{window_width},#{@wtui-cols}}' {
			set -wF -t SIDEBAR @wtui-width '#{pane_width}'
		}
		HOLD
	}`
	// resized is the window-resized hook: under narrow columns the sidebar
	// hides, and at narrow or more it shows again, unless z hid it.
	resized = `if -F '#{e|<:#{window_width},NARROW}' {
		if -F '#{!:#{window_zoomed_flag}}' { ZOOM }
	} {
		if -F '#{&&:#{window_zoomed_flag},#{!:#{@wtui-hidden}}}' { resize-pane -Z -t RIGHT }
	}
	HOLD`
	// hide is z's: the sidebar hides until it gets focus.
	hide = `ZOOM
		set -w -t SIDEBAR @wtui-hidden 1`
)

// commands is one of the commands above for this host.
func (h *Host) commands(c string) string {
	c = strings.NewReplacer("HOLD", hold, "ZOOM", zoom).Replace(c)
	return strings.NewReplacer("SIDEBAR", h.sidebar, "RIGHT", h.right, "NARROW", strconv.Itoa(narrow)).Replace(c)
}

// Key is a sidebar key, for the host to bind (see Sidebar).
type Key struct {
	// Tmux is the key as tmux names it; Name as the sidebar does, which
	// Press passes on.
	Tmux, Name string
	// Held is whether focus can go to the right side after it: the host has
	// the sidebar press it, and holds the keys typed after it until it's
	// done, so that they land on whichever side it leaves focus.
	Held bool
	// Repeats is whether it repeats after the wtui key without it, for as
	// long as tmux's repeat-time.
	Repeats bool
}

// ErrInTmux is the launcher's refusal inside tmux.
var ErrInTmux = errors.New("already in tmux, where prefix s and prefix W switch sessions: run wtui in a plain terminal")

// Host is one launch's host server. Its socket and generated config are in a
// folder of their own, which the host removes when it ends (Close, or a
// hangup once the terminal has gone).
type Host struct {
	dir string
	// the sidebar's and the right side's panes, once known
	sidebar, right string
	// presses is where the sidebar takes the held keys the host has it
	// press (see Serve)
	presses net.Listener
}

func (h *Host) socket() string      { return filepath.Join(h.dir, "socket") }
func (h *Host) configFile() string  { return filepath.Join(h.dir, "host.conf") }
func (h *Host) pressSocket() string { return filepath.Join(h.dir, "keys") }

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
// launch and attaches the terminal to it, the sidebar on the left, as wide as
// the normal server's @wtui-width says, and on the right the session
// Starting picks for the folder it's run in; with no session (or no server),
// a new one for that folder, which t starts. exe is wtui, which the host's
// panes run, with t next to it. Once the server is up, Launch pins normal to
// it, and the panes reach it by its socket too. Launch returns once the
// terminal has left the host. It refuses inside tmux (ErrInTmux), and on a
// tmux older than tmux.MinVersion.
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
	session, err := launchSession(exe, normal)
	if err != nil {
		return err
	}
	// The server is up now, so the host's panes are given its socket: they
	// reach it by that alone (wtui -S), never another server.
	if err := normal.Pin(); err != nil {
		return fmt.Errorf("can't tell tmux's socket: %w", err)
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
	// The right side starts first; the sidebar splits off to its left and
	// takes focus. The commands are words, run without a shell.
	args := []string{"-f", h.configFile(), "-T", "sync", "new-session", "-s", "wtui", "-n", "wtui", exe, "-S", normal.Socket, "right", session}
	args = append(args, ";", "split-window", "-hb", "-l", strconv.Itoa(normal.Width()), exe, "-S", normal.Socket, "sidebar")
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
	snap, _ := normal.Snapshot()
	if s := latest(snap.Sessions); s != nil {
		return s.ID
	}
	return ""
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
// the right side's pane and binds the wtui key, key, and the sidebar's keys,
// keys (see bindKeys). It holds the sidebar at width columns until a drag
// on the border sets another width, and hides it while the terminal is
// narrow, from now on. If the host hangs up (the terminal has gone, so the
// host server has exited), it removes the host's folder and exits the
// process.
func Sidebar(key string, width int, keys []Key) (*Host, error) {
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

	if h.presses, err = net.Listen("unix", h.pressSocket()); err != nil {
		return nil, err
	}
	if err := h.bindKeys(key, keys); err != nil {
		return nil, fmt.Errorf("can't bind the wtui key %s and the sidebar's keys: %w", key, err)
	}
	// Running the window-resized hook once fits the sidebar to the terminal
	// as it is at launch.
	err = h.run("set-option", "-w", "-t", h.sidebar, "@wtui-width", strconv.Itoa(width), ";",
		"set-option", "-w", "-t", h.sidebar, "@wtui-cols", "0", ";",
		"set-hook", "-g", "window-layout-changed", h.commands(layoutChanged), ";",
		"set-hook", "-g", "window-resized", h.commands(resized), ";",
		"set-hook", "-R", "-t", h.sidebar, "window-resized")
	if err != nil {
		return nil, fmt.Errorf("can't size the sidebar: %w", err)
	}
	return h, nil
}

// bindKeys binds the wtui key, key, in the host's root table: from the right
// side it focuses the sidebar (showing it, if hidden) and has the next key
// pressed there, as the wtui table binds it; from the sidebar, it focuses
// the right side. The wtui table binds every sidebar key, keys, since a key
// a table doesn't bind is lost: a held key the host has the sidebar press
// (see Press), and any other it passes on to the sidebar, where it acts on
// the active session (getting focus puts the cursor there). A held key is
// bound in the root table too, so that while the sidebar has focus, it's the
// host that has the sidebar press it; elsewhere it passes on as it is.
func (h *Host) bindKeys(key string, keys []Key) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	onSidebar := "#{==:#{pane_id}," + h.sidebar + "}"
	args := []string{"bind-key", "-n", key, "if-shell", "-F", onSidebar,
		"select-pane -t " + h.right, "select-pane -t " + h.sidebar + " ; switch-client -T wtui"}
	for _, k := range keys {
		args = append(args, ";", "bind-key")
		if k.Repeats {
			args = append(args, "-r")
		}
		args = append(args, "-T", "wtui", k.Tmux)
		if !k.Held {
			// with no key, send-keys sends the one pressed
			args = append(args, "send-keys", "-t", h.sidebar)
			continue
		}
		// Words for sh; run-shell expands formats, so # is ##. A command
		// that fails would show in the pane, so none does.
		press := strings.ReplaceAll(shellQuote(exe)+" press "+shellQuote(k.Name)+" >/dev/null 2>&1 || true", "#", "##")
		args = append(args, "run-shell", press,
			";", "bind-key", "-n", k.Tmux, "if-shell", "-F", onSidebar, "run-shell "+tmuxQuote(press), "send-keys")
	}
	return h.run(args...)
}

// shellQuote is s as one word for sh.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// tmuxQuote is s as one word of a tmux command.
func tmuxQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`).Replace(s) + `"`
}

// pressTimeout is how long, at most, the host holds the keys typed after a
// held key, should the sidebar not say it's done with it.
const pressTimeout = 5 * time.Second

// Press is what the host runs for a held key (see Key): it has the sidebar
// press the key named name (see Serve), and returns once the sidebar has
// done all the key does, or after pressTimeout. Until then, the host holds
// the keys typed after it.
func Press(name string) error {
	h, err := here()
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("unix", h.pressSocket(), pressTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(pressTimeout))
	if _, err := fmt.Fprintln(conn, name); err != nil {
		return err
	}
	_, err = io.Copy(io.Discard, conn) // until the sidebar hangs up: it's done
	return err
}

// Serve has the sidebar press the held keys the host sends it (see Press):
// it calls press with each key's Name, and press returns once the key has
// done all it does. It returns once the host has ended.
func (h *Host) Serve(press func(name string)) {
	for {
		conn, err := h.presses.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			conn.SetReadDeadline(time.Now().Add(pressTimeout))
			name, err := bufio.NewReader(conn).ReadString('\n')
			if err == nil {
				press(strings.TrimSuffix(name, "\n"))
			}
		}()
	}
}

// Hidden says whether the sidebar is hidden.
func (h *Host) Hidden() (bool, error) {
	out, err := h.tmux("display-message", "-p", "-t", h.right, "#{window_zoomed_flag}").Output()
	return strings.TrimSpace(string(out)) == "1", err
}

// Hide hides the sidebar, giving the right side, which gets focus, the whole
// window, until the wtui key focuses the sidebar again. It stays hidden as
// the terminal widens.
func (h *Host) Hide() error {
	// unless it's hidden already: zooming again would show it
	return h.run("if-shell", "-F", "-t", h.right, "#{!:#{window_zoomed_flag}}", h.commands(hide))
}

// RightTTY is the tty of the right side's pane: the nested client's tty on
// the normal server.
func (h *Host) RightTTY() (string, error) {
	out, err := h.tmux("display-message", "-p", "-t", h.right, "#{pane_tty}").Output()
	return strings.TrimSpace(string(out)), err
}

// FocusRight moves focus to the right side.
func (h *Host) FocusRight() error { return h.run("select-pane", "-t", h.right) }

// FocusSidebar moves focus to the sidebar, which shows it if it's hidden.
func (h *Host) FocusSidebar() error {
	if h.sidebar == "" { // the right side's wrapper knows only its own pane
		out, err := h.tmux("list-panes", "-F", "#{pane_id}").Output()
		if err != nil {
			return fmt.Errorf("can't list the host's panes: %w", err)
		}
		for _, id := range strings.Fields(string(out)) {
			if id != h.right {
				h.sidebar = id
			}
		}
		if h.sidebar == "" {
			return errors.New("the host has no sidebar")
		}
	}
	return h.run("select-pane", "-t", h.sidebar)
}

// ThenFocusSidebar is command (words, run without a shell), then a tmux
// command that focuses the sidebar: for a program the normal server runs,
// e.g. a git UI floating over the right side, so that focus comes back once
// it quits. It names the host by its socket, so once the host has gone, it
// does nothing.
func (h *Host) ThenFocusSidebar(command []string) []string {
	bin, err := exec.LookPath("tmux") // the normal server's PATH may differ
	if err != nil {
		bin = "tmux"
	}
	focus := shellQuote(bin) + " -S " + shellQuote(h.socket()) + " select-pane -t " + shellQuote(h.sidebar) + " >/dev/null 2>&1"
	return append([]string{"/bin/sh", "-c", `"$@"; ` + focus, "sh"}, command...)
}

// PrefixRight moves focus to the right side and passes tmux's prefix on to
// the nested client there, so the key typed next completes it.
func (h *Host) PrefixRight() error {
	return h.run("select-pane", "-t", h.right, ";", "send-keys", "-t", h.right, "C-b")
}

// Popup opens p in a popup over the whole host, centred, width by height
// cells, or as much of that as fits in the terminal, and returns once its
// program has exited, whatever its exit status. Focus stays where it was.
func (h *Host) Popup(p tmux.Popup, width, height int) error {
	out, err := h.tmux("display-message", "-p", "#{client_width} #{client_height}").Output()
	if err != nil {
		return fmt.Errorf("can't tell the terminal's size: %w", err)
	}
	var cols, rows int
	if _, err := fmt.Sscan(string(out), &cols, &rows); err != nil {
		return fmt.Errorf("can't tell the terminal's size from %q", out)
	}
	args := []string{"display-popup", "-E", "-w", strconv.Itoa(min(width, cols)), "-h", strconv.Itoa(min(height, rows)), "-T", p.Title}
	if p.Dir != "" {
		args = append(args, "-d", p.Dir)
	}
	for _, kv := range p.Env {
		args = append(args, "-e", kv)
	}
	cmd := h.tmux(append(append(args, "--"), p.Command...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	// display-popup exits with its program's status, which isn't tmux
	// failing: then it says nothing.
	if err := cmd.Run(); err != nil && strings.TrimSpace(stderr.String()) != "" {
		return errors.New(strings.TrimSpace(stderr.String()))
	}
	return nil
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
	os.Remove(h.pressSocket())
	os.Remove(h.dir)
}

// Right is the right side's wrapper. It runs the nested client, attached to
// session (an id or =name; "" for the most recent), in this process's pane.
// Once the user detaches it (prefix d), the host closes. If it ends
// otherwise (the last session has ended, or the server has), or can't
// attach, the pane says there's no session and the sidebar takes focus; as
// soon as there's a session again, the nested client attaches to the most
// recent.
func Right(normal *tmux.Client, session string) error {
	h, err := here()
	if err != nil {
		return err
	}
	h.right = os.Getenv("TMUX_PANE")
	for {
		fmt.Print("\033[H\033[2J")
		detached, said := normal.Attach(session)
		if detached {
			return h.Close()
		}
		fmt.Printf("%s\r\n\r\nNo session to show.\r\np in the sidebar opens a project; a session made anywhere shows here.\r\n", said)
		h.FocusSidebar() // the right side has nothing to type into
		session = ""
		for session == "" {
			time.Sleep(lookEvery)
			session = mostRecent(normal)
		}
	}
}

// lookEvery is how often the right side looks for a session while it has
// none to show.
const lookEvery = 500 * time.Millisecond
