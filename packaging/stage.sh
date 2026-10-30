#!/usr/bin/env bash
# Lay out the files shared by the .deb, AppImage, and tarball.
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ ! -x bin/gio-ide ]]; then
	mkdir -p bin
	ver="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
	CGO_ENABLED=1 go build -trimpath -ldflags "-X main.version=${ver}" -o bin/gio-ide ./cmd/gio-ide
fi
bash packaging/licenses.sh THIRD_PARTY_LICENSES
python3 packaging/mkicon.py packaging/gio-ide.png
rm -rf dist/stage
mkdir -p dist/stage/usr/bin \
	dist/stage/usr/share/applications \
	dist/stage/usr/share/icons/hicolor/256x256/apps \
	dist/stage/usr/share/doc/gio-ide
cp bin/gio-ide dist/stage/usr/bin/gio-ide
cp packaging/gio-ide.desktop dist/stage/usr/share/applications/gio-ide.desktop
cp packaging/gio-ide.png dist/stage/usr/share/icons/hicolor/256x256/apps/gio-ide.png
cp THIRD_PARTY_LICENSES dist/stage/usr/share/doc/gio-ide/THIRD_PARTY_LICENSES
bash packaging/install-notices.sh dist/stage/usr/share/doc/gio-ide
echo "staged dist/stage"
