// Package repo finds which git repo a folder belongs to: the one git lookup
// per session start folder that puts the session in its project, and the
// launch folder's, which says which worktree wtui starts in.
package repo

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Lookup asks git.
type Lookup struct{}

// MainClone is the main clone of the git repo dir is in, whether dir is in
// the main clone itself or in one of its worktrees, or "" if dir isn't in a
// git repo. As wt finds it: the folder that holds the repo's common git
// folder.
func (Lookup) MainClone(dir string) (string, error) {
	out, err := git(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || out == "" {
		return "", err
	}
	return filepath.Dir(out), nil
}

// Worktrees is the folder of the main clone or worktree dir is in, and the
// folders of that repo's main clone and every worktree, in git's order; ""
// and none if dir isn't in a git repo.
func (Lookup) Worktrees(dir string) (in string, all []string, err error) {
	in, err = git(dir, "rev-parse", "--path-format=absolute", "--show-toplevel")
	if err != nil || in == "" {
		return "", nil, err
	}
	out, err := git(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return "", nil, err
	}
	for _, line := range strings.Split(out, "\n") {
		if folder, ok := strings.CutPrefix(line, "worktree "); ok {
			all = append(all, folder)
		}
	}
	return in, all, nil
}

// git is what git, run in dir with args, printed, trimmed: "" if dir isn't
// in a git repo. If git fails otherwise, the error is its reason.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	msg := strings.TrimSpace(stderr.String())
	var exit *exec.ExitError
	if errors.As(err, &exit) && strings.Contains(msg, "not a git repository") {
		return "", nil
	}
	if err != nil {
		if msg == "" {
			msg = err.Error() // e.g. no git
		}
		return "", fmt.Errorf("git in %s: %s", dir, msg)
	}
	return strings.TrimSpace(string(out)), nil
}
