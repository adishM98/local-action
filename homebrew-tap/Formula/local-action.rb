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
      url "https://github.com/adishM98/local-action/releases/download/v0.9.1/local-action_0.9.1_darwin_arm64"
      sha256 "c3285ece4d5beaff2f449fecc78788b98b10cebdaebd2b60f80a498b51cf4e0b"
    end
    on_intel do
      url "https://github.com/adishM98/local-action/releases/download/v0.9.1/local-action_0.9.1_darwin_amd64"
      sha256 "67d7a000630e9386aa22be8e3e85b518b985d1646d89e77ee3d8f5d9003e3f2b"
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
