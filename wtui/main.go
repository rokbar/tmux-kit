// Command wtui is an optional, worktree-first dashboard over tmux-kit's wt.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/browser"
	"github.com/rokbar/tmux-kit/wtui/internal/detail"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("wtui", currentVersion())
		return
	}
	exe, _ := os.Executable() // without it, wt is looked for only at $WT_BIN and on PATH
	var repoDir string
	cwd, findErr := os.Getwd()
	if findErr == nil {
		repoDir, findErr = findRepo(cwd)
	}
	final, err := tea.NewProgram(newModel(wt.New(exe), detail.Reader{}, browser.Opener{}, repoDir, findErr)).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "wtui:", err)
		os.Exit(1)
	}
	// Run has restored the terminal, so tmux can have it
	if m, ok := final.(model); ok && m.handoff != "" {
		if err := handOff(m.handoff); err != nil {
			fmt.Fprintln(os.Stderr, "wtui:", err)
			os.Exit(1)
		}
	}
}
