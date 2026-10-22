# tmux-kit

A tmux config plus `wt`, which gives each git branch its own worktree and tmux
session, and `t`, which gives each project folder a session.

## Language

### Projects

**Project**:
A main clone together with its worktrees; for a session started outside any git repo, the folder it was started in. Every session belongs to exactly one project.
_Avoid_: repo (when sessions are meant too), workspace, project folder

**Projects root**:
The folder whose subfolders wtui offers as projects to open (by default `~/Projects`). Worktree folders in it aren't offered; they belong to their main clone's project.

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
A tmux session. It belongs to the worktree, main clone or folder it was started in, by its start folder, not its name, so renaming it changes nothing.

**Dev URL**:
The local address of a worktree's dev server, on a port that only that worktree owns.

### Agents

**Agent**:
A Claude Code instance running in a session's pane. Other coding tools aren't agents here.
_Avoid_: AI, assistant, bot

**Agent state**:
What an agent last reported it is doing: working, needs you (waiting on a prompt or permission), or done (finished its turn, until its window is shown). A session or project takes the most urgent state among its agents: needs you, then working, then done.

### Tools

**Git UI**:
The user's configured git program (e.g. lazygit), which wt can run in a worktree. Optional; none by default.

**Picker**:
wt's branch-first chooser, the core way to open a branch (`wt`, `wt pick`, `prefix W`). It needs nothing beyond tmux-kit's core dependencies.
_Avoid_: branch picker UI, launcher

**wtui**:
An optional app you launch from a plain terminal: the sidebar beside the active session, across all projects. It hands all branch work (browsing, opening, creating) to the picker, acts on worktrees only through wt, and on sessions and windows through tmux.
_Avoid_: the TUI, dashboard, popup

**Sidebar**:
wtui's tree of projects, their worktrees and sessions, and those sessions' windows, always on the left of the screen.

**Active session**:
The session shown on the right, next to the sidebar. Moving the sidebar's cursor never changes it; choosing a session or window does.
_Avoid_: current session, selected session

**wtui key**:
`Ctrl-\`. Pressed in a session, it makes the next key a sidebar shortcut acting on the active session; pressed alone, it switches between the sidebar and the session. tmux's prefix stays for everything inside a session.
_Avoid_: wtui prefix, leader
