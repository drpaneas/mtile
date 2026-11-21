#!/bin/bash
# Import the release credentials for one notarized macOS ZIP.
set +x
set -euo pipefail
umask 077
export LC_ALL=C
if [[ $# != 3 ]]; then
  echo 'usage: sign-macos-release.sh ARCHIVE.zip OUTPUT_DIR MAJOR.MINOR.PATCH' >&2
  exit 2
fi
: "${MTILE_MACOS_CERTIFICATE_P12:?missing Developer ID Application PKCS#12}"
: "${MTILE_MACOS_CERTIFICATE_PASSWORD:?missing PKCS#12 password}"
: "${MTILE_MACOS_NOTARY_KEY_P8:?missing App Store Connect team API private key}"
: "${MTILE_MACOS_NOTARY_KEY_ID:?missing App Store Connect key ID}"
: "${MTILE_MACOS_NOTARY_ISSUER_ID:?missing App Store Connect issuer ID}"
cert_data=$MTILE_MACOS_CERTIFICATE_P12
cert_password=$MTILE_MACOS_CERTIFICATE_PASSWORD
key_data=$MTILE_MACOS_NOTARY_KEY_P8
key_id=$MTILE_MACOS_NOTARY_KEY_ID
issuer=$MTILE_MACOS_NOTARY_ISSUER_ID
unset MTILE_MACOS_CERTIFICATE_P12 MTILE_MACOS_CERTIFICATE_PASSWORD MTILE_MACOS_NOTARY_KEY_P8
unset MTILE_MACOS_NOTARY_KEY_ID MTILE_MACOS_NOTARY_ISSUER_ID
work=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/mtile-signing.XXXXXX")
keychain="$work/release.keychain-db"
original=()
restore_search=0
cleanup() {
  local result=$?
  trap - EXIT
  if (( restore_search )); then
    if [[ -f "$keychain" ]]; then
      security delete-keychain "$keychain" >/dev/null || result=1
    fi
    security list-keychains -d user -s ${original[@]+"${original[@]}"} || result=1
  fi
  rm -rf "$work" || result=1
  exit "$result"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

security list-keychains -d user > "$work/original-search"
while IFS= read -r line; do
  line=${line#*\"}
  original+=("${line%\"*}")
done < "$work/original-search"
restore_search=1
password=$(openssl rand -hex 32)
security create-keychain -p "$password" "$keychain"
security set-keychain-settings -lut 21600 "$keychain"
security unlock-keychain -p "$password" "$keychain"
security list-keychains -d user -s "$keychain" ${original[@]+"${original[@]}"}
printf '%s' "$cert_data" | base64 -D > "$work/cert.p12"
printf '%s' "$key_data" | base64 -D > "$work/key.p8"
unset cert_data key_data
if ! security import "$work/cert.p12" -f pkcs12 -k "$keychain" -x -P "$cert_password" -T /usr/bin/codesign >/dev/null 2>&1; then
  echo 'Unable to import the Developer ID certificate and private key.' >&2
  exit 1
fi
unset cert_password
rm "$work/cert.p12"
security set-key-partition-list -S apple-tool:,apple: -s -k "$password" "$keychain" >/dev/null
unset password
identities=$(security find-identity -v -p codesigning "$keychain")
identity_re='^[[:space:]]*[0-9]+\)[[:space:]]+([A-Fa-f0-9]{40})[[:space:]]+"Developer ID Application: .+ \(([A-Z0-9]{10})\)"$'
matched=0
while IFS= read -r line; do
  if [[ $line =~ $identity_re ]]; then
    export MTILE_MACOS_SIGN_IDENTITY=${BASH_REMATCH[1]}
    export MTILE_MACOS_EXPECTED_TEAM_ID=${BASH_REMATCH[2]}
    matched=$((matched+1))
  fi
done <<< "$identities"
if (( matched != 1 )); then
  echo 'The PKCS#12 must contain exactly one valid Developer ID Application signing identity.' >&2
  exit 1
fi

export MTILE_MACOS_KEYCHAIN="$keychain" MTILE_MACOS_NOTARY_PROFILE=mtile-release
if ! xcrun notarytool store-credentials "$MTILE_MACOS_NOTARY_PROFILE" --keychain "$keychain" \
    --key "$work/key.p8" --key-id "$key_id" --issuer "$issuer" >/dev/null 2>&1; then
  echo 'Apple could not validate the App Store Connect notarization credentials.' >&2
  exit 1
fi
rm "$work/key.p8"
unset issuer key_id
sh "$(dirname "$0")/sign-macos.sh" "$@"
