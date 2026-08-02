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
      url "https://github.com/adishM98/local-action/releases/download/v0.7.0/local-action_0.7.0_darwin_arm64"
      sha256 "b3ff8b71fdafd92a238ee11f6df52a15658c69d234cef89b4f2de24e1d9a50ec"
    end
    on_intel do
      url "https://github.com/adishM98/local-action/releases/download/v0.7.0/local-action_0.7.0_darwin_amd64"
      sha256 "a780ba2c73cec971961f4c06da4a7a966dde8d692b323ef016c9e5cd907f698d"
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
