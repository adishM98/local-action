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
      url "https://github.com/adishM98/local-action/releases/download/v0.5.0/local-action_0.5.0_darwin_arm64"
      sha256 "8becf55bb504329dbb6531f3d731f847905b0f46f1222d0c0ef7a3a8757ba93c"
    end
    on_intel do
      url "https://github.com/adishM98/local-action/releases/download/v0.5.0/local-action_0.5.0_darwin_amd64"
      sha256 "00d10ba23fd9701fc69f6c9667048123f317ebda57c3f6bdf5cdb5443e929b3d"
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
