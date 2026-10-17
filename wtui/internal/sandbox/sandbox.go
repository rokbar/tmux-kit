// Package sandbox builds throwaway git repos and worktrees for wtui's tests,
// with tmux-kit's real bin/wt. It isolates a test the way test/lib/sandbox.sh
// does: a temp folder, a private tmux socket folder, HOME, WT_CONFIG=/dev/null,
// no TMUX, and no user or system git config.
//
// It changes the test process's environment, so tests that use it can't run
// in parallel.
package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Sandbox is one test's private folder and environment.
type Sandbox struct {
	// Dir is the physical path of the temp folder that holds HOME, the tmux
	// socket folder and the repos, as git reports it (macOS: /private/var/...).
	Dir string
	// Wt is tmux-kit's real bin/wt, which WT_BIN points at.
	Wt string
	t  testing.TB
}

// New isolates the test and returns its sandbox. Everything is undone, and a
// tmux server started in it killed, when the test ends.
func New(t testing.TB) *Sandbox {
	t.Helper()
	// os.MkdirTemp, not t.TempDir: a path with the test's name in it can push
	// the tmux socket past the ~104-byte limit on macOS.
	tmp, err := os.MkdirTemp("", "wtui")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })
	dir, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatal(err)
	}
	s := &Sandbox{Dir: dir, Wt: realWt(t), t: t}

	for _, k := range []string{
		"TMUX", "TMUX_PANE", "XDG_CONFIG_HOME", "GIT_DIR", "GIT_WORK_TREE",
		"GIT_INDEX_FILE", "GIT_CONFIG_GLOBAL", "FZF_DEFAULT_OPTS", "FZF_DEFAULT_OPTS_FILE",
	} {
		t.Setenv(k, "") // restores the old value when the test ends
		os.Unsetenv(k)
	}
	for k, v := range map[string]string{
		"TMUX_TMPDIR": dir, "HOME": filepath.Join(dir, "home"), "WT_CONFIG": "/dev/null",
		"WT_BIN": s.Wt, "GIT_CONFIG_NOSYSTEM": "1",
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t",
	} {
		t.Setenv(k, v)
	}
	// panes swallow keys, so wt's `npm install && npm run dev` never runs
	s.Write("home/.tmux.conf", "set -g default-command 'cat >/dev/null'\n")

	// Registered after the temp folder's removal, so it runs before it: the
	// socket has to still be there for kill-server to reach the server.
	if tmux, err := exec.LookPath("tmux"); err == nil {
		t.Cleanup(func() {
			kill := exec.Command(tmux, "kill-server")
			kill.Env = append(os.Environ(), "TMUX_TMPDIR="+dir)
			kill.Run() // fails when no server was started
		})
	}
	return s
}

// realWt finds tmux-kit's bin/wt by going up from the test's folder (its
// package's, somewhere in tmux-kit's wtui/).
func realWt(t testing.TB) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		wt := filepath.Join(dir, "bin", "wt")
		if _, err := os.Stat(filepath.Join(dir, "wtui", "go.mod")); err == nil {
			if _, err := os.Stat(wt); err != nil {
				t.Fatalf("sandbox: tmux-kit's wt: %v", err)
			}
			return wt
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("sandbox: the test's folder isn't in tmux-kit (no wtui/go.mod above it)")
		}
		dir = parent
	}
}

// NeedTmux skips the test when tmux isn't installed.
func (s *Sandbox) NeedTmux() {
	s.t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		s.t.Skip("tmux not on PATH: this test needs a tmux server for sessions")
	}
}

// Path is a path inside the sandbox.
func (s *Sandbox) Path(rel string) string { return filepath.Join(s.Dir, rel) }

// Write writes a file at a path inside the sandbox, creating its folders.
func (s *Sandbox) Write(rel, content string) {
	s.t.Helper()
	p := s.Path(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

// Script writes an executable sh script with body at a path inside the
// sandbox, e.g. a stub program, and returns its path.
func (s *Sandbox) Script(rel, body string) string {
	s.t.Helper()
	s.Write(rel, "#!/bin/sh\n"+body+"\n")
	p := s.Path(rel)
	if err := os.Chmod(p, 0o755); err != nil {
		s.t.Fatal(err)
	}
	return p
}

// Repo makes a repo in the sandbox folder name, on branch main with one commit
// of a file f, and returns its path.
func (s *Sandbox) Repo(name string) string {
	s.t.Helper()
	s.Write(name+"/f", "init\n")
	dir := s.Path(name)
	s.Git(dir, "init", "-q", "-b", "main")
	s.Git(dir, "add", ".")
	s.Git(dir, "commit", "-qm", "init")
	return dir
}

// Git runs git in dir and returns its output; the test fails if git does.
func (s *Sandbox) Git(dir string, args ...string) string {
	s.t.Helper()
	return s.run(dir, "git", args...)
}

// NewWorktree has the real wt open branch from main, a main clone, in a new
// worktree with a detached session (`wt -d [flags] <branch>`), as a user
// would. It returns the worktree's path and what wt said. wt needs tmux for
// the session, so the test skips without it.
func (s *Sandbox) NewWorktree(main, branch string, flags ...string) (dir, said string) {
	s.t.Helper()
	s.NeedTmux()
	said = s.RunWt(main, append(append([]string{"-d"}, flags...), branch)...)
	// where wt puts it: next to the main clone, as <repo>--<branch, / -> ->
	dir = filepath.Join(filepath.Dir(main), filepath.Base(main)+"--"+strings.ReplaceAll(branch, "/", "-"))
	if _, err := os.Stat(dir); err != nil {
		s.t.Fatalf("wt didn't make %s: %v\n%s", dir, err, said)
	}
	return dir, said
}

// RunWt runs the real wt in dir and returns its output; the test fails if wt
// does.
func (s *Sandbox) RunWt(dir string, args ...string) string {
	s.t.Helper()
	return s.run(dir, s.Wt, args...)
}

func (s *Sandbox) run(dir, name string, args ...string) string {
	s.t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		s.t.Fatalf("%s %s (in %s): %v\n%s", name, strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}
