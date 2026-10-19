package tmux_test

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rokbar/tmux-kit/wtui/internal/sandbox"
	"github.com/rokbar/tmux-kit/wtui/internal/tmux"
)

// agentsOf are agents as "session:window.pane state", sessions by name and
// windows and panes by index, sorted.
func agentsOf(t *testing.T, agents []tmux.Agent) []string {
	t.Helper()
	where := map[string]string{} // "<pane id> <session id> <window id>": session:window.pane
	for _, line := range strings.Split(tmuxRun(t, "-u", "list-panes", "-a", "-F", "#{pane_id} #{session_id} #{window_id}\t#{session_name}:#{window_index}.#{pane_index}"), "\n") {
		ids, at, _ := strings.Cut(line, "\t")
		where[ids] = at
	}
	states := map[tmux.AgentState]string{tmux.Working: "working", tmux.NeedsYou: "needs-you", tmux.Done: "done"}
	var got []string
	for _, a := range agents {
		at, ok := where[a.Pane+" "+a.Session+" "+a.Window]
		if !ok {
			at = fmt.Sprintf("%s %s %s (not on the server)", a.Session, a.Window, a.Pane)
		}
		got = append(got, at+" "+states[a.State])
	}
	slices.Sort(got)
	return got
}

// pane is the id of the pane at target (e.g. =b:0.1).
func pane(t *testing.T, target string) string {
	t.Helper()
	return tmuxRun(t, "display-message", "-p", "-t", target, "#{pane_id}")
}

// Agent states in any session's panes, not only the control client's,
// arrive as events within about a second of changing: every pane that has
// one, each with its session, window and pane.
func TestWatchSeesAgentStatesInEverySession(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "a")
	tmuxRun(t, "new-session", "-d", "-s", "b")
	tmuxRun(t, "split-window", "-d", "-t", "=b:0")
	w := started(t, "a")
	agents := func(want ...string) func(tmux.AgentsChanged) bool {
		return func(e tmux.AgentsChanged) bool { return slices.Equal(agentsOf(t, e.Agents), want) }
	}
	next(t, w, "no agents", agents())

	tmuxRun(t, "set-option", "-p", "-t", pane(t, "=b:0.1"), "@agent_state", "needs-you")
	tmuxRun(t, "set-option", "-p", "-t", pane(t, "=b:0.1"), "@agent_state_at", fmt.Sprint(time.Now().Unix()))
	set := time.Now()
	next(t, w, "b's needs-you", agents("b:0.1 needs-you"))
	if took := time.Since(set); took > 2*time.Second {
		t.Errorf("the change took %v to arrive, want about a second at most", took)
	}

	tmuxRun(t, "set-option", "-p", "-t", pane(t, "=b:0.0"), "@agent_state", "working")
	tmuxRun(t, "set-option", "-p", "-t", pane(t, "=a:0.0"), "@agent_state", "done")
	next(t, w, "b's working too, and a's done", agents("a:0.0 done", "b:0.0 working", "b:0.1 needs-you"))

	tmuxRun(t, "set-option", "-pu", "-t", pane(t, "=b:0.1"), "@agent_state")
	next(t, w, "b's needs-you gone", agents("a:0.0 done", "b:0.0 working"))
}

// The snapshot has every pane's agent state too, so the tree shows them at
// launch. A pane whose @agent_state is something else has none.
func TestSnapshotHasTheAgentStates(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "a")
	tmuxRun(t, "new-window", "-d", "-t", "=a:")
	tmuxRun(t, "new-session", "-d", "-s", "b c")
	tmuxRun(t, "split-window", "-d", "-t", "=b c:0")
	for target, state := range map[string]string{"=a:1.0": "done", "=b c:0.1": "needs-you", "=b c:0.0": "thinking"} {
		tmuxRun(t, "set-option", "-p", "-t", pane(t, target), "@agent_state", state)
	}
	got, err := (&tmux.Client{}).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a:1.0 done", "b c:0.1 needs-you"}; !slices.Equal(agentsOf(t, got.Agents), want) {
		t.Errorf("the snapshot's agents are %q, want %q", agentsOf(t, got.Agents), want)
	}
}

// options are pane target's agent state options, as tmux shows them.
func options(t *testing.T, target string) string {
	t.Helper()
	out, err := exec.Command("tmux", "show-options", "-p", "-t", pane(t, target)).CombinedOutput()
	if err != nil {
		t.Fatalf("tmux show-options %s: %v\n%s", target, err, out)
	}
	var opts []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(line, "@agent_state") {
			opts = append(opts, line)
		}
	}
	return strings.Join(opts, ", ")
}

// Clearing a window's done unsets both agent state options of its panes
// that say done, and only those: a working agent there stays working, and
// another window's done stays done. A window that's gone has nothing to
// clear.
func TestClearDone(t *testing.T) {
	s := sandbox.New(t)
	s.NeedTmux()
	tmuxRun(t, "new-session", "-d", "-s", "a")
	tmuxRun(t, "split-window", "-d", "-t", "=a:0")
	tmuxRun(t, "split-window", "-d", "-t", "=a:0")
	tmuxRun(t, "new-window", "-d", "-t", "=a:")
	for target, state := range map[string]string{"=a:0.0": "done", "=a:0.1": "working", "=a:1.0": "done"} {
		tmuxRun(t, "set-option", "-p", "-t", pane(t, target), "@agent_state", state)
		tmuxRun(t, "set-option", "-p", "-t", pane(t, target), "@agent_state_at", "1700000000")
	}
	window := tmuxRun(t, "display-message", "-p", "-t", "=a:0", "#{window_id}")

	if err := (&tmux.Client{}).ClearDone(window); err != nil {
		t.Fatal(err)
	}
	for target, want := range map[string]string{
		"=a:0.0": "",
		"=a:0.1": "@agent_state working, @agent_state_at 1700000000",
		"=a:0.2": "",
		"=a:1.0": "@agent_state done, @agent_state_at 1700000000",
	} {
		if got := options(t, target); got != want {
			t.Errorf("after clearing window 0's done, pane %s has %q, want %q", target, got, want)
		}
	}
	if err := (&tmux.Client{}).ClearDone("@999"); err != nil {
		t.Errorf("clearing a window that's gone: %v, want nothing to do", err)
	}
}
