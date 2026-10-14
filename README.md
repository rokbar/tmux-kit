# tmux-kit

tmux config plus `wt` (one git worktree and tmux session per branch) and `t`
(one tmux session per project folder).

## Install

```sh
brew install tmux fzf lazygit   # lazygit is optional
git clone https://github.com/rokbar/tmux-kit ~/Projects/tmux-kit
~/Projects/tmux-kit/install.sh
```

`install.sh` symlinks `~/.tmux.conf`, `~/.local/bin/wt` and `~/.local/bin/t`
into the clone (existing files go to `*.bak`) and clones the tmux plugins.
Update with `git pull`. `~/.local/bin` must be on `PATH`.

Your own tmux settings go in `~/.tmux.local.conf` (sourced last, before TPM).

## Keys (prefix = C-b)

| Key | Action |
| --- | --- |
| `prefix W` | fzf branch picker → worktree + session (`alt-enter`: new branch) |
| `prefix X` | remove the worktree you're in (asks first, refuses on unsaved work) |
| `prefix s` | sessions, most recently used first |
| `prefix e` | open the git root in your editor (`code`, or `$TMUX_KIT_EDITOR`) |
| `prefix C-s` / `C-r` | save / restore sessions (also auto-saved every 15 min and restored on start) |

To use a different editor for `prefix e`, put
`set-environment -g TMUX_KIT_EDITOR cursor` in `~/.tmux.local.conf`.

## Commands

- `t [name]`: attach to or switch to a session for the current folder.
- `wt <branch>`: open the branch in `../<repo>--<branch>` with its own session.
  It reuses an existing worktree, checks out the local or remote branch, or
  creates a new one. New worktrees get your `.env*` files copied in, the deps
  installed, and, if `package.json` has a `dev` script, a dev server on a free
  port in a `dev` window.
- `wt ls`, `wt rm <branch>`, `wt --help`.

## Config

`wt` reads `~/.config/wt/config.sh` (or `$WT_CONFIG`). See
[`config.example.sh`](config.example.sh) for the worktree location, folder
separator, remote, git UI window, dev script and port, and the files that get
copied or shared between worktrees.
