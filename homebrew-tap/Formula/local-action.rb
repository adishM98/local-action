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
      url "https://github.com/adishM98/local-action/releases/download/v0.9.6/local-action_0.9.6_darwin_arm64"
      sha256 "1e5a2653916d0e1731294473573f6aa7c9e8fa245927a4e5fd5be454ea030f71"
    end
    on_intel do
      url "https://github.com/adishM98/local-action/releases/download/v0.9.6/local-action_0.9.6_darwin_amd64"
      sha256 "af2b8163d5926e158d0bd7451f0c804381ba539ce40b51ebd19c576f92f29335"
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
