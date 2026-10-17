// Package browser opens a dev URL in the user's browser, with the system's
// opener: open on macOS, xdg-open elsewhere.
package browser

import (
	"fmt"
	"io"
	neturl "net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// patience is how long Open waits for the opener to finish, in case it fails.
// One still running by then has started the browser and stayed with it (as
// xdg-open does when it runs $BROWSER itself), so the URL is open.
var patience = 3 * time.Second

// Opener opens URLs in the browser.
type Opener struct{}

// Open opens url in the browser with the system's opener, and returns once
// the opener has finished, or after a few seconds if it hasn't. The opener
// gets url as its one argument, and runs in a session of its own with no
// terminal: it can't take wtui's keys or draw on its screen, and isn't ended
// with the popup. If it fails, the error says why, with what it said.
func (Opener) Open(url string) error {
	if !isWeb(url) {
		return fmt.Errorf("can't open %s: not an http or https URL", url)
	}
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	bin, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("can't open %s: no %s on PATH", url, name)
	}
	// stderr goes to a file rather than a pipe, which Wait would wait on
	// until the browser, which may keep it, exits. Nothing else needs the
	// file, so it's removed at once: it goes when the last process with it
	// open closes it.
	stderr, err := os.CreateTemp("", "wtui-open-")
	if err != nil {
		return fmt.Errorf("can't open %s: %w", url, err)
	}
	defer stderr.Close()
	os.Remove(stderr.Name())

	cmd := exec.Command(bin, url)
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("can't open %s: %w", url, err)
	}
	done := make(chan error, 1) // so the goroutine ends once the opener has, waited for or not
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			return nil
		}
		// read from the start whatever the file offset, which the opener's
		// children may share
		b, _ := io.ReadAll(io.NewSectionReader(stderr, 0, 1<<16))
		why := strings.TrimSpace(string(b))
		if why == "" {
			why = name + ": " + err.Error()
		}
		return fmt.Errorf("can't open %s: %s", url, why)
	case <-time.After(patience):
		return nil
	}
}

// isWeb reports whether url is a web address. The opener would take anything
// else as an option, or a file to open with its app, which can run it (e.g. a
// .command on macOS).
func isWeb(url string) bool {
	u, err := neturl.Parse(url)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
