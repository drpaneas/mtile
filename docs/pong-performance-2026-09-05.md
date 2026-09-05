# Pong performance, 2026-09-05

Pong now spends **19.7% fewer CPU cycles per game tick** after drawing and
compiler optimizations. The game source, music, sound effects, Mode 4 renderer,
and 30 Hz update cadence are unchanged. Each update still runs two simulation
steps. These are headless mGBA target measurements, not host Go timings or a
measurement on a physical cartridge.

## Changes

The court outline previously transformed and clipped both vertical edges one
pixel at a time. Each center dash used the general Bresenham line loop. The
runtime now clips axis-aligned lines once: horizontal lines use the existing
packed row filler, and vertical lines use a small byte-store loop in IWRAM.
Rectangles share this vertical path. Diagonal line stepping is unchanged.

The vertical loop writes the EWRAM framebuffer with a constant 240-byte stride.
It uses the same compiler and CPU instruction set as the rest of the runtime;
there is no new renderer or hardware peripheral. Full-screen clearing and
presentation already use DMA3 and are unchanged.

The compiler also resolves numeric table initializers using lexical scope.
Pong initializes paddle dimensions and speeds from numeric locals; recognizing
those bindings enables native fixed-point arithmetic in more field operations.
Previously, the analysis consulted only global names. That also miscompiled a
local string or parameter shadowing a numeric global: a field initialized with
`"wide"` could later read as numeric zero. Scoped lookup fixes the bug and
retains generic values for unknown or mixed initializers.

Primitive opacity, palette remapping, current draw color, inclusive endpoints,
reversed endpoints, camera, clipping, and degenerate shapes retain their
semantics. No public Lua API or renderer eligibility changed.

## Target results

All debug ROMs use the existing table-helper IWRAM optimization. Two paired
comparisons isolate drawing changes with identical generated Go, then the
compiler fix with the optimized runtime. The pinned player captures 600
synchronized game ticks after 20 warm-up ticks, with no button input and the
original audio assets enabled.

| Metric | Before this work | Drawing only | Final |
| --- | ---: | ---: | ---: |
| Median total cycles | 263,509 | 215,034 | 211,544 |
| p95 total cycles | 270,744 | 221,113 | 216,848 |
| p99 total cycles | 301,736 | 252,872 | 248,944 |
| Maximum total cycles | 406,979 | 359,240 | 355,859 |
| Median draw cycles | 159,440 | 111,214 | 110,947 |
| Median framebuffer copy cycles | 78,102 | 78,102 | 78,102 |
| Missed ticks | 0 | 0 | 0 |
| Debug ROM size | 93,696 B | 93,172 B | 89,372 B |
| IWRAM used, including stacks | 12,816 B | 12,908 B | 12,908 B |
| EWRAM used | 184,808 B | 184,808 B | 184,808 B |
| Final heap allocated | 8,872 B | 8,872 B | 8,872 B |
| Garbage collections | 0 | 0 | 0 |

The final median uses 37.7% of the 561,792-cycle game-tick budget, down from
46.9%. Total cycles fall 19.7%, draw cycles fall 30.4%, and the debug ROM shrinks
4,324 bytes. The renderer still clears and copies a complete framebuffer, so
those costs remain. This improves CPU headroom; it does not raise the configured
game rate or reduce EWRAM usage.

All 600 captured frames have identical pixels and watched values. The new host
regression test compares 840 complete framebuffers against independent geometric
predicates, covering clipping, camera, palette, opaque zero, reversed endpoints,
and one-pixel shapes. It passes against both original and optimized runtime
implementations. Invalid-color tests also preserve draw state and pixels.

Executable compiler tests verify shadowed local/parameter/block bindings,
numeric local aliases, and conservative mixed-field fallback.

Required transpiler/build/CLI tests, runtime tests, the Mode 0 runtime check,
and the renderbench package tests all pass. Audio assets remain
enabled in the target run; this test compares pixels and telemetry, not PCM
waveform identity.

## Reproduction

For the drawing comparison, make a baseline runtime from the candidate tree,
restoring only the two drawing files from the previous revision. This retains
the preceding table optimization on both sides:

```sh
mkdir -p /tmp/mtile-pong-baseline
cp -R runtime /tmp/mtile-pong-baseline/runtime
git show ad25985c3ac34902f4baeaccbc71ba03d6dcc9ca:runtime/draw.go > /tmp/mtile-pong-baseline/runtime/draw.go
git show ad25985c3ac34902f4baeaccbc71ba03d6dcc9ca:runtime/camera.go > /tmp/mtile-pong-baseline/runtime/camera.go
MTILE_PERF_BASELINE_RUNTIME=/tmp/mtile-pong-baseline/runtime \
  go test ./cmd/renderbench -tags=runtimeperfdiagnostic \
  -run '^TestRuntimePongDiagnostic$' -count=1 -v
```

Use a fresh baseline directory. `MTILE_PERF_SAMPLES=120` gives a shorter
diagnostic run; the default is 600. The test rejects changed generated Go,
different pixels or watches, added misses, exceeded tick budgets, changed EWRAM
or heap activity, and median or p95 performance regressions.

This runs both ROMs through the current compiler. The drawing-only figures
above were captured before the separate compiler change; its final arithmetic
lowering can slightly change the timings in a reproduction.

For a comparison across compiler revisions, copy
`cmd/renderbench/pong_compiler_diagnostic_test.go` into the baseline checkout
and run with `MTILE_PONG_SNAPSHOT=/tmp/pong-before.json`. Run the same test in
the candidate checkout with another output path and the saved baseline:

```sh
MTILE_PONG_SNAPSHOT=/tmp/pong-after.json MTILE_PONG_COMPARE=/tmp/pong-before.json \
  go test ./cmd/renderbench -tags=runtimeperfdiagnostic \
  -run '^TestPongCompilerDiagnostic$' -count=1 -v
```

To isolate this compiler change, the baseline should use `transpiler.go` from
`ad25985c3ac34902f4baeaccbc71ba03d6dcc9ca` and the candidate runtime. Keep
project assets, helper, and toolchain fixed. Each snapshot contains generated
Go, frame numbers, pixel hashes, watches, cycle samples, and memory results.

Tool versions, source hashes, and raw output are in
[`pong-axis-target.txt`](benchmarks/performance-audit/pong-axis-target.txt) and
[`pong-scope-target.txt`](benchmarks/performance-audit/pong-scope-target.txt).
