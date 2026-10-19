// Package editor opens a folder in the user's editor, as tmux-kit's prefix e
// does: the command line in TMUX_KIT_EDITOR, else VS Code's code, with the
// folder as its last argument.
package editor

import (
	"cmp"
	"fmt"
	"os/exec"
	"time"

	"github.com/rokbar/tmux-kit/wtui/internal/detached"
)

// patience is how long Open waits for the editor's command to finish, in
// case it fails. One still running by then (e.g. code --wait) has opened the
// folder and stayed with it.
var patience = 3 * time.Second

// Opener opens folders in the editor.
type Opener struct {
	// Editor is the editor's command line, as the normal server's global
	// TMUX_KIT_EDITOR has it, "" while that isn't set. Open reads it each
	// time, as prefix e does.
	Editor func() (string, error)
}

// Open runs the editor on folder, in that folder, detached (see
// detached.Run), and returns once the editor's command has finished, or
// after a few seconds if it hasn't. The command line goes through sh, as
// prefix e's does, so it can have arguments (e.g. "code -n"); the folder
// doesn't. If the editor fails (e.g. there's no such command), the error
// says why, with what it said.
func (o Opener) Open(folder string) error {
	editor, err := o.Editor()
	if err != nil {
		return fmt.Errorf("can't open the editor: %w", err)
	}
	cmd := exec.Command("sh", "-c", `${TMUX_KIT_EDITOR:-code} "$1"`, "sh", folder)
	cmd.Dir = folder
	// the normal server's TMUX_KIT_EDITOR, not wtui's
	cmd.Env = append(cmd.Environ(), "TMUX_KIT_EDITOR="+editor)
	if err := detached.Run(cmd, cmp.Or(editor, "code"), patience); err != nil {
		return fmt.Errorf("can't open the editor: %w", err)
	}
	return nil
}
