# Runtime performance audit — 2026-09-05

Two measured optimizations are implemented. Public Lua APIs and renderer
eligibility are unchanged. The baseline is commit
`25383ae05f43f7729b16e9003cae21a196115c28`.

## GBA target results

Paired runs use TinyGo 0.41.1, the same generated Go and assets, and the
headless mGBA player. Each comparison discards 20 warm-up ticks and compares
120 ticks aligned by runtime frame number.

| Workload | Renderer | Median cycles before → after | Reduction | Pixel differences |
| --- | --- | ---: | ---: | ---: |
| 64 negative integer keys, dynamic reads and writes | Mode 0 | 943,697 → 598,265 | 36.60% | 0 |
| Same hash workload | Mode 4 | 1,079,967 → 731,907 | 32.23% | 0 |
| Scrolling far into a large map, partial clip | Mode 4 | 603,391 → 388,066 | 35.69% | 0 |

The scrolling-map workload's p95 falls from 616,929 to 398,839 cycles, below
the 561,792-cycle 30 Hz tick budget. Its missed-tick counter falls from 140
to one cold-start miss. The concentrated hash workload remains over budget
in both versions; its unchanged 140 misses are not evidence of restored
frame rate. These are workload-specific savings, not whole-game forecasts.

EWRAM use is unchanged in every comparison. The hash workload also preserves
its 3,188-byte Lua allocation footprint and performs no collections. ROM size
changes by +68 bytes for the Mode 0 hash case and −48 bytes for each Mode 4
case. Raw output, tool versions, and reproduction details are in
[numeric-hash-target.txt](benchmarks/performance-audit/numeric-hash-target.txt)
and [map-target-before-after.txt](benchmarks/performance-audit/map-target-before-after.txt).

## Changes and correctness

`runtime/value.go` previously hashed numeric keys using their raw 16:16
payload. Every integer has 16 zero fractional bits, so integer keys outside
the array portion all entered the same bucket at power-of-two table
capacities. Folding the integer bits into the fractional bits removes that
systematic collision. Equal keys still have equal hashes. The unspecified
hash-table iteration order can change.

`runtime/maprender.go` previously visited offscreen cells before rejecting
them inside the loops. Default extents grow with the camera offset, making
the walk more expensive farther into the world. The revised walk intersects
the requested source rectangle with the map and clip once, then visits only
intersecting cells in the original order. Partial tiles, negative source
coordinates, flag masks, palettes, transparency, and streaming are preserved.

Regression tests cover numeric-key growth, deletion, GC, fractions, signed
extremes, and bucket distribution. Map tests compare 256 deterministic
camera/clip/source combinations against independent sprite draws. Target
fixtures also assert visible changes between frames so empty captures cannot
pass parity accidentally.

Host benchmarks remain allocation-free. Negative-key lookup falls from a
five-run median of 96.88 ns to 13.91 ns (about 7× faster); updates fall from
96.53 ns to 14.56 ns. The host far-camera map workload falls from 15,700 ns
to 7,963 ns. Host timings characterize the operation, while the GBA
measurements above establish the target benefit. Raw host runs are retained
beside the target evidence.

## Other findings

Small constant powers looked wasteful in generated Go, but a paired target
experiment showed identical ROM sizes and cycles, consistent with LLVM
already simplifying the helper. The proposed transpiler specialization was
removed because no target benefit was established. See
[literal-power-target.txt](benchmarks/performance-audit/literal-power-target.txt).

Disassembly also showed four byte accesses and four bounds checks in each
`NumberArrayGet`/`NumberArraySet` payload operation. Aligned word access is a
candidate for a separate measurement; it is not part of this change and no
speedup is claimed for it.

## Reproduction and validation

Export the original runtime and run the retained target diagnostics from the
repository root:

```sh
mkdir -p /tmp/mtile-perf-baseline
git archive 25383ae05f43f7729b16e9003cae21a196115c28 runtime | tar -x -C /tmp/mtile-perf-baseline
MTILE_PERF_BASELINE_RUNTIME=/tmp/mtile-perf-baseline/runtime \
  go test ./cmd/renderbench -tags=runtimeperfdiagnostic \
  -run '^TestRuntime(NumericHash|MapClipping)Diagnostic$' -count=1 -v
```

Host benchmarks run from `runtime/`:

```sh
go test -run '^$' -bench 'Benchmark(TableNumericHash|MapClippedExtent)$' -benchmem -count=5 .
```

Validation passed: root `go test ./...`, runtime `go test ./...`, Mode 0
runtime tests, focused compact/streaming map tests, target diagnostics, and
`git diff --check`. The root suite includes the required transpiler renderer
classification/documentation gates, build tests, and CLI ROM-build tests.
Existing user changes in `cmd/editor` were left untouched.
