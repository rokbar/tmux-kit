#!/usr/bin/env bash
# wtui refuses to launch inside tmux, pointing to tmux's own ways of
# switching sessions, and on a tmux older than it needs, naming the version
# it needs. Either way it exits non-zero, having started nothing.
set -euo pipefail
# shellcheck source=test/lib/wtui-build.sh
. "$(dirname "$0")/lib/wtui-build.sh"
build_wtui
wtui="$HOME/.local/bin/wtui"
export TMPDIR="$tmp/hosts" # where a host's folder would go
mkdir "$TMPDIR"

# refused <why> <command>...: the command, a wtui launch, fails saying <why>
# (a glob), and leaves no host folder behind
refused() {
  local why="$1" out
  shift
  if out="$("$@" 2>&1)"; then fail "wtui launched despite: $why"; fi
  # shellcheck disable=SC2254 # why is a glob
  case "$out" in $why) ;; *) fail "wtui didn't say '$why': $out" ;; esac
  [ -z "$(ls -A "$TMPDIR")" ] || fail "a refused launch left $(ls -A "$TMPDIR")"
}

refused '*prefix s*prefix W*' env TMUX=/tmp/tmux-0/default,1,0 "$wtui"

# a tmux that's 3.5a, with no server running
mkdir "$tmp/stubs"
cat >"$tmp/stubs/tmux" <<'EOF'
#!/bin/sh
case "$1" in
  -V) echo "tmux 3.5a" ;;
  *) echo "no server running on /tmp/tmux-0/default" >&2; exit 1 ;;
esac
EOF
chmod +x "$tmp/stubs/tmux"
refused '*needs tmux 3.6 or newer*3.5a*' env PATH="$tmp/stubs:$PATH" "$wtui"

# The normal server's version counts: one started before tmux was upgraded
cat >"$tmp/stubs/tmux" <<'EOF'
#!/bin/sh
case "$1" in
  -V) echo "tmux 3.8" ;;
  display-message) echo "3.4" ;;
  *) echo "stub tmux: $*" >&2; exit 1 ;;
esac
EOF
refused '*needs tmux 3.6 or newer*3.4*' env PATH="$tmp/stubs:$PATH" "$wtui"

echo "ok: wtui-host-refuse"
