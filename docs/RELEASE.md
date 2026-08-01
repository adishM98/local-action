# Release process (macOS)

Homebrew-only for now — no direct DMG download is offered or documented to users; the DMG this produces exists solely as the artifact the Homebrew cask installs from. Unsigned, unnotarized: Homebrew clears the quarantine bit on cask installs automatically, so this doesn't need Gatekeeper workarounds the way a browser-downloaded DMG would.

The packaged app is a **plain launcher**, not a menu-bar app: double-clicking it starts the existing server in the foreground and opens your browser to it; quitting from the Dock stops it. No systray dependency, so the build stays pure Go (no cgo) — consistent with using `modernc.org/sqlite` over a cgo-based driver elsewhere in this project.

## 1. Build the DMG

```bash
make release-macos VERSION=0.1.0
```

This runs `scripts/release-macos.sh`, which:
1. Builds the frontend and a universal (arm64+amd64) binary, `CGO_ENABLED=0`.
2. Builds `icon.icns` from `assets/logo-1024.png`.
3. Assembles `build/local-action.app` (launcher script + binary + icon + Info.plist).
4. Packages it into `build/local-action_<version>_universal.dmg` and prints its sha256.

Sanity-check the result before releasing:

```bash
open build/local-action.app                        # launches it directly, unquarantined (built locally)
curl -sf http://127.0.0.1:8090 >/dev/null && echo OK
```

## 2. Tag and publish the GitHub release

```bash
git tag v0.1.0
git push origin v0.1.0
gh release create v0.1.0 build/local-action_0.1.0_universal.dmg \
  --title "v0.1.0" \
  --notes "See CHANGELOG or write release notes here."
```

## 3. Update the Homebrew tap

The cask formula lives in `homebrew-tap/Casks/local-action.rb` in this repo, but Homebrew taps must live in their own repo named `homebrew-<name>` to be installable. First time only:

```bash
gh repo create adishM98/homebrew-local-action --public -d "Homebrew tap for local-action"
git clone https://github.com/adishM98/homebrew-local-action.git /tmp/homebrew-local-action
cp -r homebrew-tap/Casks /tmp/homebrew-local-action/
cd /tmp/homebrew-local-action && git add -A && git commit -m "Initial cask" && git push
```

Every release after that, update `homebrew-tap/Casks/local-action.rb`'s `version` and `sha256` (printed by the release script) to match, then copy it over to the tap repo the same way and push.

Users then install with:

```bash
brew install --cask adishM98/local-action/local-action
```

### Verify the cask before publishing

```bash
brew audit --cask homebrew-tap/Casks/local-action.rb
brew install --cask homebrew-tap/Casks/local-action.rb   # installs from the local file, no tap needed
```

## Data location

The packaged app stores its database, encryption key, and log under `~/Library/Application Support/local-action/` (a fixed path, unlike the source build's cwd-relative default) — see the launcher script inside the `.app` for the exact flags it passes.

## Still using `act`/Docker

The DMG only packages the local-action binary itself — it doesn't bundle `act` or Docker. The Homebrew cask declares `act` as a dependency; Docker Desktop (or equivalent) still needs to be installed and running separately.

## Not yet in scope

- A directly-downloadable DMG (no Homebrew) — deliberately not offered right now; revisit if there's demand from non-brew users.
- Code signing / notarization — no Apple Developer ID yet.
- Linux packaging (`.deb`/`.rpm`/AppImage) — not built by this script; would need its own if requested.
