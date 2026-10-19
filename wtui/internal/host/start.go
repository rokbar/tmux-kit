package host

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rokbar/tmux-kit/wtui/internal/repo"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// Folder is the folder wtui is launched from, as git sees it.
type Folder struct {
	Path string
	// Worktree is the folder of the main clone or worktree Path is in, ""
	// outside git; Worktrees are the folders of its repo's main clone and
	// every worktree.
	Worktree  string
	Worktrees []string
}

// Starting is the session (an id) of snap's that a launch from folder starts
// on, or "" if there's none, in which case wtui starts one for the folder:
//
//  1. the most recent session of the main clone or worktree the folder is
//     in, if it has one;
//  2. else, if the folder is in the start folder of a session outside git,
//     that session: the nearest such folder's, the most recent if several
//     sessions were started there;
//  3. else, the most recent session.
//
// A session is in the main clone or worktree whose folder holds its start
// folder, the longest such, as in the tree. The most recent session is the
// one with the latest activity, the one tmux moves a no-detach-on-destroy
// client to; of sessions used in the same second (all tmux says), the first
// in snap, by name.
func Starting(snap tmux.Snapshot, folder Folder) string {
	worktrees := folder.Worktrees
	if folder.Worktree != "" && !slices.Contains(worktrees, folder.Worktree) {
		worktrees = append(slices.Clone(worktrees), folder.Worktree)
	}
	var in, outside []tmux.Session // in the folder's worktree; holding it, outside git
	for _, s := range snap.Sessions {
		w := holder(worktrees, s.Path)
		switch {
		case w != "" && w == folder.Worktree:
			in = append(in, s)
		case w == "" && s.Path != "" && within(folder.Path, s.Path):
			outside = append(outside, s)
		}
	}
	if s := latest(in); s != nil {
		return s.ID
	}
	// the nearest start folder is the longest
	nearest := 0
	for _, s := range outside {
		nearest = max(nearest, len(s.Path))
	}
	outside = slices.DeleteFunc(outside, func(s tmux.Session) bool { return len(s.Path) < nearest })
	if s := latest(outside); s != nil {
		return s.ID
	}
	if s := latest(snap.Sessions); s != nil {
		return s.ID
	}
	return ""
}

// latest is the session with the latest activity, the first of those used
// in the same second, or nil if there's none.
func latest(sessions []tmux.Session) *tmux.Session {
	var best *tmux.Session
	for i, s := range sessions {
		if best == nil || s.Activity.After(best.Activity) {
			best = &sessions[i]
		}
	}
	return best
}

// holder is the folder of folders that holds dir, the longest such, or "" if
// none does.
func holder(folders []string, dir string) string {
	at := ""
	for _, f := range folders {
		if within(dir, f) && len(f) > len(at) {
			at = f
		}
	}
	return at
}

// within says whether dir is folder or inside it.
func within(dir, folder string) bool {
	return dir == folder || strings.HasPrefix(dir, strings.TrimSuffix(folder, "/")+"/")
}

// launchSession is the session the launcher's right side starts on, for the
// folder it runs in (see Starting): an id, or with no session to start on,
// =<name> of the one t starts there.
func launchSession(exe string, normal *tmux.Client) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	snap, err := normal.Snapshot()
	if err != nil {
		return "", fmt.Errorf("can't list tmux's sessions: %w", err)
	}
	folder := Folder{Path: dir}
	// where git can't look, the folder counts as outside git
	if in, all, err := (repo.Lookup{}).Worktrees(dir); err == nil {
		folder.Worktree, folder.Worktrees = in, all
	}
	if session := Starting(snap, folder); session != "" {
		return session, nil
	}
	name, err := startSession(exe)
	if err != nil {
		return "", fmt.Errorf("there's no session to show, and t can't start one here: %w", err)
	}
	return "=" + name, nil
}

// startSession has t start the session of the folder wtui runs in, without
// attaching to it (t -d), which starts the server too if it isn't running,
// and is the session's name. The t is the one next to wtui (exe), as
// install.sh puts them, else the one on PATH.
func startSession(exe string) (string, error) {
	bin := filepath.Join(filepath.Dir(exe), "t")
	if info, err := os.Stat(bin); err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		if bin, err = exec.LookPath("t"); err != nil {
			return "", fmt.Errorf("t not found next to wtui (in %s) or on PATH", filepath.Dir(exe))
		}
	}
	cmd := exec.Command(bin, "-d")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if msg := strings.TrimSpace(stderr.String()); err != nil && msg != "" {
		return "", errors.New(msg)
	}
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", errors.New("t -d printed no session name")
	}
	return name, nil
}
