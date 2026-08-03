#!/usr/bin/env bash
# Builds LocalAction.app (arm64 only — see docs/RELEASE.md) and wraps it in
# a drag-to-install DMG. Separate from scripts/release-macos.sh, which
# builds the plain CLI binaries for curl/Homebrew — this is purely additive,
# the CLI release path is untouched.
#
# Usage: scripts/package-macos-app.sh <version>   e.g. scripts/package-macos-app.sh 0.5.0
set -euo pipefail

VERSION="${1:-}"
if [[ -z "$VERSION" ]]; then
  echo "usage: $0 <version>   e.g. $0 0.5.0" >&2
  exit 1
fi
if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "version must be semver (X.Y.Z), got: $VERSION" >&2
  exit 1
fi

# act release to bundle — pinned, not "latest", so the app's behavior is
# reproducible across builds. Bump deliberately when there's a reason to
# (e.g. a real workflow that needs a newer act feature/fix); see
# https://github.com/nektos/act/releases for what's available.
ACT_VERSION="0.2.89"
ACT_SHA256="48ae218af96725f7635a66de2b87e1e346893b02add0f16b92f560296b2151fc"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

BUILD_DIR="$ROOT/build"
APP_DIR="$BUILD_DIR/LocalAction.app"
CONTENTS="$APP_DIR/Contents"

echo "==> Cleaning previous app bundle"
rm -rf "$APP_DIR"
mkdir -p "$CONTENTS/MacOS" "$CONTENTS/Resources"

echo "==> Building frontend"
(cd web && npm install && npm run build)

echo "==> Building local-action-gui (arm64, cgo enabled for the native webview)"
GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -ldflags "-X main.version=$VERSION" -o "$CONTENTS/MacOS/local-action-gui" ./cmd/local-action-gui

echo "==> Bundling act ${ACT_VERSION} (Contents/Resources/act-bin) — a real user hit"
echo "    'act: executable file not found in \$PATH' because they'd never installed"
echo "    act separately, which a double-clicked app shouldn't require at all."
ACT_DIR="$CONTENTS/Resources/act-bin"
rm -rf "$ACT_DIR"
mkdir -p "$ACT_DIR"
ACT_TARBALL="$BUILD_DIR/act_darwin_arm64.tar.gz"
curl -sL -o "$ACT_TARBALL" "https://github.com/nektos/act/releases/download/v${ACT_VERSION}/act_Darwin_arm64.tar.gz"
echo "${ACT_SHA256}  ${ACT_TARBALL}" | shasum -a 256 -c - >/dev/null || {
  echo "act download checksum mismatch — aborting, not bundling an unverified binary" >&2
  exit 1
}
tar -xzf "$ACT_TARBALL" -C "$ACT_DIR" act LICENSE
chmod +x "$ACT_DIR/act"
rm -f "$ACT_TARBALL"

echo "==> Generating icon.icns from assets/logo-1024.png"
# The plain logo has a transparent background — fine on a web page, but a
# transparent Dock icon just shows whatever's behind it. flatten-icon.swift
# flattens onto black, bakes in rounded corners (macOS doesn't auto-round a
# plain custom .icns for an unsigned local app — confirmed by it rendering
# as a hard square next to every other app's rounded tile), insets the
# square itself to 85% of the canvas (Spotify-style margin, not full bleed),
# and scales the logo mark up within that smaller square.
swift "$ROOT/scripts/flatten-icon.swift" "$ROOT/assets/logo-1024.png" "$BUILD_DIR/icon-flat.png" 0.85 1.3
ICONSET="$BUILD_DIR/icon.iconset"
rm -rf "$ICONSET"
mkdir -p "$ICONSET"
for size in 16 32 128 256 512; do
  sips -z "$size" "$size" "$BUILD_DIR/icon-flat.png" --out "$ICONSET/icon_${size}x${size}.png" >/dev/null
  double=$((size * 2))
  sips -z "$double" "$double" "$BUILD_DIR/icon-flat.png" --out "$ICONSET/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$ICONSET" -o "$CONTENTS/Resources/icon.icns"
rm -rf "$ICONSET"

echo "==> Writing Info.plist"
cat > "$CONTENTS/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>local-action</string>
  <key>CFBundleDisplayName</key><string>local-action</string>
  <key>CFBundleIdentifier</key><string>com.adishm98.local-action</string>
  <key>CFBundleVersion</key><string>${VERSION}</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>CFBundleInfoDictionaryVersion</key><string>6.0</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleExecutable</key><string>local-action-gui</string>
  <key>CFBundleIconFile</key><string>icon.icns</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>LSApplicationCategoryType</key><string>public.app-category.developer-tools</string>
  <!-- webview_go's window never registers with AppKit's window-tracking
       APIs, so macOS's "Automatic Termination" (idle apps with no visible
       windows get silently closed after a few seconds) kills it shortly
       after launch. Opt out. -->
  <key>NSSupportsAutomaticTermination</key><false/>
  <key>NSSupportsSuddenTermination</key><false/>
</dict>
</plist>
PLIST

echo "==> Assembling DMG"
DMG_STAGING="$BUILD_DIR/dmg-staging"
DMG_PATH="$BUILD_DIR/local-action_${VERSION}_darwin_arm64.dmg"
rm -rf "$DMG_STAGING" "$DMG_PATH"
mkdir -p "$DMG_STAGING"
cp -R "$APP_DIR" "$DMG_STAGING/"
ln -s /Applications "$DMG_STAGING/Applications"
hdiutil create -volname "local-action ${VERSION}" -srcfolder "$DMG_STAGING" -ov -format UDZO "$DMG_PATH" >/dev/null
rm -rf "$DMG_STAGING"

# Unversioned copy too — same reasoning as release-macos.sh: a
# releases/latest/download/ link needs a filename that doesn't change
# release to release, or a copy-pasted curl command with a literal
# "<version>" in it just downloads GitHub's error page instead of a DMG.
UNVERSIONED_DMG="$BUILD_DIR/local-action_darwin_arm64.dmg"
cp "$DMG_PATH" "$UNVERSIONED_DMG"

echo
echo "==> Done"
echo "    App:   $APP_DIR"
echo "    act:   bundled, v${ACT_VERSION} (Contents/Resources/act-bin) — no separate act install needed"
echo "    DMG:   $DMG_PATH  ($(shasum -a 256 "$DMG_PATH" | awk '{print $1}'))"
echo "           $UNVERSIONED_DMG (identical, stable filename for the 'latest' download link)"
echo
echo "No Apple Developer ID yet — this DMG is unsigned. Anyone downloading it"
echo "via a browser will hit Gatekeeper's \"unidentified developer\" warning"
echo "on first launch (right-click > Open works around it). See docs/RELEASE.md."
