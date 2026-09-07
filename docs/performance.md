# Mtile performance guide

This is the "Effective Mtile" for frame budgets: what the hardware gives
you, what each operation costs in measured ARM7TDMI cycles, and which
patterns keep a real game at 60 Hz. Every number below was measured with
`runtime/cmd/benchspike` on mGBA (cycle-accurate ARM7TDMI timing) using the
GBA's own TM0/TM1 hardware timers, on a 128x128 sheet with realistic
half-transparent art. Re-measure after runtime changes with:

Unless a section explicitly says otherwise, graphics numbers in this guide
describe the Mode 4 software framebuffer backend. The compile-time Mode 0
backend accepts the statically proven tile/BG/OBJ subset, and AUTO selects it
only when both its semantic and benchmark-backed cost proofs succeed. The exact API contract and expansion gates are in the
[render-mode compatibility guide](render-modes.md) and
[engineering plan](render-mode-plan.md).

The tables below retain their stated benchmark snapshots. The September 5
[Celeste port measurements](celeste-port-performance-2026-09-05.md) add a
bounded Mode 4 raster cache for one small static map plane. Its exact-pixel
fast path substantially changes eligible map costs; older per-cell map timings
remain relevant to fallback calls, not to the new cached path.

## Paired static-map proof

`cmd/renderbench` built `examples/testio` twice with identical source/assets,
TinyGo 0.41.1, `-gc=none -opt=2`, and the pinned headless mGBA core. After 20
warm-up frames it compared and sampled 120 synchronized 30 Hz ticks.

| Metric | Forced Mode 4 | Forced Mode 0 |
| --- | ---: | ---: |
| LCD pixel differences | 0 | 0 |
| Median / p95 total cycles | 544,417 / 544,417 | 38,409 / 38,409 |
| Median draw cycles | 465,759 | 576 |
| Median commit cycles | 76,924 | 36,341 |
| Cold tile + map upload cycles | n/a | 1,967,744 |
| Missed frames | 0 | 0 |
| ROM bytes (debug build) | 25,980 | 25,528 |
| EWRAM free | 60,600 | 94,640 |

Mode 0 avoids redrawing 600 8x8 cells and copying a 38,400-byte framebuffer on
every tick; its immutable tiles and screen map remain resident in VRAM. That is
why this deliberately static case wins by an unusually large margin. The table
does not predict mutation-heavy, scrolling, sprite, text, or primitive-heavy
games. Those supported shapes have separate parity fixtures; their cost must be
measured with `cmd/renderbench` for the actual project. The 1,967,744-cycle
(~117 ms) cold tile/map upload is excluded from the steady-state median and
occurs during forced blank, before BG0 is enabled.

The complete `tile-v1` matrix is checked in under
[`docs/benchmarks/render-modes`](benchmarks/render-modes/README.md). Camera,
two-layer, composite OBJ/text/palette, 16 `sset`, and 16 `mset` workloads all
passed parity, memory, p95, missed-frame, and ≥20% median-speed gates. Runtime
`fset` remained pixel-exact and 15.67% faster than Mode 4, but deliberately
failed the conservative 20% AUTO gate; `tile-v1` therefore selects Mode 4 for
that source shape.

Layer-first map access has its own
[`layered-map-access-v1`](benchmarks/render-modes/layered-map-access-v1.json)
record. Sixteen `mget(1,x,y)` reads and sixteen literal-layer
`mset(1,x,y,id)` writes per tick are pixel-exact and retain large Mode 0 cycle
savings. Host microbenchmarks keep the unchanged BG0 arities as explicit
regression controls and report zero allocations. Dynamic-layer mutation stays
forced-Mode-0-only in AUTO until its widest 16-write shape is measured.

### Compile-time OBJ glyph interning

Mode 0 `print()` admits static printable-ASCII strings, decimal numbers, and literal colors,
so the linker now stores one 64-byte OBJ tile per unique `(glyph,color)` pair.
The sorted atlas is uploaded once during force blank; repeated occurrences and
later frames reuse the resident tile. OAM and scanline accounting remain per
visible character, while the tiles 480-511 capacity now limits unique pairs.

The `text_intern.lua` fixture draws 32 copies of `A` in color 7. With the same
20-warm-up/120-sample capture before and after interning:

| Metric | Per-occurrence staging | Static interned atlas | Change |
| --- | ---: | ---: | ---: |
| Pixel differences vs Mode 4 | 0 | 0 | exact |
| Median total cycles | 162,434 | 71,117 | -56.22% |
| Median draw cycles | 94,901 | 16,062 | -83.08% |
| Median commit cycles | 65,551 | 53,403 | -18.53% |
| Mode 0 ROM | 53,808 B | 52,332 B | -1,476 B |
| Mode 0 EWRAM free | 94,864 B | 96,848 B | +1,984 B |
| Missed frames | 0 | 0 | unchanged |

Against Mode 4, the resulting Mode 0 cartridge is 57.91% faster instead of
3.86%, with zero differing pixels. It still budgets 32 OAM entries and 256
worst-line OBJ cycles; interning removes duplicate tile construction and
per-frame tile DMA, not the hardware cost of displaying each character.

### Packed 16x16 OBJ proof

The `pack16.lua` fixture draws eight 16x16 sprites from one immutable source
rectangle and covers all four flip combinations. Compared with commit
`e2fc183` immediately before packing, the same 20-warm-up/120-sample capture
measured:

| Metric | Four 8x8 OBJs | One packed 16x16 OBJ | Change |
| --- | ---: | ---: | ---: |
| Pixel differences vs Mode 4 | 0 | 0 | exact |
| Median total cycles | 66,914 | 61,828 | -7.60% |
| Median draw cycles | 18,148 | 11,292 | -37.78% |
| Median commit cycles | 46,832 | 48,602 | +3.78% |
| Mode 0 ROM | 51,752 B | 53,020 B | +1,268 B |
| Mode 0 EWRAM free | 96,928 B | 96,904 B | -24 B |
| Missed frames | 0 | 0 | unchanged |

The steady path uploads no packed tile data after first use. The static proof
now accounts for one OAM entry per cached rectangle and its true 16-cycle
scanline width.

### Packed 32x32 OBJ proof

The `pack32.lua` fixture draws two distinct 32x32 sprites and animates all four
flip combinations. Compared with the same pre-packing commit and capture
settings:

| Metric | Sixteen 8x8 OBJs | One packed 32x32 OBJ | Change |
| --- | ---: | ---: | ---: |
| Pixel differences vs Mode 4 | 0 | 0 | exact |
| Median total cycles | 63,259 | 48,318 | -23.62% |
| Median draw cycles | 14,302 | 3,214 | -77.53% |
| Median commit cycles | 46,832 | 43,230 | -7.69% |
| Mode 0 ROM | 52,400 B | 53,484 B | +1,084 B |
| Mode 0 EWRAM free | 96,904 B | 96,872 B | -32 B |
| Missed frames | 0 | 0 | unchanged |

The two sources fill the entire 32-cell cache and both render as one OBJ per
draw. Each still costs its true 32-pixel width in the scanline-cycle proof.

### Full OAM and scanline proof

The renderer now separates the GBA's two physical limits: 128 OAM entries per
frame and 1,226 normal-OBJ rendering cycles per scanline. Literal Y positions
are composed on the hardware's wrapping 0..255 coordinate; dynamic positions
are assumed to overlap in the worst case. Horizontally offscreen objects are
not discounted.

| Workload | OAM | Worst line | Mode 4 median | Mode 0 median | Saving | Pixels |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 64 distributed 8x8 sprites | 64 | 64 cycles | 219,164 | 149,119 | 31.96% | 0 |
| 38 overlapping packed 32x32 sprites | 38 | 1,216 cycles | 1,458,945 | 116,580 | 92.01% | 0 |

The second case is ten cycles below the official 1,226-cycle ceiling; a 39th
32-pixel OBJ is rejected at build time because 1,248 cycles cannot be proven
safe. Mode 0 missed no frames in either capture. The deliberately raster-heavy
Mode 4 side of the 38-sprite characterization missed 140 cumulative ticks, so
that record is a parity and hardware-boundary proof rather than a balanced
performance comparison.

For bounded dynamic one-cell sprites, ID 0 has an additional runtime-only fast
path: when its logical 8x8 tile is entirely the default-transparent source
color 0, the draw emits no OAM entry. Authored or `sset()`-mutated nonzero
pixels disable the fast path immediately and preserve ordinary `spr(0)` output.
The compiler deliberately continues to budget each call as active, so AUTO
eligibility never depends on a particular animation frame. The paired
`inactive_sprite_slots.lua` workload alternates 64 calls between IDs 0 and 1
and is the parity/performance fixture for this path. Across 120 synchronized
frames it records zero differing pixels and no missed frames: Mode 0 measures
68,920 median cycles versus 171,235 for Mode 4 (59.75% lower) and returns
37,088 EWRAM bytes.

### Object depth between four backgrounds

`object_depth_lanes.lua` draws the same 8x8 object after each of four real,
overlapping BG passes. Mode 0 derives priorities 3, 2, 1, and 0 from painter
order; ordinary and packed objects use the same Attribute-2 rule. The analyzer
still applies the shared OAM/scanline proof and rejects the unavailable lane
behind every materialized BG.

| Metric | Mode 4 | Mode 0 |
| --- | ---: | ---: |
| Pixel differences across 120 synchronized frames | 0 | 0 |
| Median total cycles | 1,215,615 | 50,854 |
| Median draw cycles | 1,136,997 | 6,750 |
| Median commit cycles | 76,924 | 42,528 |
| EWRAM free | 62,340 B | 82,956 B |
| Static object proof | software painter | 4 OAM / 16 line cycles |

This is a 95.82% median-cycle saving and returns 20,616 EWRAM bytes. Mode 0
records one cold-start missed frame from the four-layer upload; Mode 4 records
140 cumulative missed ticks on the deliberately full-screen four-pass raster
workload. The parity gate therefore establishes exact ordering independently
of the separate frame-delivery characterization.

```sh
cd runtime
make benchspike-gba
mgba benchspike.gba     # the table prints on screen
```

Host-side regression benchmarks (relative cost, allocation checking) live in
`runtime/bench_test.go`:

```sh
cd runtime
go test -bench=. -benchmem
```

Every benchmark must report `0 allocs/op`. The runtime never allocates on
the Go heap after `Init`; a nonzero allocation count is a bug.

## The budget

One display frame is **280,896 cycles** (1232 cycles x 228 scanlines,
59.73 Hz). The generated loop runs `_update`/`_draw` at **30 Hz** - once
every two display frames, PICO-8's native rate - so one game tick owns
**561,792 cycles**. Fixed per-tick costs already spent on your behalf:

| Cost | Cycles | % tick |
| --- | ---: | ---: |
| DMA blit of the framebuffer to VRAM | ~77,000 | 14% |
| Input poll, tick, palette, flip | ~1,000 | <1% |

The blit targets the hidden page (which cannot tear), so it runs as soon as
`_draw` finishes rather than waiting for VBlank. In practice, treat
**~480,000 cycles as the ceiling for `_update` plus `_draw` combined**.

Why 30 Hz and not 60: the measured table below shows a full-screen `map()`
is ~832k cycles and 100 sprites are ~210k. At 60 Hz those numbers made
whole game genres impossible; at 30 Hz a map redraw plus a normal sprite
load plus HUD text fits with headroom. The GBA's LCD still scans at
59.73 Hz - each rendered image is simply shown for exactly two scans,
which is how most commercial GBA games shipped. A hardware timer
(TM2/TM3) paces the flip so a fast tick can never land one frame early:
the cadence is a metronomic 2 frames per flip, which the eye reads as
steady 30 FPS, not jitter. Jitter only happens when cadence *alternates*
(1-2-1-2); the pacer makes that impossible by construction.

The frame profiler (`Context.SetProfiling`) reports exact per-phase costs
on real hardware; `MissedFrames` counts ticks that overran the two-frame
slot.

### What a tick buys you: 30 Hz vs 60 Hz

| | 60 Hz loop | 30 Hz loop (current) |
| --- | ---: | ---: |
| Budget per game tick | 280,896 cycles | **561,792 cycles** |
| Fixed blit share | 27% | **14%** |
| Full 16x16 room `map()` redraw, dense art | 126% - did not fit | **63% - fits** |
| Complete Celeste-class tick (cls + room + 30 sprites + 24 particles + HUD) | ~123% - did not fit | **61% + 14% blit = 75% - fits** |
| Aligned 8x8 sprites after `cls` | ~70-95 | **~180** |
| Input latency | up to ~17 ms | up to ~33 ms |

The last row is the honest trade-off: one extra display frame of input
latency, the same cost PICO-8 itself and most shipped GBA titles accepted.
Everything else is a doubling. The panel still scans at 59.73 Hz; each
image is simply shown for exactly two scans, paced by a hardware timer so
the cadence cannot alternate (alternation, not the rate, is what reads as
jitter).

## Measured costs per call

| Operation | Cycles | % tick (30 Hz) | Notes |
| --- | ---: | ---: | --- |
| `cls(c)` | 67,542 | 12% | DMA3 fill; cost is fixed |
| `rectfill` full screen | 67,723 | 12% | full-width spans use the same DMA fill |
| `rectfill` 32x32 | ~2,500 | <1% | word-store span loop |
| `map` 30x20, 50% transparent art | 832,381 | 148% | still cannot run every tick |
| `map` 30x20, fully opaque art | 763,901 | 136% | packed words help but cells dominate |
| `spr` 8x8 x100, aligned | 207,698 | 37% | ~2,077 per sprite |
| `spr` 8x8 x100, odd x | 225,874 | 40% | ~2,259 per sprite |
| `spr` 8x8 x100, flipped | 296,186 | 53% | ~2,962 per sprite |
| `sspr` 8x8 -> 16x16 x10 | 136,882 | 24% | ~13,700 per stretch |
| `print` 40-char row | 36,138 | 6% | ~900 per glyph |
| `line` full diagonal | 44,903 | 8% | ~187 per pixel |
| `circfill` r=16 | 22,445 | 4% | span-based |
| `sin`+`cos` x100 | 34,291 | 6% | ~343 per pair, LUT |
| framebuffer DMA blit | 77,093 | 14% | paid every tick |
| `map` 16x16 dense | 354,046 | 63% | a full PICO-8 room, worst-case art |
| `map` 16x16 half-empty | 184,446 | 33% | realistic platformer density |
| `map` 16x16 with layer mask | 101,054 | 18% | masked passes reject cells cheaply |
| `map` 30x16 half-empty | 344,718 | 61% | wide scroller band |
| **complete PICO-8 tick** | **344,785** | **61%** | cls + 16x16 room + 30 sprites + 24 particles + HUD |
| **complete GBA tick** | **352,077** | **62%** | cls + 17% 30x20 room + 1 sprite + HUD ([`mapdemo`](../examples/mapdemo)) |

The last row is the go/no-go proof for PICO-8-class ports: a full game tick
- clear, whole-room redraw, 30 sprites (some flipped), 24 particle
rectfills, HUD text - plus the 14% blit totals ~75% of the tick, leaving
~140k cycles for game logic. Lua value operations cost tens of cycles, so
logic is effectively never the bottleneck; pixels are.

### What these numbers mean in practice

- **A PICO-8-sized room (16x16 cells) redraws fully every tick** - even
  with worst-case dense art (63%). This is the workload that makes
  room-based platformer ports straightforward: no dirty-rectangle
  bookkeeping needed.
- **Map-cell cost hierarchy** (each measured): an *empty* cell (sprite 0)
  is ~free - `map()` skips it before any pixel work, so a half-empty room
  costs half a dense one. A *fully opaque* tile is the fast path (packed
  32-bit copies). A *partially transparent* tile costs ~9% more (per-byte
  fallback around its color-0 holes). Level-art rules that follow: never
  fill sky/holes with opaque "background-colored" tiles - leave them
  empty and let `cls()` provide the backdrop for free. For drawn tiles,
  baking the backdrop into the art keeps the fast path, but only when the
  backdrop is actually known (a flat sky); tiles that sit over varying
  content need their transparency, and its ~9% premium is a fair price -
  don't contort art to dodge it.
- **A dense 30x20 full-GBA-screen `map()` every tick still does not fit**
  (149%). Workarounds, in preference order: sparse platformer rooms fit
  (62% for a full tick at 17% cell density - see [`mapdemo`](../examples/mapdemo));
  half-empty maps fit (61%); masked layer passes fit (18% per pass); or draw
  once per room and per tick patch only the strip a camera scroll exposed
  (~90k cycles per 64-cell patch).
- **Sprite budget by scenario** (8x8 `spr`, blit already subtracted;
  "realistic" leaves ~20% headroom for logic, particles, and HUD):

  | Frame also does... | Max aligned | Realistic |
  | --- | ---: | ---: |
  | sprites only | 233 | ~190 |
  | `cls` + sprites | 200 | ~160 |
  | `cls` + sprites, all odd-x | 184 | ~145 |
  | `cls` + sprites, all flipped | 140 | ~110 |
  | `cls` + half-empty 16x16 `map` + sprites | 112 | ~90 |
  | `cls` + dense 16x16 `map` + sprites | 30 | ~25 |

  Rule of thumb: **~200 sprites bare, ~90-110 in a real game that also
  redraws its room**. A PICO-8-class game (15-25 live objects) never
  approaches this wall. 16x16 sprites cost ~4x an 8x8, so ~50 of them
  after `cls`. Offscreen sprites reject before pixel work and are
  near-free, so drawing the whole entity list without camera culling is
  fine.
- Prefer even x positions (the packed fast path needs 4-byte alignment);
  prefer unflipped art (pre-flip in the sheet when a sprite is always
  mirrored) - flipped sprites cost ~43% more.
- `sspr` stretching costs ~6.5x an equivalent `spr`. A few per tick are
  fine; scaling every entity every tick is not.

## How the runtime spends memory

| Region | Size | Speed | What lives there |
| --- | --- | --- | --- |
| IWRAM | 32 KB | 32-bit, zero-wait | stacks (9 KB: 8 KB user + 1 KB IRQ), blit/print/span cores (~5 KB), TinyGo runtime data |
| EWRAM | 256 KB | 16-bit, 2-wait | `Context` (framebuffer, palettes), mutable sheet/map/flag RAM, 2x56 KB Lua heap |
| VRAM | 96 KB | 16-bit | two Mode 4 pages |
| ROM | up to 32 MB | 16-bit + prefetch | code, assets, LUTs |

Sprite sheets are cell-compacted by default: when static analysis finds no
`sset()` call, generated ROM and EWRAM contain only non-empty 8x8 cells plus a
two-byte logical-ID index. Pixel drawing still reads EWRAM—the direct-ROM
prototype measured 100 sprites at 223,509 cycles versus 207,699 from EWRAM
and made a dense map exceed the tick budget. The final compact-EWRAM path
measured 100 sprites at 211,107 cycles (about 1.6% over the old dense layout)
and preserved the mapdemo-class full-tick result within measurement noise. Calling `sset()` anywhere opts
the project into a complete mutable EWRAM sheet. Flags remain mutable in
EWRAM because `fset()` is cheap at one byte per sprite.

The single most important architectural fact: **ARM code fetched from ROM
pays two bus accesses per instruction; the same code in IWRAM pays one
zero-wait access.** Moving the pixel cores to IWRAM (`//go:section .iwram`)
more than doubled sprite and print throughput without changing a line of
logic. IWRAM is the scarcest resource; only proven hot loops earn a place
there, and the linker map (`make benchspike-elf`, then check `.data` size)
must stay well under 32 KB.

Projects can opt measured hot Lua helpers into IWRAM with `iwramFunctions`
in `project.json`, for example `"iwramFunctions": ["draw_particles"]`.
Names must identify plain top-level functions from the resolved Lua source;
duplicates, unknown names, methods and the `_init`, `_update`, `_draw`
callbacks are rejected. The setting is optional, works with either renderer,
and survives editor saves. It changes code placement only: selected functions
receive `//go:noinline` and `//go:section .iwram`; their arguments, return
values and Lua behavior stay the same. Other functions retain normal placement.

The available IWRAM is shared with runtime code, data, the 8 KiB user stack,
the 1 KiB IRQ stack, and 96 reserved bytes. A helper's callees are not relocated
automatically. Check the build's linker-measured IWRAM usage and compare real
GBA cycle counts before keeping a selection; avoiding inlining can also cost
performance. The linker rejects an overflowing selection rather than silently
moving functions back to ROM. Remove names from the array to restore ordinary
placement; omitting the array preserves existing projects' behavior.

## Runtime design rules (enforced, not aspirational)

1. **Zero Go-heap allocations after Init.** `make smoke-gba` fails the
   build if TinyGo reports any runtime allocation. Lua tables and strings
   live in a fixed 56 KB semispace pair with a Cheney collector.
2. **No divides in per-pixel or per-cell loops.** ARM7TDMI has no divide
   instruction; a software divide costs hundreds of cycles. `spr`/`map`
   convert sprite ids to cells with a fixed-point reciprocal multiply.
3. **Word writes wherever alignment allows.** The framebuffer fill, packed
   sprite rows, and the VRAM blit all move 32 bits per store. VRAM itself
   cannot take 8-bit writes at all, which is why presentation is DMA of
   whole words.
4. **Fixed costs go to hardware.** `cls` and full-width `rectfill` are DMA3
   fills; the frame blit is DMA3 with the whole VDraw window to run in.
5. **Bounds checks stay out of pixel loops.** The blit cores take raw
   pointers after their callers establish bounds from the cell clamps. The
   clamps are tested; the cores are covered by the golden-framebuffer test.

## Good practice / bad practice for game code

**Do:**

- **Call `cls()` every tick without guilt.** It is a single fixed-cost
  DMA3 hardware fill: 67,542 cycles, 12% of the tick, no matter what was
  on screen. It is not a 38,400-pixel software loop. The framebuffer is
  persistent (nothing implicitly clears it), so the real choice is
  `cls` + redraw everything (simple, 12%) versus no `cls` + patch only
  what changed (fastest, but you own the bookkeeping). Start with `cls`;
  drop it only if the profiler says you must.
- Keep per-frame table work on the array part (`t[i]` with `i` in
  1..cap). Array access is a direct indexed read; hash access costs a
  probe sequence with 12-byte compares.
- Static field syntax (`object.x`) is monomorphic by default: its first
  execution validates the table and resolves the hash slot, then that access
  site assumes the value remains a table with the same field layout. This
  removes repeated type/hash/key checks from record-like game state. Replacing
  the value with another type, adding/removing fields in a way that changes its
  layout, or using different record shapes at the same function access site is
  unsupported; use indexed access (`object["x"]`) when the shape is dynamic.
- A chunk-scope, fixed-shape numeric record is scalar-replaced when the whole
  program proves that its identity and table layout are unobservable. Every
  field then becomes one direct `rt.Number` member: no table allocation,
  static field key, hash slot, or field cache is linked. Aliasing the record,
  using bracket indexing, rebinding it, adding a field, storing an explicit
  non-number, or shadowing its name automatically retains the ordinary table
  path. Debug builds expose scalarized fields as `record.field` watch entries.
  In a 64-iteration target workload, the conservative scalar path measured
  259,324 -> 43,915 median Mode 0 cycles (-83.07%) and 41,352 -> 26,212 ROM
  bytes (-36.61%), with zero differing pixels; the real first-room Celeste
  frame measured 45,224 -> 44,705 cycles and 44,780 -> 30,076 debug-ROM bytes.
- Retained maps at or below the 16,384-cell threshold select a compile-time
  compact runtime in both renderers, so their ROM cannot retain the chunk
  loader, streaming lookup, or sparse-edit table. On the first-room Celeste
  cartridge this changed the release ROM from 27,708 to 24,616 bytes (-11.2%)
  and the Mode 0 debug benchmark from 44,705 to 44,521 median cycles, with zero
  differing pixels or missed frames. Streaming maps use the fixed streaming
  path in Mode 0 (36,333 -> 28,390 cycles in the paired map-only target
  workload). Mode 4 deliberately retains its generic streaming path: forcing
  the same specialization measured 397,312 -> 411,340 cycles, so that losing
  variant is not selected.
- On a compact map, `mget`/`mset` with either their implicit BG0 form or a
  literal/proven-constant layer compile directly to that layer's dense storage
  slot. Dynamic layers and every streaming map retain the full logical-layer
  lookup. Four collision rectangles per tick measured 22,248 -> 21,720 Mode 0
  cycles and 662,505 -> 662,021 Mode 4 cycles; ROM changed by -40 and -168
  bytes respectively, with identical draw/commit costs, EWRAM, missed frames,
  and pixels.
- A cartridge with no `sset()` specializes its renderer at link time. Mode 4
  uses the compact immutable-sheet representation and removes its storage
  branch from every `spr`, `sspr`, and `map` sample; the collision fixture
  measured 711,140 -> 662,021 cycles (-6.91%), draw 629,936 -> 580,813, and
  debug ROM 24,408 -> 22,596 bytes. Mode 0 deliberately does not use that
  representation because its earlier steady-state candidate regressed by 66
  cycles. Instead, a narrower Mode 0 tag preserves the faster sheet layout and
  deletes only the impossible per-frame dirty-tile scan. That changed the same
  fixture from 47,668 -> 21,720 cycles (-54.43%), commit 41,425 -> 15,451, cold
  initialization 3,399,010 -> 3,348,082 cycles, and ROM 23,040 -> 22,544 bytes.
  Every pair had zero differing pixels, zero new missed frames, and unchanged
  EWRAM.
- The periodic Lua collector and its two 56 KiB target semispaces are omitted
  when the whole program is proven allocation-free after numeric-record scalar
  replacement. Any ordinary/aliased table, dynamic string operation, boxed
  global, uncertain call, or disabled proof keeps the original collector and
  memory unchanged. On the first-room Celeste cartridge this changed the
  release ROM from 23,036 to 21,280 bytes, debug ROM from 25,212 to 23,516,
  and free EWRAM from 96,240 to 210,928 bytes; median/p95 stayed non-regressive
  at 43,751 -> 43,747 cycles with zero differing pixels or missed frames. A
  heap-free target also replaces the otherwise unreachable handle table, scan
  queue, and root stacks with compile-only sentinel slots. On the constant
  collision workload this returned another 28,380 EWRAM bytes in both
  renderers with bit-identical frames and exactly unchanged total/draw/commit
  cycles. At that stage the release Celeste cart had 239,324 bytes free EWRAM
  and remained 21,188 ROM bytes. Allocating carts retain the full metadata and
  both semispaces.
- Audio control builtins (`sfx`, `music`, `sample`, `gba_note`, and
  `gba_stop`) participate in that same no-allocation proof: they may change
  audio state, but cannot allocate a Lua table or string. In an otherwise
  heapless cart calling `sfx` and `music` every tick, the Mode 0 median changed
  from 2,912 to 2,883 cycles, ROM from 34,476 to 32,576 bytes, and free EWRAM
  from 108,976 to 252,040 bytes. Mode 4 changed from 149,032 to 148,952 cycles,
  ROM from 33,988 to 32,100 bytes, and free EWRAM from 70,848 to 213,920 bytes.
  Both renderer pairs had zero differing pixels and zero missed frames.
- `print` stays conservatively allocating for a dynamic/non-string first
  argument because `PrintV` may call `tostr`. When that argument is a literal
  string, however, it is already interned and generated code uses the
  literal-only `PrintStaticV` call graph. That graph cannot reach `tostr`, the
  dynamic-string decoder, either 56 KiB semispace, or the root-stack metadata.
  The literal-text fixture measured 41,084 -> 36,568 Mode 0 cycles (-10.99%),
  19,596 -> 16,676 ROM bytes, and 143,068 more free EWRAM. Mode 4 measured
  164,215 -> 159,746 cycles (-2.72%), 26,136 -> 17,840 ROM bytes, and the same
  EWRAM gain. Both pairs had zero differing pixels and zero missed frames. A
  real TinyGo link test rejects any heap symbols or a Mode 0 `Context` at or
  above 16 KiB; this caught an intermediate implementation whose generic
  string decoder retained the heap even though source-level effect analysis
  correctly reported the cart as heapless.
- First-observed numeric globals, locals, numeric-for counters, arithmetic
  expressions, and numeric record fields use native 16:16 `rt.Number` storage
  and operations. Reassigning one of those bindings or fields to a different
  Lua type is unsupported. Keep genuinely polymorphic values behind bracket
  indexing or separate bindings. The paired 16-body physics fixture in
  [`examples/physics-stress`](../examples/physics-stress) measured a 32.10%
  reduction in update/overhead cycles with zero differing pixels across 600
  Mode 4 frames.
- A chunk-scope numeric binding initialized once from a literal or an earlier
  proven constant is emitted as an exact 16:16 compile-time constant. Any
  second write, pre-declaration read, shadowing, multi-assignment, computed
  initializer, or declaration after the first function retains ordinary
  storage. The paired first-room Celeste collision workload performs four
  `solid_rect` queries per tick. Constant propagation changed Mode 0 from
  22,792 to 21,720 median cycles and 22,824 to 22,544 debug-ROM bytes; Mode 4
  changed from 662,765 to 662,021 cycles. Both returned 16 more EWRAM bytes,
  produced zero differing pixels, and retained the same missed-frame counts.
  The real Mode 0 release cartridge changed from 21,280 to 21,188 ROM bytes.
- The transpiler counts unique interned static strings exactly and selects the
  smallest safe 1/8/32/128/256-entry `Context` tier. The public 256-string
  source limit and overflow behavior are unchanged. The string-free Celeste
  collision workload returned 4,080 EWRAM bytes in both renderers while ROM,
  total/draw/commit cycles, missed frames, and pixels remained exactly
  unchanged. The one-entry tier is a sentinel for the zero-string case. After
  this tier, fixed map-slot lowering, and immutable-sheet upload elision, the
  collision-only Celeste benchmark cartridge at that measurement was 20,612
  ROM bytes with 243,404 bytes of EWRAM free. This is not the later full port
  with player, objects, effects and music.
- Fixed, non-escaping global numeric array literals use compact 4-byte Number
  elements instead of 12-byte Value slots. Direct indexed reads/writes and
  `#array` stay native; an escape, shadow, hash field, or explicit non-number
  assignment keeps the generic table. The 64-particle, 512-access/tick
  diagnostic measured 3,253,949 -> 542,498 median cycles (-83.33%), 3,200 ->
  1,040 bytes of Lua heap allocation (-67.50%), and 63,940 -> 39,364 ROM bytes
  (-38.44%), with unchanged linker EWRAM and zero differing pixels across 600
  Mode 4 frames. Element types are permanent under the same monomorphic
  contract as numeric bindings.
- Direct user functions receive native `rt.Number` parameters only when every
  static call site supplies a proven number. Return positions become native
  independently when every control-flow path supplies a number in that slot;
  mixed, nil-padded, falling-through, or unknown positions stay as `rt.Value`.
  The paired 12-body,
  ~280-call/tick fixture in
  [`examples/function-physics-stress`](../examples/function-physics-stress)
  measured 985,948 -> 740,131 median cycles (-24.93%) and a 36.27% reduction
  in update/overhead residual, with zero differing pixels across 600 Mode 4
  frames. This deliberately over-budget workload amplifies call overhead; the
  percentage is not a promise for ordinary games.
- Multi-return specialization preserves Lua expansion while allowing mixed Go
  tuples such as `(rt.Number, rt.Value, rt.Number)`. In the dedicated 240-call
  fixture at
  [`examples/multi-return-physics-stress`](../examples/multi-return-physics-stress),
  native tuple results measured 2,206,075 -> 1,169,327 shipping-path cycles
  (-47.00%), 1,927,649 -> 896,819 update/overhead cycles (-53.48%), and 50,688
  -> 50,532 ROM bytes (-0.31%). With the same root-protected path in both
  builds, they measured 2,438,893 -> 1,363,142 cycles (-44.11%), 2,163,634 ->
  1,082,622 update/overhead cycles (-49.96%), and 57,148 -> 56,716 ROM bytes
  (-0.76%). Both comparisons had zero differing pixels across 600 Mode 4
  frames and unchanged EWRAM.
- Calls to transitively proven non-allocating functions bypass the generated
  temporary-root push/pop wrapper. Allocation through table creation/growth,
  string creation/concatenation, an unknown builtin, or an unproven recursive
  cycle keeps the protected path. A dedicated workload with one live table
  root and 384 pure numeric calls per tick in
  [`examples/noalloc-call-stress`](../examples/noalloc-call-stress) measured
  330,184 -> 239,432 median cycles (-27.49%) and 106,632 -> 15,774
  update/overhead cycles (-85.21%), with zero differing pixels across 600 Mode
  4 frames. Its one cold-start missed tick fell to zero. This fixture isolates
  wrapper overhead and deliberately overrepresents tiny helper calls.
- Table deletion and inspection operations (`del`, `deli`, `count`, and the
  `all`/`pairs` iterator tokens) are included in the same effect proof. Unlike
  `add`, a field/index store, or `foreach`, these operations cannot grow a
  table or invoke an allocating callback. A 128-pair-per-tick `deli`/`count`
  fixture measured 530,084 -> 472,461 Mode 0 cycles (-10.87%) and 27,948 ->
  26,796 ROM bytes; Mode 4 measured 688,174 -> 632,906 cycles (-8.03%) and
  27,976 -> 26,688 ROM bytes. EWRAM was unchanged and both pairs produced zero
  differing pixels. Mode 0 retained zero missed frames in both runs.
- Reuse tables. `local pool = {}` at `_init`, indices recycled, beats
  `add`/`del` churn: `#t` and `add` both walk the array border, and the
  collector's pause grows with live data.
- Use `flr()` results cached in locals when the same coordinate feeds
  several draws; every Lua arithmetic op is a function call at -opt=2.
- Profile on hardware early with `SetProfiling` + `PerformanceStats`;
  `MaxUpdate`/`MaxDraw` catch the worst frame, not the average.

**Avoid:**

- Repeated full-screen per-cell map rendering. A qualifying static Mode 4
  plane now uses a raster cache; measure the complete frame and its fallbacks
  before assuming it fits. See the Celeste measurements linked above.
- `sspr` scaling on many entities per frame.
- Flipping sprites at draw time when the art could carry both facings.
- String building in `_update`/`_draw`. Concat allocates from the Lua
  heap every call and drives collector pauses; `print` of a static string
  allocates nothing.
- Deep `del(t, value)` on long arrays: it is a linear search plus a
  shift. `deli(t, i)` with a known index halves that; swap-with-last and
  pop is O(1).

## Known limits (clear expectations)

| Limit | Value | Where enforced |
| --- | --- | --- |
| Update rate | 30 Hz (two 59.73 Hz scans per tick) | generated loop |
| Sprites on screen per tick (cls + draw) | ~180 8x8 | budget math above |
| Full-screen map redraws per tick | cache/renderer dependent; measure the full frame | map raster eligibility and target timing |
| Lua heap | 56 KB live data when allocation is reachable; collector and both 56 KiB semispaces are link-elided after a whole-program no-allocation proof | `runtime/heap.go`, transpiler effect analysis |
| Table array part | 256 entries | `maxArrayCap` |
| Table hash part | 128 entries | `maxHashCap` |
| String length | 4,096 bytes | `maxStringBytes` |
| Interned static strings | 256 | `maxStaticStrings` |
| Locals rooted across calls | 512 | `maxLocals` |
| Map streaming threshold | >16,384 retained plane-cells streams from ROM; compact storage is link-time specialized in both renderers, streaming storage in Mode 0 only | `mapcache.go`, build tags |
| Streaming map edits (`mset`) | 3,072 distinct cells | `MapEditFillLimit` |
| EWRAM for assets + heap + Context | 256 KB total | linker + `ValidateHeapBudget` |
| Continuous positive world coordinate | 32,767 px (4,095 whole 8px tiles) | signed 16:16 `Number` |
| Cartridge ROM | 32 MiB total | GBA address space + `ValidateROM` |

When a limit fails, it fails closed and observably: heap exhaustion returns
`HeapOOM` (nil result in Lua) and preview builds show `HEAP OOM` in the editor,
map-edit exhaustion sets `LastMapError`, and the build refuses ROMs whose
static assets cannot fit EWRAM.

## Measuring your own game

```lua
-- game.lua: watch the real per-phase costs on screen
function _draw()
  cls(0)
  -- ... your drawing ...
end
```

Build with profiling by calling `SetProfiling(true)` from generated code
(exposed to Lua as a debug builtin in a future release), or run the ROM in
mGBA and read `PerformanceStats` fields with the debugger. `MissedFrames`
counts frames whose update+draw exceeded a full display interval - the
number that must stay at zero before shipping on hardware.
