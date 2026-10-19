// Package repo finds which git repo a folder belongs to: the one git lookup
// per session start folder that puts the session in its project.
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
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
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
	return filepath.Dir(strings.TrimSpace(string(out))), nil
}
