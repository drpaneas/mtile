#!/bin/sh
# Sign the app inside a verified candidate and write a separate distribution ZIP.
set -eu
unset CDPATH

if [ "$#" != 3 ]; then
  echo 'usage: sign-macos.sh ARCHIVE.zip OUTPUT_DIR MAJOR.MINOR.PATCH' >&2
  exit 2
fi
: "${MTILE_MACOS_SIGN_IDENTITY:?set a Developer ID Application identity, or - for an ad hoc test}"
if ! printf '%s\n' "$3" | grep -Eq '^[0-9]+[.][0-9]+[.][0-9]+$'; then
  echo 'expected a numeric MAJOR.MINOR.PATCH bundle version' >&2
  exit 2
fi
INPUT=$(cd "$(dirname "$1")" && pwd)
NAME=$(basename "$1")
case "$NAME" in mtile-darwin-arm64.zip|mtile-darwin-amd64.zip) ;; *) echo 'expected a macOS Mtile archive' >&2; exit 2 ;; esac
mkdir -p "$2"
OUTPUT=$(cd "$2" && pwd)
if [ "$INPUT" = "$OUTPUT" ]; then
  echo 'choose a separate output directory to retain the original candidate' >&2
  exit 2
fi
(cd "$INPUT" && shasum -a 256 -c "$NAME.sha256")
WORK=$(mktemp -d "$OUTPUT/.sign.XXXXXX")
trap 'rm -rf "$WORK"' 0
PAYLOAD="$WORK/download"
mkdir "$PAYLOAD"
APP="$PAYLOAD/Mtile.app"
ditto -x -k "$INPUT/$NAME" "$PAYLOAD"
plutil -replace CFBundleShortVersionString -string "$3" "$APP/Contents/Info.plist"
plutil -replace CFBundleVersion -string "$3" "$APP/Contents/Info.plist"

sign() {
  if [ "$MTILE_MACOS_SIGN_IDENTITY" = '-' ]; then
    codesign --force --sign - --options runtime --timestamp=none "$1"
  else
    codesign --force --sign "$MTILE_MACOS_SIGN_IDENTITY" --options runtime --timestamp "$1"
  fi
}
# Sign the executable payload inside out. The app signature seals the editor
# named by CFBundleExecutable, its Info.plist, and the rest of the bundle.
for program in "$APP"/Contents/toolchains/go/bin/* "$APP"/Contents/toolchains/go/pkg/tool/darwin_*/* \
               "$APP"/Contents/toolchains/tinygo/bin/* "$APP/Contents/MacOS/mtile" "$APP/Contents/MacOS/mtile-player"; do
  if [ -f "$program" ] && file -b "$program" | grep -q 'Mach-O'; then
    sign "$program"
  fi
done
sign "$APP"
codesign --verify --deep --strict "$APP"
# Repack the whole download, including the writable examples beside the app.
ditto -c -k --sequesterRsrc "$PAYLOAD" "$WORK/$NAME"

if [ -n "${MTILE_MACOS_NOTARY_PROFILE:-}" ]; then
  xcrun notarytool submit "$WORK/$NAME" --keychain-profile "$MTILE_MACOS_NOTARY_PROFILE" \
    --wait --output-format json > "$WORK/notary.json"
  if [ "$(plutil -extract status raw -o - "$WORK/notary.json")" != Accepted ]; then
    cat "$WORK/notary.json" >&2
    exit 1
  fi
  xcrun stapler staple "$APP"
  xcrun stapler validate "$APP"
  spctl --assess --type execute "$APP"
  ditto -c -k --sequesterRsrc "$PAYLOAD" "$WORK/stapled.zip"
  mv "$WORK/stapled.zip" "$WORK/$NAME"
fi
(cd "$WORK" && shasum -a 256 "$NAME" > "$NAME.sha256")
mv "$WORK/$NAME" "$WORK/$NAME.sha256" "$OUTPUT/"
echo "signed archive: $OUTPUT/$NAME"
