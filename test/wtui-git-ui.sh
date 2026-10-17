#!/usr/bin/env bash
# The whole stack: wtui, built from wtui/, on the real wt in a private tmux
# server, with a stub git UI. g runs the git UI in the selected worktree's
# folder, with wtui paused and the git UI on the terminal. Once it exits,
# wtui is back and lists the repo again, so what was done in the git UI (a
# commit) shows in the detail pane. With no git UI configured, wtui's status
# line is what wt said.
set -euo pipefail
# shellcheck source=test/lib/wtui.sh
. "$(dirname "$0")/lib/wtui.sh"

build_wtui
init_demo_repo
feat="$tmp/demo--feat"
git worktree add -q "$feat" -b feat
echo wip >"$feat/wip.txt"

# a git UI, first on wtui's PATH: it notes where it runs and whether its
# stdin and stdout are a terminal, waits for a line (a real one waits for
# keys), then commits everything there, as the user would in it
mkdir "$tmp/stubs"
cat >"$tmp/stubs/stub-ui" <<EOF
#!/bin/sh
pwd -P >"$tmp/ui-pwd"
tty=no
if [ -t 0 ] && [ -t 1 ]; then tty=yes; fi
echo "\$tty" >"$tmp/ui-tty"
echo "stub git UI: enter commits"
read -r _
git add -A && git commit -qm "commit from the git UI"
EOF
chmod +x "$tmp/stubs/stub-ui"
echo "WT_GIT_UI=stub-ui" >"$tmp/config.sh"

# inside tmux, in a pane a client shows, as in the prefix w popup
start_wtui "$tmp/demo" env PATH="$tmp/stubs:$PATH" WT_CONFIG="$tmp/config.sh"
wait_for "demo--feat"
press j
wait_for "> demo--feat "
wait_for "?? wip.txt"
press g
wait_for "stub git UI: enter commits"
[ "$(cat "$tmp/ui-pwd")" = "$feat" ] || fail "the git UI ran in $(cat "$tmp/ui-pwd"), want $feat"
[ "$(cat "$tmp/ui-tty")" = yes ] || fail "the git UI's stdin and stdout aren't the terminal"
press Enter
wait_for "commit from the git UI"
wait_for "no changes"
wait_for "> demo--feat "
wtui_exited && fail "wtui exited after the git UI"

# no git UI configured: wt's explanation, in the status line
said="$("$kit/bin/wt" git-ui "$tmp/demo" 2>&1 >/dev/null || true)"
case "$said" in *"no git UI configured"*) ;; *) fail "wt git-ui doesn't say no git UI is configured: $said" ;; esac
start_wtui "$tmp/demo"
wait_for "> demo (main clone)"
press g
wait_until "wtui's status line to be wt's message" status_is "$said"
wtui_exited && fail "wtui exited after wt couldn't run the git UI"

echo "ok: wtui-git-ui"
