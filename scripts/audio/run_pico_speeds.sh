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
	output_dir=$(mktemp -d "${TMPDIR:-/tmp}/mtile-pico-speeds.XXXXXX")
fi
pico8_bin=${PICO8_BIN:-/Applications/PICO-8.app/Contents/MacOS/pico8}
pico_home="$output_dir/pico-home"
python3 "$script_dir/prepare_pico8_home.py" "$pico_home"
speed_batches=${PICO_SPEED_BATCHES:-1-64 65-128 129-192 193-255}
pico_pid=

stop_pico() {
	if [ -n "$pico_pid" ] && kill -0 "$pico_pid" 2>/dev/null; then
		kill -TERM "$pico_pid" 2>/dev/null || true
		wait "$pico_pid" 2>/dev/null || true
	fi
	pico_pid=
}
trap stop_pico EXIT HUP INT TERM

mkdir -p "$output_dir"
for bounds in $speed_batches; do
	start=${bounds%-*}
	end=${bounds#*-}
	batch_name=$(printf 'speeds-%03d-%03d' "$start" "$end")
	batch_dir="$output_dir/$batch_name"
	python3 "$script_dir/generate_pico_speed_fixture.py" \
		--output-dir "$batch_dir" --start-speed "$start" --end-speed "$end"

	pico_capture="$batch_dir/pico-$batch_name.wav"
	if [ -e "$pico_capture" ]; then
		printf 'refusing to overwrite existing PICO capture: %s\n' "$pico_capture" >&2
		exit 1
	fi
	cd "$batch_dir"
	"$pico8_bin" -home "$pico_home" -volume 256 -root_path "$batch_dir" -desktop "$batch_dir" \
		-run "$batch_dir/speeds.p8" &
	pico_pid=$!
	cd "$repo_root"
	pico_total=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["picoTotalFrames"])' "$batch_dir/manifest.json")
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

	(
		cd "$repo_root"
		go run ./cmd/mtile build -o "$batch_dir/mtile-$batch_name.gba" "$batch_dir/mtile"
	)
	mtile_total=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["mtileTotalFrames"])' "$batch_dir/manifest.json")
	capture_frames=$((mtile_total * 2 + 52))
	MGBA_CAPTURE_FRAMES=$capture_frames \
		"$script_dir/capture_mgba_audio.sh" \
		"$batch_dir/mtile-$batch_name.gba" "$batch_dir/mtile-$batch_name.wav"

	python3 "$script_dir/analyze_pico_speeds.py" \
		--manifest "$batch_dir/manifest.json" \
		--pico "$pico_capture" \
		--mgba "$batch_dir/mtile-$batch_name.wav" \
		--json "$batch_dir/result.json"
done

printf 'speed conformance artifacts: %s\n' "$output_dir"
