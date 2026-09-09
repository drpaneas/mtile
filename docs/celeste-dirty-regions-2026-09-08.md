# Dirty-region experiments on original Celeste, September 8, 2026

Keep the existing Mode 4 renderer. The tested dirty-region implementations
preserve every pixel and watched value, but all increase execution time. The
most complete prototype combines cached map coverage, native partial clears,
and native partial uploads with separate histories for both VRAM pages. Its
median frame cost is **538,178 cycles versus 513,561** for the accepted renderer
(4.79% more). Frames above the 561,792-cycle budget increase from **83 to 162**
over the same 593 logical ticks. No experimental runtime changes are enabled.

The baseline is the optimized renderer from the
[earlier Celeste performance pass](celeste-mode4-performance-2026-09-08.md),
including its packed map caches and native full-screen fill. Its rebuilt debug
ROM is byte-identical to the accepted replay ROM. These experiments use the
original `examples/celeste`, selected as Mode 4 by AUTO, with identical Lua,
assets, and scripted inputs. They make general runtime changes without
game-specific checks. Times are emulated GBA CPU cycles, not host CPU usage.

## What changes between frames

The displayed image is often mostly unchanged. Across consecutive frames,
the median is **390 changed pixels out of 38,400**, touching 52 of 600 tiles
when grouping pixels into 8×8 regions. Against the same physical VRAM page
(normally two logical frames earlier), the medians are 688 pixels and 65 tiles.
The captured hardware palette stays constant; raw Mode 4 indices were checked
against both the palette and the displayed RGBA pixels.

However, the game clears and redraws its layers each active draw. The final
nonzero pixels occupy a median **395 of 600 tiles**. Previous/current
occupancy covers 401 tiles; current/same-page occupancy covers 406. These are
optimistic estimates for conservative write tracking: transparent holes,
black pixels that were explicitly drawn, and pixels overwritten later can
increase its marked area. The occupied tiles also form a median 63 horizontal
runs of tiles, or approximately 504 short scanline spans.

An image-matching analysis of the authored terrain distinguishes 464
stable-camera transitions with moving clouds (median 381 changed pixels)
from 110 camera-changing transitions (median 7,908.5 changed pixels).
Eighteen retained/frozen transitions change nothing. These groups describe
this route's screen shake and do not establish costs for continuous terrain
scrolling.

## Measured prototypes

Every candidate completed the same 593-tick replay with identical final RGBA
hashes and all 16 watched game values. The action subset is ticks 29–190.

| Implementation | All ticks: median cycles | Action: median cycles | All ticks above budget |
| --- | ---: | ---: | ---: |
| Accepted renderer | 513,561 | 554,455.5 | 83 |
| Compare hidden VRAM and write changed words | 552,813 | 593,793 | 233 |
| Compare hidden VRAM and copy changed 32-byte blocks | 548,637 | 589,789.5 | 211 |
| Track each span; retain full clear | 693,477 | 735,313.5 | 575 |
| Track each span; partial clear through Go | 772,817 | 813,364 | 575 |
| Cache map coverage; retain full clear | 532,339 | 572,427 | 137 |
| Cache map coverage; partial clear through Go | 615,406 | 655,341.5 | 545 |
| Cache map coverage; native partial clear and upload | 538,178 | 578,110.5 | 162 |

Comparing framebuffer and VRAM bytes is itself expensive on the GBA bus.
Even a completely unchanged screen takes 106,825 cycles with the native block
comparison, before game overhead. The accepted DMA upload's median is 78,325
cycles in the replay. Reading both buffers consumes the saving from omitted
writes.

Tracking writes avoids the full-screen comparison. Caching conservative source
coverage with the packed maps reduces tracking overhead from approximately
174,000 to 18,100 cycles per tick on average. Nevertheless, partial clearing
through hundreds of small Go-dispatched fills costs more than a full clear.

Native loops remove that dispatch overhead, but the remaining fragmented work
still matters. Given exact occupancy masks for free, the isolated native clear
takes a median 57,595 cycles versus 61,831 for the existing full fill: only
4,236 cycles saved. An optimistic same-page occupancy upload takes 76,382
cycles, compared with the full DMA's 76,800-cycle bus floor. These isolated
figures exclude tracking, page-history management, and normal game interrupts.
The complete native prototype includes those costs and loses overall.

## Why the moving-square example differs

With one square on a plain background, the previous square bounds describe
the entire area that needs restoration. Celeste has multiple overlapping
maps, sprites, particles, hair, and screen shake. Clearing behind a moving
object also requires restoring every visible layer beneath it.

A renderer that retains and compares draw commands could avoid rasterizing
unchanged content, as well as avoid its clear and upload work. The sparse
actual image changes suggest potential, but this experiment does not establish
a performance gain for that different architecture. Celeste's `_draw()` also
updates particles, hair, RNG and other game state, so an exact implementation
must execute Lua once and optimize native rendering beneath it. It must
preserve painter order, camera/clip/palette state, readback and asset mutations,
and both VRAM pages' independent histories.

## Evidence and validation

- [Summary, ROM hashes, environment, and timing statistics](benchmarks/celeste-dirty-regions/2026-09-08.json).
- [All candidate frame timings and image/state hash witnesses](benchmarks/celeste-dirty-regions/frames.csv).
- [Final temporary prototype patch](benchmarks/celeste-dirty-regions/native-prototype.patch), retained for inspection only; it is not applied to the runtime.
- [Native comparison source](benchmarks/celeste-dirty-regions/native-compare.s) and [36-case result](benchmarks/celeste-dirty-regions/native-compare-results.txt).
- [Native mask source](benchmarks/celeste-dirty-regions/native-mask.s) and [1,779-case result](benchmarks/celeste-dirty-regions/native-mask-results.txt).

Native checks cover output pixels, buffer guards, source/mask immutability,
saved registers, stack, and ARM/Thumb returns. Host checks cover all framebuffer
writer paths and palette-resolved clear semantics. A 32-frame strict-checkptr
test covers both page histories, freezes, repeated uploads without flips,
multiple clears, palette changes, hardware reinitialization, and diagnostic
blit modes. Conservative packed coverage was additionally checked against
304,486 clipped and unaligned translation cases. The public Lua API and
renderer eligibility rules are unchanged.
