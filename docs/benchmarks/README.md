# Benchmark records

Files under this directory are recorded measurements. Their project paths,
absolute host paths, and SHA-256 snapshots describe the tree at the time of
each run and are intentionally left unchanged.

The [Butano and mTile comparison at 30 Hz](butano-30hz/README.md) includes
38 workloads, independent pixel/state verification, cycle and memory records,
and a reusable benchmark runner.

The subsequent [physics and map-streaming investigation](../performance-improvements-2026-10-29.md)
records instruction profiles, targeted fixes and unchanged-source before/after
verification separately from that baseline.

The next [indexed-store argument optimization](table-store-arguments-2026-10-29/README.md)
brings the unchanged 128-entity simulation inside the 30 Hz budget and records
the remaining hash-store tradeoff.

Several engineering projects have since moved out of `examples/`. When a
record names one of the old paths, use the current location below.

| Recorded path | Current location |
| --- | --- |
| `examples/testio` | [`testdata/projects/testio`](../../testdata/projects/testio) |
| `examples/color` | [`testdata/projects/color`](../../testdata/projects/color) |
| `examples/platformer` | [`testdata/projects/platformer`](../../testdata/projects/platformer) |
| `examples/audio-gb-editor` | [`testdata/projects/audio-gb-editor`](../../testdata/projects/audio-gb-editor) |
| `examples/physics-stress` | [`testdata/projects/physics-stress`](../../testdata/projects/physics-stress) |
| `examples/function-physics-stress` | [`testdata/projects/function-physics-stress`](../../testdata/projects/function-physics-stress) |
| `examples/multi-return-physics-stress` | [`testdata/projects/multi-return-physics-stress`](../../testdata/projects/multi-return-physics-stress) |
| `examples/noalloc-call-stress` | [`testdata/projects/noalloc-call-stress`](../../testdata/projects/noalloc-call-stress) |
| `examples/parallax-rectangles` | [`testdata/projects/parallax-rectangles`](../../testdata/projects/parallax-rectangles) |
| `examples/celeste-mode0` | [`research/celeste-mode0`](../../research/celeste-mode0) |
| `examples/celeste-parallax` | [`research/celeste-parallax`](../../research/celeste-parallax) |
| `examples/loop` | [`research/loop`](../../research/loop) |
| `examples/multimap`, `examples/print` | Removed; recover from git history if needed |

See [engineering fixtures](../../testdata/projects/README.md) and
[research projects](../../research/README.md) for what each project is for.
