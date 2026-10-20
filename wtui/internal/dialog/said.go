// Package dialog is wtui's dialogs: short programs, wtui subcommands, that
// the sidebar runs in popups, over the right side or the whole host. It has
// the dialogs themselves (details, a page of text such as the shortcuts),
// and how a dialog tells the sidebar what its status line should say once
// the popup has closed (Open and Say).
package dialog

import (
	"errors"
	"os"
	"strings"
)

// saidEnv names the file a dialog says what it has to say in (see Say).
const saidEnv = "WTUI_SAID"

// Open runs a dialog: open opens its popup, with env added to the
// environment the dialog runs in, and returns once the popup has closed.
// Open returns what the dialog said (see Say), "" if nothing, or why open
// failed.
func Open(open func(env []string) error) (said string, err error) {
	f, err := os.CreateTemp("", "wtui-said-")
	if err != nil {
		return "", err
	}
	f.Close()
	defer os.Remove(f.Name())
	if err := open([]string{saidEnv + "=" + f.Name()}); err != nil {
		return "", err
	}
	out, err := os.ReadFile(f.Name())
	return strings.TrimSpace(string(out)), err
}

// Say has the sidebar's status line say text once this dialog's popup has
// closed, in place of anything it said before. Outside a dialog Open
// opened, it does nothing.
func Say(text string) error {
	file := os.Getenv(saidEnv)
	if file == "" {
		return nil
	}
	if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
		return errors.New("can't tell the sidebar: " + err.Error())
	}
	return nil
}
