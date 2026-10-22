# tmux-kit

tmux config plus `wt` (one git worktree and tmux session per branch) and `t`
(one tmux session per project folder). Optional: [wtui](#wtui), which you
launch from a plain terminal to work in a sidebar of every project, worktree
and session beside the session you're in.

## Install

Needs tmux, git and fzf (macOS: `brew install tmux fzf`). Clone anywhere:

```sh
git clone https://github.com/rokbar/tmux-kit
tmux-kit/install.sh
```

`install.sh` symlinks `~/.tmux.conf`, `~/.local/bin/wt` and `~/.local/bin/t`
into the clone (existing files go to `*.bak`) and clones the tmux plugins.
Update with `git pull`. `~/.local/bin` must be on `PATH`.

Optional: `tmux-kit/install.sh --wtui` also installs [wtui](#wtui). It
downloads the latest release for your OS and CPU to `~/.local/bin/wtui` and
checks its checksum; re-run it to update. Or build it from the main branch
with Go:
`GOBIN="$HOME/.local/bin" go install github.com/rokbar/tmux-kit/wtui@latest`.

Optional: `tmux-kit/install.sh --claude` also installs the
[Claude Code plugin](#claude-code-plugin) from the clone, for wtui's agent
states. Flags combine: `install.sh --wtui --claude`.

Your own tmux settings go in `~/.tmux.local.conf` (sourced last, before TPM).

## Keys (prefix = C-b)

| Key | Action |
| --- | --- |
| `prefix W` | fzf branch picker → worktree + session (`alt-enter`: new branch, `alt-o`: new branch from origin's default branch) |
| `prefix X` | remove the worktree you're in (asks first, refuses on unsaved work) |
| `prefix s` | sessions, most recently used first |
| `prefix e` | open the git root in your editor (`code`, or `$TMUX_KIT_EDITOR`) |
| `prefix C-s` / `C-r` | save / restore sessions (also auto-saved every 15 min and restored on start) |

On macOS, `alt` keys need your terminal to send Option as Alt/Meta (e.g.
Ghostty `macos-option-as-alt = left`, iTerm2 "Left Option key: Esc+").

To use a different editor for `prefix e`, put
`set-environment -g TMUX_KIT_EDITOR cursor` in `~/.tmux.local.conf`.

## Commands

- `t [-d] [name]`: attach to or switch to a session for the current folder,
  creating it if needed. `-d` only creates it (detached) and prints its name,
  for scripts.
- `wt` (or `wt pick`, or `prefix W`): fzf branch picker. Bare `wt` opens it in
  a git repo with fzf installed, else prints help. `enter` opens the match, or
  creates the typed name if nothing matches; `alt-enter` creates the typed name
  from the current branch, `alt-o` from the remote's default branch (fetched
  first). If the worktree has no session yet, it asks for a session name
  (`enter` keeps the default).
- `wt <branch>`: open the branch in `../<repo>--<branch>` (`/` becomes `-`)
  with its own session. It reuses the worktree that has the branch checked out,
  else checks out the local or remote branch, else creates it from the current
  branch. `--from <ref>` instead creates a new branch from `<ref>` (a remote
  ref like `origin/main` is fetched first). `-d` stays detached; `-n <name>`
  names a new session.
  New worktrees get your env files (by default `.env`, `.env.local`,
  `.env.development.local`) copied from the worktree you run wt in, and shared
  files linked. With a lockfile, wt installs the deps (pnpm, bun, yarn or
  npm); if `package.json` also has a `dev` script, the install and
  `PORT=<port> <pm> run dev` run in a `dev` window, on the first free port
  from 3000 that no other worktree claims in its `.devurl` (wt writes the new
  worktree's). `--no-dev` installs only. New sessions also get a git UI
  window, if configured and installed. wt prints each step.
- `wt <path>` (`wt [-d] [-n <name>] <path>`): open the existing worktree or
  main clone at that path as it is, even with no branch checked out (detached,
  or mid-rebase, which `wt <branch>` can't open). An argument starting with `/`
  or `.` is a path (`.`, `./x`, `../<repo>--x`), never a branch; it works from
  outside the repo too. It reuses the session started in that folder, else
  starts one named after the branch, or after the folder if there's no branch;
  `-d` and `-n` work as for a branch. It never creates a worktree or branch,
  copies or links files, or starts installs or a dev server, so `--from` is an
  error, as are a missing worktree and a path that isn't a worktree's own
  folder (a folder inside one included).
- `wt ls`: list worktrees with dirty state and tmux session.
  `wt ls --porcelain` is the form for scripts: one line for the main clone,
  then one for each worktree in git's order, with tab-separated fields:

  | # | Field | Values |
  | --- | --- | --- |
  | 1 | path | absolute path of the worktree (or main clone) |
  | 2 | branch | branch name; empty if detached (incl. mid-rebase) or missing |
  | 3 | state | `clean`, `dirty` or `missing` |
  | 4 | session | name of the session started in it, even if renamed; empty if none |
  | 5 | dev URL | first line of its `.devurl`; empty if none |

  Fields are never reordered, removed or redefined, and new ones are only added
  at the end, so read the fields you know and ignore any after them. A breaking
  change would need `--porcelain=v2`, leaving `--porcelain` as it is. Known
  limitation: a tab or newline in a worktree's path breaks the format, as it
  already breaks wt's own reading of git's worktree list.
- `wt rm [-f] <branch|slug|path>`: remove the worktree and kill its sessions
  (every session started in its folder or a folder inside it); the branch is
  kept. Refuses on uncommitted changes, commits on no other branch or remote,
  or shared files that became real files instead of links. `-f` skips these
  checks. A missing worktree (its folder deleted by hand) has nothing left to
  check: `wt rm` unregisters just that one from git and kills the sessions
  started in it, `-f` or not. A locked worktree is refused even with `-f`
  (`git worktree unlock` it first).
  Exit status: `3` on a safety refusal (one of the checks above, which `-f`
  overrides), so a script can offer to force; `1` for any other failure (not a
  worktree, the main clone, locked, usage).
- `wt close` (`prefix X`): asks, then runs `wt rm` on the worktree you're in.
- `wt git-ui [<path>]`: run your git UI (`WT_GIT_UI` in the config, e.g.
  `lazygit`) in the worktree or main clone at `<path>`, by default the
  worktree you're in. It runs in that folder in the foreground, as a shell
  command (`sh -c`), and wt exits with its status. wt exits 1, saying why, if
  no git UI is configured, its program isn't installed, or `<path>` isn't the
  folder of a present worktree or the main clone. `git-ui` is a command name,
  like `ls` or `rm`, so a branch called `git-ui` that has a worktree opens by
  its path: `wt ./<path>` (e.g. `wt ../<repo>--git-ui`).
- `wt doctor`: shows the config file wt loaded, the settings in effect (and
  whether `wt_before_link` is defined) and, in a repo, the worktree root. Checks
  tmux, git, fzf and lsof, `wt` on `PATH` and, inside tmux, that `prefix W` /
  `X` run wt. Exits non-zero if tmux, git or fzf is missing (lsof is optional).
- `wt --help`: the commands and options in brief, plus examples.

Inside tmux, `t` and `wt` switch the tmux client to the session they open
(the picker's included). With `TMUX_KIT_CLIENT` set to a client's name
(`#{client_name}`), they switch that client instead of the one tmux takes for
current, and `wt rm` switches no client (wtui's moves on by itself when its
session ends). wtui sets it in its popups, where tmux's current client is the
one used last, which may be another terminal's.

## Config

`wt` reads `~/.config/wt/config.sh` (or `$WT_CONFIG`). See
[`config.example.sh`](config.example.sh) for the worktree location, folder
separator, remote, git UI window, dev script and port, and the files that get
copied or shared between worktrees.

## wtui

Optional (see [Install](#install)). Run `wtui` in a plain terminal, not inside
tmux: on the left, the sidebar, a tree of every project that has a session,
with its worktrees, its sessions and their windows; on the right, the active
session. The right side is a tmux client on your normal tmux server, so
everything there (`C-b` and your bindings, colours, the mouse, copy mode and
the clipboard) is plain tmux, and your sessions stay ordinary sessions: they
outlive wtui, and resurrect and continuum keep saving them. Each launch is
its own: two terminals can show two sessions, and closing the terminal window
ends that wtui and leaves nothing behind.

Needs tmux 3.6 or newer (wtui says so, naming the version it needs). The git
UI floats over the session with tmux 3.7 or newer, and pops up over it before
that.

### Launching

`wtui` starts on:

1. the session you used last in the worktree or main clone you run it in (or
   a folder inside it);
2. else the session started in the folder you're in, or the nearest folder
   above it, outside git;
3. else the session you used last;
4. with no session at all (or no tmux server, e.g. after a reboot), a new one
   for the folder, as `t -d` names it. That starts the server, so continuum
   restores your saved sessions into the sidebar.

Inside tmux, `wtui` refuses: there, `prefix s` and `prefix W` switch sessions.
`q` in the sidebar or `prefix d` on the right closes wtui, without asking: the
sessions live on. When the active session ends, the right side shows the
session you used last, with focus on the sidebar. When the last one ends, the
right side says so until a session exists again (`p` opens one).

### The sidebar

Projects are in alphabetical order. A git project lists its main clone first,
then its worktrees in git's order, and a worktree with one session is one row,
named after the session (several sessions go under a row for the worktree).
Other projects list their sessions by name. Sessions unfold into their
windows; the active session shows its windows, and its row, worktree and
project are highlighted. The first nine sessions are numbered in the margin.

- dimmed: a worktree with no session (`enter` opens one); `✗`: a missing
  worktree;
- after the name: `±` dirty, `:3001` the dev URL's port;
- at the right edge, each row's most urgent agent state beneath it: `!` (red)
  needs you, `*` (yellow) working, `✓` (dim green) done, until its window is
  shown (see [the plugin](#claude-code-plugin)).

The tree follows tmux within a second, wherever sessions and windows change
(`wt` or `t` in a session, another terminal). `±` and `:port` refresh every
10 s and after each action; `r` rebuilds everything now. At the bottom, the
status line says why a key doesn't apply to the row, or what wt said after an
action, above `? shortcuts`.

### Keys

The **wtui key** is `C-\` (Ctrl-backslash). Alone, it switches between the
sidebar and the session (showing a hidden sidebar). In the sidebar, press a
key alone; in a session, press the wtui key, then the key, and it acts on the
active session: `C-\ g` is the git UI of the session you're in. `C-b` stays
tmux's prefix on the right; in the sidebar, it moves focus right and passes
the prefix on, so `C-b s` works there too.

| Key | Action |
| --- | --- |
| `j` `k` `↓` `↑` | move the cursor (moving it never changes the right side) |
| `l` `→`, `h` `←`, `space` | unfold (or go to the first row under it), fold (or go to the parent), fold or unfold |
| `enter` | show the session or window on the right, with focus there; on a worktree with no session, open its session through wt first; on a project or a worktree with several sessions, fold or unfold |
| `1`–`9` | show session N |
| `tab` | show the last session; again to come back |
| `]` / `[` | show the next / previous session in the active session's project, wrapping. After the wtui key, `] ] ]` repeats without it |
| `e` | your editor on the worktree or folder (`TMUX_KIT_EDITOR`, as for `prefix e`; default VS Code) |
| `o` | the dev URL in your browser |
| `g` | your git UI (`wt git-ui`) in the worktree, floating over the session's window. The wtui key works inside it, it's still there when you come back to that session, and quitting it puts focus on the sidebar. Before tmux 3.7, it pops up over the right side instead |
| `n` | the picker, from the worktree, over the right side; what it opens becomes the active session |
| `i` | the worktree's details: branch, dev URL, changed files, last 10 commits, ahead/behind or "not pushed" |
| `x` | remove the worktree (`wt rm`), after a dialog naming every session and agent that ends. wt's safety refusal shows there, with an offer to force |
| `d` | kill the session or window under the cursor, after a dialog naming the agents that end. Its worktree stays |
| `p` | open a project from the projects root: fzf over its folders (worktrees left out). A git repo opens through `wt`, a plain folder through `t` |
| `z` | hide the sidebar; the wtui key brings it back |
| `r` | rebuild the tree |
| `?` | every shortcut |
| `esc` | back to the right side, as the wtui key |
| `q` | close wtui |

Keys typed straight after a jump (`1`–`9`, `tab`, `]`, `[`) or `enter` land in
the session it shows, in the sidebar as after the wtui key: wtui holds them
until the switch is done. For that, its host binds `enter`, the digits, `tab`,
`]`, `[`, `esc`, `C-b` and `z` itself, and in a session passes them on
unchanged.

`n`, `i`, `x`, `d` and `p` open popups over the right side, which take every
key until they close (so the wtui key can't leave them); focus then goes back
to the sidebar. Mouse: click a side to focus it, or a row to move the cursor
there; drag the border to resize the sidebar.

### Settings

wtui's settings are tmux options on your normal server, read when it
launches. Put them in `~/.tmux.local.conf`:

```
# the wtui key, as tmux names keys (default C-\)
set -g @wtui-key 'C-\'
# the sidebar's width in columns (default 30)
set -g @wtui-width 30
# the folder p offers projects from (default ~/Projects)
set -g @wtui-projects-root ~/code
```

Dragging the border sets the width for this launch, and resizing the
terminal keeps it. While the terminal is narrower than 100 columns, the
sidebar hides, and it comes back once it's wider, unless you hid it with `z`.

### Terminal chords

wtui has no one-key shortcuts like `cmd+1` of its own: have your terminal
type the wtui key and the key for you. `\x1c` is `C-\`. In Ghostty
(`~/.config/ghostty/config`):

```
keybind = cmd+1=text:\x1c1
keybind = cmd+2=text:\x1c2
keybind = cmd+3=text:\x1c3
```

These replace Ghostty's own `cmd+1`–`cmd+3` (go to tab N). Other terminals
have the same, under a name like "send text" or "send hex codes" (for
`cmd+1`: `0x1c 0x31`). With another `@wtui-key`, send that key's code
instead.

### Claude Code plugin

Optional: the `agent-state` plugin keeps each Claude Code agent's state on its
tmux pane, for wtui to show, or for you with `tmux show -p`:

| Pane option | Value |
| --- | --- |
| `@agent_state` | `working` once you submit a prompt, and after each tool call (so again once you answer a permission prompt); `needs-you` while it waits on you to answer a permission prompt or a question (its own or an MCP server's); `done` when it finishes its turn |
| `@agent_state_at` | when that was set, in seconds since the epoch |

Quitting Claude Code (or `/clear`) unsets both. Claude Code reports a
prompt or question only once it has waited about six seconds with nothing
typed, so `needs-you` comes that much later (answer sooner and it stays
`working`); its idle reminders change nothing. The plugin is hooks only, each
one tmux command against the agent's pane: outside tmux they do nothing, they
never fail or print anything, and they run alongside your own hooks. It edits
no settings file.

Install it from your clone (it needs `claude` on `PATH`):

```sh
tmux-kit/install.sh --claude
```

This points Claude Code's `tmux-kit` marketplace at the clone, adding it or
moving it there from wherever it pointed before, then installs
`agent-state@tmux-kit`, or updates it if it's already there. It changes your
Claude Code user settings, which is why it's opt-in. Claude Code loads the
plugin from the clone itself, so a `git pull` takes effect from the next
session. Re-running it is safe. Re-run it if you move the clone: until then,
the plugin doesn't load. Agents already running pick the plugin up once you
restart them.

Or in Claude Code, from this repo's marketplace (while the repo is private,
this needs access to it):

```
/plugin marketplace add rokbar/tmux-kit
/plugin install agent-state@tmux-kit
```

To list it in a marketplace of your own instead, give its entry the source
`{"source": "git-subdir", "url": "rokbar/tmux-kit", "path": "claude-plugin"}`.
