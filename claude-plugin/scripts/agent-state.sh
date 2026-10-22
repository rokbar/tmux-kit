#!/bin/sh
# Claude Code hook (see ../hooks/hooks.json): report this agent's state on its
# tmux pane, as one tmux command against $TMUX_PANE.
#   agent-state.sh working|needs-you|done  sets @agent_state, and
#                                          @agent_state_at (epoch seconds),
#                                          and marks the pane
#   agent-state.sh unset                   unsets both, and leaves the mark
#   agent-state.sh start                   only marks the pane
# The mark (@agent_plugin 1) says the agent running in the pane has this
# plugin, so wtui can tell an agent that hasn't reported yet from one whose
# state can't arrive. Nothing unmarks a pane: wtui reads the mark only while
# an agent runs there.
# Outside tmux (no $TMUX_PANE) it does nothing: without a target, tmux would
# pick a pane itself. It always exits 0 and prints nothing, even if tmux fails,
# so it never shows up in Claude Code or gets in another hook's way.
set -u
pane="${TMUX_PANE:-}"
[ -n "$pane" ] || exit 0
case "${1:-}" in
  working | needs-you | done)
    tmux set -p -t "$pane" @agent_state "$1" \; set -p -t "$pane" @agent_state_at "$(date +%s)" \
      \; set -p -t "$pane" @agent_plugin 1
    ;;
  unset) tmux set -pu -t "$pane" @agent_state \; set -pu -t "$pane" @agent_state_at ;;
  start) tmux set -p -t "$pane" @agent_plugin 1 ;;
esac </dev/null >/dev/null 2>&1
exit 0
