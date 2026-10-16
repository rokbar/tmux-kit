// Command wtui is an optional, worktree-first dashboard over tmux-kit's wt.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("wtui", currentVersion())
		return
	}
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "wtui:", err)
		os.Exit(1)
	}
}
