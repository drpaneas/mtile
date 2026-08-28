#!/bin/sh
set -eu

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
	printf 'usage: %s WAVEFORM [OUTPUT-DIR]\n' "$0" >&2
	exit 2
fi

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
waveform=$1
effect=${PICO_SFX_EFFECT:-none}
if [ "$#" -eq 2 ]; then
	output_dir=$2
else
	output_dir=$(mktemp -d "${TMPDIR:-/tmp}/mtile-pico-sfx-$waveform.XXXXXX")
fi
pico8_bin=${PICO8_BIN:-/Applications/PICO-8.app/Contents/MacOS/pico8}
pico_home="$output_dir/pico-home"
python3 "$script_dir/prepare_pico8_home.py" "$pico_home"
if [ -n "${PICO_SFX_SPEED:-}" ]; then
	fixture_speed=$PICO_SFX_SPEED
elif [ "$waveform" = noise ]; then
	# Colored-noise statistics need a longer observation window than periodic
	# waveforms. Two seconds removes the low-key centroid/timbre variance that
	# remains in one-second independent random sequences.
	fixture_speed=240
else
	fixture_speed=16
fi

set -- --output-dir "$output_dir" --waveform "$waveform" --effect "$effect" --speed "$fixture_speed"
if [ "${PICO_SFX_NOIZ:-0}" = 1 ]; then
	set -- "$@" --noiz
fi
if [ "${PICO_SFX_BUZZ:-0}" = 1 ]; then
	set -- "$@" --buzz
fi
if [ -n "${PICO_SFX_DETUNE:-}" ]; then
	set -- "$@" --detune "$PICO_SFX_DETUNE"
fi
if [ -n "${PICO_SFX_REVERB:-}" ]; then
	set -- "$@" --reverb "$PICO_SFX_REVERB"
fi
if [ -n "${PICO_SFX_DAMPEN:-}" ]; then
	set -- "$@" --dampen "$PICO_SFX_DAMPEN"
fi
if [ -n "${PICO_SFX_PITCH:-}" ]; then
	set -- "$@" --pitch "$PICO_SFX_PITCH"
fi
python3 "$script_dir/generate_pico_sfx_fixture.py" "$@"

pico_capture="$output_dir/pico-notes.wav"
if [ -e "$pico_capture" ]; then
	printf 'refusing to overwrite existing PICO capture: %s\n' "$pico_capture" >&2
	exit 1
fi

cd "$output_dir"
"$pico8_bin" -home "$pico_home" -volume 256 -root_path "$output_dir" -desktop "$output_dir" \
	-run "$output_dir/$waveform-$effect.p8" &
pico_pid=$!
cd "$repo_root"

stop_pico() {
	if kill -0 "$pico_pid" 2>/dev/null; then
		kill -TERM "$pico_pid" 2>/dev/null || true
		wait "$pico_pid" 2>/dev/null || true
	fi
}
trap stop_pico EXIT HUP INT TERM

capture_ready=false
attempt=0
reverb_level=${PICO_SFX_REVERB:-0}
pico_interval=$(((fixture_speed + 1) / 2 + 4 + reverb_level * 8))
fixture_cases=64
if [ -n "${PICO_SFX_PITCH:-}" ]; then
	fixture_cases=1
fi
pico_timeout=$(((30 + fixture_cases * pico_interval + 60) / 60 + 10))
if [ "$pico_timeout" -lt 40 ]; then
	pico_timeout=40
fi
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
	go run ./cmd/mtile build \
		-o "$output_dir/mtile-notes.gba" "$output_dir/mtile"
)

mtile_interval=$(((fixture_speed * 183 * 30 + 22049) / 22050 + 2 + reverb_level * 4))
fixture_capture_frames=$(((15 + fixture_cases * mtile_interval) * 2 + 52))
MGBA_CAPTURE_FRAMES=${MGBA_CAPTURE_FRAMES:-$fixture_capture_frames} \
	"$script_dir/capture_mgba_audio.sh" \
	"$output_dir/mtile-notes.gba" "$output_dir/mtile-notes.wav"

python3 "$script_dir/analyze_pico_sfx_notes.py" \
	--manifest "$output_dir/manifest.json" \
	--pico "$output_dir/pico-notes.wav" \
	--mgba "$output_dir/mtile-notes.wav" \
	--json "$output_dir/result.json"

printf 'conformance artifacts: %s\n' "$output_dir"
