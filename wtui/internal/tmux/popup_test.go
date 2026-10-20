package tmux_test

import (
	"os"
	"strings"
	"testing"

	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// A popup on the nested client covers all of it, inside a border: its
// program runs in the popup's folder, as words (no shell, no formats), with
// the environment asked for, and with the nested client's name in
// TMUX_KIT_CLIENT, so that what it runs (wt, t) switches that client: in a
// popup, tmux's own pick is whichever client was used last. Popup returns
// once the program has exited, whatever its exit status: that's the
// program's to report.
func TestPopupRunsOnTheNestedClient(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "a")
	nested := nestedFromOutside(t, "a") // a 100x30 terminal
	s.Write("my dir/.keep", "")
	prog := s.Script("popup.sh", `pwd -P >"$OUT/pwd"
printf '%s|' "$@" >"$OUT/args"
echo "$GREETING from $TMUX_KIT_CLIENT" >"$OUT/env"
stty size >"$OUT/size"
sleep 0.3
exit 3`)

	err := nested.Popup(tmux.Popup{
		Title:   " a test ",
		Dir:     s.Path("my dir"),
		Command: []string{prog, "a b", "#{session_name}", "c;d"},
		Env:     []string{"GREETING=hello there", "OUT=" + s.Dir},
	})
	if err != nil {
		t.Fatalf("Popup: %v", err)
	}
	for file, want := range map[string]string{
		"pwd":  s.Path("my dir"),
		"args": "a b|#{session_name}|c;d|",
		"env":  "hello there from " + nested.Nested,
		"size": "28 98", // the client's 30x100, less the border
	} {
		got, err := os.ReadFile(s.Path(file))
		if err != nil {
			t.Fatalf("the popup's program wrote no %s (or Popup returned before it was done): %v", file, err)
		}
		if strings.TrimSpace(string(got)) != want {
			t.Errorf("in the popup, %s is %q, want %q", file, strings.TrimSpace(string(got)), want)
		}
	}
}

// Without a nested client, or with one that's gone, there's nothing to open
// a popup on.
func TestPopupNeedsTheNestedClient(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "a")
	p := tmux.Popup{Command: []string{"true"}}
	if err := (&tmux.Client{}).Popup(p); err == nil {
		t.Error("with no nested client, Popup works")
	}
	if err := (&tmux.Client{Nested: "/dev/ttys999"}).Popup(p); err == nil || !strings.Contains(err.Error(), "/dev/ttys999") {
		t.Errorf("with no client on its tty, Popup() = %v, want tmux's error", err)
	}
}
