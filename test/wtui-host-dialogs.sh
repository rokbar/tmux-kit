#!/usr/bin/env bash
# Dialogs, end to end. i shows the row's details, and n runs the picker, in a
# popup on the nested client: over the right side, with focus there until it
# closes, then back on the sidebar. Clicking the sidebar works while one is
# open. Whatever the picker opens shows on the right, and only there: another
# client of the normal server stays where it was. The picker's failure ends
# up in the status line. ? shows every shortcut over the whole host until any
# key; the sidebar's last line says so.
set -euo pipefail
# shellcheck source=test/lib/host.sh
. "$(dirname "$0")/lib/host.sh"
need_host_tmux
build_wtui

# fzf is a TUI. This stub, first on the normal server's PATH (which popups
# run with), notes where the picker ran, then does what $tmp/fzf-do says:
#   cancel: Esc
#   new <branch>: alt-enter with <branch> typed
mkdir "$tmp/stubs"
cat >"$tmp/stubs/fzf" <<EOF
#!/bin/sh
pwd -P >"$tmp/fzf-pwd"
cat >/dev/null
read -r what branch <"$tmp/fzf-do"
case "\$what" in
  cancel) exit 130 ;;
  new) echo "\$branch" ;;
esac
EOF
chmod +x "$tmp/stubs/fzf"
export PATH="$tmp/stubs:$PATH"

# api: a git project, its main clone and its worktree fix each with a
# session; fix has a commit of its own. Launched in fix, wtui starts on fix.
git init -q -b main "$tmp/api"
git -C "$tmp/api" commit -q --allow-empty -m init
git -C "$tmp/api" worktree add -q -b fix "$tmp/api--fix"
git -C "$tmp/api--fix" commit -q --allow-empty -m "fix the login form"
new_session api "$tmp/api"
new_session fix "$tmp/api--fix"
for s in api fix; do tmux rename-window -t "=$s:" shell; done

# The sidebar is wide enough for the status line to hold wt's messages,
# which name folders under $tmp (long on macOS).
tmux set-option -g @wtui-width 60
launch t1 "$tmp/api--fix"
tree=' wtui
  ▾ api
1   ▸ api
2   ▾ fix
        0 shell'
wait_until "the sidebar to show the tree" sidebar_shows t1 "$tree"

# shows <text>: the terminal shows <text>
shows() { case "$(screen t1)" in *"$1"*) ;; *) return 1 ;; esac; }
gone() { ! shows "$1"; }
last_line() { host t1 capture-pane -p -t "$(sidebar_pane t1)" | awk '{ sub(/ +$/, ""); last = $0 } END { print last }'; }
[ "$(last_line)" = "? shortcuts" ] || fail "the sidebar's last line is '$(last_line)', want '? shortcuts'"

# i: fix's details over the right side, focus there, until a key closes it
type_in t1 i
wait_until "i to show fix's details" shows "fix the login form"
shows "branch fix" || fail "the details don't name the branch:
$(screen t1)"
shows "not pushed" || fail "the details don't say fix isn't pushed:
$(screen t1)"
focus_on t1 right || fail "with the details open, focus is on the $(focus t1)"
# a click on the sidebar's row for api works meanwhile; esc goes back right
click t1 5 3
wait_until "a click to move the cursor to api" eval '[ "$(cursor_row t1)" = "1   ▸ api" ] && focus_on t1 sidebar'
shows "fix the login form" || fail "the click closed the details"
type_in t1 Escape
wait_until "esc to focus the right side, under the details" focus_on t1 right
type_in t1 q
wait_until "q to close the details, focus back on the sidebar" eval 'gone "fix the login form" && focus_on t1 sidebar'
[ "$(showing t1)" = fix ] || fail "after the details, the right side shows '$(showing t1)', want fix"

# ?: every shortcut, over the whole host; any key closes it
type_in t1 '?'
wait_until "? to show the shortcuts" shows "switch sides, showing a hidden sidebar"
shows "In the sidebar, press a key alone." || fail "the shortcuts don't say how keys work:
$(screen t1)"
type_in t1 x
wait_until "a key to close the shortcuts" gone "switch sides, showing a hidden sidebar"
focus_on t1 sidebar || fail "after the shortcuts, focus is on the $(focus t1)"
[ "$(showing t1)" = fix ] || fail "after the shortcuts, the right side shows '$(showing t1)', want fix"

# Another client of the normal server, on api, used last: in a popup, tmux's
# own pick of a client. The picker mustn't move it.
term new-session -d -s plain -x 80 -y 24 env -u TMUX tmux attach -t =api
plain_tty="$(term display-message -p -t =plain: '#{pane_tty}')"
plain_on() { tmux list-clients -F '#{client_tty} #{client_session}' | awk -v t="$plain_tty" '$1 == t { s = $2 } END { print s }'; }
wait_until "the other client to attach" eval '[ "$(plain_on)" = api ]'
type_in plain z

# n, cancelled: the picker ran in fix's folder, and nothing changed
echo cancel >"$tmp/fzf-do"
type_in t1 n
wait_until "n to run the picker" test -s "$tmp/fzf-pwd"
[ "$(cat "$tmp/fzf-pwd")" = "$tmp/api--fix" ] || fail "the picker ran in $(cat "$tmp/fzf-pwd"), want $tmp/api--fix"
wait_until "the cancelled picker to give focus back to the sidebar" eval 'gone " picker " && focus_on t1 sidebar'
[ "$(showing t1)" = fix ] || fail "after a cancelled picker, the right side shows '$(showing t1)', want fix"

# n, a failure: wt can't make a branch called "a b". It says why and waits
# for a key; then the status line says it too.
said="$(cd "$tmp/api--fix" && "$kit/bin/wt" -d "a b" 2>&1 >/dev/null || true)"
[ -n "$said" ] || fail "wt says nothing about the branch name 'a b'"
squeezed() { tr -d ' \n'; }
echo "new a b" >"$tmp/fzf-do"
type_in t1 n
wait_until "the picker to show wt's failure" shows "press any key"
type_in t1 Space
wait_until "the status line to say what wt said" eval '[[ "$(sidebar_text t1 | squeezed)" == *"$(squeezed <<<"$said")" ]]'
focus_on t1 sidebar || fail "after the picker failed, focus is on the $(focus t1)"

# n, a new branch: wt asks for its session's name in the popup, then shows
# the session on the right. The sidebar follows, with focus, and the status
# line says what wt did in the end.
echo "new fresh" >"$tmp/fzf-do"
type_in t1 n
wait_until "the picker to ask for the session's name" shows "session name [fresh]"
type_in t1 Enter
wait_until "the right side to show fresh" eval '[ "$(showing t1)" = fresh ]'
wait_until "focus to come back to the sidebar, on fresh" eval 'focus_on t1 sidebar && [ "$(cursor_row t1)" = "3   ▾ fresh" ]'
said="wt: $tmp/api--fresh (tmux session fresh)"
wait_until "the status line to say what wt did" eval '[[ "$(sidebar_text t1 | squeezed)" == *"$(squeezed <<<"$said")" ]]'
[ "$(git -C "$tmp/api--fresh" log -1 --format=%s)" = "fix the login form" ] || fail "fresh doesn't start from fix"
[ "$(plain_on)" = api ] || fail "the picker moved the other client to '$(plain_on)'"

echo "ok: wtui-host-dialogs"
