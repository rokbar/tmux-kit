package wt_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// finished is what runOnTerminal's program gets once the command has run.
type finished struct{ err error }

// runner is a program that runs cmd, then quits with what it reported.
type runner struct {
	cmd tea.Cmd
	err error
	ran bool
}

func (r runner) Init() tea.Cmd { return r.cmd }

func (r runner) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if f, ok := msg.(finished); ok {
		r.err, r.ran = f.err, true
		return r, tea.Quit
	}
	return r, nil
}

func (r runner) View() tea.View { return tea.NewView("") }

// runOnTerminal runs the command an interactive operation makes, given the
// callback it reports to, in a Bubble Tea program as wtui does: it pauses the
// program while wt runs, then resumes it. There's no terminal here: wt's
// stdin is empty, and its stdout, like a terminal, is a file (/dev/null). It
// returns the error the operation reported.
func runOnTerminal(t *testing.T, operation func(done tea.ExecCallback) tea.Cmd) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	cmd := operation(func(err error) tea.Msg { return finished{err} })
	p := tea.NewProgram(runner{cmd: cmd},
		tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(out), tea.WithoutSignalHandler())
	final, err := p.Run()
	if err != nil {
		t.Fatalf("the program running wt: %v", err)
	}
	r := final.(runner)
	if !r.ran {
		t.Fatal("the operation never reported back")
	}
	return r.err
}

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

// The picker runs in the folder it's given, which wt takes as the invoking
// worktree: a new branch would start from that worktree's branch. Cancelled
// (fzf exits 130 on Esc), it's no failure. Like every wt call, it runs with
// git's credential prompts off.
func TestPickRunsThePickerInTheGivenFolder(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	feat := s.Path("demo--feat")
	s.Git(demo, "worktree", "add", "-q", feat, "-b", "feat")
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	stubFzf(t, s, `echo "$GIT_TERMINAL_PROMPT" >'`+s.Path("prompt")+`'; exit 130`)

	if err := runOnTerminal(t, func(done tea.ExecCallback) tea.Cmd { return wt.New("").Pick(feat, done) }); err != nil {
		t.Fatalf("Pick(%s): %v", feat, err)
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
// text is what wt said on stderr, which was on the terminal too.
func TestFailedPickCarriesWtsMessage(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	stubFzf(t, s, `echo 'unknown option: --accept-nth' >&2; exit 2`)

	err := runOnTerminal(t, func(done tea.ExecCallback) tea.Cmd { return wt.New("").Pick(demo, done) })
	if err == nil {
		t.Fatal("Pick worked, want it to fail")
	}
	if got, want := err.Error(), "unknown option: --accept-nth"; got != want {
		t.Errorf("error = %q, want wt's message %q", got, want)
	}
}

// terminalStderr makes a file in the sandbox the stderr of the terminal that
// runOnTerminal's program has (tea.Exec hands wt os.Stderr), until the test
// ends. It returns a func that reads what's been written there.
func terminalStderr(t *testing.T, s *sandbox.Sandbox) func() string {
	t.Helper()
	f, err := os.Create(s.Path("terminal-stderr"))
	if err != nil {
		t.Fatal(err)
	}
	was := os.Stderr
	os.Stderr = f
	t.Cleanup(func() { os.Stderr = was; f.Close() })
	return func() string { return readFile(t, s, "terminal-stderr") }
}

// wt's prompts (a new session's name; "press any key" after a failure, so the
// message stays on screen) are on stderr, at the end of the line they wait
// on. So wt's stderr goes to the terminal as well as being kept. A prompt
// was for the terminal: what went wrong is the lines before it.
func TestPickShowsWtsStderrOnTheTerminalAndLeavesOutAPromptFromTheError(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	t.Setenv("WT_BIN", s.Script("bin/wt",
		`echo 'wt: invalid branch name: a b' >&2; printf 'wt failed (see above); press any key' >&2; exit 1`))
	terminal := terminalStderr(t, s)

	err := runOnTerminal(t, func(done tea.ExecCallback) tea.Cmd { return wt.New("").Pick(demo, done) })
	if got, want := terminal(), "wt: invalid branch name: a b\nwt failed (see above); press any key"; got != want {
		t.Errorf("the terminal showed %q on stderr, want %q", got, want)
	}
	if err == nil {
		t.Fatal("Pick worked, want it to fail")
	}
	if got, want := err.Error(), "wt: invalid branch name: a b"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

// With no wt to run, nothing pauses wtui: the operation reports that at once.
func TestPickWithNoWtIsNotFound(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	t.Setenv("WT_BIN", s.Path("no/such/wt"))
	cmd := wt.New("").Pick(demo, func(err error) tea.Msg { return finished{err} })
	msg, ok := cmd().(finished)
	if !ok {
		t.Fatalf("Pick's command gave %T, want the callback's message straight away", msg)
	}
	if !errors.Is(msg.err, wt.ErrNotFound) {
		t.Errorf("error = %v, want wt.ErrNotFound", msg.err)
	}
}

// Something wt started in the background (e.g. from a config hook) can keep
// its stderr open after wt has exited. wtui doesn't wait for that, or it
// would stay paused, its terminal frozen, until that ended. Nor does it cut
// it off, which would kill it the next time it wrote there. What it writes
// once wtui has the terminal back isn't shown: it would draw over wtui.
func TestPickDoesNotWaitForWhatWtLeftRunning(t *testing.T) {
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
	terminal := terminalStderr(t, s)

	start := time.Now()
	if err := runOnTerminal(t, func(done tea.ExecCallback) tea.Cmd { return wt.New("").Pick(demo, done) }); err != nil {
		t.Errorf("Pick: %v, want no error: wt exited 0", err)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("Pick took %v: it waited for what wt left running", took)
	}
	for range 50 {
		if _, err := os.Stat(alive); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := os.Stat(alive); err != nil {
		t.Error("what wt left running died writing to stderr once wtui was back")
	}
	if got := terminal(); got != "wt: opened\n" {
		t.Errorf("the terminal showed %q on stderr, want only what wt said before it exited", got)
	}
}
