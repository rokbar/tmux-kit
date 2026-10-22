package repo_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/rokbar/tmux-kit/wtui/internal/repo"
	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
)

// A folder in a main clone or one of its worktrees, at any depth, belongs to
// the main clone.
func TestMainCloneOfAFolderInARepo(t *testing.T) {
	s := sandbox.New(t)
	main := s.Repo("api")
	s.Git(main, "worktree", "add", "-q", "-b", "fix", s.Path("api--fix"))
	s.Write("api--fix/src/f", "")
	for _, dir := range []string{main, s.Path("api/.git/.."), s.Path("api--fix"), s.Path("api--fix/src")} {
		if got, err := (repo.Lookup{}).MainClone(dir); err != nil || got != main {
			t.Errorf("MainClone(%s) = %q, %v; want %s", dir, got, err, main)
		}
	}
}

func TestAFolderOutsideGitHasNoMainClone(t *testing.T) {
	s := sandbox.New(t)
	s.Write("notes/f", "")
	if got, err := (repo.Lookup{}).MainClone(s.Path("notes")); err != nil || got != "" {
		t.Errorf("MainClone(notes) = %q, %v; want none", got, err)
	}
}

// A folder git can't look in (here, one that's gone) is a failure, with
// git's reason.
func TestALookupThatFailsSaysWhy(t *testing.T) {
	s := sandbox.New(t)
	gone := s.Path("gone")
	_, err := (repo.Lookup{}).MainClone(gone)
	if err == nil || !strings.Contains(err.Error(), gone) || !strings.Contains(err.Error(), "git") {
		t.Errorf("MainClone(gone) fails with %v, want git's reason naming the folder", err)
	}
}

// A folder in a git repo is in the main clone or worktree whose folder holds
// it, the nearest: even a worktree inside the main clone's folder. With it
// come the folders of the repo's main clone and every worktree, in git's
// order.
func TestTheWorktreeAFolderIsIn(t *testing.T) {
	s := sandbox.New(t)
	main := s.Repo("api")
	fix, nested := s.Path("api--fix"), s.Path("api/.worktrees/spike")
	s.Git(main, "worktree", "add", "-q", "-b", "fix", fix)
	s.Git(main, "worktree", "add", "-q", "-b", "spike", nested)
	s.Write("api--fix/src/f", "")
	s.Write("api/docs/f", "")
	all := []string{main, fix, nested}
	for dir, want := range map[string]string{
		main: main, s.Path("api/docs"): main, fix: fix, s.Path("api--fix/src"): fix, nested: nested,
	} {
		in, got, err := (repo.Lookup{}).Worktrees(dir)
		if err != nil || in != want || !slices.Equal(got, all) {
			t.Errorf("Worktrees(%s) = %q, %q, %v; want %q, %q", dir, in, got, err, want, all)
		}
	}
}

func TestAFolderOutsideGitIsInNoWorktree(t *testing.T) {
	s := sandbox.New(t)
	s.Write("notes/f", "")
	if in, all, err := (repo.Lookup{}).Worktrees(s.Path("notes")); err != nil || in != "" || all != nil {
		t.Errorf("Worktrees(notes) = %q, %q, %v; want none", in, all, err)
	}
}
