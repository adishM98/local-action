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
      url "https://github.com/adishM98/local-action/releases/download/v0.9.5/local-action_0.9.5_darwin_arm64"
      sha256 "b050951f4b1f6e7765750a6bf7ee01701140942687bb26e472c7827b36fac144"
    end
    on_intel do
      url "https://github.com/adishM98/local-action/releases/download/v0.9.5/local-action_0.9.5_darwin_amd64"
      sha256 "9ef6382ad67367ae63a4127364eb9b8aa0629a9c68bc1be8086b9dee61220b63"
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
