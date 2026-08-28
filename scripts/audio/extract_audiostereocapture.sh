#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
runtime_dir="$repo_root/runtime"
capture=${AUDIOSTEREO_CAPTURE_OUTPUT:-$repo_root/docs/audio/captures/mgba-stereo-routing.wav}
reference=$(mktemp "${TMPDIR:-/tmp}/mtile-audiostereo-reference.XXXXXX.wav")

cleanup() {
	rm -f -- "$reference"
}
trap cleanup EXIT HUP INT TERM

make -C "$runtime_dir" audiostereocapture-gba
MGBA_CAPTURE_FRAMES=${MGBA_CAPTURE_FRAMES:-120} \
	"$script_dir/capture_mgba_audio.sh" "$runtime_dir/audiostereocapture.gba" "$capture"
(
	cd "$runtime_dir"
	go run ./cmd/audiostereoreference -frames 65536 -output "$reference"
)
${PYTHON_BIN:-python3} "$script_dir/analyze_stereo_capture.py" "$capture" "$reference"
shasum -a 256 "$capture"
