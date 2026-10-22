package tmux_test

import (
	"os"
	"testing"

	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// The projects root is the @wtui-projects-root option, ~/Projects while it
// isn't set, with a leading ~ standing for HOME.
func TestProjectsRootIsTheWtuiProjectsRootOption(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	home := os.Getenv("HOME")
	c := &tmux.Client{}
	if got := c.ProjectsRoot(); got != home+"/Projects" {
		t.Errorf("with no server, ProjectsRoot() = %q, want %s/Projects", got, home)
	}
	tmuxRun(t, "new-session", "-d", "-s", "a")
	if got := c.ProjectsRoot(); got != home+"/Projects" {
		t.Errorf("with @wtui-projects-root unset, ProjectsRoot() = %q, want %s/Projects", got, home)
	}
	for set, want := range map[string]string{
		"~/code":        home + "/code",
		"~":             home,
		"/srv/projects": "/srv/projects",
		"/srv/~x":       "/srv/~x",
		"~other/code":   "~other/code", // another user's home isn't looked up
	} {
		tmuxRun(t, "set-option", "-g", "@wtui-projects-root", set)
		if got := c.ProjectsRoot(); got != want {
			t.Errorf("with @wtui-projects-root %s, ProjectsRoot() = %q, want %q", set, got, want)
		}
	}
}
