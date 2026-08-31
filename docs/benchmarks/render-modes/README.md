# Render-mode benchmark evidence

`tile-v1.json` is the checked-in decision record for the first public AUTO
profile; `zero-bg-text-v1.json` adds the fresh-project mapless workload;
`coin-block-v1.json` records the complete current-API background-state example;
and `background-layers-stage2-v1.json` records the original explicit BG0-BG3
slice. `background-layers-map-contract-v2.json` re-runs the four-plane workload
after the first-argument-BG API migration and dense allocation work;
`map-plane-packing-host-v1.json` isolates the two-of-four plane cost. The
original matrix uses `examples/testio`; the focused records use
their own authored projects. Every record uses TinyGo 0.41.1, a pinned headless mGBA
helper, 20 warm-up frames, and 120 synchronized 30 Hz ticks. Deferred Mode 0
VRAM/OAM work is included in total cycles.

| Workload | Pixel diff | Mode 0 median saving | EWRAM gain | AUTO |
| --- | ---: | ---: | ---: | --- |
| backdrop + OBJ text, no BG | 0 | 49.84% | 35,032 B | Mode 0 |
| testio static map | 0 | 92.94% | 34,040 B | Mode 0 |
| sub-tile camera | 0 | 89.31% | 34,040 B | Mode 0 |
| map + sspr + text + palette | 0 | 78.71% | 30,952 B | Mode 0 |
| two BG layers | 0 | 93.21% | 29,912 B | Mode 0 |
| four authored BG planes + parallax | 0 | 90.46% | 18,564 B | Mode 0 |
| four planes, new `map(bg,...)` contract | 0 | 97.22% | 21,652 B | Mode 0 |
| animated coin block + one state transition | 0 | 66.36% | 30,952 B | Mode 0 |
| 16 dirty tiles/tick | 0 | 84.91% | 34,040 B | Mode 0 |
| 16 map cells/tick | 0 | 85.03% | 34,040 B | Mode 0 |
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

The final row is not a correctness failure: both renderers are pixel-identical
and miss no frames. It is the measured cost crossover that makes runtime
`fset` a forced-Mode-0-only feature in AUTO profile `tile-v1`.

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
