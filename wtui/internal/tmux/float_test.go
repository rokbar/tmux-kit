package tmux_test

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// readFile is what the file at path holds, "" while there's none.
func readFile(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

// A command floats in a bordered pane over the window the nested client
// shows (its session's current window), one cell in from each of its
// edges, with focus in it, and runs in the folder given, its words as they
// are. The pane closes once the command exits.
func TestFloatOpensAFloatingPaneOverTheNestedClientsWindow(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	if v := strings.TrimPrefix(tmuxRun(t, "-V"), "tmux "); tmux.Older(v, tmux.FloatingVersion) {
		t.Skipf("tmux %s has no floating panes (they're new in %s)", v, tmux.FloatingVersion)
	}
	tmuxRun(t, "new-session", "-d", "-s", "api")
	tmuxRun(t, "new-window", "-d", "-t", "=api:")
	tmuxRun(t, "new-session", "-d", "-s", "beta")
	nested := nestedFromOutside(t, "api")
	tmuxRun(t, "select-window", "-t", "=api:1")
	s.Write("my folder/f", "")
	dir := s.Path("my folder")

	err := nested.Float(dir, []string{"sh", "-c", `pwd -P >ran-in; printf %s "$1" >said; while [ ! -e quit ]; do sleep 0.1; done`, "sh", "-x 'one' two"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the command to run", func() bool { return readFile(s.Path("my folder/said")) != "" })
	if got := strings.TrimSpace(readFile(s.Path("my folder/ran-in"))); got != dir {
		t.Errorf("the command ran in %s, want %s", got, dir)
	}
	if got := readFile(s.Path("my folder/said")); got != "-x 'one' two" {
		t.Errorf("the command was given %q, want -x 'one' two", got)
	}

	var floating []string
	for _, line := range strings.Split(tmuxRun(t, "list-panes", "-a", "-F",
		"#{pane_floating_flag} #{session_name}:#{window_index} #{pane_active} #{pane_left},#{pane_top} #{pane_width}x#{pane_height}"), "\n") {
		if rest, ok := strings.CutPrefix(line, "1 "); ok {
			floating = append(floating, rest)
		}
	}
	size := strings.Fields(tmuxRun(t, "display-message", "-p", "-t", "=api:1", "#{window_width} #{window_height}"))
	w, _ := strconv.Atoi(size[0])
	h, _ := strconv.Atoi(size[1])
	if want := "api:1 1 1,1 " + strconv.Itoa(w-2) + "x" + strconv.Itoa(h-2); len(floating) != 1 || floating[0] != want {
		t.Errorf("floating panes: %q, want one, %q (window, focus, corner, size in a %dx%d window)", floating, want, w, h)
	}

	s.Write("my folder/quit", "")
	waitFor(t, "the pane to close", func() bool {
		return !strings.Contains(tmuxRun(t, "list-panes", "-a", "-F", "#{pane_floating_flag}"), "1")
	})
}

// Without a nested client, nothing floats.
func TestFloatNeedsANestedClient(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "api")
	if err := (&tmux.Client{}).Float(s.Dir, []string{"true"}); err == nil {
		t.Error("with no nested client, Float succeeded")
	}
}

// A command pops up over the whole of the nested client, not any other
// client of its session, in the folder given, its words as they are, and
// Popup returns once it has exited.
func TestPopupRunsACommandOverTheNestedClientUntilItExits(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "api")
	nested := nestedFromOutside(t, "api")
	// another terminal's client on api, used since
	tmuxRun(t, "-L", "term", "new-session", "-d", "-s", "other", "-x", "100", "-y", "30",
		"env", "-u", "TMUX", "tmux", "attach", "-t", "=api")
	other := tmuxRun(t, "-L", "term", "display-message", "-p", "-t", "=other:", "#{pane_tty}")
	waitFor(t, "the other client to attach", func() bool { return showing(t, other) != "" })
	s.Write("my folder/f", "")
	dir := s.Path("my folder")

	done := make(chan error, 1)
	go func() {
		done <- nested.Popup(tmux.Popup{Dir: dir, Command: []string{"sh", "-c", `pwd -P >ran-in; echo "$1"; while [ ! -e quit ]; do sleep 0.1; done`, "sh", "-x 'one' two"}})
	}()
	screen := func(term string) string { return tmuxRun(t, "-L", "term", "capture-pane", "-p", "-t", "="+term+":") }
	waitFor(t, "the popup to show", func() bool { return strings.Contains(screen("term"), "-x 'one' two") })
	lines := strings.Split(screen("term"), "\n")
	if top := []rune(lines[0]); len(top) != 100 || top[0] != '┌' || top[99] != '┐' {
		t.Errorf("the popup's top line is %q, want its border across the client's 100 columns", lines[0])
	}
	if strings.Contains(screen("other"), "-x 'one' two") {
		t.Errorf("the popup shows on the other client too:\n%s", screen("other"))
	}
	select {
	case err := <-done:
		t.Fatalf("Popup returned while its command ran: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	if got := strings.TrimSpace(readFile(s.Path("my folder/ran-in"))); got != dir {
		t.Errorf("the command ran in %s, want %s", got, dir)
	}

	s.Write("my folder/quit", "")
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Popup: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Popup didn't return once its command exited")
	}
	waitFor(t, "the popup to close", func() bool { return !strings.Contains(screen("term"), "-x 'one' two") })
}

// (Without a nested client: popup_test.go.)
