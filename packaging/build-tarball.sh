#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
bash packaging/stage.sh
ver="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
ver="${ver//\//-}"
mkdir -p dist
tar -C dist/stage -czf "dist/gio-ide_${ver}_linux_amd64.tar.gz" .
echo "wrote dist/gio-ide_${ver}_linux_amd64.tar.gz"
