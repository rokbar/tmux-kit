package main

import (
	"errors"
	"testing"

	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
)

func TestFindRepoIsTheTopOfTheWorkingFoldersRepo(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	s.Write("demo/sub/x", "")
	if got, err := findRepo(s.Path("demo/sub")); err != nil || got != demo {
		t.Errorf("findRepo(demo/sub) = %q, %v; want %q", got, err, demo)
	}
	s.Write("plain/x", "")
	if _, err := findRepo(s.Path("plain")); !errors.Is(err, errNotInRepo) {
		t.Errorf("findRepo(plain) error = %v, want errNotInRepo", err)
	}
}
