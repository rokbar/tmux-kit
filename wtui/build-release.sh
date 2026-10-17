#!/usr/bin/env bash
# Builds wtui's release assets into <out-dir>: a static executable per OS and
# CPU named wtui_<os>_<arch>, with <tag> baked in as the version, plus
# checksums.txt in sha256sum's format. Run by .github/workflows/release.yml;
# install.sh --wtui downloads these names.
# usage: wtui/build-release.sh <tag> <out-dir>
set -euo pipefail
[ $# -eq 2 ] || { echo "usage: $0 <tag> <out-dir>" >&2; exit 1; }
tag="$1"
mkdir -p "$2"
out="$(cd "$2" && pwd)"
cd "$(dirname "$0")"

assets=()
for os in darwin linux; do
  for arch in arm64 amd64; do
    asset="wtui_${os}_${arch}"
    echo "building $asset ($tag)"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
      go build -trimpath -ldflags "-s -w -X main.version=$tag" -o "$out/$asset" .
    assets+=("$asset")
  done
done

cd "$out"
if command -v sha256sum >/dev/null; then
  sha256sum "${assets[@]}" >checksums.txt
else
  shasum -a 256 "${assets[@]}" >checksums.txt
fi
cat checksums.txt
