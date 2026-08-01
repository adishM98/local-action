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
      url "https://github.com/adishM98/local-action/releases/download/v0.2.0/local-action_0.2.0_darwin_arm64"
      sha256 "a84753a02c3f0950c3424c6bce4ba2a94e29c399f1e11eb34869217c1b00fa4e"
    end
    on_intel do
      url "https://github.com/adishM98/local-action/releases/download/v0.2.0/local-action_0.2.0_darwin_amd64"
      sha256 "70fdf290ce53f37d0ecd8192dd6a9eae269758a6f053086c513b12fba595ba4e"
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
