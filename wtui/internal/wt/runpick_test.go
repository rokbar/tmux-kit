package wt_test

import (
	"os"
	"strings"
	"testing"

	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// asTerminal makes files in the sandbox this process's stdin (with input in
// it), stdout and stderr, as a popup's terminal would be, until the test
// ends. It returns a func that reads what's been written to stdout and to
// stderr.
func asTerminal(t *testing.T, s *sandbox.Sandbox, input string) func() (stdout, stderr string) {
	t.Helper()
	s.Write("terminal-in", input)
	in, err := os.Open(s.Path("terminal-in"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(s.Path("terminal-out"))
	if err != nil {
		t.Fatal(err)
	}
	errs, err := os.Create(s.Path("terminal-err"))
	if err != nil {
		t.Fatal(err)
	}
	wasIn, wasOut, wasErr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = in, out, errs
	t.Cleanup(func() {
		os.Stdin, os.Stdout, os.Stderr = wasIn, wasOut, wasErr
		in.Close()
		out.Close()
		errs.Close()
	})
	return func() (string, string) { return readFile(t, s, "terminal-out"), readFile(t, s, "terminal-err") }
}

// In a popup, the picker runs on wtui's own terminal, the popup's: wt reads
// from it (e.g. a new session's name) and writes to it, in the folder it's
// given, the invoking worktree, and RunPick returns once wt has exited. If
// wt fails, the error's text is what it said on stderr, which the terminal
// showed too, up to the prompt that waited for a key.
func TestRunPickRunsThePickerOnThisTerminal(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	t.Setenv("WT_BIN", s.Script("bin/wt", `pwd -P >'`+s.Path("pwd")+`'
echo "$*" >'`+s.Path("args")+`'
read -r name
echo "wt: $PWD (tmux session $name)"
echo 'wt: tmux session y already exists' >&2
printf 'wt failed (see above); press any key' >&2
exit 1`))
	terminal := asTerminal(t, s, "y\n")

	err := wt.New("").RunPick(demo)
	if err == nil || err.Error() != "wt: tmux session y already exists" {
		t.Errorf("RunPick() = %v, want wt's message, up to its prompt", err)
	}
	stdout, stderr := terminal()
	if want := "wt: " + demo + " (tmux session y)\n"; stdout != want {
		t.Errorf("the terminal showed %q on stdout, want %q", stdout, want)
	}
	if want := "wt: tmux session y already exists\nwt failed (see above); press any key"; stderr != want {
		t.Errorf("the terminal showed %q on stderr, want %q", stderr, want)
	}
	if got := strings.TrimSpace(readFile(t, s, "pwd")); got != demo {
		t.Errorf("wt ran in %s, want %s", got, demo)
	}
	if got := strings.TrimSpace(readFile(t, s, "args")); got != "pick" {
		t.Errorf("wt ran as wt %s, want wt pick", got)
	}
}

// Cancelled, or once it has opened something, the picker is no failure.
func TestRunPickThatWorks(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	stubFzf(t, s, `exit 130`)
	asTerminal(t, s, "")
	if err := wt.New("").RunPick(demo); err != nil {
		t.Errorf("RunPick() = %v, want no failure: the picker was cancelled", err)
	}
}
