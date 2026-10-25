# General engine work toward sustained two-scan updates

This engineering loop starts from the completed snapshot in
`build/experiments/mode0-general-optimizations/final`. The playable Lua,
project configuration and assets of `research/celeste-mode0` are frozen.
All new production changes belong to mtile's compiler, runtime or ARM startup
library. They select paths by proven types, lifetimes, capacities, alignment
and renderer state, without testing game names or source identifiers.

## Acceptance and measurement

A 30 Hz GBA game normally advances once per two hardware LCD scans:
561,792 CPU cycles, approximately 29.86375 updates/second. The target is an
actual two-scan presentation interval, including startup, completion, restart,
death and respawn. A lower median or a CPU total under the budget does not
establish this cadence: a late VBlank admission can still repeat a frame.

The native diagnostic compares 280 complete images and gameplay snapshots
against the same source built with Mode 4. It records first-scan pixels,
presentation windows, CPU work, map/OAM transfers and audio/heap errors.
An independent host observer wraps mGBA's existing DISPCNT callback and reads
ELF/DWARF-described state. It adds no instructions or memory writes to the ROM.
The ordinary editor and release builds receive physical KEYINPUT input from
the host, including a natural completion and Start-button restart.

The baseline diagnostic route costs 358,573 median / 485,052 p95 / 826,902
maximum cycles. Its restart costs 2,788,669 cycles. The ordinary debug build
actually takes 328 LCD scans for 162 route commits, and its restart takes
11 scans. Release takes 329 route scans and the same 11-scan restart.

**Status:** work is in progress. The frozen intermediate runs below are
evidence, not a final claim of sustained cadence.

## Compiler and table operations: how the changes work

- **Static constructor plans:** compute the existing FNV hash and linear-probe
  slot order at compile time for unique static keys. Stores use the identical
  physical table layout and `pairs` order. Nil, duplicate/dynamic keys and
  unexpected metadata retain checked insertion. An unexposed literal whose
  values cannot allocate borrows its checked backing once. A stronger proof
  for complete, non-nil literals permits inline constant-offset stores after
  validating the fresh layout once. Runtime errors and static-intern failures
  invalidate the borrow before generic insertion can grow or collect.
- **Checked loop views:** fixed numeric arrays and resident map planes borrow
  validated backing after all numeric-loop bounds have evaluated. Per-element
  bounds, invalid-ID behavior and sticky errors remain. Unknown calls,
  allocation or renderer-state changes reject the relevant proof. Read-only
  map loops use a small view; writable views cache the exact renderer patch
  rectangles while retaining live signature and flag checks. Streaming maps
  keep their original lookup, eviction and mutation path.
- **Static hash reuse:** interned strings retain their original FNV hash. A
  slot hint is accepted only after checking the actual key in the current
  table; stale hints fall back to probing. The hint uses existing padding.
  The cached hash costs four bytes per reserved static-string slot.
- **Warm numeric fields:** an existing per-access-site stable field cache
  calls a compact numeric reader after its first full lookup. The receiver
  still evaluates once; a cold lookup keeps its complete Value/type check.
  A cached offset is not a general permission to assume arbitrary records
  have the same layout.
- **Fresh list reservations:** a narrowly proven fixed loop appending non-nil
  literals to an unaliased fresh list reserves the capacity that ordinary
  growth would reach. Successful values, final layout, insertion order and
  RNG order match. Reservation intentionally changes allocation/GC/OOM timing,
  as existing constructor capacity hints do. Unsafe loops keep ordinary growth.
- **Validated table paths:** table access reuses one checked handle/header;
  list operations reuse one validated slice and prefix length. Indexed `add`
  also fixes a pre-existing full-array bug: inserting within a full array
  must reserve the final element, not overwrite the hash payload after it.
  Its public renderer classification remains unchanged.

Each specialization has an internal compiler rollback option and targeted
optimized-versus-ordinary tests. They are not new public Lua APIs or new
Mode 0 eligibility rules.

## ARM memory operations: why host measurements were insufficient

The linked TinyGo ARM code showed that LLVM recognized a hand-written word
clear loop and changed it back into `__aeabi_memclr`, whose bundled libc
implementation cleared bytes individually from ROM. A faster host loop was
therefore not evidence of faster GBA execution.

`runtime/crt0/gba.s` now supplies general AAPCS-compatible `memset`, `memcpy`
and `memmove` in IWRAM. Aligned transfers use 16-byte register bursts, with
bounded word/byte tails. Moves choose direction for overlapping ranges;
mismatched alignments retain a byte path. Zero length does not access memory,
the destination is returned, and callee-saved registers are preserved.
The existing ARM EABI adapters resolve to these implementations.

Native verification exercises both GBA RAM regions, alignments, overlap
directions, length boundaries through 4,096 bytes, canaries, return values,
callee-saved registers and EABI adapters. `memset` passes 65,376 checks with
independent checksum 951358277; copy/move pass 363,194 checks with checksum
506947681. TinyGo's preinit was inspected: its BSS/data initialization uses
manual loops, so it does not call an IWRAM function before copying IWRAM.

The allocator keeps its common checked bump-and-clear path in IWRAM and its
collection/retry path in ROM. Allocation order, collection retries and sticky
errors remain unchanged. Fresh objects avoid redundant header validation and
stores of bytes the allocator has already cleared.

## Mode 0 work

Finite resident map materialization clips source rows once and copies spans
split at the GBA's 32-column screen-block boundary. Flag filtering, sprite
zero, invalid IDs and finite-map clipping retain their exact semantics.
Stable cached maps continue to use hardware scroll registers.

A successful map write passes its known tile ID to the invalidation hook,
avoiding a second lookup. Writable map views cache matching patch rectangles
for loops that cannot alter renderer state. A mutation after a map was drawn
still invalidates the next snapshot instead of changing the pending image.

Single-cell sprites use their own exact clipping/OAM path, and aligned OAM
entries use word-width stores. Multi-cell behavior, draw order, priority,
flips, the object limit and disabled tails are covered by an independent
reference path and complete-byte comparisons.

## Recorded iterations

These are diagnostic CPU costs from identical Lua/assets. They include
instrumentation; ordinary release results must be measured separately.
Nested intrusive function timers are used only to locate costs and must not
be added to one another or substituted for these totals.

| Snapshot | Route maximum | Restart maximum | Spawn maximum | CPU overruns outside restart |
| --- | ---: | ---: | ---: | ---: |
| Prior final | 826,902 | 2,788,669 | 940,489 | present |
| Round 4 | 524,222 | 1,108,192 | 625,327 | spawn |
| Round 5 | 520,690 | 774,135 | 493,354 | 0 |
| Round 6c | 508,736 | 765,129 | 493,347 | 0 |

Round 6c still does not meet the target: restart exceeds the budget and the
conservative VBlank admission repeats some otherwise affordable frames.
Its Mode 4 oracle uses 32,644 of 32,672 available IWRAM bytes, so additional
IWRAM placement must respect both renderers. The ordinary Mode 0 build uses
26,852 bytes of IWRAM and leaves 61,072 bytes of EWRAM free, versus 61,584
before cached static hashes. All 280 Round 6c pixels and gameplay states match.

Two discarded experiments are also retained: Thumb builds did not publish a
first frame, so no Thumb performance claim is made; moving Context/Heap
metadata unexpectedly expanded linked game functions and overflowed Mode 4
IWRAM. The original structure layout was restored before Round 6c.

Exact source snapshots, generated Go, ELF/ROM/link maps, native logs and
verified checksums live under `build/experiments/mode0-sustained30`.
