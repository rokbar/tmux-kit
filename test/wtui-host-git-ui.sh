#!/usr/bin/env bash
# The git UI, end to end: g runs it (wt git-ui) in the row's worktree, in a
# floating pane over the right side, with focus in it. The wtui key leaves it
# and comes back, it's still there after showing another session and coming
# back, and quitting it gives focus back to the sidebar. Without a git UI
# configured, wt's message shows until a key. Floating panes are new in tmux
# 3.7: below that, g pops the git UI up over the right side instead, which
# closes when it quits, with focus back on the sidebar.
set -euo pipefail
# shellcheck source=test/lib/host.sh
. "$(dirname "$0")/lib/host.sh"
need_host_tmux
v="$(tmux -V)" floats=1
if [[ $v =~ ([0-9]+)\.([0-9]+) ]] && ((BASH_REMATCH[1] * 100 + BASH_REMATCH[2] < 307)); then
  floats=""
fi
build_wtui

# The git UI: says where it runs, by its folder's name (and its whole path
# in $tmp/ran-in), then echoes each key typed, until q.
mkdir "$tmp/bin"
cat >"$tmp/bin/stub-ui" <<EOF
#!/bin/sh
pwd -P >"$tmp/ran-in"
EOF
cat >>"$tmp/bin/stub-ui" <<'EOF'
echo "stub git UI in [$(basename "$(pwd -P)")]"
stty -icanon -echo
while :; do
  k="$(dd bs=1 count=1 2>/dev/null)"
  if [ "$k" = q ]; then exit 0; fi
  printf '%s' "$k"
done
EOF
chmod +x "$tmp/bin/stub-ui"
# wt reads its config, and finds the git UI, in the normal server's
# environment, where the git UI's pane runs: set before the server starts.
export PATH="$tmp/bin:$PATH" WT_CONFIG="$tmp/config.sh"
echo "WT_GIT_UI=stub-ui" >"$WT_CONFIG"

# api: the main clone, with a session, and worktree fix, with one; notes, a
# plain folder with one. wtui starts on fix.
git init -q -b main "$tmp/api"
git -C "$tmp/api" commit -q --allow-empty -m init
git -C "$tmp/api" worktree add -q -b fix "$tmp/api--fix"
new_session api "$tmp/api"
new_session fix "$tmp/api--fix"
new_session notes "$tmp/notes"
for s in api fix notes; do tmux rename-window -t "=$s:" shell; done

# floating: the normal server's floating panes, one per line, as
# "<session>:<window> <pane> <active> <left>,<top> <width>x<height>"
floating() {
  { tmux list-panes -a -F '#{pane_floating_flag} #{session_name}:#{window_index} #{pane_id} #{pane_active} #{pane_left},#{pane_top} #{pane_width}x#{pane_height}' 2>/dev/null || true; } |
    awk '$1 == 1 { sub(/^1 /, ""); print }'
}
floats() { [ -n "$(floating)" ]; }
no_float() { [ -z "$(floating)" ]; }
# ran_in <folder>: the git UI last ran in <folder>
ran_in() { [ "$(cat "$tmp/ran-in" 2>/dev/null)" = "$1" ]; }
# git_ui_shows <text>: the floating pane shows <text>
git_ui_shows() {
  local pane
  pane="$(floating | awk '{ print $2; exit }')"
  [ -n "$pane" ] && tmux capture-pane -p -t "$pane" | grep -F -- "$1" >/dev/null
}
# on_the_right <text>: the terminal shows <text> on the right side, past
# the sidebar and its border
on_the_right() {
  local col
  col="$(screen t1 | awk -v t="$1" 'i = index($0, t) { print i; exit }')"
  [ -n "$col" ] && ((col > $(sidebar_width t1) + 1))
}
on() { [ "$(showing t1)" = "$1" ]; }

launch t1 "$tmp/api--fix"
wait_until "the sidebar to show fix active" eval 'sidebar_text t1 | grep -Fx "2   ▾ fix" >/dev/null'
type_in t1 Escape
wait_until "esc to focus the right side" focus_on t1 right

if [ -z "$floats" ]; then
  # From the right, the wtui key then g: fix's git UI pops up over the right
  # side, with focus there, and typing lands in it.
  type_in t1 'C-\' g
  wait_until "C-\\ g to pop the git UI up on the right side" on_the_right "stub git UI in [api--fix]"
  ran_in "$tmp/api--fix" || fail "the git UI ran in $(cat "$tmp/ran-in"), want $tmp/api--fix"
  focus_on t1 right || fail "with the git UI up, focus is on the $(focus t1)"
  type_in t1 -l abc
  wait_until "what's typed to land in the git UI" on_the_right abc
  # Quitting it closes the popup, and focus goes back to the sidebar.
  type_in t1 q
  wait_until "q to close the git UI" eval '! on_the_right "stub git UI in"'
  wait_until "focus to go back to the sidebar" focus_on t1 sidebar
  on fix || fail "after the git UI quit, the right side shows $(showing t1), want fix"
  # With no git UI configured, wt's message stays in the popup until a key.
  echo "WT_GIT_UI=" >"$WT_CONFIG"
  type_in t1 g
  wait_until "g to show wt's message" on_the_right "no git UI configured"
  on_the_right "press any key to close" || fail "wt's message doesn't say a key closes it"
  type_in t1 x
  wait_until "a key to close wt's message" eval '! on_the_right "no git UI configured"'
  wait_until "focus to go back to the sidebar after wt's message" focus_on t1 sidebar
  echo "ok: wtui-host-git-ui (a popup: floating panes are new in tmux 3.7, and this is $v)"
  exit 0
fi

# From the right, the wtui key then g: fix's git UI floats over the right
# side, one cell in from each edge of fix's window, with focus in it.
type_in t1 'C-\' g
wait_until "C-\\ g to float the git UI" git_ui_shows "stub git UI in [api--fix]"
ran_in "$tmp/api--fix" || fail "the git UI ran in $(cat "$tmp/ran-in"), want $tmp/api--fix"
wait_until "the git UI to show on the right side" on_the_right "stub git UI in [api--fix]"
size="$(tmux display-message -p -t =fix: '#{window_width} #{window_height}')"
want="fix:0 $(floating | awk '{ print $2 }') 1 1,1 $((${size% *} - 2))x$((${size#* } - 2))"
[ "$(floating)" = "$want" ] || fail "the floating panes are:
$(floating)
want one, $want (fix's window is $size)"
wait_until "focus to stay on the right side" focus_on t1 right
type_in t1 -l abc
wait_until "what's typed to land in the git UI" git_ui_shows abc

# The wtui key leaves it for the sidebar, and comes back into it.
type_in t1 'C-\'
wait_until "C-\\ to focus the sidebar" focus_on t1 sidebar
type_in t1 'C-\'
wait_until "C-\\ again to focus the right side" focus_on t1 right
type_in t1 -l def
wait_until "what's typed then to land in the git UI" git_ui_shows abcdef

# Showing another session hides it; coming back finds it still there.
type_in t1 'C-\' 3
wait_until "C-\\ 3 to show notes" on notes
wait_until "the git UI to be hidden with fix" eval '! on_the_right "stub git UI in"'
type_in t1 'C-\' 2
wait_until "C-\\ 2 to show fix again" on fix
wait_until "the git UI to show again" on_the_right "stub git UI in [api--fix]"
type_in t1 -l ghi
wait_until "what's typed after coming back to land in the git UI" git_ui_shows abcdefghi

# Quitting it closes it, and focus goes back to the sidebar.
type_in t1 q
wait_until "q to close the git UI" no_float
wait_until "focus to go back to the sidebar" focus_on t1 sidebar
on fix || fail "after the git UI quit, the right side shows $(showing t1), want fix"

# In the sidebar, g on another row (api, the main clone) runs the git UI in
# that row's folder, still over the right side: the cursor doesn't change it.
type_in t1 k
wait_until "the cursor to move up to api" eval '[ "$(cursor_row t1)" = "1   ▸ api" ]'
type_in t1 g
wait_until "g on api to float the git UI in api's main clone" git_ui_shows "stub git UI in [api]"
ran_in "$tmp/api" || fail "g on api ran the git UI in $(cat "$tmp/ran-in"), want $tmp/api"
[ "$(floating | awk '{ print $1 }')" = fix:0 ] || fail "g on api floated over $(floating), want fix's window"
on fix || fail "g on api showed $(showing t1) on the right, want fix still"
wait_until "focus to go to the git UI" focus_on t1 right
type_in t1 q
wait_until "q to close api's git UI" no_float
wait_until "focus to go back to the sidebar after api's git UI" focus_on t1 sidebar

# With no git UI configured, wt's message stays in the floating pane until a
# key, and then focus goes back to the sidebar.
echo "WT_GIT_UI=" >"$WT_CONFIG"
type_in t1 g
wait_until "g to show wt's message" git_ui_shows "no git UI configured"
git_ui_shows "press any key to close" || fail "wt's message doesn't say a key closes it"
type_in t1 x
wait_until "a key to close wt's message" no_float
wait_until "focus to go back to the sidebar after wt's message" focus_on t1 sidebar

echo "ok: wtui-host-git-ui"
