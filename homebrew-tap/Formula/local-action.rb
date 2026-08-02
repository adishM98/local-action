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
      url "https://github.com/adishM98/local-action/releases/download/v0.6.0/local-action_0.6.0_darwin_arm64"
      sha256 "7cf39eb9f4a39a42828a66864c63bf1d4a2e921e68f375fd803abb99958859f4"
    end
    on_intel do
      url "https://github.com/adishM98/local-action/releases/download/v0.6.0/local-action_0.6.0_darwin_amd64"
      sha256 "0e9a2eaa72ae77ae597b4fb8349c315548a57d6699a0feaebc2d55f1b1428979"
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
