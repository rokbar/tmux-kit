package dialog_test

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/rokbar/tmux-kit/wtui/internal/dialog"
	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
)

// stubFzf puts an fzf first on PATH that notes what it's given, its
// arguments and the lines on its stdin, then prints out and exits with
// status. It returns what it was given, once it has run.
func stubFzf(t *testing.T, s *sandbox.Sandbox, out string, status int) (args, lines func() []string) {
	s.Script("bin/fzf", fmt.Sprintf(`printf '%%s\n' "$@" >"$0.args"; cat >"$0.in"; printf '%s'; exit %d`, out, status))
	t.Setenv("PATH", s.Path("bin")+":"+os.Getenv("PATH"))
	given := func(file string) func() []string {
		return func() []string {
			b, err := os.ReadFile(s.Path("bin/fzf." + file))
			if err != nil {
				t.Fatalf("fzf didn't run: %v", err)
			}
			return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
		}
	}
	return given("args"), given("in")
}

// Choose has fzf choose one of the items, a line each, with the prompt
// given, and is the one chosen.
func TestChooseIsWhatFzfChose(t *testing.T) {
	s := sandbox.New(t)
	args, lines := stubFzf(t, s, `my.notes\n`, 0)
	got, err := dialog.Choose("project> ", []string{"api", "my.notes", "Zed"})
	if err != nil || got != "my.notes" {
		t.Errorf("Choose() = %q, %v; want my.notes, which fzf chose", got, err)
	}
	if want := []string{"api", "my.notes", "Zed"}; !slices.Equal(lines(), want) {
		t.Errorf("fzf was given %q, want %q", lines(), want)
	}
	if !slices.Contains(args(), "--prompt=project> ") {
		t.Errorf("fzf ran with %q, want the prompt project> ", args())
	}
}

// Nothing is chosen when fzf is cancelled (Esc: 130), or enter has no match
// (1).
func TestNothingIsChosenWhenFzfIsCancelled(t *testing.T) {
	for _, status := range []int{130, 1} {
		s := sandbox.New(t)
		stubFzf(t, s, ``, status)
		if got, err := dialog.Choose("project> ", []string{"api"}); err != nil || got != "" {
			t.Errorf("with fzf exiting %d, Choose() = %q, %v; want nothing chosen", status, got, err)
		}
	}
}

// fzf failing, or missing, is a failure that says so.
func TestChooseFailsWithoutAWorkingFzf(t *testing.T) {
	s := sandbox.New(t)
	stubFzf(t, s, ``, 2)
	if _, err := dialog.Choose("project> ", []string{"api"}); err == nil || !strings.Contains(err.Error(), "fzf") {
		t.Errorf("with fzf failing, Choose() fails with %v, want a failure naming fzf", err)
	}
	t.Setenv("PATH", s.Path("nowhere"))
	if _, err := dialog.Choose("project> ", []string{"api"}); err == nil || !strings.Contains(err.Error(), "fzf not found") {
		t.Errorf("with no fzf, Choose() fails with %v, want fzf not found", err)
	}
}
