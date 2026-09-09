# Division performance, September 8, 2026

The measured Celeste Mode 0 route uses **2.59% fewer total frame cycles** and
**4.90% fewer update cycles** after this pass. Its 239 recorded ticks retain
identical gameplay watches and LCD pixels. This comparison starts from the
reviewed, uncommitted working tree, which already placed division helpers in
IWRAM. It measures the additional algorithm and call-removal improvements,
not the earlier move out of ROM.

Measurements use TinyGo 0.41.1 / LLVM 20.1.1, Go 1.26.4, and the repository's
pinned mGBA core. Source fingerprints, ROM hashes, and detailed cycle/memory
results are in [the target evidence](benchmarks/division/target-2026-09-08.json).
These are emulated GBA CPU cycles, not host execution timings.

## What changed

`BorrowMapLayerRead` previously divided the map backing length twice to prove
that a complete plane existed. Checked full-width multiplication proves the
same bounds without software division. `math/bits.Mul` detects overflow on
both 32-bit targets and 64-bit hosts; TinyGo emits ARM `UMULL` instructions.
The isolated change reduced the route's total cycles by 2.41%.

The four signed/unsigned 32/64-bit EABI division helpers now normalize the
divisor and visit only the quotient bits that can be nonzero. Early exits
handle zero, one, a smaller numerator, and a quotient of one. The 32-bit
power-of-two path uses a shift and a remainder mask. Signed wrappers retain
toward-zero quotient/remainder semantics, minimum-value wrapping, and aligned
stack frames. The helpers execute in IWRAM; their ROM load images remain in
the cartridge so startup can copy the code. Unused sections are discarded.

Literal fractional division also avoids a 64-bit software divide:

- For raw divisor magnitude `d > 65536`, the compiler supplies
  `M = ceil(2^48 / d)`. With `m = abs(n) <= 2^31`, the upper word of `m*M`
  gives the exact quotient or one too large. Comparing its reconstructed
  product with `m<<16` and decrementing when needed makes the result exact.
- For `d < 65536`, write `m = q*d + r`. The result's low 32 bits are
  `(q<<16) + ((r<<16)/d)`. Since `r < d`, both divisions fit in 32 bits and
  LLVM specializes their literal divisors into multiplication and shifts.
- Existing integer, power-of-two, and three-halves specializations remain.
  Signs, final 16.16 wrapping, floor division, single operand evaluation,
  and boxed type checks are preserved.

No public Lua API or renderer eligibility changed.

## Measured results

The isolated helper check covers 3,012,900 operand pairs and executes both
implementations, totaling 6,025,800 ARM calls. Every sampled result and ABI
check passed, with no individual cycle regression against the fixed-step
IWRAM baseline. Full distributions are in
[the EABI report](benchmarks/division/eabi-2026-09-08.txt).

| Isolated operation / distribution | Mean cycles before | After |
| --- | ---: | ---: |
| Unsigned 32-bit, random | 146.66 | 23.71 |
| Unsigned 32-bit, small divisor | 368.99 | 161.94 |
| Unsigned 32-bit, power of two | 341.20 | 36.43 |
| Signed 64-bit, fixed-point inputs | 1,302.14 | 305.01 |
| Compiled literal `/1.7`, 10,096 checked inputs | 1,331.28 | 36 |
| Compiled literal `/0.6`, 10,096 checked inputs | 1,331.85 | 41 |

The persistent arithmetic ROM diagnostic performs 16 divisions and modulos
per update, warms up for 10 ticks, and checks 120 samples against independent
host formulas. Both renderers pass every numeric watch and have zero
steady-state missed frames. The workload clears a constant background, so
arithmetic watches provide the semantic check; rendering cost explains the
different whole-frame percentages.

| Workload | Mode 0 median before → after | Mode 4 median before → after |
| --- | ---: | ---: |
| Dynamic fractional denominator | 30,327 → 10,981 | 174,600 → 155,254 |
| Literal `/1.7` and `%1.7` | 24,011 → 2,995 | 167,936 → 146,955 |
| Literal `/0.6` and `%0.6` | 23,967 → 3,367 | 167,972 → 147,339 |

The Celeste route is instrumented with the existing `tileCanvasInstrument`
probe and `tests/level_completion_route.json` inputs. It covers completion,
Start restart/spawn, and a particle burst; it does not establish death/respawn
coverage. Across all 239 published ticks, all 16 watches and RGBA hashes match.
The one cold-start missed frame is unchanged.

| Celeste Mode 0 metric | Before | After |
| --- | ---: | ---: |
| Total frame cycles, sum | 50,312,322 | 49,008,546 |
| Update cycles, sum | 26,060,836 | 24,782,791 |
| Frame cycles, median / p95 | 202,675 / 359,441 | 195,120 / 347,023 |
| ROM bytes | 308,248 | 308,532 |
| IWRAM bytes | 28,012 | 28,348 |
| EWRAM free bytes | 74,656 | 74,656 |

An instruction-level profile over the first 238 published ticks removes all
2,642 signed map-validation divisions. The remaining 963 unsigned hash-bucket
divisions take 263,187 cycles instead of 370,659. No 64-bit division helper
executes in this route, so its large isolated improvement is not a prediction
of Celeste's frame-rate gain. The linked helper code grows from 532 to 868
bytes; the game still has 4,324 bytes of usable IWRAM headroom.

## Reproduction and regression checks

The [ARM division gate](../scripts/division-check/README.md) retains the frozen
baseline assembly and checks exact results, register preservation, ARM/Thumb
returns, stack alignment, and per-input cycle regressions:

```sh
python3 scripts/division-check/check.py --output-dir /tmp/mtile-division-check
MTILE_DIVISION_OUT=/tmp/mtile-division-roms \
  go test ./cmd/renderbench -tags=runtimeperfdiagnostic \
  -run '^TestDivisionTargetDiagnostic$' -count=1 -v
go test ./internal/transpiler ./internal/build ./cmd/mtile
(cd runtime && go test ./...)
(cd runtime && go test -tags=mtile_mode0 .)
```

Build the headless player first as described in the ARM gate README, or set
`MTILE_DIVISION_PLAYER` to its executable. The ROM diagnostic runs in CI after
the pinned player build. Its JSON output supports comparisons with another
checkout. Compiler property tests exhaust every raw divisor below 2 against
numerator bit boundaries, plus randomized inputs, and preserve earlier
overflow/record-initialization regression cases. The IWRAM link test checks
actual ELF placement.
