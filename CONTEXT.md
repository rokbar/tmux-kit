# tmux-kit

A tmux config plus `wt`, which gives each git branch its own worktree and tmux
session, and `t`, which gives each project folder a session.

## Language

### Worktrees

**Main clone**:
The repo's original checkout. Every other worktree is made from it, and wt never removes it.
_Avoid_: main worktree, root repo

**Worktree**:
A git worktree of the repo other than the main clone, in a folder named `<repo>--<slug>` (next to the main clone by default).
_Avoid_: checkout, workspace

**Missing worktree**:
A worktree git still has registered but whose folder is gone. Removing it only unregisters it.

**Slug**:
A branch name with `/` replaced by `-`, used in the worktree's folder name.

**Invoking worktree**:
The worktree wt is run from. New worktrees copy its local files, and a new branch starts from its branch unless told otherwise.
_Avoid_: current worktree, source worktree

**Remote default branch**:
The default branch of the configured remote (e.g. `origin/main`), the other common base for a new branch.

**Safety refusal**:
wt declining to remove a worktree because something would be lost: uncommitted changes, commits on no other branch or remote, or a shared file that isn't linked. Forcing overrides it; other removal failures can't be forced.
_Avoid_: rm error, rm failure

### Sessions

**Session**:
The tmux session started in a worktree. It belongs to that worktree by its start folder, not its name, so renaming it changes nothing.

**Dev URL**:
The local address of a worktree's dev server, on a port that only that worktree owns.

### Tools

**Git UI**:
The user's configured git program (e.g. lazygit), which wt can run in a worktree. Optional; none by default.

**Picker**:
wt's branch-first chooser, the core way to open a branch (`wt`, `wt pick`, `prefix W`). It needs nothing beyond tmux-kit's core dependencies.
_Avoid_: branch picker UI, launcher

**wtui**:
An optional, worktree-first dashboard over wt for the current repo. It sits next to the picker, hands all branch work (browsing, opening, creating) to it, and only acts through wt.
_Avoid_: the TUI, dashboard (unqualified)
