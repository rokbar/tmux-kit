#!/usr/bin/env bash
# Leaving the host, end to end: prefix d on the right closes wtui, as detaching
# plain tmux would, and so does closing the terminal; either way the host
# cleans up after itself and the sessions live on. Two launches are two
# independent hosts.
set -euo pipefail
# shellcheck source=test/lib/host.sh
. "$(dirname "$0")/lib/host.sh"
need_host_tmux
build_wtui

new_session alpha
new_session beta

# two hosts: switching session in one doesn't move the other
launch t1
launch t2
first="$(showing t1)"
[ "$(showing t2)" = "$first" ] || fail "two launches start on '$first' and '$(showing t2)', want both on the most recent"
other=alpha
[ "$first" = alpha ] && other=beta
type_in t1 Escape
wait_until "esc to focus t1's right side" focus_on t1 right
type_in t1 C-b ')' # next session
wait_until "prefix ) in t1 to show $other" eval '[ "$(showing t1)" = "$other" ]'
[ "$(showing t2)" = "$first" ] || fail "t1 switching session moved t2 to '$(showing t2)'"
[ "$(focus t2)" = sidebar ] || fail "t1's keys moved t2's focus"

# prefix d on the right closes that host, not the other one
type_in t1 C-b d
wait_until "prefix d to close wtui in t1" launcher_exited t1
[ "$(launcher_status t1)" = 0 ] || fail "wtui exited $(launcher_status t1) after prefix d:
$(screen t1)"
wait_until "t1's host to end and clean up after prefix d" host_gone t1
[ -z "$(screen t1 | awk '!/^Pane is dead/' | tr -d ' \n')" ] || fail "after prefix d, t1 shows:
$(screen t1)"
[ "$(showing t2)" = "$first" ] || fail "t1's prefix d moved t2 to '$(showing t2)'"

# closing the terminal ends its host, which cleans up after itself
close_terminal t2
wait_until "t2's host to end and clean up once its terminal closed" host_gone t2
[ "$(tmux list-sessions -F '#{session_name}' | sort | tr '\n' ' ')" = "alpha beta " ] ||
  fail "the normal server's sessions are $(tmux list-sessions -F '#{session_name}')"
[ -z "$(tmux list-clients)" ] || fail "clients are left on the normal server: $(tmux list-clients)"

# the wtui key is the normal server's @wtui-key option, read at launch
tmux set-option -g @wtui-key 'C-]'
launch t3
root="$(host t3 list-keys -T root | awk '$NF != "send-keys" { print $4 }' | sort | tr '\n' ' ')"
[ "$root" = 'C-] MouseDown1Pane MouseDrag1Border ' ] || fail "with @wtui-key C-], the host's root table binds: $root"
type_in t3 'C-]'
wait_until "C-] to focus the right side" focus_on t3 right
type_in t3 'C-]'
wait_until "C-] to focus the sidebar" focus_on t3 sidebar
# (when the last session ends, the host stays: test/wtui-host-start.sh)

echo "ok: wtui-host-leave"
