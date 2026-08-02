# Release process (macOS)

Two independent distribution paths, both built from the same source and both attached to the same GitHub release:

- **Plain binary** (`cmd/local-action`) — curl or Homebrew, run from a terminal. No `.app`, no window.
- **DMG app** (`cmd/local-action-gui`) — double-click, native window (embeds the same web UI via a webview, no external browser tab), Dock icon. arm64 only for now (see below).

They don't affect each other — building/publishing one never touches the other. `scripts/release.sh` runs the entire process below end to end (both binaries, the DMG, tag, GitHub release, Homebrew tap) — see [One-shot: `scripts/release.sh`](#one-shot-scriptsreleasesh) if you just want to run one command.

## 1. Build the plain binaries

```bash
make release-macos VERSION=0.1.0
```

This runs `scripts/release-macos.sh`, which builds the frontend once and cross-compiles two binaries with `CGO_ENABLED=0` (no C toolchain needed):

```
build/local-action_0.1.0_darwin_arm64
build/local-action_0.1.0_darwin_amd64
```

and prints each one's sha256.

Sanity-check before releasing:

```bash
build/local-action_0.1.0_darwin_arm64 -db /tmp/release-smoke.db &
curl -sf http://127.0.0.1:8090 >/dev/null && echo OK
kill %1
```

## 2. Build the DMG app

```bash
make package-macos-app VERSION=0.1.0
```

This runs `scripts/package-macos-app.sh`, which:

1. Builds `cmd/local-action-gui` for **arm64 only** — it needs `CGO_ENABLED=1` (links against WebKit/Cocoa for the embedded window), unlike the plain binary's pure cross-compile. Intel support is possible (Apple's SDK ships universal framework stubs, so cross-compiling cgo via `CC="clang -arch x86_64"` mostly works) but isn't verified here since it can't be tested without an actual Intel Mac — add it if you need it.
2. Generates `icon.icns` from `assets/logo-1024.png` via `scripts/flatten-icon.swift`. Two things it has to do that a naive flatten wouldn't: bake in the rounded corners (macOS does **not** auto-round a plain custom `.icns` for an unsigned/local-built app — without this it renders as a hard-cornered square next to every other app's rounded tile), and inset the black square to ~85% of the canvas with the logo scaled/centered on its actual visible content, not the source image's own (uneven) padding — full-bleed reads as oversized next to properly-templated icons like Spotify's.
3. Assembles `build/LocalAction.app` and wraps it in a drag-to-install DMG (`hdiutil`, built into macOS — no `create-dmg` dependency).

Output: `build/local-action_0.1.0_darwin_arm64.dmg`.

**Two gotchas if you touch `cmd/local-action-gui` or the icon script:**
- The Go runtime can migrate `main()`'s goroutine off the process's original OS thread, which silently breaks Cocoa (the app runs, the HTTP server answers, but no window or Dock icon ever appears — no crash, no error). `main.go` calls `runtime.LockOSThread()` in an `init()` before anything else specifically to prevent this; don't remove it.
- If you ever build this locally with a plain `go build` instead of the packaging script, double check the output is actually arm64 (`file` the binary). A Go toolchain running under Rosetta on Apple Silicon will silently default to producing an amd64 binary, which runs (registers as a foreground app, even shows briefly in the menu bar) but the window closes itself again within a few seconds.
- If you change the icon and it doesn't seem to update after reinstalling: that's the Dock's own persistent icon cache, not a build problem. Launchpad tends to pick up a changed icon on its own; the Dock is stickier. Force it:
  ```bash
  rm -rf ~/Library/Caches/com.apple.iconservices.store
  killall Dock; killall Finder
  ```

## 3. Tag and publish the GitHub release

```bash
git tag v0.1.0
git push origin v0.1.0
gh release create v0.1.0 build/local-action_* \
  --title "v0.1.0" \
  --notes "See CHANGELOG or write release notes here."
```

`build/local-action_*` picks up all six assets (versioned + unversioned binaries and DMG) as long as you built both in steps 1 and 2 — nothing else in `build/` matches that prefix.

## 4. Update the Homebrew tap

The formula lives in `homebrew-tap/Formula/local-action.rb` in this repo, but Homebrew taps must live in their own repo named `homebrew-<name>` to be installable — there's no way around that, it's Homebrew's own naming rule. First time only:

```bash
gh repo create adishM98/homebrew-local-action --public -d "Homebrew tap for local-action"
git clone https://github.com/adishM98/homebrew-local-action.git /tmp/homebrew-local-action
cp -r homebrew-tap/Formula /tmp/homebrew-local-action/
cd /tmp/homebrew-local-action && git add -A && git commit -m "Initial formula" && git push
```

Every release after that, update `homebrew-tap/Formula/local-action.rb`'s two `url`/`sha256` pairs (printed by the release script) to match the new version, then copy it to the tap repo the same way and push. The Homebrew formula only ever tracks the plain binary — it has no notion of the DMG app.

### Verify the formula before publishing

Homebrew won't audit/install a formula by raw file path — it must resolve through a real tap. Tap the local directory (needs to be a git repo) to validate before pushing anywhere:

```bash
cd homebrew-tap && git init -q && git add -A && git commit -q -m tmp
cd ..
brew tap adishm98/local-action "file://$(pwd)/homebrew-tap"
brew audit --formula adishm98/local-action/local-action
brew install --formula adishm98/local-action/local-action   # will fail to download until the release above actually exists
brew untap adishm98/local-action
rm -rf homebrew-tap/.git   # don't commit the throwaway repo into local-action itself
```

## One-shot: `scripts/release.sh`

```bash
scripts/release.sh 0.1.0        # prompts for confirmation before publishing anything
scripts/release.sh 0.1.0 --yes  # skip the prompt (CI use)
```

Runs steps 1–4 above in order (plain binaries → DMG app → tag/push → GitHub release with six assets → regenerate + validate + publish the Homebrew formula). Requires `gh` authenticated and `brew` installed; refuses to run on a dirty working tree or if the tag already exists.

Each release carries two copies of every asset: a versioned one (`local-action_0.1.0_darwin_arm64`, `..._darwin_arm64.dmg`, etc — what the Homebrew formula pins to by exact sha256) and an unversioned one (`local-action_darwin_arm64`, `local-action_darwin_arm64.dmg`). The unversioned copies exist so `releases/latest/download/<name>` links work as *stable* URLs — GitHub's "latest" alias needs an exact filename, and a versioned name would mean every curl command in the README breaks (or worse, silently downloads a 404 page instead of the binary) the moment a new version ships and someone reuses an old command, or copy-pastes a `<version>` placeholder verbatim without substituting it.

## Installing from a release (what users do)

**DMG (double-click, native window):**

```
https://github.com/adishM98/local-action/releases/latest
```

or via curl (stable link, no version to substitute):

```bash
curl -L -o local-action.dmg https://github.com/adishM98/local-action/releases/latest/download/local-action_darwin_arm64.dmg
open local-action.dmg
```

Open the DMG, drag `LocalAction.app` to Applications, launch it. Docker still needs to be installed and running separately — the app looks for `act` on `PATH` and falls back to common Homebrew install locations (`/opt/homebrew/bin`, `/usr/local/bin`) since a Finder-launched app gets a minimal `PATH` that often doesn't include it.

**Homebrew (plain binary, terminal):**

```bash
brew install adishM98/local-action/local-action
```

**Or directly, no Homebrew:**

```bash
curl -L -o local-action https://github.com/adishM98/local-action/releases/latest/download/local-action_darwin_arm64   # or _amd64 on Intel
chmod +x local-action
./local-action
```

Either way, Docker still needs to be installed and running separately — the Homebrew formula declares `act` as a dependency and installs it automatically; the direct-download path needs `act` installed manually (`make bootstrap` from a cloned checkout covers that too).

## Data location

A downloaded plain binary run directly behaves exactly like `make run` from source: `-db` defaults to `local-action.db` in the current directory, and the encryption key goes to `os.UserConfigDir()/local-action/seed.key`.

The DMG app is different — a double-clicked app has no meaningful working directory, so `local-action-gui` defaults both the database *and* the key to `~/Library/Application Support/local-action/` (`os.UserConfigDir()` on macOS). Logs go to `local-action.log` in that same directory, since a Finder-launched app has no visible console.

## Not yet in scope

- Code signing / notarization — no Apple Developer ID yet.
  - Plain binary: `curl` downloads (unlike Safari/Chrome) don't set the `com.apple.quarantine` xattr at all, so the direct-download path avoids Gatekeeper friction entirely — but anyone who downloads it via a browser instead may still hit it and need `xattr -d com.apple.quarantine <file>`. Homebrew installs aren't quarantined either way.
  - DMG: downloaded via a browser, it **will** get quarantined — first launch shows "Apple could not verify... unidentified developer." Right-click → Open once works around it.
- Intel (amd64) DMG — see step 2 above, not built by the script yet.
- Linux binaries / packaging — not built by this script; would need its own if requested.
