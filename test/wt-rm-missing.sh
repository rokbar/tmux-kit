#!/usr/bin/env bash
# wt rm on a missing worktree (its folder deleted by hand), by path or branch,
# with or without -f: unregisters just that worktree and ends its session,
# found by its start folder. With the folder gone there's nothing to check, so
# no safety checks run.
set -euo pipefail
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "$0")/lib/sandbox.sh"

# the names of the sessions started in <dir>
sessions_in() {
  # no tmux server is no sessions, not an error
  { tmux list-sessions -F '#{session_path}'$'\t''#{session_name}' 2>/dev/null || true; } |
    awk -F'\t' -v d="$1" '$1 == d { print $2 }'
}
# whether git still has a worktree registered at <dir>
registered() {
  local list
  list="$(git worktree list --porcelain)"
  grep -qxF "worktree $1" <<<"$list"
}

init_demo_repo
# gone: with a commit on no other branch, which a present worktree would refuse
# without -f, and its session renamed, so only its start folder finds it
wt -d gone
echo work >"$tmp/demo--gone/work.txt"
git -C "$tmp/demo--gone" add work.txt
git -C "$tmp/demo--gone" commit -qm work
tmux rename-session -t =gone renamed
# lost: missing too, and removing gone mustn't touch it
wt -d feat/lost
rm -rf "$tmp/demo--gone" "$tmp/demo--feat-lost"

# by path, without -f
wt rm ../demo--gone || fail "wt rm <path> failed on a missing worktree"
grep -qxF "wt: unregistered missing worktree $tmp/demo--gone; branch gone kept (git branch -d gone to delete)" "$tmp/log" ||
  fail "wt rm <path> didn't say what it did: $(cat "$tmp/log")"
! registered "$tmp/demo--gone" || fail "wt rm <path> left the missing worktree registered"
[ -z "$(sessions_in "$tmp/demo--gone")" ] || fail "wt rm <path> left its session: $(sessions_in "$tmp/demo--gone")"
git show-ref --verify --quiet refs/heads/gone || fail "wt rm <path> deleted the branch"
registered "$tmp/demo--feat-lost" || fail "wt rm <path> unregistered another missing worktree too"
[ "$(sessions_in "$tmp/demo--feat-lost")" = feat/lost ] || fail "wt rm <path> ended another worktree's session"

# by branch, with -f
wt rm -f feat/lost || fail "wt rm -f <branch> failed on a missing worktree"
grep -qxF "wt: unregistered missing worktree $tmp/demo--feat-lost; branch feat/lost kept (git branch -d feat/lost to delete)" "$tmp/log" ||
  fail "wt rm -f <branch> didn't say what it did: $(cat "$tmp/log")"
! registered "$tmp/demo--feat-lost" || fail "wt rm -f <branch> left the missing worktree registered"
[ -z "$(sessions_in "$tmp/demo--feat-lost")" ] || fail "wt rm -f <branch> left its session"

# one that never had a session (made by plain git)
git worktree add -q "$tmp/demo--bare" -b bare
rm -rf "$tmp/demo--bare"
wt rm ../demo--bare || fail "wt rm <path> failed on a missing worktree with no session"
! registered "$tmp/demo--bare" || fail "wt rm <path> left a missing worktree with no session registered"

# a locked one still refuses, -f or not, with exit 1 (not a safety refusal:
# forcing can't get past a lock), and keeps its registration and session
git worktree add -q "$tmp/demo--held" -b held
git worktree lock "$tmp/demo--held"
tmux new-session -d -s held -c "$tmp/demo--held"
rm -rf "$tmp/demo--held"
refuses_locked() {
  local status=0
  "$kit/bin/wt" rm "$@" >"$tmp/log" 2>&1 || status=$?
  [ "$status" = 1 ] || fail "wt rm $* exited $status for a locked missing worktree, want 1: $(cat "$tmp/log")"
  grep -qxF "wt: $tmp/demo--held is locked (git worktree unlock it first)" "$tmp/log" ||
    fail "wt rm $* didn't say it's locked: $(cat "$tmp/log")"
  registered "$tmp/demo--held" || fail "wt rm $* unregistered a locked worktree"
  [ "$(sessions_in "$tmp/demo--held")" = held ] || fail "wt rm $* ended a locked worktree's session"
}
refuses_locked ../demo--held
refuses_locked -f held

echo "ok: wt-rm-missing"
