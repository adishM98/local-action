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
      sha256 "f69621f78cd337cda8ba1604e5c14f17ab69949db568b275e4b0c72645a86361"
    end
    on_intel do
      url "https://github.com/adishM98/local-action/releases/download/v0.1.0/local-action_0.1.0_darwin_amd64"
      sha256 "50a3c36d1829c600252ed57b9d4c6a0cb843401e1ee0ccd2d296bf5a215ac9ff"
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
