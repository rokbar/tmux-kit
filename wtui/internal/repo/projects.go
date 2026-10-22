package repo

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// Project is a folder in the projects root that opens as a project.
type Project struct {
	Path string
	// Git is whether it's a git repo's main clone, whose session wt opens;
	// any other folder is a plain one, whose session t opens.
	Git bool
}

// Projects are the folders in root, the projects root, that open as
// projects, by name whatever its case: every folder in it but hidden ones
// and worktrees, which belong to their main clones' projects. A link to a
// folder is that folder, under the link's name. A folder at the top of a
// main clone is a git project; any other is a plain folder, among them one
// inside a repo but not at its top, and one git can't look in (e.g. a bare
// repo). If root can't be read (e.g. it's missing: fs.ErrNotExist), that's
// the error.
func (Lookup) Projects(root string) ([]Project, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var folders []string
	for _, e := range entries {
		path := filepath.Join(root, e.Name())
		if info, err := os.Stat(path); strings.HasPrefix(e.Name(), ".") || err != nil || !info.IsDir() {
			continue
		}
		folders = append(folders, path)
	}
	// a git lookup for each folder, a few at a time, since the user waits
	// for the list
	kinds := make([]kind, len(folders))
	slots := make(chan struct{}, lookups)
	var wg sync.WaitGroup
	for i, folder := range folders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			kinds[i] = kindOf(folder)
			<-slots
		}()
	}
	wg.Wait()
	var projects []Project
	for i, folder := range folders {
		if kinds[i] != worktree {
			projects = append(projects, Project{Path: folder, Git: kinds[i] == mainClone})
		}
	}
	slices.SortStableFunc(projects, func(a, b Project) int {
		return cmp.Compare(strings.ToLower(filepath.Base(a.Path)), strings.ToLower(filepath.Base(b.Path)))
	})
	return projects, nil
}

// lookups is how many folders' git lookups Projects runs at once.
const lookups = 8

// kind is what a folder is to git.
type kind int

const (
	plain     kind = iota // not at the top of a main clone or worktree
	mainClone             // at the top of a main clone
	worktree              // at the top of a worktree
)

// kindOf is what folder is to git: the top of a main clone or worktree is
// where git says the work tree's top is, and a worktree's git folder isn't
// the repo's common one. Where git can't look, it's a plain folder.
func kindOf(folder string) kind {
	out, err := git(folder, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-dir", "--git-common-dir")
	f := strings.Split(out, "\n")
	physical, linkErr := filepath.EvalSymlinks(folder) // as git says it
	switch {
	case err != nil || linkErr != nil || len(f) != 3 || f[0] != physical:
		return plain
	case f[1] != f[2]:
		return worktree
	}
	return mainClone
}
