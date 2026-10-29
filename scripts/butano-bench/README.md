# Butano and mTile benchmark at 30 Hz

This suite compares public C++ Butano APIs with the public mTile Lua API at
one update per two GBA LCD refreshes. It does not change engine behavior or
add public Lua builtins. The checked-in [report](../../docs/benchmarks/butano-30hz/README.md)
contains the results, limitations, and raw evidence.

Requirements: the normal mTile Go/TinyGo toolchain, devkitARM 65, a C compiler,
Python with NumPy and Matplotlib, and the existing headless mGBA player. The
scripts default to macOS devkitPro paths under `/opt/devkitpro`. Asset data is
generated directly in each engine's native format; grit is not required.

```sh
git clone https://github.com/GValiente/butano.git /private/tmp/mtile-butano-20261029
git -C /private/tmp/mtile-butano-20261029 checkout 04374b034c76b82bfdc3aa6c38d652e5e7778a68
cmake -S cmd/mtile-player -B build/player
cmake --build build/player --target mtile-player
python3 scripts/butano-bench/run.py --out /private/tmp/mtile-butano-final --samples 600 --warmup 60 --repeat 2
python3 scripts/butano-bench/verify.py /private/tmp/mtile-butano-final
python3 scripts/butano-bench/memory.py /private/tmp/mtile-butano-final
python3 scripts/butano-bench/audio.py
python3 scripts/butano-bench/calibrate.py
python3 scripts/butano-bench/report.py /private/tmp/mtile-butano-final docs/benchmarks/butano-30hz
```

`run.py --only REGEX` selects a focused subset. `--work`, `--butano` and
`--out` override the primary suite's paths. Audio and calibration use the
default `/private/tmp` paths shown above. `report.py` intentionally requires
the complete 38-workload, two-repetition dataset and passing oracle checks.
The Butano checkout must be a disposable benchmark copy: the suite adds only
the stopwatch hooks recorded in `butano-timing.patch`.

The benchmark builds use ordinary release optimizations. mTile retains its
debug mailbox; Butano writes the same mailbox outside its measured work.
Butano's F/1 stopwatch excludes intentional VBlank waiting and adds interrupt
body work back. `calibrate.py` measures the zero-work stopwatch control.
Raw numbers are never corrected by subtracting a baseline.

`driver.go` builds the mTile ROMs and captures the first completed scan for
each mailbox tick. Both engines use zero image lag. Capturing the last scan
instead can pick up the next Butano image. `verify.py` independently renders
the expected pixels and simulates signed Q16.16 state for every measured tick
in both repetitions. A cross-engine screenshot match alone is insufficient.
`recapture.py` can remeasure existing ROMs without rebuilding them.

For instruction attribution and unchanged-source performance fixes:

```sh
python3 scripts/butano-bench/profile.py /path/game.gba /path/game.gba.game.elf /private/tmp/profile
python3 scripts/butano-bench/recheck.py --baseline /private/tmp/mtile-butano-final --projects /private/tmp/mtile-butano-bench/projects --out /private/tmp/mtile-after --work /private/tmp/mtile-recheck
python3 scripts/butano-bench/verify.py /private/tmp/mtile-after
```

`profile.py` defaults to 60 warm-up ticks and 120 profiled updates. It steps the
original ROM, records instruction PCs/cycles and reconciles hardware-event time
separately, then uses ELF/DWARF inline frames to distinguish waits, debug writes
and useful/other instructions. It currently bins the first 1 MiB of ROM and
IWRAM; all other instruction addresses remain a separate counted category.
Use mailbox captures for release performance totals; profiling percentages are
exclusive instruction attribution. `recheck.py` refuses altered project/asset
manifests and rebuilds only mTile, twice capturing 600 ticks per renderer.
`--only REGEX` selects a subset. Original Butano/baseline ROMs are preserved.

`native/inspect.c` reads VRAM/OAM using the same pinned mGBA core. Its referenced
byte counts are lower bounds on current hardware references, not allocator
watermarks. `memory.py` records one snapshot at tick 660, including enabled
display layers and active OBJ entries. Mode 4 double-buffer pages are reported
as an 80 KiB reservation separately.

The two PSG probes deliberately have a separate result class. The engines
schedule PSG writes at different clocks; neither register timing nor PCM phase
is proven equal. These probes must not contribute to equal-output speedup
rankings. The mTile PSG service counters are reported independently because
some timer work runs during deliberately unmeasured frame waits.

Procedural graphics and palettes are shared by both implementations. Text uses
the repository's public-domain Misc Fixed 6x10 artwork; see
[`fonts/misc-fixed/LICENSE`](../../fonts/misc-fixed/LICENSE). Butano and its
dependencies retain their [upstream licenses](https://github.com/GValiente/butano/tree/master/licenses).
Physical-GBA confirmation is a separate outstanding validation step.
