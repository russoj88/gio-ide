#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
tool="${APPIMAGETOOL:-appimagetool}"
if ! command -v "$tool" >/dev/null && [[ ! -x /usr/local/bin/appimagetool ]]; then
	echo "appimagetool is not installed" >&2
	exit 1
fi
if [[ -x /usr/local/bin/appimagetool ]]; then
	tool=/usr/local/bin/appimagetool
fi
bash packaging/stage.sh
ver="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
ver="${ver//\//-}"
rm -rf dist/AppDir
mkdir -p dist/AppDir
cp -a dist/stage/usr dist/AppDir/usr
cp packaging/gio-ide.desktop dist/AppDir/gio-ide.desktop
cp packaging/gio-ide.png dist/AppDir/gio-ide.png
cat >dist/AppDir/AppRun <<'EOF'
#!/bin/sh
here="$(dirname "$(readlink -f "$0")")"
exec "$here/usr/bin/gio-ide" "$@"
EOF
chmod +x dist/AppDir/AppRun dist/AppDir/usr/bin/gio-ide
mkdir -p dist
ARCH=x86_64 "$tool" dist/AppDir "dist/gio-ide_${ver}_amd64.AppImage"
echo "wrote dist/gio-ide_${ver}_amd64.AppImage"
