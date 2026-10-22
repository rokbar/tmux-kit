---
status: accepted (2026-10-19, after the prototype on branch `prototype/wtui-host`)
---

# wtui is a host over tmux: a sidebar beside a nested tmux client

wtui stops being a popup you open and close and becomes the app you launch
from a plain terminal: a sidebar of projects, sessions and windows on the
left, the active session on the right. tmux stays underneath, and every
session is still a tmux session on the user's normal server. wtui runs a small
tmux server of its own (the host) with two panes: the sidebar, and an ordinary
tmux client attached to the normal server, which wtui switches between
sessions. Each launch gets its own host, which ends when its terminal closes;
nothing in it needs saving. The popup (`prefix w`) is dropped.

This supersedes two parts of ADR 0001: the popup, and "wtui never creates or
removes worktrees or sessions itself". Anything that touches a worktree still
goes only through wt (opening or starting its session, removing it, the git
UI, the picker), so wt's rules stay in one place. Plain session and window
operations (listing, switching, killing, selecting a window) call tmux
directly, since none of wt's rules apply to them.

## Considered Options

- **A sidebar pane in every session's windows**: no nesting, but an extra pane
  in every window that gets in the way of splits and zoom and is saved by
  resurrect, and one wtui per window, each with its own cursor.
- **wtui embedding a terminal emulator**: tmux's job redone in Go.
- **Replacing tmux**: loses resurrect/continuum, detaching and every existing
  session.
- **Routing session operations through new wt or t commands**: a layer with
  no rules in it.

## Consequences

- tmux runs inside tmux. The host has no prefix, so `C-b` reaches the user's
  sessions; the host binds only wtui's own key (ADR 0003).
- The git UI opens in a floating pane (tmux 3.7) over the session's window: a
  popup would be modal, and no key could leave it. It stays with that window,
  other clients of the session see it, and resurrect restores it as an
  ordinary pane. Without tmux 3.7 it falls back to a popup. Short dialogs (the
  picker, details, the remove dialog) are popups on the nested client, so they
  cover only the right side.
- The nested client attaches with `no-detach-on-destroy`, so when its session
  dies tmux moves it to the most recently active one, while the user's global
  `detach-on-destroy` stays as it is.
- One control-mode client per host watches the normal server. It can't be
  sessionless, so it follows the nested client; its only trace is a second
  attached client on the active session.
- The host cleans up from inside itself: closing the terminal window skips a
  launcher's exit handling.
