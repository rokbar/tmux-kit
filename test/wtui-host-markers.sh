#!/usr/bin/env bash
# The dirty and dev-port markers, end to end: a worktree made dirty outside
# wtui gets ± after its name, and one with a dev URL its port, by the next
# 10 s refresh at the latest; r clears ± at once once it's clean again.
set -euo pipefail
# shellcheck source=test/lib/host.sh
. "$(dirname "$0")/lib/host.sh"
need_host_tmux
build_wtui

# api: the main clone, with a session (the most recent: activity counts
# seconds); worktree docs, without one; worktree fix, with one. .devurl is
# git-excluded, as wt has it.
git init -q -b main "$tmp/api"
git -C "$tmp/api" commit -q --allow-empty -m init
echo .devurl >>"$tmp/api/.git/info/exclude"
for b in docs fix; do git -C "$tmp/api" worktree add -q -b "$b" "$tmp/api--$b"; done
new_session fix "$tmp/api--fix"
sleep 1
new_session api "$tmp/api"
tmux rename-window -t =api: shell

launch t1 "$tmp"
tree() {
  printf ' wtui\n  ▾ api\n1   ▾ api\n        0 shell\n      docs%s\n2   ▸ fix%s' "$1" "$2"
}
wait_until "the sidebar to show api's worktrees, clean" sidebar_shows t1 "$(tree '' '')"

# from outside wtui: fix gets a change, docs a dev server's URL
echo wip >"$tmp/api--fix/wip.txt"
echo http://localhost:3001 >"$tmp/api--docs/.devurl"
wait_until "the next refresh to mark fix dirty and docs on :3001" sidebar_shows t1 "$(tree ' :3001' ' ±')"

# r, well before the next refresh is due: ± goes at once
rm "$tmp/api--fix/wip.txt"
type_in t1 r
for ((i = 0; i < 30; i++)); do
  if sidebar_shows t1 "$(tree ' :3001' '')"; then break; fi
  sleep 0.1
done
sidebar_shows t1 "$(tree ' :3001' '')" ||
  fail "3 s after r, the sidebar shows:
$(sidebar_text t1)"

echo "ok: wtui-host-markers"
