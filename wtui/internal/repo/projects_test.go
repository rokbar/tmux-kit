package repo_test

import (
	"errors"
	"io/fs"
	"os"
	"slices"
	"testing"

	"github.com/rokbar/tmux-kit/wtui/internal/repo"
	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
)

// The projects in a projects root are its folders, by name whatever their
// case: a main clone is a git project, and any other folder a plain one. A
// worktree belongs to its main clone's project, so it isn't one. Nor are
// hidden folders, or files. A link to a folder is that folder, under the
// link's name. A folder git can't look in (a bare repo) is a plain folder.
func TestTheProjectsInARootAreItsFoldersButWorktrees(t *testing.T) {
	s := sandbox.New(t)
	root := s.Path("Projects")
	api := s.Repo("Projects/api")
	s.Git(api, "worktree", "add", "-q", "-b", "fix", s.Path("Projects/api--fix"))
	zed := s.Repo("Projects/Zed")
	s.Git(zed, "worktree", "add", "-q", "-b", "spike", s.Path("elsewhere/zed--spike"))
	s.Write("Projects/my.notes/todo", "")
	s.Write("Projects/.trash/old/f", "")
	s.Write("Projects/README", "")
	s.Repo("tools")
	if err := os.Symlink(s.Path("tools"), s.Path("Projects/tools")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(s.Path("elsewhere/zed--spike"), s.Path("Projects/spike")); err != nil {
		t.Fatal(err)
	}
	s.Git(s.Dir, "init", "-q", "--bare", s.Path("Projects/vault.git"))

	got, err := (repo.Lookup{}).Projects(root)
	want := []repo.Project{
		{Path: api, Git: true},
		{Path: s.Path("Projects/my.notes")},
		{Path: s.Path("Projects/tools"), Git: true},
		{Path: s.Path("Projects/vault.git")},
		{Path: zed, Git: true},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("Projects(root) = %+v, %v\nwant %+v", got, err, want)
	}
}

// A folder inside a repo that isn't the top of one, as every folder is in a
// projects root that's in a repo itself, is a plain folder.
func TestAFolderInsideARepoIsAPlainFolder(t *testing.T) {
	s := sandbox.New(t)
	s.Repo("home")
	s.Write("home/Projects/notes/f", "")
	got, err := (repo.Lookup{}).Projects(s.Path("home/Projects"))
	if want := []repo.Project{{Path: s.Path("home/Projects/notes")}}; err != nil || !slices.Equal(got, want) {
		t.Errorf("Projects(home/Projects) = %+v, %v; want %+v", got, err, want)
	}
}

// An empty root has no projects; a missing one is a failure that says so.
func TestAnEmptyOrMissingRootHasNoProjects(t *testing.T) {
	s := sandbox.New(t)
	s.Write("empty/.hidden/f", "")
	if got, err := (repo.Lookup{}).Projects(s.Path("empty")); err != nil || len(got) > 0 {
		t.Errorf("Projects(empty) = %+v, %v; want none", got, err)
	}
	if got, err := (repo.Lookup{}).Projects(s.Path("gone")); !errors.Is(err, fs.ErrNotExist) || len(got) > 0 {
		t.Errorf("Projects(gone) = %+v, %v; want none, and that it doesn't exist", got, err)
	}
}
