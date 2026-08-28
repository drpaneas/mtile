#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
AUDIOCOEXIST_TARGET=audiopcmthreecoexist exec "$script_dir/extract_audiocoexist.sh"
