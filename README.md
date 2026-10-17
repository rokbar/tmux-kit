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

Optional: `tmux-kit/install.sh --wtui` also installs wtui, a worktree dashboard
on `prefix w`. It downloads the latest release for your OS and CPU to
`~/.local/bin/wtui` and checks its checksum; re-run it to update. Or build it
from the main branch with Go:
`GOBIN="$HOME/.local/bin" go install github.com/rokbar/tmux-kit/wtui@latest`.

Your own tmux settings go in `~/.tmux.local.conf` (sourced last, before TPM).

## Keys (prefix = C-b)

| Key | Action |
| --- | --- |
| `prefix W` | fzf branch picker → worktree + session (`alt-enter`: new branch, `alt-o`: new branch from origin's default branch) |
| `prefix X` | remove the worktree you're in (asks first, refuses on unsaved work) |
| `prefix w` | wtui, the worktree dashboard (only with wtui installed; `prefix s` covers tmux's window list) |
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
- `wt rm [-f] <branch|slug|path>`: remove the worktree and kill its session;
  the branch is kept. Refuses on uncommitted changes, commits on no other
  branch or remote, or shared files that became real files instead of links.
  `-f` skips these checks. A missing worktree (its folder deleted by hand) has
  nothing left to check: `wt rm` unregisters just that one from git and kills
  the session started in it, `-f` or not. A locked worktree is refused even
  with `-f` (`git worktree unlock` it first).
  Exit status: `3` on a safety refusal (one of the checks above, which `-f`
  overrides), so a script can offer to force; `1` for any other failure (not a
  worktree, the main clone, locked, usage).
- `wt close` (`prefix X`): asks, then runs `wt rm` on the worktree you're in.
- `wt doctor`: shows the config file wt loaded, the settings in effect (and
  whether `wt_before_link` is defined) and, in a repo, the worktree root. Checks
  tmux, git, fzf and lsof, `wt` on `PATH` and, inside tmux, that `prefix W` /
  `X` run wt. Exits non-zero if tmux, git or fzf is missing (lsof is optional).
- `wt --help`: the commands and options in brief, plus examples.

## Config

`wt` reads `~/.config/wt/config.sh` (or `$WT_CONFIG`). See
[`config.example.sh`](config.example.sh) for the worktree location, folder
separator, remote, git UI window, dev script and port, and the files that get
copied or shared between worktrees.
