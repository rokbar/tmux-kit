// Command wtui is an optional app over tmux-kit's wt and tmux: run in a plain
// terminal, it opens a host with the sidebar on the left and a session on the
// right.
//
//	wtui              launch: a host on this terminal
//	wtui --version    print the version
//
// The host runs the rest itself:
//
//	wtui sidebar           the sidebar, in the host's left pane
//	wtui right [<session>] the right side's wrapper, in its right pane
//	wtui popup             the old popup, kept until the host replaces it
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/browser"
	"github.com/rokbar/tmux-kit/wtui/internal/detail"
	"github.com/rokbar/tmux-kit/wtui/internal/host"
	gitrepo "github.com/rokbar/tmux-kit/wtui/internal/repo" // the popup has its own repo
	"github.com/rokbar/tmux-kit/wtui/internal/sidebar"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
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
		err = runSidebar()
	case "right":
		err = host.Right(&tmux.Client{}, flag.Arg(1))
	case "popup":
		err = runPopup()
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

// runSidebar runs the sidebar in its host until q, then closes the host.
func runSidebar() error {
	exe, _ := os.Executable() // without it, wt is looked for only at $WT_BIN and on PATH
	normal := &tmux.Client{}
	key := normal.Key()
	h, err := host.Sidebar(key, normal.Width())
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
	c := sidebar.Clients{Host: h, Normal: normal, Repos: gitrepo.Lookup{}, Wt: wt.New(exe)}
	var m tea.Model = sidebar.New(c, key)
	// The sidebar starts with focus, as the host gives it, unless it starts
	// hidden (a narrow terminal): tmux moved focus right before the sidebar
	// asked for focus events, so it won't say so.
	if hidden, _ := h.Hidden(); hidden {
		m, _ = m.Update(tea.BlurMsg{})
	}
	_, err = tea.NewProgram(m).Run()
	return errors.Join(err, h.Close())
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
