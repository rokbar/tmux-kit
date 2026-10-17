// Command wtui is an optional, worktree-first dashboard over tmux-kit's wt.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

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
	if _, err := tea.NewProgram(newModel(wt.New(exe), repoDir, findErr)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "wtui:", err)
		os.Exit(1)
	}
}
