// Command wtui is an optional app over tmux-kit's wt and tmux: run in a plain
// terminal, it opens a host with the sidebar on the left and a session on the
// right.
//
//	wtui              launch: a host on this terminal
//	wtui --version    print the version
//
// The host runs the rest itself; the sidebar and the right side reach the
// normal server by the socket the launcher found it on:
//
//	wtui -S <socket> sidebar           the sidebar, in the host's left pane
//	wtui -S <socket> right [<session>] the right side's wrapper, in its right pane
//	wtui press <key>                   has the sidebar press a held key, for the
//	                                   host's bindings
//	wtui popup                         the old popup, kept until the host replaces it
//
// and the sidebar runs its dialogs in popups:
//
//	wtui details <folder> [<branch> [<dev URL>]]
//	                                   a worktree's details (i)
//	wtui pick                          the picker, in this folder (n)
//	wtui shortcuts <key>               every shortcut, where <key> is the wtui key (?)
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/browser"
	"github.com/rokbar/tmux-kit/wtui/internal/detail"
	"github.com/rokbar/tmux-kit/wtui/internal/editor"
	"github.com/rokbar/tmux-kit/wtui/internal/host"
	gitrepo "github.com/rokbar/tmux-kit/wtui/internal/repo" // the popup has its own repo
	"github.com/rokbar/tmux-kit/wtui/internal/sidebar"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	socket := flag.String("S", "", "the normal server's `socket`, for the host's sidebar and right side")
	flag.Parse()
	if *showVersion {
		fmt.Println("wtui", currentVersion())
		return
	}
	var err error
	switch flag.Arg(0) {
	case "":
		err = launch()
	case "sidebar":
		err = runSidebar(*socket)
	case "right":
		err = host.Right(&tmux.Client{Socket: *socket}, flag.Arg(1))
	case "press":
		// The sidebar's failures are the sidebar's to show, and the host's
		// bindings make sure nothing shows of this.
		err = host.Press(flag.Arg(1))
	case "popup":
		err = runPopup()
	case "details":
		err = runDetails(flag.Arg(1), flag.Arg(2), flag.Arg(3))
	case "pick":
		err = runPick()
	case "shortcuts":
		err = runShortcuts(flag.Arg(1))
	default:
		fmt.Fprintln(os.Stderr, "usage: wtui [--version]")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "wtui:", err)
		os.Exit(1)
	}
}

func launch() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return host.Launch(exe, &tmux.Client{})
}

// runSidebar runs the sidebar in its host until q, then closes the host. It
// reaches the normal server by socket. The normal server's changes reach the
// sidebar as they happen, from a Watch that ends before the host does. (If
// the host hangs up instead, the Watch's control client ends with this
// process.)
func runSidebar(socket string) error {
	exe, _ := os.Executable() // without it, wt is looked for only at $WT_BIN and on PATH
	normal := &tmux.Client{Socket: socket}
	key := normal.Key()
	h, err := host.Sidebar(key, normal.Width(), sidebar.Keys())
	if err != nil {
		return err
	}
	// What the sidebar runs (wt, git) is for the normal server, so it mustn't
	// see the TMUX of the host's pane: wt's tmux would be the host's. The host
	// is reached by its socket.
	os.Unsetenv("TMUX")
	os.Unsetenv("TMUX_PANE")
	if normal.Nested, err = h.RightTTY(); err != nil {
		return errors.Join(err, h.Close())
	}
	watch := normal.Watch()
	c := sidebar.Clients{
		Host: h, Normal: normal, Repos: gitrepo.Lookup{}, Wt: wt.New(exe),
		Editor: editor.Opener{Editor: normal.Editor}, Browser: browser.Opener{}, Wtui: exe,
	}
	var m tea.Model = sidebar.New(c, key)
	// The sidebar starts with focus, as the host gives it, unless it starts
	// hidden (a narrow terminal): tmux moved focus right before the sidebar
	// asked for focus events, so it won't say so.
	if hidden, _ := h.Hidden(); hidden {
		m, _ = m.Update(tea.BlurMsg{})
	}
	p := tea.NewProgram(m)
	go func() {
		for ev := range watch.Events() {
			p.Send(ev)
		}
	}()
	go h.Serve(func(key string) {
		done := make(chan struct{})
		p.Send(sidebar.Press{Key: key, Done: done})
		<-done
	})
	_, err = p.Run()
	return errors.Join(err, watch.Close(), h.Close())
}

// runPopup is the popup the host replaces, unused by the launcher.
func runPopup() error {
	exe, _ := os.Executable() // without it, wt is looked for only at $WT_BIN and on PATH
	var repoDir string
	cwd, findErr := os.Getwd()
	if findErr == nil {
		repoDir, findErr = findRepo(cwd)
	}
	final, err := tea.NewProgram(newModel(wt.New(exe), detail.Reader{}, browser.Opener{}, tmuxClient{}, repoDir, findErr)).Run()
	if err != nil {
		return err
	}
	// Run has restored the terminal, so tmux can have it
	if m, ok := final.(model); ok && m.handoff != "" {
		return handOff(m.handoff)
	}
	return nil
}
