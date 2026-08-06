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
      url "https://github.com/adishM98/local-action/releases/download/v0.9.7/local-action_0.9.7_darwin_arm64"
      sha256 "2d2b05a52f6c596b0f397f7be34f2339624b6ee64d9ebe81b2480b511252dff0"
    end
    on_intel do
      url "https://github.com/adishM98/local-action/releases/download/v0.9.7/local-action_0.9.7_darwin_amd64"
      sha256 "edb64bba3a092b0b9321ec953fd5289f3a7940036e252a4b75cdcbcb033ceea0"
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
