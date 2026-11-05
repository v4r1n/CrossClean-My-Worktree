#!/usr/bin/env bash
# Builds build/CleanMyWT.app (and build/wtscan). Pass --install to copy the app to /Applications.
set -euo pipefail
cd "$(dirname "$0")/.."

swift build -c release
BIN="$(swift build -c release --show-bin-path)"

[ -f Resources/AppIcon.icns ] || swift scripts/make-icon.swift Resources/AppIcon.icns

APP=build/CleanMyWT.app
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$BIN/CleanMyWT" "$APP/Contents/MacOS/"
cp Resources/Info.plist "$APP/Contents/"
cp Resources/AppIcon.icns "$APP/Contents/Resources/"
cp "$BIN/wtscan" build/wtscan
codesign --force --sign - "$APP"
echo "Built $APP and build/wtscan"

if [[ "${1:-}" == "--install" ]]; then
  rm -rf /Applications/CleanMyWT.app
  cp -R "$APP" /Applications/
  echo "Installed /Applications/CleanMyWT.app"
fi
