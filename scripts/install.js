const fs = require("fs");
const path = require("path");
const https = require("https");
const { execSync } = require("child_process");

const REPO = "navneetguptacse/cee.io";

function getPlatformArch() {
  const platformMap = {
    darwin: "darwin",
    linux: "linux",
    win32: "windows",
  };

  const archMap = {
    x64: "amd64",
    arm64: "arm64",
  };

  const platform = platformMap[process.platform];
  const arch = archMap[process.arch];

  if (!platform || !arch) {
    throw new Error(
      `Unsupported platform/architecture: ${process.platform} ${process.arch}`,
    );
  }

  const ext = process.platform === "win32" ? ".exe" : "";
  const binaryName = `cee-${platform}-${arch}${ext}`;
  return { platform, arch, ext, binaryName };
}

function downloadFile(url, dest) {
  return new Promise((resolve, reject) => {
    https
      .get(url, (res) => {
        if (
          res.statusCode >= 300 &&
          res.statusCode < 400 &&
          res.headers.location
        ) {
          return resolve(downloadFile(res.headers.location, dest));
        }
        if (res.statusCode !== 200) {
          return reject(
            new Error(`Download failed with status HTTP ${res.statusCode}`),
          );
        }

        const file = fs.createWriteStream(dest);
        res.pipe(file);
        file.on("finish", () => {
          file.close(() => resolve());
        });
      })
      .on("error", reject);
  });
}

async function main() {
  const { ext, binaryName } = getPlatformArch();
  const binDir = path.join(__dirname, "..", "bin");
  const targetBin = path.join(binDir, `cee${ext}`);

  if (!fs.existsSync(binDir)) {
    fs.mkdirSync(binDir, { recursive: true });
  }

  const releaseUrl = `https://github.com/${REPO}/releases/latest/download/${binaryName}`;

  console.log(
    `==> Installing CEE native binary for ${process.platform}-${process.arch}...`,
  );

  try {
    console.log(`==> Downloading prebuilt binary from GitHub Releases...`);
    await downloadFile(releaseUrl, targetBin);
    fs.chmodSync(targetBin, 0o755);
    console.log(`✓ Successfully installed CEE binary to ${targetBin}`);
    return;
  } catch (err) {
    console.log(`--> Prebuilt download not available (${err.message}).`);
  }

  // Fallback: build from source if 'go' compiler is installed
  try {
    const hasGo = execSync("go version", { stdio: "pipe" }).toString();
    if (hasGo) {
      console.log(`--> Found local Go compiler: ${hasGo.trim()}`);
      console.log(`--> Building cee from source via 'go build'...`);
      const repoRoot = path.join(__dirname, "..");
      execSync(`go build -ldflags="-s -w" -o "${targetBin}" ./cmd/cee`, {
        cwd: repoRoot,
        stdio: "inherit",
      });
      fs.chmodSync(targetBin, 0o755);
      console.log(
        `✓ Successfully compiled and installed CEE binary to ${targetBin}`,
      );
      return;
    }
  } catch {
    // go not installed
  }

  console.error(
    `ERROR: Could not download prebuilt binary or build from source.`,
  );
  console.error(
    `Please visit https://github.com/${REPO}/releases to download manually.`,
  );
  process.exit(1);
}

main().catch((err) => {
  console.error("Installation failed:", err.message);
  process.exit(1);
});
