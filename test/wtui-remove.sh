#!/usr/bin/env bash
# The whole stack: wtui, built from wtui/, on the real wt in a private tmux
# server. x on a worktree with uncommitted changes: wtui asks first, wt refuses
# for safety, wtui shows wt's reasons and offers to force, and forcing removes
# the worktree and ends its session. wtui runs in that worktree's folder, as
# the prefix w popup does when opened from its session, so it has to go on
# listing the repo once that folder is gone. wt's wording is free to change
# (wt's own tests pin it), so this only looks for what it names. On the way,
# the detail pane shows what git says about the selected worktree.
set -euo pipefail
# shellcheck source=test/lib/wtui.sh
. "$(dirname "$0")/lib/wtui.sh"

build_wtui
init_demo_repo
wt -d feat
feat="$tmp/demo--feat"
[ "$(session_in "$feat")" = feat ] || fail "want session feat started in demo--feat, got '$(session_in "$feat")'"
echo wip >"$feat/wip.txt"

# says <text>: whether wtui shows <text>, which may be wrapped: wt's messages
# name folders by their full path, which can be wider than the pane
says() { case "$(screen | tr -d ' \n')" in *"$(printf %s "$1" | tr -d ' \n')"*) ;; *) return 1 ;; esac; }

start_wtui "$feat"
wait_for "> demo (main clone)"
press j
wait_for "> demo--feat "
# the detail pane shows what git says about it
wait_for "?? wip.txt"
press x
wait_for "remove demo--feat and end session feat?"
press y
wait_for "force?"
# wt's reasons name the worktree and what would be lost
for want in "$feat" "wip.txt" "y: force"; do
  says "$want" || fail "the force prompt doesn't show '$want':
$(screen)"
done
[ -d "$feat" ] || fail "demo--feat is gone before forcing"

press y
# back at the list, without demo--feat's row, under wt's message about the
# folder it removed (the force prompt named it too, so check all three at once)
removed() { shows "x: remove" && ! shows "demo--feat " && says "$feat"; }
wait_until "the list after the forced removal" removed
[ ! -d "$feat" ] || fail "demo--feat's folder is still there"
registered="$(git -C "$tmp/demo" worktree list --porcelain)"
case "$registered" in *"worktree $feat"*) fail "git still has demo--feat registered:
$registered" ;; esac
[ -z "$(session_in "$feat")" ] || fail "session feat still runs after the removal"
shows "> demo (main clone)" || fail "the main clone isn't listed and selected:
$(screen)"
! wtui_exited || fail "wtui exited after the removal:
$(screen)"

echo "ok: wtui-remove"
