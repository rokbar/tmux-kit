package browser

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// opener is the system's opener this test runs on: open on macOS, xdg-open
// elsewhere.
func opener() string {
	if runtime.GOOS == "darwin" {
		return "open"
	}
	return "xdg-open"
}

// stubOpeners puts an open and an xdg-open that run body (sh) first on PATH,
// so no real opener, or browser, ever runs. $RECORD is a file the stubs can
// write to, which stubOpeners returns, and $FLAG one that's there until the
// test ends, for stubs that run until then. TMPDIR is a folder of its own.
func stubOpeners(t *testing.T, body string) (record string) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"open", "xdg-open"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	record, flag := filepath.Join(dir, "record"), filepath.Join(dir, "flag")
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(flag) }) // ends the stubs that wait for it
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("RECORD", record)
	t.Setenv("FLAG", flag)
	t.Setenv("TMPDIR", t.TempDir())
	return record
}

// recorded is the lines the stubs wrote to record.
func recorded(t *testing.T, record string) []string {
	t.Helper()
	b, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("the opener didn't run: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// withPatience has Open wait d for the opener while the test runs.
func withPatience(t *testing.T, d time.Duration) {
	t.Helper()
	was := patience
	patience = d
	t.Cleanup(func() { patience = was })
}

// openSoon runs Open(url), and fails the test if it hasn't returned within
// 10 s: it would have waited on something it shouldn't.
func openSoon(t *testing.T, url string) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- Opener{}.Open(url) }()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatalf("Open(%q) hasn't returned after 10 s", url)
		return nil
	}
}

// The system's opener gets the URL as its one argument, as it is: no shell
// sees it.
func TestOpensTheURLWithTheSystemsOpener(t *testing.T) {
	record := stubOpeners(t, `printf '%s\n' "${0##*/}" "$@" >"$RECORD"`)
	url := "http://localhost:4100/a b?x=1&y=$(exit 7);z='q'"
	if err := openSoon(t, url); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got, want := recorded(t, record), []string{opener(), url}; !slices.Equal(got, want) {
		t.Errorf("ran %q, want %q", got, want)
	}
}

// The opener (and the browser it may start) is no part of wtui's terminal:
// it can't read wtui's keys or draw on its screen, and it's in a session of
// its own, so it isn't ended with wtui.
func TestOpenerRunsDetachedFromTheTerminal(t *testing.T) {
	if _, err := exec.LookPath("ps"); err != nil {
		t.Skip("no ps on PATH to read the opener's process group with")
	}
	// pipes stand in for wtui's terminal, which go test may not give it
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, stdout := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = inR, outW
	t.Cleanup(func() {
		os.Stdin, os.Stdout = stdin, stdout
		for _, f := range []*os.File{inR, inW, outR, outW} {
			f.Close()
		}
	})
	record := stubOpeners(t, `
[ /dev/stdin -ef /dev/null ] && echo "stdin /dev/null" >>"$RECORD"
[ /dev/stdout -ef /dev/null ] && echo "stdout /dev/null" >>"$RECORD"
[ "$(ps -o pgid= -p $$ | tr -d ' ')" = $$ ] && echo "own process group" >>"$RECORD"
true`)
	if err := openSoon(t, "http://localhost:4100"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got, want := recorded(t, record), []string{"stdin /dev/null", "stdout /dev/null", "own process group"}; !slices.Equal(got, want) {
		t.Errorf("the opener found %q, want %q", got, want)
	}
}

// If the opener fails, the error says why: what it said on stderr, or its
// exit status if it said nothing. It leaves no file behind.
func TestFailedOpenerSaysWhy(t *testing.T) {
	for _, c := range []struct{ body, want string }{
		{`echo "xdg-open: no method available for opening '$1'" >&2; exit 3`,
			"can't open http://localhost:4100: xdg-open: no method available for opening 'http://localhost:4100'"},
		{`exit 4`, "can't open http://localhost:4100: " + opener() + ": exit status 4"},
	} {
		stubOpeners(t, c.body)
		err := openSoon(t, "http://localhost:4100")
		if err == nil || err.Error() != c.want {
			t.Errorf("Open with %s: error %v, want %q", c.body, err, c.want)
		}
		if left, _ := os.ReadDir(os.Getenv("TMPDIR")); len(left) > 0 {
			t.Errorf("Open left %v in TMPDIR", left)
		}
	}
}

// With no opener installed (e.g. no xdg-open on a server), the error says so.
func TestNoOpenerOnPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	want := "can't open http://localhost:4100: no " + opener() + " on PATH"
	if err := openSoon(t, "http://localhost:4100"); err == nil || err.Error() != want {
		t.Errorf("error %v, want %q", err, want)
	}
}

// An opener can run the browser itself and stay until it's closed (as
// xdg-open does with $BROWSER): Open doesn't wait for that, only for a while
// in case it fails.
func TestOpenerThatStaysIsNotWaitedFor(t *testing.T) {
	withPatience(t, 100*time.Millisecond)
	record := stubOpeners(t, `echo started >"$RECORD"; while [ -e "$FLAG" ]; do sleep 0.05; done`)
	if err := openSoon(t, "http://localhost:4100"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	// it may not have got far by then: it's still running, until the test ends
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(record); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Error("the opener never started")
}

// The browser an opener starts can keep the opener's stderr after the opener
// has exited: Open returns when the opener exits, not when the browser does.
func TestBrowserKeepingTheOpenersStderrIsNotWaitedFor(t *testing.T) {
	withPatience(t, time.Hour)
	stubOpeners(t, `(while [ -e "$FLAG" ]; do sleep 0.05; done) & exit 0`)
	if err := openSoon(t, "http://localhost:4100"); err != nil {
		t.Fatalf("Open: %v", err)
	}
}

// A dev URL is a web address. Anything else could be an option to the opener
// or a file it would open, even run (a .command on macOS), so it isn't
// handed to it.
func TestOnlyWebURLsAreOpened(t *testing.T) {
	record := stubOpeners(t, `echo ran >"$RECORD"`)
	for _, url := range []string{"-a Calculator", "/Applications/Calculator.app", "./run.command", "file:///etc/passwd", "localhost:3000", "http://"} {
		want := "can't open " + url + ": not an http or https URL"
		if err := openSoon(t, url); err == nil || err.Error() != want {
			t.Errorf("Open(%q): error %v, want %q", url, err, want)
		}
	}
	if _, err := os.Stat(record); err == nil {
		t.Errorf("the opener ran: %q", recorded(t, record))
	}
	for _, url := range []string{"HTTPS://localhost:3000/", "http://127.0.0.1:4100/path?q=1#top"} {
		if err := openSoon(t, url); err != nil {
			t.Errorf("Open(%q): %v", url, err)
		}
	}
}
