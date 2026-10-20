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
	gitrepo "github.com/rokbar/tmux-kit/wtui/internal/repo"
	"github.com/rokbar/tmux-kit/wtui/internal/sidebar"
	"github.com/rokbar/tmux-kit/wtui/internal/tfolder"
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

// runProject is the open-project dialog (p), its arguments [-d] <root>: it
// has fzf choose a project of the projects root, root, then opens its
// session, a git repo's through wt and a plain folder's through t, and
// switches the nested client to it, since this popup is that client's. With
// -d, the popup is the host's, while the right side shows no session: then it
// only opens the session, which the right side shows once there. When the
// root is missing or empty, it says so in the popup; its failures are for
// the status line.
func runProject(args []string) error {
	detach := len(args) > 0 && args[0] == "-d"
	if detach {
		args = args[1:]
		// In the host's popup, TMUX names the host: wt and t must find the
		// normal server, as the sidebar's own calls do.
		os.Unsetenv("TMUX")
		os.Unsetenv("TMUX_PANE")
	}
	if len(args) != 1 {
		return dialog.Say("usage: wtui project [-d] <projects root>")
	}
	root := args[0]
	projects, err := gitrepo.Lookup{}.Projects(root)
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
	if err := openProject(projects[i], detach); err != nil {
		return dialog.Say(err.Error())
	}
	return nil
}

// openProject opens p's session, through wt for a git repo and t for a plain
// folder, and switches the client this runs in to it, unless detach.
func openProject(p gitrepo.Project, detach bool) error {
	exe, _ := os.Executable()         // without it, wt and t are looked for only at $WT_BIN and on PATH
	path, err := filepath.Abs(p.Path) // so wt takes it for a path, not a branch
	switch {
	case err != nil:
		return err
	case p.Git && detach:
		return wt.New(exe).Open(path, path)
	case p.Git:
		return wt.New(exe).OpenAndSwitch(path, path)
	case detach:
		_, err := tfolder.New(exe).Start(path)
		return err
	}
	return tfolder.New(exe).Open(path)
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
