package wt_test

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// gitUIIs has wt read a config of its own, which sets WT_GIT_UI to command,
// as a user's would.
func gitUIIs(t *testing.T, s *sandbox.Sandbox, command string) {
	t.Helper()
	s.Write("config.sh", "WT_GIT_UI='"+command+"'\n")
	t.Setenv("WT_CONFIG", s.Path("config.sh"))
}

// gitUICommand starts the words of GitUICommand(path) in folder dir, with
// stdin a pipe the test writes keys to, and stdout and stderr together in
// out. done gives how it exited, once it has.
func gitUICommand(t *testing.T, dir, path string) (done <-chan error, keys io.WriteCloser, out *strings.Builder) {
	t.Helper()
	words, err := wt.New("").GitUICommand(path)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(words[0], words[1:]...)
	cmd.Dir = dir
	out = &strings.Builder{}
	cmd.Stdout, cmd.Stderr = out, out
	if keys, err = cmd.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exit := make(chan error, 1)
	go func() { exit <- cmd.Wait() }()
	t.Cleanup(func() { cmd.Process.Kill() })
	return exit, keys, out
}

// exited is whether the command done is for has exited within d, and how.
func exited(done <-chan error, d time.Duration) (bool, error) {
	select {
	case err := <-done:
		return true, err
	case <-time.After(d):
		return false, nil
	}
}

// The git UI's command line runs it in the worktree at the path it's
// given, from whatever folder it runs in, as the git UI's own terminal:
// wt adds nothing to what it shows.
func TestGitUICommandRunsTheGitUIInTheGivenWorktree(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	feat := s.Path("demo--feat")
	s.Git(demo, "worktree", "add", "-q", feat, "-b", "feat")
	s.Script("stubs/stub-ui", `pwd -P >'`+s.Path("ui-pwd")+`'; echo the git UI`)
	t.Setenv("PATH", s.Path("stubs")+string(os.PathListSeparator)+os.Getenv("PATH"))
	gitUIIs(t, s, "stub-ui")

	done, _, out := gitUICommand(t, s.Dir, feat)
	if ok, err := exited(done, 10*time.Second); !ok || err != nil {
		t.Fatalf("the git UI's command line: exited %v, %v; it said %q", ok, err, out)
	}
	if got := strings.TrimSpace(readFile(t, s, "ui-pwd")); got != feat {
		t.Errorf("the git UI ran in %s, want %s", got, feat)
	}
	if got := out.String(); got != "the git UI\n" {
		t.Errorf("the git UI's terminal showed %q, want only what it said", got)
	}
}

// When wt can't run the git UI (none is configured, or it isn't
// installed), what it says stays on the terminal until a key is pressed,
// since the terminal closes once the command line exits.
func TestGitUICommandKeepsWtsMessageUntilAKey(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	for _, c := range []struct{ gitUI, says string }{
		{"", "no git UI configured"},
		{"no-such-git-ui", "git UI not installed: no-such-git-ui"},
	} {
		gitUIIs(t, s, c.gitUI)
		done, keys, out := gitUICommand(t, s.Dir, demo)
		if ok, err := exited(done, 500*time.Millisecond); ok {
			t.Fatalf("with WT_GIT_UI=%q, the git UI's command line exited at once (%v), saying %q", c.gitUI, err, out)
		}
		if got := out.String(); !strings.Contains(got, c.says) || !strings.HasSuffix(got, "press any key to close") {
			t.Errorf("with WT_GIT_UI=%q, the terminal shows %q, want wt's %q, then that a key closes it", c.gitUI, got, c.says)
		}
		keys.Write([]byte("x"))
		if ok, err := exited(done, 10*time.Second); !ok || err != nil {
			t.Errorf("with WT_GIT_UI=%q, after a key, the git UI's command line: exited %v, %v", c.gitUI, ok, err)
		}
	}
}

// Without a wt to run, there's no command line.
func TestGitUICommandNeedsWt(t *testing.T) {
	sandbox.New(t)
	t.Setenv("WT_BIN", "")
	t.Setenv("PATH", t.TempDir())
	if words, err := wt.New("").GitUICommand("/p/api"); !errors.Is(err, wt.ErrNotFound) {
		t.Errorf("with no wt, GitUICommand = %q, %v; want ErrNotFound", words, err)
	}
}
