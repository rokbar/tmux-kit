#!/usr/bin/env bash
# The kit's tmux.conf leaves prefix w as tmux's window list, with or without
# wtui installed: wtui is launched from a plain terminal now, not a popup.
set -euo pipefail
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "$0")/lib/sandbox.sh"

# Offline: the config ends by running TPM; here it's a no-op.
mkdir -p "$HOME/.tmux/plugins/tpm" "$HOME/.local/bin"
printf '#!/bin/sh\n' >"$HOME/.tmux/plugins/tpm/tpm"
chmod +x "$HOME/.tmux/plugins/tpm/tpm"

# prefix w as bound by a fresh private server loading <config>. Filtered from
# the whole table: tmux 3.7's `list-keys -T prefix w` prints nothing. Spaces
# squeezed, since list-keys pads its columns.
prefix_w_with() {
  tmux -f "$1" new-session -d 'cat >/dev/null'
  tmux list-keys -T prefix | tr -s ' ' | grep '^bind-key -T prefix w ' || true
  tmux kill-server
}

tmux_default="$(prefix_w_with /dev/null)"
[ -n "$tmux_default" ] || fail "tmux has no default prefix w binding to compare with"

[ "$(prefix_w_with "$kit/tmux.conf")" = "$tmux_default" ] ||
  fail "without wtui, prefix w isn't tmux's default: $(prefix_w_with "$kit/tmux.conf")"

printf '#!/bin/sh\n' >"$HOME/.local/bin/wtui"
chmod +x "$HOME/.local/bin/wtui"
[ "$(prefix_w_with "$kit/tmux.conf")" = "$tmux_default" ] ||
  fail "with wtui installed, prefix w isn't tmux's default: $(prefix_w_with "$kit/tmux.conf")"

echo "ok: tmux-wtui-binding"
