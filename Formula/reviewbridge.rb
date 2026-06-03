class Reviewbridge < Formula
  desc "Routes PR/MR review comments into the right Claude Code session"
  homepage "https://github.com/ahmedennaime/reviewbridge"
  version "0.1.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/ahmedennaime/reviewbridge/releases/download/v#{version}/reviewbridge-darwin-arm64"
      sha256 "REPLACE_WITH_SHA256_DARWIN_ARM64"
    end
    on_intel do
      url "https://github.com/ahmedennaime/reviewbridge/releases/download/v#{version}/reviewbridge-darwin-amd64"
      sha256 "REPLACE_WITH_SHA256_DARWIN_AMD64"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/ahmedennaime/reviewbridge/releases/download/v#{version}/reviewbridge-linux-arm64"
      sha256 "REPLACE_WITH_SHA256_LINUX_ARM64"
    end
    on_intel do
      url "https://github.com/ahmedennaime/reviewbridge/releases/download/v#{version}/reviewbridge-linux-amd64"
      sha256 "REPLACE_WITH_SHA256_LINUX_AMD64"
    end
  end

  def install
    bin.install Dir["reviewbridge*"][0] => "reviewbridge"
  end

  def post_install
    config_file = File.expand_path("~/.reviewbridge/config.yaml")
    system bin/"reviewbridge", "init" unless File.exist?(config_file)
  end

  test do
    assert_match "reviewbridge", shell_output("#{bin}/reviewbridge --help")
    assert_match version.to_s, shell_output("#{bin}/reviewbridge version")
  end
end
