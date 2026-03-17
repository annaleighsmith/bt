#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${HOME}/.local/bin"
mkdir -p "$INSTALL_DIR"

echo "Building bt..."
go build -o "${INSTALL_DIR}/bt" .

echo "Installed bt to ${INSTALL_DIR}/bt"

if [[ ":${PATH}:" != *":${INSTALL_DIR}:"* ]]; then
  echo "Warning: ${INSTALL_DIR} is not in your PATH"
fi
