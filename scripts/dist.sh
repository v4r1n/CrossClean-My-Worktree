#!/usr/bin/env bash
# Cross-compiles cleanmywt and wtscan for every supported platform into dist/,
# one zip per platform, plus SHA256SUMS.txt.
#
#   scripts/dist.sh [version]      e.g. scripts/dist.sh v1.2.0
set -euo pipefail

version="${1:-dev}"
version="${version#v}"
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
rm -rf dist
mkdir -p dist

targets=(
  windows/amd64 windows/arm64
  darwin/arm64 darwin/amd64
  linux/amd64 linux/arm64
)

for target in "${targets[@]}"; do
  goos="${target%/*}"
  goarch="${target#*/}"
  ext=""
  [[ "$goos" == windows ]] && ext=".exe"
  name="CleanMyWT-${version}-${goos}-${goarch}"
  stage="dist/${name}"
  mkdir -p "$stage"
  for cmd in cleanmywt wtscan; do
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
      go build -trimpath -ldflags "-s -w" -o "${stage}/${cmd}${ext}" "./cmd/${cmd}"
  done
  cp README.md LICENSE "$stage/"
  (cd dist && zip -qr "${name}.zip" "$name")
  rm -rf "$stage"
  echo "built dist/${name}.zip"
done

(cd dist && if command -v sha256sum >/dev/null; then sha256sum ./*.zip; else shasum -a 256 ./*.zip; fi > SHA256SUMS.txt)
