// Package wt is wtui's client for tmux-kit's wt, and the only code in wtui
// that knows wt's command line.
package wt

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// State is whether a worktree has uncommitted changes, or has lost its folder.
type State string

const (
	Clean   State = "clean"
	Dirty   State = "dirty"
	Missing State = "missing" // git still has it registered, but its folder is gone
)

// Worktree is the main clone or one of the repo's worktrees, as wt lists it.
type Worktree struct {
	Path      string
	Branch    string // empty when detached (including mid-rebase) or missing
	State     State
	Session   string // the session started in it; empty if none
	DevURL    string // empty if none
	MainClone bool
}

var (
	// ErrNotFound is no wt to run: not at $WT_BIN, next to wtui, or on PATH.
	ErrNotFound = errors.New("wt not found")
	// ErrTooOld is a wt without the `ls --porcelain` wtui reads: the call
	// failed, or printed a line with fewer than its 5 fields.
	ErrTooOld = errors.New("wt too old for wtui")
)

// callError is a wt call that failed as kind (e.g. ErrTooOld). Its text is
// what wt said, or what went wrong when wt said nothing.
type callError struct {
	kind error
	msg  string
}

func (e *callError) Error() string { return e.msg }
func (e *callError) Unwrap() error { return e.kind }

// Client runs wt for the wtui executable at wtuiPath.
type Client struct{ wtuiPath string }

// New returns the client for the wtui executable at wtuiPath (os.Executable).
// Each call runs $WT_BIN if it is set, else the wt next to wtui, else the wt
// on PATH: install.sh puts wt and wtui in ~/.local/bin, which the tmux
// server's PATH may lack.
func New(wtuiPath string) *Client { return &Client{wtuiPath: wtuiPath} }

// List returns the main clone, then every worktree, of the repo dir is in.
// A wt without `ls --porcelain` is ErrTooOld, whatever it printed.
func (c *Client) List(dir string) ([]Worktree, error) {
	out, err := c.run(dir, "ls", "--porcelain")
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return nil, &callError{ErrTooOld, failureText(exit)}
	}
	if err != nil {
		return nil, err
	}
	// path, branch, state, session, dev URL; wt only ever adds fields at the end
	var rows []Worktree
	for i, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 5 {
			return nil, &callError{ErrTooOld,
				fmt.Sprintf("wt ls --porcelain printed %q, not 5 tab-separated fields", line)}
		}
		rows = append(rows, Worktree{
			Path: f[0], Branch: f[1], State: State(f[2]), Session: f[3], DevURL: f[4],
			MainClone: i == 0,
		})
	}
	return rows, nil
}

// Open has wt open the session of the main clone or worktree at path,
// starting one with wt's default name if it has none, without switching to it
// (`wt -d <path>`). wt runs in dir, as for List. If wt fails, the error's text
// is what it said.
func (c *Client) Open(dir, path string) error {
	_, err := c.run(dir, "-d", path)
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return errors.New(failureText(exit))
	}
	return err
}

// run runs wt with args in dir and returns its stdout. A wt that exits
// non-zero is an *exec.ExitError holding its stderr.
func (c *Client) run(dir string, args ...string) ([]byte, error) {
	bin, err := c.find()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	// git must never stop to ask for credentials: nobody would see the prompt
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	return cmd.Output()
}

// failureText is what a wt that exited non-zero printed on stderr, or its
// exit status if it printed nothing there.
func failureText(exit *exec.ExitError) string {
	if msg := strings.TrimSpace(string(exit.Stderr)); msg != "" {
		return msg
	}
	return "wt: " + exit.Error()
}

// find returns the wt to run. A WT_BIN that isn't a program is an error, not
// a reason to run some other wt.
func (c *Client) find() (string, error) {
	if bin := os.Getenv("WT_BIN"); bin != "" {
		if !isProgram(bin) {
			return "", &callError{ErrNotFound, "wt not found: WT_BIN is " + bin + ", which isn't a program"}
		}
		return bin, nil
	}
	where := "on PATH"
	if c.wtuiPath != "" {
		dir := filepath.Dir(c.wtuiPath)
		if bin := filepath.Join(dir, "wt"); isProgram(bin) {
			return bin, nil
		}
		where = "next to wtui (in " + dir + ") or on PATH"
	}
	if bin, err := exec.LookPath("wt"); err == nil {
		return bin, nil
	}
	return "", &callError{ErrNotFound, "wt not found " + where + ".\n" +
		"Install tmux-kit (its install.sh links wt into ~/.local/bin), or set WT_BIN to wt's path."}
}

// isProgram reports whether path is an executable file.
func isProgram(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0
}
