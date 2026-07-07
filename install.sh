#!/bin/sh
set -e

REPO="sudhir-asuracore/context-sync-mcp"
BINARY="contextsync"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux)
    PLATFORM="linux"
    ;;
  darwin)
    PLATFORM="darwin"
    ;;
  *)
    echo "Error: Unsupported OS '$OS'. Only Linux and macOS are supported by this script."
    exit 1
    ;;
esac

# Detect Architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64)
    ARCH="amd64"
    ;;
  arm64|aarch64)
    ARCH="arm64"
    ;;
  *)
    echo "Error: Unsupported architecture '$ARCH'."
    exit 1
    ;;
esac

FILENAME="${BINARY}-${PLATFORM}-${ARCH}"
URL="https://github.com/${REPO}/releases/latest/download/${FILENAME}"

echo "Installing ${BINARY} for ${PLATFORM}/${ARCH}..."
echo "Downloading from: ${URL}"

# Download to a temporary file
TMP_BIN=$(mktemp)
curl -L -f "$URL" -o "$TMP_BIN"

# Make it executable
chmod +x "$TMP_BIN"

# Determine installation directory
INSTALL_DIR="/usr/local/bin"
if [ ! -d "$INSTALL_DIR" ]; then
    INSTALL_DIR="/usr/bin"
fi

echo "Installing to ${INSTALL_DIR}/${BINARY}..."

# Move to destination (use sudo if needed)
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP_BIN" "$INSTALL_DIR/$BINARY"
else
    echo "Root privileges required to install to ${INSTALL_DIR}. Prompting for sudo..."
    sudo mv "$TMP_BIN" "$INSTALL_DIR/$BINARY"
fi

echo "Installation complete! You can now run '${BINARY}'."
