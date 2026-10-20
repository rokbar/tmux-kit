#!/usr/bin/env bash
# The host, end to end: wtui launched in a plain terminal (a fake one) opens a
# host over the normal server (the sandbox's), with the sidebar on the left
# and, on the right, a tmux client on the session with the most recent
# activity. Keys and the mouse switch sides, the nesting doesn't show, and q
# closes wtui, leaving the normal server as it was.
set -euo pipefail
# shellcheck source=test/lib/host.sh
. "$(dirname "$0")/lib/host.sh"
need_host_tmux
build_wtui

# beta has the most recent activity (session_activity counts seconds). A plain
# client shows it already, so it isn't what tmux itself would attach to. The
# normal server sets titles and has the mouse on, as tmux-kit's config does.
new_session alpha
tmux set-option -g set-titles on \; set-option -g set-titles-string '#S' \; set-option -g mouse on
new_session gamma
sleep 1
new_session beta
term new-session -d -s plain -x 80 -y 24 env -u TMUX tmux attach -t =beta

launch t1
[ "$(showing t1)" = beta ] || fail "the right side shows '$(showing t1)', want beta, the most recent session"
[ "$(host t1 list-panes | wc -l | tr -d ' ')" = 2 ] || fail "the host has panes: $(host t1 list-panes)"
[ "$(sidebar_width t1)" = 30 ] || fail "the sidebar is $(sidebar_width t1) columns wide, want 30"
# The right side's client: when its session ends, tmux moves it to the most
# recent session (no-detach-on-destroy), and it passes synchronized updates
# on (the sync feature; tmux 3.8 detects it in tmux anyway).
nested="$(tmux list-clients -F '#{client_tty} #{client_flags} #{client_termfeatures}' | awk -v t="$(nested_client t1)" '$1 == t')"
case "$nested" in *no-detach-on-destroy*) ;; *) fail "the right side's client isn't no-detach-on-destroy: $nested" ;; esac
case "$nested" in *sync*) ;; *) fail "the right side's client lacks the sync feature: $nested" ;; esac

# The host binds nothing but the wtui key (the normal server's @wtui-key,
# unset here: C-\), the mouse's (a click, a second or third click, a drag on
# the border) and the sidebar's held keys, so every other key reaches the
# side with focus. A held key, but in the sidebar, passes on as it is: its
# binding ends with send-keys, which sends the key pressed
# (test/wtui-host-jumps.sh types them on the right).
root="$(host t1 list-keys -T root | awk '$NF != "send-keys" { print $4 }' | sort | tr '\n' ' ')"
[ "$root" = 'C-\\ MouseDown1Pane MouseDrag1Border SecondClick1Pane TripleClick1Pane ' ] ||
  fail "the host's root table binds: $root"

# The terminal's title follows the session shown, whichever side has focus.
title() { [ "$(term display-message -p -t =t1: '#{pane_title}')" = "$1" ]; }
wait_until "the terminal's title to be beta" title beta

# The sidebar starts with focus. The wtui key there goes right, and on the
# right back to the sidebar; esc there goes right too.
[ "$(focus t1)" = sidebar ] || fail "focus starts on the $(focus t1), want the sidebar"
type_in t1 'C-\'
wait_until "the wtui key in the sidebar to focus the right side" focus_on t1 right
type_in t1 'C-\'
wait_until "the wtui key on the right to focus the sidebar" focus_on t1 sidebar
type_in t1 Escape
wait_until "esc in the sidebar to focus the right side" focus_on t1 right
[ "$(showing t1)" = beta ] || fail "switching sides moved the right side to '$(showing t1)'"

# The user's bindings work on the right (prefix ) is the next session), and
# the title follows.
type_in t1 C-b ')'
wait_until "prefix ) on the right to show gamma" eval '[ "$(showing t1)" = gamma ]'
wait_until "the terminal's title to follow, to gamma" title gamma
type_in t1 C-b '('
wait_until "prefix ( on the right to show beta again" eval '[ "$(showing t1)" = beta ]'

# C-b in the sidebar goes right and passes the prefix on: C-b c there makes a
# window in the session on the right.
windows() { tmux list-windows -t "=$1" -F x | wc -l | tr -d ' '; }
type_in t1 'C-\'
wait_until "the wtui key to focus the sidebar" focus_on t1 sidebar
type_in t1 C-b
wait_until "C-b in the sidebar to focus the right side" focus_on t1 right
type_in t1 c
wait_until "C-b c from the sidebar to make a window in beta" eval '[ "$(windows beta)" = 2 ]'

# A click focuses a side, however quickly it follows the last: tmux before
# 3.7 takes a click within double-click-time (300 ms) of the last for a
# second click (SecondClick1Pane), even on another pane, and the next for a
# third (TripleClick1Pane). Here the right side has focus, and the mouse
# hasn't been clicked yet: a click there, then at once one on the sidebar,
# are a click and a second click. Dragging the border (column 31) resizes
# the sidebar.
click t1 80 10
click t1 5 10
wait_until "a quick second click, on the sidebar, to focus it" focus_on t1 sidebar
click t1 80 10
wait_until "a click on the right side to focus it" focus_on t1 right
drag t1 31 10 25 10
wait_until "dragging the border to make the sidebar 24 columns wide" eval '[ "$(sidebar_width t1)" = 24 ]'

# The nesting doesn't show: the mouse wheel on the right reaches the session
# (copy mode), 24-bit colour reaches the terminal, and so does a copy to the
# clipboard (OSC 52).
type_in t1 -l $'\e[<64;80;10M'
wait_until "the wheel on the right to put beta in copy mode" \
  eval '[ "$(tmux display-message -p -t =beta: "#{pane_in_mode}")" = 1 ]'
tmux send-keys -t =beta: -X cancel
tmux split-window -d -t =beta: "printf '\\033[48;2;10;200;30mtruecolour\\033[0m'; exec cat >/dev/null"
wait_until "24-bit colour to reach the terminal" eval 'term capture-pane -e -p -t =t1: | grep "48;2;10;200;30m" >/dev/null'
tmux set-buffer -w -t "$(nested_client t1)" copied-on-the-right
wait_until "a copy on the right to reach the terminal's clipboard" \
  eval '[ "$(term show-buffer 2>/dev/null)" = copied-on-the-right ]'

# q in the sidebar closes wtui: the launcher exits 0, and the host has ended
# and removed its socket and config. The normal server is as it was: its
# sessions live on, and its detach-on-destroy is unchanged.
detach_on_destroy="$(tmux show-options -gv detach-on-destroy)"
type_in t1 'C-\'
wait_until "the wtui key to focus the sidebar" focus_on t1 sidebar
type_in t1 q
wait_until "q to close wtui" launcher_exited t1
[ "$(launcher_status t1)" = 0 ] || fail "wtui exited $(launcher_status t1) after q:
$(screen t1)"
wait_until "the host to end and clean up after q" host_gone t1
# nothing from wtui or tmux, but the terminal's own note that its pane has ended
[ -z "$(screen t1 | awk '!/^Pane is dead/' | tr -d ' \n')" ] || fail "after q, the terminal shows:
$(screen t1)"
[ "$(tmux list-sessions -F '#{session_name}' | sort | tr '\n' ' ')" = "alpha beta gamma " ] ||
  fail "after q, the normal server's sessions are $(tmux list-sessions -F '#{session_name}')"
[ "$(tmux show-options -gv detach-on-destroy)" = "$detach_on_destroy" ] ||
  fail "detach-on-destroy is $(tmux show-options -gv detach-on-destroy), was $detach_on_destroy"
[ "$(tmux list-clients -t =beta -F x | wc -l | tr -d ' ')" = 1 ] ||
  fail "after q, beta's clients are: $(tmux list-clients -t =beta)"

echo "ok: wtui-host"
