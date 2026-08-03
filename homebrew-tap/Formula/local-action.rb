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
      url "https://github.com/adishM98/local-action/releases/download/v0.8.0/local-action_0.8.0_darwin_arm64"
      sha256 "d4b3a74c5b997b7e5dd600f7c6a81e383b909c824c7f80b42d39873d18bf057b"
    end
    on_intel do
      url "https://github.com/adishM98/local-action/releases/download/v0.8.0/local-action_0.8.0_darwin_amd64"
      sha256 "8a6266d19c22502e6f486ab24720b611974eb892278414af591da4f79c576d73"
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
