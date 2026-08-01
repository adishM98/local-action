#!/usr/bin/env bash
# End-to-end macOS release: build binaries, tag + push, publish the GitHub
# release, regenerate the Homebrew formula, and push it to the separate
# homebrew-local-action tap repo.
#
# Usage: scripts/release.sh <version> [--yes]
#   --yes   skip the confirmation prompt (for CI; interactive use should
#           leave it off so a mistyped version doesn't silently publish)
set -euo pipefail

VERSION="${1:-}"
YES=""
[[ "${2:-}" == "--yes" ]] && YES=1

if [[ -z "$VERSION" ]]; then
  echo "usage: $0 <version> [--yes]   e.g. $0 0.1.0" >&2
  exit 1
fi
if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "version must be semver (X.Y.Z), got: $VERSION" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

GH_REPO="adishM98/local-action"
TAP_REPO="adishM98/homebrew-local-action"
TAP_DIR="$ROOT/../homebrew-local-action-tap"
TAG="v$VERSION"

echo "==> Preflight checks"
command -v gh >/dev/null || { echo "gh CLI not found — required for release + tap repo" >&2; exit 1; }
command -v brew >/dev/null || { echo "brew not found — required to validate the formula" >&2; exit 1; }
gh auth status >/dev/null 2>&1 || { echo "gh not authenticated — run 'gh auth login'" >&2; exit 1; }

if [[ -n "$(git status --porcelain)" ]]; then
  echo "Working tree not clean — commit or stash first:" >&2
  git status --short >&2
  exit 1
fi
CURRENT_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
if [[ "$CURRENT_BRANCH" != "main" ]]; then
  echo "Warning: on branch '$CURRENT_BRANCH', not 'main'."
fi
if git rev-parse "$TAG" >/dev/null 2>&1; then
  echo "Tag $TAG already exists — bump the version or delete it first." >&2
  exit 1
fi

echo
echo "This will, in order:"
echo "  1. Build frontend + darwin/arm64 + darwin/amd64 binaries"
echo "  2. git tag $TAG, git push origin $TAG"
echo "  3. gh release create $TAG (public, on $GH_REPO)"
echo "  4. Regenerate homebrew-tap/Formula/local-action.rb for $VERSION"
echo "  5. Validate it with brew audit (local tap, no push)"
echo "  6. Push the updated formula to $TAP_REPO (creating it if it doesn't exist yet)"
echo
if [[ -z "$YES" ]]; then
  read -r -p "Proceed with releasing $TAG? [y/N] " reply
  [[ "$reply" =~ ^[Yy]$ ]] || { echo "Aborted."; exit 1; }
fi

echo
echo "==> 1. Building binaries"
"$ROOT/scripts/release-macos.sh" "$VERSION"

SHA_ARM="$(shasum -a 256 "$ROOT/build/local-action_${VERSION}_darwin_arm64" | awk '{print $1}')"
SHA_AMD64="$(shasum -a 256 "$ROOT/build/local-action_${VERSION}_darwin_amd64" | awk '{print $1}')"

echo
echo "==> 2. Tagging and pushing $TAG"
git tag "$TAG"
git push origin "$TAG"

echo
echo "==> 3. Creating GitHub release"
gh release create "$TAG" \
  "$ROOT/build/local-action_${VERSION}_darwin_arm64" \
  "$ROOT/build/local-action_${VERSION}_darwin_amd64" \
  --repo "$GH_REPO" \
  --title "$TAG" \
  --generate-notes

echo
echo "==> 4. Regenerating homebrew-tap/Formula/local-action.rb"
mkdir -p "$ROOT/homebrew-tap/Formula"
cat > "$ROOT/homebrew-tap/Formula/local-action.rb" <<FORMULA
class LocalAction < Formula
  desc "Self-hosted web UI for running GitHub Actions workflows locally via act + Docker"
  homepage "https://github.com/${GH_REPO}"
  license "MIT"

  # Plain prebuilt binary, not built from source here — same asset the
  # curl-based install in the README uses (see docs/RELEASE.md). No .app
  # bundle, so this is a Formula, not a Cask.
  depends_on "act"

  on_macos do
    on_arm do
      url "https://github.com/${GH_REPO}/releases/download/${TAG}/local-action_${VERSION}_darwin_arm64"
      sha256 "${SHA_ARM}"
    end
    on_intel do
      url "https://github.com/${GH_REPO}/releases/download/${TAG}/local-action_${VERSION}_darwin_amd64"
      sha256 "${SHA_AMD64}"
    end
  end

  def install
    bin.install Dir["local-action_*"].first => "local-action"
  end

  test do
    # -h exits via Go's flag package without starting the server.
    system "#{bin}/local-action", "-h"
  end
end
FORMULA

echo
echo "==> 5. Validating formula (local tap, not pushed anywhere)"
brew untap adishm98/local-action-validate >/dev/null 2>&1 || true
(cd "$ROOT/homebrew-tap" && rm -rf .git && git init -q && git add -A && git -c user.email=release@local -c user.name=release commit -q -m validate)
brew tap adishm98/local-action-validate "file://$ROOT/homebrew-tap"
if ! brew audit --formula adishm98/local-action-validate/local-action; then
  brew untap adishm98/local-action-validate
  rm -rf "$ROOT/homebrew-tap/.git"
  echo "Formula failed brew audit — not pushing to the tap." >&2
  exit 1
fi
brew untap adishm98/local-action-validate
rm -rf "$ROOT/homebrew-tap/.git"

echo
echo "==> 6. Publishing to $TAP_REPO"
if ! gh repo view "$TAP_REPO" >/dev/null 2>&1; then
  gh repo create "$TAP_REPO" --public -d "Homebrew tap for local-action"
fi
if [[ -d "$TAP_DIR/.git" ]]; then
  git -C "$TAP_DIR" pull --quiet
else
  gh repo clone "$TAP_REPO" "$TAP_DIR"
fi
mkdir -p "$TAP_DIR/Formula"
cp "$ROOT/homebrew-tap/Formula/local-action.rb" "$TAP_DIR/Formula/local-action.rb"
(cd "$TAP_DIR" && git add -A && git commit -q -m "local-action $TAG" && git push)

echo
echo "==> Done. Released $TAG."
echo "    brew install $TAP_REPO/local-action"
echo "    curl -L -o local-action https://github.com/${GH_REPO}/releases/download/${TAG}/local-action_${VERSION}_darwin_arm64"
