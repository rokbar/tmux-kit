# tmux-kit

tmux config plus `wt` (one git worktree and tmux session per branch) and `t`
(one tmux session per project folder).

## Install

Needs tmux, git and fzf (macOS: `brew install tmux fzf`). Clone anywhere:

```sh
git clone https://github.com/rokbar/tmux-kit
tmux-kit/install.sh
```

`install.sh` symlinks `~/.tmux.conf`, `~/.local/bin/wt` and `~/.local/bin/t`
into the clone (existing files go to `*.bak`) and clones the tmux plugins.
Update with `git pull`. `~/.local/bin` must be on `PATH`.

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

- `t [name]`: attach to or switch to a session for the current folder.
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
- `wt ls`: list worktrees with dirty state and tmux session.
- `wt rm [-f] <branch|slug|path>`: remove the worktree and kill its session;
  the branch is kept. Refuses on uncommitted changes, commits on no other
  branch or remote, or shared files that became real files instead of links.
  `-f` skips these checks.
- `wt close` (`prefix X`): asks, then runs `wt rm` on the worktree you're in.
- `wt doctor`: shows the config file wt loaded, the settings in effect (and
  whether `wt_before_link` is defined) and, in a repo, the worktree root. Checks
  tmux, git, fzf and lsof, `wt` on `PATH` and, inside tmux, that `prefix W` /
  `X` run wt. Exits non-zero if tmux, git or fzf is missing (lsof is optional).
- `wt --help`: one line per command, plus examples.

## Config

`wt` reads `~/.config/wt/config.sh` (or `$WT_CONFIG`). See
[`config.example.sh`](config.example.sh) for the worktree location, folder
separator, remote, git UI window, dev script and port, and the files that get
copied or shared between worktrees.
