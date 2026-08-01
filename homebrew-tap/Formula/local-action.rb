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
      url "https://github.com/adishM98/local-action/releases/download/v0.3.0/local-action_0.3.0_darwin_arm64"
      sha256 "6e7b47ee296bcdc9c85a2daaee33f17910320474355fd958426043f8662ffb00"
    end
    on_intel do
      url "https://github.com/adishM98/local-action/releases/download/v0.3.0/local-action_0.3.0_darwin_amd64"
      sha256 "218351669c98653a97f1aecf54ac912c83a47f4c99d8d6d789b6af953f5ccc0b"
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
