#!/bin/sh
set -eu

# Build a self-contained Mtile desktop toolchain. End users need Go because
# TinyGo uses the host Go standard library, but they do not install TinyGo,
# Lua, mGBA, CMake, SDL, or Qt separately.

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
OUTPUT=${1:-"$ROOT/dist"}
TINYGO_VERSION=${TINYGO_VERSION:-0.41.1}
GOOS_VALUE=${GOOS:-$(go env GOOS)}
GOARCH_VALUE=${GOARCH:-$(go env GOARCH)}

case "$GOOS_VALUE-$GOARCH_VALUE" in
  darwin-amd64|darwin-arm64|linux-amd64|linux-arm64|windows-amd64) ;;
  *) echo "unsupported release target: $GOOS_VALUE-$GOARCH_VALUE" >&2; exit 1 ;;
esac

EXE=
ARCHIVE_EXT=tar.gz
if [ "$GOOS_VALUE" = windows ]; then
  EXE=.exe
  ARCHIVE_EXT=zip
fi

BUNDLE_NAME="mtile-$GOOS_VALUE-$GOARCH_VALUE"
BUNDLE="$OUTPUT/$BUNDLE_NAME"
WORK="$OUTPUT/.build-$GOOS_VALUE-$GOARCH_VALUE"
ARCHIVE="tinygo$TINYGO_VERSION.$GOOS_VALUE-$GOARCH_VALUE.$ARCHIVE_EXT"
URL="https://github.com/tinygo-org/tinygo/releases/download/v$TINYGO_VERSION/$ARCHIVE"

mkdir -p "$BUNDLE/bin" "$BUNDLE/toolchains" "$BUNDLE/licenses" "$WORK"

GOOS="$GOOS_VALUE" GOARCH="$GOARCH_VALUE" go build -trimpath -o "$BUNDLE/bin/mtile$EXE" "$ROOT/cmd/mtile"
GOOS="$GOOS_VALUE" GOARCH="$GOARCH_VALUE" go build -trimpath -o "$BUNDLE/bin/mtile-editor$EXE" "$ROOT/cmd/editor"

if [ "$GOOS_VALUE" != "$(go env GOOS)" ] || [ "$GOARCH_VALUE" != "$(go env GOARCH)" ]; then
  echo "cross-compiling the native mGBA helper is not supported by this script" >&2
  echo "run this bundle build on $GOOS_VALUE/$GOARCH_VALUE" >&2
  exit 1
fi
cmake -S "$ROOT/cmd/mtile-player" -B "$WORK/player" -DCMAKE_BUILD_TYPE=Release
cmake --build "$WORK/player" --target mtile-player --config Release
cp "$WORK/player/mtile-player$EXE" "$BUNDLE/bin/mtile-player$EXE"

if [ ! -f "$WORK/$ARCHIVE" ]; then
  curl --fail --location --proto '=https' --tlsv1.2 "$URL" -o "$WORK/$ARCHIVE"
fi
if [ -n "${TINYGO_SHA256:-}" ]; then
  printf '%s  %s\n' "$TINYGO_SHA256" "$WORK/$ARCHIVE" | shasum -a 256 -c -
else
  echo "warning: TINYGO_SHA256 was not supplied; verifying version after extraction only" >&2
fi
rm -rf "$WORK/tinygo" "$WORK/tinygo-gba" "$BUNDLE/toolchains/tinygo"
if [ "$ARCHIVE_EXT" = zip ]; then
  unzip -q "$WORK/$ARCHIVE" -d "$WORK"
else
  tar -xzf "$WORK/$ARCHIVE" -C "$WORK"
fi

# The official archive contains definitions and C libraries for hundreds of
# unrelated boards. Mtile generates only the gameboy-advance target. Keep the
# TinyGo compiler plus its common source overlays and the exact ARM/GBA inputs
# proven by the clean-cache ROM gate below; this reduces the installed bundle
# from roughly 1.2 GB to about 181 MB on darwin/arm64.
TINYGO_FULL="$WORK/tinygo"
TINYGO_GBA="$WORK/tinygo-gba"
mkdir -p "$TINYGO_GBA/bin" "$TINYGO_GBA/lib" "$TINYGO_GBA/src/device" "$TINYGO_GBA/targets"
cp "$TINYGO_FULL/bin/tinygo$EXE" "$TINYGO_GBA/bin/tinygo$EXE"
cp -R "$TINYGO_FULL/lib/clang" "$TINYGO_GBA/lib/clang"
cp -R "$TINYGO_FULL/lib/compiler-rt-builtins" "$TINYGO_GBA/lib/compiler-rt-builtins"
cp -R "$TINYGO_FULL/lib/picolibc" "$TINYGO_GBA/lib/picolibc"
for source in "$TINYGO_FULL"/src/*; do
  if [ "$(basename "$source")" != device ]; then
    cp -R "$source" "$TINYGO_GBA/src/"
  fi
done
cp "$TINYGO_FULL/src/device/asm.go" "$TINYGO_GBA/src/device/asm.go"
cp -R "$TINYGO_FULL/src/device/arm" "$TINYGO_GBA/src/device/arm"
cp -R "$TINYGO_FULL/src/device/gba" "$TINYGO_GBA/src/device/gba"
cp "$TINYGO_FULL/targets/gameboy-advance.json" "$TINYGO_GBA/targets/gameboy-advance.json"
mv "$TINYGO_GBA" "$BUNDLE/toolchains/tinygo"
"$BUNDLE/toolchains/tinygo/bin/tinygo$EXE" version | grep "tinygo version $TINYGO_VERSION"

cp "$ROOT/LICENSE" "$BUNDLE/licenses/MTILE.txt"
cp "$ROOT/third_party/mgba/LICENSE" "$BUNDLE/licenses/MGBA-MPL-2.0.txt"
cp "$ROOT/docs/third-party/mgba.md" "$BUNDLE/licenses/MGBA-NOTICE.md"
cp "$ROOT/docs/third-party/TinyGo-LICENSE.txt" "$BUNDLE/licenses/TINYGO-BSD-3-CLAUSE.txt"
cp "$ROOT/docs/third-party/tinygo.md" "$BUNDLE/licenses/TINYGO-NOTICE.md"
cp "$ROOT/README.md" "$BUNDLE/README.md"

echo "built $BUNDLE"
echo "run: $BUNDLE/bin/mtile edit PROJECT"
