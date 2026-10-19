#!/usr/bin/env bash
# The tree keeps itself up to date, end to end: one control-mode client per
# host watches the normal server, following the right side's client. prefix
# s on the right to another session moves the highlight; sessions and
# windows created, renamed and killed outside show within a second or so;
# the control client leaves other sessions' recency alone, and ends with the
# host.
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

# the session the control client is on (it has no tty), if any
ctl_on() { tmux list-clients -F '#{client_control_mode} #{session_name}' | awk '$1 == 1 { sub(/^1 /, ""); print }'; }

# row_style <t> <row>: the SGR parameters of <row> in <t>'s sidebar, joined
# with ; between ;s. ESC becomes ~ first, so awk needn't match it.
row_style() {
  host "$1" capture-pane -e -p -t "$(sidebar_pane "$1")" | tr '\033' '~' | awk -v want="$2" '
    {
      line = $0; params = ";"
      while (match(line, /~\[[0-9;]*m/)) {
        params = params substr(line, RSTART + 2, RLENGTH - 3) ";"
        line = substr(line, 1, RSTART - 1) substr(line, RSTART + RLENGTH)
      }
      sub(/ +$/, "", line)
      if (line == want) { print params; exit }
    }'
}
highlighted() { case "$(row_style t1 "$1")" in *';36;'*) ;; *) return 1 ;; esac; }

# soon <what> <cmd> [<arg>...]: like wait_until, but within about 3 s: the
# tree follows events, it doesn't poll for them.
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

launch t1
wait_until "the sidebar to show the tree" sidebar_shows t1 ' wtui
  ▾ work
1   ▸ alpha
2   ▾ beta
        0 shell
3   ▸ gamma'

# The control client is on beta, the active session, with the right flags.
wait_until "the control client to attach to beta" eval '[ "$(ctl_on)" = beta ]'
flags="$(tmux list-clients -F '#{client_control_mode} #{client_flags}' | awk '$1 == 1 { print $2 }')"
for f in ignore-size no-output no-detach-on-destroy; do
  case ",$flags," in *",$f,"*) ;; *) fail "the control client's flags are $flags, want $f among them" ;; esac
done
sleep 1 # so that a bump of alpha's recency would show
alpha="$(tmux display-message -p -t =alpha: '#{session_activity} #{session_last_attached}')"

# prefix s on the right to gamma: the highlight moves to gamma, which shows
# its windows, beta folds again, and the control client follows. Back in the
# sidebar, the cursor is on gamma.
type_in t1 Escape
wait_until "esc to focus the right side" focus_on t1 right
type_in t1 C-b s
wait_until "prefix s to list the sessions" eval 'screen t1 | grep -q "gamma: "'
key="$(screen t1 | sed -n 's/.*(\([0-9a-z]\)) *[-+]* *gamma: .*/\1/p' | head -n 1)"
type_in t1 "$key"
wait_until "prefix s to show gamma on the right" eval '[ "$(showing t1)" = gamma ]'
tree=' wtui
  ▾ work
1   ▸ alpha
2   ▸ beta
3   ▾ gamma
        0 shell'
soon "the tree to follow the right side to gamma" sidebar_shows t1 "$tree"
soon "gamma to be highlighted" highlighted '3   ▾ gamma'
highlighted '2   ▸ beta' && fail "beta is still highlighted: $(row_style t1 '2   ▸ beta')"
wait_until "the control client to follow the right side to gamma" eval '[ "$(ctl_on)" = gamma ]'
type_in t1 'C-\'
wait_until "the wtui key to focus the sidebar" focus_on t1 sidebar
wait_until "the cursor to be on gamma" eval '[ "$(cursor_row t1)" = "3   ▾ gamma" ]'

# Sessions and windows created, renamed and killed outside show.
new_session kappa "$tmp/work"
soon "a session made outside to show" sidebar_shows t1 "$tree
4   ▸ kappa"
tmux new-window -d -t =gamma: -n logs
soon "a window made outside to show" sidebar_shows t1 "$tree
        1 logs
4   ▸ kappa"
tmux rename-window -t =gamma:1 server
tmux rename-session -t =kappa lambda
soon "a window and a session renamed outside to show" sidebar_shows t1 "$tree
        1 server
4   ▸ lambda"
tmux kill-window -t =gamma:1
tmux kill-session -t =lambda
soon "a window and a session killed outside to go" sidebar_shows t1 "$tree"

# alpha was never shown, so its recency is as it was.
[ "$(tmux display-message -p -t =alpha: '#{session_activity} #{session_last_attached}')" = "$alpha" ] ||
  fail "alpha's activity and last attach went from $alpha to $(tmux display-message -p -t =alpha: '#{session_activity} #{session_last_attached}')"

# The control client ends with the host: after q, and after closing the
# terminal.
type_in t1 q
wait_until "q to close the host" host_gone t1
[ -z "$(ctl_on)" ] || fail "after q, a control client is still on $(ctl_on)"
launch t2
wait_until "t2's control client to attach" eval '[ -n "$(ctl_on)" ]'
close_terminal t2
wait_until "t2's host to end" host_gone t2
wait_until "t2's control client to end with its host" eval '[ -z "$(ctl_on)" ]'

echo "ok: wtui-host-live"
