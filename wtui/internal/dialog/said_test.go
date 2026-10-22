package dialog_test

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/rokbar/tmux-kit/wtui/internal/dialog"
)

// popup is open's stand-in for a popup: it runs the dialog's program, sh
// with script, with env added to its environment, as tmux would, and notes
// what env was.
func popup(t *testing.T, script string, got *[]string) func(env []string) error {
	return func(env []string) error {
		*got = env
		cmd := exec.Command("sh", "-c", script)
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("the dialog's program: %v\n%s", err, out)
		}
		return nil
	}
}

// What a dialog says (Say, in a wtui subcommand run in the popup) is what
// Open returns once the popup has closed, for the sidebar's status line. A
// dialog that says nothing has Open return "".
func TestWhatADialogSaysIsWhatOpenReturns(t *testing.T) {
	var env []string
	said, err := dialog.Open(popup(t, `printf 'removed api--fix\n' >"$WTUI_SAID"`, &env))
	if err != nil || said != "removed api--fix" {
		t.Errorf("Open() = %q, %v; want what the dialog said, removed api--fix", said, err)
	}
	if len(env) != 1 || !strings.HasPrefix(env[0], "WTUI_SAID=") {
		t.Fatalf("the dialog's environment got %q, want WTUI_SAID only", env)
	}
	if _, err := os.Stat(strings.TrimPrefix(env[0], "WTUI_SAID=")); !os.IsNotExist(err) {
		t.Errorf("what the dialog said is still in its file (%v)", err)
	}

	if said, err := dialog.Open(popup(t, "true", &env)); err != nil || said != "" {
		t.Errorf("for a dialog that says nothing, Open() = %q, %v; want nothing", said, err)
	}
}

// Say, run in a dialog, says the text for the sidebar's status line, in
// place of anything it said before. Outside a dialog, it does nothing.
func TestSay(t *testing.T) {
	said, err := dialog.Open(func(env []string) error {
		for _, kv := range env {
			name, value, _ := strings.Cut(kv, "=")
			t.Setenv(name, value)
		}
		if err := dialog.Say("wt: refusing"); err != nil {
			return err
		}
		return dialog.Say("wt: removed api--fix\nand its session")
	})
	if err != nil || said != "wt: removed api--fix\nand its session" {
		t.Errorf("Open() = %q, %v; want the last thing said", said, err)
	}
	t.Setenv("WTUI_SAID", "")
	if err := dialog.Say("anything"); err != nil {
		t.Errorf("outside a dialog, Say() = %v, want it to do nothing", err)
	}
}

// If the popup can't open, Open says why.
func TestOpenFailsWithItsPopup(t *testing.T) {
	cant := errors.New("can't find client: /dev/ttys999")
	if _, err := dialog.Open(func([]string) error { return cant }); !errors.Is(err, cant) {
		t.Errorf("Open() = %v, want %v", err, cant)
	}
}
