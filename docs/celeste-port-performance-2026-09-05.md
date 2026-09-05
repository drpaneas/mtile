# Celeste Classic port: GBA runtime findings, 2026-09-05

The one-room Celeste Classic port exposed a native stack overflow and repeated
rendering/table work that the smaller examples did not exercise. These changes
preserve Lua API behavior, renderer selection, fixed-point arithmetic, and the
authored game's presentation. They do not establish a new public API contract.

**Whole-game verification is complete; the 30 Hz performance target is not
met.** The final 162-update route exactly matches the native PICO-8 controller,
but 148 updates exceed the 561,792-cycle budget. The implementation and
measurements below distinguish behavioral parity from real-time performance.

## Fix the stack before trusting telemetry

The failing ROM showed a red framebuffer and impossible heap statistics. Native
disassembly identified a 3,400-byte stack frame in `update_player`, compared with
the runtime's entire 2,048-byte user stack. Its caller `main` retained another
664 bytes; `entity_update` independently used 1,904 bytes before nested helpers.
The resulting stack overwrite could appear to be a Lua heap fault.

`runtime/crt0/gba.ld` now reserves **8 KiB for the user stack**, keeping the IRQ
stack separate. This is native CPU stack storage, not an increase to the Lua
heap. The linker still enforces the physical IWRAM capacity shared by stacks and
hot code. A subsequent 1,800-LCD-frame native run no longer showed corruption.
The 8 KiB reserve provides room for the observed nested calls; it is not a proof
of sufficient stack for every possible Lua program or unbounded recursion.

## Cache the static map's pixels once

For a fixed, single stored map plane no larger than 30×20 tiles, the Mode 4
runtime can now construct a raster of original palette indices. The generated
startup calls `PrepareMapRaster` **after `game.Init`**, so object-marker removals
via `mset` are included, and before visible gameplay/profiling. Preparation does
not modify the framebuffer, camera, clip, palette, or transparency state.

Each raster row has a bounded list of opaque runs. A run stores its 16-bit pixel
offset, 8-bit length, and 8-bit row in four bytes. The warm draw copies only those
runs: transparent holes leave previously drawn clouds/background pixels intact.
This avoids rediscovering the same tiles, source addresses, and transparent
pixels on every frame.

The full, unshifted 240-pixel-wide case uses one **164-byte ARM routine in
IWRAM**, `mtile_copy_map_raster_full`, to process the whole list. After aligning
each run, `LDMIA`/`STMIA` copy groups of 16 bytes; byte prefixes and tails preserve
neighboring transparent pixels. One call handles the entire plane instead of
one Go helper invocation per run. The ordinary Go implementation remains the
host-test oracle.

Camera shifts, clipping, and cell subrectangles use the general span path. For
unaligned source/destination relationships, it combines two aligned source
words with shifts. It never relies on an unaligned ARM7TDMI word load behaving
like a modern CPU load. Prefix/tail and allocation-bound checks keep those reads
inside the source raster.

### Eligibility and bounded memory

The cache requires Mode 4, a read-only spritesheet, one stored map plane, no map
streaming, and a raster fitting within 240×160 pixels. Draw calls use the cache
only with an identity draw palette, source color zero as the sole transparent
color, and no active tile flag mask. Other calls use the existing cell renderer
with its original semantics. In particular, this does not make any new shape
eligible for Mode 0 or change AUTO renderer selection.

A real map mutation invalidates the cache; a same-value `mset` does not.
Palette/transparency resets do not invalidate the original source-index raster.
A list exceeding **2,048 runs** fails closed to the cell renderer, without
repeated rebuilds while the map stays unchanged. Runtime construction and drawing
perform no dynamic allocation.

For a full 240×160 map, storage is 38,400 pixel bytes plus 8,192 span bytes:
**46,592 bytes / 45.5 KiB EWRAM**. The generator calculates affordability using
the target's `unsafe.Sizeof` values for Context, game globals, asset RAM, and the
native audio controller when present. Each size is converted to `int` before
subtraction, preventing unsigned constant underflow. It conservatively reserves
both Lua heaps, the debug mailbox, and an additional 8 KiB margin. Unaffordable
caches compile to zero-length arrays; ineligible cartridges omit them entirely.
The legacy software audio profile is excluded from this optimization.

### Exact pixels and native timing

`runtime/mapraster_test.go` compares cached/fallback framebuffers over 900 seeded
camera/clip/subrectangle/palette/mask combinations and multiple logical BGs.
The nonuniform existing background detects accidental writes through transparent
holes. Further cases cover real/no-op map edits, reinitialization, preparation
after Init edits, list overflow, missing/undersized buffers, deliberately
unaligned storage, narrow maps, and all horizontal byte alignments.

The native probe builds matching ROMs with `DisableMapRasterCache` on/off and
compares SHA-256 hashes of the entire displayed RGBA frame. Every pair below
matched exactly. Values are median `LastDraw` cycles after warm-up and **include
the fixture's `cls(0)`**, not just the map copy.

| Camera | Cell renderer | Cached renderer | Saved cycles |
| --- | ---: | ---: | ---: |
| 0, 0 | 551,878 | 167,744 | 384,134 |
| 0, 1 | 548,690 | 235,557 | 313,133 |
| 1, 1 | 660,510 | 316,189 | 344,321 |
| 2, 1 | 654,631 | 315,524 | 339,107 |
| 3, 1 | 648,748 | 314,267 | 334,481 |

The unshifted fixture's matching RGBA SHA-256 is
`55c3a39234569da8dc7e70185c7484b4b4805abc550226c218a7216b738a3d8f`.
The native evidence directory was `celeste-map-raster-3395118396`; each rerun
creates and prints a new directory containing both projects and ROMs. This
fixture has no Init marker removal; the host prewarm test separately verifies
that semantic requirement.

An intermediate full-game draw-pass comparison measured approximately 102K
cycles attributable to the cached map, versus approximately 481K originally.
This comparison identifies the map improvement; final integrated measurements
appear below.

## Keep numeric builtin results numeric

`min`, `max`, and `mid` already convert their arguments with `NumberOr(0)` and
always return a number. The transpiler now records that numeric result even when
an argument arrives as a generic Lua `Value`. Arguments are still evaluated and
converted using the existing rules; the optimization removes unnecessary boxing
from subsequent arithmetic rather than changing arithmetic or coercion.

`numeric_builtin_result_test.go` runs both generic and specialized generated
code with numeric, string, Boolean, nil, and table inputs. A separate fixture
checks argument evaluation order and side effects. The Celeste presentation code
uses this result information to keep repeated coordinates in native 16.16
locals while retaining the source formulas.

## Avoid roots around a write that cannot allocate

The generated `gluTableSetStable` and `gluTableSetStableNumber` helpers now take
a direct warm path once a stable field offset is known. That runtime path
resolves the table's stable handle on each use and overwrites an existing slot;
it cannot allocate or trigger GC. Therefore it does not need a temporary root
scope around the write itself. The first write retains the original root pushes,
failure cleanup, table semantics, and cache establishment.

`stable_write_gc_test.go` deliberately allocates more than a thousand temporary
tables/strings in right-hand-side expressions, performs moving collections
between writes, and checks retained strings/nested tables. It also fills the root
stack to exercise first-write failure cleanup and a warm nonallocating write.
The optimization does not make unproven dynamic table layouts stable; it applies
only where the existing compiler analysis already selected stable field access.

The three warm get/get-number/set entry points live in IWRAM. The numeric setter
wrapper is inlined and calls the same set entry point. First-use validation
remains in ROM behind non-inlined helpers, conserving IWRAM for repeatedly
executed instructions.

## Reduce repeated work in presentation

Cloud, ambient-particle, smoke, and hair helpers cache numeric fields in locals
and store their final state once. This preserves particle counts, lifetimes,
subpixel remainders, and the original hair division by 1.5. Presentation parity
checks passed 422 native reference rows; no reduced particle count or simplified
hair formula is needed for these gains.

The existing IWRAM `fillRowWord` helper now checks the clipped row's starting
pointer once and writes four words per loop iteration instead of repeating a
framebuffer bounds check for every word. A regression checks all 86,760
horizontal spans across three rows and detects writes into neighboring rows.
The final project selects `draw_ambient_particles`, `draw_background`, and
`draw_hair` through `iwramFunctions`. Their generated calculations are unchanged;
`noinline` keeps their bodies in IWRAM. Adding `update_smoke` saved little more
while leaving only 184 bytes free in the instrumented build, so it was omitted.

## Final integrated measurements

The production ROM is **367,904 bytes**, with **16,392 bytes free EWRAM** and
**30,472 / 32,672 bytes IWRAM used**, including both native stacks. The route
probe adds globals and telemetry and uses 30,500 bytes IWRAM. The authored map
is byte-identical to its pre-port copy.

With the same current runtime, native route median frame work falls from
**762,878 cycles with game helpers in ROM to 679,900 with the selected three
helpers in IWRAM**. The latter route peaks at 1,068,674 cycles. Every one of its
162 controller states matches PICO-8 raw 16.16 values; completion and Start
restart pass, hair retains five valid nodes, and PSG samples are audible.
These counts include probe overhead and exclude initial spawn/Start reload
from the route statistics. **148/162 route updates are over budget.**

A separate production-source stress ROM ran for **1,800 LCD frames**, including
walking into spikes, respawn, jump/dash inputs, and idle. It published 575 game
ticks with no heap error or audio overflow. Frame work was median **601,574**,
p95 **751,100**, maximum **1,333,742** cycles; **431/575** ticks exceeded the
budget. Nonzero PCM was observed on 1,629 LCD frames. This lifecycle run
includes spawn/respawn costs; it is a different workload from the completion
route and its median must not be substituted for the route median.

Reproduction artifacts were printed under `mtile-celeste-gba-t918ap78` for the
route and `celeste-target-2700853389` for stress. They contain traces and native
screenshots. The source-oracle suite also passed 1,766 physics/lifecycle states,
422 presentation rows, 1,245 entity updates, and 262,144 sine/cosine comparisons.
Audio verification retained 2,016 source notes and 42 patterns, passed 212,627
controller checks, and produced non-silent native PCM for all 18 SFX and 5
songs. PSG timbre/effect/timing differences remain documented in the example.

The completed functionality therefore preserves tested per-update behavior,
but the game can run more slowly than the original in real time. Further work
is required for steady 30 Hz; no such performance guarantee is made here.

## Reproduce the checks

The example's native probes require the local asset checkout, TinyGo toolchain,
and `build/player/mtile-player`. They make temporary copies instead of editing
the authored game.

```sh
go test ./internal/transpiler ./internal/build ./cmd/mtile
(cd runtime && go test ./...)
go run examples/celeste/tests/map_raster_probe.go
go run examples/celeste/tests/performance_probe.go full no-map
python3 examples/celeste/tests/verify_all.py --gba
```

The final required root-package suite, `internal/assetsync`, `internal/editor`,
and complete runtime suite passed. A real TinyGo ELF test confirms configured
helper placement in IWRAM; both renderers' generated-code tests preserve all
output except placement directives. Focused numeric, stable-write GC, and map
generator/runtime checks pass. Pong also still builds with the shared runtime
and larger stack (87,332-byte ROM, 76,784 bytes free EWRAM).
