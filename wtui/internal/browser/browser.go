// Package browser opens a dev URL in the user's browser, with the system's
// opener: open on macOS, xdg-open elsewhere.
package browser

import (
	"fmt"
	neturl "net/url"
	"os/exec"
	"runtime"
	"time"

	"github.com/rokbar/tmux-kit/wtui/internal/detached"
)

// patience is how long Open waits for the opener to finish, in case it fails.
// One still running by then has started the browser and stayed with it (as
// xdg-open does when it runs $BROWSER itself), so the URL is open.
var patience = 3 * time.Second

// Opener opens URLs in the browser.
type Opener struct{}

// Open opens url in the browser with the system's opener, and returns once
// the opener has finished, or after a few seconds if it hasn't. The opener
// gets url as its one argument, and runs detached (see detached.Run). If it
// fails, the error says why, with what it said.
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
	if err := detached.Run(exec.Command(bin, url), name, patience); err != nil {
		return fmt.Errorf("can't open %s: %w", url, err)
	}
	return nil
}

// isWeb reports whether url is a web address. The opener would take anything
// else as an option, or a file to open with its app, which can run it (e.g. a
// .command on macOS).
func isWeb(url string) bool {
	u, err := neturl.Parse(url)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
