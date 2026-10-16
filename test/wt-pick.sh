#!/usr/bin/env bash
# The picker marks branches that have a worktree / tmux session, shows what a
# branch is, and still opens or creates the branch it did before.
set -euo pipefail
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "$0")/lib/sandbox.sh"

# fzf is a TUI. This stub records the rows and options wt passes it, then acts
# as if $FZF_KEY was pressed with $FZF_QUERY typed: for enter, the real fzf
# (--filter, so non-interactive) picks the top match with wt's options, and
# accept-or-print-query falls back to the query. Attaching needs a terminal,
# so the tmux stub records the target instead.
REAL_FZF="$(command -v fzf)" || fail "needs fzf (filters the rows like the picker)"
REAL_TMUX="$(command -v tmux)" || fail "needs tmux"
export STUB_DIR="$tmp" REAL_FZF REAL_TMUX
mkdir "$tmp/bin"
cat >"$tmp/bin/fzf" <<'EOF'
#!/usr/bin/env bash
printf '%s\0' "$@" >"$STUB_DIR/fzf-args"
cat >"$STUB_DIR/fzf-in"
case "$FZF_KEY" in
  enter)
    top="$("$REAL_FZF" "$@" --filter "$FZF_QUERY" <"$STUB_DIR/fzf-in")" || [ $? = 1 ] || exit 2
    printf '%s\n' "${top:-$FZF_QUERY}" | head -n1 ;;
  alt-enter) printf '%s\n' "$FZF_QUERY" ;;
  alt-o) printf ':remote %s\n' "$FZF_QUERY" ;;
  old) echo "unknown option: --accept-nth" >&2; exit 2 ;; # fzf < 0.60
  *) exit 130 ;;
esac
EOF
cat >"$tmp/bin/tmux" <<'EOF'
#!/usr/bin/env bash
case "$1" in attach | switch-client) printf '%s\n' "$*" >"$STUB_DIR/attached"; exit 0 ;; esac
exec "$REAL_TMUX" "$@"
EOF
chmod +x "$tmp/bin/fzf" "$tmp/bin/tmux"
export PATH="$tmp/bin:$PATH"
[ "$(command -v fzf)" = "$tmp/bin/fzf" ] || fail "stub fzf isn't first on PATH"

# the row fzf got for <branch>: the line with a field that is exactly it
row() { awk -F'\t' -v b="$1" '{ for (i = 1; i <= NF; i++) if ($i == b) { print; exit } }' "$tmp/fzf-in"; }
# value of option <opt> in fzf's last argv (--opt=v or --opt v)
fzf_opt() {
  local a prev=""
  while IFS= read -r -d '' a; do
    case "$a" in "$1="*) printf '%s' "${a#"$1="}"; return ;; esac
    if [ "$prev" = "$1" ]; then printf '%s' "$a"; return; fi
    prev="$a"
  done <"$tmp/fzf-args"
  return 1
}
# consumes the record, so each check sees only the last wt pick
attached() { [ "$(cat "$tmp/attached" 2>/dev/null)" = "attach -t =$1" ] && rm "$tmp/attached"; }

# demo: main; feat (worktree, session "api"); parked (worktree, no session);
# feature (local only); remote-only (only on origin)
init_demo_repo
git init -q --bare "$tmp/origin.git"
git remote add origin "$tmp/origin.git"
git push -q origin main
git switch -q -c remote-only
git commit -q --allow-empty -m "remote work"
git push -q origin remote-only
git switch -q -c feature main
git commit -q --allow-empty -m "feature work"
git switch -q main
git branch -q -D remote-only
wt -d -n api feat
wt -d parked
tmux kill-session -t =parked
# a second session in feat's dir (e.g. from `t`): the row names the one wt opens
tmux new-session -d -s other -c "$tmp/demo--feat"

FZF_KEY=enter FZF_QUERY=feat wt pick </dev/null || fail "picking feat failed"
attached api || fail "picking feat didn't open its session api"
case "$(row feat)" in *●*"[api]"*) ;; *) fail "feat row lacks ● and [api]: $(row feat)" ;; esac
case "$(row parked)" in *●*) ;; *) fail "parked row lacks ●: $(row parked)" ;; esac
case "$(row parked)" in *"["*) fail "parked row shows a session: $(row parked)" ;; esac
for b in feature remote-only; do
  case "$(row "$b")" in "") fail "no row for $b" ;; *●* | *"["*) fail "$b row has markers: $(row "$b")" ;; esac
done

# markers are display-only: "api" (only feat's session name) matches no
# branch, so enter creates it. The newline takes the default session name.
printf '\n' | FZF_KEY=enter FZF_QUERY=api wt pick || fail "enter on api failed"
[ "$(git -C "$tmp/demo--api" symbolic-ref --short HEAD 2>/dev/null)" = api ] ||
  fail "enter on no branch match didn't create api: $(cat "$tmp/log")"
attached demo--api || fail "new branch api didn't open in session demo--api"

# alt-enter / alt-o (fzf prints the query / ":remote <query>"): a new branch
# from the current branch / origin's default branch
for k in "alt-enter:fe:main" "alt-o:hotfix:origin/main"; do
  IFS=: read -r key b base <<<"$k"
  printf '\n' | FZF_KEY=$key FZF_QUERY=$b wt pick || fail "$key on $b failed"
  [ "$(git -C "$tmp/demo--$b" symbolic-ref --short HEAD 2>/dev/null)" = "$b" ] || fail "$key didn't create $b"
  grep -qxF "wt: new branch $b from $base" "$tmp/log" || fail "$key's $b isn't from $base: $(cat "$tmp/log")"
  attached "$b" || fail "$key didn't open $b's session"
done

header="$(fzf_opt --header)" || fail "fzf got no --header"
for want in "enter: open (creates <query> if no match)" "alt-enter: new branch from main" \
  "alt-o: new branch from origin/main"; do
  grep -qxF "$want" <<<"$header" || fail "header lacks '$want': $header"
done

# fzf errors (e.g. too old for these options): wt fails and keeps fzf's message
# on screen, instead of exiting 0, which in a popup closes it without a word
FZF_KEY=old "$kit/bin/wt" pick </dev/null >"$tmp/log" 2>&1 && fail "wt pick exited 0 when fzf failed"
grep -qF "unknown option: --accept-nth" "$tmp/log" || fail "fzf's error isn't shown: $(cat "$tmp/log")"

# the preview, run the way fzf does for a focused row: {N} -> its field N, quoted
preview="$(fzf_opt --preview)" || fail "fzf got no --preview"
for want in "feature:feature work" "remote-only:remote work"; do
  b="${want%%:*}"
  cmd="$(row "$b" | C="$preview" awk -F'\t' '{ c = ENVIRON["C"]; for (i = 1; i <= NF; i++) gsub("\\{" i "\\}", "\047" $i "\047", c); print c }')"
  out="$(sh -c "$cmd" 2>&1)" || fail "preview of $b failed: $cmd: $out"
  grep -qF "${want#*:}" <<<"$out" || fail "preview of $b doesn't show its commit '${want#*:}': $out"
done

echo "ok: wt-pick"
