class LocalAction < Formula
  desc "Self-hosted web UI for running GitHub Actions workflows locally via act + Docker"
  homepage "https://github.com/adishM98/local-action"
  license "MIT"

  # Plain prebuilt binary, not built from source here — same asset the
  # curl-based install in the README uses (see docs/RELEASE.md). No .app
  # bundle, so this is a Formula, not a Cask.
  depends_on "act"

  on_macos do
    on_arm do
      url "https://github.com/adishM98/local-action/releases/download/v0.1.0/local-action_0.1.0_darwin_arm64"
      sha256 "539caf2c09144a0ad1065011d5063e0f00f04a74ab3ef755c4c9944d9c9f6248"
    end
    on_intel do
      url "https://github.com/adishM98/local-action/releases/download/v0.1.0/local-action_0.1.0_darwin_amd64"
      sha256 "44f59f4f8a3cf16e29eb12c1d61fde1323b8f4bf6f3e59453b0bf0640dd10ee1"
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
