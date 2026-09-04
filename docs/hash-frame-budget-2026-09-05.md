# Hash workload: frame budget met

The unchanged 64-key workload now fits the 30 Hz budget in both renderers.
Every one of 600 captured ticks passed the deadline check, with zero missed
ticks, exact numeric checksums, and identical pixels between renderers.

The previous implementation is commit
`d5c96506fa0a61724655b22a155ff7bc6a233871`, including the integer-hash fix
from the [initial audit](performance-audit-2026-09-05.md).

| Renderer | Previous cycles/tick | Current worst tick | Budget remaining | Missed ticks |
| --- | ---: | ---: | ---: | ---: |
| Mode 0 | 598,265 | 326,897 | 234,895 cycles / 41.8% | 0 |
| Mode 4 | 731,907 | 461,235 | 100,557 cycles / 17.9% | 0 |

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

Neither change alters the Lua workload, public API semantics, renderer
eligibility, or table layout. Final ROM sizes are 26,116 bytes in Mode 0 and
27,328 bytes in Mode 4. EWRAM use and the 3,188-byte Lua allocation footprint
are unchanged from the previous implementation.

## Evidence and acceptance

Tests use TinyGo 0.41.1 and the headless mGBA player, with 20 warm-up ticks
followed by 600 synchronized ticks. The gate checks every tick against the
deadline and verifies that the cumulative missed-tick counter never grows
after warm-up. Both counters also remain zero during warm-up in this run.
The checksum is independently verified through debug telemetry on every
tick, including signed 16:16 wraparound at tick 480.

Raw measurements and tool/workload fingerprints:

- [Final deadline gate and assembly evidence](benchmarks/performance-audit/numeric-hash-frame-budget.txt)
- [Runtime-only bucket comparison](benchmarks/performance-audit/numeric-hash-bucket-target.txt)
- [Compiler-only root filtering comparison](benchmarks/performance-audit/hash-root-filter-target.txt)

Run the retained acceptance test from the repository root:

```sh
go test ./cmd/renderbench -tags=runtimeperfdiagnostic \
  -run '^TestRuntimeNumericHashFrameBudget$' -count=1 -v
```

Regression coverage compares bucket selection with modulo at every supported
nonzero capacity, exercises collisions/tombstones/growth/GC for non-power-of-two
tables, and verifies generated root helpers across collection, nested
allocation, and partial root-stack failure.

Passed: `go test ./internal/transpiler`, `go test ./internal/build ./cmd/mtile`,
`go test ./cmd/renderbench`, runtime `go test ./...`, Mode 0 runtime tests,
the target deadline gate, and `git diff --check`.
