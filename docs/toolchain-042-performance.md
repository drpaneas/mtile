# Runtime tuning with TinyGo 0.42

The September 9, 2026 pass retained a small numeric table lookup improvement:
**12.09% fewer frame cycles in the Mode 0 numeric-hash workload and 7.05% fewer
in Mode 4**. It applies to games that use numeric keys in the hash part of a
Lua table. It does not require game changes or alter renderer eligibility.

This change does **not** make Celeste run faster in practice. Its measured
completion route remains at 24.81 updates per second with the software
renderer. The renderer-only control is cycle-identical before and after.

## What changed

The table probe loop already executes in IWRAM. Numeric lookup keys now compare
their exact fixed-point payload there, instead of calling generic equality
through ROM. Testing the lookup key's kind first also lets LLVM avoid the
generic call when a numeric key encounters a different kind in a collision.
Other keys retain the existing equality implementation, including equivalent
static and heap strings. Hashes, buckets, tombstones, iteration order, table
storage and garbage collection are unchanged.

The [runtime patch](benchmarks/toolchain-042/runtime-change.patch) is the entire
measured behavior change. A mixed-key host test compares lookups against an
independent linear scan using generic equality through collisions, growth,
deletion, reinsertion and collection. It covers fractional and extreme numeric
keys, booleans, table identity and static/heap string aliases.

## Paired target measurements

Both sides use **Go 1.27.1, TinyGo 0.42.0 and LLVM 22.1.4**, with `-gc=none
-opt=2`, debug telemetry and the same native mGBA player. This isolates a runtime
optimization under the new toolchain; it is not a comparison against TinyGo
0.41.1. GBA hardware timers report cartridge cycles, not host CPU utilization.

The table and renderer controls capture 600 synchronized logical ticks after
20 warm-up ticks. Their before/after images match at every pixel. The numeric
workload also checks every checksum, including signed fixed-point wraparound.

| Workload | Renderer | Before median | After median | Change |
| --- | --- | ---: | ---: | ---: |
| 64 numeric hash reads and writes per tick | Mode 0 | 197,967 | 174,031 | -12.09% |
| Same numeric hash workload | Mode 4 | 339,705 | 315,769 | -7.05% |
| 16 string hash reads and writes per tick | Mode 0 | 61,309 | 61,661 | +0.57% |
| Same string hash workload | Mode 4 | 197,281 | 197,633 | +0.18% |
| Scrolling map control | Mode 0 | 1,242 | 1,242 | unchanged |
| Same scrolling map control | Mode 4 | 187,039 | 187,039 | unchanged |

The table workloads have identical median, p95 and maximum timings within each
variant. No control misses a frame, changes Lua heap allocation or consumes
additional EWRAM. Numeric cartridges save 16 ROM bytes and 60 IWRAM bytes;
string cartridges add 52 bytes to each. The small string-key slowdown is an
accepted tradeoff, below the diagnostic's 1% p95 regression limit. The benefit
is specific to dynamic numeric hash access, rather than every table operation.

## Celeste control

The 280-update replay includes spawning, the 162-update completion route,
completion UI, restart, death and respawn. Every published image and watched
gameplay value matches. Both variants emit audio without overflow or heap
failure. These are instrumented Mode 4 ROMs of `examples/celeste`.

| Route metric | Before | After |
| --- | ---: | ---: |
| Median frame cycles | 550,243 | 550,283 |
| p95 frame cycles | 643,580 | 643,580 |
| Maximum frame cycles | 668,750 | 665,746 |
| Updates above 561,792-cycle budget | 59 / 162 | 61 / 162 |
| LCD scans for 162 updates | 390 | 390 |
| Displayed updates per second | 24.81 | 24.81 |
| ROM bytes | 362,100 | 362,140 |
| IWRAM bytes | 29,680 | 29,720 |
| EWRAM free bytes | 26,008 | 26,008 |

The route median changes by less than 0.01%, and two extra updates cross the
work-budget threshold without changing the displayed cadence. This is a
regression control, not an FPS improvement claim. Celeste still exceeds the
30 Hz budget on many software-rendered frames.

## Evidence and reproduction

[Toolchain fingerprints](benchmarks/toolchain-042/metadata.json),
[table measurements](benchmarks/toolchain-042/table-target.txt),
[renderer and Celeste measurements](benchmarks/toolchain-042/renderer-celeste-target.txt),
and [Celeste phase summaries](benchmarks/toolchain-042/celeste-summary.json)
are checked in alongside all cycle samples and pixel hashes. The two Celeste
trace files also retain every watched value. Temporary build paths in these
records identify the original run; the ROM hashes identify its outputs.

To repeat the comparison, copy the current runtime into two separate snapshot
directories and reverse `runtime-change.patch` in the baseline only. Build
`build/player/mtile-player` using the repository's native-player instructions,
then run from the repository root with absolute snapshot/output paths:

```sh
MTILE_LLVM22_BASELINE_RUNTIME=/tmp/mtile-before/runtime \
MTILE_LLVM22_CANDIDATE_RUNTIME=/tmp/mtile-after/runtime \
MTILE_LLVM22_OUT=/tmp/mtile-toolchain-comparison \
go test ./cmd/renderbench -tags=runtimeperfdiagnostic \
  -run '^TestToolchain042RuntimeDiagnostic$' -count=1 -timeout=15m -v

(cd runtime && go test ./...)
(cd runtime && go test -tags=mtile_mode0 ./...)
```

Both snapshots must supply the runtime API expected by the current generator.
The diagnostic refuses numeric regressions, checks a 1% p95 limit on controls,
and compares pixels, memory use and missed frames. Celeste phase timings remain
visible for review even when overall display cadence is unchanged.
