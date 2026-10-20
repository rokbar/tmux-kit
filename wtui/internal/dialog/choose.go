package dialog

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Choose has fzf, one of tmux-kit's core programs, choose one of items with
// prompt, on this process's terminal (a popup's), and is the one chosen: ""
// if none was (Esc, or enter with no match).
func Choose(prompt string, items []string) (string, error) {
	bin, err := exec.LookPath("fzf")
	if err != nil {
		return "", errors.New("fzf not found on PATH: tmux-kit needs it")
	}
	cmd := exec.Command(bin, "--prompt="+prompt)
	cmd.Stdin = strings.NewReader(strings.Join(items, "\n") + "\n")
	cmd.Stderr = os.Stderr // fzf draws on the terminal, and says there why it fails
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && (exit.ExitCode() == 1 || exit.ExitCode() == 130) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("fzf failed: %w", err)
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}
