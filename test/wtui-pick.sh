#!/usr/bin/env bash
# The whole stack: wtui, built from wtui/, on the real wt in a private tmux
# server, with a stub fzf. n runs the picker in the selected worktree's
# folder, with wtui paused. Cancelled, the picker hands back to wtui, which
# lists the repo again. A failure shows in wtui's status line as wt said it.
# A new branch starts from that worktree's branch with its local files, after
# wt has asked for the session's name on the terminal; wt then takes the tmux
# client to the new session, and wtui exits. Outside tmux, wt attaches to the
# session in wtui's terminal instead, and wtui is back once the user detaches.
set -euo pipefail
# shellcheck source=test/lib/wtui.sh
. "$(dirname "$0")/lib/wtui.sh"

build_wtui
init_demo_repo
feat="$tmp/demo--feat"
git worktree add -q "$feat" -b feat
git -C "$feat" commit -q --allow-empty -m "feat work"
echo "SECRET=feat" >"$feat/.env" # a local file, which wt copies from the invoking worktree

# fzf is a TUI. This stub, first on wtui's PATH, notes where the picker ran
# and what it offered, then does what $tmp/fzf-do says:
#   cancel: Esc, after a worktree was made meanwhile (elsewhere)
#   new <branch>: alt-enter with <branch> typed
mkdir "$tmp/stubs"
cat >"$tmp/stubs/fzf" <<EOF
#!/bin/sh
pwd -P >"$tmp/fzf-pwd"
printf '%s\n' "\$@" >"$tmp/fzf-args"
cat >/dev/null
read -r what branch <"$tmp/fzf-do"
case "\$what" in
  cancel) git -C "$tmp/demo" worktree add -q "$tmp/demo--meanwhile" -b meanwhile; exit 130 ;;
  new) echo "\$branch" ;;
esac
EOF
chmod +x "$tmp/stubs/fzf"

# row <folder>: wtui's row for <folder>, without the selection mark, its
# fields one space apart. awk reads to the end, so tmux never writes into a
# closed pipe (pipefail).
row() { screen | awk -v f="$1" '{ sub(/^> /, "") } $1 == f && !found { found = 1; $1 = $1; print }'; }
# row_starts <folder> <fields>: whether wtui's row for <folder> starts with
# <fields>, e.g. its branch, state and session; a dev URL's port may vary
row_starts() { case "$(row "$1") " in "$2 "*) ;; *) return 1 ;; esac; }

# inside tmux, in a pane a client shows, as in the prefix w popup
start_wtui "$tmp/demo" env PATH="$tmp/stubs:$PATH"
wait_for "demo--feat"
press j
wait_for "> demo--feat "

echo cancel >"$tmp/fzf-do"
press n
wait_until "wtui to list the worktree made meanwhile" row_starts demo--meanwhile "demo--meanwhile meanwhile clean no session"
wtui_exited && fail "wtui exited after the picker was cancelled"
wait_for "> demo--feat "
[ "$(cat "$tmp/fzf-pwd")" = "$feat" ] || fail "the picker ran in $(cat "$tmp/fzf-pwd"), want $feat"
grep -qxF "alt-enter: new branch from feat" "$tmp/fzf-args" ||
  fail "the picker doesn't offer a new branch from feat: $(cat "$tmp/fzf-args")"

# a failure: wt can't make a branch called "a b". It says why on stderr, then
# waits for a key so that stays on screen; wtui passed both to the terminal.
# Back in wtui, the status line is what wt said, without the prompt that was
# for the terminal.
said="$(cd "$feat" && "$kit/bin/wt" -d "a b" 2>&1 >/dev/null || true)"
[ -n "$said" ] || fail "wt says nothing about the branch name 'a b'"
echo "new a b" >"$tmp/fzf-do"
press n
wait_for "$said"
press Space
wait_until "wtui's status line to be wt's message" status_is "$said"
wtui_exited && fail "wtui exited after the picker failed"

echo "new fresh" >"$tmp/fzf-do"
press n
wait_for "session name [fresh]"
press Enter
wait_until "wtui to exit once the picker opened fresh" wtui_exited
[ "$(wtui_status)" = 0 ] || fail "wtui exited $(wtui_status) after the picker opened fresh:
$(screen)"
[ "$(client_session "$term_pane")" = fresh ] ||
  fail "the client shows '$(client_session "$term_pane")' after the picker, want fresh"
[ "$(session_in "$tmp/demo--fresh")" = fresh ] ||
  fail "want session fresh started in demo--fresh, got '$(session_in "$tmp/demo--fresh")'"
[ "$(git -C "$tmp/demo--fresh" rev-parse HEAD)" = "$(git -C "$feat" rev-parse HEAD)" ] ||
  fail "fresh doesn't start from feat"
[ "$(cat "$tmp/demo--fresh/.env" 2>/dev/null)" = "SECRET=feat" ] || fail "feat's .env wasn't copied to demo--fresh"

# outside tmux: wt attaches to the new session in wtui's terminal (its pane
# here), which wtui gave it; once the user detaches, wtui is back
start_wtui "$tmp/demo" env -u TMUX -u TMUX_PANE PATH="$tmp/stubs:$PATH"
wait_for "> demo (main clone)"
echo "new outside" >"$tmp/fzf-do"
press n
wait_for "session name [outside]"
press Enter
wait_until "a tmux client on session outside in wtui's terminal" client_on "$wtui_pane" outside
tmux detach-client -s =outside
wait_until "wtui to list demo--outside and its session" row_starts demo--outside "demo--outside outside clean outside"
wtui_exited && fail "wtui exited after the picker, outside tmux"

echo "ok: wtui-pick"
