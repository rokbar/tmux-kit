package tmux

import "errors"

// KillSession kills session (an id), and its windows with it. If the nested
// client shows it, tmux moves that client to the session with the most
// recent activity (see Attach).
func (c *Client) KillSession(session string) error {
	// tmux takes an empty target for a session of its choosing
	if session == "" {
		return errors.New("no session to kill")
	}
	_, err := c.output("kill-session", "-t", session)
	return err
}

// KillWindow kills session's window window (ids), as prefix & does. If it's
// the session's last window, the session ends too, as with KillSession.
func (c *Client) KillWindow(session, window string) error {
	if session == "" || window == "" {
		return errors.New("no window to kill")
	}
	_, err := c.output("kill-window", "-t", session+":"+window)
	return err
}
