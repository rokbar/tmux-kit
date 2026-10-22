#!/usr/bin/env bash
# Symlink tmux-kit into $HOME and install tmux plugins. Re-run safe; existing
# files are moved to <file>.bak. Update later with: git pull
# --wtui also installs (or updates) wtui from the latest GitHub release into
# ~/.local/bin/wtui; WTUI_VERSION=<tag> picks another release.
# --claude also installs (or updates) the agent-state Claude Code plugin from
# this clone, through the claude CLI, which edits Claude Code's user settings.
set -euo pipefail
kit="$(cd "$(dirname "$0")" && pwd)"
bin="$HOME/.local/bin"

with_wtui="" with_claude=""
for arg in "$@"; do
  case "$arg" in
    --wtui) with_wtui=1 ;;
    --claude) with_claude=1 ;;
    *) echo "unknown option: $arg (usage: install.sh [--wtui] [--claude])" >&2; exit 1 ;;
  esac
done

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

# Downloads the release's wtui_<os>_<arch> and checksums.txt into a temp folder
# next to $bin/wtui, verifies the checksum, then moves it into place, so any
# failure leaves the installed wtui as it was. Called as `install_wtui || …`,
# where set -e is off, so every step checks for failure itself.
wtui_tmp=""
trap 'rm -rf "$wtui_tmp"' EXIT
install_wtui() {
  local os arch sum asset url f expected actual
  case "$(uname -s)" in
    Darwin) os=darwin sum=(shasum -a 256) ;;
    Linux) os=linux sum=(sha256sum) ;;
    *) echo "wtui: unsupported OS $(uname -s) (releases are for macOS and Linux)" >&2; return 1 ;;
  esac
  case "$(uname -m)" in
    arm64 | aarch64) arch=arm64 ;;
    x86_64 | amd64) arch=amd64 ;;
    *) echo "wtui: unsupported CPU $(uname -m) (releases are for arm64 and x86_64)" >&2; return 1 ;;
  esac
  for f in curl "${sum[0]}"; do
    command -v "$f" >/dev/null || { echo "wtui: needs $f" >&2; return 1; }
  done
  asset="wtui_${os}_${arch}"
  url="https://github.com/rokbar/tmux-kit/releases"
  if [ -n "${WTUI_VERSION:-}" ]; then url+="/download/$WTUI_VERSION"; else url+="/latest/download"; fi

  wtui_tmp="$(mktemp -d "$bin/.wtui.XXXXXX")" || return 1
  for f in "$asset" checksums.txt; do
    curl -fsSL -o "$wtui_tmp/$f" "$url/$f" ||
      { echo "wtui: download failed: $url/$f" >&2; return 1; }
  done
  expected="$(awk -v f="$asset" '$2 == f || $2 == "*" f { print $1 }' "$wtui_tmp/checksums.txt")"
  [ -n "$expected" ] || { echo "wtui: no checksum for $asset in $url/checksums.txt" >&2; return 1; }
  actual="$(cd "$wtui_tmp" && "${sum[@]}" "$asset" | awk '{ print $1 }')"
  if [ "$actual" != "$expected" ]; then
    echo "wtui: checksum mismatch for $url/$asset (expected $expected, got $actual); not installed" >&2
    return 1
  fi
  chmod +x "$wtui_tmp/$asset" && mv -f "$wtui_tmp/$asset" "$bin/wtui" || return 1
  echo "installed $bin/wtui ($("$bin/wtui" --version 2>/dev/null || echo "$asset"))"
}

# Points marketplace tmux-kit at this clone (adding it, or re-pointing it in
# place, which keeps its plugins: never marketplace remove, which uninstalls
# them), then installs agent-state@tmux-kit and updates it to the clone's
# commit. Each step is safe to re-run. Called as `install_claude || …`, like
# install_wtui, so a failing step stops it by itself.
install_claude() {
  command -v claude >/dev/null || { echo "agent-state plugin: needs claude (Claude Code) on PATH" >&2; return 1; }
  claude_plugin marketplace add "$kit" &&
    claude_plugin install agent-state@tmux-kit &&
    claude_plugin update agent-state@tmux-kit || return 1
  echo "installed the agent-state plugin; restart running Claude Code sessions to load it"
}
claude_plugin() { claude plugin "$@" || { echo "agent-state plugin: claude plugin $* failed" >&2; return 1; }; }

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
for cmd in tmux git fzf; do command -v "$cmd" >/dev/null || missing+=" $cmd"; done
if [ -n "$missing" ]; then echo "missing:$missing (brew install$missing)"; fi
case ":$PATH:" in *":$bin:"*) ;; *) echo "add $bin to PATH" ;; esac
# if either fails, the rest still runs (the reload too) and install.sh exits 1
wtui_failed=""
if [ -n "$with_wtui" ]; then install_wtui || wtui_failed=1; fi
claude_failed=""
if [ -n "$with_claude" ]; then install_claude || claude_failed=1; fi
if [ -n "${TMUX:-}" ]; then tmux source-file "$HOME/.tmux.conf" && echo "reloaded tmux config"; fi
if [ -n "$wtui_failed" ]; then echo "wtui not installed" >&2; fi
if [ -n "$claude_failed" ]; then echo "agent-state plugin not installed" >&2; fi
if [ -n "$wtui_failed$claude_failed" ]; then exit 1; fi
echo "done. optional: cp $kit/config.example.sh ~/.config/wt/config.sh"
