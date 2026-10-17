#!/usr/bin/env bash
# wt ls lists the main clone, then each worktree: branch, clean / dirty /
# missing, session and, with --porcelain, dev URL. The human form stays as it
# was; --porcelain is wt's stable form for scripts. Both make one tmux query,
# however many worktrees.
set -euo pipefail
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "$0")/lib/sandbox.sh"

# exact match; on failure, a diff with tabs shown as \t
same() {
  [ "$1" = "$2" ] && return
  diff <(printf '%s\n' "$2" | sed -n l) <(printf '%s\n' "$1" | sed -n l) >&2 || true
  fail "$3 (diff: want, got)"
}
# a porcelain line: the arguments, tab-separated
row() { local IFS=$'\t'; printf '%s\n' "$*"; }

init_demo_repo
wt ls --porcelain || fail "wt ls --porcelain failed"
same "$(cat "$tmp/log")" "$(row "$tmp/demo" main clean "" "")" "wt ls --porcelain, main clone only"

# bugfix: clean, made by plain git at a path that sorts before the main
# clone's, with a session started there by hand (as `t` does)
git worktree add -q "$tmp/bugfix" -b bugfix
tmux new-session -d -s bugfix -c "$tmp/bugfix"

# feat: dirty; its session renamed, and a second session there (e.g. from `t`)
# that doesn't replace it (tmux lists sessions by name, so the renamed one comes
# first, as for session_for); a dev URL file, of which only the first line counts
wt -d -n api feat
tmux rename-session -t =api renamed
tmux new-session -d -s zz -c "$tmp/demo--feat"
printf 'http://localhost:4100\nhttp://localhost:9999\n' >"$tmp/demo--feat/.devurl"
echo wip >"$tmp/demo--feat/wip.txt"

# gone: missing (its folder deleted by hand), its session still running
wt -d gone
rm -rf "$tmp/demo--gone"

# rebase: stopped on a conflict mid-rebase, so detached (no branch) and dirty
git worktree add -q "$tmp/demo--rebase" -b rebase
echo theirs >"$tmp/demo--rebase/package.json"
git -C "$tmp/demo--rebase" commit -qam "rebase side"
echo ours >package.json
git commit -qam "main side"
! git -C "$tmp/demo--rebase" rebase main >/dev/null 2>&1 || fail "rebase didn't stop on its conflict"
tmux new-session -d -s rebasing -c "$tmp/demo--rebase"

wt ls || fail "wt ls failed"
same "$(cat "$tmp/log")" "$tmp/demo  main
$tmp/bugfix  bugfix  [tmux: bugfix]
$tmp/demo--feat  feat  dirty  [tmux: renamed]
$tmp/demo--gone  (missing; git worktree prune)
$tmp/demo--rebase  (detached)  dirty  [tmux: rebasing]" "human wt ls changed"

# path, branch, state, session, dev URL: the main clone, then each worktree in
# git's order
porcelain="$(
  row "$tmp/demo" main clean "" ""
  row "$tmp/bugfix" bugfix clean bugfix ""
  row "$tmp/demo--feat" feat dirty renamed http://localhost:4100
  row "$tmp/demo--gone" "" missing gone ""
  row "$tmp/demo--rebase" "" dirty rebasing ""
)"
wt ls --porcelain || fail "wt ls --porcelain failed"
same "$(cat "$tmp/log")" "$porcelain" "wt ls --porcelain"
# the same from a worktree: still the main clone first
(cd "$tmp/demo--feat" && wt ls --porcelain) || fail "wt ls --porcelain failed in a worktree"
same "$(cat "$tmp/log")" "$porcelain" "wt ls --porcelain run in a worktree"

# a format this wt doesn't have is an error, so a script asking for one can tell
"$kit/bin/wt" ls --porcelain=v2 >"$tmp/log" 2>&1 && fail "wt ls --porcelain=v2 succeeded: $(cat "$tmp/log")"
grep -q '^usage:' "$tmp/log" || fail "wt ls --porcelain=v2 didn't print usage: $(cat "$tmp/log")"

# one tmux query per call, however many worktrees: a shim first on PATH logs
# each tmux run, then hands it to the real tmux
mkdir "$tmp/shim"
printf '#!/bin/sh\necho "$*" >>"%s/tmux-runs"\nexec "%s" "$@"\n' "$tmp" "$(command -v tmux)" >"$tmp/shim/tmux"
chmod +x "$tmp/shim/tmux"
one_tmux_query() {
  : >"$tmp/tmux-runs"
  (export PATH="$tmp/shim:$PATH" && wt "$@") || fail "wt $* failed with the tmux shim"
  [ "$(grep -c '' "$tmp/tmux-runs")" = 1 ] ||
    fail "wt $* ran tmux $(grep -c '' "$tmp/tmux-runs") times, want 1:"$'\n'"$(cat "$tmp/tmux-runs")"
}
one_tmux_query ls
one_tmux_query ls --porcelain

echo "ok: wt-ls"
