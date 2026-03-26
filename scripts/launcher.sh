#!/usr/bin/env bash
set -euo pipefail

# Auto-detect platform and exec the correct binary.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${SCRIPT_DIR}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "${ARCH}" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    arm64)   ARCH="arm64" ;;
esac

BINARY="${BIN_DIR}/dopplerconfig-${OS}-${ARCH}"

if [ ! -f "${BINARY}" ]; then
    echo "ERROR: Binary not found: ${BINARY}" >&2
    echo "Run 'make build-all' to compile for all platforms." >&2
    exit 1
fi

exec "${BINARY}" "$@"
