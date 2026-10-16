#!/usr/bin/env bash
# wt --help is short: a line per command and option, examples, the config path.
set -euo pipefail
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "$0")/lib/sandbox.sh"

wt --help || fail "wt --help failed"
help="$(cat "$tmp/log")"
n="$(grep -c '' <<<"$help")"
[ "$n" -le 25 ] || fail "wt --help is $n lines, want <= 25"

commands="$(sed '/^examples:/,$d' <<<"$help")"
for c in pick ls rm close; do
  grep -qE "^  wt \[?$c\]?( |\$)" <<<"$commands" || fail "no line for wt $c: $help"
done
for o in -d -n --from --no-dev -f; do
  grep -qE "(^| |\[)$o( |\]|\$)" <<<"$commands" || fail "option $o not in help: $help"
done

# each example's command: the text up to the gap before its description
examples="$(sed -n '/^examples:/,/^$/p' <<<"$help" | awk -F'  +' 'NF > 1 { print $2 }')"
for e in "wt" "wt feat/x" "wt --from origin/main feat/y" "wt ls" "wt rm feat/x"; do
  grep -qxF "$e" <<<"$examples" || fail "no example '$e': $help"
done

grep -qF "/.config/wt/config.sh" <<<"$(sed -n '/^examples:/,$p' <<<"$help")" ||
  fail "no config path after the examples: $help"
echo "ok: wt-help"
