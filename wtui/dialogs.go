package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/detail"
	"github.com/rokbar/tmux-kit/wtui/internal/dialog"
	"github.com/rokbar/tmux-kit/wtui/internal/repo"
	"github.com/rokbar/tmux-kit/wtui/internal/sidebar"
	"github.com/rokbar/tmux-kit/wtui/internal/tfolder"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
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

// runPick runs the picker here, in this folder, the invoking worktree. What
// wt said it did, or its failure, which wt has shown in the popup already, is
// for the status line.
func runPick() error {
	exe, _ := os.Executable() // without it, wt is looked for only at $WT_BIN and on PATH
	dir, err := os.Getwd()
	var said string
	if err == nil {
		said, err = wt.New(exe).RunPick(dir)
	}
	if err != nil {
		return dialog.Say(err.Error())
	}
	return dialog.Say(said)
}

// runRemove is the remove dialog of the worktree at path, run in its main
// clone: it asks question, then has wt remove it. What wt said, or its
// failure, is for the status line.
func runRemove(path, question string) error {
	if path == "" || question == "" {
		return errors.New("usage: wtui remove <path> <question>")
	}
	exe, _ := os.Executable() // without it, wt is looked for only at $WT_BIN and on PATH
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	final, err := tea.NewProgram(dialog.Remove(wt.New(exe), dir, path, strings.Split(question, "\n"))).Run()
	if err != nil {
		return err
	}
	return dialog.Say(final.(dialog.Removing).Said())
}

// runProject is the open-project dialog (p), its arguments [-d] <root>: it
// has fzf choose a project of the projects root, root, then opens its
// session, a git repo's through wt and a plain folder's through t, and
// switches the nested client to it, since this popup is that client's. With
// -d, the popup is the host's, while the right side shows no session: then it
// only opens the session, on the normal server at socket, which the right
// side shows once there. When the root is missing or empty, it says so in
// the popup; what wt said it did, or the failures, are for the status line.
func runProject(socket string, args []string) error {
	detach := len(args) > 0 && args[0] == "-d"
	if detach {
		args = args[1:]
		// In the host's popup, TMUX names the host: wt and t must reach the
		// normal server, by its socket once known, as the sidebar does. With
		// -d, they only start a session there, never switch or attach.
		os.Unsetenv("TMUX")
		os.Unsetenv("TMUX_PANE")
		if socket != "" {
			os.Setenv("TMUX", tmux.InServer(socket))
		}
	}
	if len(args) != 1 {
		return dialog.Say("usage: wtui [-S <socket>] project [-d] <projects root>")
	}
	root := args[0]
	projects, err := repo.Lookup{}.Projects(root)
	none := func(why string) error {
		return showPage("No projects to open: "+why, "",
			"p offers the folders in the projects root. To have it offer",
			"another's, set @wtui-projects-root in ~/.tmux.local.conf.")
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return none(tilde(root) + " is missing.")
	case err != nil:
		return dialog.Say("can't list the projects: " + err.Error())
	case len(projects) == 0:
		return none(tilde(root) + " has no folders.")
	}
	names := make([]string, len(projects))
	for i, p := range projects {
		names[i] = filepath.Base(p.Path)
	}
	name, err := dialog.Choose("project> ", names)
	if err != nil {
		return dialog.Say(err.Error())
	}
	i := slices.Index(names, name)
	if i < 0 {
		return nil // cancelled
	}
	said, err := openProject(projects[i], detach)
	if err != nil {
		return dialog.Say(err.Error())
	}
	return dialog.Say(said)
}

// openProject opens p's session, through wt for a git repo and t for a plain
// folder, and switches the client this runs in to it, unless detach. It
// returns what wt said it did; t says nothing.
func openProject(p repo.Project, detach bool) (string, error) {
	exe, _ := os.Executable()         // without it, wt and t are looked for only at $WT_BIN and on PATH
	path, err := filepath.Abs(p.Path) // so wt takes it for a path, not a branch
	switch {
	case err != nil:
		return "", err
	case p.Git && detach:
		return wt.New(exe).Open(path, path)
	case p.Git:
		return wt.New(exe).OpenAndSwitch(path, path)
	case detach:
		_, err := tfolder.New(exe).Start(path)
		return "", err
	}
	return "", tfolder.New(exe).Open(path)
}

// tilde is path with ~ for HOME, as the user would write it.
func tilde(path string) string {
	home, err := os.UserHomeDir()
	if rest, ok := strings.CutPrefix(path, home); err == nil && ok && (rest == "" || rest[0] == '/') {
		return "~" + rest
	}
	return path
}

// showPage shows lines until any key closes them.
func showPage(lines ...string) error {
	_, err := tea.NewProgram(dialog.Page(lines)).Run()
	return err
}

// runShortcuts shows every shortcut, where key is the wtui key.
func runShortcuts(key string) error {
	_, err := tea.NewProgram(dialog.Page(sidebar.Shortcuts(key))).Run()
	return err
}
