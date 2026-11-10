#!/usr/bin/env bash
# Builds build/CleanMyWT.app and build/wtscan.
#
#   --universal      build for both Apple silicon and Intel
#   --version X.Y.Z  stamp the version into Info.plist
#   --zip            also write release archives to dist/
#   --install        copy the app to /Applications
set -euo pipefail
cd "$(dirname "$0")/.."

UNIVERSAL=0 ZIP=0 INSTALL=0 VERSION=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --universal) UNIVERSAL=1 ;;
    --zip) ZIP=1 ;;
    --install) INSTALL=1 ;;
    --version) VERSION="${2#v}"; shift ;;
    *) echo "unknown option: $1" >&2; exit 1 ;;
  esac
  shift
done

# Builds each architecture separately and merges them with lipo; this works with
# just the Command Line Tools (`swift build --arch a --arch b` needs full Xcode).
if [[ $UNIVERSAL == 1 ]]; then
  ARCHS=(arm64 x86_64)
else
  ARCHS=("$(uname -m)")
fi
OUT=build/bin
rm -rf "$OUT" && mkdir -p "$OUT"
for product in CleanMyWT wtscan; do
  slices=()
  for arch in "${ARCHS[@]}"; do
    triple="$arch-apple-macosx14.0"
    swift build -c release --triple "$triple" --product "$product"
    slices+=("$(swift build -c release --triple "$triple" --show-bin-path)/$product")
  done
  lipo -create "${slices[@]}" -output "$OUT/$product"
done

[ -f Resources/AppIcon.icns ] || swift scripts/make-icon.swift Resources/AppIcon.icns

APP=build/CleanMyWT.app
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$OUT/CleanMyWT" "$APP/Contents/MacOS/"
cp Resources/Info.plist "$APP/Contents/"
cp Resources/AppIcon.icns "$APP/Contents/Resources/"
if [[ -n "$VERSION" ]]; then
  plutil -replace CFBundleShortVersionString -string "$VERSION" "$APP/Contents/Info.plist"
  plutil -replace CFBundleVersion -string "$VERSION" "$APP/Contents/Info.plist"
fi
cp "$OUT/wtscan" build/wtscan
codesign --force --sign - "$APP"
codesign --force --sign - build/wtscan
echo "Built $APP and build/wtscan ($(lipo -archs build/wtscan))"

if [[ $ZIP == 1 ]]; then
  SUFFIX="${VERSION:+-$VERSION}"
  rm -rf dist && mkdir -p dist
  ditto -c -k --keepParent "$APP" "dist/CleanMyWT$SUFFIX.zip"
  ditto -c -k build/wtscan "dist/wtscan$SUFFIX.zip"
  (cd dist && shasum -a 256 ./*.zip > SHA256SUMS.txt)
  echo "Wrote dist/: $(ls dist | tr '\n' ' ')"
fi

if [[ $INSTALL == 1 ]]; then
  rm -rf /Applications/CleanMyWT.app
  cp -R "$APP" /Applications/
  echo "Installed /Applications/CleanMyWT.app"
fi
