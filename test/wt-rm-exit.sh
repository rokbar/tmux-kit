#!/usr/bin/env bash
# wt rm's exit status tells a safety refusal (something would be lost; -f
# overrides it) from other failures, so a script (wtui) can offer -f for the
# one and not the others: 3 for each kind of safety refusal, 1 for the rest.
set -euo pipefail
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "$0")/lib/sandbox.sh"

# wt rm <args> exits <status>, printing a line that is exactly <line>: on
# stdout if it succeeds, else on stderr, where a script reads the reason. That
# stream ends up in $tmp/log.
rm_exits() {
  local want="$1" line="$2" status=0 stream=err
  shift 2
  "$kit/bin/wt" rm "$@" >"$tmp/out" 2>"$tmp/err" || status=$?
  [ "$status" = "$want" ] || fail "wt rm $* exited $status, want $want: $(cat "$tmp/out" "$tmp/err")"
  if [ "$want" = 0 ]; then stream=out; fi
  cp "$tmp/$stream" "$tmp/log"
  grep -qxF -- "$line" "$tmp/log" || fail "wt rm $* didn't print '$line' on std$stream: $(cat "$tmp/out" "$tmp/err")"
}

# wt rm <branch> is a safety refusal for <reason> alone, with its usual message,
# and keeps the worktree; then wt rm -f removes it anyway
refuses_until_forced() {
  local branch="$1" reason="$2" dir="$tmp/demo--$1" other
  rm_exits 3 "wt: not removing $dir:" "$branch"
  grep -qF -- "$reason" "$tmp/log" || fail "wt rm $branch didn't give '$reason': $(cat "$tmp/log")"
  for other in "uncommitted changes:" "commits on no other branch or remote:" "not a link to the main clone"; do
    [ "$other" = "$reason" ] || ! grep -qF -- "$other" "$tmp/log" ||
      fail "wt rm $branch gave '$other' as well: $(cat "$tmp/log")"
  done
  grep -qxF "(wt rm -f to force)" "$tmp/log" || fail "wt rm $branch didn't offer -f: $(cat "$tmp/log")"
  [ -d "$dir" ] || fail "wt rm $branch refused but removed $dir"
  rm_exits 0 "wt: removed $dir; branch $branch kept (git branch -d $branch to delete)" -f "$branch"
  [ ! -e "$dir" ] || fail "wt rm -f $branch left $dir"
}

init_demo_repo
# .shared: linked into new worktrees, so a real file in its place is a safety
# refusal. Gitignored (in the info/exclude all worktrees share), so it isn't
# also an uncommitted change.
echo 'WT_LINK_FILES=(.shared)' >"$tmp/config.sh"
export WT_CONFIG="$tmp/config.sh"
echo .shared >>.git/info/exclude
echo main >.shared

# uncommitted changes
git worktree add -q "$tmp/demo--dirty" -b dirty
echo wip >"$tmp/demo--dirty/wip.txt"
refuses_until_forced dirty "uncommitted changes:"

# commits on no other branch or remote
git worktree add -q "$tmp/demo--ahead" -b ahead
git -C "$tmp/demo--ahead" commit -q --allow-empty -m ahead
refuses_until_forced ahead "commits on no other branch or remote:"

# a shared file that isn't linked: a real file, different from the main clone's
git worktree add -q "$tmp/demo--unlinked" -b unlinked
echo changed >"$tmp/demo--unlinked/.shared"
refuses_until_forced unlinked "not a link to the main clone"

# every other failure is 1, -f or not: forcing can't fix these
# not a worktree: a plain folder, a branch with no worktree
mkdir "$tmp/plain"
rm_exits 1 "wt: not a git dir: ../plain" ../plain
rm_exits 1 "wt: not a worktree: $tmp/demo--nope" -f nope
# the main clone, by path and by branch
rm_exits 1 "wt: refusing to remove the main clone" .
rm_exits 1 "wt: refusing to remove the main clone" -f main
# locked, even when it would also be a safety refusal
git worktree add -q "$tmp/demo--held" -b held
git worktree lock "$tmp/demo--held"
rm_exits 1 "wt: $tmp/demo--held is locked (git worktree unlock it first)" -f held
echo wip >"$tmp/demo--held/wip.txt"
rm_exits 1 "wt: $tmp/demo--held is locked (git worktree unlock it first)" held
[ -d "$tmp/demo--held" ] || fail "wt rm removed a locked worktree"
# usage: no worktree, an unknown option
rm_exits 1 "usage:"
rm_exits 1 "usage:" -x held
# a step that crashes (here git, in a worktree whose .git points nowhere) is
# 1 too, not the crashing program's own status
git worktree add -q "$tmp/demo--broken" -b broken
echo "gitdir: $tmp/nowhere" >"$tmp/demo--broken/.git"
status=0
"$kit/bin/wt" rm broken >"$tmp/log" 2>&1 || status=$?
[ "$status" = 1 ] || fail "wt rm exited $status when git failed, want 1: $(cat "$tmp/log")"
[ -d "$tmp/demo--broken" ] || fail "wt rm removed $tmp/demo--broken after git failed"

# wt close (prefix X) passes wt rm's failure through as before, a safety
# refusal included: wt rm's message, a pause for a key, then exit 1
git worktree add -q "$tmp/demo--closing" -b closing
echo wip >"$tmp/demo--closing/wip.txt"
status=0
(cd "$tmp/demo--closing" && printf 'y\nk' | "$kit/bin/wt" close) >"$tmp/log" 2>&1 || status=$?
[ "$status" = 1 ] || fail "wt close exited $status after a safety refusal, want 1: $(cat "$tmp/log")"
grep -qxF "wt: not removing $tmp/demo--closing:" "$tmp/log" || fail "wt close didn't show wt rm's refusal: $(cat "$tmp/log")"
[ -d "$tmp/demo--closing" ] || fail "wt close removed a worktree wt rm refused"

echo "ok: wt-rm-exit"
