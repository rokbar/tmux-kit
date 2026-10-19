#!/usr/bin/env bash
# The sidebar's tree, end to end: projects (a git main clone with its
# worktree, and a plain folder), their sessions, and the active session's
# windows. enter on another session shows it on the right, with focus there,
# and whenever the sidebar gets focus, its cursor is on the active session.
set -euo pipefail
# shellcheck source=test/lib/host.sh
. "$(dirname "$0")/lib/host.sh"
need_host_tmux
build_wtui

# api: a main clone and a worktree, each with a session; work: a plain
# folder with three sessions, beta the most recent (activity counts seconds).
git init -q -b main "$tmp/api"
git -C "$tmp/api" commit -q --allow-empty -m init
git -C "$tmp/api" worktree add -q -b fix "$tmp/api--fix"
new_session api "$tmp/api"
new_session fix "$tmp/api--fix"
new_session alpha "$tmp/work"
new_session gamma "$tmp/work"
sleep 1
new_session beta "$tmp/work"
tmux rename-window -t =beta: shell

launch t1
tree=' wtui
  ▾ api
    ▸ api
    ▸ fix
  ▾ work
    ▸ alpha
    ▾ beta
        0 shell
    ▸ gamma'
wait_until "the sidebar to show the tree" sidebar_shows t1 "$tree"
[ "$(focus t1)" = sidebar ] || fail "focus starts on the $(focus t1), want the sidebar"

# The sidebar shows it has focus: its cursor, on beta, the active session.
# Moving it changes nothing on the right; enter on gamma shows gamma there,
# with focus there too, and the cursor goes.
cursor_on() { [ "$(cursor_row t1)" = "$1" ]; }
wait_until "the cursor to show on beta" cursor_on '    ▾ beta'
type_in t1 j j
wait_until "j j to move the cursor to gamma" cursor_on '    ▸ gamma'
[ "$(showing t1)" = beta ] || fail "moving the cursor moved the right side to '$(showing t1)'"
type_in t1 Enter
wait_until "enter on gamma to show it on the right" eval '[ "$(showing t1)" = gamma ]'
wait_until "enter to focus the right side" focus_on t1 right
wait_until "the sidebar to show it lost focus" cursor_on ''

# Back in the sidebar (the wtui key), the cursor is on gamma, the active
# session, wherever it was before: here, moved to alpha before esc. So k,
# enter shows beta, the session above gamma.
type_in t1 'C-\'
wait_until "the wtui key to focus the sidebar" focus_on t1 sidebar
type_in t1 k k
type_in t1 Escape
wait_until "esc to focus the right side" focus_on t1 right
[ "$(showing t1)" = gamma ] || fail "esc moved the right side to '$(showing t1)'"
type_in t1 'C-\'
wait_until "the wtui key to focus the sidebar" focus_on t1 sidebar
type_in t1 k Enter
wait_until "k, enter from gamma to show beta" eval '[ "$(showing t1)" = beta ]'

echo "ok: wtui-host-tree"
