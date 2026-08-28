#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
runtime_dir="$repo_root/runtime"
map_file="$runtime_dir/audiostereostream.map"
elf_file="$runtime_dir/audiostereostream.elf"
rom_file="$runtime_dir/audiostereostream.gba"

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

make -C "$runtime_dir" audiostereostream-gba audiostereostream-elf

symbol_address() {
	awk -v symbol="$1" '$NF == symbol { print "0x" $1; exit }' "$map_file"
}

status_addr=$(symbol_address main.streamStatus)
boundaries_addr=$(symbol_address main.streamBoundaries)
underruns_addr=$(symbol_address main.streamUnderruns)
late_addr=$(symbol_address main.streamLateBoundaries)
max_late_addr=$(symbol_address main.streamMaxLateSamples)
mixed_addr=$(symbol_address main.streamMixedBlocks)
queued_addr=$(symbol_address main.streamQueuedBlocks)

for value in "$status_addr" "$boundaries_addr" "$underruns_addr" "$late_addr" \
	"$max_late_addr" "$mixed_addr" "$queued_addr"; do
	if [ -z "$value" ]; then
		printf 'stereo stream telemetry symbols are missing from %s\n' "$map_file" >&2
		exit 1
	fi
done

mgba_log=$(mktemp "${TMPDIR:-/tmp}/mtile-audiostereostream.XXXXXX")
gdb_commands=$(mktemp "${TMPDIR:-/tmp}/mtile-audiostereostream-gdb.XXXXXX")
mgba_pid=

cleanup() {
	if [ -n "$mgba_pid" ]; then
		kill "$mgba_pid" 2>/dev/null || true
		wait "$mgba_pid" 2>/dev/null || true
	fi
	rm -f "$mgba_log" "$gdb_commands"
}
trap cleanup EXIT HUP INT TERM

cat >"$gdb_commands" <<EOF
set pagination off
set architecture arm
target remote 127.0.0.1:2345
break main.publishTelemetry
ignore 1 239
continue
x/wx $status_addr
x/uw $boundaries_addr
x/uw $underruns_addr
x/uw $late_addr
x/uw $max_late_addr
x/uw $mixed_addr
x/uw $queued_addr
x/hx 0x04000082
x/hx 0x040000c6
x/hx 0x040000d2
x/wx 0x040000c0
x/wx 0x040000cc
set \$observed_status = *(unsigned int*)$status_addr
set \$observed_underruns = *(unsigned int*)$underruns_addr
set \$observed_queued = *(unsigned int*)$queued_addr
if \$observed_status != 0xA057B001
  echo stereo stream is not running\n
  quit 1
end
if \$observed_underruns != 0
  echo stereo stream reported an unforced underrun\n
  quit 1
end
if \$observed_queued != 3
  echo stereo stream did not retain three queued pairs\n
  quit 1
end
EOF

"$mgba_bin" -g "$rom_file" >"$mgba_log" 2>&1 &
mgba_pid=$!
sleep 2

"$gdb_bin" -q -batch -x "$gdb_commands" "$elf_file"
