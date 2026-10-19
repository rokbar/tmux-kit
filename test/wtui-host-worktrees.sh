#!/usr/bin/env bash
# Worktrees in the sidebar's tree, end to end: a git project lists its main
# clone, then its worktrees in git's order, as wt lists them. A worktree with
# several sessions has them under it, and one without a session is dimmed,
# a missing one marked ✗. enter on a worktree without a session has wt open
# its session, then shows it on the right, with focus there.
set -euo pipefail
# shellcheck source=test/lib/host.sh
. "$(dirname "$0")/lib/host.sh"
need_host_tmux
build_wtui

# api: the main clone, with a session; worktree fix, with two (one started
# in a folder in it); worktree docs, without one; and a missing worktree.
# api is the most recent session (activity counts seconds).
git init -q -b main "$tmp/api"
git -C "$tmp/api" commit -q --allow-empty -m init
for b in fix docs gone; do git -C "$tmp/api" worktree add -q -b "$b" "$tmp/api--$b"; done
rm -rf "$tmp/api--gone"
new_session fix "$tmp/api--fix"
new_session fix-web "$tmp/api--fix/web"
sleep 1
new_session api "$tmp/api"
tmux rename-window -t =api: shell

# wtui runs in a folder outside the repo: wt lists api in its main clone
launch t1 "$tmp"
tree=' wtui
  ▾ api
    ▾ api
        0 shell
      docs
    ▾ fix
      ▸ fix
      ▸ fix-web
      api--gone ✗'
wait_until "the sidebar to show api's worktrees" sidebar_shows t1 "$tree"

# dimmed <row>: the sidebar's row <row>, as sidebar_text shows it, has an SGR
# 2 (dim) among its styles. ESC becomes ~ first, as for cursor_row.
dimmed() {
  host t1 capture-pane -e -p -t "$(sidebar_pane t1)" | tr '\033' '~' | awk -v row="$1" '
    {
      line = $0; dim = 0
      while (match(line, /~\[[0-9;]*m/)) {
        n = split(substr(line, RSTART + 2, RLENGTH - 3), p, ";")
        for (i = 1; i <= n; i++) if (p[i] == "2") dim = 1
        line = substr(line, 1, RSTART - 1) substr(line, RSTART + RLENGTH)
      }
      sub(/ +$/, "", line)
      if (line == row && dim) found = 1
    }
    END { exit !found }'
}
dimmed '      docs' || fail "docs, without a session, isn't dimmed"
dimmed '      api--gone ✗' || fail "api--gone, missing, isn't dimmed"
if dimmed '    ▾ fix'; then fail "fix, with sessions, is dimmed"; fi
cursor_on() { [ "$(cursor_row t1)" = "$1" ]; }
wait_until "the cursor to show on api" cursor_on '    ▾ api'

# enter on docs: wt opens its session, named after its branch, and the right
# side shows it, with focus there.
type_in t1 j j
wait_until "the cursor to move to docs" cursor_on '      docs'
type_in t1 Enter
wait_until "enter on docs to show its new session" eval '[ "$(showing t1)" = docs ]'
wait_until "enter to focus the right side" focus_on t1 right
[ "$(tmux display-message -p -t =docs: '#{session_path}')" = "$tmp/api--docs" ] ||
  fail "docs's session started in $(tmux display-message -p -t =docs: '#{session_path}'), want $tmp/api--docs"
# docs is now its session's row, active, so unfolded; its window's name is
# tmux's to choose, so windows are left out.
tree=' wtui
  ▾ api
    ▸ api
    ▾ docs
    ▾ fix
      ▸ fix
      ▸ fix-web
      api--gone ✗'
without_windows() { [ "$(sidebar_text t1 | awk '!/^ +[0-9]+ /')" = "$1" ]; }
wait_until "the sidebar to show docs's session" without_windows "$tree"
if dimmed '    ▾ docs'; then fail "docs, with a session now, is dimmed"; fi

echo "ok: wtui-host-worktrees"
