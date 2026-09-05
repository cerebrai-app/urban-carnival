#!/usr/bin/env bash
# Wrap a prebuilt cerebrai-desktop binary in a macOS .app bundle, and
# optionally build a .dmg installer around it.
#
# The desktop binary is built elsewhere (`make build-desktop`, or goreleaser
# in .github/workflows/release.yml) so its version ldflags stay the single
# source of truth. This script only assembles the bundle; it runs on macOS
# only because it uses sips, iconutil, hdiutil and codesign.
#
# Usage:
#   build/macos/package-app.sh --exe bin/cerebrai-desktop --version 1.2.3 [options]
#
#   --exe PATH        prebuilt cerebrai-desktop binary (required)
#   --version STRING  version string for Info.plist / dmg name (default: dev)
#   --arch STRING     arch label for the .dmg filename (default: `uname -m`)
#   --outdir DIR      where to write the bundle/installer (default: dist/macos)
#   --name STRING     app (and .app bundle) name (default: Cerebrai)
#   --dmg             also build <name>-<version>-<arch>.dmg
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

exe=
version=dev
arch=$(uname -m)
outdir=dist/macos
app_name=Cerebrai
bundle_id=app.cerebrai.desktop
make_dmg=0

while [ $# -gt 0 ]; do
	case "$1" in
	--exe) exe=$2; shift 2 ;;
	--version) version=$2; shift 2 ;;
	--arch) arch=$2; shift 2 ;;
	--outdir) outdir=$2; shift 2 ;;
	--name) app_name=$2; shift 2 ;;
	--dmg) make_dmg=1; shift ;;
	-h | --help) sed -n '2,18p' "$0"; exit 0 ;;
	*) echo "unknown argument: $1" >&2; exit 2 ;;
	esac
done

[ -n "$exe" ] || { echo "--exe is required" >&2; exit 2; }
[ -f "$exe" ] || { echo "--exe: no such file: $exe" >&2; exit 2; }

mkdir -p "$outdir"
app="$outdir/$app_name.app"
rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"

cp "$exe" "$app/Contents/MacOS/$app_name"
chmod +x "$app/Contents/MacOS/$app_name"

# icon.png -> icon.icns. sips/iconutil want a .iconset directory of the
# conventional sizes; missing sizes just render blurrier, so a full set is
# cheap insurance.
iconset=$(mktemp -d)/icon.iconset
mkdir -p "$iconset"
for size in 16 32 128 256 512; do
	sips -z "$size" "$size" "$here/icon.png" --out "$iconset/icon_${size}x${size}.png" >/dev/null
	retina=$((size * 2))
	sips -z "$retina" "$retina" "$here/icon.png" --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$iconset" -o "$app/Contents/Resources/icon.icns"
rm -rf "$(dirname "$iconset")"

# CFBundleShortVersionString must be one to three dot-separated integers, so
# drop a leading v and any -prerelease / +build / -dirty suffix. The full
# string still goes in CFBundleVersion.
short=$(printf '%s' "$version" | sed -E 's/^v//; s/[-+].*$//')
[ -n "$short" ] || short=0.0.0

sed -e "s|@APP_NAME@|$app_name|g" \
	-e "s|@BUNDLE_ID@|$bundle_id|g" \
	-e "s|@VERSION@|$version|g" \
	-e "s|@SHORT_VERSION@|$short|g" \
	"$here/Info.plist.in" >"$app/Contents/Info.plist"

printf 'APPL????' >"$app/Contents/PkgInfo"

# Ad-hoc signature. Not a substitute for a Developer ID + notarization (users
# still need to right-click > Open, or clear the quarantine xattr, on a
# download), but an arm64 bundle won't launch at all without at least this.
if command -v codesign >/dev/null; then
	codesign --force --sign - --timestamp=none "$app" >/dev/null 2>&1 || \
		echo "warning: ad-hoc codesign failed; the bundle may not launch" >&2
fi

echo "built $app"

if [ "$make_dmg" = 1 ]; then
	dmg="$outdir/${app_name}-${version}-${arch}.dmg"
	rm -f "$dmg"
	staging=$(mktemp -d)
	cp -R "$app" "$staging/"
	ln -s /Applications "$staging/Applications"
	hdiutil create -volname "$app_name $version" -srcfolder "$staging" \
		-fs HFS+ -format UDZO -ov "$dmg" >/dev/null
	rm -rf "$staging"
	echo "built $dmg"
fi
