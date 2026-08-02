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
      url "https://github.com/adishM98/local-action/releases/download/v0.4.0/local-action_0.4.0_darwin_arm64"
      sha256 "8379647a44cb87f23db63a921f4f24f5ab1e443c5f1f045faba538dac5f9bec9"
    end
    on_intel do
      url "https://github.com/adishM98/local-action/releases/download/v0.4.0/local-action_0.4.0_darwin_amd64"
      sha256 "79d86df7f93fa75c9de061add0e55309fae84b52bedbbcc3c4cde9a60e1e8f06"
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
