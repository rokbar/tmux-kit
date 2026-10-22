package editor

import (
	"errors"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
)

// stubEditors puts editors named after names, which run body (sh), first on
// PATH. $RECORD is a file they can write to, which stubEditors returns, and
// $FLAG one that's there until the test ends, for editors that run until
// then. TMPDIR is a folder of its own.
func stubEditors(t *testing.T, s *sandbox.Sandbox, body string, names ...string) (record string) {
	t.Helper()
	for _, name := range names {
		s.Script("bin/"+name, body)
	}
	record, flag := s.Path("record"), s.Path("flag")
	s.Write("flag", "")
	t.Cleanup(func() { os.Remove(flag) }) // ends the editors that wait for it
	t.Setenv("PATH", s.Path("bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("RECORD", record)
	t.Setenv("FLAG", flag)
	t.Setenv("TMPDIR", s.Path("tmp"))
	s.Write("tmp/.keep", "")
	return record
}

// recorded is the lines the editors wrote to record.
func recorded(t *testing.T, record string) []string {
	t.Helper()
	b, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("the editor didn't run: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// editor is an Opener whose TMUX_KIT_EDITOR is line.
func editor(line string) Opener {
	return Opener{Editor: func() (string, error) { return line, nil }}
}

// openSoon opens folder with o, and fails the test if that hasn't returned
// within 10 s: it would have waited on something it shouldn't.
func openSoon(t *testing.T, o Opener, folder string) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- o.Open(folder) }()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatalf("Open(%q) hasn't returned after 10 s", folder)
		return nil
	}
}

// The editor runs in the folder, its command line as TMUX_KIT_EDITOR has it
// (words split as prefix e's sh splits them), with the folder as its last
// argument, as it is: no shell sees it. TMUX_KIT_EDITOR in wtui's own
// environment doesn't count; the normal server's does.
func TestRunsTheEditorOnTheFolder(t *testing.T) {
	s := sandbox.New(t)
	record := stubEditors(t, s, `{ pwd -P; printf '%s\n' "${0##*/}" "$@"; } >"$RECORD"`, "myedit", "other")
	t.Setenv("TMUX_KIT_EDITOR", "other")
	s.Write("my $(exit 7) 'project'/f", "")
	folder := s.Path("my $(exit 7) 'project'")
	if err := openSoon(t, editor("myedit --new-window"), folder); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got, want := recorded(t, record), []string{folder, "myedit", "--new-window", folder}; !slices.Equal(got, want) {
		t.Errorf("ran %q, want %q", got, want)
	}
}

// With TMUX_KIT_EDITOR unset, the editor is VS Code's code, as for prefix e.
func TestTheEditorIsCodeByDefault(t *testing.T) {
	s := sandbox.New(t)
	record := stubEditors(t, s, `printf '%s\n' "${0##*/}" "$@" >"$RECORD"`, "code")
	if err := openSoon(t, editor(""), s.Dir); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got, want := recorded(t, record), []string{"code", s.Dir}; !slices.Equal(got, want) {
		t.Errorf("ran %q, want %q", got, want)
	}
}

// If the editor fails, the error says why: what it said on stderr (e.g.
// that there's no such editor), or its exit status if it said nothing. So
// does a failure to read TMUX_KIT_EDITOR.
func TestAFailedEditorSaysWhy(t *testing.T) {
	s := sandbox.New(t)
	stubEditors(t, s, `[ "$1" = -q ] && exit 4; echo "myedit: can't open $1" >&2; exit 3`, "myedit")
	for _, c := range []struct{ line, want string }{
		{"myedit", "can't open the editor: myedit: can't open " + s.Dir},
		{"myedit -q", "can't open the editor: myedit -q: exit status 4"},
	} {
		if err := openSoon(t, editor(c.line), s.Dir); err == nil || err.Error() != c.want {
			t.Errorf("Open with %s: error %v, want %q", c.line, err, c.want)
		}
	}
	err := openSoon(t, editor("no-such-editor"), s.Dir)
	if err == nil || !strings.HasPrefix(err.Error(), "can't open the editor: ") || !strings.Contains(err.Error(), "no-such-editor") {
		t.Errorf("Open with no such editor: error %v, want it to say so", err)
	}
	unread := Opener{Editor: func() (string, error) { return "", errors.New("no server running on /tmp/tmux-1/default") }}
	if err := openSoon(t, unread, s.Dir); err == nil || err.Error() != "can't open the editor: no server running on /tmp/tmux-1/default" {
		t.Errorf("Open without TMUX_KIT_EDITOR: error %v", err)
	}
	if left, _ := os.ReadDir(os.Getenv("TMPDIR")); len(left) > 1 {
		t.Errorf("Open left %v in TMPDIR", left)
	}
}

// The editor is no part of wtui's terminal: it can't read wtui's keys, and
// it's in a session of its own. One that stays (e.g. code --wait) isn't
// waited for, only for a while in case it fails.
func TestTheEditorRunsDetachedAndIsNotWaitedFor(t *testing.T) {
	if _, err := exec.LookPath("ps"); err != nil {
		t.Skip("no ps on PATH to read the editor's process group with")
	}
	s := sandbox.New(t)
	was := patience
	patience = 100 * time.Millisecond
	t.Cleanup(func() { patience = was })
	t.Setenv("TEST_PGID", strconv.Itoa(syscall.Getpgrp()))
	record := stubEditors(t, s, `
[ /dev/stdin -ef /dev/null ] && echo "stdin /dev/null" >>"$RECORD"
[ "$(ps -o pgid= -p $$ | tr -d ' ')" != "$TEST_PGID" ] && echo "own process group" >>"$RECORD"
echo started >>"$RECORD"
while [ -e "$FLAG" ]; do sleep 0.05; done`, "myedit")
	if err := openSoon(t, editor("myedit"), s.Dir); err != nil {
		t.Fatalf("Open: %v", err)
	}
	for range 100 {
		if b, _ := os.ReadFile(record); strings.Contains(string(b), "started") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if got, want := recorded(t, record), []string{"stdin /dev/null", "own process group", "started"}; !slices.Equal(got, want) {
		t.Errorf("the editor found %q, want %q", got, want)
	}
}
