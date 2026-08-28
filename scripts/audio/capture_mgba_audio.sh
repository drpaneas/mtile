#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
	printf 'usage: %s ROM OUTPUT.wav\n' "$0" >&2
	exit 2
fi

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
rom=$1
output=$2
mgba_bin=${MGBA_BIN:-/Applications/mGBA.app/Contents/MacOS/mgba}
source_dir=${MGBA_SOURCE:-}

if [ ! -f "$rom" ]; then
	printf 'ROM not found: %s\n' "$rom" >&2
	exit 1
fi
if [ ! -x "$mgba_bin" ]; then
	printf 'mGBA executable not found: %s\n' "$mgba_bin" >&2
	exit 1
fi

if [ -z "$source_dir" ]; then
	for candidate in \
		"${TMPDIR:-/tmp}/mgba-0.10.3" \
		/private/tmp/mgba-0.10.3 \
		"$repo_root/third_party/mgba-0.10.3"; do
		if [ -f "$candidate/include/mgba/core/interface.h" ]; then
			source_dir=$candidate
			break
		fi
	done
fi
if [ -z "$source_dir" ] || [ ! -f "$source_dir/include/mgba/core/interface.h" ]; then
	printf 'mGBA 0.10.3 source not found; set MGBA_SOURCE to its source directory\n' >&2
	exit 1
fi

mgba_version=$($mgba_bin --version 2>&1 || true)
case "$mgba_version" in
	*" 0.10.3 ("*) ;;
	*)
		printf 'capture is pinned to mGBA 0.10.3, found: %s\n' "$mgba_version" >&2
		exit 1
		;;
esac

source_version=$(awk '
	/set\(LIB_VERSION_MAJOR/ { gsub(/\)/, "", $2); major=$2 }
	/set\(LIB_VERSION_MINOR/ { gsub(/\)/, "", $2); minor=$2 }
	/set\(LIB_VERSION_PATCH/ { gsub(/\)/, "", $2); patch=$2 }
	END { print major "." minor "." patch }
' "$source_dir/version.cmake")
if [ "$source_version" != "0.10.3" ]; then
	printf 'MGBA_SOURCE must be mGBA 0.10.3, found %s\n' "$source_version" >&2
	exit 1
fi

runner_dir=$(mktemp -d "${TMPDIR:-/tmp}/mtile-mgba-capture.XXXXXX")
capture_log="$runner_dir/mgba.log"
capture_lib="$runner_dir/mgba-audio-capture.dylib"
runner="$runner_dir/mgba"

cleanup() {
	case "$runner_dir" in
		"${TMPDIR:-/tmp}"/mtile-mgba-capture.*) rm -rf -- "$runner_dir" ;;
	esac
}
trap cleanup EXIT HUP INT TERM

compiler=${CC:-/usr/bin/clang}
if [ ! -x "$compiler" ]; then
	compiler=$(command -v cc || true)
fi
if [ -z "$compiler" ]; then
	printf 'C compiler not found; set CC to Apple clang\n' >&2
	exit 1
fi

"$compiler" -std=c11 -O2 -Wall -Wextra -dynamiclib -undefined dynamic_lookup \
	-I"$source_dir/include" -I"$source_dir/src" \
	-o "$capture_lib" "$script_dir/mgba_audio_capture.c"
cp "$mgba_bin" "$runner"
codesign --force --sign - "$runner" >/dev/null 2>&1

output_dir=$(dirname -- "$output")
mkdir -p "$output_dir"
capture_frames=${MGBA_CAPTURE_FRAMES:-120}
status=0
env DYLD_FORCE_FLAT_NAMESPACE=1 \
	DYLD_INSERT_LIBRARIES="$capture_lib" \
	MGBA_CAPTURE_ROM="$rom" \
	MGBA_CAPTURE_OUTPUT="$output" \
	MGBA_CAPTURE_FRAMES="$capture_frames" \
	"$runner" >"$capture_log" 2>&1 || status=$?
sed -n '/^capture:/p' "$capture_log"
if [ "$status" -ne 0 ]; then
	sed -n '1,160p' "$capture_log" >&2
	printf 'mGBA internal capture failed with status %s\n' "$status" >&2
	exit "$status"
fi
if [ ! -s "$output" ]; then
	printf 'mGBA reported success but did not produce %s\n' "$output" >&2
	exit 1
fi
