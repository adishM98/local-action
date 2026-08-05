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
      url "https://github.com/adishM98/local-action/releases/download/v0.9.3/local-action_0.9.3_darwin_arm64"
      sha256 "e7fe2d071282f2be8fa7834a911e6f885f864f5238aa6e024f02109d59107d4c"
    end
    on_intel do
      url "https://github.com/adishM98/local-action/releases/download/v0.9.3/local-action_0.9.3_darwin_amd64"
      sha256 "3a722766b94d640b61cd54fb036207645a55f97934f6a5fbbf909a4fe2b665bc"
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
