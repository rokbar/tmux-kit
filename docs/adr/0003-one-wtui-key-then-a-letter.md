# One wtui key, then a letter

In a session, wtui's shortcuts are `Ctrl-\` followed by one key; in the
sidebar, the same key alone. `Ctrl-\` by itself switches between the sidebar
and the session. Every sidebar action key works this way from a session,
acting on the active session; only cursor keys are sidebar-only. tmux keeps
`C-b` for everything inside a session, and wtui owns `Ctrl-\` for everything
across sessions. The key is bound in the host, which sees keys before the
nested client, its popups and floating panes, so it works everywhere.

`Ctrl-\` because it is a plain control character: every terminal sends it with
no setup, and shells, editors and Claude Code don't bind it (in a shell it's
SIGQUIT, which inside wtui no program sees). dtach and abduco use it as their
escape key for the same reason. Single chords such as Cmd-1 or Ctrl-Tab are
the terminal's business: a terminal can map one onto the two keys, and wtui
needn't know.

## Considered Options

- **`prefix Tab`** (bound in the user's tmux, asking the host to focus the
  sidebar): tmux gives a popup every key, so it can't leave one.
- **Ctrl-Tab**: terminals with tabs take it, and without an extended-keys
  protocol it arrives as a plain Tab; making it work needs terminal config.
- **Alt chords**: need a setting in every macOS terminal (Option types
  characters).
- **Cmd chords**: never reach a terminal program.
- **Another Ctrl+letter**: shells, editors and Claude Code use nearly all of
  them.
- **A short window after `Ctrl-\` in the sidebar**, so that `Ctrl-\ 3` works
  there too: it swallowed lazygit's `q` and closed wtui.

## Consequences

- The keys that pick a session run in the host rather than in the sidebar
  program, so keys typed straight after a jump land in the new session.
- Besides the wtui key, the host binds the keys after which focus can go
  right (the jumps `1`–`9`, `Tab`, `]` and `[`, `Enter`, and `Esc`, `C-b`
  and `z`) in its root table, acting only while the sidebar has focus: it has
  the sidebar press the key and holds the keys typed after it until the key
  is done, so keys typed ahead after a jump or `Enter` land in the session it
  shows, not in the sidebar. Elsewhere they pass on as they are.
