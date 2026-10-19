package tmux

import "strings"

// AgentState is what an agent last reported it's doing, as its pane's
// @agent_state option says (tmux-kit's Claude Code plugin sets it). States
// go by urgency, so the most urgent of several is the greatest.
type AgentState int

const (
	// NoAgent is a pane without an agent state.
	NoAgent AgentState = iota
	// Done is an agent that has finished its turn (done).
	Done
	// Working is an agent at work (working).
	Working
	// NeedsYou is an agent waiting on a prompt or permission (needs-you).
	NeedsYou
)

// states are the agent states by the values @agent_state takes.
var states = map[string]AgentState{"done": Done, "working": Working, "needs-you": NeedsYou}

// Agent is the agent state of a pane, in its window and session.
type Agent struct {
	Session, Window, Pane string // ids
	State                 AgentState
}

// AgentsChanged is an agent state set, changed or unset in any pane. Agents
// are every pane's that has one now, in no particular order.
type AgentsChanged struct{ Agents []Agent }

func (AgentsChanged) event() {}

// agentFormat is every pane on the server that has an agent state, for
// parseAgents: the server's sessions, their windows and their panes, each
// pane as session|window|pane|state; (a , would start the format for the
// item tmux is at).
const agentFormat = "#{S:#{W:#{P:#{?@agent_state,#{session_id}|#{window_id}|#{pane_id}|#{@agent_state};,}}}}"

// parseAgents is agentFormat's value: the panes with an agent state. One
// whose @agent_state isn't a state is left out.
func parseAgents(value string) []Agent {
	var agents []Agent
	for _, record := range strings.Split(value, ";") {
		f := strings.Split(record, "|")
		if len(f) != 4 {
			continue
		}
		if state, ok := states[f[3]]; ok {
			agents = append(agents, Agent{Session: f[0], Window: f[1], Pane: f[2], State: state})
		}
	}
	return agents
}

// ClearDone clears done in window (an id): it unsets both agent state
// options (@agent_state and @agent_state_at) of the window's panes that say
// done. Each pane is checked again as it's cleared, in the same tmux
// command, so a state set since (an agent at work again) stays.
func (c *Client) ClearDone(window string) error {
	now, err := c.output("display-message", "-p", agentFormat)
	if err != nil {
		return err
	}
	var args []string
	cleared := map[string]bool{} // a linked window's panes are in each of its sessions
	for _, a := range parseAgents(now) {
		if a.Window != window || a.State != Done || cleared[a.Pane] {
			continue
		}
		cleared[a.Pane] = true
		if len(args) > 0 {
			args = append(args, ";")
		}
		args = append(args, "if-shell", "-F", "-t", a.Pane, "#{==:#{@agent_state},done}",
			"set-option -pu -t "+a.Pane+" @agent_state ; set-option -pu -t "+a.Pane+" @agent_state_at")
	}
	if len(args) == 0 {
		return nil
	}
	_, err = c.output(args...)
	return err
}
