#!/usr/bin/env bash
# wt <path> opens the existing worktree or main clone at that path, as it is,
# including one with no branch (mid-rebase), and never creates anything.
set -euo pipefail
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "$0")/lib/sandbox.sh"

# the sessions started in <dir> are exactly <name>
session_in() {
  local got
  # no tmux server is no sessions, not an error
  got="$({ tmux list-sessions -F '#{session_path}'$'\t''#{session_name}' 2>/dev/null || true; } |
    awk -F'\t' -v d="$1" '$1 == d { print $2 }')"
  [ "$got" = "$2" ] || fail "want session '$2' started in $1, got '$got'"
}

init_demo_repo
# what wt sets up in a new worktree, so opening one by path can show it doesn't:
# a file to copy, one to link and (demo's lockfile and dev script) an install
# and a dev server
echo 'WT_LINK_FILES=(.scratch)' >"$tmp/config.sh"
export WT_CONFIG="$tmp/config.sh"
echo A >.env
mkdir .scratch

# the main clone, by .: a session named after its branch
wt -d . || fail "wt -d . failed in the main clone"
session_in "$tmp/demo" main

# a worktree made with plain git, by ../<folder>: named after its branch, and
# opened as it is
git worktree add -q "$tmp/demo--feat" -b feat
branches="$(git branch)"
wt -d ../demo--feat || fail "wt -d ../demo--feat failed"
session_in "$tmp/demo--feat" feat
for f in .env .scratch .devurl; do
  [ ! -e "$tmp/demo--feat/$f" ] && [ ! -L "$tmp/demo--feat/$f" ] || fail "wt <path> set up $f"
done
[ "$(tmux list-windows -t =feat | grep -c '')" = 1 ] || fail "wt <path> opened windows besides the first"
! grep -E '^wt: (copied|linked|running|dev server|new branch) ' "$tmp/log" >/dev/null ||
  fail "wt <path> reported setting something up: $(cat "$tmp/log")"
[ "$(git branch)" = "$branches" ] || fail "wt <path> changed the branches: $(git branch)"

# a worktree stopped on a conflict mid-rebase, so with no branch, by ./<folder>
# from the folder the worktrees are in (not a repo): named after its folder,
# cleaned up like any session name (. is _)
git worktree add -q "$tmp/demo--v1.2" -b v1.2
echo theirs >"$tmp/demo--v1.2/package.json"
git -C "$tmp/demo--v1.2" commit -qam "v1.2 side"
echo ours >package.json
git commit -qam "main side"
! git -C "$tmp/demo--v1.2" rebase main >/dev/null 2>&1 || fail "rebase didn't stop on its conflict"
(cd "$tmp" && wt -d ./demo--v1.2) || fail "wt -d ./demo--v1.2 failed for a worktree mid-rebase"
session_in "$tmp/demo--v1.2" demo--v1_2

# by absolute path: the session started there is reused, found by its folder
tmux rename-session -t =feat renamed
wt -d "$tmp/demo--feat" || fail "wt -d <absolute path> failed"
session_in "$tmp/demo--feat" renamed
grep -qxF "wt: $tmp/demo--feat (tmux session renamed)" "$tmp/log" || fail "renamed not reused: $(cat "$tmp/log")"

# -n names a new session
git worktree add -q "$tmp/demo--named" -b named
wt -d -n custom ../demo--named || fail "wt -d -n custom <path> failed"
session_in "$tmp/demo--named" custom

# without -d it attaches, or switches inside tmux. Both need a client, so this
# tmux shim records them instead.
REAL_TMUX="$(command -v tmux)"
mkdir "$tmp/bin"
printf '#!/bin/sh\ncase "$1" in attach | switch-client) echo "$*" >"%s/attached"; exit 0 ;; esac\nexec "%s" "$@"\n' \
  "$tmp" "$REAL_TMUX" >"$tmp/bin/tmux"
chmod +x "$tmp/bin/tmux"
sock="$(tmux display-message -p '#{socket_path}')"
case "$sock" in "$tmp"/*) ;; *) fail "tmux server isn't the sandbox's: $sock" ;; esac
(export PATH="$tmp/bin:$PATH" && wt ../demo--named) || fail "wt <path> failed to attach"
[ "$(cat "$tmp/attached")" = "attach -t =custom" ] || fail "wt <path> didn't attach to custom: $(cat "$tmp/attached")"
(export PATH="$tmp/bin:$PATH" TMUX="$sock,1,0" && wt ../demo--named) || fail "wt <path> failed to switch"
[ "$(cat "$tmp/attached")" = "switch-client -t =custom" ] || fail "wt <path> didn't switch to custom: $(cat "$tmp/attached")"

# wt <args> exits 1, saying "wt: <message>"
fails() {
  local message="$1" status=0
  shift
  "$kit/bin/wt" "$@" >"$tmp/log" 2>&1 || status=$?
  [ "$status" = 1 ] || fail "wt $* exited $status, want 1: $(cat "$tmp/log")"
  grep -qF "wt: $message" "$tmp/log" || fail "wt $* didn't say 'wt: $message': $(cat "$tmp/log")"
}

# a missing worktree (its folder deleted by hand): still registered, not opened
git worktree add -q "$tmp/demo--gone" -b gone
rm -rf "$tmp/demo--gone"
fails "missing worktree: $tmp/demo--gone" -d ../demo--gone
session_in "$tmp/demo--gone" ""
# outside the repo, git can't tell which repo a gone folder was in
(cd "$tmp" && fails "not a git dir: ./demo--gone" -d ./demo--gone)

# folders that aren't worktrees: a plain one, one that doesn't exist, and one
# inside a worktree
mkdir "$tmp/plain" "$tmp/demo--feat/src"
fails "not a git dir: ../plain" -d ../plain
fails "not a git dir: ./nope" -d ./nope
fails "not a worktree: ../demo--feat/src" -d ../demo--feat/src

# --from makes a new branch, so a path can't take it, before or after
branches="$(git branch)"
fails "--from" -d --from main ../demo--feat
fails "--from" -d ../demo--feat --from main
[ "$(git branch)" = "$branches" ] || fail "wt <path> --from changed the branches: $(git branch)"

echo "ok: wt-open-path"
