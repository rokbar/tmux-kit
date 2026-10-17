#!/usr/bin/env bash
# A new worktree never inherits the invoking worktree's .devurl: wt writes it
# only for the worktree that owns the port.
set -euo pipefail
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "$0")/lib/sandbox.sh"

init_demo_repo
wt -d src
[ -f "$tmp/demo--src/.devurl" ] || fail "wt didn't write .devurl"
cd "$tmp/demo--src"

wt -d --no-dev feat
[ ! -e "$tmp/demo--feat/.devurl" ] || fail "--no-dev worktree got src's .devurl"

wt -d dev
[ -f "$tmp/demo--dev/.devurl" ] || fail "dev worktree has no .devurl"
url="$(cat "$tmp/demo--dev/.devurl")"
[ "$url" != "$(cat "$tmp/demo--src/.devurl")" ] || fail "dev worktree got src's port"
grep -qF "dev server on $url " "$tmp/log" || fail ".devurl $url isn't the dev server's port"
echo "ok: wt-devurl"
