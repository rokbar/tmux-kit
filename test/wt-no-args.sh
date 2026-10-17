#!/usr/bin/env bash
# Bare `wt` opens the picker inside a repo (fzf installed), else prints usage.
set -euo pipefail
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "$0")/lib/sandbox.sh"

# fzf is a TUI: this stub records that the picker opened, then quits like Esc.
# Asserted first on PATH, so the real fzf never runs and blocks the test.
mkdir "$tmp/bin"
printf '#!/bin/sh\ntouch "%s/fzf-ran"\nexit 130\n' "$tmp" >"$tmp/bin/fzf"
chmod +x "$tmp/bin/fzf"
export PATH="$tmp/bin:$PATH"
[ "$(command -v fzf)" = "$tmp/bin/fzf" ] || fail "stub fzf isn't first on PATH"
# consumes the marker, so each check sees only the wt runs since the last one
picker_opened() { [ -e "$tmp/fzf-ran" ] && rm "$tmp/fzf-ran"; }

init_demo_repo
wt || fail "bare wt failed in a repo"
picker_opened || fail "bare wt in a repo didn't open the picker"

for h in -h --help; do
  wt "$h" || fail "wt $h failed"
  grep -q '^usage:' "$tmp/log" || fail "wt $h didn't print usage"
  ! picker_opened || fail "wt $h opened the picker"
done
grep -q 'bare wt opens it' "$tmp/log" || fail "usage doesn't say bare wt opens the picker"

# fzf not installed: a PATH of just what bare wt needs (system dirs may hold fzf)
mkdir "$tmp/nofzf"
for c in bash git cat; do ln -s "$(command -v "$c")" "$tmp/nofzf/"; done
(
  export PATH="$tmp/nofzf"
  ! command -v fzf >/dev/null || fail "fzf still on the no-fzf PATH"
  wt || fail "bare wt failed in a repo without fzf"
)
grep -q '^usage:' "$tmp/log" || fail "bare wt without fzf didn't print usage"

# outside a repo; the ceiling stops git finding a repo above $tmp
mkdir "$tmp/norepo"
cd "$tmp/norepo"
export GIT_CEILING_DIRECTORIES="$tmp"
wt || fail "bare wt failed outside a repo"
grep -q '^usage:' "$tmp/log" || fail "bare wt outside a repo didn't print usage"
wt --help || fail "wt --help failed outside a repo"
grep -q '^usage:' "$tmp/log" || fail "wt --help outside a repo didn't print usage"
! picker_opened || fail "wt outside a repo opened the picker"

echo "ok: wt-no-args"
