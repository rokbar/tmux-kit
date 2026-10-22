#!/usr/bin/env bash
# d, end to end: it kills the cursor's row, a window or a session, once a
# dialog over the right side has asked and y has answered it; n kills
# nothing. The dialog names the agents that end. Killing the active session
# moves the right side to the most recent session, with focus and the
# cursor on it in the sidebar, and leaves its worktree. On a project, d
# shows a hint.
set -euo pipefail
# shellcheck source=test/lib/host.sh
. "$(dirname "$0")/lib/host.sh"
need_host_tmux
build_wtui

# api: a git project, its main clone and its worktree fix each with a
# session; fix has two windows, shell and dev. notes, a plain folder's
# session, is made last. Launched in fix, wtui starts on fix.
git init -q -b main "$tmp/api"
git -C "$tmp/api" commit -q --allow-empty -m init
git -C "$tmp/api" worktree add -q -b fix "$tmp/api--fix"
new_session api "$tmp/api"
new_session fix "$tmp/api--fix"
tmux new-window -d -t "=fix:" -n dev -c "$tmp/api--fix"
new_session notes
for s in api fix notes; do tmux rename-window -t "=$s:0" shell; done

launch t1 "$tmp/api--fix"
wait_until "the sidebar to show the tree" sidebar_shows t1 ' wtui
  ▾ api
1   ▸ api
2   ▾ fix
        0 shell
        1 dev
  ▾ notes
3   ▸ notes'

shows() { case "$(screen t1)" in *"$1"*) ;; *) return 1 ;; esac; }
gone() { ! shows "$1"; }
squeezed() { tr -d ' \t\n'; }
# unmarked: lines without the agent state at their right edge, after blanks
# that may be tabs (see test/wtui-host-agents.sh)
unmarked() { sed -E 's/[[:blank:]]+(!|\*|✓)$//'; }
cursor_on() { [ "$(cursor_row t1 | unmarked)" = "$1" ]; }
windows_of() { tmux list-windows -t "=$1:" -F '#{window_name}' | awk '{ printf "%s%s", sep, $0; sep = " " }'; }

# api, shown just before fix, is the most recent session but fix: notes
# comes after fix in the tree, and was made after api
type_in t1 1
wait_until "1 to show api" eval '[ "$(showing t1)" = api ]'
type_in t1 'C-\' 2
wait_until "the wtui key, then 2, to show fix" eval '[ "$(showing t1)" = fix ]'
type_in t1 'C-\'
wait_until "the wtui key to focus the sidebar, on fix" eval 'focus_on t1 sidebar && cursor_on "2   ▾ fix"'

# on a project: a hint, and no dialog
type_in t1 k k d
wait_until "d on api's project row to show a hint" eval '[[ "$(sidebar_text t1 | squeezed)" == *"apiisn'"'"'tasessionorawindow"* ]]'
focus_on t1 sidebar || fail "after d on a project, focus is on the $(focus t1)"

# an agent at work in fix's dev window, as the plugin would set it
tmux set -p -t "=fix:1.0" @agent_state working
wait_until "the sidebar to see the agent" eval '[[ "$(sidebar_text t1)" == *"1 dev"*"*"* ]]'

# d on dev, answered n: the dialog names dev's agent; nothing is killed
type_in t1 j j j j
wait_until "the cursor to be on fix's dev" cursor_on "        1 dev"
type_in t1 d
wait_until "d to ask whether to kill dev" shows "Kill window 1 dev of session fix?"
shows "1 dev: working" || fail "the dialog doesn't name dev's agent:
$(screen t1)"
shows "y: kill   n/q/esc: cancel" || fail "the dialog doesn't say how to answer it:
$(screen t1)"
focus_on t1 right || fail "with the dialog open, focus is on the $(focus t1)"
# the dialog reaches the normal server by the socket the launcher found it on
dialog="$HOME/.local/bin/wtui -S $tmp/tmux-$(id -u)/default kill $(tmux display-message -p -t =fix:1 '#{session_id}:#{window_id}') "
ps ax -o args= | awk -v d="$dialog" 'index($0, d) == 1 { found = 1 } END { exit !found }' ||
  fail "no dialog runs as '${dialog}…'; wtui's processes are:
$(ps ax -o args= | awk -v w="$HOME/.local/bin/wtui" 'index($0, w) == 1')"
type_in t1 n
# as after any dialog, the cursor is back on the active session
wait_until "n to close the dialog, focus back on the sidebar" eval 'gone "Kill window 1 dev" && focus_on t1 sidebar && cursor_on "2   ▾ fix"'
[ "$(windows_of fix)" = "shell dev" ] || fail "after n, fix's windows are '$(windows_of fix)', want shell dev"

# d on dev, answered y: dev, and its agent, are gone; fix stays on the right
type_in t1 j j
wait_until "the cursor to be on fix's dev" cursor_on "        1 dev"
type_in t1 d
wait_until "d to ask again" shows "Kill window 1 dev of session fix?"
type_in t1 y
wait_until "y to kill dev" eval '[ "$(windows_of fix)" = shell ]'
wait_until "the tree to drop dev" eval '[ "$(sidebar_text t1 | unmarked)" = " wtui
  ▾ api
1   ▸ api
2   ▾ fix
        0 shell
  ▾ notes
3   ▸ notes" ]'
wait_until "focus to come back to the sidebar, on fix" eval 'focus_on t1 sidebar && cursor_on "2   ▾ fix"'
[ "$(showing t1)" = fix ] || fail "after killing dev, the right side shows '$(showing t1)', want fix"

# d on fix, the active session: it ends, and the right side shows api, the
# most recent session left, with the sidebar's focus and cursor on it. fix's
# worktree stays, a row without a session.
type_in t1 d
wait_until "d to ask whether to kill fix" shows "Kill session fix and its one window?"
shows "The worktree api--fix stays." || fail "the dialog doesn't say fix's worktree stays:
$(screen t1)"
type_in t1 y
wait_until "y to kill fix, and the right side to show api" eval '[ "$(showing t1)" = api ]'
if tmux has-session -t =fix 2>/dev/null; then fail "fix is still there after d and y"; fi
wait_until "the tree to drop fix" sidebar_shows t1 ' wtui
  ▾ api
1   ▾ api
        0 shell
      fix
  ▾ notes
2   ▸ notes'
wait_until "focus and the cursor to be on api" eval 'focus_on t1 sidebar && cursor_on "1   ▾ api"'
[ -d "$tmp/api--fix" ] || fail "killing fix's session removed its worktree's folder"
git -C "$tmp/api" worktree list --porcelain | awk -v d="worktree $tmp/api--fix" '$0 == d { found = 1 } END { exit !found }' ||
  fail "killing fix's session removed its worktree from git"

# d on notes, which ends some other way before y: the status line says what
# tmux said
type_in t1 j j j j
wait_until "the cursor to be on notes" cursor_on "2   ▸ notes"
notes="$(tmux display-message -p -t =notes: '#{session_id}')"
type_in t1 d
wait_until "d to ask whether to kill notes" shows "Kill session notes and its one window?"
tmux kill-session -t =notes
type_in t1 y
wait_until "the status line to say notes was gone" eval '[[ "$(sidebar_text t1 | squeezed)" == *"$(squeezed <<<"can'"'"'t find session: $notes")"* ]]'
focus_on t1 sidebar || fail "after a failed kill, focus is on the $(focus t1)"
[ "$(showing t1)" = api ] || fail "after a failed kill, the right side shows '$(showing t1)', want api"

echo "ok: wtui-host-kill"
