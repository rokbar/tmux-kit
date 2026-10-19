package tmux

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"
)

// Event is a change on the normal server, as a Watch sees it.
type Event interface{ event() }

// ActiveSessionChanged is the nested client showing another session (an
// id): because it was asked to, because the user switched it (prefix s), or
// because its session ended and tmux moved it.
type ActiveSessionChanged struct{ Session string }

func (ActiveSessionChanged) event() {}

// SessionsChanged is a session created, closed or renamed.
type SessionsChanged struct{}

func (SessionsChanged) event() {}

// WindowsChanged is a session's windows changing: one added, closed or
// renamed, or another one made its current window. Windows are what they are
// now, by index.
type WindowsChanged struct {
	Session string // an id
	Windows []Window
}

func (WindowsChanged) event() {}

// subscription is a format a Watch's control client subscribes to
// (refresh-client -B), and the event each of its values makes. tmux expands
// it for the client's session, and says what it is at once and whenever that
// changes, looking once a second. For anything beyond that session, the
// format walks the server itself (#{S:...}): a %* (every pane) subscription
// covers the client's session only.
type subscription struct {
	name   string // letters only
	format string // without a '
	event  func(value string) Event
}

// subscriptions are what every Watch subscribes to. To watch something more
// of the server's, add one, with an Event type for its values.
var subscriptions = []subscription{
	{"agents", agentFormat, func(value string) Event { return AgentsChanged{parseAgents(value)} }},
}

// Watch is the normal server's control-mode client, which reports changes
// there as events. It can't be without a session, so it attaches to the
// session the nested client shows and follows it from session to session:
// its only trace is a second client attached to the active session. It
// doesn't resize windows or get their output.
type Watch struct {
	c      *Client
	events chan Event
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
	// windows are every session's windows, by session id, as last listed:
	// tmux doesn't say which session a window event is about, so a Watch
	// lists them all again and tells which changed.
	windows map[string][]Window
}

// retry is how often a Watch looks for a session to attach to while the
// nested client shows none (or there's no server).
const retry = 500 * time.Millisecond

// Watch starts watching the normal server, from c's nested client, until
// Close. Once the nested client shows a session, the first event says which.
func (c *Client) Watch() *Watch {
	w := &Watch{c: c, events: make(chan Event, 16), stop: make(chan struct{}), done: make(chan struct{})}
	go w.run()
	return w
}

// Events are the changes as they happen, until Close.
func (w *Watch) Events() <-chan Event { return w.events }

// Close ends the control client, and once it has gone, the events.
func (w *Watch) Close() error {
	w.once.Do(func() { close(w.stop) })
	<-w.done
	return nil
}

func (w *Watch) run() {
	defer close(w.done)
	defer close(w.events)
	again := false // whether a control client has been attached before
	for {
		if session, _ := w.c.ActiveSession(); session != "" && w.attach(session, again) {
			again = true
			// e.g. the last session has gone, or the server has
			w.send(SessionsChanged{})
		}
		select {
		case <-w.stop:
			return
		case <-time.After(retry):
		}
	}
}

// send gives the events' reader ev, unless the Watch is closing.
func (w *Watch) send(ev Event) {
	select {
	case <-w.stop:
	default:
		select {
		case w.events <- ev:
		case <-w.stop:
		}
	}
}

// attach runs a control client on session (an id) until it ends or the
// Watch closes, and says whether it ran. Attached again, it first says the
// sessions changed, since it saw nothing while it wasn't.
func (w *Watch) attach(session string, again bool) bool {
	cmd := w.c.command("-u", "-C", "attach-session", "-t", session, "-f", "ignore-size,no-output,no-detach-on-destroy")
	in, err := cmd.StdinPipe()
	if err != nil {
		return false
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return false
	}
	if cmd.Start() != nil {
		return false
	}
	if again {
		w.send(SessionsChanged{})
	}
	for _, s := range subscriptions {
		fmt.Fprintf(in, "refresh-client -B '%s::%s'\n", s.name, s.format)
	}
	ended := make(chan struct{})
	go func() {
		select {
		case <-w.stop:
		case <-ended:
		}
		in.Close() // the control client ends once its input does
	}()
	w.windows, _ = w.c.allWindows()
	w.send(ActiveSessionChanged{session})
	w.read(out, in, session)
	close(ended)
	cmd.Wait()
	return true
}

// read reads the control client's notifications from out until it ends,
// giving it commands on in. It was attached to session.
func (w *Watch) read(out io.Reader, in io.Writer, session string) {
	follow := func(to string) {
		fmt.Fprintf(in, "switch-client -t '%s'\n", to)
		w.send(ActiveSessionChanged{to})
	}
	attached := false
	lines := bufio.NewScanner(out)
	lines.Buffer(nil, 1<<20)
	for lines.Scan() {
		kind, rest, _ := strings.Cut(lines.Text(), " ")
		switch kind {
		case "%session-changed":
			// The control client's own, first when it has attached: the
			// nested client may have moved since it was looked up, unseen.
			if !attached {
				attached = true
				if now, err := w.c.ActiveSession(); err == nil && now != "" && now != session {
					follow(now)
				}
			}
		case "%client-session-changed":
			// <client> <session id> <session name>; a client is named by
			// its tty
			client, rest, _ := strings.Cut(rest, " ")
			to, _, _ := strings.Cut(rest, " ")
			if client == w.c.Nested && w.c.Nested != "" {
				follow(to)
			}
		case "%sessions-changed", "%session-renamed":
			w.send(SessionsChanged{})
		case "%window-add", "%window-close", "%window-renamed",
			"%unlinked-window-add", "%unlinked-window-close", "%unlinked-window-renamed",
			"%session-window-changed":
			w.windowsChanged()
		case "%subscription-changed":
			// <name> <where the format was expanded> : <value>
			name, _, _ := strings.Cut(rest, " ")
			_, value, _ := strings.Cut(rest, " : ")
			for _, s := range subscriptions {
				if s.name == name {
					w.send(s.event(value))
				}
			}
		}
	}
}

// windowsChanged lists every session's windows again, and tells of each
// session whose windows aren't what they were. A session gone isn't told of:
// that's a SessionsChanged.
func (w *Watch) windowsChanged() {
	now, err := w.c.allWindows()
	if err != nil {
		return
	}
	for session, windows := range now {
		if !slices.Equal(windows, w.windows[session]) {
			w.send(WindowsChanged{session, windows})
		}
	}
	w.windows = now
}

// allWindows are every session's windows, by session id.
func (c *Client) allWindows() (map[string][]Window, error) {
	out, err := c.output("list-windows", "-a", "-F", windowFormat)
	if err != nil {
		return nil, err
	}
	all := map[string][]Window{}
	for _, line := range strings.Split(out, "\n") {
		if session, w, ok := parseWindow(line); ok {
			all[session] = append(all[session], w)
		}
	}
	return all, nil
}
