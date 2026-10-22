#!/usr/bin/env bash
# Agent state in the sidebar, end to end: a pane's @agent_state (as
# tmux-kit's Claude Code plugin sets it, here by hand) shows at the right
# edge of its session's row and every row above it within about a second,
# the most urgent winning; "done" clears once its window is shown, including
# when it arrives in a window shown already, and no other state ever does.
set -euo pipefail
# shellcheck source=test/lib/host.sh
. "$(dirname "$0")/lib/host.sh"
need_host_tmux
build_wtui

# work: a plain folder with three sessions, beta the most recent (activity
# counts seconds). Windows are named, so automatic-rename leaves them be.
new_session alpha "$tmp/work"
new_session gamma "$tmp/work"
sleep 1
new_session beta "$tmp/work"
for s in alpha beta gamma; do tmux rename-window -t "=$s:" shell; done

# agent <session> <state>: sets the agent state of <session>'s pane, as the
# plugin does
agent() { tmux set -p -t "=$1:0.0" @agent_state "$2" \; set -p -t "=$1:0.0" @agent_state_at "$(date +%s)"; }
# options <session>: its pane's agent state options, as tmux shows them
options() { tmux show-options -p -t "=$1:0.0" | awk '/^@agent_state/ { printf "%s%s", sep, $0; sep = ", " }'; }

# mark <row>: the agent state at the right edge of <row> in t1's sidebar (a
# row shows as its text, blanks, then the mark), "" for none. The blanks may
# be tabs: the sidebar's renderer moves over blank cells with them, and tmux
# keeps them in what it captures.
mark() {
  sidebar_text t1 | awk -v row="$1" '
    index($0, row) == 1 {
      rest = substr($0, length(row) + 1)
      if (rest !~ /^[ \t]/ && rest != "") next
      gsub(/[ \t]/, "", rest)
      print rest
      exit
    }'
}
marked() { [ "$(mark "$1")" = "$2" ]; }
# unmarked: lines without their marks (alternatives, as ✓ is several bytes)
unmarked() { sed -E 's/[[:blank:]]+(!|\*|✓)$//'; }

# soon <what> <cmd> [<arg>...]: like wait_until, but within about 3 s: the
# subscription reports changes once a second.
soon() {
  local what="$1" i
  shift
  for ((i = 0; i < 30; i++)); do
    if "$@"; then return 0; fi
    sleep 0.1
  done
  fail "$what didn't happen within 3 s; the sidebar shows:
$(sidebar_text t1)"
}

agent gamma done # before launch: the snapshot has it
launch t1
wait_until "the sidebar to show the tree" eval '[ "$(sidebar_text t1 | unmarked)" = " wtui
  ▾ work
1   ▸ alpha
2   ▾ beta
        0 shell
3   ▸ gamma" ]'
marked '3   ▸ gamma' ✓ || fail "at launch, gamma is marked '$(mark '3   ▸ gamma')', want ✓"
marked '  ▾ work' ✓ || fail "at launch, work is marked '$(mark '  ▾ work')', want ✓"
marked '2   ▾ beta' '' || fail "at launch, beta is marked '$(mark '2   ▾ beta')', want nothing"

# needs you, in a session that isn't shown, on its row and its project's,
# whose done it outranks
agent alpha needs-you
soon "alpha to need you" marked '1   ▸ alpha' '!'
marked '  ▾ work' '!' || fail "work is marked '$(mark '  ▾ work')', want ! from alpha"
agent beta working
soon "beta to be working" marked '2   ▾ beta' '*'
marked '        0 shell' '*' || fail "beta's shell is marked '$(mark '        0 shell')', want *"

# enter on gamma shows it: its done clears, on the normal server too
type_in t1 j j
wait_until "the cursor to be on gamma" eval '[ "$(cursor_row t1 | unmarked)" = "3   ▸ gamma" ]'
type_in t1 Enter
wait_until "enter to show gamma" eval '[ "$(showing t1)" = gamma ]'
soon "gamma's done to clear" marked '3   ▾ gamma' ''
soon "gamma's options to go" eval '[ -z "$(options gamma)" ]'

# Other states never clear: alpha shown (from outside, as prefix s would)
# still needs you, and beta, shown before, is still working.
tmux switch-client -c "$(nested_client t1)" -t =alpha
wait_until "alpha to be shown" eval '[ "$(showing t1)" = alpha ]'
sleep 1.5 # a subscription's worth and then some
marked '1   ▾ alpha' '!' || fail "with alpha shown, it's marked '$(mark '1   ▾ alpha')', want !"
case "$(options alpha)" in '@agent_state needs-you, @agent_state_at '*) ;; *) fail "alpha's options are $(options alpha)" ;; esac
case "$(options beta)" in '@agent_state working, @agent_state_at '*) ;; *) fail "beta's options are $(options beta)" ;; esac

# done arriving in the window shown clears too
agent alpha done
soon "alpha's done, shown, to clear" eval '[ -z "$(options alpha)" ]'
soon "alpha's row to show nothing" marked '1   ▾ alpha' ''
marked '  ▾ work' '*' || fail "work is marked '$(mark '  ▾ work')', want * from beta"

echo "ok: wtui-host-agents"
