// Package detached runs the programs that open windows of their own (the
// browser's opener, the editor) apart from wtui: in a session of their own,
// with no terminal, so they can't take wtui's keys or draw on its screen,
// and don't end with it.
package detached

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Run runs cmd in a session of its own, its stdin and stdout /dev/null, and
// returns once it has finished, or after patience if it hasn't: one still
// running by then has got going and stayed with what it opened (as
// xdg-open does when it runs $BROWSER itself). If it fails, the error is
// what it said on stderr, or else name and how it ended.
func Run(cmd *exec.Cmd, name string, patience time.Duration) error {
	// stderr goes to a file rather than a pipe, which Wait would wait on
	// until whatever the program started, which may keep it, exits. Nothing
	// else needs the file, so it's removed at once: it goes when the last
	// process with it open closes it.
	stderr, err := os.CreateTemp("", "wtui-open-")
	if err != nil {
		return err
	}
	defer stderr.Close()
	os.Remove(stderr.Name())

	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1) // so the goroutine ends once the program has, waited for or not
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			return nil
		}
		// read from the start whatever the file offset, which the program's
		// children may share
		b, _ := io.ReadAll(io.NewSectionReader(stderr, 0, 1<<16))
		if why := strings.TrimSpace(string(b)); why != "" {
			return errors.New(why)
		}
		return errors.New(name + ": " + err.Error())
	case <-time.After(patience):
		return nil
	}
}
