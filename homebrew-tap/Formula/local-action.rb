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
      url "https://github.com/adishM98/local-action/releases/download/v0.9.0/local-action_0.9.0_darwin_arm64"
      sha256 "defc05e181af48192fcf32f50f8f2cc86ab41494f884ce4f29f3a6955ba5fad4"
    end
    on_intel do
      url "https://github.com/adishM98/local-action/releases/download/v0.9.0/local-action_0.9.0_darwin_amd64"
      sha256 "93e525bcc5931c7f172f45441aab782202b4ec54262603c1e6502304decb4bf2"
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
