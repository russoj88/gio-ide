#!/usr/bin/env bash
# Copy license notices into a directory.
# The Go tools license is not the gio-ide license.
# A root LICENSE file, when present, is copied as LICENSE.
set -euo pipefail
cd "$(dirname "$0")/.."
dest="${1:?dest directory}"
mkdir -p "$dest"

copy() {
	local src="$1" name="$2"
	cp "$src" "$dest/$name"
	chmod 644 "$dest/$name"
}

if [[ -f LICENSE ]]; then
	copy LICENSE LICENSE
fi
copy internal/gopls/LICENSE golang-x-tools-LICENSE
copy internal/gopls/PATENTS golang-x-tools-PATENTS
copy internal/gopls/NOTICE.md golang-x-tools-NOTICE.md
copy internal/thirdparty/gofumpt/LICENSE gofumpt-LICENSE
copy internal/thirdparty/gofumpt/LICENSE.google gofumpt-LICENSE.google
copy internal/thirdparty/goimports/LICENSE goimports-LICENSE
