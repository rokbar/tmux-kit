// Package wt is wtui's client for tmux-kit's wt, and the only code in wtui
// that knows wt's command line.
package wt

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
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
	// ErrSafetyRefusal is wt declining to remove a worktree because something
	// would be lost (exit status 3). Unlike any other failure to remove one,
	// forcing gets past it.
	ErrSafetyRefusal = errors.New("wt refused to remove the worktree: something would be lost")
)

// callError is a wt call that failed as kind (e.g. ErrTooOld; nil for no
// particular kind). Its text is what wt said, or what went wrong when wt said
// nothing.
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
	out, err := c.run(dir, always(ErrTooOld), "ls", "--porcelain")
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
	_, err := c.run(dir, noKind, "-d", path)
	return err
}

// Remove has wt remove the worktree at path, or unregister it if it's missing,
// and end its session (`wt rm [-f] <path>`). force skips wt's safety checks.
// wt runs in dir, as for List. It returns what wt said it did. If wt refuses
// because something would be lost, the error is an ErrSafetyRefusal; either
// way, its text is what wt said.
func (c *Client) Remove(dir, path string, force bool) (string, error) {
	args := []string{"rm", path}
	if force {
		args = []string{"rm", "-f", path}
	}
	out, err := c.run(dir, removeKind, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Pick is the picker (`wt pick`), run in dir, which wt takes as the invoking
// worktree: a new worktree copies its local files, and a new branch starts
// from its branch unless the picker is told otherwise. The picker needs the
// terminal, so this is a command that pauses wtui while the picker runs,
// then reports to done. wt exits 0 whether the picker opened something or
// was cancelled; if it fails, the error's text is what it said.
func (c *Client) Pick(dir string, done tea.ExecCallback) tea.Cmd {
	return c.onTerminal(dir, done, "pick")
}

// onTerminal is a command that runs wt with args in dir on the terminal:
// tea.Exec pauses wtui, gives wt the terminal, and resumes wtui once wt has
// exited, then reports to done. If wt exits non-zero, done's error is of no
// particular kind, with what wt said as its text. Without a wt to run,
// nothing pauses: done gets that at once.
func (c *Client) onTerminal(dir string, done tea.ExecCallback, args ...string) tea.Cmd {
	cmd, err := c.command(dir, args...)
	if err != nil {
		return func() tea.Msg { return done(err) }
	}
	return tea.Exec(&terminalRun{cmd: cmd}, done)
}

// terminalRun is wt run by tea.Exec, which hands it the terminal's input and
// output. What wt says on stderr goes to the terminal too, since its prompts
// are there (e.g. for a new session's name), and is also kept, for the
// error if wt fails.
type terminalRun struct {
	cmd    *exec.Cmd
	stderr io.Writer // the terminal's
}

func (r *terminalRun) SetStdin(in io.Reader)   { r.cmd.Stdin = in }
func (r *terminalRun) SetStdout(out io.Writer) { r.cmd.Stdout = out }
func (r *terminalRun) SetStderr(out io.Writer) { r.stderr = out }

func (r *terminalRun) Run() error {
	// A pipe of wtui's own, not exec's: exec would wait for every process
	// holding it to let go, and wt can leave one running in the background.
	read, write, err := os.Pipe()
	if err != nil {
		return err
	}
	stderr := &stderrCopy{terminal: r.stderr}
	copied := make(chan struct{})
	go func() {
		io.Copy(stderr, read) // until everything holding the pipe has let go
		read.Close()
		close(copied)
	}()
	r.cmd.Stderr = write
	err = r.cmd.Run()
	write.Close()
	// What wt left running can go on holding it: wtui waits for that only so
	// long, or it would stay paused. After that, what comes is drained, not
	// shown, since wtui has the terminal back; nor cut off, which would kill
	// the writer.
	select {
	case <-copied:
	case <-time.After(stderrGrace):
	}
	said := stderr.stop()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return &callError{nil, failureText(beforePrompt(said), exit)}
	}
	return err
}

// stderrGrace is how long wtui waits, once wt has exited, for what wt left
// running to let go of its stderr.
const stderrGrace = time.Second

// stderrCopy is wt's stderr on its way to the terminal, kept as well, until
// stop.
type stderrCopy struct {
	mu       sync.Mutex
	terminal io.Writer
	kept     bytes.Buffer
	stopped  bool
}

func (c *stderrCopy) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.stopped {
		c.kept.Write(p)
		c.terminal.Write(p) // a terminal that's gone mustn't stop the copy
	}
	return len(p), nil
}

// stop has what comes after be dropped, and returns what was kept.
func (c *stderrCopy) stop() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = true
	return c.kept.String()
}

// beforePrompt is what wt said on stderr up to a prompt it left waiting at
// the end of the last line (e.g. "press any key", so its message stays on
// screen): the prompt was for the terminal.
func beforePrompt(said string) string { return said[:strings.LastIndex(said, "\n")+1] }

// removeKind is the kind of error a `wt rm` that exited with status is: 3 is
// a safety refusal, anything else no particular kind.
func removeKind(status int) error {
	if status == 3 {
		return ErrSafetyRefusal
	}
	return nil
}

// always is kind, whatever wt's exit status.
func always(kind error) func(status int) error {
	return func(int) error { return kind }
}

// noKind is no particular kind of error, whatever wt's exit status.
func noKind(int) error { return nil }

// run runs wt with args in dir and returns its stdout. If wt exits non-zero,
// the error's kind is kindOf(its exit status), and its text is what wt said.
func (c *Client) run(dir string, kindOf func(status int) error, args ...string) ([]byte, error) {
	cmd, err := c.command(dir, args...)
	if err != nil {
		return nil, err
	}
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return nil, &callError{kindOf(exit.ExitCode()), failureText(string(exit.Stderr), exit)}
	}
	return out, err
}

// command is wt with args, to run in dir.
func (c *Client) command(dir string, args ...string) (*exec.Cmd, error) {
	bin, err := c.find()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	// git must never stop to ask for credentials: behind the TUI, nobody
	// would see the prompt. Calls on the terminal too, so every call is alike
	// (wt falls back to local refs when a fetch fails).
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	return cmd, nil
}

// failureText is what a wt that exited non-zero said on stderr, or its exit
// status if it said nothing there.
func failureText(said string, exit *exec.ExitError) string {
	if msg := strings.TrimSpace(said); msg != "" {
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
