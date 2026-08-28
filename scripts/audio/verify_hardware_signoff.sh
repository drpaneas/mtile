#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
bundle=${1:-${GBA_AUDIO_SIGNOFF_DIR:-$repo_root/build/audio-hardware-signoff}}

if [ ! -d "$bundle" ]; then
	printf 'hardware sign-off bundle not found: %s\n' "$bundle" >&2
	exit 1
fi

expected='audioref-ch1.gba audioref-ch2.gba audioref-ch3.gba audioref-ch4.gba audiopsg.gba audiopsgcoexist.gba'
for name in $expected; do
	if [ ! -f "$bundle/$name" ]; then
		printf 'missing required PSG ROM: %s\n' "$name" >&2
		exit 1
	fi
done

for file in "$bundle"/*.gba; do
	name=${file##*/}
	allowed=0
	for expected_name in $expected; do
		if [ "$name" = "$expected_name" ]; then
			allowed=1
			break
		fi
	done
	if [ "$allowed" -ne 1 ]; then
		printf 'unexpected ROM in PSG-only bundle: %s\n' "$name" >&2
		exit 1
	fi
done

for metadata in SHA256SUMS CASES.json RESULTS.md BUILD-INFO.txt; do
	if [ ! -s "$bundle/$metadata" ]; then
		printf 'missing or empty bundle metadata: %s\n' "$metadata" >&2
		exit 1
	fi
done

if command -v sha256sum >/dev/null 2>&1; then
	(cd "$bundle" && sha256sum -c SHA256SUMS)
else
	(cd "$bundle" && shasum -a 256 -c SHA256SUMS)
fi

python3 "$script_dir/verify_physical_signoff.py" --allow-pending "$bundle"

printf 'PSG-only hardware sign-off bundle: PASS\n'
