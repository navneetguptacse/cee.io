#!/usr/bin/env bash
# CEE (Code Execution Engine) Installer
# Installs the precompiled 'cee' CLI directly to /usr/local/bin
# Usage: curl -fsSL https://raw.githubusercontent.com/navneetguptacse/cee.io/main/install.sh | bash

set -e

REPO="navneetguptacse/cee.io"
BINARY_NAME="cee"
INSTALL_DIR="/usr/local/bin"

# 1. Detect Operating System
OS="$(uname -s)"
case "$OS" in
  Darwin*)  PLATFORM="darwin" ;;
  Linux*)   PLATFORM="linux" ;;
  *)
    echo "Unsupported operating system: $OS"
    echo "For Windows, please download the binary manually from https://github.com/$REPO/releases"
    exit 1
    ;;
esac

# 2. Detect Architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

echo "==> Detected system: ${PLATFORM}-${ARCH}"

# 3. Determine Installation Destination
if [ ! -d "$INSTALL_DIR" ] || [ ! -w "$INSTALL_DIR" ]; then
  SUDO="sudo"
else
  SUDO=""
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

TARGET_BINARY="cee-${PLATFORM}-${ARCH}"
RELEASE_URL="https://github.com/${REPO}/releases/latest/download/${TARGET_BINARY}"

echo "==> Downloading CEE binary from GitHub..."
HTTP_CODE=$(curl -sL -w "%{http_code}" -o "${TMP_DIR}/${BINARY_NAME}" "$RELEASE_URL")

# Fallback: if release asset does not exist yet and Go is installed locally, build via 'go install'
if [ "$HTTP_CODE" != "200" ]; then
  echo "--> Prebuilt release asset not found (HTTP $HTTP_CODE)."
  if command -v go >/dev/null 2>&1; then
    echo "--> Found local Go installation. Building from source via 'go install'..."
    go install "github.com/${REPO}/cmd/cee@latest"
    GOPATH_BIN="$(go env GOPATH)/bin"
    if [ -f "${GOPATH_BIN}/${BINARY_NAME}" ]; then
      echo "==> Successfully installed to ${GOPATH_BIN}/${BINARY_NAME}"
      echo "Make sure ${GOPATH_BIN} is in your PATH."
      exit 0
    fi
  else
    echo "ERROR: Could not download prebuilt release and 'go' compiler is not installed."
    echo "Please visit https://github.com/${REPO}/releases to download the binary manually."
    exit 1
  fi
fi

chmod +x "${TMP_DIR}/${BINARY_NAME}"

echo "==> Installing ${BINARY_NAME} to ${INSTALL_DIR}..."
$SUDO install -m 755 "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"

echo "==> Successfully installed CEE CLI to ${INSTALL_DIR}/${BINARY_NAME}!"
echo ""
"${INSTALL_DIR}/${BINARY_NAME}" --help

