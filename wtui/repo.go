package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// errNotInRepo is wtui opened in a folder that isn't in a git repo. It's
// found before wt runs, since wt failing there would look like an old wt.
var errNotInRepo = errors.New("not in a git repo")

// findRepo returns the top folder of the main clone or worktree dir is in.
func findRepo(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		// with git's reason, which may be another one (e.g. a repo owned by
		// someone else that git won't read)
		return "", fmt.Errorf("%w: %s\n\n%s", errNotInRepo, dir, strings.TrimSpace(string(exit.Stderr)))
	}
	if err != nil {
		return "", err // e.g. no git
	}
	return strings.TrimSpace(string(out)), nil
}
