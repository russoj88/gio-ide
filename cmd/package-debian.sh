#!/usr/bin/env bash
# Build a Debian .deb and a standalone Linux binary. Invoke from anywhere:
#   ./cmd/package-debian.sh 0.1.0
#   ./cmd/package-debian.sh 0.1.0 dist
# CI on main passes 0.0.0. The GitHub Release tag stays v0.0.0.
set -euo pipefail

root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$root"
umask 022

if [ "$#" -lt 1 ]; then
	printf 'usage: %s <version> [outdir]\n' "$0" >&2
	exit 1
fi

ver="$1"
outdir="${2:-dist}"

case "$ver" in
	'' | */* | *' '* | *[!0-9A-Za-z.+~-]*)
		printf 'refusing version: %s\n' "$ver" >&2
		exit 1
		;;
	[!0-9]*)
		printf 'version must start with a digit: %s\n' "$ver" >&2
		exit 1
		;;
esac

if ! command -v go >/dev/null 2>&1; then
	echo "go is not on PATH" >&2
	exit 1
fi
if ! command -v dpkg-deb >/dev/null 2>&1 || ! command -v dpkg >/dev/null 2>&1; then
	echo "dpkg-deb is required to build a .deb" >&2
	exit 1
fi
if ! command -v python3 >/dev/null 2>&1; then
	echo "python3 is required to draw the icon" >&2
	exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
	echo "jq is required to collect third-party licenses" >&2
	exit 1
fi

goos="$(go env GOOS)"
if [ "$goos" != linux ]; then
	printf 'Debian packages must be built on Linux (GOOS=%s)\n' "$goos" >&2
	exit 1
fi
goarch="$(go env GOARCH)"
debarch="$(dpkg --print-architecture)"

bin_name="gio-ide_${ver}_linux_${goarch}"
deb_name="gio-ide_${ver}_${debarch}.deb"

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

echo "building ./cmd/gio-ide"
CGO_ENABLED=1 go build -trimpath -ldflags "-s -w -X main.version=${ver}" -o "$workdir/gio-ide" ./cmd/gio-ide
chmod 755 "$workdir/gio-ide"

if ldd "$workdir/gio-ide" | grep -q 'not found'; then
	echo "ldd reports missing libraries" >&2
	ldd "$workdir/gio-ide" >&2
	exit 1
fi

# ldd prints /lib and /lib64. readlink -f follows those links into /usr.
# Ubuntu records the /usr path. Debian bookworm records the /lib path.
# Query the ldd path when the canonical path has no owner.
pkgs=()
while read -r so; do
	[ -n "$so" ] || continue
	canon="$(readlink -f "$so")"
	line="$(dpkg -S "$canon" 2>/dev/null || true)"
	if [ -z "$line" ] && [ "$so" != "$canon" ]; then
		line="$(dpkg -S "$so" 2>/dev/null || true)"
	fi
	pkg="${line%%:*}"
	if [ -z "$pkg" ]; then
		printf 'no Debian package owns %s (%s)\n' "$so" "$canon" >&2
		exit 1
	fi
	IFS=', ' read -r -a parts <<<"$pkg"
	for one in "${parts[@]}"; do
		[ -n "$one" ] || continue
		pkgs+=("$one")
	done
done < <(ldd "$workdir/gio-ide" | awk '/=> \// { print $3; next } $1 ~ /^\// { print $1 }')

mapfile -t deps < <(printf '%s\n' "${pkgs[@]}" | sort -u)
if [ "${#deps[@]}" -eq 0 ]; then
	echo "ldd reported no shared libraries" >&2
	exit 1
fi
dep_csv="$(printf '%s\n' "${deps[@]}" | paste -sd, - | sed 's/,/, /g')"

python3 packaging/mkicon.py "$workdir/gio-ide.png"
bash packaging/licenses.sh "$workdir/THIRD_PARTY_LICENSES"

pkgdir="$workdir/pkg"
mkdir -p "$pkgdir/DEBIAN" \
	"$pkgdir/usr/bin" \
	"$pkgdir/usr/share/applications" \
	"$pkgdir/usr/share/icons/hicolor/256x256/apps" \
	"$pkgdir/usr/share/doc/gio-ide"

cp "$workdir/gio-ide" "$pkgdir/usr/bin/gio-ide"
cp packaging/gio-ide.desktop "$pkgdir/usr/share/applications/gio-ide.desktop"
cp "$workdir/gio-ide.png" "$pkgdir/usr/share/icons/hicolor/256x256/apps/gio-ide.png"
cp "$workdir/THIRD_PARTY_LICENSES" "$pkgdir/usr/share/doc/gio-ide/THIRD_PARTY_LICENSES"
bash packaging/install-notices.sh "$pkgdir/usr/share/doc/gio-ide"
chmod 755 "$pkgdir/usr/bin/gio-ide"
chmod 644 \
	"$pkgdir/usr/share/applications/gio-ide.desktop" \
	"$pkgdir/usr/share/icons/hicolor/256x256/apps/gio-ide.png" \
	"$pkgdir/usr/share/doc/gio-ide/"*

installed_size="$(du -sk "$pkgdir/usr" | awk '{print $1}')"

cat >"$pkgdir/DEBIAN/control" <<EOF
Package: gio-ide
Version: ${ver}
Section: devel
Priority: optional
Architecture: ${debarch}
Maintainer: Simple Systems <noreply@simplesystems.tech>
Homepage: https://github.com/russoj88/gio-ide
Installed-Size: ${installed_size}
Depends: ${dep_csv}
Suggests: golang-go
Description: Go code editor for Linux
 Gio desktop editor with in-process Go code intelligence.
EOF

mkdir -p "$outdir"
outdir="$(CDPATH= cd -- "$outdir" && pwd)"

dpkg-deb --root-owner-group --build "$pkgdir" "$outdir/$deb_name"
cp "$workdir/gio-ide" "$outdir/$bin_name"
chmod 755 "$outdir/$bin_name"

(
	cd "$outdir"
	sha256sum "$deb_name" "$bin_name" >SHA256SUMS
)

echo "wrote $outdir/$deb_name"
echo "wrote $outdir/$bin_name"
echo "wrote $outdir/SHA256SUMS"
