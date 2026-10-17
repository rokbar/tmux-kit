package detail_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rokbar/tmux-kit/wtui/internal/detail"
	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
)

// read reads the details of the worktree at path; the test fails if it errors.
func read(t *testing.T, path string) detail.Details {
	t.Helper()
	d, err := detail.Reader{}.Read(path)
	if err != nil {
		t.Fatalf("Read(%s): %v", path, err)
	}
	return d
}

// Each changed file is a line, as git's short status shows it: staged,
// changed and untracked.
func TestChangedFiles(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	feat := s.Path("demo--feat")
	s.Git(demo, "worktree", "add", "-q", feat, "-b", "feat")
	if got := read(t, feat).Changes; len(got) != 0 {
		t.Errorf("a clean worktree's changes = %q, want none", got)
	}

	s.Write("demo--feat/f", "changed\n")
	s.Write("demo--feat/staged.txt", "staged\n")
	s.Git(feat, "add", "staged.txt")
	s.Write("demo--feat/new.txt", "new\n")
	want := []string{" M f", "A  staged.txt", "?? new.txt"}
	if got := read(t, feat).Changes; !slices.Equal(got, want) {
		t.Errorf("changes = %q, want %q", got, want)
	}
	// the worktree's own: the main clone has none
	if got := read(t, demo).Changes; len(got) != 0 {
		t.Errorf("the main clone's changes = %q, want none", got)
	}
}

// The last 10 commits, newest first, a line each as `git log --oneline`
// shows them: the short hash, then the subject.
func TestAtMostTheLastTenCommits(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo") // its first commit is "init"
	feat := s.Path("demo--feat")
	s.Git(demo, "worktree", "add", "-q", feat, "-b", "feat")
	if got := read(t, feat).Commits; len(got) != 1 || !oneline(got[0], "init") {
		t.Errorf("commits = %q, want just init", got)
	}

	for i := 1; i <= 11; i++ {
		s.Git(feat, "commit", "-q", "--allow-empty", "-m", fmt.Sprintf("c%d", i))
	}
	got := read(t, feat).Commits
	want := []string{"c11", "c10", "c9", "c8", "c7", "c6", "c5", "c4", "c3", "c2"}
	if len(got) != len(want) {
		t.Fatalf("commits = %q, want the last 10: %q", got, want)
	}
	for i := range want {
		if !oneline(got[i], want[i]) {
			t.Errorf("commit %d = %q, want a short hash then %q", i+1, got[i], want[i])
		}
	}
}

// withRemote gives the repo of mainClone (a sandbox.Repo) a remote, origin: a
// bare repo in the sandbox folder, which main is pushed to and tracks.
func withRemote(s *sandbox.Sandbox, mainClone string) string {
	remote := s.Path("remote.git")
	s.Git(s.Dir, "init", "-q", "--bare", remote)
	s.Git(mainClone, "remote", "add", "origin", remote)
	s.Git(mainClone, "push", "-q", "-u", "origin", "main")
	return remote
}

// How far HEAD is ahead of and behind its upstream, as the worktree's repo
// last fetched it: wtui never fetches.
func TestAheadAndBehindItsUpstream(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	remote := withRemote(s, demo)
	feat := s.Path("demo--feat")
	s.Git(demo, "worktree", "add", "-q", feat, "-b", "feat")
	s.Git(feat, "push", "-q", "-u", "origin", "feat")
	d := read(t, feat)
	if d.Upstream != "origin/feat" || d.Ahead != 0 || d.Behind != 0 {
		t.Errorf("just pushed: upstream %q, %d ahead, %d behind; want origin/feat, 0, 0", d.Upstream, d.Ahead, d.Behind)
	}

	// pushed, then dropped: 1 behind; then 2 new: 2 ahead
	s.Git(feat, "commit", "-q", "--allow-empty", "-m", "pushed")
	s.Git(feat, "push", "-q")
	s.Git(feat, "reset", "-q", "--hard", "HEAD~1")
	s.Git(feat, "commit", "-q", "--allow-empty", "-m", "local 1")
	s.Git(feat, "commit", "-q", "--allow-empty", "-m", "local 2")
	// and someone else pushes to feat, which isn't fetched
	other := s.Path("other")
	s.Git(s.Dir, "clone", "-q", "-b", "feat", remote, other)
	s.Git(other, "commit", "-q", "--allow-empty", "-m", "theirs")
	s.Git(other, "push", "-q")

	d = read(t, feat)
	if d.Upstream != "origin/feat" || d.Ahead != 2 || d.Behind != 1 {
		t.Errorf("upstream %q, %d ahead, %d behind; want origin/feat, 2, 1", d.Upstream, d.Ahead, d.Behind)
	}
}

// A branch with no upstream isn't pushed, even when the repo has a remote,
// and neither is a detached HEAD (as mid-rebase): no upstream, and no error.
func TestNotPushedWithoutAnUpstream(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	withRemote(s, demo)
	feat := s.Path("demo--feat")
	s.Git(demo, "worktree", "add", "-q", feat, "-b", "feat")
	s.Git(feat, "commit", "-q", "--allow-empty", "-m", "local")
	detached := s.Path("demo--detached")
	s.Git(demo, "worktree", "add", "-q", "--detach", detached)

	for _, path := range []string{feat, detached} {
		d := read(t, path)
		if d.Upstream != "" || d.Ahead != 0 || d.Behind != 0 {
			t.Errorf("%s: upstream %q, %d ahead, %d behind; want none: not pushed", path, d.Upstream, d.Ahead, d.Behind)
		}
		if len(d.Commits) == 0 {
			t.Errorf("%s: no commits, want them read all the same", path)
		}
	}
}

// git must never stop to ask for credentials: nobody would see the prompt.
func TestGitRunsWithGitTerminalPromptOff(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	withRemote(s, demo) // so every git call runs, upstream ones included
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	s.Script("bin/git", `echo "$GIT_TERMINAL_PROMPT" >>'`+s.Path("prompt")+`'; exec '`+realGit+`' "$@"`)
	t.Setenv("PATH", s.Path("bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GIT_TERMINAL_PROMPT", "1")

	read(t, demo)
	got, err := os.ReadFile(s.Path("prompt"))
	if err != nil {
		t.Fatal(err)
	}
	if prompts := lines(string(got)); slices.ContainsFunc(prompts, func(p string) bool { return p != "0" }) {
		t.Errorf("git ran with GIT_TERMINAL_PROMPT set to %q, want 0 every time", prompts)
	}
}

// The user may be running git in the worktree at the same time, so reading it
// takes no lock git can do without: git status would otherwise lock the index
// to write back what it learned about files whose timestamps changed.
func TestReadingLeavesTheIndexAlone(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	feat := s.Path("demo--feat")
	s.Git(demo, "worktree", "add", "-q", feat, "-b", "feat")
	index := strings.TrimSpace(s.Git(feat, "rev-parse", "--path-format=absolute", "--git-path", "index"))
	// the same content, a day older: git has to look again, and would note it
	day := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(filepath.Join(feat, "f"), day, day); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}

	if got := read(t, feat).Changes; len(got) != 0 {
		t.Errorf("changes = %q, want none: only a timestamp changed", got)
	}
	if after, err := os.ReadFile(index); err != nil || !bytes.Equal(after, before) {
		t.Errorf("reading the worktree rewrote its index (%v)", err)
	}
}

// lines is text's lines.
func lines(text string) []string { return strings.Split(strings.TrimSuffix(text, "\n"), "\n") }

// The user's git config can decorate or colour git's output when it isn't
// going to a terminal. The pane shows the lines plain, whatever it says.
func TestLinesArePlainWhateverTheUsersConfigSays(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	s.Write("demo/f", "changed\n")
	for _, kv := range [][2]string{{"log.decorate", "full"}, {"color.ui", "always"}} {
		s.Git(demo, "config", kv[0], kv[1])
	}
	d := read(t, demo)
	if want := []string{" M f"}; !slices.Equal(d.Changes, want) {
		t.Errorf("changes = %q, want %q", d.Changes, want)
	}
	if len(d.Commits) != 1 || !oneline(d.Commits[0], "init") {
		t.Errorf("commits = %q, want just init, plain", d.Commits)
	}
}

// A git that fails without saying why is reported by its exit status.
func TestGitFailingSilentlyIsReportedByItsExitStatus(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	s.Script("bin/git", "exit 3")
	t.Setenv("PATH", s.Path("bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, err := detail.Reader{}.Read(demo)
	if err == nil || err.Error() != "git status: exit status 3" {
		t.Errorf("error = %v, want git status: exit status 3", err)
	}
}

// oneline reports whether line is a commit as `git log --oneline` shows it,
// with subject.
func oneline(line, subject string) bool {
	return regexp.MustCompile(`^[0-9a-f]{7,} ` + regexp.QuoteMeta(subject) + `$`).MatchString(line)
}
