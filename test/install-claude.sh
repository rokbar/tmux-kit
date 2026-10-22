#!/usr/bin/env bash
# install.sh --claude installs the agent-state plugin (or updates it) from the
# clone install.sh runs from, through the claude CLI: it points marketplace
# tmux-kit at the clone, then installs and updates agent-state@tmux-kit.
set -euo pipefail
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "$0")/lib/sandbox.sh"

# Offline: install.sh skips plugins that are already there.
plugins="$HOME/.tmux/plugins"
mkdir -p "$plugins/tpm" "$plugins/tmux-resurrect" "$plugins/tmux-continuum"

# stub claude: logs each call's arguments to $STUB_CLAUDE_LOG, and fails one
# whose arguments start with $STUB_CLAUDE_FAIL
mkdir "$tmp/bin"
export STUB_CLAUDE_LOG="$tmp/claude.log" STUB_CLAUDE_FAIL=""
cat >"$tmp/bin/claude" <<'EOF'
#!/usr/bin/env bash
echo "$*" >>"$STUB_CLAUDE_LOG"
case "$*" in
  "$STUB_CLAUDE_FAIL"*) [ -n "$STUB_CLAUDE_FAIL" ] && { echo "stub claude: failed" >&2; exit 1; } ;;
esac
echo "stub claude: ok"
EOF
chmod +x "$tmp/bin/claude"
real_path="$PATH"
export PATH="$tmp/bin:$PATH"

run_install() { rm -f "$STUB_CLAUDE_LOG"; "$kit/install.sh" "$@" >"$tmp/out" 2>&1; }
calls() { cat "$STUB_CLAUDE_LOG" 2>/dev/null || true; }
want="plugin marketplace add $kit
plugin install agent-state@tmux-kit
plugin update agent-state@tmux-kit"

run_install --claude || fail "install.sh --claude failed: $(cat "$tmp/out")"
[ "$(calls)" = "$want" ] || fail "install.sh --claude ran: $(calls)"
run_install --claude || fail "re-running install.sh --claude failed: $(cat "$tmp/out")"
[ "$(calls)" = "$want" ] || fail "re-running install.sh --claude ran: $(calls)"

# PATH as it was, without any folder that has a claude
no_claude=""
IFS=: read -ra dirs <<<"$real_path"
for d in "${dirs[@]}"; do [ -x "$d/claude" ] || no_claude+="${no_claude:+:}$d"; done

# no claude: the rest is installed, it says why the plugin isn't, and exits 1
rm -rf "$HOME/.local/bin"
if PATH="$no_claude" run_install --claude; then fail "install.sh --claude without claude succeeded"; fi
grep -q "needs claude" "$tmp/out" || fail "install.sh --claude without claude didn't say so: $(cat "$tmp/out")"
grep -q "plugin not installed" "$tmp/out" || fail "install.sh --claude without claude didn't say the plugin isn't installed: $(cat "$tmp/out")"
[ -L "$HOME/.local/bin/wt" ] || fail "install.sh --claude without claude didn't install the rest"

# a step that fails: it's named, the steps after it don't run, and install.sh
# exits 1 once the rest is installed
step=0
while IFS= read -r call; do
  step=$((step + 1))
  rm -rf "$HOME/.local/bin"
  if STUB_CLAUDE_FAIL="$call" run_install --claude; then fail "install.sh --claude succeeded despite a failing claude $call"; fi
  grep -qF "claude $call failed" "$tmp/out" || fail "a failing claude $call wasn't named: $(cat "$tmp/out")"
  [ "$(calls)" = "$(head -n "$step" <<<"$want")" ] || fail "with claude $call failing, install.sh --claude ran: $(calls)"
  [ -L "$HOME/.local/bin/wt" ] || fail "with claude $call failing, install.sh --claude didn't install the rest"
done <<<"$want"

run_install || fail "plain install.sh failed: $(cat "$tmp/out")"
[ -z "$(calls)" ] || fail "plain install.sh ran claude: $(calls)"

# The real CLI, with this test's HOME as its config (the user's stays out of
# it): marketplace tmux-kit pointing at a copy of the clone elsewhere (e.g. a
# worktree since removed), with the plugin installed from it, ends up
# pointing at the clone, with the plugin still installed and enabled.
export PATH="$real_path"
if ! command -v claude >/dev/null || ! command -v jq >/dev/null; then
  echo "note: $(basename "$0"): claude or jq isn't on PATH, so the real CLI isn't tried"
  echo "ok: install-claude"
  exit 0
fi
export CLAUDE_CONFIG_DIR="$HOME/.claude" DISABLE_AUTOUPDATER=1
mkdir "$tmp/elsewhere"
cp -R "$kit/.claude-plugin" "$kit/claude-plugin" "$tmp/elsewhere/"
real() { claude plugin "$@" </dev/null >"$tmp/log" 2>&1 || { cat "$tmp/log" >&2; fail "claude plugin $*"; }; }
real marketplace add "$tmp/elsewhere"
real install agent-state@tmux-kit

# where marketplace tmux-kit points, and agent-state@tmux-kit's state
source_of() { claude plugin marketplace list --json | jq -r '.[] | select(.name == "tmux-kit") | .path'; }
plugin_state() { claude plugin list --json | jq -c '.[] | select(.id == "agent-state@tmux-kit") | {enabled, errors}'; }
[ "$(source_of)" = "$tmp/elsewhere" ] || fail "the stale source wasn't set up: $(source_of)"

for run in first again; do
  "$kit/install.sh" --claude </dev/null >"$tmp/out" 2>&1 || fail "install.sh --claude with the real CLI ($run) failed: $(cat "$tmp/out")"
  [ "$(source_of)" = "$kit" ] || fail "after install.sh --claude ($run), tmux-kit points at $(source_of)"
  [ "$(plugin_state)" = '{"enabled":true,"errors":null}' ] ||
    fail "after install.sh --claude ($run), agent-state@tmux-kit is $(plugin_state)"
done

echo "ok: install-claude"
