package wt_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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

	rebase := midRebase(t, s, demo, "rebase")

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

// midRebase makes a worktree of mainClone (a sandbox.Repo, on branch main),
// next to it on a new branch, stopped on a conflict mid-rebase onto main (so
// detached and dirty), and returns its path. It commits to main.
func midRebase(t *testing.T, s *sandbox.Sandbox, mainClone, branch string) string {
	t.Helper()
	dir := mainClone + "--" + branch
	s.Git(mainClone, "worktree", "add", "-q", dir, "-b", branch)
	for _, side := range []struct{ dir, f string }{{dir, "theirs"}, {mainClone, "ours"}} {
		if err := os.WriteFile(filepath.Join(side.dir, "f"), []byte(side.f+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		s.Git(side.dir, "commit", "-qam", side.f)
	}
	if out, err := exec.Command("git", "-C", dir, "rebase", "main").CombinedOutput(); err == nil {
		t.Fatalf("rebase didn't stop on its conflict:\n%s", out)
	}
	return dir
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

// wtui lists worktrees every few seconds, so the git status wt runs for it
// must take no optional lock (refreshing the index): the user's own git
// would find the index locked.
func TestListRunsWithOptionalLocksOff(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	t.Setenv("GIT_OPTIONAL_LOCKS", "1")
	t.Setenv("WT_BIN", s.Script("bin/wt",
		`echo "$GIT_OPTIONAL_LOCKS" >'`+s.Path("locks")+`'; exec '`+s.Wt+`' "$@"`))
	list(t, demo)
	got, err := os.ReadFile(s.Path("locks"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "0\n" {
		t.Errorf("wt ls ran with GIT_OPTIONAL_LOCKS=%q, want 0", strings.TrimSpace(string(got)))
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

// The sidebar lists every project it shows, wherever wtui runs: run in a
// project's main clone, the list is that project's, from a folder outside it.
func TestListsAProjectFromOutsideIt(t *testing.T) {
	s := sandbox.New(t)
	api := s.Repo("api")
	fix, _ := s.NewWorktree(api, "fix")
	docs, _ := s.NewWorktree(s.Repo("docs"), "draft")
	s.RunWt(docs, "-d", "main")
	t.Chdir(s.Path("home"))

	sameRows(t, list(t, api), []wt.Worktree{
		{Path: api, Branch: "main", State: wt.Clean, MainClone: true},
		{Path: fix, Branch: "fix", State: wt.Clean, Session: "fix"},
	})
	sameRows(t, list(t, s.Path("docs")), []wt.Worktree{
		{Path: s.Path("docs"), Branch: "main", State: wt.Clean, Session: "main", MainClone: true},
		{Path: docs, Branch: "draft", State: wt.Clean, Session: "draft"},
	})
}

// Opening a worktree that has no session starts one there, as wt names it by
// default (its branch, else its folder), so the next list shows it. This works
// for the main clone and a worktree mid-rebase too, which has no branch.
func TestOpenStartsASessionInTheWorktree(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	demo := s.Repo("demo")
	feat := s.Path("demo--feat")
	s.Git(demo, "worktree", "add", "-q", feat, "-b", "feat")
	rebase := midRebase(t, s, demo, "rebase")

	c := wt.New("")
	for _, path := range []string{feat, rebase, demo} {
		if err := c.Open(demo, path); err != nil {
			t.Fatalf("Open(%s): %v", path, err)
		}
	}
	sameRows(t, list(t, demo), []wt.Worktree{
		{Path: demo, Branch: "main", State: wt.Clean, Session: "main", MainClone: true},
		{Path: feat, Branch: "feat", State: wt.Clean, Session: "feat"},
		{Path: rebase, State: wt.Dirty, Session: "demo--rebase"},
	})
}

// A failed open reads as what wt said, e.g. for a worktree whose folder is gone.
func TestFailedOpenCarriesWtsMessage(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	gone := s.Path("demo--gone")
	s.Git(demo, "worktree", "add", "-q", gone, "-b", "gone")
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	// what wt itself says when asked to open it
	cmd := exec.Command(s.Wt, "-d", gone)
	cmd.Dir = demo
	var said strings.Builder
	cmd.Stderr = &said
	if cmd.Run() == nil {
		t.Fatalf("wt -d %s worked, want it to fail", gone)
	}

	err := wt.New("").Open(demo, gone)
	if err == nil {
		t.Fatalf("Open(%s) worked, want it to fail", gone)
	}
	if want := strings.TrimSpace(said.String()); err.Error() != want || want == "" {
		t.Errorf("error = %q, want wt's message %q", err, want)
	}
}

// recordWt points WT_BIN at the real wt, wrapped so that what it prints is
// also kept. The returned func gives what the last wt call printed on stdout
// and on stderr, trimmed, so a test can compare the client with wt itself.
func recordWt(t *testing.T, s *sandbox.Sandbox) func() (stdout, stderr string) {
	t.Helper()
	out, errOut := s.Path("wt.out"), s.Path("wt.err")
	t.Setenv("WT_BIN", s.Script("bin/recording-wt",
		`'`+s.Wt+`' "$@" >'`+out+`' 2>'`+errOut+`'; status=$?
cat '`+out+`'; cat '`+errOut+`' >&2; exit $status`))
	return func() (string, string) {
		t.Helper()
		read := func(p string) string {
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			return strings.TrimSpace(string(b))
		}
		return read(out), read(errOut)
	}
}

// A removal, of a worktree or of a missing one (which wt only unregisters),
// says what wt said it did, e.g. that it kept the branch.
func TestRemoveCarriesWtsMessage(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	feat := s.Path("demo--feat")
	s.Git(demo, "worktree", "add", "-q", feat, "-b", "feat")
	gone := s.Path("demo--gone")
	s.Git(demo, "worktree", "add", "-q", gone, "-b", "gone")
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	wtSaid := recordWt(t, s)

	// gone first: removing feat prunes every missing worktree
	for _, path := range []string{gone, feat} {
		said, err := wt.New("").Remove(demo, path, false)
		if err != nil {
			t.Fatalf("Remove(%s): %v", path, err)
		}
		if want, _ := wtSaid(); said != want || want == "" {
			t.Errorf("Remove(%s) said %q, want wt's message %q", path, said, want)
		}
	}
	sameRows(t, list(t, demo), []wt.Worktree{
		{Path: demo, Branch: "main", State: wt.Clean, MainClone: true},
	})
}

// wt refusing because removing would lose work (here, uncommitted changes) is
// a safety refusal, with wt's reasons as its text. Forcing gets past it.
func TestDirtyWorktreeIsASafetyRefusalThatForceGetsPast(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	feat := s.Path("demo--feat")
	s.Git(demo, "worktree", "add", "-q", feat, "-b", "feat")
	s.Write("demo--feat/wip.txt", "wip\n")
	wtSaid := recordWt(t, s)
	c := wt.New("")

	_, err := c.Remove(demo, feat, false)
	if !errors.Is(err, wt.ErrSafetyRefusal) {
		t.Fatalf("Remove(%s) = %v, want a wt.ErrSafetyRefusal", feat, err)
	}
	if _, want := wtSaid(); err.Error() != want || !strings.Contains(want, "wip.txt") {
		t.Errorf("refusal = %q, want wt's reasons %q", err, want)
	}
	if rows := list(t, demo); len(rows) != 2 {
		t.Errorf("after the refusal, rows = %+v, want feat still there", rows)
	}

	if _, err := c.Remove(demo, feat, true); err != nil {
		t.Fatalf("forced Remove(%s): %v", feat, err)
	}
	sameRows(t, list(t, demo), []wt.Worktree{
		{Path: demo, Branch: "main", State: wt.Clean, MainClone: true},
	})
}

// Failures that forcing can't get past, such as the main clone or a locked
// worktree, are not safety refusals. Their text is what wt said.
func TestOtherRemoveFailuresAreNoSafetyRefusal(t *testing.T) {
	s := sandbox.New(t)
	demo := s.Repo("demo")
	locked := s.Path("demo--locked")
	s.Git(demo, "worktree", "add", "-q", locked, "-b", "locked")
	s.Git(demo, "worktree", "lock", locked)
	s.Write("demo--locked/wip.txt", "wip\n") // a lock comes before the safety checks
	wtSaid := recordWt(t, s)
	c := wt.New("")

	for _, call := range []struct {
		path  string
		force bool
	}{{demo, false}, {locked, false}, {locked, true}} {
		_, err := c.Remove(demo, call.path, call.force)
		if err == nil {
			t.Fatalf("Remove(%s, force %v) worked, want it to fail", call.path, call.force)
		}
		if errors.Is(err, wt.ErrSafetyRefusal) {
			t.Errorf("Remove(%s, force %v) = %v, a safety refusal; want another failure", call.path, call.force, err)
		}
		if _, want := wtSaid(); err.Error() != want || want == "" {
			t.Errorf("Remove(%s, force %v) error = %q, want wt's message %q", call.path, call.force, err, want)
		}
	}
	if rows := list(t, demo); len(rows) != 2 {
		t.Errorf("rows = %+v, want the main clone and the locked worktree still there", rows)
	}
}
