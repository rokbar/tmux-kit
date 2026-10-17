package wt_test

import (
	"errors"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
	"github.com/rokbar/tmux-kit/wtui/internal/wt"
)

// list runs the client's list operation in dir; the test fails if it errors.
func list(t *testing.T, dir string) []wt.Worktree {
	t.Helper()
	rows, err := wt.New("").List(dir)
	if err != nil {
		t.Fatalf("List(%s): %v", dir, err)
	}
	return rows
}

func sameRows(t *testing.T, got, want []wt.Worktree) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows:\n got %+v\nwant %+v", got, want)
	}
}

func TestListsTheMainCloneAlone(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	sameRows(t, list(t, demo), []wt.Worktree{
		{Path: demo, Branch: "main", State: wt.Clean, MainClone: true},
	})
}

// Made with plain git (as some of test/wt-ls.sh's are), so this runs without
// tmux, which wt needs to open a worktree.
func TestListsEveryRowState(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")

	clean := s.Path("demo--clean")
	s.Git(demo, "worktree", "add", "-q", clean, "-b", "clean")

	// dirty, with a dev URL file, of which only the first line counts
	dirty := s.Path("demo--dirty")
	s.Git(demo, "worktree", "add", "-q", dirty, "-b", "dirty")
	s.Write("demo--dirty/wip.txt", "wip\n")
	s.Write("demo--dirty/.devurl", "http://localhost:4100\nhttp://localhost:9999\n")

	// missing: its folder deleted by hand
	gone := s.Path("demo--gone")
	s.Git(demo, "worktree", "add", "-q", gone, "-b", "gone")
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	// stopped on a conflict mid-rebase, so detached and dirty
	rebase := s.Path("demo--rebase")
	s.Git(demo, "worktree", "add", "-q", rebase, "-b", "rebase")
	s.Write("demo--rebase/f", "theirs\n")
	s.Git(rebase, "commit", "-qam", "rebase side")
	s.Write("demo/f", "ours\n")
	s.Git(demo, "commit", "-qam", "main side")
	if out, err := exec.Command("git", "-C", rebase, "rebase", "main").CombinedOutput(); err == nil {
		t.Fatalf("rebase didn't stop on its conflict:\n%s", out)
	}

	want := []wt.Worktree{
		{Path: demo, Branch: "main", State: wt.Clean, MainClone: true},
		{Path: clean, Branch: "clean", State: wt.Clean},
		{Path: dirty, Branch: "dirty", State: wt.Dirty, DevURL: "http://localhost:4100"},
		{Path: gone, State: wt.Missing},
		{Path: rebase, State: wt.Dirty},
	}
	sameRows(t, list(t, demo), want)
	// the same from inside a worktree: still the main clone first
	sameRows(t, list(t, dirty), want)
}

// A wt from after wtui: the real porcelain, with fields added at the end.
func TestIgnoresExtraTrailingFields(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	feat := s.Path("demo--feat")
	s.Git(demo, "worktree", "add", "-q", feat, "-b", "feat")
	t.Setenv("WT_BIN", s.Script("bin/newer-wt",
		`'`+s.Wt+`' "$@" | awk '{ print $0 "\tadded\tlater" }'`))
	sameRows(t, list(t, demo), []wt.Worktree{
		{Path: demo, Branch: "main", State: wt.Clean, MainClone: true},
		{Path: feat, Branch: "feat", State: wt.Clean},
	})
}

// git must never stop wt to ask for credentials: nobody would see the prompt.
func TestWtRunsWithGitTerminalPromptOff(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	t.Setenv("WT_BIN", s.Script("bin/wt",
		`echo "$GIT_TERMINAL_PROMPT" >'`+s.Path("prompt")+`'; exec '`+s.Wt+`' "$@"`))
	list(t, demo)
	got, err := os.ReadFile(s.Path("prompt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "0\n" {
		t.Errorf("wt ran with GIT_TERMINAL_PROMPT=%q, want 0", strings.TrimSpace(string(got)))
	}
}

// The tmux server's PATH may lack ~/.local/bin, where install.sh puts both wt
// and wtui, so a wt next to wtui comes before PATH; WT_BIN overrides both.
func TestFindsWtByWtBinThenNextToWtuiThenOnPath(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	// each candidate wt notes that it ran, then hands over to the real wt
	stub := func(rel, name string) string {
		return s.Script(rel, `echo `+name+` >'`+s.Path("ran")+`'; exec '`+s.Wt+`' "$@"`)
	}
	t.Setenv("WT_BIN", stub("env/wt", "WT_BIN"))
	stub("app/wt", "next-to-wtui")
	stub("path/wt", "PATH")
	t.Setenv("PATH", s.Path("path")+string(os.PathListSeparator)+os.Getenv("PATH"))

	ran := func(wtui string) string {
		t.Helper()
		if _, err := wt.New(wtui).List(demo); err != nil {
			t.Fatalf("List: %v", err)
		}
		got, err := os.ReadFile(s.Path("ran"))
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(got))
	}
	if got := ran(s.Path("app/wtui")); got != "WT_BIN" {
		t.Errorf("with WT_BIN set, ran the wt from %s, want WT_BIN", got)
	}
	t.Setenv("WT_BIN", "")
	if got := ran(s.Path("app/wtui")); got != "next-to-wtui" {
		t.Errorf("with a wt next to wtui, ran the wt from %s, want next-to-wtui", got)
	}
	if got := ran(s.Path("elsewhere/wtui")); got != "PATH" {
		t.Errorf("with no wt next to wtui, ran the wt from %s, want PATH", got)
	}
}

func TestNoWtIsNotFound(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	t.Setenv("WT_BIN", "")
	t.Setenv("PATH", s.Path("empty"))
	if err := listErr(t, demo); !errors.Is(err, wt.ErrNotFound) {
		t.Errorf("error = %v, want wt.ErrNotFound", err)
	}
}

// WT_BIN names the wt to run: a wrong one is reported, not swapped for another.
func TestWtBinThatIsNoProgramIsNotFound(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	t.Setenv("WT_BIN", s.Path("no/such/wt"))
	if err := listErr(t, demo); !errors.Is(err, wt.ErrNotFound) {
		t.Errorf("error = %v, want wt.ErrNotFound", err)
	}
}

// listErr runs the client's list operation in dir, which should fail.
func listErr(t *testing.T, dir string) error {
	t.Helper()
	rows, err := wt.New("").List(dir)
	if err == nil {
		t.Fatalf("List(%s) = %+v, want an error", dir, rows)
	}
	return err
}

// A wt from before `ls --porcelain` that ignored the flag printed its human list.
func TestHumanListIsTooOld(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	s.Git(demo, "worktree", "add", "-q", s.Path("demo--feat"), "-b", "feat")
	t.Setenv("WT_BIN", s.Script("bin/old-wt", `exec '`+s.Wt+`' ls`))
	if err := listErr(t, demo); !errors.Is(err, wt.ErrTooOld) {
		t.Errorf("error = %v, want wt.ErrTooOld", err)
	}
}

// A wt with no `ls` takes `ls` for a branch and rejects --porcelain.
func TestFailingListIsTooOldAndCarriesWtsMessage(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	t.Setenv("WT_BIN", s.Script("bin/old-wt", `echo 'usage: wt [options] <branch>' >&2; exit 1`))
	err := listErr(t, demo)
	if !errors.Is(err, wt.ErrTooOld) {
		t.Errorf("error = %v, want wt.ErrTooOld", err)
	}
	if got, want := err.Error(), "usage: wt [options] <branch>"; got != want {
		t.Errorf("error text = %q, want wt's stderr %q", got, want)
	}
}

// Worktrees opened by wt, as a user would: each gets a session, and in a JS
// project with a dev script, a dev server whose URL wt records.
func TestListsTheSessionAndDevURLOfWorktreesWtOpened(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	s.Write("demo/package.json", `{"scripts":{"dev":"x"}}`+"\n")
	s.Write("demo/package-lock.json", "{}\n")
	s.Git(demo, "add", ".")
	s.Git(demo, "commit", "-qm", "dev script")

	feat, said := s.NewWorktree(demo, "feat", "-n", "api")
	url := regexp.MustCompile(`dev server on (\S+)`).FindStringSubmatch(said)
	if url == nil {
		t.Fatalf("wt didn't say where feat's dev server is:\n%s", said)
	}
	// missing: its folder deleted by hand, its session still running
	gone, _ := s.NewWorktree(demo, "gone")
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	sameRows(t, list(t, demo), []wt.Worktree{
		{Path: demo, Branch: "main", State: wt.Clean, MainClone: true},
		{Path: feat, Branch: "feat", State: wt.Clean, Session: "api", DevURL: url[1]},
		{Path: gone, State: wt.Missing, Session: "gone"},
	})
}
