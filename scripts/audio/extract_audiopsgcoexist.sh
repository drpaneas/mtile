#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
runtime_dir="$repo_root/runtime"
mgba_bin=${MGBA_BIN:-/Applications/mGBA.app/Contents/MacOS/mgba}

if [ ! -x "$mgba_bin" ]; then
	printf 'mGBA executable not found: %s\n' "$mgba_bin" >&2
	exit 1
fi

if [ "${PSG_COEXIST_SKIP_BUILD:-0}" != "1" ]; then
	make -C "$runtime_dir" audiopsgcoexist-gba audiopsgcoexist-elf
fi

"${PYTHON_BIN:-python3}" "$script_dir/read_mgba_psg_coexist.py" \
	--mgba "$mgba_bin" \
	--rom "$runtime_dir/audiopsgcoexist.gba" \
	--map "$runtime_dir/audiopsgcoexist.map" \
	--seconds "${PSG_COEXIST_SECONDS:-10}" \
	--mode "${PSG_COEXIST_MODE:-paced}" \
	--fps-target "${PSG_COEXIST_FPS_TARGET:-60}" \
	--minimum-service-calls "${PSG_COEXIST_MIN_SERVICE_CALLS:-2048}"
