class Cee < Formula
  desc "CEE - High-performance Code Execution Engine CLI"
  homepage "http://100.52.188.50"
  version "1.0.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "http://100.52.188.50/download/cee-darwin-arm64"
      sha256 "0dcd8efa1baf7b15749ec43509be2b396740bf8e697309078885eea71dde94db"
    else
      url "http://100.52.188.50/download/cee-darwin-amd64"
      sha256 "225d3d6a38e4d5dd60224496757d53e409f3c110e66b706c9734e0d63573ae09"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "http://100.52.188.50/download/cee-linux-arm64"
      sha256 "e1ff1d7479bab70a6c43a9365e4e83c22f7fd32f0b9085d688ff775bf48b3fd3"
    else
      url "http://100.52.188.50/download/cee-linux-amd64"
      sha256 "d744fefd053416ecb556b033756d8cdc33947e7cf7c7ee522a158534907d7d1d"
    end
  end

  def install
    bin_file = Dir["cee-*"].first || (OS.mac? ? (Hardware::CPU.arm? ? "cee-darwin-arm64" : "cee-darwin-amd64") : (Hardware::CPU.arm? ? "cee-linux-arm64" : "cee-linux-amd64"))
    bin.install bin_file => "cee"
  end

  test do
    assert_match "CEE", shell_output("#{bin}/cee --help")
  end
end
