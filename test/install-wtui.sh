#!/usr/bin/env bash
# install.sh --wtui installs the latest release's wtui_<os>_<arch> for this
# machine as ~/.local/bin/wtui, checked against checksums.txt; it refuses an
# unsupported OS/CPU, a failed download or a bad checksum without touching the
# installed wtui. Plain install.sh never downloads wtui.
set -euo pipefail
# shellcheck source=test/lib/sandbox.sh
. "$(dirname "$0")/lib/sandbox.sh"

# Offline: install.sh skips plugins that are already there, and the config's
# TPM run (on reload) is a no-op.
plugins="$HOME/.tmux/plugins"
mkdir -p "$plugins/tpm" "$plugins/tmux-resurrect" "$plugins/tmux-continuum"
printf '#!/bin/sh\n' >"$plugins/tpm/tpm"
chmod +x "$plugins/tpm/tpm"

mkdir "$tmp/bin"
# stub curl: serves $STUB_RELEASE/<file named by the URL> to -o <path>, 404s
# anything else, and logs each URL to $STUB_CURL_LOG
export STUB_RELEASE="$tmp/release" STUB_CURL_LOG="$tmp/curl.log"
cat >"$tmp/bin/curl" <<'EOF'
#!/usr/bin/env bash
out=/dev/stdout url=
while [ $# -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift ;;
    http*) url="$1" ;;
  esac
  shift
done
echo "$url" >>"$STUB_CURL_LOG"
src="$STUB_RELEASE/${url##*/}"
[ -f "$src" ] || { echo "curl: (22) The requested URL returned error: 404" >&2; exit 22; }
cat "$src" >"$out"
EOF
# stub go: the release script's builds become scripts whose --version names
# the version baked in with -X main.version, the target and CGO_ENABLED
cat >"$tmp/bin/go" <<'EOF'
#!/usr/bin/env bash
[ "$1" = build ] || { echo "stub go: only go build" >&2; exit 1; }
out= version=
while [ $# -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift ;;
    -ldflags) version="$(printf '%s\n' "$2" | sed -n 's/.*-X main\.version=\([^ ]*\).*/\1/p')"; shift ;;
  esac
  shift
done
printf '#!/bin/sh\necho "wtui %s (%s/%s, CGO_ENABLED=%s)"\n' \
  "$version" "${GOOS:-}" "${GOARCH:-}" "${CGO_ENABLED:-}" >"$out"
chmod +x "$out"
EOF
chmod +x "$tmp/bin/curl" "$tmp/bin/go"
export PATH="$tmp/bin:$PATH"
unset WTUI_VERSION
[ "$(command -v curl)" = "$tmp/bin/curl" ] || fail "stub curl isn't first on PATH"

# the release workflow's own build, so names and checksums match a real release
release() {
  bash "$kit/wtui/build-release.sh" "$1" "$tmp/$1" >"$tmp/log" 2>&1 ||
    { cat "$tmp/log" >&2; fail "build-release.sh $1 failed"; }
}
serve() { rm -rf "$tmp/release"; cp -R "$tmp/$1" "$tmp/release"; }
release v0.0.1
release v0.0.2
[ "$(cd "$tmp/v0.0.1" && LC_ALL=C ls | tr '\n' ' ')" = \
  "checksums.txt wtui_darwin_amd64 wtui_darwin_arm64 wtui_linux_amd64 wtui_linux_arm64 " ] ||
  fail "a release isn't the four binaries plus checksums.txt: $(ls "$tmp/v0.0.1")"
# checksums.txt in sha256sum's format, so `sha256sum -c` (or shasum's) checks it
if command -v sha256sum >/dev/null; then check=(sha256sum --strict -c); else check=(shasum -a 256 --strict -c); fi
(cd "$tmp/v0.0.1" && "${check[@]}" checksums.txt >/dev/null) ||
  fail "${check[*]} can't check the release's checksums.txt: $(cat "$tmp/v0.0.1/checksums.txt")"

case "$(uname -s)/$(uname -m)" in
  Darwin/arm64) host=darwin/arm64 ;;
  Darwin/x86_64) host=darwin/amd64 ;;
  Linux/aarch64 | Linux/arm64) host=linux/arm64 ;;
  Linux/x86_64) host=linux/amd64 ;;
  *) fail "no wtui release for this test machine: $(uname -s)/$(uname -m)" ;;
esac
asset="wtui_${host/\//_}"
latest=https://github.com/rokbar/tmux-kit/releases/latest/download
wtui="$HOME/.local/bin/wtui"

run_install() { rm -f "$tmp/curl.log"; "$kit/install.sh" "$@" >"$tmp/out" 2>&1; }
installed() { "$wtui" --version; }
# whether prefix <key> opens a popup. Not grep -q: it can exit before tmux has
# written the whole table, and pipefail then fails on tmux's SIGPIPE (tmux 3.4)
popup_bound() { tmux list-keys -T prefix | tr -s ' ' | grep "^bind-key -T prefix $1 display-popup " >/dev/null; }

! run_install --wtiu || fail "install.sh accepted an unknown option"
[ ! -e "$HOME/.local/bin" ] || fail "install.sh with an unknown option installed things"

serve v0.0.1
run_install || fail "plain install.sh failed: $(cat "$tmp/out")"
[ ! -e "$tmp/curl.log" ] || fail "plain install.sh called curl: $(cat "$tmp/curl.log")"
[ ! -e "$wtui" ] || fail "plain install.sh installed wtui"

# inside tmux: the reload at the end must see the new wtui, so prefix w binds
tmux -f /dev/null new-session -d 'cat >/dev/null'
TMUX="$(tmux display -p '#{socket_path},#{pid},0')" run_install --wtui ||
  fail "install.sh --wtui failed: $(cat "$tmp/out")"
[ -x "$wtui" ] || fail "install.sh --wtui didn't install an executable $wtui"
[ "$(installed)" = "wtui v0.0.1 ($host, CGO_ENABLED=0)" ] ||
  fail "installed $(installed), want v0.0.1 for $host"
[ "$(cat "$tmp/curl.log")" = "$latest/$asset
$latest/checksums.txt" ] || fail "install.sh --wtui downloaded: $(cat "$tmp/curl.log")"
popup_bound w ||
  fail "after install.sh --wtui in tmux, prefix w isn't bound to wtui"
tmux kill-server

serve v0.0.2
run_install --wtui || fail "re-running install.sh --wtui failed: $(cat "$tmp/out")"
[ "$(installed)" = "wtui v0.0.2 ($host, CGO_ENABLED=0)" ] ||
  fail "re-run left $(installed), want v0.0.2"

# install.sh --wtui [args] fails saying <why> and keeps v0.0.2 installed, with
# nothing left behind in ~/.local/bin
refused() {
  local why="$1"
  shift
  if run_install --wtui "$@"; then fail "install.sh --wtui succeeded despite: $why"; fi
  grep -qi "$why" "$tmp/out" || fail "install.sh --wtui didn't say '$why': $(cat "$tmp/out")"
  [ "$(installed)" = "wtui v0.0.2 ($host, CGO_ENABLED=0)" ] || fail "$why: installed wtui changed"
  [ "$(cd "$HOME/.local/bin" && LC_ALL=C ls -A | tr '\n' ' ')" = "t wt wtui " ] ||
    fail "$why: ~/.local/bin holds $(ls -A "$HOME/.local/bin")"
}

serve v0.0.2
printf 'tampered' >>"$tmp/release/$asset"
refused "checksum mismatch"

serve v0.0.2
grep -v "$asset" "$tmp/v0.0.2/checksums.txt" >"$tmp/release/checksums.txt"
refused "no checksum"

serve v0.0.2
rm "$tmp/release/$asset"
refused "download failed"

serve v0.0.2
rm "$tmp/release/checksums.txt"
refused "download failed"

# no release yet: the plain install still finishes, reload included
rm "$tmp/release"/*
tmux -f /dev/null new-session -d 'cat >/dev/null'
TMUX="$(tmux display -p '#{socket_path},#{pid},0')" refused "download failed"
popup_bound W ||
  fail "a failed install.sh --wtui in tmux didn't reload the config"
tmux kill-server

# other machines: a fake uname, plus the other OS's checksum tool if missing
mkdir "$tmp/other-machine"
REAL_UNAME="$(command -v uname)"
export REAL_UNAME
cat >"$tmp/other-machine/uname" <<'EOF'
#!/bin/sh
case "$1" in
  -s) echo "${FAKE_OS:-$("$REAL_UNAME" -s)}" ;;
  -m) echo "${FAKE_CPU:-$("$REAL_UNAME" -m)}" ;;
  *) exec "$REAL_UNAME" "$@" ;;
esac
EOF
command -v shasum >/dev/null || printf '#!/bin/sh\n[ "$1" = -a ] && shift 2\nexec sha256sum "$@"\n' >"$tmp/other-machine/shasum"
command -v sha256sum >/dev/null || printf '#!/bin/sh\nexec shasum -a 256 "$@"\n' >"$tmp/other-machine/sha256sum"
chmod +x "$tmp/other-machine"/*

PATH="$tmp/other-machine:$PATH" FAKE_OS=FreeBSD refused "unsupported OS"
[ ! -e "$tmp/curl.log" ] || fail "install.sh --wtui downloaded for an unsupported OS"
PATH="$tmp/other-machine:$PATH" FAKE_CPU=i686 refused "unsupported CPU"
[ ! -e "$tmp/curl.log" ] || fail "install.sh --wtui downloaded for an unsupported CPU"

serve v0.0.2
for machine in Darwin/arm64:darwin/arm64 Darwin/x86_64:darwin/amd64 \
  Linux/aarch64:linux/arm64 Linux/x86_64:linux/amd64; do
  os="${machine%%/*}" cpu="${machine#*/}" cpu="${cpu%%:*}" target="${machine#*:}"
  PATH="$tmp/other-machine:$PATH" FAKE_OS="$os" FAKE_CPU="$cpu" run_install --wtui ||
    fail "install.sh --wtui on $os/$cpu failed: $(cat "$tmp/out")"
  [ "$(installed)" = "wtui v0.0.2 ($target, CGO_ENABLED=0)" ] ||
    fail "on $os/$cpu installed $(installed), want $target"
done

# WTUI_VERSION=<tag> installs that release (e.g. a pre-release) instead
serve v0.0.1
WTUI_VERSION=v0.0.1 run_install --wtui || fail "install.sh --wtui with WTUI_VERSION failed: $(cat "$tmp/out")"
pinned=https://github.com/rokbar/tmux-kit/releases/download/v0.0.1
[ "$(cat "$tmp/curl.log")" = "$pinned/$asset
$pinned/checksums.txt" ] || fail "with WTUI_VERSION, install.sh --wtui downloaded: $(cat "$tmp/curl.log")"
[ "$(installed)" = "wtui v0.0.1 ($host, CGO_ENABLED=0)" ] ||
  fail "with WTUI_VERSION=v0.0.1, installed $(installed)"

echo "ok: install-wtui"
