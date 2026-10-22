#!/usr/bin/env bash
# The Claude Code plugin's hooks (claude-plugin/hooks/hooks.json), run as
# Claude Code runs them: each matching command through sh, with the event's
# JSON on stdin and $TMUX_PANE from the agent's pane. Each sets that pane's
# @agent_state and @agent_state_at, or unsets both, and prints nothing. Each
# but SessionEnd marks the pane as the plugin's (@agent_plugin), and nothing
# unmarks it.
# Also: the repo root's marketplace lists the plugin.
set -euo pipefail
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "$0")/lib/sandbox.sh"

command -v jq >/dev/null || { echo "skip: $(basename "$0"): jq isn't on PATH, so hooks.json can't be read"; exit 0; }
plugin="$kit/claude-plugin"
hooks="$plugin/hooks/hooks.json"
[ -f "$hooks" ] || fail "no $hooks"

# The marketplace's entry for the plugin's folder has the plugin's own name:
# people install by the entry's name, and a different manifest name breaks it.
market="$kit/.claude-plugin/marketplace.json"
entry="$(jq -r '.plugins[] | select(.source == "./claude-plugin") | .name' "$market")"
name="$(jq -r .name "$plugin/.claude-plugin/plugin.json")"
[ -n "$entry" ] && [ "$entry" = "$name" ] || fail "$market lists ./claude-plugin as '$entry', but the plugin is '$name'"
# Claude Code's own check of both manifests and hooks.json, where it's installed
# (with this test's HOME as its config, so the user's stays out of it)
if command -v claude >/dev/null; then
  for dir in "$kit" "$plugin"; do
    CLAUDE_CONFIG_DIR="$HOME/.claude" claude plugin validate "$dir" >"$tmp/out" 2>&1 ||
      fail "claude plugin validate $dir: $(cat "$tmp/out")"
  done
else
  echo "note: $(basename "$0"): claude isn't on PATH, so the manifests aren't validated"
fi

# The commands hooks.json runs for <event>, one per line, as Claude Code picks
# them: matcher groups whose matcher is empty, "*" or missing, or lists
# <value> (exact names split on | or ,). A matcher Claude Code would read as a
# regular expression isn't supported here.
commands_for() {
  jq -r --arg event "$1" --arg value "${2:-}" '
    .hooks[$event] // [] | .[]
    | (.matcher // "") as $m
    | if ($m | test("^[A-Za-z0-9_ ,|-]*$") | not) then error("regex matcher \($m)") else . end
    | select($m == "" or $m == "*" or ($m | split("[|,]"; null) | map(gsub("^ +| +$"; "")) | index($value)))
    | .hooks[] | select(.type == "command") | .command' "$hooks"
}

# Runs the hooks for <event> [<matcher value>] with $TMUX_PANE as it is now,
# like Claude Code: through sh, from the project folder, with the event's JSON
# on stdin. Each must exit 0 and print nothing.
hook() {
  local event="$1" value="${2:-}" cmd status
  local input="{\"session_id\":\"s1\",\"cwd\":\"$tmp\",\"hook_event_name\":\"$event\",\"notification_type\":\"$value\",\"reason\":\"$value\",\"source\":\"$value\"}"
  while IFS= read -r cmd; do
    status=0
    (cd "$tmp" && CLAUDE_PLUGIN_ROOT="$plugin" sh -c "$cmd" <<<"$input" >"$tmp/out" 2>"$tmp/err") || status=$?
    [ "$status" = 0 ] || fail "$event hook exited $status: $cmd: $(cat "$tmp/err")"
    [ ! -s "$tmp/out" ] || fail "$event hook wrote to stdout: $(cat "$tmp/out")"
    [ ! -s "$tmp/err" ] || fail "$event hook wrote to stderr: $(cat "$tmp/err")"
  done < <(commands_for "$event" "$value")
}

# <pane>'s @agent_state and @agent_state_at, as "<state> <at>" (empty if unset)
state_of() { tmux display -p -t "$1" '#{@agent_state} #{@agent_state_at}'; }

# <pane>'s @agent_plugin (empty if unset)
mark_of() { tmux display -p -t "$1" '#{@agent_plugin}'; }

# <pane> says <state>, set between epoch seconds <after> and now
says() {
  local pane="$1" want="$2" after="$3" got at
  got="$(state_of "$pane")"
  [ "${got% *}" = "$want" ] || fail "$pane's @agent_state is '${got% *}', want '$want'"
  at="${got#* }"
  case "$at" in '' | *[!0-9]*) fail "$pane's @agent_state_at is '$at', want epoch seconds" ;; esac
  [ "$at" -ge "$after" ] && [ "$at" -le "$(date +%s)" ] ||
    fail "$pane's @agent_state_at is $at, want between $after and now ($(date +%s))"
}

# every hook, with a matcher value that runs it, then <command> after each
every_hook() {
  local h
  for h in "SessionStart startup" UserPromptSubmit "PostToolUse Bash" "PostToolUseFailure Bash" "Notification permission_prompt" Stop "SessionEnd other"; do
    # shellcheck disable=SC2086 # split into event and matcher value
    hook $h
    "${@:-true}"
  done
}

# the agent's pane, and another pane: the active one, in another window
tmux new-session -d -s s 'cat >/dev/null'
agent="$(tmux display -p -t s '#{pane_id}')"
other="$(tmux new-window -P -F '#{pane_id}' -t s 'cat >/dev/null')"
export TMUX_PANE="$agent"

# starting (or resuming, clearing, compacting, forking) marks the pane, and
# sets no state
for source in startup resume clear compact fork; do
  tmux set -pu -t "$agent" @agent_plugin
  hook SessionStart "$source"
  [ "$(mark_of "$agent")" = 1 ] || fail "after SessionStart ($source), $agent's @agent_plugin is '$(mark_of "$agent")'"
  [ "$(state_of "$agent")" = " " ] || fail "SessionStart ($source) set a state: $(state_of "$agent")"
done

# each state marks the pane too: an agent that picked the plugin up without
# starting again (/reload-plugins) is marked once it reports anything
for h in UserPromptSubmit "PostToolUse Bash" "PostToolUseFailure Bash" "Notification permission_prompt" Stop; do
  tmux set -pu -t "$agent" @agent_plugin
  # shellcheck disable=SC2086 # split into event and matcher value
  hook $h
  [ "$(mark_of "$agent")" = 1 ] || fail "after $h, $agent's @agent_plugin is '$(mark_of "$agent")'"
done

t0="$(date +%s)"
hook UserPromptSubmit
says "$agent" working "$t0"

# a permission prompt, then carrying on once it's answered
hook Notification permission_prompt
says "$agent" needs-you "$t0"
hook PostToolUse Bash
says "$agent" working "$t0"
# also when the tool it was allowed fails (Claude Code's PostToolUse is for
# tools that succeed)
hook Notification permission_prompt
hook PostToolUseFailure Bash
says "$agent" working "$t0"

# an MCP server's questions are the agent waiting on you too
for question in elicitation_dialog elicitation_url_dialog; do
  hook UserPromptSubmit
  hook Notification "$question"
  says "$agent" needs-you "$t0"
done

# done at the end of the turn; reminders and other notifications after it
# leave it done, with the time it was set
hook UserPromptSubmit
hook Stop
says "$agent" done "$t0"
done_at="$(state_of "$agent")"
sleep 1
for type in idle_prompt auth_success elicitation_complete elicitation_response; do
  hook Notification "$type"
  [ "$(state_of "$agent")" = "$done_at" ] ||
    fail "a $type notification changed the state from '$done_at' to '$(state_of "$agent")'"
done

# quitting, or /clear, unsets both options, and leaves the mark
for reason in prompt_input_exit clear; do
  hook UserPromptSubmit
  hook SessionEnd "$reason"
  opts="$(tmux show -p -t "$agent")"
  case "$opts" in *@agent_state*) fail "after SessionEnd ($reason), $agent has: $opts" ;; esac
  [ "$(mark_of "$agent")" = 1 ] || fail "SessionEnd ($reason) unmarked $agent"
done

# only the agent's own pane was ever touched
[ "$(state_of "$other")" = " " ] || fail "the hooks set $other, not the agent's pane: $(state_of "$other")"
[ -z "$(mark_of "$other")" ] || fail "the hooks marked $other, not the agent's pane"

# outside tmux, every hook does nothing, silently
hook UserPromptSubmit
tmux set -pu -t "$agent" @agent_plugin
before="$(state_of "$agent")"
unchanged() {
  [ -z "$(mark_of "$agent")" ] || fail "with no \$TMUX_PANE, a hook marked $agent"
  [ "$(state_of "$agent")" = "$before" ] || fail "with no \$TMUX_PANE, a hook changed $agent from '$before' to '$(state_of "$agent")'"
  [ "$(state_of "$other")" = " " ] || fail "with no \$TMUX_PANE, a hook set $other: $(state_of "$other")"
}
(
  unset TMUX_PANE
  every_hook unchanged
)

# tmux failing doesn't fail a hook or make it print: a pane that's gone, and
# a tmux that errors
(
  TMUX_PANE=%999
  every_hook
)
mkdir "$tmp/bin"
printf '#!/bin/sh\necho "tmux out"\necho "tmux err" >&2\nexit 1\n' >"$tmp/bin/tmux"
chmod +x "$tmp/bin/tmux"
(
  PATH="$tmp/bin:$PATH"
  [ "$(command -v tmux)" = "$tmp/bin/tmux" ] || fail "failing tmux isn't first on PATH"
  every_hook
)

echo "ok: agent-state-plugin"
