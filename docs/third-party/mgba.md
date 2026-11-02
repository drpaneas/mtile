# mGBA component

Mtile's in-editor **Develop** workspace uses a separate, hidden helper built against
[mGBA](https://mgba.io/). The helper is a narrow headless frontend: the Go
editor sends GBA keys over stdin and receives 240×160 RGBA frames, stereo PCM,
hardware audio/input registers, and the editor-only debug mailbox over stdout.
It does not open an SDL or Qt window.

The source checkout pins mGBA 0.10.3 as the `third_party/mgba` Git submodule.
mGBA is licensed under the Mozilla Public License 2.0. Mtile's own source
remains under its project license; modifications to MPL-covered mGBA files, if
any, must remain available under MPL-2.0. The helper frontend in
`cmd/mtile-player` is Mtile source. Its CMake build applies one narrowly
scoped compatibility change to a generated copy of mGBA's `gba.c`: HALT
must remain awake when `IE & IF` already contains an enabled pending request,
independently of IME. The pinned submodule itself remains unchanged.

The exact transformation is in
[`mgba_halt_compat.cmake`](../../cmd/mtile-player/mgba_halt_compat.cmake).
The generated source retains its MPL-2.0 notice. Release bundles include
the transformation as `licenses/MGBA-HALT-COMPAT.cmake`; corresponding
source is the pinned upstream revision plus this transformation.
`MTILE_MGBA_HALT_COMPAT=OFF` reproduces the unmodified core for diagnostics.

Release builds must ship the mGBA license and make the exact corresponding
mGBA source (the pinned submodule revision plus any patches) available.

Build the helper locally with:

```sh
cmake -S cmd/mtile-player -B build/player -G Ninja -DCMAKE_BUILD_TYPE=Release
cmake --build build/player --target mtile-player
```

To verify the pending-request hardware contract, build `mtile-halt-test`
and run `ctest --test-dir build/player --output-on-failure`. Its 20 cases
include enabled/disabled requests with both IME and CPSR IRQ masking states.
