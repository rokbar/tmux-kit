#!/usr/bin/env bash
# Setting up a new worktree says what wt did: one line per copy, link and
# install it actually did, nothing for what it skipped.
set -euo pipefail
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "$0")/lib/sandbox.sh"

# the last wt run's lines that report a setup step
steps() { grep -E '^wt: (copied|linked|running) ' "$tmp/log" || true; }
want_line() { grep -qxF "$1" "$tmp/log" || fail "want '$1' in: $(cat "$tmp/log")"; }

init_demo_repo
# not 0 or 1 (tmux's and tmux-kit's), so the install line can't guess the index
echo "set -g base-index 5" >>"$HOME/.tmux.conf"
# a real config (still inside $tmp): WT_LINK_FILES has no default
echo 'WT_LINK_FILES=(.scratch .notes)' >"$tmp/config.sh"
export WT_CONFIG="$tmp/config.sh"

# nothing to copy or link yet, and no lockfile on this branch: no install
git switch -q -c plain
git rm -q package-lock.json
git commit -qm "no lockfile"
git switch -q main
wt -d plain
[ -z "$(steps)" ] || fail "reported steps it didn't take: $(steps)"
want_line "wt: $tmp/demo--plain (tmux session plain)"

# .env.development.local and .notes don't exist, so they aren't named
echo A >.env
echo B >.env.local
mkdir .scratch
wt -d --no-dev feat
want_line "wt: copied .env, .env.local"
want_line "wt: linked .scratch"
want_line "wt: running npm install (window 5)"

# with a dev server, install runs first in its window
wt -d dev
want_line "wt: running npm install (window dev)"
grep -qE '^wt: dev server on http://localhost:[0-9]+ \(window dev\)$' "$tmp/log" ||
  fail "no dev server line: $(cat "$tmp/log")"

# reopening a worktree sets nothing up
wt -d feat
[ -z "$(steps)" ] || fail "reopening feat reported setup steps: $(steps)"

echo "ok: wt-narrate"
