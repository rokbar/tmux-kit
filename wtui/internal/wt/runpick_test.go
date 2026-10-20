package wt_test

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// stubFzf puts an fzf first on PATH that runs body (sh), after noting the
// folder it runs in at fzf-pwd and its arguments, one per line, at fzf-args.
// fzf is a TUI, which a test can't drive, and the real one would wait for keys.
func stubFzf(t *testing.T, s *sandbox.Sandbox, body string) {
	t.Helper()
	s.Script("stubs/fzf", `pwd -P >'`+s.Path("fzf-pwd")+`'
printf '%s\n' "$@" >'`+s.Path("fzf-args")+`'
cat >/dev/null
`+body)
	t.Setenv("PATH", s.Path("stubs")+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// readFile is the content of a file in the sandbox; the test fails without it.
func readFile(t *testing.T, s *sandbox.Sandbox, rel string) string {
	t.Helper()
	b, err := os.ReadFile(s.Path(rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

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

// Cancelled (fzf exits 130 on Esc), or once it has opened something, the
// picker is no failure. wt takes the folder it runs in as the invoking
// worktree: a new branch would start from that worktree's branch. Like every
// wt call, it runs with git's credential prompts off.
func TestRunPickThatWorks(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	feat := s.Path("demo--feat")
	s.Git(demo, "worktree", "add", "-q", feat, "-b", "feat")
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	stubFzf(t, s, `echo "$GIT_TERMINAL_PROMPT" >'`+s.Path("prompt")+`'; exit 130`)
	asTerminal(t, s, "")

	if err := wt.New("").RunPick(feat); err != nil {
		t.Errorf("RunPick() = %v, want no failure: the picker was cancelled", err)
	}
	if got := strings.TrimSpace(readFile(t, s, "fzf-pwd")); got != feat {
		t.Errorf("the picker ran in %s, want %s", got, feat)
	}
	if args := readFile(t, s, "fzf-args"); !strings.Contains(args, "alt-enter: new branch from feat\n") {
		t.Errorf("the picker doesn't offer a new branch from feat, the invoking worktree's:\n%s", args)
	}
	if got := readFile(t, s, "prompt"); got != "0\n" {
		t.Errorf("wt ran with GIT_TERMINAL_PROMPT=%q, want 0", strings.TrimSpace(got))
	}
}

// When the picker fails (here, an fzf too old for wt's options), the error's
// text is what wt said on stderr.
func TestFailedRunPickCarriesWtsMessage(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	stubFzf(t, s, `echo 'unknown option: --accept-nth' >&2; exit 2`)
	asTerminal(t, s, "")

	err := wt.New("").RunPick(demo)
	if err == nil {
		t.Fatal("RunPick worked, want it to fail")
	}
	if got, want := err.Error(), "unknown option: --accept-nth"; got != want {
		t.Errorf("error = %q, want wt's message %q", got, want)
	}
}

// With no wt to run, there's no picker.
func TestRunPickWithNoWtIsNotFound(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	t.Setenv("WT_BIN", s.Path("no/such/wt"))
	asTerminal(t, s, "")
	if err := wt.New("").RunPick(demo); !errors.Is(err, wt.ErrNotFound) {
		t.Errorf("RunPick() = %v, want wt.ErrNotFound", err)
	}
}

// Something wt started in the background (e.g. from a config hook) can keep
// its stderr open after wt has exited. RunPick doesn't wait for that, or the
// popup would stay open, frozen, until that ended. Nor does it cut it off,
// which would kill it the next time it wrote there. What it writes once wt
// has exited isn't shown.
func TestRunPickDoesNotWaitForWhatWtLeftRunning(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	pid, alive := s.Path("pid"), s.Path("alive")
	t.Setenv("WT_BIN", s.Script("bin/wt", `(sleep 3; echo late >&2; echo yes >'`+alive+`'; sleep 30) &
echo $! >'`+pid+`'
echo 'wt: opened' >&2`))
	t.Cleanup(func() {
		if b, err := os.ReadFile(pid); err == nil {
			exec.Command("pkill", "-P", strings.TrimSpace(string(b))).Run()
			exec.Command("kill", strings.TrimSpace(string(b))).Run()
		}
	})
	terminal := asTerminal(t, s, "")

	start := time.Now()
	if err := wt.New("").RunPick(demo); err != nil {
		t.Errorf("RunPick() = %v, want no error: wt exited 0", err)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("RunPick took %v: it waited for what wt left running", took)
	}
	for range 50 {
		if _, err := os.Stat(alive); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := os.Stat(alive); err != nil {
		t.Error("what wt left running died writing to stderr once wt had exited")
	}
	if _, stderr := terminal(); stderr != "wt: opened\n" {
		t.Errorf("the terminal showed %q on stderr, want only what wt said before it exited", stderr)
	}
}
