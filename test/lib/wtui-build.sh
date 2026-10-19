# shellcheck shell=bash
# Sourced by the tests that build wtui (after set -euo pipefail), through
# test/lib/wtui.sh or test/lib/host.sh, in place of test/lib/sandbox.sh, which
# it sources. Defines build_wtui and stop_wtui; on exit, stops wtui before the
# sandbox's cleanup.

# build_wtui keeps Go's caches inside the sandbox, so it writes nothing
# outside it. It takes wtui's modules from the user's module download cache
# when that has them (read-only, so offline works too), else from the user's
# GOPROXY. Both are found before the sandbox replaces HOME, and with it the
# user's go env file.
go_modcache="" go_proxy=""
if command -v go >/dev/null && go_env="$(go env GOMODCACHE GOPROXY)"; then
  { IFS= read -r go_modcache; IFS= read -r go_proxy; } <<<"$go_env"
fi
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "${BASH_SOURCE[0]}")/sandbox.sh"
# wtui finds the wt next to it (see build_wtui), as when installed
unset WT_BIN

# build_wtui: builds wtui into the sandbox's ~/.local/bin, with wt and t next
# to it, as install.sh puts them. Without Go, it skips the whole test: it says
# so and exits 0.
build_wtui() {
  if ! command -v go >/dev/null; then
    echo "skip: $(basename "$0"): go isn't on PATH, so wtui can't be built"
    exit 0
  fi
  mkdir -p "$HOME/.local/bin"
  # -modcacherw: the sandbox's module cache must be deletable
  (cd "$kit/wtui" && GOENV=off GOFLAGS=-modcacherw \
    GOPATH="$tmp/go" GOCACHE="$tmp/go/cache" GOMODCACHE="$tmp/go/mod" \
    GOPROXY="${go_modcache:+file://$go_modcache/cache/download,}${go_proxy:-https://proxy.golang.org,direct}" \
    go build -buildvcs=false -o "$HOME/.local/bin/wtui" . >"$tmp/log" 2>&1) ||
    fail "go build of wtui failed: $(cat "$tmp/log")"
  ln -s "$kit/bin/wt" "$HOME/.local/bin/wt"
  ln -s "$kit/bin/t" "$HOME/.local/bin/t"
}

# stop_wtui: kills every process run from the sandbox's ~/.local/bin, where
# build_wtui puts wtui: each wtui, and each wt or t it runs from there (a
# script's path is its command line's second word, after bash), and waits
# until they've gone. Anything of theirs left once the sandbox's folder has
# gone would find, through TMUX_TMPDIR, the user's own server in /tmp. They're
# killed outright, so none runs anything more on its way out. Zombies have
# exited already.
stop_wtui() {
  local i pids
  for ((i = 0; i < 50; i++)); do
    pids="$(ps -A -o pid= -o stat= -o args= | awk -v bin="$tmp/home/.local/bin/" '
      $2 !~ /^Z/ && (index($3, bin) == 1 || index($4, bin) == 1) { print $1 }')"
    if [ -z "$pids" ]; then return 0; fi
    # shellcheck disable=SC2086 # a word per pid
    kill -KILL $pids 2>/dev/null || true
    sleep 0.1
  done
  echo "$(basename "$0"): wtui processes left running: $pids" >&2
}
wtui_cleanup() {
  stop_wtui
  cleanup
}
trap wtui_cleanup EXIT
