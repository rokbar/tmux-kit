// Package detail reads what wtui's details dialog (i) shows about the main
// clone or a worktree, with read-only git run in its folder alone.
package detail

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Details is what the details dialog shows about the main clone or a worktree.
type Details struct {
	// Changes is its changed files, a line each as git's short status shows
	// them, e.g. " M f" or "?? new.txt"; none when it's clean.
	Changes []string
	// Commits is its last 10 commits, newest first, a line each as
	// `git log --oneline` shows them: the short hash, then the subject.
	Commits []string
	// Upstream is the upstream of its branch, e.g. "origin/feat".
	Upstream string
	// Ahead is how many commits HEAD has that its upstream doesn't, and
	// Behind how many its upstream has that HEAD doesn't, as last fetched.
	Ahead, Behind int
}

// Reader reads the details of the main clone or a worktree with git.
type Reader struct{}

// Read returns the details of the main clone or worktree at path. If git
// fails, the error's text names the git command and says what git said.
func (Reader) Read(path string) (Details, error) {
	// the short format, as no config can change it (e.g. colour it)
	status, err := git(path, "status", "--porcelain")
	if err != nil {
		return Details{}, err
	}
	// no decorations, colours or signatures, whatever the user's config says:
	// just the hash and subject
	log, err := git(path, "log", "--oneline", "--no-decorate", "--no-color", "--no-show-signature", "-10")
	if err != nil {
		return Details{}, err
	}
	d := Details{Changes: lines(status), Commits: lines(log)}
	// git can't name it when the branch has none, or there's no branch (a
	// detached HEAD, as mid-rebase): either way, it isn't pushed
	upstream, err := git(path, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		return d, nil
	}
	d.Upstream = strings.TrimSpace(upstream)
	counts, err := git(path, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return Details{}, err
	}
	if _, err := fmt.Sscan(counts, &d.Ahead, &d.Behind); err != nil {
		return Details{}, fmt.Errorf("git rev-list printed %q, not two counts", counts)
	}
	return d, nil
}

// git runs git with args in dir and returns its output. If git exits
// non-zero, the error's text is what it said on stderr, or its exit status if
// it said nothing there. It takes no lock git can do without (e.g. status
// writing back the index), since the user may be running git in dir at the
// same time.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"--no-optional-locks"}, args...)...)
	cmd.Dir = dir
	// git must never stop to ask for credentials: nobody would see the prompt
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		said := strings.TrimSpace(string(exit.Stderr))
		if said == "" {
			said = exit.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], said)
	}
	return string(out), err
}

// lines is out's lines; none if it's empty.
func lines(out string) []string {
	if out = strings.TrimRight(out, "\n"); out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}
