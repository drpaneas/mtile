#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/../.." && pwd)
runtime_dir="$repo_root/runtime"
target=${AUDIOCOEXIST_TARGET:-audiocoexist}
map_file="$runtime_dir/$target.map"
elf_file="$runtime_dir/$target.elf"
rom_file="$runtime_dir/$target.gba"

mgba_bin=${MGBA_BIN:-/Applications/mGBA.app/Contents/MacOS/mgba}
gdb_bin=${GDB_BIN:-$(command -v gdb || true)}

if [ ! -x "$mgba_bin" ]; then
	printf 'mGBA executable not found: %s\n' "$mgba_bin" >&2
	exit 1
fi
if [ -z "$gdb_bin" ] || [ ! -x "$gdb_bin" ]; then
	printf 'GDB not found; set GDB_BIN to an ARM-capable GDB executable\n' >&2
	exit 1
fi

make -C "$runtime_dir" "${target}-gba" "${target}-elf"

symbol_address() {
	awk -v symbol="$1" '$NF == symbol { print "0x" $1; exit }' "$map_file"
}

status_addr=$(symbol_address main.coexistStatus)
count_addr=$(symbol_address main.coexistCount)
cycles_size=$(awk '$NF == "main.coexistMaxCycles" { print $3; exit }' "$map_file")

if [ -z "$status_addr" ] || [ -z "$count_addr" ] || [ -z "$cycles_size" ]; then
	printf 'coexistence telemetry symbols are missing from %s\n' "$map_file" >&2
	exit 1
fi

case_count=$((0x$cycles_size / 4))
mgba_log=$(mktemp "${TMPDIR:-/tmp}/mtile-audiocoexist.XXXXXX")
gdb_commands=$(mktemp "${TMPDIR:-/tmp}/mtile-audiocoexist-gdb.XXXXXX")
mgba_pid=

cleanup() {
	if [ -n "$mgba_pid" ]; then
		kill "$mgba_pid" 2>/dev/null || true
		wait "$mgba_pid" 2>/dev/null || true
	fi
	rm -f "$mgba_log"
	rm -f "$gdb_commands"
}
trap cleanup EXIT HUP INT TERM

"$mgba_bin" -g "$rom_file" >"$mgba_log" 2>&1 &
mgba_pid=$!
sleep 2

set -- \
	main.coexistMaxCycles \
	main.coexistMaxServiceCycles \
	main.coexistMaxServiceBlocks \
	main.coexistBoundaries \
	main.coexistUnderruns \
	main.coexistLateBoundaries \
	main.coexistMaxLateSamples \
	main.coexistFadeRemaining \
	main.coexistRewinds \
	main.coexistFallbacks \
	main.coexistSampleVoices \
	main.coexistSampleStarts \
	main.coexistFailedSampleStarts \
	main.coexistInvalidSampleCalls

{
	printf 'set architecture arm\n'
	printf 'target remote 127.0.0.1:2345\n'
	printf 'watch *(unsigned int*)%s\n' "$status_addr"
	printf 'continue\ncontinue\n'
	printf 'x/wx %s\n' "$status_addr"
	printf 'x/uw %s\n' "$count_addr"
	for symbol do
		address=$(symbol_address "$symbol")
		if [ -z "$address" ]; then
			printf 'missing telemetry symbol: %s\n' "$symbol" >&2
			exit 1
		fi
		printf 'x/%suw %s\n' "$case_count" "$address"
	done
	printf 'x/uw %s\n' "$(symbol_address main.coexistRecoveryQueued)"
	printf 'x/uw %s\n' "$(symbol_address main.coexistRecoveryBoundaries)"
	printf 'x/uw %s\n' "$(symbol_address main.coexistRecoveryUnderruns)"
	printf 'x/uw %s\n' "$(symbol_address main.coexistFinalUnderruns)"
} >"$gdb_commands"

"$gdb_bin" -q -batch -x "$gdb_commands" "$elf_file"
