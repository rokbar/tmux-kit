# shellcheck shell=bash
# Sourced by the end-to-end test/wtui-host-*.sh (after set -euo pipefail) in
# place of test/lib/sandbox.sh, which it sources: the spec's seam 4. wtui,
# built from wtui/, runs in fake terminals over the sandbox's tmux server,
# which plays the user's normal server: bare `tmux` reaches it, since
# TMUX_TMPDIR is private and TMUX unset.
#
# A fake terminal is a session of another throwaway tmux server, the terminal
# server (`term`): its one pane is the terminal, typed into with send-keys and
# read with capture-pane. What's copied to its clipboard (OSC 52) becomes one
# of the terminal server's buffers. Each wtui launched in one starts a host,
# its own tmux server, in a folder of its own under $TMPDIR (here $tmp).
# Terminals are named, so a test can run several.
#
#   need_host_tmux               skips the test below tmux 3.6, wtui's minimum
#   build_wtui                   builds wtui (test/lib/wtui-build.sh)
#   launch <t> [<dir> [<word>...]]
#                                opens terminal <t> running wtui in <dir>
#                                (default $tmp), and waits until its host is up
#   open_terminal <t> [<dir> [<word>...]]
#                                the same without waiting
#   type_in <t> <key>...         types into <t>, as tmux send-keys names keys
#   click <t> <col> <row>        a left click, 1-based, as the terminal sends it
#   drag <t> <col> <row> <col> <row>
#                                a left-button drag from one cell to another
#   screen <t>                   what <t> shows
#   term <tmux args>             tmux on the terminal server
#   host <t> <tmux args>         tmux on <t>'s host server
#   sidebar_pane <t>, right_pane <t>
#                                the host's panes (ids)
#   focus <t>                    which side has focus: "sidebar" or "right"
#   focus_on <t> <side>          whether <side> has focus
#   showing <t>                  the session <t>'s right side shows, if any
#   nested_client <t>            the right side's client on the normal server
#                                (its name: a tty)
#   sidebar_width <t>            the sidebar's width in columns
#   wait_until <what> <cmd> [<arg>...]
#                                runs <cmd> every 0.1 s until it succeeds; after
#                                20 s fails the test, showing every terminal
#   launcher_exited <t>          whether wtui (the launcher) has exited
#   launcher_status <t>          its exit status, once it has
#   host_gone <t>                whether <t>'s host server has exited and its
#                                folder is gone (wtui cleans up after itself)
#   close_terminal <t>           closes <t>, as closing a terminal window does
#   new_session <name> [<dir>]   a session on the normal server, started in
#                                <dir> (default $tmp/<name>, made if missing)
#
# On exit it kills every server it started: the terminal server, every host
# left over, and the normal server (test/lib/sandbox.sh).

# shellcheck source=test/lib/wtui-build.sh
. "$(dirname "${BASH_SOURCE[0]}")/wtui-build.sh"

# Hosts make their folders under $TMPDIR: here, inside the sandbox.
export TMPDIR="$tmp"

# The terminal server loads no config.
term() { tmux -L wtui-term -f /dev/null "$@"; }

# Every host server is in a folder $tmp/wtui-*, its socket named "socket".
kill_servers() {
  local s
  term kill-server 2>/dev/null || true
  for s in "$tmp"/wtui-*/socket; do
    if [ -S "$s" ]; then tmux -S "$s" kill-server 2>/dev/null || true; fi
  done
  cleanup
}
trap kill_servers EXIT

need_host_tmux() {
  local v
  v="$(tmux -V)"
  if [[ ! $v =~ ([0-9]+)\.([0-9]+) ]] || ((BASH_REMATCH[1] * 100 + BASH_REMATCH[2] >= 306)); then
    return 0
  fi
  echo "skip: $(basename "$0"): wtui needs tmux 3.6 or newer, and this is $v"
  exit 0
}

new_session() {
  local dir="${2:-$tmp/$1}"
  mkdir -p "$dir"
  tmux new-session -d -s "$1" -c "$dir"
}

# The terminal is 120x35. Its pane stays after wtui exits, so its screen and
# exit status can be read. wtui runs as from a plain terminal: the terminal's
# own tmux server sets TMUX in its pane, so it's unset. The command is given
# to tmux as words, so no shell runs it.
open_terminal() {
  local t="$1" dir="${2:-$tmp}"
  shift
  if [ $# -gt 0 ]; then shift; fi
  rm -f "$tmp/$t.host"
  term new-session -d -s "$t" -x 120 -y 35 -c "$dir" \
    env -u TMUX -u TMUX_PANE "$@" "$HOME/.local/bin/wtui" \; \
    set-option -w -t "=$t:" remain-on-exit on \; \
    set-option -s set-clipboard on
}

launch() {
  open_terminal "$@"
  wait_until "wtui's host to come up in terminal $1" host_up "$1"
}

# host_up <t>: <t>'s host is up: the sidebar has drawn, and the right side's
# client is attached to the normal server.
host_up() {
  find_host "$1" && [ -n "$(host "$1" capture-pane -p -t "$(sidebar_pane "$1")" | tr -d ' \n')" ] &&
    [ -n "$(showing "$1")" ]
}

# find_host <t>: notes <t>'s host server, the one whose client is on <t>'s
# terminal, in $tmp/<t>.host, so it can still be named once it's gone.
find_host() {
  local tty s
  [ -s "$tmp/$1.host" ] && return 0
  tty="$(term display-message -p -t "=$1:" '#{pane_tty}')"
  for s in "$tmp"/wtui-*/socket; do
    [ -S "$s" ] || continue
    if [ "$(tmux -S "$s" list-clients -F '#{client_tty}' 2>/dev/null | awk -v t="$tty" '$0 == t')" = "$tty" ]; then
      echo "$s" >"$tmp/$1.host"
      return 0
    fi
  done
  return 1
}

host() {
  local t="$1"
  shift
  find_host "$t" || fail "terminal $t has no host"
  tmux -S "$(cat "$tmp/$t.host")" "$@"
}

type_in() {
  local t="$1"
  shift
  term send-keys -t "=$t:" "$@"
}

# SGR mouse reports (button 0 pressed, moved with it held, released)
click() {
  type_in "$1" -l $'\e[<0;'"$2;$3M"
  type_in "$1" -l $'\e[<0;'"$2;$3m"
}
drag() {
  type_in "$1" -l $'\e[<0;'"$2;$3M"
  type_in "$1" -l $'\e[<32;'"$4;$5M"
  type_in "$1" -l $'\e[<0;'"$4;$5m"
}

screen() { term capture-pane -p -t "=$1:"; }

# The sidebar is the pane running `wtui sidebar`; the right side, the other.
# awk reads to the end, so tmux never writes into a closed pipe (pipefail).
sidebar_pane() { host "$1" list-panes -F '#{pane_id} #{pane_start_command}' | awk '$NF == "sidebar" { id = $1 } END { print id }'; }
right_pane() { host "$1" list-panes -F '#{pane_id} #{pane_start_command}' | awk '$NF != "sidebar" { id = $1 } END { print id }'; }

focus() {
  local active
  active="$(host "$1" list-panes -F '#{pane_active} #{pane_id}' | awk '$1 == 1 { id = $2 } END { print id }')"
  if [ "$active" = "$(sidebar_pane "$1")" ]; then echo sidebar; else echo right; fi
}

focus_on() { [ "$(focus "$1")" = "$2" ]; }

sidebar_width() { host "$1" display-message -p -t "$(sidebar_pane "$1")" '#{pane_width}'; }

# The right side's client is the normal server's client on the right pane's
# tty; tmux names a client by its tty.
nested_client() { host "$1" display-message -p -t "$(right_pane "$1")" '#{pane_tty}'; }

# Space-separated: tmux turns a tab into _ without a UTF-8 locale.
showing() {
  local tty
  tty="$(nested_client "$1")"
  { tmux list-clients -F '#{client_tty} #{client_session}' 2>/dev/null || true; } |
    awk -v t="$tty" '$1 == t && !found { found = 1; sub(/^[^ ]* /, ""); print }'
}

wait_until() {
  local what="$1" i t
  shift
  for ((i = 0; i < 200; i++)); do
    if "$@"; then return 0; fi
    sleep 0.1
  done
  for t in $(term list-sessions -F '#{session_name}' 2>/dev/null); do
    what="$what
terminal $t shows:
$(screen "$t")"
  done
  fail "timed out waiting for $what"
}

launcher_exited() { [ "$(term display-message -p -t "=$1:" '#{pane_dead}')" = 1 ]; }
launcher_status() { term display-message -p -t "=$1:" '#{?pane_dead_signal,signal #{pane_dead_signal},#{pane_dead_status}}'; }

host_gone() {
  local s
  s="$(cat "$tmp/$1.host")"
  ! tmux -S "$s" list-sessions >/dev/null 2>&1 && [ ! -e "$(dirname "$s")" ]
}

close_terminal() { term kill-session -t "=$1"; }
