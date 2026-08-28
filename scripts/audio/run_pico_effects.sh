#!/bin/sh
set -eu

if [ "$#" -gt 1 ]; then
	printf 'usage: %s [OUTPUT-DIR]\n' "$0" >&2
	exit 2
fi

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
if [ "$#" -eq 1 ]; then
	output_dir=$1
else
	output_dir=$(mktemp -d "${TMPDIR:-/tmp}/mtile-pico-effects.XXXXXX")
fi
pico8_bin=${PICO8_BIN:-/Applications/PICO-8.app/Contents/MacOS/pico8}
pico_home="$output_dir/pico-home"
python3 "$script_dir/prepare_pico8_home.py" "$pico_home"

python3 "$script_dir/generate_pico_effect_fixture.py" --output-dir "$output_dir"
pico_capture="$output_dir/pico-effects.wav"
if [ -e "$pico_capture" ]; then
	printf 'refusing to overwrite existing PICO capture: %s\n' "$pico_capture" >&2
	exit 1
fi

cd "$output_dir"
"$pico8_bin" -home "$pico_home" -volume 256 -root_path "$output_dir" -desktop "$output_dir" \
	-run "$output_dir/effects.p8" &
pico_pid=$!
cd "$repo_root"
stop_pico() {
	if kill -0 "$pico_pid" 2>/dev/null; then
		kill -TERM "$pico_pid" 2>/dev/null || true
		wait "$pico_pid" 2>/dev/null || true
	fi
}
trap stop_pico EXIT HUP INT TERM

pico_total=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["picoTotalFrames"])' "$output_dir/manifest.json")
pico_timeout=$((pico_total / 60 + 12))
capture_ready=false
attempt=0
while [ "$attempt" -lt "$pico_timeout" ]; do
	if [ -s "$pico_capture" ] && [ "$(wc -c < "$pico_capture")" -gt 44 ]; then
		capture_ready=true
		break
	fi
	if ! kill -0 "$pico_pid" 2>/dev/null; then
		break
	fi
	sleep 1
	attempt=$((attempt + 1))
done
if [ "$capture_ready" != true ]; then
	printf 'PICO-8 did not produce a non-empty capture: %s\n' "$pico_capture" >&2
	exit 1
fi
sleep 1
stop_pico
trap - EXIT HUP INT TERM

(
	cd "$repo_root"
	go run ./cmd/mtile build -o "$output_dir/mtile-effects.gba" "$output_dir/mtile"
)
mtile_total=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["mtileTotalFrames"])' "$output_dir/manifest.json")
capture_frames=$((mtile_total * 2 + 52))
MGBA_CAPTURE_FRAMES=${MGBA_CAPTURE_FRAMES:-$capture_frames} \
	"$script_dir/capture_mgba_audio.sh" \
	"$output_dir/mtile-effects.gba" "$output_dir/mtile-effects.wav"

python3 "$script_dir/analyze_pico_effects.py" \
	--manifest "$output_dir/manifest.json" \
	--pico "$pico_capture" \
	--mgba "$output_dir/mtile-effects.wav" \
	--json "$output_dir/result.json"

printf 'effect conformance artifacts: %s\n' "$output_dir"
