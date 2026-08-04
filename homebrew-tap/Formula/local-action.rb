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
      url "https://github.com/adishM98/local-action/releases/download/v0.9.2/local-action_0.9.2_darwin_arm64"
      sha256 "36c74fcc1ec5058d78221f59d36f89ed55705404c59470cd0cbb1bc1f72aafbd"
    end
    on_intel do
      url "https://github.com/adishM98/local-action/releases/download/v0.9.2/local-action_0.9.2_darwin_amd64"
      sha256 "2385ba35772bdc0fb971f4e1c5b66e2b46229dc3f87469d17376a714669dddec"
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
