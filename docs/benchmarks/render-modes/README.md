# Render-mode benchmark evidence

`tile-v1.json` is the checked-in decision record for the first public AUTO
profile. Every workload uses `examples/testio` assets, TinyGo 0.41.1, a pinned
headless mGBA helper, 20 warm-up frames, and 120 synchronized 30 Hz ticks.
Deferred Mode 0 VRAM/OAM work is included in total cycles.

| Workload | Pixel diff | Mode 0 median saving | EWRAM gain | AUTO |
| --- | ---: | ---: | ---: | --- |
| testio static map | 0 | 92.94% | 34,040 B | Mode 0 |
| sub-tile camera | 0 | 89.31% | 34,040 B | Mode 0 |
| map + sspr + text + palette | 0 | 78.71% | 30,952 B | Mode 0 |
| two BG layers | 0 | 93.21% | 29,912 B | Mode 0 |
| 16 dirty tiles/tick | 0 | 84.91% | 34,040 B | Mode 0 |
| 16 map cells/tick | 0 | 85.03% | 34,040 B | Mode 0 |
| runtime flag reclassification | 0 | 15.67% | 34,040 B | Mode 4 |

The final row is not a correctness failure: both renderers are pixel-identical
and miss no frames. It is the measured cost crossover that makes runtime
`fset` a forced-Mode-0-only feature in AUTO profile `tile-v1`.

Reproduce a workload from the repository root:

```sh
go run ./cmd/renderbench \
  -project examples/testio \
  -source cmd/renderbench/testdata/workloads/mset16.lua \
  -player build/player/mtile-player \
  -warmup 20 -samples 120
```

The JSON records exact project, helper, and ROM SHA-256 values. It truthfully
marks the source tree dirty because this record was generated during the
implementation run; CI regenerates clean-tree reports as build artifacts.
