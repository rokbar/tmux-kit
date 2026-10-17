#!/usr/bin/env bash
# wt doctor shows which config wt loaded and the settings in effect, and checks
# the setup: deps, wt on PATH and, inside tmux, tmux-kit's prefix W / X keys.
# It fails only when a required dep (tmux, git, fzf) is missing.
set -euo pipefail
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "$0")/lib/sandbox.sh"

# the last wt run's output, without indentation
out() { sed 's/^ *//' "$tmp/log"; }
# a line of it that is <text>, or that starts with it
line() { out | grep -qxF -- "$1" || fail "want line '$1' in: $(cat "$tmp/log")"; }
line_starts() {
  out | awk -v p="$1" 'index($0, p) == 1 { f = 1 } END { exit !f }' ||
    fail "want a line starting '$1' in: $(cat "$tmp/log")"
}

init_demo_repo
mkdir "$tmp/norepo"
# outside a repo: the ceiling stops git finding one above $tmp
outside() { (cd "$tmp/norepo" && export GIT_CEILING_DIRECTORIES="$tmp" && "$@"); }
# wt doctor outside a repo, on a PATH of just $tmp/deps. Outside a repo and
# tmux, doctor itself needs only bash and git.
doctor_deps() { outside env PATH="$tmp/deps" "$kit/bin/wt" doctor >"$tmp/log" 2>&1; }

# config file there: its settings, defaults for the rest, the hook, the repo's root
cat >"$tmp/config.sh" <<EOF
WT_ROOT="$tmp/wts"
WT_REMOTE=upstream
WT_LINK_FILES=(.notes "my notes")
wt_before_link() { :; }
EOF
export WT_CONFIG="$tmp/config.sh"
# not a wt setting: Windows Terminal sets it
export WT_SESSION=terminal
wt doctor || fail "wt doctor failed in a repo"
line "config: $tmp/config.sh (loaded)"
line "WT_ROOT=$tmp/wts"
line "WT_REMOTE=upstream"
line 'WT_LINK_FILES=(.notes my\ notes)'
line "WT_SEP=--"
line "WT_COPY_FILES=(.env .env.local .env.development.local)"
line "wt_before_link: defined"
line "worktree root: $tmp/wts"
! out | grep -q '^WT_SESSION' || fail "env var WT_SESSION shown as a setting: $(cat "$tmp/log")"
# not in tmux (the sandbox unset TMUX): no server to ask, so none is asked
line "tmux keys: not checked (not in tmux)"
line_starts "alt keys: "

# no config file there: defaults; outside a repo, nothing repo-specific
export WT_CONFIG="$tmp/none/config.sh"
outside wt doctor || fail "wt doctor failed outside a repo"
line "config: $tmp/none/config.sh (not found; using defaults)"
line "WT_REMOTE=origin"
line "WT_ROOT=''"
line "WT_LINK_FILES=()"
line "wt_before_link: not defined"
! grep -q "worktree root" "$tmp/log" || fail "worktree root shown outside a repo: $(cat "$tmp/log")"

# deps and wt, found on a PATH of just these (system dirs hold some). Doctor
# only looks them up, so tmux, fzf and lsof can be stubs.
mkdir "$tmp/deps"
ln -s "$(command -v bash)" "$(command -v git)" "$tmp/deps/"
for c in tmux fzf lsof; do printf '#!/bin/sh\nexit 1\n' >"$tmp/deps/$c" && chmod +x "$tmp/deps/$c"; done
ln -s "$kit/bin/wt" "$tmp/deps/wt"
doctor_deps || fail "wt doctor failed with every dep: $(cat "$tmp/log")"
for c in tmux git fzf lsof; do line "$c: $tmp/deps/$c"; done
line "wt on PATH: $tmp/deps/wt"

# lsof is optional: only the dev server's port check needs it
rm "$tmp/deps/lsof" "$tmp/deps/wt"
doctor_deps || fail "wt doctor failed without lsof: $(cat "$tmp/log")"
line_starts "lsof: missing"
line_starts "wt on PATH: not found"

# a required dep missing: the full report, then a non-zero exit
rm "$tmp/deps/fzf"
doctor_deps && fail "wt doctor exited 0 without fzf: $(cat "$tmp/log")"
line "fzf: missing"
line "tmux: $tmp/deps/tmux"
line_starts "alt keys: "
line "wt: missing: fzf"

# inside tmux: the keys of the server $TMUX names. Only ever the sandbox's
# private server (TMUX_TMPDIR), never the user's.
tmux new-session -d -s keys
sock="$(tmux display-message -p '#{socket_path}')"
case "$sock" in "$tmp"/*) ;; *) fail "tmux server isn't the sandbox's: $sock" ;; esac
pid="$(tmux display-message -p '#{pid}')"
in_tmux() { (export TMUX="$sock,$pid,0" && "$@"); }
in_tmux wt doctor || fail "wt doctor failed in tmux"
line_starts "prefix W: not bound to wt pick"
line_starts "prefix X: not bound to wt close"
# tmux-kit's own bindings, as tmux.conf has them
grep -E '^bind [WX] ' "$kit/tmux.conf" >"$tmp/keys.conf"
[ "$(grep -c '' "$tmp/keys.conf")" = 2 ] || fail "tmux.conf has no bind W / bind X"
tmux source-file "$tmp/keys.conf"
in_tmux wt doctor || fail "wt doctor failed in tmux with the keys bound"
line "prefix W: wt pick"
line "prefix X: wt close"
# each key on its own, and only wt pick itself: W runs wt picker, X W's wt pick
tmux bind-key W display-popup -E "wt picker"
tmux bind-key X display-popup -E "wt pick"
in_tmux wt doctor || fail "wt doctor failed in tmux with near-miss keys"
line_starts "prefix W: not bound to wt pick"
line_starts "prefix X: not bound to wt close"

# $TMUX's server is gone (stale env): say so; checking the keys there would
# start a new server, with the user's tmux.conf
(export TMUX="$tmp/gone,1,0" && wt doctor) || fail "wt doctor failed with a stale \$TMUX"
line_starts "tmux keys: not checked (no tmux server at $tmp/gone)"

echo "ok: wt-doctor"
