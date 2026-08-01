cask "local-action" do
  version "0.1.0"
  sha256 "065d8e4b04ad3bb15c9eee13df9969fd684ec94c61b9ee8f04be7667ed816cdb"

  url "https://github.com/adishM98/local-action/releases/download/v#{version}/local-action_#{version}_universal.dmg"
  name "local-action"
  desc "Self-hosted web UI for running GitHub Actions workflows locally via act + Docker"
  homepage "https://github.com/adishM98/local-action"

  # Unsigned/unnotarized (no Apple Developer ID yet) — Homebrew clears
  # com.apple.quarantine on cask installs automatically, unlike a DMG
  # downloaded directly in a browser, so no manual xattr step needed here.
  auto_updates false

  depends_on formula: "act"
  # Docker itself isn't a brew formula dependency — it's usually installed as
  # Docker Desktop (its own cask) or already present; local-action just needs
  # the daemon reachable, not any particular install method.

  app "local-action.app"

  zap trash: [
    "~/Library/Application Support/local-action",
  ]
end
