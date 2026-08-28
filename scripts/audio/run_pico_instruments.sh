#!/bin/sh
set -eu

if [ "$#" -gt 1 ]; then
	printf 'usage: %s [OUTPUT-DIR]\n' "$0" >&2
	exit 2
fi

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
if [ "$#" -eq 1 ]; then
	output_dir=$1
else
	output_dir=$(mktemp -d "${TMPDIR:-/tmp}/mtile-pico-instruments.XXXXXX")
fi

mkdir -p "$output_dir"
for waveform in triangle tilted-saw saw square pulse organ noise phaser; do
	waveform_dir="$output_dir/$waveform"
	if [ -s "$waveform_dir/result.json" ] && python3 -c \
		'import json,sys; d=json.load(open(sys.argv[1])); raise SystemExit(d["summary"]["failures"] != 0 or d["summary"]["cases"] != 64)' \
		"$waveform_dir/result.json"; then
		printf '%s: existing 64/64 passing result\n' "$waveform"
		continue
	fi
	"$script_dir/run_pico_sfx_isolated.sh" "$waveform" "$waveform_dir"
done

printf 'instrument conformance artifacts: %s\n' "$output_dir"
