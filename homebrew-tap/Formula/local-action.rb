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
      url "https://github.com/adishM98/local-action/releases/download/v0.9.4/local-action_0.9.4_darwin_arm64"
      sha256 "bf999b21805ee1bc724f8d66a843d432f5c604efb1ee1b87b3a85095ed40e9bc"
    end
    on_intel do
      url "https://github.com/adishM98/local-action/releases/download/v0.9.4/local-action_0.9.4_darwin_amd64"
      sha256 "44699331f964cb5bfad21ca1dba6237c5d6cc360114d56789df1edb88cb4c058"
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
