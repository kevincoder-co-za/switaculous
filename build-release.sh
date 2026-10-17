#!/bin/sh
# Builds every release artifact that can be made on Linux or macOS:
# the Windows and Linux binaries, the zip, and the .deb and .rpm packages.
# The Windows MSI is built on Windows with install.ps1 or packaging/windows/build-msi.ps1.
set -e
cd "$(dirname "$0")"
VERSION="${1:-$(cat VERSION)}"

# Building a tagged commit whose VERSION file says something else would ship
# binaries that report the wrong version, so stop early instead.
tag="$(git describe --exact-match --tags 2>/dev/null || true)"
if [ -n "$tag" ] && [ "$tag" != "v$VERSION" ]; then
    echo "This commit is tagged $tag but VERSION says $VERSION. Use: go run ./tools/release <version>" >&2
    exit 1
fi
rm -rf dist && mkdir -p dist

# The Windows program icon is compiled in from switaculous.ico.
for arch in amd64 arm64; do
    go tool rsrc -ico packaging/windows/switaculous.ico -arch "$arch" -o "rsrc_windows_$arch.syso"
done

for target in "windows amd64" "windows arm64" "linux amd64" "linux arm64" "darwin arm64"; do
    set -- $target
    extension=""
    [ "$1" = "windows" ] && extension=".exe"
    GOOS=$1 GOARCH=$2 go build -trimpath -ldflags "-s -w" -o "dist/switaculous-$1-$2$extension" .
done

for arch in amd64 arm64; do
    go run ./tools/package -version "$VERSION" -arch "$arch" -binary "dist/switaculous-linux-$arch" -out dist
done

# The Windows zip holds the programs and their licenses; the source code is on GitHub.
rm -rf dist/Switaculous && mkdir -p dist/Switaculous
cp dist/switaculous-windows-amd64.exe dist/Switaculous/switaculous.exe
cp dist/switaculous-windows-arm64.exe dist/Switaculous/switaculous-arm64.exe
cp README.md LICENSE THIRD_PARTY_NOTICES.md dist/Switaculous/
(cd dist && rm -f "Switaculous-$VERSION-windows.zip" && zip -qr "Switaculous-$VERSION-windows.zip" Switaculous)
ls -la dist
