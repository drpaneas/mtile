#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
runtime_dir="$repo_root/runtime"

profile=${AUDIO_HARDWARE_PROFILE:-sample}
case "$profile" in
sample)
	artifact=audiohardware
	expected_samples=1
	;;
standard)
	artifact=audiohardwarestandard
	expected_samples=0
	;;
*)
	printf 'unknown audio hardware profile: %s\n' "$profile" >&2
	exit 1
	;;
esac
map_file="$runtime_dir/$artifact.map"
elf_file="$runtime_dir/$artifact.elf"
rom_file="$runtime_dir/$artifact.gba"

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

make -C "$runtime_dir" "$artifact-gba" "$artifact-elf"

symbol_address() {
	awk -v symbol="$1" '$NF == symbol { print "0x" $1; exit }' "$map_file"
}

status_addr=$(symbol_address main.hardwareStatus)
ticks_addr=$(symbol_address main.hardwareTicks)
boundaries_addr=$(symbol_address main.hardwareBoundaries)
underruns_addr=$(symbol_address main.hardwareUnderruns)
late_addr=$(symbol_address main.hardwareMaxLateSamples)
switch_addr=$(symbol_address main.hardwareSwitchFailures)
queued_addr=$(symbol_address main.hardwareQueued)
rewinds_addr=$(symbol_address main.hardwareRewinds)
fallbacks_addr=$(symbol_address main.hardwareFallbacks)
drops_addr=$(symbol_address main.hardwareDrops)
samples_addr=$(symbol_address main.hardwareSampleVoices)
command_progress_addr=$(symbol_address main.hardwareCommandProgress)
pending_addr=$(symbol_address main.hardwarePendingCommands)
music_starts_addr=$(symbol_address main.hardwareMusicStarts)
music_stops_addr=$(symbol_address main.hardwareMusicStops)
fade_checks_addr=$(symbol_address main.hardwareFadeChecks)

for value in "$status_addr" "$ticks_addr" "$boundaries_addr" "$underruns_addr" \
	"$late_addr" "$switch_addr" "$queued_addr" "$rewinds_addr" "$fallbacks_addr" \
	"$drops_addr" "$samples_addr" "$command_progress_addr" "$pending_addr" \
	"$music_starts_addr" "$music_stops_addr" "$fade_checks_addr"; do
	if [ -z "$value" ]; then
		printf 'hardware-soak telemetry symbols are missing from %s\n' "$map_file" >&2
		exit 1
	fi
done

mgba_log=$(mktemp "${TMPDIR:-/tmp}/mtile-audiohardware.XXXXXX")
gdb_commands=$(mktemp "${TMPDIR:-/tmp}/mtile-audiohardware-gdb.XXXXXX")
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
x/uw $ticks_addr
x/uw $boundaries_addr
x/uw $underruns_addr
x/uw $late_addr
x/uw $switch_addr
x/uw $queued_addr
x/uw $rewinds_addr
x/uw $fallbacks_addr
x/uw $drops_addr
x/uw $samples_addr
x/uw $command_progress_addr
x/uw $pending_addr
x/uw $music_starts_addr
x/uw $music_stops_addr
x/uw $fade_checks_addr
set \$observed_status = *(unsigned int*)$status_addr
set \$observed_underruns = *(unsigned int*)$underruns_addr
set \$observed_switches = *(unsigned int*)$switch_addr
set \$observed_queued = *(unsigned int*)$queued_addr
set \$observed_rewinds = *(unsigned int*)$rewinds_addr
set \$observed_fallbacks = *(unsigned int*)$fallbacks_addr
set \$observed_drops = *(unsigned int*)$drops_addr
set \$observed_samples = *(unsigned int*)$samples_addr
set \$observed_pending = *(unsigned int*)$pending_addr
set \$observed_music_starts = *(unsigned int*)$music_starts_addr
set \$observed_music_stops = *(unsigned int*)$music_stops_addr
set \$observed_fade_checks = *(unsigned int*)$fade_checks_addr
if \$observed_status != 0xA0A00001
  echo hardware soak is not running\n
  quit 1
end
if \$observed_underruns != 0
  echo hardware soak reported an unforced underrun\n
  quit 1
end
if \$observed_switches != 0
  echo hardware soak reported a stream switch failure\n
  quit 1
end
if \$observed_queued != 3
  echo hardware soak did not retain three queued pages\n
  quit 1
end
if \$observed_rewinds < 2
  echo hardware soak did not exercise both deferred music fades\n
  quit 1
end
if \$observed_fallbacks != 0 || \$observed_drops != 0
  echo hardware soak reported command fallback or drop\n
  quit 1
end
if \$observed_samples != $expected_samples
  echo hardware soak reported the wrong active PCM voice count\n
  quit 1
end
if \$observed_pending != 0 || \$observed_music_starts != 2 || \$observed_music_stops != 1 || \$observed_fade_checks != 1
  echo hardware soak did not apply its early music commands\n
  quit 1
end
EOF

"$mgba_bin" -g "$rom_file" >"$mgba_log" 2>&1 &
mgba_pid=$!
sleep 2

"$gdb_bin" -q -batch -x "$gdb_commands" "$elf_file"
