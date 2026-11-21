#!/bin/sh
# Sign the app inside a verified candidate and write a separate distribution ZIP.
set -eu
unset CDPATH
export LC_ALL=C

if [ "$#" != 3 ]; then
  echo 'usage: sign-macos.sh ARCHIVE.zip OUTPUT_DIR MAJOR.MINOR.PATCH' >&2
  exit 2
fi
: "${MTILE_MACOS_SIGN_IDENTITY:?set a Developer ID Application identity, or - for an ad hoc test}"
case "$MTILE_MACOS_SIGN_IDENTITY" in
  -) if [ -n "${MTILE_MACOS_NOTARY_PROFILE:-}" ]; then
       echo 'A notarized package requires a Developer ID Application identity.' >&2; exit 2
     fi ;;
  *) : "${MTILE_MACOS_NOTARY_PROFILE:?a Developer ID release requires a notarization profile}" ;;
esac
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
    set -- --force --sign "$MTILE_MACOS_SIGN_IDENTITY" --options runtime --timestamp "$1"
    if [ -n "${MTILE_MACOS_KEYCHAIN:-}" ]; then set -- --keychain "$MTILE_MACOS_KEYCHAIN" "$@"; fi
    codesign "$@"
  fi
}

nested_code() {
  action=$1
  app=$2
  # The app signature covers the main executable. Include executable Mach-O
  # helpers anywhere in the private SDKs, keeping object files and source intact.
  find "$app/Contents" -type f \( -perm -100 -o -perm -010 -o -perm -001 -o -name '*.dylib' -o -name '*.so' \) \
    -print > "$WORK/code-paths"
  while IFS= read -r program; do
    [ "$program" != "$app/Contents/MacOS/mtile-editor" ] || continue
    kind=$(file -b "$program")
    case "$kind" in
      *Mach-O*executable*|*Mach-O*'shared library'*|*Mach-O*bundle*) "$action" "$program" ;;
    esac
  done < "$WORK/code-paths"
}

verify_developer_id() {
  codesign --verify --strict --all-architectures -R "$requirement" "$1"
  codesign --display --verbose=4 "$1" > "$WORK/signature.txt" 2>&1
  if ! grep -Eq 'flags=0x[[:xdigit:]]+\([^)]*runtime[^)]*\)' "$WORK/signature.txt" ||
     ! grep -Eq '^Timestamp=.+' "$WORK/signature.txt" ||
     grep -Eiq '^Timestamp=(none|not set|invalid)$' "$WORK/signature.txt"; then
    echo "Missing hardened runtime or secure timestamp on $1" >&2
    exit 1
  fi
}

nested_code sign "$APP"
sign "$APP"
codesign --verify --deep --strict "$APP"
# Repack the whole download, including the writable examples beside the app.
ditto -c -k --sequesterRsrc "$PAYLOAD" "$WORK/$NAME"

if [ "$MTILE_MACOS_SIGN_IDENTITY" != '-' ]; then
  codesign --display --verbose=4 --extract-certificates="$WORK/cert-" "$APP" > "$WORK/signature.txt" 2>&1
  leaf=$(shasum -a 1 "$WORK/cert-0")
  leaf=${leaf%% *}
  team=$(sed -n 's/^TeamIdentifier=//p' "$WORK/signature.txt")
  if ! printf '%s\n' "$team" | grep -Eq '^[A-Z0-9]{10}$' ||
     [ "$team" != "${MTILE_MACOS_EXPECTED_TEAM_ID:-$team}" ]; then
    echo 'Missing or unexpected Apple Developer Team ID on the signed app.' >&2
    exit 1
  fi
  if printf '%s\n' "$MTILE_MACOS_SIGN_IDENTITY" | grep -Eq '^[[:xdigit:]]{40}$' &&
     [ "$leaf" != "$(printf '%s' "$MTILE_MACOS_SIGN_IDENTITY" | tr A-F a-f)" ]; then
    echo 'The signed app does not use the selected release certificate.' >&2
    exit 1
  fi
  requirement="=anchor apple generic and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = \"$team\" and certificate leaf = H\"$leaf\""
  verify_developer_id "$APP"
  set -- notarytool submit "$WORK/$NAME" --keychain-profile "$MTILE_MACOS_NOTARY_PROFILE" --wait --output-format json
  if [ -n "${MTILE_MACOS_KEYCHAIN:-}" ]; then set -- "$@" --keychain "$MTILE_MACOS_KEYCHAIN"; fi
  xcrun "$@" > "$WORK/notary.json"
  if [ "$(plutil -extract status raw -o - "$WORK/notary.json")" != Accepted ]; then
    echo 'Apple did not accept the notarization submission:' >&2
    cat "$WORK/notary.json" >&2
    exit 1
  fi
  xcrun stapler staple "$APP"
  ditto -c -k --sequesterRsrc "$PAYLOAD" "$WORK/stapled.zip"
  mv "$WORK/stapled.zip" "$WORK/$NAME"

  # Verify the stapled ticket and every shipped signature after a ZIP round trip.
  mkdir "$WORK/final"
  ditto -x -k "$WORK/$NAME" "$WORK/final"
  final_app="$WORK/final/Mtile.app"
  codesign --verify --deep --strict "$final_app"
  verify_developer_id "$final_app"
  nested_code verify_developer_id "$final_app"
  xcrun stapler validate "$final_app"
  spctl --assess --type execute "$final_app"
fi
(cd "$WORK" && shasum -a 256 "$NAME" > "$NAME.sha256" && shasum -a 256 -c "$NAME.sha256")
mv "$WORK/$NAME" "$WORK/$NAME.sha256" "$OUTPUT/"
echo "signed archive: $OUTPUT/$NAME"
