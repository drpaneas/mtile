#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
runtime_dir="$repo_root/runtime"
output_dir=${GBA_AUDIO_SIGNOFF_DIR:-$repo_root/build/audio-hardware-signoff}

# The production handoff contains only the four Game Boy PSG channels. Legacy
# Direct Sound/FIFO/PCM experiments are intentionally not built here.
make -C "$runtime_dir" audioref-gba audiopsg-gba audiopsgcoexist-gba
mkdir -p "$output_dir"

hash_file() {
	file=$1
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$file" | awk '{print $1}'
	else
		shasum -a 256 "$file" | awk '{print $1}'
	fi
}

printf '%-30s %10s  %s\n' ROM BYTES SHA256
: >"$output_dir/SHA256SUMS"
for name in \
	audioref-ch1 audioref-ch2 audioref-ch3 audioref-ch4 \
	audiopsg audiopsgcoexist
do
	file="$runtime_dir/$name.gba"
	bundle_file="$output_dir/$name.gba"
	cp "$file" "$bundle_file"
	bytes=$(wc -c <"$bundle_file" | tr -d ' ')
	hash=$(hash_file "$bundle_file")
	printf '%-30s %10s  %s\n' "$name.gba" "$bytes" "$hash"
	printf '%s  %s\n' "$hash" "$name.gba" >>"$output_dir/SHA256SUMS"
done

(cd "$runtime_dir" && go run ./internal/cmd/audiorefmanifest \
	-format json -output "$output_dir/CASES.json")
(cd "$runtime_dir" && go run ./internal/cmd/audiorefmanifest \
	-format markdown -output "$output_dir/RESULTS.md")

commit=unavailable
if git -C "$repo_root" rev-parse HEAD >/dev/null 2>&1; then
	commit=$(git -C "$repo_root" rev-parse HEAD)
fi
{
	printf 'generated_utc=%s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
	printf 'source_commit=%s\n' "$commit"
	printf 'go=%s\n' "$(go version)"
	printf 'tinygo=%s\n' "$(tinygo version | head -n 1)"
	printf 'scope=Game Boy PSG CH1-CH4 only; Direct Sound/PCM deferred\n'
} >"$output_dir/BUILD-INFO.txt"

"$script_dir/verify_hardware_signoff.sh" "$output_dir"
printf '\nHardware sign-off bundle: %s\n' "$output_dir"
