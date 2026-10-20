package main

import (
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/detail"
	"github.com/rokbar/tmux-kit/wtui/internal/dialog"
	"github.com/rokbar/tmux-kit/wtui/internal/sidebar"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// The dialogs, which the sidebar runs in popups (see sidebar.Model.dialog).
// What they say for the sidebar's status line, they say with dialog.Say.

// runDetails is the details dialog of the main clone or worktree in folder,
// on branch, with dev URL devURL, as wt listed them ("" for none).
func runDetails(folder, branch, devURL string) error {
	_, err := tea.NewProgram(dialog.Details(detail.Reader{}, folder, branch, devURL)).Run()
	return err
}

// runPick runs the picker here, in this folder, the invoking worktree. Its
// failure, which wt has shown in the popup already, is for the status line.
func runPick() error {
	exe, _ := os.Executable() // without it, wt is looked for only at $WT_BIN and on PATH
	dir, err := os.Getwd()
	if err == nil {
		err = wt.New(exe).RunPick(dir)
	}
	if err != nil {
		return dialog.Say(err.Error())
	}
	return nil
}

// runRemove is the remove dialog of the worktree removal names (see
// dialog.Removal.Arg), run in its main clone: what wt said, or its failure,
// is for the status line.
func runRemove(removal string) error {
	r, err := dialog.ParseRemoval(removal)
	if err != nil {
		return err
	}
	exe, _ := os.Executable() // without it, wt is looked for only at $WT_BIN and on PATH
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	final, err := tea.NewProgram(dialog.Remove(wt.New(exe), dir, r)).Run()
	if err != nil {
		return err
	}
	return dialog.Say(final.(dialog.Removing).Said())
}

// runShortcuts shows every shortcut, where key is the wtui key.
func runShortcuts(key string) error {
	_, err := tea.NewProgram(dialog.Page(sidebar.Shortcuts(key))).Run()
	return err
}
