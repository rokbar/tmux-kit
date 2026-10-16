#!/usr/bin/env bash
# wt rm removes a worktree wt just created, in a repo that doesn't gitignore
# the .devurl wt writes into it.
set -euo pipefail

kit="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
# physical path, as git reports it (macOS: /var -> /private/var)
tmp="$(cd "$tmp" && pwd -P)"

# Isolation, before the trap so cleanup can only reach the private tmux server.
# HOME too: keeps out the user's tmux.conf (continuum would restore real
# sessions), git config (signing, global ignores that may list .devurl).
unset TMUX TMUX_PANE XDG_CONFIG_HOME GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE
export TMUX_TMPDIR="$tmp" WT_CONFIG=/dev/null HOME="$tmp/home" GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
cleanup() { tmux kill-server 2>/dev/null || true; rm -rf "$tmp"; }
trap cleanup EXIT

mkdir -p "$HOME"
# panes swallow keys, so wt's `npm install && npm run dev` never runs
echo "set -g default-command 'cat >/dev/null'" >"$HOME/.tmux.conf"

fail() { echo "FAIL: $*" >&2; exit 1; }
# quiet unless it fails
wt() { "$kit/bin/wt" "$@" >"$tmp/log" 2>&1 || { cat "$tmp/log" >&2; return 1; }; }

mkdir "$tmp/demo"
cd "$tmp/demo"
git init -q -b main
echo '{"scripts":{"dev":"x"}}' >package.json
echo '{}' >package-lock.json
git add .
git commit -qm init

wt -d feat
[ -f "$tmp/demo--feat/.devurl" ] || fail "wt didn't write .devurl"
[ -z "$(git -C "$tmp/demo--feat" status --porcelain)" ] || fail ".devurl shows in git status"
wt rm feat || fail "wt rm refused a worktree wt just created"

wt -d feat
[ "$(grep -cxF .devurl .git/info/exclude)" = 1 ] || fail "want .devurl once in info/exclude"
echo "ok: wt-rm"
