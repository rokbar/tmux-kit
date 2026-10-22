#!/usr/bin/env bash
# p, end to end. It offers the folders in the projects root (~/Projects
# while @wtui-projects-root isn't set), worktrees left out, in fzf over the
# right side, and the one chosen becomes the active session: a git repo
# opened through wt, which the status line quotes, a plain folder through t,
# named as t names it. Only the right side moves: another client of the
# normal server, used last, stays where it was. Cancelling changes nothing,
# and a missing or empty root says so. With no session on the right, p works
# too, in a popup on the host over the right side, which reaches the normal
# server by its socket, and the right side then shows what it opened.
set -euo pipefail
# shellcheck source=test/lib/host.sh
. "$(dirname "$0")/lib/host.sh"
need_host_tmux
build_wtui

# fzf is a TUI. This stub, first on PATH (the normal server's, which popups
# on the nested client run with, and the host's), notes what it's offered,
# then does what $tmp/fzf-do says:
#   cancel: Esc
#   pick <name>: chooses <name>
#   hold <name>: waits until $tmp/fzf-go exists, then chooses <name>
mkdir "$tmp/stubs"
cat >"$tmp/stubs/fzf" <<EOF
#!/bin/sh
cat >"$tmp/fzf-offered"
read -r what name <"$tmp/fzf-do"
case "\$what" in
  cancel) exit 130 ;;
  hold) while [ ! -e "$tmp/fzf-go" ]; do sleep 0.1; done ;;
esac
echo "\$name"
EOF
chmod +x "$tmp/stubs/fzf"
export PATH="$tmp/stubs:$PATH"

root="$HOME/Projects"
new_session start "$tmp/start"
# The sidebar is wide enough for the status line to hold wt's messages,
# which name folders under $tmp (long on macOS).
tmux set-option -g @wtui-width 50
launch t1 "$tmp/start"

# shows <text>: the terminal shows <text>
shows() { case "$(screen t1)" in *"$1"*) ;; *) return 1 ;; esac; }
gone() { ! shows "$1"; }
sessions() { tmux list-sessions -F '#{session_name}' 2>/dev/null | sort | tr '\n' ' '; }
started_in() { [ "$(tmux display-message -p -t "=$1:" '#{session_path}')" = "$2" ]; }
# p_says <what> <text>: p shows <text> over the right side, until a key
# closes it and focus is back on the sidebar
p_says() {
  type_in t1 p
  wait_until "p to say $1" shows "$2"
  focus_on t1 right || fail "with p's message open, focus is on the $(focus t1)"
  type_in t1 q
  wait_until "a key to close p's message, focus back on the sidebar" eval 'gone "$2" && focus_on t1 sidebar'
}

# a missing root, then an empty one
p_says "the root is missing" "~/Projects is missing"
mkdir "$root"
p_says "the root is empty" "~/Projects has no folders"
[ "$(showing t1)" = start ] || fail "after p's messages, the right side shows '$(showing t1)', want start"

# api: a git repo, with its worktree api--fix next to it; my.notes, a plain
# folder; and what isn't offered: a hidden folder and a file
git init -q -b main "$root/api"
git -C "$root/api" commit -q --allow-empty -m init
git -C "$root/api" worktree add -q -b fix "$root/api--fix"
mkdir "$root/my.notes" "$root/.trash"
touch "$root/README"

# Another client of the normal server, on start, used last: in a popup,
# tmux's own pick of a client. Opening a project mustn't move it.
term new-session -d -s plain -x 80 -y 24 env -u TMUX tmux attach -t =start
plain_tty="$(term display-message -p -t =plain: '#{pane_tty}')"
plain_on() { tmux list-clients -F '#{client_tty} #{client_session}' | awk -v t="$plain_tty" '$1 == t { s = $2 } END { print s }'; }
wait_until "the other client to attach" eval '[ "$(plain_on)" = start ]'
type_in plain z

# p, cancelled: fzf is offered api and my.notes, and nothing changes
echo cancel >"$tmp/fzf-do"
before="$(sessions)"
type_in t1 p
wait_until "p to run fzf" test -s "$tmp/fzf-offered"
[ "$(cat "$tmp/fzf-offered")" = "api
my.notes" ] || fail "p offered:
$(cat "$tmp/fzf-offered")
want api and my.notes"
wait_until "the cancelled p to give focus back to the sidebar" eval 'gone " open a project " && focus_on t1 sidebar'
[ "$(showing t1)" = start ] || fail "after a cancelled p, the right side shows '$(showing t1)', want start"
[ "$(sessions)" = "$before" ] || fail "a cancelled p changed the sessions from $before to $(sessions)"

# p, api: wt opens the main clone's session, named after its branch, and
# the right side shows it, the sidebar's cursor on it. The status line says
# what wt did.
squeezed() { tr -d ' \n'; }
status_says() { [[ "$(sidebar_text t1 | squeezed)" == *"$(squeezed <<<"$1")" ]]; }
echo "pick api" >"$tmp/fzf-do"
type_in t1 p
wait_until "the right side to show api's session, main" eval '[ "$(showing t1)" = main ]'
started_in main "$root/api" || fail "main started in $(tmux display-message -p -t =main: '#{session_path}'), want $root/api"
wait_until "focus to come back to the sidebar, on main" eval 'focus_on t1 sidebar && [ "$(cursor_row t1)" = "1   ▾ main" ]'
[ "$(plain_on)" = start ] || fail "opening api moved the other client to '$(plain_on)'"
wait_until "the status line to say what wt did" status_says "wt: $root/api (tmux session main)"

# p, my.notes: t opens its session, named as t names it. t says nothing, so
# neither does the status line.
echo "pick my.notes" >"$tmp/fzf-do"
type_in t1 p
wait_until "the right side to show my.notes' session, my_notes" eval '[ "$(showing t1)" = my_notes ]'
started_in my_notes "$root/my.notes" || fail "my_notes started in $(tmux display-message -p -t =my_notes: '#{session_path}'), want $root/my.notes"
wait_until "focus to come back to the sidebar, on my_notes" eval 'focus_on t1 sidebar && [ "$(cursor_row t1)" = "2   ▾ my_notes" ]'
[ "$(plain_on)" = start ] || fail "opening my.notes moved the other client to '$(plain_on)'"
wait_until "the status line to say nothing" eval '! status_says "wt: $root/api (tmux session main)"'

# With no session left, p opens over the right side on the host, and what
# it opens shows on the right. The host's popup reaches the normal server by
# its socket, whatever the environment it runs in says: here its
# TMUX_TMPDIR, which would lead wt and t to another server (one t would
# start, killed on exit).
mkdir "$tmp/elsewhere"
elsewhere="$tmp/elsewhere/tmux-$(id -u)/default"
trap 'tmux -S "$elsewhere" kill-server 2>/dev/null || true; kill_servers' EXIT
host t1 set-environment -g TMUX_TMPDIR "$tmp/elsewhere"
no_session() {
  wait_until "the right side to say there's no session" eval 'host t1 capture-pane -p -t "$(right_pane t1)" | grep -q "No session to show"'
  wait_until "the sidebar to take focus" focus_on t1 sidebar
}
for s in main my_notes start; do tmux kill-session -t "=$s"; done
no_session
echo "hold my.notes" >"$tmp/fzf-do"
type_in t1 p
wait_until "p to open over the right side" eval 'screen t1 | head -1 | grep -q "│┌.*open a project"'
screen t1 | tail -1 | grep -q "│└" || fail "p's popup doesn't reach the bottom of the right side:
$(screen t1)"
touch "$tmp/fzf-go"
wait_until "the right side to show my_notes" eval '[ "$(showing t1)" = my_notes ]'
started_in my_notes "$root/my.notes" || fail "my_notes started in $(tmux display-message -p -t =my_notes: '#{session_path}'), want $root/my.notes"
wait_until "the cursor to be on my_notes" eval '[ "$(cursor_row t1)" = "1   ▾ my_notes" ]'

# api, the same way: through wt, and the status line says what wt did
tmux kill-session -t =my_notes
no_session
echo "pick api" >"$tmp/fzf-do"
type_in t1 p
wait_until "the right side to show main" eval '[ "$(showing t1)" = main ]'
started_in main "$root/api" || fail "main started in $(tmux display-message -p -t =main: '#{session_path}'), want $root/api"
wait_until "the cursor to be on main" eval '[ "$(cursor_row t1)" = "1   ▾ main" ]'
wait_until "the status line to say what wt did" status_says "wt: $root/api (tmux session main)"
[ ! -S "$elsewhere" ] || fail "p started a server in TMUX_TMPDIR's folder: $(tmux -S "$elsewhere" list-sessions 2>&1)"

echo "ok: wtui-host-projects"
