# wt config: copy to ~/.config/wt/config.sh (or point $WT_CONFIG at it) and
# keep only what you change. Sourced by bash; defaults shown.

# WT_ROOT=""          # dir for worktrees; empty: next to the main clone
# WT_SEP="--"         # worktree dir = <repo><WT_SEP><slug>
# WT_REMOTE="origin"  # remote to check out existing branches from
# WT_GIT_UI=""        # extra window in new sessions, e.g. "lazygit", "gitui", "tig"
# WT_DEV_SCRIPT="dev" # package.json script for the "dev" window; "" disables
# WT_PORT_START=3000  # first dev server port tried

# Copied from the worktree you run wt in (gitignored local files):
# WT_COPY_FILES=(.env .env.local .env.development.local)

# Symlinked to the main clone, so every worktree shares one copy (default: none):
# WT_LINK_FILES=(.notes .vscode/settings.json)

# Runs in each new worktree before linking; e.g. make sure shared files exist
# in the main clone so they always get linked:
# wt_before_link() {
#   local main="$1" dir="$2"
#   mkdir -p "$main/.notes"
# }
