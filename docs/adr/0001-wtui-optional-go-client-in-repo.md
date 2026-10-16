# wtui is an optional Go program in tmux-kit, and acts only through wt

tmux-kit's runtime is bash, tmux, git and fzf, and that stays true. wtui, a
lazygit-style dashboard over wt, is the one exception: an optional Go program
(Bubble Tea v2) in `wtui/`, which only people who want it build or install.
The picker (`wt pick`, `prefix W`) stays the core way to open a branch and
doesn't depend on wtui.

wtui never creates or removes worktrees or sessions itself. It calls wt, plus
read-only git for previews, so the rules for naming, setting up and safely
removing worktrees stay in one place. It lives in this repo rather than its own
because wt's machine-readable output and the code that reads it have to change
together.

## Considered Options

- **bash + fzf** (no new dependency): fine for a list with key bindings, but
  poor at the parts that make wtui worth having: forms, the
  refuse → confirm → force flow of `wt rm`, live per-row state, and several
  views.
- **wtui replacing the picker**: that would make a compiled binary a core
  dependency of tmux-kit.
- **A separate repo**: wt's output format would become an interface between
  two repos that needs coordinated releases, and wtui has no users outside wt.
