# shellcheck shell=bash
# Sourced by the popup's whole-stack test/wtui-*.sh (after set -euo pipefail)
# in place of test/lib/sandbox.sh, which it sources: builds wtui and runs the
# popup (`wtui popup`, kept until the host replaces it) in a pane of the
# sandbox's private tmux server, with a tmux client showing that pane, as in
# real use. Defines build_wtui (from test/lib/wtui-build.sh), start_wtui,
# press, screen, wait_for, status_is, wait_until, wtui_exited, wtui_status,
# client_session, client_on and session_in. Keys that act on rows do nothing
# until wtui's list has arrived, so wait_for a row before pressing them.

# shellcheck source=test/lib/wtui-build.sh
. "$(dirname "${BASH_SOURCE[0]}")/wtui-build.sh"

# start_wtui <dir> [<word>...]: runs the popup, `wtui popup`, in <dir>, in a
# new session "wtui", and attaches a tmux client to that session from the pane
# of another session, "term", which stands in for a terminal. <word>s go
# before wtui on its command line, e.g. `env -u TMUX` to run it as if outside
# tmux. Ends the previous wtui and term sessions first. Sets wtui_pane and
# term_pane (pane ids). The pane stays after wtui exits, so its screen and
# exit status can be read. Both sessions start in $tmp, so neither counts as a
# worktree's session. Commands are given to tmux as words, so the user's shell
# doesn't run them.
# wtui's window keeps its 100x30 (window-size manual) when the client attaches:
# a resize just as wtui starts can come before Bubble Tea listens for SIGWINCH,
# and wtui would go on drawing for the old size, garbling the screen.
start_wtui() {
  local dir="$1"
  shift
  tmux kill-session -t =term 2>/dev/null || true
  tmux kill-session -t =wtui 2>/dev/null || true
  # shellcheck disable=SC2016 # sh expands $1 and $@ in the pane, not here
  wtui_pane="$(tmux new-session -d -P -F '#{pane_id}' -s wtui -x 100 -y 30 -c "$tmp" \
    /bin/sh -c 'cd "$1" && shift && exec "$@"' sh "$dir" "$@" "$HOME/.local/bin/wtui" popup \; \
    set-option -w -t =wtui: remain-on-exit on \; \
    set-option -w -t =wtui: window-size manual)"
  term_pane="$(tmux new-session -d -P -F '#{pane_id}' -s term -x 100 -y 30 -c "$tmp" \
    env -u TMUX tmux attach -t =wtui)"
  wait_until "a tmux client on session wtui" client_on "$term_pane" wtui
}

# press <key>...: sends keys to wtui, as tmux send-keys names them
press() { tmux send-keys -t "$wtui_pane" "$@"; }

# screen: what wtui's pane shows
screen() { tmux capture-pane -p -t "$wtui_pane"; }

# wait_for <text>: waits for wtui's pane to show <text>
wait_for() { wait_until "wtui to show '$1'" shows "$1"; }
shows() { case "$(screen)" in *"$1"*) ;; *) return 1 ;; esac; }

# status_is <text>: whether wtui's status line, the line above its keys, is
# <text>. awk reads to the end, so tmux never writes into a closed pipe
# (pipefail).
status_is() {
  [ "$(screen | awk 'index($0, "↑/↓ j/k: move") == 1 && !done { print prev; done = 1 } { prev = $0 }')" = "$1" ]
}

# wait_until <what> <command> [<arg>...]: runs the command every 0.1 s until it
# succeeds. After 20 s it fails the test, naming <what> and showing wtui's pane.
wait_until() {
  local what="$1" i
  shift
  for ((i = 0; i < 200; i++)); do
    if "$@"; then return 0; fi
    sleep 0.1
  done
  fail "timed out waiting for $what. wtui's pane shows:
$(screen)"
}

# wtui_exited: whether wtui has exited (its pane is dead) and tmux has its exit
# status. tmux marks the pane dead once its terminal closes, but only has the
# status once it has reaped wtui, on SIGCHLD. Before 3.6, tmux can lose that
# signal (libutempter sets SIGCHLD to SIG_DFL while it updates utmp for the
# closing pane), and wtui stays a zombie, so a dead pane without a status gets
# the server a SIGCHLD of its own, as tmux 3.6 does itself.
wtui_exited() {
  case "$(tmux display-message -p -t "$wtui_pane" '#{pane_dead} #{pane_dead_status}#{pane_dead_signal}')" in
    "1 "?*) ;;
    "1 ")
      kill -CHLD "$(tmux display-message -p '#{pid}')" 2>/dev/null || true
      return 1
      ;;
    *) return 1 ;;
  esac
}

# wtui_status: wtui's exit status, or "signal <n>", once it has exited
wtui_status() { tmux display-message -p -t "$wtui_pane" '#{?pane_dead_signal,signal #{pane_dead_signal},#{pane_dead_status}}'; }

# client_session <pane>: the session of the tmux client running in <pane>, if
# one is. Space-separated: tmux turns a tab into _ without a UTF-8 locale. awk
# reads to the end, so tmux never writes into a closed pipe (pipefail).
client_session() {
  local tty
  tty="$(tmux display-message -p -t "$1" '#{pane_tty}')"
  tmux list-clients -F '#{client_tty} #{client_session}' |
    awk -v t="$tty" '$1 == t && !found { found = 1; sub(/^[^ ]* /, ""); print }'
}

# client_on <pane> <session>: whether the tmux client in <pane> shows <session>
client_on() { [ "$(client_session "$1")" = "$2" ]; }

# session_in <dir>: the names of the sessions started in <dir>, one per line
session_in() {
  { tmux list-sessions -F '#{session_name} #{session_path}' 2>/dev/null || true; } |
    awk -v d="$1" '{ name = $1; sub(/^[^ ]* /, "") } $0 == d { print name }'
}
