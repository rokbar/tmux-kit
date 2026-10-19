#!/usr/bin/env bash
# t -d [name] starts the folder's session, detached, if it's missing and prints
# its name. It never switches or attaches a client, inside tmux or out. Names
# are t's: the folder or the given name, minus a leading dot, . and : -> _.
set -euo pipefail
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "$0")/lib/sandbox.sh"

# t, quiet unless it fails
t() { "$kit/bin/t" "$@" >"$tmp/log" 2>&1 || { cat "$tmp/log" >&2; return 1; }; }
printed() { [ "$(cat "$1")" = "$2" ] || fail "$3 printed '$(cat "$1")', want '$2'"; }
# started_in <session> <dir>: <session> exists and was started in <dir>
started_in() {
  local path
  path="$(tmux display-message -p -t "=$1:" '#{session_path}' 2>/dev/null)" || fail "no session $1"
  [ "$path" = "$2" ] || fail "session $1 started in $path, want $2"
}
sessions() { tmux list-sessions -F '#{session_name}' | sort; }
clients() { tmux list-clients -F '#{client_session}' | sort; }

mkdir "$tmp/proj" "$tmp/.my.app" "$tmp/a:b"
cd "$tmp/proj"

# no server yet: t -d starts it, with the folder's session
t -d || fail "t -d failed with no tmux server"
printed "$tmp/log" proj "t -d in proj"
started_in proj "$tmp/proj"
[ "$(clients)" = "" ] || fail "t -d attached a client: $(clients)"

# an existing session of that name is reused, wherever it was started
tmux new-session -d -s other -c "$tmp"
before="$(sessions)"
t -d || fail "t -d failed on its own session"
printed "$tmp/log" proj "t -d again"
t -d other || fail "t -d other failed"
printed "$tmp/log" other "t -d other"
started_in other "$tmp"
[ "$(sessions)" = "$before" ] || fail "t -d created a session for an existing name: $(sessions)"

# naming: no leading dot, . and : -> _, for the folder and a given name
cd "$tmp/.my.app"
t -d || fail "t -d failed in .my.app"
printed "$tmp/log" my_app "t -d in .my.app"
started_in my_app "$tmp/.my.app"
cd "$tmp/a:b"
t -d || fail "t -d failed in a:b"
printed "$tmp/log" a_b "t -d in a:b"
started_in a_b "$tmp/a:b"
cd "$tmp/proj"
t -d .x.y:z || fail "t -d .x.y:z failed"
printed "$tmp/log" x_y_z "t -d .x.y:z"
started_in x_y_z "$tmp/proj"

# The rest runs t as typed in a pane: a terminal on stdin, $TMUX set unless
# `env -u TMUX` drops it. A tmux client, in a pane of session term, stands in
# for the terminal the user sees.

# wait_until <what> <command> [<arg>...]: runs the command every 0.1 s until it
# succeeds; fails the test after 20 s
wait_until() {
  local what="$1" i
  shift
  for ((i = 0; i < 200; i++)); do
    if "$@"; then return 0; fi
    sleep 0.1
  done
  fail "timed out waiting for $what"
}
# run_in <session> <word>...: runs the <word>s in a new window of <session>, in
# $tmp/proj. What they print goes to $run.out, their exit status to $run.status.
n=0
run_in() {
  local session="$1"
  shift
  n=$((n + 1))
  run="$tmp/run$n"
  # shellcheck disable=SC2016 # sh expands $0 and $@ in the pane, not here
  tmux new-window -d -t "=$session:" -c "$tmp/proj" \
    /bin/sh -c '"$@" >"$0.out" 2>&1; echo $? >"$0.status"' "$run" "$@"
}
# exited: waits for the last run_in's words to exit, and fails unless with 0
exited() {
  wait_until "$* to exit" test -e "$run.status"
  [ "$(cat "$run.status")" = 0 ] || fail "$* exited $(cat "$run.status"): $(cat "$run.out")"
}
clients_are() { [ "$(clients)" = "$1" ]; }

tmux new-session -d -s term -c "$tmp" env -u TMUX tmux attach -t =proj
wait_until "a client on proj" clients_are proj

# inside tmux: the client t -d runs from stays on its session
run_in proj "$kit/bin/t" -d fresh
exited "t -d fresh inside tmux"
printed "$run.out" fresh "t -d fresh inside tmux"
started_in fresh "$tmp/proj"
[ "$(clients)" = proj ] || fail "t -d inside tmux moved the client: $(clients)"
# without -d, t switches it, as before
run_in proj "$kit/bin/t" fresh
exited "t fresh inside tmux"
[ "$(clients)" = fresh ] || fail "t inside tmux didn't switch the client to fresh: $(clients)"

# outside tmux: t -d exits without attaching, though it has a terminal
run_in fresh env -u TMUX "$kit/bin/t" -d outside
exited "t -d outside, outside tmux"
printed "$run.out" outside "t -d outside, outside tmux"
started_in outside "$tmp/proj"
[ "$(clients)" = fresh ] || fail "t -d outside tmux attached a client: $(clients)"
# without -d, t attaches, as before
run_in fresh env -u TMUX "$kit/bin/t" outside
wait_until "t outside tmux to attach to outside" clients_are "fresh
outside"
tmux detach-client -s =outside
exited "t outside, outside tmux"

echo "ok: t-detached"
