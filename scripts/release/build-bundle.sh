#!/bin/sh
set -eu

# Build a self-contained Mtile desktop toolchain. End users need Go because
# TinyGo uses the host Go standard library, but they do not install TinyGo,
# Lua, mGBA, CMake, SDL, or Qt separately.

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
OUTPUT=${1:-"$ROOT/dist"}
TINYGO_VERSION=${TINYGO_VERSION:-0.42.0}
TINYGO_SHA256=${TINYGO_SHA256:-}
GOOS_VALUE=${GOOS:-$(go env GOOS)}
GOARCH_VALUE=${GOARCH:-$(go env GOARCH)}

case "$GOOS_VALUE-$GOARCH_VALUE" in
  darwin-amd64|darwin-arm64|linux-amd64|linux-arm64|windows-amd64) ;;
  *) echo "unsupported release target: $GOOS_VALUE-$GOARCH_VALUE" >&2; exit 1 ;;
esac

# Fail before building or downloading anything for an unsupported host.
if [ "$GOOS_VALUE" != "$(go env GOHOSTOS)" ] || [ "$GOARCH_VALUE" != "$(go env GOHOSTARCH)" ]; then
  echo "cross-compiling the native mGBA helper is not supported by this script" >&2
  echo "run this bundle build on $GOOS_VALUE/$GOARCH_VALUE" >&2
  exit 1
fi

if [ "${#TINYGO_SHA256}" -ne 64 ]; then
  echo "set TINYGO_SHA256 to the official archive's 64-character SHA-256 digest" >&2
  exit 1
fi
case "$TINYGO_SHA256" in
  *[!0-9a-fA-F]*) echo "TINYGO_SHA256 must be hexadecimal" >&2; exit 1 ;;
esac

mkdir -p "$OUTPUT"
OUTPUT=$(CDPATH= cd -- "$OUTPUT" && pwd)

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

cmake -S "$ROOT/cmd/mtile-player" -B "$WORK/player" -DCMAKE_BUILD_TYPE=Release
cmake --build "$WORK/player" --target mtile-player --config Release
PLAYER="$WORK/player/mtile-player$EXE"
if [ ! -f "$PLAYER" ]; then
  PLAYER="$WORK/player/Release/mtile-player$EXE"
fi
cp "$PLAYER" "$BUNDLE/bin/mtile-player$EXE"

if [ ! -f "$WORK/$ARCHIVE" ]; then
  curl --fail --location --proto '=https' --tlsv1.2 "$URL" -o "$WORK/$ARCHIVE"
fi
printf '%s  %s\n' "$TINYGO_SHA256" "$WORK/$ARCHIVE" | shasum -a 256 -c -
rm -rf "$WORK/tinygo" "$WORK/tinygo-gba" "$BUNDLE/toolchains/tinygo"
if [ "$ARCHIVE_EXT" = zip ]; then
  unzip -q "$WORK/$ARCHIVE" -d "$WORK"
else
  tar -xzf "$WORK/$ARCHIVE" -C "$WORK"
fi

# The official archive contains definitions and C libraries for hundreds of
# unrelated boards. Mtile generates only the gameboy-advance target. Keep the
# TinyGo compiler plus its common source overlays and the exact ARM/GBA inputs
# exercised by the ROM smoke build below; this reduces the installed bundle
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
cp "$ROOT/runtime/maxmod/COPYING" "$BUNDLE/licenses/MAXMOD-ISC.txt"
cp "$ROOT/docs/third-party/maxmod.md" "$BUNDLE/licenses/MAXMOD-NOTICE.md"
cp "$ROOT/fonts/JetBrainsMono/OFL.txt" "$BUNDLE/licenses/JETBRAINS-MONO-OFL.txt"
cp "$ROOT/fonts/OpenDyslexic/LICENSE.txt" "$BUNDLE/licenses/OPENDYSLEXIC-LICENSE.txt"
cp "$ROOT/fonts/pico-8-wide.otf/license.txt" "$BUNDLE/licenses/PICO-8-WIDE-CC0.txt"
cp "$ROOT/scripts/release/BUNDLE-README.md" "$BUNDLE/README.md"
cp "$ROOT/LICENSE" "$BUNDLE/LICENSE"

{
  printf 'Mtile revision: '
  git -C "$ROOT" rev-parse HEAD
  if [ -n "$(git -C "$ROOT" status --porcelain --untracked-files=normal)" ]; then
    echo 'Working tree: modified (development bundle)'
  else
    echo 'Working tree: clean'
  fi
  printf 'Target: %s/%s\n' "$GOOS_VALUE" "$GOARCH_VALUE"
  go version
  "$BUNDLE/toolchains/tinygo/bin/tinygo$EXE" version
  printf 'TinyGo archive SHA-256: %s\n' "$TINYGO_SHA256"
  printf 'mGBA revision: '
  git -C "$ROOT/third_party/mgba" rev-parse HEAD
} > "$BUNDLE/BUILD-INFO.txt"

# Build outside the checkout so -trimpath binaries use their embedded runtime.
# Explicit compiler/root paths prevent a system TinyGo from hiding bundle gaps.
# This is a smoke gate; release sign-off must still use a clean user cache.
SMOKE=$(mktemp -d "${TMPDIR:-/tmp}/mtile-bundle-smoke.XXXXXX")
trap 'rm -rf "$SMOKE"' 0
(
  cd "$SMOKE"
  "$BUNDLE/bin/mtile$EXE" new game
  TINYGOROOT="$BUNDLE/toolchains/tinygo" "$BUNDLE/bin/mtile$EXE" build \
    -tinygo "$BUNDLE/toolchains/tinygo/bin/tinygo$EXE" game
  test -s game/game.gba
)

echo "built $BUNDLE"
echo "start with $BUNDLE/README.md"
echo "run: $BUNDLE/bin/mtile edit PROJECT"
