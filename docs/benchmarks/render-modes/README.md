# Render-mode benchmark evidence

`tile-v1.json` is the checked-in decision record for the first public AUTO
profile; `zero-bg-text-v1.json` adds the fresh-project mapless workload;
`coin-block-v1.json` records the complete current-API background-state example;
and `background-layers-stage2-v1.json` records the original explicit BG0-BG3
slice. `background-layers-map-contract-v2.json` re-runs the four-plane workload
after the first-argument-BG API migration and dense allocation work;
`map-plane-packing-host-v1.json` isolates the two-of-four plane cost;
`packed-sprite16-v1.json` and `packed-sprite32-v1.json` record the immutable
square hardware-OBJ lowerings; and `oam64-v1.json` plus `scanline38-v1.json`
record the full OAM and near-limit scanline proofs.
`dynamic-sprite-frames-v1.json` records non-contiguous runtime frame selection
from the statically enumerated set `{1,32,35}`;
`inactive-sprite-slots-v1.json` records the bounded `{0,1}` inactive-slot path;
`object-depth-lanes-v1.json` records four interleaved BG-relative OBJ lanes;
`glyph-intern-v1.json` records 32 text occurrences sharing one static tile;
and `layered-map-access-v1.json` records the layer-first `mget`/`mset` API,
legacy hot-path controls, literal-layer AUTO bound, and dynamic-layer holdback.
The original matrix, packed-sprite, and OAM-budget records use
`examples/testio`; the other focused records use their own authored projects.
Every record uses TinyGo 0.41.1, a pinned headless mGBA helper, 20 warm-up
frames, and 120 synchronized 30 Hz ticks. Deferred Mode 0 VRAM/OAM work is
included in total cycles.

| Workload | Pixel diff | Mode 0 median saving | EWRAM gain | AUTO |
| --- | ---: | ---: | ---: | --- |
| backdrop + OBJ text, no BG | 0 | 49.84% | 35,032 B | Mode 0 |
| 32 text occurrences, one interned glyph/color tile | 0 | 57.91% | 37,016 B | Mode 0 |
| testio static map | 0 | 92.94% | 34,040 B | Mode 0 |
| sub-tile camera | 0 | 89.31% | 34,040 B | Mode 0 |
| map + sspr + text + palette | 0 | 78.71% | 30,952 B | Mode 0 |
| two BG layers | 0 | 93.21% | 29,912 B | Mode 0 |
| four authored BG planes + parallax | 0 | 90.46% | 18,564 B | Mode 0 |
| four planes, new `map(bg,...)` contract | 0 | 97.22% | 21,652 B | Mode 0 |
| animated coin block + one state transition | 0 | 66.36% | 30,952 B | Mode 0 |
| eight packed 16x16 sprites | 0 | 72.13% | 37,064 B | Mode 0 |
| two packed 32x32 sprites | 0 | 77.82% | 37,056 B | Mode 0 |
| 64 distributed 8x8 sprites | 0 | 31.96% | 37,088 B | Mode 0 |
| 38 packed 32x32 sprites at 1,216 line cycles | 0 | 92.01% | 37,056 B | Mode 0 |
| 16 bounded runtime sprites, frames `{1,32,35}` | 0 | 60.20% | 37,088 B | Mode 0 |
| 64 bounded runtime slots, frames `{0,1}` | 0 | 59.75% | 37,088 B | Mode 0 |
| four BG-relative object-depth lanes | 0 | 95.82% | 20,616 B | Mode 0 |
| 16 dirty tiles/tick | 0 | 84.91% | 34,040 B | Mode 0 |
| 16 map cells/tick | 0 | 85.03% | 34,040 B | Mode 0 |
| 16 BG1 reads/tick (`mget(1,...)`) | 0 | 97.19% | 21,652 B | Mode 0 |
| 16 BG1 writes/tick (`mset(1,...)`) | 0 | 90.79% | 34,040 B | Mode 0 |
| dynamic-layer read/write, one per tick | 0 | 96.96% | 29,912 B | Mode 4 pending widest-shape proof |
| runtime flag reclassification | 0 | 15.67% | 34,040 B | Mode 4 |

The coin-block capture includes active tile animation, its deterministic hit,
the one-time coin award, and the stable used state. Mode 0 measured 102,790
median cycles versus 305,554 for Mode 4 and used an 8,540-byte smaller ROM. Its
3,273,893-cycle cold upload is the material cost: the trade is favorable for a
long-lived tile scene, but should not be generalized to frequent scene reloads.

The Stage 2 four-plane capture measured 114,788 median cycles in Mode 0 versus
1,203,503 in Mode 4, an 8,116-byte smaller ROM, and exact pixels across all 120
moving parallax frames. Its parity gate passes. The generic full gate remains
false because four 4 KiB hardware screen shadows reduce the framebuffer memory
gain to 18,564 bytes (below the one-size 24 KiB threshold) and the initial Mode
0 cold upload records one cumulative miss; Mode 4 misses 140 frames on the same
workload. This is an accepted multi-BG trade: explicit planes add no map cells
over the former four stacked strips, Mode 0 still returns memory, and it removes
about 90% of steady per-frame work. The project remains forced Mode 0 so that
this teaching example does not hide that choice.

The v2 migration capture uses the checked-in `parallax_layers.lua` source,
whose first argument is always BG0..BG3 and whose source rows contain the real
stars, mountains, hills, and ground planes. Across 120 exactly synchronized
runtime frames it records zero differing pixels, including the frame where all
four source windows wrap together. Mode 0 measures 33,382 median cycles versus
1,199,670 for Mode 4 (97.22% saving) and returns 21,652 EWRAM bytes. Its parity
gate passes; the full gate remains intentionally false because the run records
two cumulative Mode 0 missed frames and remains below the generic 24 KiB memory
threshold. This keeps the cold/upload-boundary trade-off visible without
accepting an ordinally misaligned capture as pixel evidence.

The host packing benchmark compares four full planes with the sparse logical
set BG1+BG3. Median work falls from 167,554 ns/op to 82,937 ns/op (50.50%) with
zero allocations. For a 60x20 map the build omits 4,800 bytes of empty map
payload from ROM and small-map EWRAM, and Mode 0 omits two 4 KiB screen
shadows: 12,992 EWRAM bytes saved before small per-layer metadata arrays.

The glyph-intern capture draws 32 copies of `A` in color 7. Mode 0 reports the
same 32 OAM entries and 256 worst-line cycles as the former implementation,
but stores and uploads one unique tile. Its median falls from the recorded
162,434-cycle pre-interning baseline to 71,117 cycles (56.22%); draw work falls
from 94,901 to 16,062 cycles (83.08%). Compared with Mode 4 it saves 57.91%,
returns 37,016 EWRAM bytes, remains pixel-exact, and misses no frames.

The final row is not a correctness failure: both renderers are pixel-identical
and miss no frames. It is the measured cost crossover that makes runtime
`fset` a forced-Mode-0-only feature in AUTO profile `tile-v1`.

The layered-map record separates compatibility from performance. Legacy
`mget(x,y)`/`mset(x,y,id)` still lower to their original BG0 runtime methods.
Five-run host medians show warm streamed BG1 access within 1.1% for reads and
6.3% for changed writes versus BG0, with zero allocations; compact BG1 access
pays the small logical-to-dense-plane lookup (3.773 ns/read and 3.966 ns/write).
On ARM7TDMI, all three 120-frame workloads are pixel-exact with ample p95
headroom. Literal-layer writes are therefore admitted under the existing
16-direct-calls bound. The dynamic workload proves exact forced Mode 0 behavior
for one write per tick, but AUTO remains conservative until a 16-write dynamic
target workload is measured.

The packed-sprite captures additionally compare the Mode 0 implementation with
commit `e2fc183` immediately before the cache. For eight draws of one immutable
16x16 source, packing reduces median total work from 66,914 to 61,828 cycles
(7.60%) and draw work from 18,148 to 11,292 cycles (37.78%). For two distinct
32x32 sources, it reduces total work from 63,259 to 48,318 cycles (23.62%) and
draw work from 14,302 to 3,214 cycles (77.53%). The 32x32 result costs 1,084 ROM
bytes and 32 EWRAM bytes relative to its baseline. The renderer continues to
charge the true 16/32-pixel scanline width, while cached rectangles consume one
OAM entry.

The OAM records cover the newly widened AUTO profile. `oam64.lua` uses 64
separate OAM entries distributed over eight rows and remains exact with no
missed frames in either renderer. `scanline38.lua` uses one cached 32x32 source
38 times on the same scanlines: 38 OAM entries and 1,216 of the available 1,226
normal-OBJ cycles. It is pixel-exact and misses no Mode 0 frames. Its Mode 4
capture misses 140 cumulative ticks, so the parity gate passes while the full
missed-frame gate remains false; the record characterizes the hardware boundary
rather than claiming a fair Mode 4 throughput comparison.

The bounded runtime-frame capture changes one scalar ID among the explicit,
non-contiguous constants 1, 32, and 35 while drawing sixteen 8x8 objects. All
120 synchronized frames are pixel-identical. Mode 0 measures 69,588 median
cycles versus 174,842 for Mode 4 (60.20% saving), returns 37,088 EWRAM bytes,
uses 16 OAM entries and 32 worst-line OBJ cycles, and misses no frames in
either renderer. The 3,400,790-cycle Mode 0 cold upload is recorded separately
from the steady-state median.

The inactive-slot capture alternates 64 one-cell calls between the explicit
IDs 0 and 1. All 120 synchronized frames are pixel-identical. The compiler
continues to report the conservative 64-entry/64-cycle maximum, while runtime
tests separately prove that the fully transparent ID-0 frames emit no OAM
entries. Mode 0 measures 68,920 median cycles versus 171,235 for Mode 4
(59.75% saving), returns 37,088 EWRAM bytes, and misses no frames in either
renderer.

The object-depth capture draws one sprite after each of four non-empty map
passes over the checked-in background-layer art. Mode 0 writes OBJ priorities
3/2/1/0 while Mode 4 paints the same statements immediately. All 120 frames
are pixel-identical. Mode 0 measures 50,854 median cycles versus 1,215,615 for
Mode 4 (95.82% saving), returns 20,616 EWRAM bytes, and uses four OAM entries
with a 16-cycle worst line. The parity gate passes; the full gate remains false
because four-layer cold initialization records one Mode 0 miss and the
deliberately raster-heavy Mode 4 side records 140 cumulative missed ticks.

Reproduce a workload from the repository root:

```sh
go run ./cmd/renderbench \
  -project examples/coin-block \
  -player build/player/mtile-player \
  -warmup 20 -samples 120
```

Reproduce the four-plane Stage 2 parity decision:

```sh
go run ./cmd/renderbench \
  -project examples/background-layers \
  -source cmd/renderbench/testdata/workloads/parallax_layers.lua \
  -player build/player/mtile-player \
  -warmup 20 -samples 120 -gate parity
```

Reproduce the isolated host packing comparison:

```sh
cd runtime
go test -run '^$' \
  -bench 'BenchmarkMapAll(SparseTwoOfFour|Four)$' \
  -benchmem -count=5 .
```

The JSON records exact project, helper, and ROM SHA-256 values. It truthfully
marks the source tree dirty because this record was generated during the
implementation run; CI regenerates clean-tree reports as build artifacts.
