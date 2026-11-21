#!/bin/sh
set -eu
unset CDPATH

# Build one native, relocatable desktop download with an offline ROM toolchain.

ROOT=$(cd -- "$(dirname -- "$0")/../.." && pwd)
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
OUTPUT=$(cd -- "$OUTPUT" && pwd)

EXE=
ARCHIVE_EXT=tar.gz
if [ "$GOOS_VALUE" = windows ]; then
  EXE=.exe
  ARCHIVE_EXT=zip
fi

BUNDLE_NAME="mtile-$GOOS_VALUE-$GOARCH_VALUE"
WORK="$OUTPUT/.build-$GOOS_VALUE-$GOARCH_VALUE"
ARCHIVE="tinygo$TINYGO_VERSION.$GOOS_VALUE-$GOARCH_VALUE.$ARCHIVE_EXT"
URL="https://github.com/tinygo-org/tinygo/releases/download/v$TINYGO_VERSION/$ARCHIVE"
if [ -n "${TINYGO_ARCHIVE:-}" ] && [ ! -f "$TINYGO_ARCHIVE" ]; then
  echo "TinyGo archive does not exist: $TINYGO_ARCHIVE" >&2
  exit 1
fi
TINYGO_ARCHIVE=${TINYGO_ARCHIVE:-"$WORK/$ARCHIVE"}
sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$@"
  else
    shasum -a 256 "$@"
  fi
}
mkdir -p "$WORK"
if [ ! -f "$TINYGO_ARCHIVE" ]; then
  curl --fail --location --proto '=https' --tlsv1.2 "$URL" -o "$WORK/$ARCHIVE.part"
  mv "$WORK/$ARCHIVE.part" "$WORK/$ARCHIVE"
  TINYGO_ARCHIVE="$WORK/$ARCHIVE"
fi
printf '%s  %s\n' "$TINYGO_SHA256" "$TINYGO_ARCHIVE" | sha256 -c -

# Fresh staging prevents files from a previous toolchain or build leaking into
# the release. Only publish the local archive after the offline smoke succeeds.
STAGE=$(mktemp -d "$WORK/stage.XXXXXX")
SMOKE=$(mktemp -d "${TMPDIR:-/tmp}/mtile-bundle-smoke.XXXXXX")
trap 'rm -rf "$STAGE" "$SMOKE"' 0
BUNDLE="$STAGE/$BUNDLE_NAME"
BIN="$BUNDLE/bin"
TOOLCHAINS="$BUNDLE/toolchains"
RESOURCES="$BUNDLE"
if [ "$GOOS_VALUE" = darwin ]; then
  BUNDLE="$STAGE/Mtile.app"
  BIN="$BUNDLE/Contents/MacOS"
  TOOLCHAINS="$BUNDLE/Contents/toolchains"
  RESOURCES="$BUNDLE/Contents/Resources"
fi
mkdir -p "$BIN" "$TOOLCHAINS/go" "$RESOURCES/licenses"
if [ "$GOOS_VALUE" = darwin ]; then
  # Match the minimum OS of the pinned upstream TinyGo binary on each CPU.
  MACOSX_DEPLOYMENT_TARGET=14.0
  if [ "$GOARCH_VALUE" = amd64 ]; then MACOSX_DEPLOYMENT_TARGET=15.0; fi
  export MACOSX_DEPLOYMENT_TARGET
  min_flag="-mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET"
  export CGO_CFLAGS="${CGO_CFLAGS:--O2 -g} $min_flag"
  export CGO_CXXFLAGS="${CGO_CXXFLAGS:--O2 -g} $min_flag"
  export CGO_LDFLAGS="${CGO_LDFLAGS:--O2 -g} $min_flag"
  cat > "$BUNDLE/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>Mtile</string>
  <key>CFBundleIdentifier</key><string>com.github.drpaneas.mtile</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleExecutable</key><string>mtile-editor</string>
  <key>LSMinimumSystemVersion</key><string>$MACOSX_DEPLOYMENT_TARGET</string>
  <key>NSHighResolutionCapable</key><true/>
</dict></plist>
PLIST
fi

go build -trimpath -o "$BIN/mtile$EXE" "$ROOT/cmd/mtile"
if [ "$GOOS_VALUE" = windows ]; then
  go build -trimpath -ldflags=-H=windowsgui -o "$BIN/mtile-editor$EXE" "$ROOT/cmd/editor"
else
  go build -trimpath -o "$BIN/mtile-editor$EXE" "$ROOT/cmd/editor"
fi
if [ "$GOOS_VALUE" != darwin ]; then
  cp "$BIN/mtile-editor$EXE" "$BUNDLE/Mtile$EXE"
fi

set -- -DCMAKE_BUILD_TYPE=Release
case "$GOOS_VALUE" in
  darwin) set -- "$@" "-DCMAKE_OSX_DEPLOYMENT_TARGET=$MACOSX_DEPLOYMENT_TARGET" ;;
  windows) set -- "$@" -DCMAKE_MSVC_RUNTIME_LIBRARY=MultiThreaded ;;
esac
cmake -S "$ROOT/cmd/mtile-player" -B "$WORK/player" "$@"
cmake --build "$WORK/player" --target mtile-player --config Release
PLAYER="$WORK/player/mtile-player$EXE"
if [ ! -f "$PLAYER" ]; then
  PLAYER="$WORK/player/Release/mtile-player$EXE"
fi
cp "$PLAYER" "$BIN/mtile-player$EXE"

# TinyGo uses the matching native go executable, standard library sources and
# Go tools when it loads packages. Keep this SDK relocatable with the compiler.
GO_ROOT=$(go env GOROOT)
if [ "$GOOS_VALUE" = windows ] && command -v cygpath >/dev/null 2>&1; then
  GO_ROOT=$(cygpath -u "$GO_ROOT")
fi
for entry in bin lib pkg src VERSION go.env PATENTS; do
  cp -RL "$GO_ROOT/$entry" "$TOOLCHAINS/go/"
done
# Go's downloaded toolchain module has read-only directories in the module
# cache. Normalize the private copy so staging and extracted apps are removable.
chmod -R u+w "$TOOLCHAINS/go"
# Homebrew keeps the SDK license one directory above its libexec GOROOT.
GO_LICENSE="$GO_ROOT/LICENSE"
if [ ! -f "$GO_LICENSE" ]; then GO_LICENSE="$GO_ROOT/../LICENSE"; fi
cp "$GO_LICENSE" "$TOOLCHAINS/go/LICENSE"
cp "$GO_LICENSE" "$RESOURCES/licenses/GO-BSD-3-CLAUSE.txt"

if [ "$ARCHIVE_EXT" = zip ]; then
  unzip -q "$TINYGO_ARCHIVE" -d "$STAGE"
else
  tar -xzf "$TINYGO_ARCHIVE" -C "$STAGE"
fi

# Keep the ARM/GBA target plus the common source overlays and native compiler.
TINYGO_FULL="$STAGE/tinygo"
TINYGO_GBA="$TOOLCHAINS/tinygo"
mkdir -p "$TINYGO_GBA/bin" "$TINYGO_GBA/lib" "$TINYGO_GBA/src/device" "$TINYGO_GBA/targets"
cp -R "$TINYGO_FULL/bin/." "$TINYGO_GBA/bin/"
cp -R "$TINYGO_FULL/lib/clang" "$TINYGO_GBA/lib/clang"
cp -R "$TINYGO_FULL/lib/compiler-rt-builtins" "$TINYGO_GBA/lib/compiler-rt-builtins"
cp -R "$TINYGO_FULL/lib/picolibc" "$TINYGO_GBA/lib/picolibc"
cp "$TINYGO_FULL/lib/picolibc-stdio.c" "$TINYGO_GBA/lib/"
for source in "$TINYGO_FULL"/src/*; do
  if [ "$(basename "$source")" != device ]; then
    cp -R "$source" "$TINYGO_GBA/src/"
  fi
done
cp "$TINYGO_FULL/src/device/asm.go" "$TINYGO_GBA/src/device/asm.go"
cp -R "$TINYGO_FULL/src/device/arm" "$TINYGO_GBA/src/device/arm"
cp -R "$TINYGO_FULL/src/device/gba" "$TINYGO_GBA/src/device/gba"
cp "$TINYGO_FULL/targets/gameboy-advance.s" "$TINYGO_GBA/targets/gameboy-advance.s"
cp "$TINYGO_FULL/targets/gameboy-advance.json" "$TINYGO_GBA/targets/gameboy-advance.json"
cp -R "$ROOT/scripts/release/tinygo-extra" "$TINYGO_GBA/targets/mtile"
compiler_version() {
  GOROOT="$TOOLCHAINS/go" TINYGOROOT="$TINYGO_GBA" PATH="$TOOLCHAINS/go/bin:$PATH" \
    "$TINYGO_GBA/bin/tinygo$EXE" version
}
compiler_version | grep "tinygo version $TINYGO_VERSION "

# Distribute the license and notice files of the modules actually linked on
# this OS. Some modules include third-party attributions in NOTICE.md.
go list -deps -f '{{with .Module}}{{if and .Version (not .Replace)}}{{.Path}}@{{.Version}}{{"\t"}}{{.Dir}}{{end}}{{end}}' \
  "$ROOT/cmd/mtile" "$ROOT/cmd/editor" > "$STAGE/modules"
sort -u "$STAGE/modules" > "$STAGE/modules.sorted"
while IFS="$(printf '\t')" read -r module directory; do
  [ -n "$module" ] || continue
  if [ "$GOOS_VALUE" = windows ] && command -v cygpath >/dev/null 2>&1; then
    directory=$(cygpath -u "$directory")
  fi
  notices="$RESOURCES/licenses/go-modules/$module"
  mkdir -p "$notices"
  found=
  for notice in "$directory"/LICENSE* "$directory"/LICENCE* "$directory"/COPYING* "$directory"/NOTICE*; do
    [ -f "$notice" ] || continue
    cp "$notice" "$notices/"
    found=yes
  done
  if [ -z "$found" ]; then
    echo "missing dependency license for $module" >&2
    exit 1
  fi
done < "$STAGE/modules.sorted"

cp "$ROOT/LICENSE" "$RESOURCES/licenses/MTILE.txt"
cp "$ROOT/third_party/mgba/LICENSE" "$RESOURCES/licenses/MGBA-MPL-2.0.txt"
cp "$ROOT/docs/third-party/mgba.md" "$RESOURCES/licenses/MGBA-NOTICE.md"
cp "$ROOT/cmd/mtile-player/mgba_halt_compat.cmake" "$RESOURCES/licenses/MGBA-HALT-COMPAT.cmake"
cp "$ROOT/docs/third-party/TinyGo-LICENSE.txt" "$RESOURCES/licenses/TINYGO-BSD-3-CLAUSE.txt"
cp "$ROOT/docs/third-party/tinygo.md" "$RESOURCES/licenses/TINYGO-NOTICE.md"
cp "$ROOT/runtime/maxmod/COPYING" "$RESOURCES/licenses/MAXMOD-ISC.txt"
cp "$ROOT/docs/third-party/maxmod.md" "$RESOURCES/licenses/MAXMOD-NOTICE.md"
cp "$ROOT/fonts/JetBrainsMono/OFL.txt" "$RESOURCES/licenses/JETBRAINS-MONO-OFL.txt"
cp "$ROOT/fonts/OpenDyslexic/LICENSE.txt" "$RESOURCES/licenses/OPENDYSLEXIC-LICENSE.txt"
cp "$ROOT/fonts/pico-8-wide.otf/license.txt" "$RESOURCES/licenses/PICO-8-WIDE-CC0.txt"
mkdir -p "$RESOURCES/licenses/aurora"
for name in README.md COPYING pfunc_pal_Aurora11.lua; do
  cp "$ROOT/runtime/third_party/aurora/$name" "$RESOURCES/licenses/aurora/"
done
cp "$ROOT/scripts/release/BUNDLE-README.md" "$RESOURCES/README.md"
cp "$ROOT/LICENSE" "$RESOURCES/LICENSE"

# Ship the public examples from the release revision. A developer checkout
# can also contain private projects, imported cartridge assets and built ROMs.
PROJECT_ROOT="$BUNDLE"
if [ "$GOOS_VALUE" = darwin ]; then
  # Keep editable projects beside the app, outside its code signature.
  PROJECT_ROOT="$STAGE"
  cp "$RESOURCES/README.md" "$RESOURCES/LICENSE" "$PROJECT_ROOT/"
fi
git -C "$ROOT" -c core.autocrlf=false archive --format=tar --output="$STAGE/examples.tar" HEAD -- examples
tar -xf "$STAGE/examples.tar" -C "$PROJECT_ROOT"

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
  compiler_version
  printf 'TinyGo archive SHA-256: %s\n' "$TINYGO_SHA256"
  printf 'mGBA revision: '
  git -C "$ROOT/third_party/mgba" rev-parse HEAD
} > "$RESOURCES/BUILD-INFO.txt"

# Exercise normal executable-relative discovery outside the checkout. Empty
# user caches and PATH prevent a host installation from satisfying this gate.
mkdir -p "$SMOKE/home" "$SMOKE/empty-path"
(
  cd "$SMOKE"
  unset MTILE_TINYGO GOROOT TINYGOROOT GOFLAGS
  export HOME="$SMOKE/home" USERPROFILE="$SMOKE/home" LOCALAPPDATA="$SMOKE/home/local"
  export XDG_CACHE_HOME="$SMOKE/home/cache" GOCACHE="$SMOKE/home/gocache"
  export GOPATH="$SMOKE/home/gopath" GOMODCACHE="$SMOKE/home/modcache"
  export GOENV=off GOWORK=off GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local
  export PATH="$SMOKE/empty-path"
  "$BIN/mtile$EXE" doctor
  "$BIN/mtile$EXE" new -template coin-quest game
  "$BIN/mtile$EXE" build game
  test -s game/game.gba
)
cp "$SMOKE/game/game.gba" "$RESOURCES/coin-quest.gba"

case "$GOOS_VALUE" in
  linux)
    DIST="$BUNDLE_NAME.tar.gz"
    tar -czf "$STAGE/$DIST" -C "$STAGE" "$BUNDLE_NAME"
    ;;
  windows)
    DIST="$BUNDLE_NAME.zip"
    (cd "$STAGE" && cmake -E tar cf "$DIST" --format=zip "$BUNDLE_NAME")
    ;;
  darwin)
    DIST="$BUNDLE_NAME.zip"
    # Seal the local candidate. Developer ID signing/notarization uses
    # sign-macos.sh on this same archive before public distribution.
    codesign --force --sign - "$BIN/mtile"
    codesign --force --sign - "$BIN/mtile-player"
    codesign --force --sign - "$BUNDLE"
    codesign --verify --deep --strict "$BUNDLE"
    # Store Unix permissions and symlinks for Finder and command-line unzip.
    (cd "$STAGE" && zip -qyr "$DIST" "$(basename "$BUNDLE")" examples README.md LICENSE)
    ;;
esac
(cd "$STAGE" && sha256 "$DIST" > "$DIST.sha256")
mv "$STAGE/$DIST" "$OUTPUT/$DIST"
mv "$STAGE/$DIST.sha256" "$OUTPUT/$DIST.sha256"
echo "built $OUTPUT/$DIST"
echo "checksum: $OUTPUT/$DIST.sha256"
