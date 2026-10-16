# shellcheck shell=bash
# Sourced by test/*.sh (after set -euo pipefail): private temp env. Sets $kit,
# $tmp, HOME=$tmp/home; owns the EXIT trap (kills the private tmux server,
# deletes $tmp). Defines fail, wt (quiet bin/wt), init_demo_repo.
kit="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmp="$(mktemp -d)"
# physical path, as git reports it (macOS: /var -> /private/var)
tmp="$(cd "$tmp" && pwd -P)"

# Isolation, before the trap so cleanup can only reach the private tmux server.
# HOME too: keeps out the user's tmux.conf (continuum would restore real
# sessions), git config (signing, global ignores that may list .devurl).
# FZF_DEFAULT_OPTS*: the user's fzf options would change what a real fzf matches.
unset TMUX TMUX_PANE XDG_CONFIG_HOME GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE FZF_DEFAULT_OPTS FZF_DEFAULT_OPTS_FILE
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

# $tmp/demo: one commit, npm + a dev script, so new worktrees get a dev server;
# cd's into it
init_demo_repo() {
  mkdir "$tmp/demo"
  cd "$tmp/demo"
  git init -q -b main
  echo '{"scripts":{"dev":"x"}}' >package.json
  echo '{}' >package-lock.json
  git add .
  git commit -qm init
}
