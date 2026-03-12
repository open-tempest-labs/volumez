class Volumez < Formula
  desc "FUSE filesystem that unifies multiple storage backends under a single mountpoint"
  homepage "https://github.com/open-tempest-labs/volumez"
  url "https://github.com/open-tempest-labs/volumez/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "" # Will be filled in after release
  license "Apache-2.0"
  head "https://github.com/open-tempest-labs/volumez.git", branch: "main"

  depends_on "go" => :build
  depends_on "macfuse"

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "./cmd/volumez"

    # Install example configuration
    (etc/"volumez").install "volumez.example.json"
  end

  def caveats
    <<~EOS
      volumez requires macFUSE to be installed and loaded.

      To install macFUSE:
        brew install --cask macfuse

      After installation, you may need to:
        1. Allow the macFUSE kernel extension in System Settings > Privacy & Security
        2. Restart your computer

      Example configuration installed to:
        #{etc}/volumez/volumez.example.json

      To get started:
        1. Copy the example config: cp #{etc}/volumez/volumez.example.json ~/volumez.json
        2. Edit ~/volumez.json with your backend configuration
        3. Create a mount point: mkdir /tmp/mymount
        4. Run: volumez -config ~/volumez.json -mount /tmp/mymount
    EOS
  end

  test do
    assert_match "volumez v", shell_output("#{bin}/volumez -version")
  end
end
