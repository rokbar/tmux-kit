#!/usr/bin/env bash
# Jumping between sessions, end to end. Sessions are numbered in the
# sidebar. From the right side, the wtui key then 1-9, tab, ] or [ shows a
# session, and what's typed straight after lands there, even typed at once;
# ] and [ repeat. Every other sidebar key after the wtui key is pressed in
# the sidebar. In the sidebar, those keys and enter work alone, just as
# safely, and so does C-b.
set -euo pipefail
# shellcheck source=test/lib/host.sh
. "$(dirname "$0")/lib/host.sh"
need_host_tmux
build_wtui

# api: a plain folder with one session; work: one with four; zed: one with
# one, which becomes a git repo while wtui runs. alpha is the most recent
# (activity counts seconds). Windows are named, so automatic-rename leaves
# them be.
new_session api "$tmp/api"
for s in beta delta gamma; do new_session "$s" "$tmp/work"; done
new_session zed "$tmp/zed"
sleep 1
new_session alpha "$tmp/work"
for s in api alpha beta delta gamma zed; do tmux rename-window -t "=$s:" shell; done

# typed <session> <text>: <session>'s pane shows <text>, typed there (its
# panes run cat, so typing only echoes). The texts here avoid the letters
# sidebar keys use, so a key typed into the sidebar instead does nothing.
typed() { tmux capture-pane -p -t "=$1:" | grep -F -- "$2" >/dev/null; }
on() { [ "$(showing t1)" = "$1" ] && focus_on t1 right; }

launch t1
wait_until "the sidebar to number the sessions" sidebar_shows t1 ' wtui
  ▾ api
1   ▸ api
  ▾ work
2   ▾ alpha
        0 shell
3   ▸ beta
4   ▸ delta
5   ▸ gamma
  ▾ zed
6   ▸ zed'
type_in t1 Escape
wait_until "esc to focus the right side" on alpha

# From the right: the wtui key then 3, as a terminal sends a chord mapped
# onto them (together), then text typed at once: it lands in beta, number 3.
type_in t1 'C-\' 3
type_in t1 -l musta
wait_until "C-\\ 3 to show beta" on beta
wait_until "the text typed after C-\\ 3 to land in beta" typed beta musta
# the same, typed one key at a time, but as fast
type_in t1 'C-\'
type_in t1 4
type_in t1 -l mustb
wait_until "C-\\ then 4 to show delta" on delta
wait_until "the text typed after C-\\ 4 to land in delta" typed delta mustb

# tab: the last session, and again, back
type_in t1 'C-\' Tab
type_in t1 -l mustc
wait_until "C-\\ tab to show beta, the last session" on beta
wait_until "the text typed after C-\\ tab to land in beta" typed beta mustc
type_in t1 'C-\' Tab
wait_until "C-\\ tab again to show delta" on delta

# ] ] ] walks work's sessions from alpha, after the wtui key once; ] at the
# end goes round to the first, [ at the start to the last.
type_in t1 'C-\' 2
wait_until "C-\\ 2 to show alpha" on alpha
type_in t1 'C-\' ']' ']' ']'
type_in t1 -l mustf
wait_until "C-\\ ] ] ] to show gamma" on gamma
wait_until "the text typed after C-\\ ] ] ] to land in gamma" typed gamma mustf
type_in t1 'C-\' ']'
wait_until "C-\\ ] on gamma, the last, to show alpha" on alpha
type_in t1 'C-\' '['
wait_until "C-\\ [ on alpha, the first, to show gamma" on gamma
# a ] typed after the repeat time is the session's
sleep 1
type_in t1 ']'
type_in t1 -l mustm
wait_until "] typed a second later to land in gamma" typed gamma ']mustm'
[ "$(showing t1)" = gamma ] || fail "a ] typed a second after one showed $(showing t1)"

# api is alone in its project: C-\ ] there shows a hint, and focus is right,
# on api, all the same.
type_in t1 'C-\' 1
wait_until "C-\\ 1 to show api" on api
type_in t1 'C-\' ']'
type_in t1 -l musts
wait_until "C-\\ ] in api to say why it can't" eval 'sidebar_text t1 | grep -F "no other session in api" >/dev/null'
wait_until "focus to be on the right, on api" on api
wait_until "the text typed after C-\\ ] in api to land there" typed api musts

# The keys the host runs in the sidebar pass on as they are on the right.
for k in m 1 ']' '[' z Tab Enter; do type_in t1 "$k"; done
wait_until "1, ], [, z, tab and enter typed on the right to reach api" eval \
  '[ "$(tmux capture-pane -p -t =api: | grep -c "^musts.*m1\]\[z")" = 1 ]'
[ "$(showing t1)" = api ] || fail "keys typed on the right moved it to $(showing t1)"

# In the sidebar: enter on alpha, then text typed at once, lands in alpha.
type_in t1 'C-\'
wait_until "the wtui key to focus the sidebar" focus_on t1 sidebar
type_in t1 j j j
wait_until "j j j to move the cursor to alpha" eval '[ "$(cursor_row t1)" = "2   ▸ alpha" ]'
type_in t1 Enter
type_in t1 -l mustt
wait_until "enter on alpha to show it" on alpha
wait_until "the text typed after enter to land in alpha" typed alpha mustt
# the same with the text in the same burst, from beta
type_in t1 'C-\'
wait_until "the wtui key to focus the sidebar" focus_on t1 sidebar
type_in t1 j j # past alpha's window
wait_until "j j to move the cursor to beta" eval '[ "$(cursor_row t1)" = "3   ▸ beta" ]'
type_in t1 Enter m u s t u
wait_until "enter on beta to show it" on beta
wait_until "the text typed with enter to land in beta" typed beta mustu
# a number, tab, ] and [ alone in the sidebar
type_in t1 'C-\'
wait_until "the wtui key to focus the sidebar" focus_on t1 sidebar
type_in t1 5 m u s t v
wait_until "5 in the sidebar to show gamma" on gamma
wait_until "the text typed with 5 to land in gamma" typed gamma mustv
type_in t1 'C-\'
wait_until "the wtui key to focus the sidebar" focus_on t1 sidebar
type_in t1 Tab m u s t w
wait_until "tab in the sidebar to show beta, the last session" on beta
wait_until "the text typed with tab to land in beta" typed beta mustw
type_in t1 'C-\'
wait_until "the wtui key to focus the sidebar" focus_on t1 sidebar
type_in t1 '[' m u s t y
wait_until "[ in the sidebar to show alpha, before beta" on alpha
wait_until "the text typed with [ to land in alpha" typed alpha musty

# C-b c, typed at once in the sidebar, makes a window in the session shown.
type_in t1 'C-\'
wait_until "the wtui key to focus the sidebar" focus_on t1 sidebar
type_in t1 C-b c
wait_until "C-b c from the sidebar to make a window in alpha" \
  eval '[ "$(tmux list-windows -t =alpha -F x | wc -l | tr -d " ")" = 2 ]'
wait_until "C-b to focus the right side" on alpha

# The wtui key then r is r in the sidebar: it rebuilds the tree, looking each
# session's folder up in git again, so zed, a git repo now, lists its
# worktree.
git init -q -b main "$tmp/zed"
git -C "$tmp/zed" commit -q --allow-empty -m init
git -C "$tmp/zed" worktree add -q -b wip "$tmp/zed--wip"
type_in t1 'C-\' r
wait_until "C-\\ r to rebuild the tree with zed's worktree" eval 'sidebar_text t1 | grep -E "^      wip[[:blank:]]*$" >/dev/null'
[ "$(focus t1)" = sidebar ] || fail "after C-\\ r, focus is on the $(focus t1), want the sidebar"

echo "ok: wtui-host-jumps"
