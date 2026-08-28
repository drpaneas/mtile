#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
AUDIO_HARDWARE_PROFILE=standard exec "$script_dir/extract_audiohardware.sh"
