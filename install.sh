#!/usr/bin/env bash
# Symlink tmux-kit into $HOME and install tmux plugins. Re-run safe; existing
# files are moved to <file>.bak. Update later with: git pull
set -euo pipefail
kit="$(cd "$(dirname "$0")" && pwd)"
bin="$HOME/.local/bin"

link() {
  local src="$1" dst="$2"
  if [ -L "$dst" ] && [ "$(readlink "$dst")" = "$src" ]; then return; fi
  if [ -e "$dst" ] || [ -L "$dst" ]; then
    mv "$dst" "$dst.bak"
    echo "backed up $dst -> $dst.bak"
  fi
  ln -s "$src" "$dst"
  echo "linked $dst"
}

mkdir -p "$bin"
link "$kit/tmux.conf" "$HOME/.tmux.conf"
link "$kit/bin/wt" "$bin/wt"
link "$kit/bin/t" "$bin/t"

# clone directly so no prefix+I is needed; TPM manages them after
plugins="$HOME/.tmux/plugins"
mkdir -p "$plugins"
for p in tpm tmux-resurrect tmux-continuum; do
  if [ ! -d "$plugins/$p" ]; then git clone -q "https://github.com/tmux-plugins/$p.git" "$plugins/$p"; fi
done

missing=""
for cmd in tmux git fzf lazygit; do command -v "$cmd" >/dev/null || missing+=" $cmd"; done
if [ -n "$missing" ]; then echo "missing:$missing (brew install$missing)"; fi
case ":$PATH:" in *":$bin:"*) ;; *) echo "add $bin to PATH" ;; esac
if [ -n "${TMUX:-}" ]; then tmux source-file "$HOME/.tmux.conf" && echo "reloaded tmux config"; fi
echo "done. optional: cp $kit/config.example.sh ~/.config/wt/config.sh"
