# Physics and map streaming performance work

The [Butano comparison](benchmarks/butano-30hz/README.md) identifies two large
mTile performance gaps at 30 Hz. This file tracks their diagnosis and focused
improvements. The original comparison remains an immutable baseline.

## Entity simulation and array access

| Workload | Butano median cycles | mTile Mode 0 median cycles | mTile observed updates per second |
| --- | ---: | ---: | ---: |
| 32 entities | 11,896 | 340,525 | 29.864 |
| 64 entities | 17,758 | 680,269 | 19.909 |
| 128 entities | 28,985 | 1,359,757 | 11.946 |
| 64 entities with drag | 24,832 | 676,432 | 19.909 |

The 30 Hz budget is 561,792 cycles. Both engines compute the same signed
Q16.16 trajectories and checksums. Generated-code inspection found that these
fixtures use **global generic Lua tables**, not the compact numeric-array path.
That optimization currently requires chunk-scope `local` array declarations.
The original report's description of the numeric-array path has been corrected;
its measured results have not changed.

### Profile and first fix

Instruction profiling over 120 updates attributes about **49.5%** of useful
instruction cycles to `gluPushRoots`, the `gluTableSet` wrapper, `PushLocal`
and `PopLocals` (exclusive inline-leaf costs). Each entity writes four slots;
the generated helper protected the table and live values before every store,
even though writing an existing array slot cannot allocate or collect.

The generated helper now first attempts `Context.TryTableSetArray`: a checked,
non-allocating store to an already allocated integer-indexed slot. It preserves
the full tagged value, including nil, booleans and string/table references.
Unsupported keys, invalid owners and hash insertion/growth retain the original
rooted fallback. This is an internal compiler/runtime change; the Lua API and
renderer classifications are unchanged.

The unchanged-source measurement drops the 64-entity median from **680,269 to
347,989 cycles** (1.95×, 48.8% fewer cycles), restoring exact two-refresh pacing.
The new profile has approximately 347,698 useful instruction cycles per tick,
against 679,993 before. Checked `tableData` access remains about 17.1% of that
new instruction cost; generic value handling and generated code remain costly.

A diagnostic control changed only the four array bindings to chunk-scope
`local` and measured **40,677 cycles** with matching pixels and checksums.
This is evidence of the remaining representation/compiler opportunity, **not**
the accepted fix or a result for the unchanged-source benchmark. Extending
numeric specialization to globals needs proof for rebinding, aliases, element
types, out-of-range access and growth before implementation.

## Mode 0 map window and chunk streaming

The diagonal `scroll_chunk_cross` workload moves 9 pixels horizontally and
3 vertically per update through a 256×128-cell map. It crosses tile and
32×32-cell chunk boundaries.

| Metric | Butano | mTile Mode 0 |
| --- | ---: | ---: |
| Median cycles | 8,855 | 289,299 |
| p95 cycles | 11,061 | 315,429 |
| Maximum cycles | 25,827 | 345,109 |
| Median share of the 30 Hz budget | 1.58% | 51.50% |

### Profile and first fix

About **86.8%** of useful instruction cost is attributed to `mapCursorCell`
(48.9%) and its caller's full-map construction loop (37.9%). Chunk loading
itself accounts for approximately **0.4%**. The repeated decoding and per-cell
screen-entry construction are the primary bottleneck on this route.

The streamed full-map path now resolves a chunk once per row span and uses the
existing filtered halfword copy. Spans split at both source chunk boundaries
and destination 32-column screen blocks, which need not align. Each span is
consumed before another chunk can evict its backing storage. Finite bounds,
sprite limits, flag masks, linked-tile exclusion and sparse edits are retained.

The unchanged-source measurement gives **64,272 / 88,901 / 117,310 cycles**
(median / p95 / maximum). Median cost is **4.50× lower**, freeing approximately
40 percentage points of the 30 Hz CPU budget (51.50% → 11.44%). Both versions
maintain exact two-refresh pacing. Useful instruction cost averaged over the
profile route falls from 282,121 to 57,268 cycles per tick; separately attributed
hardware-event cost stays at 8,194 cycles per tick.

Remaining work includes chunk-cache lookup/touch costs, clearing/copying screen
shadows, and exploiting unchanged columns when the map source window moves.
Any incremental update must preserve draw-time mutation snapshots and finite
edges. Butano still has a substantial headroom advantage in this workload.

## Profiling method and evidence

[`profile.py`](../scripts/butano-bench/profile.py) builds a small native helper
against the existing pinned mGBA core. It steps the **unmodified linked ROM**,
records per-PC instruction cycles from `mTimingCurrentTime`, and attributes them
with ELF symbols plus DWARF inline frames. Both before/after profiles cover
published ticks 60→180. Pending hardware events, intentional frame waits and
debug publication are reported separately. All categories reconcile exactly
to elapsed emulated cycles. Percentages above refer to exclusive useful/other
instruction costs, not host execution time or inclusive call stacks.

The profiler currently records ROM code in the first 1 MiB and IWRAM; instructions
outside those regions are counted separately. There were none in these profiles.
Profiles are diagnostic attribution over one 120-tick route, so use the separate
600-tick mailbox captures for median, p95, maximum and cadence. These remain
emulator measurements; physical GBA validation is outstanding.

Machine-readable [profiles and measurement records](benchmarks/performance-profiling-2026-10-29/README.md)
are saved separately from the original baseline.

## Proof required for improvements

- Preserve the baseline ROMs and measurement records.
- Record hot-symbol cycles and the exact profiling method and limitations.
- Compare release builds with 60 warm-up ticks and two 600-tick runs, including
  p95, maximum, observed cadence, ROM and RAM.
- Verify every captured image and game-state checksum against the independent
  oracle. Include relevant boundary, aliasing, mutation and invalid-input tests.
- Preserve renderer eligibility and exact Lua API semantics. Any public API
  change must satisfy `AGENTS.md` and update the renderer contract in the same
  change.
- Run `go test ./internal/transpiler ./internal/build ./cmd/mtile` and
  `cd runtime && go test ./...` before accepting product changes.

## Results

Mode 0 medians below use the **original, unchanged global-array Lua fixtures**.

| Workload | Before cycles | After cycles | Speedup | After share of 30 Hz budget | Before → after Hz |
| --- | ---: | ---: | ---: | ---: | ---: |
| 32 entities | 340,525 | 172,936 | 1.97× | 30.78% | 29.864 → 29.864 |
| 64 entities | 680,269 | 347,989 | 1.95× | 61.94% | 19.909 → 29.864 |
| 128 entities | 1,359,757 | 695,189 | 1.96× | 123.74% | 11.946 → 19.909 |
| 64 entities with drag | 676,432 | 344,534 | 1.96× | 61.33% | 19.909 → 29.864 |
| Fast diagonal chunk crossing | 289,299 | 64,272 | 4.50× | 11.44% | 29.864 → 29.864 |

The physics fix also benefits Mode 4: 64 entities fall from 703,175 to 369,075
cycles and recover 29.864 Hz. The map-span change applies to the Mode 0 streamed
full-map path. Slow streaming already reuses cached frames: its median changes
only from 1,416 to 1,348 cycles, but p95 drops from 278,500 to 62,731 cycles.

Linked EWRAM usage does not increase in these cases. Physics Mode 0 adds 160
bytes of IWRAM and reduces ROM by 592–712 bytes across the four fixtures.
Streamed map cases add 496 bytes of IWRAM and 808–888 bytes of ROM; row spans
borrow existing map-cache storage and do not allocate a new screen/map buffer.

**Remaining target after the first round:** 128 entities still exceed the 30 Hz budget. The next
physics investigation should distinguish caller-side variadic root-argument
packing/temporary zeroing from checked table access, then consider conservative
global-array specialization. The local-array control shows substantial remaining
headroom but does not establish that arbitrary global tables can be specialized.

The full recheck covers **38 workloads, 74 renderer cases and 88,800 measured
ticks**, with two identical repetition medians per case. Every captured pixel
hash and state checksum matches the independent oracle. Ten cases improve;
the other 64 retain exactly the same median, p95 and maximum. Mode 0 maintains
two-refresh cadence in **35/36** compatible workloads (previously 33/36), and
Mode 4 in **22/38** (previously 20/38).

Required Go gates and full generic Mode 0 tests with one and four backgrounds
pass. Focused forced-streaming pixel tests also pass. Running every generic
fixture with forced-streaming tags has existing resident-fixture failures,
reproduced from the unmodified baseline; these are recorded in the evidence
rather than attributed to this patch. See the [saved records and raw evidence](benchmarks/performance-profiling-2026-10-29/README.md).

## Follow-up: defer root-argument preparation

The first round was committed as `84c6d9a`. The next change moves the checked
array-slot fast path to the generated indexed-store callsite, before preparing
variadic roots. All source operands are still evaluated once, in the original
order. Failure enters a separate rooted helper. Keys outside the runtime's
possible array range skip the extra runtime attempt entirely.

| Mode 0 physics | First-round median | New median | New observed Hz |
| --- | ---: | ---: | ---: |
| 32 entities | 172,936 | 116,528 | 29.864 |
| 64 entities | 347,989 | 232,304 | 29.864 |
| 128 entities | 695,189 | 463,856 | 29.864 |
| 64 entities with drag | 344,534 | 234,710 | 29.864 |

**128 entities now meet the 30 Hz target in both renderers.** Mode 0 maximum
is 463,924 cycles and Mode 4 maximum is 484,381, against a 561,792-cycle budget.
This uses the original global-array source, without numeric-array specialization.
The 128-entity Mode 0 median improves another 33.3% and leaves 17.4% CPU headroom.

The targeted recheck covers all four physics workloads and three graphics
controls in both renderers, with 16,800 independently verified ticks. A further
4,800 verified ticks cover two hash-overwrite controls. One reveals a tradeoff:
hash-backed tables with small integer keys are about 4.0% slower because the
caller tries the array path before preparing the fallback. Both versions retain
30 Hz in that probe. Large hash keys become 8.4% faster by skipping that attempt.

Checked table metadata and generic value handling remain optimization targets;
global-array specialization has not been expanded. The current stage achieves
the 128-entity cadence target while preserving the tagged-table representation.
See [profiles, exact results, limitations and proof](benchmarks/table-store-arguments-2026-10-29/README.md).

Review cleanup routes uncached string-field stores directly to the rooted
helper and removes the obsolete `gluTableSet` wrapper. This avoids an array-slot
attempt that could never succeed. The 128-entity Mode 0 ROM is byte-identical
afterward; the saved measurement snapshot remains unchanged.

## Feasibility investigation: reaching Butano's baseline

The [2026-10-30 investigation](benchmarks/butano-parity-2026-10-30/README.md)
keeps engine source unchanged at `660c8d5` and tests explicit Lua/project
tuning. Private compact numeric arrays and an IWRAM physics helper reach
10,239 / 17,747 / 19,925 cycles for 32 entities / 64 entities / 64 with drag.
At 128 entities, two contiguous blocks of 64 records reach 22,540 cycles.
These match or beat the original Butano baseline, with the 64-entity difference
treated as a practical tie. They do not replace the original-source results.

Applying ARM/IWRAM tuning to Butano lowers its medians to 8,343 / 10,071 /
13,527 / 10,848 cycles for 32 / 64 / 128 / 64-with-drag entities. That still
leads the supported tuned-Lua paths by factors of 1.23–1.84. A separate
hand-edited native-Go control reaches 7,839 cycles for 128 entities with static
buffers/code in IWRAM. This demonstrates backend headroom; safe automatic
lowering of generic Lua tables to that representation remains unimplemented.

A larger scrolling cache window reduces mean cost from 64,771 to 12,086 cycles,
but increases p95 to 132,324 versus Butano's 11,061. Incremental map updates
remain necessary to approach its tail latency. All variants and limits are
recorded separately, with 25,200 verified ticks and no production engine edits.
