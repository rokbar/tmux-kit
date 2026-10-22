#!/usr/bin/env bash
# x, end to end: on a worktree with uncommitted changes, the remove dialog
# over the right side names every session that ends with it and the agent in
# one; wt refuses for safety, the dialog shows why and offers to force, and
# forcing removes the worktree and ends both sessions. The right side then
# shows the session used last, as when any session ends, with focus on the
# sidebar; the tree drops the worktree, and the status line says what wt did.
set -euo pipefail
# shellcheck source=test/lib/host.sh
. "$(dirname "$0")/lib/host.sh"
need_host_tmux
build_wtui

# api: a git project. Its worktree fix has two sessions, fix in its folder
# and review in a folder inside it, with an agent at work; and a file not
# committed. Launched in fix's folder, wtui starts on the worktree's most
# recent session: fix, made after review (or in the same second, and first
# by name). notes, a plain folder, is the session used last once fix and
# review end (activity counts seconds); not api, which wt would switch to.
git init -q -b main "$tmp/api"
git -C "$tmp/api" commit -q --allow-empty -m init
git -C "$tmp/api" worktree add -q -b fix "$tmp/api--fix"
new_session api "$tmp/api"
new_session review "$tmp/api--fix/src"
new_session fix "$tmp/api--fix"
sleep 1.1
new_session notes "$tmp/notes"
for s in api fix notes; do tmux rename-window -t "=$s:" shell; done
tmux rename-window -t =review: claude
tmux set -p -t "=review:0.0" @agent_state working
echo wip >"$tmp/api--fix/wip.txt"

launch t1 "$tmp/api--fix"
# in_tree <name>: the sidebar has a row named <name> (fix: the worktree's,
# by its branch, and its session's). Not just the text: once wt has removed
# api--fix, the status line names it, and branch fix.
in_tree() { sidebar_text t1 | grep -E "[▸▾] $1([[:space:]]|\$)" >/dev/null; }
wait_until "the sidebar to show fix's sessions" eval 'in_tree review && in_tree fix && in_tree notes && [ "$(showing t1)" = fix ]'

# shows <text>: the terminal shows <text>
shows() { case "$(screen t1)" in *"$1"*) ;; *) return 1 ;; esac; }

type_in t1 x
wait_until "x to ask about removing api--fix" shows "Remove worktree api--fix and end sessions fix and review?"
shows "The agent in review ends too:" && shows "  0 claude: working" || fail "the dialog doesn't name review's agent:
$(screen t1)"
focus_on t1 right || fail "with the dialog open, focus is on the $(focus t1)"

type_in t1 y
wait_until "wt to refuse, offering to force" eval 'shows "uncommitted changes" && shows "force?"'
shows "wip.txt" || fail "the refusal doesn't name the file that would be lost:
$(screen t1)"
[ -d "$tmp/api--fix" ] || fail "api--fix is gone before forcing"

type_in t1 y
wait_until "the right side to show notes, focus on the sidebar" eval '[ "$(showing t1)" = notes ] && focus_on t1 sidebar'
wait_until "the cursor to be on notes" eval '[[ "$(cursor_row t1)" == *notes ]]'
wait_until "the tree to drop api--fix and its sessions" eval '! in_tree review && ! in_tree fix'
squeezed() { tr -d ' \n'; }
wait_until "the status line to say what wt did" eval '[[ "$(sidebar_text t1 | squeezed)" == *"wt:removed"* ]]'
[ ! -e "$tmp/api--fix" ] || fail "api--fix's folder is still there"
for s in fix review; do
  ! tmux has-session -t "=$s" 2>/dev/null || fail "session $s still runs after the removal"
done
case "$(git -C "$tmp/api" worktree list --porcelain)" in *"$tmp/api--fix"*) fail "git still has api--fix registered" ;; esac

echo "ok: wtui-host-remove"
