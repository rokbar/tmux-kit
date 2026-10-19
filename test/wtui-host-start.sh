#!/usr/bin/env bash
# Which session is on the right, end to end. At launch: the session of the
# worktree wtui is launched in, else the most recent session; with no
# server, a new session for the launch folder, as t -d starts it. When the
# active session ends, tmux moves the right side to the most recent session,
# and the sidebar takes focus, its cursor there. When the last session ends,
# the right side says there's none, then shows the next session made. The
# normal server's detach-on-destroy stays as it was.
set -euo pipefail
# shellcheck source=test/lib/host.sh
. "$(dirname "$0")/lib/host.sh"
need_host_tmux
build_wtui

# right_shows <t> <text>: <t>'s right side shows <text>
right_shows() { case "$(host "$1" capture-pane -p -t "$(right_pane "$1")")" in *"$2"*) ;; *) return 1 ;; esac; }
cursor_on() { [ "$(cursor_row "$1")" = "$2" ]; }

# No server: wtui has t start the launch folder's session, named as t names
# it (no leading dot), and shows it.
mkdir -p "$tmp/.proj"
launch t1 "$tmp/.proj"
[ "$(showing t1)" = proj ] || fail "launched in .proj with no server, the right side shows '$(showing t1)', want proj, as t -d names it"
[ "$(tmux display-message -p -t =proj: '#{session_path}')" = "$tmp/.proj" ] ||
  fail "proj started in $(tmux display-message -p -t =proj: '#{session_path}'), want $tmp/.proj"
type_in t1 q
wait_until "q to close t1's host" host_gone t1
detach_on_destroy="$(tmux show-options -gv detach-on-destroy)"

# api: a git project, its main clone and its worktree fix each with a
# session. notes, a plain folder's session, is the most recent (activity
# counts seconds). Launched in a folder in fix, wtui starts on fix.
git init -q -b main "$tmp/api"
git -C "$tmp/api" commit -q --allow-empty -m init
git -C "$tmp/api" worktree add -q -b fix "$tmp/api--fix"
mkdir -p "$tmp/api--fix/src"
new_session api "$tmp/api"
new_session fix "$tmp/api--fix"
sleep 1
new_session notes
launch t2 "$tmp/api--fix/src"
[ "$(showing t2)" = fix ] || fail "launched in fix's worktree, the right side shows '$(showing t2)', want fix, though notes is more recent"
type_in t2 q
wait_until "q to close t2's host" host_gone t2

# Launched in $HOME, where no session started: the most recent session,
# latest. It runs a shell, so that exit ends it.
sleep 1
mkdir -p "$tmp/latest"
tmux new-session -d -s latest -c "$tmp/latest" sh
launch t3 "$HOME"
[ "$(showing t3)" = latest ] || fail "launched in \$HOME, the right side shows '$(showing t3)', want latest, the most recent session"

# exit in latest, on the right: tmux moves the right side to the most recent
# session left, next, and the sidebar takes focus, its cursor on next.
sleep 1
new_session next
type_in t3 'C-\'
wait_until "the wtui key to focus the right side" focus_on t3 right
type_in t3 exit Enter
wait_until "exit to end latest, and the right side to show next" eval '[ "$(showing t3)" = next ]'
wait_until "the sidebar to take focus once latest ended" focus_on t3 sidebar
wait_until "the cursor to be on next" cursor_on t3 '4   ▾ next'
if tmux has-session -t =latest 2>/dev/null; then fail "latest is still there after exit"; fi
[ "$(tmux show-options -gv detach-on-destroy)" = "$detach_on_destroy" ] ||
  fail "detach-on-destroy is $(tmux show-options -gv detach-on-destroy), was $detach_on_destroy"

# Once the last session has ended, the right side says there's none and
# that p opens a project, and the sidebar has focus; the tree is empty,
# with nothing in the status line. A session then made outside shows, with
# the cursor on it.
type_in t3 'C-\'
wait_until "the wtui key to focus the right side" focus_on t3 right
for s in api fix notes proj next; do tmux kill-session -t "=$s"; done
wait_until "the right side to say there's no session" right_shows t3 "No session to show"
right_shows t3 "p in the sidebar opens a project" || fail "the right side doesn't say p opens a project:
$(host t3 capture-pane -p -t "$(right_pane t3)")"
wait_until "the sidebar to take focus once the last session ended" focus_on t3 sidebar
wait_until "the sidebar to show no session" sidebar_shows t3 ' wtui'
if launcher_exited t3; then fail "wtui exited when the last session ended"; fi
new_session delta
wait_until "the right side to show delta, made outside" eval '[ "$(showing t3)" = delta ]'
wait_until "the cursor to be on delta" cursor_on t3 '1   ▾ delta'

echo "ok: wtui-host-start"
