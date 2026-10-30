#!/usr/bin/env bash
# Local .deb. CI calls cmd/package-debian.sh directly.
set -euo pipefail
cd "$(dirname "$0")/.."
raw="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
ver="${raw#v}"
ver="${ver//\//-}"
if [[ ! "$ver" =~ ^[0-9] ]]; then
	sha="${ver%-dirty}"
	sha="${sha//[^A-Za-z0-9.]/.}"
	ver="0.1.0+git.${sha}"
	if [[ "$raw" == *-dirty ]]; then
		ver="${ver}.dirty"
	fi
fi
exec ./cmd/package-debian.sh "$ver" dist
