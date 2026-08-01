# Release process (macOS)

Plain prebuilt binaries attached to a GitHub release — no `.app` bundle, no installer. Two ways to get them: a direct `curl` download, or `brew install` via a Homebrew Formula (not Cask — there's no `.app`, just a binary) that installs the same asset.

## 1. Build the binaries

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

## 2. Tag and publish the GitHub release

```bash
git tag v0.1.0
git push origin v0.1.0
gh release create v0.1.0 build/local-action_0.1.0_darwin_* \
  --title "v0.1.0" \
  --notes "See CHANGELOG or write release notes here."
```

## 3. Update the Homebrew tap

The formula lives in `homebrew-tap/Formula/local-action.rb` in this repo, but Homebrew taps must live in their own repo named `homebrew-<name>` to be installable — there's no way around that, it's Homebrew's own naming rule. First time only:

```bash
gh repo create adishM98/homebrew-local-action --public -d "Homebrew tap for local-action"
git clone https://github.com/adishM98/homebrew-local-action.git /tmp/homebrew-local-action
cp -r homebrew-tap/Formula /tmp/homebrew-local-action/
cd /tmp/homebrew-local-action && git add -A && git commit -m "Initial formula" && git push
```

Every release after that, update `homebrew-tap/Formula/local-action.rb`'s two `url`/`sha256` pairs (printed by the release script) to match the new version, then copy it to the tap repo the same way and push.

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

## Installing from a release (what users do)

```bash
brew install adishM98/local-action/local-action
```

or directly, no Homebrew:

```bash
curl -L -o local-action https://github.com/adishM98/local-action/releases/download/v0.1.0/local-action_0.1.0_darwin_arm64   # or _amd64 on Intel
chmod +x local-action
./local-action
```

Either way, Docker still needs to be installed and running separately — the Homebrew formula declares `act` as a dependency and installs it automatically; the direct-download path needs `act` installed manually (`make bootstrap` from a cloned checkout covers that too).

## Data location

A downloaded binary run directly behaves exactly like `make run` from source: `-db` defaults to `local-action.db` in the current directory, and the encryption key goes to `os.UserConfigDir()/local-action/seed.key`. There's no fixed Application Support path here — that was specific to the `.app`-bundle approach, which this doesn't use.

## Not yet in scope

- Code signing / notarization — no Apple Developer ID yet. `curl` downloads (unlike Safari/Chrome) don't set the `com.apple.quarantine` xattr at all, so the direct-download path avoids Gatekeeper friction entirely — but anyone who downloads the binary via a browser instead may still hit it and need `xattr -d com.apple.quarantine <file>`. Homebrew installs aren't quarantined either way.
- Linux binaries / packaging — not built by this script; would need its own if requested.
