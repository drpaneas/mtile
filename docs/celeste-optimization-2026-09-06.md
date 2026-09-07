# Celeste: readable numbers and measured GBA optimizations

This follows the [September 5 baseline](celeste-port-performance-2026-09-05.md).
The local `examples/celeste` cartridge is still a Mode 4 project selected by
AUTO. Its source reference remains the local Celeste Classic PICO-8 cartridge.
The user explicitly accepted small numerical differences for readability.

## Readable constants are a source improvement

Mtile converts a numeric literal to a 16.16 integer at build time. Writing
`0.6` instead of `0.5999908447265625` does not make arithmetic cheaper: both
produce one integer constant. The former rounds to 39322; the latter encodes
39321, the value produced by PICO-8's truncation. In an isolated native A/B,
changing just the literals changed neither the ROM size nor cycle statistics.

The custom sine correction did execute extra work on every call. Replacing it
with mtile's existing sine/cosine removed 5,092 ROM bytes and reduced the
162-update route median from 679,900 to 670,656 cycles (1.36%). These are an
isolated earlier pair, not the final integrated build. The maximum sine error
over all 65,536 phases is 15/65536; the eight principal directions remain exact.

Verification now distinguishes explicit tolerances from exact behavior. The
route retains exact positions, collisions, freeze states and completion, with
velocity drift bounded by 1/65536 horizontally and 15/65536 vertically. The
broader suite exposes two position exceptions: up to two pixels in the ice
release/reversal fixture and one pixel in the flying-fruit flee fixture. Tiny
velocity changes can cross an integer movement threshold a tick earlier.
Consequently, these results do not establish universal visual indistinguishability.
The original reference traces are unchanged. The local `NUMBERS.md` and
`tests/numeric_tolerances.json` explain each permitted difference.

## Make cached field writes cheap before preparing arguments

Generated Lua table writes previously called a helper with a variable-length
list of GC roots. Its cached branch already avoided pushing those roots, but
the caller still constructed the argument array before calling the helper.
The ARM output contained temporary Value copies and `__aeabi_memclr8` calls
even for ordinary repeated writes such as `player.x = ...`.

The transpiler now tests the already-proven stable field offset at the call
site. A warm write calls a compact, nonallocating runtime method directly;
only a first-use write constructs the roots and calls the original checked
helper. A numeric warm call's context, handle, offset and number fit in the
four ARM argument registers; TinyGo may further specialize away a known context.
The runtime resolves the handle on every write,
so a moving collection does not invalidate it.

The existing stable-layout analysis remains the gate. This does not generalize
the optimization to arbitrary mutable table layouts. Lua operand evaluation,
multiassignment order and the checked cold path are preserved. A regression
forces an earlier indexed store to grow its hash and collect while a later
cached store still needs both an old owner and a newly allocated RHS object.
Further tests cover nested values, moving collections and root-stack exhaustion.

In a paired native route run using the same frozen game source and runtime:

| Measure | Original caller | Compact cached write |
| --- | ---: | ---: |
| Frame median | 670,555 | 612,644 |
| Frame maximum | 969,744 | 901,252 |
| Update median | 114,696 | 81,880 |
| Draw median | 478,896 | 456,028 |
| Route updates over 561,792 cycles | 147/162 | 139/162 |
| ROM bytes | 379,132 | 385,976 |
| IWRAM bytes | 30,984 | 31,648 |

All 193 published probe states matched exactly between the two engine builds;
completion and restart framebuffer hashes also matched. The speed improvement
trades additional generated code and 664 IWRAM bytes for less work. These
figures precede the final map-tail refinement and source allocation changes.
Evidence directories: `mtile-celeste-gba-699xudxc` and
`mtile-celeste-gba-y935m0lf`.

## Avoid boxed results in truth-only conditions

Lua `and` and `or` return operand values. That full behavior is still required
in assignments and function arguments. An `if`, `while` or `until` needs only
truthiness, so the transpiler can join those branches using a native Boolean
instead of copying a whole Lua Value. Right-hand calls stay inside their
short-circuit branches. Tests include all combinations of nil, false, true,
zero, strings and tables, as well as allocating RHS calls and multiple returns.

This is a small improvement: an isolated idle pair went from 551,987 to 551,637
median frame cycles and used 92 fewer IWRAM bytes. It should not be credited
with the larger cached-write savings.

## Specialize division by 1.5 without changing its result

The hair loop still made ten `__aeabi_ldivmod` calls per frame. For a proven
numeric operand divided by a literal encoding as positive or negative 1.5,
the transpiler now emits an exact private helper. Raw division is trunc(2*n/3).
For unsigned magnitude m <= 2^31, the high word of m*0xaaaaaaab is exactly
floor(2*m/3); restoring the sign preserves truncation even at INT_MIN.
The helper's source comment proves the rounding bound, and generated-code
tests compare over 3.29 million signed inputs per enabled/disabled variant.
Boxed values, other divisors, dynamic zero division and floor division retain
their previous paths.

The ARM hair loop now has two UMULL sites and no 64-bit division calls. A paired
native route fell from 611,640 to 587,186 median cycles and 849,765 to 814,725
maximum cycles. All 193 watch snapshots matched exactly. IWRAM fell by 72 bytes.
The readable Lua `/ 1.5` expression is unchanged.

## Copy the shaken map as a single ARM operation

The Mode 4 raster cache now submits the entire clipped span list to one ARM
routine when the cached map is 240 pixels wide. It replaces hundreds of helper
calls with alignment-specific word-copy loops and small byte prefixes/tails.
The unshifted and narrower-map paths retain their existing behavior.

Native map fixtures including the clear fell from 316,189 to 227,497 cycles at
camera `(1,1)`, with similar 88.7–88.9K savings at horizontal offsets two and
three. The routine uses 556 IWRAM bytes and no additional EWRAM. Fifteen full
framebuffer comparisons against the generic renderer passed, including negative
cameras, clipping, partial maps and transparent holes over a colored background.
Detailed alignment reasoning and reproduction commands are in the preceding
performance document.

## Reuse expired smoke objects

The game now keeps expired smoke tables in a private free list. Creating smoke
reinitializes every field and consumes the same five random values in the same
order. Active particles remain in source order and there is no particle cap.
Respawn returns active particles to the pool; a new game clears both lists.
This retains memory up to the observed concurrency high-water mark and still
allocates when a larger burst appears for the first time.

A separate 800-update sustained-burst fixture reduced the update p95 from
131,188 to 53,265 cycles, frame p95 from 343,724 to 267,181, and observed
collections from two to zero. Total frame work fell 13.9%. The ordinary route
median was essentially unchanged, so this targets allocation spikes rather
than claiming a universal frame-rate improvement. Even this fixture retains
an 884,495-cycle first-use peak after pooling.

The dedicated verification compares 500 snapshots of every active particle's
fields, ordering and count, complete framebuffer hashes, and the final RNG
state. They match exactly, including 14 forced collections and respawn/new-game
boundaries. The local `SMOKE-POOL.md` describes the reset and ownership rules.

## Copy live GC payloads in words and sweep handles once

The collector copied each payload with a byte loop. Merely changing it to Go's
`copy` still produced bytewise target copying. The final implementation checks
alignment, copies groups of four 32-bit words, then remaining words and the
exact byte tail. Unusual unaligned offsets retain a byte fallback. The existing
range checks precede all accesses; semispaces are disjoint. The final handle
sweep now restores forwarded handles and reclaims dead handles in one pass,
preserving free-list order.

A native paired workload retained 7,760 bytes and compared 300 updates with five
collections. Generated Go, checksums, collection ticks, heap usage and handle
state matched exactly. Each measured collection fell from approximately
563,739 to 223,458 cycles, a 60.4% reduction. This adds 116 ROM bytes and no
IWRAM/EWRAM. It is an isolated collector result, not a claim that total frames
are 60% faster. Odd-sized payloads, repeated space flips, free-list reuse and
payload boundaries have focused runtime regressions.

The final expanded fixture includes odd-length strings mixed with tables:
8,248 live bytes, five collections, 300 identical ticks. Collection time fell
from 605,443 to 257,739 cycles (57.4%), with the same memory/code-size tradeoff.
Evidence: `celeste-gc-probe-2296293269`. The full runtime suite and checkptr=2
regressions passed.

## Mode 0 feasibility

The local `examples/celeste/MODE0.md` records two isolated prototypes. Replacing
circles with sprites alone is insufficient: the current Mode 0 analyzer also
needs provable draw-call shapes and finite sprite IDs, and scrolling cloud
backgrounds can make tilemap maintenance expensive.

A prototype with simpler sprite clouds, baked hair/dots, fixed render slots and
no shake initially measured 506,587 median cycles, over budget on 43/162 updates.
Rebuilt with the final engine, it measures **469,336 median, 644,322 p95 and
703,972 maximum cycles**, over budget on **31/162** updates. All positions and
velocities on its route remain exactly equal to PICO-8. The prototype uses the
earlier precise game arithmetic; it omits dynamic entity render slots, limits
visible smoke and simplifies the completion screen. Thus it is not an equivalent
replacement or a pure renderer A/B for the full port.

The project is preserved under `examples/celeste/tests/experiments/mode0-project`.
The final trace is `mtile-celeste-gba-4mgni85o`. Production assets and public Lua
API renderer classifications were not weakened to admit it.

## Final complete-game route

The final production ROM is 373,980 bytes with 16,384 free EWRAM bytes. The
instrumented route build uses 31,552/32,672 IWRAM bytes and includes its probe
overhead. It completes all 162 source-oracle updates, exits the room and restarts
with Start, with exact positions/discrete states and the declared vx/vy tolerances.

| Same route workload | September 5 baseline | Final September 6 |
| --- | ---: | ---: |
| Frame median | 679,900 | 587,186 |
| Frame p95 | — | 741,010 |
| Frame maximum | 1,068,674 | 814,725 |
| Updates above 561,792 cycles | 148/162 | 111/162 |

This is approximately 13.6% less median work and 23.8% less worst-route work.
It still fails a universal 30 Hz frame budget. The worst route spikes here
occur without collection, so the GC improvement alone cannot remove them.
The remaining costs include update work during actions and rendering; a future
budget-focused change needs to measure those action frames specifically.
Native evidence: `mtile-celeste-gba-wsxkwmrk`. Spawn/reload are excluded from
route quantiles and are separately reported by the probe.

The production 1,800-LCD-frame stress run observes 760 logical updates, with
514,370 median, 589,837 p95 and 1,259,317 maximum cycles; 85/760 exceed budget.
The earlier same-input-schedule run observed 575 updates with 601,574 median
cycles and 431/575 over budget. Inputs here are scheduled by LCD frame, so
faster execution changes the logical-tick alignment; this is a lifecycle stress
test, not the controlled route A/B above. The final run reports no heap error,
no audio overflow, audible PCM in 1,631 LCD frames, and no collections. Smoke
reuse prevents those collection triggers in this run; the independent GC
fixtures prove collector speed for workloads that still collect.
Evidence: `celeste-target-2956878621`.

## Verification status

The final required package tests (`internal/transpiler`, `internal/build`,
`cmd/mtile`) and the complete runtime suite pass. Native source verification
passes 262,144 trig comparisons, 1,766 physics/lifecycle states, 422 presentation
rows and 1,245 entity updates with the documented numerical exceptions. The
smoke comparison passes 500 exact state/pixel/RNG snapshots. Engine A/B checks
retain exact results independently of the accepted source rounding.

The aggregate runner initially stopped at the audio import check because the
editor had reordered JSON object keys. Parsed content was identical. The
checker now canonicalizes object keys while preserving values/types and array
order, and rejects duplicate keys. No audio assets were regenerated. Four
checker regressions and the rerun audio stage pass: 2,016 source notes, 42
patterns, 8,150 events, 212,627 controller checks, source cooldown timing and
native audible PCM for all 18 SFX and five songs. The final stress stage was
run separately and passed its behavior/heap/audio assertions, with the timing
failures reported above. Audio remains the existing PSG approximation.

AUTO fallback diagnostics now remap to the original Lua module just like forced
Mode 0 errors. A focused regression checks the module filename and position;
this corrects diagnostic metadata without changing renderer eligibility.

## Reproduce

With the local ignored Celeste assets, PICO-8 installation, TinyGo toolchain and
native mGBA player available:

```sh
go test ./internal/transpiler ./internal/build ./cmd/mtile
(cd runtime && go test ./...)
python3 examples/celeste/tests/verify_all.py --gba
go run examples/celeste/tests/map_raster_probe.go parity
```

`DisableConditionSpecialization`, `DisableStableWriteSpecialization` and
`DisableConstantDivision` are
internal build-service/transpiler options for paired measurements; they add no
public Lua builtins or renderer semantic changes.
