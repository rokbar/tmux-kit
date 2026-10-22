// Package tfolder is wtui's client for tmux-kit's t, which gives a plain
// folder its session, named after the folder: the only code in wtui that
// knows t's command line.
package tfolder

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Client runs the t next to the wtui executable at wtuiPath, as install.sh
// puts them, else the one on PATH.
type Client struct{ wtuiPath string }

// New is the client for the wtui executable at wtuiPath (os.Executable).
func New(wtuiPath string) *Client { return &Client{wtuiPath: wtuiPath} }

// Start has t start the session of folder dir, unless it has one, without
// attaching or switching to it (`t -d`), and is the session's name. With no
// tmux server running, t starts one.
func (c *Client) Start(dir string) (string, error) {
	out, err := c.run(dir, "-d")
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(out)
	if name == "" {
		return "", errors.New("t -d printed no session name")
	}
	return name, nil
}

// Open has t open the session of folder dir, starting it unless it has one,
// and switch the tmux client t runs in to it (`t`): in a popup of wtui's,
// the one TMUX_KIT_CLIENT names. It's for inside tmux: outside, t would
// attach this process's terminal instead.
func (c *Client) Open(dir string) error {
	if os.Getenv("TMUX") == "" {
		return errors.New("t can only switch to a session inside tmux")
	}
	_, err := c.run(dir)
	return err
}

// run runs t with args in dir and returns its stdout. If t fails, the error
// is what it said on stderr.
func (c *Client) run(dir string, args ...string) (string, error) {
	bin, err := c.find()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if msg := strings.TrimSpace(stderr.String()); err != nil && msg != "" {
		return "", errors.New(msg)
	}
	return string(out), err
}

// find is the t to run.
func (c *Client) find() (string, error) {
	bin := filepath.Join(filepath.Dir(c.wtuiPath), "t")
	if info, err := os.Stat(bin); err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
		return bin, nil
	}
	if bin, err := exec.LookPath("t"); err == nil {
		return bin, nil
	}
	return "", fmt.Errorf("t not found next to wtui (in %s) or on PATH", filepath.Dir(c.wtuiPath))
}
