#!/usr/bin/env bash
# Builds plain local-action binaries for macOS (arm64 + amd64) for attaching
# directly to a GitHub release — no .app bundle, no Homebrew/tap, no
# installer. Download, chmod +x, run.
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

BUILD_DIR="$ROOT/build"

echo "==> Cleaning build/"
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

echo "==> Building frontend"
(cd web && npm install && npm run build)

echo "==> Building binaries (CGO disabled — pure Go, no toolchain needed to cross-compile)"
for arch in arm64 amd64; do
  name="local-action_${VERSION}_darwin_${arch}"
  GOOS=darwin GOARCH="$arch" CGO_ENABLED=0 go build -o "$BUILD_DIR/$name" ./cmd/local-action
  echo "    $name  ($(shasum -a 256 "$BUILD_DIR/$name" | awk '{print $1}'))"
  # Unversioned copy too — GitHub's releases/latest/download/<name> alias
  # needs a name that doesn't change release to release, or a copy-pasted
  # curl command with a literal "<version>" in it 404s (GitHub's error page
  # gets saved as if it were the binary, which then just fails to run).
  cp "$BUILD_DIR/$name" "$BUILD_DIR/local-action_darwin_${arch}"
done

echo
echo "==> Done — binaries in build/"
echo "Next: gh release create v$VERSION build/local-action_${VERSION}_darwin_* build/local-action_darwin_* --title \"v$VERSION\" --notes \"...\""
echo "See docs/RELEASE.md."
