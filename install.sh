#!/usr/bin/env bash
# CEE (Code Execution Engine) Installer
# Installs the precompiled 'cee' CLI directly to /usr/local/bin
# Usage: curl -fsSL http://100.52.188.50/install.sh | bash

set -e

REPO="navneetguptacse/cee.io"
BINARY_NAME="cee"
INSTALL_DIR="/usr/local/bin"
CEE_SERVER="${CEE_SERVER:-http://100.52.188.50}"

# 1. Detect Operating System
OS="$(uname -s)"
case "$OS" in
  Darwin*)  PLATFORM="darwin" ;;
  Linux*)   PLATFORM="linux" ;;
  *)
    echo "Unsupported operating system: $OS"
    echo "For Windows, please download cee-windows-amd64.exe manually from ${CEE_SERVER}/download/cee-windows-amd64.exe"
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
DOWNLOAD_URL="${CEE_SERVER}/download/${TARGET_BINARY}"

echo "==> Downloading CEE binary from ${CEE_SERVER}..."
HTTP_CODE=$(curl -sL -w "%{http_code}" -o "${TMP_DIR}/${BINARY_NAME}" "$DOWNLOAD_URL")

# Fallback: if server binary not found and Go is installed locally, build via 'go install'
if [ "$HTTP_CODE" != "200" ]; then
  echo "--> Server binary not found (HTTP $HTTP_CODE)."
  if command -v go >/dev/null 2>&1; then
    echo "--> Found local Go installation. Building from source via 'go install'..."
    go install "github.com/${REPO}/cmd/cee@latest" || true
    GOPATH_BIN="$(go env GOPATH)/bin"
    if [ -f "${GOPATH_BIN}/${BINARY_NAME}" ]; then
      echo "==> Successfully installed to ${GOPATH_BIN}/${BINARY_NAME}"
      echo "Make sure ${GOPATH_BIN} is in your PATH."
      exit 0
    fi
  fi
  echo "ERROR: Could not download prebuilt release from ${DOWNLOAD_URL}."
  exit 1
fi

chmod +x "${TMP_DIR}/${BINARY_NAME}"

echo "==> Installing ${BINARY_NAME} to ${INSTALL_DIR}..."
$SUDO install -m 755 "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"

echo "==> Successfully installed CEE CLI to ${INSTALL_DIR}/${BINARY_NAME}!"
echo ""
"${INSTALL_DIR}/${BINARY_NAME}" --help

