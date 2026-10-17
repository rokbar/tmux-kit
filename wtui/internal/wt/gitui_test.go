package wt_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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

// The git UI runs in the worktree at the path it's given, whatever folder wt
// runs in.
func TestGitUIRunsTheGitUIInTheGivenWorktree(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	feat := s.Path("demo--feat")
	s.Git(demo, "worktree", "add", "-q", feat, "-b", "feat")
	s.Script("stubs/stub-ui", `pwd -P >'`+s.Path("ui-pwd")+`'`)
	t.Setenv("PATH", s.Path("stubs")+string(os.PathListSeparator)+os.Getenv("PATH"))
	gitUIIs(t, s, "stub-ui")

	for _, c := range []struct{ dir, path string }{{demo, feat}, {feat, demo}} {
		if err := runOnTerminal(t, func(done tea.ExecCallback) tea.Cmd { return wt.New("").GitUI(c.dir, c.path, done) }); err != nil {
			t.Fatalf("GitUI(%s, %s): %v", c.dir, c.path, err)
		}
		if got := strings.TrimSpace(readFile(t, s, "ui-pwd")); got != c.path {
			t.Errorf("GitUI(%s, %s): the git UI ran in %s, want %s", c.dir, c.path, got, c.path)
		}
	}
}

// With no git UI configured, the error's text is wt's explanation of how to
// configure one.
func TestGitUINotConfiguredCarriesWtsMessage(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	// what wt itself says when asked to run it
	cmd := exec.Command(s.Wt, "git-ui", demo)
	var said strings.Builder
	cmd.Stderr = &said
	if cmd.Run() == nil {
		t.Fatal("wt git-ui worked with no git UI configured, want it to fail")
	}
	want := strings.TrimSpace(said.String())
	if !strings.Contains(want, "no git UI configured") {
		t.Fatalf("wt git-ui failed for another reason: %s", want)
	}

	err := runOnTerminal(t, func(done tea.ExecCallback) tea.Cmd { return wt.New("").GitUI(demo, demo, done) })
	if err == nil {
		t.Fatal("GitUI worked with no git UI configured, want it to fail")
	}
	if err.Error() != want {
		t.Errorf("error = %q, want wt's message %q", err, want)
	}
}
