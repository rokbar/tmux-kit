package host_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/rokbar/tmux-kit/wtui/internal/host"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// session is a session with id $<n>, named name, started in path, last used
// at second at.
func session(n int, name, path string, at int64) tmux.Session {
	return tmux.Session{ID: fmt.Sprintf("$%d", n), Name: name, Path: path, Activity: time.Unix(at, 0)}
}

// The sessions most launches below choose from, by name: a git project api
// (main clone /p/api; worktrees /p/api--fix, and /p/api/.worktrees/spike
// inside the main clone's folder), and plain folders /home/u/notes and
// /home/u/notes/drafts. spike is the most recent, then notes, drafts, api,
// api-web and fix.
var sessions = tmux.Snapshot{Sessions: []tmux.Session{
	session(1, "api", "/p/api", 300),
	session(2, "api-web", "/p/api/web", 200),
	session(3, "drafts", "/home/u/notes/drafts", 400),
	session(4, "fix", "/p/api--fix", 100),
	session(5, "notes", "/home/u/notes", 500),
	session(6, "spike", "/p/api/.worktrees/spike", 800),
}}

// api's main clone and worktrees, as git lists them
var api = []string{"/p/api", "/p/api--fix", "/p/api/.worktrees/spike"}

// A launch from a folder starts on, in order: the most recent session of the
// main clone or worktree it's in; else the session whose start folder (the
// nearest, outside git) holds it; else the most recent session; and with no
// session at all, none, which has wtui start one there.
func TestStartingSession(t *testing.T) {
	for _, c := range []struct {
		name   string
		snap   tmux.Snapshot
		folder host.Folder
		want   string
	}{
		{
			"in a worktree, its session, though others are more recent",
			sessions, host.Folder{Path: "/p/api--fix/src", Worktree: "/p/api--fix", Worktrees: api}, "$4",
		},
		{
			"in the main clone, the most recent of its sessions, but not a worktree's inside its folder",
			sessions, host.Folder{Path: "/p/api/docs", Worktree: "/p/api", Worktrees: api}, "$1",
		},
		{
			"in a worktree inside the main clone's folder, the worktree's session",
			sessions, host.Folder{Path: "/p/api/.worktrees/spike", Worktree: "/p/api/.worktrees/spike", Worktrees: api}, "$6",
		},
		{
			"in a session's start folder outside git, that session, though another is more recent",
			sessions, host.Folder{Path: "/home/u/notes"}, "$5",
		},
		{
			"inside a folder outside git that two sessions' start folders hold, the nearest one's, though the other is more recent",
			sessions, host.Folder{Path: "/home/u/notes/drafts/old"}, "$3",
		},
		{
			"in a git folder whose worktree has no session, inside a session's start folder outside git, that session",
			sessions, host.Folder{Path: "/home/u/notes/repo", Worktree: "/home/u/notes/repo", Worktrees: []string{"/home/u/notes/repo"}}, "$5",
		},
		{
			"in a worktree inside the main clone's folder, without a session, not the main clone's session: the most recent",
			sessions, host.Folder{Path: "/p/api/.worktrees/new", Worktree: "/p/api/.worktrees/new", Worktrees: append(api, "/p/api/.worktrees/new")}, "$6",
		},
		{
			"in a worktree without a session, the most recent session",
			sessions, host.Folder{Path: "/p/api--docs", Worktree: "/p/api--docs", Worktrees: append(api, "/p/api--docs")}, "$6",
		},
		{
			"in a folder no session's start folder holds, the most recent session",
			sessions, host.Folder{Path: "/home/u"}, "$6",
		},
		{
			"a folder whose name starts like a start folder's isn't in it",
			sessions, host.Folder{Path: "/home/u/notes-old"}, "$6",
		},
		{
			"of sessions used in the same second, the first by name",
			tmux.Snapshot{Sessions: []tmux.Session{session(1, "a", "/a", 5), session(2, "b", "/b", 5)}},
			host.Folder{Path: "/home/u"}, "$1",
		},
		{
			"no session at all, none",
			tmux.Snapshot{}, host.Folder{Path: "/p/api", Worktree: "/p/api", Worktrees: api}, "",
		},
	} {
		if got := host.Starting(c.snap, c.folder); got != c.want {
			t.Errorf("%s: Starting(%+v) = %q, want %q", c.name, c.folder, got, c.want)
		}
	}
}
