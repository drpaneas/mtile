#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)

cd "$repo_root"
go test ./internal/assetsync -run '^$' -fuzz '^FuzzNativeAudioCompilerNeverPanics$' -fuzztime=1s

cd "$repo_root/runtime"
go test ./audio -run '^$' -fuzz '^FuzzNativeControllerCommandsNeverEscapeFourChannels$' -fuzztime=1s
