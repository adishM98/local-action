#!/usr/bin/env bash
# Builds a universal (arm64+amd64) local-action.app and packages it into a
# DMG — the artifact the Homebrew cask (homebrew-tap/Casks/local-action.rb)
# installs from. macOS only (uses lipo/hdiutil/iconutil).
#
# Usage: scripts/release-macos.sh <version>   e.g. scripts/release-macos.sh 0.1.0
set -euo pipefail

VERSION="${1:-}"
if [[ -z "$VERSION" ]]; then
  echo "usage: $0 <version>   e.g. $0 0.1.0" >&2
  exit 1
fi
if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "version must be semver (X.Y.Z), got: $VERSION" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

APP_NAME="local-action"
BUNDLE_ID="com.adishm.local-action"
BUILD_DIR="$ROOT/build"
APP_DIR="$BUILD_DIR/$APP_NAME.app"
DMG_NAME="${APP_NAME}_${VERSION}_universal.dmg"

echo "==> Cleaning build/"
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

echo "==> Building frontend"
(cd web && npm install && npm run build)

echo "==> Building universal binary (arm64 + amd64, CGO disabled)"
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o "$BUILD_DIR/local-action-arm64" ./cmd/local-action
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o "$BUILD_DIR/local-action-amd64" ./cmd/local-action
lipo -create -output "$BUILD_DIR/local-action-universal" "$BUILD_DIR/local-action-arm64" "$BUILD_DIR/local-action-amd64"
rm "$BUILD_DIR/local-action-arm64" "$BUILD_DIR/local-action-amd64"

echo "==> Building icon.icns from assets/logo-1024.png"
ICONSET="$BUILD_DIR/icon.iconset"
mkdir -p "$ICONSET"
for size in 16 32 64 128 256 512; do
  sips -z "$size" "$size" assets/logo-1024.png --out "$ICONSET/icon_${size}x${size}.png" >/dev/null
  double=$((size * 2))
  sips -z "$double" "$double" assets/logo-1024.png --out "$ICONSET/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$ICONSET" -o "$BUILD_DIR/icon.icns"
rm -rf "$ICONSET"

echo "==> Assembling $APP_NAME.app"
mkdir -p "$APP_DIR/Contents/MacOS" "$APP_DIR/Contents/Resources"
cp "$BUILD_DIR/local-action-universal" "$APP_DIR/Contents/MacOS/local-action-bin"
chmod +x "$APP_DIR/Contents/MacOS/local-action-bin"
cp "$BUILD_DIR/icon.icns" "$APP_DIR/Contents/Resources/icon.icns"

cat > "$APP_DIR/Contents/MacOS/local-action" <<'LAUNCHER'
#!/bin/bash
# Launcher: starts the server (if not already running) and opens the UI in
# the default browser. Runs the server in the foreground so quitting the app
# from the Dock cleanly stops it — deliberately no menu-bar/tray UI (see
# docs/RELEASE.md for why).
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DATA_DIR="$HOME/Library/Application Support/local-action"
mkdir -p "$DATA_DIR"

if command -v nc >/dev/null 2>&1 && nc -z 127.0.0.1 8090 2>/dev/null; then
  open "http://127.0.0.1:8090"
  exit 0
fi

( sleep 1; open "http://127.0.0.1:8090" ) &
exec "$DIR/local-action-bin" -db "$DATA_DIR/local-action.db" >>"$DATA_DIR/local-action.log" 2>&1
LAUNCHER
chmod +x "$APP_DIR/Contents/MacOS/local-action"

cat > "$APP_DIR/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>local-action</string>
  <key>CFBundleDisplayName</key><string>local-action</string>
  <key>CFBundleIdentifier</key><string>${BUNDLE_ID}</string>
  <key>CFBundleVersion</key><string>${VERSION}</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>CFBundleExecutable</key><string>local-action</string>
  <key>CFBundleIconFile</key><string>icon.icns</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST

echo "==> Packaging DMG"
STAGE="$BUILD_DIR/dmg-stage"
mkdir -p "$STAGE"
cp -R "$APP_DIR" "$STAGE/"
ln -s /Applications "$STAGE/Applications"
rm -f "$BUILD_DIR/$DMG_NAME"
hdiutil create -volname "$APP_NAME" -srcfolder "$STAGE" -ov -format UDZO "$BUILD_DIR/$DMG_NAME" >/dev/null
rm -rf "$STAGE" "$BUILD_DIR/local-action-universal"

SHA256="$(shasum -a 256 "$BUILD_DIR/$DMG_NAME" | awk '{print $1}')"

echo
echo "==> Done: build/$DMG_NAME"
echo "    sha256: $SHA256"
echo
echo "This DMG is only consumed by the Homebrew cask — not distributed directly."
echo "Next: see docs/RELEASE.md for tagging, GitHub release, and updating the Homebrew tap."
