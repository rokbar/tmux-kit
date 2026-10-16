#!/usr/bin/env bash
# wt rm removes a worktree wt just created, in a repo that doesn't gitignore
# the .devurl wt writes into it.
set -euo pipefail
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "$0")/lib/sandbox.sh"

init_demo_repo
wt -d feat
[ -f "$tmp/demo--feat/.devurl" ] || fail "wt didn't write .devurl"
[ -z "$(git -C "$tmp/demo--feat" status --porcelain)" ] || fail ".devurl shows in git status"
wt rm feat || fail "wt rm refused a worktree wt just created"

wt -d feat
[ "$(grep -cxF .devurl .git/info/exclude)" = 1 ] || fail "want .devurl once in info/exclude"
echo "ok: wt-rm"
