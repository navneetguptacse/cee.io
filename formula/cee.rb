class Cee < Formula
  desc "CEE - High-performance Code Execution Engine CLI"
  homepage "https://github.com/navneetguptacse/cee.io"
  url "https://github.com/navneetguptacse/cee.io.git", branch: "main"
  version "1.0.0"
  license "MIT"

  depends_on "go" => :build

  def install
    system "go", "build", "-ldflags", "-s -w", "-o", bin/"cee", "./cmd/cee"
  end

  test do
    assert_match "CEE", shell_output("#{bin}/cee --help")
  end
end
