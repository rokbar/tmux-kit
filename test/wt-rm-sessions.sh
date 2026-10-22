#!/usr/bin/env bash
# wt rm ends every session started in the worktree: in its folder or in one
# inside it, whatever they're called now. Sessions anywhere else stay, a
# folder whose name only starts with the worktree's included. Run in one of
# those sessions, wt ends that one last, so the others still end. In wtui's
# popup (TMUX_KIT_CLIENT names its client), wt leaves the client where it is:
# tmux moves wtui's client on by itself.
set -euo pipefail
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "$0")/lib/sandbox.sh"

# the names of every session, sorted, on one line
sessions() { { tmux list-sessions -F '#{session_name}' 2>/dev/null || true; } | sort | tr '\n' ' '; }

init_demo_repo
tmux new-session -d -s demo -c "$tmp/demo"
wt -d feat
tmux rename-session -t =feat renamed
tmux new-session -d -s second -c "$tmp/demo--feat"
mkdir "$tmp/demo--feat/web" "$tmp/demo--feat2"
tmux new-session -d -s web -c "$tmp/demo--feat/web"
tmux new-session -d -s feat2 -c "$tmp/demo--feat2"

wt rm feat || fail "wt rm feat failed"
grep -qxF "wt: removed $tmp/demo--feat; branch feat kept (git branch -d feat to delete)" "$tmp/log" ||
  fail "wt rm feat didn't say what it did: $(cat "$tmp/log")"
[ "$(sessions)" = "demo feat2 " ] || fail "after wt rm feat, the sessions are $(sessions), want demo feat2"

# from a window of one of them, listed first: the others end before it
wt -d inner
tmux rename-session -t =inner inner-a
mkdir "$tmp/demo--inner/src"
tmux new-session -d -s inner-b -c "$tmp/demo--inner/src"
tmux new-window -d -t =inner-a: "'$kit/bin/wt' rm inner >'$tmp/inner.log' 2>&1"
for ((i = 0; i < 100; i++)); do
  if [ "$(sessions)" = "demo feat2 " ]; then break; fi
  sleep 0.1
done
[ "$(sessions)" = "demo feat2 " ] || fail "wt rm run in inner-a left the sessions $(sessions), want demo feat2: $(cat "$tmp/inner.log")"
[ ! -e "$tmp/demo--inner" ] || fail "wt rm run in inner-a left $tmp/demo--inner"

# Inside tmux, wt moves its client off a session it ends; in wtui's popup, it
# doesn't. This shim says the client is on the session, and records switches.
REAL_TMUX="$(command -v tmux)"
mkdir "$tmp/bin"
cat >"$tmp/bin/tmux" <<EOF
#!/bin/sh
case "\$1" in
  display-message) echo moving; exit 0 ;;
  switch-client) echo "\$*" >>"$tmp/switched"; exit 0 ;;
esac
exec "$REAL_TMUX" "\$@"
EOF
chmod +x "$tmp/bin/tmux"
sock="$(tmux display-message -p '#{socket_path}')"
case "$sock" in "$tmp"/*) ;; *) fail "tmux server isn't the sandbox's: $sock" ;; esac
for client in "" /dev/ttys999; do
  rm -f "$tmp/switched"
  wt -d moving
  (export PATH="$tmp/bin:$PATH" TMUX="$sock,1,0" TMUX_KIT_CLIENT="$client" && wt rm moving) ||
    fail "wt rm moving failed (TMUX_KIT_CLIENT='$client')"
  [ "$(sessions)" = "demo feat2 " ] || fail "wt rm moving left the sessions $(sessions) (TMUX_KIT_CLIENT='$client')"
  if [ -z "$client" ]; then
    [ "$(cat "$tmp/switched" 2>/dev/null)" = "switch-client -t =demo" ] ||
      fail "wt rm didn't move its client to the main clone's session: $(cat "$tmp/switched" 2>/dev/null)"
  else
    [ ! -e "$tmp/switched" ] || fail "wt rm in wtui's popup moved the client: $(cat "$tmp/switched")"
  fi
done

echo "ok: wt-rm-sessions"
