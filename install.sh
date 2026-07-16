#!/bin/sh
set -e

REPO="https://github.com/0xProgress/retry.git"
TMP_DIR=$(mktemp -d)
INSTALL_DIR="${PREFIX:-/usr/local/bin}"

cleanup() {
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT

echo "Building retry from source..."

git clone --depth 1 "$REPO" "$TMP_DIR"
cd "$TMP_DIR"
make build

if [ -w "$INSTALL_DIR" ]; then
    cp retry "$INSTALL_DIR/retry"
else
    sudo cp retry "$INSTALL_DIR/retry"
fi

echo "retry installed to $INSTALL_DIR/retry"
echo "Run 'retry --help' to get started."