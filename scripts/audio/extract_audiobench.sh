#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
runtime_dir="$repo_root/runtime"
map_file="$runtime_dir/audiobench.map"
elf_file="$runtime_dir/audiobench.elf"
rom_file="$runtime_dir/audiobench.gba"

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

make -C "$runtime_dir" audiobench-gba audiobench-elf

symbol_address() {
	awk -v symbol="$1" '$NF == symbol { print "0x" $1; exit }' "$map_file"
}

status_addr=$(symbol_address main.benchStatus)
cycles_addr=$(symbol_address main.benchCycles)
count_addr=$(symbol_address main.benchCount)
failure_addr=$(symbol_address main.benchFailure)
cycles_size=$(awk '$NF == "main.benchCycles" { print $3; exit }' "$map_file")

if [ -z "$status_addr" ] || [ -z "$cycles_addr" ] || [ -z "$count_addr" ] || [ -z "$failure_addr" ] || [ -z "$cycles_size" ]; then
	printf 'benchmark telemetry symbols are missing from %s\n' "$map_file" >&2
	exit 1
fi

case_count=$((0x$cycles_size / 4))
mgba_log=$(mktemp "${TMPDIR:-/tmp}/mtile-audiobench.XXXXXX")
mgba_pid=

cleanup() {
	if [ -n "$mgba_pid" ]; then
		kill "$mgba_pid" 2>/dev/null || true
		wait "$mgba_pid" 2>/dev/null || true
	fi
	rm -f "$mgba_log"
}
trap cleanup EXIT HUP INT TERM

"$mgba_bin" -g "$rom_file" >"$mgba_log" 2>&1 &
mgba_pid=$!
sleep 2

"$gdb_bin" -q -batch "$elf_file" \
	-ex 'set architecture arm' \
	-ex 'target remote 127.0.0.1:2345' \
	-ex "watch *(unsigned int*)$status_addr" \
	-ex continue \
	-ex continue \
	-ex "x/wx $status_addr" \
	-ex "x/uw $count_addr" \
	-ex "x/wx $failure_addr" \
	-ex "x/${case_count}uw $cycles_addr"
