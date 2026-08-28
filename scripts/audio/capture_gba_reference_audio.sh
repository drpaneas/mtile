#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
runtime_dir="$repo_root/runtime"
output_dir=${GBA_AUDIO_REFERENCE_OUTPUT_DIR:-$repo_root/docs/audio/captures/v2/mgba}

if [ "${GBA_AUDIO_REFERENCE_SKIP_BUILD:-0}" != "1" ]; then
	make -C "$runtime_dir" audioref-gba
fi
mkdir -p "$output_dir"

reference_kinds=${GBA_AUDIO_REFERENCE_KINDS:-"ch1 ch2 ch3 ch4"}
for kind in $reference_kinds; do
	case "$kind" in
		ch1) case_count=12 ;;
		ch2|ch3|ch4) case_count=10 ;;
		fifoa|fifob) case_count=6 ;;
		all) case_count=2 ;;
		*) echo "unsupported reference kind: $kind" >&2; exit 2 ;;
	esac
	# One truth case is visible for two seconds; mGBA counts 60 VBlanks/s.
	capture_frames=$((case_count * 120))
	rom="$runtime_dir/audioref-$kind.gba"
	output="$output_dir/audioref-$kind.wav"
	MGBA_CAPTURE_FRAMES=$capture_frames \
		"$script_dir/capture_mgba_audio.sh" "$rom" "$output"
	"${PYTHON_BIN:-python3}" "$script_dir/analyze_gba_reference_capture.py" \
		--kind "$kind" --input "$output"
	shasum -a 256 "$rom" "$output"
done

if [ "$reference_kinds" = "ch1 ch2 ch3 ch4" ]; then
	"${PYTHON_BIN:-python3}" "$script_dir/summarize_gba_reference_captures.py" \
		--runtime "$runtime_dir" --captures "$output_dir" \
		--output "$output_dir/reference-captures.json"
fi
