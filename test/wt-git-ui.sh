#!/usr/bin/env bash
# wt git-ui [<path>] runs the configured git UI (WT_GIT_UI) in the foreground in
# a worktree: the one wt runs in, or the worktree or main clone at <path>.
set -euo pipefail
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "$0")/lib/sandbox.sh"

# a git UI: records the folder it ran in and its arguments, copies a line of
# stdin to stdout and stderr, and exits $STUB_UI_EXIT (default 0)
mkdir "$tmp/bin"
cat >"$tmp/bin/stub-ui" <<EOF
#!/bin/sh
pwd -P >"$tmp/ran"
printf '%s\n' "\$@" >"$tmp/args"
IFS= read -r line || line=""
echo "out: \$line"
echo "err: \$line" >&2
exit "\${STUB_UI_EXIT:-0}"
EOF
chmod +x "$tmp/bin/stub-ui"
export PATH="$tmp/bin:$PATH"
[ "$(command -v stub-ui)" = "$tmp/bin/stub-ui" ] || fail "stub git UI isn't first on PATH"

# the config's git UI is <command>
export WT_CONFIG="$tmp/config.sh"
git_ui_is() { printf 'WT_GIT_UI=%q\n' "$1" >"$WT_CONFIG"; }

# wt git-ui <args>, from the current folder, with "typed" on stdin: stdout in
# $tmp/out, stderr in $tmp/err, exit status in $status
git_ui() {
  status=0
  "$kit/bin/wt" git-ui "$@" <<<typed >"$tmp/out" 2>"$tmp/err" || status=$?
}
# the last wt git-ui ran its git UI in <dir> (consumes the record), and exited
# <status> (default 0)
ran_in() {
  [ "$status" = "${2:-0}" ] || fail "wt git-ui exited $status, want ${2:-0}: $(cat "$tmp/err")"
  [ -e "$tmp/ran" ] || fail "the git UI didn't run: $(cat "$tmp/err")"
  [ "$(cat "$tmp/ran")" = "$1" ] || fail "the git UI ran in $(cat "$tmp/ran"), want $1"
  rm "$tmp/ran"
}
# wt git-ui <args> exits 1 without running the git UI, saying <message> on
# stderr, ended by a newline (wtui shows stderr up to its last newline)
fails() {
  local message="$1"
  shift
  git_ui "$@"
  [ "$status" = 1 ] || fail "wt git-ui $* exited $status, want 1: $(cat "$tmp/err")"
  [ ! -e "$tmp/ran" ] || fail "wt git-ui $* ran the git UI in $(cat "$tmp/ran")"
  grep -qF -- "$message" "$tmp/err" || fail "wt git-ui $* didn't say '$message' on stderr: $(cat "$tmp/err")"
  [ -z "$(tail -c1 "$tmp/err")" ] || fail "wt git-ui $*'s stderr doesn't end with a newline: $(cat "$tmp/err")"
  [ ! -s "$tmp/out" ] || fail "wt git-ui $* wrote to stdout: $(cat "$tmp/out")"
}

init_demo_repo
git_ui_is stub-ui

# with no <path>, the invoking worktree: the main clone, and a worktree run
# from a folder inside it
git_ui
ran_in "$tmp/demo"
git worktree add -q "$tmp/demo--feat" -b feat
mkdir "$tmp/demo--feat/src"
(cd "$tmp/demo--feat/src" && git_ui && ran_in "$tmp/demo--feat")

# a <path>: a worktree, relative or absolute and from outside the repo, and
# the main clone from a worktree
git_ui ../demo--feat
ran_in "$tmp/demo--feat"
mkdir "$tmp/norepo"
(cd "$tmp/norepo" && git_ui "$tmp/demo--feat" && ran_in "$tmp/demo--feat")
(cd "$tmp/demo--feat" && git_ui ../demo && ran_in "$tmp/demo")

# run as a shell command (quotes group words), in the foreground with wt's
# stdin, stdout and stderr, so a real git UI gets the terminal; wt adds nothing
# to either output, and exits with the git UI's status
git_ui_is "stub-ui 'two words' three"
STUB_UI_EXIT=5 git_ui ../demo--feat
ran_in "$tmp/demo--feat" 5
[ "$(cat "$tmp/args")" = "two words"$'\n'"three" ] || fail "WT_GIT_UI wasn't run as a shell command; args: $(cat "$tmp/args")"
[ "$(cat "$tmp/out")" = "out: typed" ] || fail "want only the git UI's stdout, with wt's stdin: $(cat "$tmp/out")"
[ "$(cat "$tmp/err")" = "err: typed" ] || fail "want only the git UI's stderr: $(cat "$tmp/err")"
git_ui_is stub-ui

# git-ui is a command name, like ls: a branch called git-ui that has a worktree
# opens by its path, and wt git-ui there runs the git UI
git worktree add -q "$tmp/demo--git-ui" -b git-ui
wt -d ../demo--git-ui || fail "wt -d ../demo--git-ui failed"
grep -qxF "wt: $tmp/demo--git-ui (tmux session git-ui)" "$tmp/log" ||
  fail "the git-ui branch's worktree didn't open as session git-ui: $(cat "$tmp/log")"
(cd "$tmp/demo--git-ui" && git_ui && ran_in "$tmp/demo--git-ui")

# anything else is an error: a missing worktree (its folder deleted by hand), a
# plain folder, a path that doesn't exist, a folder inside a worktree, and a
# branch, which isn't a path
git worktree add -q "$tmp/demo--gone" -b gone
rm -rf "$tmp/demo--gone"
fails "wt: missing worktree: $tmp/demo--gone" ../demo--gone
mkdir "$tmp/plain"
fails "wt: not a git dir: ../plain" ../plain
fails "wt: not a git dir: ./nope" ./nope
fails "wt: not a worktree: ../demo--feat/src" ../demo--feat/src
fails "wt: not a path: feat" feat
# usage errors: more than one <path>, an option
fails "usage:" ../demo--feat ../demo
fails "usage:" -h
# and with no <path>, outside a repo; the ceiling stops git finding one above $tmp
(cd "$tmp/norepo" && export GIT_CEILING_DIRECTORIES="$tmp" && fails "wt: not in a git repo")

# no git UI configured: the message names the setting and the config file wt
# read, here one that doesn't set it, then the default one, which doesn't exist
: >"$WT_CONFIG"
fails "wt: no git UI configured" ../demo--feat
grep -qF "WT_GIT_UI" "$tmp/err" || fail "the not-configured message doesn't name WT_GIT_UI: $(cat "$tmp/err")"
grep -qF "$WT_CONFIG" "$tmp/err" || fail "the not-configured message doesn't name $WT_CONFIG: $(cat "$tmp/err")"
(
  unset WT_CONFIG
  fails "wt: no git UI configured"
  grep -qF "$HOME/.config/wt/config.sh" "$tmp/err" ||
    fail "the not-configured message doesn't name the default config file: $(cat "$tmp/err")"
)

# configured, but its program (the first word) isn't installed
git_ui_is "no-such-ui --all"
fails "wt: git UI not installed: no-such-ui" ../demo--feat
grep -qF "WT_GIT_UI" "$tmp/err" || fail "the not-installed message doesn't name WT_GIT_UI: $(cat "$tmp/err")"

echo "ok: wt-git-ui"
