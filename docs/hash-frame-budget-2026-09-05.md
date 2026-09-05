# Hash workload: frame budget met

The unchanged 64-key workload now fits the 30 Hz budget in both renderers.
Every one of 600 captured ticks passed the deadline check, with zero missed
ticks, exact numeric checksums, and identical pixels between renderers.

The previous implementation is commit
`d5c96506fa0a61724655b22a155ff7bc6a233871`, including the integer-hash fix
from the [initial audit](performance-audit-2026-09-05.md).

| Renderer | Previous cycles/tick | Current worst tick | Budget remaining | Missed ticks |
| --- | ---: | ---: | ---: | ---: |
| Mode 0 | 598,265 | 282,865 | 278,927 cycles / 49.6% | 0 |
| Mode 4 | 731,907 | 414,899 | 146,893 cycles / 26.1% | 0 |

The deadline is **561,792 cycles**, or two GBA video frames per Lua update.
Previously, this particular workload could not finish an update before that
deadline. Such overruns can slow game updates and produce uneven motion;
they do not indicate data corruption. The figures above describe this
workload, rather than guaranteeing the same margin for other games.

## What changed

The largest remaining cost was bucket selection. ARM7TDMI has no hardware
integer divider, and the hash lookup used a full software remainder operation
on every read and write. This test executed **128 software divisions per
tick**. `runtime/table.go` now uses a bit mask when the capacity is a power
of two. Other capacities retain the original remainder operation, preserving
their exact bucket assignments and collision handling. The runtime-only
change brought Mode 4 down to 463,411 cycles, already within budget.

The transpiler also avoids putting numbers, booleans, nil, and static ROM
strings on the moving heap's protection stack. Heap tables and dynamic
strings remain protected. This saves a further 3,200 cycles in Mode 0 and
2,176 in Mode 4, and removes 792 ROM bytes in each renderer. Existing
compile-time root-capacity checks remain conservative.

These changes and the IWRAM follow-up below preserve the Lua workload, public
API semantics, renderer eligibility, and table layout. Final ROM sizes are
26,140 bytes in Mode 0 and 27,352 bytes in Mode 4. EWRAM use and the
3,188-byte Lua allocation footprint
are unchanged from the previous implementation.

## IWRAM follow-up

`tableData` and `hashFind` now execute from the GBA's fast internal RAM.
Their function bodies are unchanged; `//go:section .iwram` changes placement
and `//go:noinline` keeps the code from being inlined back into ROM callers.
The existing startup code already copies these sections into RAM.

Compared with commit `ad25985c3ac34902f4baeaccbc71ba03d6dcc9ca`, which includes
the mask and root-filter optimizations, placement alone reduces this workload
from 326,897 to 282,865 cycles in Mode 0 (13.47%) and from 461,235 to 414,899
in Mode 4 (10.05%). These measurements include the extra branches needed to
call code across memory regions. ELF inspection verifies the function bodies
at `0x0300...`, with small call veneers remaining in ROM.

The trade is 752 additional IWRAM bytes in the Mode 0 benchmark and 776 in
Mode 4, plus 24 ROM bytes each. IWRAM use depends on the functions retained
by each cartridge: Physics Stress and Pong each add 1,144 bytes, leaving
26,936 and 19,856 bytes free respectively. A Background Layers cartridge
that does not need these helpers has no memory increase. EWRAM use is
unchanged in all three integration controls. IWRAM has a fixed capacity,
and the linker continues to reject cartridges that exceed it.

Pong's separate 120-tick pixel comparison also passes with zero misses,
although its total median cost increases 0.39% and p95 increases 0.29%.
Its draw phase changes only 18 cycles. The source of the small total-cost
increase was not isolated; the hash workload's improvement should not be
generalized to all games. See the
[integration evidence](benchmarks/performance-audit/table-iwram-integration.txt).

## Evidence and acceptance

Tests use TinyGo 0.41.1 and the headless mGBA player, with 20 warm-up ticks
followed by 600 synchronized ticks. The gate checks every tick against the
deadline and verifies that the cumulative missed-tick counter never grows
after warm-up. Both counters also remain zero during warm-up in this run.
The checksum is independently verified through debug telemetry on every
tick, including signed 16:16 wraparound at tick 480.

Raw measurements and tool/workload fingerprints:

- [IWRAM comparison, linked addresses, and 600-tick checks](benchmarks/performance-audit/table-iwram-target.txt)
- [Earlier deadline gate and division evidence](benchmarks/performance-audit/numeric-hash-frame-budget.txt)
- [Runtime-only bucket comparison](benchmarks/performance-audit/numeric-hash-bucket-target.txt)
- [Compiler-only root filtering comparison](benchmarks/performance-audit/hash-root-filter-target.txt)

Run the retained acceptance test from the repository root:

```sh
go test ./cmd/renderbench -tags=runtimeperfdiagnostic \
  -run '^TestRuntimeNumericHashFrameBudget$' -count=1 -v
```

Reproduce the IWRAM comparison against the preceding implementation:

```sh
mkdir -p /tmp/mtile-table-iwram-baseline
git archive ad25985c3ac34902f4baeaccbc71ba03d6dcc9ca runtime | tar -x -C /tmp/mtile-table-iwram-baseline
MTILE_PERF_BASELINE_RUNTIME=/tmp/mtile-table-iwram-baseline/runtime \
  go test ./cmd/renderbench -tags=runtimeperfdiagnostic \
  -run '^TestRuntimeTableIWRAMDiagnostic$' -count=1 -v
```

Regression coverage compares bucket selection with modulo at every supported
nonzero capacity, exercises collisions/tombstones/growth/GC for non-power-of-two
tables, and verifies generated root helpers across collection, nested
allocation, and partial root-stack failure.

Passed: `go test ./internal/transpiler`, `go test ./internal/build ./cmd/mtile`,
`go test ./cmd/renderbench`, runtime `go test ./...`, Mode 0 runtime tests,
the target deadline and IWRAM gates, integration ROM builds, and
`git diff --check`.
