#!/usr/bin/env bash
# The whole stack: wtui, built from wtui/, on the real wt in a private tmux
# server. enter on a worktree with no session has wt start one there, then wtui
# exits and takes the tmux client to it; run outside tmux, wtui attaches to it
# once it has exited.
set -euo pipefail
# shellcheck source=test/lib/wtui.sh
. "$(dirname "$0")/lib/wtui.sh"

build_wtui
init_demo_repo
git worktree add -q "$tmp/demo--feat" -b feat

# inside tmux, in a pane a client shows, as in the prefix w popup
start_wtui "$tmp/demo"
wait_for "demo--feat"
[ -z "$(session_in "$tmp/demo--feat")" ] || fail "demo--feat has a session before enter"
press j
wait_for "> demo--feat "
press Enter
wait_until "wtui to exit after enter" wtui_exited
[ "$(wtui_status)" = 0 ] || fail "wtui exited $(wtui_status) after enter:
$(screen)"
[ "$(session_in "$tmp/demo--feat")" = feat ] ||
  fail "want session feat started in demo--feat, got '$(session_in "$tmp/demo--feat")'"
[ "$(client_session "$term_pane")" = feat ] ||
  fail "the client shows '$(client_session "$term_pane")' after enter, want feat"

# outside tmux: once wtui has exited, its terminal (here, its pane) is a tmux
# client attached to the session. The main clone has no session yet either.
start_wtui "$tmp/demo" env -u TMUX -u TMUX_PANE
wait_for "> demo (main clone)"
press Enter
wait_until "a tmux client on session main in wtui's terminal" client_on "$wtui_pane" main
[ "$(session_in "$tmp/demo")" = main ] ||
  fail "want session main started in the main clone, got '$(session_in "$tmp/demo")'"

echo "ok: wtui-open"
